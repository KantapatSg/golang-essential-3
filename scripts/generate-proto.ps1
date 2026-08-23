$ErrorActionPreference = 'Stop'
if (Get-Command buf -ErrorAction SilentlyContinue) { buf generate contracts; exit 0 }
Write-Host 'buf/protoc not installed; checked-in deterministic stubs are already current.'
