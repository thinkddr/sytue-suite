# Sytue Drive (`services/drive`)

Servidor de archivos **Sytue-native** de la suite **Sytue One**: almacenamiento,
sincronización y compartición de ficheros. Reimplementación **clean-room** sobre
estándares abiertos (**WebDAV — RFC 4918**) con una **API REST propia**. Código
propietario © Sytue; dependencias de terceros solo permisivas (BSD/MIT/Apache).

- **Puerto:** `8082` (HTTP + WebDAV) · **Subdominio:** `drive.sytue.eu`
- **Datastore:** PostgreSQL (metadatos, cuotas, outbox de eventos) — única fuente
  de verdad. **Sin Redis/Valkey.**
- **Blobs:** Object Storage S3 (Scaleway prod / MinIO dev), bucket `drive-blobs`,
  aislados por **prefijo de tenant**.
- **Auth:** JWT de **Sytue ID** validado vía JWKS (`platform/pkg/authn`).
- **Multi-tenant:** aislamiento por **RLS** en PostgreSQL (`app.tenant`) + prefijo
  S3 por tenant.

## Capacidades

- **Carpetas:** árbol por tenant, raíz autogestionada, crear / renombrar / mover
  (con detección de ciclos) / borrar (soft, subárbol completo).
- **Ficheros:** subir (streaming a S3, checksum SHA-256, reserva de cuota),
  descargar (stream o redirección a URL **prefirmada**), renombrar, mover,
  borrar, **versionado** al sustituir.
- **Group folders:** carpetas compartidas a nivel de organización (`is_group`).
- **Compartición:** enlaces públicos con **token** de alta entropía (opción de
  expiración), y por **usuario**/**grupo** de Sytue ID, con permiso `read`/`write`.
- **Búsqueda:** por nombre de fichero (case-insensitive) dentro del tenant.
- **Cuotas por tenant:** límite y uso mantenidos transaccionalmente; lectura para
  cualquier miembro, cambio de límite solo `admin`/`owner`. La cuota del plan es
  **una para toda la suite** y Drive es su dueño: Sytue Cast declara lo que ocupa en
  cada organización por `PUT /internal/quota/external/cast` (secreto compartido
  `CAST_INTERNAL_SECRET`, valor absoluto e idempotente) y `used_bytes` lo suma, con
  el desglose en `external_usage`.
- **WebDAV (RFC 4918):** montado en `/dav` con el mismo Bearer JWT
  (`golang.org/x/net/webdav`, BSD-3-Clause).
- **Eventos:** publicación en el **outbox** sobre PostgreSQL (`platform/pkg/events`).

## Estructura

```
services/drive/
├── cmd/drive/main.go            # binario (serve | migrate | healthcheck)
├── internal/
│   ├── config/                  # carga de configuración (env, 12-factor)
│   ├── domain/                  # tipos y reglas de negocio (sin transporte)
│   ├── store/                   # persistencia pgx + RLS por tenant
│   ├── service/                 # casos de uso (store + S3 + eventos) y rutas WebDAV
│   ├── http/                    # router chi, handlers REST, descarga pública
│   └── webdav/                  # webdav.FileSystem propio sobre el servicio
├── db/migrations/               # migraciones golang-migrate (.up/.down.sql)
├── openapi.yaml                 # contrato de la API REST
└── Dockerfile                   # multi-stage golang:1.26 -> distroless/static
```

## Configuración (variables de entorno)

| Variable | Defecto | Descripción |
|---|---|---|
| `PORT` | `8082` | Puerto HTTP/WebDAV |
| `PUBLIC_URL` | `https://drive.sytue.localhost` | URL pública (enlaces de compartición) |
| `DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | Endpoint JWKS |
| `OIDC_AUDIENCE` | `sytue-drive` | Audiencia esperada del token |
| `S3_ENDPOINT` | `minio:9000` | Endpoint S3 (acepta `http(s)://host:port`) |
| `S3_REGION` | `fr-par` | Región S3 (solo UE) |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | — (obligatorias) | Credenciales S3 |
| `S3_BUCKET` | `sytue-dev-drive-blobs` | Bucket de blobs |
| `S3_USE_SSL` | `false` | TLS hacia S3 (`true` en prod) |
| `MAX_UPLOAD_MB` | `5120` | Tamaño máximo de subida por fichero |
| `DEFAULT_QUOTA_GB` | `10` | Cuota inicial por tenant |
| `CAST_INTERNAL_SECRET` | dev: `dev-cast-internal-secret`; fuera, vacío | Secreto compartido con Sytue Cast para declarar su uso de la cuota (opcional; vacío = la puerta no abre) |
| `PRESIGN_TTL` | `15m` | Validez de URLs prefirmadas |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Logging estructurado |

No hay secretos en el repositorio: las credenciales llegan por entorno
(Scaleway Secret Manager / Sealed Secrets en prod).

## Ejecución

```sh
# Migraciones + servidor (las migraciones se aplican también al arrancar).
go run ./services/drive/cmd/drive

# Solo migrar (Job de despliegue).
go run ./services/drive/cmd/drive migrate
```

En la suite completa se levanta con `make dev-up` (Docker Compose); el contexto
de build de la imagen es la **raíz del repo** (módulo único `git.sytue.eu/sytue/one`).

## Endpoints

REST bajo `/api/v1` (ver [`openapi.yaml`](./openapi.yaml)); WebDAV bajo `/dav`;
descarga pública por enlace en `/public/{token}`; sondas `/healthz` y `/readyz`.

## Seguridad

Deny-by-default: toda la API exige JWT de Sytue ID; el aislamiento entre tenants
lo garantiza RLS en PostgreSQL (más el prefijo S3). Consultas siempre
parametrizadas; cuerpos limitados; cabeceras `X-Content-Type-Options: nosniff`
en las descargas; tokens de enlace con 256 bits de entropía y resolución vía
función `SECURITY DEFINER` acotada. Los enlaces públicos pueden expirar y
revocarse.
