# Decisiones de arquitectura (ADR) — Sytue One

Registro único de las decisiones arquitectónicas significativas: **qué** se decidió,
**por qué**, qué se descartó y qué consecuencias tiene. Sustituye al antiguo directorio
`docs/adr/` (un fichero por decisión).

> El **porqué** vive aquí. El **qué** normativo (nombres, dominios, puertos, versiones,
> licencias) vive en [`CONVENTIONS.md`](CONVENTIONS.md) y prevalece si divergen.

**Reglas:** numeración secuencial e inmutable; una decisión por entrada; una decisión
aceptada no se edita para cambiarla — se añade otra que la supersede. Decisiones triviales
o reversibles sin coste no necesitan entrada.

<a id="indice"></a>

## Índice

| #    | Decisión                                                                                                                                                     | Estado                                                                               |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------ |
| 0001 | [Soberanía 100 % UE sobre Scaleway](#0001)                                                                                                                   | accepted                                                                             |
| 0002 | [Keycloak como IdP nuclear](#0002)                                                                                                                           | superseded → 0020                                                                    |
| 0003 | [Nextcloud + OnlyOffice para archivos y ofimática](#0003)                                                                                                    | superseded → 0017                                                                    |
| 0004 | [Astro + Preact como stack de frontend](#0004)                                                                                                               | accepted                                                                             |
| 0005 | [Mattermost para chat y vídeo](#0005)                                                                                                                        | superseded → 0017, 0021                                                              |
| 0006 | [Stalwart como servidor de correo](#0006)                                                                                                                    | superseded → 0017                                                                    |
| 0007 | [Sytue Shell + Sytue Hub](#0007)                                                                                                                             | accepted (reconciliado)                                                              |
| 0008 | [Valkey en lugar de Redis](#0008)                                                                                                                            | superseded → 0019                                                                    |
| 0009 | [Todo FOSS libre y estrategia de fork](#0009)                                                                                                                | superseded → 0017, 0018                                                              |
| 0010 | [Clientes propios sobre la UI upstream](#0010)                                                                                                               | superseded → 0017                                                                    |
| 0011 | [Modelo PaaS y multi-tenancy](#0011)                                                                                                                         | accepted                                                                             |
| 0012 | [Servicios gestionados de Sytue Cloud](#0012)                                                                                                                | accepted (reconciliado)                                                              |
| 0013 | [Pipeline de build con Kaniko](#0013)                                                                                                                        | accepted (⚠ revisión pendiente)                                                      |
| 0014 | [IaC dual: Compose local + Helm/Terraform](#0014)                                                                                                            | accepted                                                                             |
| 0015 | [Licenciamiento AGPL-3.0 del código propio](#0015)                                                                                                           | superseded → 0018                                                                    |
| 0016 | [Stack único: Go + Astro/Preact + PostgreSQL](#0016)                                                                                                         | accepted                                                                             |
| 0017 | [Reimplementación nativa clean-room](#0017)                                                                                                                  | accepted                                                                             |
| 0018 | [Licenciamiento Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr))](#0018)                                                                                                                | accepted                                                                             |
| 0019 | [PostgreSQL como datastore único](#0019)                                                                                                                     | accepted                                                                             |
| 0020 | [Sytue ID como IdP nativo en Go](#0020)                                                                                                                      | accepted                                                                             |
| 0021 | [LiveKit como SFU de medios de Meet](#0021)                                                                                                                  | accepted                                                                             |
| 0023 | [Eliminación de los motores AGPL de terceros](#0023)                                                                                                         | accepted                                                                             |
| 0023 | [Sustrato de Sytue Cloud: Kapsule](#0023)                                                                                                                    | accepted                                                                             |
| 0024 | [Apps de escritorio (Fyne) y móvil (PWA + Capacitor)](#0024)                                                                                                 | accepted                                                                             |
| 0025 | [Sytue Rooms: mensajería y videollamada, un servicio y una base](#0025)                                                                                      | accepted                                                                             |
| 0026 | [Sytue Analytics: anonimato por construcción, medición en primera parte](#0026)                                                                              | accepted                                                                             |
| 0027 | [Excepción temporal Bun/Kaniko para v0.1.6 interna](#0027)                                                                                                   | accepted · expira 2026-10-01                                                         |
| 0028 | [Benchmarks anónimos entre clientes de Sytue Cloud](#0028)                                                                                                   | accepted                                                                             |
| 0029 | [Apps open-source como servicio: catálogo cerrado, no imagen libre](#0029)                                                                                   | accepted                                                                             |
| 0030 | [Firma digital: qué se promete bajo eIDAS y de dónde sale la clave](#0030)                                                                                   | accepted                                                                             |
| 0031 | [Crossplane adopta la infraestructura por riesgo; RDB y Kapsule se quedan fuera](#0031)                                                                      | accepted                                                                             |
| 0032 | [Productos open-source en Sytue Cloud: copyleft sí, «source available» no; y quién se publica](#0032)                                                        | accepted                                                                             |
| 0033 | [Sytue PDF se dibuja en SVG, no se rasteriza](#0033)                                                                                                         | accepted (ampliado)                                                                  |
| 0034 | [OCR: Tesseract autoalojado en un servicio propio; la capa de texto la escribe Sytue PDF](#0034)                                                             | accepted · **el motor de OCR lo revisa el [0035](#0035)**                            |
| 0035 | [Sytue AI: dos modelos en Scaleway, tope blando por usuario y OCR por modelo de visión](#0035)                                                               | accepted                                                                             |
| 0036 | [Cloudflare fuera: el portal entra por Pangolin y el DNS lo sirve Scaleway](#0036)                                                                           | accepted                                                                             |
| 0037 | [La plomería compartida se comparte de verdad](#0037)                                                                                                        | accepted                                                                             |
| 0038 | [Cuentas personales: registro gratuito con 5 GB y plan de 1 TB](#0038)                                                                                       | accepted · **el alta la revisa el [0040](#0040)**                                    |
| 0040 | [@usuario global y conversaciones con personas de otras organizaciones](#0040)                                                                               | accepted · **el alta la revisa el [0041](#0041)**                                    |
| 0041 | [Móvil verificado por SMS en el alta y cuatro identificadores de acceso](#0041)                                                                              | accepted                                                                             |
| 0042 | [España sin gestoría: el banco entra por Norma 43 y presentar por ti espera al certificado y a VeriFactu](#0042)                                             | accepted · **el PDF de la factura y el aviso por correo los añade el [0043](#0043)** |
| 0043 | [El ERP es de toda la organización: permisos por módulo, tareas y documentos, y Sytue AI que crea con confirmación](#0043)                                   | accepted                                                                             |
| 0044 | [VERI*FACTU dentro del ERP: la base garantiza la cadena y el envío sale con un certificado del servicio](#0044)                                              | accepted                                                                             |
| 0045 | [Una sola interfaz en Gio para web, escritorio y móvil: sytue.com en Gio, sytue.eu en la actual](#0045)                                                      | superseded → 0048 (escritorio y móvil) y → 0049 (sytue.com)                          |
| 0046 | [Los medios de Rooms no se escriben en Go: el transporte sí, los códecs y la captura no](#0046)                                                              | accepted · acota el [0045](#0045) · enmendado el 2026-09-25 (voz y vídeo nativos)    |
| 0047 | [Los avisos de Android salen del SSE del Hub, y el empaquetador va parcheado](#0047)                                                                         | accepted · implementa el [0039](#0039)                                               |
| 0048 | [La interfaz nativa se rehace en Fyne con widgets propios, y lo que no es del toolkit sale a `apps/nucleo`](#0048)                                           | accepted · supersede al [0045](#0045) en escritorio y móvil                          |
| 0049 | [`sytue.com` sirve la suite Astro con un segundo juego de imágenes, no la de Gio](#0049)                                                                     | accepted · supersede al [0045](#0045) en la web                                      |
| 0050 | [Un solo inicio de sesión para las trece apps nativas: llavero para el secreto, carpeta compartida para las preferencias, servidor para la presencia](#0050) | accepted · implementa el [0048](#0048)                                               |
| 0051 | [Firma con el certificado del sistema, en dos pasos, con sello cualificado de Izenpe, y «Pedir firma»](#0051)                                                | accepted · desarrolla el [0030](#0030)                                               |
| 0052 | [Canjear un enlace de Docs, también desde otra organización](#0052)                                                                                          | accepted · aplica el [0040](#0040) a los documentos                                  |
| 0053 | [Un aviso es de la persona: la campana enseña los de todas sus organizaciones](#0053)                                                                        | accepted · completa el [0040](#0040) y el [0052](#0052)                              |
| 0054 | [«Borrar mi cuenta»: la persona se da de baja sola, con 30 días para deshacerlo, y se borra lo suyo y no lo de su organización](#0054)                       | accepted                                                                             |
| 0055 | [La URL canónica de cada producto es su host de producto; la organización va al PATH, no al subdominio](#0055)                                               | accepted                                                                             |
| 0056 | [La organización como segmento de PATH dentro de cada producto](#0056)                                                                                       | accepted                                                                             |
| 0057 | [Pedir firma a alguien de otra organización: el PDF sale por un enlace de Drive](#0057)                                                                      | accepted · completa el [0051](#0051) §7 · aplica el [0040](#0040)                    |
| 0058 | [El CI corre en GitHub Actions; los workflows se portan sin cambiar lo que comprueban](#0058)                                                                | accepted · puntos 1 y 5 sustituidos por el [0060](#0060)                             |
| 0059 | [Peticiones de firma de fuera: quien recibe bloquea y su organización decide, sin que quien pide se entere](#0059)                                           | accepted · completa el [0057](#0057)                                                 |
| 0060 | [Runners propios en el frontal, imágenes en ghcr.io y forks en GitHub](#0060)                                                                                | accepted · sustituye los puntos 1 y 5 del [0058](#0058)                              |
| 0061 | [Sytue Cast: vídeo bajo demanda y directo, con portada pública y cuota de Drive](#0061)                                                                      | accepted                                                                             |
| 0066 | [El CI construye con un BuildKit sin root y propio, no con el builder clásico por el proxy de Docker](#0066)                                                 | accepted · completa el [0060](#0060)                                                 |
| 0077 | [Rooms cifra en los dispositivos mediante Matrix; los canales externos conservan la protección real de su proveedor](#0077)                                  | accepted · excepción expresa a 0016, 0018 y retirada de motores AGPL                 |
| 0078 | [Widgets nativos: contadores con caducidad en un App Group, Apple tras un interruptor y nada en Windows/Linux](#0078)                                        | accepted                                                                             |

<a id="el-pivote-sytue-native-2026-07-01"></a>

### El pivote «Sytue Native» (2026-07-01)

Las decisiones **0016–0022** formalizan el giro a un **stack único** (Go + Astro/Preact +
PostgreSQL), **productos propietarios reimplementados clean-room** y la **eliminación de
todos los motores AGPL de terceros** (Keycloak, Nextcloud, OnlyOffice, Mattermost,
Stalwart) y de Redis/Valkey. En consecuencia 0002, 0003, 0005, 0006, 0008, 0009, 0010 y
0015 quedan superseded — se conservan resumidas como memoria histórica del _porqué_.

- **0016** es la decisión madre: condiciona 0017, 0019, 0020, 0021, 0022.
- **0022** retira los motores; habilita **0017** (cómo se reimplementa) y **0018** (licencia).
- **0021** (LiveKit) es la única excepción a «sin apps de terceros en el runtime de cliente»,
  acotada al plano de medios.
- **0001** (soberanía UE) condiciona toda elección de proveedor y región.
- **0007** y **0012** siguen vigentes con nota de reconciliación: la decisión de producto se
  mantiene, su implementación se realinea al stack del pivote.

---

<a id="0001"></a>

## 0001 — Soberanía 100 % UE sobre Scaleway

`accepted` · 2026-06-30 · infra, gdpr, sovereignty

**Contexto.** La propuesta de valor frente a Google/Microsoft es la soberanía digital
verificable: ningún dato personal sale de la UE ni queda sujeto a acceso extraterritorial
(_US CLOUD Act_). Hace falta un proveedor UE con Kubernetes gestionado, Postgres, S3,
registry, DNS, LB, secretos y un provider Terraform maduro.

**Decisión.** **Scaleway** como proveedor único de producción, solo regiones UE: principal
`fr-par` (zonas 1/2/3), DR en `nl-ams`. Cómputo en **Kapsule**, datos en **RDB
(PostgreSQL)**, blobs en **Object Storage S3** versionado, imágenes en **Container
Registry**, secretos en **Secret Manager**, materializados directamente como `Secret`
de Kubernetes por el bootstrap de producción (nunca en git). Ningún
proveedor estadounidense para datos personales; ninguna transferencia fuera de la UE.

**Descartado.** Hyperscalers US (CLOUD Act, incompatible con el principio nuclear);
OVHcloud (válido y soberano, peor encaje de catálogo/DX — queda como ruta de portabilidad);
Hetzner (sin K8s gestionado ni catálogo PaaS); multi-cloud desde el día uno (complejidad
que diluye el time-to-market).

**Consecuencias.** Cumplimiento RGPD por diseño y mensaje comercial diferenciador; a cambio,
**lock-in** de proveedor mitigado usando primitivas portables (K8s, S3, PostgreSQL, OIDC),
catálogo menor que un hyperscaler (algunas piezas se construyen en casa) y DR limitada a
`nl-ams`. Toda decisión posterior de infraestructura queda acotada a «¿existe y es soberano
en Scaleway UE?».

---

<a id="0002"></a>

## 0002 — Keycloak como IdP nuclear · superseded por [0020](#0020)

`superseded` · 2026-06-30 · identity, sso

**Decisión (histórica).** **Keycloak 26** como IdP nuclear (marca Sytue ID, realm único
`sytue`), con clients OIDC por superficie, clients de federación por motor
(nextcloud/mattermost/onlyoffice/stalwart), aprovisionamiento SCIM/eventos vía Sytue Hub,
grupos `org/<tenant>` con roles `owner`/`admin`/`member`/`billing`, MFA/WebAuthn obligatorio
para admin y owner, y cookies host-only `__Host-` con SSO OAuth mediante Sytue ID.

**Descartado entonces.** Ory (Hydra/Kratos) por coste de ensamblaje; Authentik/Zitadel por
madurez; SaaS US (Auth0/Okta/Entra) por soberanía; IdP propio por riesgo — criterio que el
pivote acabó revirtiendo con librerías OIDC probadas.

**Por qué cayó.** El pivote elimina los motores federados y unifica el backend en Go: un
motor Java de terceros como pieza nuclear contradice el stack único. Ver [0020](#0020).

---

<a id="0003"></a>

## 0003 — Nextcloud + OnlyOffice · superseded por [0017](#0017)

`superseded` · 2026-06-30 · drive, docs

**Decisión (histórica).** **Nextcloud 30** como motor de Drive (WebDAV/OCS) y backend de
Calendar (CalDAV) y Contacts (CardDAV); **OnlyOffice Document Server 8.2** como motor de
ofimática, embebido en un cliente propio. Sin exponer sus UIs nativas.

**Descartado entonces.** Seafile (CalDAV/CardDAV limitados), ownCloud/OCIS (madurez),
servidores DAV dedicados (más piezas), Collabora (menor fidelidad OOXML), CryptPad
(modelo de documento distinto).

**Por qué cayó.** Motores AGPL de terceros; se reimplementan como servicios Go propios.

---

<a id="0004"></a>

## 0004 — Astro + Preact como stack de frontend

`accepted` · 2026-06-30 · frontend, design-system

**Contexto.** Todas las superficies deben verse y comportarse como **una sola aplicación**,
mezclando páginas de marketing y apps interactivas ricas, con JS mínimo por defecto y un
patrón BFF para OIDC+PKCE.

**Decisión.** **Astro + Preact** (islands) obligatorio en todas las superficies web, con
**Tailwind** y los tokens de `@sytue/ui`. Monorepo en `apps/frontend/` con paquetes
compartidos `@sytue/{ui,shell,auth,api-clients,config}` y una app por producto. Los
**endpoints Astro actúan de BFF**: tras OIDC+PKCE emiten una cookie httpOnly de dominio
`.sytue.eu` que permite saltar entre subdominios sin re-login; **ningún secreto en el
cliente**. Modo claro/oscuro y **WCAG AA** obligatorios.

**Descartado.** Next.js/React (más JS por defecto), SvelteKit (ecosistema, design system
duplicado), SPA pura sin SSR (SEO/rendimiento inicial), Preact solo (reinventar routing/SSR).

**Consecuencias.** Rendimiento y accesibilidad excelentes por defecto; un solo stack y un
solo design system. Riesgo de divergencia visual si una app crea componentes ad-hoc:
prohibido — los componentes canónicos viven **una sola vez** en `@sytue/ui`. Con el pivote,
las apps se repuntan a los servicios Go; el framework no cambia.

---

<a id="0005"></a>

## 0005 — Mattermost para chat y vídeo · superseded por [0017](#0017), [0021](#0021)

`superseded` · 2026-06-30 · chat, meet

**Decisión (histórica).** **Mattermost Team Edition** como motor de Chat y **Mattermost
Calls + rtcd** para Meet, con la webapp tematizada/forkeada para montar la Sytue Shell.
Excepción deliberada a «cliente propio desde cero»: su UI _era_ la referencia estética.

**Descartado entonces.** Matrix/Element (alineación estética y operación de Synapse),
Rocket.Chat (licencias de add-ons), Zulip (UX distinta), Jitsi (segundo producto separado).

**Por qué cayó.** Se retira el motor: Chat pasa a servicio Go (WebSocket) y Meet combina
señalización Go + LiveKit como SFU.

---

<a id="0006"></a>

## 0006 — Stalwart como servidor de correo · superseded por [0017](#0017)

`superseded` · 2026-06-30 · mail

**Decisión (histórica).** **Stalwart** como servidor único SMTP/IMAP/**JMAP** y un cliente
Astro+Preact propio tipo Gmail sobre JMAP, con SPF/DKIM/DMARC/MTA-STS/TLS-RPT y almacén
cifrado.

**Descartado entonces.** Postfix+Dovecot+puente JMAP (superficie operativa), Cyrus
(complejidad), Mailu/Mailcow (sin JMAP nativo), webmails existentes (imposible alinear UX).

**Por qué cayó.** Motor de terceros: Mail se reimplementa en Go (`emersion/*` MIT + JMAP
propio). Es el reto de mayor magnitud del pivote.

---

<a id="0007"></a>

## 0007 — Sytue Shell + Sytue Hub para integración seamless

`accepted` (reconciliado por el pivote) · 2026-06-30 · integration, shell, hub

**Contexto.** Integración seamless estilo Google Workspace: login único, shell común,
conmutador de apps, búsqueda y notificaciones globales, presencia y acciones cruzadas, sin
que cada app reinvente la topbar ni la lógica cross-app.

**Decisión.** Dos piezas complementarias:

1. **Sytue Shell** (`@sytue/shell`) — topbar unificada, app switcher, búsqueda global,
   centro de notificaciones, presencia y menú de cuenta. Se monta en **cada** app.
2. **Sytue Hub** — servicio de integración con búsqueda federada, notificaciones unificadas
   (SSE/WebSocket), presencia e **intents cross-app** (adjuntar de Drive, crear evento desde
   un correo, compartir en Chat).

Ninguna app habla directamente con los backends para funciones transversales.

**Descartado.** Que cada app implemente lo suyo (divergencia), Shell sin Hub (acoplamiento y
secretos dispersos), monolito único (rompe despliegue independiente).

**Reconciliación (pivote).** El Hub es un **servicio Go** (no TypeScript/Fastify), federa
los servicios Go propios, **desaparece el puente SCIM hacia motores**, autoriza con JWT de
Sytue ID y usa **PostgreSQL** —sin Valkey— para caché/eventos.

**Consecuencias.** El Hub es servicio crítico y posible cuello de botella: HA, diseño
asíncrono y degradación elegante de la búsqueda federada.

---

<a id="0008"></a>

## 0008 — Valkey en lugar de Redis · superseded por [0019](#0019)

`superseded` · 2026-06-30 · cache, licensing

**Decisión (histórica).** **Valkey** (fork BSD de Redis, Linux Foundation) como caché y cola,
tras el cambio de Redis a RSALv2/SSPL — no libre y por tanto incompatible con el principio
FOSS de entonces. Descartados KeyDB (impulso), DragonflyDB (BSL) y brokers no-Redis
(reescribir integraciones).

**Por qué cayó.** Desaparecen los consumidores que exigían la API de Redis; el datastore
único es Postgres.

---

<a id="0009"></a>

## 0009 — Todo FOSS libre y estrategia de fork · superseded por [0017](#0017), [0018](#0018)

`superseded` · 2026-06-30 · licensing, strategy

**Decisión (histórica).** Todo el stack FOSS libre (Apache-2.0/MIT/AGPL-3.0/BSD) con una
**estrategia de personalización graduada**: (1) tematización, (2) cliente propio sobre
API/protocolo libre, (3) fork del frontend cuando la integración lo exigía — manteniendo el
backend upstream. Prohibidas SSPL/BSL/source-available.

**Por qué cayó.** Sin motores que forkear y con obra original propia, la política pasa a
producto propietario con dependencias solo permisivas.

---

<a id="0010"></a>

## 0010 — Clientes propios sobre la UI upstream · superseded por [0017](#0017)

`superseded` · 2026-06-30 · frontend, ux

**Decisión (histórica).** Para Drive, Mail, Calendar, Contacts y Docs, **clientes propios
Astro+Preact desde cero** consumiendo solo los protocolos libres del motor (WebDAV/OCS,
JMAP, CalDAV, CardDAV, editor embebido), con `@sytue/shell` + `@sytue/ui`. Excepción:
Chat/Meet reutilizaba la webapp de Mattermost.

**Descartado entonces.** Tematizar la UI nativa (techo de personalización bajo), forkear
cinco frontends (mantenimiento), iframes (frágil y sin cross-app), monolito único.

**Por qué cayó.** Ya no hay «UI upstream»: los clientes consumen servicios Go propios. El
stack de frontend ([0004](#0004)) sigue vigente.

---

<a id="0011"></a>

## 0011 — Modelo PaaS y multi-tenancy

`accepted` · 2026-06-30 · paas, multi-tenant, k8s

**Decisión.** Modelo jerárquico **Organización (tenant) → Workspace → Usuarios**, con mapeo
1:1 a través de toda la pila: `tenant <slug>` ⇄ grupo de identidad `org/<slug>` ⇄ **namespace
Kapsule `tenant-<slug>`** (con `ResourceQuota` + `NetworkPolicy`) ⇄ almacenamiento del
tenant (buckets/prefijos S3) ⇄ superficies de producto del tenant.

- Roles `owner`, `admin`, `member`, `billing` dentro del grupo de la organización.
- Aislamiento de cómputo por namespace, con labels normativas
  (`app.kubernetes.io/part-of=sytue-one`, `sytue.eu/env`, `sytue.eu/tenant`).
- PaaS: `git push → webhook → build rootless → registry → deploy → URL con TLS`. Apps en
  `<app>.apps.sytue.eu`, previews en `<proyecto>-pr-<n>.preview.sytue.eu`, dominios propios
  por CNAME a `ingress.sytue.eu`.
- Cuotas, métricas y facturación **por tenant**.

**Descartado.** Multi-tenancy solo por `tenant_id` en tablas compartidas (aislamiento débil),
un clúster por tenant (coste), namespaces sin NetworkPolicy/quota (vecino ruidoso), un
modelo de tenancy distinto por producto (rompe «un alta = acceso a todo»).

**Consecuencias.** Aislamiento real apto para RGPD y facturación por consumo; a cambio,
reconciliación idempotente del mapeo, overhead por tenant (automatizar el onboarding) y
presión sobre el plano de control al crecer los namespaces (ver la célula/sharding de
[0023](#0023)).

---

<a id="0012"></a>

## 0012 — Servicios gestionados de Sytue Cloud (Vercel × Supabase)

`accepted` (reconciliado por el pivote) · 2026-06-30 · paas, dx

**Decisión.** Sytue Cloud combina un **flujo de deploy estilo Vercel** (conectar repo →
build → preview por PR → promoción a prod → rollback, con dominios y TLS automáticos) con un
**catálogo de servicios gestionados estilo Supabase**, todos soberanos y con paridad
dev/prod:

- **Sytue Postgres** — PostgreSQL gestionado con **API REST automática (PostgREST)**,
  pooling (PgBouncer), backups y branching de datos.
- **Sytue DB Studio** — explorador de tablas, editor SQL y generador de API/credenciales,
  con diseño Sytue, embebido en el dashboard.
- **Sytue Storage** (S3 por proyecto), **Cache/Queue**, **Functions**, **Cron**,
  **Secrets/Env** y **Auth gestionada** opcional para apps de cliente.
- **Conexión cero-fricción:** crear un servicio inyecta automáticamente sus credenciales en
  los deployments del proyecto. Observabilidad en lenguaje claro, sin jerga cloud.

**Descartado.** Usar Vercel/Supabase/GCP reales (US, propietarios); adoptar una PaaS FOSS
existente como producto (ninguna integra el catálogo con nuestro tenancy/IdP — se reutilizan
ideas y componentes); exponer Kubernetes crudo (contradice «más simple que Google Cloud»);
Hasura en vez de PostgREST (más pesado para la simplicidad buscada).

**Reconciliación (pivote).** El control plane es **Go** (`services/cloud/`); Cache/Queue es
**PostgreSQL**, no Valkey; la Auth gestionada se respalda en **Sytue ID**; el código es
propietario. PostgREST/PgBouncer se mantienen.

---

<a id="0013"></a>

## 0013 — Pipeline de build con Kaniko

`accepted` · 2026-06-30 · paas, ci, build · ⚠ **revisión pendiente**

**Contexto.** Construir imágenes dentro del clúster con Docker `build` exige demonio y
privilegios: inaceptable en multi-tenant.

**Decisión.** **Kaniko** como herramienta de build del PaaS: cada build es un **Job de
Kubernetes rootless** que toma el repo tras el webhook, detecta framework o usa el
Dockerfile, construye la imagen OCI **sin demonio ni privilegios**, la publica en el
registry UE y dispara el deploy. Build-cache en bucket S3 versionado.

**Descartado.** Docker-in-Docker / socket del host (escape a host), BuildKit rootless
(mejor caché pero un daemon más que operar — vigilado), Buildah (encaje algo peor como Job
efímero), Buildpacks (complementario, no sustituto), CI SaaS externo (soberanía).

**⚠ Seguimiento.** **Kaniko fue archivado por Google (2025)**: procede una decisión de
corrección para migrar a **BuildKit rootless** o al fork mantenido **antes de GA** — ejecuta
código de clientes, y CVEs sin parche es inaceptable. El contrato externo (push → URL) no
cambia.

---

<a id="0014"></a>

## 0014 — IaC dual: Compose local + Helm/Terraform en Scaleway

`accepted` · 2026-06-30 · infra, devex, iac

**Decisión.** **Paridad lógica** con dos herramientas:

- **Desarrollo = LOCAL con Docker Compose.** `make dev-up` levanta la suite entera en
  `deploy/compose/`, bajo `*.sytue.localhost` con **TLS local (mkcert)** vía **Traefik**, con
  datos de ejemplo. Los servicios gestionados de Cloud tienen equivalente local (PostgreSQL +
  PostgREST + MinIO).
- **Producción = SCALEWAY.** **Terraform** aprovisiona la infraestructura en
  `infra/terraform/` con módulos reutilizables (`network`, `kapsule`, `database`,
  `object-storage`, `registry`, `dns`, `secrets`, `loadbalancer`) y entornos
  dev/staging/prod; **Helm** despliega el umbrella chart `sytue-one` en Kapsule desde
  `deploy/helm/`. Estado de Terraform en backend S3.
- **Misma configuración lógica** (dominios, clients OIDC, variables) parametrizada por
  entorno: cambian endpoints y escala, **no la arquitectura**. Secretos nunca en git.

**Descartado.** K8s local (kind/k3d) por defecto (arranque pesado, peor DX — queda como
perfil avanzado), solo nube (caro y lento), Helmfile/Kustomize como base, Pulumi, y forzar
una sola herramienta para ambos entornos.

**Consecuencias.** Onboarding rápido y menos «funciona en mi máquina»; a cambio, **dos
definiciones que mantener** (riesgo de divergencia: todo servicio nuevo se añade a la vez a
Compose y al chart) y fallos que solo aparecen en Kapsule (NetworkPolicy, quotas,
scheduling) → staging antes de prod.

---

<a id="0015"></a>

## 0015 — Licenciamiento AGPL-3.0 del código propio · superseded por [0018](#0018)

`superseded` · 2026-06-30 · licensing

**Decisión (histórica).** Código propio bajo **AGPL-3.0**: la cláusula de red obliga a
publicar el fuente a quien ofrezca una versión modificada como servicio, cerrando el
«agujero ASP» de la GPL. Compatible con los motores libres de entonces. Descartadas
permisivas (permitirían SaaS cerrado sin contrapartida), GPL-3.0 (no cubre servicio),
open core (partes no libres) y source-available (no OSI).

**Por qué cayó.** Sin motores AGPL y siendo cada producto obra original, el copyleft deja de
ser necesario y limita el negocio.

---

<a id="0016"></a>

## 0016 — Stack único y total: Go + Astro/Preact + PostgreSQL

`accepted` · 2026-07-01 · **decisión madre del pivote**

**Contexto.** El diseño 0002–0015 ensamblaba motores de terceros: stack poliglota (PHP,
Rust, Java, TypeScript, Go), cinco motores AGPL que operar/tematizar/forkear/federar, y un
datastore en memoria además de Postgres.

**Decisión.** Stack **único y total**, sin excepciones de lenguaje en backend:

- **Backend = Go.** Módulo único `git.sytue.eu/sytue/one`; librerías compartidas en
  `platform/pkg/*`; un binario por servicio en `services/<svc>/cmd/<svc>/main.go`; capas
  `internal/{http,domain,store}`; `pgx` + `sqlc`, migraciones `goose`/`golang-migrate`,
  router `chi`, logs `slog`, `context` en todo I/O; Dockerfile multi-stage a distroless;
  cabecera SPDX propietaria por archivo.
- **Frontend = Astro + Preact** (islands), Tailwind — ver [0004](#0004); los SDKs de
  `@sytue/api-clients` apuntan ahora a los servicios Go.
- **Datastore ÚNICO = PostgreSQL.** Estado, colas y caché sobre Postgres ([0019](#0019)).
- **Blobs grandes** en Object Storage S3 (Scaleway prod / MinIO dev).

Única excepción de aplicación de terceros: **LiveKit** ([0021](#0021)).

**Descartado.** Mantener el stack poliglota; backend en TypeScript/Node (peor encaje para
servidores de larga vida y protocolos SMTP/IMAP/DAV); Rust (curva y ecosistema de groupware
permisivo menos maduro); conservar Valkey; permitir «islas» de otros lenguajes por servicio.

**Consecuencias.** Un solo modelo mental y superficie operativa mínima (un datastore, un
runtime, un stack de front); a cambio, hay que **reimplementar** lo que daban los motores —
esfuerzo grande, con riesgo de quedarse por debajo del upstream en casos límite (antispam,
recurrencias CalDAV, fidelidad OOXML). Todo backend nuevo nace como `services/<svc>/` en Go.

---

<a id="0017"></a>

## 0017 — Reimplementación nativa clean-room sobre estándares abiertos

`accepted` · 2026-07-01 · supersede 0003, 0005, 0006, 0009, 0010

**Contexto.** Con los motores retirados, hace falta una política de ingeniería que produzca
**código original propietario** que interopere con clientes estándar sin incurrir en obra
derivada de código copyleft.

**Decisión.** Cada producto se reimplementa en Go como **obra original mediante proceso
clean-room** sobre estándares y RFCs abiertos, usando **solo librerías permisivas**
(MIT/BSD/Apache/ISC):

| Producto                     | Estándar implementado                                  | Librerías de apoyo                                        |
| ---------------------------- | ------------------------------------------------------ | --------------------------------------------------------- |
| **Sytue ID**                 | OIDC/OAuth2 (Auth Code+PKCE), JWKS, SCIM, MFA/WebAuthn | `ory/fosite` o `zitadel/oidc`                             |
| **Sytue Drive**              | WebDAV + API REST propia                               | `x/net/webdav`, `emersion/go-webdav`                      |
| **Sytue Docs/Sheets/Slides** | modelo documental OOXML/ODF + colab CRDT/OT (WS)       | editor propio + `coder/websocket`                         |
| **Sytue Mail**               | SMTP/IMAP/JMAP                                         | `emersion/go-smtp`, `go-imap`, `go-message` + JMAP propio |
| **Sytue Calendar**           | CalDAV + API REST                                      | `emersion/go-webdav`, `emersion/go-ical`                  |
| **Sytue Contacts**           | CardDAV + API REST                                     | `emersion/go-webdav`, `emersion/go-vcard`                 |
| **Sytue Rooms**              | WebSocket + API REST                                   | `coder/websocket`                                         |

**Reglas clean-room (vinculantes).**

- **No se consulta el código fuente de los motores AGPL.** Se trabaja desde RFCs,
  especificaciones y APIs públicas: las ideas, la funcionalidad, el _look & feel_ y los
  protocolos no tienen copyright; el **código sí**. Prohibido copiar, traducir o portar.
- Las librerías enlazadas deben ser permisivas (gate de licencias en CI); se conservan
  avisos NOTICE.
- Cada implementación registra su **estándar de referencia** y sus **librerías**; cabecera
  SPDX propietaria por archivo.
- **Interoperabilidad como criterio de aceptación**: cada servicio se valida contra clientes
  estándar de su protocolo, no contra el motor que reemplaza.

**Descartado.** Mantener motores y construir solo clientes; reescribir «leyendo el código
AGPL como referencia» (obra derivada — riesgo legal inaceptable); adquirir una base
propietaria de terceros; implementar protocolos cerrados en vez de estándares.

**Riesgos (magnitud explícita).** **Correo es el reto mayor** (entregabilidad, antispam,
colas, reputación de IP, SPF/DKIM/DMARC/MTA-STS/TLS-RPT); **ofimática es el segundo**
(fidelidad OOXML/ODF + colaboración). Mitigación: alcance por fases, librerías permisivas
probadas y suites de interoperabilidad por protocolo.

---

<a id="0018"></a>

## 0018 — Licenciamiento propietario del código de producto (© Sytue)

`accepted` · 2026-07-01 · supersede 0015

**Decisión.** El código de producto de Sytue es **Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr))**, sin copyleft ni
obligación de publicar fuente: cubre `services/*`, `platform/pkg/*`, las superficies
Astro+Preact y los paquetes `@sytue/*`.

- **Cabecera SPDX por archivo Go:** `MIT` + `© Sytue`; cada paquete
  frontend con su `LICENSE` propietario.
- **Dependencias incorporadas al producto: solo permisivas** — MIT/BSD/Apache-2.0/ISC/
  PostgreSQL y, con cautela, MPL-2.0. Se conservan avisos LICENSE/NOTICE.
- **Prohibido AGPL/GPL y source-available** (SSPL/BSL/MSL/Elastic/RSAL) como dependencia
  enlazada, incorporada o motor del producto entregado al cliente. Los servicios de
  infraestructura operativa requieren decisión individual y nunca se presumen permitidos.
- **LiveKit (Apache-2.0)** es la única aplicación de terceros permitida en el runtime de cliente.
- **Gate de licencias de producto en CI** con SBOM (CycloneDX/SPDX): cualquier dependencia no
  permitida que llegue al artefacto rompe el build. Una licencia desconocida exige decisión; el
  copyleft fuerte y _source-available_ permanecen prohibidos en el perímetro de producto.

**Enmienda de alcance — 2026-08-01.** Las herramientas internas de ingeniería se evalúan en
un perímetro distinto. Un ejecutable o servicio GPL/AGPL puede usarse para control de versiones,
forja, CI, análisis o administración cuando sea autónomo, no se enlace ni empaquete con artefactos
Sytue, no se ofrezca como funcionalidad a clientes, se inventaríe y se cumplan íntegramente sus
obligaciones (incluida la oferta de fuente y modificaciones cuando aplique). Su salida no se
considera automáticamente derivada: el gate verifica que nada de la herramienta entre en los
binarios, imágenes o instaladores. Las licencias _source-available_ siguen bloqueadas por defecto
también para tooling y exigen ADR específico. Esta separación no relaja la política de origen UE,
que se aplica de forma independiente a producto, infraestructura y tooling.

**Descartado.** Seguir con AGPL, permisiva para el código propio, open core, source-available.

**Consecuencias.** Control total del producto y del modelo de negocio; a cambio, disciplina
de cumplimiento estricta (cabeceras, SBOM, NOTICE) y riesgo de arrastrar copyleft transitivo
→ bloqueo automatizado en CI. No afecta a los datos ni al código de los clientes.

---

<a id="0019"></a>

## 0019 — PostgreSQL como datastore único (colas y caché sobre Postgres)

`accepted` · 2026-07-01 · supersede 0008

**Decisión.** **PostgreSQL es el datastore ÚNICO**: estado, colas y caché sobre Postgres. Se
elimina Valkey/Redis del stack.

- **Colas / trabajos en background:** `riverqueue` o `pgmq` (colas transaccionales) y/o
  `LISTEN`/`NOTIFY` para fan-out; workers Go, _at-least-once_, reintentos y dead-letter.
- **Caché:** tablas con TTL/invalidación o caché en proceso dentro de cada servicio.
- **Realtime/pub-sub:** `LISTEN`/`NOTIFY` como bus ligero; WebSocket/SSE hacia el cliente
  desde los propios servicios Go.
- **Blobs grandes:** a Object Storage, **no** a Postgres.
- **Multi-tenant:** esquema/RLS por tenant; prefijos S3 por tenant.

**Descartado.** Conservar Valkey (pieza extra sin consumidores que la exijan), volver a
Redis (licencia), broker dedicado (NATS/RabbitMQ/Kafka — sobredimensionado; reconsiderable
con una decisión nueva si la escala lo exige), caché distribuida propia.

**Consecuencias.** Una sola pieza de datos que operar y respaldar, y **colas
transaccionales** (encolar en la misma transacción que el cambio de estado elimina el
«commit en BD pero no en la cola»); a cambio, techo de rendimiento distinto al de un KV en
memoria → `LISTEN/NOTIFY` en vez de polling agresivo, autovacuum e índices afinados,
particionado/archivado de las tablas de cola y caché en proceso para _hot paths_.

> **Nota posterior (vigente).** El invariante de escala llevó a adoptar **DragonflyDB** como
> caché/pub-sub a gran escala para uso interno; ver `CONVENTIONS.md`. La regla «sin KV
> externo» de esta decisión queda acotada a lo que Postgres cubre bien.

---

<a id="0020"></a>

## 0020 — Sytue ID como IdP nativo en Go (OIDC/OAuth2/SCIM/MFA)

`accepted` · 2026-07-01 · supersede 0002

**Decisión.** **Sytue ID** es un servicio Go propio que implementa, sobre librerías
Apache-2.0 (`ory/fosite` o `zitadel/oidc`): **OIDC/OAuth2** Authorization Code + **PKCE**,
refresh tokens, **JWKS**, discovery, **SCIM** y **MFA/WebAuthn** (SAML según necesidad
enterprise). Datos de identidad en PostgreSQL.

- **Clients OIDC** (BFF confidencial por superficie): uno por app. No hay clients de
  federación de motores.
- **Cada servicio Go valida los JWT vía JWKS** con el middleware `authn` de
  `platform/pkg/authn`, y aplica autorización por roles/tenant.
- **Grupos/roles:** `org/<tenant>` con `owner`, `admin`, `member`, `billing`; **MFA/WebAuthn
  obligatorio** para `admin`/`owner`. Un alta en Sytue ID → acceso a toda la suite.
- **SSO:** cookie host-only `__Host-` por BFF tras OIDC+PKCE; Sytue ID evita repetir credenciales.
- El BFF de Meet mintea el JWT de LiveKit **tras** validar OIDC.

**Descartado.** Mantener Keycloak (motor Java de terceros como pieza nuclear); otro IdP de
terceros desplegado (sí usamos sus librerías); SaaS de identidad US; implementar OIDC «a
mano» sin librería madura (riesgo de seguridad altísimo).

**Riesgo dominante.** Es la **decisión de mayor riesgo de seguridad de la suite**: un fallo
en tokens, sesiones, PKCE, refresh, JWKS o WebAuthn compromete a **todos** los productos.
Mitigación obligatoria: librerías probadas para los flujos criptográficos, **auditoría de
seguridad externa antes de producción**, conformidad OIDC y pentest. Además es punto único
de fallo del SSO → HA, Postgres replicado y runbook de recuperación prioritario.

---

<a id="0021"></a>

## 0021 — LiveKit (Apache-2.0) como SFU de medios de Sytue Rooms

`accepted` · 2026-07-01 · meet, webrtc

**Contexto.** La señalización y las salas pueden ser un servicio Go propio, pero el **plano
de medios WebRTC** —un SFU con simulcast, control de congestión, gestión de pérdidas,
TURN/ICE— es ingeniería especializada de años.

**Decisión.** **Sytue Rooms** = **servicio Go propio de señalización/salas** (salas, permisos,
presencia y estado en PostgreSQL, OIDC contra Sytue ID) + **LiveKit** (Apache-2.0, escrito
en Go) como **SFU de medios**, la **única aplicación de terceros permitida en el runtime de
cliente**. El BFF mintea
el JWT de LiveKit tras validar OIDC; el frontend usa el SDK web dentro de la app Astro.

**Descartado.** Reimplementar el SFU (años de trabajo y altísimo riesgo), Mattermost
Calls/rtcd (acoplado al motor retirado), Jitsi (stack Java y producto completo que no
encaja), construir sobre Pion (recae en reimplementar el SFU), SaaS de vídeo US.

**Consecuencias.** Vídeo de calidad de producción sin asumir el riesgo del SFU; a cambio,
dependencia de un componente de terceros (versión fijada, CVEs, NOTICE) y operación WebRTC
exigente (TURN/ICE, NAT, rango UDP). Excepción **acotada**: ninguna otra app de terceros sin
decisión registrada. El estado vive en Postgres; LiveKit solo transporta medios.

---

<a id="0022"></a>

## 0023 — Eliminación de los motores AGPL de terceros

`accepted` · 2026-07-01 · engines, licensing

**Decisión.** Se eliminan del stack **todos** los motores de terceros AGPL — Keycloak,
Nextcloud, OnlyOffice, Mattermost y Stalwart — más Valkey/Redis:

| Motor retirado   | Función                     | Sustituto Sytue-native (Go)                             |
| ---------------- | --------------------------- | ------------------------------------------------------- |
| **Keycloak**     | Identidad/SSO/SCIM/MFA      | **Sytue ID** ([0020](#0020))                            |
| **Nextcloud**    | Archivos + CalDAV + CardDAV | **Drive** + **Calendar** + **Contacts** ([0017](#0017)) |
| **OnlyOffice**   | Ofimática colaborativa      | **Docs/Sheets/Slides** ([0017](#0017))                  |
| **Stalwart**     | Correo SMTP/IMAP/JMAP       | **Sytue Mail** ([0017](#0017))                          |
| **Mattermost**   | Chat + vídeo                | **Sytue Rooms** ([0017](#0017), [0021](#0021))          |
| **Valkey/Redis** | Caché/cola                  | **PostgreSQL** ([0019](#0019))                          |

Fuera del repo quedan la configuración de Keycloak, el Hub y la PaaS en TypeScript y los
temas de los motores; sin charts de motores AGPL ni clients OIDC de federación.

**Descartado.** Mantener los motores; **retirar solo algunos** (mantendría un motor ajeno en el
runtime de cliente, sus obligaciones y la heterogeneidad que se quiere eliminar); sustituirlos por
otros motores de terceros permisivos, salvo LiveKit.

**Consecuencias.** Cero dependencia AGPL incorporada al producto y cero motor AGPL en su runtime;
stack único sin rebases de forks; a cambio, pérdida de funcionalidad madura que hay que reconstruir
(ver magnitudes en [0017](#0017)) y **riesgo de cumplimiento clean-room**: garantizar que nadie
consulta la fuente AGPL. Esta decisión no regula ejecutables autónomos de tooling interno, cuyo
perímetro quedó separado en la enmienda de [0018](#0018).

---

<a id="0023"></a>

## 0023 — Sustrato de Sytue Cloud: Kapsule, Coolify como referencia, Firecracker como escotilla

`accepted` · 2026-07-10 · paas, substrate

**Contexto.** ¿Puede un Coolify refactorizado servir de base para Sytue Cloud? ¿Es
Kubernetes/Kapsule el sustrato correcto, o conviene **Firecracker** (modelo Fly.io) para el
aislamiento multi-tenant hostil? La decisión afecta al invariante de escala (cientos de
miles de usuarios), a «backend siempre Go» y al modelo de tenancy ya fijado.

**Decisión.**

1. **Coolify: referencia de dominio, no fuente. Sin atajo MVP.** Se descarta portarlo o
   forkearlo, y también correr Coolify real tras la UI Astro vía su API: aporta ~cero código
   reutilizable a un stack Go (su valor es el motor agnóstico que orquesta, que Go maneja
   nativamente), su arquitectura es **single-tenant** y el atajo viola «backend siempre Go» +
   escala. Se toma su **modelo de dominio** (apps/servicios/envs/deploys/preview/catálogo
   one-click) como referencia.
2. **Sustrato: Kubernetes gestionado (Kapsule)**, reafirmando [0011](#0011) — tenant →
   namespace con ResourceQuota + NetworkPolicy. Ya en producción en el repo:
   `services/cloud/internal/controller/kubeplatform.go` (Server-Side Apply de
   Deployment/Service/Ingress/HPA), el builder OCI sin demonio, los charts y los módulos
   Terraform. Kapsule aporta el plano de control mutualizado; el ecosistema es permisivo.
3. **Firecracker: rechazado como sustrato inicial, preservado como escotilla.** Un
   orquestador tipo `flyd` son 2-3 años-ingeniero de sistemas distribuidos que K8s ya pagó, y
   tiraría el `services/cloud` que ya funciona. La opción se preserva vía la interfaz
   **`controller.Platform`**: una célula Firecracker/Kata futura se enchufa sin tocar el plano
   de control. El kernel compartido es la debilidad honesta de K8s; se mitiga (no se iguala)
   con PodSecurity `restricted`, seccomp, `automountServiceAccountToken=false`,
   NetworkPolicy y **pools dedicados por plan**.

**Descartado.** Fork/port de Coolify; Firecracker primario; **Nomad** (BSL 1.1).

**Seguimiento (huecos detectados, por valor).**

1. **Static-first offload:** servir sitios estáticos desde S3 vía un `cloud-edge` compartido
   en vez de un contenedor siempre-on por sitio (~10× más barato a 100k sitios).
2. **Egress de `NetworkPolicy`:** permitir Internet **excepto** RFC1918, CIDR del clúster y
   `169.254.169.254`, para que un tenant no alcance servicios internos.
3. **`LimitRange` por namespace de tenant**, además de la `ResourceQuota`.
4. **Escala de listados:** cursores **keyset** (no OFFSET) + cron de purga/rollup de
   builds/webhooks/outbox/previews retirados/usage.
5. **Sharding en células:** columna `organizations.cell` para repartir orgs entre clústers.
6. **`scale-to-zero`** de previews/free con KEDA (prod de pago `minReplicas≥1`).
7. ⚠ **Kaniko archivado** → migrar a BuildKit rootless antes de GA (ver [0013](#0013)).

---

<a id="0024"></a>

## 0024 — Apps de escritorio (Fyne) y móvil (PWA + Capacitor)

`accepted` · 2026-07-11 (revisada 2026-09-20) · desktop, mobile

**Contexto.** Para la paridad con OnlyOffice hacen falta apps de escritorio y móvil, con
restricciones: **código único** (no mantener editores nativos aparte), clean-room permisivo,
soberanía, offline razonable y **un solo lenguaje de sistema (Go)**.

**Decisión.** Mantener el dominio y las APIs compartidas, con interfaces nativas donde se
instalan como aplicación:

- **Escritorio → Fyne 2.8.1** (Go, BSD-3). Las vistas se renderizan de forma nativa en
  macOS, Windows y Linux, sin WebView2, WKWebView, WebKitGTK ni Chromium. El shell y sus
  vistas están en `apps/desktop/shell/gui/`; consumen las mismas APIs autenticadas de Sytue
  ID y los servicios Go que las apps web.
- **Móvil → PWA instalable como base, Capacitor (MIT) para tiendas** y APIs nativas,
  reutilizando la misma web app.
- **Offline por capacidad, no por plataforma.** Las apps Fyne guardan su estado local y lo
  sincronizan con las APIs al reconectar; las apps móviles mantienen su estrategia PWA/
  Capacitor. Acciones irreversibles o dependientes de verdad en tiempo real no se simulan. El
  backend Go **no** se empaqueta en el cliente. Contrato: `docs/roadmap.md#offline-native`.

**Descartado.** **Tauri 2** (core en Rust: segundo lenguaje de sistema, sin ventaja neta
frente a Fyne), **Wails** (depende de un WebView), **Electron** (empaqueta Chromium, 100+ MB),
React Native / nativo / Flutter (base de código separada → duplicar los editores), solo PWA sin
Capacitor (sin tiendas).

**Coste / seguimiento.** Pipelines nuevos (firma de código, notarización macOS, Xcode/Android
SDK y cuentas de tienda); mantener tests de recuperación/conflictos, OIDC PKCE, deep links
`sytue://` y almacenamiento local seguro.

---

<a id="0025"></a>

## 0025 — Sytue Rooms: mensajería y videollamada, un servicio y una base

`accepted` · 2026-07-28 · rooms, arquitectura

**Contexto.** Chat y Meet nacieron como dos servicios Go con dos bases de datos, y desde el
primer día tuvieron que hablarse: la llamada arranca DESDE un canal, así que el vídeo debe
preguntar «¿puede esta persona leer el canal?» antes de abrir la sala, y al colgar debe
publicar la tarjeta de «llamada finalizada» EN ese canal. Esa conversación viajaba por HTTP
entre procesos con un secreto compartido.

El acoplamiento no era teórico: si el secreto faltaba en un entorno, el puente quedaba a nil y
la verja del canal fallaba **cerrada** — abrir una llamada devolvía 403 sin explicación posible
para quien lo sufría. Pasó en dev y en el chart. Y la tarjeta de fin de llamada nunca podrá ser
atómica con el mensaje mientras sean dos bases.

Para el usuario, además, nunca fueron dos productos: la llamada es un botón del canal.

**Decisión.** **Un servicio** (`services/rooms`) y **una base** (`sytue_rooms`). La mensajería
vive en `internal/`, la videollamada en `internal/media/`, y el puente entre ambas es una
llamada en proceso (`media/service.InProcessNotifier`) dentro de la misma transacción. Un solo
`events_outbox`, un solo despliegue, un solo host (`rooms.<dominio>`: el frontend sirve `/` y el
servicio Go `/api`, `/ws` y `/hooks`). El SFU LiveKit sigue siendo un componente aparte
([0021](#0021)) en `rooms-sfu.<dominio>`: transporta medios, no guarda estado.

**Descartado.** Mantener los dos servicios arreglando el secreto (deja en pie la imposibilidad
de la atomicidad y dos despliegues que versionar juntos); una base compartida entre dos
servicios (peor: acoplamiento de esquema sin la ventaja de la transacción); un monolito de la
suite entera (Rooms se fusiona porque comparte dominio —el canal—, no por moda).

**Consecuencias.** Desaparecen `CHAT_INTERNAL_SECRET` en este camino, un chart, una imagen y
una base. El escalado del vídeo deja de ser independiente del de la mensajería — se acepta
porque el trabajo pesado del vídeo lo hace el SFU, no este proceso, que solo orquesta salas,
tokens y avisos. La migración de un entorno ya desplegado es con parada y está escrita en
[`operations.md` §1.4.4](./operations.md#44-fusión-de-sytue_chat--sytue_meet--sytue_rooms-una-sola-vez-por-entorno);
volver atrás después de la primera escritura pierde datos.

---

<a id="0026"></a>

## 0026 — Sytue Analytics: anonimato por construcción, medición en primera parte

`accepted` · 2026-07-29 · analytics, gdpr, privacy, cloud

**Contexto.** Quien despliega una app en Sytue Cloud no tiene forma de saber cuánta gente la
visita. Adoptar algo existente está descartado antes de empezar, y no por gusto:
**Plausible (AGPL) y Matomo (GPL)** son aplicaciones de analítica que entrarían en el
**perímetro B** (runtime que procesa datos de clientes), no simples herramientas internas. §12 no
afirma que un servicio separado vuelva copyleft el producto, pero exige ADR, aislamiento,
cumplimiento de fuente, origen UE y plan de salida; además romperían el principio de no añadir otro
motor de producto. **Umami** es MIT, luego la licencia no es el problema, pero es Node y
[`§0`](./CONVENTIONS.md#0-principios-nucleares-en-orden-de-prioridad) principio 1 cierra la puerta
—«sin excepciones de lenguaje en backend»—; **GA4, PostHog, Fathom, Vercel Analytics** son
procesadores US y chocan con [0001](#0001) y con `security.md §1` («no se emplean servicios US de
analítica»). **Se construye en Go, en el monorepo.**

Lo que queda por decidir es todo lo demás, y **una sola pieza lo decide**: contar visitantes
únicos sin cookies y sin guardar IPs.

### 1. Identidad efímera: HMAC con sal diaria que se destruye

`visitor = HMAC-SHA256(sal_del_día, site_id ‖ ip ‖ user_agent)`, truncado a 16 bytes. HMAC y no
`hash(sal‖datos)`: es la primitiva correcta para hashear con clave, está en la stdlib
(`crypto/hmac`) y no cuesta nada. **La sal se genera una vez al día, se guarda cifrada con
[`platform/pkg/keycrypto`](../platform/pkg/keycrypto/keycrypto.go) en una fila de
`sytue_analytics`, se cachea en proceso y el cron la BORRA al cambiar el día.** Cifrada porque un
volcado de Postgres sin la KEK del gestor de secretos no permite recalcular ningún hash; en
Postgres y no en Valkey porque §0.1 exige justificación de rendimiento y aquí es **una lectura al
día**, no un camino caliente.

Esto no es una política de privacidad: es lo que el producto **puede** y **no puede** hacer.

1. **Sin la sal no hay correlación entre días.** Nadie —ni Sytue con acceso root— puede
   reconstruir a un visitante de ayer. El perfilado a largo plazo deja de estar prohibido y pasa
   a ser **imposible**. Su precio, que se acepta y se documenta en la interfaz: **no existen
   «visitantes únicos del mes», ni retorno, ni cohortes.** Un rango largo muestra **suma de
   únicos diarios**, etiquetado como tal — nunca un único mensual deduplicado, que sería mentira.
   Una visita que cruza medianoche cuenta como dos.
2. **`site_id` dentro del HMAC.** La misma persona en los sitios de dos clientes produce
   identificadores **incorrelacionables**, y no por contrato sino porque el dato para cruzarlos no
   existe. «Sin seguimiento fuera de los servicios de cada cliente» es aquí una **imposibilidad
   estructural**.
3. **Sin cookies no hace falta banner** — pero solo si de verdad no se toca el dispositivo, así
   que se convierte en invariante de código, no en promesa: **prohibido `document.cookie`,
   `localStorage`, `sessionStorage`, `IndexedDB`, Cache API y cualquier huella activa** (canvas,
   WebGL, fuentes, `hardwareConcurrency`, `deviceMemory`) — **incluido `screen.*`**, que las
   Directrices EDPB 2/2023 tratan como acceso al equipo terminal; el tipo de dispositivo se deduce
   del user-agent, que el navegador ya envía. **Un test de CI busca esos identificadores en el
   script COMPILADO y rompe el build.** Honestidad sobre el alcance: durante 24 h el hash es dato
   personal seudonimizado (base jurídica: interés legítimo); **destruida la sal, es anónimo**. El
   texto legal exacto lo firma el dueño con asesoría (ver «Decisiones de negocio» del informe).

### 2. Minimización como esquema: lo que no se escribe no se filtra

| Se guarda                 | Origen                 | Regla                                                 |
| ------------------------- | ---------------------- | ----------------------------------------------------- |
| `visitor`                 | HMAC del §1            | `bytea(16)`, vive 48 h                                |
| `path`                    | `location.pathname`    | **sin query string**, ≤255 chars                      |
| `referrer_host`           | `Referer`              | **solo el host**; si es el propio sitio → `NULL`      |
| `utm_*`, `ref`            | query string           | **allowlist de 6 claves** exactas, ≤64 chars cada una |
| `country`                 | IP, en memoria         | ISO-3166-1 alpha-2; desconocido → `ZZ`                |
| `browser`, `os`, `device` | user-agent, en memoria | 3 enums pequeños                                      |
| `event`                   | atributo declarado     | **allowlist por sitio**, ≤50 nombres                  |

**Nunca, en ninguna tabla ni log: la IP** (ni hasheada, ni truncada a /24 — sigue identificando),
**la URL completa, el `Referer` completo, el user-agent crudo, el idioma** (un bit de
reidentificación que no aporta nada al producto) ni identificador alguno de la app del cliente.

- **La query string se descarta EN EL NAVEGADOR**, no en el servidor. Un `?email=`, un
  `?reset_token=` o un id de sesión que llegue al servidor ya ha cruzado la red y ha pasado por un
  log de acceso o un volcado. El servidor **además** corta por `?` todo lo que reciba, porque el
  colector es público y cualquiera puede enviarle lo que quiera. Los dos, y el del navegador
  primero.
- **País sin IP.** Fichero **DB-IP IP-to-Country Lite (CC BY 4.0)** empotrado en la imagen,
  búsqueda binaria sobre rangos `net/netip` ordenados (~40 líneas, cero dependencias, IPv4 e IPv6
  igual). La IP es una **variable local** del handler: entra en el HMAC, entra en la búsqueda de
  país, entra en el rate-limit y **no se asigna a ningún campo que se persista**. Consecuencia
  operativa que hay que cumplir o el producto miente: **los logs de acceso de Traefik del hostname
  de ingestión no retienen IP de cliente.**
- **Cardinalidad de rutas:** no se normaliza automáticamente (adivinar es peor); lo que acota el
  crecimiento es que el agregado guarda **top-N por día** más un cajón `(otras)`.

### 3. Volumen: partición diaria + agregados, y el panel nunca toca el crudo

Mismo patrón que el rollup de `usage_records`
([`store/purge.go`](../services/cloud/internal/store/purge.go)): detalle corto, agregado largo.

- `analytics_events` **particionada por RANGE en `ts`, una partición por día**; el cron crea 7 días
  por delante y **`DROP TABLE`** las caducadas — borrar millones de filas sin vacuum ni WAL.
- **Retención del crudo: 48 h.** Deliberadamente corta: es donde vive el hash seudónimo. Coste
  aceptado y explícito: **las definiciones de los agregados no se pueden cambiar
  retroactivamente.**
- `analytics_hourly` y `analytics_daily` (totales) + `analytics_daily_breakdown`
  `(site, day, dim, value, pageviews, visitors)`. Dos tablas de totales y no una porque **los
  únicos no se suman**: el día se calcula sobre el crudo, no sumando horas. Los agregados **no
  contienen ningún identificador de visitante** — son anónimos, y por eso pueden vivir años.
- **Las visitas se reconstruyen en el rollup** (mismo `visitor`, hueco < 30 min), no en el
  navegador: **no hay cookie de sesión ni id de sesión enviado**, y la reconstrucción muere con el
  crudo.
- **Trampa concreta:** bajo `FORCE ROW LEVEL SECURITY`, PostgreSQL **rechaza `COPY … FROM`**. El
  diseño obvio de firehose no está disponible: se escribe con **`INSERT` multi-fila** por lotes
  (~1 s o N eventos).

### 4. Multi-tenant: el que escribe no puede leer

`ENABLE` + `FORCE` + predicado explícito, como el resto. Además, dos políticas por comando —el
proceso de ingestión es intrínsecamente cross-tenant y no tiene ninguna razón para leer:

```sql
CREATE POLICY analytics_events_tenant ON analytics_events
    USING (tenant = current_setting('app.tenant', true));
CREATE POLICY analytics_events_ingest ON analytics_events
    FOR INSERT WITH CHECK (current_setting('app.ingest', true) = 'on');
```

El rollup y la purga corren bajo `app.maintenance='on'`, idéntico a `runtime_logs` y a
`PurgeExpired`. **Nunca un rol con `BYPASSRLS`.**

### 5. Dónde vive: `services/analytics`, y por qué NO dentro de `services/cloud`

**Servicio propio**, base `sytue_analytics`, dos binarios: `analytics-ingest` (público, sin auth,
superficie mínima) y `analytics-api` (JWT vía `platform/pkg/authn` + crons). El argumento decisivo
no es la pureza sino el **radio de explosión**: el colector es el endpoint **público, no
autenticado y de mayor volumen de toda la plataforma** —recibe el tráfico agregado de las apps de
todos los clientes— y `services/cloud` tiene en memoria `SECRET_ENC_KEY`, credenciales de
Kubernetes y la facturación. Un fallo en el parser del beacon no puede estar a un salto de eso. A
ello se suma que su base es la que decide si se puede desplegar, y no puede depender de un
firehose.

No es el caso de [0025](#0025): allí la fusión la exigía **una transacción** (el mensaje y la
tarjeta de fin de llamada). Aquí no hay ninguna transacción que cruce. La composición —crear el
sitio, activar el interruptor— la hace el **BFF del dashboard con el token del usuario**, en dos
llamadas: **no se crea ningún secreto interno compartido**, que era la lección de 0025.

**El frontend NO es una app nueva**: es una sección de `cloud-dashboard`
(`/projects/[slug]/analytics`). Un dominio, un client OIDC y una entrada de switcher menos.

### 6. En primera parte: `/_sytue/` en el propio dominio del cliente

**El script y el colector se sirven bajo el dominio del propio sitio**, no desde un host de Sytue.
`applyIngress` ya construye las reglas del Ingress
([`kubeplatform.go`](../services/cloud/internal/controller/kubeplatform.go)): activar analítica
**añade una segunda ruta `/_sytue` (Prefix)** que Traefik prioriza sobre `/` por ser más
específica, apuntando a un `Service` `ExternalName` del namespace del tenant hacia
`analytics-ingest`. Se paga con `allowExternalNameServices=true` en Traefik — acotado porque los
tenants no tienen acceso a la API de Kubernetes: los Ingress los escribe solo el control plane.

Lo que compra, y es mucho más de lo que parece:

- **Nada de terceros**: ningún dominio externo que bloquear, **cero peticiones cross-origin, cero
  CORS, y el cliente no tiene que abrir su CSP** — un script de analítica de terceros obliga a
  todos sus clientes a relajar `script-src`.
- **El sitio se identifica por el `Host`**, así que **el script no lleva ningún identificador**: el
  snippet es una etiqueta sin configurar. Y desaparece por construcción toda la clase de ataques
  «inflar los números de otro cliente falsificando su `site_id`»: **no se puede apuntar el beacon a
  A diciendo que eres B, porque el sitio ES el hostname.**
- El suelo honesto que queda: **quien pueda enviar peticiones HTTP a un sitio puede inflar sus
  cuentas.** Es irreducible en cualquier colector público sin cookies. Se amortigua con
  `Sec-Fetch-Site: cross-site` → rechazo (cabecera prohibida, el JS no puede falsificarla),
  `Origin`/`Host` discordantes → rechazo, `POST` obligatorio (nada de píxel GET: se cachea y se
  prefetchea, y ensucia), límite de cuerpo con `httpx.limits` y **token-bucket en memoria por IP**
  (`httpx.TokenBucket`; techo conocido: N réplicas → N× el límite, suficiente para amortiguar
  abuso, y sin un viaje a Postgres por evento).

### 7. Inyección: interruptor en el panel, y en tiempo de BUILD

De las tres opciones reales, **ninguna sirve sola**:

- _Que la plantilla lo traiga activado_ — `domain.TemplateManifest` es un **esquema cerrado**
  (`DisallowUnknownFields`) que declara build, env y servicios, **no ficheros**: el HTML viene del
  repo del usuario. Sirve para proyectos nuevos y no hace nada por los existentes.
- _Interruptor que inyecta una variable de entorno_ — `SYTUE_ANALYTICS_*` no hace nada por sí
  sola: alguien tiene que leerla y emitir la etiqueta. **Eso es un SDK con otro nombre**, y no
  existe para un sitio estático.
- _Inyección en el borde (Traefik)_ — **descartada**: no hay proxy que modifique respuestas y
  construirlo obliga a bufferizar o parsear el HTML de todas las apps, descomprimir y recomprimir,
  romper el streaming del SSR y el `Content-Length`, y modificar en silencio la salida del cliente.

**El interruptor hace dos cosas de verdad, y hay un tercer camino manual:**

1. **Publica `/_sytue` en el Ingress del proyecto** — la parte que la app no puede hacer sola.
2. **Inyecta `<script defer src="/_sytue/s.js"></script>` antes de `</head>` en tiempo de build**,
   sobre los ficheros que **produce la propia plataforma** (`detect.go` ya genera el Dockerfile que
   copia `dist/` a nginx). Determinista, coste cero en petición, y **funciona en proyectos ya
   existentes en su siguiente despliegue**.
3. Para SSR y apps con servidor, donde no hay artefacto estático que tocar: **el snippet de una
   línea**, con botón de copiar en el panel. Sin claves, sin configuración.

### 8. El script, los bots y las señales del navegador

- **Presupuesto: < 1,5 KB gzip, con budget que rompe el build en CI** (§0.8 ya lo hace con
  bundles). `navigator.sendBeacon` (no bloquea la navegación, sobrevive al `unload`, y con
  `text/plain` no dispara preflight), `fetch(…, {keepalive:true})` de reserva. Engancha
  `pushState`/`popstate` para SPAs. `try/catch` mudo: **jamás rompe la página del cliente**.
- **DNT y GPC: innegociables, sin interruptor.** `navigator.globalPrivacyControl === true` o
  `navigator.doNotTrack === '1'` → **el script no envía nada**; y el servidor honra también
  `Sec-GPC: 1`, por si alguien esquiva el script. Motivo: el producto entero se vende como
  «privacidad por imposibilidad, no por política», y un interruptor para ignorar GPC
  **reintroduce exactamente la política que quitamos**; además obligaría a explicar al cliente que
  puede aumentar sus números apagándolo, que es la conversación que no queremos tener. Cuesta unos
  puntos de conteo y compra la afirmación entera.
- **Bots.** Sin filtrar, los números son basura. Lista **`crawler-user-agents` (MIT)** empotrada,
  cotejada contra el user-agent **antes de cualquier escritura**; se descarta, no se marca (marcar
  es guardar más). Ceiling honesto y escrito: **el filtrado por user-agent lo derrota cualquiera
  que quiera derrotarlo** — quita la basura sincera (crawlers, monitores, previsualizadores), no a
  un atacante. El filtrado por rangos de datacenter (segundo dataset, ASN) **queda fuera hasta que
  un cliente reporte números inflados desde nubes**.
- **Clics y eventos sin SDK.** Automáticos y sin tocar nada: **enlaces salientes** (solo el host
  destino), **descargas** (extensión en lista fija) y `mailto:`/`tel:`. Para lo demás, **un
  atributo HTML**: `data-sytue-event="alta"`, con **un solo listener delegado**. Y la parte que lo
  hace GUI de verdad: **objetivos por patrón de ruta** (`/gracias*`) declarados en el panel, que se
  calculan sobre datos que ya existen y **no requieren ni una línea en la app del cliente**. Los
  nombres de evento son **allowlist por sitio**: sin ella, `data-sytue-event="pepe@correo.com"`
  convierte el almacén «anónimo» en un canal de exfiltración. **No hay API JS** (`window.sytue(…)`)
  en v1; se añadiría solo si aparece algo que no sea ni una página ni un clic.

**Descartado.** Plausible y Matomo (AGPL/GPL y motores adicionales en el perímetro B: obligaciones,
operación y salida innecesarias frente al servicio propio); Umami (MIT pero Node — la licencia
vale, el lenguaje no, §0 principio 1); GA4/PostHog/
Fathom/Vercel Analytics (procesadores US, [0001](#0001)); **ClickHouse** como almacén (Apache-2.0,
así que la licencia no es el problema: es un **segundo datastore que operar** para una función
gratuita, contra [0019](#0019) — el particionado + rollup mantiene a Postgres en pie, y el gatillo
para reabrirlo está escrito abajo); cookies, `localStorage` y huella digital; guardar la IP en
cualquier forma; identidad de visitante entre días; **inyección en el borde reescribiendo HTML**;
un SDK de JavaScript; un dominio de script de terceros (`t.sytue.eu`) — que solo se construirá si
el negocio decide vender esto a sitios **fuera** de Sytue Cloud.

**Consecuencias.**

- Un servicio, una base y dos binarios más que operar; **ninguna dependencia nueva de terceros**
  salvo dos ficheros de datos (DB-IP CC BY 4.0, `crawler-user-agents` MIT) que exigen
  **atribución en NOTICE**.
- El panel **nunca consulta el crudo**, que además solo vive 48 h — y por eso **las métricas
  históricas no se pueden redefinir a posteriori**.
- **Faltan primitivas de gráfico:** no existe **ninguna** en el repo (`@sytue/ui` no tiene ni una;
  el `Chart.tsx` de Sheets son 980 líneas atadas al motor de fórmulas). Hay que crear en
  `@sytue/ui` —una sola vez, como manda [0004](#0004)— tres piezas mínimas en SVG sin
  dependencias: serie temporal, lista de barras y tesela de estadística (que `usage.astro` ya
  improvisa en línea y también arregla).
- **Requiere actualizar [`CONVENTIONS.md`](./CONVENTIONS.md)**: fila de producto en §1, puertos
  **8094** (api) y **8095** (ingest) en §6, y `ANALYTICS_DB_NAME=sytue_analytics`. Sin eso, el
  contrato canónico y el código divergen.
- Los **5 idiomas** (EN base + ES/FR/DE/IT) son condición de terminado
  ([§13.1](./CONVENTIONS.md#13-1-idiomas-de-producto)), incluidos los nombres de métrica y los
  textos que explican qué NO se puede medir.
- **Gatillos escritos para reabrir esta decisión:** si el crudo de un solo día no cabe en una
  partición manejable en Postgres (o el rollup no cierra el día dentro de la hora), vuelve
  ClickHouse a la mesa con una decisión nueva; si aparecen números inflados desde rangos de nube,
  entra el filtrado por ASN; si un cliente exige cifras certificadas, la lista IAB/ABC es de pago
  y es decisión de negocio.

---

<a id="0027"></a>

## 0027 — Excepción temporal Bun/Kaniko para `v0.1.6` interna

`accepted` · 2026-08-01 · seguridad, soberanía, release · **expira 2026-10-01**

**Contexto.** La primera producción real tendrá como máximo diez usuarios internos, ningún cliente
y sirve para validar iteraciones. El candidato `v0.1.6` ha superado build, tests, integración con
PostgreSQL, escaneo de vulnerabilidades, secretos, licencias y generación de SBOM; el workflow de
release exige además firmar cada imagen antes de publicar el borrador. Aun así, el frontend
SSR/BFF ejecuta Bun —producto bajo control de Anthropic (EE. UU.)— y la construcción OCI usa
Kaniko —originado por Google y archivado—. La política general solo exceptúa GitHub y por tanto
impediría publicar el candidato sin una decisión expresa del propietario.

**Decisión.** Se autoriza **una sola excepción**, exclusivamente para producir y operar
`v0.1.6` en la validación interna de Sytue One:

- Bun puede permanecer como runtime de los frontends de `v0.1.6` y como herramienta de build.
- Kaniko puede construir las imágenes de esa release y los builds de prueba de Sytue Cloud.
- La excepción incluye descargar sus imágenes oficiales fijadas por digest dentro de GitHub
  Actions; todas las imágenes resultantes viven privadas en Scaleway Registry, con SBOM y firma.
- No autoriza clientes, GA, nuevas dependencias de origen extracomunitario, ampliar Sytue Cloud a
  cargas de terceros ni reutilizar la excepción para `v0.1.7` o cualquier release posterior.

**Controles compensatorios.** Registro privado UE; imágenes y Actions fijadas; firma cosign;
SBOM CycloneDX; escaneo OSV/Trivy/gitleaks; ejecución rootless, seccomp, capacidades eliminadas,
filesystem de solo lectura y egress acotado. Los proveedores de origen no procesan datos de
clientes: el runtime se ejecuta en Scaleway y esta fase no tiene clientes.

**Salida obligatoria.** Antes de otra release se trasladan los BFF/SSR a Go y se generan
frontends estáticos sin Bun en runtime; Kaniko se reemplaza por una cadena mantenida y aprobada por
el gate de origen. El 2026-10-01 la excepción caduca aunque no exista sustituto. Renovarla exige
otra ADR y una nueva aprobación expresa; el silencio no la prorroga.

<a id="0028"></a>

## 0028 — Benchmarks anónimos entre clientes de Sytue Cloud

- **Fecha:** 2026-08-05
- **Estado:** accepted
- **Contexto:** ADR 0023 (Kapsule), ADR 0026 (Analytics), `services/cloud/internal/domain/benchmark.go`

**Contexto.** El motor de ajuste de tamaño (`RecommendInstanceSize`) decide con el histórico del
propio proyecto y exige tres días de observación para proponer una bajada. Eso deja un hueco justo
donde más dinero se pierde: los primeros días, cuando el cliente eligió tamaño a ciegas y casi
siempre se pasó. Durante esa ventana la única respuesta honesta que sabíamos dar era «aún estamos
midiendo».

El dato que llenaría el hueco existe, pero es de otros clientes: cientos de proyectos con el mismo
framework, los mismos servicios y el mismo tamaño ya nos dicen cuánto consume una app así. Usarlo
choca de frente con el invariante que sostiene la plataforma —la RLS existe precisamente para que
los datos de un inquilino no lleguen a otro—, de modo que no basta con «anonimizar»: hay que
decidir bajo qué condiciones se cruza esa línea y dejarlas verificables.

**Decisión.** Se autoriza la agregación entre inquilinos **solo** para publicar percentiles de
ocupación de memoria por cohorte, y bajo cinco condiciones que son parte de la decisión, no de la
implementación:

1. **Cohorte = framework + servicios + tamaño.** Nada de identificadores, nombres, dominios ni
   trazas de actividad. La clave describe la forma técnica de una app, no a quién pertenece.
2. **Umbral de k-anonimato de 5 EMPRESAS distintas**, contadas por inquilino y no por proyecto.
   Contar proyectos convertiría el umbral en un adorno: a un solo cliente le bastaría con crear
   cinco proyectos para «anonimizarse» a sí mismo.
3. **El umbral se aplica al PUBLICAR, no al consultar.** Una cohorte que no lo alcanza no se
   escribe, así que el camino de lectura no puede filtrarla mal porque no existe. La tabla
   `sizing_benchmarks` no tiene columna `tenant` —una fila no pertenece a nadie— y el umbral está
   replicado como `CHECK (tenants >= 5)`: cambiarlo obliga a tocar el código y la migración.
4. **Solo porcentajes sobre el límite del tamaño**, nunca cifras absolutas. Saber que la mediana de
   una cohorte es el 12 % no permite deducir el consumo de ninguna empresa concreta.
5. **Reemplazo completo en cada recálculo.** Una cohorte que hoy reúne cuatro empresas porque la
   quinta se dio de baja desaparece. Con `upsert` seguiría publicada para siempre con las cifras
   del día en que sí llegó al umbral.

Y dos límites sobre lo que la cohorte puede hacer con la recomendación:

- **El histórico propio siempre gana.** La cohorte solo rellena `insufficient_data`; en cuanto el
  proyecto tiene datos suyos, mandan los suyos.
- **La cohorte NUNCA propone subir, y baja un solo escalón.** Subir por el consumo de terceros es
  cobrarle más a alguien de quien no sabemos nada; y con cero datos propios, saltar de `pro` a
  `basic` de un clic es una apuesta. Un escalón se corrige solo al día siguiente y el ahorro llega
  igual. Además exige el mismo margen que el motor con datos propios: el p90 de la cohorte tiene
  que caber holgado en el tamaño menor.

La razón que ve la interfaz es `cohort_over_provisioned`, distinta de `over_provisioned` a
propósito: la pantalla no puede decir «hemos medido tu app» cuando lo que hemos medido son las de
otros, y el texto nombra cuántas empresas hay detrás para que el cliente pueda contrastarlo.

**Consecuencias.** Un proyecto recién creado recibe una recomendación de ahorro accionable el día
cero en vez de esperar tres días, que era el momento en que el sobredimensionado se convertía en
factura. A cambio, la plataforma mantiene una tabla global sin dueño: es la única de Sytue Cloud
que se lee sin predicado de inquilino, y por eso su política de escritura es de mantenimiento y su
contenido está acotado por `CHECK`. El recálculo recorre todos los inquilinos y es la consulta más
cara del reconciliador; va espaciada a seis horas y con ventana de siete días para que el fin de
semana no haga oscilar la recomendación.

**Alternativas descartadas.** Publicar el agregado sin umbral (una cohorte de dos clientes ES un
cliente); calcular la cohorte al vuelo en cada petición (obligaría a leer filas crudas de otros
inquilinos en el camino de lectura del usuario, que es exactamente lo que la RLS impide); pedir
consentimiento explícito para participar (el resultado sería una muestra sesgada hacia quien ya
está optimizado, es decir, un benchmark que empeora el consejo de quien más lo necesita).

<a id="0029"></a>

## 0029 — Apps open-source como servicio: catálogo cerrado, no imagen libre

- **Fecha:** 2026-08-05
- **Estado:** accepted
- **Contexto:** ADR 0022 (motores AGPL fuera), ADR 0023 (Kapsule), `internal/domain/appcatalog.go`

**Contexto.** La promesa de Sytue Cloud es componer: levantar una caché, un buscador o una cola
junto a tu app, de uno en uno, y que se conecten solos. La forma obvia de implementarlo —«dime el
nombre de una imagen y la arranco»— convierte el control-plane en un **ejecutor de código
arbitrario de terceros dentro del clúster**, y eso no se arregla después con políticas de red: el
contenedor ya corre, ya tiene las variables del proyecto y ya está dentro del namespace del
cliente.

**Decisión.** El cliente **elige del catálogo, no escribe una imagen**. El catálogo es código
—`services/cloud/internal/domain/apps/*.json`, embebido y revisado— y cada entrada cumple cuatro
invariantes que se comprueban en un test, no de palabra:

1. **Digest, nunca etiqueta.** `valkey:8` apunta hoy a un contenido y mañana a otro, así que lo
   que se revisó no es lo que corre. Un `@sha256:…` es el único identificador que no cambia bajo
   nuestros pies, y es lo que permite firmar y auditar.
2. **Nuestro registro.** Todas las imágenes se replican al registro de la UE. Depender de Docker
   Hub en el camino de aprovisionamiento es depender de su rate limit para que un cliente pueda
   crear un servicio.
3. **Licencia comprobada, por allowlist.** Apache-2.0, BSD-2/3, ISC, MIT, MPL-2.0 y
   PostgreSQL. AGPL quedaba fuera por lo mismo que en la [0022](#0022) —**revisado por la
   [0032](#0032)**, que la admite junto a GPL/LGPL para imágenes que corren sin modificar—; y
   también las «source available» que se anuncian como open source y no lo son —SSPL, BUSL,
   Elastic v2, Sustainable Use—, porque **prohíben exactamente esto**: ofrecerlas como servicio
   gestionado, y eso sigue igual. Es allowlist y no denylist porque esas licencias aparecen más
   rápido de lo que se actualizaría una lista de prohibidas.
4. **Sin privilegios posibles.** El manifiesto no tiene campo para `privileged`, `hostPath`,
   `hostNetwork` ni capacidades. No es que se validen: es que no se pueden expresar.

La integración entre servicios —lo de «se conectan aunque los crees de uno en uno»— la resuelve
`exports`: cada entrada declara qué variables inyecta en la app del cliente (`VALKEY_URL`,
`MEILI_URL`…). Su valor es una plantilla con **dos** sustituciones, `{host}` y `{port}`, y ninguna
más. Deliberadamente no es un lenguaje: ese valor acaba dentro de la app del cliente, y cada
capacidad que se añadiera —leer otro servicio, componer una credencial, mirar el entorno— sería
una vía nueva para que una entrada del catálogo filtre algo que no le corresponde.

Los límites de CPU, memoria y disco son **obligatorios** en cada entrada y tienen tope. Un
servicio sin techo se come la cuota del proyecto y tumba la app a la que venía a ayudar.

**Consecuencias.** No se puede desplegar «cualquier cosa de Docker Hub», y eso es una limitación
real frente a un Coolify o un Dokku. A cambio, todo lo que corre en el clúster está revisado,
fijado y auditado, y el cliente no tiene que entender licencias para no meterse en un problema
legal. Ampliar el catálogo es un pull request con una imagen replicada: barato, pero con revisión.

Una entrada escrita cuya imagen **aún no está replicada** se carga, se valida y se publica como
`pendiente de espejo` —no se ofrece y tampoco se esconde—: ofrecerla fallaría al tirar de la
imagen, después de haberle prometido el servicio al cliente; esconderla haría que nadie replicase
nunca la imagen porque nadie la echaría de menos.

**El registro no se escribe en el catálogo.** Cada entrada guarda el digest de nuestra copia
(`mirrored_digest`) y NADA del host: la referencia la compone `ImageRef()` con el `REGISTRY_URL` del
despliegue. Escribirlo en el manifiesto lo dejaba apuntando a un namespace que en producción se
llama distinto —el del PaaS es `sytue-<env>-paas`, no el defecto de dev—, y el fallo no aparecía
hasta que un cliente intentaba crear el servicio. Vacío = pendiente de espejo.

**El espejo lo hace CI, no un portátil.** `.sygit/workflows/mirror-catalog.yml` (y su gemelo de
GitHub) replica lo que falte y anota el digest en el manifiesto. Se dispara al cambiar el catálogo,
no con la release: añadir una app no tiene por qué esperar a la siguiente versión de la suite. La
razón de que no sea manual es concreta: empujar exige una credencial con
`ContainerRegistryFullAccess`, y la alternativa era repartir una clave de escritura de producción
entre portátiles para una tarea que se hace tres veces al año.

**El espejo es un comando, y verifica lo que escribe.** `scripts/mirror-catalog-images.sh` copia
con `crane` —nunca `docker pull` + `push`, que reconstruye el manifiesto, sube una sola
arquitectura y por tanto CAMBIA el digest—, comprueba que el digest se conservó y solo entonces lo
fija en el manifiesto. Así «el catálogo dice replicada» y «está replicada» son lo mismo. Cada
entrada guarda además `upstream` y `upstream_digest`, de modo que el espejo se puede auditar y
rehacer sin adivinar nada.

**Lo que la primera tanda enseñó, y por qué la lista salió más corta.** Al resolver las imágenes
de verdad —versiones, digests, usuario y volúmenes leídos del registro, no de la memoria— dos
candidatas se cayeron solas:

- **OpenSearch** exige `vm.max_map_count=262144`, un sysctl de NODO. PodSecurity `restricted` no
  permite pedirlo desde el pod, y ponerlo en todos los nodos por un servicio de un inquilino no es
  una decisión que le corresponda. Meilisearch cubre la búsqueda sin nada de eso.
- **Ghost** necesita MySQL para funcionar en producción, así que como servicio suelto no arranca:
  entraría cuando el catálogo sepa componer una entrada con su base de datos.

Y una tercera lección quedó en el manifiesto: cuatro de las cinco imágenes **no declaran usuario**
—corren como root— y bajo `restricted` no arrancarían. Por eso `run_as_user` es obligatorio, y por
eso su valor se lee del `/etc/passwd` de la imagen en vez de suponerlo: el de RabbitMQ es **100**,
no el 999 que la convención sugería. `writable_paths` nació de lo mismo: el arranque de RabbitMQ
genera su configuración en `/etc/rabbitmq`, y con la raíz de solo lectura moriría.

**Alternativas descartadas.** Imagen libre con escaneo previo (el escaneo no dice nada de lo que
el contenedor HACE en tiempo de ejecución); imagen libre en un namespace aislado sin acceso a los
servicios del proyecto (entonces no compone nada, que era el objetivo); catálogo con denylist de
licencias (llega siempre tarde).

---

<a id="0030"></a>

## 0030 — Firma digital: qué se promete bajo eIDAS y de dónde sale la clave

- **Fecha:** 2026-08-08
- **Estado:** accepted · **3 decisiones del propietario abiertas** (ver el final)
- **Contexto:** [`roadmap.md#3`](./roadmap.md) «Transversal — FALTA: firma digital»; ola **P3** de
  Sytue PDF («firma con certificado (PKI)», lo que separa la edición Pro de la básica);
  ADR [0001](#0001) (EU-only), [0017](#0017)/[0018](#0018) (clean-room, propietario),
  [0023](#0023) (sin motores de terceros), [0024](#0024) (apps de escritorio);
  `services/mail/internal/smime`, `platform/pkg/cms`, `platform/pkg/keycrypto`

**Contexto.** «Firma digital» es una casilla en el roadmap, y debajo hay **tres productos
distintos con consecuencias legales distintas**. Confundirlos es el fallo caro: no cuesta nada
poner un botón que diga «firmar», y cuesta mucho que un cliente lo use creyendo que tiene el valor
de una firma manuscrita y descubra en un juicio que no. El producto es EU-only, así que quien
decide qué vale es **eIDAS** (Reglamento UE 910/2014, revisado por el 2024/1183), y el nivel que
se promete cambia el diseño entero: la firma, el almacenamiento y hasta quién puede tener la clave.

Además, esto llega a un repo donde **ya se firma**: `services/mail/internal/smime` construye un
CMS SignedData a mano con `encoding/asn1` desde agosto, y se verifica en sus tests con `openssl`.
Así que la pregunta «¿se puede sin dependencias?» no es abierta para la mitad del problema — ya
está contestada por código que funciona.

### 1. Qué se promete: firma **avanzada** (AdES), y se dice con esas palabras

eIDAS define tres niveles, y solo el tercero equivale a la firma manuscrita:

| Nivel                 | Qué exige                                                                                                                                                                                                | Dónde está en el producto                 |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| **Simple (SES)**      | Nada más que ser electrónica. Art. 25.1: no se le puede negar efecto jurídico _por ser_ electrónica, pero quien la invoca carga con probar que es del firmante                                           | La firma **dibujada** ya prevista en P3   |
| **Avanzada (AdES)**   | Art. 26: ligada unívocamente al firmante, permite identificarlo, creada con datos que el firmante usa **bajo su control exclusivo**, y detecta cualquier cambio posterior del documento                  | **Lo que promete Sytue**                  |
| **Cualificada (QES)** | AdES + **certificado cualificado** de un prestador de la lista de confianza de la UE + **QSCD** (dispositivo cualificado de creación). Art. 25.2: efecto equivalente a la firma manuscrita en toda la UE | **No se promete**: no depende de nosotros |

**Decisión: se promete AdES**, y la interfaz **nombra el nivel en cada firma**. QES queda fuera
del alcance hasta que haya contrato con un prestador cualificado, porque no es una cuestión de
código: exige un tercero acreditado y auditado, y no hay forma de fabricarla en casa.

De aquí sale la regla dura, que aplica a la UI y al material comercial: **nunca se dice «firma
legal» ni «firma válida» a secas.** Se dice qué nivel es, quién emitió el certificado, y contra
qué anclas se comprobó. Un «✓ Firmado» verde sobre una firma que el receptor no puede verificar es
una mentira con estilo.

### 2. De dónde sale la clave: **no la custodiamos**

Tres orígenes posibles, y el orden importa porque el primero es el que desbloquea P3:

- **(a) Certificado del propio usuario, en tránsito y nunca en reposo.** Sube su PKCS#12 en el
  momento de firmar, la clave vive en memoria durante la operación y se destruye al terminar. No
  hay columna, no hay copia de seguridad, no hay rotación: no hay nada que rotar.
- **(b) Prestador cualificado remoto (QTSP) con firma en la nube.** La clave nace y muere dentro
  del QSCD del prestador; nosotros enviamos el resumen y recibimos la firma. Es el **único** camino
  a QES y el único que además no nos convierte en custodios de nada.
- **(c) Tarjeta o eID nacional (DNIe y equivalentes).** Exige middleware PKCS#11 en la máquina del
  usuario: el navegador no llega. Su hogar natural es la **app de escritorio Fyne** de
  [ADR-0024](#0024), no la web. Queda anotado, no planificado.

**Y lo que NO se hace, que es la parte importante: Sytue no guarda la clave privada de firma de
una persona.** Ni siquiera cifrada con `platform/pkg/keycrypto`, que técnicamente podríamos hacer
esta tarde. Tres razones, dichas en voz alta:

1. **Rompe el art. 26.c.** «Control exclusivo» significa que el firmante, y solo él, puede firmar.
   Si nosotros tenemos la clave, podemos firmar sin él; y esa es literalmente la primera pregunta
   que hace un perito cuando alguien repudia una firma. Una AdES custodiada por el proveedor es
   más débil que una AdES de verdad, y la diferencia no se arregla con un aviso legal.
2. **Es el objetivo más valioso que tendría la plataforma.** Un almacén con las claves de firma de
   miles de organizaciones no se remedia rotando un secreto cuando se filtra: se remedia
   **repudiando firmas ya emitidas**, hacia atrás, y sin saber cuáles.
3. **Guardar la clave privada de alguien es una responsabilidad, no una funcionalidad.** Se
   presenta siempre como comodidad («firma con un clic, sin subir nada cada vez») y esa comodidad
   es real; lo que se está comprando con ella es un riesgo que no se puede deshacer.

> **Por qué la CA de S/MIME no es precedente.** `services/mail/internal/smime/ca.go` sí custodia
> claves, y está bien: son claves que **emitimos nosotros** para una identidad de buzón de la
> organización, con nuestra propia ancla, y jamás se presentan como firma avanzada de una persona
> física. Allí la alternativa era _no firmar el correo en absoluto_. Aquí la alternativa es una
> firma que el receptor rechaza, que es peor que ninguna.

### 3. Cero dependencias nuevas: qué sale con la stdlib y qué no

Determinado pieza a pieza, no de memoria:

| Pieza                                    | ¿Solo stdlib?                 | Evidencia                                                                                                                                    |
| ---------------------------------------- | ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| **Empaquetado CMS/PKCS#7 SignedData**    | **Sí**                        | `platform/pkg/cms` (entregado con este ADR): `encoding/asn1` + `crypto/x509`. Cruzado con `openssl cms` en **las dos direcciones**           |
| **Verificación de una firma ajena**      | **Sí**                        | ídem: resumen, atributos firmados, `signing-certificate-v2` y cadena                                                                         |
| Cadena, caducidad y usos del certificado | **Sí**                        | `x509.Certificate.Verify`                                                                                                                    |
| Revocación por CRL                       | **Sí**                        | `x509.ParseRevocationList`                                                                                                                   |
| Revocación por OCSP                      | **Sí, sin dependencia nueva** | `golang.org/x/crypto/ocsp`, ya directa en `go.mod`                                                                                           |
| Petición y validación de sello RFC 3161  | **Sí (el código)**            | ASN.1 sobre HTTP; el token **es** un CMS SignedData que `platform/pkg/cms` ya verifica. Lo que falta no es código, es el TSA — §4            |
| Incrustar la firma en el PDF (PAdES)     | **Sí**                        | actualización incremental + `/ByteRange`; el motor de `services/docs` ya lee (`pdfread.go`) y escribe (`export_pdf.go`) PDF sin dependencias |
| **Leer un PKCS#12 real**                 | **NO**                        | ver abajo                                                                                                                                    |
| **Firma cualificada (QES)**              | **NO, y no por código**       | exige QTSP + QSCD                                                                                                                            |

**El único hueco técnico es PKCS#12, y no es el que parece.** La respuesta cómoda —«ya está,
`golang.org/x/crypto/pkcs12` viene con una dependencia que tenemos»— es **falsa**, y se comprobó
en vez de suponerse. Ese paquete está **congelado** (su propia documentación dice que no acepta
funcionalidad nueva y remite a otro) y solo entiende el cifrado de los años noventa:

- Un `.p12` exportado por **OpenSSL 3 con sus valores por defecto** falla con
  `pkcs12: unknown digest algorithm: 2.16.840.1.101.3.4.2.1` — ni siquiera acepta que el MAC sea
  SHA-256, y menos aún PBES2/AES.
- Y uno generado con `-legacy` **pero con la cadena del emisor dentro** (que es lo que entrega
  cualquier CA) falla con `pkcs12: expected exactly two safe bags in the PFX PDU`.

Es decir: el fichero que un usuario real tiene hoy en el escritorio no se lee. Salidas posibles,
las tres honestas:

1. **`software.sslmate.com/src/go-pkcs12`** (BSD-3, permisiva, dentro de la allowlist de
   `security.md §5`). Es la que recomienda la propia documentación de la stdlib congelada.
2. **Escribirlo en casa.** PBES2 es **PBKDF2 + AES-CBC**, y las dos están en la stdlib
   (`crypto/pbkdf2` desde Go 1.24, `crypto/aes`): no sería inventar criptografía, sería parsear un
   sobre. Pero son varios cientos de líneas de ASN.1 sobre un formato hostil, y toda esa superficie
   de parseo la alimenta **un fichero que sube un desconocido**.
3. **No leer PKCS#12 nunca**: el origen de clave es (b) o (c) de §2, y se depende del QTSP antes.

**Recomendación: la 1**, acotada a una función de importación aislada. Pero la decisión de
dependencia es del propietario, no de quien escribe el ADR — y **hasta que se tome, P3 avanza
entero salvo la importación de certificado propio**: verificar, empaquetar, incrustar y firmar
desde (b) o (c) no dependen de esto.

### 4. Sello de tiempo: sin él la firma caduca con el certificado

Una firma se puede verificar mientras el certificado esté vigente y se pueda comprobar que no
estaba revocado. El día que el certificado caduca —dos o tres años— un verificador ya **no puede
distinguir «se firmó cuando el certificado valía» de «se firmó después»**, y la firma deja de
sostener nada. Un contrato firmado hoy tiene que seguir siendo verificable en 2040.

Lo que lo arregla son dos cosas, y ninguna es opcional a largo plazo:

- **Sello de tiempo RFC 3161** de un tercero, que es quien fija el _cuándo_ de forma que no dependa
  del reloj del firmante ni del nuestro.
- **Datos de validación incrustados** (OCSP/CRL dentro del documento, el DSS de PAdES): permiten
  comprobar la revocación dentro de diez años, cuando el servidor OCSP del emisor lleve tiempo
  apagado.

**Perfil objetivo: PAdES-B-LT** (firma + sello + datos de validación). PAdES-B-LTA —el sello de
archivo que se re-sella periódicamente— queda para cuando existan documentos que de verdad vivan
décadas; añadirlo después es una capa más, no una reescritura.

**El cliente RFC 3161 está ENTREGADO** (2026-08-14, `services/docs/internal/domain/pdftimestamp.go`):
petición, validación del `PKIStatus`, nonce contra reenvíos, `certReq` para que el token se sostenga
solo dentro del fichero, y el token colgado de la firma como atributo **no firmado** —el único sitio
donde cabe, porque el sello se calcula SOBRE la firma y no existe hasta que la firma está hecha—.
El chart pasa `SYTUE_DOCS_SELLO_URL` siempre y la credencial va por Secret.

> **Revisado por el [0051](#0051) (2026-09-26):** la autoridad es ahora **Izenpe**, cualificada
> (Certum gratuito no lo es), el token se autentica contra su raíz, y en la firma en dos pasos un
> fallo de la autoridad NO firma sin sello a escondidas: se pregunta. El origen de clave (c) de §2
> —tarjeta, DNIe y los almacenes del sistema— lo desarrolla el mismo ADR: el servicio de los dos
> pasos está entregado y los almacenes de cada plataforma llegan uno a uno.

**El TSA elegido (2026-08-14): CERTUM**, `http://time.certum.pl` — Asseco Data Systems S.A.,
Polonia. Es el **único servicio de sellado gratuito operado por un prestador que está en la lista de
confianza de la UE**, así que cumple el invariante de soberanía ([ADR-0001](#0001)) sin coste y sin
contrato, y su raíz está en los almacenes habituales: un validador muestra un sello reconocido y no
un «emisor desconocido». Verificado contra el servicio real, y también contra Izenpe
(`tsa.izenpe.com`) y QuoVadis/DigiCert Europe (`ts.quovadisglobal.com/eu`), que quedan como
alternativas — la primera es sector público vasco y su servicio libre no está pensado para uso
comercial; la segunda tiene matriz estadounidense, que es lo que la descarta frente a Certum.

**La letra pequeña, y no es menor: el servicio gratuito NO emite sellos CUALIFICADOS.** Los
gratuitos nunca lo son — un sello cualificado (eIDAS art. 42) es un servicio de pago. Lo que se gana
igualmente es lo TÉCNICO, que es la mayor parte del problema: la firma sigue verificándose cuando el
certificado caduque. Lo que NO se gana es la presunción legal de exactitud de la fecha (art. 41.2);
sigue siendo prueba admisible, pero sin esa presunción. **Subir a cualificado es cambiar la URL por
la del servicio de pago del MISMO proveedor: ni una línea de código.** Eso convierte lo que era una
decisión bloqueante en una decisión de cuándo.

**Y una consecuencia operativa que hubo que resolver: las TSA hablan HTTP PLANO.** Todas — ninguna
de las tres ofrece HTTPS, y es defendible, porque de ida viaja un hash y de vuelta algo firmado por
la autoridad. Lo que sí puede hacer un intermediario es **sustituir el token por otro auténtico de
OTRA huella**, y contra eso está `comprobarToken`: se valida que el TSTInfo del token sella la
huella pedida y lleva el nonce de esta petición. Sin esa comprobación el PDF se guardaría con un
sello que no cubre su firma, sin dar ningún error, y se descubriría el día en que hiciera falta la
prueba. El chart abre el puerto 80 solo para esto, con el motivo escrito al lado.

**Y una decisión de diseño que importa más de lo que parece: un fallo del TSA NO impide firmar.** Si
la autoridad está caída o tarda más del plazo, la firma se emite SIN sello. Lo contrario convertiría
un tercero en un punto único de fallo del producto: nadie podría firmar un contrato porque un
servidor ajeno no responde. Tiene su propio check.

**La regla mientras no haya TSA** —y esto es lo que evita el problema del «✓ verde»—: una firma sin
sello se emite, se guarda y **se muestra diciendo que no lo tiene**: «sin sello de tiempo:
verificable hasta _(fecha de caducidad del certificado)_». No se pinta un reloj que no existe. En
la misma línea, `signing-time` es **lo que declara el firmante**, no una prueba, y así está
comentado en el código que lo lee.

### 5. Dónde vive cada cosa, y por qué así no bloquea nada

- **`platform/pkg/cms`** (nuevo, entregado con este ADR) es el envoltorio común: una firma S/MIME,
  una firma dentro de un PDF y un sello RFC 3161 son **los tres** un CMS SignedData. Lo que cambia
  entre ellos es dónde se pega el resultado, no qué se firma.
- **La incrustación PAdES va en `services/docs`**, que es donde está el motor de PDF.
- **El origen de la clave se enchufa como `crypto.Signer`** —la interfaz de la stdlib, no una
  nuestra—, y esa es la pieza que hace que §2 no bloquee a §3: una clave efímera en memoria, un
  HSM por PKCS#11 y un QTSP remoto son los tres un `crypto.Signer`. Se decide después sin
  reescribir nada.
- `services/mail/internal/smime` conserva su CMS propio hasta que migre, con el mismo precedente
  que `platform/pkg/keycrypto` frente a `services/id`. No se toca desde aquí.

Una consecuencia de diseño que merece nombre propio: **`cms.Verify` se niega a verificar si no le
dicen contra qué anclas confía** (`ErrSinAnclas`). El almacén de raíces del sistema **no** es la
lista de confianza de la UE, y dejar que la función caiga en él sería tomar la decisión de
confianza en silencio, con lo que trajera la imagen del contenedor.

**Consecuencias.**

- **La verificación deja de estar bloqueada hoy.** Abrir un PDF firmado por un cliente y decir
  quién lo firmó, con qué certificado y contra qué ancla, no depende de ninguna de las tres
  decisiones abiertas. Es la mitad del valor de P3 y ya está.
- P3 puede entregar la firma dibujada (SES) y la verificación de firmas ajenas de inmediato, y
  AdES en cuanto haya un origen de clave.
- **No se anuncia QES en ningún material comercial** hasta que exista contrato con un QTSP.
- Sytue **no ofrece «guardar mi certificado»** como funcionalidad. Si un cliente lo pide, la
  respuesta es (b), no una casilla.

**Alternativas descartadas.**

- **Custodiar la clave del usuario cifrada con `keycrypto`.** Era lo cómodo y lo barato: firma con
  un clic, sin subir nada cada vez, y la maquinaria de cifrado en reposo ya existe. Rompe el
  control exclusivo del art. 26.c, convierte la plataforma en el objetivo más valioso que tendría,
  y su fallo no se remedia rotando un secreto sino repudiando firmas ya emitidas.
- **Emitir nosotros los certificados de firma con una CA propia**, como sí se hace en S/MIME.
  Descartada para documentos: una CA nuestra no está en ninguna lista de confianza, así que la
  firma solo valdría dentro de casa — y lo que P3 vende es exactamente que valga **fuera**.
- **Traer una librería de PDF que ya firme** (PDFBox, iText, PDF.js…). AGPL o licencia comercial,
  contra [0017](#0017) y [0022](#0022), y contra el hecho de que el motor propio ya lee y escribe.
- **Implementar PBES2 dentro de `platform/pkg/cms` sin preguntar**, para tapar el hueco de PKCS#12
  y poder enseñar la función terminada. Descartada por el invariante: la decisión de dependencia
  es del propietario, y esconderla dentro de otro paquete es justamente la manera de que nadie la
  revise. **El hueco se documenta, no se rellena.**
- **Revender un servicio de firma de terceros no cualificado** de los que se anuncian como «firma
  legal» sin estar en la lista de confianza. Descartada: es exactamente la confusión que este ADR
  existe para no cometer, y con nuestra marca encima.
- **Esperar a tener QTSP para empezar.** Descartada: el empaquetado y la verificación no dependen
  de esa decisión, y dejarlos parados habría mantenido «firma digital — FALTA» otro trimestre por
  una razón que no era técnica.

**Lo que tiene que decidir el propietario.**

1. **PKCS#12** — (i) añadir `software.sslmate.com/src/go-pkcs12` (BSD-3), (ii) escribir el parseo
   de PBES2 en casa sobre `crypto/pbkdf2` + `crypto/aes`, o (iii) no leer P12 nunca y depender del
   QTSP. **Recomendación: (i).** Bloquea solo la importación de certificado propio.
2. **Prestador cualificado (QTSP/TSA)** — cuál, de la lista de confianza de la UE y alojado en la
   UE, para sello de tiempo; y si además se quiere QES, para firma remota. Contrato y coste por
   sello.
3. **Lista de confianza** — si se ingiere la LOTL europea (XML firmado, con su mantenimiento) o
   cada despliegue declara sus anclas. Determina qué se le puede decir a un usuario que abre un
   PDF firmado fuera de Sytue.

---

<a id="0031"></a>

## 0031 — Crossplane adopta la infraestructura por riesgo; RDB y Kapsule se quedan fuera

`accepted` · 2026-08-10 · infra, iac, crossplane

**Contexto.** [0014](#0014) puso la infraestructura de producción en Terraform/OpenTofu.
Funciona, pero el estado vive en un bucket S3 y el despliegue depende de él: `02b-secretos.sh`
y `03-desplegar.sh` leen **8 valores** con `tofu output` (el endpoint del RDB, los dos
endpoints del Registry, la IP de ingress, la IP de medios de LiveKit, el CIDR privado, el
endpoint S3 y el kubeconfig). Sin credencial del bucket de estado no se puede desplegar —de
ahí el modo de recuperación `SYTUE_INFRA_FROM_ENV=1`—, y un `tofu init` en `environments/prod`
tarda más de 30 minutos porque arrastra un provider de AWS de 1,4 GB que ningún recurso usa.

Crossplane pone la reconciliación dentro del clúster y elimina el estado externo. La pregunta
no era si migrar, sino **qué** migrar y **en qué orden**, con todos los recursos ya sirviendo
tráfico.

**Decisión.**

1. **Se genera un provider propio con Upjet** a partir del provider de Terraform de Scaleway
   (2.80.0, el mismo que fija el lock de OpenTofu), en modo **no-fork**: los recursos se
   reconcilian con el esquema Go dentro del proceso, sin ejecutar Terraform ni empaquetar su
   binario. Así **no entra BUSL en la cadena de suministro**.
2. **Los recursos se generan en orden de riesgo si se recrean**, no de comodidad:
   DNS → Object Storage y Registry → red/IP/máquinas/secretos. Van 5 de 22 tipos.
3. **RDB (`scaleway_rdb_*`) y Kapsule (`scaleway_k8s_*`) se quedan fuera**, y no como «para
   más adelante»: como decisión. Crossplane corre **en** Kapsule, así que gestionar el clúster
   desde dentro es circular —un fallo de reconciliación puede dejar el pool sin nodos y el
   controlador que lo arreglaría está en esos nodos— y el `kapsule_kubeconfig` es justamente
   la credencial para llegar allí. Y `scaleway_rdb_instance` no es un recurso que «se recree»:
   se lleva los datos. No hay beneficio operativo que compense ese riesgo.
4. **La adopción es siempre en dos tiempos**: `managementPolicies: ["Observe"]` primero,
   comprobar que `spec.forProvider` coincide con `status.atProvider`, `tofu state rm` después,
   y solo entonces conceder `Create`/`Update`. **`Delete` no se concede** a los buckets con
   datos de cliente. El procedimiento completo, con qué mirar antes y después de cada paso,
   está en [`infra/crossplane/ADOPCION.md`](../infra/crossplane/ADOPCION.md).
5. **Los `output` se sustituyen por un ConfigMap** `sytue-infra-salidas` que publica
   `infra/crossplane/publicar-salidas.sh` leyendo los `status.atProvider`, **con los mismos
   nombres de clave** que los outputs de OpenTofu para que migrar un consumidor sea cambiar de
   dónde lee, no qué formato lee.

**Alternativas descartadas.**

- **Migrarlo todo de golpe.** El provider de Scaleway tiene cientos de recursos; generarlos
  todos da un módulo que nadie revisa, y adoptar 55 bloques a la vez hace que el primer error
  de external-name se descubra cuando ya hay varios recursos recreados.
- **Usar el provider de Crossplane de Scaleway de un tercero.** No hay uno mantenido con la
  cobertura que hace falta, y un provider de terceros en la ruta de aprovisionamiento es un
  actor más con credenciales de administración del proyecto.
- **Modo CLI de Upjet** (cada reconciliación lanza un binario de Terraform). Mete Terraform y
  el provider de Scaleway **binarios** en la imagen, con BUSL detrás, para hacer lo mismo.
- **Instalar `crossplane-contrib/provider-kubernetes`** y publicar las salidas con un `Object`
  y `spec.references[].patchesFrom`, que las mantendría al día solo. Es la solución de manual,
  pero cuesta otro provider en el clúster y en la cadena de suministro para refrescar valores
  que solo se leen al desplegar. Se reconsidera el día que algo dentro del clúster los
  necesite en caliente.
- **Adoptar también el bucket `tfstate`.** Es el estado del propio OpenTofu: mientras OpenTofu
  gestione algo, la dependencia es circular.

**Consecuencias.**

- **OpenTofu no desaparece.** Aunque se adoptara todo lo adoptable, `environments/prod` seguiría
  existiendo para RDB y Kapsule, y el despliegue seguiría sacando el kubeconfig de su estado.
  El objetivo no es «Crossplane sustituye a OpenTofu», es **«OpenTofu solo toca lo que no se
  puede recrear»**.
- **Se pierde el `plan`.** OpenTofu enseña lo que va a pasar antes de que pase; Crossplane no.
  La fase Observe es el sustituto, y es manual: no escala a decenas de objetos. Es el coste
  reconocido de esta decisión.
- **Dos definiciones de la misma infraestructura mientras dure la migración**, con el mismo
  riesgo de divergencia que ya señalaba [0014](#0014) para Compose y Helm. Se acota con la
  regla de que un recurso sale del `.tf` (bloque `removed` con `destroy = false`) en el mismo
  cambio en el que entra en Crossplane.
- **Hoy Crossplane no gestiona nada en producción.** Lo que hay es el pipeline de generación,
  cinco tipos de recurso, manifiestos en Observe y el procedimiento. Nada aplicado.

## 0032 — Crossplane al 100 % no es viable, y al máximo posible sale caro en cadena de suministro

**Fecha**: 2026-08-10 · **Estado**: aceptada · **Sustituye parcialmente a**: 0031

### El contexto que faltaba

El ADR 0031 decidió adoptar la infraestructura por riesgo y dejar RDB y Kapsule
fuera. Esa decisión sigue en pie, pero se tomó sin medir lo que Crossplane AÑADE
al stack. Con el criterio de mantener la superficie de suministro lo más pequeña
posible, ese número manda sobre los demás.

### Lo medido

**Crossplane con Upjet no sustituye a Terraform: lo empaqueta.** El provider
generado declara `github.com/scaleway/terraform-provider-scaleway/v2 v2.80.0` y
`github.com/hashicorp/terraform-plugin-sdk/v2` como dependencias DIRECTAS. Lo que
hoy es un binario que se ejecuta y termina pasaría a ser una biblioteca que corre
dentro de un pod, siempre encendido, con permisos sobre toda la cuenta de Scaleway.

|                                      | Módulos en el árbol |
| ------------------------------------ | ------------------- |
| Toda la suite Sytue (doce servicios) | 526                 |
| **El provider de Crossplane, solo**  | **483**             |
| De ellos, NUEVOS en nuestro stack    | **286**             |

Duplicar la superficie de suministro auditada para gestionar diecinueve recursos
de infraestructura no es una mejora de mantenimiento, es lo contrario. Y los 286
nuevos no son código de nicho: 29 módulos de HashiCorp, 21 de go-openapi, **14 de
AWS** —en una infraestructura que no tiene un solo recurso de AWS—, 7 de sigstore
y 5 de etcd.

El binario resultante son 99 MB. `tofu` es un binario que no vive en el clúster y
que solo descarga providers cuando se ejecuta.

**Y hay un coste permanente**: Crossplane exige su núcleo (`crossplane` +
`rbac-manager`) y el pod del provider corriendo siempre, más ~3 CRDs por tipo de
recurso —15 con cinco tipos, así que los 22 serían unos 66—. En un clúster de dos
nodos con un presupuesto de 100 EUR/mes, eso son pods y memoria que hoy no se
gastan.

### La decisión

**No se persigue el 100 %, ni el máximo posible.** Crossplane se queda como está:
DNS, Object Storage y Registry generados, con el procedimiento de adopción escrito
(`infra/crossplane/ADOPCION.md`) y sin instalar en producción, disponible el día
que haya un motivo que supere a este coste.

**OpenTofu sigue siendo la herramienta de infraestructura.** Es un binario, sin
nada corriendo, con tres providers fijados por lockfile y verificados por hash.

### Lo que sí hay que hacer, y es la mitad del beneficio por una centésima del coste

El motivo original para migrar era que `tofu init` tarda **más de 30 minutos** y
bloquea todo despliegue. Eso NO lo causa OpenTofu: lo causa un provider de AWS de
1,4 GB declarado en `environments/prod/providers.tf` para leer del estado módulos
ya retirados, **sin un solo recurso `aws_*` en el repo**. La propia nota del
fichero dice que debía eliminarse «tras ese apply y comprobar `tofu providers`».
Nadie lo hizo.

Quitarlo, junto con la caché de plugins compartida, recupera casi todo el tiempo
perdido. Es una tarde de trabajo contra las semanas de una migración que además
duplicaría la superficie de suministro.

**Hecho el mismo día, y medido:**

|                                  | Antes   | Después   |
| -------------------------------- | ------- | --------- |
| `tofu init` en limpio            | >30 min | **2,7 s** |
| Providers en el lockfile de prod | 3       | 1         |
| `.terraform` en disco            | 3,7 GB  | 204 MB    |

El estado remoto ya no los referencía —de 37 recursos con 3 de `aws` y 2 de
`random` (serial 27) a 34, todos de Scaleway (serial 50)—, así que la declaración
solo servía para descargar. `tofu validate` pasa con el árbol podado. Los otros
tres entornos (dev, staging, sytue-git) nunca tuvieron el lastre.

Dos cosas se añadieron para que no vuelva:

- **Un gate**, en `deploy/production/tests/deriva-infra-test.sh`: falla si el
  lockfile de cualquier entorno fija un provider sin un solo recurso suyo en el
  código. Mira el LOCKFILE y no solo el `.tf`, porque es el que manda en lo que se
  descarga y una entrada puede sobrevivir ahí a que se limpie la declaración.
- **Caché de plugins compartida** (`TF_PLUGIN_CACHE_DIR`) en
  `lib-tofu-backend.sh`. El aislamiento por `TF_DATA_DIR` se mantiene —es
  correcto y evita que una caché histórica reclame providers retirados— pero el
  binario se guarda una sola vez en vez de una por directorio.

Al escribir ese gate salió una trampa que no tenía nada que ver y vale para todo
el repo: `«$var»` **mata el script**. El `»` son los bytes `C2 BB`, bajo locale C
bash se los traga como parte del nombre de la variable, y con `set -u` eso es un
`unbound variable`. Lo peor es dónde vive: en las ramas de error, o sea que solo
explota cuando el gate acaba de detectar algo y se lleva el diagnóstico por
delante. Había otro caso igual en `runner-flota-test.sh`. **Variable pegada a
« »: llaves siempre.**

### Cuándo reconsiderarlo

- Si la infraestructura pasa a definirse por API en vez de por ficheros —clientes
  que crean recursos a demanda—, que es el caso de uso para el que Crossplane
  existe y que hoy no tenemos.
- Si Scaleway publica un provider NATIVO de Crossplane, sin Upjet ni el SDK de
  Terraform dentro. Ahí desaparece la mayor parte de los 286 módulos.
- Si OpenTofu deja de mantenerse o cambia de licencia.

Ninguna de las tres depende de nosotros, y por eso el trabajo hecho se conserva en
vez de borrarse: el día que ocurra, el pipeline ya funciona.

<a id="0032"></a>

## 0032 — Productos open-source en Sytue Cloud: copyleft sí, «source available» no; y quién se publica

- **Fecha:** 2026-08-12
- **Estado:** accepted
- **Contexto:** ADR [0022](#0022) (motores AGPL fuera de la suite), ADR [0029](#0029) (catálogo
  cerrado), `services/cloud/internal/domain/appcatalog.go`

**Contexto.** El catálogo de la 0029 nació para levantar piezas de APOYO junto a la app del
cliente: una caché, un buscador, una cola. La siguiente vuelta del producto pide otra cosa —elegir
VARIOS productos completos, tipo Nextcloud o un CMS, y que los que se integran entre sí arranquen ya
conectados—, y ahí la 0029 chocaba consigo misma en dos puntos:

- Su allowlist era solo permisiva, y **prácticamente todo el software libre de producto es
  GPL o AGPL**: Nextcloud, WordPress, Grafana, Metabase, Vaultwarden, Mastodon. Con la lista tal
  cual, el catálogo no podía salir nunca de la infraestructura.
- Ninguna entrada tenía Ingress, a propósito. Correcto para una caché; para un CMS significa
  desplegarlo y no poder abrirlo.

**Decisión 1 — entran AGPL, GPL y LGPL; siguen fuera las «source available».**

La [0022](#0022) retira los motores AGPL de DENTRO de la suite, y esa decisión no se toca: allí el
código propietario de Sytue se integra con ellos, y esa combinación es lo que el copyleft alcanza.
Aquí el caso es otro: la imagen corre **tal cual, sin modificar**, en el namespace del cliente, sin
que nada nuestro se enlace con ella. Eso es agregación, no obra derivada.

Y «sin modificar» no es una promesa de intenciones: es lo que ya comprobaba `validateAppMirror`
—`mirrored_digest` tiene que ser **idéntico** a `upstream_digest`—. Una copia con un byte cambiado
no cumple esa igualdad y no entra en el catálogo, así que la premisa de la que depende todo lo
anterior la verifica una máquina en cada carga.

La obligación que sí queda —AGPL §13 y GPL §6: quien usa el servicio puede obtener el fuente— se
cumple con un campo nuevo, `source`, **obligatorio** cuando la licencia es copyleft y que la interfaz
enseña al lado de la licencia. Es obligatorio en la validación porque la alternativa era confiar en
que quien añade una entrada AGPL se acuerde.

Las «source available» (SSPL, BUSL, Elastic v2, Sustainable Use) siguen fuera y por el motivo
contrario: no es que su copyleft nos alcance, es que **prohíben expresamente ofrecerlas como
servicio gestionado**.

**Decisión 2 — publicar en internet lo decide el catálogo (`web_port`), nunca el cliente.**

Una entrada declara por qué puerto se publica, o no lo declara y entonces solo se alcanza desde
dentro del proyecto. Con `web_port`, el aprovisionador crea Ingress + TLS de cert-manager en
`<app>-<proyecto>.<dominio de apps>`.

Un interruptor «publicar» en el panel habría sido más flexible y es lo que hace un Coolify. También
habría puesto el panel de administración de RabbitMQ en internet —con la contraseña que la
plataforma acaba de generar— el día que alguien marcara la casilla sin saber que el 15672 es un
panel. Se revisa una vez por entrada, al escribirla, y ya no hay clic que equivocarse.

Hizo falta además una NetworkPolicy propia (`allow-published-apps`): la que había abre desde Traefik
**solo** los puertos 3000 y 8080, así que un Nextcloud escuchando en el 80 habría tenido Ingress,
certificado emitido y timeout, sin nada en los logs que apuntara a la red.

**Decisión 3 — las apps se integran entre sí, y funciona en cualquier orden.**

Una entrada declara `integrations`: con qué OTRA app del catálogo se conecta, **la URL de la
documentación oficial que lo describe** y qué variables recibe. `docs` es obligatorio y es lo que
separa una integración de una conjetura: sin ese ancla, «Sytue las conecta solas» acabaría siendo
Sytue inventando la configuración de un proyecto ajeno, y el fallo aparecería al arrancar un
contenedor en casa de un cliente.

El aprovisionador resuelve la integración MIRANDO qué hay levantado en el proyecto (etiquetas
`sytue.eu/app-id` + `sytue.eu/project-id`), no una relación guardada. El filtro por proyecto no es
opcional: el namespace es del TENANT y dentro caben varios proyectos, así que sin él la app de uno
se conectaría a la caché de otro y ninguna de las dos partes se enteraría.

Y el orden no importa, que es la parte que costó: `reconcileServices` **solo** aprovisiona lo que
está en `provisioning`, o sea que un servicio `ready` no se vuelve a mirar jamás. Sin nada más,
crear la caché DESPUÉS del CMS no le habría llegado nunca al CMS —sin error, sin aviso, sin
conexión—. Por eso dar de alta o borrar una app **reencola** las hermanas que se integran con ella
(`ReprovisionServices`), y por eso solo se admite **una instancia de cada app por proyecto**: con
dos, «la caché del proyecto» dejaría de ser una y la integración elegiría por orden de listado.

**Consecuencias.** El catálogo puede contener productos completos, y el cliente ve la licencia, el
fuente y si tendrá URL antes de crear nada. A cambio: cada entrada nueva exige comprobar que su
imagen arranca **no-root y bajo PodSecurity `restricted`**, y ahí se caen bastantes candidatas
—la imagen oficial de Nextcloud arranca como root para ajustar permisos, así que entrará por otra
imagen o no entrará—. Añadir una app sigue siendo un pull request con revisión, y ahora también con
una comprobación de UID y de puerto.

---

<a id="0033"></a>

## 0033 — Sytue PDF se dibuja en SVG, no se rasteriza

`accepted` · 2026-08-13 · pdf, docs, frontend

**Contexto.** Sytue PDF necesita un lienzo: sin ver el documento no se puede anotar, ni
rellenar, ni firmar donde toca. La forma evidente de conseguirlo es rasterizar cada página a
imagen, que es lo que hacen casi todos los visores. Con nuestros invariantes eso significa
escribir un motor de gráficos entero —curvas, tramas, antialiasing, composición y, sobre
todo, interpretar cada formato de fuente incrustada para dibujar sus contornos— porque no se
puede meter PDF.js (AGPL), ni wkhtmltopdf, ni un binario externo.

**Decisión.** Traducir el flujo de contenido del PDF a **SVG** en el servidor y dejar que
rasterice el motor del navegador. La correspondencia es casi uno a uno: `m/l/c/re` son un
`<path>`, `cm` es un `transform` —la matriz `[a b c d e f]` de PDF y `matrix(a b c d e f)`
de SVG son literalmente la misma función—, `Do` sobre una imagen es un `<image>`, y `sh` es
un `<linearGradient>`. El SVG se inserta en el DOM, no en un `<img>`.

Sale gratis lo que una imagen no da:

- **el texto es TEXTO**: se selecciona, se copia y lo lee un lector de pantalla. Resaltar es
  tomar los rectángulos de la selección del navegador, que es lo que hace un visor de
  escritorio y que sobre un mapa de bits habría que reinventar peor;
- **el zoom no pierde calidad** ni obliga a volver a pedir la página;
- **una sola representación vectorial sirve de referencia** para las seis plataformas de
  ADR-0024; el escritorio Fyne la renderiza de forma nativa y el móvil Capacitor la muestra
  dentro de su cliente web;
- la capa de anotaciones **comparte sistema de coordenadas** con el documento.

**Lo que hubo que resolver, y no era obvio.** Una fuente compuesta con codificación
Identity-H trae la tabla `cmap` vacía —el PDF nunca la necesita, sus códigos son índices de
glifo— así que el navegador no encuentra nada y pinta cuadritos. Se **reconstruye** esa tabla
invirtiendo /ToUnicode, y con eso un documento de InDesign o de LaTeX se ve con SU tipografía
y además se puede copiar. Cuando no hay forma —Type1 antiguo, CFF suelto— se sustituye por
una familia genérica y `textLength` mantiene la línea cuadrada.

**Descartado.** **Rasterizar en Go** (el motor de gráficos y el de fuentes, decenas de miles
de líneas, y el resultado no es seleccionable); **PDF.js** (AGPL, incompatible con el
clean-room permisivo de `security.md#4`); **un binario externo** (rompe el invariante de cero
dependencias y añade una superficie de ataque en el camino de un fichero de fuera); **Canvas
en el cliente** (haría falta el mismo intérprete pero en JavaScript, y perdería la selección
de texto igual que una imagen).

**Coste / seguimiento.** Lo que SVG no expresa degrada a algo visible y nunca a un hueco
mudo: los patrones de mosaico se traducen a `<pattern>`, los sombreados de malla se aproximan
por su color medio, y CCITT, JBIG2 y JPEG 2000 —que ningún navegador decodifica— dejan un
recuadro gris en su sitio. El SVG de una página compleja pesa cientos de kilobytes, así que
se sirve por lotes y con un tope de 24 MiB por página. Un decodificador CCITT propio es la
deuda más clara: es el formato de casi todo lo escaneado, y hoy esas páginas no se ven.

**Una salida MÁS, la página rasterizada (2026-09-22).** Esta decisión NO se deshace: sin
`formato`, o con `formato=svg`, `op: render` sigue entregando el SVG con `<text>`, y la web
sigue usándolo para seleccionar, buscar y leer en voz alta. Lo que se añade es
`formato=png` para el cliente que no tiene un motor de SVG con texto detrás. Es el
escritorio Fyne: todo SVG pasa por `oksvg`, que no implementa `<text>` —medido a 120×120,
un `<rect>` pinta 10.000 píxeles y un `<text>` pinta 0—, así que CertiPDF nativo enseñaba el
papel y las rayas de una factura sin una sola cifra.

- **No es el motor que aquí se descartó.** El intérprete, los colores, las tipografías, las
  imágenes y el recorte ya existían: el MISMO recorrido del flujo pinta además en un mapa
  de bits (`pdfraster.go`, menos de mil líneas) con `x/image/vector`, y los glifos salen de
  `x/image/font/sfnt` sobre la misma tipografía que el SVG incrusta. `x/image` ya estaba
  en `go.mod`: cero dependencias nuevas. En modo PNG no se construye el SVG.
- **Lo que el PNG no hace y el SVG sí:** regla par-impar (`f*` sale como no-cero), modos de
  fusión, mosaicos (no se pintan) y sombreados (su color medio). Type1 y CFF sueltos, que
  `sfnt` no lee, se dibujan con las Go Fonts, cada glifo en su sitio y con el avance del PDF.
- **Coste, medido en el frontal** (ARM64, no la máquina de producción, con carga 7-68 en
  6 núcleos; CPU de proceso, mediana de 5, dos tandas que coinciden; A4 a ×2 ≈ 1,9 Mpx;
  siete documentos reales; `pdfraster_medida_test.go`): dibujar 13-183 ms, codificar el
  PNG 21-98 ms con `BestSpeed` (30-142 con la compresión por defecto, que era del 40 al
  85 % de la CPU de la página, a cambio de hasta un 3 % menos de bytes: no compensa), y
  el SVG de la misma página 1-92 ms. Se reservan 10-35 MiB por página (el SVG, 0,6-24),
  de los que 7,4 son el propio mapa de bits RGBA. Pesa 45-710 KiB, del orden del SVG
  (64-1.182 KiB).
- **Caché y tamaño.** El servidor NO guarda nada: la promesa de `/pdf/*` es que el fichero
  se procesa en memoria y se devuelve (`no-store`), y un PNG de un contrato sigue siendo el
  contrato. La caché es del cliente, por (documento, página, escala). El cliente pide la
  escala EXACTA de su pantalla (zoom × factor HiDPI) para no reescalar, y solo la ventana
  que se mira: lotes de 8 como máximo, no los 20 del SVG. Tope de 16 Mpx por página
  (64 MiB, un A4 a ×5,6), comprobado antes de reservar.
- **Fidelidad**, contra poppler como vara de medir (no se usa en el servicio): en páginas
  con tipografía TrueType o CID incrustada se apartan más de 64 niveles entre el 0,3 y el
  5,8 % de los píxeles —el antialiasing de los bordes del texto—; con Type1 sustituida,
  un 10 %, y se lee.
- **Lo que salió al medir, en el intérprete COMPARTIDO:** `Q` restauraba el estado del `q`
  ANTERIOR y no el suyo. Con una matriz puesta antes del `q` —Google Slides abre así todas
  sus páginas— todo lo posterior al primer `Q` caía fuera de la hoja, **también en el SVG
  de la web**. Arreglado. Y queda uno sin arreglar: el decodificador de JPEG 2000 devuelve
  los colores cambiados (verde y magenta) en los escaneos de archive.org, en las dos
  salidas por igual, y tarda de 1,2 s (x86) a 15 s (el frontal) y reserva 1,7 GiB por página.

---

<a id="0034"></a>

## 0034 — OCR: Tesseract autoalojado en un servicio propio; la capa de texto la escribe Sytue PDF

`accepted` · 2026-08-13 · pdf, ocr, infra · **sustituye a la primera redacción del mismo día**

**Contexto.** El OCR es la última pieza de la ola P8 y la única de todo Sytue PDF sin camino
evidente en la biblioteca estándar. Reconocer caracteres es un problema de visión por
computador: no es «unas tablas más», es un modelo.

**La observación que ordena la decisión:** «OCR» son DOS cosas que se confunden porque
llegan juntas.

1. **Reconocer**: de píxeles a caracteres con su posición. Es el modelo.
2. **Escribir la capa**: meter ese texto en el PDF como texto INVISIBLE (modo de render 3)
   exactamente encima de donde está dibujado, para que el documento se pueda buscar,
   seleccionar y copiar sin que cambie ni un píxel de lo que se ve.

Solo la primera necesita un modelo. **La segunda es formato de PDF y ya se sabe hacer.**

**Decisión del fundador (2026-08-13), que reemplaza la primera redacción de este ADR:** el
motor tiene que ser **software libre y autoalojable**. Nada de llamar a un proveedor de IA,
ni soberano. Con eso, la decisión de negocio que este ADR dejaba abierta **desaparece**: no
hay proveedor que elegir ni coste por página que negociar.

**Decisión.**

- **El motor es [Tesseract 5](https://github.com/tesseract-ocr/tesseract), Apache-2.0**,
  dentro de la imagen. Licencia permisiva —mejor de lo que el fundador pedía, que admitía
  GPL—, más de cien idiomas y el estándar de facto desde hace veinte años.
- **Vive en un servicio propio, `services/ocr`** (Go, puerto 8096, ClusterIP **sin
  ingress**). Es el binario de Tesseract con una capa HTTP delante: entra una imagen, salen
  palabras con su caja. No sabe nada de PDF.
- **La capa de texto la escribe Sytue Docs**, con el motor de la casa y sin nada nuevo
  (`CapaDeTextoPDF` en `pdfocr.go`). Docs también hace **la conversión de coordenadas**, que
  es lo que traduce los píxeles del reconocedor al espacio del PDF (`ImagenColocada.RectDePDF`).
- **La imagen que se reconoce sale del PROPIO PDF**, no de un rasterizado. En un documento
  escaneado la página **es** una imagen incrustada, así que se extrae con su matriz de
  colocación (`ImagenDeEscaneo`) y se manda tal cual. Sin pérdida de calidad, sin viaje al
  cliente y sin rasterizador — que es lo que [ADR-0033](#0033) decidió no escribir.
- **La credencial es el JWT del usuario**, que Docs reenvía. Ni secreto compartido entre
  servicios ni confianza en la red: un reconocimiento queda atribuido a quien lo pidió.
- **El endpoint que ESCRIBE la capa sigue aceptando texto reconocido venga de donde venga.**
  No pide saber quién lo reconoció, así que un escaneo procesado fuera se puede incorporar.

**Por qué un servicio y no una biblioteca.** Enlazar `libtesseract` con cgo metería C en un
binario que hoy es estático sobre distroless, y los ~100 MB de datos de idioma viajarían en
**cada réplica de Docs** para una función que usa una petición de cada mil. Aparte, el OCR es
CPU pura a ráfagas: como servicio escala solo, sin arrastrar al editor.

**El precio, dicho aquí para que no se descubra tarde.** Es la **única imagen de la suite que
no es distroless**: Tesseract necesita un gestor de paquetes que distroless no tiene. Se
compensa con base `debian:bookworm-slim` fijada por digest, `--no-install-recommends`, sin
cachés de apt, proceso no-root con el mismo uid 65532 que el resto,
`readOnlyRootFilesystem` y el temporal de cada página en un `emptyDir` **en memoria** —para
que una nómina escaneada no llegue a tocar el disco del nodo—. La `NetworkPolicy` solo
permite salir hacia Sytue ID (JWKS) y Cockpit: **si algún día apareciera tráfico saliente
desde este pod, sería un incidente, no una configuración que falta.**

**Descartado.**

- **Un proveedor de IA (incluso soberano)**, que era la decisión anterior: el fundador la
  revocó. Añadía un coste por página, una dependencia externa y una decisión de negocio
  pendiente, a cambio de nada que Tesseract no dé autoalojado.
- **Rasterizar en el cliente y subir PNG**, que planteaba la primera redacción: funciona,
  pero para un escaneo es peor —la imagen ya está dentro del PDF— y hace inviable el lote de
  un expediente de trescientas páginas.
- **Tesseract compilado a WASM en el navegador** (tesseract.js, Apache-2.0). Es tentador
  porque no añade ningún sistema y el documento no sale del dispositivo. Se descarta por el
  lote y por el móvil: descargar ~15 MB de modelo y reconocer trescientas páginas en una
  pestaña no es un producto. Queda anotado como camino si algún día se quiere un modo
  «sin servidor».
- **Un motor propio.** Es un proyecto de investigación, no una ola.
- **Enviar el PDF a un servicio de OCR de terceros**: es exactamente lo que Sytue existe para
  evitar; ahí acaban las nóminas.

**Consecuencias.** La herramienta se ofrece **solo si hay servicio detrás**: `/pdf/ocr`
devuelve `disponible` y la interfaz esconde el botón cuando es falso, con la misma disciplina
que el asistente de IA — pero al revés que aquel, aquí el chart **sí** existe y `ocr.enabled`
viene en `true`, así que en un despliegue normal está encendido. Los idiomas instalados son
los cinco de producto (`spa`, `eng`, `fra`, `deu`, `ita`) más `osd` para detectar
orientación; pedir uno que no esté se **rechaza** en vez de caer al inglés, porque reconocer
un documento en español con el modelo inglés no da un error: da un texto plausible y mal, que
nadie revisa. Y un matiz que conviene recordar: un PDF escaneado que YA venga con capa de OCR
se busca y se copia en Sytue PDF sin hacer nada, porque el render conserva el texto invisible
en vez de descartarlo.

<a id="0035"></a>

## 0035 — Sytue AI: dos modelos en Scaleway, tope blando por usuario y OCR por modelo de visión

`accepted` · 2026-09-09 · ai, infra, pdf, fotos · **revisa el motor de OCR del [0034](#0034)**

**Contexto.** `services/ai` llevaba meses escrito y sin desplegar: `POST /api/v1/assist`,
defensa anti-inyección, límite de tasa y tests. Lo que faltaba no era código, era **elegir
proveedor**, y eso bloqueaba también el panel del editor, que se oculta solo cuando el BFF no
ve `SYTUE_AI_URL`.

**Lo que se midió, que es lo que ordena todo lo demás.** Contra Scaleway Generative APIs
(fr-par), el 2026-09-09, con tres repeticiones por tarea y sin tope de salida, sobre cuatro
tareas reales de la suite (respuesta de Mail, fórmula de Calcus, extracción de factura de
certiDocs, titular de Notepads) más la receta de revelado de Fotos:

| modelo                | €/10.000   | latencia  | fórmula         | revelado                    |
| --------------------- | ---------- | --------- | --------------- | --------------------------- |
| mistral-small-3.2-24b | **0,59 €** | **0,6 s** | mal 3 de 3      | +1 paso (el doble)          |
| pixtral-12b           | 1,15 €     | 0,5 s     | mal 3 de 3      | +2 pasos (revienta la foto) |
| gpt-oss-120b          | 1,71 €     | 2,8 s     | bien 3 de 3     | +1/+0,5/+1                  |
| gemma-4-26b           | 5,66 €     | 14,0 s    | **bien 3 de 3** | **+0,5/+0,1/+0,3**          |

**Decisión.**

- **Dos modelos, y el modo lo elige quien llama** (`"modo":"calidad"`), no el servicio
  adivinando desde la acción: un panel en línea sabe que su usuario está mirando el cursor y
  un trabajo de fondo sabe que no hay nadie esperando. `mistral-small-3.2-24b` atiende lo
  interactivo; `gemma-4-26b` lo que puede esperar y tiene que salir bien.
- **`reasoning_effort: high` en el modelo de calidad**, que es contraintuitivo y por eso queda
  escrito: sale más barato (5,43 €/10.000), más rápido (8,1 s) y más certero (9 de 9) que el
  defecto sin fijar. `low` y `medium` divagan. Y `none` es OCHO veces más barato con el mismo
  acierto **en texto**, pero se deja la exposición al analizar una foto: por eso el esfuerzo
  se puede pedir por petición y no es un ajuste global.
- **Tope blando de 1,50 € por usuario y mes sobre el modelo caro.** Al agotarlo NO se corta:
  la petición se sirve con el modelo rápido y la respuesta lo dice (`degradado: true`). El
  contador vive en la caché compartida, con el mes dentro de la clave para que se reinicie
  solo. Es blando en tres sentidos asumidos: se comprueba antes y se cobra después —la
  petición que cruza el tope se sirve entera—, un vaciado de caché regala el mes, y sin caché
  no hay tope. Se degrada ABIERTO a propósito: quedarse sin asistente por una pieza opcional
  es peor que gastar de más, y el servicio lo avisa al arrancar.
- **Whisper-large-v3 para audio** (0,003 €/minuto: una reunión de una hora, 18 céntimos) y
  **qwen3-embedding-8b para vectores** (0,10 €/millón). Ninguno pasa por el presupuesto: no se
  facturan por token de salida y su gasto no se dispara solo.

**El motor de OCR: esto revisa el 0034, y conviene decirlo claro.** El 0034 decidió que el
motor «tiene que ser software libre y autoalojable. Nada de llamar a un proveedor de IA, ni
soberano». **El fundador lo revisa el 2026-09-09**: el OCR pasa a hacerse con el modelo de
visión —Gemma 4 mientras haya presupuesto, Mistral Small después—, con una condición explícita
de vuelta: **si la calidad no acompaña, se regresa a Tesseract**. El chart `ocr` y el servicio
NO se retiran, precisamente para que esa vuelta sea cambiar quién llama y no reconstruir nada.

Lo medido el mismo día sobre una factura sintética limpia: los tres transcriben el texto
íntegro y sin error —Gemma con `esfuerzo: none` en 0,8 s y 0,011 céntimos, Gemma con `high` en
2,3 s y 0,025, Mistral en 0,7 s y 0,006—. Para OCR, `none` es lo correcto: el doble de barato
y el triple de rápido con salida idéntica. **Lo que esa prueba NO cubre** es el caso que de
verdad decide: un escaneo torcido, con sombra o a baja resolución. Ahí es donde Tesseract, que
lleva veinte años en ese problema, puede seguir ganando. La condición de vuelta del párrafo
anterior existe por eso.

**Lo que Scaleway NO tiene, y hay que saberlo antes de prometer nada.** No hay ningún modelo
de imagen: ni en Generative APIs (15 modelos) ni en Managed Inference (57). Las etiquetas de
tarea son `chat, instruct, code, vision, reasoning, embeddings, audio_transcription`, y
`/v1/images/generations`, `/v1/images/edits` y `/v1/videos/generations` responden 404. `vision`
es imagen de ENTRADA. Por eso la IA de Sytue Photos **no edita píxeles: escribe la receta** que ya
sabe aplicar `services/syphotos/internal/service/receta.go`, con las mismas fórmulas en Go y
en el navegador. Editar de verdad —borrar un objeto, rellenar fondo— exigiría un modelo de
difusión en una GPU propia, que se paga por hora encendida y no por uso: es otra decisión y no
está tomada.

**Consecuencias.**

- Chart `ai` en el umbrella, con `global.ai.enabled` como ÚNICO interruptor: lo miran el chart
  del servicio y el del frontend, y con uno por subchart se podrían contradecir.
- La clave del proveedor va en un Secret con `optional: true`, y es **la secret key sola, no
  el par**: Scaleway emite las API keys como access key + secret key, pero Generative APIs solo
  acepta la secret key como Bearer —comprobado el 2026-09-09: con la access key devuelve 403, y
  con las dos juntas también—. Es la convención que ya sigue el repo para las APIs REST de
  Scaleway. Sin ella el servicio arranca
  sano y responde 503 `ai_not_configured`, que es lo que el panel ya sabe leer para ocultarse.
  Un secreto obligatorio dejaría el pod en `CreateContainerConfigError`, que no se parece en
  nada a «falta configurar la IA».
- El tope de salida del modelo de calidad es cuatro veces el del rápido y no es holgura: los
  tokens de razonamiento salen por ahí. Con 1024, Gemma devuelve la respuesta **vacía**, que
  llega como un 502 sin causa visible. Por eso `finish_reason: "length"` tiene su propio error
  (`ai_truncated`) y se comprueba ANTES que el vacío.

<a id="0036"></a>

## 0036 — Cloudflare fuera: el portal entra por Pangolin y el DNS lo sirve Scaleway

`accepted` · 2026-09-10 · infra, dns, correo, certificados

**Contexto.** El portal de `sytue.eu` entraba por **Cloudflare Tunnel** y su zona DNS vivía en
Cloudflare. De ahí colgaban cinco cosas que no se parecen entre sí y que había que mover a la
vez o no moverlas: la entrada del tráfico web, el reto DNS-01 de los certificados, el DDNS que
persigue la IP doméstica, el guardián que repone los MX y el comodín que emite el frontal.

**La decisión.** Ninguna pieza de la instalación habla ya con Cloudflare. Queda esto:

| Lo que hacía Cloudflare           | Quién lo hace ahora                                                                         |
| --------------------------------- | ------------------------------------------------------------------------------------------- |
| Túnel de entrada al portal        | **Pangolin** (Traefik + WireGuard) en la máquina de medios, con el agente `newt` en el nodo |
| Zona DNS autoritativa             | **Scaleway DNS** (`ns0`/`ns1.dom.scw.cloud`)                                                |
| DDNS del registro `mx`            | `bin/ddns.sh`, contra la API de Scaleway                                                    |
| MX, SPF, DMARC y TLS-RPT          | `bin/mx-dns.sh`, contra la API de Scaleway                                                  |
| Reto DNS-01 de cert-manager       | webhook `acme.scaleway.com` + `ClusterIssuer` con `ACME_SOLVER=dns01-scaleway`              |
| Comodín `*.<dominio>` del frontal | Traefik del frontal con `provider: scaleway`                                                |

**Lo que se aprendió moviéndolo, que es lo que no se ve en un diagrama.**

**El proveedor de DNS se averigua, no se configura.** Una variable dice lo que alguien creyó el
día que la escribió. El día de la migración, el DDNS siguió actualizando la zona vieja cada
cinco minutos **sin un solo error** mientras la buena se quedaba con la IP anterior. Ahora
`ns_autoritativos()` pregunta con `dig +trace`, que baja desde la raíz: ni caché, ni validación,
ni la opinión de un resolutor que resulta ser autoritativo de la zona que se acaba de abandonar
—`1.1.1.1` seguía contestando los NS antiguos horas después de cambiar la delegación—.

**La API de DNS de Scaleway tiene tres trampas, y las tres contestan `200`:**

1. el `name` es el **label relativo** y el apex es la cadena vacía; con el nombre completo, las
   consultas devuelven cero registros como si no existieran, y las escrituras aterrizan en el
   apex de la zona;
2. un destino **sin punto final es relativo**: un MX hacia `mx.sytue.eu` se guarda como
   `mx.sytue.eu.sytue.eu.`, que no resuelve;
3. la **prioridad del MX viaja dentro de `data`** (`10 mx.sytue.eu.`) además de en su campo, así
   que comparar contra el destino a secas hace creer que hay un MX ajeno.

Ninguna de las tres da error. Por eso los dos guiones que escriben DNS **comprueban lo que
devuelve la API**, y `mx-dns.sh` tiene `--dry`: un guion que toca el DNS del correo tiene que
poder ensayarse.

**Un rojo que no significa nada enseña a no mirar los rojos.** Mientras un guion no sabía
escribir en el proveedor nuevo, dejaba su timer en rojo cada quince minutos. Se prefirió que
saliera con 0 avisando, y que quien vigile de verdad sea `bin/verify.sh` contra el DNS público
—que es lo que ve quien entrega el correo—.

**Dos detalles del frontal que se afinaron al terminar (2026-09-10).** El reto del
comodín pregunta el TXT a **los autoritativos de la zona** —`ns0`/`ns1.dom.scw.cloud`—
y no a un resolver público: lego solo los usa para ver si su propio reto ya está
publicado, así que preguntarlo en el origen evita esperar a cachés ajenas y no
depende de terceros. Y el contenedor recibe **`SCALEWAY_PROJECT_ID`** además de la
secret key: para lego es opcional —solo exige la clave—, pero la zona se busca
dentro de un proyecto, y sin él va al de por defecto de la credencial; el día que
esa clave sirva a dos proyectos, el TXT del reto se publicaría en el equivocado.

**Lo que NO cambió, y no por olvido.** El asistente de dominio propio del portal
(`apps/frontend/apps/portal`) sigue ofreciendo Cloudflare: ahí el token es **del cliente**, para
su zona, y no tiene nada que ver con la nuestra.

**Cuándo reconsiderarlo.** Si el frontal deja de ser Pangolin o la zona vuelve a moverse, lo que
hay que revisar es la tabla de arriba entera: son seis piezas que dependen de las dos primeras
filas, y moverlas de una en una es como se llega a escribir en una zona que ya no manda.

<a id="0037"></a>

## 0037 — La plomería compartida se comparte de verdad: catálogo, guardianes y reglas de eficiencia

`accepted` · 2026-09-12 · código, mantenimiento, rendimiento

**Contexto.** Cada producto de esta suite se ha creado copiando el anterior. Es la forma más
rápida de arrancar y la más cara de mantener, porque lo copiado **diverge solo** y lo hace por
donde nadie mira. No es una preocupación teórica; todo lo de abajo está medido en este repo:

| Lo copiado                                                                                  | Cómo había divergido                                                                                      |
| ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| El endpoint de auditoría de compliance, en Mail y Drive                                     | ~490 líneas idénticas salvo 36, y una validación contestaba **en español en uno y en inglés en el otro**  |
| El `middleware.ts` de doce superficies, con un «copia este fichero tal cual» en su cabecera | Al medirlo, solo **cinco de doce** seguían siendo iguales byte a byte                                     |
| El getter de access token, en tres apps                                                     | **Márgenes de caducidad distintos** (60 s contra 30 s): así es como una app manda tokens ya caducados     |
| Los siete endpoints de `/auth/*`, en doce apps                                              | 82 ficheros y 927 líneas; el GET del logout escrito de dos formas y el JSON de `/auth/token` de otras dos |

**La decisión.** La plomería vive una vez, con su catálogo escrito y con tests que fallan cuando
alguien vuelve a copiarla.

1. **Catálogo normativo** en [`CONVENTIONS.md` §13.2](CONVENTIONS.md#13-2-lo-que-ya-existe): los
   22 paquetes de `platform/pkg/` y los 10 de `apps/frontend/packages/`, cada uno con para qué
   sirve. Antes de escribir plomería se busca ahí; si existe y no encaja, se **amplía**, no se
   copia.
2. **Guardianes que comparan por igualdad**, no por pertenencia
   (`plomeria-compartida.test.ts`, `rutas-auth-reexportan.test.ts`). Así fallan también cuando una
   excepción se deduplica y nadie borra su entrada: una lista blanca que nadie poda deja de
   proteger en dos refactores. `rutas-auth-reexportan` ya se puso en rojo por eso mismo, al
   unificar el `/auth/token` de Docs, Drive y Portal.
3. **Reglas de eficiencia** en [§13.3](CONVENTIONS.md#13-3-eficiencia), cada una salida de un
   fallo concreto, no de un principio general.

**Lo que NO se unifica, que es la mitad de la decisión.** Duplicación no es lo mismo que
variación deliberada, y confundirlas produce abstracciones peores que el copia-pega. Se midieron
tres candidatos y **los tres se dejaron separados a propósito**:

- `ValidName` existe en cuatro servicios con tres implementaciones, y las tres son correctas: 128
  bytes en Calendar/Contacts, 255 en Drive porque son nombres de fichero, y 100 runas en Rooms,
  que valida **nombres de canal** y por eso no rechaza `/` ni `..`. Unificarlas pediría un
  parámetro de longitud y un flag de «seguro para rutas».
- `googleReason`/`graphCode` difieren entre Calendar, Contacts y Drive porque cada uno mapea los
  estados HTTP que **de verdad emite**: Drive los de subida (413/415/507), los otros el 412 de
  peticiones condicionales. Se comprobó servicio por servicio que cada omisión encaja.
- El listado de auditoría de **Sytue ID** no entra en el paquete común: empuja filtros y keyset a
  una consulta del store en vez de filtrar en memoria, que es otra arquitectura.

El criterio: si la diferencia es de **dominio** (qué significa un nombre aquí), quédense
separadas; si es de **accidente** (un comentario reflowado, un mensaje traducido, un número que
alguien tocó en un sitio), es duplicación.

**Lo que se aprendió, y es lo que no se ve en el diff.**

**Definir algo no lo aplica.** El middleware de compresión `sytue-compress` llevaba **siete días
desplegado** en el clúster, bien escrito, con `zstd`/`br`/`gzip` y `text/event-stream` excluido
para no bufferizar los SSE — y **no lo referenciaba ni uno de los 49 ingresses**. La plataforma
servía 345 kB en claro. Un manifiesto puede enseñar un objeto perfectamente formado que es
decorado. Ahora un gate del render exige que todo Middleware con `compress` aparezca en el
argumento del entrypoint de Traefik.

**Optimizar lo que no se ejecuta es trabajo perdido con aspecto de progreso.** Un `setInterval`
de refresco de sesión venía anotado como pendiente de arreglar —le faltaba la pausa por
visibilidad—. Al ir a ponérsela: **no lo llamaba nadie**, y el refresco real lo hace el servidor
bajo demanda, que es estrictamente mejor. Se borró en vez de arreglarse.

**El cuello no está donde se supone.** Se buscaban 700 ms de hilo principal en el portal. Medido:
1146 tareas y **ninguna pasaba de 50 ms**, con TBT 0 y cero CSS o JS sin usar. Lo que había era
que nada iba comprimido. Y las siete llamadas a API «de la cadena crítica» resultaron **no estar
en el camino del LCP**: su desglose son TTFB y render delay, sin fase de carga de recurso.

**Consecuencias medidas.**

|                                             |                                                                                   |
| ------------------------------------------- | --------------------------------------------------------------------------------- |
| Auditoría: dos ficheros de 247 y 245 líneas | 57 y 56, con la lógica en `platform/pkg/auditlog` y **con pruebas, que no tenía** |
| `/auth/token` y su getter                   | Una implementación para las doce superficies (antes tres a mano)                  |
| Resolver una ruta de WebDAV                 | De **seis transacciones a dos**, y ya no crece con la profundidad                 |
| Añadir 200 fotos a un álbum                 | De 200 sentencias con 200 `max(position)` a **una**                               |
| Portal, bytes transferidos                  | 345.712 → **104.978 B**; LCP 1.792 → 1.172 ms; Performance 100                    |

**Cuándo reconsiderarlo.** El catálogo de §13.2 se desfasa solo: lo generan las carpetas, no un
acuerdo. Si al mirar el repo no coincide, manda el repo. Y las 136 consultas de store sin `LIMIT`
que quedan anotadas **no se priorizaron a propósito**: las bases del piloto pesan entre 8 y 18 MB
y elegir cuál arreglar sería adivinar. Se revisan cuando haya volumen real, midiendo
`pg_stat_user_tables`, no antes.

<a id="0038"></a>

## 0038 — Cuentas personales: registro gratuito con 5 GB y plan de 1 TB por 5 EUR al mes

`accepted` · 2026-09-13 · producto, identidad, cuotas, facturación

**Contexto.** Hasta aquí nadie podía entrar en Sytue One por su cuenta: las cuentas nacían por
invitación de una organización o por un IdP externo, y la web pública decía «Empezar gratis —
sin tarjeta» en un botón que llevaba a una pantalla de login. Se quiere que una persona sola pueda
registrarse, tener 5 GB gratis para todos los productos y, si necesita más, pagar 5 EUR al mes por
1 TB. Sin organización propia —su organización es su correo—, y por tanto sin Sytue Admin ni SyERP.

**La decisión.** Una cuenta personal ES una organización de una persona, en el modelo de datos de
siempre —el alta guiada ya lo había decidido así (mig 0027)—, distinguida por dos columnas nuevas
(mig 0063):

| Columna                                                      | Qué decide                                                                                                                                     |
| ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `organizations.kind` = `personal`                            | Que no hay Sytue Admin ni SyERP. **No puede apoyarse en el rol**: la persona es `owner` de su propia organización, y por rol entraría en todo. |
| `org_subscriptions.plan` = `personal_free` \| `personal_1tb` | La cuota. 5 GiB de Drive y 1 GiB de buzón gratis; 1 TiB y 25 GiB pagando.                                                                      |

Y de ahí salen cinco piezas, cada una en el sitio que ya existía para lo suyo:

1. **`/signup` en Sytue ID.** Crea la persona y su organización personal en una transacción, con el
   contrato gratuito y el alta guiada ya hecha. El slug se deriva del correo
   (`domain.PersonalSlug`: `p-` + resumen), porque un correo no es un slug válido y el slug es
   subdominio y namespace. El prefijo `p-` queda reservado: una organización no puede tomarlo.
2. **La única defensa es la verificación del correo.** No se puede iniciar sesión hasta demostrar
   control del buzón. Sin eso cualquiera abriría cuentas —con su espacio— a nombre de direcciones
   ajenas. Y el alta no revela si una dirección ya existe: la respuesta es la misma.
3. **El plan viaja en el token** (`org_kind`, `plan`, `storage_bytes`, `mailbox_bytes`) y cada
   servicio lo aplica sin preguntar: Drive fija su cuota, Mail la del buzón, SyERP y Sytue Admin
   rechazan a las personales (`authn.RequireOrgKind`). Un token sin los claims se lee como
   organización: es anterior a la distinción, y lo que había antes eran organizaciones.
4. **Una cuenta personal nunca se queda fuera.** La gratuita no paga, y la de 1 TB que deja de pagar
   **vuelve a ser gratuita**: lo que cambia es la cuota, no la puerta. Cerrarle la sesión a alguien
   por un recibo devuelto sería cerrarle su propio correo.
5. **El cobro reutiliza Mollie tal cual.** Un checkout personal fija plan e importe en el servidor
   —los asientos que mande el navegador se ignoran— y el webhook exige que lo cobrado sea
   exactamente el precio del plan contratado; un plan gratuito no cobra nunca.

**Lo que se decidió NO hacer, y por qué.**

- **Un solo contador para «todas las apps».** Se pidió 5 GB _totales_. Hoy son **dos contadores**:
  Drive —donde viven ficheros, fotos (sus originales) y documentos, así que una cifra los cubre— y
  el buzón de Mail, que lleva el suyo. Un contador único pediría un libro mayor entre servicios
  que no existe, y la alternativa —que Mail reserve contra Drive en cada mensaje— acopla dos
  servicios por algo que se resuelve declarando el reparto en un sitio (`platform/pkg/commerce`).
  Si algún día hace falta el contador único, el bus de eventos ya está.
- **Aprovisionar almacenamiento «en Scaleway» al comprar.** No es cómo funciona el sistema: el
  object storage es un endpoint por despliegue con un prefijo por tenant, no un bucket por persona.
  Lo que sí existe es la capacidad del despliegue: **los blobs de los tenants `p-*` van desde el
  primer día a un bucket propio en Scaleway One Zone** (segundo backend de `objectstore`,
  `S3_PERSONAL_*`), que crece con el uso y cuesta cero vacío, mientras las organizaciones siguen en
  el disco del piloto. Las tarifas medidas y el detalle están en
  [`roadmap.md`](roadmap.md#almacenamiento-de-cuentas-personales-en-scaleway). Con esos números
  el plan de 1 TB por 5 EUR está **por debajo de coste si se llena** (8,22 EUR/mes en la clase
  barata): vive de que el uso medio quede lejos del techo.
- **DNS por cuenta personal.** Cada organización recibe `<slug>.<dominio>`; una personal no lo
  necesita —entra por `one.<dominio>`— y publicar un registro por cada alta gratuita sería
  regalar la zona. Consecuencia: en el piloto no hay MX para ellas, y por tanto no reciben correo
  entrante. Es la misma limitación que hoy tiene cualquier organización del piloto sin
  `tenantDNSAddress`.

**Lo que se aprendió midiendo antes de escribir.** Tres cosas parecían necesarias y ya existían: el
recorrido «individual» del alta guiada, la verificación de correo con su token firmado, y la
pasarela de pago con su webhook. Y una discrepancia que no es de esta decisión pero conviene
dejar anotada: la web dice «5 € por usuario y mes» para organizaciones y `commerce` cobra 600.

**Consecuencias.** Tres migraciones de comportamiento en `RequirePaidSubscription`: las cuentas
personales siempre tienen derecho de uso; el precio unitario admite 0 y 500; y `Entitled` ya no es
solo «pagado y con asientos». En el piloto no hay Mollie configurado, así que el plan de 1 TB
responde «pagos no disponibles» hasta que se configure —la cuenta gratuita funciona completa—.

Y desde el 2026-09-13 el paso a 1 TB está **cerrado en todas las instalaciones**, no solo donde
falta Mollie: `commerce.PersonalUpgradeAvailable = false` hace que Sytue ID rechace el checkout de
una cuenta personal (503), que el estado del contrato lo diga (`upgrade_available`) y que la app
de Cuenta no ofrezca el botón; www anuncia el precio como «próximamente». El registro gratuito
sale antes que el cobro; abrirlo es cambiar esa constante cuando se decida vender.

**Precios finales (2026-09-13, misma tarde):** tres planes y los mismos productos en los tres —
`personal_free` (una persona, 5 GB), `standard` (5 EUR por usuario y mes, 200 GB por usuario) y
`pro` (20 EUR por usuario y mes, 1 TB por usuario)—, para organizaciones (por asiento) y para
personas solas. El asiento de 6 EUR con 500 GB desaparece; lo contratado pasa a `standard` (mig
0063). Todos los planes permiten conectar almacenamiento S3 propio (Drive, `external_storage`), que
no cuenta en la cuota. Y dos matices sobre la cuenta personal: **sí entra en Sytue Admin**, solo
para integrar aplicaciones y migrar datos (`requireOrgKind` cierra usuarios, grupos, roles,
dominios e invitaciones), y **su buzón cuelga del dominio padre** (`nombre@sytue.eu`), que se
administra en platform.<dominio>: un subdominio con su hash sería una dirección que nadie sabría
teclear. Y todo el almacenamiento nuevo —de cualquier tenant— va a Scaleway One Zone, con las
versiones anteriores en Glacier como copia; solo los tenants listados en `S3_LOCAL_TENANTS` se
quedan en el disco del piloto.

**Nombres finales y condiciones (2026-09-14):** el de 5 EUR se llama **Pro** y el de 20 EUR
**Enterprise**, que incluye **Soporte Premium**; las **instituciones educativas** tienen el Pro al
50 % (`organizations.education`, lo concede la plataforma con motivo y auditoría; el checkout y la
comprobación del pago usan `commerce.Plan.UnitPriceCents(education)`); y la **revalorización
comprometida de por vida** es de 0,50 EUR por usuario cada cuatro años, y nada más. El alta
gratuita va protegida por el captcha ALTCHA del propio Sytue (`services/id/internal/captcha`,
prueba de trabajo sin terceros ni cookies) y el limitador por IP. El **almacenamiento conectado**
admite un bucket S3, un servidor WebDAV (Nextcloud/ownCloud, la CLI de Internxt), **Google Drive**
y **OneDrive** por consentimiento OAuth en el servicio de integraciones, y **el NAS del cliente por
el túnel de Sytue Cloud**: Drive crea el sitio, el recurso y el destino en el proxy de la
plataforma por su API de integración, y el cliente solo ejecuta un comando en su NAS —nunca ve el
proxy—; el destino se acota a su red (IP privada o nombre local) para que el túnel no pueda servir
un sitio cualquiera bajo nuestro certificado. Todas las clases exigen aceptar en cada conexión el
aviso de responsabilidad: las copias de seguridad y la recuperación de lo que vive en un almacén
conectado son del cliente (usted), y Sytue® queda liberada de toda responsabilidad por su pérdida.
Proton Drive no tiene API pública (cifrado de extremo a extremo con protocolo propio) y no se
anuncia hasta que la tenga.

<a id="0039"></a>

## 0039 — Aviso a móvil: Android no sale de la UE, iOS sale con lo mínimo que existe

`accepted` · 2026-09-14 · push, mobile, privacy, gdpr, sovereignty

**Contexto.** La suite ya avisa con el navegador cerrado por **Web Push** (RFC 8030/8291,
`services/rooms`): el contenido va cifrado extremo a extremo y el servicio de push del
fabricante —Google, Mozilla o Apple— no puede leerlo, pero **sí ve el endpoint** (un
identificador seudónimo del aparato) **y cuándo** llega cada aviso. Ninguno de esos tres está
en la UE. Al llevar la suite a apps nativas hay que decidir por dónde entra el aviso, y la
respuesta no es la misma en las dos plataformas.

**Android: no hace falta ningún tercero.** La app puede mantener abierto el **flujo SSE del
hub** que ya consume el shell web (`NotificationCenter.tsx` con `EventSource`) desde un
servicio en primer plano. El aviso nace en nuestro clúster y muere en el aparato: ni FCM, ni
Google Play Services, ni un identificador fuera de la UE. Funciona además en los teléfonos sin
servicios de Google, donde FCM sencillamente no existe.

**UnifiedPush queda para después, y no lo necesitamos.** UnifiedPush resuelve que _varias_ apps
compartan _una_ conexión que mantiene un distribuidor. Como aquí solo nos avisamos a nosotros
mismos, un distribuidor añade una pieza —y un servidor de push que mantener— sin quitar
ninguna. Si algún día interesa compartir la conexión, el emisor actual **ya vale**: UnifiedPush
3 habla Web Push, y `validatePushEndpoint` no tiene lista blanca de fabricantes, solo exige
`https` y la barrera antiSSRF. Se apuntaría el endpoint y ya.

**iOS: no hay alternativa, así que se decide qué se cuenta.** El sistema mata la conexión al
pasar a segundo plano y **APNs es el único camino** para despertar una app; Apple no tiene
región europea. Lo que se elige es el contenido, y se elige el mínimo que existe:

- El aviso es una **constante**, `{"aps":{"content-available":1}}`, con
  `apns-push-type: background`. Sin título, sin texto, sin autor, sin canal, sin contador.
- La app despierta y **pide el mensaje a nuestros servidores**, que sí están en la UE, y pinta
  ella la notificación.
- Apple puede saber que ese aparato recibió algo y cuándo. **No qué, ni de quién.**
- Que el cuerpo sea una constante y no algo que se compone es deliberado: impide que nadie le
  añada un campo «solo para depurar» sin ver lo que está publicando. Lo fija el test
  `TestAPNSNoMandaNadaMasQueElDespertar`.

**Consecuencia para lo que prometemos.** «Ningún dato personal sale de la UE» (§1.2 de
`security.md`) deja de ser cierto sin matices en cuanto hay avisos con la app cerrada, y no
solo en iOS: el **Web Push que ya está en producción** enseña el endpoint y los tiempos a
Google, Mozilla o Apple. Se corrige la afirmación en vez de dejarla; un texto legal que no se
cumple es peor que una excepción escrita. Quien no quiera ni eso puede apagar los avisos:
`POST /me/push/apns/forget` y `POST /me/push/unsubscribe` borran el aparato.

**Alternativas descartadas.** _FCM en Android_ (identificador y metadatos a Google sin
necesidad, y no funciona sin Play Services). _Push de contenido en iOS_ con el texto en el aviso
(se lo daríamos a Apple a cambio de ahorrar una petición). _Refresco en segundo plano_ sin APNs
(lo decide iOS, tarda de minutos a horas: no sirve para una llamada ni para un DM).

---

<a id="0040"></a>

## 0040 — @usuario global y conversaciones con personas de otras organizaciones

`accepted` · 2026-09-14 · identidad, rooms, multi-tenancy, privacidad · **revisa el alta del [0038](#0038)**

**Contexto.** Rooms solo hablaba dentro de una organización: cada fila con su `tenant` y su RLS,
y las verjas exigiendo el equipo. Y el alta autoservicio pedía un correo ajeno que había que
verificar antes de entrar. Se pidieron dos cosas: que todo usuario tenga un **@usuario** con el
que le encuentre alguien de otra organización —y que el alta pida ese @usuario en vez de un
correo, porque es la parte delantera de su dirección: **@ana es ana@sytue.eu**—, y poder **abrir
canales, mensajes directos y reuniones con gente de fuera** escribiendo solo su correo o su
@usuario.

**Decisión: el @usuario (Sytue ID).**

1. **`users.username`, global y único sin mayúsculas** (mig 0065). Comparte espacio de nombres
   con las direcciones del dominio raíz, así que sus reglas son las de una parte local de buzón
   (`domain.ValidUsername` es un punto fijo de la normalización de direcciones), los reservados
   son los de `platform/pkg/mailaddr` y **no se asigna un @usuario cuya dirección en el dominio
   raíz es de otra persona**.
2. **El alta pide @usuario, nombre y contraseña.** La cuenta nace con `usuario@<dominio>` como
   correo de acceso y como buzón exacto (`store.ReserveMailbox`; si está cogido, no se crea
   nada), **verificada**, y entra al momento. La verificación del 0038 existía porque la
   dirección era AJENA; esta es nuestra. Y decir «ese @usuario no está disponible» no filtra
   nada: un @usuario es público por definición. Lo que protege el alta sigue siendo el captcha y
   el limitador por IP. Las cuentas antiguas con correo sin verificar siguen como estaban.
3. **Quien no lo elige recibe uno** —invitación, SCIM, SSO—: `nombre.apellidos`, y
   `nombre.apellidos2` si está cogido. Lo pone `store.CreateUser`, que es la única vía por la que
   pasan las seis altas. La columna nace anulable para no romper las altas de los pods del
   binario anterior durante el despliegue; a esas y a todas las cuentas previas se lo da un
   barrido (`AsignarUsernamesPendientes`) con la misma regla, y **las direcciones del dominio
   raíz van primero**. El `NOT NULL` es el paso de contract de la versión siguiente.
4. **Se entra con el correo o con el @usuario** (con o sin arroba).
5. **`POST /api/v1/people:resolve`** encuentra a alguien de cualquier organización, con cuatro
   barreras: exacto (ni prefijos ni listados), lo mínimo (identificador, nombre y @usuario;
   **nunca** el correo de acceso —puede ser un Gmail— ni la organización), por persona y con
   tope, y solo cuentas activas. Es lo que `directory:resolve` no hace a propósito.

**Decisión: conversaciones con gente de fuera (Rooms).**

1. **La conversación vive en la organización de quien la abre**, con sus filas, su RLS y su
   retención. La persona de fuera es una fila más de `channel_members` con **`externo`** (mig
   0036), y esa marca sustituye al requisito del equipo en las verjas de lectura y escritura,
   **solo en ese canal**. Todas las verjas pasan por `requireChannelAccess` y
   `requireWriteAccess`: el cambio está en dos funciones.
2. **Su nombre lo da Sytue ID, no el navegador** de quien la añade: es como la verán todos. Rooms
   pregunta con el token de quien llama, así que el límite por persona de Sytue ID se aplica.
3. **Su token trae SU organización.** Las rutas de un canal, un mensaje o una sala concretos
   pasan a la organización anfitriona cuando allí está como externa (`chathttp.EnAnfitrion`), y
   **sin los roles de su token**: un owner de la suya no es nada en la anfitriona, y con los
   roles puestos el vídeo le daría la moderación y las grabaciones. El tiempo real suscribe cada
   canal en la sala de su organización. La única consulta entre organizaciones es «¿en qué
   organizaciones está esta persona como externa?», y va por una política de mantenimiento de
   **solo lectura que solo ve las filas `externo`**.
4. **Un grupo de personas tiene una sola conversación**, la abra quien la abra: el slug de un DM
   es el del conjunto y no depende de la organización, así que antes de crear se busca donde el
   que abre ya está como externo.
5. **Reuniones.** La llamada de esas conversaciones —DM, grupo o canal— es la reunión con gente
   de fuera, con la misma sala de espera y el mismo bloqueo. Una reunión suelta de la
   organización anfitriona **no** se abre a nadie de fuera por estar en uno de sus DM.

**Revisado el 2026-10-16 (decisión del dueño: «Que acepte primero»).** La persona de fuera ya
no entra al añadirla: recibe una invitación (`invitaciones_canal`, mig 0041 de Rooms, tabla
aparte de `channel_members`) y no lee, no escribe ni le llega nada hasta aceptarla; puede
rechazarla. Vale para canales, DM y grupos, y abrir ella misma el DM es aceptarlo. Quien ya
estaba dentro antes de la 0041 sigue dentro.

**Lo que no se hizo, y por qué** (pendiente en
[`roadmap.md`](roadmap.md#conversaciones-con-personas-de-otras-organizaciones-lo-que-falta)).

- **Adjuntos** en esas conversaciones: el fichero es del Drive de la anfitriona y Drive resuelve
  por el tenant del token.
- **Avisos push** a la persona de fuera: sus suscripciones están en su organización.
- **«Guardados», «Hilos», borradores y programados** de esas conversaciones no salen en sus
  listas, que se leen por organización.
- **Recuperar la contraseña** de una cuenta creada por @usuario: el enlace va a su propio buzón.
  Resuelto en el [0041](#0041).
- **Cambiar el @usuario**, y una **política de organización** para cerrar el contacto con fuera.

**Descartado.** _Pedir además un correo externo_ (devolvía la verificación y la enumeración).
_Un tenant «compartido» para las conversaciones entre organizaciones_ (no tendría dueño que
decidiera retención ni eDiscovery). _Copiar la conversación en cada organización_ (ediciones,
reacciones e hilos divergirían). _Cambiar de organización por una cabecera_ (con una sola fila
externa, quien la tuviera entraría en las reuniones sueltas de la anfitriona, que admiten a
cualquiera de la organización). _Dar a todo el mundo su @usuario como alias en el dominio raíz_ (es una decisión de
correo de cada organización, no un efecto secundario de tener nombre).

<a id="0041"></a>

## 0041 — Móvil verificado por SMS en el alta y cuatro identificadores de acceso

`accepted` · 2026-09-15 · identidad, privacidad, terceros, ux · **revisa el alta del [0040](#0040)**

**Contexto.** Con el [0040](#0040) el alta pedía @usuario, nombre y contraseña, y la cuenta
nacía al momento: nada ataba una cuenta a una persona, y quien perdía la contraseña de una cuenta
creada por @usuario no tenía por dónde recuperarla —el enlace iba a su propio buzón—. Se pidió
que el alta **exija un móvil verificado con un código de un solo uso por SMS**, que el **correo
de recuperación** sea opcional y se ponga en Sytue ID, y que se pueda **entrar con el @usuario,
la dirección de Sytue, el correo de recuperación o el móvil**, siempre con la misma contraseña.

**Decisión.**

1. **La cuenta no nace sin el código.** El alta guarda lo rellenado en `signup_phone_codes` (mig
   0066; la contraseña ya en Argon2id) y manda un código de seis dígitos. Diez minutos, cinco
   intentos —gastados antes de comparar, en la misma sentencia que comprueba el tope—, reenvío
   tras un minuto, y el hash del código ligado a su alta. El SMS lleva la línea
   `@<emisor> #<código>` para que Android e iOS lo rellenen solo en Sytue ID.
2. **Que un móvil ya tenga cuenta no se dice en el formulario**, que sería decirle a cualquiera de
   quién es un número: se manda el SMS igual y se dice tras el código, a quien ha demostrado que
   el número es suyo. Con el correo de recuperación, igual: se dice al abrir el enlace.
3. **Los SMS cuestan dinero y son la puerta del fraude de tarificación.** Además del captcha y el
   limitador del formulario, dos limitadores compartidos: 3 SMS seguidos por número (luego uno
   cada 20 minutos) y 10 por IP (luego uno cada 5).
4. **Sweego manda los SMS** (`api.sweego.io/send`, SMS transaccional). Lo opera Mindbaz SAS
   (Lille) con alojamiento en Francia (OVHcloud y Scaleway); recibe solo el número y el código,
   y el aviso de privacidad lo nombra. Scaleway no tiene API de SMS. Frente a Brevo, que ya
   estaba aprobado para el relé de correo, sale un 20 % más barato: 0,035 € por SMS a España
   (Portugal 0,0225 €, Francia 0,040 €, Alemania 0,078 €; sin IVA, sin cuota), unos 42 € al mes
   con mil altas. Una eSIM propia en un móvil sale más barata a partir de ~300 SMS al mes, pero
   el «SMS ilimitado» de las tarifas es para uso entre personas, no cubre números extranjeros y
   deja el alta en manos de un teléfono encendido.
5. **Sin clave no hay alta.** `SYTUE_ID_SMS_SWEEGO_API_KEY` (secreto `sytue-id`, clave
   `sms-sweego-api-key`; en el piloto, `SMS_SWEEGO_API_KEY` en `.secrets.env`) es opcional para
   arrancar: sin ella, en producción `/signup` responde que el alta no está disponible y el resto
   del IdP sigue igual. En desarrollo el SMS se escribe en el log.
6. **Cuatro identificadores, una contraseña.** `userByLogin` resuelve el móvil (E.164; se admiten
   espacios y el prefijo 00, nunca un número sin prefijo de país), el @usuario, cualquier
   dirección de Sytue de la persona —buzón o correo de acceso— y el correo de recuperación, en ese
   orden: una dirección de un buzón gana a la recuperación de otra cuenta. Vale igual para
   passkeys y «olvidé mi contraseña», que manda el enlace al correo de acceso **y** al de
   recuperación.
7. **El correo de recuperación pide la contraseña y se confirma por enlace** (`PUT
/api/v1/me/recovery-email`, tarjeta «Acceso y recuperación» de Mi cuenta). Con una sesión
   robada, poner un correo propio y pedir el reseteo sería quedarse la cuenta. No vale una
   dirección del dominio de la suite ni el correo de acceso; al confirmarlo, una dirección que ya
   da acceso a otra cuenta se rechaza. Los enlaces de verificación llevan ahora su propósito.
8. **El login con contraseña lleva el mismo captcha ALTCHA que el alta** (`LOGIN_CAPTCHA_ENABLED`,
   activo por defecto; reto en `/login/captcha`). Se resuelve solo al pulsar «Continuar» —la
   casilla ya no es obligatoria en ninguno de los dos— y el servidor lo comprueba antes de mirar
   la cuenta, así que un envío sin reto no gasta un Argon2id ni el cupo de intentos de nadie.
   Las passkeys no lo llevan: no hay nada que adivinar.
9. **El alta es un recorrido de tres pasos con su barra de progreso**: tus datos (una frase bajo
   cada campo que dice para qué es), el código del móvil y **«Todo listo»**, que resume @usuario,
   correo y móvil, recuerda que se entra con cualquiera de ellos y lleva a **Sytue Hub** —o a la
   app que pidió entrar, si el alta empezó en una—.

**Lo que no se hizo, y por qué** (pendiente en
[`roadmap.md`](roadmap.md#conversaciones-con-personas-de-otras-organizaciones-lo-que-falta)).
Cambiar el móvil desde Mi cuenta (pide volver a verificar el número nuevo), pedir un móvil a las
cuentas anteriores, y un tope global de SMS por hora.

**Descartado.** _Adivinar el prefijo de país_ por el idioma o la IP (daría de alta el número de
otra persona). _Decir en el formulario que el móvil ya tiene cuenta_ (enumeración de teléfonos).
_Negarse a arrancar sin clave de SMS_, como con el SMTP (el día del despliegue tiraría el IdP
entero por una función que se puede apagar sola). _Recuperar la contraseña por SMS_ (no se pidió,
y el SMS es el factor más fácil de secuestrar con un duplicado de SIM). _Una pregunta por pantalla_
en el alta (obligaba a llevar la contraseña de una pantalla a otra dentro de la página o a guardar
estado por paso; tres pasos dan la misma claridad).

<a id="0042"></a>

## 0042 — España sin gestoría: el banco entra por Norma 43 y presentar por ti espera al certificado y a VeriFactu

`accepted` · 2026-09-15 · erp, fiscal, banca, ux · diseño completo en [`architecture.md` §7.5](architecture.md#75-españa-sin-gestoría-banco-obligaciones-guiadas-y-presentación)

**Contexto.** Se pidió que un empresario sin conocimientos de impuestos, contabilidad ni
informática pueda llevar todas sus obligaciones desde Sytue ERP: guiado paso a paso, con avisos
semanales, con una fecha propia cuatro días hábiles antes de la que da la administración, con
cada movimiento del banco convertido en tarea y casado solo cuando se pueda, integrado con los
bancos y las administraciones españolas, con el certificado de la empresa y presentando «lo que
haya» en la fecha límite. Y que se diseñe para empresarios, no para gestorías.

**Decisión.**

1. **El banco entra por Norma 43, no por PSD2.** Todos los bancos españoles lo dan desde la
   banca de empresas y leerlo no pide contrato ni licencia; PSD2 exige un proveedor con licencia
   de información de cuentas, con coste, contrato y datos que salen a un tercero. El casado es el
   mismo para los dos: cuando se elija proveedor, sus movimientos entran por él.
2. **Cada movimiento es una tarea hasta que tiene factura o motivo**, y se casa solo con criterio
   de contable prudente: importe exacto, fecha razonable, una prueba en el concepto y ninguna
   alternativa igual de probable, y solo contra facturas confirmadas. Un cobro mal asignado es
   peor que una pregunta. Todo se deshace con un asiento anulado, nunca borrando.
3. **La fecha que se enseña es «tu fecha»**: cuatro días hábiles antes del último día útil, que
   es el de domiciliación cuando el pago se puede domiciliar. Los festivos salen de la resolución
   anual del BOE; el límite legal se corre solo con los nacionales y el margen cuenta también los
   de la comunidad, que es el lado seguro en los dos casos.
4. **Recordatorio semanal por calendario importado**, no suscrito: Google descarta siempre las
   alarmas de un calendario suscrito y Apple lo hace por defecto. El aviso por correo o push
   espera a que el ERP tenga identidad de servicio.
5. **Presentar por ti queda en fase 2, detrás de VeriFactu y del certificado.** Presentar exige la
   conformidad de quien declara, lo presentado no se anula y el fichero de cada modelo hay que
   validarlo contra la Agencia Tributaria antes de enviarlo; hacerlo sin eso sería presentar a
   ciegas en nombre de alguien. VeriFactu va antes porque es obligatorio para el propio producto
   desde el 1 de enero de 2027.

**Consecuencias.** El ERP ya calcula el 130 como el formulario (acumulado menos lo pagado) y sabe
qué le falta a cada obligación. El dato de España crece: calendario laboral, territorios, guías y
el 184. Una sociedad vasca o navarra ve la Seguridad Social y el Registro, no sus impuestos, y la
pantalla le dice por qué.

**Lo que no se hizo, y por qué.** Certificado, presentación directa y DEHú (fase 2 y 3 en
§7.5); el proveedor PSD2 (decisión de contrato y privacidad); el adjunto del PDF de la factura
(el catálogo no tiene tipo fichero); los tipos de IVA por línea (hacen falta para el 303
presentado, no para el calculado); y separar en el cargo de la Tesorería la cuota del autónomo de
la de sus trabajadores (hoy va entera a 642).

**Descartado.** _Suscripción webcal_ con token de capacidad (ruta pública, ingreso nuevo y, aun
así, sin alarmas). _Casar por importe solo_ (dos recibos iguales del mismo mes). _Presentar sin
mandato registrado_. _Contar festivos regionales para correr el límite legal_ (si la
administración no los aplica, se presentaría tarde).

<a id="0043"></a>

## 0043 — El ERP es de toda la organización: permisos por módulo, tareas y documentos, y Sytue AI que crea con confirmación

`accepted` · 2026-09-15 · erp, ia, permisos, correo · diseño en [`architecture.md` §7.6](architecture.md#76-el-erp-de-toda-la-organización-permisos-tareas-documentos-y-sytue-ai-que-crea)

**Contexto.** Se pidió que Sytue AI cree registros de cualquier módulo, varios a la vez y con
todo lo relacionado (un gasto de Claude Code de 180 € crea el proveedor Anthropic si no existe),
consultando internet para los datos, y que deje la tarea de subir el PDF con vencimiento hoy; que
en cada cargo del banco diga desde dónde se descarga la factura o a quién pedírsela y con qué
correo; que el ERP sea de toda la organización, con permisos por módulo al estilo de Odoo desde
Sytue Admin; que al subir una factura o un documento se diga qué tiene que llevar; que
Obligaciones esté siempre activo tras el asistente; y que el aviso de los diez días llegue por
correo a quien tenga permiso en Obligaciones.

**Decisión.**

1. **Proponer y confirmar, en dos pasos.** La IA propone un plan; el servicio lo valida, lo
   completa con lo que ya existe y lo comprueba; la persona lo ve y lo crea en una transacción.
   Guardar lo que conteste el modelo pondría en el libro datos que nadie ha mirado.
2. **Internet son comprobaciones, no navegación.** VIES para el NIF-IVA, HEAD/GET para páginas y
   MX para correos, desde el servicio y con barrera SSRF. El modelo no busca en la web: Sytue AI
   no navega y un buscador sería un tercero nuevo con las consultas de la organización. Lo que el
   modelo sabe de memoria se comprueba y lo que no se confirma se deja vacío.
3. **Los permisos viven en el ERP y se editan en Sytue Admin.** El ERP es quien los aplica en cada
   petición y los módulos son de su catálogo. Cuatro niveles como los grupos de acceso de Odoo,
   un defecto por módulo y solo lo distinto guardado.
4. **Tareas y documentos en tablas propias**, no como campos del catálogo: cuelgan de cualquier
   registro y también de las obligaciones, que no son registros, y subir el documento cierra la
   tarea. Los ficheros en Postgres con tope de 10 MiB: una factura pesa cientos de kilobytes y
   así heredan RLS y copia de seguridad sin otro almacén ni otro permiso.
5. **El correo sale por el MSA propio con un buzón de servicio**, desde un planificador dentro del
   servicio que reclama cada envío en la base de datos: una vez por obligación y periodo aunque
   haya varias réplicas.

**Consecuencias.** Un miembro nuevo ya no ve la contabilidad ni las nóminas hasta que se le dé
acceso; owner y admin no notan nada. El ERP necesita un buzón de servicio para avisar y, sin él,
no avisa. El VIES recibe el NIF-IVA de los proveedores europeos que proponga la IA.

**Lo que no se hizo, y por qué.** Reglas por fila (las «record rules» de Odoo: ver solo lo propio
dentro de un módulo), historial de cambios por registro y seguidores, OCR del PDF subido para
rellenar la factura, y leer el correo de facturas entrante. Van después en la paridad con Odoo.

**Descartado.** _Crear directamente lo que conteste el modelo_. _Un buscador web para la IA_
(tercero nuevo y sin forma de comprobar lo que devuelve). _Guardar los PDF en Drive o S3_ (otro
almacén y otro permiso para ficheros pequeños que son de la organización, no de una persona).
_Permisos como scopes de Sytue ID_ (el ERP tendría que preguntar a ID en cada petición por
módulos que ID no conoce). _Aviso por push en vez de correo_ (se pidió correo, y el push solo
llega a quien tiene la app instalada).

<a id="0044"></a>

## 0044 — VERI*FACTU dentro del ERP: la base garantiza la cadena y el envío sale con un certificado del servicio

`accepted` · 2026-09-15 · erp, fiscal, verifactu · diseño en [`architecture.md` §7.7](architecture.md#77-verifactu-cada-factura-registrada-en-la-agencia-tributaria-al-emitirla)

**Contexto.** Desde el 1-1-2027 (sociedades) y el 1-7-2027 (resto), todo programa con el que un
empresario español factura tiene que cumplir el Reglamento de sistemas informáticos de
facturación, y quien lo produce responde con hasta 150.000 € por año y tipo de programa si no.
Se pidió avanzar en todo lo necesario con la modalidad VERI*FACTU.

**Decisión.**

1. **Solo VERI\*FACTU**, sin la modalidad que conserva y firma registros sin enviarlos: el
   producto es en la nube, la Agencia Tributaria guarda la copia que da fe, y sin firma de
   registros ni registro de eventos hay menos que puede fallar.
2. **El registro se genera en la transacción que emite la factura** y **la base impide lo que no
   se puede deshacer**: registros inalterables, cadena sin bifurcaciones y factura emitida sin
   retoques, con triggers y restricciones en la migración. Así un camino nuevo que emita una
   factura sin pasar por `emitirVerifactu` falla en vez de dejar un hueco.
3. **Una cadena y un número de instalación por organización**, porque cada una es un obligado
   distinto (FAQ de la AEAT para desarrolladores, apartado 4): `TipoUsoPosibleMultiOT=S` e
   `IndicadorMultiplesOT=N`.
4. **El envío sale desde syerp con TLS mutuo y un certificado del servicio**, no por el
   Integration Gateway: es SOAP con certificado de cliente a un organismo público, y llevar el
   certificado al gateway sería otro sitio donde guardarlo. El control de flujo lo guarda la base
   para que dos réplicas no se salten la espera.
5. **La factura dice lo que falta antes de emitirse** (NIF, tipo de IVA vigente, régimen sin IVA):
   lo que la AEAT rechazaría después obliga a subsanar, y un empresario sin conocimientos no sabe
   qué es un alta por rechazo.

**Consecuencias.** Una factura confirmada ya no se edita ni se borra: se corrige con una
rectificativa o, si la AEAT la rechazó, se corrige y se vuelve a enviar. Los contactos necesitan
NIF para facturarles. El ERP tiene por primera vez una factura imprimible.

**Lo que falta, y es del dueño.** El **certificado**: de Sytue como colaborador social (convenio
con la AEAT, que permite presentar por los clientes sin apoderamiento uno a uno) o como apoderado
de cada cliente; sin él no se envía nada. **Suscribir la declaración responsable** de cada
versión (`verifactu.DeclaracionFecha`), sin la cual producción no se puede activar.

**Lo que no se hizo, y por qué.** Factura simplificada (F2): el modelo exige cliente. IGIC e
IPSI: Canarias, Ceuta y Melilla quedan fuera hasta que las facturas tengan su impuesto. Varias
líneas con tipos distintos: la factura del ERP tiene una base y un IVA. Rectificativa por
sustitución y facturas emitidas por terceros. Consultar en la AEAT lo presentado.

**Descartado.** _Generar el registro después, en segundo plano_ (la norma lo pide al expedir y la
factura podría salir sin él). _Guardar el certificado de cada cliente_ (un empresario sin
conocimientos no sabe exportarlo y custodiarlos es un riesgo que no hace falta). _Pasar el
envío por el Integration Gateway_. _Dejar editar una factura aceptada y subsanar sin más_ (la
norma solo admite subsanar lo que no exige rectificativa, y distinguirlo no es cosa de quien
factura).

<a id="0045"></a>

## 0045 — Una sola interfaz en Gio para web, escritorio y móvil: sytue.com en Gio, sytue.eu en la actual

`superseded` → [0048](#0048) en escritorio y móvil · 2026-09-20 · frontend, escritorio, móvil, gio · código en `apps/gio/`

> **Superado el 2026-09-21 por el [0048](#0048)**, que devuelve escritorio y móvil a Fyne con
> widgets propios y deja la web en Astro + Preact. `apps/gio` **no se borra**: sirve hoy
> `sytue.com` en producción y se queda hasta que el dueño decida qué publica ese dominio. Lo de
> abajo se conserva porque explica de dónde vienen `apps/gio/` y el reparto de ficheros.
>
> **Sustituido por Fyne y retirado del repositorio el 2026-09-23**: la suite en Gio, su
> servidor web y el escritorio de Wails que la abría salen del árbol (`sytue.com` ya es un 301
> a sytue.eu). La copia del código queda en `gio-archivado/`, fuera de git.

**Contexto.** Se pidió migrar la suite entera —las catorce apps Astro + Preact de `apps/frontend/`
y el escritorio Fyne de `apps/desktop/shell/gui/`— a [Gio](https://gioui.org), un único código Go
que compila a WebAssembly, escritorio y móvil, «píxel a píxel» y sin webview. Medido antes de
empezar: son **unas 440.000 líneas** de aplicación más 36.000 de paquetes compartidos. No es una
tarea; es un programa de trabajo, y este ADR deja escrito a dónde va, en qué orden y con qué
reglas.

**Decisión.** La tomó el dueño el 2026-09-20, con los cuatro costes de más abajo a la vista:

- **Gio es el destino en los tres sitios** —web, escritorio y móvil—, con **un solo código**. No
  «Gio en nativo y Astro en la web»: la razón de hacerlo es dejar de mantener dos interfaces.
- **Primero, convivir en el repositorio.** `apps/gio/` vive junto a `apps/frontend/` y
  `apps/desktop/`, y no se borra nada de lo que hay. Mientras dure, la web es la fuente de tokens,
  catálogos y reglas, y Gio la lee o la compara.
- **Y al acabar, convivir en producción, por dominio.** Cuando las fases estén completas,
  **`sytue.com` publica la versión Gio entera y `sytue.eu` sigue con la actual** (Astro + Preact
  - Go). No hay relevo que borre nada: son dos frentes sobre los mismos servicios y los mismos
    datos, y el [0004](#0004) sigue vigente para `sytue.eu`.

  > **Esto dejó de ser cierto el 2026-09-21.** El port a Gio se canceló —escritorio y móvil van
  > a Fyne, la web sigue en Astro + Preact + Go— y `sytue.com` pasa a servir **la suite Astro**,
  > con un segundo juego de imágenes desplegado en el frontal: [ADR 0049](#0049). Lo demás de
  > este ADR (el reparto de ficheros, `apps/gio/`) se conserva porque explica de dónde viene lo
  > que sigue en el repositorio.

- **Fyne se sustituye, no se borra en seco.** Es hoy el cliente de Windows y Linux, y solo
  `shell.Run` lo llama (`gui.RunGUI`). Desde el 2026-09-20 queda **congelado** —ni se amplía ni se
  arregla— y desaparece en la MISMA PR que ponga la shell de Gio en ese punto, para que no haya una
  release sin escritorio. La tubería de publicación, `bundles.json` y las descargas no se tocan.
  El contrato declarativo de pantallas (`packages/ui/src/native/`, el árbol `NodoUI`) se conserva:
  cambia quién lo pinta.
- **No es un calco: se porta y se mejora.** Lo que el puerto vaya encontrando mejorable —función,
  interfaz, experiencia o diseño— se implementa por el camino, y queda apuntado en
  [`roadmap.md` §Gio](roadmap.md#gio-mejoras-identificadas-durante-el-puerto) con su decisión:
  qué se cambió, si va solo en Gio o también en la suite actual, y por qué. Una mejora sin
  apuntar es una divergencia entre los dos dominios que nadie sabrá explicar. Donde NO se toca
  nada es en lo que tiene que dar el mismo resultado: fórmulas, plurales, fechas, permisos.
- `ci-gio.yml` vigila el módulo —build nativo y WASM, vet, test sin ninguno saltado, lint y
  catálogos al día—, porque `go build ./...` en la raíz no entra en un módulo aparte.

**Con qué reglas se construye (fase 1, ya hecha, sin interfaz).**

1. **Mandan las fuentes, no el encargo.** El encargo traía valores que no están en el código y
   se portó lo que hay: la curva estándar es `(0.2, 0, 0, 1)`, `fade-in` es solo opacidad,
   `drawer-in` entra desde −100 % sin tocar la opacidad, el pulso del skeleton es 1 → 0,55 → 1,
   `accent-700` es `#224656`, son 26 idiomas y no 24, los cortes son los de Tailwind (640/768/1024)
   y Gio estable es la v0.10.2. También pedía APIs que son de Fyne y no de Gio
   (`canvas.Rectangle`, `app.Settings().ThemeVariant()`).
2. **Lo que se puede leer no se copia** ([0037](#0037)). Los colores salen de la MISMA hoja que
   la web: `scripts/sync-branding.sh` deja una copia de `branding/tokens/colors.css` y
   `internal/theme` la interpreta al arrancar, con el tema oscuro como reasignación sobre `:root`.
   Lo que sí hubo que escribir a mano (escala, radios, sombras, keyframes, reglas de caché,
   claves de catálogo) tiene un test que busca cada valor, literal, en su fuente de la web.
3. **Donde la web se apoya en el navegador, la referencia es el navegador.** Plural y formato de
   números se comparan contra vectores sacados del `Intl` real: `golang.org/x/text` lleva CLDR 32
   y ya no contesta lo mismo (maltés, islandés, «1234» sin punto en español, U+202F en francés).
4. **Una sesión y un margen de caducidad.** Nativo: Authorization Code + PKCE al loopback con el
   cliente `sytue-desktop` de la migración 0069, y el refresh token en el llavero del sistema. Web:
   **el token no baja al navegador** —ni a memoria—; lo guarda el BFF y lo pone él (ver la
   partida 3). El encargo pedía `sessionStorage` y un redirect `sytue://` que el IdP no tiene
   registrado; no se hizo ninguna de las dos cosas.
5. **Se porta lo que existe.** `@sytue/offline` no tiene cola de mutaciones; se portó el almacén
   por dueño, la conectividad y la caché «red primero» con sus reglas de privacidad.

**Dos hallazgos medidos, que valen aunque esto no siga.**

- **En la web 1 rem son 14 px, no 16.** `theme.css` fija `html { font-size: var(--text-base) }`
  y eso reescala todo lo declarado en rem: tipografía y espaciado de Tailwind salen al 87,5 % de
  lo que dicen los comentarios de la hoja. Medido con Playwright contra `www.sytue.eu`: `html`
  14 px y el `h1` de `--text-4xl` a 42 px, no a 48. `text-sm` se ve a 11,4 px. Gio porta lo que
  se ve (`theme.Rem = 14`); si resulta que fue un accidente, es una línea en cada lado.
- **Los `Figtree-*.ttf` del escritorio Fyne son páginas HTML de GitHub**, no fuentes
  (`750188b16`: se bajó la URL `blob`). El binario lleva 1,3 MB de HTML embebidos como tipografía.

**Lo que la decisión obliga a construir, y no estaba en el encargo.** No son motivos para no
hacerlo —ya está decidido—; son las cinco partidas que más pesan, para que ninguna aparezca como
sorpresa.

1. **Un editor.** `gioui.org/x/richtext` pinta texto con estilos; **no edita**. Docs, Hojas,
   Diapos, PDF y Notas son 90.000 líneas sobre `contenteditable`. En Gio no se portan: se escribe
   un motor de edición y composición de texto. Es la partida más grande del programa.
2. **Accesibilidad y SEO de la web.** Gio en WASM pinta en un `<canvas>`: sin lector de pantalla,
   sin selección ni búsqueda del navegador, IME limitado y nada que indexar. La web en Gio
   necesita un espejo semántico en el DOM (el árbol de `io/semantic` de Gio, que en escritorio ya
   alimenta a los lectores) y `www` necesita HTML que un buscador pueda leer. Hasta que exista y
   se mida, esas superficies no se relevan: la Directiva (UE) 2019/882 obliga desde junio de 2025.
3. **El BFF.** Las apps Astro no son solo interfaz: guardan la sesión tras la cookie httpOnly. El
   de Gio es `services/gioweb`, **hecho** ([arquitectura](architecture.md#gioweb)): sirve el
   WebAssembly y guarda la sesión SELLADA (AES-256-GCM) en una cookie `__Host-` httpOnly. Va un
   paso más allá que el de Astro, que entrega el token a la página por `/auth/token`: aquí la API
   se pide al mismo origen (`/svc/<producto>/api/…`) y el `Authorization` lo pone el servidor, así
   que ningún script de la página alcanza un token y los servicios no necesitan CORS nuevo.
4. **El login en el móvil.** iOS y Android no pueden volver a un loopback; piden un cliente OIDC
   con redirect propio, o sea una migración en `services/id`.

5. **`sytue.com` como segundo frente.** Hoy `sytue.com` responde 301 a `sytue.eu`
   (`sytue-com-redirect`, `bc3000c15`): el día de publicar hay que retirarlo. La cookie de sesión
   de la suite es de `.sytue.eu` y no viaja a `.sytue.com`, así que el frente Gio necesita su
   propio BFF y su cliente OIDC con redirect en `sytue.com`. **Hecho, salvo el relevo:** UN
   cliente confidencial para toda la suite (`sytue-gio`, migración 0070, con su audiencia en
   `firstPartyAudiences` —sin eso todos los servicios contestan 401, que es lo que le pasó a
   Fotos—); UN host canónico, porque la cookie `__Host-` es de un solo host y el `redirect_uri`
   es literal (los alias reciben un 308 hacia él); los servicios no aceptan ningún origen nuevo,
   porque el navegador solo habla con `gioweb`; y `sytue.com` ya está en el inventario.
   **Mientras el puerto no esté completo vive en `gio.sytue.eu`**, que entra por el comodín del
   frontal sin tocar nada, y `sytue.com` sigue con su 301. El relevo es cambiar `GIO_HOST` y el
   frontal: [runbook](operations.md#gio-el-segundo-frente).

Esto **sustituye** a lo decidido el 2026-09-13 (PR #429), que entendía «nativo» como lo que rodea
a la ventana de una web: con Gio la interfaz entera es nativa.

**Consecuencias.** `apps/gio/` es un módulo aparte, dentro de osv-scanner y govulncheck. Nada de
la web actual cambia por esto, salvo los defectos que el puerto destape en ella, que se arreglan
también allí.

**Lo que falta de la fase 1.** El motor de fórmulas: 11.400 líneas y unas 360 funciones, con 1.877
aserciones en sus tests. Portarlo «casi igual» es peor que no portarlo —una hoja de cálculo que
diverge en el decimoquinto decimal no avisa—, así que va con el mismo método que `Intl`: vectores
generados por el motor TypeScript y el puerto Go comparado contra ellos, fichero a fichero.
También el formato de fechas (no hay CLDR de fechas en `x/text`) e Inter para griego y búlgaro.

**Descartado.** _bbolt para el almacén offline_ (bloquea su fichero para un proceso y la suite
son varios binarios a la vez). _Un espejo tipado de los colores_ (es lo que ya son `tokens.ts` y
el `theme.go` de Fyne, y los dos hay que acordarse de tocarlos). _`x/text/message` para los
números_ (CLDR 32). _Guardar el access token_ (dura minutos y con él la entrada no cabe en el
Credential Manager de Windows).

<a id="0046"></a>

## 0046 — Los medios de Rooms no se escriben en Go: el transporte sí, los códecs y la captura no

`accepted` · 2026-09-21 · rooms, gio, móvil, escritorio, medios · acota el [0045](#0045), no lo
cambia · guardián en `apps/gio/internal/medios/vp8_test.go`

**Contexto.** El [0045](#0045) manda que la interfaz sea Gio en los tres sitios, sin webview. Hoy
las videollamadas de Rooms las hace el navegador: `apps/mobile/rooms` (Capacitor) y
`apps/desktop/rooms` (Wails) no tienen **ni una línea** de `livekit|webrtc|getUserMedia|RTCPeer`;
quien habla con el SFU es `livekit-client` dentro del webview. Se pidió evaluar —y si salía,
escribir— un cliente WebRTC/LiveKit nativo en Go con captura de cámara, micrófono y pantalla, para
quitar ese webview. Esto es lo que se midió antes de escribir nada.

**Lo medido** (2026-09-21, piloto: Ryzen 5 5600X, Go 1.27.1; SFU LiveKit v1.13.7, el pin del repo).

_1. El transporte funciona, y es barato._ `github.com/livekit/server-sdk-go/v2` v2.18.1 sobre
`pion/webrtc` v4 conecta a un LiveKit real: señalización, ICE, DTLS y SRTP, y recibe las pistas de
los demás. Con un publicador escribiendo muestras VP8 ya codificadas a 30 fps, el suscriptor gasta:

| participantes | pistas | CPU (1 núcleo) | RSS    | recibido  |
| ------------- | ------ | -------------- | ------ | --------- |
| 1             | 2      | 1,1 %          | 38 MiB | 1,6 Mbps  |
| 4             | 8      | 3,4 %          | 39 MiB | 6,4 Mbps  |
| 9             | 18     | 8,6 %          | 41 MiB | 14,5 Mbps |
| 16            | 32     | 14,6 %         | 42 MiB | 25,7 Mbps |

Lineal, ~0,9 % de un núcleo por participante y memoria plana. Emitir cuesta ~1,6 % de CPU y
40-60 MiB por conexión de sala. El binario mínimo —sin interfaz y sin captura— son **26,5 MiB**
con `-s -w`, y arrastra **193 módulos**. Compila con CGO desactivado a Linux, macOS y Windows;
**Android necesita `-ldflags=-checklinkname=0`** porque `wlynxg/anet` v0.0.5 (vía `pion/ice`)
enlaza por `//go:linkname` a `net.zoneCache`, que Go 1.27 ya no permite; `js/wasm` no compila
(paquete `e2ee`).

_2. No existe ni un códec de vídeo en Go puro, ni para descodificar._ `golang.org/x/image/vp8` es
el único descodificador VP8 en Go, y **rechaza todo fotograma que no sea clave**: «Golden / AltRef
frames are not implemented», con el comentario de upstream «only used for video, not still
images». Es el descodificador de WebP. En una videollamada uno de cada sesenta fotogramas es
clave, así que no descodifica nada. `pion/opus` v0.1.0 solo descodifica: tampoco hay codificador
Opus. El propio pion lo dice: lleva empaquetadores, no códecs, y su `getUserMedia` «requires Cgo».

_3. Con cgo el rendimiento sobra; el problema no es la velocidad._ libvpx descodifica 720p en
**0,74 ms** por fotograma y 360p en **0,23 ms** (un hilo). Nueve participantes a 360p30 son el
6,2 % de un núcleo. Lo que falta no es potencia: es que ese código está en C y hay que cruzarlo a
cinco plataformas.

_4. La captura no está escrita para las plataformas que importan._ Gio v0.10.2 **no tiene nada de
medios** —ni audio, ni cámara, ni pantalla— aunque sí expone el puente JNI en Android.
`pion/mediadevices` v0.10.0 cubre cámara en Linux/macOS/Windows, micrófono por miniaudio y
pantalla **solo en X11**; todo con cgo, y **sin un solo driver de Android ni de iOS** (con
`GOOS=android` selecciona por error el fichero de X11 y no compila). Lo que habría que escribir:

|                | Android          | iOS          | macOS            | Linux           | Windows      |
| -------------- | ---------------- | ------------ | ---------------- | --------------- | ------------ |
| cámara         | Camera2 / JNI    | AVFoundation | mediadevices     | v4l2            | mediadevices |
| micrófono      | AudioRecord      | AudioUnit    | miniaudio        | miniaudio       | miniaudio    |
| altavoz        | AAudio           | AudioUnit    | por escribir     | por escribir    | por escribir |
| pantalla       | MediaProjection  | ReplayKit    | ScreenCaptureKit | PipeWire+portal | DXGI         |
| codificar V    | MediaCodec       | VideoToolbox | libvpx           | libvpx          | libvpx       |
| descodificar V | MediaCodec       | VideoToolbox | libvpx           | libvpx          | libvpx       |
| Opus           | libopus          | libopus      | libopus          | libopus         | libopus      |
| eco (AEC/NS)   | APM de libwebrtc | ídem         | ídem             | ídem            | ídem         |

Cuarenta casillas; `mediadevices` cubre ocho, y ninguna de las dieciséis de móvil. La fila del
eco es la que más se olvida: sin cancelación de eco una llamada con altavoz es inusable, y la
única implementación seria es el APM de libwebrtc, que es C++.

_5. Nadie lo hace así, ni LiveKit._ Su SDK de cliente en Rust no implementa medios: depende de
`libwebrtc` (vía `webrtc-sys`), y por debajo están también los de Flutter, Swift y Kotlin. El de
Go se llama **server**-sdk-go precisamente porque está pensado para agentes de servidor, que no
capturan nada.

**Decisión.** **No se escribe un cliente de medios en Go.** El encargo era «evaluar y, si la
medición lo respalda, implementar»; no lo respalda. Se para aquí, con la medición escrita, y
Rooms sigue haciendo las llamadas como hoy. Esto **acota** el [0045](#0045): la interfaz de Rooms
se porta a Gio como el resto de la suite; la **llamada** no, y no es deuda de nadie.

**Si algún día hay que hacerlo, el camino no es Go puro.** Es cgo contra libwebrtc —por
`livekit-ffi`, que es lo que ya usa el SDK de Rust—, aceptando lo que eso trae: una biblioteca
nativa por arquitectura, una cadena de compilación cruzada por plataforma, y las clases Java de
Android conviviendo con la actividad de Gio. Es un encargo propio, no un remate de este.

**Y la condición que cualquier implementación futura tiene que cumplir.** Captura de cámara,
micrófono o pantalla **solo con consentimiento continuo y visible**, permiso pedido en el momento
de usarlo y no antes, explicando función, dato, duración e impacto, y el rechazo degradando de
forma segura —entrar a la llamada sin cámara, no quedarse fuera—. Es lo que ya hace la pantalla
previa de la web (`PreJoin`): se publica **exactamente** lo que el usuario eligió, en vez de
llamar a `enableCameraAndMicrophone()`.

**Cuándo reconsiderarlo, y quién avisa.** `apps/gio/internal/medios/vp8_test.go` descodifica un
interfotograma VP8 real con `golang.org/x/image/vp8` y comprueba que falla por el motivo medido.
El día que upstream implemente Golden/AltRef, ese test se pone en rojo: es la señal de reabrir
esto, no de borrar el test. Lo otro que lo reabriría es que aparezca un codificador Opus o VP8/AV1
en Go puro, o drivers de Android/iOS en `pion/mediadevices`.

**Descartado.** _Implementar solo la recepción_ (sin descodificador tampoco se pinta, y media
llamada no es una llamada). _`pion/mediadevices` tal cual_ (no tiene móvil, y su pantalla es
X11: ni Wayland, ni macOS, ni Windows). _Seguir con el webview dentro de Gio_ (lo prohíbe el
encargo, y con razón: sería la tercera interfaz).

**Enmienda del 2026-09-25 — se reabre, y por la puerta que el propio ADR dejaba.** El dueño
decidió llevar audio y vídeo a la app nativa de Rooms (ahora Fyne, [0048](#0048)) en escritorio y
en móvil. Dos de las condiciones de «cuándo reconsiderarlo» se cumplen ya: hay **codificador Opus
en Go puro** (`github.com/tphakala/go-opus` v1.1.0, BSD-3, transliteración de libopus;
codifica solo CELT, que a 32 kbps mono basta para voz, y descodifica los tres modos) y **VP8 en Go
puro con interfotogramas** (`github.com/thesyncim/govpx`, BSD-3, codificador y descodificador;
en su medida, 1,8 ms por fotograma de 720p al descodificar). La fila del códec deja de ser C.

Lo que queda escrito así:

- **Transporte**: `livekit/server-sdk-go/v2` v2.18.1 sobre `pion/webrtc` v4.2.21, como se midió
  arriba. `livekit/protocol` se queda en v1.49.0 porque v1.52 rompe la compilación del SDK.
  Android sigue pidiendo `-ldflags=-checklinkname=0` (anet); lo pone `empaquetar.sh` por GOFLAGS.
- **Micrófono y altavoz**: miniaudio por `gen2brain/malgo` (cgo sin bibliotecas que enlazar),
  que cubre las cinco plataformas —CoreAudio, WASAPI, PulseAudio/ALSA, AAudio/OpenSL ES—. Es la
  única pieza con C, y va aparte (`apps/fyne/llamada/altavoz`) para que el resto se pruebe sin
  tarjeta.
- **El token y el SFU son los de la web**: `POST /rooms/{id}/join` del servicio (y
  `from-source`/`by-source` + `knock` para los canales), sin endpoint propio. Los medios van al SFU
  de la suite, no a terceros.
- **Guardián**: `apps/fyne/llamada/sfu_test.go` levanta el mismo `livekit-server` que se despliega
  y comprueba que la voz de una persona llega como tono a la otra, que su cámara llega como imagen
  y que colgar libera el sitio en el acto. Se vio en rojo quitando cada una de las tres cosas.

**Segunda enmienda, 2026-09-26: la captura también.** La cámara y la pantalla se leen con la
API de cada sistema (AVFoundation y ScreenCaptureKit, Media Foundation y GDI, V4L2 y X11,
Camera2), en `apps/fyne/llamada/camara`, y el eco lo quita el sistema: VoiceProcessingIO en macOS
e iOS y el DSP de captura de voz en Windows. Es la primera fila de la tabla de arriba que se
escribe; la de la pantalla del móvil sigue sin estar. Lo que se ha probado y dónde, en el roadmap.

**Lo que sigue sin estar, y por qué** (el detalle, en [roadmap](./roadmap.md#llamadas-nativas)):
la **captura de cámara y de pantalla** —la fila de «cámara» y «pantalla» de la tabla sigue siendo
por plataforma y no tiene código en Go—; la **cancelación de eco** en escritorio —se pide la del
sistema donde se puede (AAudio «comunicación de voz», categoría «reproducir y grabar» de iOS), y
en escritorio no la hay: con altavoces, auriculares—. Recibir la cámara y la pantalla de los demás
sí funciona, porque solo pide descodificar.

**La condición del consentimiento sigue en pie**: la antesala publica solo lo que se eligió, el
permiso del micrófono se pide al abrirla (Android) o al abrir la captura (macOS e iOS, con
`NSMicrophoneUsageDescription`), y sin permiso se entra a escuchar. El test de `vp8_test.go` que se
citaba arriba se fue con Gio; su papel lo hace ahora `sfu_test.go`.

---

<a id="0047"></a>

## 0047 — Los avisos de Android salen del SSE del Hub, y el empaquetador va parcheado hasta que gogio lo acepte

`accepted` · 2026-09-21 · móvil, gio, avisos, empaquetado, soberanía · **implementa el
[0039](#0039)** para Android

**Contexto.** El [0039](#0039) ya decidió por dónde entra un aviso en Android: el **flujo SSE
del Hub** que la web consume con `EventSource`, sostenido por un **servicio en primer plano**.
Sin FCM, sin Play Services y sin ningún identificador fuera de la UE. Lo que faltaba era poder
hacerlo, y lo que lo impedía no era el código nuestro sino **gogio**: escribe él el
`AndroidManifest.xml` desde una plantilla cerrada donde no cabe ni `POST_NOTIFICATIONS`
—obligatorio desde `targetSdk 33`, y los APK salen con 35— ni un `<service>`.

Mientras tanto, `@capacitor/push-notifications` en `apps/mobile/rooms` era **el único sitio de
toda la suite donde un mensaje avisa con la app cerrada**, y por eso ese envoltorio no se podía
apagar.

**Decisión 1: parche, no bifurcación, y con huecos genéricos.** `gogio-avisos.patch` son **doce
líneas** repartidas en tres ficheros de gogio, y no añaden una bandera por caso sino **dos
huecos**: `-permissions` (permisos extra tal cual) y `-appmanifest` (XML literal dentro de
`<application>`). Así gogio no necesita otra bandera la próxima vez, que es la forma en que esto
se puede mandar aguas arriba en vez de arrastrarlo.

Y **no hay fork que mantener**: nada que clonar ni que alojar. `gogio-sytue.sh` coge de la caché
de módulos la versión EXACTA que fija `versions.env`, le aplica el parche en un temporal y
compila. Subir de versión es cambiar ese número; si el parche deja de aplicar, el guion **falla**
y eso es justo el aviso de que gogio ha tocado esa zona. El día que lo acepten, se borran el
guion y el `.patch` y se vuelve a `gogio` a secas.

Se rechazó **bajar a `-targetsdk 32`**, que lo desbloquearía hoy sin tocar nada: Play Store exige
≥ 34, así que sería cambiar un bloqueo visible por uno que aparece el día de publicar.

**Decisión 2: la capa nativa es solo presentación, y se mide.** El criterio del dueño es limitar
al máximo lo específico de cada plataforma. Aquí eso se reparte así:

- **En Go, una vez para las catorce apps y las tres plataformas** (`apps/gio/apps/avisos`): el
  cliente SSE y su reconexión con espera creciente, qué merece interrumpir, el descarte de
  repetidos, los textos —los del Hub, ya traducidos—, a dónde lleva cada aviso y el número del
  badge.
- **En Java, cuarenta líneas sin una sola decisión** (`internal/sistema/Avisos.java`):
  «enseña esto», «pon este número», y el envoltorio del servicio en primer plano. Recibe la
  notificación permanente ya construida, como `Parcelable` en el `Intent`.

La prueba de que está bien repartido es que **la capa de iOS pueda ser igual de fina**:
`sistema.Avisar`, `Callar` y `PedirAvisos` son las mismas tres funciones, con
`UNUserNotificationCenter` detrás y sin repetir ninguna regla.

**Decisión 3: `specialUse` y no `dataSync`.** Desde Android 15 un servicio en primer plano de
tipo `dataSync` tiene un **tope de seis horas al día** —el sistema llama a `onTimeout()`—, y una
conexión de avisos que se apaga a media tarde no es una conexión de avisos. `specialUse` no tiene
tope; a cambio Google Play pide una justificación, que va declarada en el propio manifiesto
(`PROPERTY_SPECIAL_USE_FGS_SUBTYPE`) y es la de verdad.

**Permisos, que son tres y los únicos.** `POST_NOTIFICATIONS`, `FOREGROUND_SERVICE` y
`FOREGROUND_SERVICE_SPECIAL_USE`, justificados uno a uno —función, dato, duración, impacto y
alternativa, como pide el encargo v4 §5— en `permisosDelManifiesto`. El de avisar **se pide en
contexto**: al abrir la campana por primera vez, no al arrancar; y si la persona dice que no, la
campana sigue contando y marcando.

**Qué está verificado y qué no, dicho como es.** El APK está construido en el piloto y
**verificado en el artefacto** con `aapt2`: el manifiesto declara los tres permisos y ninguno
más, el `<service eu.sytue.gio.Avisos>` con `foregroundServiceType` `specialUse`, y la clase está
en `classes.dex`. **No se ha visto correr en un teléfono**: no hay dispositivo ni emulador (el
piloto no tiene `/dev/kvm`). Los guardianes cubren lo que sí se puede ver en rojo sin teléfono
—que el nombre del servicio y la firma JNI no diverjan entre el XML, el Java y el Go, y que los
permisos no crezcan— porque esas tres cadenas sueltas son lo que el compilador no relaciona y lo
que fallaría solo en el aparato.

**Descartado.** _Un `.jar` versionado en el repositorio_ (`empaquetar-movil.sh` compila el `.java`
con el `javac` que gogio ya exige, y gogio recoge los `*.jar` del directorio de cada paquete Go:
un hueco menos que parchearle). _Parchear gogio para que compile `.java` nuestros_ (por lo mismo:
se resuelve desde nuestro lado). _`START_STICKY` en el servicio_ (resucitarlo sin el Go que lo
alimenta es una notificación permanente delante de una conexión que no existe). _Pedir
`POST_NOTIFICATIONS` al arrancar_ (el diálogo sin contexto que el v4 §5 no quiere).

**Addendum (2026-09-27, Fyne).** Con Gio retirado, el servicio es
`eu.sytue.nativo.Avisos` y lo declara solo el manifiesto del Hub (`empaquetar.sh`), que avisa
por las trece. La CLI de Fyne tampoco sabe escribir `foregroundServiceType`: lo añade
`android-binario.py` al manifiesto compilado y `verificar_aab` lo exige en el AAB. Los
permisos del Hub son esos tres más `USE_BIOMETRIC`, que llevan las trece por BiometricPrompt
(el bloqueo de la sesión y los cuadernos de Notepads).

<a id="0048"></a>

## 0048 — La interfaz nativa se rehace en Fyne con widgets propios, y lo que no es del toolkit sale a `apps/nucleo`

`accepted` · 2026-09-21 · escritorio, móvil, fyne, paridad · sustituye al [0045](#0045) en escritorio y móvil; la web sigue con el [0004](#0004) · código en `apps/fyne/` y `apps/nucleo/`

**Contexto.** El [0045](#0045) decidió una sola interfaz en Gio para web, escritorio y móvil.
Un mes después el dueño cambia de destino: **Fyne v2.8.1** (última estable, 2026-08-26) para
**escritorio y móvil**, con **tema, widgets y layout propios** hasta llegar a **paridad 1:1 a
nivel de píxel** con las web apps. La web **no** entra: `sytue.eu` sigue siendo Astro + Preact

- Go, y por tanto **no hay destino WebAssembly que sostener**, que era uno de los costes del

45.

Es el segundo cambio de toolkit en un mes (Fyne → Gio → Fyne). Eso, y no la elección en sí, es
el dato que manda en esta decisión.

**Decisión.**

- **Lo que no es del toolkit sale primero, a un módulo con nombre neutral.** `apps/nucleo`
  reúne las ~19.300 líneas que no tocan ningún toolkit: cliente HTTP/WS de los servicios
  (`api`), catálogo de idiomas (`i18n`), sesión OIDC y llavero (`auth`), capacidades del
  sistema (`sistema`), diálogos de fichero (`ficheros`), motor de fórmulas (`formula`), HTML a
  texto (`htmltext`), cola sin conexión (`offline`) y `webref`. Van también los **tokens de
  color** —la hoja `colors.css` y las trescientas líneas que la interpretan— y los **cinco
  .ttf de Figtree**. Estaban bajo `apps/gio/internal/`, donde Fyne no los puede importar; la
  alternativa era copiarlos, y este repositorio ya sabe lo que cuesta
  ([0037](#0037)). El nombre no lleva el toolkit **a propósito**.
- **La interfaz se compone con primitivas, no se configura.** `apps/fyne/ui` no usa un solo
  widget de serie de Fyne: cada control es una cáscara `fyne.Widget` con su
  `WidgetRenderer` propio, y dentro solo `canvas.Rectangle`, `canvas.Text`, `canvas.Image` y
  otros widgets nuestros. Los layouts (`ui.Fila`, `ui.Columna`) también son propios, porque los
  de serie usan `theme.Padding()` como hueco y ese número lo mueve el ajuste de tamaño de letra
  del sistema. El patrón completo, con sus seis reglas y el porqué medido de cada una, está en
  el comentario de paquete de `apps/fyne/ui/ui.go`.
- **El tema de Fyne existe para dos cosas y nada más**: la LETRA con la que rasteriza
  `canvas.Text` y el fondo del lienzo. Los colores de los widgets se piden por token
  (`tema.Color("border-strong", v)`), nunca por `fyne.Theme`: su mapa tiene cuarenta nombres y
  la hoja ciento cincuenta y pico, y además `theme.Size()` va multiplicado por la escala de
  letra del usuario, con lo que la geometría dejaría de ser nuestra.
- **Gio no se borra.** `sytue.com` sirve hoy la suite en Gio y no se puede dejar sin nada. El
  port a Gio queda **cancelado** —no se añaden apps— pero el árbol `apps/gio` se queda
  compilando y desplegable hasta que el dueño decida qué sirve ese dominio. El
  [0045](#0045) queda sustituido en escritorio y móvil, y en suspenso para la web.

**Lo que se conserva y no se vuelve a averiguar.** Los números medidos contra producción de las
seis apps auditadas (Hub, Mail, Drive, Calendar, Notepads, Contacts), los 120 diálogos con su
talla, y los vectores de color del inspector. **No dependen del toolkit** y son el activo más
caro que hay en este trabajo.

**Hasta dónde llega el píxel, medido el 2026-09-21 contra Fyne v2.8.1.** Esto es lo que hay que
saber antes de portar dieciséis apps, y no se repite en cada PR:

1. **Color: exacto.** Fyne compone alfa en **sRGB**, igual que el navegador —el fallo que tuvo
   Gio, que componía en lineal y sacaba todos los bordes a media fuerza, aquí **no existe**—.
   Queda la cuantización del alfa a un byte: `rgba(ink, .12)` sobre blanco da (228,231,232)
   contra los (227,230,232) de producción. **±1 nivel de 255**, así que un gate de captura
   necesita esa tolerancia; el byte a byte no va a salir nunca. Los bordes se guardan aplanados
   y opacos en `apps/nucleo/tokens` y entonces el píxel sí es exacto.
2. **Geometría: exacta a partir de ×2 para los medios píxeles.** Fyne cuantiza por coordenada
   al píxel de dispositivo y **no antialiasea los lados rectos** (las esquinas redondeadas sí).
   A ×1 un `h-9` (31,5) ocupa 32 filas; a ×2 son 63 clavadas. Los CUARTOS de píxel —19,25 de
   interlineado, 15,75— no son exactos ni a ×2: harían falta ×4. El error es siempre positivo,
   así que **se acumula al apilar cajas**. Por eso la escala se declara con sus decimales y
   **no se redondea en nuestro código**: redondear congela a ×1 un error que la pantalla
   arregla sola.
3. **El borde no puede ser `StrokeWidth`.** Fyne lo pinta centrado en el contorno: a ×1 reparte
   su cobertura entre dos filas y sale a media fuerza. `ui.Caja` lo pinta como dos rectángulos
   rellenos, que va bien a cualquier escala y además da el `box-sizing: border-box` de la hoja.
4. **Tipografía: era el único techo que no dependía de nosotros, y ya está quitado.** Ver
   [«El conformador va parcheado»](#0048-texto) más abajo. `github.com/go-text/typesetting`
   escalaba la fuente a `ceil(tamaño)` antes de colocar un glifo, así que con el rem de 14
   —siete de los diez peldaños son fraccionarios— `text-base` (12,25) se conformaba como 13:
   **+6,12 %**. Hoy el ancho de un texto queda a **≤0,17 px** del que da el navegador.
5. **La caja de línea hay que imponerla.** `canvas.Text.MinSize().Height` devuelve la caja de
   la FUENTE, 1,2 em: para `text-base` son 14,70, no la caja CSS de 19,25 que es la que ocupa
   sitio en el layout de la web. (Antes del parche del conformador daba 15,59, porque ese 1,2 em
   salía del ppem redondeado a 13; hoy es 1,2 × 12,25 exacto, pero sigue sin ser la caja de
   CSS.) `ui.Medir` devuelve la caja de CSS y `ui.ColocarTexto` centra el glifo dentro de ella.

**Consecuencia, y es la decisión de fondo:** **la paridad alcanzable es de un píxel de
dispositivo en geometría y color, y de dos décimas de píxel en el ancho de un texto**. Las dos
son tan buenas como el propio navegador.

<a id="0048-texto"></a>

### El conformador va parcheado: ppem exacto y no `ceil` (2026-09-21)

**Lo que estaba mal.** `github.com/go-text/typesetting` escalaba la fuente a `ceil(tamaño)`
antes de pedirle los avances a harfbuzz (`shaping/shaping.go`), así que **ningún tamaño
fraccionario existía**: se conformaba el entero siguiente. Con `1 rem = 14 px` casi toda la
escala de la suite es fraccionaria y **cada rótulo de la interfaz nativa salía más ancho**.
Medido en el piloto con el pintor de software, `Guardar cambios` en Figtree Regular:

| escalón     | px de CSS | antes  | después | error que llevaba         |
| ----------- | --------- | ------ | ------- | ------------------------- |
| `text-2xs`  | 9,625     | 76,08  | 73,25   | +3,86 %                   |
| `text-xs`   | 10,5      | 83,75  | 79,97   | +4,73 %                   |
| `text-sm`   | 11,375    | 91,39  | 86,59   | +5,54 %                   |
| `text-base` | 12,25     | 98,97  | 93,25   | **+6,13 %**               |
| raíz        | 14        | 106,59 | 106,59  | 0 (entero: nunca lo tocó) |
| `text-lg`   | 15,75     | 121,78 | 119,92  | +1,55 %                   |
| `text-xl`   | 19,25     | 152,25 | 146,52  | +3,91 %                   |
| `text-3xl`  | 31,5      | 243,58 | 239,78  | +1,58 %                   |

**Y no lo arreglaba una pantalla HiDPI**, como decían el ADR y las tres notas del código hasta
esta fecha. Fyne conforma en **px de CSS** y multiplica por la escala del lienzo **después**
(`internal/painter/font.go`), así que la extensión de tinta medida se encoge lo mismo a ×1
(−6,06 %) que a ×2 (−5,61 %). La creencia de que el error «se parte por dos a ×2» era falsa y
estaba escrita en cuatro sitios.

**Decisión: se aplica, con fork alojado.** Una línea:
`font.XScale = int32(input.Size) << (scaleShift - 6)`. `input.Size` ya es un `fixed.Int26_6`
—el tamaño por 64— y harfbuzz escala por `XScale/upem`, así que pasarlo en crudo pide el tamaño
exacto sin perder la precisión de 1/64 px que el desplazamiento por `scaleShift` existe para
conservar.

**Contra el navegador, que es la referencia**: 54 medidas (seis rótulos reales × nueve peldaños,
Figtree Regular) contra `getBoundingClientRect()` del Figtree variable de producción, con el
hinting de Chromium apagado. Error medio **de +3,30 % a +0,03 %**; el peor caso, **de 8,33 px a
0,17 px**.

**Por qué un fork y no un `.patch` como el de gogio ([0047](#0047)).** gogio es una
**herramienta** que invoca un guion nuestro, y ese guion puede parchear una copia temporal.
`typesetting` es una **biblioteca** que entra en cada `go build`, cada `go test` y cada IDE: no
hay un solo sitio por el que pase todo. Un `replace` a un directorio generado dejaría un clon
recién hecho sin compilar hasta que alguien corriera un `make`. Así que hay fork, y el precio
se paga en la política de abajo.

**Política de actualización del fork.**

- Vive en `github.com/thinkddr/typesetting`. La rama **`main` es el espejo intacto de
  upstream** y **`sytue`** es `v0.3.4` + el parche. El `replace` apunta a un **tag**
  (`v0.3.4-sytue.1`), nunca a una rama.
- **Rebasar** = traer upstream a `main`, `git rebase <tag nuevo> sytue`, tag `vX.Y.Z-sytue.1` y
  cambiar el `replace`. El diff contra upstream son **dos ficheros y 27 líneas** —el parche y el
  `module`, que cambia porque un `replace` exige que el destino declare la ruta por la que se le
  requiere—, así que un rebase que no aplique limpio es la señal de que upstream ha tocado esa
  zona, no un conflicto que resolver a ojo.
- **Quién lo comprueba:** quien suba `fyne.io/fyne/v2`. Fyne fija la versión de `typesetting` en
  su go.mod; si sube, el fork se rebasa sobre esa misma versión en el mismo PR. El gate de
  versiones de `AGENTS.md` §1 ya obliga a mirarlo.
- **Qué avisa:** `ui.TestElConformadorNoRedondea`, que compara el ancho conformado contra la
  proporción desde un patrón de 64 px y falla si alguien quita el `replace` o si un rebase
  pierde el parche. Visto en rojo en las dos direcciones el 2026-09-21.
- **Aguas arriba:** el parche está escrito para mandarse a `go-text/typesetting` tal cual, con
  su comentario en inglés. Mientras no lo acepten, el fork se rebasa; el día que lo hagan, se
  borran el `replace` y el fork.

**Soberanía: el fork es PRIVADO a propósito.** Publicarlo mandaría su ruta a `proxy.golang.org`
y a `sum.golang.org` en cuanto alguien lo resolviera, que es exactamente la fuga que
`compliance/supply-chain/README.md` §A da por cerrada para los módulos `git.sytue.eu/...`. Como
`git.gay` ya está inventariado en `providers.tsv` como excepción temporal hasta el 2026-12-31,
**no hace falta ninguna fila nueva en `blocked-baseline.tsv`**. El precio es que quien compile
`apps/fyne` necesita `GOPRIVATE=git.gay/*` y credenciales de git.gay —el piloto y el CI ya las
tienen—, y sin eso el `go mod download` falla en el proxy, no en silencio.

### URL entrante: fork temporal de Fyne v2.8.1 (2026-09-22)

Fyne v2.8.1 solo exponía `App.OpenURL`, que abre una URL **hacia fuera**. No había
contrato ni bridge para la URL que entrega el sistema al volver del navegador, abrir
otra app por su esquema o iniciar una aplicación cerrada. Eso hacía imposible
terminar el login móvil RFC 8252 aunque `auth.OIDC.LoginEsquema` y `Vuelta` ya
existieran: el callback no tenía dónde entrar.

Se mantiene la dependencia pública `fyne.io/fyne/v2`, en la versión estable declarada en `apps/fyne/go.mod`, pero se reemplaza por
el tag público de `github.com/thinkddr/fyne/v2` fijado en `apps/fyne/go.mod` (la historia está en `apps/fyne/upstream/README.md`). Conserva la licencia BSD-3-Clause de upstream y las modificaciones del fork se distribuyen bajo Apache-2.0. Añade la interfaz
opcional `fyne.URLHandler`, sin ensanchar `fyne.App` ni romper implementaciones de
terceros. La URL se encola hasta registrar el handler y se entrega en el hilo gráfico.
Android cubre el intent inicial y `onNewIntent`; iOS, la URL de lanzamiento y
`application:openURL:options:`; macOS, el Apple Event `kAEGetURL`; Linux y Windows,
el argumento con que arranca el binario.

Los tags originales de upstream se conservan; `main` del fork contiene las modificaciones de Sytue.
Al actualizar Fyne: integrar el tag estable nuevo, ejecutar los tests del
fork en CI, crear `vX.Y.Z-sytue.N` y cambiar el `replace` en el mismo cambio. Se
propondrá el parche aguas arriba; cuando lo acepten, se elimina el fork y el
`replace`, no se mantiene una API paralela.

**Y un tercer fork, de GLFW (2026-09-23).** `apps/fyne/go.mod` reemplaza también
`github.com/go-gl/glfw/v3.4/glfw` por el tag de `github.com/thinkddr/glfw/v3.4/glfw` fijado en `apps/fyne/go.mod`
(privado, bajo las licencias de upstream: BSD-3-Clause el enlace de Go y zlib GLFW), sobre
la base de go-gl que requiere la versión vigente de Fyne. GLFW solo acepta ficheros soltados, y
su XDnD de X11 y su `wl_data_device` de Wayland no se ven desde fuera: el parche añade
texto y enlaces soltados (`glfwSetDropTextCallback`, `apps/fyne/soltar.AlSoltar`) y,
desde `.3`, el aviso de ficheros arrastrados por encima (`glfwSetDragCallback`,
`soltar.AlArrastrar`). Las mismas reglas que el de Fyne: una rama con el parche,
un tag por versión y el `replace` en el mismo cambio; lo que lleva cada
versión, cómo se probó y cómo quitarlo, en `apps/fyne/upstream/README.md`.

**El envío aguas arriba está preparado (2026-09-22)**, en
[`apps/fyne/upstream/`](../apps/fyne/upstream/README.md): el parche exportado (13
ficheros, 235 líneas añadidas y ninguna borrada, con su prueba) y el texto de la PR en
inglés. Falta abrirla, y eso pide la cuenta personal del dueño en GitHub. La issue viva
que lo pide es `fyne-io/fyne#1847`; no hay ninguna PR abierta que lo resuelva.

**Y la otra punta: quién RECIBE el enlace.** El emisor (`apps/nucleo/api/abrir.go`)
lo tenía todo y no había receptor: el enlace habría llegado a una ventana en blanco.
`api.LeerApertura` desarma el enlace —es la vuelta de `RutaDeApertura`, atada con él por
ida y vuelta— y `sesion.Suite.AlAbrirFichero` lo entrega ya desarmado a la app dueña del
formato, que lo carga en segundo plano. Si no puede —sin sesión, el fichero ya no está,
un formato que no entiende— el enlace **sigue a la misma página en la web**, que es lo
que ya hacía Drive con la app sin instalar: nunca una ventana en blanco.

Docus, Calcus y Diapos abren el fichero como lo abre `/open-from-drive`, pero SIN crear
documento (`api.ImportarDeDrive`): se trae de Drive por id, `POST /api/v1/import` lo
convierte al modelo de la web —HTML, el JSON de la hoja o el de la presentación— y se
enseña en **solo lectura**, porque es una conversión y no un documento de Docs, y lo que
se escribiera encima no tendría dónde guardarse. Photos hace lo de `/abrir`: indexa (es
idempotente y no copia nada) y enseña la variante grande en el visor. CertiPDF sigue con
el respaldo web hasta que su pantalla hable con el servicio.

Al cablearlo salieron dos cosas que no eran del fork y que habrían hecho que nada de
esto funcionara: los esquemas de esos cuatro binarios se llamaban por su **marca**
(`sytue-docus`) y no por la **clave del producto** (`sytue-docs`), que es lo que emiten
las otras apps, lo que declara `PRODUCT_SCHEMES` en la web y lo que Sytue ID tiene dado
de alta (migración 0071) — el síntoma habría sido «la app nativa no está instalada»;
y en el móvil la sesión pedía el token con el `client_id` del escritorio. Los dos
guardianes están en `apps/fyne/sesion`.

**Una app abierta recibe el enlace; no se abre otra.** En macOS, iOS y Android lo hace el
sistema —en Android, con `launchMode="singleTask"`, que `empaquetar.sh` pone ya: sin él
la vuelta del navegador crea otra actividad y `onNewIntent` no se llama—. En Linux y
Windows abrir un esquema es ejecutar el programa otra vez, así que `sesion` abre un
enchufe local por aplicación y la segunda copia le pasa la URL a la primera y se va. Y
el enlace que ARRANCA la app se guarda hasta que el `main` registra su receptor: Fyne lo
despacha dentro de `sesion.Abrir`, antes de que exista nadie a quien dárselo.

**El hinting no se mueve, y se ha mirado.** `go-text/render` escala el contorno del glifo por el
tamaño en **coma flotante** y lo rasteriza con `rasterx`, que **no ajusta a rejilla**: el ppem
del conformador solo gobierna los avances. Comprobado además con los ojos, comparando los PNG
del pintor de software a ×1 y a ×2 con y sin el parche: mismo grosor de trazo, mismo
antialiasing, solo el tracking más apretado.

**Y el Figtree variable de la web contra las instancias estáticas del nativo: son la misma
fuente.** Era el riesgo abierto que quedaba —la web monta `@fontsource-variable/figtree@5.3.0` y
`apps/nucleo/fuentes` embebe cinco .ttf estáticos— y nadie lo había medido. Medido ahora, a
**640 px** para que ni el redondeo del navegador ni el de harfbuzz cuenten, sobre 32
combinaciones de ocho cadenas por los cuatro pesos (400/500/600/700): la desviación máxima es
**0,0098 %**, o sea **0,7 px sobre 7.098**. Es ruido. **No hay que embeber el variable**, y la
diferencia que se veía a tamaños pequeños era la cuantización de avances a píxel entero que hace
Chromium en Linux, no la fuente.

**Coste aceptado.**

- **Se rehace la capa de presentación por tercera vez.** Lo que sobrevive es
  `apps/nucleo` y los números medidos; lo que se rehace son las ~4.400 líneas de
  `apps/gio/internal/ui` y las pantallas de cada app.
- **`gioui.org` se queda en el go.mod del núcleo** por dos ficheros `//go:build android` (el
  puente JNI que pide `app.JavaVM()` y `app.AppContext()`). No se compila en ningún destino de
  escritorio. Se quita con unas veinte líneas el día que se empaquete Android en Fyne, que es
  cuando habrá con qué probarlo.
- **Los guardianes de píxel corren a ×2**, que no es la escala en la que se despliega
  necesariamente: `SetScale` solo existe en el lienzo de software; en el binario real la escala
  la deciden la pantalla y `FYNE_SCALE`. A ×2 se mide el widget; a ×1 se mediría el redondeo.

**Descartado.** _Seguir con Gio_ (decisión del dueño; y el `ceil` del texto es el mismo, así que
no habría ganado nada en el techo que importa). _Configurar los widgets de serie de Fyne_ (su
tema escala la geometría con la letra del sistema: la paridad se acaba antes de empezar).
_Copiar los ocho paquetes a `apps/fyne`_ ([0037](#0037), y ya sabemos cómo acaba). _Redondear la
escala a enteros para que cuadre a ×1_ (congela el error donde la pantalla lo arreglaría).
<a id="0049"></a>

## 0049 — `sytue.com` sirve la suite Astro con un segundo juego de imágenes, no la de Gio

`accepted` · 2026-09-21 · frontend, despliegue, oidc · supersede al [0045](#0045) en la web · código en `deploy/sytue-com/`

**Contexto.** El [0045](#0045) repartía los dos dominios: `sytue.com` publicaría la suite en Gio
(WebAssembly) y `sytue.eu` seguiría con Astro + Preact. El port a Gio **queda cancelado** —la
interfaz de escritorio y móvil se migra a Fyne y la web sigue siendo Astro + Preact + Go—, así que
`sytue.com` se queda sin destino. La decisión del dueño es que **sirva la suite**, no que redirija
a `sytue.eu`.

**La trampa, y es el centro de todo.** Los frontends Astro **hornean las URL públicas en tiempo de
build**: `scripts/publicar-imagen-frontend.sh` les pasa `PUBLIC_OIDC_ISSUER`, `PUBLIC_HUB_URL`,
`PUBLIC_SYTUE_DRIVE_BASE_URL` y tres más, todas derivadas de `DOMINIO`. Servir esas mismas
imágenes en `sytue.com` da una suite que **se ve en `.com` y habla con `.eu`**: login y cookies
cruzando dominios.

**Decisión.** La tomó el dueño el 2026-09-21: **un segundo juego de las trece imágenes Astro
construido con `DOMINIO=sytue.com` y desplegado en el frontal** (`sytue-rooms-media`), no en el
piloto. **Los dos frentes comparten Sytue ID, servicios y base de datos**; lo único que se duplica
es el frontend.

**Lo que costó medir, y lo que la medición cambió.**

- **El horneado casi no existe.** De las ~30 lecturas de `PUBLIC_*` de la suite, **todas menos
  tres** pasan por `serverEnv()`, que fusiona el entorno de build con `process.env` **dejando
  ganar al segundo**; el chart ya inyecta cada una por contenedor. Las tres que no:
  `apps/www/src/lib/downloads.ts` (SSG, y no son dominios de la suite),
  `packages/config/src/health-endpoint.ts` (versión y commit, que SÍ son de la imagen) y
  `packages/config/src/tokens.ts`. Esa última es la única que ata la imagen a un dominio: de ella
  sale `suiteApex` y con él los enlaces entre apps. O sea que el segundo juego existe por **un
  fichero**.
- **`PUBLIC_OIDC_ISSUER` y `SYTUE_OIDC_ISSUER` son variables DISTINTAS**, y de ahí sale la pieza
  que hace viable todo esto. La primera se hornea y solo alimenta `suiteApex`; la segunda es de
  ejecución y es la que usa el BFF. Así el frente de `.com` hornea sus enlaces en `.com` y a la
  vez inicia sesión contra el **único** Sytue ID, `id.sytue.eu`. Verificado el 2026-09-21 con la
  imagen real: `GET /one/auth/login` con `Host: one.sytue.com` responde `302` a
  `https://id.sytue.eu/oauth2/authorize?…&redirect_uri=https%3A%2F%2Fone.sytue.com%2Fone%2Fauth%2Fcallback`.
- **Un IdP emite un `iss`.** No hay «Sytue ID de `.com`»: quien entre por `.com` inicia sesión en
  `id.sytue.eu` y vuelve. `id.sytue.com` y `hub.sytue.com` sí existen como hosts —el `appDomains`
  horneado los nombra— y se enrutan al piloto reescribiendo el `Host`.
- **Las cookies no necesitan nada.** Son `__Host-`: host-only y sin `Domain`, y
  `resolveOidcConfig` **lanza** si alguien define `SYTUE_COOKIE_DOMAIN`. Cada host tiene la suya,
  así que `.com` no depende de que valga una de `.eu`. La consecuencia, escrita para que nadie la
  descubra sobre la marcha: **la sesión no se comparte entre los dos frentes**; entrar en `.com`
  es un login propio que el SSO del IdP resuelve sin volver a pedir contraseña.
- **El coste del segundo juego es mucho menor de lo temido.** En el registro, la segunda variante
  de una app añade **~7 MB** y no 647: todas las capas menos el `dist` son las mismas (medido en
  el piloto con dos builds del portal). En el runner, el juego de `.com` es el **nativo** —los
  runners son ARM64 y el frontal también—, así que se construye **sin** `bun install --cpu` ni
  `crane rebase`, al revés que el de `.eu`, que es el cruzado. Referencia medida en la v3.3.9: 13
  frontends en 30 min 49 s de runner, 2 min 22 s cada uno, el 79 % del total de la publicación.
- **Y el frontal aguanta.** Un contenedor SSR ocupa 19-46 MiB en reposo y 64 MiB tras 260
  peticiones; los catorce de producción suman 552 MiB y 17 milicores entre todos. Trece más caben
  en los 21 GB libres del frontal sin acercarse a Pangolin, al SFU ni a los tres runners.

**El coste que sí hay que aceptar: la latencia del SSR.** Los BFF del frontal no pueden usar
nombres del clúster, así que llaman al backend por `https://<producto>.sytue.eu`. Medido desde el
frontal el 2026-09-21: **33 ms con conexión reutilizada** y **116 ms en frío**, de los que 85 son
el saludo TLS. **No hay atajo**: `id.sytue.eu` ya resuelve a esa misma máquina (ping 0,06 ms), o
sea que esos 33 ms son el túnel de Pangolin hasta el piloto — el viaje físico París↔España, no el
proxy. Una página con llamadas **en cadena** las suma; cuando alguna se note, lo que hay que
quitar es la cadena, no el salto.

**Lo único que hubo que cambiar en el código** fue añadir `sytue.com` a `SYTUE_ALLOWED_DOMAINS`
(`@sytue/config/astro-security`). Sin eso el síntoma no es un error claro: Astro deja de creerse
el `Host`, `Astro.url` cae a `http://localhost:<puerto>`, el BFF salta al host canónico —sacando a
la persona del dominio— y `checkOrigin` rechaza todo POST, logout y refresco de sesión incluidos.
Nada de eso falla al desplegar: falla al entrar.

**Guardianes** (los dos vistos en rojo a mano antes de darlos por buenos):

- `apps/frontend/packages/config/src/dominio-horneado.test.ts` compara **por igualdad** los tres
  ficheros que pueden leer `import.meta.env.PUBLIC_*` sin `serverEnv`. Un cuarto lo para, porque
  cada lectura nueva engorda el precio de tener dos juegos de imágenes.
- `TestRedirectUrisDeSytueComCubrenElCompose` (en `make migrate-check`) ata los
  `SYTUE_OIDC_REDIRECT_URI` que anuncia `deploy/sytue-com/docker-compose.yml` a los registrados en
  Sytue ID. Es el fallo que no se parece a un fallo: el contenedor arranca, `/healthz` contesta, y
  lo único que no funciona es entrar, solo por `.com`.

**Orden de encendido, y el camino de vuelta.** Primero la migración 0094, luego los contenedores,
y el bloque de Traefik el último: al revés, `sytue.com` deja de servir Gio antes de que lo nuevo
pueda iniciar sesión. **Gio no se apaga**; su router sigue puesto con la misma prioridad, así que
quitar el bloque de `sytue-com` devuelve el dominio a Gio en el acto. Detalle en
`deploy/sytue-com/README.md`.

**Descartado.** _Redirigir `sytue.com` a `sytue.eu`_ (el encargo dice servir, no redirigir).
_Resolver las URL públicas en ejecución desde la cabecera `Host`_: llegó a estar implementado y
verificado —lista blanca de hosts en el BFF y enlaces entre apps al mismo origen— y se descartó al
decidir el dueño la vía de las dos imágenes. Sirve para cualquier dominio futuro sin publicar
nada, pero exige que los enlaces entre apps sean relativos, y eso cambia las URL visibles de toda
la suite en `sytue.eu`, que es la referencia de paridad. Queda medido por si un tercer dominio lo
hace rentable: con enlaces absolutos, ir de Mail a Drive cuesta **cuatro redirecciones y una
vuelta entera por el IdP**, porque las cookies son host-only. _Un solo host en `.com` sirviendo la
suite por prefijos_, como el host de organización: funciona, pero obliga a lo mismo de arriba.
<a id="0050"></a>

## 0050 — Un solo inicio de sesión para las trece apps nativas: llavero para el secreto, carpeta compartida para las preferencias, servidor para la presencia

`accepted` · 2026-09-22 · escritorio, móvil, oidc, fyne · código en `apps/nucleo/ajustes`, `apps/nucleo/auth`, `apps/fyne/sesion`

**Contexto.** La suite instalada son **trece binarios**, uno por producto ([0048](#0048) §separación
de apps). El dueño pide que la sesión sea **transversal**: quien entra en Sytue Mail no vuelve a
entrar en Drive. Y con ella, la **apariencia**, el **idioma** y la **presencia**. Cuatro cosas que
la persona ve como una sola —«mi cuenta»— y trece procesos que tienen que coincidir.

**Cuatro cosas, tres sitios.** La tentación es un único almacén. Es un error, y cada uno falla de
una manera distinta:

|                            | dónde                         | por qué ahí y no en otro sitio                                                                                                                                               |
| -------------------------- | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Sesión** (refresh token) | llavero del sistema           | es un secreto: Keychain en macOS e iOS, Secret Service en Linux, Credential Manager en Windows, Keystore en Android                                                          |
| **Apariencia**             | `<config>/sytue/ajustes.json` | no es un secreto, y en macOS el llavero **puede pedir autorización**: un arranque que pregunta por el tema es absurdo. Y si el llavero no está, la app tiene que abrir igual |
| **Idioma**                 | ídem                          | ídem                                                                                                                                                                         |
| **Presencia**              | el servidor (`api.Hub`)       | **la ven los demás**. Una copia local sería una segunda verdad que se queda vieja en cuanto se cambia desde la web o desde el teléfono                                       |

**`fyne.Preferences` no vale, y no es una opinión.** En `fyne.io/fyne/v2@v2.8.1`, el almacén es
`filepath.Join(app.RootConfigDir(), a.UniqueID())` (`app/preferences_other.go:18`): una carpeta
**por `ID` de `FyneApp.toml`**. Trece apps, trece `preferences.json`, trece temas. En Android e iOS
sí es una sola carpeta (`preferences_android.go:19`, «we are in a sandbox»), pero ahí también hay
una sola app instalada, así que tampoco comparte nada entre productos. Y es **JSON en claro**, con
lo que para el secreto queda descartado por partida doble.

**El fichero compartido dice QUÉ ha cambiado, nunca guarda lo que vale en otro sitio.** Es la regla
que ordena el reparto: de la sesión solo hay una **marca** —un número en nanosegundos, que solo
sirve para ser distinto del anterior— y quien la ve vuelve a preguntarle al llavero. Un guardián
recorre la carpeta entera y falla si aparece algo que huela a credencial
(`TestLaMarcaDeSesionNoLlevaLaSesion`).

**El aviso entre procesos es UNO: un sondeo de dos segundos** (`ajustes.Vigilar`). Dos lecturas de
menos de cien bytes, sin dependencia nueva y sin una implementación por plataforma; `inotify` /
`FSEvents` / `ReadDirectoryChangesW` habrían sido tres. Lo que cada cosa hace con el aviso:

- **Cerrar sesión: en el acto**, en las trece. No admite «en el siguiente arranque».
- **Tema: en caliente.** Cambiarlo en Mail lo cambia en Drive sin reabrir nada.
- **Idioma: al siguiente arranque.** Las pantallas reciben su traductor al construirse y rehacerlas
  enteras sería reabrir la app por dentro. Es lo que hace cualquier escritorio, y está dicho aquí
  en vez de quedarse a medias.
- **Presencia: no pasa por el fichero.** Se le pregunta al Hub en el mismo latido que la mantiene
  viva, porque **también hay que enterarse de lo que se cambie desde la web o el teléfono**, y para
  eso el fichero no sirve. Añadirle una marca sería un segundo mecanismo para un caso que el
  primero ya cubre.

**Fyne no deja forzar la variante del tema, y eso cambia el diseño.**
`settings.ThemeVariant()` devuelve `s.variant`, que sale del sistema operativo, y `SetTheme` **no la
toca** (`app/settings.go:75-86`). `theme.LightTheme()`/`DarkTheme()` tampoco valdrían: los widgets de
la suite no piden color por `fyne.Theme` ([0048](#0048) §tema). La elección se resuelve en
`tema.De`, por donde **ya pasaban los 243 puntos del árbol** que preguntan la variante, en vez de
cambiar los 243.

**El flujo OIDC es el de RFC 8252, y ya estaba.** Authorization Code + PKCE contra un cliente
**público** —un binario instalado no puede guardar un secreto—, con el navegador **del sistema** y
no una vista dentro de la app (§8.1): así el dominio y el candado se ven, y la sesión que ya hubiera
con Sytue ID se reutiliza. La vuelta del navegador es lo único que cambia entre plataformas: en el
escritorio, un servidor efímero en `127.0.0.1:8921` (§7.3, cliente `sytue-desktop`, migración 0069);
en el móvil, el esquema propio de cada app (§7.1, cliente `sytue-mobile`, migración 0071, con los
catorce esquemas registrados uno a uno). **No hace falta migración nueva**: los dos clientes y sus
`redirect_uri` ya están dados de alta.

**La carrera de la renovación, que era el riesgo de verdad.** Sytue ID **rota** el refresh token en
cada uso y, si le presentan uno gastado, **revoca la familia entera**. Con trece procesos y una sola
entrada del llavero, dos apps que caduquen a la vez leen el mismo token, las dos lo presentan y la
segunda deja fuera a las trece. Mirar lo guardado antes de renovar no basta: hay que impedir que
dos miren a la vez. `Compartida` envuelve **leer‑renovar‑guardar** en un cerrojo **entre procesos**.

El cerrojo es un fichero creado con `O_EXCL` y **no `flock`**: `O_EXCL` es la misma llamada en los
cinco destinos —incluidos Windows y Android— y `flock` habría sido un fichero por plataforma en
`golang.org/x/sys`. El precio es que un proceso que muera lo deja puesto, y por eso **caduca a los
diez segundos**. El guardián son **trece procesos de verdad**, no goroutines: sin cerrojo, «el
contador acabó en 1 y tendría que ser 13».

**Descartado.** _Un fichero 0600 con el refresh token_ (es lo que hacía el escritorio viejo; un
secreto en claro no es un llavero, y en un equipo compartido cualquier proceso del mismo usuario lo
lee). _`fyne.Preferences` para la sesión_ (texto plano) _y para las preferencias_ (por app, que es
justo lo contrario de lo que se pide). _Un solo almacén para las cuatro cosas_ (el llavero pidiendo
permiso al arrancar para saber el tema). _Guardar la presencia en local_ (segunda verdad).
_`singleflight` en vez del cerrojo_ (resuelve la carrera dentro de un proceso, y el problema es
entre trece). _Avisar con `fsnotify`_ (una dependencia y tres implementaciones para ahorrar dos
lecturas de cien bytes cada dos segundos; se reconsidera si alguna vez se mide que molesta).

<a id="0051"></a>

## 0051 — Firma con el certificado del sistema, en dos pasos, con sello cualificado de Izenpe, y «Pedir firma»

`accepted` · 2026-09-26 · docs, firma, eIDAS, seguridad · **desarrolla el [0030](#0030) §2 (c)
y sustituye su §4 en la autoridad de sellado** · código en
`services/docs/internal/domain/pdfsigndospasos.go`, `…/http/pdffirmasistema.go`,
`…/http/pdfpedirfirma.go`, `apps/nucleo/api/docs_pdf_firma.go`; mig. docs `0007` y `0011`

**Contexto.** El certificado de firma de la mayoría de la gente en Europa no está en un fichero:
está en el Llavero de macOS o iOS, en el almacén de Windows, en el KeyChain de Android, en una
tarjeta o en el DNI electrónico, y **su clave privada no se puede sacar de ahí** —en una tarjeta,
físicamente—. Hasta ahora CertiPDF firmaba con un `.p12` que se subía al motor, y para lo demás
remitía a la aplicación oficial del país. El dueño decidió (2026-09-24): firmar con el certificado
del sistema, en dos pasos, contra **todos** los almacenes, y con sello de tiempo **cualificado**
de la UE (PAdES-B-T). Y (2026-09-25) que se pueda **pedir la firma a otra persona**.

**Decisión.**

1. **Dos pasos; la clave no sale del equipo.** El servicio prepara (`POST /api/v1/pdf/sign/prepare`):
   el PDF con el hueco y el `/ByteRange` escritos, los atributos firmados (content-type,
   message-digest y signing-certificate-v2, **sin** signing-time: PAdES lo prohíbe) y su resumen
   SHA-256, más el `DigestInfo` ya envuelto para las funciones «en crudo» (CKM_RSA_PKCS,
   NONEwithRSA). La app pide al SISTEMA que firme ese resumen con la clave elegida —una llamada a
   su función de firma, sin escribir ni un byte de ASN.1— y devuelve la firma
   (`POST /api/v1/pdf/sign/complete`). El servicio monta el CMS y lo incrusta. El motor es UNO: los
   dos pasos usan las mismas piezas que la firma con `.p12`, con el mismo perfil PAdES
   (`/SubFilter /ETSI.CAdES.detached`).
2. **La ligadura vive en la base, no en el cliente** (mig. 0007). La preparación es de UNA persona
   de UNA organización (RLS), vale **diez minutos**, se reserva mientras se completa (dos vueltas a
   la vez no dan dos firmas) y **se gasta** al completar. Se guardan la huella del PDF preparado
   —el PDF lo devuelve la app: 25 MiB no se aparcan en la base—, los atributos y la cadena. Al
   completar se exige: misma persona, **el mismo documento byte a byte**, los atributos resumiendo
   ESE documento y nombrando ESE certificado, y una firma de ESE resumen con ESA clave. Un cliente
   que mienta no consigue una firma. Cinco preparaciones abiertas por persona como mucho.
3. **Todo lo que puede fallar después del PIN se comprueba antes.** El certificado tiene que estar
   vigente, ser de firma (digitalSignature o contentCommitment, el del DNIe), RSA ≥ 2048 o ECDSA
   P-256/384/521; la cadena que manda la app tiene que ir **en orden** (cada certificado emitido
   por el siguiente) y **caber** en el hueco con el sello. Descubrirlo después es tirarle el trabajo
   a quien ya puso el PIN en su tarjeta.
4. **Lo que ve la persona ANTES de firmar**, del servicio y no de la app: quién firma (el CN del
   certificado; el nombre que mande la app no cuenta), quién lo emitió, hasta cuándo vale, el nivel
   («puede ser cualificada» solo si el certificado declara QcCompliance **y** QcSSCD y viene con la
   cadena de su emisor; si no, «avanzada»: la lista de confianza de la UE no se consulta aquí, así
   que se promete de menos, [0030](#0030) §1), cuántas páginas y qué autoridad sellará.
5. **Sello cualificado de Izenpe, y sin sello no se firma en silencio.** La autoridad pasa a ser
   **Izenpe** (`http://tsa.izenpe.com`), medida el 2026-09-24 contra la lista de confianza de la UE
   como servicio de sellado CUALIFICADO en vigor; el Certum gratuito del [0030](#0030) §4 **no** es
   cualificado. El token se autentica contra la raíz «ROOT CA QC IZENPE» fijada en el chart
   (`SELLO_ANCLAS`, obligatoria con `SELLO_URL`), tiene que declarar SHA-256, sellar la huella del
   valor de la firma y traer el nonce de la petición. **Esto cambia la regla del 0030 §4 para los
   dos pasos:** si la autoridad no responde, el servicio contesta `pdf_timestamp_unavailable`, la
   preparación sigue viva, y **la misma firma** vale para reintentar o para seguir sin sello por
   decisión explícita (`sin_sello=1`): el sello no entra en lo firmado, así que no hay que volver a
   pedir el PIN. A Izenpe solo viaja una huella: no es subencargado ([`security.md` §11.6](security.md#116-sello-de-tiempo-izenpe)).
6. **Los almacenes, uno por plataforma, debajo y fino** ([0024](#0024), la interfaz sigue siendo
   Fyne): PKCS#11 (tarjetas y DNIe) en los tres escritorios; Llavero en macOS e iOS
   (Security.framework); almacén de Windows por CNG; KeyChain de Android con una clase Java propia
   del selector en la APK, junto a las de #983 (decidido por el dueño). ECDSA sale en DER del
   Llavero y de Android y en r‖s de CNG y PKCS#11: la app dice cuál (`formato=p1363`) y el servicio
   lo normaliza.
7. **«Pedir firma»** (mig. 0011). Desde CertiPDF, a otra persona **de la misma organización**, por
   su correo (lo resuelve Sytue ID con `directory:resolve` y la sesión de quien pide; el navegador
   nunca elige un `sub`). El PDF no se copia: vive en la unidad de la organización y la petición
   guarda su id y su ruta, que tiene que acabar en el nombre de ese fichero. Quien la recibe tiene
   un aviso `docs.signature.requested` de origen `pdf` y un correo desde avisos@, los dos con el
   enlace al PDF (`/docs/pdf?path=`); al abrirlo, una franja le dice quién pide y qué, con
   «Rechazar» y «Firmar». Firma como firme siempre (sistema, `.p12` o rúbrica), la copia firmada
   queda en Drive y **se entrega sola**: el servicio baja original y firmado con la sesión de quien
   firma y exige más firmas que el original y que la última cubra el documento entero. Entonces
   quien pidió recibe `docs.signature.completed` con el enlace al firmado (o
   `docs.signature.declined`). Una pendiente por fichero y persona; treinta peticiones por hora
   y persona como mucho.

**Consecuencias.**

- La firma con el certificado del sistema no convierte a Sytue en custodio de nada: por el
  servicio pasan un resumen, una firma y una cadena pública. El control exclusivo del art. 26.c
  queda donde estaba, en el equipo o la tarjeta de la persona.
- Con Izenpe, la fecha de cada firma sellada tiene la presunción del art. 41.2. Su política da un
  número de sellos al mes gratis y el número no se publica: si se agota, se contrata **sin cambiar
  código** (credencial por Secret).
- «Firmada», en una petición, significa «quien la recibió entregó un PDF con una firma nueva que lo
  cubre entero»; **quién** firmó lo dice el certificado dentro del PDF y CertiPDF lo enseña al
  abrirlo. No se confunde una cosa con la otra en la interfaz.

**Alternativas descartadas.**

- **Un solo paso en el cliente** (la app monta el CMS y sube el PDF firmado). Descartada: el
  formato criptográfico estaría escrito seis veces, una por plataforma, y el servicio no podría
  comprobar nada de lo que le llega.
- **Guardar el PDF preparado en el servidor** en vez de su huella: 25 MiB por preparación en la
  base o en un bucket, para ahorrar una subida que la app ya tiene hecha.
- **Firmar sin sello cuando la autoridad cae**, como el [0030](#0030) §4. En dos pasos la persona
  está delante y acaba de poner el PIN: se le pregunta, no se decide por ella.
- **Pedir firma a cualquier dirección**, también de fuera. El PDF vive en la unidad de la
  organización y quien no es de ella no puede abrirlo; hacerlo exige compartirlo antes por Drive.
  Queda para cuando se decida cómo (DECISIÓN anotada en los pendientes del encargo).
  **Decidido el 26/09: [0057](#0057)** (primero se comparte por Drive, después se pide).

<a id="0052"></a>

## 0052 — Canjear un enlace de Docs, también desde otra organización

`accepted` · 2026-09-26 · docs, multi-tenancy, compartir, seguridad · **aplica el [0040](#0040)** · código en `services/docs/internal/http/canje.go`, mig. docs `0009`

**Contexto.** La vista pública `/s/{token}` (#1042) enseña el documento a cualquiera con el
enlace, en solo lectura. Un enlace de **editor** o de **comentarista** prometía más, y «Abrir en
el editor» solo abría a quien ya tenía acceso: su organización o una invitación. El dueño decidió
que el enlace se **canjee** por un permiso de quien lo abre con su sesión, **también si es de otra
organización**. Un documento vive en la organización de su dueño, con su RLS por `tenant`, y el
token de quien viene de fuera trae SU organización.

**Decisión.**

1. **Canjear es un POST con sesión** (`POST /api/v1/share/redeem`, el token en el CUERPO para
   que no quede en el log de peticiones), no abrir una URL: escribe un permiso. En la web lo lanza el botón de `/s/{token}` con un formulario
   (`/s/{token}/abrir`), con el origen comprobado por Astro y la cookie de sesión que no viaja
   en un POST de otro sitio. Sin sesión, al login y de vuelta a la vista pública.
2. **Solo se canjean enlaces de editor o comentarista.** Uno de lector no añade nada a `/s/`.
3. **Lo canjeado cuelga del enlace** (`doc_permissions.via_link`): la lectura de permisos solo
   cuenta los de enlaces sin revocar y sin caducar, así que **revocar el enlace quita también lo
   canjeado**, y borrarlo lo borra (`ON DELETE CASCADE`). Un permiso DIRECTO que la persona ya
   tuviera no se toca: ni se rebaja ni pasa a depender del enlace (se contesta con el que tiene).
   Al revés también: lo que el dueño concede a mano —o al aceptar una petición de edición— deja
   de colgar del enlace. Si ya puede lo mismo o más, no se escribe nada. **Quitar a alguien que
   entró por un enlace lo BLOQUEA** desde esa hora (`doc_permissions.bloqueado_desde`, mig.
   0010): no se le canjea ningún enlace que ya existiera —tampoco otro vivo del mismo
   documento—; uno creado después por el dueño, sí. Qué permiso cuenta lo dice una sola función,
   `docs_permiso_vivo` (enlace sin revocar ni caducar, y sin bloquear).
4. **El documento se queda en la organización anfitriona**, como las conversaciones del 0040. Quien
   viene de fuera es una fila de `doc_permissions` con **`externo`**, escrita en la anfitriona.
5. **Sus peticiones pasan a la anfitriona SIN nada de su token salvo el `sub`.** El middleware
   `enAnfitrion` de `/documents/{id}` solo actúa si el documento NO está en la organización del
   token y la persona es externa en la de su dueño; entonces la petición sigue en esa
   organización y `subjectFrom` la reduce a su `sub`: ni sus grupos (sus roles de fuera no son
   nada aquí) ni su organización (un permiso «para toda la organización X» no se le aplica).
6. **Las lecturas entre organizaciones** son dos `SECURITY DEFINER`: `docs_anfitrion(documento,
sub)` devuelve el slug de la anfitriona solo si esa persona tiene allí un permiso `externo`
   vivo sobre ese documento, y `docs_de_fuera(sub, filtros, cursor)` los metadatos de la lista
   de esos documentos, para que **salgan en la lista de su organización** mezclados por fecha.
   El canje resuelve el enlace con `docs_canjear_enlace`, la misma superficie que
   `docs_resolve_share` más el id del enlace. Que un documento es de la organización del token
   se recuerda cinco minutos, para no pagar una lectura más en cada petición.
7. **Quitar el acceso corta el canal en vivo.** Revocar el enlace o quitar a la persona le cierra
   el WebSocket (el `PublishRole` con rol vacío lo cierra). Y lo que no avisa al quitarse —un
   enlace que caduca, un permiso de grupo o de toda la organización que se retira— también:
   cada conexión vuelve a preguntar por su rol cada minuto. Y se cierra cuando caduca su token:
   al reconectar, el nuevo dice lo que vale AHORA (quien salió de la organización ya no trae su
   `tenant`). Si aún puede entrar por otro lado, al reconectar se autoriza de nuevo.
8. **En el diálogo de compartir del dueño, la persona de fuera sale con su nombre**: el directorio
   de su organización no la conoce, y se pregunta a `people:resolve` (0040), que da nombre o
   @usuario y nunca su correo ni su organización.

**Revisión de seguridad** (26/09, antes de fusionar, y otra vez al resolver lo que dejó). Sin
escaladas ni oráculos entre organizaciones. Todo lo que encontró está resuelto: lo concedido a
mano no se desprende de un enlace canjeado; la lista no cuenta lo canjeado de un enlace revocado;
el canje dice el rol real cuando no escribe; quitar a alguien lo bloquea para ese enlace; el
canal en vivo se corta al quitar el acceso; y **el log de peticiones de TODA la suite registra el
patrón de la ruta y no la URL** (`httpx.RequestLogger`, con un guardián que falla si un token
aparece en el log), y **el log de accesos de Traefik no apunta la ruta** (guardián sobre toda
configuración de Traefik del repositorio que lo active, y sobre todo nginx: `access_log off`). En
producción ni el Traefik del frontal ni el del clúster ni el nginx del frontal escriben log de
accesos, y las superficies Astro no registran peticiones. Los
avisos entre organizaciones son el [0053](#0053).

**Alternativas descartadas.** _Copiar el documento a la organización de quien lo abre_ (dos
verdades que divergen al primer cambio). _Mover a la persona a la organización anfitriona_ (le
daría todo lo que su token no dice). _Una sesión efímera con el token del enlace_ (el WS, los
comentarios y las ops necesitan una identidad, y el enlace no la tiene). _Que el canje no dependa
del enlace_ (revocar el enlace dejaría dentro a todos los que lo abrieron).

<a id="0053"></a>

## 0053 — Un aviso es de la persona: la campana enseña los de todas sus organizaciones

`accepted` · 2026-09-26 · hub, avisos, multi-tenancy, seguridad · **completa el [0040](#0040) y el [0052](#0052)** · código en `services/hub/internal/store/notifications.go`, `services/hub/internal/notify/hub.go`, mig. hub `0005`

**Contexto.** Con el 0040 una persona conversa en Rooms con otra organización, y con el 0052 comenta
o edita un documento de otra organización. Los avisos de esa actividad —una mención, un
comentario— los emite el servicio en la organización ANFITRIONA, porque es donde vive el recurso.
Y la campana del Hub listaba por la organización del token: el invitado no los veía nunca. El
dueño lo autorizó el 26/09 «con revisión».

**Decisión.**

1. **Un aviso es de su destinatario, no de la organización.** El `sub` lo emite Sytue ID y es único
   en toda la suite; la dirección de su organización (`mailbox`) la asigna Sytue ID. Quien recibe
   un aviso lo ve y lo marca como leído desde la sesión de CUALQUIERA de sus organizaciones.
2. **La RLS del Hub lo dice con una política propia y estrecha** (`notifications_destinatario_lee`
   y `_marca`, PERMISSIVE, solo `SELECT` y `UPDATE`): una fila es visible si su destinatario es
   uno de los buzones de `app.destinatarios`. Crear avisos sigue siendo en la organización del
   emisor y la purga sigue siendo la de mantenimiento; el aislamiento por organización de todo lo
   demás (presencia, preferencias) no cambia.
3. **Entre organizaciones cuentan solo los buzones PERSONALES, y salen del token**
   (`domain.BuzonesPersonales`): el `sub`, que Sytue ID emite único, y sus direcciones
   principales en TODAS sus organizaciones activas (`mailbox` y el claim `mailboxes`), que Sytue
   ID asigna en un dominio verificado, no se repiten entre organizaciones y **deriva en cada
   emisión del token**: quien sale de una organización deja de tener esa dirección en el
   siguiente. El claim solo va a las superficies de la suite que pintan la campana (una lista
   cerrada, `clientesConCampana`, con un guardián que la cruza con los despliegues; la forja o la
   pasarela, aunque sean globales, no) y nunca en un token delegado: el dominio de cada dirección dice en qué otra organización está, y eso no se le
   cuenta a una app de terceros ni a quien actúa en nombre de otro (0040). Hasta diez: el token
   viaja en la cookie de sesión. Y un token **delegado** (`act`) no abre nada de esto en el Hub. **El correo de
   entrar no, aunque venga con `email_verified`**: ese claim lo pone a true el alta que hace una
   administración, SAML o una federación, sin que nadie pruebe que la dirección es suya, y con él
   quien administra una organización leería en todas las demás los avisos a esa dirección (lo
   encontró la revisión de seguridad). Los avisos al correo de entrar se siguen viendo, como
   antes, en la organización que los mandó. Sin `sub` no hay buzones y la política no deja ver
   nada. El separador (`\x1f`) no puede aparecer en un buzón, y uno que lo trajera se descarta.
4. **Las consultas siguen filtrando por destinatario** además de la RLS: son dos cerraduras, y los
   tests se ven en rojo quitando cada una.
5. **El tiempo real, por persona.** El reparto SSE entrega un aviso a un buzón personal en todas
   las sesiones de esa persona, sea cual sea su organización; uno a su correo de entrar, solo en
   la organización que lo mandó. Los eventos de ESTADO (el avatar, la
   franja de solo lectura que se desbloquea) siguen solo en la organización en la que ocurren.

**Qué se enseña.** El aviso de otra organización lleva su título, el nombre de quien lo hizo y el
nombre del recurso: exactamente lo que ese emisor decidió mandar a esa persona. El enlace lleva al
recurso en la organización anfitriona, donde el 0040 y el 0052 ya deciden si puede abrirlo.

6. **Un aviso de fuera se ve como tal.** La campana (web, bandeja del Hub y Fyne) dice de qué
   organización viene cuando no es la de la sesión (`organization`, que el Hub rellena solo
   entonces). Y **una organización no puede llenar la campana de nadie**: como mucho 200 avisos
   por hora a un mismo destinatario (`notify.TopeAvisosPorHora`); pasado el tope, 429.
7. **De un aviso solo cambia si está leído.** La política de UPDATE es por fila; un trigger
   (`notifications_solo_se_marca`, mig. 0006) la hace por columna: fuera de la fusión de
   organizaciones, cambiar cualquier otra cosa falla.
8. **Lo demás de la persona también es suyo.** Sus reglas de silencio valen para lo que le llega de
   cualquier organización (basta con que lo calle en una); «Tus datos» lleva sus avisos de todas.
9. **Una dirección que deja de ser suya deja de abrirle nada.** Cuando Sytue ID retira una
   dirección, o su titular sale de la organización, el Hub borra sus avisos en todas las
   organizaciones y la apunta con la hora (`buzones_olvidados`, `/internal/notifications/forget`):
   un token emitido ANTES —el de quien la tenía vive unos minutos— ya no la usa, y quien la
   reciba no hereda nada. Sytue ID se lo pide al Hub ANTES de liberar la dirección, y si el Hub
   no contesta la retirada se reintenta en el siguiente barrido.
10. **El tope es por tipo** (organización, destinatario, origen y tipo): quien agote uno no calla
    los demás. Y hay uno total, de 1000 a la hora, para que variar el tipo no lo esquive. Y a OTRA persona, `POST /notifications` contesta 202 sin cuerpo, se entregue o no:
    devolver el aviso —o no devolverlo cuando lo tiene silenciado— diría qué ha callado.

**Revisión de seguridad** (26/09, antes de fusionar, y otra vez al resolver lo que dejó). Sin fugas
por la base: la variable sale solo del token, vive en la transacción y la política no amplía
INSERT ni DELETE. Encontró el hueco del `email_verified` (punto 3) y lo que resuelven los puntos 6
a 10. La tercera pasada descartó una tabla de buzones recordados (dejaba a quien salía de una
organización leyendo sus avisos) a favor del claim `mailboxes`, y las dos últimas lo acotaron a las
superficies de la suite y a tokens no delegados y añadieron el tope total. No queda nada pendiente.

**Tests (vistos en rojo).** `TestAvisosDeLaPersona` (Postgres de verdad): la destinataria ve y marca
el suyo desde su organización, y el de su correo de entrar no le cruza; no lo leen, ni lo cuentan, ni lo marcan uno a uno o todos de golpe
ni otra persona de la misma organización, ni otra de otra organización, ni un token sin buzones.
En rojo quitando el filtro por destinatario del store, y quitándolo además de la política.
`TestDispatchPorPersona` (en rojo con el reparto por organización) y
`TestBuzonesPersonalesSinElCorreoDeEntrar`. `TestAvisosRevision` (Postgres de verdad), en rojo
quitando cada cosa: el trigger, el tope, las reglas de la persona, el olvido del buzón (y que un
token anterior no lo use) y los buzones de sus otras organizaciones.

**Alternativas descartadas.** _Emitir el aviso en la organización de origen del destinatario_ (el
emisor no la conoce: el `people:resolve` del 0040 no la revela, a propósito). _Una copia del aviso
por organización_ (dos que marcar como leídos, y el emisor tendría que saber dónde está cada uno).
_Una función SECURITY DEFINER_ (hubiera repetido en SQL el filtro por destinatario que la política
ya expresa, con más superficie).

<a id="0054"></a>

## 0054 — «Borrar mi cuenta»: la persona se da de baja sola, con 30 días para deshacerlo, y se borra lo suyo y no lo de su organización

`accepted` · 2026-09-26 · id, rgpd, google-play, seguridad · código en `services/id/internal/http/account_delete.go`, `services/id/internal/store/borrado.go`, `services/id/internal/merge/personas.go`, mig. id `0098`

**Contexto.** Google Play rechazaba la seguridad de los datos de las trece apps: exige declarar
cómo se crean las cuentas y, si se crean, una web donde pedir que se borren. La cuenta se crea en
Sytue ID (usuario, contraseña y SMS) y nadie podía borrarla por sí mismo: solo existía la baja
desde Sytue Admin (mig. 0092) y la de SCIM. El dueño lo pidió el 26/09.

**Decisión.**

1. **Una página de Go con URL estable**, `https://id.<dominio>/account/delete`, y no de la app
   «Mi cuenta»: la reautenticación la hace el IdP con su cookie, que la app no ve. La app enlaza
   a ella desde su tarjeta «Borrar mi cuenta»; Fyne llega por «Mi cuenta», que abre esa misma
   app. Va en `id.ingress.rutasDeCuenta`, y el test de Ingress lo vigila.
2. **Confirmación fuerte.** Se escribe la cuenta (@usuario o correo) y se demuestra que es ella
   ahora: la contraseña, o una identificación de menos de cinco minutos (`frescuraStepUp`). Con
   segundo factor, la contraseña sola no vale —robarla bastaría para borrar una cuenta que para
   entrar pide dos—: hay que volver a entrar con los dos o con una passkey
   (`/login/?return_to=/account/delete`). El POST comprueba el origen y va limitado como el login.
3. **No se puede dejar una organización sin nadie que la administre.** Si es su única
   propietaria activa, o la administra y no hay otra propietaria ni administradora activa, se
   bloquea y se dice cuáles. La comprobación va dentro de la transacción, con las organizaciones
   bloqueadas `FOR UPDATE`, para que dos copropietarias que se van a la vez no se crucen. La
   cuenta personal no cuenta: es de la persona.
4. **Periodo de gracia: el de la baja que ya existía**, 30 días (`retencionBaja`, mig. 0092). La
   baja propia es la misma baja con `closed_by` = la propia cuenta, que solo se puede escribir
   así: Sytue Admin no deja darse de baja a uno mismo y SCIM no escribe `closed_by`. **Un
   administrador no la puede deshacer** —es la supresión que pidió la persona—: `RestoreUser` y
   el listado de bajas recuperables la excluyen, y solo la deshace privacidad a petición suya
   (privacy@sytue.eu).
5. **Lo que sirve para entrar se borra al confirmar**, en la misma transacción, no al vencer la
   ventana: sesiones, refresh, códigos, contraseña, passkeys, TOTP, códigos de recuperación,
   contraseñas de aplicación, dispositivos, enlaces con IdP externos, elevaciones vivas y la foto
   (también la de las fichas de Contactos). Restaurar devuelve la cuenta, no el acceso: quien
   vuelve pone contraseña nueva por la recuperación.
6. **Lo que otros servicios guardan de ESA persona y no de su organización** —avisos y
   presencia del Hub; push, preferencias, borradores, guardados, lectura, hilos seguidos y
   categorías de Rooms; favoritos de Drive— se borra en el barrido siguiente (cada diez
   minutos), por el mismo camino que la fusión: mismas credenciales, `InMergeTx`. La lista es
   cerrada (`merge.DatosDeLaPersona`) y un test comprueba que cada tabla existe con su
   `subject` y su política de fusión.
7. **Lo de la organización se queda en la organización**: correo, archivos (también «Mi unidad»,
   que es del tenant), eventos, mensajes en canales. Lo decide quien la administra. Las
   facturas, el plazo legal; el registro de seguridad, doce meses.
8. **Vencida la ventana, la ficha se anonimiza** (correo, @usuario, nombre, móvil, correo de
   recuperación, atributos) y se quitan sus membresías. No se borra la fila: la referencian
   tablas sin cascada, y su buzón —que es de la organización— sigue ocupando la dirección para
   que nadie se quede con ella y reciba lo que siguen mandándole. Una cuenta purgada no vuelve a
   recibir @usuario.
9. **El contenido de una cuenta personal** (su correo, sus archivos, sus fotos) deja de estar
   accesible al confirmar y su borrado, al vencer la ventana, **es manual**: vive en diez bases y
   en el almacén de objetos y no hay todavía un borrado de tenant. El barrido lo avisa a nivel
   error, que dispara la alerta, y corre el mes del art. 12.3 RGPD. Está en `roadmap.md`.

**Revisión de seguridad (2026-09-26), todo resuelto antes de fusionar.** Una cuenta que ya no
está activa deja de valer al instante también con un access token vivo: `requireAccessToken` y la
introspección miran su estado, y la verificación de contraseñas de aplicación de Mail también
(antes una baja de Sytue Admin o de SCIM seguía abriendo el buzón por IMAP). El recuento de
propietarias (`CountOrgOwners`) cuenta solo las activas y bloquea la organización, así que una
baja de Sytue Admin y una baja propia no se cruzan. Una nueva baja borra la marca de datos ya
borrados; la purga repite el borrado de credenciales; el borrado en otros servicios espera a que
caduque el access token (los servicios validan el JWT por su cuenta), va en orden aleatorio para
que una cuenta que falla no tape a las demás, y sus fallos van a nivel error. La contraseña fallada
consume el mismo cupo por cuenta que el login. La última administradora de la plataforma no se
puede ir, y la baja quita la administración de plataforma. Con segundo factor, una passkey sin
verificación de usuario no basta. La cuenta personal purgada se queda con su id como nombre. En la segunda y la tercera vuelta de
la revisión: los recuentos de propietarias y administradoras cuentan las activas **más la cuenta
afectada** (si no, no se podía retirar a una propietaria suspendida habiendo otra activa), el
bloqueo es `FOR NO KEY UPDATE` para no frenar los inicios de sesión, la administración de la
plataforma se bloquea también, y si no se puede leer el estado de la cuenta el token no vale.
La tercera vuelta no encontró nada.

**Alternativas descartadas.** _La página en la app «Mi cuenta»_ (no ve la cookie del IdP y
habría que inventar un reto por API). _Borrar la fila de `users`_ (fallaría con las referencias
sin cascada y liberaría la dirección de correo). _Un endpoint de borrado por servicio_ (diez
secretos y diez despliegues para borrar unas filas que ya se alcanzan como en la fusión).
_Esperar a los 30 días para borrar las credenciales_ (una cuenta que su titular ha pedido borrar
no debe poder usarse ese mes). _Dejar que la contraseña baste siempre_ (rebaja el segundo factor).

<a id="0055"></a>

## 0055 — La URL canónica de cada producto es su host de producto; la organización va al PATH, no al subdominio

`accepted` · 2026-09-26 · web, id, despliegue · código en mig. id `0099`, `deploy/helm/charts/sytue-frontend/templates/deployment.yaml`, `values.yaml` (`orgHost`), guardián `platform/pkg/migcheck/migcheck_test.go`

**Contexto.** Entra en un producto y el login da vueltas: `rooms.sytue.eu` → 302 a
`corporate.sytue.eu/rooms` (el host del `redirect_uri` canónico) → IdP → vuelta. La cadena la
provoca la canónica de organización (mig. 0054): el BFF empieza SIEMPRE el flujo OIDC en el host
del callback (`bff.ts`), y la cookie de sesión nace ahí. Con más interacciones ENTRE
organizaciones y con externos (enlaces de reunión, comparticiones, el canje de la 0052), el
modelo que escala es el host de PRODUCTO —que ya usan los enlaces entre apps (`appUrl`), los
avisos (`hubnotify`) y las apps nativas (`api.ServiceURL`), los tres sin saltos—, y el lugar de
la organización es el PATH dentro del producto, no el subdominio: `rooms.sytue.eu/rooms/<org>/…`
hace la org compartible por enlace y deja los subdominios para productos y para el dominio de
CORREO.

**Decisión.**

1. **Canónica = host de producto con prefijo**: `https://mail.sytue.eu/mail/auth/callback`
   (mig. 0099, trece clients; `platform` sin prefijo, `account` sigue global en
   `id.<dominio>/account` de la 0097). El chart compone `PUBLIC_URL` y
   `SYTUE_OIDC_REDIRECT_URI` con el subdominio de la app; entra donde entre el navegador, el
   login ya no salta de host. NINGÚN BFF cambia: el salto a `SYTUE_OIDC_REDIRECT_URI` era el
   mecanismo, no el problema.
2. **El host de organización queda en transición, no se retira.** Sigue sirviendo el MISMO
   prefijo (el `base` de Astro se hornea) y su callback sigue registrado (0054 y hermanas): un
   login empezado ahí funciona, con su 302 a producto. La decisión del 2026-08-14 («las dos
   direcciones válidas») sigue en pie; lo que cambia es CUÁL se anuncia. Cuando la fase 2 ponga
   la organización como segmento de PATH, el comodín pasa a 301 y se retira; el subdominio
   sobrevive como dominio de correo.
3. **El guardián exige la canónica.** `TestRedirectUrisDeLosHostsDeSuiteCubrenElChart`
   comprueba AHORA TAMBIÉN `https://<subdomain>.<dominio><path>/auth/callback` para cada app
   del chart, visto en rojo quitando una fila de la 0099. Un `redirect_uri` que no casa no da
   error de despliegue — da un login que falla solo en ese host.
4. **La org por path es la FASE 2, app a app**: rutas bajo `[org]/` tras el prefijo de la app,
   helpers `r()` y `appUrl` org-aware, aviso y deep-link nativo con org, y entrada sin org
   redirigida a la activa. La sesión no cambia: la organización activa viaja firmada en el
   token (claim `tenant`) y ningún servicio la lee del host hoy.

**Por qué y no otra cosa.** _Segundo juego de imágenes como en `sytue.com`_ (0049) no hace
falta: no cambia el dominio, solo el host que anuncia cada imagen ya desplegada, y el único
fichero que ata imagen a dominio (`tokens.ts`) no se toca. _Resolver las URL por cabecera Host_
lo descartó el 0049 por obligar a enlaces relativos; aquí no se resuelve nada en ejecución: la
canónica es una constante más del chart. _Cookies de dominio compartido_ para no repetir login
entre productos rompería el aislamiento deliberado de `__Host-` (`cookies.ts`, riesgo R6) y no
hace falta: el SSO lo aporta el IdP. _Registrar solo la canónica de producto_ y borrar la de
organización dejaría sin login a quien entre por el host viejo hasta que la fase 2 termine.

**Precedentes medidos.** Dev loguea por host de producto desde 0002 (forma raíz), y el frente
`sytue.com` (0049/0094) loguea por host de producto CON PREFIJO —exactamente la forma de esta
migración— desde su alta.

<a id="0056"></a>

## 0056 — La organización como segmento de PATH dentro de cada producto

`accepted` · 2026-09-26 · web, nativas, avisos · código en `apps/frontend/packages/config/src/organizacion.ts`, `astro-middleware.ts` (`middlewareDeOrganizacion`), mig. none, guardián `middleware-org.test.ts`

**Contexto.** El ADR 0055 dejó la canónica en el host de producto y anunció el norte: con más
interacciones ENTRE organizaciones y con externos (reuniones, comparticiones, avisos, el canje),
los enlaces tienen que decir A QUÉ ORGANIZACIÓN apuntan, y el sitio de la org es el PATH
(`rooms.sytue.eu/rooms/<org>/channels/x`), no el subdominio. La sesión no cambia: la org activa
viaja en el token (claim `tenant`) y ningún servicio la lee del host.

**Decisión.**

1. **Las páginas NO se mueven.** Un `[org]` en el router de cada app sería un recorrido de
   trece apps por cientos de ficheros con un solo beneficio tipográfico. Lo que cambia es el
   middleware compartido (`middlewareDeOrganizacion`, compuesto ANTES del de idioma): una URL
   con org se sirve por dentro (rewrite a la ruta real, que sigue con su prefijo) con la org en
   `locals.org`; una URL sin org con sesión se redirige a la canónica con un 302 LOCAL (mismo
   host, sin IdP); sin sesión pasa, porque la guarda manda al login y al volver el 302 entra
   solo.
2. **Idempotente por construcción, no por fe.** El rewrite lleva la marca `?__org=<org>`: si el
   runtime reejecuta el middleware sobre la petición reescrita, la segunda pasada ve la marca y
   sirve. No se depende de cómo reejecute (o no) el runtime. El 301 que cierra la transición del
   host de organización se prepara tras `orgHost.retirarComodin` (chart de frontends), apagado
   por defecto: es la fase final y la enciende el operador en su despliegue de cierre.
3. **El primer segmento tras el prefijo es SIEMPRE la org, por construcción.** Distinguir
   «¿org o ruta?» con una lista de rutas es pedir que una página nueva rompa el parsing en
   silencio. Las URL sin org son transicionales; las excepciones (`api`, `auth`, assets,
   `healthz`, y lo que cada app declare: `s` de Docs, `public` de Drive, `p` de Fotos) pasan
   sin org.
4. **LA ORG DEL PATH NO AUTORIZA.** Autoriza el token (claim `tenant`) y el servicio. Si la org
   del path difiere de la de la sesión NO se redirige a ciegas: un externo con enlace de otra
   organización tiene que aterrizar donde está — el canje (0052), los externos de Rooms (0040)
   y las comparticiones lo resuelven. Redirigirle a SU org sería romperle el enlace.
5. **Los enlaces entre apps, los avisos y las nativas llevan la org cuando la conocen**
   (`appUrl(key, path, org)`, `hubnotify`, `api.AppURL`); los que no la llevan siguen
   funcionando con el 302 del punto 1. La barra de direcciones converge a la forma con org.
6. **Interruptor operativo:** `SYTUE_ORG_PATH=off` apaga el comportamiento entero sin
   desplegar código nuevo.

**Por qué y no otra cosa.** _Mover las páginas bajo `[org]/`_: cientos de ficheros y guardianes
de paridad para el mismo resultado de URL. _Org por query_: no es un path, no se comparte igual
y el ADR lo pedía como segmento. _Cookie por org para multi-org simultánea_: es otra ola (la
sesión lleva UNA org activa; dos pestañas de orgs distintas comparten sesión y la verja del
middleware no lo simula). _Redirigir en desacuerdo org del path ≠ sesión_: rompe el enlace del
externo (punto 4).

**Ambito de esta ola.** Las doce superficies de producto (no `www`, no `platform`, no `account`,
que son globales). La retirada del comodín `<org>.sytue.eu` como 301 y la baja de la publicación
DNS de organizaciones quedan para el despliegue que cierre la transición.

<a id="0057"></a>

## 0057 — Pedir firma a alguien de otra organización: el PDF sale por un enlace de Drive

`accepted` · 2026-09-27 · docs, firma, multi-tenancy, compartir, seguridad · **completa el [0051](#0051) §7** (que lo dejó descartado «hasta que se decida cómo») · código en `services/docs/internal/http/pdffirmaexterna.go`, `…/pdfpedirfirma.go`, `services/docs/internal/drive/cliente.go`, `platform/pkg/peticiones/avisos.go` (`DeFuera`), mig. docs `0012`

**Contexto.** «Pedir firma» del [0051](#0051) solo llegaba a gente de la misma organización: el
PDF vive en la unidad de la organización y quien no es de ella no puede abrirlo. El dueño decidió
el 26/09 que se pueda pedir también fuera: **primero se comparte el PDF por Drive, reutilizando lo
que Drive ya tiene, y después se pide la firma**; la interfaz detecta que la persona es de fuera,
explica el paso y lo hace con confirmación. Sin un sistema de permisos nuevo. Lo único que Drive
tiene hoy para que algo salga de la organización es el **enlace de solo lectura** (`kind=link`):
las comparticiones con personas y grupos se resuelven por la organización del token.

**Decisión.**

1. **Quién es de fuera lo dice Sytue ID.** Por correo, primero `directory:resolve` en la
   organización de quien pide, como siempre; si no está, o si se escribe un @usuario,
   `people:resolve` del [0040](#0040) (`peticiones.DeFuera`): su `sub` y su nombre, nunca su
   correo ni su organización. Y si ese `sub` resulta ser de la organización (un @usuario de un
   compañero), es de dentro. Por `sub` solo se pide dentro. Antes de buscar a nadie se comprueba
   que el PDF es de quien pide.
2. **Nada sin confirmar, y a quién.** Con alguien de fuera, el servicio contesta 409
   `pdf_signature_recipient_external` con su nombre y un `confirm`, y no crea nada. La interfaz
   (web y Fyne) lo explica —un enlace de Drive de solo lectura que caduca en 30 días, que nadie
   ve, que deja una copia en la unidad de la otra persona, visible para su organización— y al
   confirmar manda la misma petición con ese `confirm`: si ya no resuelve a la misma persona, vuelve
   a preguntar.
3. **El enlace es de Drive y lo crea quien pide.** El servicio llama a `POST /api/v1/shares` de
   Drive con la sesión de quien pide: su permiso sobre el fichero y **la verja de etiquetas de
   Drive** (una etiqueta con `BlockExternalShare` lo bloquea aquí igual:
   `pdf_signature_share_blocked`). El enlace sale en la lista de comparticiones del fichero en
   Drive, donde se puede revocar. Los topes se comprueban antes de crearlo.
4. **El token no lo ve nadie.** Se guarda **cifrado** (`enlace`, con la clave del servicio,
   `SYTUE_DOCS_SECRET_ENC_KEY`, y **atado a su petición** como dato asociado del AEAD: no vale en
   otra fila ni para otra cosa; sin clave no se ofrece) y solo lo usa este servicio: cuando quien
   firma abre la petición (`POST …/{id}/open`), baja el PDF por el enlace y deja una copia en SU
   unidad (carpeta «Sytue Docs»); si la borra, al abrir otra vez se trae otra. No viaja en el
   aviso, ni en un correo, ni a ningún navegador, ni a las trazas (`telemetry.Middleware` nombra
   el span por el patrón de la ruta). Se borra de la fila al cerrarse la petición sin firmar y al
   traerse el firmado; el enlace del original lo revoca la sesión de quien pidió en cuanto la
   petición deja de estar pendiente (al cancelar, al traerse el firmado, o la próxima vez que
   lista las suyas, que es lo único que puede hacerlo tras un rechazo), y la caducidad es la red.
5. **La petición vive en la organización de quien pide**, como el documento del [0052](#0052):
   quien firma es una fila con `externo`. Las rutas de una petición (`/signature-requests/{id}/…`)
   pasan a esa organización para su destinatario de fuera (`peticionEnAnfitrion`), **sin nada de
   su token salvo el `sub`**, y cada sentencia sigue exigiendo destinatario o solicitante. Las
   lecturas entre organizaciones son tres `SECURITY DEFINER`: `docs_firma_anfitrion(id, sub)` (solo
   el slug, solo a su destinatario de fuera), `docs_firmas_de_fuera(sub, tenant)` para su lista,
   **sin la ruta del original ni su id** (dirían cómo organiza sus carpetas otra organización), y
   `docs_firmas_pendientes_para(sub)` (un número, para el tope).
6. **Lo firmado es EL documento.** La petición guarda la huella (SHA-256) del original al pedir.
   La copia que se trae quien firma tiene que tener esa huella —si el original cambió, no se
   trae—, y al entregar, su copia también: si la tocó, firmaría otro documento. Lo entregado tiene
   que tener más firmas, la última cubriéndolo entero ([0051](#0051) §7) y, además, **las mismas
   páginas con el mismo texto** que el original. Firmar reescribe el fichero la primera vez, así
   que no se puede exigir que empiece por el original; lo que no cambia es lo que se lee, y sin
   esto se podría entregar cualquier otro PDF firmado. Esta comprobación vale también para las
   peticiones de dentro. En un escaneo sin capa de texto solo ata el número de páginas.
7. **La entrega vuelve igual.** Al entregar, el servicio crea con la sesión de quien firma un
   enlace a la copia firmada (7 días), cifrado igual, y guarda la huella de lo comprobado. Quien
   pidió se la trae a SU unidad (`POST …/{id}/fetch-signed`), con el nombre del original y
   «(firmado)», y tiene que ser exactamente lo entregado: si el fichero cambió después en la otra
   unidad, no se trae. Hasta entonces quien pidió no ve ni la ruta ni el id del firmado.
8. **Avisos, sí; correo, no; y nunca el correo de nadie.** El aviso del Hub llega a la persona en
   cualquiera de sus organizaciones ([0053](#0053)) y lleva a CertiPDF en la SUYA (`/docs/pdf` sin
   organización ni ruta: la portada, donde está la petición). Sin correo: Sytue ID no da el suyo,
   y mandar desde avisos@ a la dirección que teclee cualquiera sería prestar el remitente. A cada
   lado se le nombra por su nombre (o lo que tecleó quien pidió), nunca por el correo de su token.
9. **Nadie llena la lista de otra persona.** Además de las 30 por hora de quien pide, alguien de
   fuera puede tener como mucho 5 pendientes de una misma persona y 20 de todas las organizaciones
   (`pdf_signature_request_recipient_busy`), contadas bajo cerrojo. «Abrir» y «traer» van de una
   en una por petición (`ocupada`, que caduca a los dos minutos) y con tope por persona, porque
   cada vuelta baja y sube hasta 25 MiB.

**Consecuencias.**

- La copia de quien firma queda en la unidad de SU organización, que en Drive es un espacio de
  toda la organización: lo dice la interfaz antes de confirmar y en la fila de la portada.
- Esto no abre ninguna vía nueva de sacar ficheros: es la misma que el botón «Compartir» de
  Drive, con el token guardado en vez de enseñado. En Drive cualquiera de la organización puede
  crear un enlace a un fichero que ve; si una organización quiere limitarlo, es una política de
  Drive, no de esto.
- El enlace de la copia firmada vive en la unidad de quien firma y solo lo puede revocar esa
  persona (desde Drive); su token se borra al traerlo y caduca a los 7 días.
- La clave del servicio es una sola, como la de los conectores: si se cambia, las peticiones de
  fuera pendientes dejan de poder abrirse (`pdf_signature_request_link_gone`) y hay que pedirlas de
  nuevo. Una bajada de la migración 0012 borra las filas de fuera; sus enlaces quedan hasta caducar
  con un token que ya no tiene nadie.

**Alternativas descartadas.** _Compartir en Drive con la persona (`kind=user`) entre
organizaciones_: Drive resuelve todo por la organización del token; abrirlo es el sistema de
permisos nuevo que el dueño descartó. _Mandar el enlace en el aviso o por correo_: sería un
secreto al portador en el Hub, en un buzón y en el historial. _Guardar el PDF en este servicio_:
25 MiB en la base por petición, lo mismo que descartó el [0051](#0051). _Exigir que lo firmado
empiece por el original_: la primera firma reescribe el fichero.

**Revisión de seguridad** (27/09, antes de fusionar, independiente). Sin hallazgos altos. Todo lo
que encontró está resuelto: lo firmado no se ataba al original (huellas y mismo texto, punto 6);
nada limitaba cuántas peticiones recibe alguien de fuera (punto 9); un @usuario de un compañero
salía como de fuera (punto 1); el correo del token llegaba a la otra organización y se intentaba un
correo con la sesión equivocada (punto 8); enlaces sin revocar tras un rechazo o sin traer (punto
4); el span de trazas con la URL entera, que llevaría el token de un enlace público (punto 4; el
middleware no estaba montado en ningún servicio, pero ya no puede); el nombre del firmado lo ponía
la otra organización (punto 7); una copia borrada dejaba la petición sin salida y dos «abrir» a la
vez dejaban dos copias (puntos 4 y 9); el token cifrado no iba atado a su fila (punto 4); los topes
no eran atómicos y el enlace se creaba antes de comprobarlos (puntos 3 y 9); se buscaba a la
persona antes de comprobar el fichero y la confirmación no decía a quién (puntos 1 y 2); la
migración no cerraba `copia_ruta` ni los tokens de una petición cerrada (restricciones de la 0012);
y un 401 de Drive se contaba como «etiqueta que no deja compartir» (`drive.ErrProhibido`). Las dos
cosas de producto que dejó para el dueño —que alguien pueda **bloquear** a quien le pide firmas y
que una organización pueda **cerrar** las que llegan de fuera— se decidieron el 27/09: [0059](#0059).

**Tests (vistos en rojo).** `TestPedirFirmaAFuera`, `TestCancelarAFueraQuitaElEnlace`,
`TestPeticionEnAnfitrion` y `TestElTokenNoValeEnOtraPeticion` (servicio); `TestPeticionesDeFirmaAFuera`
y `TestNadieLlenaLaListaDeAlguienDeFuera` (Postgres de verdad); `TestElSpanNoLlevaLaURL`
(telemetría); y los de la web, el SDK y Fyne. Cada guardián se vio en rojo quitando lo que vigila.

<a id="0058"></a>

## 0058 — El CI corre en GitHub Actions; los workflows se portan sin cambiar lo que comprueban

`accepted` (los puntos 1 y 5, sustituidos por [0060](#0060)) · 2026-09-27 · CI, operación · código en `.github/workflows/`, `.github/actions/`, `.github/RUNNERS.md`, `deploy/sytue-git/bin/{ci-check,publicar-asset}.sh`, guardianes `deploy/production/tests/release-workflow-test.sh`, `.github/tests/ci-sin-azar-test.sh`, `scripts/check-toolchain-versions.sh`

**Contexto.** El origen del repositorio pasa de git.gay (Forgejo) a
`github.com/thinkddr/sytue-one`. Los veinticinco workflows de `.forgejo/workflows/` ya
estaban escritos en la sintaxis de GitHub Actions —contexto `github.*`, `GITHUB_TOKEN`,
`hashFiles`— porque Forgejo la copia; lo que los ataba a la forja era el entorno (tres
runners propios en el frontal, con un Docker filtrado y la caché de Go montada del
host), la plomería que evitaba leer otros repositorios (clonado con `git` a mano,
`upload-artifact` vendorizada, un guion propio contra el servidor de caché del runner) y
una docena de llamadas a `/api/v1`.

**Decisión.** Portar los veinticinco tal cual —los mismos gates, los mismos
`timeout-minutes`, los mismos secretos— cambiando solo lo que GitHub hace distinto:

1. **Runners alojados de GitHub** (`ubuntu-24.04-arm`) y no los tres runners propios
   re-registrados: registrar el `actions-runner` de GitHub en un host que también sirve
   producción es un cambio de infraestructura que no se puede probar desde el
   repositorio, y los alojados no piden ninguna: el repositorio es privado, así que
   gastan los minutos del plan (y se facturan al agotarlos), que es el precio de no
   operar máquinas. Cada job corre en
   el MISMO contenedor por digest que daban los runners de Forgejo (`node:24-bookworm`),
   para que la plomería que instala herramientas como root siga valiendo sin tocarla.
   ARM64 y no amd64 porque es la arquitectura sobre la que está medido todo lo que hay.
2. **Acciones oficiales fijadas por SHA** (`checkout`, `upload-artifact`, `cache`) en
   lugar del clonado a mano, la copia vendorizada y el guion de caché: la razón para
   evitarlas (la forja no dejaba leer otros repositorios) ya no existe, y el gate de
   toolchain exige que cada `uses:` lleve el SHA de `versions.env`.
3. **API de GitHub donde la forja hablaba otra cosa:** check runs en vez de estados de
   commit para decidir si un commit está en verde, `git/refs` para marcar una versión
   como desplegable, `upload_url` y `releases/assets/<id>` para los assets, `pull_requests`
   del listado de runs para el cancelador, Renovate en modo `github`.
4. **`permissions:` mínimos por flujo**, que Forgejo ignoraba y GitHub aplica.
5. **Lo que sigue en git.gay se queda ahí y se nombra:** los forks privados de Fyne
   (`SYTUE_FORKS_SERVER`, `SYTUE_GIT_FORKS_TOKEN`) y el registro de imágenes de la suite
   (`DESTINO`, con `vars.SYTUE_REGISTRY` para moverlo). Cambiar el registro es otra
   decisión, del despliegue, no del CI.

**Alternativas descartadas.** _Runners propios en GitHub_ (el frontal re-registrado): es
la vía de vuelta documentada en `RUNNERS.md`, no el punto de partida. _`ubuntu-24.04`
amd64_: iguales de máquina y de precio; se descarta solo por no cambiar dos cosas a la
vez (la forja y la arquitectura medida). _Reescribir los flujos «a la GitHub»_ (setup-go,
setup-node, buildx): tiraría los gates de versiones por sha256 y de imágenes por digest
que existen para que el CI no cambie solo.

**Consecuencias.** La caché de Go (~9 GB de módulos) compite con el tope de 10 GB por
repositorio de `actions/cache`; si expulsa, se cachea solo la de compilación. `schedule:`
vuelve a funcionar (en git.gay no se disparaba nunca) y el timer del piloto que lo suplía
sobra. En el inventario de cadena de suministro `github.com` y `api.github.com` quedan
como `temporary-exception` renovada hasta 2027-03-27: la forja está fuera de la UE y el
gate obliga a volver a decidirlo cada seis meses, que es lo honesto —`pinned-artifact`
diría que no sale nada nuestro, y salen el código y las incidencias—. Los secretos se
dan de alta en GitHub con los mismos
nombres; el de paquetes y el de los forks siguen siendo de git.gay. `desplegar-forja.yml`
y `espejo-herramientas.yml` (Sytue Git, retirado) y los flujos contra el registro de
Scaleway (en baja) se portan a mano y sin disparo automático, pendientes de su baja formal.

<a id="0059"></a>

## 0059 — Peticiones de firma de fuera: quien recibe bloquea y su organización decide, sin que quien pide se entere

`accepted` · 2026-09-27 · docs, firma, multi-tenancy, privacidad, seguridad · **completa el [0057](#0057)** · código en `services/docs/internal/http/pdffirmabloqueos.go`, `services/id/internal/http/internal_organizaciones.go`, `apps/frontend/apps/console/src/components/ExternalSignaturesPanel.tsx`, mig. docs `0013`

**Contexto.** Con el [0057](#0057) cualquiera que sepa un @usuario puede pedir una firma a esa
persona desde otra organización, y abrir la petición deja una copia en la unidad de SU
organización. La revisión de seguridad del 0057 dejó dos decisiones al dueño, y el 27/09 las tomó:
**sí** a que una persona bloquee a quien le pide firmas desde fuera —desde la petición recibida, y
se quita desde sus ajustes—, y **sí** a un ajuste en Sytue Admin para que la organización decida
qué hace con las que le llegan de fuera: abiertas (por defecto), cerradas o solo de organizaciones
de confianza. **Quien pide no se entera**: la petición se «envía», pero no llega.

**Decisión.**

1. **No llegar es `silenciada`.** Una petición de fuera que no debe llegar se crea igual que
   cualquier otra —la misma respuesta, el mismo enlace de Drive, la ve en su lista, cuenta en sus
   topes y la puede cancelar— pero no llega: sin aviso, fuera de la lista de quien firma, sin
   organización anfitriona y sin poder abrirse, firmarse ni rechazarse (las sentencias de quien
   firma exigen `NOT silenciada`, además del middleware). Decirle a quien pide que no llegó le
   contaría a quién le ha bloqueado alguien o en qué organización está una persona, que
   `people:resolve` calla a propósito ([0040](#0040)). Por lo mismo:
   - **se decide igual siempre**: primero Sytue ID, luego la política y luego el bloqueo, sin
     atajos —un bloqueo que cortara antes se notaría en el tiempo y en el 503 de un Sytue ID caído—;
   - **el tope de 20 pendientes de fuera de quien recibe ya no devuelve 429**: lo que pase de ahí
     se crea silenciado. Con un 429 quien pide podría llenar ese tope desde otras cuentas y contar
     cuáles de sus peticiones llegaron. Los topes que cuentan lo de quien pide (por hora y por
     par) siguen siendo errores: los podría contar él mismo;
   - lo silenciado no cuenta para ese tope, y el bloqueo se mira otra vez DENTRO del INSERT: uno
     que llegue entre la comprobación y la creación también silencia.
2. **El bloqueo es de la persona**, no de una organización: vale desde cualquiera de las suyas y
   contra quien pide desde cualquiera de las suyas (`pdf_firma_bloqueos`, con `subject` = quien
   bloquea y un `tenant` que solo dice desde dónde lo hizo). Nadie lo lee directamente: todo pasa
   por funciones `SECURITY DEFINER` que reciben el `sub` del token de quien bloquea. Bloquear
   silencia lo que ya estaba pendiente de esa persona en todas las organizaciones; desbloquear no
   resucita lo silenciado. Hasta 500 por persona, bajo cerrojo.
3. **Dónde se hace.** En la franja de la petición recibida (web y Fyne), con confirmación en la
   misma franja y en cualquier estado de la petición —también tras rechazarla o firmarla—; se
   quita en la portada de CertiPDF, en «Personas bloqueadas», que es donde están sus peticiones:
   CertiPDF no tiene otra pantalla de ajustes y la lista viaja con las peticiones. Los bloqueos
   van en «Tus datos» (`signature_blocks`) y «Borrar mi cuenta» se lleva los de la persona
   (`DatosDeLaPersona`, [0054](#0054)).
4. **La política es de la organización** (`pdf_firma_politica`, con su RLS y su
   `_platform_merge`); la guarda quien administra (`owner`/`admin`, no delegable) desde «Firmas de
   fuera» en Sytue Admin, y se aplica en dos momentos:
   - **Al pedir**, el servicio pregunta a Sytue ID en qué organizaciones activas está quien firma
     (`POST /internal/people/orgs`, solo desde dentro y con un secreto PROPIO,
     `sytue-id-orgs-persona`, compartido solo con Docs; sin él no se pide fuera) y llega si
     **alguna** la admite —abierta o sin fila, o de confianza con la organización de quien pide en
     su lista— o si la persona tiene **cuenta personal**, donde decide ella con sus bloqueos. La
     cuenta personal no viaja por su nombre (se deriva de su dirección): Sytue ID solo dice que la
     hay. Si Sytue ID no contesta, no se crea nada: dejarla llegar se saltaría la política.
   - **Al mirar, abrir y firmar**: la lista de quien firma solo enseña lo que admite la
     organización de SU sesión, y abrir o firmar desde una que no lo admite es
     `pdf_signature_org_closed` —dejaría ficheros en la unidad de esa organización—. Rechazar y
     bloquear se pueden siempre.
5. **Las organizaciones de confianza** se escriben por el nombre de su dirección (el `acme` de
   `…/docs/acme/`), hasta cien, sin la propia. Las organizaciones de Sytue ID no se borran (pasan
   a `deleted`) y su nombre no se reutiliza, así que la confianza no la hereda nadie. En una fusión
   (y en un renombrado, que usa el mismo motor) la política del origen se descarta si el destino
   tiene la suya, y quien confiaba en el origen pasa a confiar en el destino, sin repetidos ni
   confiar en sí misma (`platform_merge_prepare` de Docs, solo dentro de una fusión).
6. **Las funciones que cruzan organizaciones, también en producción.** En producción la aplicación
   y las migraciones usan el MISMO rol, dueño de todo y sin BYPASSRLS; con FORCE ROW LEVEL
   SECURITY, un `SECURITY DEFINER` por sí solo no salta la RLS (lo encontró Drive en su 0003), y
   en los tests lo tapaba un dueño superusuario. Era así en TODAS las de Docs: el enlace público
   de un documento, el canje entre organizaciones ([0052](#0052)) y las peticiones de fuera
   ([0057](#0057)). Todas llevan ahora `SET app.docs_definer = 'on'` —vale mientras se ejecutan y
   se restaura solo al salir— y cada tabla que leen tiene una política aditiva que solo abre con
   ese valor. Los tests de integración de Docs ponen esas funciones en manos del rol de la
   aplicación, como en producción, para que no vuelva a taparse.

**Consecuencias.**

- Quien pide ve «pendiente» para siempre en una petición que no llegó; puede cancelarla. Una
  silenciada por la política sigue así aunque la organización abra después.
- Si una organización cierra, las peticiones de fuera —también las ya firmadas o rechazadas—
  dejan de verse desde sus sesiones; desde una cuenta personal se siguen viendo.
- Un aviso que ya llegó antes del bloqueo se queda en la campana y lleva a la portada de CertiPDF,
  donde la petición ya no está.
- La lista de bloqueados guarda el nombre que la persona vio en la petición: es SU registro de a
  quién bloqueó, y se va con ella.
- Drive no apunta cuándo se usa un enlace, así que el enlace de una silenciada que nadie usa no
  dice nada que no diga una petición ignorada.
- Bajar la migración 0013 cancela las silenciadas: quien las pidió las verá «canceladas».

**Alternativas descartadas.** _Decir «bloqueado» o «no admitido» a quien pide_: filtraría el
bloqueo y las organizaciones de la persona. _No crear el enlace de Drive a una silenciada_: la
lista de comparticiones del fichero en Drive delataría la diferencia. _Aplicar la política solo al
mirar_: el aviso ya habría llegado. _Guardar los bloqueos por organización_: bloquear en una no
serviría en las otras. _Que la cuenta personal no cuente_: alguien no podría recibir nada en lo
suyo por estar además en una organización cerrada. _Reutilizar el secreto del directorio interno_:
es de una tarea de mantenimiento que se retira.

**Revisión de seguridad** (27/09, antes de fusionar, independiente). Sin hallazgos altos; todo lo
que encontró está resuelto: el tope de quien recibe servía de oráculo (punto 1); el bloqueo cortaba
antes de preguntar a Sytue ID (punto 1); las funciones fallaban abiertas si el dueño no salta la
RLS, y resultó que TODAS las de Docs estaban así (punto 6); el secreto del directorio pasaba a un
uso permanente y la cuenta personal viajaba por su nombre (punto 4); el texto de «cerrada»
prometía de más con las cuentas personales; firmar no miraba la política (punto 4); un bloqueo
entre la comprobación y el INSERT (punto 1); los bloqueos como datos personales (punto 3); solo se
bloqueaba desde una pendiente (punto 3); la bajada y su `app.merge`; el tope de bloqueos sin
cerrojo y el «demasiados» al bloquearse a sí misma; repetidos en la confianza tras una fusión; la
reutilización de nombres de organización (punto 5); una copia huérfana si la petición desaparece
mientras se trae (se borra); la sección de la consola a quien no podía usarla (ahora solo
owner/admin); y la respuesta de Sytue ID leída sin tope. Y los catálogos de Fyne, regenerados.

<a id="0060"></a>

## 0060 — Runners propios en el frontal, imágenes en ghcr.io y forks en GitHub

`accepted` · 2026-09-27 · CI, despliegue · sustituye los puntos 1 y 5 de [0058](#0058) · código en `deploy/rooms-sfu/{instalar-runners-github.sh,runners/sytue-github-runner.service,drenar-y-reiniciar.sh,podar-ci.sh}`, `.github/workflows/`, `.github/actions/toolchain/forks.sh`, `apps/fyne/go.mod`, `deploy/helm/bin/{actualizar-suite,configurar-git-root,common}.sh`

**Contexto.** 0058 portó el CI a GitHub con dos cosas pendientes de decidir y una
contradicción: los jobs iban a runners ALOJADOS de GitHub (`ubuntu-24.04-arm`), que en
un repositorio privado cobran por minuto y contradicen la decisión del dueño del
2026-09-23 («el CI va entero en el frontal»), y dejaba en git.gay el registro de
imágenes y los forks de Fyne. El dueño decidió el 27/09 que git.gay deja de usarse para
todo y que los forks pasan a `github.com/thinkddr`, privados.

**Decisión.**

1. **Runners:** `actions/runner` de GitHub (ARM64) en el frontal, `sytue-global`, `sytue-second`, `sytue-third` y, desde el 2026-09-28, `sytue-fourth` (el dueño aprobó el cuarto: tres se quedaban cortos para una ronda completa),
   con la etiqueta `sytue-arm64`, el usuario `sytue-runner` y el Docker del CI sin root
   de siempre. Cada job en un contenedor de ese Docker (`container: &ci` en el
   workflow, con el proxy filtrado como socket y la caché de Go del host montados), que
   es lo que hacía forgejo-runner con su `config.yml`. Registrados una vez con un token
   de una hora; en el host no queda ningún token de GitHub.
2. **Registro:** `ghcr.io/<dueño>`, publicado con `GITHUB_TOKEN` (`packages: write`).
   Paquetes privados, ligados al repositorio.
3. **Forks:** `thinkddr/{fyne,typesetting,glfw}`, privados, con todas sus ramas y tags;
   los lee `SYTUE_GIT_FORKS_TOKEN` (fine-grained, solo lectura de los tres). El de
   typesetting declaraba su ruta de git.gay en `module`, así que lleva un tag nuevo
   (`v0.3.4-sytue.2`) que solo cambia esa línea.
4. **Perfil:** cada release publica `completo` (el piloto enciende Cloud), y el
   actualizador acepta una marca `completo` también en un servidor sin Cloud, porque
   lleva todas las imágenes de `piloto`.
5. **Piloto:** mismo mecanismo (marca `desplegable/…` en el remoto, imágenes del
   registro), con el remoto por SSH a GitHub y la misma clave de despliegue, dada de
   alta como Deploy key de solo lectura.

**Alternativas descartadas.** _Runners alojados_: de pago en privado, y el dueño ya
había decidido lo contrario. _El runner de GitHub dentro de un contenedor por job
(efímero)_: más fiel, pero pide un token de administración del repositorio guardado en
el host para registrarse en cada job; el contenedor por job del propio runner da el
mismo aislamiento sin esa credencial. _Imágenes públicas en ghcr.io_: ahorran la
credencial del piloto, pero publican los binarios de un producto privado; es decisión
del dueño.

**Consecuencias.** El runner de GitHub monta siempre `/var/run/docker.sock` en el
contenedor del job y no tiene opción para evitarlo; en el Docker sin root esa ruta es
el socket de producción, inaccesible desde el espacio de usuario del CI (medido), y el
proxy va por `DOCKER_HOST`. Al parar, el runner de GitHub cancela su job (no espera),
así que `drenar-y-reiniciar.sh` espera a que no haya un `Runner.Worker`. La poda del
frontal reconoce los jobs de GitHub por su red `github_network_*`. Un flujo solo con
`workflow_dispatch` no se puede despachar si no está en la rama por defecto, que por
eso tiene que ser `main` (lo es desde el 2026-09-28). Los runners de Forgejo se retiraron
del frontal el mismo 27/09 (`retirar-runners-forgejo.sh`; `instalar-runners.sh` los
devuelve). **git.gay se archiva, no se borra** (dueño, 2026-09-28): repositorio en solo
lectura y baja de sus registros de runners, pero solo cuando la primera v3.7.x esté en
producción desplegada desde GitHub. Los pasos, en
[operations §2.1e](operations.md#21e-archivar-git-gay).

<a id="0061"></a>

## 0061 — Sytue Cast: vídeo bajo demanda y directo, con portada pública y cuota de Drive

`accepted` · 2026-09-28 · producto, vídeo, directo, multi-tenancy, almacenamiento · decisiones del dueño del 27/09 · código en `services/cast/**` (mig. cast `0001`–`0003`), `platform/pkg/{livekit,personas}`, `platform/pkg/objectstore/rango.go`, `deploy/helm/charts/cast`, mig. id `0100`, `apps/frontend/apps/cast`, `apps/fyne/{cmd,apps}/cast`

**Contexto.** La suite no tenía vídeo. Las grabaciones de Rooms acababan en Drive como
ficheros sueltos, sin reproductor, y quien quería publicar un vídeo o emitir desde OBS tenía
que salir a un proveedor de fuera de la UE. El dueño pidió una app «como Vimeo»: subir,
publicar y ver, emitir en directo, con portada pública y canal por organización, y el listón
«100 % funcional y bonita».

**Decisión.**

1. **Un servicio, dos procesos.** `services/cast` (:8099) sirve la API, `/media` y el chat.
   El mismo binario, con el subcomando `transcoder`, toma trabajos de `transcode_jobs` y corre
   `ffmpeg` en una segunda imagen (debian-slim). La cola es Postgres: `FOR UPDATE SKIP LOCKED`
   en una transacción de mantenimiento (cruza organizaciones), un solo trabajo activo por vídeo
   y clase (índice parcial único), tres reintentos con espera de 30 s, 2 min y 8 min, y los
   trabajos `running` de más de tres horas vuelven a la cola, porque la réplica que los tenía
   murió. Terminar un trabajo y mover el vídeo pasan en la misma transacción.
2. **Derivadas:** HLS H.264 (fMP4, segmentos de 4 s) en 1080p (si el original lo da), 720p y
   360p; WebM VP9/Opus 720p para Fyne; póster. El original se conserva. _Ampliado el
   2026-09-28:_ las dos escaleras llevan las mismas calidades, las que elige quien sube
   (`qualities` en el alta y `PUT /videos/{id}/qualities`); el WebM va uno por calidad en
   `/media/…/webm/<q>p.webm`, y `webm/720p.webm` sigue valiendo; el vídeo expone
   `qualities`, `hls_qualities` y `webm_qualities`, play `webm_qualities`, y
   `GET /videos:estimate` da lo que ocupará cada una (migración cast `0007`). Los vídeos
   de antes reciben la escalera entera (dueño, 28/09: todos), poco a poco desde el barrido
   (`CAST_SWEEP_BACKFILL_QUALITIES`).
3. **Cuatro visibilidades:** `public` (portada y canal, indexable), `unlisted` (con enlace,
   incrustable), `org` y `people` (personas por @usuario, de cualquier organización, con el
   criterio de externos de [0040](#0040)). Toda la decisión de acceso vive en
   `internal/acceso`. La transacción corre siempre en el tenant **del vídeo**, no en el del token.
4. **`/media` con token propio:** HMAC-SHA256 sobre `vídeo|alcance|caducidad`, 6 h. El alcance
   (`ver` o `original`) evita que un enlace de reproducción descargue el original. Este punto se
   aparta del contrato inicial, que solo firmaba vídeo y caducidad.
5. **Directo:** LiveKit Ingress (RTMP para OBS en `cast-ingest.<dominio>:1935`) y Egress junto
   al SFU de Rooms. El paquete de LiveKit pasa de Rooms a `platform/pkg/livekit`. La room es
   `cast-<tenant>-<vídeo>`. La grabación vuelve como vídeo bajo demanda por la misma cola. El
   egress es **ParticipantEgress** y no TrackComposite: graba al emisor con su audio sin montar
   una plantilla web.
6. **Cuota compartida con Drive:** Cast pregunta a Drive por la puerta interna antes de aceptar
   una subida (507 con `available_bytes` si no cabe) y le informa del uso. Tope de 8 GiB por
   vídeo. Si Drive contesta que no hay sitio, o rechaza la consulta, no se sube. Si Drive
   no contesta (red, tiempo agotado o 5xx), la subida se acepta y queda en el registro: una
   caída de Drive no puede tumbar Cast.
7. **Lo anónimo** (portada, canal, ficha pública y comentarios públicos) está en
   `/api/v1/public/*`, que es lo que publica el ingress. El `/public/*` sin prefijo solo se sirve
   dentro del clúster, al BFF, con su propio limitador por IP.
8. **Personas por @usuario:** el cliente de `people:resolve` que llevaba Rooms pasa a
   `platform/pkg/personas`, compartido por los dos.

**Confirmado por el dueño el 28/09**, tal como está implementado: el aviso de «en directo»
solo llega a la lista de personas del vídeo; el título improvisado de un directo sin título va
en español; `usageInDrive` cuenta los bytes del original más las derivadas; los bytes de la
grabación de un directo cuentan igual que una subida; `poster:frame` sustituye el póster
propio; y el audio va muxado en cada variante HLS.

**Drive ofrece «Subir a Cast»** (dueño, 28/09) en sus ficheros de vídeo. Es una COPIA explícita
y no un «abrir en el sitio», como los de Fotos o CertiPDF: el vídeo necesita su
transcodificación y sus derivadas en Cast, y el fichero de Drive se queda donde está. La copia
cuenta en la misma cuota del plan. Va en la web y en la app nativa a la vez, como toda función
de Cast: la paridad 1:1 entre web y Fyne es obligatoria, y la única excepción es el incrustado.

**Decidido por el dueño el 28/09** (la web tal como se entregó):

1. El chat de un directo solo se lee con sesión; sin ella, la ficha muestra «Entra para
   participar». Es el mismo requisito que pone el servicio: el WebSocket exige token.
2. Lo que se sube nace con visibilidad `org`. Hacerlo público es un paso explícito de quien
   lo sube.
3. Cast figura como `disponible` en el catálogo desde que la web está completa, sin esperar a
   la app nativa (`nativa: false` hasta que llegue).

**Ampliado el 2026-10-03: «Presentar con Cast» desde Sytue Diapos** (encargo del roadmap de Office,
`diapos-cast`; por defecto lo más conservador, a la espera del dueño). Un directo por WebRTC puede
llevar una presentación:

1. **Quién emite, dónde.** Diapos programa el directo en Cast (`POST /lives` con `source=webrtc`,
   visibilidad `org`) desde su BFF (`/api/cast-presentar`, con el token de quien presenta) y abre la
   página de emitir de Cast con `?live=<id>`, donde se comparte la pantalla como en cualquier directo
   desde el navegador. La grabación vuelve como vídeo bajo demanda por la misma cola.
2. **Qué viaja: solo la posición** (número, total y título de la diapositiva, y la referencia
   `docs:<id>`). Ni el contenido ni las notas del ponente salen de Diapos: las notas siguen en la
   vista del moderador de quien presenta. El BFF reconstruye cada campo y el servicio rechaza lo que
   no conoce.
3. **API** (`services/cast/internal/diapositivas`, migración `0008`): `PUT /lives/{id}/slide`
   —solo el dueño del directo, programado o en emisión— y `GET /videos/{id}/slides` (y
   `/public/…`) para quien pueda ver el vídeo por el mismo `acceso` que la reproducción. Cada cambio
   es un capítulo (`slide_marks`, hasta 2000, con los ms desde que empezó el directo); la
   presentación (`live_presentations`) guarda quién la lanzó y cuándo, y la primera diapositiva
   deja una línea en el registro. Cast no tiene registro de auditoría de cumplimiento (el de
   `platform/pkg/auditlog` es de Mail y Drive): esa fila y esa línea son la huella.
4. **Quien mira** ve «Diapositiva n de N · título» bajo el directo (sondeo de 3 s, sin sesión si es
   público) y, en la grabación, la lista de capítulos para saltar a cada diapositiva.

Solo web: la paridad con la app nativa (Fyne) queda pendiente.

**Consecuencias.** Hay una base de datos, un bucket y dos Deployments más. El transcodificador
es el único pod de la suite con `ffmpeg`, así que es el que hay que vigilar en cada CVE de
ffmpeg. La cuota de Drive pasa a ser de toda la suite, y el día que otro producto guarde
ficheros tendrá que entrar por la misma puerta. El número de este ADR estaba reservado como
0057 en el contrato de trabajo, pero se asignó a otra decisión el mismo día. Las referencias
del código apuntan aquí.

<a id="0062"></a>

## 0062 — Sytue Web usa un IR validado y publicación estática inmutable

`accepted` · 2026-09-30 · Sytue Web, seguridad, rendimiento

El editor, la IA y los exportadores intercambian el mismo IR versionado. La publicación valida
el IR, bloquea contenido de demostración y datos legales incompletos, y escribe una salida
estática versionada; una vista previa es otro artefacto con token y caducidad. Así no se ejecuta
contenido de cliente al publicar ni se mezcla un borrador con la versión viva.

<a id="0063"></a>

## 0063 — Los dominios propios se verifican antes de entrar al borde

`accepted` · 2026-09-30 · Sytue Web, DNS, seguridad

Cada dominio exige TXT de control y destino CNAME/A indicado por Sytue Web. Solo los dominios
verificados pueden pasar por `/_borde/permitido`; el ápex y `www` se mantienen como un par para
evitar que una variante no verificada capture tráfico.

<a id="0064"></a>

## 0064 — La analítica de Sytue Web es de primera parte y visitantes es una estimación diaria

`accepted` · 2026-09-30 · Analytics, privacidad

Sytue Web registra su hostname internamente en Analytics y sirve el colector bajo `/_sytue/`.
El panel recibe solo agregados y debe llamar «estimación diaria» a visitantes: la sal rota la
correlación entre días y no se prometen únicos de periodo.

<a id="0065"></a>

## 0065 — El código exportado consume el contrato público generado

`accepted` · 2026-09-30 · Sytue Web, exportación, interoperabilidad

React, Next, Astro y clientes nativos se generan desde la tabla de rutas públicas. Formularios y
checkout resuelven ALTCHA mediante ese cliente; no se duplican URLs, campos ni secretos en una
plantilla de destino.

<a id="0066"></a>

## 0066 — El CI construye con un BuildKit sin root y propio, no con el builder clásico por el proxy de Docker

`accepted` · 2026-09-28 · CI, seguridad, disco · decisión del dueño del 28/09 («pasar a BuildKit») · código en `deploy/rooms-sfu/docker-ci/{sytue-buildkit-ci,sytue-buildkit-ci-puente,sytue-docker-ci-proxy,sytue-docker-ci-proxy-unix}.service`, `deploy/rooms-sfu/docker-ci/buildkitd.toml`, `deploy/rooms-sfu/{instalar-docker-ci,podar-ci}.sh`, `scripts/{construir-imagen,publicar-imagen-go,publicar-imagen-frontend}.sh`, `.github/workflows/{imagenes-produccion,security,imagenes-rama}.yml`, `.github/actions/toolchain/accion.sh`

**Contexto.** Los jobs del frontal hablan con Docker por un proxy HTTP de lista cerrada
(tecnativa/HAProxy): ni contenedores, ni exec, ni volúmenes. BuildKit abre `/session` con
un _upgrade_ a h2c que HAProxy no puede relayar sin convertir el filtro en un túnel, así
que se construía con el builder clásico (`DOCKER_BUILDKIT=0`). El clásico no tiene
leases: su caché son imágenes colgantes, y la poda del host (`podar-ci.sh`, #96) podía
quitarle a un build en marcha una colgante de más de 2 h que estaba reutilizando. Era un
riesgo aceptado a cambio de no llenar el disco.

**Opciones, medidas en el frontal.**

1. **Abrir `/session` en el proxy.** El tecnativa v0.5 no tiene esa opción: habría que
   escribir la configuración de HAProxy a mano y relayar el h2c en crudo. Lo que pasa por
   ese túnel ya no lo ve el filtro: es la API entera de BuildKit dentro de Docker, con los
   permisos del Docker del CI (el de Docker concede `network.host` por defecto). Y la
   caché seguiría siendo la del daemon que comparte con los contenedores de los jobs. Se
   descarta: el filtro deja de filtrar justo en lo que más puede hacer.
2. **Un `buildkitd` por job, dentro del contenedor** (lo que hacían `imagenes-rama` y
   `release.yml` en los runners viejos). El contenedor del job no trae `buildkitd` y no
   puede montar sus capas sin quitarle a TODOS los jobs el perfil seccomp —cualquier rama
   podría montar—, y cada job empezaría con la caché vacía. Se descarta.
3. **Un `buildkitd` sin root, compartido, con su propio socket.** Es el elegido. `docker
buildx --driver remote` y `buildctl` hablan con él igual; se usa `buildctl`, que es un
   binario estático con su sha256 en `versions.env`, porque el `docker.io` de Debian que
   instala la imagen de los jobs no trae `buildx`.

**Decisión.**

1. **`sytue-buildkit-ci.service`:** `buildkitd` v0.33.0 bajo `rootlesskit`, como el usuario
   **`sytue-buildkit`** (propio, sin shell, con su rango subordinado 265536–331071), con
   `--net=slirp4netns --disable-host-loopback` y los datos en `/srv/ci/buildkit` (el volumen
   del CI, 0700). Snapshotter `overlayfs` en el espacio de usuario, `process-mode sandbox`.
   Sin permisos inseguros (`insecure-entitlements = []`: ni `security.insecure` ni
   `network.host`).
2. **El socket de los jobs:** el del demonio es de `sytue-buildkit` con 0660 y los jobs no
   lo abren. Lo ofrece `sytue-buildkit-ci-puente.service`, un `socat` que corre como
   `sytue-runner` con el grupo `sytue-buildkit` como único extra, en
   `/srv/ci/run/buildkitd.sock`; cada flujo que construye lo monta en
   `/run/sytue-ci/buildkitd.sock` y anuncia `BUILDKIT_HOST`. Nada con root toca a los jobs:
   el puente del proxy de Docker (`sytue-docker-ci-proxy-unix`), que corría como root, pasa
   también a `sytue-runner`.
3. **El proxy de Docker cierra `/build`** (`BUILD=0`): el builder clásico ya no está al
   alcance de ningún job. Es lo que hace que el riesgo aceptado de la poda desaparezca por
   construcción y no por disciplina.
4. **`scripts/construir-imagen.sh`** traduce la forma de `docker build` a `buildctl`
   (`--push` sube, `--tar` exporta en formato `docker save` para trivy). Medias Docker
   (`oci-mediatypes=false`) y sin atestaciones, para que `crane append/rebase/index` y el
   actualizador del piloto no noten el cambio. La imagen no pasa por ningún daemon: se van
   las etiquetas locales, `soltar_*` y el paso «Borrar las imágenes de este turno».
5. **La caché se recoge sola** (`buildkitd.toml`: 80 GB como mucho, un 40 % del volumen
   libre siempre, 10 GB que no se tocan) y la poda del host la baja con
   `buildctl prune --keep-duration <2 h con jobs, 30 min sin ellos o en emergencia>
--free-storage <objetivo>`, que no toca lo que un build tiene en uso. Ningún job poda.
6. **Multiarquitectura, igual que antes:** los servicios Go se cruzan con `go build` y se
   montan con `crane`; Analytics construye su Dockerfile en la nativa (`-base-arm64`) y
   monta encima los binarios amd64 (#86); una etapa de runtime con `RUN` (el `ffmpeg` del
   transcodificador de Cast, #103) se construye para su arquitectura con `--platform` y la
   base fijada al digest de esa variante, emulando solo sus `RUN` con el
   `qemu-user-static` del frontal; los frontends cruzan con `bun install --cpu x64` y
   `crane rebase`.

**Qué puede hacer un job, antes y después.**

|                                             | Builder clásico por el proxy                                                                          | BuildKit del CI                                                                          |
| ------------------------------------------- | ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| API de Docker                               | `/build`, `/images`, `/auth`                                                                          | `/images`, `/auth` (sin `/build`)                                                        |
| Un `RUN` que escapa de su contenedor cae en | el espacio de usuario de `sytue-runner`, dueño del Docker del CI y de las credenciales de los runners | el de `sytue-buildkit`, que no es dueño de nada más                                      |
| Red de un `RUN`                             | slirp4netns del Docker del CI                                                                         | slirp4netns propio, sin el `127.0.0.1` del frontal (medido: `wget 127.0.0.1:2377` falla) |
| Podar la caché de todos                     | `POST /build/prune`                                                                                   | `buildctl prune`, con leases; lo prohíbe el gate de `ci-sin-azar`                        |
| Procesos con root que le hablan             | el `socat` del proxy                                                                                  | ninguno                                                                                  |

**Medido en el frontal el 28/09** (flujo `imagenes-produccion` despachado en una rama
canario que no se fusiona, con tres a cinco imágenes y sin marcar nada como desplegable;
comprobado por dentro: `app/*`, `bun`, `ffmpeg` y `ffprobe` son del ELF de su
arquitectura, y `sharp` es el de x64):

| Imagen                                                   | Clásico, caché caliente (v3.8.2) | BuildKit, caché vacía | BuildKit, caché caliente |
| -------------------------------------------------------- | -------------------------------- | --------------------- | ------------------------ |
| `sytue-id` (crane, sin Dockerfile)                       | 22 s                             | 20 s                  | 17–20 s                  |
| `sytue-analytics` (base nativa + binarios amd64)         | 2 min 04 s                       | 1 min 28 s            | 1 min 13 s–1 min 18 s    |
| `sytue-frontend-portal` (amd64 cruzada + `crane rebase`) | 2 min 32 s                       | 1 min 17 s            | 1 min 03 s–1 min 15 s    |
| `sytue-frontend-portal-com` (arm64 nativa)               | 2 min 48 s                       | 53 s                  | 21–27 s                  |
| `sytue-cast-transcoder` (`RUN apt-get` emulado, #103)    | —                                | 5 min 32 s            | —                        |

El primer frontend de cada turno con el builder clásico tardaba ~9 min 30 s (v3.7.12, tres
turnos a la vez); con BuildKit las etapas `deps` y `prod-deps` corren en paralelo y la
caché es de todos. **Disco:** la publicación entera v3.8.2 (clásico) subió `/srv/ci` de 50
a 75 GB y dejó 101 imágenes en el daemon, 29 GB de ellas colgantes que solo la poda recoge;
con BuildKit el daemon no crece con los builds y la caché de BuildKit, tras cuatro
publicaciones del canario (cinco imágenes), ocupaba 9,8 GB, con un techo de 80 GB y su
propia recogida.

**La primera release con BuildKit, v3.8.5** (28/09): los tres turnos de
`imagenes-produccion` publicaron las 35 imágenes en 8 min 35 s (con el clásico, 43 min en
la v3.7.12 con los tres turnos a la vez, y 48 en la v3.8.2), `cerrar` comprobó la
arquitectura de todas y las marcó desplegables, y los tres `trivy image` de `security`
construyeron y escanearon con `--tar`. El daemon de Docker del CI no ganó ni una imagen; la
caché de BuildKit quedó en 19 GB. Ese mismo día, con los cuatro runners drenados
(`sytue-drenar-y-reiniciar --parar`), `instalar-docker-ci.sh proxy` dejó el proxy sin
`/build` (`POST /build` → 403) y su puente como `sytue-runner`.

**Consecuencias.** Una etiqueta cortada ANTES de este cambio no puede reconstruir sus
imágenes (su `imagenes-produccion` usa `docker build` y el proxy ya no abre `/build`): se
corta una versión nueva. Los jobs montan los sockets de los puentes por inodo: reiniciar un
puente les deja sin Docker o sin BuildKit, así que solo se hace sin jobs
(`instalar-docker-ci.sh proxy` se niega si hay alguno). Ninguno de los dos puentes depende
con `Requires=` de lo que tiene detrás (el del proxy de Docker lo hacía y pasa a `Wants=`),
así que el proxy y el demonio sí se pueden reiniciar con jobs en marcha. `imagenes-rama` y `release.yml` publican en el registro de Scaleway, que
está en baja: `imagenes-rama` ya va por este BuildKit, pero, como `release.yml`, construye la
arquitectura del runner (arm64) y los nodos de Scaleway son amd64; queda como pregunta al
dueño si se portan a los publicadores multiarquitectura o se retiran con Scaleway.
Lo vigilan `deploy/production/tests/buildkit-ci-test.sh` (demonio sin root, sin loopback
ni permisos inseguros, puente sin root, proxy sin `/build`, flujos que montan el socket),
`.github/tests/ci-sin-azar-test.sh` (ni builder clásico ni poda del compartido en el CI),
`podar-ci-test.sh` y los tests de los publicadores, todos vistos en rojo saboteando lo que
vigilan.

<a id="0067"></a>

## 0067 — El catálogo de plantillas de Sytue Web crece por sector, se genera desde código y lo que hay que cambiar impide publicar

`accepted` · 2026-10-02 · Sytue Web, plantillas, galería · encargo del dueño («cuantas más plantillas mejor, para los negocios pequeños y medianos de dueños con pocos conocimientos informáticos») · código en `scripts/pages-plantillas/`, `services/pages/internal/plantillas/` y `services/pages/internal/domain/legal.go`

- **Una plantilla por sector**, no variantes cosméticas: se escriben pensando en lo que quiere ver el cliente de ese oficio
  (carta, tarifas, urgencias, pasos, horario, cómo llegar, cita o presupuesto) y con textos reales y llanos en es y en. 14
  categorías para la galería (`belleza`, `comida`, `comercio`, `oficios`, `motor`, `profesionales`, `salud`, `deporte`,
  `educacion`, `creativos`, `turismo`, `local`, `comunidad`, `web`). Cada plantilla lleva `category` y `keywords` (sinónimos
  de uso común por idioma) para el buscador.
- **Idiomas: es + en**, que es lo único que admite `IDIOMAS_VISITANTE`. Las otras 24 lenguas se añaden con la pieza de
  traducción, igual que a las diez de partida.
- **Los JSON se generan**: `lib.py` (piezas), `sectores/<clave>.py` (contenido), `looks.py` (paletas AA) y `generar.py`. El JSON
  generado es el que manda y lo valida `catalogo_test.go`; `vista-previa.mjs` pinta el IR según §4.2 para revisar el diseño.
- **Nada inventado que se publique por descuido**: sin imágenes remotas ni de stock (solo rellenos), sin opiniones, cifras,
  premios ni certificaciones inventados. Nombre, teléfono, correo, dirección, punto del mapa, «Tu ciudad», números de colegiado
  o de registro (`00-0000`) y toda nota «de ejemplo» llevan un marcador de `domain.TextosDemo`, y la publicación `live` se
  niega con `demo_content` mientras alguno siga en el sitio. El test exige que todo texto visible que dé instrucciones esté
  cubierto por un marcador, y prohíbe la jerga de marketing y las promesas sin datos.
- **Galería**: buscador que entiende «pelu» o «barbero» (sin tildes, plurales, es y en), grupos por categoría y vista previa en un
  iframe aislado. Sin plantilla preelegida.

<a id="0068"></a>

## 0068 — Sytue Bases: tablas relacionales colaborativas como producto propio, con ACL por base y conector de solo lectura

`accepted` · 2026-10-04 · Bases, permisos, interoperabilidad · decisión del dueño (2026-10-01: «lanzarlo tras los P0») · fila P1 «Tablas relacionales» de el roadmap de office · código en `services/bases/` (API Go, :8102) y, en las piezas siguientes, `apps/frontend/apps/bases/` (web, :4337)

- **Por qué un producto y no una hoja de Calcus.** Una tabla relacional necesita lo que una hoja no tiene: esquema con tipos que
  el servidor hace cumplir, relaciones con integridad referencial, permisos y auditoría propios y vistas que no son una
  rejilla. Ocultarlo dentro de Calcus mezclaría el modelo de celdas con el de registros. Calcus es CONSUMIDOR (pieza D).
- **Modelo.** base → tablas → campos tipados → registros. Tipos: texto, número, fecha, casilla, selección única y múltiple y
  enlace a registro de otra tabla de la MISMA base. El tipo de un campo no cambia tras crearse (los datos escritos dejarían de
  cumplirlo). Los valores van en `base_records.data` (jsonb por id de campo); los enlaces, en `base_links`, que es lo que
  permite que Postgres garantice la integridad.
- **Integridad referencial en la base de datos, no solo en el código.** Las claves foráneas son COMPUESTAS `(tenant, id)`:
  una comprobación de clave foránea la hace el propietario y se salta la RLS, así que sin el tenant en la clave un id ajeno
  colaría un enlace entre organizaciones. Las de los enlaces llevan además la tabla (`to_table_id` ↔ `link_table_id`), de modo
  que ni un enlace a un registro de otra tabla es posible. El borrado es seguro: un registro o una tabla a los que apunta un
  enlace no se borran (409); el propietario puede forzar el de un registro, que quita sus enlaces; borrar la base entera sí
  arrastra todo (claves diferidas, ver la migración).
- **ACL coherente con Docs**: propietario > editor > comentarista > lector, por base y opcionalmente por tabla (la entrada de
  tabla SUMA a la de la base, nunca resta; deny-by-default). El propietario es el `owner_sub` y no se concede por ACL. Los
  principales son un usuario o TODA la organización; no hay grupos (el token no los lleva) y no se comparte con otras
  organizaciones. Editor: registros y vistas. Propietario: esquema, importar, permisos, auditoría, borrar. La matriz está en
  `docs/security.md#bases-acl` y la prueba `TestMatrizACL`. La decisión se toma SIEMPRE en el servidor, dentro de la misma
  transacción que la operación (una petición, una transacción): no hay hueco entre comprobar y escribir. Sin acceso es 404
  (no se revela que existe); con acceso insuficiente, 403.
- **Vistas**: rejilla, Kanban (por un campo de selección) y formulario. Su configuración (filtros, orden, agrupación) se valida
  contra los campos reales al guardarla y NUNCA se concatena en SQL: los ids de campo y los valores viajan como parámetros, los
  operadores son una lista cerrada por tipo y la dirección del orden, una de dos constantes (`TestConstruirConsultaNoConcatenaEntrada`).
  `contains` usa `position()`, no `LIKE`: los comodines no significan nada.
- **Auditoría** de cada operación (quién, qué registro y campo, antes y después acotados a 1 KiB), también de importar y
  exportar. Sin claves foráneas: sobrevive al borrado de lo que audita. Solo la lee el propietario.
- **Importar/exportar CSV.** Importar crea UNA tabla nueva (esquema → propietario) con tipos inferidos por columna (número,
  fecha ISO, casilla o texto; `+34…`, `0x10` o `1_000` son texto). Límites: 5 MiB, 10.000 filas, 50 columnas, 10.000 caracteres
  por celda, UTF-8 sin NUL, todo o nada. Un valor que empieza por `=` entra como texto: la defensa de la «inyección de
  fórmulas» está al EXPORTAR (se antepone `'` a los textos que empiezan por `= + - @`, TAB o CR; los números no se tocan).
- **Límites** (cada uno protege una consulta o un cuerpo): 200 bases por organización, 50 tablas, 100 campos, 1.000.000 de
  registros por tabla (eran 50.000 hasta bases-2), 50 vistas, 200 permisos; páginas de 500 como mucho; toda consulta lleva
  LIMIT y ninguna OFFSET.
- **Paginación por cursor (bases-2, pieza 1).** Los registros se paginan por cursor (keyset): `GET /tables/{id}/records`
  devuelve `next_cursor` y lo acepta en `?cursor=`; `offset` ya no existe. El cursor es opaco (base64url de un JSON) y lleva
  el valor de cada clave del orden de la vista en el último registro entregado más su `created_at` y su id, que desempatan
  siempre. **No va firmado porque no concede nada**: la autorización es la de cada petición, y quien lo falsifica solo elige
  desde dónde sigue leyendo una tabla que ya puede leer; sus valores se validan en Go antes de llegar a Postgres (422, nunca
  un 500) y viajan como parámetros (`TestCursorHostil`). Es **estable con el orden de la vista**: lleva la firma de ese orden
  y, si la vista cambia de orden entre dos páginas, se rechaza y el cliente vuelve a empezar; borrar el registro en el que se
  quedó no repite ni se salta nada (`TestCursorEstableAnteCambios`), que es lo que un OFFSET no garantiza. Los textos ordenan
  por sus primeros 128 caracteres: así la clave cabe en un cursor que viaja en la URL (un texto admite 10.000); los que empatan
  ahí se desempatan por antigüedad. La auditoría ya iba por id descendente (`?before=`), que es un cursor, y las vistas de una
  tabla son 50 como mucho y se listan de una vez.
- **Índices y medidas** (200.000 registros, Postgres de los agentes, mejor de 5, `TestMedirPaginacion` con `BASES_MEDIR=1`):
  en el orden por defecto la página sale de `base_records_por_tabla (tenant, table_id, created_at, id)` sin ordenar nada —la
  primera, 1 ms de consulta; una profunda pasa de 92 ms (OFFSET) a 3,6 ms con toda la operación (ACL, campos y enlaces
  incluidos)—. En una vista ORDENADA por un campo, la página profunda baja de 253 ms a 95 ms y cuesta lo mismo que la primera
  (86-100 ms): es un recorrido de la tabla con top-N, porque la clave es `data ->> <campo>` con el campo como parámetro y un
  índice de expresión por campo exigiría DDL por cada campo que alguien ordene. Techo anotado: si las vistas ordenadas de
  tablas grandes pesan, índice de expresión creado al guardar la vista. El techo de registros se comprobaba contando la tabla
  en cada alta (64 ms con 200.000); ahora lo lleva `base_tables.record_count`, que mantiene un trigger por sentencia en todas
  las vías (rejilla, formulario público, importación, cascadas), y la comprobación cuesta 0,5 ms.
- **Exportación por tramos y enlaces con su valor (bases-2, pieza 2).** Exportar ya no carga la tabla en memoria: recorre
  la tabla por cursor en tramos de 500, cada uno en una transacción corta que vuelve a autorizar (no queda una transacción
  abierta mientras alguien descarga despacio, y quitarle el acceso corta la exportación en el tramo siguiente). Se audita al
  empezar, con el número de registros de la tabla. Si un tramo falla con medio fichero ya enviado, la conexión se corta para
  que la descarga falle en vez de cerrar bien un fichero incompleto. Un campo de enlace sale con el TEXTO de los registros
  enlazados (su campo principal: el primero con valor, el mismo que enseña la interfaz y el conector) además de su id: en
  CSV, dos columnas —`<campo>` con los textos y `<campo> (id)` con los ids—; en JSON, `link_values` junto a `links`, en el
  mismo orden. El texto de un registro enlazado lo escribió alguien en otra tabla y pasa por la misma neutralización de
  fórmulas que cualquier otro (`TestExportarEnlacesConValores`).
- **Campos de fórmula (bases-2, pieza 3).** Un campo de tipo `formula` guarda una expresión y el tipo de su resultado
  (`number` o `text`, que no cambia, como no cambia el de ningún campo). **Se evalúa en el servidor**, con el motor de
  fórmulas de la suite (`apps/nucleo/formula`, el de Calcus; no hay un segundo motor), y el resultado **se guarda en el
  registro** como cualquier otro valor: por eso filtros, orden, cursor, exportación y conector ven lo mismo sin saber que es
  una fórmula, y el navegador no calcula nada. Nadie lo escribe desde fuera (422). El lenguaje es el de una hoja de cálculo,
  recortado: las únicas referencias son a campos, entre llaves y POR ID —`{campo}` del mismo registro (un enlace vale el
  número de registros enlazados) y `{enlace.campo}`, los valores de ese campo en los registros enlazados, para `SUM`,
  `COUNT`, `MIN`, `MAX`…—; celdas, rangos, nombres y matrices se rechazan (`formula.Inspeccionar`, nuevo en el motor, dice
  qué usa una fórmula sin evaluarla), y solo valen las 37 funciones de una lista cerrada: puras, sin reloj ni azar (nada
  volátil), sin red, sin bucles ni matrices y ninguna que fabrique textos enormes. Cada valor entra en el motor como literal
  tipado: un texto que empieza por `=` es un texto. **Coste acotado por construcción**: 1.000 caracteres, 200 nodos, 20
  fórmulas por tabla y 100 valores por agregado. **Ciclos**: se rechazan al guardar, entre campos de una tabla y dando la
  vuelta por otras (grafo de dependencias de toda la base); si aun así apareciera uno, el motor lo deja vacío. Un campo que
  usa una fórmula no se borra (409). Un error (`#DIV/0!`, desbordamiento) o un resultado de otro tipo quedan vacíos.
  **Recálculo**: los campos y enlaces del propio registro, en la misma escritura; cuando cambia un registro ENLAZADO, se
  recalculan los que enlazan con él y los que dependan de esos, en la misma transacción mientras quepan en un presupuesto de
  500 registros; si no caben —o al crear o cambiar una fórmula en una tabla grande— la tabla se anota en `base_recalc` y un
  bucle del servicio la recorre por tramos de 500 (`SKIP LOCKED` entre réplicas); entretanto la página lo dice
  (`recalculating`). Un recálculo no sube `rev` ni se audita: no es una edición de nadie. Definir o cambiar una fórmula es
  esquema (propietario). Como la etiqueta de un enlace, un agregado enseña a quien lee una tabla un dato derivado de otra
  que quizá no puede leer: lo decide el propietario al definirla. Techos anotados: la interfaz crea fórmulas pero no las
  edita (la API sí, `PATCH /fields/{id}`), no hay resultado de tipo fecha y no se guarda el código del error.
- **Adjuntos (bases-2, pieza 4).** Un campo de tipo `attachment` guarda ficheros. Los bytes van al almacén de objetos de la
  suite (`platform/pkg/objectstore`, bucket `bases-blobs`, las mismas variables `S3_*` y el mismo cifrado en reposo del
  proveedor que Drive o Docs; no hay cifrado propio) y en Postgres queda la ficha (`base_attachments`: nombre, tipo, tamaño,
  SHA-256). **La clave del objeto no la elige nadie**: es `bases/<base>/<adjunto>` bajo el prefijo de la organización. **El
  tipo lo dice el contenido** (`http.DetectContentType` sobre los bytes; la cabecera `Content-Type` de quien sube ni se
  mira) y no entra lo que un navegador o un sistema ejecutaría: HTML, SVG, scripts, ejecutables (por contenido y por
  extensión) ni un fichero cuyo contenido no es lo que promete su nombre (`foto.png` que es un PDF). **Límites**: 20 MiB por
  fichero, 20 ficheros por registro y campo, 2 GiB por base (cuota), dos subidas a la vez por réplica (el fichero se lee
  entero en memoria). **La descarga la sirve el servicio**, no una URL firmada: el permiso de leer ese registro se comprueba
  en cada descarga, el adjunto ha de ser de ESE registro (conocer su id no basta), y sale siempre como descarga
  (`Content-Disposition: attachment`, `nosniff`, CSP `sandbox`). Subir y borrar es de editor; leer, de lector. Las imágenes
  PNG, JPEG y GIF de hasta 8 Mpx llevan miniatura (`platform/pkg/miniatura`, con su barrera anti-bomba). **Borrado seguro**:
  la ficha cae en cascada con su registro, campo, tabla o base, y un trigger anota la clave en `base_attachment_purge`, de
  donde un bucle del servicio borra el objeto y su miniatura; ninguna vía de borrado tiene que acordarse del almacén. Subir
  son tres pasos (reservar la clave en esa cola con una hora de margen, subir, confirmar con el registro bloqueado), así que
  una subida a medias no deja ni ficha sin objeto ni objeto sin dueño. En la exportación sale el nombre de los ficheros
  (neutralizado como cualquier texto); no se filtra ni se ordena por un campo de adjuntos, y un formulario público no los
  admite. Sin almacén configurado el servicio arranca y los adjuntos responden 503. Techos anotados: sin antivirus, sin
  deduplicar por huella, y tras una fusión de organizaciones el objeto se queda bajo el prefijo con el que se subió
  (`store_tenant`).
- **Rejilla por ventanas.** La web guarda como mucho tres páginas de 100 (`lib/ventanas.ts`) y recuerda el cursor con el que
  pidió cada una: «Cargar más» añade la siguiente y suelta la primera; «Cargar anteriores» vuelve a pedir la anterior con su
  cursor. Con botones y no con desplazamiento infinito: valen igual con teclado y lector, y el rango cargado se anuncia.
- **Tablero y formulario (pieza C).** El tablero Kanban agrupa por un campo de selección; mover una tarjeta es escribir ese
  campo, con el selector «Mover a» de la tarjeta (vale igual con ratón, teclado, lector y móvil; sin arrastrar y soltar). El formulario es una vista
  con los campos que se piden y, para recoger datos de clientes, **enlaces públicos revocables**: token de 32 bytes del que
  solo se guarda el hash, cupo de envíos por enlace, caducidad opcional y prueba de trabajo ALTCHA (`platform/pkg/altcha`,
  sin cookies ni terceros) atada al enlace. Solo el propietario publica y revoca. Un formulario público nunca rellena enlaces
  entre tablas ni campos que la vista no incluya (se rechazan, no se ignoran), y el registro queda auditado con el actor
  `form:<enlace>`.
- **Conector de solo lectura hacia Calcus (pieza D).** El propietario concede una CONEXIÓN a una tabla o a una de sus
  vistas: un token (`sbc_…`, del que solo se guarda el hash) que siempre caduca (90 días por defecto, 365 como mucho), se
  puede rotar (sube `version`) y revocar. Calcus lo guarda cifrado como credencial de un origen `bases` de sus datos
  externos y lo manda a una raíz FIJA (config de servicio `BASES_URL`, nunca del usuario). Sytue Bases lo comprueba contra
  la base en CADA lectura —no hay caché—, así que revocar corta de inmediato. No existe ruta de escritura para un token de
  conector. Cada lectura devuelve la PROCEDENCIA (base, tabla, vista, quién concedió, cuándo, versión), que Calcus enseña y
  guarda con la tabla, y queda auditada (`connector.read`, actor `conn:<id>`). Drive, ERP y Pages como consumidores quedan
  fuera: el mismo token y la misma ruta les valdrían, pero cada uno necesita su pantalla de destino.
- **Accesibilidad auditada en el CI (bases-2, pieza A).** `test/e2e/accesibilidad-bases.mjs` pasa axe-core (WCAG 2.2 AA),
  teclado, lector, zoom al 200 % y objetivos táctiles a 320, 390 y 1280 px, en claro y oscuro, sobre los bancos de
  componentes (`/bases/dev/rejilla`, `/bases/dev/vistas`) y sobre las PANTALLAS ENTERAS (`/bases/dev/app`: las islas reales con
  el marco y la API contestada por la prueba): lista de bases, base abierta, tablero y compartir. Corre en el job
  `accesibilidad-office` de `ci-frontend.yml`. Una pantalla nueva se suma con una línea en `PANTALLAS`. El listón táctil es
  44 × 44 px con puntero grueso (regla en `styles/app.css`); una celda de la rejilla es contenido y queda en 24. La cabecera
  y la barra lateral son del marco común y no se miden aquí.
- **Conectores hacia Drive, SyERP y Sytue Web (bases-2, pieza B).** Una conexión es de UN consumidor (`consumer`:
  `calcus`, `drive`, `erp`, `web`; migración `0020`), y los tres nuevos exigen acotarla a una VISTA: lo que sale de Bases
  hacia un fichero, un informe o un sitio público es lo que el propietario dejó en una vista, nunca la tabla entera. El
  consumidor se identifica al leer (`X-Sytue-Consumer`) y dice para qué organización lee (`X-Sytue-Tenant`): si no casan con
  la conexión, la lectura es el mismo 401 que un token revocado y queda auditada como `connector.denied`. No es autenticación
  del servicio —la ruta no sale del clúster y la credencial sigue siendo el token—: es lo que impide que el token concedido a
  un informe publique un sitio, o que se use desde otra organización (Bases no se comparte entre organizaciones). Calcus
  conserva lo que tenía (tabla entera, sin cabecera de organización) porque sus conexiones son anteriores. El cliente es UNO,
  `platform/pkg/basesconn` (petición a raíz fija, forma del token antes de mandar nada, topes, procedencia que ha de nombrar al
  consumidor, y CSV con la defensa de fórmulas): los consumidores lo importan. Pruebas:
  `TestConectorPorConsumidorVistaYOrganizacion` (un token no lee otra vista ni otra base, ni vale en otro consumidor u
  organización; revocar corta en el acto) y `TestConectorPorHTTPExigeConsumidorYOrganizacion`.
- **SyERP consume una vista como fuente de informes.** Administración guarda el token con `POST /conexiones/bases`
  (sellado con `keycrypto`, como los demás secretos de `conexiones`); el servidor LEE con él antes de guardarlo —así sabe que
  es para SyERP y para esta organización— y anota las columnas y la procedencia. La fuente `bases` solo existe para
  owner/admin: el token es de la organización, no de la persona, así que Bases no puede decidir por cada lector y lo decide
  SyERP con el mismo criterio que para guardarla. Cada informe lee de nuevo (sin caché: revocar en Bases da «no disponible»
  en la siguiente lectura) y la tabla lleva la procedencia de ESA lectura, que la interfaz enseña bajo la tabla. Techo: una
  vista por organización (la clave de `conexiones` es el proveedor) y sin pantalla para pegar el token (hoy, por API).
  `TestFuenteBasesDeLosInformes`.
- **Sytue Drive exporta una vista a una carpeta.** `POST /folders/{id}/imports/bases {token}` («Nuevo → Vista de Sytue
  Bases»): Drive TIRA de Bases con el token de una conexión concedida a Sytue Drive, escribe un CSV (con la defensa de
  fórmulas de `basesconn.CSV`; nombre saneado a partir de tabla y vista) por el camino de cualquier subida —cuota,
  deduplicación, versiones— y anota la PROCEDENCIA por versión en `drive_file_provenance` (migración `0029`, RLS forzada,
  clave ajena compuesta `(tenant, file_id)`), que se lee con `GET /files/{id}/provenance`. Tirar y no empujar: así la
  procedencia la dice Bases en su respuesta y no un campo que el llamante pueda inventar, y Bases no gana una dependencia de
  salida. El token se usa una vez y NO se guarda: refrescar es volver a exportar (versión nueva del mismo fichero), y revocar
  la conexión impide hacerlo sin tocar lo ya exportado. Techo: solo «Mi unidad» (las unidades de equipo y las carpetas
  compartidas tienen su propio camino de subida) y la procedencia aún no se enseña en el panel de detalles del fichero.
  `TestExportarVistaDeBases`.
- **Sytue Web lista los registros de una vista en una sección del sitio, como INSTANTÁNEA.** «Insertar → Tabla desde
  Sytue Bases» pide el token de una conexión concedida a Sytue Web y `POST /sites/{id}/bases/table {token}` devuelve la vista
  como el cuerpo de un nodo `table` corriente, que entra en el borrador y se pinta y se valida como cualquier otra tabla (cada
  celda pasa por el escapado del renderizador). No hay nodo nuevo en el IR, ni consulta al servir el sitio, ni token guardado:
  lo publicado es exactamente lo que quien edita vio. Solo owner/admin (los mismos que publican): lo que entra acaba en una
  página pública. Una vista que no cabe (50 filas × 8 columnas) se rechaza entera, no se recorta. La procedencia queda en el
  nombre del nodo (solo se ve en el editor) y en la auditoría de la base. **Revocar la conexión no retira lo ya publicado**
  —es contenido del sitio—: impide volver a traerlo. Techo: los datos se refrescan volviendo a insertar la tabla; una sección
  enlazada que se actualice al publicar pide un tipo nuevo en el IR (TS y su espejo en Go), el token sellado por sitio y
  decidir qué hace una publicación cuando la conexión ya no vale. `TestTablaDesdeBasesExigeAdminYTokenDeConexion`,
  `TestATablaDeSitio`.
- **Solo web** (`nativa: false` en la paridad): decisión conservadora hasta que el dueño diga lo contrario; es una pantalla
  de rejilla que en Fyne costaría una pieza propia. **Superado el 2026-10-05**: la pieza se hizo
  (`apps/fyne/apps/bases`), la app tiene alta (`apps/fyne/cmd/bases`, `nativa: true`, esquema `sytue-bases`, migración
  0109 de Sytue ID para la vuelta del login en el móvil) y llega a la API por el Ingress del último punto de este ADR.
  Lo que la web tiene y la nativa no (importar, adjuntos, comentarios, conexiones, scripts, tiempo real) sigue siendo
  deuda, y en el teléfono solo está vista a 360 de ancho contra el servicio falso.
- **Fuera de alcance de la primera entrega** (y por qué): grupos de ACL, transferir la propiedad, comentarios por
  registro (el rol «comentarista» lee igual que el lector hasta que existan). La paginación por cursor llegó en bases-2.
- **Encendida por defecto (2026-10-04, decisión del dueño).** Se entregó apagada y en `desarrollo`; desde esa fecha el
  catálogo la da por `disponible`, el chart la despliega (`bases.enabled: true`), el Hub la anuncia y el piloto la levanta
  salvo `BASES_ENABLED=false`. Apagada sigue sin dejar ni un objeto. Sus dos secretos los rellena `generate-secrets.sh`,
  que `deploy.sh` ejecuta antes de tocar nada. **Producción y staging (Scaleway) la mantienen apagada** en sus overlays:
  sus guiones no crean aún los secretos de Bases ni existen su DNS y su certificado, y encenderla a ciegas dejaría el pod
  sin arrancar; es una decisión pendiente del dueño, con los pasos en
  [operations.md](operations.md#bases-encendido).
- **Revisión adversarial (pieza F):** seis hallazgos corregidos y dieciocho mutaciones vistas en rojo, en
  [security.md](security.md#bases-adversarial).
- **Comentarios por registro (bases-2, colaboración A · 2026-10-04).** Cada registro tiene un hilo de comentarios de UN
  nivel (raíz y respuestas), en `base_comments` con las mismas reglas que el resto: RLS forzada, claves compuestas
  `(tenant, id)` y, en la respuesta, también el registro —una respuesta no puede colgar de un hilo de otro registro ni de
  otra organización aunque el código se equivocara—. Con ellos el **comentarista se distingue por fin del lector**: lee y
  comenta (`AccComentar`), no edita. **Borrar** sigue la regla de Docs (PR #353): quien lo escribió, mientras aún pueda
  comentar, o quien modera (editor o propietario); una raíz borrada que tiene respuestas deja una lápida sin texto ni autor,
  para no llevarse por delante lo que escribieron otros, y desaparece con su última respuesta. Texto llano de 1 a 2.000
  caracteres, 500 comentarios por registro. **Menciones** con la forma de la suite (`@ana@empresa.eu`,
  `hubnotify.Mencionados`). **Avisos por el Hub** (`bases.comment`, `bases.mention`) a la propietaria de la base y, en una
  respuesta, a quienes escriben en ese hilo, nunca a quien comenta; y **solo a quien en ese instante puede leer el
  registro**: los del hilo se filtran en la misma transacción que guarda el comentario, y una mención se resuelve en Sytue
  ID con la sesión de quien escribe (solo contesta por gente de su organización) y se comprueba contra la ACL antes de
  avisar —el aviso lleva un trozo del texto, y mandárselo a quien no ve el registro sería enseñárselo—. **Auditoría**:
  `comment.create` y `comment.delete` con el registro y los ids, SIN el texto (un comentario borrado no sobrevive en la
  traza). El servicio pasa a necesitar `HUB_BASE_URL`, `HUB_INTERNAL_SECRET` (obligatorio fuera de desarrollo, del Secret
  `sytue-hub-internal`), `ID_BASE_URL` y `PUBLIC_URL`. En la web, un panel lateral (el `Modal` del sistema en `right`) que se
  abre desde la fila o la tarjeta y desde el enlace del aviso (`?table=&record=`).
- **Grupos como principal de la ACL (bases-2, colaboración B · 2026-10-05).** Una entrada puede ser de un **grupo de
  Sytue ID** (`principal_kind = 'group'`), como en Docs y en Drive. Se investigó de dónde sale la pertenencia: **el token ya
  la lleva** —Sytue ID emite en el access token el claim `groups` con los nombres de los grupos de la persona en la
  organización activa, que es lo que Drive usa para compartir por grupo—, así que **no hace falta una consulta servidor a
  servidor ni tocar Sytue ID**: `authn.Claims.Groups` lo lee (antes Drive releía el payload por su cuenta) y Bases lo pasa a
  `domain.ResolverRol`. La «caché de caducidad corta» es la vida del propio token: entrar o salir de un grupo surte efecto
  cuando se renueva, en minutos, y el canal en vivo (pieza C) se cierra al caducar el suyo. Reglas: el grupo se guarda por su
  NOMBRE, que es lo que trae el token (limitación conocida y anotada para el dueño: renombrar un grupo en Sytue ID deja
  huérfana la entrada, y un grupo nuevo con el nombre viejo la heredaría; pasar a identificadores exige que el token los
  emita); solo aplica dentro de su organización (la fila lleva `tenant` y la RLS); las clases no se confunden (un grupo
  llamado como un `sub` no es esa persona); una clase desconocida no aplica; y como mucho 200 grupos del token entran en la
  consulta. El aviso de un comentario a alguien que solo accede por un grupo no sale (el servicio no conoce los grupos de un
  tercero): deny-by-default también para avisar.
- **Conceder a una persona por su dirección.** `POST …/permissions` con `email` resuelve la dirección en el directorio de
  Sytue ID con la sesión de quien concede (solo contesta por gente de SU organización) y guarda cómo se llama
  (`principal_label`) para la lista. Quien no gestiona los permisos no llega a consultar el directorio.
- **Transferir la propiedad.** El propietario puede dársela a **otra persona de su organización**, identificada SOLO por su
  dirección —el servicio nunca acepta un `sub` del cliente: que el destino es de la organización lo dice Sytue ID—. Quien la
  tenía se queda como **editor** (decisión conservadora: no pierde su trabajo de golpe, pero deja de decidir esquema,
  permisos, conectores y borrado; el nuevo propietario puede quitarlo). La fila de la base se bloquea, así que dos
  transferencias a la vez no se pisan. Queda en la auditoría (`base.transfer`, antes y después) y el nuevo propietario
  recibe un aviso por el Hub (`bases.transferred`). En la web pide confirmación con el nombre de la base y la dirección
  del destino.
- **Compartir con otras organizaciones** (sobre el [0040](#0040) y el [0052](#0052)). **La base vive en la organización
  anfitriona**, con sus filas, su RLS, su auditoría y sus límites; la persona de fuera es una entrada más de la ACL, de clase
  **`external`**, con su `sub`. El propietario la invita por su **@usuario o su dirección**, que el servicio resuelve con
  `people:resolve` (da el `sub` y cómo se llama; nunca su correo ni su organización); un `sub` pegado por el cliente no se
  acepta, y si la persona resulta ser de la propia organización no es «de fuera» (`is_member`). **Rol acotado**: lector,
  comentarista o editor, en la base o en una tabla; nunca propietaria.
  - **Su token trae SU organización**, así que en qué organización se atiende la petición lo decide el servicio y **nunca
    el cliente** (ni cabecera, ni consulta, ni cuerpo; el 0040 descartó la cabecera): `enAnfitrion` mira el recurso de la
    ruta (base, tabla, registro, vista o comentario) y, solo si NO existe en la organización del token y sí en una donde la
    persona es invitada, la petición sigue en la anfitriona con `Actor.Externo`.
  - **Allí no es nada más que su `sub`**: `ResolverRol` solo le cuenta las entradas `external` a su nombre. Ni «toda la
    organización», ni un grupo de la anfitriona que se llame como uno suyo, ni una entrada de persona con su mismo `sub`, ni
    la propiedad aunque el `sub` coincida. Resolver la anfitriona no concede nada: la ACL de la base se decide después, en la
    transacción de la operación, así que **no ve nada más de esa organización** (otra base abierta «a toda la organización»
    le da 404).
  - **La única lectura entre organizaciones** es «¿en qué organizaciones es invitada esta persona?», por una política de
    solo lectura (`base_permissions_externo`) que solo enseña las entradas `external` de la persona declarada en la
    transacción. Se recuerda 30 s por réplica; eso no retrasa una **revocación** —la ACL se comprueba en cada operación y
    quitar la entrada corta en el acto—, solo que una invitación recién hecha tarde eso en poder abrirse.
  - **En su lista** la base sale detrás de las suyas, con la anfitriona (`host_org`). **Auditado** en la anfitriona: la
    invitación, la revocación y todo lo que el invitado hace, con su `sub`.
  - **Lo que no se hizo, y por qué.** Avisos del Hub a la persona de fuera (su campana está en su organización, como en el 0040) y los nombres de la gente de la anfitriona en los comentarios cuando lee alguien de fuera (el directorio contesta
    por organización: salen como «Alguien»). Una política de organización para cerrar el compartir con fuera queda para el
    dueño.
- **Coedición en tiempo real (bases-2, colaboración C · 2026-10-05).** Se ven al instante los cambios de otras personas
  en la rejilla y en el tablero, y quién está en cada celda. **El patrón es el de Docs** (PR #284-#305 y #369): contrato
  versionado, cambios en orden con replay al reconectar, operaciones por celda con identificador e idempotencia, presencia
  y autorización del servicio. **El transporte no**, y por eso no se ha extraído plomería de `services/docs/internal/collab`
  a `platform/pkg`: lo que allí hay es un WebSocket con salas en memoria repartidas por DragonflyDB y tipado a operaciones
  CRDT, y **la API de Bases no tenía Ingress** (así nació; desde el 2026-10-05 sale solo `/api/v1`, ver el último punto
  de este ADR), así que un navegador no podía abrirle un WebSocket. Lo común que sí existía se importa:
  `httpx.AbrirSSE` y el `Idempotency-Key` que el proxy del BFF ya deja pasar.
  - **Flujo de cambios por SSE a través del BFF** (`GET /bases/{id}/events`, contrato `v=1`; otra versión, 400). El registro
    de cambios es **la propia auditoría**, que toda escritura ya rellena en su transacción: no hay un segundo camino que
    alguien pueda olvidar. Para seguirla **sin huecos** cada fila lleva la transacción que la escribió (`xact`) y solo se
    entregan las de transacciones anteriores a la más antigua aún en curso, en orden `(xact, id)`: una transacción lenta que
    confirma tarde no queda por detrás del cursor (`TestFlujoDeCambiosSinHuecosConTransaccionesLentas`). Funciona tras
    PgBouncer —LISTEN/NOTIFY no— y la base de datos es el único relé entre réplicas. Latencia: un tic (1 s). **Límite
    conocido**: ese horizonte es de TODA la instancia de Postgres, así que una transacción de escritura que se quede abierta
    mucho tiempo en cualquier base de la instancia (una migración que reescribe una tabla, una carga masiva) RETRASA la
    entrega mientras dure; no pierde ni desordena nada, y editar sigue funcionando por la API. Si llegara a molestar, el
    camino es una secuencia por base asignada bajo bloqueo, a cambio de serializar las escrituras de cada base.
  - **Por el flujo no viajan datos**: solo qué ha cambiado (acción, tabla, registro, campo, quién y qué operación). El
    cliente vuelve a pedir ese registro por la API (`GET /records/{id}`), que autoriza la lectura. Los cambios de permisos
    salen como `access`, sin quién ni a quién.
  - **Autorización en cada tanda**: cada tic resuelve la ACL de quien escucha dentro de la transacción que lee, filtra cada
    cambio por la tabla que puede leer y, si ya no tiene acceso, se despide (`bye access`) y cierra: **un permiso revocado
    corta el flujo abierto** en el tic siguiente. Tampoco sobrevive a su token (`bye token`; el navegador reconecta con uno
    nuevo, que dice lo que vale ahora).
  - **Reconexión con replay**: cada evento lleva su cursor como `id`; `EventSource` reconecta solo y manda `Last-Event-ID`.
    Quien se queda más de 500 cambios atrás recibe `resync` y recarga.
  - **Operaciones por celda** (`PUT /records/{rid}/cells/{fid}` con `Idempotency-Key`): no piden la versión del registro,
    así que dos personas en celdas distintas del mismo registro no se estorban; en la misma celda **gana el último
    escritor y se le avisa** si pisa un valor que no había visto (`expected` → `conflict`). El identificador queda en la
    traza con índice único: repetir la operación tras un corte no la aplica dos veces (`replayed`), ni a la vez desde ocho
    sitios. La edición por versión (`PATCH /records/{id}` con `rev`) sigue existiendo para quien edite un registro entero.
  - **Presencia** (`PUT /bases/{id}/presence`): la pestaña dice en qué celda está; **quién es lo pone el servicio** (del
    token), la celda ha de ser de una tabla que puede leer, y nadie usa la pestaña de otra persona. Se guarda en la base de
    datos con caducidad de 30 s (así la ven todas las réplicas) y cada cual ve la celda de los demás solo en las tablas que
    lee; en las que no, sabe que la persona está en la base y nada más.
  - **Límites**: 8 flujos por persona y 200 por réplica (cada flujo es una transacción corta por tic), 100 pestañas
    presentes por base, 500 cambios por tanda. Techo anotado en el código: una sala por base con un solo sondeo si una base
    junta muchas pestañas.
- **Scripts de usuario (bases-2, automatización · 2026-10-05; PR #432, #437 y #439).** Sobre el runtime aislado común del
  [0069](#0069): el servidor NO ejecuta código; el script corre en el navegador de quien lo lanza y cada cosa que hace es
  una petición corriente a esta API con SU sesión. **No hay credencial de script ni camino propio**, así que un script
  nunca puede más que quien lo ejecuta, sin que haya que demostrarlo operación por operación.
  - **Cuelga de una base.** `base_scripts`, `base_script_versions` y `base_script_runs` (migración `0030`), con las reglas
    de siempre (RLS forzada, claves compuestas y diferidas). El registro de ejecuciones no lleva claves: sobrevive al
    borrado del script y de la base. No se escribe en `base_audit` —es el registro de cambios del flujo en vivo y un
    script no es un cambio de datos—; lo que el script escribe sí queda ahí, a nombre de quien lo ejecutó.
  - **Versiones inmutables y aprobación.** Una versión es `(código, SHA-256, manifiesto)`; editar crea otra, en borrador,
    que no hereda la aprobación (un trigger lo impide también por SQL). Un script es `private` (solo su autora; no existe
    para nadie más, tampoco para la propietaria de la base) o `shared` (lo ejecutan quienes acceden a la base, si la
    versión está aprobada). **Aprueba la propietaria de la base**, con el hash del código que revisó, y puede aprobar el
    suyo (la decisión del dueño para Docs, tal cual). No un administrador de la organización: en Bases no tiene acceso
    implícito a las bases, y dárselo para revisar abriría una lectura nueva; la propietaria ya ve todo en la suya.
  - **Superficie `sytue.v1`, corta y cerrada**: `tables.list`, `fields.list`, `views.list`, `records.list`,
    `records.get`, `records.create`, `records.update` y `records.delete` (nunca forzado). Fuera, a propósito: esquema,
    permisos, compartir, conectores, formularios, adjuntos, comentarios, exportar, importar y la auditoría. La lista vive
    en `domain.OperacionesDeScript` y en su espejo de la web, atados por una prueba.
  - **Manifiesto** (`v: 1`): operaciones, tablas de la base (vacío = las que la persona lea), `write`, `max_ops` (hasta
    500; aquí cada operación es una transacción del servidor) y `max_ms` (hasta 30 s). Un manifiesto que declara una
    operación que escribe sin `write` se rechaza entero. Sin ejecución desatendida: Bases no tiene disparadores.
  - **Confinado a su base.** El servidor no distingue una petición de un script de una de la persona, así que el
    confinamiento lo pone el anfitrión de la web: toda tabla ha de ser de la base del script y todo registro se pide antes
    y ha de ser de una tabla permitida. Sin eso, un script aprobado podría copiar a su base otra a la que quien lo ejecuta
    tiene acceso y su autora no. El razonamiento, en [security.md](security.md#bases-scripts).
  - **Cuotas fijas y registro**: las mismas de Docs (60 ejecuciones por persona y hora, 600 por organización, 10 minutos
    reservados), bajo cerrojo. Cada ejecución deja quién, qué versión con su hash, resultado, duración, operaciones y un
    código corto; los intentos denegados también, sin gastar cuota. Lo lee la propietaria.
  - **Quién**: cualquiera con un rol en la base crea scripts y ejecuta los suyos o los aprobados, también un lector (el
    servidor le niega los que escriben). Fuera quedan quien viene de otra organización y quien solo accede a alguna tabla.
  - **En la web**: botón «Scripts» de la base y un panel con lista, código, manifiesto, versiones, proponer, aprobar o
    rechazar, ejecutar con la salida, la cuota y el registro; el consentimiento se pide antes de la primera ejecución y
    tras cualquier cambio del manifiesto. En los 26 idiomas. Entra en la auditoría de accesibilidad de Bases.
  - **Techos anotados**: el código se edita en un área de texto sin numeración (el editor con numeración es de Docs;
    llevarlo a `@sytue/ui` si hace falta en los dos); el manifiesto se escribe como JSON; un script no se renombra; la
    lectura es de 100 registros por página y de 256 KiB por respuesta; y no hay disparadores (al guardar un registro, por
    horario): pedirían ejecución desatendida y otra decisión de quién responde de lo que corre solo. Sin app en Fyne.
- **La API sale por un Ingress, limitado a `/api/v1` (2026-10-05, decisión del dueño; REVIERTE el «sin Ingress» de este
  ADR).** Hasta esta fecha la API no tenía Ingress y solo la alcanzaban pods de la suite. Se revierte porque **una app
  nativa no tiene por dónde entrar**: la de Fyne ya existe (`apps/fyne/apps/bases`) y habla con la API con el token de la
  persona, como las de Drive y el resto, pero el único camino era el BFF de la web, que solo acepta la cookie de sesión.
  Se descartó que el BFF aceptara también el token (dos formas de sesión en una pieza pensada para una, y un camino
  distinto al del resto de servicios con app nativa) y dejar Bases solo en la web. **Qué sale**: `bases.<dominio>/api/v1`
  y nada más; la ruta va escrita en la plantilla del chart (`deploy/helm/charts/bases/templates/ingress.yaml`), no en los
  values, porque todo `/api/v1` exige un token de Sytue ID y lo demás que sirve el mismo puerto no lleva sesión. **Qué
  sigue dentro**: los formularios públicos y el conector de solo lectura (`/_bases/v1/…`), que entran por el BFF y por los
  consumidores, y las sondas. Comparte host y certificado con la web: `/api/v1` es más específica que el `/api` del BFF,
  que no tiene rutas propias ahí. **No se abre CORS**: una app nativa no lo necesita y la web habla con su BFF. Lo que
  cambia al recibir tráfico directo, lo que se revisó y lo que se corrigió (los plazos de subida, descarga y exportación),
  en [security.md](security.md#bases-api-publica); lo vigila `helm-render-test.sh`, que mira todos los caminos al Service
  y no solo el Ingress del chart. Producción y staging siguen con Bases apagada: allí no cambia nada.

<a id="0069"></a>

## 0069 — Los scripts de usuario corren aislados en el navegador, contra una API versionada que se traduce al registro común de comandos

`accepted` · 2026-10-04 · Docus, Calcus, Diapos y, desde el 2026-10-05, Bases · ROADMAP-OFFICE `automatizacion-1` · código en `services/docs/internal/{domain,store,http}/scripts*.go`, migración `0022`, `platform/pkg/guiones` y, en el cliente, `apps/frontend/packages/scripts` (el runtime común) y el módulo de scripts de `apps/frontend/apps/docs/src/editor/`

- **Lo que ya había no es un sandbox y no se anuncia como tal.** El grabador de macros (`macros.ts`) valida pasos contra el
  registro de comandos y nada más; ejecuta lo que el usuario grabó con sus propios permisos. Los scripts son otra cosa y se
  construyen ENCIMA del mismo registro (`comandos.json`): es la superficie de la API, no una segunda.
- **Sin autoridad ambiental.** El código del usuario corre en un `<iframe sandbox="allow-scripts">` SIN `allow-same-origin`
  (origen opaco: ni cookies, ni storage, ni DOM del anfitrión), con CSP `default-src 'none'` en la página del iframe (sin red,
  sin `importScripts` de fuera, sin `connect-src`) y, dentro, un Worker que se mata (`terminate`) al pasar el tiempo. Todo lo
  que cruza es `postMessage` con mensajes de forma fija, validados y de tamaño acotado. El Worker no hereda el origen del
  iframe: el iframe no tiene, y un Worker `blob:` del iframe tampoco; fetch, XHR y WebSocket mueren por la CSP además de por
  el origen opaco.
- **La API es `sytue.v1.*`**, versionada y estable. El script pide; el ANFITRIÓN traduce cada llamada a un comando del
  registro y lo valida con los validadores de las macros (comando existente, válido para el tipo de documento, permiso de
  ahora, valor admitido) y, además, contra el manifiesto. Lo que el anfitrión despacha va por los mismos `dispatch` de siempre
  y acaba en `/ops`, donde el servidor reautoriza ACL y protección en cada escritura: **un script nunca es una vía para
  saltarse la protección**; si el editor no lo permite a la persona, el script tampoco.
- **Manifiesto por script** (JSON `v: 1`): comandos permitidos, familias de lectura, escritura sí/no, ámbito (hojas y
  rangos), `max_ops`, `max_ms`, y si puede correr sin interacción. El consentimiento se pide antes de la primera ejecución y
  cada cambio del manifiesto lo pide de nuevo. El servidor guarda el manifiesto canónico (listas ordenadas, cotas por defecto)
  y es lo que se ejecuta; el que se le enseñó a la persona es ese.
- **Versiones inmutables y revisión.** Una versión es `(código, hash SHA-256, manifiesto)` y no se reescribe (un trigger lo
  impide también por SQL). Editar crea una versión nueva en borrador. Un script de la organización lo ejecutan otras personas
  solo si un administrador aprobó ESA versión, y la aprobación lleva el hash del código revisado; el servidor la rechaza si no
  coincide. El autor puede probar sus borradores. Los scripts privados de otra persona no existen para nadie (404),
  administradores incluidos.
- **Quién ejecuta qué.** El servidor autoriza cada ejecución (`POST …/runs`): versión ejecutable, rol en el documento que pide
  el manifiesto (editor si escribe), tipo de documento y documento del script, y cuota. Devuelve el código y el manifiesto
  autorizados, que son exactamente lo que el cliente ejecuta, y un id de ejecución que el cliente cierra con el resultado.
- **Cuotas por hora móvil**: 60 ejecuciones por persona, 600 por organización y 10 minutos de tiempo reservado por persona
  (cuenta el máximo del manifiesto hasta que se cierra con el medido: abrir y abandonar no sale gratis). Se comprueban y se
  reservan en la misma transacción, serializada por organización.
- **Auditoría sin contenido**: quién, qué script y versión (con el hash), qué documento, resultado, duración, operaciones y
  un código de error corto. Los intentos que el servidor deniega también se anotan y no gastan cuota. Los resultados que
  cierran la ejecución los informa el cliente (el servidor no ve el código correr): son diagnóstico, no prueba; lo que sí es
  prueba es que se autorizó y qué versión.
- **Superficie de `sytue.v1`** (pieza B, `apps/frontend/apps/docs/src/editor/scripts/`): `run(comando, valor?)`,
  `select(hoja, rango)`, `read.cell|range|text|slides|selection|title()` y `log(...)`. Nada más cruza la frontera: el
  protocolo (`protocolo.ts`) admite cinco tipos de mensaje con claves exactas, y un objeto con `__proto__`, `constructor` o
  una clave de más se rechaza. Una llamada denegada ABORTA la ejecución (`denied` con el motivo): un script que intenta lo
  que no puede no sigue como si nada. Los límites los hace cumplir el anfitrión (`anfitrion.ts`), no el sandbox: tiempo
  (`timeout`), operaciones, mensajes por segundo, llamadas sin esperar, tamaño de mensaje, de salida y de datos devueltos
  (`limit`). La memoria de un Worker de otro origen no se puede medir desde fuera: la acota el tiempo.
- **Ámbito**: las lecturas de celdas y `select` han de caer dentro de las hojas y rangos del manifiesto; en Calcus, un
  comando que edita exige además que la selección actual esté dentro. En Docus y Diapos el ámbito no acota la escritura
  (no hay rangos): la acotan la lista de comandos y `write`.
- **Medido en Chromium real** (`test/e2e/scripts-sandbox.mjs`, 33 casos más un marco impostor, en el contenedor de agentes):
  fetch, XHR, WebSocket, `importScripts`, `import()`, `eval`, cookies, storage, `parent`/`top`, bucle infinito, memoria,
  spam de mensajes, mensajes forjados y comandos sin permiso fallan con su código y un servidor canario no recibe ninguna
  petición. Visto en rojo desactivando a mano cada defensa: sin la CSP el canario recibe tres peticiones (el `fetch` del
  prototipo, `importScripts` e `import()`: el preludio solo no basta); con `allow-same-origin` nada arranca (el anfitrión
  solo acepta mensajes de origen `null`); sin el reloj del anfitrión, el bucle de memoria corrió 677 s.
- **Interfaz** (pieza C): grupo «Scripts» en la pestaña Vista, junto a «Macros» (comando `scriptsManage` del registro
  común). El panel (`dialogs/ScriptsDialog.tsx`) lista, edita (textarea con numeración, sin dependencias), guarda versiones,
  cambia el ámbito, propone, aprueba o rechaza, ejecuta y enseña la salida, la cuota de la hora y el registro paginado
  (el de la organización a un administrador; el del documento a su propietario). «Ejecutar» corre la versión GUARDADA —se
  apaga con cambios sin guardar— y lo que corre es el código y el manifiesto que el servidor devuelve al autorizar; si ese
  manifiesto no es el consentido, no se ejecuta y queda anotado (`manifiesto_cambio`). El consentimiento se recuerda por
  (script, hash del manifiesto canónico) en el navegador de la persona: es una pregunta, no la frontera de seguridad.
  El BFF (`pages/api/scripts.ts`) reenvía por una tabla CERRADA de rutas (`lib/scriptsRuta.ts`).
- **La lista de comandos de los scripts es MÁS CORTA que la de las macros** (decisión del dueño, 2026-10-04). El registro
  común marca con `script: true` lo que un script puede usar, y lo que no lleva la marca no se abre por olvido: dentro,
  editar contenido y formato, celdas, filas y columnas, ordenar y filtrar, tablas, insertar enlace (pasa por el saneado) y
  pegar; fuera, la IA (`aifill`, `iafill`, `aiPanel`: gasta cuota y saca datos a un modelo), lo que abre diálogos o gestores
  (`scriptsManage`, macros, nombres, comentarios, combinación de correspondencia), lo que sale del documento (compartir,
  presentar, datos externos, imágenes y medios), permisos, protección, versiones, plantillas, marca y deshacer/rehacer. Lo
  exigen tres sitios: el validador del manifiesto del servidor (`domain.ParseManifiesto`, con la copia embebida del registro
  que vigila `sync-office-catalog.sh`) al guardar, el del cliente, y el anfitrión al despachar (`comando_vetado`). El
  servidor vuelve a validar el manifiesto en CADA ejecución: una versión guardada antes con un comando que ya no se admite
  no se borra ni se toca, pero no se ejecuta (400, auditado como `manifest_not_allowed`) hasta que se guarde otra corregida.
- **Fuera de alcance**: Fyne (un runtime nativo sería otro diseño), ERP (la API admite más superficies, pero no se
  conecta aquí), red desde los scripts y bibliotecas de terceros. Bases se conectó después: ver el punto siguiente.
- **El runtime es común y cada producto aporta su superficie (2026-10-05, PR #432).** Para dar scripts a Sytue Bases no se
  copió el sandbox: el iframe, la CSP, el Worker, el protocolo y el bucle del anfitrión pasaron a
  `apps/frontend/packages/scripts` (`@sytue/scripts`), PARAMETRIZADOS. Un producto declara su superficie como datos
  (`{ op, min, max, json? }`) y el paquete genera `sytue.v1`; el producto no escribe código que corra dentro del sandbox.
  El bucle (`ejecutarAislado`) recibe la lista cerrada de operaciones, las cotas de esa ejecución y un `atender` que solo
  contesta valor, rechazo o límite; tiempo, operaciones, ritmo, tamaño, salida, forma de los mensajes y el saneado de lo
  devuelto se quedan en el paquete, fuera del alcance del producto. Docs conserva su superficie y su traductor
  (`traducirLlamada`) y sus tres suites pasan sin tocar una línea (56 pruebas), además de las 35 de Chromium. `json` deja
  viajar un objeto como cadena JSON de tamaño acotado (un registro de Bases) sin ensanchar el protocolo. Las reglas del
  servidor que no son de un producto —cuotas, estados, quién ejecuta cuál, hash— están en `platform/pkg/guiones`, que
  importan `services/docs` y `services/bases`. La superficie y el servidor de Bases, en el [0068](#0068).
- **Para el dueño**: un administrador puede aprobar su propio script (en una organización de una sola persona no habría
  otra salida); queda en la auditoría quién aprobó. Se puede exigir un segundo administrador si se prefiere.

<a id="0070"></a>

## 0070 — La presencia de Docs se integra con Rooms: la sala de un documento nace del documento y su ACL manda

`accepted` · 2026-10-04 · Docus, Calcus, Diapos y Sytue Rooms · decisión del dueño · código en `services/rooms/internal/media/service/documento.go`, `apps/frontend/apps/docs/src/pages/api/llamada.ts` y `apps/frontend/apps/docs/src/editor/layout/EditorHeader.tsx`

- **Qué se ve.** La cabecera del editor ya mostraba quién está en el documento (`/api/presence`, `state/cursors.ts`, anuncio
  `aria-live`). Ahora los avatares llevan el nombre cuando se conoce, y si Rooms está disponible aparece «Llamar desde el
  documento» (o «Unirse a la llamada (n)» si ya hay una). Quien está en la llamada lo marca con un anillo y con su nombre
  accesible («Ana, en la llamada»). La llamada se abre en la ventana propia de Rooms (`/pip?documento=…`), así que el
  documento no se abandona.
- **Qué viaja a Rooms: solo el id y el título del documento.** Es un origen más de las salas (`kind: document`, junto a
  `calendar` y `chat`), con `ref_id` = UUID del documento. Nunca el contenido ni la lista de personas con acceso. El título lo
  pide el BFF a Docs con el token de quien llama, no lo manda el navegador.
- **La ACL del documento manda, en cada petición.** Rooms no replica quién puede ver el documento: al crear la sala, al
  mirarla (`by-source`, `GET /rooms/{id}`), al unirse, al llamar a la puerta y al listar participantes presenta el token de la
  persona a `GET /api/v1/documents/{id}` de Docs. Docs aplica su propia resolución de roles (usuario, grupos, organización).
  Vale lo mismo para el creador, para un administrador del tenant y para quien ya participó: quitar a alguien del documento le
  cierra la llamada en la siguiente petición. Falla cerrada: sin `DOCS_URL`, sin token, con Docs caído o con 5xx, se deniega.
  Las salas de documento no salen en `GET /rooms` (su título es el del documento).
- **Sin acoplamiento duro.** El BFF sondea Rooms con 2,5 s de espera; si no está desplegado, no resuelve o contesta 5xx,
  responde `available: false` y la cabecera no pinta nada, sin error. Docs no depende de Rooms para abrir ni guardar.
- **Integración por lo que ya existe.** Reutiliza `from-source` y `by-source` de Rooms y su evento `meet.room.created`, que ahora
  lleva `created_by`; el fan-out de llamadas de Rooms ignora los orígenes que no son canal. No hay canal nuevo en el Hub.
- **Auditoría de quién creó la sala.** La sala guarda `created_by` y el origen (`document`, id, título) y el evento de creación
  lo publica. El panel «Actividad» de Docs sigue siendo el de las operaciones del documento; no se mezclan.
- **Techo.** El sondeo de quién está en la llamada es cada 15 s con la pestaña visible, no por WebSocket; el botón se oculta
  por debajo de 768 px, donde la cabecera no cabe. Un aviso del Hub a las personas con acceso cuando alguien abre una llamada
  queda sin hacer: exigiría resolver la lista de personas desde Docs, justo lo que este diseño evita copiar.

### Cursores de hoja anclados por identidad

`SheetData` gana `rowIds` y `colIds` (`{ id }[]`, con `rowBase`/`colBase`): identidad de las filas y columnas, implícita
(`t0`, `t1`…) hasta que alguien inserta o borra. El cursor de hoja viaja además con `sheetId`, `rows` y `cols`; el receptor los
resuelve contra su modelo y, si falta alguno (cliente anterior, fila borrada), usa los índices de siempre, así que el protocolo
v1 no cambia. Se guardan como lista con `id` para que `fusionar3` conserve dos inserciones concurrentes.
Pruebas: `apps/frontend/apps/docs/test/sheets-anclas.test.ts`.

<a id="0071"></a>

## 0071 — El color de la suite es elegible: petróleo por defecto y azul cobalto, como eje aparte del modo

`accepted` · 2026-10-04 · toda la suite (web y nativas) · decisión del dueño · código en `branding/tokens/colors.css`, `branding/tokens/appearance.css`, `apps/frontend/packages/config/src/contraste-paleta.test.ts`, `apps/frontend/packages/ui/src/theme/`, `services/id/internal/http/admin_branding.go`

- **Qué se decide.** La suite deja de tener un único color de marca. Ofrece dos, **petróleo** y **azul cobalto**; quien no
  ha elegido nada ve petróleo. El esmeralda, que fue la marca única durante un día, desaparece como opción. Elige cada
  persona, con un color por defecto de la organización que fija su administrador.
- **Por qué dos y por qué estos.** El dueño vio la suite en esmeralda, naranja, teja, azul y petróleo. El petróleo queda
  por defecto porque la acción deja de parecerse al verde de «éxito»: un botón y una insignia de «Firmado» ya no comparten
  color. El cobalto es la alternativa viva, y es donde más se ve el ámbar de «estás aquí».
- **El color es un eje aparte del modo.** `data-color` en la raíz, junto a `data-theme`: dos colores por tres modos (claro,
  traslúcido, oscuro), más el oscuro del sistema. Petróleo es el valor sin atributo, así que una página que no sabe nada
  del color pinta bien. No es un cuarto y un quinto tema: mezclarlo con el modo habría dado seis bloques completos en vez de
  cuatro de petróleo más lo que cambia en cobalto.
- **El color tiñe también los neutros.** Lienzo, líneas y texto llevan un punto del tono, y los derivados (escala
  `neutral-*`, `bg-subtle`, `chrome-hover`) se derivan igual para cada color. Los colores de estado, de producto y de
  persona no cambian. «Info» sigue siendo azul en los dos.
- **Cómo no se rompe.** `contraste-paleta.test.ts` mide los pares que pinta la interfaz en los dos colores por los cuatro
  modos, aplicando los bloques como la cascada, y comprueba que cada modo de cobalto repite lo que su bloque claro declara:
  `[data-color]` y `[data-theme]` pesan lo mismo y a igual peso gana el que va después.
- **Dónde vive la elección.** En el mismo sitio que la del modo, sin plomería aparte: una cookie cosmética en el apex
  (`sytue-color`) más el almacenamiento local, leídas por el mismo lector y aplicadas por el mismo script del primer
  pintado (`packages/ui/src/theme/init.ts`), que es el que también llevan el IdP y las páginas autocontenidas. No es un
  dato de la cuenta: como el modo, no dice nada de quién es la persona. Seguir a la organización es NO tener elección: la
  opción del selector que lo dice borra la cookie y el almacenamiento, y el orden de siempre hace el resto. Va una vez,
  en `useEje`, para el color y para la forma.
- **El color de la organización.** Columna `organizations.color` (migración 0107 de Sytue ID), que el administrador fija
  en Sytue Admin (`PUT /api/v1/admin/org/color`). Viaja en el ID token como `org_color` —solo si no es el de la casa— y el
  BFF la deja junto a la sesión en una cookie legible, `sytue-color-org`, porque el script del primer pintado no ve la
  sesión. Orden: la persona, luego su organización, luego petróleo. Cambiarlo no invalida sesiones: llega con el siguiente
  token de cada uno.
- **La lista de colores vive en tres sitios y dos pruebas la atan.** La hoja de tokens (un bloque `[data-color]` por
  color), `THEME_COLORS` en la web y `domain.OrgColors` en Sytue ID. La base solo garantiza la forma del valor, para que
  añadir un color no pida reescribir un `CHECK`. `init.test.ts` y `TestLosColoresDeOrganizacionExistenEnLaHoja` fallan si
  uno de los lados se queda atrás.
- **Lo que queda fuera de esta decisión.** Las apps nativas leen hoy solo los bloques de petróleo de la hoja: su lector
  de tokens no tiene todavía la dimensión del color, así que ni la elección ni el color de la organización les llegan.

<a id="0072"></a>

## 0072 — «Color de cada app»: tercera opción del eje de color, carril y acción en el color de producto

`accepted` · 2026-10-05 · web (las nativas, cuando lean el eje de color) · decisión del dueño · código en `branding/tokens/colors.css`, `branding/tokens/appearance.css`, `apps/frontend/packages/ui/src/theme/init.ts`, `apps/frontend/packages/config/src/contraste-paleta.test.ts`

- **Qué se decide.** El eje de color de [0071](#0071) gana un tercer valor, `app`: cada app pinta el carril y la acción
  —botón principal, enlace, selección, foco— con su color de producto (`--sytue-product-<app>`). Es una opción más del
  mismo selector, para la persona y como color por defecto de la organización. Los neutros siguen siendo los de la casa.
- **Los valores salen de la hoja, no de un cálculo en el cliente.** Cada app aporta una rampa de once pasos derivada de
  su color de producto, y el reparto por modo se escribe una vez con `var()`: el 600 lleva rótulo blanco (4,7:1), el 700
  es el carril y el enlace (6,6:1 con blanco), el 400 la acción en oscuro con rótulo oscuro (4,7:1) y el 300 el enlace en
  oscuro (6:1). Son 13 líneas por app en vez de un tema entero por app y modo. `contraste-paleta.test.ts` mide cada app en
  los cuatro modos con esos umbrales.
- **Cómo sabe la hoja qué app es.** `data-app` en la raíz. En la web lo pone el script del primer pintado con el prefijo
  bajo el que se sirve la app, que ya estaba publicado antes de él: ningún layout cambia y no parpadea. Donde el prefijo
  no es la clave de producto (`/admin`, `/web`) la hoja lleva el alias, y un guardián lee el `base` de cada app y falla si
  una se queda sin color al cambiar de prefijo. Una superficie servida en la raíz publica el prefijo vacío y lo dice con la
  primera etiqueta de su host: es como Sytue Admin toma su color también en `platform.<dominio>`.
- **Cada editor, su color.** Docus, Calcus, Diapos y CertiPDF comparten `/docs`, así que ahí el prefijo no dice el
  producto. Su layout —y la vista pública de un enlace compartido— pone `data-app` en el `<html>` desde el servidor, con
  el producto del documento o de la lista que se enseña, y el script del primer pintado respeta el atributo que ya
  encuentra: no parpadea. Al abrir otro documento sin recargar lo cambia `aplicarMarca`, que es quien ya cambiaba el icono
  de la pestaña. Con esto, que PDF y Diapos solo tiñan el carril tiene efecto también en la web.
- **Las superficies de la casa se quedan en petróleo.** El Hub y Mi cuenta no llevan rampa: son el marco de la suite, y
  el Hub enseña a la vez las teselas de todas las apps. Con `data-color='app'` y sin rampa propia el reparto da los
  valores de petróleo. Tampoco la llevan el IdP ni las páginas autocontenidas, que no dicen qué app son.
- **El ámbar de «estás aquí» vale en todas.** Con el carril a 6,6:1 contra el blanco, el ámbar llega a 3:1 sobre el ítem
  activo en las 19 apps y en los cuatro modos; no ha hecho falta el blanco de respaldo.
- **Cinco apps solo tiñen el carril** (decisiones del dueño, 2026-10-05): Mail, PDF, Cast, Diapos y ERP. El color se
  queda en el carril y el marco; botón principal, enlace, selección y foco van en el primario de la casa. En la hoja esas
  apps declaran la rampa del carril (`--sytue-rail-*`) y no la del primario, así que la web y las nativas lo recogen sin más
  código.
- **La regla: las apps cuyo color se confunde con el de peligro en insignias, enlaces y foco.** No en el botón, que es lo
  que menos engaña. Con la acción en rojo, en Mail el anillo de foco de un campo parecía un error de validación, «Cerrar
  sesión» dejaba de distinguirse de un enlace y el botón principal parecía destructivo. En Diapos y ERP, que son naranjas,
  el botón principal y el de peligro se distinguen juntos, pero la insignia del tono de la app y la de peligro son las dos
  una píldora rosada con texto rojizo, el campo enfocado y el campo con error se separan más por el grosor del aro que por
  el color, y el enlace queda pegado al texto de peligro.
- **Es una lista decidida, no una fórmula, y se dice tal cual.** Ninguna medida separa a las que se confunden de la que no:

  | App       | Desvío de tono (OKLCH) respecto a `danger` | ΔE2000 con `danger` | Decisión           |
  | --------- | ------------------------------------------ | ------------------- | ------------------ |
  | PDF       | −1,9°                                      | 4,0                 | solo carril        |
  | Mail      | −11,9°                                     | 9,8                 | solo carril        |
  | Cast      | −15,5°                                     | 15,5                | solo carril        |
  | Diapos    | +16,4°                                     | 11,0                | solo carril        |
  | Bases     | +17,8°                                     | 13,9                | carril y acción    |
  | ERP       | +19,8°                                     | 14,2                | solo carril        |
  | Sytue Web | −37,6°                                     | 24,9                | fuera de la franja |

  Bases queda entre Diapos y ERP por tono y más cerca de `danger` que Cast por distancia, y aun así no se confunde: es un
  marrón chocolate, más oscuro y apagado que el rojo vivo. Lo que decide es mirarlo: la muestra del arnés de capturas (un
  diálogo de borrado con el botón principal y el de peligro juntos, un campo enfocado, otro con error e insignias, en claro
  y en oscuro), que es la que vio el dueño de las seis apps de la tabla.

- **Lo que sí se mide es quién tiene que pasar por esa decisión.** Una app cuyo color cae a menos de 26° del tono de
  `--sytue-danger` está en la franja. `contraste-paleta.test.ts` lleva las dos listas —solo carril, y en la franja con
  acción— y falla si una app con bloque cae en la franja sin estar apuntada en una: no la decide solo, pide la decisión.

<a id="0073"></a>

## 0073 — Rooms: denunciar y bloquear, con el bloqueo aplicado en el servidor y la denuncia en la organización anfitriona

`accepted` · 2026-10-06 · servicio, web y nativa · decisión del dueño («añade denuncia y bloqueo para rooms y cast y lo marcamos como que existe») · código en `services/rooms/internal/store/moderacion.go`, `services/rooms/internal/http/moderacion.go`, `apps/frontend/apps/rooms/src/components/Moderation.tsx`, `apps/fyne/apps/rooms/moderacion.go` · migración `services/rooms/db/migrations/0038_denuncias_y_bloqueos.up.sql`

- **Por qué ahora.** Google Play pregunta en la clasificación de contenido si la app permite bloquear y denunciar usuarios
  y contenido, y su política de contenido generado por usuarios lo exige cuando el contenido principal es de usuarios. Rooms
  no tenía ninguna de las dos cosas: lo único que existía era que el autor o un administrador del canal borrasen un
  mensaje, y que un administrador expulsara a alguien.
- **Denunciar.** Se denuncia un mensaje (y con él a su autor) o a una persona en un canal que se comparte con ella. El
  motivo sale de una lista cerrada —acoso, contenido sexual, violencia, spam, otro— y admite una nota de hasta 500
  caracteres. La denuncia es **de la organización que aloja la conversación** ([0040](#0040)): quien viene de fuera
  denuncia ahí, y es ahí donde se modera.
- **Quién modera.** Los administradores del **equipo** que aloja el canal —que ya podían borrar un mensaje ajeno y
  expulsar— y los administradores de la **organización** (rol `owner` o `admin` del token). A los del equipo se les avisa
  por el Hub (`rooms.report`), sin decir quién denuncia ni a quién; los de la organización lo ven al abrir la cola, porque
  Rooms no sabe quiénes son hasta que entran. La cola ofrece tres acciones y una denuncia se cierra con una: descartar,
  borrar el mensaje, expulsar del canal (esta última no en un mensaje directo).
- **Lo que no se revela.** Nadie ve una denuncia contra sí mismo, administre lo que administre. Y quién denunció no sale
  en ninguna respuesta de la API, tampoco a quien modera: para actuar no hace falta, y es lo que impide que le llegue al
  denunciado. Está en la base.
- **Y el operador.** Desde el 2026-10-12 las denuncias llegan también a la cola del operador de la plataforma, que sí ve
  las que son contra quien administra ([0075](#0075)).
- **Que no sirva para acosar.** Una persona denuncia cada mensaje una vez, y a otra persona en un canal una vez mientras
  esa denuncia siga abierta; repetirla devuelve la que había y no avisa a nadie. Hay un límite por persona en la ruta y un
  tope de 20 denuncias en 24 horas contado en la base, que vale con varias réplicas.
- **Bloquear es de la persona, no de la organización.** El `sub` de Sytue ID es el mismo en todas las organizaciones, así
  que el bloqueo vale también en la conversación que dos personas comparten en otra. Por eso `user_blocks` se lee y se
  escribe por la política de mantenimiento, siempre por el `sub` del token verificado.
- **Qué hace un bloqueo, en el servidor.** Un mensaje directo de dos personas con un bloqueo en cualquiera de los dos
  sentidos no admite mensajes nuevos —tampoco los programados ni la tarjeta de una llamada—: el servicio responde 403 con
  el mismo texto a los dos, sin decir quién bloqueó. Y los avisos (Web Push, APNs, Hub) de un autor no le llegan a quien le
  ha bloqueado, en ningún canal. En los canales y grupos compartidos el mensaje sigue llegando y el cliente lo enseña
  **plegado**, con un botón para leerlo: un canal de trabajo no puede tener huecos que solo ve una persona.
- **Se rechaza, no se esconde.** La alternativa en el directo era aceptar el mensaje y no enseñárselo a quien bloqueó. Se
  descarta: le mentiría a quien escribe sobre la entrega y dejaría en la retención de la organización mensajes que nadie
  vio.
- **Bloquear no borra nada.** La conversación sigue donde estaba, con la retención de la organización que la aloja. El
  bloqueo sí es un dato de la persona: se borra con su cuenta (`services/id/internal/merge`). Las denuncias, no: son de la
  organización.
- **La traza.** La fila de la denuncia dice quién, a quién, sobre qué y cuándo, y quién la resolvió, cómo y cuándo; se
  resuelve una sola vez y se borra doce meses después de resolverse ([0075](#0075)). Hay además una línea de log por denuncia y por resolución. Ni el texto del mensaje
  ni la nota van al log, y el texto del mensaje no se copia a la denuncia: la cola lo lee en vivo, así que un mensaje ya
  borrado no reaparece ahí.
- **Lo que queda fuera.** No hay bloqueo a nivel de organización (que una organización vete a una persona de fuera), ni
  apelación del denunciado, ni detección automática de contenido. Una persona bloqueada sigue viendo los mensajes de quien
  la bloqueó en los canales compartidos: el bloqueo protege a quien bloquea, no esconde a nadie de un canal de trabajo.

<a id="0074"></a>

## 0074 — Cast: denunciar y bloquear, con la denuncia abierta a quien mira sin cuenta y dos bloqueos aplicados en el servidor

`accepted` · 2026-10-06 · servicio, web y nativa · decisión del dueño («añade denuncia y bloqueo para rooms y cast y lo marcamos como que existe») · código en `services/cast/internal/social/denuncias.go`, `services/cast/internal/social/bloqueos.go`, `services/cast/internal/http/moderacion.go`, `apps/frontend/apps/cast/src/components/Moderacion.tsx`, `apps/fyne/apps/cast/moderacion.go` · migración `services/cast/db/migrations/0009_denuncias_bloqueos.up.sql` · hermana de [0073](#0073) (Rooms)

- **Por qué ahora.** Lo mismo que en [0073](#0073): Google Play lo exige a una app cuyo contenido es de sus usuarios. En
  Cast pesa más, porque tiene **portada pública**: lo ve quien no tiene cuenta. Lo único que existía era que el autor, el
  dueño del vídeo o quien administra la organización borrasen un comentario o un mensaje del chat.
- **Qué se denuncia.** Un vídeo o directo, un comentario, un mensaje del chat, una persona y un canal. El motivo sale de una
  lista cerrada —acoso, contenido sexual o desnudos, violencia, spam o engaño, derechos de autor, otro— y admite un texto de
  hasta 1000 caracteres. De lo denunciado se guarda una foto (de quién es y un extracto de 300 caracteres): el chat se
  retira a los 30 días y un comentario se puede borrar, y la cola tiene que seguir diciendo qué se denunció. En esto se
  aparta de Rooms, que lee el mensaje en vivo.
- **Solo lo que aparece en el vídeo.** Un comentario o un mensaje se denuncian dentro de SU vídeo, y una persona solo si
  aparece en él (lo publica o ha escrito en él). Así no se puede denunciar ni bloquear un `sub` cualquiera, ni sondear
  cuáles existen.
- **Sin cuenta también.** Quien mira la portada pública puede denunciar lo que ve sin cuenta: vídeos públicos o no listados,
  sus comentarios, quien los publica y el canal. No el chat, que no se lee sin sesión. Va con prueba de trabajo ALTCHA
  (`platform/pkg/altcha`, la de los formularios públicos de Bases), con un tope por IP compartido entre réplicas, y sin
  guardar la IP: solo un HMAC para no contar dos veces a la misma. La alternativa —exigir cuenta— dejaba sin puerta
  justo a quien Google pide que la tenga.
- **De quién es la denuncia y quién actúa.** Es de la organización dueña del **contenido**, no de la de quien denuncia. La
  ven y la resuelven quienes administran esa organización (rol `owner` o `admin` del token, que solo vale en su
  organización). La cola ofrece: descartar; borrar el comentario o el mensaje; retirar el vídeo de la portada pública (pasa
  a no listado); despublicarlo (pasa a la organización); y bloquear a la persona en toda la organización. Actuar cierra
  todas las denuncias abiertas sobre lo mismo; descartar, solo esa.
- **A quién se avisa.** Por el Hub (`cast.report`), a quienes administran la organización **y Cast ha visto**: el Hub
  entrega por persona y Cast no tiene la lista de miembros, así que apunta a cada administrador cuando abre Cast. Si lo
  denunciado es de otra persona en un vídeo ajeno, también al dueño del vídeo, que ya podía borrarlo. Nunca al denunciado
  ni a quien denuncia, y sin decir quién fue.
- **Lo que no se revela.** Quién denunció no sale en ninguna respuesta de la API: ni en la cola ni en la traza, donde
  figura como `account` o `anonymous`. Está en la base (`reports.reporter_sub`). Y, como en Rooms, **nadie ve una denuncia
  contra sí mismo**, administre lo que administre: ni en la cola, ni en la cuenta de abiertas, ni en la traza —cada fila de
  una denuncia lleva `target_sub` en el detalle, para que siga escondida cuando la denuncia ya se haya purgado—, ni para
  resolverla. La primera versión se la enseñaba a quien administra («en Cast quien publica es la organización»); el dueño lo
  cambió el 2026-10-12: si quien publica es quien administra, la denuncia la ven las demás personas que administran y el
  operador de la plataforma, en su cola ([0075](#0075)).
- **Que no sirva para acosar.** Una denuncia abierta por persona y objetivo (índice único parcial): repetirla contesta lo
  mismo y no avisa a nadie. Cinco de golpe y una cada veinte segundos por persona; diez de golpe y una cada treinta
  segundos por IP sin cuenta. La respuesta es siempre un 204 vacío.
- **Dos bloqueos, los dos en el servidor.** El **personal**: quien bloquea deja de recibir los comentarios y los mensajes
  del chat de esa persona (el servicio no los manda, tampoco por el WebSocket) y sus avisos de directo; y esa persona deja
  de poder comentar y escribir en los vídeos de quien la bloqueó. El **de organización**, que ponen quienes la
  administran: esa persona no escribe en ningún vídeo de la organización. Es lo que Rooms dejó fuera y aquí hace falta,
  porque en Cast escribe gente de fuera.
- **Se rechaza y se dice.** A quien está bloqueado el servicio le contesta 403 `blocked` al escribir, y la app se lo dice.
  No se le dice quién le bloqueó, pero sabe que no puede escribir ahí: aceptar el comentario y no enseñarlo le mentiría.
- **Bloquear no borra nada**, y se deshace: el personal, en «Denuncias y bloqueos»; el de organización, quien administra,
  en la misma pantalla.
- **La traza.** `moderation_audit`: una fila por denuncia, por acción y por bloqueo de organización, en la misma
  transacción que lo que audita, con quién actuó. Se lee con los filtros y el CSV de `platform/pkg/auditlog`.
- **Exportar y borrar.** «Exportar tus datos» de Cast lleva a quién ha bloqueado la persona y las denuncias que ELLA puso,
  de cualquier organización (qué, por qué, cuándo y si sigue abierta), sin la copia de lo denunciado ni la acción que se
  tomó, y nunca las que otros pusieron contra ella (política `reports_de_quien_denuncia`, migración 0011). «Borrar mi
  cuenta» borra sus bloqueos por el mismo camino que `user_blocks` de Rooms (`merge.DatosDeLaPersona`); `subject` es una
  columna generada sobre `blocker_sub`, porque renombrarla rompería a los pods anteriores durante el despliegue. Los
  bloqueos que otros le pusieron son de esas personas y se quedan; sus denuncias son de la organización del contenido y
  se purgan con su plazo ([0075](#0075)).
- **Varias réplicas.** Cast corre con dos o más. Las soluciones ALTCHA ya usadas se guardan en la base (`altcha_used`,
  migración 0012, con `altcha.EnTabla`, la misma forma que ya tenían Sytue ID, Bases y Web): una solución vale una vez
  en todas las réplicas. Y lo que el chat difunde en una —un mensaje, un borrado, un bloqueo recién puesto— le llega a
  las conexiones de las demás por LISTEN/NOTIFY de PostgreSQL (`Servicio.Relevar`, `events.Notificar`), con su origen
  para descartar el eco; antes, ni el bloqueo ni los mensajes cruzaban de réplica. El recuento de espectadores sigue
  siendo el de cada réplica. Si el NOTIFY falla, la trama se queda en su réplica, como antes.
- **Lo que queda fuera.** No hay apelación, ni detección automática, ni aviso a los administradores que no han abierto
  Cast nunca. El recuento de espectadores de un directo es el de la réplica de cada cual, no el total.

<a id="0075"></a>

## 0075 — Cola de denuncias del operador de la plataforma: las mismas filas, el sí de Sytue ID en cada petición y la traza en su registro

`accepted` · 2026-10-12 · Cast, Rooms, Sytue ID y consola de plataforma (solo web) · decisión del dueño («cola también para Sytue»; «un administrador no ve las denuncias contra sí mismo; Sytue sí») · código en `platform/pkg/operador`, `services/id/internal/http/platform_moderacion.go`, `services/cast/internal/http/moderacion.go`, `services/rooms/internal/store/operador.go`, `apps/frontend/apps/console/src/components/PlatformModeration.tsx` · migraciones `services/cast/db/migrations/0010_cola_del_operador.up.sql` y `services/rooms/db/migrations/0039_cola_del_operador.up.sql` · completa [0073](#0073) y [0074](#0074)

- **Qué.** Las denuncias de Cast y de Rooms llegan a dos colas: la de quien administra la organización dueña del contenido y
  la del operador de la plataforma (Sytue), en su consola (`platform.<dominio>/moderacion`). No hay copia: son las mismas
  filas, así que resolver en una cierra en la otra. `resolved_by` y `resolved_from` (`org` | `platform`) dicen quién y desde
  dónde, y en Cast la traza de la organización (`moderation_audit`) lleva además `from: platform`.
- **Qué ve el operador.** Todas las organizaciones, también las denuncias contra quien administra cada una (que esa persona
  ya no ve). Ve lo que la denuncia guarda o señala, y nada más: en Cast, el extracto de 300 caracteres, el nombre del
  denunciado y el texto de quien denuncia; en Rooms, el canal, el nombre y el mensaje denunciado recortado a 300
  caracteres. No hay enlace para entrar en el contenido del cliente, y quién denunció no viaja nunca.
- **Qué puede hacer.** Lo mismo que la cola de la organización: en Cast, descartar, borrar el comentario o el mensaje,
  retirar el vídeo de la portada, despublicarlo y bloquear a la persona en la organización; en Rooms, descartar, borrar el
  mensaje y expulsar del canal. Suspender la organización ya existe en la consola de plataforma, con su segunda persona, y
  la pantalla lleva allí. **Suspender a una persona a nivel de plataforma no existe** y no se ha inventado: hoy solo la
  suspende quien administra su organización.
- **Quién es operador lo dice Sytue ID, en cada petición.** El servicio no lo deduce de un rol ni de un claim: antes de
  listar o de resolver pregunta a `POST /api/v1/platform/moderation/access` con el token de quien llama y lo que va a hacer
  (servicio, acción, organización, denuncia). Esa ruta va detrás del mismo gate que el resto de la API de plataforma
  —allowlist, segundo factor y elevación viva—, escribe la fila del registro y contesta 204. Si no es 204, el servicio no
  enseña nada; si Sytue ID no puede escribir la fila, deniega. La comprobación y la traza son la misma llamada: no hay
  lectura sin rastro.
- **Solo desde la consola de plataforma.** Los servicios exigen además que el token sea del client `sytue-platform`: un
  operador elevado que navega por la web de Cast no abre ahí la cola de todas las organizaciones. Y la consola de
  plataforma solo se alcanza por el túnel administrativo.
- **La traza.** En el registro de Sytue ID, `platform.moderation.<servicio>.<acción>` con el operador, la organización
  desde la que actúa (`from_org`) y el client; con la organización afectada, la fila sale también en el registro de esa
  organización, que ve que Sytue resolvió una denuncia suya. Al listar no hay una organización: la fila dice qué se pidió.
- **Conservación (las dos colas, Cast y Rooms).** Una denuncia abierta se conserva hasta que alguien la resuelve. Resuelta o
  descartada, se borra **doce meses** después de su resolución, y con ella lo que guarda de lo denunciado: en Cast, el
  extracto, el nombre y el texto de quien denuncia; en Rooms, que no copia el mensaje, la fila entera (quién, a quién,
  motivo, nota y resolución). Lo hace un barrido diario de todas las organizaciones (`PurgarDenuncias`, por la política de
  mantenimiento). La **traza no se purga**: no lleva contenido —identificadores, acción y quién actuó— y
  `platform/pkg/auditlog` no fija plazo para ninguna traza de la suite; ponerle uno es decisión del dueño (propuesta:
  24 meses, el doble que la denuncia, para poder reconstruir una resolución discutida). La política de privacidad lo dice
  en los 26 idiomas.
- **Tratamiento por cuenta del cliente.** Que el operador lea y actúe sobre denuncias de una organización cliente es
  tratamiento de datos por cuenta de ese cliente (contenido denunciado, nombres, el texto de quien denuncia). **Tiene que
  figurar en las condiciones y en el DPA**: es decisión del dueño revisarlo antes de desplegar.
- **Lo que queda fuera.** Avisos al operador por el Hub (la cola es la verdad y se mira al elevarse), apelación, detección
  automática y suspensión de personas a nivel de plataforma.

<a id="0076"></a>

## 0076 — Docs se separa en cuatro productos con app, host y client propios; `docs.<dominio>/docs` desaparece sin redirecciones

`accepted` · 2026-10-12 · Docus, Calcus, Diapos, CertiPDF, Sytue ID y despliegue · decisión del dueño, por preguntas directas
(«cada editor es un producto con su nombre, su app nativa y su ficha en las tiendas; la URL tiene que decir lo mismo») ·
código en `apps/frontend/packages/office`, `apps/frontend/apps/{docus,calcus,diapos,certipdf}` · migración
`services/id/db/migrations/0111_clientes_oidc_de_la_ofimatica.up.sql` · runbook en
[operations.md](operations.md#ofimatica-una-app-por-producto) · sustituye la decisión del 2026-09-09 de servir los cuatro
productos bajo `/docs` con `?kind=`

- **Qué.** Cuatro productos con host y prefijo propios, con la organización en la ruta ([ADR 0056](#0056)):
  `docus.<dominio>/docus` (documentos), `calcus.<dominio>/calcus` (hojas), `diapos.<dominio>/diapos` (presentaciones) y
  `certipdf.<dominio>/certipdf` (PDF). Las subrutas se conservan: `docs.sytue.eu/docs/corporate/edit?id=<uuid>` de una hoja
  pasa a `calcus.sytue.eu/calcus/corporate/edit?id=<uuid>`, y el estudio de PDF a `certipdf.sytue.eu/certipdf/<org>/pdf?path=…`.
- **Cuatro apps, no una compilada cuatro veces.** Cada producto es una app de Astro (`apps/frontend/apps/<producto>`) con
  su `base`, su BFF y su client OIDC (`sytue-docus`, `sytue-calcus`, `sytue-diapos`, `sytue-certipdf`). Lo común —el editor,
  CertiPDF, las vistas, el middleware y el BFF que habla con el servicio— sale al paquete `@sytue/office` y no se copia
  ([ADR 0037](#0037)): las páginas de cada app son finas y dicen qué producto son; los endpoints `/api/*` los inyecta la
  integración `sytueOffice()`; las rutas de `/auth/*` reexportan como en el resto de la suite.
- **El servicio Go sigue siendo uno.** Documentos, hojas y presentaciones comparten modelo, colaboración, permisos,
  papelera y plantillas, y CertiPDF guarda sus ficheros en Drive; partirlo no daría nada y duplicaría la migración de
  datos. Ya filtra por tipo (`?kind=`), que es lo único que cada app necesita. Su API sigue en `docs.<dominio>/api/v1`:
  la usan las apps nativas instaladas, Drive (WOPI) y los BFF, y ninguna persona la ve en la barra de direcciones.
- **Un documento en la app de otro producto se va a la suya.** Si `calcus…/edit?id=` recibe una presentación, el servidor
  la manda al editor de Diapos con la misma organización y la misma vuelta a Drive: un enlace compuesto sin saber el tipo
  —un aviso, una búsqueda— no abre una presentación en Calcus.
- **`docs.<dominio>/docs` desaparece SIN redirecciones** (elegido frente a «redirigir al producto correcto»). **Lo que se
  pierde:** todo enlace guardado con esa forma —en un correo, un aviso viejo del Hub, un marcador, una nota o un mensaje
  de Rooms— deja de funcionar y da 404; los avisos ya emitidos no se reescriben. Los que se generan desde este cambio
  salen con el host del producto.
- **Sytue ID.** Los cuatro clients nacen en la 0111 sin secreto (lo pone el despliegue), y `docus`, `calcus`, `diapos` y
  `certipdf` son slugs reservados: una organización con ese nombre chocaría con el host del producto en el comodín. `docs`
  sigue reservado (host de la API).

<a id="0077"></a>

## 0077 — Rooms cifra en los dispositivos mediante Matrix; los canales externos conservan la protección real de su proveedor

`accepted` · 2026-10-16 · decisión expresa del titular en esta sesión · acota las
restricciones de lenguaje y licencia de 0016, 0018 y de la retirada de motores AGPL

**Problema.** Guardar y procesar texto en el servicio de Rooms impedía ofrecer E2E.
Añadir un candado al asistente no cambia quién puede leer el mensaje. Además, los
SDK web y Fyne deben intercambiar mensajes, recuperar historial y retirar dispositivos
sin entregar sus claves al servidor.

**Decisión.** El contenido nativo se cifra en los clientes con las implementaciones
mantenidas de Olm/Megolm: el SDK Matrix web con Rust/WASM y mautrix-go con GoOlm en
Fyne. Synapse, sin modificaciones, funciona como transporte separado, con base de
datos y credenciales propias; no se incorpora su código al producto propietario.
Las versiones están en CONVENTIONS §3 y los manifiestos fijan la imagen por digest.
Se conservan las licencias, avisos y acceso a las fuentes exactas de estas piezas.
La excepción MPL se limita a mautrix-go y su dependencia go.mau.fi/util, sin abrir
el gate a otras dependencias copyleft.

Sytue ID sigue siendo la autoridad de identidad. El inicio de sesión Matrix usa OIDC
y PKCE; las contraseñas, el registro público y la federación se desactivan. El servicio
Rooms aprovisiona salas privadas según su ACL y conserva referencias opacas a eventos
cifrados. El controlador administrativo permanece como miembro de metadatos:
Synapse exige que el administrador siga dentro para gestionar una sala privada, y
vacía el estado actual cuando sale el último miembro local. No ejecuta un cliente
criptográfico. Los SDK comparten claves únicamente con dispositivos verificados
por SAS o firmados por una identidad ya verificada; el MXID fijo del controlador
queda excluido de verificación y de solicitudes de claves, aunque publique un
dispositivo autofirmado. Esta separación se comprueba con los motores reales;
no depende de que la cuenta carezca de dispositivos en el momento del despliegue.
Las salas nuevas usan la versión 12 y su poder implícito del creador.

La suspensión de una cuenta y la retirada de una organización revocan el transporte
antes de confirmar el cambio en ID. Las reactivaciones salen del outbox compartido
después del commit, con versiones monotónicas y comprobación del estado actual de ID.
Los fallos no abren una vía de texto en claro. Recuperación, verificación de dispositivos,
adjuntos y borradores se resuelven en el cliente; el historial anterior se identifica
como anterior al E2E y no se presenta como cifrado retroactivamente. Las claves de
llamadas nativas deben circular por eventos Matrix autenticados y cifrados, nunca
por la señalización legible del SFU.

**Canales externos.** El titular autorizó intercambio completo con Slack, Teams y
Google mediante consentimiento de cada cuenta, y pidió usar E2E externo cuando exista
una API oficial compatible. Esas conversaciones se distinguen de Rooms nativo y
declaran la protección efectiva de su proveedor. No se descifra y reenvía una sala
nativa por activar una conexión. La opción de enviar solo avisos/enlaces sigue siendo
válida. Una videollamada que solicita E2E debe comprobar la confirmación del proveedor;
si no puede activarlo, falla de forma explícita sin reducir la protección a escondidas.

**Consecuencias.** Se opera un transporte adicional y se mantienen dos SDK compatibles.
Antes de publicar clientes se exige verificar intercambio web/Fyne, recuperación,
expulsión de miembros y dispositivos, adjuntos y medios. La aprobación arquitectónica
no demuestra por sí sola que esa validación esté terminada. La interoperabilidad
europea de WhatsApp queda aplazada por decisión del titular; la configuración de Meta
Business queda pendiente hasta disponer de una sesión administrativa autorizada.

**Fuentes.** [Especificación Matrix](https://spec.matrix.org/latest/client-server-api/),
[OIDC de Synapse](https://element-hq.github.io/synapse/latest/openid.html) y
[cifrado de reuniones Teams en Microsoft Graph](https://learn.microsoft.com/en-us/graph/api/application-post-onlinemeetings?tabs=http&view=graph-rest-1.0).

<a id="0078"></a>

## 0078 — Widgets nativos: contadores con caducidad en un App Group, Apple tras un interruptor y nada en Windows/Linux

`accepted` · 2026-10-21 · PR #967 · completa la sesión compartida de las apps de Fyne

**Problema.** Las apps de Fyne querían un widget en la pantalla de inicio (iOS, Android) y en
el escritorio de macOS, con un acceso directo a abrir y buscar. Un widget lo pinta el sistema
en otro proceso que no tiene ni la sesión, ni la red, ni el Keychain de la app; lo que lea
tiene que dejárselo la app escrito antes, y eso no puede convertirse en una copia en claro
de los datos de la persona. Además, en Apple compartir una carpeta entre la app y su
extensión pide un App Group registrado a mano en Developer Portal: encenderlo sin ese paso
rompe la firma de iOS, Mac App Store y Developer ID de las quince apps a la vez.

**Decisión.**

- **El formato del snapshot** (`apps/nucleo/widgets`, versión 1): un JSON por app,
  `widgets/<bundle>.json` dentro de la carpeta de ajustes de la suite, con `version`,
  `product`, `generation`, `updated_at`, `expires_at`, `active` y como mucho doce
  `counters` `{kind, value}`. `kind` sale de una lista cerrada (`unread`, `notes`,
  `calendars`, `address_books`…) y `value` es un entero acotado. El lector nativo
  (`Snapshot.swift`, `WidgetSnapshot.java`) rechaza campos de más, ficheros de más de
  16 KiB y cualquier valor fuera de esa forma.
- **Límites de privacidad.** Solo **contadores**: ni asuntos, ni nombres, ni títulos de
  evento, ni texto de mensajes, ni enlaces; el tipo no los admite. Un snapshot **caduca**
  (15 min al publicarlo, 30 como máximo) y se ignora caducado, con fecha futura o de otra
  generación de sesión (`sesion.marca`, opaca, nunca el sujeto ni el correo). Se **borra al
  cerrar sesión o al activar el bloqueo** de la suite, y el lector comprueba la marca y el
  bloqueo aunque el borrado falle. En iOS se escribe con `NSFileProtectionComplete` y la
  extensión pide Data Protection Complete: con el teléfono bloqueado no se lee. La app
  publica solo lo que ya ha pedido para su propia pantalla, nunca una petición extra, y
  captura la sesión **antes** de pedirlo: una respuesta tardía de otra cuenta no se publica.
  Las extensiones no tienen red ni Keychain.
- **Dónde vive.** En Android, la carpeta privada del propio APK, que lee su
  `AppWidgetProvider` (`eu.sytue.nativo.Widgets`). En Apple, el App Group
  **`group.eu.sytue.suite`** (`ajustes/grupo_apple.go`), común a las quince apps y a sus
  quince extensiones `eu.sytue.<app>.widgets`, que es también donde pasan a vivir los
  ajustes de la suite mientras están encendidos. Los tokens siguen en el Keychain (`<equipo>.eu.sytue.suite`), nunca
  en el App Group.
- **Apple, tras un interruptor único**: `SYTUE_WIDGETS_APPLE` en
  `apps/fyne/xcode/Sytue.xcconfig`, **`NO`** hasta que el dueño haga los pasos de
  [«Activar los widgets de Apple»](operations.md#activar-widgets-apple). Lo leen
  `empaquetar.sh` (`widgets-apple`), `generar.py`, `widgetkit-apple.py` y
  `release-desktop.yml`. Apagado, cada host se firma como antes de los widgets: sin
  extensión dentro, sin App Group, Developer ID sin perfil y la Mac App Store con el grupo
  `<equipo>.eu.sytue.suite` de siempre. Y si en marcha el sistema no da la carpeta del
  grupo, `ajustes` vuelve a la anterior sin error. Está en un fichero versionado y no en una
  variable del repositorio para que **viaje con el tag**: un tag anterior sale apagado, y
  encenderlo se ve en el diff.
- **Android, encendido**: no depende de nada externo y su empaquetado lo comprueba
  (`android-widgets-test.py`, `verificar_aab`).
- **Windows y Linux, nada.** El tablero de Widgets de Windows 11 pide empaquetar en MSIX, un
  proveedor COM y Adaptive Cards: otra cadena de distribución para un contador. Linux no tiene
  una superficie estándar (cada escritorio la suya, o ninguna). Sin un lector, Go **no
  escribe** snapshots: `widgets.Disponible()` solo es cierto en Android y en un host de Apple
  que haya cargado el puente de WidgetKit, que `empaquetar.sh` enlaza solo con el
  interruptor encendido.

**Descartado.** _Leer los datos desde la extensión con su propia sesión_: pondría un refresh
token en un proceso que el sistema arranca cuando quiere y que no puede pedir
reautenticación. _Títulos y próximos eventos en el widget_: es lo que se vería con el
teléfono en la mesa; se puede reconsiderar con una decisión expresa. _Una variable de
repositorio como interruptor_: no viaja con el tag y un re-despacho de una release antigua
encendería los widgets sobre fuentes que no los traen.

**Consecuencias.** Hasta encender el interruptor, iOS y macOS no tienen widgets, y la Mac
App Store sigue con el grupo antiguo. Encenderlo es un commit (el interruptor y
`python3 apps/fyne/xcode/generar.py`) después de los pasos manuales; desde ese tag,
`release-desktop` trae los perfiles de las apps y de sus widgets con `apple-perfiles.py`.
Ningún widget se ha visto aún en un dispositivo; lo que falta está en
[roadmap](roadmap.md#widgets-nativos-lo-que-falta).
