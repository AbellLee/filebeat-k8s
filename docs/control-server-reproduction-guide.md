# Control Server 复现与演进指南

> 面向希望从零复现、重构或增强 Filebeat-Ops Control Server 的工程师。本文需要与 `docs/sidecar-implementation-guide.md` 配套阅读：Control Server 负责计算 desired state，control-sidecar 负责把 desired state 安全落到节点并回报 actual state。目标不是逐行复制当前代码，而是把控制面必须具备的模型、接口、状态流转和部署验收讲清楚，使后来者可以复刻当前版本，并在关键位置开发出更好的应用。

---

## 1. 复现目标

复现出的 Control Server 至少需要做到：

- 管理 Filebeat 采集策略：创建、查询、更新、删除、查看 revision、回滚 revision。
- 将抽象 policy 渲染为 Filebeat `filestream` input YAML。
- 为每个 policy 保存不可变 revision，并计算 rendered config checksum。
- 接收 control-sidecar 注册、心跳和 apply-result 上报。
- 根据集群 ID、启用状态和节点标签，向 agent 下发当前应生效的配置文件集合。
- 支持 poll 和 watch 两种配置拉取方式。
- 可选支持 Kubernetes `FilebeatPolicy` CRD，把声明式 CR 同步为内部 policy。
- 可选提供 Kubernetes 资源发现接口，供 Grafana 插件创建策略时选择 namespace、Deployment、Pod、container 和 label。

配套 sidecar 至少需要做到的内容请以 `docs/sidecar-implementation-guide.md` 为准，尤其是：

- Agent 身份：`agent_id = cluster_id:node_name`。
- 文件协议：sidecar 只写 Filebeat 默认 `inputs.d/fbctl-*.yml`，Filebeat 通过 `inputs.d/*.yml` 热加载。
- 安全落盘：checksum 校验、原子写入、孤儿文件清理、apply-result 上报。
- 节点适配：维护 `container_stdio` 所需的 `/var/log/klog-stdio/...` 和 `container_file` 所需的 `/var/log/klog/...` 符号链接。
- 降级能力：watch 失败降级 poll，Kubernetes API 或 runtime 异常时状态清晰可观测。

非目标：

- 不重新实现 Filebeat 的采集能力。
- 不绑定具体日志存储系统；当前示例基础配置输出到 Kafka，复现者可替换为 Elasticsearch、Kafka、Logstash 或其他输出。
- 不要求一开始就实现完整灰度发布、多租户 RBAC 或多副本 leader election。

推荐阅读顺序：

1. 先读本文的第 2 到第 9 节，理解控制面模型、API、revision 和配置下发。
2. 再读 `docs/sidecar-implementation-guide.md` 的第 3 到第 9 节，理解节点侧落盘、安全和容器文件采集。
3. 最后回到本文第 14 节，把两份文档里的增强建议合并成下一版产品路线。

---

## 2. 总体架构

```text
Grafana App Plugin / API Client
        |
        | /api/v1/policies, /api/v1/agents, /api/v1/cluster/options
        v
Control Server
        |
        | stores policy metadata, revisions, agents, apply results
        v
MySQL / PostgreSQL
        ^
        |
        | /api/v1/agent/register, heartbeat, config, watch, apply-result
        |
control-sidecar  <---- shared inputs.d ---->  Filebeat
        |
        | optional: maintain /var/log/klog-stdio/... and /var/log/klog/... symlinks
        v
Kubernetes node
```

Control Server 本身应保持无状态，所有策略、版本和 Agent 状态都落在数据库里。这样 HTTP Server 可以重启而不丢配置，sidecar 也可以通过 checksum 快速判断是否需要重新落盘。

这套系统最重要的边界是：

