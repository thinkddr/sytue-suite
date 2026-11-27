<a id="cadena-de-suministro-ue"></a>
# EU supply chain

This directory puts into practice the criterion in `docs/CONVENTIONS.md`: to be
`eu-approved`, hosting data in a European region is not enough. The parent or
controlling entity, the distribution of the artifact and any data processing
must remain under European Union entities and operation.

<a id="dos-riesgos-distintos-que-este-inventario-mezcla-en-una-sola-columna"></a>
## Two different risks that this inventory mixes in a single column

Added on 2026-08-11 after reviewing it with the owner, who put it this way: **what
matters is that our project's contents do not go through online providers or
connections outside the EU**. That separates two things that today share the same
`status`, and confusing them leads to misreading the counter:

**A · EGRESS — something of ours travels outside.** Code, data, secrets, names of
repositories or services, dependency lists, artifact digests, telemetry, user
identifiers. This is the risk the criterion targets, and it is not fixed with a
permissive license or by hosting something in France: either it is cut, or it
leaves. Three real cases found and closed that same day:

- `osv-scanner` sent the project's full dependency inventory to `api.osv.dev` on
  **every** run. Cut with `--offline-vulnerabilities`.
- `cosign` published to the Sigstore transparency log (US) the full reference of
  each image, its digest and the date: the service inventory and the release
  cadence. Turned off at both ends.
- An empty `GOPRIVATE` would have sent the paths of our private modules
  `git.sytue.eu/...` to Google's module proxy and checksum database as soon as a
  `go.mod` required another one of ours. Closed before it happened.

**B · INGRESS — we bring in a public artifact.** A binary, a base image, a
module, a chart. Here **nothing of ours leaves**: the risk is supply-chain
integrity —being given something other than what we asked for— and it is mitigated
with a pinned digest, a checked sha256 and our own mirror, not by cutting the connection.

**Why the distinction matters when reading the counter.** Of the 49 `blocked` in
August 2026, **34 were not providers**. This section already said so about Go
module namespaces —`gonum.org`, `dario.cat`, `pgregory.net`, `go.uber.org`,
`gopkg.in`…—: with `GOPROXY=https://proxy.golang.org,direct` those domains are
**never contacted**, because the module is served by `proxy.golang.org`,
inventoried separately.

**On 2026-08-11 the `identifier` status that this text already assumed existed
was implemented** —it was documented but not in the code— and it was applied to
the other two families that meet the same condition:

- **Kubernetes API groups and label prefixes.** `monitoring.coreos.com`
  is an `apiVersion` field; `app.kubernetes.io/name` and `topology.kubernetes.io/zone`
  are labels. **232 lines** of `blocked-baseline.tsv` existed only to
  silence them.
- **Project sites cited in metadata**, such as the `home:` of a `Chart.yaml`
  (`min.io`, `prometheus.io`, `www.pgbouncer.org`). The real channel of those products
  —`docker.io`, `docker.dragonflydb.io`— remains blocked and counted separately.

`identifier` **relaxes nothing**: the row still requires evidence, an owner and a
review that expires after 183 days. The only change is that it is no longer counted
as a dependency, because it is not one. The counter ended at **15 actual download
channels**, which are the real work.

**And of those fifteen, five were closed the same day with `pinned-artifact`**: the
image registries (`docker.io`, `gcr.io`, `ghcr.io`, `quay.io`,
`docker.dragonflydb.io`). It is the literal application of what section B above
says —the INGRESS risk is about integrity and is mitigated with a digest, not by
cutting the connection—, and here the mitigation **exists and is checked**:

- At runtime the charts point only to `rg.fr-par.scw.cloud`; the twenty third-party
  images are in `upstream-images.txt` with an immutable digest and signed, and
  `upstream-vigencia-test.sh` rejects any that is not catalogued.
- At build time, the **thirty `FROM`** lines of the Dockerfiles carry `@sha256:`.

That last point is not taken on trust: `TestLosRegistrosFijadosLoEstanDeVerdad` walks
the Dockerfiles and fails if any of them loses its digest. The label describes a
real mitigation or it is worth nothing.

In addition, three rows contradicted their own content: `dario.cat` declares its
headquarters in **Spain**, `alpinelinux.org` its fiscal host in the **Open Collective
Europe Foundation (EU)** and `postgresql.org` in **Canada**. All three were listed as
blocked due to non-EU governance.

**And the counter goes up again the same day**, because the on-premise profile entered
git and brought with it four channels that were not visible before: `get.k3s.io`,
`download.docker.com`, `raw.githubusercontent.com` and `charts.jetstack.io`.
They are real downloads with no mitigation —two of them execute what they download
(`curl … | sh`) and one points to a third party's `main` branch—, so they go in as
`blocked` with their baseline row, not as `pinned-artifact`. The effective count is
**fourteen**, not ten: closing them requires pinning the version and checksum in
`deploy/helm/bin/install-host.sh`, which does not do so today.

Rule of thumb when classifying a row: **ask what travels and in which
direction**. If the answer is “something of ours, outward”, it is a non-compliance
that must be closed. If it is “a public artifact, inward”, what is needed is to
pin its digest and mirror it, and the `status` describes its governance, not a leak.

<a id="archivos-y-gate"></a>
## Files and gate

