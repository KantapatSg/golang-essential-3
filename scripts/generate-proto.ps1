$ErrorActionPreference = 'Stop'
if (Get-Command buf -ErrorAction SilentlyContinue) { Push-Location contracts; buf generate; Pop-Location; exit 0 }
# CI/developer machines without a global binary can still reproduce native stubs.
Push-Location contracts
go run github.com/bufbuild/buf/cmd/buf@v1.46.0 generate
Pop-Location
