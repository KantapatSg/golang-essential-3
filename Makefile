.PHONY: test vet build compose-config proto

test:
	powershell -ExecutionPolicy Bypass -File scripts/test.ps1

vet:
	powershell -ExecutionPolicy Bypass -File scripts/vet.ps1

build:
	powershell -ExecutionPolicy Bypass -File scripts/build.ps1

compose-config:
	docker compose -f deploy/docker-compose.yml config --quiet

proto:
	powershell -ExecutionPolicy Bypass -File scripts/generate-proto.ps1
