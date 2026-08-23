$ErrorActionPreference = 'Stop'
$modules = @('contracts','services/api-gateway','services/identity-service','services/task-service','services/activity-service','services/analytics-service','services/analytics-worker')
foreach ($module in $modules) { Push-Location $module; go test ./...; Pop-Location }
if ($env:SKIP_FRONTEND -ne '1') { Push-Location frontend; npm test; Pop-Location }