| 组件 | 应负责 | 不应负责 |
|---|---|---|
| Control Server | policy 管理、revision、YAML 渲染、节点标签匹配、desired files 计算、agent 状态存储 | 直接操作节点文件、直接调用 Filebeat、解释容器 runtime 细节 |
| control-sidecar | 注册、心跳、拉取 desired files、校验并写入 `inputs.d`、维护 `/var/log/klog-stdio` 与 `/var/log/klog`、上报 actual state | 创建策略、渲染 YAML、决定策略是否匹配节点 |
| Filebeat | 读取本地 inputs、采集日志、输出到下游系统 | 感知 Control Server 或 policy 数据库 |

如果后续要做成更好的应用，应优先保持这个边界。Control Server 返回“文件集合”，sidecar 不解析 policy；sidecar 返回“应用结果”，Control Server 不进入节点操作系统。

---

## 3. 推荐目录结构

可以参考当前仓库的分层：

```text
internal/control/
  types.go        # Policy、PolicyRevision、Agent request、ConfigFile 等领域模型
  render.go       # Filebeat YAML 渲染、文件名、checksum、selector 工具

server/cmd/control-server/
  main.go               # 入口、路由、HTTP handler、配置读取
  db.go                 # 数据库连接、MySQL/PostgreSQL 方言适配、自动迁移
  policy_sync.go        # API 与 Operator 共享的 upsert/delete policy 逻辑
  operator.go           # FilebeatPolicy CRD list/watch/reconcile/status
  cluster_discovery.go  # Kubernetes namespace/deployment/pod/node label 发现
```

建议保持 `internal/control` 不依赖 Gin、SQL 或 Kubernetes client。这样渲染逻辑可以单测，sidecar 也能复用 checksum、文件名和类型定义。

---

## 4. 运行依赖

最低依赖：

| 组件 | 当前实现 | 复现建议 |
|---|---|---|
| 语言 | Go 1.26 | 跟随 `go.mod`，保持 server/sidecar 单仓编译 |
| HTTP 框架 | Gin v1.11 | 可替换，但路由和 JSON 契约保持稳定 |
| 数据库 | MySQL 8.4 默认，兼容 PostgreSQL | 生产建议使用托管 MySQL/PostgreSQL |
| K8s Client | client-go v0.36 | 仅 Operator 和资源发现需要 |
| 配置格式 | Filebeat YAML | 渲染后保存 revision 和 checksum |
| 容器化 | Docker + Alpine | server 构建为静态二进制 |

关键环境变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DATABASE_URL` | `mysql://filebeat:filebeat@localhost:3306/filebeat_ops?parseTime=true` | 支持 `mysql://`、MySQL DSN、`postgres://`、`postgresql://` |
| `PORT` | `8080` | HTTP 监听端口 |
| `AGENT_TOKEN` | `dev-agent-token` | sidecar 调用 agent API 的共享 token |
| `WATCH_POLL_INTERVAL` | `2s` | watch 请求内部检查配置变化的间隔 |
| `WATCH_MAX_TIMEOUT` | `60s` | 单次 watch 允许的最大 timeout |
| `OPERATOR_ENABLED` | `false` | 是否启动内置 FilebeatPolicy Operator |
| `OPERATOR_CLUSTER_ID` | `dev` | CRD 未指定集群时的默认集群 ID |
| `OPERATOR_NAMESPACE` | 空 | 限制 Operator watch 指定 namespace；空表示全量 |
| `OPERATOR_RESYNC_INTERVAL` | `30s` | Operator watch 断开后的重建间隔 |

---

## 5. 数据模型

### `policies`

保存策略当前规格和当前 revision 指针。

