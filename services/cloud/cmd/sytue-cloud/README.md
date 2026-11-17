<!-- SPDX-License-Identifier: Apache-2.0 -->
# sytue-cloud

Thin client for the Sytue Cloud control plane REST API. It covers the operational
tree (deploy, rollback, logs, quick queries) and convenience CRUD.
**Declarative** management lives in the Terraform provider; this CLI is the
**imperative** side. No dependencies outside the stdlib.

<a id="configuración-entorno--flags"></a>
## Configuration (environment + flags)

| Flag            | Environment          | Default                 |
|-----------------|----------------------|-------------------------|
| `--api URL`     | `SYTUE_CLOUD_API`    | `http://127.0.0.1:8090` |
| `--token TOKEN` | `SYTUE_CLOUD_TOKEN`  | —                       |
| `-o, --output`  | `SYTUE_CLOUD_OUTPUT` | `table`                 |

The token is an `scl_...` (scope `read<write<deploy`) or a Sytue ID JWT.
**Prefer `$SYTUE_CLOUD_TOKEN`**: `--token` stays in the shell history and is
visible in `ps`. The token is never printed or written to logs.

```sh
export SYTUE_CLOUD_TOKEN=scl_xxx
export SYTUE_CLOUD_API=https://cloud.sytue.eu
```

Default output is an aligned **table**; `-o json` returns the raw API body
for scripting (`| jq`). Errors go to stderr, exit code ≠ 0.

<a id="comandos"></a>
## Commands

```sh
# Proyectos
sytue-cloud projects list
sytue-cloud projects get <id>
sytue-cloud projects create --slug web --repo https://git.sytue.eu/acme/web.git \
    --framework astro --build-strategy buildpacks
sytue-cloud projects delete <id>
sytue-cloud projects restart <id>                 # reinicia sin recompilar ni cambiar de versión
sytue-cloud projects clone <id> --slug web-staging  # copia configuración y variables
psql "$(sytue-cloud projects db-url <id>)"        # cadena de conexión con contraseña

# Variables de entorno / secretos
sytue-cloud env list --project <id>
sytue-cloud env set --project <id> API_URL=https://api.acme.com
sytue-cloud env set --project <id> API_KEY=xxx --secret --scope production
sytue-cloud env unset --project <id> API_KEY        # resuelve KEY -> id y borra
sytue-cloud env pull --project <id> > .env          # el .env del entorno (revela secretos)

# Dominios
sytue-cloud domains list --project <id>
sytue-cloud domains add --project <id> --hostname app.acme.com   # devuelve DNS
sytue-cloud domains remove <domain-id>

# Servicios gestionados
sytue-cloud services list --project <id>
sytue-cloud services create --project <id> --kind postgres --name db
sytue-cloud services create --project <id> --kind cron --config schedule=@daily --config image=rg.../job
sytue-cloud services delete <service-id>
sytue-cloud services run <service-id>                        # ejecuta ahora un cron o un job
sytue-cloud services logs <service-id> [--lines 200]         # su registro reciente
sytue-cloud services backups <service-id>
sytue-cloud services restore <service-id> --backup <fichero> # DESTRUCTIVO
curl -o copia.sql.gz "$(sytue-cloud services download <service-id> --backup <fichero>)"

# Deployments
sytue-cloud deployments list --project <id> --limit 20 [--cursor <c>]
sytue-cloud deployments create --project <id>              # build+deploy desde git
sytue-cloud deployments create --project <id> --image rg.fr-par.scw.cloud/sytue/app@sha256:...  # BYO
sytue-cloud deploy --project <id>                          # alias de 'deployments create'
sytue-cloud deployments get <id>
sytue-cloud deployments rollback <deployment-id>            # vuelve a una producción anterior
sytue-cloud deployments promote <deployment-id>             # publica la imagen de una PREVIEW
sytue-cloud deployments cancel <id>

# Logs de build (texto plano; filtrado server-side)
sytue-cloud logs --build <build-id> [--q error] [--level ERROR] [--limit 500]
sytue-cloud logs --deployment <deployment-id>             # resuelve su build_id

# Logs de EJECUCIÓN del proyecto (los que guarda el recolector)
sytue-cloud logs --project <id> --q timeout --since 2026-08-01 [--limit 200]

# Uso y facturación
sytue-cloud usage [--from 2026-07-01T00:00:00Z --to 2026-08-01T00:00:00Z]
sytue-cloud billing account
sytue-cloud billing invoices [--limit 12] [--cursor <c>]
sytue-cloud billing invoice <id>

sytue-cloud version | help
```

Flags and positional arguments can be interleaved: `deployments get <id> -o json`
and `deployments get -o json <id>` are equivalent.

<a id="no-cubierto-la-api-no-lo-expone-no-se-inventa"></a>
## Not covered (the API does not expose it; nothing is invented)

- **Log streaming (`--follow`)**: there is no SSE/WS endpoint. Only the static build
  log (`GET /builds/{id}/logs`); `--follow` returns an explicit error.
- Resources with an endpoint but no command yet (YAGNI until requested): org,
  members, tokens, uptime-checks, alerts, builds (list/get), `usage/detail`,
  `billing/spend-limit`, `billing/invoices:generate`, project GitHub connect,
  storage, DB Studio and the deploy URL (it is issued from the dashboard, and
  via the API it is `POST /tokens` with `project_id` + scope `deploy`).
  They are added with the same pattern when needed.

> The line “per-project logs do not exist” was here until 2026-08-17,
> when they had already existed for several waves: a README that ages lies with more
> authority than the code.
