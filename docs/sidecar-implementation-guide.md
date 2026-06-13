# Control Sidecar 设计与实现指南

> 目标读者：需要从零实现、重构或增强 Filebeat-Ops control sidecar 的工程师。
>
> 最后更新：2026-06-11

---

## 1. 目标

control sidecar 是运行在 Filebeat DaemonSet Pod 中的节点侧控制进程。它的目标不是替代 Filebeat，也不是在节点上做策略决策，而是把控制面已经计算好的期望配置安全、稳定、可观测地落到本节点，让 Filebeat 通过 `inputs.d` 热加载配置。

一个合格的 sidecar 至少要做到：

1. 能唯一标识当前节点上的 Agent。
2. 能向 control-server 注册并周期性上报心跳。
3. 能以 poll 或 watch 模式拉取期望配置。
4. 能校验配置完整性并原子写入 Filebeat 动态配置目录。
5. 能清理已经被控制面删除的旧配置文件。
6. 能上报配置应用成功或失败的结果。
7. 能维护统一的工作负载日志视图：`container_file` 使用容器 rootfs 视图，`container_stdio` 使用控制器维度的标准输出视图。
8. 在控制面、Kubernetes API、容器运行时或本地文件系统部分异常时，具备清晰的降级行为。

更好的 sidecar 还应该具备：

1. 清晰的模块边界，方便单元测试和替换实现。
2. 指标、结构化日志和健康状态暴露。
3. 退避重试、优雅退出和可控资源占用。
4. 更严格的安全边界和更少的宿主机权限。
5. 可扩展的容器运行时适配能力。

---

## 2. 系统位置

```text
Control Server
  ├─ Policy CRUD
  ├─ Policy Revision
  ├─ Config Rendering
  └─ Agent APIs
       ▲
       │ HTTP + X-Agent-Token
       ▼
DaemonSet Pod on each Kubernetes node
  ├─ control-sidecar
  │    ├─ register / heartbeat
  │    ├─ pull desired configs
  │    ├─ write inputs.d/fbctl-*.yml
  │    └─ maintain /var/log/klog and /var/log/klog-stdio symlinks
  │
  └─ filebeat
       └─ reload inputs.d/*.yml
```

control sidecar 与 Filebeat 的关系是文件协议关系：

- sidecar 写入 Filebeat 默认的 `inputs.d/*.yml` 外部 input 配置目录
- Filebeat 读取并热加载 `inputs.d/*.yml`
- sidecar 不直接调用 Filebeat API
- Filebeat 不直接感知 control-server

`inputs.d/*.yml` 是 Filebeat 配置中的相对路径，实际目录应位于 Filebeat 的 `path.config/inputs.d`。使用容器镜像时，通常把共享卷同时挂载到 Filebeat 容器和 sidecar 容器中的同一个 `path.config/inputs.d` 位置。

---

## 3. 职责边界

### 3.1 sidecar 负责什么

| 职责 | 说明 |
|---|---|
| Agent 身份 | 根据 `CLUSTER_ID` 和 `NODE_NAME` 生成 `agent_id = cluster_id:node_name` |
| 节点信息采集 | 读取节点名、Pod 名、命名空间、sidecar 版本、Filebeat 版本、节点标签 |
| 注册 | 调用 `POST /api/v1/agent/register` |
| 心跳 | 调用 `POST /api/v1/agent/heartbeat` |
| 配置拉取 | 调用 `GET /api/v1/agent/config` 或 `GET /api/v1/agent/watch` |
| 配置落盘 | 校验 checksum，写入 `fbctl-*.yml`，清理孤儿配置 |
| 应用结果上报 | 调用 `POST /api/v1/agent/apply-result` |
| 工作负载日志视图维护 | 维护 `container_file` rootfs 视图和 `container_stdio` 控制器视图 |

### 3.2 sidecar 不负责什么

| 非职责 | 应由谁负责 |
|---|---|
| 创建、修改、删除采集策略 | Grafana 插件 / control-server / CRD operator |
| 渲染 Filebeat YAML | control-server |
| 决定某条策略是否匹配某个节点 | control-server，根据 Agent 注册的 node labels 过滤 |
| 管理 Filebeat 输出目标 | Filebeat 基础配置 |
| 存储策略版本和审计 | control-server 数据库 |
| 保证日志最终送达 | Filebeat 和下游日志系统 |

这个边界很重要。sidecar 要尽量轻，越少参与业务策略解释，越容易在节点上稳定运行。

---

## 4. 推荐模块结构

现有实现集中在一个 `main.go` 中，便于原型推进。正式应用建议拆成以下模块：

```text
sidecar/
  cmd/control-sidecar/
    main.go                    # 只负责启动、组装依赖、处理退出信号

  internal/sidecar/
    app/
      runner.go                # 主循环：注册、心跳、拉配置、应用、上报
    config/
      config.go                # 环境变量读取、默认值、校验
    agent/
      identity.go              # agent_id、节点标签发现、版本信息
    client/
      client.go                # control-server HTTP client
      types.go                 # API request / response DTO
    apply/
      applier.go               # checksum、原子写入、孤儿清理
      filename.go              # managed filename 校验
    symlink/
      manager.go               # Informer、事件处理、reconcile
      runtime.go               # containerd / k3s / CRI 运行时发现
      rootfs.go                # /hostproc/<pid>/root 解析策略
      stdio.go                 # /var/log/containers 标准输出控制器视图
    observability/
      metrics.go               # Prometheus 指标
      log.go                   # 结构化日志封装
```