| 字段 | 说明 |
|---|---|
| `id` | policy 稳定 ID，建议只允许安全文件名字符或统一 `SafeName` |
| `name` | 展示名 |
| `cluster_id` | 目标集群 |
| `namespace` | Kubernetes namespace 路径收窄条件；`container_stdio` 和 `container_file` 必填 |
| `controller_type` | 控制器类型，建议小写枚举：`deployment`、`statefulset`、`daemonset`、`job`、`cronjob`、`pod` |
| `controller_name` | 控制器名称，例如 `payment-api`；与 `controller_type` 共同标识工作负载 |
| `pod_selector` | 高级/兼容字段；当前实现会存储和从 CRD 转换，但默认路径收窄不再依赖它 |
| `pod_name` | 兼容/排障字段，不再作为 `container_stdio` / `container_file` 的主路径收窄条件；裸 Pod 可表达为 `controller_type=pod`、`controller_name=<podName>` |
| `container_name` | 容器名称路径收窄条件；`container_stdio` 和 `container_file` 必填 |
| `node_selector` | 节点 label selector，服务端下发前匹配 agent 上报的 node labels |
| `log_type` | `container_stdio`、`host_file`、`container_file` |
| `log_path` | `host_file` 使用宿主机可见绝对路径；`container_file` 使用用户传入的容器内绝对路径 |
| `enabled` | 是否下发 |
| `priority` | 影响文件名排序，渲染为 `fbctl-<priority>-<policyID>.yml` |
| `current_revision` | 当前生效 revision |
| `custom_fields` | 渲染到 Filebeat input `fields`，例如 `__project__`、`__logstore__` |

目标模型的主收窄维度是：

```text
{namespace}/{controllerType}/{controllerName}/{container}
```

因此 `policies` 表、管理 API、Grafana 表单和 CRD 转换都应把 `controller_type`、`controller_name`、`container_name` 当成容器日志策略的一等字段。`pod_name` 只能保留为兼容旧数据、人工排障或展示字段，不能再作为新建策略的必填目标。

迁移建议：

- 数据库先新增可空列 `controller_type`、`controller_name`；API 层先对新请求强校验，避免一次迁移影响 `host_file` 和历史数据。
- 对旧的 Pod 级策略，如果只能从 `pod_name` 识别目标，可以回填为 `controller_type=pod`、`controller_name=<pod_name>`，并在 UI 标记为需要人工确认。
- 常用查询建议建立组合索引：`cluster_id, enabled, namespace, controller_type, controller_name, container_name`，便于后续做预览、冲突检测和影响范围分析。

### `policy_revisions`

保存每次创建、更新或回滚产生的渲染结果。

| 字段 | 说明 |
|---|---|
| `policy_id` | 关联 `policies.id` |
| `revision` | 单 policy 内递增 |
| `rendered_config` | 完整 Filebeat YAML |
| `checksum` | `sha256:<hex>` |
| `created_by` | `api`、`operator:filebeatpolicy/<ns>/<name>` 或请求头 `X-User` |

当前回滚行为是“复制历史 rendered_config 到新的 revision”，并把 `current_revision` 指向新 revision；它不会把 `policies` 表里的当前规格字段恢复为历史值。更好的实现是把 policy spec snapshot 也保存进 revision，回滚时同时恢复规格和 rendered config。

### `agents`

保存每个 sidecar 的注册、心跳、当前 checksum 和最近一次应用状态。

Agent ID 推荐使用：

```text
<cluster_id>:<node_name>
```

### `agent_apply_results`

保存 apply-result 历史，用于审计和排障。

---

## 6. HTTP API 契约

### 健康检查

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/healthz` | 进程存活 |
| `GET` | `/readyz` | 数据库可用 |

### 管理 API

当前管理 API 没有内置鉴权，通常通过 Grafana Data Proxy、Ingress 或 API Gateway 承担用户认证。复现生产版本时建议补上 RBAC。

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/policies` | 创建 policy，生成 revision 1 |
| `GET` | `/api/v1/policies` | 列出 policy |
| `GET` | `/api/v1/policies/:id` | 获取 policy |
| `PUT` | `/api/v1/policies/:id` | 更新 policy，生成新 revision |
| `DELETE` | `/api/v1/policies/:id` | 删除 policy，revision 级联删除 |
| `GET` | `/api/v1/policies/:id/revisions` | 查看 revision 历史 |
| `POST` | `/api/v1/policies/:id/rollback` | 复制历史 rendered config 为新 revision |
| `GET` | `/api/v1/agents` | 查看 Agent 状态 |
| `GET` | `/api/v1/agents/:id` | 查看单个 Agent |
| `GET` | `/api/v1/cluster/options` | Kubernetes 资源发现选项 |

