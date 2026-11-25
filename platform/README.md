<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

<a id="sytue-one--núcleo-go-platform"></a>
# Sytue One — Go core (`platform/`)

Shared libraries of the **single Go module** `git.sytue.eu/sytue/one` (with `go.mod`
at the repo **root**). All of the suite's services (`services/<svc>/`)
reuse them to keep identical behavior in config, logging, HTTP,
data access, identity, events and storage.

> Canonical stack (see [`docs/CONVENTIONS.md`](../docs/CONVENTIONS.md) v4):
> **Go 1.26 + PostgreSQL 18** as the **single** datastore and source of truth (state,
> queues and cache on Postgres; **Valkey** only optional where §0.1 measures it; **never
> Redis**), blobs in **S3 Object Storage** (MinIO dev / Scaleway prod), identity
> via **Sytue ID** (OIDC). Third-party dependencies **permissive only**
> (MIT/BSD/Apache). Product code **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)** (SPDX header per file).
>
> **Security and privacy by default, and hardened** (CONVENTIONS §0.7): these
> libraries implement deny-by-default, least privilege, multi-tenant RLS,
> security headers + CSP + HSTS, CSRF, rate limiting, HttpOnly/Secure/SameSite
> cookies, validation/sanitization, parameterized queries,
> correlated OTel observability and logs **without PII** (GDPR art. 25).

<a id="paquetes-platformpkg"></a>
## Packages (`platform/pkg/*`)

