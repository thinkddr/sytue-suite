# Sytue Suite (Sytue One)

**Autor:** Daniel Delgado Ramos ([thinkddr](https://github.com/thinkddr))  
**Licencia:** [Apache-2.0](LICENSE) · **Repositorio público:** [github.com/thinkddr/sytue-suite](https://github.com/thinkddr/sytue-suite)

**Sytue Suite** es una suite colaborativa, de productividad, gestión empresarial (ERP) y plataforma cloud (PaaS) **100 % europea y soberana**, desarrollada desde cero (*clean-room* sobre estándares abiertos) bajo licencia **Apache-2.0**.

Reúne identidad (SSO/OIDC/MFA/Passkeys/SCIM), correo, almacenamiento en la nube, ofimática colaborativa (documentos, hojas de cálculo, presentaciones, bases de datos y edición/firma de PDF), galería de fotos, notas, calendario, contactos, mensajería y videollamadas en tiempo real, streaming de vídeo, creador de sitios web, ERP y plataforma PaaS bajo **una sola identidad, una sola estética y una sola experiencia**.

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