兼容 Grafana 查询参数的别名接口：

| 方法 | 路径 |
|---|---|
| `GET` / `PUT` / `DELETE` | `/api/v1/policy-by-id?id=<id>` |
| `GET` | `/api/v1/policy-revisions?id=<id>` |
| `POST` | `/api/v1/policy-rollback?id=<id>` |

#### Policy 请求体目标契约

`POST /api/v1/policies` 和 `PUT /api/v1/policies/:id` 应接受并返回同一套 policy 字段。容器日志策略必须使用 controller-aware scope，不再要求调用方传入具体 Pod 名：

```json
{
  "id": "payment-app",
  "name": "payment app",
  "cluster_id": "dev",
  "namespace": "payment",
  "controller_type": "deployment",
  "controller_name": "payment-api",
  "container_name": "app",
  "node_selector": "nodepool=online",
  "log_type": "container_stdio",
  "enabled": true,
  "priority": 100,
  "custom_fields": {
    "__project__": "cloudnet",
    "__logstore__": "payment"
  }
}
```

不同日志类型的必填字段：

| `log_type` | 必填字段 | 路径语义 |
|---|---|---|
| `container_stdio` | `cluster_id`、`namespace`、`controller_type`、`controller_name`、`container_name` | Control Server 默认渲染 `/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}.log` |
| `container_file` | `cluster_id`、`namespace`、`controller_type`、`controller_name`、`container_name`、`log_path` | `log_path` 是用户传入的容器内路径，例如 `/app/logs/*.log`，服务端渲染到 `/var/log/klog/...` |
| `host_file` | `cluster_id`、`log_path` | `log_path` 是宿主机可见绝对路径，不使用 Kubernetes controller scope |

兼容字段处理建议：

- `pod_name` 可以继续出现在响应中，用于兼容旧 UI 或展示历史策略，但新建/更新容器日志策略时不应依赖它。
- `pod_selector` 可以保留为高级筛选字段，但默认实现应优先使用 controller scope 渲染路径；若要让 selector 真正生效，需要额外做 Kubernetes selector 解析或 Filebeat metadata 过滤。
- API 层应显式校验 `controller_type/controller_name/container_name`，不要把它们隐式藏在 `pod_selector`、`pod_name` 或前端表单状态里。

### Agent API

Agent API 必须带：

```http
X-Agent-Token: <AGENT_TOKEN>
```

也兼容：

