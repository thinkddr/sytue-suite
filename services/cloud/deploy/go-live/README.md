<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

<a id="sytue-cloud--artefactos-ejecutables-de-go-live"></a>
# Sytue Cloud — Executable go-live artifacts

**Turnkey** artifacts that turn the prose runbook
[`docs/operations.md#5-go-live-de-sytue-cloud`](../../../../docs/operations.md#5-go-live-de-sytue-cloud) into (almost) commands.
The narrative guide (what each piece enables, what remains gated) is still that one; here
is the executable “how”.

> **Honesty.** These artifacts **PREPARE**, they do not switch anything on. Nothing starts without
> credentials and live Scaleway infra. What still needs infra/code is
> marked below and in each file.

<a id="ficheros"></a>
## Files

| File | What it is | When it is used |
|---|---|---|
| [`github-app-manifest.json`](./github-app-manifest.json) | “Create GitHub App from manifest” manifest with the real minimum permissions (contents:read, pull_requests:write, metadata:read), push/pull_request events and the callback (`/api/v1/github/callback`) and webhook (`/webhooks/git`) URLs. | Runbook §6 (GitHub App). |
| [`secrets.example.yaml`](./secrets.example.yaml) | Template of the **names** of all go-live secrets, grouped by K8s Secret. Placeholders, never values. | §0/§2/§5/§6 (depositing secrets). |
| [`values-go-live.yaml`](./values-go-live.yaml) | Prod Helm overlay: `RUNNER_MODE=kube`, build images by `@sha256:<PIN>`, `CERT_ISSUER`, domains, TLS. | §2/§3 (deploying the charts). |
| [`verify.sh`](./verify.sh) | Committed script that runs the §7 checklist as best-effort commands (health, pods, secrets, smoke and backup CronJobs). | §7 (verification). |

<a id="orden-de-uso"></a>
## Order of use

1. **Provision infra** with Terraform (`infra/terraform/environments/prod`) — runbook §2.
2. **Register the GitHub App** from `github-app-manifest.json` — runbook §6. Map
   the conversion response (`id`/`pem`/`client_id`/`client_secret`/`webhook_secret`)
   to `GITHUB_APP_ID`/`GITHUB_APP_PRIVATE_KEY`/`GITHUB_APP_CLIENT_ID`/`GITHUB_APP_CLIENT_SECRET`/`WEBHOOK_SECRET`.
3. **Mint and deposit secrets** (Scaleway Secret Manager → External Secrets Operator) according to
   `secrets.example.yaml`. Create `sytue-registry` in `sytue-paas-builds`.
4. **Re-pin by digest** the build images (`crane digest …` / `skopeo`) and
   set them in `values-go-live.yaml` — runbook §3.
5. **Deploy the charts** with the overlay (commands in the header of
   `values-go-live.yaml`) — runbook §2.
6. **Verify**: `services/cloud/deploy/go-live/verify.sh` — runbook §7.

<a id="huecos-siguen-necesitando-códigoinfra"></a>
## Gaps (still need code/infra)

Verified against the real code and charts:

- ✅ **Chart env contract — RESOLVED** (`eb18de7`). The 3 charts
  (`cloud-api/builder/controller`) were reconciled with `config.go`: they inject the 4
  required ones (`DATABASE_URL`, `SECRET_ENC_KEY`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`) via
  `secretKeyRef` and the feature env (`GITHUB_APP_*`, `CNB_*`, `KANIKO_IMAGE`,
  `WEBHOOK_SECRET`, `SCW_*`, `S3_BACKUP_*`) via their real consumer, `secretKeyRef
  optional:true`. Verified with `helm lint`/`template` (default and with this overlay).
  *(Before: `cloud-api` and `cloud-builder` used dead names (`PAAS_*`/`CONTROL_DB_URL`/
  `KANIKO_JOBS_NAMESPACE`…) and did not provide the required ones → they crashed on startup.)*
- ✅ **Sytue One subscription**: Sytue ID handles the sign-up and renewal of
  6 EUR/user/month through Mollie. Cloud contains no second payment gateway and does not
  accept payment webhooks; it only receives identities already authorized by ID.
- **Image digests**: the `@sha256:<PIN>` values in `values-go-live.yaml` are
  placeholders; re-pin with `crane`/`skopeo` before prod.

With the chart contract resolved, activation is **only credentials + infra**: without
the real Scaleway account and GitHub App, none of this starts — but no glue is missing anymore.
