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

  $member = Invoke-RestMethod "$base/api/v1/auth/login" -Method Post -ContentType 'application/json' -Body (@{ email='member@example.com'; password='member123' } | ConvertTo-Json)
  $memberHeaders = @{ Authorization = "Bearer $($member.access_token)" }
  $products = Invoke-RestMethod "$base/api/v1/products" -Headers $memberHeaders
  if (@($products.products).Count -lt 2) { throw 'inventory catalog did not return sellable products' }

  function New-Order([string]$productId, [string]$scenario, [string]$key) {
    $headers = @{ Authorization = "Bearer $($member.access_token)"; 'Idempotency-Key' = $key }
    $body = @{ items = @(@{ product_id = $productId; quantity = 1 }); payment_scenario = $scenario } | ConvertTo-Json -Depth 5
    Invoke-RestMethod "$base/api/v1/orders" -Method Post -Headers $headers -ContentType 'application/json' -Body $body
  }

  function Wait-OrderTerminal([string]$id) {
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
      $order = Invoke-RestMethod "$base/api/v1/orders/$id" -Headers $memberHeaders
      if ($order.status -in @('CONFIRMED', 'CANCELLED', 'REJECTED')) { return $order }
      Start-Sleep -Seconds 1
    }
    throw "order $id did not reach a terminal state"
  }

  $success = New-Order 'prod-mug' 'success' ([guid]::NewGuid().ToString())
  if ($success.status -ne 'PENDING') { throw "order did not return PENDING: $($success.status)" }
  $successFinal = Wait-OrderTerminal $success.id
  if ($successFinal.status -ne 'CONFIRMED') { throw "happy path ended as $($successFinal.status)" }

  $declined = New-Order 'prod-coffee' 'decline' ([guid]::NewGuid().ToString())
  $declinedFinal = Wait-OrderTerminal $declined.id
  if ($declinedFinal.status -ne 'CANCELLED') { throw "payment decline ended as $($declinedFinal.status)" }

  $outOfStock = New-Order 'prod-shirt' 'success' ([guid]::NewGuid().ToString())
  $outOfStockFinal = Wait-OrderTerminal $outOfStock.id
  if ($outOfStockFinal.status -ne 'REJECTED') { throw "out-of-stock ended as $($outOfStockFinal.status)" }

  $idempotencyKey = [guid]::NewGuid().ToString()
  $first = New-Order 'prod-mug' 'success' $idempotencyKey
  $second = New-Order 'prod-mug' 'success' $idempotencyKey
  if ($first.id -ne $second.id) { throw 'idempotency retry created a second order' }

  $admin = Invoke-RestMethod "$base/api/v1/auth/login" -Method Post -ContentType 'application/json' -Body (@{ email='admin@example.com'; password='admin123' } | ConvertTo-Json)
  $headers = @{ Authorization = "Bearer $($admin.access_token)" }
  $swaggerContent = (& $curl.Source -fsS "$base/swagger/") -join "`n"
  if ($swaggerContent -notmatch 'SwaggerUIBundle') { throw 'Swagger UI is not served' }
  $openapiContent = (& $curl.Source -fsS "$base/openapi.yaml") -join "`n"
  foreach ($path in @(
    '/api/v1/auth/logout',
    '/api/v1/orders',
    '/api/v1/notifications/read-all',
    '/api/v1/admin/activities/orders',
    '/api/v1/admin/inventory/reservations',
    '/api/v1/admin/payments',
    '/api/v1/admin/analytics/orders/summary'
  )) {
    if ($openapiContent.IndexOf($path, [System.StringComparison]::Ordinal) -lt 0) { throw "OpenAPI is missing $path" }
  }
  $frontendContent = (& $curl.Source -fsS 'http://localhost:3000/') -join "`n"
  if ($frontendContent -notmatch 'Order Relay') { throw 'Order-first frontend landing page is not served' }
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
  for ($i = 0; $i -lt 60; $i++) {
    try {
      $summary = Invoke-RestMethod "$base/api/v1/analytics/summary" -Headers $headers
      if ($summary.total_events -ge 1) { $analyticsReady = $true; break }
    } catch { }
    Start-Sleep -Seconds 2
  }
  if (-not $analyticsReady) { throw 'analytics projection did not become visible' }

  $orderSummary = Invoke-RestMethod "$base/api/v1/admin/analytics/orders/summary" -Headers $headers
  if ($orderSummary.created -lt 3 -or $orderSummary.confirmed -lt 1 -or $orderSummary.cancelled -lt 1 -or $orderSummary.rejected -lt 1) {
    throw 'order analytics summary did not include all acceptance outcomes'
  }

  $adminOrders = Invoke-RestMethod "$base/api/v1/orders?page=1&page_size=100" -Headers $headers
  if (@($adminOrders.items | Where-Object { $_.id -eq $success.id }).Count -ne 1) {
    throw 'admin Order view did not include the member happy-path order'
  }
  $reservations = Invoke-RestMethod "$base/api/v1/admin/inventory/reservations" -Headers $headers
  if (@($reservations.reservations).Count -lt 2) { throw 'Inventory reservations are not visible to admin' }
  $payments = Invoke-RestMethod "$base/api/v1/admin/payments" -Headers $headers
  if (@($payments.payments).Count -lt 2) { throw 'Payment outcomes are not visible to admin' }
  $orderActivities = Invoke-RestMethod "$base/api/v1/admin/activities/orders" -Headers $headers
  if (@($orderActivities.items).Count -lt 3) { throw 'admin Order activity projection is empty' }

  $notifications = Invoke-RestMethod "$base/api/v1/notifications?page=1&page_size=100" -Headers $memberHeaders
  if (@($notifications.items).Count -lt 3) { throw 'member Notification projection did not receive Order outcomes' }

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
  $orderRows = (& docker compose -f deploy/docker-compose.yml exec -T clickhouse clickhouse-client --query "SELECT count() FROM analytics.order_events FINAL").Trim()
  if ([int64]$orderRows -lt 3) { throw 'ClickHouse has no projected order events' }

  if ($env:RUN_BROWSER_E2E -eq '1') {
    Push-Location frontend
    try {
      $env:PLAYWRIGHT_BASE_URL = 'http://127.0.0.1:3000'
      $env:E2E_REAL_BACKEND = '1'
      & npm run e2e
      if ($LASTEXITCODE -ne 0) { throw 'browser Order workflows failed' }
    } finally {
      Pop-Location
    }
  }
  Write-Host "compose acceptance ok task_events=$rows order_events=$orderRows success=$($success.id) decline=$($declined.id) out_of_stock=$($outOfStock.id)"
} catch {
  Write-Host 'compose acceptance failed; collecting focused diagnostics'
  & docker compose -f deploy/docker-compose.yml ps
  & docker compose -f deploy/docker-compose.yml logs --tail=120 analytics-worker analytics-service task-service kafka clickhouse
  throw
} finally {
  & docker compose -f deploy/docker-compose.yml down
}
