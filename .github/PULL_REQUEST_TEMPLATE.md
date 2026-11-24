<!-- SPDX-License-Identifier: Apache-2.0 -->

<a id="descripción"></a>
## Description

<!-- What changes and why. Reference the issue if there is one. -->

<a id="checklist-de-paridad-y-versiones"></a>
## Parity and versions checklist

- [ ] **Web → native parity (CONVENTIONS §13.5)**: if this PR adds or touches a web app feature, the registry `apps/frontend/packages/config/src/paridad.ts` includes it with a status in `escritorio` and `movil` ('si', or 'planificado' with `deuda`). TopBar = the only exception.
- [ ] In every wave with pending web features in the registry: ci-desktop and ci-mobile run when the release is created — do not cut one with a new 'planificado' without its `deuda`.
- [ ] Dependencies/Actions/images at the latest stable version, with their row in `compliance/supply-chain/blocked-baseline.tsv` if it introduces a non-EU host.
- [ ] Gates run locally BEFORE pushing (the pre-push hook runs them).
- [ ] A test left behind (§13.4) and, if there is new SQL, an integration test against Postgres.