模块依赖方向建议如下：

```text
cmd
  → app
      → config
      → agent
      → client
      → apply
      → symlink
      → observability
```

`apply` 和 `symlink` 应尽量不依赖 HTTP client，便于独立测试。

---

## 5. API 契约

所有 Agent 接口都应携带：

```http
X-Agent-Token: <agent token>
```

### 5.1 注册 Agent

```http
POST /api/v1/agent/register
Content-Type: application/json
```

请求：

```json
{
  "id": "dev:node-a",
  "cluster_id": "dev",
  "node_name": "node-a",
  "pod_name": "filebeat-agent-xxxxx",
  "namespace": "filebeat-ops",
  "agent_version": "1.0.0",
  "filebeat_version": "9.4.1",
  "current_config_checksum": "",
  "node_labels": {
    "nodepool": "online",
    "zone": "hk-1"
  }
}
```

响应：

```json
{
  "id": "dev:node-a"
}
```

实现要求：

- 如果 `id` 为空，服务端可以用 `cluster_id:node_name` 补齐。
- sidecar 启动后必须先注册，再拉配置。
- 节点标签应在注册时上报，服务端后续根据 `agent_id` 读取已注册的节点标签来过滤策略。

### 5.2 心跳

```http
POST /api/v1/agent/heartbeat
Content-Type: application/json
```

请求：

```json
{
  "id": "dev:node-a",
  "cluster_id": "dev",
  "node_name": "node-a",
  "current_config_checksum": "abc123"
}
```

响应：

```json
{
  "status": "ok"
}
```

实现要求：

- 心跳失败不能导致 sidecar 退出。
- 心跳中应带上当前本地已成功应用的 checksum。
- 如果配置应用失败，`current_config_checksum` 不应更新为失败配置的 checksum。

### 5.3 Poll 拉配置

```http
GET /api/v1/agent/config?agent_id=dev%3Anode-a&checksum=abc123
```

响应，无变化：

```json
{
  "changed": false,
  "checksum": "abc123"
}
```

响应，有变化：

```json
{
  "changed": true,
  "checksum": "new-checksum",
  "files": [
    {
      "filename": "fbctl-100-payment-app.yml",
      "content": "- type: filestream\n  id: \"payment-app-r1\"\n"
    }
  ]
}
```

实现要求：

- sidecar 应优先使用 `agent_id` 请求配置。
- 如果不用 `agent_id`，至少必须传 `cluster_id`，但这样服务端无法做节点标签过滤。
- sidecar 不应解析策略，只处理服务端返回的文件集合。

### 5.4 Watch 长轮询

```http
GET /api/v1/agent/watch?agent_id=dev%3Anode-a&checksum=abc123&timeout=25s
```

响应格式与 poll 一致。

实现要求：

- watch HTTP client 超时时间应大于请求中的 `timeout`，例如 `timeout + 10s`。
- watch 失败时应降级到 poll。
- watch 超时但无变化时，服务端返回 `changed: false`，sidecar 正常进入下一轮。

### 5.5 上报应用结果

```http
POST /api/v1/agent/apply-result
Content-Type: application/json
```

请求：

```json
{
  "agent_id": "dev:node-a",
  "checksum": "new-checksum",
  "status": "success",
  "message": "applied"
}
```

失败示例：

```json
{
  "agent_id": "dev:node-a",
  "checksum": "new-checksum",
  "status": "failed",
  "message": "checksum mismatch: server=xxx local=yyy"
}
```

实现要求：

- 配置应用成功或失败都要上报。
- 上报失败只记录日志，不应导致主循环退出。

---

## 6. 配置项

### 6.1 必需配置

| 环境变量 | 示例 | 说明 |
|---|---|---|
| `CONTROL_SERVER_URL` | `http://filebeat-control-server:8080` | control-server 地址 |
| `AGENT_TOKEN` | 从 Secret 注入 | Agent 接口认证 token |
| `CLUSTER_ID` | `dev` | 集群 ID |
| `NODE_NAME` | Downward API `spec.nodeName` | 当前节点名 |
| `INPUTS_DIR` | `${path.config}/inputs.d` | Filebeat 默认外部 input 配置目录；实际挂载位置应与 Filebeat `path: inputs.d/*.yml` 指向的目录一致 |

### 6.2 推荐配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `POD_NAME` | `local-sidecar` | sidecar 所在 Pod 名 |
| `POD_NAMESPACE` | `default` | sidecar 所在命名空间 |
| `NODE_LABELS` | 空 | 手工覆盖节点标签，支持 `k=v,k2=v2` 或 JSON |
| `CONFIG_MODE` | `poll` | `poll` 或 `watch` |
| `WATCH_ENABLED` | `false` | 为 `true` 时强制启用 watch |
| `WATCH_TIMEOUT` | `25s` | 单次 watch 请求超时 |
| `POLL_INTERVAL` | `30s` | poll 模式主循环间隔 |
| `RUN_ONCE` | `false` | 测试用，运行一轮后退出 |
| `AGENT_VERSION` | `dev` | sidecar 版本 |
| `FILEBEAT_VERSION` | `unknown` | Filebeat 版本 |

