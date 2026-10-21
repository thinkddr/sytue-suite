<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue Cloud — Artefactos ejecutables de go-live

Artefactos **turnkey** que convierten el runbook en prosa
[`docs/operations.md#5-go-live-de-sytue-cloud`](../../../../docs/operations.md#5-go-live-de-sytue-cloud) en (casi) comandos.
La guía narrativa (qué activa cada pieza, qué queda gated) sigue siendo esa; aquí
está el «cómo» ejecutable.

> **Honestidad.** Estos artefactos **PREPARAN**, no encienden. Nada arranca sin
> credenciales e infra Scaleway viva. Lo que aún necesita infra/código está
> marcado abajo y en cada fichero.

## Ficheros

| Fichero | Qué es | Cuándo se usa |
|---|---|---|
| [`github-app-manifest.json`](./github-app-manifest.json) | Manifest «Create GitHub App from manifest» con los permisos mínimos reales (contents:read, pull_requests:write, metadata:read), eventos push/pull_request y las URLs de callback (`/api/v1/github/callback`) y webhook (`/webhooks/git`). | §6 del runbook (GitHub App). |
| [`secrets.example.yaml`](./secrets.example.yaml) | Plantilla de **nombres** de todos los secretos de go-live, agrupados por Secret de K8s. Placeholders, nunca valores. | §0/§2/§5/§6 (depositar secretos). |
| [`values-go-live.yaml`](./values-go-live.yaml) | Overlay de Helm de prod: `RUNNER_MODE=kube`, imágenes de build por `@sha256:<PIN>`, `CERT_ISSUER`, dominios, TLS. | §2/§3 (desplegar los charts). |
| [`verify.sh`](./verify.sh) | Script guardado que ejecuta el checklist §7 como comandos best-effort (health, pods, secrets, smoke y CronJobs de backup). | §7 (verificación). |

## Orden de uso

1. **Aprovisionar infra** con Terraform (`infra/terraform/environments/prod`) — runbook §2.
2. **Registrar la GitHub App** desde `github-app-manifest.json` — runbook §6. Mapea
   la respuesta de conversión (`id`/`pem`/`client_id`/`client_secret`/`webhook_secret`)
   a `GITHUB_APP_ID`/`GITHUB_APP_PRIVATE_KEY`/`GITHUB_APP_CLIENT_ID`/`GITHUB_APP_CLIENT_SECRET`/`WEBHOOK_SECRET`.
3. **Acuñar y depositar secretos** (Scaleway Secret Manager → External Secrets Operator) según
   `secrets.example.yaml`. Crear `sytue-registry` en `sytue-paas-builds`.
4. **Re-pinnear por digest** las imágenes de build (`crane digest …` / `skopeo`) y
   fijarlas en `values-go-live.yaml` — runbook §3.
5. **Desplegar los charts** con el overlay (comandos en la cabecera de
   `values-go-live.yaml`) — runbook §2.
6. **Verificar**: `services/cloud/deploy/go-live/verify.sh` — runbook §7.

## Huecos (siguen necesitando código/infra)

Verificado contra el código y los charts reales:

- ✅ **Contrato de env de los charts — RESUELTO** (`eb18de7`). Los 3 charts
  (`cloud-api/builder/controller`) se reconciliaron con `config.go`: inyectan las 4
  obligatorias (`DATABASE_URL`, `SECRET_ENC_KEY`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`) por
  `secretKeyRef` y la feature-env (`GITHUB_APP_*`, `CNB_*`, `KANIKO_IMAGE`,
  `WEBHOOK_SECRET`, `SCW_*`, `S3_BACKUP_*`) por su consumidor real, `secretKeyRef
  optional:true`. Verificado con `helm lint`/`template` (default y con este overlay).
  *(Antes: `cloud-api` y `cloud-builder` usaban nombres muertos —`PAAS_*`/`CONTROL_DB_URL`/
  `KANIKO_JOBS_NAMESPACE`…— y no proveían las obligatorias → crasheaban al arrancar.)*
- ✅ **Suscripción de Sytue One**: Sytue ID aplica el alta y la renovación de
  6 EUR/usuario/mes mediante Mollie. Cloud no contiene una segunda pasarela ni
  acepta webhooks de pago; recibe únicamente identidades ya autorizadas por ID.
- **Digests de imágenes**: los `@sha256:<PIN>` de `values-go-live.yaml` son
  placeholders; re-pinnear con `crane`/`skopeo` antes de prod.

Con el contrato de charts resuelto, la activación es **solo credenciales + infra**: sin
la cuenta Scaleway y la GitHub App reales, nada de esto arranca — pero ya no falta pegamento.