| Package        | Purpose |
|----------------|-----------|
| `config`       | **Typed** configuration loading from the environment (12-factor) with prefixes, defaults, validation and error accumulation. `Base` common to all services. |
| `log`          | Structured logger with `log/slog` (JSON in prod, text in dev). Propagation via `context.Context`. |
| `telemetry`    | **Correlated** observability (traces + metrics + logs): W3C Trace Context spans (`traceparent` propagation), aggregated counters/histograms, **OTLP/HTTP exporter** to the collector (without pulling in the OTel SDK), injection of `trace_id`/`span_id` into the logger and **protected pprof** (deny-by-default, requires an authorization Guard). |
| `httpx`        | **chi** router with a middleware chain that is **SECURE by default**: request-id, logging, recovery, **security headers** (configurable CSP, HSTS only over TLS, nosniff, X-Frame-Options/frame-ancestors, Referrer-Policy, Permissions-Policy, COOP/CORP/COEP), restrictive **CORS** (allowlist), **body limit**, token-bucket **rate limiting**, double-submit **CSRF**, timeout. Helpers for **HttpOnly/Secure/SameSite=strict cookies**, **canonical JSON** responses/errors, body decoding+validation and `/healthz`/`/readyz` probes. Server with *graceful* shutdown and safe timeouts. |
| `pg`           | **pgx** pool with limits, transaction helpers (`InTx`, `InTenantTx`, `InRLSTx`), **migration runner**, tenancy/**RLS** (`SetTenant`/`WithTenant` via `set_config('app.tenant', …, true)`, `TenantRLSPolicySQL` policy generator with deny-by-default `FORCE ROW LEVEL SECURITY`) and a **shared rate limiter** on Postgres. Queries are always **parameterized**. |
| `objectstore`  | MinIO/Scaleway-compatible **S3** client (minio-go): put/get/delete, presigned URLs, *health*. Per-tenant isolation through **key prefixes**. |
| `validation`   | Reusable input **validation and sanitization**: accumulating `Validator` (required/len/email/slug/allowlist/regex), text sanitization (control chars, BiDi/zero-width anti *Trojan Source*), truncation by runes. Integrates with `httpx.FailValidation` (422 with per-field details). |
| `authn`        | **OIDC** verifier for Sytue ID JWTs: **JWKS** with caching/rotation (jwx), signature/issuer/audience/**exp/nbf/iat** validation, **algorithm allowlist** (asymmetric; never `none` or HS*), mandatory `kid`, claim extraction **without PII** (`Claims.LogValue`), `RequireAuth` / `RequireRole` / `RequireTenant` / `RequireScope` middlewares. |
| `events`       | **Event bus on PostgreSQL** (no Redis): transactional **outbox** pattern + **LISTEN/NOTIFY**. `Dispatcher` (*at-least-once* delivery with `FOR UPDATE SKIP LOCKED` and fallback polling) and `Subscriber` (ephemeral real-time NOTIFY). |
| `tenancy`      | **Tenant** (organization) context. Canonical slug ⇄ `org/<slug>` (Sytue ID) ⇄ `tenant-<slug>` (Kapsule) ⇄ S3 prefix. |

<a id="convenciones"></a>
## Conventions

- **Layers per service:** `cmd/<svc>/main.go` + `internal/{http,domain,store}`.
- **Proprietary SPDX header** in every Go file (`MIT`).
- **`context.Context`** in all I/O; errors wrapped with `%w`.
- Per-service versioned **migrations** (`db/migrations/*.up.sql` / `*.down.sql`),
  embedded with `//go:embed` and applied with `pg.MigrateUp`.

<a id="ejemplo-de-cableado-de-un-servicio"></a>
## Example of wiring a service

```go
// services/<svc>/cmd/<svc>/main.go
ctx := context.Background()

base, _ := config.LoadBase("SYTUE_DRIVE", "sytue-drive")
logger := log.New(log.Options{Level: base.LogLevel, Format: base.LogFormat,
    Service: base.ServiceName, Version: base.Version})
slog.SetDefault(logger)
ctx = log.Into(ctx, logger)

pool, _ := pg.Connect(ctx, pg.Config{DSN: dsn})
defer pool.Close()
_ = pg.MigrateUp(migrationsFS, ".", dsn)

verifier, _ := authn.NewVerifier(ctx, authn.VerifierConfig{
    Issuer: "https://id.sytue.eu", Audience: "sytue-drive"})

// Observabilidad correlacionada (OTLP hacia el colector de la suite).
exp, _ := telemetry.NewOTLPHTTPExporter(telemetry.OTLPHTTPConfig{Endpoint: otlpEndpoint})
tp, _ := telemetry.NewProvider(ctx, telemetry.Config{
    Service: base.ServiceName, Version: base.Version, Env: string(base.Env), Exporter: exp})
defer tp.Shutdown(ctx)
telemetry.SetLogInjector(log.With) // inyecta trace_id/span_id en los logs

// Rate-limiting compartido sobre Postgres (sin Redis/Valkey).
limiter := pg.NewRateLimiter(pool, pg.RateLimiterConfig{PerSecond: 20, Burst: 40})

// Router SEGURO por defecto: cabeceras endurecidas + CORS + rate-limit + CSRF.
r := httpx.NewRouter(httpx.RouterOptions{
    Logger: logger, RequestTimeout: 30 * time.Second,
    CORS:      &httpx.CORSConfig{AllowedOrigins: []string{"https://drive.sytue.eu"}, AllowCredentials: true},
    RateLimit: &httpx.RateLimitConfig{Limiter: limiter},
    CSRF:      &httpx.CSRFConfig{CookieDomain: ".sytue.eu"},
})
r.Use(tp.Middleware) // traza cada petición y correlaciona logs

httpx.MountHealth(r, base.Version, 3*time.Second,
    httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz})

// pprof protegido (deny-by-default): solo admins autenticados.
_ = telemetry.RegisterPprof(r, telemetry.PprofConfig{
    Guard: func(h http.Handler) http.Handler { return verifier.RequireAuth(verifier.RequireRole("admin")(h)) }})

r.Group(func(pr chi.Router) {
    pr.Use(verifier.RequireAuth)               // exige JWT de Sytue ID
    pr.Use(verifier.RequireTenant)             // exige organización activa (RLS)
    pr.Use(verifier.RequireRole("member"))     // autorización por rol
    pr.Get("/v1/files", listFiles)             // tenant disponible en el contexto
})

srv := httpx.NewServer(httpx.ServerConfig{Addr: base.HTTPAddr, Handler: r,
    ShutdownTimeout: base.ShutdownTimeout})
_ = srv.Run(ctx) // bloquea; apagado graceful al cancelar el contexto
```

## Tests

Each package includes unit tests that do **not** require external services
(PostgreSQL/MinIO/Sytue ID): config validation, tenant slugs, JSON
error/response contract, health probes, S3 key derivations, claims and
Bearer extraction. Paths that touch the network are tested in per-service integration tests.

```sh
go test ./platform/...
```