### 6.3 容器文件日志相关配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `KLOG_DIR` | `/var/log/klog` | sidecar 维护的 `container_file` rootfs 符号链接根目录 |
| `KLOG_STDIO_DIR` | `/var/log/klog-stdio` | sidecar 维护的 `container_stdio` 控制器视图根目录 |
| `HOSTFS_DIR` | `/hostfs` | 宿主机 `/` 的挂载点 |
| `HOSTPROC_DIR` | `/hostproc` | 宿主机 `/proc` 的挂载点 |
| `CONTAINERD_STATE_DIR` | 空 | containerd state 目录，空时尝试默认路径 |
| `RECONCILE_INTERVAL` | `60s` | symlink-manager 定期校准间隔 |

---

## 7. 主流程

### 7.1 启动流程

```text
read config
  ↓
validate required config
  ↓
discover node labels
  ├─ if NODE_LABELS is set: parse NODE_LABELS
  └─ else: call Kubernetes Node API
  ↓
detect container runtime
  ↓
create KLOG_DIR
  ↓
start symlink-manager goroutine
  ↓
create control-server client
  ↓
register agent
  ↓
enter main loop
```

### 7.2 主循环

伪代码：

```go
currentChecksum := ""

for {
    heartbeat(agentID, currentChecksum)

    resp, err := pullConfig(currentChecksum)
    if err != nil {
        log error
        sleep(loopSleep)
        continue
    }

    if resp.Changed {
        err := applyConfig(inputsDir, resp)
        if err != nil {
            reportApplyResult(agentID, resp.Checksum, "failed", err.Error())
        } else {
            currentChecksum = resp.Checksum
            reportApplyResult(agentID, resp.Checksum, "success", "applied")
        }
    }

    if runOnce {
        return
    }

    sleep(loopSleep)
}
```

注意事项：

- `currentChecksum` 表示本地已成功应用的配置集合。
- pull 请求中的 checksum 应使用 `currentChecksum`。
- apply 失败时不能更新 `currentChecksum`。
- 如果进程重启，当前实现会从空 checksum 开始；更好的实现可以从本地 manifest 恢复上次成功 checksum。

---

## 8. 配置应用算法

### 8.1 输入

```go
type ConfigFile struct {
    Filename string
    Content  string
}

type DesiredConfigResponse struct {
    Changed  bool
    Checksum string
    Files    []ConfigFile
}
```

### 8.2 文件名规则

只允许 sidecar 管理以下文件：

```text
fbctl-*.yml
```

必须拒绝：

- 包含 `/` 或 `\` 的文件名
- `../xxx.yml`
- 绝对路径
- 非 `fbctl-` 前缀
- 非 `.yml` 后缀，推荐正式实现补上该限制

当前最小校验：

```go
strings.HasPrefix(filename, "fbctl-") &&
filepath.Base(filename) == filename
```

正式实现推荐：

```text
^fbctl-[0-9]{3,5}-[a-zA-Z0-9._-]+\.yml$
```

### 8.3 Checksum 校验

sidecar 必须本地重新计算配置集合 checksum：

```text
localChecksum = ConfigSetChecksum(files)
if localChecksum != response.Checksum:
    fail apply
```

checksum 的要求：

- 对文件内容敏感。
- 对文件名敏感。
- 对文件顺序不敏感。
- 使用稳定排序后再计算。

### 8.4 原子写入

流程：

```text
for each file in response.files:
  validate filename
  tmp = inputsDir + "/." + filename + ".tmp"
  final = inputsDir + "/" + filename
  write tmp
  rename tmp to final
```

要求：

- `inputsDir` 不存在时创建。
- 使用 `rename` 替换，避免 Filebeat 读到半写文件。
- 文件权限建议 `0644`。
- 生产增强建议写入后 `fsync(tmp)`，rename 后 `fsync(inputsDir)`。

### 8.5 孤儿清理

写入新文件后：

```text
keep = filenames from response.files
for each file matching inputsDir/fbctl-*.yml:
    if file not in keep:
        remove file
