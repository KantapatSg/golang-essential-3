# Inventory and Observability Revision Evidence

> Started: 2026-08-24 (Asia/Bangkok)
>
> P0 status: **Passed — baseline and reconciliation fixtures locked**

This ledger is additive to the verified Task/Order evidence. Docs 31/32 remain uncommitted design
inputs and were not overwritten. No source, schema, topic, tag, database, or Docker volume was
deleted during P0.

## P0 — Baseline, contracts and fixture reconciliation

**Commit / branch:** baseline `da86f7b`; branch `codex/inventory-observability-revision`.

**Rollback tags verified:**

- `v0.1.0-predeploy` -> `31f11bd`
- `v0.2.0-order-predeploy` -> annotated tag object `d986647` (runtime `8a23279`)
- `v0.2.1-order-predeploy` -> `113f141` (runtime `dbb326c`)

**Commands and results:**

```text
docker compose -f deploy/docker-compose.yml config --quiet    # passed
docker volume ls                                                # read-only inventory; volumes preserved
git ls-remote --tags origin ...                                 # all three rollback tags present
graphify . --code-only --no-viz                                 # 1,293 nodes / 2,490 edges
graphify cluster-only . --no-viz                               # graph report generated
```

The initial full graphify pass reported 46 documentation files requiring semantic extraction and
no configured LLM backend; the code-only AST pass was used for the architecture map without
reading or exposing secrets.

**Live reconciliation fixture (read-only):**

```text
ClickHouse analytics.order_events FINAL:
  OrderCreated=32, InventoryReserved=23, InventoryRejected=8,
  PaymentCompleted=15, PaymentFailed=8,
  InventoryReleaseRequested=8, InventoryReleased=8

ClickHouse tables:
  analytics.order_events  ReplacingMergeTree  ORDER BY (order_id,event_id)
  analytics.task_events   ReplacingMergeTree  ORDER BY (event_date,event_type,task_id,event_id)

Gateway catalog: 3 seeded products
Prometheus active targets: 10, all health=up
Redis keyspace: Identity session:* keys observed; no catalog cache keys observed
```

**Locked P0 discrepancies:**

1. V1 ClickHouse contains Payment outcomes but no canonical terminal `OrderConfirmed`,
   `OrderRejected`, or `OrderCancelled` events; no historical terminal events will be fabricated.
2. Inventory products, balances, and reservations are currently memory-owned; restart durability
   and Add Stock require the P2 PostgreSQL boundary.
3. Redis currently contains Identity sessions only; catalog cache behavior is unimplemented.
4. V1 analytics uses legacy event semantics and has no typed V2 table or confirmed revenue query.
5. Prometheus targets are healthy, but business transaction/cache metrics are not yet emitted.
6. Grafana Business Analytics remains a Task projection and is not evidence for Order V2 analytics.

**Known limitation:** retained named volumes contain historical acceptance data. Counts above are
baseline diagnosis only, not a clean-run business fixture. P7 will use a unique `run_id` and
reconcile only events produced by that run; volumes remain intact.

**Next phase:** P1 canonical terminal events and compensation state.

## P1 — Canonical terminal events and compensation

**Status:** **Passed — focused contract/state/service tests**.

Implemented on the working branch (without touching the preserved design inputs):

- Added one shared set of Order/Inventory event-name constants.
- Added `CANCELLING` and the explicit `PaymentFailed -> CANCELLING -> InventoryReleased ->
  CANCELLED` transition. Payment success emits `OrderConfirmed`; out-of-stock emits
  `OrderRejected`; confirmed orders emit `InventoryConsumed`.
- Order consumers now use `FetchMessage` and commit only after the state side effect and publish
  succeed. PostgreSQL mode records the processed event and terminal outbox row in the same
  transaction with a row lock; memory mode retains event-id idempotency for focused tests.
- Inventory handles `OrderConfirmed` consumption idempotently and emits `InventoryConsumed`.
- Activity already records the complete envelope; persisted Notification message mapping now
  covers release, rejection, cancellation, and consumption milestones.
- Added additive `order_processed_events` migration; no existing table/topic/data/volume was
  removed.

**Focused verification:**

```text
go test ./contracts ./services/order-service/... ./services/inventory-service/...   # passed
go test ./services/notification-service/... ./services/activity-service/...         # passed
```

