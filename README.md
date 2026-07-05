# BenefitCoins

A simulated **multi-tenant core-banking platform** that tracks fractional "benefit coins" kids earn by
doing tasks (e.g. take out the trash = 0.15 coin) and spend on rewards (1.0 coin per reward). It's built
the way a real bank core is structured — as a learning project — and runs as a small SaaS: anyone can
**sign up to open a household**, and each household is fully isolated from every other.

- **TigerBeetle** is the authoritative double-entry **ledger** (balances + immutable postings).
- **Postgres** is the **metadata system-of-record** (tenants, customers, logins, accounts directory,
  task catalog, transaction envelopes, idempotency keys, audit log).
- A **Go** JSON API backend (`/api/v1`) and a separate **React** SPA frontend (marketing landing page +
  self-serve signup + operator/holder apps), installable as a **PWA** with real-time notifications
  (redemption requests, chore verification, bounty alerts, monthly statements) via Server-Sent Events
  and Web Push.

## Core-banking concepts

| Banking term | Here |
|---|---|
| Tenant | A **household**, the isolation boundary. Self-serve signup opens one and its first operator |
| Customer | A party within a household: an **operator** (parent/back-office) or a **holder** (kid) |
| Account | A holder's deposit account, mapped 1:1 to a TigerBeetle account |
| GL accounts | Each household's own **Issuance GL** (mints coins) and **Redemption GL** (sink for spent coins) |
| Authorization hold → settlement | A kid request is a *pending* transfer; an operator *settles* (posts) or *voids* it |
| Minor units | 1 coin = **1000 minor units**; currency code `BNC` (scale 3) |
| Idempotency key | Required on every mutating request; replays return the stored response |
| Audit log | Append-only record of every state-changing action |

Money never goes negative and can't be double-spent: customer accounts carry TigerBeetle's
`debits_must_not_exceed_credits` invariant, checked at **hold-creation** time.

## Balances

- **Settled (current)** = `credits_posted − debits_posted`
- **Available** = settled − outstanding redemption holds (`debits_pending`)
- **Awaiting approval** = pending earnings not yet settled (`credits_pending`)

## Configuration

Copy `.env.example` to `.env` and adjust as needed — it's loaded automatically from the working
directory (falling back to real env vars if both are set, e.g. in a container). Defaults match
`docker-compose.yml`. Key vars: `DATABASE_URL`, `TB_ADDRESS`, `JWT_SECRET`. There is no seeded
operator — households are created via self-serve signup.

**Notifications.** The in-app notification bell and live (SSE) updates always work with zero setup.
Web Push (real OS-level notifications, including when the app isn't open) is optional: run
`make vapid-keys` and set `VAPID_PUBLIC_KEY`/`VAPID_PRIVATE_KEY`/`VAPID_SUBJECT` to enable it — same
best-effort philosophy as statement emailing (`SMTP_*`).

**White-label branding.** The SaaS operator can rebrand the whole product with env vars —
`PRODUCT_NAME`, `SITE_NAME`, `COIN_NAME`, `COIN_NAME_PLURAL`, and `COIN_CODE`. These are display-only
(the internal ledger currency stays `BNC`); the SPA fetches them from the public `GET /api/v1/config`
and renders them everywhere. The signed-in app is a modern online-banking dashboard (sidebar, account
tiles, statements) with inline explanations of digital-banking concepts — balance types, authorization
holds, settlement, the general ledger, and a double-entry posting view — plus a **Learn** glossary, so
it doubles as a teaching tool.

## Tests

```bash
make test               # unit tests (money, auth, notify, config) — no infra needed

# full end-to-end test (HTTP + Postgres + TigerBeetle): start infra first
make pg                 # terminal A
make tb                 # terminal B
make test-integration   # terminal C
```

The integration test (`internal/api/integration_test.go`) walks the entire lifecycle: sign up a
household, onboard a holder, submit a task (earn hold → settled), reject an underfunded redemption,
accumulate a coin, redeem (hold → settled), verify available/settled/awaiting balances at each step,
confirm idempotent replay returns the same transaction, check operator-only authorization, and assert a
second household cannot see or touch the first's data.

## API

The full surface is documented in [`openapi.yaml`](./openapi.yaml). Highlights:

```
POST /api/v1/auth/signup                                       # open a household + first operator
POST /api/v1/auth/login | /auth/refresh | /auth/logout
GET  /api/v1/accounts | /accounts/{id} | /accounts/{id}/balance | /accounts/{id}/transactions
POST /api/v1/accounts/{id}/earnings        # holder: submit a completed task (earn hold)
POST /api/v1/accounts/{id}/redemptions     # holder: request a 1-coin reward (redeem hold)
GET  /api/v1/tasks   POST/PATCH /api/v1/tasks[/{id}]            # operator manages catalog
POST /api/v1/customers   GET /api/v1/customers[/{id}]           # operator onboards customers
GET  /api/v1/transactions?status=pending                       # operator work queue
POST /api/v1/transactions/{id}/settle | /void                  # operator approves/rejects
GET  /api/v1/audit                                             # operator audit log
GET  /api/v1/notifications                                     # in-app feed + unread count
GET  /api/v1/notifications/stream                               # live updates (Server-Sent Events)
POST /api/v1/push/subscribe   DELETE /api/v1/push/subscribe    # Web Push subscription management
```
