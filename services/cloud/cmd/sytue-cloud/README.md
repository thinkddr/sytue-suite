<!-- SPDX-License-Identifier: Apache-2.0 -->
# sytue-cloud

Cliente delgado de la API REST del control plane de Sytue Cloud. Cubre el árbol
operacional (deploy, rollback, logs, consultas rápidas) y CRUD de conveniencia.
La gestión **declarativa** vive en el provider de Terraform; este CLI es lo
**imperativo**. Sin dependencias fuera de la stdlib.

## Configuración (entorno + flags)

| Flag            | Entorno              | Default                 |
|-----------------|----------------------|-------------------------|
| `--api URL`     | `SYTUE_CLOUD_API`    | `http://127.0.0.1:8090` |
| `--token TOKEN` | `SYTUE_CLOUD_TOKEN`  | —                       |
| `-o, --output`  | `SYTUE_CLOUD_OUTPUT` | `table`                 |

El token es un `scl_...` (scope `read<write<deploy`) o un JWT de Sytue ID.
**Prioriza `$SYTUE_CLOUD_TOKEN`**: `--token` queda en el history del shell y es
visible en `ps`. El token nunca se imprime ni se registra en logs.

```sh
export SYTUE_CLOUD_TOKEN=scl_xxx
export SYTUE_CLOUD_API=https://cloud.sytue.eu
```

Salida por defecto en **tabla** alineada; `-o json` devuelve el cuerpo crudo de
la API para scripting (`| jq`). Errores a stderr, código de salida ≠ 0.

## Comandos

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

Las flags y los posicionales se pueden intercalar: `deployments get <id> -o json`
y `deployments get -o json <id>` son equivalentes.

## No cubierto (la API no lo expone; no se inventa)

- **Streaming de logs (`--follow`)**: no hay endpoint SSE/WS. Solo log de build
  estático (`GET /builds/{id}/logs`); `--follow` devuelve error explícito.
- Recursos con endpoint pero sin comando aún (YAGNI hasta que se pidan): org,
  members, tokens, uptime-checks, alerts, builds (list/get), `usage/detail`,
  `billing/spend-limit`, `billing/invoices:generate`, project GitHub connect,
  almacenamiento, DB Studio y la URL de despliegue (se emite desde el panel, y
  por API es `POST /tokens` con `project_id` + scope `deploy`).
  Se añaden con el mismo patrón cuando haga falta.

> La línea «los logs por proyecto no existen» estuvo aquí hasta el 2026-08-17,
> cuando ya llevaban olas existiendo: un README que envejece miente con más
> autoridad que el código.
