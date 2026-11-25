<a id="sytue-one--pruebas-de-carga-y-rendimiento-load"></a>
# Sytue One — load and performance tests (`load/`)

**[k6](https://k6.io/)** scripts (Grafana, Apache-2.0) to measure the
**performance** of Sytue One's key Go services against the contract's
SLOs. Performance is a **core principle** of the suite and a
*Definition of Done* criterion: these scripts run in CI with **thresholds that break the
build** when they are not met ([`docs/CONVENTIONS.md`](../docs/CONVENTIONS.md) §0.8,
full strategy in [`docs/operations.md#4-rendimiento-y-seo`](../docs/operations.md#4-rendimiento-y-seo)).

> License: code Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr) (`MIT`).
> k6 is an external tool (Apache-2.0); it is not included in the artifacts.

<a id="slo-objetivos-de-nivel-de-servicio"></a>
## SLO (service level objectives)

| Metric | Target | k6 threshold |
|---|---|---|
| API latency **p95** | **< 200 ms** | `http_req_duration: p(95)<200` |
| API latency **p99** | < 500 ms | `http_req_duration: p(99)<500` |
| Error rate | < 1 % | `http_req_failed: rate<0.01` |
| Passing checks | > 99 % | `checks: rate>0.99` |

All configurable via environment: `-e SLO_P95_MS=150 -e SLO_ERROR_RATE=0.005`.

<a id="estructura"></a>
## Structure

```
load/
├── lib/
│   ├── config.js     # bases por servicio, SLO, umbrales y perfiles de carga
│   └── checks.js     # aserciones (estado, latencia p95, JSON)
├── id/               # Sytue ID  (OIDC/JWKS — hot path de validación de tokens)
│   ├── smoke.js      # humo: 1 VU, endpoints públicos + salud
│   └── load.js       # carga: discovery/jwks/userinfo bajo concurrencia
├── drive/            # Sytue Drive (REST: listado/búsqueda con keyset)
│   ├── smoke.js
│   └── load.js
└── chat/             # Sytue Chat  (REST + WebSocket de tiempo real)
    ├── smoke.js
    ├── load.js       # REST: teams → channels → messages (keyset)
    └── ws.js         # WS: N conexiones concurrentes (fan-out/presencia)
```

**id / drive / chat** are covered because they are the services with the most
demanding load pattern: token validation (all suite traffic goes through Sytue ID),
read-heavy listings (Drive) and real time (Chat). The same pattern is
replicated for the other services by copying a directory and adjusting the routes.

<a id="tipos-de-prueba"></a>
## Test types

- **smoke** — minimal load (1 VU, 10 iterations). Verifies that the endpoints
  respond and meet the SLO. It runs on **every deployment** and as a quick
  *gate* in CI before the load test.
- **load** — sustained load with a ramp (`ramping-vus`): ramp-up → plateau of `VUS`
  → ramp-down. Detects latency regressions under realistic concurrency.

Configurable profile: `-e VUS=100 -e RAMP=30s -e DURATION=5m`.

<a id="requisitos"></a>
## Requirements

Install k6 (not versioned): `brew install k6` · `apt install k6` · or the
`grafana/k6` image. Requires the services to be running (`make dev-up` brings up the local
suite on `*.sytue.localhost`).

<a id="uso"></a>
## Usage

```bash
# Humo local (dev, *.sytue.localhost). Endpoints públicos, sin token.
k6 run load/id/smoke.js
k6 run load/drive/smoke.js
k6 run load/chat/smoke.js

# Carga contra staging, con token de Sytue ID para rutas autenticadas.
k6 run -e ID_URL=https://id.staging.sytue.eu \
       -e VUS=100 -e DURATION=5m load/id/load.js

k6 run -e DRIVE_URL=https://drive.staging.sytue.eu \
       -e ACCESS_TOKEN="$TOKEN" -e VUS=80 load/drive/load.js

# Tiempo real: 200 conexiones WebSocket concurrentes contra Chat.
k6 run -e CHAT_URL=wss://chat.staging.sytue.eu \
       -e ACCESS_TOKEN="$TOKEN" -e VUS=200 -e DURATION=2m load/chat/ws.js
```

<a id="variables-de-entorno"></a>
### Environment variables

| Variable | Description | Default |
|---|---|---|
| `ID_URL` / `DRIVE_URL` / `CHAT_URL` | Service base URL (no trailing slash) | `https://<svc>.sytue.localhost` |
| `ACCESS_TOKEN` | Sytue ID JWT for authenticated routes | *(empty)* |
| `VUS` | Virtual users at the plateau | `50` |
| `RAMP` / `DURATION` | Ramp / plateau duration | `30s` / `2m` |
| `SLO_P95_MS` / `SLO_P99_MS` / `SLO_ERROR_RATE` | SLO thresholds | `200` / `500` / `0.01` |

> **Secrets:** `ACCESS_TOKEN` is **never** versioned; it is injected in CI from the
> secrets manager. The scripts contain no credentials.

<a id="rendimiento-del-backend-go-lo-que-estas-pruebas-verifican"></a>
## Go backend performance (what these tests verify)

Load tests are the **safety net** of a backend designed for the
SLO. Performance levers in the services (contract §0.8):

- **`pprof`** — each service exposes `net/http/pprof` on an **internal** (non-public)
  diagnostics port for CPU/heap/goroutine profiling.
  On a regression detected by k6: `go tool pprof http://<svc>:<diag>/debug/pprof/profile`.
- **PostgreSQL** — indexes and `EXPLAIN (ANALYZE, BUFFERS)` on hot
  queries; **keyset pagination** (no `OFFSET`) in the Drive/Chat listings;
  **no N+1**; materialized views where worthwhile; right-sized `pgx` pool.
- **Cache where it measurably helps** — `Cache-Control` on Sytue ID discovery/JWKS; cache
  on Postgres by default and **optional Valkey** (§0.1) for Chat fan-out/presence,
  sessions and *rate limiting* if it brings a measurable improvement.
- **HTTP** — HTTP/2, `keep-alive`, br/gzip compression; safe *timeouts*.

## CI

In CI, `smoke` runs on every PR (quick gate) and `load` on the main branch /
nightly against an ephemeral environment. k6 thresholds return a non-zero exit code
when breached, breaking the build. The JSON summary is archived
(`k6 run --summary-export`) to track the p95 trend over time.
