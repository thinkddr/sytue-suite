<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

<a id="instalar-sytue-suite"></a>
# Install Sytue Suite

Three commands and a wizard that does the rest: it checks the requirements, generates all
the passwords and keys, builds the suite, starts it, creates your
administrator account and verifies that everything responds.

---

<a id="1-antes-de-empezar"></a>
## 1. Before you start

| You need | How to check | If you don't have it |
| --- | --- | --- |
| **Docker** with **4 GB of memory** (the full suite, once running, uses ~1.4 GB; the first build compiles in parallel and benefits from 8 GB) | `docker info` | [Docker Desktop](https://docs.docker.com/get-docker/) (macOS/Windows) or Docker Engine (Linux). In Docker Desktop: *Settings → Resources → Memory*. |
| **mkcert** (trusted local HTTPS) | `mkcert -version` | macOS: `brew install mkcert nss` · Debian/Ubuntu: `sudo apt install mkcert libnss3-tools` · Windows: `choco install mkcert` |
| **git**, **bash** and **openssl** | `git --version` | They come with macOS and Linux. On Windows, use WSL 2. |

About **30 GB of free disk space** (the first build reaches ~25 GB between images and cache) and ports **80** and **443** on your machine unused.

---

<a id="2-instalar"></a>
## 2. Install

```bash
git clone https://github.com/thinkddr/sytue-suite.git
cd sytue-suite
./sytue-deploy
```

Press **Enter** at each question to keep the recommended option. The wizard
walks through five steps (its prompts are printed in Spanish; the step and option names below are quoted exactly as shown):

1. **Dónde instalar** (where to install) → *En este equipo con Docker* (on this machine with Docker) (for a server with your domain, see [section 6](#6-instalar-en-un-servidor-con-tu-dominio)).
2. **Requisitos** (requirements) → checks Docker, its memory, mkcert and the ports. If something is missing, it
   tells you how to install it and stops without touching anything.
3. **Servicios** (services) → *Suite completa* (full suite), *Ofimática* (office; less memory), *Mínimo* (minimal) or *A medida* (custom).
4. **Configuración y secretos** (configuration and secrets) → choose the administrator password (Enter = a
   random one) and, if you want, a provider for Sytue AI. It generates `deploy/compose/.env` with
   keys unique to your machine (permissions 0600, ignored by Git).
5. **Construcción y arranque** (build and startup) → creates the local HTTPS certificate, builds the images
   (**the first time takes between 15 and 40 minutes**; afterwards it is almost immediate), starts the
   containers, creates your administrator in Sytue ID, waits for all services to be
   healthy and runs an end-to-end verification.

No questions? `./sytue-deploy --yes` installs the full suite with the recommended
values (`--profile office` for the lightweight version).

---

<a id="3-entrar"></a>
## 3. Sign in

When it finishes, the wizard shows your credentials. Open **https://one.sytue.localhost**:

| Field | Value |
| --- | --- |
| Email | `admin@sytue.localhost` |
| Password | the one you chose or the one the wizard generated (it is in `deploy/compose/.env`, variable `SYTUE_ID_DEV_SEED_PASSWORD`) |
| Verification code | the administration account asks for a second factor: add the key **`JBSWY3DPEHPK3PXP`** to your authenticator app (Google Authenticator, 1Password, Aegis…) |

On the sign-in screen, tick the **human verification** checkbox before pressing
*Continue* (it solves itself in a second). `member@sytue.localhost`, with the same
password and no second factor, is a regular user account (without access to Sytue Admin).

Each application lives on its own subdomain:

| Application | Address |
| --- | --- |
| Sytue One (portal) | https://one.sytue.localhost |
| Sytue Admin | https://admin.sytue.localhost |
| Mail · Drive · Calendar · Contacts | https://mail.sytue.localhost · https://drive.sytue.localhost · https://calendar.sytue.localhost · https://contacts.sytue.localhost |
| SyDocus · SyCalcus · SyDiapos · SyCertiPDF | https://docus.sytue.localhost · https://calcus.sytue.localhost · https://diapos.sytue.localhost · https://certipdf.sytue.localhost |
| Rooms · Notepads | https://rooms.sytue.localhost · https://notes.sytue.localhost |
| SyERP · Cast · Web · Bases | https://erp.sytue.localhost · https://cast.sytue.localhost · https://web.sytue.localhost · https://bases.sytue.localhost |
| Sytue Cloud | https://cloud.sytue.localhost |
| Public website | https://www.sytue.localhost |

`*.sytue.localhost` points to your own machine without any configuration: browsers
resolve `.localhost` to `127.0.0.1`.

> **End-to-end encryption (Matrix).** Several apps store your data encrypted on the
> device, with Matrix as the transport. The installer embeds the same Matrix server as
> production (Synapse, with Sytue ID as its only sign-in) in every mode, unless you pass
> `--sin-matrix`. The first time you open one of those apps it asks you to set up the
> device (“Set up this device”), to confirm in Matrix and to protect the key with your
> password or a passkey. Verified on 2026-11-17: Rooms and Notepads work end to end;
> Mail and Drive are still being finished in the apps themselves.

---

<a id="4-día-a-día"></a>
## 4. Day to day

| I want to… | Command |
| --- | --- |
| Stop the suite (keeps the data) | `make dev-down` |
| Start it again | `make dev-up` |
| See the status of each service | `make dev-ps` |
| See a service's logs | `make dev-logs SVC=id` |
| Update to the latest version | `git pull && ./sytue-deploy` (reuses your configuration and your data) |
| Load sample data | `make dev-demo` |
| Start from scratch (**deletes all data**) | `./sytue-deploy --reset` |

---

<a id="5-si-algo-falla"></a>
## 5. If something fails

- **«Docker está instalado pero no responde»** (Docker is installed but not responding) → open Docker Desktop (or `colima start`,
  or `sudo systemctl start docker`) and run `./sytue-deploy` again.
- **The browser warns about an invalid certificate** → run `mkcert -install` and
  restart the browser.
- **A service keeps restarting** → `make dev-ps` to see which one and
  `make dev-logs SVC=<servicio>` to see why. If it is due to memory, increase Docker's memory or
  install the lightweight version: `./sytue-deploy --profile office`.
- **“password authentication failed” after deleting `deploy/compose/.env`** → the
  database passwords are set the first time; without that `.env`, start from
  scratch with `./sytue-deploy --reset`.
- **Check that everything responds** → `scripts/e2e/smoke.sh` (the wizard runs it at the
  end and saves the result in `deploy/compose/.verificacion.log`).

---

<a id="6-instalar-en-un-servidor-con-tu-dominio"></a>
## 6. Install on a server with your domain

The whole suite on **a single Linux machine with Docker**, reachable as
`https://<app>.tu-dominio` and with **Let's Encrypt** certificates that renew themselves.

**Beforehand:** a wildcard DNS record `*.suite.tuempresa.eu` (and `suite.tuempresa.eu`)
pointing to the server's IP, and ports `80` and `443/TCP` open (web and
certificates), plus `7881/TCP` and `50000-50100/UDP` (video calls). mkcert is not needed.

```bash
./sytue-deploy --mode servidor --domain suite.tuempresa.eu --acme-email tu@tuempresa.eu
```

The wizard also asks you:

- **Who uses it** (`--usuarios`):
  - `uno`: only your account, no public sign-up. Ideal for personal use.
  - `varios` (default): a team. You invite each person from
    `https://admin.suite.tuempresa.eu`; with `--registro-abierto`, in addition, anyone can
    create an account at `https://id.suite.tuempresa.eu/signup` (`--registro-cerrado`
    closes it again).
- **Owner** (`--admin-email`) and **organization** (`--org-slug`). The owner's password
  is generated and stored in `deploy/compose/.env` (`SYTUE_PROPIETARIO_PASSWORD`).
  When you sign in for the first time, enable two-step verification at
  `https://id.suite.tuempresa.eu/account`.

For a **local network or testing** without Internet, use `--tls interno` (mkcert
certificates: every machine that connects must trust its CA, `mkcert -CAROOT`).

Running the same command again is safe: it reuses the configuration, builds only what
changed and brings back up whatever was stopped. This is also how you change the
user mode (`--usuarios uno|varios`) without touching the data. `--reset` starts from scratch
**deleting the data**.

> SMTP and IMAP for external mail clients are not published in this mode: webmail
> works fully. To publish a mail server, use the cluster (section 8).

---

<a id="7-todo-en-un-solo-contenedor"></a>
## 7. Everything in a single container

The same server mode, packaged in **one container and one volume**: inside it runs its
own Docker (Docker in Docker, which is why it needs `--privileged`) with the whole suite.

```bash
docker build -f deploy/todo-en-uno/Dockerfile -t sytue-suite .
docker run -d --name sytue --privileged --restart unless-stopped \
  -p 80:80 -p 443:443 -p 7881:7881 -p 50000-50100:50000-50100/udp \
  -v sytue-datos:/datos \
  -e SYTUE_DOMINIO=suite.tuempresa.eu -e SYTUE_ACME_EMAIL=tu@tuempresa.eu \
  -e SYTUE_USUARIOS=varios \
  sytue-suite
docker logs -f sytue          # la primera vez construye la suite dentro (20-60 min)
```

Without `SYTUE_DOMINIO`, it stays on `https://one.suite.localhost` so you can try it on your own
machine (internal certificates: `docker cp sytue:/datos/ca/rootCA.pem .` and import it
as a trusted authority). All the variables are in the header of
[`deploy/todo-en-uno/Dockerfile`](deploy/todo-en-uno/Dockerfile). Everything that must be
kept lives in the `sytue-datos` volume: deleting the container does not delete the suite.

---

<a id="8-instalar-en-un-clúster-kubernetes-avanzado"></a>
## 8. Install on a Kubernetes cluster (advanced)

For production with your own domain. You need `kubectl`, `helm`, an image
registry, PostgreSQL 16+ and S3 storage.

```bash
./sytue-deploy --mode helm
```

The wizard asks for the domain, registry, PostgreSQL, S3 and the initial owner, and generates in
`deploy/helm/` (outside Git) everything needed. At the end it prints the seven commands to
run in order: push the images to your registry, create the databases,
apply the Secrets, install the chart and create the owner. Details and variable
reference: [`docs/operations.md`](docs/operations.md#13-prod-en-scaleway-terraform--helm).

---

<a id="9-sytue-office"></a>
## 9. Sytue Office: the apps on your computer, no server

Sytue Office is the local edition of the native apps: Docus, Calcus, Diapos, CertiPDF, Drive,
Mail, Calendar, Contacts, Notepads, Photos, Bases, Rooms, Cast, ERP and the Hub. No account, no
server, no Docker: everything lives in `~/Sytue Office`, in the clear, and it only goes online
for what you set up (a mail or calendar account, a cloud drive, an AI provider).

**On Linux or macOS**, build and install it from this repository:

```bash
./sytue-deploy --mode sytue-office
```

You need **Go 1.21 or newer** (it downloads the exact version the apps ask for by itself),
`git` and `python3`. On **Linux**, also a C compiler, `pkg-config`, `xz` and the desktop headers:
if any are missing, the wizard prints the `apt-get` or `dnf` command that installs them. Cast,
the Hub, Photos and Rooms also need the PipeWire 0.3.65+ headers (Debian 12, Ubuntu 23.04 or
newer); without them the wizard installs the other apps and says so. On **macOS**, the Xcode
command line tools (`xcode-select --install`).

- `--office-apps docus,calcus,mail` installs only those apps; `--yes` skips the questions.
- On Linux the apps go to `~/.local` and show up in the applications menu as «Sytue … Office»
  (`PREFIJO=/another/dir` puts them elsewhere). On macOS, to `~/Applications`.
- The packages it builds stay in `apps/fyne/dist/`. To remove the apps on Linux, unpack that
  `.tar.xz` and run its `instalar.sh --quitar`.

**On Windows**, or without building: every release of this repository attaches the Sytue Office
packages — `sytue-office-linux-<arch>.tar.xz` (with its `instalar.sh`), the Windows installer, the
macOS `.dmg` and, per app, an Android AAB and universal APK — with a `SHA256SUMS` file. They are
**unsigned** (no Authenticode, Developer ID or Play upload key here): check each file against
`SHA256SUMS`. They are built by [`.github/workflows/office.yml`](.github/workflows/office.yml) when a
release is published.

