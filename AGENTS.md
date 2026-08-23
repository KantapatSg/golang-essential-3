# golang-essential-3 Agent Instructions

## Current state

The repository already contains the Project 2 backend baseline, Project 3 frontend,
Analytics/ClickHouse services, Prometheus/Grafana artifacts, CI, and local Compose.
Source presence is not the same as runtime verification. Read the status ledger before
changing code so unfinished acceptance gates are not mistaken for missing features.

## Source of truth

1. Read `README.md` and `docs/23_MASTER_BLUEPRINT.md`.
2. Read `docs/25_PHASE_CONTEXT_RESUME.md` for the latest evidence and exact resume point.
3. Work in the order defined by `docs/16_IMPLEMENTATION_PHASES.md`.
4. Use `docs/24_TEST_ACCEPTANCE_MATRIX.md` and `docs/18_DEFINITION_OF_DONE.md` as gates.
5. Follow `docs/17_COMMENTING_GUIDE.md` for learning-focused comments.

## Architecture rules

- `contracts/proto` is the gRPC contract source of truth.
- Each service owns its business code and database; do not import business packages across bounded contexts.
- The Gateway is the only public backend edge. Internal gRPC, Kafka, databases, Prometheus, and ClickHouse stay private.
- PostgreSQL is the transactional source of truth. ClickHouse is an eventually consistent analytical projection.
- Task mutation and Outbox insertion must commit in the same PostgreSQL transaction.
- Kafka consumers must be idempotent and commit offsets only after their side effects succeed.
- Redis task cache may fail open to the reader database. Identity refresh session must fail closed.
- Do not put user ID, task ID, event ID, email, or request ID in Prometheus labels.

## Implementation rules

- Preserve useful Project 2 comments and update them when behavior changes.
- Add short Thai comments at important decisions/invariants/failure boundaries; do not narrate obvious syntax.
- Do not manually edit generated protobuf files. Regenerate them from `.proto`.
- Add focused tests in the same phase as behavior changes.
- Keep docs and diagrams consistent with implemented behavior; clearly label future targets.
- Do not add Kubernetes, service mesh, tracing backend, or schema registry without a new acceptance requirement.

## Required checks

```powershell
make test
make vet
make build
docker compose -f deploy/docker-compose.yml config --quiet
```

Add frontend and integration commands to this file when those components are implemented.

```powershell
Push-Location frontend
npm ci
npm run lint
npm test
npm run build
npm run e2e
Pop-Location
./scripts/smoke-test.ps1
```

## Stop point

Complete and verify through Phase 10, then report results. Do not create paid cloud resources, configure a custom domain, or deploy to Render until the user explicitly approves Phase 11.
