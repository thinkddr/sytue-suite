# Sytue Contacts (`services/contacts`)

Servidor de contactos **Sytue-native** de la suite **Sytue One**: libretas de
direcciones, contactos y grupos. Reimplementación **clean-room** sobre estándares
abiertos (**vCard 4.0 — RFC 6350** y **CardDAV — RFC 6352**) con una **API REST
propia**. Código propietario © Sytue; dependencias de terceros solo permisivas
(BSD/MIT/Apache): `emersion/go-webdav` (CardDAV) y `emersion/go-vcard` (MIT).

- **Puerto:** `8088` (HTTP + CardDAV) · **Subdominio:** `contacts.sytue.eu`
- **Datastore:** PostgreSQL (libretas, tarjetas, contactos, grupos, outbox de
  eventos) — única fuente de verdad. **Sin Redis/Valkey. Sin Object Storage**
  (las fotos se referencian por URL, no se embeben).
- **Auth:** JWT de **Sytue ID** validado vía JWKS (`platform/pkg/authn`).
- **Multi-tenant:** aislamiento por **RLS** en PostgreSQL (`app.tenant`).

## Capacidades

- **Libretas de direcciones (colecciones CardDAV):** por usuario/tenant, libreta
  por defecto autogestionada, crear / renombrar / borrar (soft); `ctag` +
  `sync_token` para sincronización incremental (RFC 6578).
- **Contactos (vCard 4.0):** crear / leer / actualizar (condicional por
  **If-Match**/ETag) / borrar; proyección normalizada (N, FN, ORG, TITLE,
  NICKNAME, BDAY, NOTE) más **correos** y **teléfonos** con `TYPE`/`PREF`.
  Exportación de la vCard canónica (`text/vcard; version=4.0`).
- **Grupos (vCard `KIND=group`):** miembros por `UID` (propiedad `MEMBER`,
  RFC 6350 6.6.5), crear / leer / actualizar / borrar.
- **Búsqueda:** full-text (`tsvector` GIN) sobre nombre, organización, apodo y
  notas, dentro del tenant (opcionalmente acotada a una libreta).
- **CardDAV (RFC 6352):** montado en `/dav` con el mismo Bearer JWT, sobre
  `emersion/go-webdav/carddav`. Descubrimiento por `/.well-known/carddav`
  (RFC 6764). Compatible con clientes estándar (iOS/macOS, Thunderbird, Evolution,
  DAVx⁵).
- **Eventos:** publicación en el **outbox** sobre PostgreSQL (`platform/pkg/events`).

## Estructura

```
services/contacts/
├── cmd/contacts/main.go         # binario (serve | migrate | healthcheck)
├── internal/
│   ├── config/                  # carga de configuración (env, 12-factor)
│   ├── domain/                  # tipos, reglas y códec vCard 4.0 (sin transporte)
│   ├── store/                   # persistencia pgx + RLS por tenant; CardDAV store
│   ├── carddav/                 # carddav.Backend (emersion/go-webdav) sobre store
│   └── http/                    # router chi, handlers REST, export vCard
├── db/migrations/               # migraciones golang-migrate (.up/.down.sql)
├── openapi.yaml                 # contrato de la API REST
└── Dockerfile                   # multi-stage golang:1.26 -> distroless/static
```

## Configuración (variables de entorno)

| Variable | Defecto | Descripción |
|---|---|---|
| `PORT` | `8088` | Puerto HTTP/CardDAV |
| `PUBLIC_URL` | `https://contacts.sytue.localhost` | URL pública |
| `DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `DB_MAX_CONNS` | `10` | Tamaño máximo del pool pgx |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | Endpoint JWKS |
| `OIDC_AUDIENCE` | `sytue-contacts` | Audiencia esperada del token |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Logging estructurado |

No hay secretos en el repositorio: las credenciales llegan por entorno
(Scaleway Secret Manager / Sealed Secrets en prod).

## Ejecución

```sh
# Migraciones + servidor (las migraciones se aplican también al arrancar).
go run ./services/contacts/cmd/contacts

# Solo migrar (Job de despliegue).
go run ./services/contacts/cmd/contacts migrate
```

En la suite completa se levanta con `make dev-up` (Docker Compose); el contexto
de build de la imagen es la **raíz del repo** (módulo único `git.sytue.eu/sytue/one`).

## Endpoints

REST bajo `/api/v1` (ver [`openapi.yaml`](./openapi.yaml)); CardDAV bajo `/dav`;
descubrimiento en `/.well-known/carddav`; sondas `/healthz` y `/readyz`.

### CardDAV (RFC 6352)

El árbol de recursos sigue el modelo estándar:

```
/dav/principals/{sub}/                      # principal del usuario
/dav/addressbooks/{sub}/                     # home set de libretas
/dav/addressbooks/{sub}/{libreta}/           # colección (libreta)
/dav/addressbooks/{sub}/{libreta}/{uid}.vcf  # recurso vCard (contacto o grupo)
```

Soporta `PROPFIND`, `REPORT` (`addressbook-query`, `addressbook-multiget`,
`sync-collection`), `GET`/`PUT`/`DELETE` con `If-Match`/`If-None-Match`, y
`MKCOL` extendido para crear libretas.

## Seguridad

Deny-by-default: toda la API y el árbol CardDAV exigen JWT de Sytue ID; el
aislamiento entre tenants lo garantiza **RLS** en PostgreSQL (variable de sesión
`app.tenant` fijada por transacción). Consultas siempre parametrizadas; cuerpos
limitados; tamaño máximo de vCard acotado (`max-resource-size`); escritura
condicional por ETag. Sin datos personales en logs.
