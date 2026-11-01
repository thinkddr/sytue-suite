<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue One — Núcleo Go (`platform/`)

Librerías compartidas del **módulo Go único** `git.sytue.eu/sytue/one` (con `go.mod`
en la **raíz** del repo). Todos los servicios de la suite (`services/<svc>/`) las
reutilizan para mantener un comportamiento idéntico en config, logging, HTTP,
acceso a datos, identidad, eventos y almacenamiento.

> Stack canónico (ver [`docs/CONVENTIONS.md`](../docs/CONVENTIONS.md) v4):
> **Go 1.26 + PostgreSQL 18** como datastore **único** y fuente de verdad (estado,
> colas y caché sobre Postgres; **Valkey** solo opcional donde mida §0.1; **nunca
> Redis**), blobs en **Object Storage S3** (MinIO dev / Scaleway prod), identidad
> vía **Sytue ID** (OIDC). Dependencias de terceros **solo permisivas**
> (MIT/BSD/Apache). Código de producto **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)** (cabecera SPDX por archivo).
>
> **Seguridad y privacidad por defecto y reforzadas** (CONVENTIONS §0.7): estas
> librerías materializan deny-by-default, mínimo privilegio, RLS multi-tenant,
> cabeceras de seguridad + CSP + HSTS, CSRF, rate-limiting, cookies
> HttpOnly/Secure/SameSite, validación/saneado, consultas parametrizadas,
> observabilidad OTel correlacionada y logs **sin PII** (RGPD art. 25).

## Paquetes (`platform/pkg/*`)

| Paquete        | Propósito |
|----------------|-----------|
| `config`       | Carga de configuración **tipada** desde el entorno (12-factor) con prefijos, defaults, validación y acumulación de errores. `Base` común a todos los servicios. |
| `log`          | Logger estructurado con `log/slog` (JSON en prod, texto en dev). Propagación por `context.Context`. |
| `telemetry`    | Observabilidad **correlacionada** (traces + metrics + logs): spans W3C Trace Context (propagación `traceparent`), contadores/histogramas agregados, **exporter OTLP/HTTP** hacia el colector (sin arrastrar el SDK OTel), inyección de `trace_id`/`span_id` en el logger y **pprof protegido** (deny-by-default, requiere Guard de autorización). |
| `httpx`        | Router **chi** con cadena de middlewares **SEGURA por defecto**: request-id, logging, recovery, **cabeceras de seguridad** (CSP configurable, HSTS solo TLS, nosniff, X-Frame-Options/frame-ancestors, Referrer-Policy, Permissions-Policy, COOP/CORP/COEP), **CORS** restrictivo (allowlist), **límite de body**, **rate-limiting** token-bucket, **CSRF** double-submit, timeout. Helpers de **cookies HttpOnly/Secure/SameSite=strict**, respuestas/errores **JSON canónicos**, decodificación+validación de cuerpos y sondas `/healthz`/`/readyz`. Servidor con apagado *graceful* y timeouts seguros. |
| `pg`           | Pool **pgx** con límites, helpers de transacción (`InTx`, `InTenantTx`, `InRLSTx`), **runner de migraciones**, tenancy/**RLS** (`SetTenant`/`WithTenant` vía `set_config('app.tenant', …, true)`, generador de políticas `TenantRLSPolicySQL` con `FORCE ROW LEVEL SECURITY` deny-by-default) y **rate-limiter compartido** sobre Postgres. Consultas siempre **parametrizadas**. |
| `objectstore`  | Cliente **S3** compatible MinIO/Scaleway (minio-go): put/get/delete, URLs prefirmadas, *health*. Aislamiento por tenant mediante **prefijos de clave**. |
| `validation`   | **Validación y saneado** de entrada reutilizable: `Validator` acumulativo (required/len/email/slug/allowlist/regex), saneado de texto (control chars, BiDi/zero-width anti *Trojan Source*), truncado por runes. Se integra con `httpx.FailValidation` (422 con detalles por campo). |
| `authn`        | Verificador **OIDC** de los JWT de Sytue ID: **JWKS** con caché/rotación (jwx), validación de firma/issuer/audiencia/**exp/nbf/iat**, **allowlist de algoritmos** (asimétricos; nunca `none` ni HS*), `kid` obligatorio, extracción de claims **sin PII** (`Claims.LogValue`), middlewares `RequireAuth` / `RequireRole` / `RequireTenant` / `RequireScope`. |
| `events`       | **Bus de eventos sobre PostgreSQL** (sin Redis): patrón **outbox** transaccional + **LISTEN/NOTIFY**. `Dispatcher` (entrega *at-least-once* con `FOR UPDATE SKIP LOCKED` y poll de respaldo) y `Subscriber` (NOTIFY efímero en tiempo real). |
| `tenancy`      | Contexto de **tenant** (organización). Slug canónico ⇄ `org/<slug>` (Sytue ID) ⇄ `tenant-<slug>` (Kapsule) ⇄ prefijo S3. |

## Convenciones

- **Capas por servicio:** `cmd/<svc>/main.go` + `internal/{http,domain,store}`.
- **Cabecera SPDX propietaria** en cada archivo Go (`MIT`).
- **`context.Context`** en todo I/O; errores envueltos con `%w`.
- **Migraciones** versionadas por servicio (`db/migrations/*.up.sql` / `*.down.sql`),
  embebidas con `//go:embed` y aplicadas con `pg.MigrateUp`.

## Ejemplo de cableado de un servicio

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

Cada paquete incluye tests unitarios que **no** requieren servicios externos
(PostgreSQL/MinIO/Sytue ID): validación de config, slugs de tenant, contrato de
errores/respuestas JSON, sondas de salud, derivaciones de claves S3, claims y
extracción de Bearer. Las rutas que tocan red se prueban en integración por servicio.

```sh
go test ./platform/...
```