```

作用：

- 策略被删除后，本节点对应配置也会删除。
- 节点标签变化导致策略不再匹配时，旧配置会被清理。

注意：

- 只清理 `fbctl-*.yml`，避免误删用户或 Filebeat 自己的配置。
- 如果删除失败，应上报 apply failed。

### 8.6 更好的本地状态文件

当前实现只靠内存中的 `currentChecksum`。正式实现建议增加本地状态文件：

```text
inputs.d/.fbctl-state.json
```

示例：

```json
{
  "checksum": "abc123",
  "applied_at": "2026-06-11T10:00:00Z",
  "files": [
    {
      "filename": "fbctl-100-payment-app.yml",
      "sha256": "file-content-sha256"
    }
  ]
}
```

好处：

- sidecar 重启后可以恢复当前 checksum。
- 可以发现本地文件被人为修改。
- 可以在上报心跳时更准确地报告本地状态。

---

## 9. Symlink Manager 设计

Symlink Manager 负责维护工作负载日志视图，让 Filebeat 可以按 `namespace / controllerType / controllerName / pod / container` 这套用户感知的层级读取日志，而不是直接依赖 Kubernetes 原始日志路径。

它维护两类视图：

| 视图 | 用途 | 渲染路径 |
|---|---|---|
| `container_file` rootfs 视图 | 采集容器内部文件 | `/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/{logPath}` |
| `container_stdio` 控制器视图 | 采集容器标准输出/错误 | `/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/*.log` |

`container_file` 类型的策略需要采集容器内部路径，例如：

```text
/app/logs/app.log
```

Filebeat 运行在宿主机视角，不能直接访问业务容器内部文件。因此 sidecar 维护一个稳定路径：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/{pod}/containers/{container}/{logPath}
```

其中：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/{pod}/containers/{container}
    → /hostproc/<pid>/root
```

这样 Filebeat 看到的是一个稳定入口，背后由 sidecar 跟随 Pod 和容器生命周期更新。

`container_stdio` 类型本质上读取 Kubernetes 节点上的标准输出日志：

```text
/var/log/containers/{pod}_{namespace}_{container}-*.log
```

这个原始路径本身不包含控制器类型和控制器名称。因此 sidecar 额外维护一套控制器视图：

```text
/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/{pod}/containers/{container}/{logFileName}.log
    → /var/log/containers/{pod}_{namespace}_{container}-*.log
```

其中 `{logFileName}.log` 可以直接使用真实日志文件的 basename。Filebeat 通过 `*.log` 读取该目录下的标准输出日志。

`controllerType` 是规范化后的小写控制器类型，例如 `deployment`、`statefulset`、`daemonset`、`job`、`cronjob`、`replicaset`、`pod`。普通裸 Pod 使用 `pod`；无法识别时可使用 `unknown`，但应打日志便于排查。

`controllerName` 是用户感知的控制器名称，例如 Deployment 名 `payment-api`、StatefulSet 名 `mysql`、DaemonSet 名 `node-exporter`。普通裸 Pod 可以使用 Pod 名作为 `controllerName`。路径段必须经过安全化处理，避免 `/`、`..`、空字符串等不安全值进入本地文件路径。

### 9.1 监听范围

只监听当前节点 Pod：

```text
fieldSelector = spec.nodeName=<NODE_NAME>
```

需要 RBAC：

```text
resources: pods
verbs: get, list, watch
```

### 9.2 事件处理

| 事件 | 行为 |
|---|---|
| Pod Add | 如果 Pod 是 Running 或 Succeeded，为每个容器创建 rootfs 链接和 stdio 链接 |
| Pod Update | 如果容器 ID 变化，刷新 rootfs 链接和 stdio 链接 |
| Pod Delete | 删除 `/var/log/klog/{namespace}/{controllerType}/{controllerName}/{pod}` 和 `/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/{pod}` |
| Informer 事件缺失 | 由定期 reconcile 兜底 |

### 9.3 Reconcile

默认每 60s 执行一次：

```text
list current node pods
for each active pod:
    syncPod(pod)

scan KLOG_DIR:
    remove pod dirs not in active namespace/controllerType/controllerName/pod set
    remove empty controllerName dirs
    remove empty controllerType dirs
    remove empty namespace dirs

scan KLOG_STDIO_DIR:
    remove pod dirs not in active namespace/controllerType/controllerName/pod set
    remove empty controllerName dirs
    remove empty controllerType dirs
    remove empty namespace dirs
```

作用：

- 补偿 watch 事件丢失。
- 补偿 containerd `init.pid` 晚于 Pod 事件写入的时序问题。
- 清理残留目录。

### 9.4 Rootfs 目标选择

目标优先级：

```text
1. /hostproc/<init-pid>/root
2. /hostproc/<pid-found-by-cgroup>/root
3. /hostfs/<containerd-state>/<container-id>/rootfs
```

#### 策略 1：读取 init.pid

路径：

```text
<rootfsPrefix>/<containerID>/init.pid
```

如果能读到 PID 且 `/hostproc/<pid>/root` 存在，则链接到它。

优点：

- 能看到容器运行时 mount namespace。
- 能看到 emptyDir、PVC、hostPath 等卷挂载后的文件视图。

#### 策略 2：扫描 /proc cgroup

当 `init.pid` 不可用时，扫描：

```text
/hostproc/*/cgroup
```

如果 cgroup 内容包含完整容器 ID 或前 12 位短 ID，则认为该 PID 属于目标容器，然后使用：

```text
/hostproc/<pid>/root
```

注意：

- 应限制扫描数量，避免节点进程过多导致 CPU 抖动。
- 当前实现最多扫描 4096 个进程。

#### 策略 3：回退到 bundle rootfs

如果前两种策略失败，使用：

```text
<rootfsPrefix>/<containerID>/rootfs
```

这个路径可能看不到卷挂载后的文件，因此只能作为兜底。

### 9.5 containerd state 目录

默认尝试：

```text
/hostfs/data/container/state/io.containerd.runtime.v2.task/k8s.io
```

可通过 `CONTAINERD_STATE_DIR` 覆盖宿主机上的 state 目录，例如：

```text
/run/k3s/containerd
/run/containerd
/data/container/state
```

正式实现建议支持多个候选路径：

```text
/run/k3s/containerd
/run/containerd
/var/run/containerd
/data/container/state
```

并在日志中打印最终命中的路径。

### 9.6 链接路径

对每个容器创建 rootfs 视图：

```text
controllerType, controllerName = resolveControllerIdentity(pod)
linkDir  = KLOG_DIR / namespace / controllerType / controllerName / pod / containers
linkPath = linkDir / containerName
target   = resolved rootfs
```

同时创建 stdio 视图：

```text
stdioDir = KLOG_STDIO_DIR / namespace / controllerType / controllerName / pod / containers / containerName
targets  = glob("/var/log/containers/{pod}_{namespace}_{containerName}-*.log")

