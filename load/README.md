# Sytue One — pruebas de carga y rendimiento (`load/`)

Scripts **[k6](https://k6.io/)** (Grafana, Apache-2.0) para medir el
**rendimiento** de los servicios Go clave de Sytue One frente a los SLO del
contrato. El rendimiento es una **máxima** de la suite y criterio de
*Definition of Done*: estos scripts corren en CI con **umbrales que rompen el
build** si se incumplen ([`docs/CONVENTIONS.md`](../docs/CONVENTIONS.md) §0.8,
estrategia completa en [`docs/operations.md#4-rendimiento-y-seo`](../docs/operations.md#4-rendimiento-y-seo)).

> Licencia: código propietario © Sytue (`MIT`).
> k6 es una herramienta externa (Apache-2.0); no se incorpora a los artefactos.

## SLO (objetivos de nivel de servicio)

| Métrica | Objetivo | Umbral k6 |
|---|---|---|
| Latencia API **p95** | **< 200 ms** | `http_req_duration: p(95)<200` |
| Latencia API **p99** | < 500 ms | `http_req_duration: p(99)<500` |
| Tasa de errores | < 1 % | `http_req_failed: rate<0.01` |
| Checks correctos | > 99 % | `checks: rate>0.99` |

Todos parametrizables por entorno: `-e SLO_P95_MS=150 -e SLO_ERROR_RATE=0.005`.

## Estructura

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

Se cubren **id / drive / chat** por ser los servicios con el patrón de carga más
exigente: validación de tokens (todo el tráfico de la suite pasa por Sytue ID),
lectura intensiva de listados (Drive) y tiempo real (Chat). El mismo patrón se
replica para el resto de servicios copiando un directorio y ajustando rutas.

## Tipos de prueba

- **smoke** — carga mínima (1 VU, 10 iteraciones). Verifica que los endpoints
  responden y cumplen el SLO. Se ejecuta en **cada despliegue** y como *gate*
  rápido en CI antes de la prueba de carga.
- **load** — carga sostenida con rampa (`ramping-vus`): subida → meseta de `VUS`
  → bajada. Detecta regresiones de latencia bajo concurrencia realista.

Perfil parametrizable: `-e VUS=100 -e RAMP=30s -e DURATION=5m`.

## Requisitos

Instala k6 (no se versiona): `brew install k6` · `apt install k6` · o imagen
`grafana/k6`. Requiere los servicios en marcha (`make dev-up` levanta la suite
local en `*.sytue.localhost`).

## Uso

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

### Variables de entorno

| Variable | Descripción | Por defecto |
|---|---|---|
| `ID_URL` / `DRIVE_URL` / `CHAT_URL` | Base del servicio (sin barra final) | `https://<svc>.sytue.localhost` |
| `ACCESS_TOKEN` | JWT de Sytue ID para rutas autenticadas | *(vacío)* |
| `VUS` | Usuarios virtuales en la meseta | `50` |
| `RAMP` / `DURATION` | Duración de rampa / meseta | `30s` / `2m` |
| `SLO_P95_MS` / `SLO_P99_MS` / `SLO_ERROR_RATE` | Umbrales SLO | `200` / `500` / `0.01` |

> **Secretos:** `ACCESS_TOKEN` **nunca** se versiona; se inyecta en CI desde el
> gestor de secretos. Los scripts no contienen credenciales.

## Rendimiento del backend Go (lo que estas pruebas verifican)

Las pruebas de carga son la **red de seguridad** de un backend diseñado para el
SLO. Palancas de rendimiento en los servicios (contrato §0.8):

- **`pprof`** — cada servicio expone `net/http/pprof` en un puerto de
  diagnóstico **interno** (no público) para perfilado de CPU/heap/goroutines.
  Ante una regresión detectada por k6: `go tool pprof http://<svc>:<diag>/debug/pprof/profile`.
- **PostgreSQL** — índices y `EXPLAIN (ANALYZE, BUFFERS)` en las consultas
  calientes; **paginación keyset** (no `OFFSET`) en los listados de Drive/Chat;
  **sin N+1**; vistas materializadas donde compense; pool `pgx` dimensionado.
- **Caché donde mida** — `Cache-Control` en discovery/JWKS de Sytue ID; caché
  sobre Postgres por defecto y **Valkey opcional** (§0.1) para fan-out/presencia
  de Chat, sesiones y *rate-limiting* si aporta mejora medible.
- **HTTP** — HTTP/2, `keep-alive`, compresión br/gzip; *timeouts* seguros.

## CI

En CI, `smoke` corre en cada PR (gate rápido) y `load` en la rama principal /
nightly contra un entorno efímero. Los umbrales k6 devuelven código de salida
distinto de cero al incumplirse, rompiendo el build. Se archiva el resumen JSON
(`k6 run --summary-export`) para seguir la tendencia de p95 en el tiempo.
