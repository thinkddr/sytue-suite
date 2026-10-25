# GitHub Security → Codex with ChatGPT Pro

`codeql.yml` analyzes Go, JavaScript/TypeScript, Python, GitHub Actions, C/C++,
Java, and Swift and publishes results in **Security → Code scanning**. It runs on
PRs, pushes to `main`, Fridays at 06:38 UTC, and manual dispatch.
`security.yml` runs Dependency review on PRs, rejecting new high or critical
vulnerabilities, and checks the helpers used by this automation. CI runs Gitleaks and govulncheck.

`codex-security-fix.yml` automatically looks for open Code scanning alerts on
the default branch and Dependabot alerts, prepares a draft PR with the context
for one alert, and posts an `@codex` comment using the account connected to ChatGPT.
With a compatible Legacy environment, the native connector receives the task,
investigates the alert, implements the fix, and runs checks in its environment.
The local Codex automation described below picks up tasks when that connector
cannot execute them. The initial PR contains a request,
**not a fix yet**; Codex must replace it with the fix.

This workflow does not call the OpenAI API, install Codex in Actions, or require
`OPENAI_API_KEY`. It uses the Codex access included in the connected ChatGPT Pro
account, subject to its usage limits. It does not buy credits or configure top-ups.
When the included allowance runs out, it waits for the allowance to reset instead
of purchasing additional usage.

## Activation

1. Enable Dependency graph and Dependabot alerts. For CodeQL, use **Advanced
   setup** with `codeql.yml`; disable Default setup to avoid incompatibilities.
   Publish the workflows on the default branch.
