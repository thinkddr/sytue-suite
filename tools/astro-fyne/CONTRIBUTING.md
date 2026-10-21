<!-- SPDX-License-Identifier: Apache-2.0 -->
# Contributing

Use Bun 1.4.3 and the Go version in `native/go.mod`. Pin new dependencies and Actions to
verified stable versions. Keep the public distribution independent of private repositories,
fonts, product assets and services. Optional host adapters belong outside this distribution.

Open a pull request with a concrete source example, the expected browser behavior, the native
behavior and any affected rendering profile. A feature needs a meaningful positive scenario
and a case that exposes the previous failure. Run builds and tests on a remote machine or CI;
the checked-in workflow runs `bash ci/run.sh` on a hosted ARM64 runner. It retains evidence
even when a gate fails. Formatting and editing source do not require running a desktop app.

The gates cover compiler diagnostics, type safety, generated output freshness, native tests
with the race detector, browser/native event traces, an exact image comparison and Go
vulnerability analysis. Inspect the CI run before merging. A passing profile establishes that
profile, not universal compatibility. See [COMPATIBILITY.md](COMPATIBILITY.md).

Unsupported syntax or native behavior must produce a diagnostic. Do not discard attributes,
callbacks, styles or assets silently. Do not substitute an embedded browser or a screenshot
for interactive native widgets. Explicit visual tolerances must remain visible in results;
the default comparison accepts zero changed pixels.

Keep the source IR, lowering, native behavior and capture contract consistent when adding a
feature. Include resources in source hashes. Preserve mounting and event semantics. A
generation command must preserve handwritten files and complete source validation before
writing its artifacts.

This project does not accept external contributions: only the maintainer, Daniel Delgado
Ramos, commits to it. The code is Apache-2.0. Preserve existing copyright notices and add an SPDX identifier
to source files. Explain the provenance and license of bundled resources. Do not include
credentials, user data or third-party assets without a redistribution license.
