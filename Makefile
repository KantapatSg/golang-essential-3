.PHONY: test vet build compose-config proto frontend-lint frontend-test frontend-build

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

frontend-lint:
	powershell -NoProfile -Command "Set-Location frontend; npm run lint"

frontend-test:
	powershell -NoProfile -Command "Set-Location frontend; npm test"

frontend-build:
	powershell -NoProfile -Command "Set-Location frontend; npm run build"
