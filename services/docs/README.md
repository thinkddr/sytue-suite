<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue Docs/Sheets/Slides (`services/docs`)

Backend Go de la **ofimática colaborativa** de Sytue One (`docs.sytue.eu`, puerto
dev **8083**). Documentos de texto, hojas de cálculo y presentaciones, con
**edición en tiempo real** por WebSocket, **versionado**, **comentarios**,
**permisos** y **exports OOXML/ODF**. Producto **Sytue-native, propietario
(© Sytue)** y **clean-room**: implementado sobre estándares abiertos (ECMA-376
OOXML, OASIS ODF) y la literatura pública de CRDTs; **sin** motores AGPL
(OnlyOffice/Nextcloud quedan fuera) y **sin Redis** (el estado, las operaciones
y la difusión cross-réplica viven en **PostgreSQL**).

> Parte del módulo Go único `git.sytue.eu/sytue/one` (go.mod en la **raíz** del
> repo). Reutiliza `platform/pkg/*` (config, log, httpx, pg, objectstore, authn,
> events, tenancy). El **editor visual** vive en `apps/frontend/packages/office` y lo montan las apps de Docus, Calcus, Diapos y CertiPDF (`apps/frontend/apps/{docus,calcus,diapos,certipdf}`, ADR 0076).

## Estrategia de colaboración: CRDT (no OT)

La convergencia se logra con un **CRDT de secuencia tipo RGA** (Replicated
Growable Array) con identidades densas e independientes (`Dot{actor, counter}`)
y **reloj Lamport**, más **registros LWW** para celdas/formato/propiedades.
Decisión y motivos:

- **El servidor solo SECUENCIA y persiste** operaciones (append-only en
  `doc_operations`); **no transforma** (no es OT). La fusión es determinista en
  cliente y servidor (`internal/domain/crdt.go`), lo que evita un servidor de
  transformación con estado complejo y encaja con un datastore único.
- **Offline-first / reconexión:** un cliente envía su `last_seq` (`hello`) y
  recibe el *replay* de las operaciones posteriores; converge sin conflictos.
- **Idempotencia y conmutatividad:** aplicar una operación dos veces o en
  distinto orden produce el mismo estado (propiedad CRDT) — ver tests.
- **Multi-réplica sin Redis:** si hay varias réplicas del servicio, la difusión
  cross-réplica se hace por el **bus de eventos sobre PostgreSQL**
  (`platform/pkg/events`, LISTEN/NOTIFY + outbox). El `Hub` en memoria es el
  fan-out local por proceso.

El modelo por tipo de documento se construye sobre estas primitivas:

| Tipo | Modelo |
|------|--------|
| `document` | secuencia RGA de caracteres + marcas de formato (LWW) |
| `spreadsheet` | mapa LWW de celdas (`"Hoja!fila:col" → valor/fórmula`) |
| `presentation` | secuencia RGA de diapositivas + LWW de propiedades |

## Arquitectura (capas)

```
services/docs/
├── cmd/docs/main.go            # binario: carga config, migra, cablea y sirve
├── internal/
│   ├── config/                 # configuración (prefijo SYTUE_DOCS)
│   ├── domain/                 # modelo, CRDT, autorización, export, Service
│   ├── store/                  # persistencia pgx (RLS por tenant)
│   ├── collab/                 # hub + clientes WebSocket (coder/websocket)
│   └── http/                   # router chi, handlers, mapeo de errores
├── db/migrations/              # esquema versionado (golang-migrate)
├── openapi.yaml                # contrato HTTP
└── Dockerfile                  # multi-stage -> distroless static (rootless)
```

## Datos (PostgreSQL, multi-tenant con RLS)

- `documents` — metadatos + `seq` (vector de versión lógico) + papelera.
- `doc_operations` — log **inmutable** de operaciones CRDT (append-only).
- `doc_snapshots` — estado CRDT consolidado (inline en `jsonb` o en S3).
- `doc_versions` — historial de versiones nombradas/automáticas (restaurables).
- `doc_permissions` — ACL **deny-by-default** (user/group/tenant + rol).
- `doc_share_links` — enlaces públicos (token **hasheado** sha-256).
- `doc_comments` — comentarios anclados a referencias CRDT estables.
- `doc_blobs` — metadatos de exports en S3 (OOXML/ODF/PDF/HTML/MD).
- `events_outbox` — bus de eventos transaccional.

Aislamiento multi-tenant por **RLS** (`current_setting('app.tenant')`, fijado por
`platform/pkg/pg.SetTenant`). La **única** lectura cross-tenant permitida es la
resolución de un enlace público, vía la función `SECURITY DEFINER`
`docs_resolve_share(token_hash)` de superficie mínima (devuelve solo
`document_id/tenant/role` de enlaces activos).

## Permisos (deny-by-default)

Roles crecientes: `viewer < commenter < editor < owner`. El `owner_sub` del
documento es siempre `owner`. Para el resto, el rol efectivo es el **máximo**
entre las entradas de ACL aplicables (usuario, grupos de Sytue ID, tenant). Sin
entrada aplicable ⇒ **sin acceso**. Los enlaces de compartición nunca conceden
`owner`.

## Identidad

