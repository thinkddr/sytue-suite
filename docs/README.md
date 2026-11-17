<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos (thinkddr)
-->

<a id="documentación--sytue-suite"></a>

# Documentation — Sytue Suite

**One topic, one document.** All the documentation lives in the nine files below. If two
contradict each other, `CONVENTIONS.md` wins; fix the other one in the same change.

| Document                               | What it contains                                                                                                                                                                                                                       | When to open it                                                               |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| ★ **[CONVENTIONS.md](CONVENTIONS.md)** | **Canonical contract**: core principles, product catalog, stack versions, ports, monorepo layout, identity, tenancy, licenses and, in annex §16, naming, domains and DNS.                                                              | Before naming, versioning or exposing anything. It overrides everything else. |
| **[architecture.md](architecture.md)** | Suite architecture (C4, SSO/intents/deploy flows, multi-tenancy, topology), Shell+Hub integration, Sytue Cloud, Integration Gateway (outbound), compatibility layer (inbound), enterprise identity and the home of documents in Drive. | To understand how the pieces fit together before touching one.                |
| **[decisions.md](decisions.md)**       | The architecture decisions (ADRs) with context, rejected alternatives and consequences, including the “Sytue Native” pivot and [0037](decisions.md#0037), which sets how code is shared.                                               | When you wonder _why_ something is the way it is, or are about to change it.  |
| **[security.md](security.md)**         | Security and GDPR, STRIDE threat model, OWASP ASVS L2 checklist, clean-room policy and licenses.                                                                                                                                       | When touching auth, personal data, dependencies or the license gate.          |
| **[operations.md](operations.md)**     | Deployment and operations guide: interactive CLI (`./sytue-deploy`), Docker Compose, Kubernetes/Helm, environment variables, migrations, identity, tenants, mail, backups, secrets, CI/CD, e2e tests and performance/SEO.              | To bring up, deploy, configure, verify or recover something.                  |
| **[design.md](design.md)**             | Art direction, design system (`@sytue/ui`, `--sytue-*` tokens, AppShell, components, WCAG AA, guided setup) and assets.                                                                                                                | Before writing a single line of UI.                                           |
| **[roadmap.md](roadmap.md)**           | Status and maturity per product, delivered architecture and the suite's public roadmap.                                                                                                                                                | To learn the current capabilities and how the suite is evolving.              |
| **[paridad-pdf.md](paridad-pdf.md)**   | Sytue CertiPDF feature by feature, with the reference implementation of each tool.                                                                                                                                                     | To look up the PDF editing, eIDAS signing and OCR capabilities.               |
| **[business.md](business.md)**         | Platform vision (positioning, interoperability, Platform OS) and sizing model.                                                                                                                                                         | To understand the product and interoperability approach.                      |

**Outside `docs/`:** `branding/` is the source of truth for logos, icons, illustrations and tokens.

<a id="reglas"></a>

<a id="reglas"></a>

## Rules

- **Do not create new files in `docs/`.** New material goes in as a section of the document it
  belongs to, with its `<a id="...">` so it can be linked. If it really does not fit in any of them,
  that is a sign that a decision is missing, not a file.
- **Before writing code**, read [`CONVENTIONS.md` §13.2](CONVENTIONS.md#13-2-lo-que-ya-existe):
  the catalog of what already exists in `platform/pkg/` and in `apps/frontend/packages/`, what is
  NOT unified and why, and the tests that turn red when someone copies plumbing again.
- **Every claim, anchored to the code.** The documents cite `fichero:línea` (file:line) or the command that
  verifies it; what cannot be verified is marked as pending, not presented as done.
- **When you deliver something, update it where it was recorded** (status in `roadmap.md`, decision in
  `decisions.md`), not in a new document.
