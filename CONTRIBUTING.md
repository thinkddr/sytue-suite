<a id="contribuir-a-sytue-one"></a>
# Contributing to Sytue One

Thank you for contributing. Sytue One is a **100% EU, sovereign** suite, reimplemented as an
**original product** under the Apache-2.0 license (clean-room on open standards). Before
you start, read the canonical contract [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md): it is the
**source of truth** for names, domains, ports, versions, OIDC clients and licenses.
Every change must match it.

<a id="stack-único"></a>
## Single stack

`Go + Astro + Preact + PostgreSQL` (+ S3 for blobs). **100% Go backend** (single module
`git.sytue.eu/sytue/one`, one binary per service). **Frontend** Astro + Preact. **Single
datastore** PostgreSQL (state, queues and cache). **No Redis/Valkey** and **no third-party
AGPL engine at all** (Keycloak, Nextcloud, OnlyOffice, Mattermost, Stalwart are out). The
only third-party app is **LiveKit** (Apache-2.0), the media SFU for Sytue Meet.

**Versions are not quoted here.** The policy is **always the latest stable** and the live
numbers are in the table in
[`docs/CONVENTIONS.md` §3](docs/CONVENTIONS.md#3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro),
which is their source of truth, and `make toolchain-check` checks that the environment matches. This
paragraph used to say “Astro 5” and “PostgreSQL 16” when the table was already at 7 and 18: a number
repeated in prose goes stale without anyone noticing.

<a id="principios-que-rigen-toda-contribución"></a>
## Principles that govern every contribution

1. **Design first, and cross-cutting.** Use the `@sytue/ui` components and mount
   `@sytue/shell`. Do not create ad-hoc UI or component variants.
2. **The IdP is core.** All authentication goes through **Sytue ID** (in-house Go service) via
   OIDC + PKCE with a BFF and an httpOnly `.sytue.eu` cookie; each service validates JWTs via JWKS.
   No product manages its own credentials.
3. **Seamless integration.** Reuse Sytue Hub (search, notifications, presence,
   intents) instead of duplicating integration.
4. **100% EU.** Only Scaleway EU regions. Do not introduce dependencies on US
   providers for personal data.
5. **Clean-room and Apache-2.0 license.** Product code is **Apache-2.0, © 2026 Daniel Delgado
   Ramos (thinkddr)**. Only **permissive third-party dependencies** are allowed
   (MIT/BSD/Apache-2.0/ISC/PostgreSQL/MPL-2.0); copying/porting AGPL/GPL or
   _source-available_ code is **forbidden**. Implementers work from RFCs/specs, **never** from the code of
   AGPL engines. See [`docs/security.md#4-politica-de-reimplementacion-clean-room`](docs/security.md#4-politica-de-reimplementacion-clean-room) and
   [`docs/security.md#5-licencias-propiedad-y-cumplimiento`](docs/security.md#5-licencias-propiedad-y-cumplimiento).
6. **Security and privacy by default.** Multi-tenant RLS, parameterized queries,
   deny-by-default, secrets only in a secrets manager (placeholders in git).

<a id="entorno-de-desarrollo"></a>
## Development environment

```bash
./sytue-deploy       # genera deploy/compose/.env y levanta toda la suite (*.sytue.localhost)
make dev-up          # después: reconstruye lo cambiado y arranca

make go-build        # compila todos los servicios Go
make go-test         # tests del módulo

make fe-install      # dependencias del frontend (Bun; versión en §3 de CONVENTIONS)
make fe-dev          # apps Astro en caliente (4321-4331)
```

More detail in the [README](README.md) and in [`docs/operations.md#1-runbook-de-operacion`](docs/operations.md#1-runbook-de-operacion).

<a id="el-editor-y-los-gates-tienen-que-decir-lo-mismo"></a>
### The editor and the gates have to say the same thing

`.vscode/` is not versioned (personal settings are personal), but there are four settings that are not
a matter of taste but facts about the repository. Without them the editor fills up with red and yellow marks that
the console does not see, and you end up distrusting both. In VS Code, `.vscode/settings.json`:

```jsonc
{
  // El TypeScript DEL REPOSITORIO, no el que trae el editor: con una versión anterior,
  // opciones que solo existen en la nuestra salen mal en pantalla y bien en consola.
  "typescript.tsdk": "apps/frontend/node_modules/typescript/lib",
  "typescript.enablePromptUseWorkspaceTsdk": true,
  // Tailwind 4 usa `@theme` y `@source`; el validador de CSS del editor no los conoce.
  "css.lint.unknownAtRules": "ignore",
  // shellcheck tiene que seguir los `source` (-x) y resolverlos desde el directorio del
  // script (-P SCRIPTDIR); si no, inventa 24 avisos de «no encuentro common.sh».
  "shellcheck.customArgs": ["-x", "-P", "SCRIPTDIR"],
  // Los tests de integración de Go viven detrás de `//go:build integration`; sin la
  // etiqueta, gopls los da por excluidos del build.
  "gopls": { "build.buildFlags": ["-tags=integration"] },
}
```

In other editors, the equivalent: the workspace TypeScript SDK, CSS validation that does not
complain about unknown at-rules, and the `integration` build tag for the Go
language server.

And the other way round: **if the editor flags something the gate does not, it usually means the
gate is missing coverage**, not that the editor is overreaching. It happened with 139 test files (`49c0bd2d`): they were
outside the `include` of their `tsconfig`, so the editor checked them and the console did not.

<a id="la-documentación-no-la-formatea-prettier"></a>
### Documentation is not formatted by Prettier

The `.md` files at the root and in `docs/` **are kept out of Prettier on purpose**, not by
oversight: it reflows tables by aligning every cell, and with 300-character cells (there are
plenty of them) a single row becomes a 500-character line that nobody reads in a diff.
It would change ten thousand lines without improving a single comma of what the reader sees.

What is checked is what can lie: `make docs-links` verifies that **every
internal link leads somewhere** (the file exists and the anchor exists inside it).
It is part of `make gates`. In documentation that is the source of truth, a broken link does
more harm than a misaligned table.

<a id="convenciones-de-código"></a>
## Code conventions

- **Go:** single module `git.sytue.eu/sytue/one`; one binary per service
  (`services/<svc>/cmd/<svc>`); `internal/{http,domain,store}` layers; wrapped errors;
  `context` on all I/O; `slog` for logs; versioned migrations;
  **parameterized** queries (no dynamic SQL). `gofmt`, `go vet`, `golangci-lint`, tests
  (`make go-fmt`, `make go-lint`, `make go-test`). SPDX header (`Apache-2.0`) and copyright notice in every file.
- **Frontend:** **strict TypeScript**; **oxlint + Prettier** (`make lint`, `make fmt`);
  ESLint only covers `*.astro` files, which oxlint does not parse yet.
  No UI outside `@sytue/ui`.
- **Terraform** formatted (`make tf-fmt`), reusable modules, state in a Scaleway S3
  backend.
- **Product documentation in English**; code identifiers in English. Some code comments and UI defaults are still in Spanish.
- Respect the monorepo structure (§7 of CONVENTIONS.md). Do not duplicate utilities that
  already live in `platform/pkg/*` (Go) or in the `@sytue/*` packages (frontend).

<a id="flujo-de-trabajo"></a>
## Workflow

1. Create a branch from `main`: `feat/<área>-<descripción>` or `fix/<...>`.
2. Make small, focused changes; add/update documentation where appropriate.
3. Run `make fmt` and `make lint` before pushing.
4. Open a Pull Request describing the what and the why, linking to the relevant
   CONVENTIONS.md section or ADR. If you introduce an architecture decision, add an
   ADR in [`docs/decisions.md`](docs/decisions.md).
5. Make sure the change keeps **dev/prod parity** and does not break the contract.

<a id="definition-of-done-resumen"></a>
## Definition of Done (summary)

- Backend in Go; data in PostgreSQL (multi-tenant via RLS); blobs in S3.
- Mounts `@sytue/shell` + uses `@sytue/ui` (if it is a user-facing surface).
- Authenticates via OIDC + PKCE against Sytue ID, validates JWTs via JWKS, `.sytue.eu` cookie.
- Works in local dev and is deployable on Scaleway (Helm/Terraform).
- 100% EU, no secrets in git, **no Redis/Valkey, no AGPL dependencies**.
- Documentation updated in English.

<a id="licencia-de-las-contribuciones"></a>
## License of contributions

**Pull requests are welcome** from anyone. The sole maintainer is
[@thinkddr](https://github.com/thinkddr) (Daniel Delgado Ramos): every PR requires his review and
approval, and only he can merge it. By contributing you agree that your contribution is published under
**Apache-2.0** (© 2026 Daniel Delgado Ramos, see [`LICENSE`](LICENSE)). Never include secrets,
credentials or real environment variable values. Vulnerabilities are reported through
*Private vulnerability reporting*, not in public issues.