```http
Authorization: Bearer <AGENT_TOKEN>
```

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/v1/agent/register` | 注册或更新 agent 基本信息 |
| `POST` | `/api/v1/agent/heartbeat` | 上报心跳和当前 checksum |
| `GET` | `/api/v1/agent/config?agent_id=&cluster_id=&checksum=` | poll 拉取配置 |
| `GET` | `/api/v1/agent/watch?agent_id=&cluster_id=&checksum=&timeout=25s` | 长轮询拉取配置 |
| `POST` | `/api/v1/agent/apply-result` | 上报落盘结果 |

这些接口是本文与 `sidecar-implementation-guide.md` 之间最关键的契约。复现者可以改内部实现，但应尽量保持以下语义稳定：

- sidecar 启动后必须先 register，再 heartbeat 和拉配置。
- `agent_id` 优先于 `cluster_id`，因为只有 `agent_id` 能让服务端读取已注册 node labels 并执行节点级过滤。
- heartbeat 里的 checksum 只能代表“本地已成功应用”的配置集合。
- config/watch 返回的是完整 desired files 集合，不是增量 patch。
- apply-result 无论成功失败都应上报，失败时不要推进本地 current checksum。

---

## 7. Policy 生命周期

### 创建

1. 解析 JSON 请求。
2. 补默认值：
   - `enabled=true`
   - `priority=100`
   - `log_type=container_stdio`
   - 未传 `id` 时使用 `SafeName(name)`
3. 校验：
   - `id/name/cluster_id` 必填。
   - `log_type` 必须在允许列表中。
   - `container_stdio` 和 `container_file` 必须填写 `namespace`、`controller_type`、`controller_name`、`container_name`。
   - `controller_type` 必须是受支持的小写枚举；裸 Pod 用 `controller_type=pod`、`controller_name=<podName>` 表达。
   - `pod_name` 不再作为容器日志策略的目标字段；新 API 可以接收它用于兼容，但不能用它替代 `controller_type/controller_name`。
   - `host_file` 的 `log_path` 只能在 `/var/log/`、`/opt/logs/`、`/data/logs/` 等允许前缀下。
   - `container_file` 的 `log_path` 必须是容器内绝对路径，不能包含 `..`，不能在 `/etc/`、`/proc/`、`/sys/`、`/dev/`、`/run/secrets/` 等敏感目录下。
4. 调用 renderer 生成 Filebeat YAML。
5. 在一个事务里写入 `policies` 和 `policy_revisions`。

### 更新

1. 读取当前 policy。
2. 使用 URL 中的 `:id` 作为强制 ID，忽略 body 中可能变更的 ID。
3. `current_revision + 1`。
4. 重新渲染并在事务里更新 policy、插入 revision。

### 删除

删除 `policies` 记录，依赖外键级联删除 `policy_revisions`。sidecar 下次拉取到新 checksum 后，会清理本地不再出现在服务端响应里的 `fbctl-*.yml`。

### 回滚

1. 根据 `policy_id + revision` 找到历史 `rendered_config`。
2. 创建一个新的 revision，内容等于历史 `rendered_config`。
3. 更新 `policies.current_revision`。

复现当前版本时按此行为即可。若要做得更好，建议 revision 表增加 `policy_snapshot` JSON，回滚时恢复 policy 当前规格，避免 UI 展示规格与实际 rendered config 不一致。

---

## 8. 配置下发逻辑

Agent 请求配置时，Control Server 的核心步骤是：

1. 确定目标：
   - 如果传入 `agent_id`，从 `agents` 表读取该 agent 的 `cluster_id` 和 `node_labels`。
   - 如果只传入 `cluster_id`，使用该集群并按空 node labels 处理。
2. 查询当前集群已启用 policy：

```sql
SELECT p.id, p.priority, p.node_selector, r.rendered_config
FROM policies p
JOIN policy_revisions r
  ON r.policy_id = p.id
 AND r.revision = p.current_revision
WHERE p.enabled = true
  AND p.cluster_id = $1
ORDER BY p.priority, p.id;
```

3. 对每条 policy 使用 `node_selector` 匹配 agent node labels。
4. 生成文件名：

```text
fbctl-<zero-padded-priority>-<safe-policy-id>.yml
```

例如：

```text
fbctl-100-payment-app.yml
fbctl-150-offline-node-only.yml
```

5. 对文件集合计算顺序无关 checksum：
   - 先按 filename 排序。
   - 逐个写入 filename、分隔符、content、分隔符到 SHA256。
6. 如果 checksum 与 agent 当前 checksum 相同，返回：

```json
{
  "changed": false,
  "checksum": "sha256:..."
}
```

7. 如果不同，返回：

```json
{
  "changed": true,
  "checksum": "sha256:...",
  "files": [
    {
      "filename": "fbctl-100-payment-app.yml",
      "content": "- type: filestream\n..."
    }
  ]
}
```

watch 模式不需要额外状态表。服务端在请求 timeout 期限内按 `WATCH_POLL_INTERVAL` 重算配置集合；有变化就立即返回，没有变化则超时返回 `changed=false`。

sidecar 侧应用算法应参考 `docs/sidecar-implementation-guide.md`：

```text
server desired files
  -> sidecar recomputes ConfigSetChecksum(files)
  -> reject if checksum mismatch
  -> validate every filename
  -> write .<filename>.tmp
  -> rename tmp to final
  -> remove orphan fbctl-*.yml
  -> report apply-result
