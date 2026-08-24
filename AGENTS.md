# golang-essential-3 Agent Instructions

## Current state

The repository already contains the Project 2 backend baseline, Project 3 frontend,
Analytics/ClickHouse services, Prometheus/Grafana artifacts, CI, and local Compose.
Source presence is not the same as runtime verification. Read the status ledger before
changing code so unfinished acceptance gates are not mistaken for missing features.

The verified `main` / `v0.1.0-predeploy` runtime is the Task version. The Order Processing
Platform revision is locally/CI verified through R11 on `codex/order-platform-revision`; tag
`v0.2.1-order-predeploy` points to the verified Order release commit. Existing tag
`v0.2.0-order-predeploy` is preserved as-is; never use
Task-version evidence to substantiate Order behavior.

The Inventory and Observability post-release revision is implemented and locally/CI verified in
`docs/31_INVENTORY_OBSERVABILITY_REVISION.md` and
`docs/32_INVENTORY_OBSERVABILITY_HANDOFF.md`; use
`docs/33_INVENTORY_OBSERVABILITY_EVIDENCE.md` for its phase evidence. Do not use
Order R0-R11 evidence to claim the new terminal-event, durable-inventory, Redis,
ClickHouse V2, transaction-metric, dashboard, or Add Stock behavior exists.

## Source of truth

For the verified Task release:

1. Read `README.md`, `docs/23_MASTER_BLUEPRINT.md`, and `docs/25_PHASE_CONTEXT_RESUME.md`.
2. Treat `docs/16_IMPLEMENTATION_PHASES.md`, `docs/24_TEST_ACCEPTANCE_MATRIX.md`, and
   `docs/18_DEFINITION_OF_DONE.md` as historical implementation/evidence gates.

For the implemented Order revision:

1. Read `docs/26_ORDER_PLATFORM_OVERVIEW.md` through
   `docs/29_ORDER_REVISION_HANDOFF.md` completely.
2. Start from the Exact Resume Point in `docs/29_ORDER_REVISION_HANDOFF.md`.
3. Work through R0-R11 in `docs/28_ORDER_IMPLEMENTATION_PLAN.md` in order.
4. Preserve Task evidence and the `v0.1.0-predeploy` rollback baseline.
5. Follow `docs/17_COMMENTING_GUIDE.md` plus the Order-specific comment rules in docs 28/29.

For the Inventory and Observability revision:

1. Read the complete Order source-of-truth set above and `docs/30_ORDER_REVISION_EVIDENCE.md`.
2. Then read `docs/31_INVENTORY_OBSERVABILITY_REVISION.md` and
   `docs/32_INVENTORY_OBSERVABILITY_HANDOFF.md` completely.
3. Start from the Exact Resume Point in docs 32 and work through P0-P8 in order.
4. Treat docs 31/32 as the target only for this post-release revision; preserve historical evidence.
5. Do not mark a P phase passed without its focused tests and reconciliation evidence.

## Architecture rules

- `contracts/proto` is the gRPC contract source of truth.
- Each service owns its business code and database; do not import business packages across bounded contexts.
- The Gateway is the only public backend edge. Internal gRPC, Kafka, databases, Prometheus, and ClickHouse stay private.
- PostgreSQL is the transactional source of truth. ClickHouse is an eventually consistent analytical projection.
- Task mutation and Outbox insertion must commit in the same PostgreSQL transaction.
- Kafka consumers must be idempotent and commit offsets only after their side effects succeed.
- Redis task cache may fail open to the reader database. Identity refresh session must fail closed.
- Do not put user ID, task ID, event ID, email, or request ID in Prometheus labels.

During the Order revision, replace the Task-specific target with these rules without
weakening the general boundaries above:

- Order creation and Outbox insertion commit in the same Order PostgreSQL transaction.
- Inventory, Payment, Activity, Notification, and Analytics own their state/projections.
- The synchronous request returns `PENDING`; Inventory/Payment outcomes travel through Kafka.
- Consumers are at-least-once and must make business side effects idempotent.
- Do not put order, user, payment, event, email, or request identifiers in metric labels.
- Use additive compatibility-safe migrations through R11; do not delete Task data or volumes.

For Inventory and Observability P0-P8, add these rules without weakening any boundary above:

- PostgreSQL inventory state and movement ledger are authoritative; Redis catalog data is disposable.
- Catalog cache failure fails open to Inventory PostgreSQL; Identity refresh-session failure remains fail closed.
- `OrderConfirmed`, `OrderRejected`, and `OrderCancelled` are canonical terminal outcomes.
- Payment decline remains `CANCELLING` until `InventoryReleased`; do not report terminal cancellation early.
- Preserve ClickHouse V1 history and migrate reads to a typed V2 projection without fabricating outcomes.
- Prometheus stores bounded aggregate metrics; never label metrics with order, user, product, event, request,
  email, SKU, token, or free-form error identifiers.

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

The Task release is already stopped before historical Render Phase 11. The Order revision is
verified through R11 and stops before R12. Do not create paid cloud resources, configure a
custom domain, or deploy to Render without explicit approval.

The Inventory and Observability P0-P8 revision stops after the CI-green
`v0.3.0-inventory-observability-predeploy` handoff and before any Render action.
