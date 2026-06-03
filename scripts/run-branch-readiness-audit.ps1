param(
  [string]$ComposeFile = "docker-compose.prod.yml",
  [string]$SqlFile = "scripts/branch_readiness_audit.sql",
  [string]$OutDir = "reports",
  [string]$OutFile = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($OutFile)) {
  $stamp = Get-Date -Format "yyyyMMdd_HHmmss"
  $OutFile = Join-Path $OutDir "branch_readiness_audit_$stamp.txt"
}

New-Item -ItemType Directory -Force $OutDir | Out-Null

Write-Host "[AUDIT] branch readiness SQL: $SqlFile"
Write-Host "[AUDIT] output: $OutFile"

if (-not (Test-Path $SqlFile)) {
  throw "SQL file not found: $SqlFile"
}

$sql = Get-Content $SqlFile -Raw

if (-not [string]::IsNullOrWhiteSpace($env:DATABASE_URL) -and (Get-Command psql -ErrorAction SilentlyContinue)) {
  $output = $sql | psql $env:DATABASE_URL -v ON_ERROR_STOP=1 -At -F "|"
} else {
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "docker is required when DATABASE_URL+psql are not available"
  }
  $pgUser = docker compose -f $ComposeFile exec -T postgres printenv POSTGRES_USER 2>$null
  if ([string]::IsNullOrWhiteSpace($pgUser)) { $pgUser = "turcompany" }
  $pgDb = docker compose -f $ComposeFile exec -T postgres printenv POSTGRES_DB 2>$null
  if ([string]::IsNullOrWhiteSpace($pgDb)) { $pgDb = "turcompany" }
  $output = $sql | docker compose -f $ComposeFile exec -T postgres psql -v ON_ERROR_STOP=1 -U $pgUser.Trim() -d $pgDb.Trim() -At -F "|"
}

$output | Tee-Object -FilePath $OutFile

$critical = 0
$warn = 0
foreach ($line in $output) {
  $parts = $line -split '\|', 4
  if ($parts.Count -lt 3) { continue }
  $count = [int64]$parts[2]
  if ($parts[1] -eq "CRITICAL" -and $count -gt 0) { $critical += $count }
  if ($parts[1] -eq "WARN" -and $count -gt 0) { $warn += $count }
}

Write-Host "[AUDIT] critical unresolved rows: $critical"
Write-Host "[AUDIT] warning rows: $warn"

if ($critical -gt 0) {
  Write-Error "[AUDIT][FAIL] Fix CRITICAL branch readiness rows before production deploy."
  exit 2
}

Write-Host "[AUDIT][OK] No critical branch readiness gaps found."
