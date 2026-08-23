$ErrorActionPreference = 'Stop'
$modules = @('contracts','services/api-gateway','services/identity-service','services/task-service','services/activity-service')
foreach ($module in $modules) { Push-Location $module; go test ./...; Pop-Location }