- `providers.tsv`: versioned inventory with controller, headquarters, region,
  purpose, primary evidence, owner, review date, expiry and applicable license
  or terms.
- `blocked-baseline.tsv`: exact inherited debt `host + file`. It is not an
  allowlist: every row remains a non-compliance that must disappear.
- `tools/eusupply`: extracts references from tracked CI, Docker, Compose, Helm,
  Terraform, script, Go module and Bun lock files. It fails on an unknown host,
  evidence older than 183 days, an expired exception, a new blocked reference
  or an obsolete baseline row.

Run locally:

```sh
make eu-supply-chain-check
```

The same gate is part of `make gates`, GitHub `security` during the
transition and the Sytue Git pilot. The baseline can only shrink; it must not be
expanded to make CI pass without a documented decision.

<a id="estado-de-la-migración-2026-08-02"></a>
## Migration status (2026-08-02)

Already approved or migrated:

- automation of a self-hosted Sytue Git (Gitea) on Scaleway France and pilot
  workflows prepared; the actual go-live is still pending applying the infrastructure;
- runtime, registry, S3 and managed observability on Scaleway;
- payments with Mollie B.V. (Netherlands);
- AI inference by default with Scaleway Generative APIs and Mistral; Anthropic has
  been withdrawn from the service;
- Traefik (French SAS), with its vendored chart, and a static GeoIP database from
  DB-IP (France).

**Withdrawn on 2026-10-06**: the forge is GitHub, the `services/ghcompat` gateway
left the repository (security audit, CLOUD-02) and with it the `claude.ai` row
of `blocked-baseline.tsv`. What follows is the record of the decision that was
in force until then.

Decided on 2026-08-15, **Claude Code on Sytue Git**: the gateway presented
the forge as a GitHub Enterprise Server so it could be connected to Claude Code,
which, with a self-hosted forge, only speaks GHES.

The starting topology was the **self-hosted environment** one —sessions on a
runner of ours in the EU, which clones from inside the network— precisely so as not
to open anything inbound. **That point was revised the same day, and it must be said
clearly**: the Claude Code connection cannot be established unless its
infrastructure reaches the instance, so the owner decided to open the
gateway port (8443) **restricted to Anthropic's outbound range**,
`160.79.104.0/21`, in the forge's security group.

What that means, without embellishment:

- **The translated API is reachable from United States infrastructure**, and
  that API serves file contents, not just metadata. It is a
  departure from the strict EU-only criterion, taken knowingly.
- **It is not general access**: the door is closed to everything else, and the scope
  —which repositories are visible— is set not by the network rule but by the bot
  user `claude-bot`, which only belongs to the organization's `claude` team.
- **Cloning remains internal** as long as the sessions run on the self-hosted
  runner: the one cloning is our machine.
- **Whatever each session reads goes to the model**, which is inherent to using
  Claude and not to this gateway.

With the port open, the hosted Code Review and the contribution metrics also come
in, which were previously left out. The way back, if it is to be narrowed,
is to close 8443 and move to the orchestrator's SCM connector; it was described in
the service's README, which no longer exists.

Temporary exceptions, both with a mandatory exit before 2026-10-01:

- GitHub exclusively as a migration bridge until two verified releases are
  completed on Sytue Git;
- Bun while the JavaScript BFF/SSR is retired according to ADR-0027.

Blockers that are not resolved just by changing the hosting region:

1. **Toolchains and governance:** Go/Google, Kubernetes/Helm/CNCF/Linux
   Foundation, npm/Microsoft and Node/OpenJS do not meet the EU headquarters
   criterion. They require an architecture ADR and code migration, not a mere mirror.
2. **Upstream artifacts:** ~~Docker Hub, GCR, GHCR, Quay, base images and the
   chart served from GitHub must be imported only once, have their signature and
   checksum verified and be promoted to Scaleway's Registry/Object Storage.~~ **DONE and
   verified on 2026-08-11**: the five registries move to `pinned-artifact`
   (mirror with an immutable digest and signature at runtime; `@sha256:` in the thirty
   build `FROM` lines), and the Traefik chart is vendored —`traefik.github.io`
   no longer appears in any file—. The second point still holds: **the
   mirror reduces operational exposure, it does not make the origin EU**; that is why
   the status is called “pinned artifact” and not “approved”.
3. **Public PKI:** Let's Encrypt belongs to a US entity.
   HARICA (Greece) offers ACME, but requires prior registration and validation of the
   organization; the issuer is not changed until that process is complete so as not
   to interrupt TLS.
4. **Self-hosted components that are non-EU or whose control cannot be verified:** Dragonfly,
   PostgreSQL/PgBouncer, MinIO, LiveKit, PostgREST, Paketo and transitive
   dependencies need replacement or an explicit decision. Self-hosting them
   avoids data transfers, but does not satisfy the origin rule.

<a id="orden-de-salida"></a>
## Exit order

1. finish Sytue Git and cut GitHub after two green releases;
2. close the artifact ingress with private mirrors and reproducible builds;
3. contract/validate HARICA and change the ClusterIssuers;
4. remove Bun/Node from the runtime and from the production build;
5. approve the ADR for replacing Go/Kubernetes and carry out the migration by
   bounded context;
6. replace the remaining components and bring `blocked-baseline.tsv` down to zero.

The chain cannot be declared "100 % EU" until the baseline is empty
and the inventory contains no temporary exceptions.