P1 does not claim historical V1 terminal events: existing ClickHouse rows remain unchanged and
canonical terminal events are produced only from newly processed live outcomes.

## P2 — Durable Inventory and Stock Operations

**Status:** **Focused implementation complete; Compose persistence gate remains for P7.**

- Inventory protobuf now exposes `ListInventory`, `AdjustStock`, and `ListStockMovements`.
- Inventory-owned PostgreSQL schema is additive (`inventory_products`, stock movements,
  reservations, processed events, and outbox). The Compose stack provisions an additive
  `inventory_db`; existing databases and volumes are untouched.
- Seed rows are insert-if-absent and never overwrite an operator adjustment. Stock uses
  `on_hand - reserved` as the available projection, with row locking for signed adjustments
  and event reservations. Negative available balances are rejected.
- Adjust Stock is admin-only at both Gateway and Inventory metadata boundaries, requires an
  Idempotency-Key, hashes the request payload, and records the movement plus `StockAdjusted`
  outbox row transactionally.
- Reserve, release, and consume have PostgreSQL transaction paths and durable processed-event /
  outbox records; the existing in-memory path remains for isolated tests without a DSN.

**Focused verification:**

```text
powershell -ExecutionPolicy Bypass -File scripts/test.ps1   # passed with SKIP_FRONTEND=1
docker compose -f deploy/docker-compose.yml config --quiet # passed before/after P2 config
```

The running stack was not restarted or re-seeded during P2 to avoid mutating retained baseline
fixtures. P7 will rebuild only service images and run a unique acceptance `run_id`, then verify
restart persistence against the additive Inventory database.

## P3 — Redis catalog cache

**Status:** **Focused implementation complete; live/browser cache proof remains for P7.**

- Inventory now uses a versioned catalog key (`catalog:v1:list:all`) with a 60-second cache-aside
  TTL. PostgreSQL/memory remains the source of truth; cache errors fall back to the source.
- gRPC responses include `x-cache-status` (`HIT`, `MISS`, or `BYPASS`), and Gateway exposes it as
  `X-Cache-Status` with CORS `ExposeHeaders` for the portfolio UI.
- Stock adjustment/reserve/release/consume paths invalidate the catalog only after their mutation
  transaction succeeds. Redis is optional for catalog availability and is not coupled to Identity
  session fail-closed behavior.
- Low-cardinality Prometheus counters cover hits, misses, bypasses, errors, and invalidations;
  no order/customer/token values enter cache keys or metric labels.

**Focused verification:**

```text
$env:SKIP_FRONTEND="1"; powershell -ExecutionPolicy Bypass -File scripts/test.ps1  # passed
docker compose -f deploy/docker-compose.yml config --quiet                         # passed
```

P7 will exercise MISS -> HIT, post-adjustment invalidation -> MISS, and Redis-down BYPASS against
a unique acceptance run while retaining the existing Redis volume.

## P4 — ClickHouse Analytics V2

**Status:** **Schema/query/worker focused implementation complete; end-to-end event reconciliation remains for P7.**

- Added additive `analytics.order_events_v2`; V1 history and `analytics.order_events` remain
  unchanged. V2 uses typed UUID/DateTime/Int64/Array columns, LowCardinality dimensions,
  monthly partitions, and `ReplacingMergeTree(ingested_at)` with
  `ORDER BY (event_date,event_type,order_id,event_id)`.
- The analytics worker projects only received live envelopes into V2 and preserves event ID,
  schema version, payload, environment, and optional acceptance `run_id`. It does not synthesize
  terminal events from historical Payment rows. Kafka offsets commit only after ClickHouse insert.
- Order Summary/Funnel now read V2 canonical events. Revenue sums `amount_minor` only for
  `OrderConfirmed`; PaymentCompleted is not treated as confirmed revenue. Queries use the
  `event_date` ORDER BY prefix, bounded grouping, and `FINAL` for immediate ReplacingMergeTree
  correctness.

**ClickHouse evidence (live additive schema, no data deletion):**

```text
DESCRIBE analytics.order_events_v2  # typed schema created successfully
system.tables: ReplacingMergeTree / toYYYYMM(event_date) /
  event_date,event_type,order_id,event_id
EXPLAIN indexes=1 ... WHERE event_date >= toDate(now())  # bounded query plan; empty clean date range
```

