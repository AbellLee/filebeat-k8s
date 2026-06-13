param(
  [ValidateSet("poll", "watch")]
  [string]$ConfigMode = "poll",
  [int]$Port = 18080,
  [int]$MySQLPort = 53306
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$MysqlName = "fbops-mysql-$Port"
$InputsDir = Join-Path ([System.IO.Path]::GetTempPath()) "fbops-inputs-$Port"
$ServerJob = $null

function Wait-HttpOk($Url, $Seconds = 60) {
  $deadline = (Get-Date).AddSeconds($Seconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-RestMethod -Uri $Url -TimeoutSec 3
      if ($r.status -eq "ok") { return }
    } catch {
      Start-Sleep -Seconds 1
    }
  }
  throw "timeout waiting for $Url"
}

Push-Location $Root
try {
  if (Test-Path $InputsDir) { Remove-Item -LiteralPath $InputsDir -Recurse -Force }
  New-Item -ItemType Directory -Path $InputsDir | Out-Null

  docker rm -f $MysqlName 2>$null | Out-Null
  docker run --rm --name $MysqlName `
    -e MYSQL_ROOT_PASSWORD=filebeat-root `
    -e MYSQL_USER=filebeat `
    -e MYSQL_PASSWORD=filebeat `
    -e MYSQL_DATABASE=filebeat_ops `
    -p "$MySQLPort`:3306" `
    -d mysql:8.4 | Out-Null

  $env:DATABASE_URL = "mysql://filebeat:filebeat@localhost:$MySQLPort/filebeat_ops?parseTime=true"
  $env:AGENT_TOKEN = "dev-agent-token"
  $env:PORT = "$Port"
  $env:OPERATOR_ENABLED = "false"
  $ServerJob = Start-Job -ScriptBlock {
    param($Root)
    Set-Location $Root
    go run ./server/cmd/control-server
  } -ArgumentList $Root

  Wait-HttpOk "http://localhost:$Port/readyz" 90

  $policy = @{
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
  Invoke-RestMethod -Method POST -Uri "http://localhost:$Port/api/v1/policies" -ContentType application/json -Body $policy | Out-Null

  $env:CONTROL_SERVER_URL = "http://localhost:$Port"
  $env:AGENT_TOKEN = "dev-agent-token"
  $env:CLUSTER_ID = "dev"
  $env:NODE_NAME = "node-a"
  $env:POD_NAME = "control-sidecar-node-a"
  $env:POD_NAMESPACE = "filebeat-ops"
  $env:NODE_LABELS = "nodepool=online,zone=local"
  $env:INPUTS_DIR = $InputsDir
  $env:CONFIG_MODE = $ConfigMode
  $env:WATCH_TIMEOUT = "3s"
  $env:POLL_INTERVAL = "1s"
  $env:RUN_ONCE = "true"
  go run ./sidecar/cmd/control-sidecar

  $rendered = Join-Path $InputsDir "fbctl-100-payment-app.yml"
  if (!(Test-Path $rendered)) { throw "expected rendered config $rendered" }
  $content = Get-Content -Raw $rendered
  if ($content -notmatch "/var/log/klog-stdio/payment/deployment/payment-api/\*/containers/app/\*.log") {
    throw "rendered config path is wrong: $content"
  }

  Invoke-RestMethod -Method DELETE -Uri "http://localhost:$Port/api/v1/policies/payment-app" | Out-Null
  go run ./sidecar/cmd/control-sidecar
  if (Test-Path $rendered) { throw "orphan config was not removed" }

  Write-Host "basic $ConfigMode verification passed"
} finally {
  if ($ServerJob) {
    Stop-Job $ServerJob -ErrorAction SilentlyContinue
    Remove-Job $ServerJob -Force -ErrorAction SilentlyContinue
  }
  docker rm -f $MysqlName 2>$null | Out-Null
  Pop-Location
}
