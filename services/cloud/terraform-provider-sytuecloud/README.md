<!-- SPDX-License-Identifier: Apache-2.0 -->
# terraform-provider-sytuecloud

Terraform provider for the **Sytue Cloud** control plane (the PaaS in
`services/cloud`). Manage projects, environment variables, custom domains and
managed backing services as IaC — parity with the Scaleway/DigitalOcean
providers (roadmap WS7/E3).

Built on [`terraform-plugin-framework`](https://github.com/hashicorp/terraform-plugin-framework)
(HashiCorp, **MPL-2.0** — permitted). It is a **separate, nested Go module**
(`services/cloud/terraform-provider-sytuecloud`) so the framework's heavy
dependency tree never enters the monorepo root `go.mod`.

## Configuration

| Setting    | Provider attribute | Env var             | Default                       |
|------------|--------------------|---------------------|-------------------------------|
| API base   | `endpoint`         | `SYTUE_CLOUD_API`   | `https://cloud-api.sytue.eu`  |
| Auth token | `token` (Sensitive)| `SYTUE_CLOUD_TOKEN` | — (required)                  |

The token is a `scl_...` API token (scope `write`/`deploy`) or a Sytue ID JWT,
sent as `Authorization: Bearer <token>`. It is marked `Sensitive` (never
printed) and TLS verification is always on.

```hcl
terraform {
  required_providers {
    sytuecloud = { source = "sytue/sytuecloud" }
  }
}
provider "sytuecloud" {} # reads SYTUE_CLOUD_TOKEN / SYTUE_CLOUD_API
```

See [`examples/main.tf`](examples/main.tf) for a full project + env var +
domain + managed service.

## Resources

| Resource                      | Create | Read (drift → remove on 404) | Update            | Delete | Import ID              |
|-------------------------------|:------:|:----------------------------:|-------------------|:------:|------------------------|
| `sytuecloud_project`          | ✅     | GET `/projects/{id}`         | PATCH             | ✅     | `<id>`                 |
| `sytuecloud_env_var`          | ✅ PUT | list + match by key          | PUT (upsert)      | ✅     | `<project_id>/<key>`   |
| `sytuecloud_domain`           | ✅     | list + match by id           | replace-only¹     | ✅     | `<project_id>/<id>`    |
| `sytuecloud_managed_service`  | ✅     | GET `/services/{id}`         | replace-only¹     | ✅     | `<id>`                 |

¹ The API exposes no update endpoint for domains/services, so any change forces
replacement (`RequiresReplace`).

### Data source

- `sytuecloud_project` — look up an existing project by `id` **or** `name`.

### Notes / behaviour

- **`slug`** is required on `sytuecloud_project` (the API's create key) and is
  immutable, as is `git_provider`.
- **Secret env vars**: the API masks secret values on read, so the provider
  reconciles the value only for non-secret vars; a secret's value drift is
  invisible by design.

## Deferred (documented, not implemented)

Kept out to cover the CORE ~80% of use while proving the pattern:

- **`deployment`** — deployments are *imperative* (trigger a build/deploy),
  not a declarative resource; use the `sytue-cloud deploy` CLI or CI. Not a
  Terraform resource.
- **`member` / `api_token`** — org membership and token issuance (tokens return
  a one-time secret; awkward as declarative state).
- **`uptime_check` / `alert_policy` / `billing`** — same CRUD pattern as the
  implemented resources; add on demand.
- **GitLab** git provider connection (`/projects/{id}/github` is GitHub-only
  today).

## Testing / verification

```sh
go build ./... && go vet ./... && go test ./... && gofmt -l .
```

Unit tests run with **no network**: `internal/client` uses `httptest.Server`
(create/read/update/delete/404/typed-error); `internal/provider` validates
every resource/data-source schema.

**Acceptance tests** (`TestAccProjectResource`) use `resource.Test` and are
gated by `TF_ACC` plus `SYTUE_CLOUD_TOKEN`; they **skip** by default.
End-to-end verification against a live control plane is **deferred** until a
disposable test tenant exists.
