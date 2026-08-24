# Order Platform Revision Evidence

> Updated: 2026-08-24 (Asia/Bangkok)

This ledger records the current Order-first draft without rewriting the Task baseline. The baseline
remains `main` / `v0.1.0-predeploy`. Existing `v0.2.0-order-predeploy` is preserved as-is; it is
not overwritten by this revision.

## Current phase closeout

| Phase | Result | Evidence |
|---|---|---|
| R0 | Passed | `codex/order-platform-revision`; uncommitted draft audited and preserved |
| R1-R6 | Passed | Existing Order contracts/services/projections plus focused Go tests |
| R7 | Passed locally | Frontend lint/test/build; Playwright browser E2E 3/3 |
| R8 | Passed locally | Compose Grafana datasource, Prometheus targets and dashboard checks |
| R9 | Passed locally | `make test`, `make vet`, `make build`, `make compose-config`, frontend lint/test/build |
| R10 | Passed locally and CI | Compose acceptance exit 0; CI run `32700681239` passed |
| R11 | Passed | Commit `dbb326c` pushed; CI run `32701188316` passed all four jobs; `v0.2.1-order-predeploy` created; existing tags preserved |

## R7 browser evidence

Command:

```powershell
Push-Location frontend
$env:PLAYWRIGHT_BASE_URL='http://127.0.0.1:3000'
$env:E2E_REAL_BACKEND='1'
npm run e2e
Pop-Location
```

Result: **3 passed**

- Landing explains Fiber REST, gRPC, Kafka and ClickHouse boundaries.
- Member creates a success order, payment-declined order and zero-stock order through the UI.
- Member sees persisted notification projection and marks all read.
- Admin opens Orders, Inventory, Payments, Activity, Analytics and System routes.
- Login sets the refresh session as an HttpOnly cookie, so a full page reload can restore the session.

## R10/R8 Compose evidence

Command:

```powershell
$env:SKIP_BUILD='1'
$env:RUN_BROWSER_E2E='1'
pwsh -NoProfile -File scripts/compose-acceptance.ps1
```

Result: **exit 0**

Observed output:

```text
compose acceptance ok task_events=13 order_events=79
success=cfae93d6-1eba-42c3-8907-53bc0f2efa84
decline=80ec47c6-de60-4073-8a20-347970c430a9
out_of_stock=643e3b7c-abf4-483c-a6d0-ba9d9e9ff394
```

The acceptance covered REST → gRPC → PostgreSQL/outbox → Kafka, all three Order outcomes,
idempotency, admin projections, notifications, ClickHouse order events, Grafana health/datasource,
Prometheus targets and browser E2E. The script stopped containers in `finally`; it did not remove
named volumes or Task data.

## R9 quality commands

```powershell
make test
make vet
make build
docker compose -f deploy/docker-compose.yml config --quiet
```

The verified runtime commit is `dbb326cdf00b961dac82bf5be23ff21feabaf634`; CI run `32701188316` passed all four jobs. Tag `v0.2.1-order-predeploy` points to this commit (annotated tag object `113f1413`).

## Known limitations and rollback

- PostgreSQL and ClickHouse local volumes are retained and may contain prior Task/Order fixtures.
- Existing `v0.2.0-order-predeploy` points to the earlier verified revision and must not be retagged.
- No Render resource, custom domain or paid cloud service was created.
- Rollback remains `v0.1.0-predeploy`; no Task schema/source/volume was removed.
