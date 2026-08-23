$ErrorActionPreference = 'Stop'
$env:COMPOSE_PROGRESS = 'quiet'
$base = if ($env:BASE_URL) { $env:BASE_URL } else { 'http://localhost:8080' }

function Invoke-Compose([string]$arguments) {
  & docker compose -f deploy/docker-compose.yml $arguments.Split(' ')
  if ($LASTEXITCODE -ne 0) { throw "docker compose failed: $arguments" }
}

try {
  $upCommand = if ($env:SKIP_BUILD -eq '1') { 'up -d' } else { 'up --build -d' }
  Invoke-Compose $upCommand
  $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
  if (-not $curl) { $curl = Get-Command curl -ErrorAction Stop }
  $ready = $false
  for ($i = 0; $i -lt 60; $i++) {
    try {
      $health = Invoke-RestMethod "$base/health/ready"
      if ($health.status -eq 'ready') { $ready = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
  }
  if (-not $ready) { throw 'gateway did not become ready within 120 seconds' }

  $smokePassed = $false
  for ($i = 0; $i -lt 30; $i++) {
    & pwsh -NoProfile -File scripts/smoke-test.ps1
    if ($LASTEXITCODE -eq 0) { $smokePassed = $true; break }
    Start-Sleep -Seconds 2
  }
  if (-not $smokePassed) { throw 'basic smoke test failed' }

  $admin = Invoke-RestMethod "$base/api/v1/auth/login" -Method Post -ContentType 'application/json' -Body (@{ email='admin@example.com'; password='admin123' } | ConvertTo-Json)
  $headers = @{ Authorization = "Bearer $($admin.access_token)" }
  $swaggerContent = (& $curl.Source -fsS "$base/swagger/") -join "`n"
  if ($swaggerContent -notmatch 'SwaggerUIBundle') { throw 'Swagger UI is not served' }
  $openapiContent = (& $curl.Source -fsS "$base/openapi.yaml") -join "`n"
  foreach ($path in @('/api/v1/auth/logout','/api/v1/activities','/api/v1/analytics/summary')) {
    if ($openapiContent.IndexOf($path, [System.StringComparison]::Ordinal) -lt 0) { throw "OpenAPI is missing $path" }
  }
  $frontendContent = (& $curl.Source -fsS 'http://localhost:3000/') -join "`n"
  if ($frontendContent -notmatch 'signal ledger') { throw 'frontend landing page is not served' }
  $frontendOpenapi = (& $curl.Source -fsS 'http://localhost:3000/openapi.yaml') -join "`n"
  if ($frontendOpenapi.IndexOf('/api/v1/auth/logout', [System.StringComparison]::Ordinal) -lt 0) { throw 'frontend proxy does not expose OpenAPI' }
  $grafanaReady = $false
  for ($i = 0; $i -lt 30; $i++) {
    try {
      $grafanaHealth = Invoke-RestMethod 'http://localhost:3001/api/health'
      if ($grafanaHealth.database -eq 'ok') { $grafanaReady = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
  }
  if (-not $grafanaReady) { throw 'Grafana did not become ready' }
  $clickhouseDatasource = Invoke-RestMethod 'http://localhost:3001/api/datasources/name/ClickHouse'
  if ($clickhouseDatasource.type -ne 'grafana-clickhouse-datasource') { throw 'Grafana ClickHouse datasource is not provisioned' }

  $activityReady = $false
  for ($i = 0; $i -lt 30; $i++) {
    try {
      $activities = Invoke-RestMethod "$base/api/v1/activities" -Headers $headers
      if (@($activities).Count -ge 1) { $activityReady = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
  }
  if (-not $activityReady) { throw 'activity projection did not become visible' }

  $analyticsReady = $false
  for ($i = 0; $i -lt 30; $i++) {
    try {
      $summary = Invoke-RestMethod "$base/api/v1/analytics/summary" -Headers $headers
      if ($summary.total_events -ge 1) { $analyticsReady = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
  }
  if (-not $analyticsReady) { throw 'analytics projection did not become visible' }

  $targetsReady = $false
  $down = @()
  for ($i = 0; $i -lt 30; $i++) {
    try {
      $targets = Invoke-RestMethod 'http://localhost:9090/api/v1/targets'
      $down = @($targets.data.activeTargets | Where-Object { $_.health -ne 'up' })
      if (@($targets.data.activeTargets).Count -gt 0 -and $down.Count -eq 0) { $targetsReady = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
  }
  if (-not $targetsReady) { throw "Prometheus targets are down or unavailable: $($down.Count)" }

  $rows = (& docker compose -f deploy/docker-compose.yml exec -T clickhouse clickhouse-client --query "SELECT count() FROM analytics.task_events FINAL").Trim()
  if ([int64]$rows -lt 1) { throw 'ClickHouse has no projected events' }
  Write-Host "compose acceptance ok events=$rows"
} finally {
  & docker compose -f deploy/docker-compose.yml down
}
