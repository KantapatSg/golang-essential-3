# golang-essential-3 Agent Instructions

## Current state

The repository contains a working Project 2 backend baseline plus the Project 3 design pack. Frontend, Analytics/ClickHouse, Prometheus, Grafana, CI, and Render deployment are not implemented yet.

## Source of truth

1. Read `README.md` and `docs/00_OVERVIEW_MINDMAP.md`.
2. Work in the order defined by `docs/16_IMPLEMENTATION_PHASES.md`.
3. Use `docs/18_DEFINITION_OF_DONE.md` as the acceptance gate.
4. Follow `docs/17_COMMENTING_GUIDE.md` for learning-focused comments.

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

## Stop point

Complete and verify through Phase 10, then report results. Do not create paid cloud resources, configure a custom domain, or deploy to Render until the user explicitly approves Phase 11.