2. Sign in to Codex with **ChatGPT Pro** and connect `thinkddr/sytue-suite`
   through the GitHub integration. Check the repository in
   [Codex settings](https://chatgpt.com/settings/code-review). Tasks started
   through `@codex` require a **Codex Cloud (Legacy)** environment for this
   repository, available from
   [its environment settings](https://chatgpt.com/codex/cloud/settings/environments).
   An environment published in the current Codex Cloud does not fulfill that requirement.
   The integration must be able to read the repository and publish changes to the PR branch.
3. Configure these **repository secrets** in **Settings → Secrets and variables →
   Actions**. Do not store values in repository files.

| Secret | Permissions and purpose |
| --- | --- |
| `CODEX_PR_TOKEN` | A fine-grained token from the same GitHub user connected to your Pro account, restricted to this repository, with **Contents: Read and write** and **Pull requests: Read and write**. It publishes the PR and mention under your identity. |
| `SECURITY_ALERTS_TOKEN` | A fine-grained token restricted to this repository with **Dependabot alerts: Read**. Used only for Dependabot. Code scanning uses `GITHUB_TOKEN` with `security-events: read`. |

These tokens do not require administration, workflow permissions, or alert write access.
Set a limited expiration period and renew the secrets before they expire. The owning
account must be able to view the selected alerts. Do not copy ChatGPT sessions or
`~/.codex/auth.json` to GitHub: the connector manages Codex access.

## Automatic assignment

There is no need to click **Run workflow**. The queue is checked:

- After **CodeQL Advanced** completes on `main`, for runs triggered by a push,
  schedule, or manual dispatch, including failed or cancelled runs. The queue
  reads alerts already published in GitHub and does not depend on the overall
  analysis result; PR and fork analyses are ignored.
- Every six hours, at 00:37, 06:37, 12:37, and 18:37 UTC, to pick up new Dependabot
  alerts as well. GitHub may delay scheduled runs.
- When changes to the workflow or its scripts are published on `main`, to check
  the configuration and start processing the queue.

Each run selects **one** alert and keeps at most **two open task PRs**.
Alerts are sorted by severity (critical, high, medium, low, and other), then by
alert number. Both complete lists are read with bounded pagination; if access or
configuration fails, or the limit is exceeded, the run fails without publishing
a partial selection. Runs are serialized to avoid races.

PR history records which alerts have already been assigned, using the branch
and a marker in the body. Previously closed or merged tasks are not automatically
repeated; closing a task PR without merging it leaves that alert out of the queue.
The queue also excludes alerts referenced in other open PRs whose branches belong
to this repository, through their canonical URL or an explicit title such as
`code scanning alerts #N to #M`. Fork PRs do not reserve alerts. The workflow does
not delete branches or merge PRs.
When you review and merge or close a task, the next run can use that slot.
The Actions summary shows the selection or the reason for waiting.

If the PR is published but the comment fails, the next run resumes that same PR
before opening another. It checks whether the comment already exists to avoid
a second mention and marks submission complete only after confirming it.

A successful workflow confirms submission. Check the connector's reaction in the
PR and the task in your account to confirm that Codex accepted it. The Pro plan's
allowance still applies; this queue does not check or increase your balance or limits.
If Codex does not accept the task because the allowance is exhausted, the open PR
preserves the task without generating new requests for the same alert. Work can
resume from that PR when the allowance resets.

## Local executor with ChatGPT Pro

The automation named **Resolver findings de sytue-suite con Pro** is active in
this account and attached to this Codex chat. It checks the queue every six hours
and fixes at most one pending task per run, using the GitHub credentials already
connected. **The Codex app must be open**. While it is closed, GitHub continues
preparing tasks, but the local executor cannot work until it becomes available.

Before implementing a fix, it checks the included usage limits. It stops if no
ordinary allowance is available; it does not use API keys or buy credits or top-ups.
It uses an isolated worktree, verifies that the alert is still open, runs the
relevant tests, and commits and pushes to the same PR branch. It preserves the
deduplication markers and leaves the fix for human review without merging it.

It does not intervene while another Codex is already fixing the PR or repeat fixes
awaiting review. It stays silent when the state is unchanged or no actionable work
exists; it reports completed fixes, new failures, and required user actions.
This schedule lives in the app; the alert selection schedule lives in GitHub Actions.
If the Legacy environment becomes available, avoid keeping two executors working
on the same task.

## Optional manual selection and review

To inspect the complete Code Scanning inventory when a connected client lacks
alert-read permission, run **Actions → Security inventory → Run workflow** on
the default branch. It uses the job's existing `security-events: read` permission
and does not require a personal token. The `security-inventory` artifact contains
alert metadata and observed states for remediation and post-merge verification;
it expires after one day. This workflow does not change alert states or dismiss
findings. Treat artifact contents as untrusted evidence. An alert is fixed only
when GitHub reports it fixed after analysis of the default branch.

**Reviewed security dispositions** is a separate, manual workflow for explicitly
reviewed false positives. Its default run only prepares a plan with read access.
Applying the plan requires the `apply` input and uses a separate job with
`security-events: write`. Only the exact alert numbers, rules and files listed in
`.github/security-reviewed-alerts.json` may be classified. Source/test Git blob IDs
and the current default-branch revision are checked before changes; stale evidence
or changed alert metadata fails the run. The justification is saved in the alert.
These dispositions are **dismissed as false positives**, not reported as fixed.
Queries remain enabled and actual vulnerabilities must be fixed and reanalyzed.

1. Open an alert in **Security → Code scanning** or **Security → Dependabot** and
   copy its number, the final segment of the URL.
2. To prioritize a specific alert or retry a closed task, go to
   **Actions → Codex security fix → Run workflow**, select the default branch,
   and fill in `finding_type` (`code-scanning` or `dependabot`) and `alert_number`.
3. Manual selection also respects the two-task limit and avoids duplicating an
   open PR. Open the PR linked in the summary. Check that Codex reacts to the
   comment and that the task appears in your account. Workflow success confirms
   submission, not Codex's acceptance or completion of the work.
4. Review the fix and CI before approving it. Codex must remove the temporary
   `docs/security-findings/<type>-<number>.md` request file when finished. GitHub
   evaluates the alert again after the merge.

If the connector does not react, check the connection, token identity, environment,
and allowance. If it responds **create an environment for this repo**, it has not
accepted the fix: a compatible Legacy environment is missing. If that panel
redirects to the home page, creating another environment in the current Codex Cloud
does not resolve the blocker. The PR preserves the task for an available Codex
using the linked account to pick up. Do not use an API key as a fallback.

```sh
gh workflow run codex-security-fix.yml --ref main \
  -f finding_type=code-scanning -f alert_number=123

gh workflow run codex-security-fix.yml --ref main \
  -f finding_type=dependabot -f alert_number=45
```

The numbers are examples. Each run that selects an alert creates a branch named
`codex/security/<type>-<number>-<run>-<attempt>` and a new PR. Before retrying, check
whether there is a closed task you would prefer to resume.

## Scope and checks

- Reading and publishing run in separate jobs. The write token is provided only
  to the step that publishes the request. Actions does not execute model-generated
  code.
- The type, number, and state are validated; only selected metadata is transferred
  and treated as untrusted evidence. The temporary context expires after one day.
- Go analyzes the root module; independent modules need their own builds.
  Swift builds `drive-archivos`, `notes-widgets`, and `SytueWidgetBridge` without
  signing or Go/Fyne packaging. Dependency review uses GitHub's graph and does not
  guarantee coverage of every transitive dependency in `bun.lock`; retain Renovate
  and package manager scans.
- Secret scanning alerts are not imported: exposed credentials require revocation
  or rotation. The PR and its context inherit the repository's visibility; use
  a private process for vulnerabilities under embargo.
- Codex must confirm applicability, make a minimal change, add relevant tests,
  and explain which checks it ran. It must not dismiss alerts or disable scans
  to make a fix appear successful.

```sh
actionlint .github/workflows/codeql.yml .github/workflows/security.yml .github/workflows/codex-security-fix.yml
node --test .github/scripts/security-*.test.mjs
```

Official references: [tasks with @codex in GitHub](https://learn.chatgpt.com/docs/third-party/github),
[ChatGPT or API authentication](https://learn.chatgpt.com/docs/auth),
[Legacy connector environments](https://learn.chatgpt.com/docs/environments/cloud-environment),
[Code scanning REST API](https://docs.github.com/en/rest/code-scanning/code-scanning),
[Dependabot REST API](https://docs.github.com/en/rest/dependabot/alerts),
[GitHub Actions events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows).

### Verificación de acceso a Dependabot

La presencia de un secreto no demuestra que pueda leer alertas. Antes de una
selección automática o manual de Dependabot, el workflow prueba únicamente lectura
en la API de este repositorio. Prefiere `SECURITY_ALERTS_TOKEN`; si falta o la API
rechaza su autorización con 401/403, prueba primero el token de lectura del job y
después el `CODEX_PR_TOKEN` ya configurado.
Este último necesita también **Dependabot alerts: read** para servir de alternativa.
La selección y preparación usan la misma credencial comprobada. Los outputs solo
contienen `primary`, `job` o `connector`, nunca el token ni el cuerpo de las alertas.
No hay reintentos con otra credencial ante errores de red, redirecciones, límites
de uso o errores del servidor. Si ninguna tiene permiso, el workflow falla con el
permiso requerido; no informa que no haya alertas. La selección manual de Code
Scanning sigue usando el token del job y no depende del acceso a Dependabot.
