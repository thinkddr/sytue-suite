<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

# SyERP (`:8098`)

The Sytue One ERP: contacts, sales, purchasing, inventory, manufacturing,
invoicing, accounting, projects, HR, maintenance, support and eleven more
areas. **24 modules, 36 models**, one service, one database.

<a id="la-decisión-que-ordena-todo-lo-demás"></a>
## The decision that orders everything else

An ERP “with every module” written module by module means two hundred tables,
two hundred CRUDs that differ only in their column names, and a
migration every time someone wants a new field. Not here.

**The model catalog is DATA.** It lives in
[`apps/nucleo/erp/catalogo/modulos/*.json`](../../apps/nucleo/erp/catalogo/modulos) (shared with
the ERP of Sytue Office, the local edition): each file
declares models with their fields, their states and their list columns. The core does not
have a single line of code specific to Sales, Fleet or Quality.

```jsonc
{
  "clave": "pedido_venta", "modulo": "ventas", "secuencia": "PV",
  "estados": ["borrador", "confirmado", "entregado", "facturado", "cancelado"],
  "campos": [
    { "clave": "cliente", "tipo": "relacion", "modelo": "contacto", "requerido": true },
    { "clave": "fecha", "tipo": "fecha", "requerido": true },
    { "clave": "total", "tipo": "dinero" }
  ],
  "lista": ["ref", "cliente", "fecha", "total", "estado"]
}
```

The consequences, which are the product:

- **Adding a module is a JSON file.** No DDL, no migration, no table
  lock, no new HTTP route, no new screen, no frontend changes.
- **A single API.** `/api/v1/{modelo}` serves all 36 models.
- **A single interface.** The app (`apps/frontend/apps/erp`) generates the list and the detail view
  from the catalog this service provides.
- **The five languages, once.** Common labels (date, amount, customer…)
  live in [`terminos.json`](../../apps/nucleo/erp/catalogo/terminos.json) with their five
  translations and are shared by the 24 modules; a field without a label **prevents
  startup**, it does not fall back to English. `TestTodoTraducidoEnLosCincoIdiomas` guards this.

<a id="lo-que-no-va-en-jsonb-el-dinero"></a>
## What does NOT go in JSONB: money

The master data lives in `records.data` (jsonb). Accounting has its own tables,
typed columns and real constraints:

- **Amounts in integer cents** (`bigint`). Not a single `float` in any money
  column: 0.1 + 0.2 is not 0.3 and an invoice cannot depend on that.
  Non-monetary quantities travel as a decimal string and Postgres casts them to
  `numeric` to add them up.
- **Balancing is enforced by Postgres**, not by the code: `move_lines_cuadre` is a
  `CONSTRAINT TRIGGER … DEFERRABLE` that checks `sum(debe) = sum(haber)` when
  the transaction commits.
- **What is posted is not touched.** `moves_inmutables` and `move_lines_inmutables`
  reject updating, deleting **and adding** to a posted journal entry; the only
  allowed transition is voiding it. Corrections are made with another entry, which is what
  separates an accounting ledger from a spreadsheet.

  The `INSERT` in that list arrived late, and it is worth writing down: the trigger
  covered only `UPDATE`/`DELETE`, so **adding** lines to an already
  closed entry was allowed. A single stray line was stopped by the balance check by accident,
  but a balanced pair —a debit and its credit— got in without anything complaining.
  `TestAsientoAsentadoEsInmutable/colar_un_par_equilibrado` found it by
  requiring the failure to come from the trigger and not from just any error. That is why an
  entry is written as a draft, its lines are added and it is closed at the end:
  that order is no longer a code convention, the database enforces it.
- **One document, one entry.** Partial unique index `moves_un_asiento_por_documento`:
  a retry, two tabs or a race between replicas crash into the
  index instead of duplicating revenue.

A credit note is not a special case: it is the same invoice in negative, and each amount is
placed on its natural side, inverted if it comes in negative. Covered by
`TestAsientoRectificativo`.

<a id="rendimiento-medido"></a>
## Performance, measured

Real numbers, not estimates: 20,000 rows per model in a PostgreSQL with
up-to-date statistics. They are reproduced with
`go test -tags integration -run "TestElListadoUsaSuIndice|TestTiempoDeListado"`
and with `go test -bench Vista ./internal/catalogo`.

| | Before | Now |
|---|---|---|
| First page of a list | 9.4 ms · cost 2114 · **20,000 rows read** | **0.56 ms** · cost 8.88 · 51 rows |
| List filtered by state | 5.5 ms | **0.54 ms** |
| Catalog (every page load) | 144 µs · **115 KB allocated** | **7.9 ns · 0 B** |
| Catalog over the wire, on return | 32 KB | **304, no body** |

The three causes, and none was the one it seemed to be:

1. **`tenant` was missing from the `WHERE`.** RLS enforces it, yes, but by comparing it
   with `current_setting`, which is a stable expression and **cannot serve as an
   ORDERED index condition**. `tenant` is the first column of
   `records_listado`, so without the explicit equality PostgreSQL could not
   walk the index in order: it read the entire model and did a top-N to
   serve fifty rows. With it, *Index Scan*, and it stops at the page.
   The policy is still applied as a `Filter`: this does not replace it, it gives the
   planner the information the policy cannot give it.

2. **The catalog was translated and serialized on every request.** It is immutable —it is
   baked into the image—, so now it is computed once per language at
   startup. It costs 160 KB of memory in total and leaves the handler as a byte
   copy. With an ETag on top, returning to a screen does not download a single byte of it.