for each target:
    linkPath = stdioDir / basename(target)
    linkPath -> target
```

说明：

- stdio 目标是实际 `/var/log/containers/` 下的日志文件或符号链接，不能直接把 glob 当作 symlink target。
- 容器重启后 ContainerID 变化，`/var/log/containers/{pod}_{namespace}_{container}-*.log` 的真实文件名也会变化；Pod Update 或 reconcile 应刷新 stdioDir 下的链接。
- stdioDir 中不再存在的目标链接应被清理，避免 Filebeat 继续读取旧容器日志。

如果已有链接且目标一致，跳过。

如果目标不同：

```text
remove old link
create new symlink
```

### 9.7 与 Filebeat 路径的关系

策略中的容器内路径：

```text
/app/logs/*.log
```

`container_file` 会被 control-server 渲染成：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/app/logs/*.log
```

这里使用 `*` 匹配某个控制器名下的 Pod，因此同一个 Deployment / StatefulSet / DaemonSet / Job 滚动时，新 Pod 会自然被同一条策略采集，同时避免同 namespace、同 controllerType 下其他控制器的 Pod 被误采。

`container_stdio` 会被 control-server 渲染成：

```text
/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/*.log
```

这样 stdio 和 container_file 都可以按控制器收窄范围；区别只是一个读标准输出文件视图，一个读容器 rootfs 视图。

### 9.8 controller identity 解析

推荐解析规则：

```text
if pod has controller ownerReference:
    kind = ownerReference.kind
    name = ownerReference.name
else:
    kind = "Pod"
    name = pod.name

if kind == "ReplicaSet":
    try resolve ReplicaSet ownerReference
    if ReplicaSet owner kind == "Deployment":
        kind = "Deployment"
        name = ReplicaSet owner name

if kind == "Job":
    try resolve Job ownerReference
    if Job owner kind == "CronJob":
        kind = "CronJob"
        name = Job owner name

controllerType = lowercase(kind)
controllerName = safePathSegment(name)
```

说明：

- Deployment 创建的 Pod 直接 owner 通常是 ReplicaSet。为了让路径体现用户感知的控制器身份，推荐额外读取 ReplicaSet 并归一化为 `deployment/{deploymentName}`。
- 如果不想增加 ReplicaSet 读取权限，最小实现可以先使用直接 owner，即 `replicaset/{replicaSetName}`。
- CronJob 创建 Job，再由 Job 创建 Pod。若希望归一化为 `cronjob/{cronJobName}`，需要读取 Job ownerReference；最小实现可以先使用 `job/{jobName}`。
- 解析失败不要阻塞配置同步，可使用 `unknown/{podName}` 或 `pod/{podName}` 并输出告警日志。
- `controllerName` 单独成层比 `{controllerName}-*` 更稳，因为它不依赖 Pod 命名规则，也避免 `api` 误匹配 `api-v2` 这类前缀碰撞。

---

## 10. Kubernetes 部署要求

### 10.1 Pod 结构

```yaml
containers:
  - name: filebeat
    volumeMounts:
      - name: dynamic-inputs
        mountPath: /usr/share/filebeat/inputs.d
      - name: varlog
        mountPath: /var/log
        readOnly: true
      - name: hostfs
        mountPath: /hostfs
        readOnly: true
      - name: hostproc
        mountPath: /hostproc
        readOnly: true

  - name: control-sidecar
    env:
      - name: CONTROL_SERVER_URL
      - name: AGENT_TOKEN
      - name: CLUSTER_ID
      - name: NODE_NAME
      - name: POD_NAME
      - name: POD_NAMESPACE
      - name: INPUTS_DIR
      - name: KLOG_DIR
      - name: HOSTFS_DIR
      - name: HOSTPROC_DIR
      - name: CONFIG_MODE
      - name: WATCH_TIMEOUT
      - name: POLL_INTERVAL
      - name: CONTAINERD_STATE_DIR
    volumeMounts:
      - name: dynamic-inputs
        mountPath: /usr/share/filebeat/inputs.d
      - name: varlog
        mountPath: /var/log
      - name: hostfs
        mountPath: /hostfs
        readOnly: true
      - name: hostproc
        mountPath: /hostproc
        readOnly: true
```

上面的 `/usr/share/filebeat/inputs.d` 是常见容器镜像中的 `path.config/inputs.d` 示例。如果实际 Filebeat 使用其他 `path.config`，两个容器都应挂载到对应的 `<path.config>/inputs.d`。

### 10.2 Volumes

```yaml
volumes:
  - name: dynamic-inputs
    emptyDir: {}
  - name: varlog
    hostPath:
      path: /var/log
      type: Directory
  - name: hostfs
    hostPath:
      path: /
      type: Directory
  - name: hostproc
    hostPath:
      path: /proc
      type: Directory
```

### 10.3 RBAC

最小 RBAC：

```yaml
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["replicasets"]
    verbs: ["get", "list", "watch"]
```

用途：

- `pods list/watch`：symlink-manager 监听本节点 Pod 生命周期。
- `nodes get`：发现当前节点 labels。
- `replicasets get/list/watch`：可选，用于将 Deployment Pod 的直接 owner `ReplicaSet` 归一化为 `deployment`。

如果节点标签通过 `NODE_LABELS` 显式注入，可以考虑取消 `nodes get`。

### 10.4 安全上下文

当前实现需要：

- 写 `/var/log/klog`
- 写 `/var/log/klog-stdio`
- 写 Filebeat 默认 `inputs.d`
- 读 `/hostfs`
- 读 `/hostproc`

因此通常以 root 运行，但不需要 `privileged: true`。

生产建议：

- 不开启 privileged。
- `hostfs`、`hostproc` 只读挂载。
- sidecar 对 `/var/log` 只需要写 `/var/log/klog` 和 `/var/log/klog-stdio`，可以考虑将这两个目录独立成更窄的 hostPath。
- 使用 `readOnlyRootFilesystem: true`，只给 inputs 和 klog 目录写权限。
- 增加 `allowPrivilegeEscalation: false`。
- 按实际需要收紧 Linux capabilities。

---

## 11. Filebeat 基础配置要求

Filebeat 必须开启动态 inputs：

```yaml
filebeat.config.inputs:
  enabled: true
  path: inputs.d/*.yml
  reload.enabled: true
  reload.period: 10s
```

如果需要采集符号链接路径，应确认渲染出的 input 开启：

```yaml
prospector.scanner.symlinks: true
```

对于 container stdio，推荐使用 sidecar 维护的控制器视图：

```text
/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/*.log
```

底层真实目标仍来自 Kubernetes 节点标准输出日志：

```text
/var/log/containers/{pod}_{namespace}_{container}-*.log
```

对于 container file，典型路径：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/{logPath}
```

---

## 12. 错误处理策略

| 场景 | 当前推荐行为 |
|---|---|
| 读取配置失败 | 记录错误，下一轮重试 |
| 心跳失败 | 记录错误，不退出 |
| 注册失败 | 启动失败并退出，交给 Kubernetes 重启 |
| checksum 不一致 | 拒绝应用，上报 failed |
| 写文件失败 | 上报 failed，不更新 currentChecksum |
| 删除孤儿文件失败 | 上报 failed，不更新 currentChecksum |
| watch 失败 | 记录错误，降级 poll |
| Kubernetes client 不可用 | 禁用 symlink-manager，但配置同步继续运行 |
| containerd state 不可用 | 当前实现启动失败；更好的实现可以仅禁用 symlink-manager |
| `/hostproc/<pid>/root` 不可用 | 回退 cgroup 扫描，再回退 bundle rootfs |

一个更稳的正式实现建议：

- 配置同步是主功能，不应因为 symlink-manager 初始化失败而完全退出。
- symlink-manager 可以独立暴露 degraded 状态。
- 拉取配置失败应使用指数退避，避免 control-server 故障时形成请求风暴。
- 每类错误应有明确 metric。

---

## 13. 可观测性

### 13.1 日志

日志字段建议：

| 字段 | 说明 |
|---|---|
| `agent_id` | `cluster_id:node_name` |
| `cluster_id` | 集群 ID |
| `node_name` | 节点名 |
| `checksum` | 当前或目标配置 checksum |
| `config_mode` | poll / watch |
| `operation` | register / heartbeat / pull_config / apply_config / symlink_sync |
| `status` | success / failed / skipped |
| `duration_ms` | 操作耗时 |

关键日志：

- sidecar 启动参数摘要，注意隐藏 token。
- 注册成功。
- 配置变更和应用成功。
- 配置应用失败原因。
- watch 降级 poll。
- symlink 创建、刷新、删除。
- rootfs 回退到 bundle rootfs。

### 13.2 指标

建议暴露 Prometheus metrics：

```text
filebeat_ops_sidecar_heartbeat_total{status}
filebeat_ops_sidecar_config_pull_total{mode,status}
filebeat_ops_sidecar_config_apply_total{status}
filebeat_ops_sidecar_current_config_checksum
filebeat_ops_sidecar_managed_files
filebeat_ops_sidecar_symlink_total{status}
filebeat_ops_sidecar_symlink_reconcile_total{status}
filebeat_ops_sidecar_rootfs_resolution_total{strategy,status}
filebeat_ops_sidecar_control_server_request_duration_seconds{path,method,status}
```

如果不想引入 HTTP 端口，也可以先输出结构化日志，由日志平台聚合。

### 13.3 健康状态

更好的实现可以提供本地 HTTP：

```text
GET /healthz
GET /readyz
GET /metrics
```

`readyz` 可检查：

- 最近一次注册是否成功。
- 最近一次配置拉取是否在合理时间内发生。
- 最近一次配置应用是否成功。
- symlink-manager 是否运行或是否 degraded。

---

## 14. 测试计划

### 14.1 单元测试

配置应用：

- checksum 一致时成功。
- checksum 不一致时失败。
- 拒绝包含路径分隔符的 filename。
- 拒绝非 `fbctl-*.yml` 文件。
- 原子写入后 final 文件内容正确。
- 响应 files 为空时清理所有 managed 文件。
- 只清理 `fbctl-*.yml`，不清理其他文件。

HTTP client：

- register 请求 body 正确。
- heartbeat 请求 body 正确。
- watch 请求 timeout 参数正确。
- watch 失败后 fallback poll。
- 非 2xx 响应返回可读错误。

节点标签：

- 解析 `k=v,k2=v2`。
- 解析 JSON。
- 非法格式忽略或报错，按产品约定固定。

Rootfs 解析：

- `init.pid` 可用时使用 `/hostproc/<pid>/root`。
- `init.pid` 不可用时扫描 cgroup。
- cgroup 中只有短容器 ID 时也能匹配。
- 全部失败时回退 bundle rootfs。
- 扫描进程数量有上限。

Symlink：

- Pod Running 时创建 rootfs 链接和 stdio 链接。
- Pod Pending 时跳过。
- ContainerID 变化时刷新 rootfs 链接和 stdio 链接。
- stdio 链接指向 `/var/log/containers/{pod}_{namespace}_{container}-*.log` 的真实匹配文件。
- stdio 目标文件变化后清理旧链接并创建新链接。
- Pod 删除时删除两类视图目录。
- Reconcile 清理 rootfs 和 stdio 两类孤儿目录。

### 14.2 集成测试

本地闭环：

```powershell
.\scripts\verify-basic.ps1 -ConfigMode poll
.\scripts\verify-basic.ps1 -ConfigMode watch
```

容器闭环：

```powershell
.\scripts\verify-containers.ps1
```

Kubernetes 集成：

1. 部署 control-server 和 Filebeat DaemonSet。
2. 创建 `container_stdio` 策略。
3. 确认 Filebeat 默认 `inputs.d/fbctl-*.yml` 被写入。
4. 更新策略，确认文件内容更新。
5. 删除策略，确认文件被清理。
6. 创建 `container_file` 策略。
7. 创建业务 Pod 并写入容器内日志文件。
8. 确认 `/var/log/klog/{ns}/{controllerType}/{controllerName}/{pod}/containers/{container}` 指向可访问 rootfs。
9. 确认 `/var/log/klog-stdio/{ns}/{controllerType}/{controllerName}/{pod}/containers/{container}/*.log` 指向 `/var/log/containers/` 下的真实 stdio 日志。
10. 重启业务 Pod，确认 rootfs 链接和 stdio 链接都跟随新 ContainerID。
11. 删除业务 Pod，确认两类链接目录都被清理。

### 14.3 故障注入

| 故障 | 期望 |
|---|---|
| control-server 不可达 | sidecar 不退出，持续重试 |
| token 错误 | 拉取失败，日志明确 |
| checksum 被篡改 | 拒绝应用，上报 failed |
| inputsDir 只读 | 应用失败，上报 failed |
| Kubernetes API 不可达 | 配置同步继续，symlink-manager degraded |
| containerd state 目录错误 | symlink-manager degraded，配置同步继续 |
| `/hostproc` 未挂载 | 回退 bundle rootfs 或 degraded |
| Pod 快速创建删除 | reconcile 最终清理残留 |

---

## 15. 从零实现顺序

建议按以下顺序实现，能最快得到可验证闭环：

1. 定义配置结构和环境变量读取。
2. 实现 Agent ID 和节点标签读取。
3. 实现 control-server client。
4. 实现 register。
5. 实现 heartbeat。
6. 实现 poll 拉配置。
7. 实现 checksum 校验和配置落盘。
8. 实现 apply-result 上报。
9. 增加 watch 拉配置和 fallback poll。
10. 增加 RUN_ONCE，方便测试。
11. 实现 symlink-manager 的 runtime 检测。
12. 实现 Pod Informer。
13. 实现 rootfs 解析策略。
14. 实现 symlink 创建、刷新和删除。
15. 实现 reconcile。
16. 增加单元测试、集成测试和容器测试。
17. 增加 metrics、结构化日志和健康检查。

---

## 16. 对当前实现的增强建议

这些建议可以让新的 sidecar 比当前版本更稳：

1. **将 symlink-manager 初始化失败降级，而不是让整个 sidecar 启动失败。**
   配置同步是主功能，工作负载日志视图是增强功能，两者应该隔离故障面。

2. **引入本地 state manifest。**
   记录上次成功应用的 checksum 和文件内容 hash，sidecar 重启后不必从空 checksum 开始。

3. **补强文件名校验。**
   当前只校验 `fbctl-` 前缀和 basename，正式实现建议增加 `.yml` 后缀和正则限制。

4. **写文件增加 fsync。**
   对生产节点来说，`write + rename` 已经避免半写，但 fsync 能提升异常断电时的一致性。

5. **HTTP 请求增加指数退避和抖动。**
   control-server 故障时，所有节点同时重试可能形成压力。

6. **支持 token 轮转。**
   可以定期从文件读取 token，而不是只在启动时从环境变量读取。

7. **支持 mTLS。**
   对跨集群或非可信网络部署，`X-Agent-Token` 可以升级为 mTLS + token 双因子。

8. **增加 metrics 端口。**
   让运维能看到每个节点的拉取、应用、失败和 symlink 状态。

9. **增强 runtime 探测。**
   除 containerd 外，预留 CRI-O、Docker shim 历史环境或不同发行版路径的适配接口。

10. **降低 hostPath 写范围。**
    sidecar 现在写 `/var/log/klog` 和 `/var/log/klog-stdio`，可以考虑将 hostPath 精确到这两个目录。

11. **刷新节点标签。**
    当前节点标签主要在启动注册时上报。如果节点标签会动态变化，sidecar 应周期性刷新并重新注册。

12. **增加 Filebeat reload 观测。**
    可以读取 Filebeat HTTP endpoint `127.0.0.1:5066`，确认 reload 是否成功。

13. **拆分 init container 或权限。**
    如果未来安全要求更高，可将目录初始化、运行时探测、配置同步拆分成不同权限边界。

14. **增加灰度保护。**
    对同一节点配置文件数量、单文件大小、总配置大小设置上限，避免控制面错误导致节点资源异常。

15. **更清晰的 degraded 状态。**
    例如配置同步正常但 symlink-manager 异常，应在 Agent 状态中显示 `partial_degraded`，而不是只有 success / failed。

---

## 17. 验收标准

一个复现实现可以按以下标准验收：

| 类别 | 标准 |
|---|---|
| 注册 | sidecar 启动后能在 `/api/v1/agents` 看到对应 Agent |
| 心跳 | `last_heartbeat_at` 持续更新 |
| 配置拉取 | 创建策略后，目标节点收到 `changed: true` |
| 配置落盘 | 生成 `fbctl-*.yml`，内容与服务端渲染一致 |
| 完整性 | checksum 不一致时拒绝应用 |
| 清理 | 删除或不匹配的策略对应配置被清理 |
| 节点过滤 | node selector 不匹配的策略不会落盘 |
| Watch | watch 模式下配置变化能及时生效 |
| 回退 | watch 失败时 poll 仍可工作 |
| 工作负载日志视图 | Running Pod 生成 `/var/log/klog/.../containers/...` rootfs 链接和 `/var/log/klog-stdio/.../containers/.../*.log` stdio 链接 |
| 容器重启 | ContainerID 变化后链接目标更新 |
| Pod 删除 | 删除 Pod 后链接目录被清理 |
| 可测试 | 核心逻辑有单元测试，闭环脚本通过 |

---

## 18. 最小可行伪代码

```go
func main() {
    cfg := config.LoadFromEnv()
    logger := log.New(...)

    identity := agent.BuildIdentity(cfg)
    client := client.New(cfg.ControlServerURL, cfg.AgentToken)
    applier := apply.New(cfg.InputsDir)

    symlinkMgr, err := symlink.NewManager(cfg)
    if err != nil {
        logger.Warn("symlink-manager degraded", "error", err)
    } else {
        go symlinkMgr.Run(context)
    }

    if err := client.Register(identity); err != nil {
        logger.Fatal("register failed", "error", err)
    }

    checksum := applier.LoadLastChecksum()

    for {
        if err := client.Heartbeat(identity.AgentID, checksum); err != nil {
            logger.Warn("heartbeat failed", "error", err)
        }

        resp, err := client.PullConfig(identity.AgentID, checksum, cfg.ConfigMode)
        if err != nil {
            logger.Warn("pull config failed", "error", err)
            sleepWithBackoff()
            continue
        }

        if resp.Changed {
            if err := applier.Apply(resp); err != nil {
                client.ReportApplyResult(identity.AgentID, resp.Checksum, "failed", err.Error())
            } else {
                checksum = resp.Checksum
                client.ReportApplyResult(identity.AgentID, resp.Checksum, "success", "applied")
            }
        }

        sleep(cfg.LoopSleep())
    }
}
```

---

## 19. 与当前代码的对应关系

| 文档概念 | 当前实现位置 |
|---|---|
| 主循环 | `sidecar/cmd/control-sidecar/main.go` 的 `run()` |
| HTTP client | `client`、`getJSON()`、`postJSON()`、`do()` |
| 配置读取 | `readConfig()` |
| 配置拉取 | `pullConfig()`、`desiredConfig()`、`watchConfig()` |
| 配置落盘 | `applyConfig()` |
| 节点标签发现 | `discoverNodeLabels()`、`fetchKubernetesNodeLabels()` |
| symlink-manager | `runSymlinkManager()` |
| runtime 探测 | `detectRuntime()`、`resolveContainerdPrefix()` |
| rootfs 解析 | `processRootfsPath()`、`findContainerPIDByCgroup()` |
| 定期校准 | `reconcile()` |
| API DTO | `internal/control/types.go` |

---

## 20. 结论

control sidecar 的本质是一个节点侧同步器：

```text
control-server desired state
  → sidecar validates and applies
  → Filebeat reloads local files
  → sidecar reports actual state
```

它的复杂点不在 HTTP，而在节点环境的不确定性：

- Pod 生命周期有时序差异。
- 容器 runtime 路径在不同集群里可能不同。
- `/proc/<pid>/root` 比 bundle rootfs 更准确，但也更依赖进程状态。
- Filebeat reload 是异步的，sidecar 写文件成功不等于 Filebeat 一定加载成功。

因此，正式实现时应该把 sidecar 做成“配置同步稳定、工作负载日志视图可降级、状态清晰可观测”的节点代理，而不是把所有异常都压成单一成功或失败。
