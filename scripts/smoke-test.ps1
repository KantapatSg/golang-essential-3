$ErrorActionPreference = 'Stop'
$base = if ($env:BASE_URL) { $env:BASE_URL } else { 'http://localhost:8080' }
Invoke-RestMethod "$base/healthz" | Out-Null
$body = @{ email='member@example.com'; password='member123' } | ConvertTo-Json
$token = Invoke-RestMethod "$base/api/v1/auth/login" -Method Post -ContentType 'application/json' -Body $body
$headers = @{ Authorization = "Bearer $($token.access_token)" }
$task = Invoke-RestMethod "$base/api/v1/tasks" -Method Post -Headers $headers -ContentType 'application/json' -Body (@{ title='smoke'; description='compose' } | ConvertTo-Json)
$id = if ($task.id) { $task.id } else { $task.Id }
Invoke-RestMethod "$base/api/v1/tasks/$id" -Headers $headers | Out-Null
Write-Host "smoke ok task=$id"