**Rules applied:** `schema-pk-plan-before-creation`, `schema-pk-cardinality-order`,
`schema-pk-prioritize-filters`, `schema-types-native-types`, `schema-types-lowcardinality`,
`schema-partition-low-cardinality`, `insert-mutation-avoid-update`, and
`insert-optimize-avoid-final`. V2 uses `FINAL` only in bounded reads; no `OPTIMIZE FINAL` or
mutating UPDATE was introduced.

**Focused verification:**

```text
go test ./services/analytics-service/... ./services/analytics-worker/...  # passed
docker exec deploy-clickhouse-1 clickhouse-client ... 003_order_events_v2.sql # passed
```

## P5 — Prometheus transaction and cache metrics

**Status:** **Passed focused metric exposition checks.**

- Order `/metrics` now exposes low-cardinality request, quote-error, transaction, and
  transaction-error counters.
- Inventory `/metrics` exposes transaction, adjustment, cache hit/miss/bypass/error/invalidation
  counters. Labels do not contain order IDs, customer IDs, idempotency keys, or tokens.
- Gateway already exposes bounded HTTP request and duration counters; Analytics and the worker
  retain bounded request/query/retry/DLQ/lag metrics.

**Focused verification:**

```text
go test ./services/order-service/... ./services/inventory-service/... # passed
curl http://localhost:9106/metrics                              # service_ready + inventory counters
curl http://localhost:9107/metrics                              # service_ready + order counters
```

The existing running images predate these counters; P7 will rebuild only application images and
capture live exposition plus Prometheus target/series evidence.

## P6 — Grafana and frontend portfolio UX

**Status:** **Passed frontend/Grafana focused gates.**

- The member Catalog now shows the live `X-Cache-Status` badge (HIT/MISS/BYPASS) beside the
  asynchronous stock explanation. The admin Inventory surface now supports signed stock
  adjustments with idempotency, displays available balances, reservation state, and the durable
  stock movement ledger.
- Analytics copy identifies ClickHouse V2 and confirmed-event revenue semantics. Existing polling
  Order, Activity, Notification, Payment, and System flows remain intact.
- Grafana now provisions six dashboards: Platform Overview, Event Pipeline, Business Analytics
  (V2), Order Transactions, Redis & Inventory, and ClickHouse V2. The former Task-only business
  query was replaced by typed V2 event/revenue queries; Task dashboards remain compatibility-only.

**Focused verification:**

```text
npm test -- --run   # 8 tests passed
npm run lint        # passed
npm run build       # passed; Vite production bundle generated
dashboard JSON ConvertFrom-Json validation # six dashboards valid
```

## P7 — Compose, API, browser, and observability acceptance

**Status:** **Passed — local acceptance completed against retained named volumes.**

The acceptance run rebuilt only the affected application images and created the additive
`inventory_db` database because it was absent from the retained Postgres volume. No database,
schema, topic, or Docker volume was dropped or reset.

**Compose/readiness evidence:**

```text
docker compose -f deploy/docker-compose.yml config --quiet              # passed
docker compose ... build inventory-service order-service api-gateway \
  analytics-service analytics-worker frontend                         # passed
docker compose ... up -d inventory-service order-service api-gateway \
  analytics-service analytics-worker frontend                          # passed
Gateway /health/live, /health/ready, /openapi.yaml                     # HTTP 200
Frontend :3000/, Grafana :3001/api/health, Prometheus /-/ready         # HTTP 200
Prometheus active targets: 10; healthy targets: 10
```

**Live business flow evidence:**

- Catalog returned `X-Cache-Status: MISS` then `HIT`; after a signed admin adjustment, the next
  catalog read returned `MISS` then `HIT`, proving mutation invalidation and repopulation.
- The same stock adjustment Idempotency-Key returned the same movement with `replayed: true` and
  only one movement row. The admin movement ledger was readable after the transaction.
- Three new member orders reached `CONFIRMED`, `CANCELLED`, and `REJECTED`. Their Activity
  timelines contained canonical events, including `OrderConfirmed`/`InventoryConsumed` and
  `PaymentFailed`/`InventoryReleaseRequested`/`InventoryReleased`/`OrderCancelled`.