```

因此 Control Server 返回的 `filename` 和 `checksum` 必须被视为协议字段，而不是展示字段。更好的 Control Server 可以额外返回每个文件的 content checksum、schema version 和 server render time，帮助 sidecar 做更强的一致性检查。

---

## 9. 渲染规则

目标 renderer 输出 Filebeat `filestream` input。

### `container_stdio`

默认路径按工作负载层级收窄，而不是按 Pod 文件名 glob 直接收窄。目标层级为：

```text
{namespace}/{controllerType}/{controllerName}/{container}
```

推荐由 sidecar 为标准输出维护独立的 controller-aware 入口，Control Server 渲染时按以下层级生成路径：

```text
/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}.log
```

其中 `*` 匹配该控制器下滚动产生的 Pod。这样 `container_stdio` 使用 `/var/log/klog-stdio`，`container_file` 使用 `/var/log/klog`，两者都按 `{namespace}/{controllerType}/{controllerName}/{container}` 收窄，策略不再依赖具体 Pod 名。

会开启：

```yaml
prospector.scanner.symlinks: true
parsers:
  - container: {}
```

并通过 `add_fields` 和 JavaScript processor 补充基础 Kubernetes 身份字段。JavaScript processor 需要能识别 controller-aware klog 路径，把 namespace、controller、pod 和 container 写入事件字段。

### `host_file`

使用用户传入的宿主机可见绝对路径，不渲染 Kubernetes 身份字段。路径必须通过 allow/deny list 校验。

### `container_file`

使用用户传入的容器内路径，例如：

```text
/app/logs/*.log
```

Control Server 会渲染为 sidecar symlink-manager 维护的路径：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/app/logs/*.log
```

新 API 不应允许容器日志策略缺少 controller 或 container。为了兼容历史数据，迁移期间可以把缺失字段渲染成通配符，但这应被视为降级行为并在 UI/API 中提示风险：

```text
/var/log/klog/{namespace}/*/*/containers/*/app/logs/*.log
```

目标 sidecar 实现会在每个节点上维护：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/{pod}/containers/{container}
```

链接目标优先指向：

```text
/hostproc/<pid>/root
```

这样可以看到容器运行时 mount namespace 中的 emptyDir、PVC、hostPath 等卷挂载内容；找不到进程根目录时回退到 containerd bundle rootfs。

### 当前渲染边界

- `pod_selector` 字段当前会保存、展示并从 CRD 转换，但 renderer 不会生成事件级 pod label 过滤。要做得更好，有两条路线：
  - 在服务端通过 Kubernetes API 把 selector 解析成具体 Pod，再渲染精确路径。
  - 在 Filebeat 侧补可靠的 Kubernetes metadata enrichment，然后在 processor 阶段做 label filter。
- 当前 renderer 不生成 input-level `drop_event`，因为 Kubernetes metadata 在 input 早期阶段不一定可用。
- `container_stdio` 应使用 `/var/log/klog-stdio/{namespace}/{controllerType}/{controllerName}/*/containers/{container}.log`；`container_file` 应使用 `/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/{logPath}`。采用该设计时，需要同步修改：
  - Control Server 的 `ContainerFileKlogPath()`。
  - `container_stdio` 的默认路径推导。
  - sidecar 的 symlink-manager 链接目录。
  - DaemonSet volume mount 与验收脚本。
  - 项目总览和部署文档。

### 与 Sidecar 指南对齐的目标形态

为了让后续应用更接近生产系统，建议把容器日志路径统一为“按工作负载组织”：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/{pod}/containers/{container}
```

Control Server 渲染时可输出：

```text
/var/log/klog/{namespace}/{controllerType}/{controllerName}/*/containers/{container}/{logPath}
```

这样同一个 Deployment、StatefulSet、DaemonSet 或 Job 滚动发布时，策略不需要绑定具体 Pod 名，也能避免同 namespace 下其他工作负载被误采。sidecar 指南里已经给出 controller identity 解析规则：Pod -> ReplicaSet -> Deployment，Job -> CronJob，无法识别时降级为 `pod/{podName}` 或 `unknown/{podName}`。

