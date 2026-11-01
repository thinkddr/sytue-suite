# Sytue Suite (Sytue One)

**Autor:** Daniel Delgado Ramos ([thinkddr](https://github.com/thinkddr))  
**Licencia:** [Apache-2.0](LICENSE) · **Repositorio público:** [github.com/thinkddr/sytue-suite](https://github.com/thinkddr/sytue-suite)

**Sytue Suite** es una suite colaborativa, de productividad, gestión empresarial (ERP) y plataforma cloud (PaaS) **100 % europea y soberana**, desarrollada desde cero (*clean-room* sobre estándares abiertos) bajo licencia **Apache-2.0**.

Reúne identidad (SSO/OIDC/MFA/Passkeys/SCIM), correo, almacenamiento en la nube, ofimática colaborativa (documentos, hojas de cálculo, presentaciones, bases de datos y edición/firma de PDF), galería de fotos, notas, calendario, contactos, mensajería y videollamadas en tiempo real, streaming de vídeo, creador de sitios web, ERP y plataforma PaaS bajo **una sola identidad, una sola estética y una sola experiencia**.

![Sytue One — portal de la suite](docs/capturas/web-one.webp)

---

## Capturas de pantalla

> Todas las capturas usan una organización de demostración con datos ficticios (dominios reservados `.test`/`.example`).

### Aplicaciones web

| Sytue Mail | Sytue Drive |
| --- | --- |
| ![Sytue Mail](docs/capturas/web-mail.webp) | ![Sytue Drive](docs/capturas/web-drive.webp) |
| **SyDocus — documentos** | **SyCalcus — hojas de cálculo** |
| ![SyDocus](docs/capturas/web-docs.webp) | ![SyCalcus](docs/capturas/web-sheets.webp) |
| **SyDiapos — presentaciones** | **SyCertiPDF — edición y firma de PDF** |
| ![SyDiapos](docs/capturas/web-slides.webp) | ![SyCertiPDF](docs/capturas/web-pdf.webp) |
| **Sytue Calendar** | **Sytue Contacts** |
| ![Sytue Calendar](docs/capturas/web-calendar.webp) | ![Sytue Contacts](docs/capturas/web-contacts.webp) |
| **Sytue Photos** | **Sytue ID — cuenta y seguridad** |
| ![Sytue Photos](docs/capturas/web-photos.webp) | ![Sytue ID](docs/capturas/web-id.webp) |
| **Sytue Web — creador de sitios** | **Sytue Hub — app nativa** |
| ![Sytue Web](docs/capturas/web-pages.webp) | ![Sytue Hub](docs/capturas/nativa-hub.png) |

### Aplicaciones nativas (Fyne · macOS, Windows, Linux)

| Sytue Rooms | SyERP |
| --- | --- |
| ![Sytue Rooms](docs/capturas/nativa-rooms.png) | ![SyERP](docs/capturas/nativa-erp.png) |
| **Sytue Cast** | **Sytue Bases** |
| ![Sytue Cast](docs/capturas/nativa-cast.png) | ![Sytue Bases](docs/capturas/nativa-bases.png) |
| **Sytue Notepads** | **Móvil (iOS / Android): Mail y Rooms** |
| ![Sytue Notepads](docs/capturas/nativa-notes.png) | <img src="docs/capturas/movil-mail.png" width="48%" alt="Sytue Mail móvil"> <img src="docs/capturas/movil-rooms.png" width="48%" alt="Sytue Rooms móvil"> |

---

## Despliegue guiado en 1 comando (`./sytue-deploy`)

Ningún archivo del repositorio contiene contraseñas ni secretos predefinidos. El asistente interactivo **`./sytue-deploy`** te guía paso a paso para elegir el modo de despliegue (**Docker Compose** o **Kubernetes con Helm**), seleccionar los servicios deseados, configurar la cuenta administradora inicial y **autogenerar criptográficamente todas las contraseñas y claves AES-256 / HMAC / OIDC**:

```bash
git clone https://github.com/thinkddr/sytue-suite.git
cd sytue-suite

# Lanza el asistente interactivo paso a paso:
./sytue-deploy
# (o bien: make deploy)
```

También puedes usarlo en modo no interactivo para automatizaciones:

```bash
# Generar deploy/compose/.env con todos los secretos autogenerados sin iniciar contenedores:
./sytue-deploy --mode compose --profile full --non-interactive --only-config

# Generar deploy/helm/values-custom.yaml y deploy/helm/apply-secrets.sh para Kubernetes:
./sytue-deploy --mode helm --domain suite.miempresa.eu --namespace sytue-prod --non-interactive --only-config
```

Consulta la guía rápida en [`QUICKSTART.md`](QUICKSTART.md) y la referencia completa de variables de entorno en [`docs/operations.md`](docs/operations.md#variables-de-entorno).

---

## Integraciones y conectores con terceros

El **Sytue Integration Gateway** (`services/integrations`) conecta la suite con medio centenar de servicios externos: Google Workspace, Microsoft 365, Slack, Zoom, GitHub, Jira, Salesforce, Dropbox, WhatsApp Business, buzones IMAP/POP3, proveedores SAML/OIDC y más. Todo el tráfico hacia terceros sale por él, con política por organización, y las credenciales de cada conexión se guardan **selladas con AES-256**: nunca en claro.

> **Requisito:** el gateway se despliega con **Kubernetes/Helm**. El perfil de Docker Compose todavía no lo incluye, así que en Compose los conectores no están disponibles. El único proveedor externo de ese perfil es Sytue AI, que el asistente configura en el paso 3.

### Activarlas paso a paso con el CLI

```bash
# Dentro del despliegue completo (paso 4 de 6 del asistente):
./sytue-deploy --mode helm

# O solo las integraciones, sobre un despliegue Helm ya existente (se puede repetir;
# también `make deploy-integrations`):
./sytue-deploy --integrations --domain suite.miempresa.eu
```

El asistente recorre tres niveles:

1. **Base del gateway (automática).** Genera la KEK que sella las credenciales, el secreto del cliente `sytue-gateway` en Sytue ID y el secreto interno que comparte con Drive. Sin esta base el pod de integraciones no arranca. **Ninguna clave existente se rota al volver a ejecutarlo**: rotar la KEK dejaría ilegibles las credenciales ya guardadas.
2. **Apps OAuth de plataforma (opcional, proveedor a proveedor).** Para cada proveedor te dice dónde registrar la app, qué opciones elegir y qué **URI de redirección** declarar, y después te pide el `client_id` y el `client_secret` (este sin mostrarlo en pantalla). Con una app de plataforma, cada organización conecta **con un clic**: solo aprueba la pantalla de permisos. Si no la registras, cada organización tendrá que registrar la suya (*BYO*).
3. **El resto de conectores** (SAML, OIDC, IMAP/POP3, tokens de API) no necesita nada en el despliegue: los da de alta cada organización desde Sytue Admin.

La URI de redirección es **la misma para todos los proveedores**:

```
https://integrations.<tu-dominio>/api/v1/oauth/callback
```

| Proveedor | Dónde se registra la app | Qué cubre |
| --- | --- | --- |
| Google | [Google Cloud Console → Credenciales](https://console.cloud.google.com/apis/credentials) · ID de cliente OAuth «Aplicación web» | Workspace, Drive, Gmail, Chat y Meet (una sola app) |
| Microsoft | [Microsoft Entra → Registros de aplicaciones](https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade) · multiinquilino, redirección «Web» | Microsoft 365, Azure, OneDrive, Teams y Outlook (una sola app) |
| Slack | [api.slack.com/apps](https://api.slack.com/apps) · desde manifiesto ([`integraciones-marketplace/slack`](integraciones-marketplace/slack)) | SSO/SCIM, canales y salas de Rooms |
| Zoom | [Zoom App Marketplace](https://marketplace.zoom.us/develop/create) · «General App» | Reuniones |
| GitHub | [New OAuth App](https://github.com/settings/applications/new) | Repositorios y migración |
| Jira Cloud | [Atlassian Developer Console](https://developer.atlassian.com/console/myapps/) · OAuth 2.0 (3LO) | Jira y SSO de Atlassian |
| Salesforce | Setup → App Manager → *Connected App* | CRM |
| ServiceNow | System OAuth → *Application Registry* | Incidencias y servicios |
| Dropbox | [Dropbox App Console](https://www.dropbox.com/developers/apps) | Almacenamiento conectado en Drive |
| Business Central | Registro de app en Entra con la API de Dynamics 365 Business Central | ERP |
| Databricks | Account console → *App connections* | Datos |
| Yahoo Mail | [Yahoo Developer](https://developer.yahoo.com/apps/) | Buzones y migración |
| WhatsApp Business | [Meta for Developers](https://developers.facebook.com/apps/) · app de tipo *Business* | Conversaciones en Rooms (webhook: `/api/v1/whatsapp/webhook`) |

### Qué genera y cómo se aplica

| Fichero (en `deploy/helm/`, fuera de Git) | Contenido |
| --- | --- |
| `integrations-oauth.tsv` · `integrations-oauth-clients.json` (0600) | Las apps OAuth registradas. El asistente las conserva al volver a ejecutarlo. |
| `apply-integrations.sh` (0700) | Crea o completa los Secrets `sytue-integrations`, `sytue-integrations-platform-oauth`, `sytue-id-client-secrets` y `sytue-drive` (solo genera lo que falta) y reinicia los Deployments afectados. |
| `values-integrations.yaml` | Activa `id.clientSecrets` para que Sytue ID dé de alta el cliente `sytue-gateway`. |

```bash
bash deploy/helm/apply-secrets.sh          # si aún no lo has aplicado
bash deploy/helm/apply-integrations.sh
helm upgrade --install sytue-one deploy/helm/sytue-one -n sytue-prod \
  -f deploy/helm/sytue-one/values.yaml \
  -f deploy/helm/values-custom.yaml \
  -f deploy/helm/values-integrations.yaml
```

### Conectar una organización

Con la suite en marcha, un administrador de la organización entra en **Sytue Admin → Aplicaciones** (`https://admin.<tu-dominio>/admin/apps`), pulsa **Conectar** en la aplicación que quiera y aprueba los permisos en el proveedor. A partir de ahí, cada persona usa la conexión desde su app (Drive, Mail, Rooms…) sin copiar tokens ni webhooks. Para publicar las apps en las tiendas de Slack, Teams, Google Workspace o Zoom, consulta [`integraciones-marketplace/`](integraciones-marketplace/README.md).

---

## Stack único y arquitectura

`Go + Astro + Preact + Fyne + PostgreSQL`. Las versiones exactas del stack están fijadas en [`docs/CONVENTIONS.md` §3](docs/CONVENTIONS.md#3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro).

| Capa | Tecnología |
| --- | --- |
| **Backend** | **Go** — módulo único `git.sytue.eu/sytue/one`, 16 microservicios en `services/<svc>/cmd/<svc>` y librerías comunes en `platform/pkg/*` |
| **Frontend Web** | **Astro + Preact** (*islands*), Bun, Tailwind v4, TypeScript estricto (`apps/frontend/`) |
| **Apps Nativas** | **Fyne (Go)** — 15 aplicaciones nativas para Linux, macOS, Windows, Android e iOS/iPadOS (`apps/fyne/`) |
| **Base de datos** | **PostgreSQL 18** — aislamiento multi-tenant con *Row-Level Security* (`ENABLE` + `FORCE RLS`), colas transaccionales y `LISTEN/NOTIFY` |
| **Tiempo real / Caché** | **Dragonfly** — presencia efímera y *fan-out* de eventos |
| **Almacenamiento S3** | **Object Storage S3** (MinIO en local / S3 en producción) para bloques y adjuntos |
| **Medios WebRTC** | **LiveKit** (Apache-2.0) — SFU para videollamadas en *Sytue Rooms* y directos en *Sytue Cast* |
| **Ingress / TLS** | **Traefik 3** + **cert-manager** (ACME / Let's Encrypt en prod, `mkcert` en local) |

---

## Catálogo de productos

| Producto | Descripción | Servicio Go | Puerto | Superficie Web |
| --- | --- | --- | --- | --- |
| **Sytue One** | Portal unificado y lanzador de la suite | `id` + `hub` | — | `one.<dominio>` |
| **Sytue ID** | Proveedor de identidad nuclear (OIDC/OAuth2/PKCE, MFA, Passkeys, SCIM 2.0, Acceso Condicional) | `services/id` | `8081` | `id.<dominio>` |
| **Sytue Drive** | Archivos en la nube, carpetas, enlaces compartidos y WebDAV | `services/drive` | `8082` | `drive.<dominio>` |
| **SyDocus / SyCalcus / SyDiapos / SyCertiPDF** | Procesador de textos, hojas de cálculo, presentaciones y editor/firmador eIDAS de PDF | `services/docs` | `8083` | `docus.` / `calcus.` / `diapos.` / `certipdf.<dominio>` |
| **Sytue Rooms** | Mensajería por canales, hilos, cifrado E2EE y videollamadas WebRTC | `services/rooms` | `8085` | `rooms.<dominio>` |
| **Sytue Mail** | Servidor y webmail JMAP, IMAP, SMTP, DKIM y filtros Sieve | `services/mail` | `8086` | `mail.<dominio>` |
| **Sytue Calendar** | Calendarios, eventos, invitaciones iMIP y CalDAV | `services/calendar` | `8087` | `calendar.<dominio>` |
| **Sytue Contacts** | Agenda de contactos y grupos con CardDAV | `services/contacts` | `8088` | `contacts.<dominio>` |
| **Sytue Notepads** | Notas rápidas y cuadernos sincronizados | `services/notes` | `8089` | `notes.<dominio>` |
| **Sytue Cloud** | Plataforma PaaS (despliegues desde Git, bases de datos PostgreSQL y PostgREST) | `services/cloud` | `8090` | `cloud.<dominio>` |
| **Sytue Hub** | Búsqueda global federada, notificaciones SSE, presencia e intents | `services/hub` | `8092` | `hub.<dominio>` |
| **Integration Gateway** | Conectores salientes BYO-SaaS con auditoría de egress y DLP | `services/integrations` | `8093` | `admin.<dominio>` |
| **Sytue Analytics** | Analítica web en primera parte sin cookies ni rastreadores | `services/analytics` | `8094` / `8095` | `cloud.<dominio>` |
| **Sytue Photos** | Galería fotográfica con álbumes y línea temporal | `services/photos` | `8097` | `photos.<dominio>` |
| **SyERP** | Facturación, contabilidad, inventario, TPV, RR. HH., proyectos y VeriFactu | `services/syerp` | `8098` | `erp.<dominio>` |
| **Sytue Cast** | Vídeo bajo demanda y emisiones en directo (HLS) | `services/cast` | `8099` | `cast.<dominio>` |
| **Sytue Web** | Creador visual de sitios web y tiendas online con exportación estática | `services/pages` | `8100` | `pages.<dominio>` |
| **Sytue Bases** | Bases de datos relacionales colaborativas (tabla, kanban, calendario, formulario) | `services/bases` | `8102` | `bases.<dominio>` |
| **Sytue Admin** | Consola de administración de organización, políticas e integraciones | `id` + `hub` | — | `admin.<dominio>` |

---

## Estructura del monorepo

```text
sytue-suite/
├── sytue-deploy              Asistente interactivo paso a paso de configuración y despliegue
├── README.md  QUICKSTART.md  Makefile  .env.example  CONTRIBUTING.md  LICENSE  NOTICE
├── go.mod  go.sum            Módulo único Go (git.sytue.eu/sytue/one)
├── platform/pkg/             Librerías Go compartidas (authn, httpx, pg, events, crypto, objectstore...)
├── services/                 16 microservicios Go independientes (cmd/<svc>/main.go + db/migrations)
├── apps/
│   ├── frontend/             Monorepo Bun (Astro + Preact): 22 apps web y 10 paquetes @sytue/*
│   ├── fyne/                 15 aplicaciones nativas de escritorio y móvil en Go + Fyne
│   └── mobile/               Clientes móviles complementarios
├── deploy/
│   ├── compose/              Entorno Docker Compose (docker-compose.yml, frontlite.yml, .env.example)
│   └── helm/                 Umbrella chart de Kubernetes (sytue-one) y subcharts por servicio
├── branding/                 Logos, iconos, ilustraciones y tokens de diseño (--sytue-*)
└── docs/                     Documentación canónica (CONVENTIONS, architecture, operations, security...)
```

---

## Comandos habituales (`Makefile`)

```bash
make deploy        # Asistente interactivo paso a paso (./sytue-deploy)
make dev-up        # Levanta el stack en Docker Compose (con certificados mkcert)
make dev-seed      # Crea la organización y el usuario administrador inicial en Sytue ID
make dev-app P=mail,drive,docs  # Levanta únicamente el núcleo + los perfiles indicados
make dev-down      # Detiene los contenedores conservando los volúmenes
make go-build      # Compila todos los servicios Go (go build ./...)
make go-test       # Ejecuta los tests unitarios de Go
make fe-install    # Sincroniza activos de marca e instala dependencias con Bun
make fe-build      # Compila los paquetes y aplicaciones del frontend
make helm-lint     # Valida los charts de Helm de Kubernetes
```

---

## Documentación

| Documento | Contenido |
| --- | --- |
| [`QUICKSTART.md`](QUICKSTART.md) | Guía rápida para configurar y levantar Sytue Suite en minutos |
| [`docs/README.md`](docs/README.md) | Índice maestro de la documentación |
| [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) | **Contrato canónico**: stack, puertos, dominios, arquitectura compartida y licencias |
| [`docs/operations.md`](docs/operations.md) | **Guía de despliegue y operación**: CLI `./sytue-deploy`, variables de entorno, Compose, Helm y backups |
| [`docs/architecture.md`](docs/architecture.md) | Arquitectura C4, SSO, Shell+Hub, Cloud, Integration Gateway, SyERP y clientes nativos |
| [`docs/security.md`](docs/security.md) | Seguridad y RGPD, modelo STRIDE, OWASP ASVS L2 y política *clean-room* |
| [`docs/decisions.md`](docs/decisions.md) | Registro de decisiones de arquitectura (ADR) |
| [`docs/design.md`](docs/design.md) | Dirección de arte y sistema de diseño (`@sytue/ui`) |
| [`docs/roadmap.md`](docs/roadmap.md) | Estado del proyecto y hoja de ruta por producto |

---

## Licencia y autoría

Distribuido bajo la licencia **Apache License 2.0**.  
Copyright © 2026 **Daniel Delgado Ramos ([thinkddr](https://github.com/thinkddr))**. Consulta [`LICENSE`](LICENSE) y [`NOTICE`](NOTICE) para más detalles.
