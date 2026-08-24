# Order Platform Revision Evidence

> Updated: 2026-08-24 (Asia/Bangkok)

This ledger records the revision evidence without rewriting the Task baseline. The baseline remains
`main`/`v0.1.0-predeploy`; all revision runtime commits are on `codex/order-platform-revision`.

## Phase closeout

| Phase | Result | Evidence |
|---|---|---|
| R0 | Passed | Branch created; baseline tag preserved; dirty design documents retained |
| R1-R4 | Passed | Contract/state/consumer tests; commits `06cce7a`, `3ef690c`, `0b8f635`, `ff890f9`, `89859ad`, `aae8de0` |
| R5-R6 | Passed | Activity/notification and ClickHouse projection tests; commits `7eb048a`, `b01ab0b`, `93a8b85` |
| R7-R8 | Passed | Frontend lint/test/build and observability configuration; commits `94bdd30`, `2349ea4`, `ba2a24f`, `264b1d9` |
| R9 | Passed | `scripts/test.ps1`, `scripts/vet.ps1`, `scripts/build.ps1`, `docker compose ... config`; commit `8ee2e2b` |
| R10 | Passed | `SKIP_BUILD=1 pwsh -NoProfile -File scripts/compose-acceptance.ps1` exited 0; commit `1304c6b` |
| R11 | In progress | Branch pushed to `origin`; GitHub Actions run `32693374593` |

## R10 scenario result

`compose-acceptance.ps1` verified:

- REST Gateway -> gRPC Order/Inventory quote -> PostgreSQL outbox returns `PENDING`.
- Kafka Inventory reservation + Payment success reaches `CONFIRMED`.
- Payment decline reaches `CANCELLED` and emits release/compensation.
- Zero-stock shirt reaches `REJECTED`.
- Repeating one `Idempotency-Key` returns the same order id.
- Activity and persisted notification projections become visible by polling.
- ClickHouse order events and admin summary include created, confirmed, cancelled, and rejected outcomes.
- Swagger/OpenAPI, Grafana health/ClickHouse datasource, and all Prometheus targets are ready.

The acceptance command reported `task_events=7` and `order_events=30` on the retained local volumes.

## Commands

```text
pwsh -NoProfile -File scripts/test.ps1                 # passed
pwsh -NoProfile -File scripts/vet.ps1                 # passed
pwsh -NoProfile -File scripts/build.ps1               # passed
docker compose -f deploy/docker-compose.yml config   # passed
$env:SKIP_BUILD='1'; pwsh -NoProfile -File scripts/compose-acceptance.ps1  # passed, exit 0
```

## Known local-volume note

The existing PostgreSQL and ClickHouse named volumes predated the revision migrations, so their
one-time init hooks did not rerun. No volume or data was deleted. Missing `order_db`,
`notification_db`, and `analytics.order_events` were created additively for acceptance; fresh
volumes use the committed init/migration files. A deployment migration runner is still recommended
before any non-local rollout.

## Rollback and stop point

Rollback remains `v0.1.0-predeploy`; no Task schema/source/volume was removed. R12 Render deployment
is deliberately not started and no paid Render resource was created.