---

## 10. 本地复现步骤

### 10.1 使用 Go 直接运行

准备 MySQL：

```powershell
docker run --rm --name fbops-mysql `
  -e MYSQL_ROOT_PASSWORD=filebeat-root `
  -e MYSQL_USER=filebeat `
  -e MYSQL_PASSWORD=filebeat `
  -e MYSQL_DATABASE=filebeat_ops `
  -p 53306:3306 `
  -d mysql:8.4
```

启动 Control Server：

```powershell
$env:DATABASE_URL = "mysql://filebeat:filebeat@localhost:53306/filebeat_ops?parseTime=true"
$env:AGENT_TOKEN = "dev-agent-token"
$env:PORT = "18080"
go run ./server/cmd/control-server
```

检查：

```powershell
Invoke-RestMethod http://localhost:18080/readyz
```

创建一条 policy：

```powershell
$body = @{
  id = "payment-app"
  name = "payment app"
  cluster_id = "dev"
  namespace = "payment"
  controller_type = "deployment"
  controller_name = "payment-api"
  container_name = "app"
  node_selector = "nodepool=online"
  log_type = "container_stdio"
  enabled = $true
  priority = 100
  custom_fields = @{
    "__project__" = "cloudnet"
    "__logstore__" = "payment"
  }
} | ConvertTo-Json -Depth 10

Invoke-RestMethod `
  -Method POST `
  -Uri http://localhost:18080/api/v1/policies `
  -ContentType application/json `
  -Body $body
```

模拟 agent 注册：

```powershell
$headers = @{ "X-Agent-Token" = "dev-agent-token" }
$agent = @{
  cluster_id = "dev"
  node_name = "node-a"
  pod_name = "control-sidecar-node-a"
  namespace = "filebeat-ops"
  node_labels = @{
    nodepool = "online"
    zone = "local"
  }
} | ConvertTo-Json -Depth 10

Invoke-RestMethod `
  -Method POST `
  -Uri http://localhost:18080/api/v1/agent/register `
  -Headers $headers `
  -ContentType application/json `
  -Body $agent
```

拉取配置：

```powershell
Invoke-RestMethod `
  -Uri "http://localhost:18080/api/v1/agent/config?agent_id=dev%3Anode-a&checksum=" `
  -Headers $headers
```

### 10.2 使用 Docker Compose

只启动数据库和 Control Server：

```powershell
docker compose up --build mysql control-server
```

默认服务地址：

```text
http://localhost:18080
```

如需连同 sidecar 和 Grafana 一起启动，先构建 Grafana 插件：

```powershell
cd .\grafana-plugin
npm.cmd install
npm.cmd run build
cd ..
docker compose up --build
```

---

## 11. Kubernetes 复现步骤

### 11.1 准备镜像

```powershell
docker build -f deploy/docker/server.Dockerfile -t filebeat-ops-server:local .
docker build -f deploy/docker/sidecar.Dockerfile -t filebeat-ops-sidecar:local .
```

远端集群需要推送到镜像仓库，并在 kustomize overlay 中替换镜像名。

### 11.2 准备 Secret

不要直接复用示例中的明文。生产环境至少替换：

```yaml
stringData:
  database-url: mysql://<user>:<password>@<host>:3306/filebeat_ops?parseTime=true
  agent-token: <random-long-token>
  kafka-hosts: <broker-1>:9092,<broker-2>:9092
