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
