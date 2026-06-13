param()

$ErrorActionPreference = "Stop"

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
  go test ./sidecar/internal/sidecar/symlink
  Write-Host "container symlink unit checks passed"
} finally {
  Pop-Location
}