Cada petición se valida contra **Sytue ID** (OIDC/JWKS) con el middleware
`authn.RequireAuth` de la plataforma. El `sub`, los grupos y el `tenant` se
extraen del JWT para resolver la autorización. El canal WebSocket exige el mismo
JWT (Bearer) y rol mínimo `viewer`; editar requiere `editor`.

## Exports OOXML/ODF (clean-room)

`internal/domain/export.go` genera paquetes **OPC** (OOXML, ECMA-376: docx/xlsx/
pptx) y **ODF** (OASIS 1.3: odt/ods/odp) con la stdlib de Go (`archive/zip`,
`encoding/xml`), además de PDF/HTML/Markdown. Obra original sobre las
especificaciones públicas; los binarios se materializan en Object Storage S3
(bucket `docs-blobs`).

## Configuración (entorno, prefijo `SYTUE_DOCS_`)

| Variable | Por defecto | Descripción |
|---|---|---|
| `SYTUE_DOCS_HTTP_ADDR` | `:8083` | dirección de escucha HTTP |
| `SYTUE_DOCS_DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `SYTUE_DOCS_OIDC_ISSUER` | `https://id.sytue.eu` | issuer de Sytue ID |
| `SYTUE_DOCS_OIDC_AUDIENCE` | `sytue-docs` | audiencia esperada |
| `SYTUE_DOCS_OIDC_JWKS_URL` | (derivada del issuer) | endpoint JWKS |
| `SYTUE_DOCS_S3_ENDPOINT` | — | Object Storage (MinIO dev / Scaleway prod) |
| `SYTUE_DOCS_S3_REGION` | `fr-par` | región S3 (solo UE) |
| `SYTUE_DOCS_S3_ACCESS_KEY` / `_SECRET_KEY` | — | credenciales S3 |
| `SYTUE_DOCS_S3_BUCKET` | `docs-blobs` | bucket de blobs/exports |
| `SYTUE_DOCS_S3_USE_SSL` | `false` | TLS hacia S3 |
| `SYTUE_DOCS_SNAPSHOT_INLINE_LIMIT` | `262144` | tope (bytes) de snapshot inline |
| `SYTUE_DOCS_DRIVE_URL` | `http://drive:8082` | Sytue Drive: la unidad donde vive el fichero de cada documento |
| `SYTUE_DOCS_DRIVE_TIMEOUT` | `15s` | tope por llamada a Drive |
| `SYTUE_DOCS_LOG_LEVEL` / `_LOG_FORMAT` | `info` / `json` | logging |

> Sin secretos en código ni en logs: todo por entorno / gestor de secretos.

> **El fichero del documento vive en Sytue Drive.** Un documento nuevo se crea
> también como `.odt`/`.ods`/`.odp` en la carpeta `Sytue Docs/` de la unidad de
> quien lo crea, y nace anclado a él (`source_ref`, mig. 0006). La llamada va con
> el **JWT de esa persona**, no con un secreto de servicio, para que Drive aplique
> su propia autorización. Consecuencia visible: crear un documento **consume cuota
> de Drive**, y con la unidad llena la creación responde 507. El diseño completo,
> lo que falta y el plan para los documentos anteriores están en
> [`docs/architecture.md#docs-en-drive`](../../docs/architecture.md#docs-en-drive).

## Protocolo WebSocket (`/v1/documents/{id}/ws`)

Mensajes JSON. Cliente→servidor: `hello` (sincronización por `last_seq`), `ops`
(edición), `cursor` (presencia efímera), `ping`. Servidor→cliente: `welcome`
(rol + seq), `ops` (operaciones secuenciadas), `ack` (confirmación con seq),
`presence`, `error`.

## API REST

Contrato completo en [`openapi.yaml`](./openapi.yaml). Resumen:

- `GET/POST /v1/documents`, `GET/PATCH/DELETE /v1/documents/{id}`, `PUT …/star`
- `GET …/state`, `GET/POST …/ops`, `GET …/ws`
- `GET/POST …/versions`, `POST …/versions/{vid}/restore`
- `GET/POST …/comments`, `PUT …/comments/{cid}/resolve`, `DELETE …/comments/{cid}`
- `GET/PUT/DELETE …/permissions`, `GET/POST …/shares`, `DELETE …/shares/{lid}`
- `GET /v1/share/{token}` (público, sin JWT)
- `GET /v1/share/{token}/view` (público, sin JWT): su vista de solo lectura en HTML, la de `/s/{token}` en la web
- `POST /v1/share/redeem` `{token}` (con sesión): canjea un enlace de editor o comentarista por un permiso de quien llama, también desde otra organización (ADR 0052)
- `GET/POST …/exports`, `GET …/exports/{bid}`
- Sondas `/healthz` (liveness) y `/readyz` (readiness: PostgreSQL + S3).

## Tests

```sh
go test ./services/docs/...
```

Cubren la **convergencia CRDT** (independencia de orden, idempotencia,
tombstones, LWW), la **resolución de roles** (deny-by-default) y los **exports**
(docx como ZIP OPC válido, ODF con `mimetype` primero y sin comprimir). No
requieren PostgreSQL/MinIO/Sytue ID.
