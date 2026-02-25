param(
  [string]$EnvFile = ".env.prod",
  [string]$ComposeFile = "docker-compose.prod.yml",
  [switch]$DryRun,
  [switch]$SmokeOnly
)

$ErrorActionPreference = "Stop"
$ok=0; $warn=0; $fail=0
$fixes = New-Object System.Collections.Generic.List[string]
function Ok($m){ Write-Host "[OK] $m" -ForegroundColor Green; $script:ok++ }
function Warn($m){ Write-Host "[WARN] $m" -ForegroundColor Yellow; $script:warn++; $script:fixes.Add($m) }
function Fail($m){ Write-Host "[FAIL] $m" -ForegroundColor Red; $script:fail++; $script:fixes.Add($m) }
function IsTrue($v){ return @('1','true','yes','on') -contains ($v.ToString().ToLower()) }

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Fail "Установите Docker" } else { Ok "docker найден" }
try { docker compose version *> $null; Ok "docker compose найден" } catch { Fail "Установите Docker Compose v2" }
if (-not (Test-Path $EnvFile)) { Fail "Создайте $EnvFile"; exit 1 }

Get-Content $EnvFile | ForEach-Object {
  if ($_ -match '^\s*#' -or $_ -notmatch '=') { return }
  $parts = $_.Split('=',2)
  [Environment]::SetEnvironmentVariable($parts[0].Trim(), $parts[1].Trim())
}
Ok "Загружен $EnvFile"

$required = @('JWT_SECRET','TELEGRAM_ENABLE','SMTP_HOST','SMTP_PORT','SMTP_USER','SMTP_PASSWORD','SIGN_EMAIL_TOKEN_PEPPER','SIGN_EMAIL_VERIFY_BASE_URL','SIGN_BASE_URL')
foreach($name in $required){ if([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))){ Fail "Заполните $name" } else { Ok "$name задан" } }
if([string]::IsNullOrWhiteSpace($env:DATABASE_URL)){
  foreach($n in @('POSTGRES_USER','POSTGRES_PASSWORD','POSTGRES_DB','POSTGRES_HOST')){ if([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($n))){ Fail "Заполните $n" } }
} else { Ok "DATABASE_URL задан" }
if(IsTrue($env:TELEGRAM_ENABLE)){
  if([string]::IsNullOrWhiteSpace($env:TELEGRAM_BOT_TOKEN) -and [string]::IsNullOrWhiteSpace($env:TELEGRAM_APITOKEN)){ Fail "Заполните TELEGRAM_BOT_TOKEN или TELEGRAM_APITOKEN" }
  if([string]::IsNullOrWhiteSpace($env:TELEGRAM_WEBHOOK_URL)){ Fail "Заполните TELEGRAM_WEBHOOK_URL" }
}

$compose = Get-Content $ComposeFile -Raw
if($compose -match 'api:\s*[\s\S]*DATABASE_URL'){ Ok "api получает DATABASE_URL" } else { Fail "Добавьте DATABASE_URL в api" }
if($compose -match 'api:\s*[\s\S]*CONFIG_PATH'){ Ok "api получает CONFIG_PATH" } else { Fail "Добавьте CONFIG_PATH в api" }

if(-not $DryRun -and -not $SmokeOnly){
  docker compose -f $ComposeFile up -d --build
  Ok "Стек поднят"
}

try {
  $health = Invoke-WebRequest -Uri "http://localhost:$($env:API_PORT ? $env:API_PORT : 4000)/healthz" -Method Get -UseBasicParsing
  if($health.StatusCode -eq 200){ Ok "GET /healthz = 200" } else { Fail "GET /healthz = $($health.StatusCode)" }
} catch { Fail "GET /healthz неуспешен" }

Write-Host "`n===== PREFLIGHT REPORT ====="
Write-Host "OK: $ok"
Write-Host "WARN: $warn"
Write-Host "FAIL: $fail"
if($fixes.Count -gt 0){ Write-Host "`nРекомендации:"; $fixes | ForEach-Object { Write-Host " - $_" } }
if($fail -gt 0){ exit 1 }