- Persisted in-app Notifications contained the order-created, payment, inventory, and terminal
  projections for those order IDs.
- ClickHouse V2 contained only received live envelopes for the acceptance order IDs. A separate
  acceptance order was processed with `RUN_ID=p7-accept-25690824160755`; its V2 rows included
  `OrderCreated`, `InventoryReserved`, `PaymentCompleted`, `OrderConfirmed`, and
  `InventoryConsumed` with that run ID. No historical Payment rows were converted to terminal
  events.
- Compose acceptance now generates a unique `compose-accept-*` `RUN_ID`, captures an RFC3339
  acceptance window, and polls the ranged V2 Order Summary for up to 120 seconds until all three
  terminal outcomes reconcile. This fixes the reproduced CI race where the first summary read
  arrived before the ClickHouse worker projection; it does not lower the outcome thresholds.
- The first local rerun exposed a host-culture edge case (`yyyy` rendered as Buddhist year `2569`);
  acceptance timestamps now use invariant culture before the RFC3339 query is sent. The corrected
  run passed with `run_id=compose-accept-a30f02d456e44b1eb1d5bdf327d2853f` and exactly three
  run-scoped V2 order IDs.
- Live service metrics exposed `service_ready 1`, Inventory cache/transaction counters, and Order
  transaction counters. Prometheus reported all 10 configured targets healthy; Grafana health was
  `{"database":"ok","version":"11.5.2"...}`.

**Browser evidence:**

```text
SKIP_BUILD=1 ./scripts/compose-acceptance.ps1                         # passed; unique run ID/window
PLAYWRIGHT_BASE_URL=http://localhost:3000 E2E_REAL_BACKEND=1 npm run e2e
3 passed (landing/login, member success/decline/out-of-stock/notifications,
admin operations navigation)
```

The first attempt against the Vite development origin (`127.0.0.1:4173`) was intentionally
retained as a harness finding: that origin has no API proxy. The Compose frontend origin uses the
Nginx `/api` proxy and passed all three browser scenarios.

**Known acceptance limitations:** retained volumes include earlier Task/Order fixtures, so
aggregate counts are not clean-room counts. Acceptance assertions were scoped to newly-created
order IDs and the unique run ID above. Redis-down `BYPASS` remains covered by focused service
tests; the retained Compose Redis service was not stopped during this acceptance run.

## P8 — Release handoff gates

**Status:** **Local gates and public CI passed; pre-deploy tag is the final release action.**

Final local checks on the candidate branch:

```text
make test                                      # passed: all Go packages + 8 frontend tests
make vet                                       # passed after dcf81c7 replay clone fix
make build                                     # passed: Go binaries + Vite production bundle
make compose-config                            # passed
powershell -ExecutionPolicy Bypass -File scripts/smoke-test.ps1  # passed
Push-Location frontend; npm ci; npm run lint; npm test; npm run build; Pop-Location  # passed
fallback secret scan (gitleaks unavailable)    # no private-key/token signatures found
```

`npm audit --audit-level=high` reports two existing moderate React Router advisories and proposes
a breaking `react-router-dom` 7 upgrade; no forced upgrade was applied in this pre-deploy scope.
This remains a known limitation for the release report. No credential, session token, or private
key was added to the repository.

The runtime fix and evidence are committed separately (`dcf81c7`, `d7bebf5`). The preserved dirty
inputs are intentionally excluded from release staging: `AGENTS.md`, untracked design handoffs
`docs/31_INVENTORY_OBSERVABILITY_REVISION.md` and `docs/32_INVENTORY_OBSERVABILITY_HANDOFF.md`,
and generated `graphify-out/`. They contain no runtime fix. Public branch/CI and
`v0.3.0-inventory-observability-predeploy` must point to the same final commit; Render remains
HOLD and no cloud resource is created.

Public CI verification for the final candidate:

```text
commit: d01f1e32b706242a22717fe9193055ecd6917316
run:    32713281493
jobs:   backend, frontend, compose, integration — passed
url:    https://github.com/KantapatSg/golang-essential-3/actions/runs/32713281493
```

The final tag must be created on this exact CI-green commit. Existing rollback tags remain
unchanged; Render deployment remains HOLD.