3. **And a flicker that was not about performance but about correctness:** when switching
   models, the rows of the previous one stayed on screen painted with the
   columns of the new one —a table of dashes that looked like a data error—.

In the browser, lists already seen are painted **instantly** from an in-memory
cache and refreshed in the background; writing invalidates that model's cache, because
serving a stale list right after creating something is the fastest way to make
someone believe it was not saved.

`TestElListadoUsaSuIndice` is a **gate on the plan**, not a measurement: whoever
removes `tenant` from the `WHERE` breaks nothing visible —the list still comes out
fine, just seventeen times slower— and that is why there is a test looking at the
`EXPLAIN`.

**What was measured and deliberately NOT touched:** text search takes 9.8 ms and
does not use `records_etiqueta_trgm` — at this size PostgreSQL is right to prefer the
scan, and the same goes for a literal pattern or a composite GIN on
`btree_gin`. The index stays for when the table grows; the alternative was
adding a new extension with no measurable improvement.

<a id="conectores"></a>
## Connectors

| Connector | Direction | How |
|---|---|---|
| **Salesforce** | inbound | `POST /api/v1/importar/salesforce`. It goes out through the **Sytue Integration Gateway** (`:8093`), forwarding the importing user's authorization: SyERP stores no Salesforce credentials and has no service account with access to every organization. Idempotent by `externo_id`: importing twice updates, it does not duplicate. It maps `Account`, `Contact` and `Opportunity`. |
| **Stripe** | inbound | `POST /webhooks/stripe/{org}`. No session —Stripe does not present a Sytue ID token—: its authentication is the **HMAC-SHA256 signature** of the raw body, with a 5-minute window against replays and constant-time comparison. `invoice.paid` marks the invoice as paid and writes the collection entry. Event replays are cut off by the primary key of `eventos_entrantes`. |

What does **not** exist yet, stated plainly: **charging from SyERP** (creating a Stripe payment
link from an invoice). The gateway's data plane is
**read-only** (`TabularSource`), so a write connector first needs
that plane in `services/integrations`. Meanwhile the invoice is created
in Stripe and matched by `externo_id` or by `metadata.syerp_ref`.

<a id="licencias"></a>
## Licenses

Our own model, written from the functional domain. That an ERP has
opportunities, delivery notes, timesheets or double-entry journal entries are ideas
of the trade —and accounting from 1494—, not licensed code: **no third-party source
has been consulted or ported** (CONVENTIONS §12). No LGPL/AGPL dependencies
dragged in: the service uses the same stack as the rest of the suite
(chi, pgx, `platform/pkg/*`).

<a id="lo-que-no-reimplementa"></a>
## What it does NOT reimplement

The suite already has mail, documents, spreadsheets, calendar, chat, video,
storage and signing. SyERP does **not** build them again: where another ERP puts in
its own messaging or document module, here Sytue Mail, Docus,
Drive and Rooms are used. It is the half of the code that does not exist.

The four guarantees have their test against a real PostgreSQL in
`internal/store/libro_integration_test.go` (`-tags integration`), and the negative cases
are tied to the trigger's message: a test that only requires “that it fails” would stay
green the day it fails for some other reason.

<a id="uso"></a>
## Usage

```bash
# Validar los manifiestos y ver qué hay. Sin base de datos y sin red.
go run ./services/syerp/cmd/syerp catalogo

# Las garantías de la base, contra un Postgres efímero (testcontainers).
go test -tags integration ./services/syerp/internal/store/...

# Levantar en dev (perfil propio: no arrastra los treinta servicios).
docker compose -p sytue-one-dev -f deploy/compose/docker-compose.yml \
  --profile erp up -d postgres id syerp erp-web
```

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | PostgreSQL (single source of truth) |
| `OIDC_ISSUER` / `OIDC_JWKS_URL` | Sytue ID token validation |
| `INTEGRATIONS_BASE_URL` | Egress gateway (default `http://integrations:8093`) |
| `SECRET_ENC_KEY` | Base64 AES-256: seals the webhook signing secret. **Required in prod** |

<a id="rutas"></a>
## Routes

```
GET    /api/v1/catalogo                    catálogo traducido + módulos activos
POST   /api/v1/modulos/{modulo}            encender/apagar un módulo        (admin)
GET    /api/v1/{modelo}                    lista (q, estado, campo/valor, cursor)
POST   /api/v1/{modelo}                    crear
GET    /api/v1/{modelo}/{id}               leer
PUT    /api/v1/{modelo}/{id}               actualizar
DELETE /api/v1/{modelo}/{id}               a la papelera
POST   /api/v1/{modelo}/{id}/estado        mover en el flujo
POST   /api/v1/{modelo}/{id}/asentar       llevar al libro por partida doble
GET    /api/v1/{modelo}/{id}/asientos      asientos de un documento
GET    /api/v1/libro                       libro diario (desde, hasta)
GET    /api/v1/balance                     sumas y saldos
POST   /api/v1/libro/{id}/anular           anular un asiento               (admin)
GET    /api/v1/conexiones                  conectores configurados         (admin)
POST   /api/v1/conexiones/{proveedor}      configurar Stripe/Salesforce    (admin)
POST   /api/v1/conexiones/{p}/probar       probar sin escribir (1 fila)    (admin)
POST   /api/v1/importar/{p}                importar (salesforce, odoo,
                                           business-central, holded)       (admin)
POST   /webhooks/stripe/{org}              webhook de Stripe          (sin sesión)
```
