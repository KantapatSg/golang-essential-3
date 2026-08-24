$ErrorActionPreference = 'Stop'
$modules = @('contracts','services/api-gateway','services/identity-service','services/task-service','services/activity-service','services/analytics-service','services/analytics-worker','services/inventory-service','services/order-service')
foreach ($module in $modules) { Push-Location $module; go vet ./...; Pop-Location }