```

### 11.3 部署

```powershell
kubectl apply -k deploy/kubernetes/base
```

或使用 overlay：

```powershell
kubectl apply -k deploy/kubernetes/overlays/dev
kubectl apply -k deploy/kubernetes/overlays/prod
```

检查：

```powershell
kubectl -n filebeat-ops get pods
kubectl -n filebeat-ops get svc
kubectl -n filebeat-ops logs deploy/filebeat-control-server
kubectl -n filebeat-ops logs ds/filebeat-agent -c control-sidecar
```

临时访问 Control Server：

```powershell
kubectl -n filebeat-ops port-forward svc/filebeat-control-server 18080:8080
```

---

## 12. FilebeatPolicy Operator

Operator 内置在 Control Server 进程中，通过环境变量启用：

```powershell
$env:OPERATOR_ENABLED = "true"
$env:OPERATOR_CLUSTER_ID = "dev"
$env:OPERATOR_NAMESPACE = ""
$env:OPERATOR_RESYNC_INTERVAL = "30s"
```

它会：

1. List 当前所有 `FilebeatPolicy`。
2. 逐个 reconcile 成内部 policy。
3. Watch 后续 Added、Modified、Deleted 事件。
4. 写回 status：
   - `phase`
   - `message`
   - `policyID`
   - `revision`
   - `checksum`
   - `observedGeneration`
   - `lastSyncTime`

CRD 到 policy 的转换规则：

| CRD 字段 | Policy 字段 |
|---|---|
| `spec.id` | `id`；为空时使用 `crd-<namespace>-<name>` |
| `spec.name` | `name`；为空时使用 CR name |
| `spec.clusterID` | `cluster_id`；为空时使用 `OPERATOR_CLUSTER_ID` |
| `spec.namespace` | `namespace` |
| `spec.namespaceSelector.matchNames[0]` | `namespace` fallback |
| `spec.controllerType` | `controller_type` |
| `spec.controllerName` | `controller_name` |
| `spec.containerName` | `container_name` |
| `spec.podSelector.matchLabels` | `pod_selector`，排序后转 `k=v,k2=v2` |
| `spec.nodeSelector.matchLabels` | `node_selector` |
| `spec.logType` | `log_type` |
| `spec.logPath` | `log_path` |
| `spec.enabled` | `enabled` |
| `spec.priority` | `priority` |

生产多副本时需要注意：当前 Operator 没有 leader election。更好的做法是：

- 只让一个 Control Server 副本设置 `OPERATOR_ENABLED=true`；或
- 引入 Kubernetes leader election；或
- 把 Operator 拆成独立 Deployment。

---

## 13. 验收清单

建议至少执行：

```powershell
go test ./...
```

手工验收：

- `GET /readyz` 返回 `{"status":"ok"}`。
- 创建 policy 后 `current_revision=1`。
- 更新 policy 后 `current_revision` 递增。
- `GET /api/v1/policies/:id/revisions` 能看到历史 rendered config 和 checksum。
- 带匹配 `node_selector` 的 agent 能拿到对应 `fbctl-*.yml`。
- 不匹配 `node_selector` 的 agent 拿不到该 policy。
- 相同 checksum 再次拉取返回 `changed=false`。
- `watch` 请求在配置变化时提前返回，在无变化时超时返回 `changed=false`。
- sidecar apply 成功后，`/api/v1/agents` 中 `last_apply_status=success`。
- 删除 policy 后，sidecar 下次 apply 会删除本地孤儿 `fbctl-*.yml`。

当前仓库还提供脚本：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-basic.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-basic.ps1 -ConfigMode watch -Port 18082 -MySQLPort 53307
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-containers.ps1
```

如果复现者选择补齐 pod label 过滤，请把验收脚本也同步为“既验证路径收窄，也验证 label 过滤实际生效”。如果保持当前 renderer 行为，则不要把 `pod_selector` 当作已经生效的事件过滤条件。

和 `sidecar-implementation-guide.md` 对齐后，还应补充节点侧验收：

- sidecar 重启后能继续根据本地状态或服务端 checksum 收敛到正确配置。
- checksum 被篡改时拒绝落盘。
- 非 `fbctl-*.yml` 文件不会被清理。
- watch 失败时 poll fallback 可用。
- Kubernetes API 不可用时，配置同步继续运行，symlink-manager 标记 degraded。
- `container_file` 链接目标优先使用 `/hostproc/<pid>/root`，失败时才回退 bundle rootfs。
- Pod 删除或 ContainerID 变化后，`/var/log/klog/...` 最终一致。

