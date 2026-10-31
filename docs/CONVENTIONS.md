# Sytue One — Contrato canónico (single source of truth) · v4 "Sytue Native"

> Fuente de verdad de todo el monorepo. Cualquier nombre, dominio, puerto, versión,
> licencia, stack o ruta **debe** coincidir con lo aquí definido. Marca padre: **Sytue**.
> Suite: **Sytue One**.

<a id="0-principios-nucleares-en-orden-de-prioridad"></a>

## 0. Principios nucleares (en orden de prioridad)

1. **Stack ÚNICO y TOTAL: `Go + Astro + Preact + PostgreSQL`.** Todo **backend** se escribe en
   **Go**; toda **superficie web** en **Astro + Preact** (islands); el **datastore principal y
   fuente de verdad** es **PostgreSQL** (estado, colas y caché por defecto sobre Postgres). Las
   **versiones** no se citan aquí: viven en la tabla de
   [§3](#3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro), que es su fuente de
   verdad, y la política es **siempre la última estable**. Un número repetido en prosa envejece sin
   que nadie lo note; este decía «Astro» con la tabla ya en 7. Blobs
   grandes (ficheros, adjuntos, documentos) en **Object Storage S3** (Scaleway en prod, MinIO en dev).
   - **Valkey (BSD-3-Clause) — caché/cola OPCIONAL permitida.** Donde aporte una **mejora de
     rendimiento medible** de las apps (p. ej. fan-out de Chat/presencia, sesiones, rate-limiting,
     colas de alto volumen), **se permite Valkey** como capa de aceleración. No es obligatorio y
     **no** sustituye a PostgreSQL como fuente de verdad; se introduce solo con justificación de
     rendimiento. **Nunca Redis** (licencia no-OSI). Sin excepciones de lenguaje en backend salvo §0.5.
2. **Productos Sytue-native y originales.** No se despliega **ningún motor de
   terceros AGPL** (Nextcloud, OnlyOffice, Stalwart, Mattermost, Keycloak quedan **fuera**). Cada
   producto es **obra original de Sytue** (reimplementación **clean-room** sobre **estándares
   abiertos**), con licencia **Apache-2.0** (© 2026 Daniel Delgado Ramos).
   **Excepción expresa del titular (2026-10-16, [ADR 0077](decisions.md#0077)):**
   Rooms puede operar Synapse sin modificar, como transporte Matrix separado (AGPL),
   y usar mautrix-go con su dependencia go.mau.fi/util (MPL-2.0) en los clientes Fyne. La excepción a lenguaje y licencia
   queda limitada a esas piezas de E2E y no autoriza otros motores ni copiar su código
   al producto propietario. Las claves de mensajes permanecen en los clientes.
3. **Diseño primero — y en TODO el conjunto.** Lenguaje visual moderno (sidebar + topbar +
   contenido) con la marca Sytue. Una sola estética/biblioteca de componentes (`@sytue/ui` +
   `@sytue/shell`) para todas las superficies. Nada de UI ad-hoc.
4. **El IdP es la pieza NUCLEAR.** _Sytue ID_ es un **servicio Go propio** (OIDC/OAuth2/SAML/SCIM);
   identidad, SSO, autorización, grupos/tenancy, aprovisionamiento y presencia pasan por él.
5. **Soberanía 100 % UE por origen y operación.** Solo regiones UE de Scaleway y, siempre que
   exista alternativa viable, productos/proveedores originarios y bajo control último de la UE;
   alojar en Europa no cambia el origen. GitHub es la única excepción temporal general aprobada,
   limitada a repositorios, Actions y Releases durante el P0 de migración de `roadmap.md`. La
   excepción puntual de Bun/Kaniko para la release interna `v0.1.6` está cerrada por alcance y
   fecha en [ADR 0027](./decisions.md#0027); no autoriza nuevas dependencias ni una release posterior.
6. **Cumplimiento legal (clean-room).** Ver §12: reimplementación original sobre estándares
   abiertos y **librerías Go permisivas** (MIT/BSD/Apache); prohibido copiar/portar código AGPL.
   Las excepciones de aplicación de terceros son **LiveKit** (Apache-2.0, Go), como
   **SFU de medios WebRTC**, y el transporte Matrix separado aprobado en §0.2.
7. **Seguridad y privacidad por DEFECTO, por DISEÑO y REFORZADAS — en TODO el código (principio
   transversal que gobierna §1–§6).** Es criterio de _Definition of Done_ de cada PR. Detalle en
   [`docs/security.md#1-seguridad-y-rgpd`](./security.md#1-seguridad-y-rgpd). Mínimos no negociables:
   - **Privacidad por defecto (RGPD art. 25):** minimización de datos (solo el dato necesario),
     configuración más protectora por defecto, finalidad y retención limitadas con borrado
     automático, pseudonimización/cifrado, derechos del interesado (DSR) nativos, **sin telemetría
     sin consentimiento**, datos **solo en UE**.
   - **Seguridad por defecto:** autorización **deny-by-default** y mínimo privilegio; aislamiento
     multi-tenant con **RLS** en PostgreSQL; **TLS 1.3 + HSTS**; cookies `HttpOnly`/`Secure`/`SameSite`;
     **CSP** estricta y cabeceras de seguridad; protección **CSRF**; validación/saneado de entrada y
     **consultas parametrizadas** (sin SQL dinámico); **rate-limiting**/anti-fuerza-bruta;
     **MFA/WebAuthn**; secretos solo en gestor de secretos (**nunca** en código/logs); cifrado en
     reposo y de campos sensibles; **auditoría** inmutable; logs sin datos personales.
   - **Reforzada (defensa en profundidad):** modelado de amenazas por servicio; **OWASP ASVS** como
     vara; **SAST/DAST** + escaneo de dependencias/secretos en CI; **SBOM** y firma de artefactos;
     contenedores **rootless/distroless** y `securityContext` restrictivo; **NetworkPolicies
     deny-by-default**; **auditoría de seguridad obligatoria del IdP propio** antes de producción.
8. **Rendimiento como MÁXIMA en todos los componentes; y SEO en las superficies públicas.**
   Criterio de _Definition of Done_, con presupuestos que **rompen el build** en CI.
   - **Rendimiento por defecto:** SLO/presupuestos de latencia por servicio (objetivo p95 API
     < 200 ms, TTFB bajo); **Go** eficiente (pprof, mínimas asignaciones, concurrencia, pool pgx);
     **PostgreSQL** optimizado (índices, `EXPLAIN`, paginación _keyset_, sin N+1, vistas
     materializadas); caché donde **mida** mejora (Postgres o **Valkey** opcional §0.1); HTTP/2-3,
     compresión (br/gzip), keep-alive; **frontend Astro con islands y JS mínimo**, code-splitting,
     carga diferida, imágenes AVIF/WebP, edge/CDN en UE; **Core Web Vitals** verdes (LCP/INP/CLS);
     presupuesto de tamaño de bundle.
   - **SEO (superficies públicas: `www`, landing, contenido público compartible):** **SSR/SSG** con
     Astro, HTML semántico, metadatos + **Open Graph/Twitter Cards**, **datos estructurados JSON-LD
     (schema.org)**, `sitemap.xml` + `robots.txt`, URLs canónicas, **hreflang** (i18n), CWV como
     factor de ranking, accesibilidad WCAG AA. Las superficies **autenticadas** llevan `noindex`
     (no se indexan); el SEO se concentra en lo público.
   - **Medición continua:** Lighthouse/CWV y carga (**k6**) en CI con presupuestos; profiling continuo.

<a id="1-marca-y-catalogo-de-productos-todos-sytue-native-stack-go--astropreact--postgresql"></a>

## 1. Marca y catálogo de productos (todos Sytue-native, stack `Go + Astro+Preact + PostgreSQL`)

| Producto                     | Descripción                           | Backend (Go) — estándar/protocolo                                                                                                                     | Frontend                         | Subdominio                                                   |
| ---------------------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------- | ------------------------------------------------------------ |
| **Sytue One**                | Portal/lanzador                       | (usa Sytue ID + Hub)                                                                                                                                  | Astro+Preact                     | `one.sytue.eu`                                               |
| **Sytue ID**                 | Identidad/SSO/MFA/SCIM                | Servicio Go: **OIDC/OAuth2/SAML/SCIM** (libs Apache/MIT)                                                                                              | Astro+Preact                     | `id.sytue.eu`                                                |
| **Sytue Drive**              | Archivos/sync                         | Servicio Go: **WebDAV** + API REST propia; PG + S3                                                                                                    | Astro+Preact                     | `drive.sytue.eu`                                             |
| **Sytue Docs/Sheets/Slides** | Ofimática colaborativa                | Servicio Go: modelo documental + **colab CRDT/OT** (WS); PG + S3                                                                                      | Astro+Preact (editor propio)     | `docs.sytue.eu`                                              |
| **Sytue Mail**               | Correo                                | Servidor Go: **SMTP/IMAP/JMAP**; PG + S3 (blobs)                                                                                                      | Astro+Preact                     | `mail.sytue.eu`, `mx.sytue.eu`                               |
| **Sytue Calendar**           | Calendario                            | Servicio Go: **CalDAV** + API REST; PG                                                                                                                | Astro+Preact                     | `calendar.sytue.eu`                                          |
| **Sytue Contacts**           | Contactos                             | Servicio Go: **CardDAV** + API REST; PG                                                                                                               | Astro+Preact                     | `contacts.sytue.eu`                                          |
| **Sytue Notepads**           | Notas y cuadernos                     | Servicio Go: modelo propio + API REST; cifrado E2E en cliente; PG                                                                                     | Astro+Preact                     | `notes.sytue.eu`                                             |
| **Sytue Rooms**              | Mensajería y videollamada             | UN servicio Go: **WebSocket** + API REST + salas/señalización; **LiveKit** (SFU medios, Apache-2.0) solo transporta; PG                               | Astro+Preact (+ LiveKit web SDK) | `rooms.sytue.eu` (SFU en `rooms-sfu.`)                       |
| **Sytue Cloud**              | PaaS (Vercel × Supabase × GCloud)     | Servicio Go: control plane (deploy/build/servicios) + PostgREST; PG + S3                                                                              | Astro+Preact                     | `cloud.sytue.eu`                                             |
| **Sytue Admin**              | Admin / tenants                       | Servicio Go: admin API (sobre Sytue ID + Hub)                                                                                                         | Astro+Preact                     | `admin.sytue.eu` (antes `console.*`, hoy `301`)              |
| **Sytue Hub**                | Integración (shell backend)           | Servicio Go: búsqueda/notificaciones/presencia/intents; PG                                                                                            | —                                | `hub.sytue.eu`                                               |
| **Sytue Analytics**          | Analítica web de las apps del cliente | Dos servicios Go: colector (público, sin auth) + API de lectura; PG. **Sin cookies**: HMAC con sal diaria destruida ([ADR 0026](./decisions.md#0026)) | En `cloud-dashboard`             | **Primera parte**: `/_sytue` bajo el dominio de cada cliente |
| Sitio público                | Marketing                             | —                                                                                                                                                     | Astro+Preact                     | `sytue.eu`,`www.`                                            |

> La dirección canónica de cada producto es su subdominio (columna «Subdominio»):
> por ella van los enlaces entre apps, los avisos, las apps nativas y el login
> (ADR 0055). `<slug>.sytue.eu` sigue sirviendo la suite durante la transición.

> **Licencia:** todo el código de producto Sytue es **Apache-2.0 (© 2026 Daniel Delgado Ramos)**. Las **librerías**
> Go/JS incorporadas son **permisivas** (MIT/BSD/Apache-2.0) — compatibles con la Apache-2.0 del
> producto, conservando sus avisos (NOTICE). Las excepciones son **LiveKit** (SFU de medios) y
> **Synapse sin modificar**, separado del producto propietario (§0.2). Los dos módulos
> MPL del cliente Matrix conservan sus fuentes y avisos. No se copia código AGPL al producto.
> El tooling interno autónomo tiene un perímetro de licencia separado (§12): no forma parte del
> producto ni queda fuera de la política de soberanía de origen.

<a id="dominios-de-ejecucion-del-paas-sytue-cloud"></a>

### Dominios de ejecución del PaaS (Sytue Cloud)

- Apps prod de clientes: `https://<app>.apps.sytue.eu` · Previews: `https://<proyecto>-pr-<n>.preview.sytue.eu`
- Dominios propios: CNAME → `ingress.sytue.eu` + TLS automático.

<a id="2-dominio-e-infraestructura"></a>

## 2. Dominio e infraestructura

- **Dominio raíz:** `sytue.eu`. **Cloud:** Scaleway (solo UE). **Región:** `fr-par` (DR `nl-ams`).
- **Object Storage (S3):** `fr-par`, buckets versionados (drive-blobs, mail-blobs, docs-blobs,
  backups, build-cache PaaS, tfstate). **PostgreSQL** gestionado (Scaleway RDB) para todo el estado.
- Nombres de recursos: `sytue-<entorno>-<servicio>`. Labels k8s: `app.kubernetes.io/part-of=sytue-one`,
  `sytue.eu/env`, `sytue.eu/tenant`, `sytue.eu/stack=go`.

<a id="3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro"></a>

## 3. Versiones y stack — POLÍTICA "última estable" + cadena de suministro

**Política (obligatoria):** se usa **siempre la última versión ESTABLE (GA)** de cada lenguaje,
librería, componente y dependencia, para máxima madurez y mínima exposición a CVEs. Se **fija
exactamente** (go.mod/go.sum, `bun.lock`, tags de imagen por digest, `terraform.lock.hcl`) para
reproducibilidad, y se **actualiza de forma continua y automática** vía **Renovate/Dependabot**.
**Prohibido:** versiones EOL, RC/beta/nightly, o sin parches de seguridad. Donde el proyecto ofrezca
**LTS**, se usa la **LTS vigente** (no la "Current") para equilibrar novedad y estabilidad.
**Verificación sin vulnerabilidades en CI (gate que rompe el build):** `govulncheck` (Go),
`osv-scanner` (JS), `trivy`/`grype` (imágenes), **SBOM CycloneDX/SPDX** por artefacto.

Pisos vigentes (junio 2026; Renovate los sube al salir nuevas estables):

| Capa                                              | Tecnología                                                                                 | Versión (última estable / piso)                                                         |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| **Backend**                                       | **Go**                                                                                     | **1.26.x**                                                                              |
| Router/HTTP Go                                    | `net/http` + `chi` (o stdlib)                                                              | última estable                                                                          |
| Acceso a datos                                    | `pgx` + `sqlc`; migraciones `goose`/`golang-migrate`                                       | última estable                                                                          |
| IdP (libs)                                        | `ory/fosite` o `zitadel/oidc` (Apache-2.0) para OIDC/OAuth2 correcto                       | última estable                                                                          |
| WebDAV/CalDAV/CardDAV                             | `golang.org/x/net/webdav`, `emersion/go-webdav`, `go-ical`, `go-vcard` (BSD/MIT)           | última estable                                                                          |
| Correo                                            | `emersion/go-smtp`, `go-imap`, `go-message` (MIT) + JMAP propio                            | última estable                                                                          |
| Realtime                                          | `coder/websocket` (MIT)                                                                    | última estable                                                                          |
| **Frontend**                                      | **Astro / Preact**                                                                         | **Astro 7.x / Preact 10.29.x** (Preact 11 ya es GA; se espera la ventana de maduración) |
| Estilos                                           | Tailwind CSS                                                                               | **4.x** (última)                                                                        |
| Runtime FE / build / bundler / gestor de paquetes | **Bun** (MIT) — runtime, `bun install`, workspaces y bundler; sustituye a Node.js + pnpm   | última estable                                                                          |
| Interfaz nativa                                   | Fyne, con los parches de la suite fijados en `apps/fyne/go.mod`                            | **2.8.2**                                                                               |
| **Datastore**                                     | **PostgreSQL**                                                                             | **18.x**                                                                                |
| Colas/caché                                       | **PostgreSQL** por defecto (`riverqueue`/`pgmq`/LISTEN-NOTIFY); **Valkey** opcional (§0.1) | última estable                                                                          |
| Blobs                                             | Object Storage S3 (Scaleway) / MinIO (dev)                                                 | última estable                                                                          |
| Medios WebRTC                                     | **LiveKit** (Apache-2.0, Go) — _Sytue Rooms_                                               | última estable                                                                          |
| Transporte cifrado Rooms                          | Synapse sin modificar (AGPL), servidor independiente; excepción §0.2                       | **1.163.0**, imagen fijada por digest en `deploy/helm/charts/matrix/values.yaml`        |
| Matrix web                                        | matrix-js-sdk (Apache-2.0), criptografía oficial Rust/WASM                                 | **44.0.0 / 18.9.0**; ventana mínima de maduración del lockfile                          |
| Matrix Fyne                                       | mautrix-go y go.mau.fi/util (MPL-2.0), GoOlm                                               | **0.32.0 / 0.11.0**, excepción limitada §0.2                                            |
| Ingress/TLS                                       | Traefik 3 (Go/MIT) + cert-manager (Go/Apache)                                              | última estable                                                                          |
| Orquestación                                      | Kubernetes/Kapsule (última GA soportada por Scaleway) · Helm 3 · Kaniko                    | última estable                                                                          |
| IaC                                               | OpenTofu (MPL-2.0, preferido sobre Terraform BUSL) · provider scaleway/scaleway            | última estable                                                                          |

> **Eliminados del stack:** Keycloak, Nextcloud, OnlyOffice, Mattermost, Stalwart (motores AGPL de
> terceros) y **Redis** (licencia no-OSI). Su funcionalidad la dan **servicios Sytue-native en Go** +
> **PostgreSQL**. **Valkey** (BSD) permitido como caché/cola **opcional** por rendimiento (§0.1).
> **Nota:** "última estable" = GA, nunca beta/RC; prioriza **madurez + ausencia de CVEs**.

<!-- El ancla explícita conserva el enlace antiguo aunque el título ya no lleve versiones. -->

<a id="4-frontend-astro-7--preact-1029-ultima-estable--repuntado-a-los-servicios-go"></a>
<a id="4-frontend"></a>

## 4. Frontend (Astro + Preact, última estable — versiones en §3) — repuntado a los servicios Go

- Monorepo **Bun workspaces** en `apps/frontend/` (runtime/instalación/bundler con **Bun**, no Node/pnpm).
  Astro se ejecuta sobre Bun; los BFF (endpoints Astro) se sirven con Bun. Tailwind v4 + tokens de `@sytue/ui`.
- **Paquetes:** `@sytue/ui` (design system), `@sytue/shell` (topbar + app switcher + búsqueda +
  notificaciones + presencia + cuenta, montado en cada app), `@sytue/auth` (OIDC+PKCE contra
  **Sytue ID** + BFF), `@sytue/api-clients` (**SDKs a los servicios Go propios**: id, drive, docs,
  mail, calendar, contacts, notes, rooms, hub, cloud; + LiveKit web SDK para la videollamada), `@sytue/config`.
- **Apps:** `portal`, `console`, `cloud-dashboard`, `mail`, `drive`, `calendar`, `contacts`,
  `docs`, `notes`, `rooms`, `www`. Todas montan `@sytue/shell` y usan `@sytue/ui`.
- **SSO sin cookies de dominio:** cada BFF emite una cookie **host-only `__Host-`**, httpOnly,
  tras OIDC+PKCE contra Sytue ID. Cambiar de app es transparente porque el IdP conserva su propia
  sesión host-only. Nunca usar `Domain=.sytue.eu`: Cloud aloja código no confiable bajo ese apex.
  Sin secretos en el cliente. Modo claro/oscuro + WCAG AA.
- **El primer render del cliente debe dar el MISMO árbol que el del servidor.** Preact hidrata
  emparejando por **posición**: si el cliente pinta un hijo de más o de menos, reutiliza el
  elemento equivocado y le **conserva los atributos del SSR**. No avisa por consola, no lo ve
  TypeScript y no lo caza ninguna aserción de texto — el nodo existe y hasta tiene el rótulo
  correcto, solo que dentro de la caja de otro. El aviso de «sin conexión» de la topbar dejó dos
  defectos en producción a la vez: un **cuadro gris vacío** con borde y fondo para todo el que
  estuviera conectado, y —al estar de verdad sin red— el aviso pintado **dentro del separador de
  1px** y con `aria-hidden`, invisible y mudo justo cuando hay que leerlo.
  Regla práctica: **todo lo que dependa del navegador arranca en el valor neutro y se corrige en un
  `useEffect`**, nunca en el inicializador de `useState`. Ojo con `navigator`: Node ≥21 lo define
  como global **sin `onLine`**, así que `!navigator.onLine` es `true` en el servidor y el SSR sale
  creyendo que nadie tiene red. Comprobar `onLine === false`, no la falsedad.
  Puerta: `test/e2e/tests/topbar-hidratacion.spec.ts` mide las cajas en el DOM.

### 4.1 La marca de la barra superior es la de cada organización

Una organización puede subir su logotipo y sustituye al nuestro en la barra de las
once superficies. Quien no suba ninguno ve el nuestro con **la cuña teñida** de un
color derivado de su slug (doce tokens `--sytue-orgmark-*`, todos medidos por encima
de 3:1 contra el badge y contra el monograma). Determinista, nunca aleatorio: un
color que cambia entre recargas o entre apps se lee como un fallo. Solo cambia la
cuña; el badge y el monograma no se tocan, porque lo que se personaliza es el acento.

- **El reencuadre es del backend** (`platform/pkg/logofit`, solo stdlib). Averigua
  cuál es el color del margen y cada lado se come el suyo, de donde salen sin
  programarlos por separado los casos «los cuatro bordes», «solo horizontales»,
  «solo verticales», «uno solo» y «ninguno». Manda la transparencia cuando la hay
  —un alfa a cero es el diseñador diciendo dónde NO hay logo— y el recuento de
  colores del perímetro es solo el respaldo para JPEG, que no tiene alfa. Dos
  guardarraíles: si ningún color domina el borde no se recorta nada (degradados,
  logos sobre foto) y nunca más del 25 % de un lado (marcas diseñadas como placa).
- **El byte que se sirve nunca es el byte que se subió.** Se decodifica y se vuelve
  a codificar a PNG siempre, aunque la entrada ya viniera perfecta: es lo que
  permite servirlo en una ruta pública sin confiar en el fichero de nadie. El
  original no se guarda. SVG NO se acepta: no se puede reencuadrar sin rasterizar y
  es el formato con más superficie de ataque; el mensaje de error explica que hay
  que exportar a PNG.
- **Servido a escala sin tocar la base por pintado.** El hash del contenido viaja en
  el claim `org_logo` del ID token; la barra compone `?v=<hash>` sobre
  `id.sytue.eu/branding/<slug>/logo.png` y la respuesta es `immutable` un año. Al
  cambiar el logo cambia el hash, y con él la URL. Sin logo: 404 con caché corta.
- El binario vive en `bytea` (`org_branding`, migración 0051) y **no** en
  `platform/pkg/objectstore`, que es la norma de la casa: `services/id` no lo
  conoce, y cablear S3 entero en el servicio que custodia el JWKS para mover 30 KB
  no se paga. El día que ID guarde algo grande de verdad, ese es el momento.

<a id="5-lenguaje-de-diseno--sobrio-y-atractivo-estilo-mattermost--sytue-con-mucho-carino"></a>

## 5. Lenguaje de diseño — sobrio y atractivo estilo Mattermost × Sytue (con mucho cariño)

La interfaz gráfica es un valor de producto de primer orden: **estilo SOBRIO y ATRACTIVO como
Mattermost**, cuidado en cada detalle. Sidebar izquierdo (oscura/neutra) + topbar (shell global) +
contenido claro; **dark mode completo**; densidad cómoda de herramienta profesional; paleta contenida
con un acento; tipografía con jerarquía clara; bordes/radios suaves y sombras sutiles; **estados
hover/focus/active visibles** y focus ring accesible; **empty states** cuidados (icono + texto +
acción); microinteracciones CSS discretas (150–200 ms). Tokens de marca Sytue (primario/acentos/
neutros/semánticos, escala 4px) — **nada de HEX sueltos**. Componentes canónicos una sola vez en
`@sytue/ui`; las apps no crean variantes. Coherencia transversal: toda la suite se ve y se comporta
como **una sola aplicación**. WCAG AA. Guía en `docs/design.md#2-sistema-de-diseno` + `branding/`.

Los botones son rectangulares, con esquinas de **4 px** (`rounded-control`), en las dos
formas de la suite, incluido `www`, el acceso a la cuenta y las apps nativas. Los botones
de solo icono mantienen su proporción cuadrada y el mismo radio.

<a id="6-puertos-dev--docker-compose--dominio-dev-sytuelocalhost"></a>

## 6. Puertos (dev / docker-compose) — dominio dev `*.sytue.localhost`

| Servicio Go (backend)            | Puerto         |     | Frontend Astro (dev) | Puerto |
| -------------------------------- | -------------- | --- | -------------------- | ------ |
| Sytue ID                         | 8081           |     | portal               | 4321   |
| Sytue Drive                      | 8082 (+WebDAV) |     | console              | 4322   |
| Sytue Docs (+colab WS)           | 8083           |     | cloud-dashboard      | 4323   |
| Sytue Rooms (+WS)                | 8085           |     | www                  | 4324   |
| Sytue Mail (HTTP/JMAP)           | 8086           |     | mail                 | 4325   |
| Sytue Calendar (+CalDAV)         | 8087           |     | drive                | 4326   |
| Sytue Contacts (+CardDAV)        | 8088           |     | calendar             | 4327   |
| Sytue Notepads                   | 8089           |     | contacts             | 4328   |
| Sytue Cloud API                  | 8090           |     | (libre, era docs)    | 4329   |
| Sytue Cloud builder              | 8091           |     | rooms                | 4330   |
| Sytue Hub                        | 8092           |     | notes                | 4331   |
| Sytue Integration Gateway        | 8093           |     |                      |        |
| Sytue Analytics colector         | 8094           |     |                      |        |
| Sytue Analytics API              | 8095           |     |                      |        |
| Sytue OCR (interno, sin ingress) | 8096           |     |                      |        |
| Sytue Cast (+WS del chat)        | 8099           |     |                      |        |
| Sytue Bases                      | 8102           |     | bases                | 4337   |
|                                  |                |     | docus                | 4338   |
|                                  |                |     | calcus               | 4339   |
|                                  |                |     | diapos               | 4340   |
|                                  |                |     | certipdf             | 4341   |

| Infra                                  | Puerto                                |
| -------------------------------------- | ------------------------------------- |
| Traefik HTTP/HTTPS/dashboard           | 80 / 443 / 8080                       |
| PostgreSQL                             | 5432                                  |
| Object Storage MinIO (dev) API/consola | 9000 / 9001                           |
| LiveKit (Meet, WebRTC)                 | 7880 (WS) / 7881 (TCP) / 7882 UDP mux |
| Sytue Mail SMTP/IMAP                   | 25 / 465 / 587 / 993                  |

> **Sin** puertos de Redis/Valkey ni de motores AGPL (Keycloak/Nextcloud/OnlyOffice/Mattermost/Stalwart).

<a id="7-layout-del-monorepo"></a>

## 7. Layout del monorepo

```
sytueOne/
├── docs/            CONVENTIONS.md architecture.md decisions.md security.md
│                    operations.md design.md roadmap.md business.md README.md
├── platform/        # Go: módulo único y librerías compartidas — catálogo en §13.2
│   ├── go.mod (module git.sytue.eu/sytue/one)   go.sum
│   └── pkg/{asistente,auditlog,authn,cache,cms,commerce,config,events,httpx,ingressroutes,
│            keycrypto,log,logofit,migcheck,miniatura,objectstore,pdfdoc,pdffuente,pdfsimple,personas,pg,
│            pgtest,ssrfhttp,telemetry,tenancy,validation}
├── services/        # Go: un servicio por backend de producto
│   ├── ai/ analytics/ calendar/ cast/ cloud/ contacts/ docs/ drive/
│   │   hub/ id/ integrations/ mail/ notes/ ocr/ rooms/ syerp/ syphotos/
│   │   └── cada uno: cmd/<svc>/main.go · internal/{http,store,domain,...} · db/migrations · openapi.yaml · Dockerfile
├── apps/frontend/   # Astro + Preact (repuntado a services/*)
│   ├── apps/{account,calendar,cloud-dashboard,console,contacts,docs,drive,erp,
│   │         mail,notes,photos,portal,rooms,www}
│   └── packages/{ai,api-clients,auth,cloud,config,escena,formula,i18n,offline,scripts,shell,ui}
├── apps/fyne/       # Interfaz nativa (escritorio y móvil); su empaquetado, en scripts/
├── apps/nucleo/     # Núcleo Go de los clientes nativos, sin toolkit
├── apps/mobile/     # Capacitor (Rooms; la suite ya no publica)
├── deploy/          compose/ (dev local)   helm/ (prod Kapsule)   private-production/ (piloto)
├── infra/terraform/ modules/{network,kapsule,database,object-storage,registry,dns,secrets,loadbalancer,livekit}
│                    environments/{dev,staging,prod}
├── branding/        logo/ icons/ tokens/ brand-guidelines.md   (sin themes de motores de terceros)
└── scripts/
```

> Esta lista se desfasa sola: la generan las carpetas, no un acuerdo. Si al mirar
> el repo no coincide, manda el repo — y actualízala aquí, que es donde la busca
> quien llega nuevo.

> **Eliminados** (motores AGPL / TS): `identity/keycloak/`, `apps/hub/` (TS), `apps/paas/` (TS),
> `branding/themes/{keycloak,nextcloud,mattermost,onlyoffice}`. Sustituidos por `services/*` (Go).

<a id="8-identidad-y-sso-sytue-id--servicio-go-nuclear"></a>

## 8. Identidad y SSO (Sytue ID — servicio Go nuclear)

- **Sytue ID** es un servicio **Go** propio que implementa **OIDC/OAuth2** (Authorization Code +
  PKCE, refresh, JWKS, discovery), **SAML** (opcional), **SCIM** (aprovisionamiento) y **MFA/WebAuthn**,
  sobre librerías Apache-2.0 (`ory/fosite`/`zitadel/oidc`). Datos en **PostgreSQL**.
- **Clients OIDC (BFF confidencial por superficie):** `sytue-one`, `sytue-console`, `sytue-cloud`
  (+ `sytue-cloud-api`), `sytue-mail`, `sytue-drive`, `sytue-calendar`, `sytue-contacts`, `sytue-docs`,
  `sytue-notes`, `sytue-rooms`, `sytue-hub`. **No** hay clients de federación de motores (ya no hay motores).
- **Cada servicio Go valida los tokens de Sytue ID** (JWKS) y aplica autorización por roles/tenant.
- **LiveKit (videollamada):** el BFF `sytue-rooms` mintea el **JWT de LiveKit** (API key/secret) tras validar OIDC.
- **Grupos/roles:** `org/<tenant>`; roles `owner`, `admin`, `member`, `billing`. MFA/WebAuthn
  obligatorio para `admin`/`owner` al administrar la identidad de una organización, y para
  la administración de plataforma. Entrar en las apps o en «Mi cuenta» no obliga a enrolar
  MFA por tener esos roles: el alta de MFA es voluntaria, salvo política explícita de la
  organización. Si la persona ya lo activó, el login exige su segundo factor. Un alta en
  Sytue ID → acceso a toda la suite (tenancy en §9).

<a id="9-multi-tenant"></a>

## 9. Multi-tenant

**La organización ACTIVA de una sesión** es la que va en el claim `tenant` del access token, y de
ella deriva TODO el aislamiento de la suite. Se elige al abrir la sesión (`createSession`, que
comprueba membresía + estado + derecho de uso) y se cambia con
`PUT /api/v1/me/active-organization` (`services/id/internal/http/me_orgs.go`), que aplica las mismas
tres verjas, mueve las sesiones vivas y revoca los refresh tokens —llevan la organización congelada—.
El token que el navegador ya tiene NO cambia: hay que rehacer el login SSO, que con la cookie de
sesión viva es una redirección, no un login. Sin este endpoint, la activa era la membresía más
antigua y no había forma de cambiarla: quien es `member` en la vieja y `owner` en la nueva recibía
403 en toda la consola.

Organización (tenant) → Workspace → Usuarios. tenant ⇄ grupo en Sytue ID `org/<slug>` ⇄ namespace
Kapsule `tenant-<slug>` (ResourceQuota + NetworkPolicy) ⇄ esquema/filas por tenant en PostgreSQL ⇄
prefijos S3 dedicados. **Sytue Cloud (PaaS):** git push → build Kaniko (Job rootless) → Scaleway
Registry → deploy (Serverless Containers o namespace) → URL preview/prod con TLS. Servicios gestionados
(Sytue Postgres/PostgREST, Storage, Functions, Cron) inyectan credenciales en los deployments del proyecto.

<a id="10-seguridad-y-rgpd-detalle-en-security-gdprmd"></a>

## 10. Seguridad y RGPD (detalle en security-gdpr.md)

Residencia solo UE. TLS 1.3; cifrado en reposo (S3/Block + cifrado de columnas sensibles en Postgres).
Secretos en Scaleway Secret Manager, materializados como `Secret` de Kubernetes
por el bootstrap de producción (nunca en git). Correo: SPF+DKIM+DMARC+MTA-STS+TLS-RPT.
Auditoría centralizada (Sytue ID + cada servicio + Hub). Backups: snapshots PG + versionado S3.
Cumplimiento: DPA, ROPA, runbook DSR, retención. La **auth crítica** (Sytue ID) se somete a auditoría
de seguridad antes de producción (ver `security.md#4-politica-de-reimplementacion-clean-room` y `security.md#1-seguridad-y-rgpd`).

<a id="11-integracion-seamless-sytue-shell--sytue-hub"></a>

## 11. Integración seamless (Sytue Shell + Sytue Hub)

`@sytue/shell` (front) aporta navegación/app switcher/búsqueda/notificaciones/presencia/cuenta a cada
app. **Sytue Hub** (servicio Go, `hub.sytue.eu`): búsqueda federada sobre los servicios (drive/mail/
calendar/contacts/notes/rooms), notificaciones unificadas (SSE/WS), presencia, **intents cross-app**
(adjuntar de Drive en Mail, crear evento desde un correo, compartir en Chat). Autoriza con tokens de Sytue ID.

<a id="12-licencias-propiedad-y-politica-clean-room"></a>

## 12. Licencias, propiedad y política clean-room

- **Código de producto Sytue = PROPIETARIO (© Sytue).** No copyleft; no obligación de publicar fuente.
- **Reimplementación clean-room:** los productos son **obra original** sobre **estándares abiertos**
  (OIDC, WebDAV, CalDAV, CardDAV, SMTP/IMAP/JMAP, OOXML/ODF, ActivityPub/propio) — las ideas, la
  funcionalidad, el _look & feel_ y los protocolos **no** tienen copyright. **Prohibido** copiar,
  traducir o portar código AGPL (sería obra derivada). Quien implementa **no consulta** el código
  fuente de los motores AGPL; trabaja desde RFCs/specs/APIs públicas. Proceso detallado y registro en
  [`docs/security.md#4-politica-de-reimplementacion-clean-room`](./security.md#4-politica-de-reimplementacion-clean-room).
- **Dependencias de producto permitidas:** solo **librerías permisivas** (MIT/BSD/Apache-2.0/ISC/
  PostgreSQL/MPL-2.0). Se conservan avisos (NOTICE/atribución). Las aplicaciones de terceros
  admitidas son **LiveKit** y **Synapse sin modificar**, con los límites de §0.2. **Prohibido**
  incorporar, enlazar, empaquetar o redistribuir dependencias AGPL/GPL o _source-available_
  (SSPL/BSL/MSL/Elastic/RSAL). Gate de licencias en CI (SBOM CycloneDX/SPDX).
- **Infraestructura operativa:** cualquier motor de terceros que atienda el runtime o procese datos
  de clientes se aprueba caso por caso en una decisión registrada, con licencia, términos de uso,
  aislamiento, origen y plan de salida. No hereda el permiso del tooling interno.
- **Tooling interno de ingeniería:** Git, forjas, CI, analizadores y herramientas de administración
  se ejecutan como procesos/servicios independientes y **no se incorporan al producto**. Pueden usar
  GPL/AGPL si no se enlazan ni empaquetan con Sytue One, no se ofrecen a clientes, constan en el
  inventario de tooling y se cumplen todas sus obligaciones de licencia, incluidas modificaciones y
  oferta de fuente cuando corresponda. El gate de producto demuestra que su código no llega a
  binarios, imágenes o instaladores. _Source-available_ continúa denegado por defecto y requiere ADR.
- **Soberanía independiente de la licencia:** los tres perímetros —producto, infraestructura y
  tooling— pasan además el gate de origen UE. Que una herramienta sea FOSS, interna o esté alojada
  en una región UE no convierte a su proveedor/producto en europeo.
- **Marcas:** la marca de cara al usuario es **Sytue**; no se usan marcas de terceros. Matriz y
  obligaciones en [`docs/security.md#5-licencias-propiedad-y-cumplimiento`](./security.md#5-licencias-propiedad-y-cumplimiento).

<a id="13-convenciones-de-codigo"></a>

## 13. Convenciones de código

- **Go:** módulo único `git.sytue.eu/sytue/one`; un binario por servicio (`services/<svc>/cmd/<svc>`);
  capas `internal/{http,domain,store}`; errores envueltos; `context` en todo I/O; `slog` para logs;
  migraciones versionadas; tests + `go vet`/`golangci-lint`. Cabecera SPDX propietaria por archivo.
- **Frontend:** TypeScript estricto; oxlint + Prettier (ESLint solo para `*.astro`, que oxlint
  todavía no parsea); nada de UI fuera de `@sytue/ui`.
- **Datos:** PostgreSQL como única verdad; migraciones por servicio; multi-tenant por esquema/RLS.
  Las migraciones son **expand → contract** y lo comprueba un gate en CI (`platform/pkg/migcheck`,
  dentro de `go test ./...`): un `.up.sql` con `DROP COLUMN`, `RENAME`, cambio de tipo, `SET NOT NULL`,
  `TRUNCATE` o retirada de política/constraint/función rompe el build salvo que el fichero lleve
  `-- sytue:destructive-ok <razón>`. Motivo en `operations.md#42-verificar-y-revertir`.
- **IaC:** Terraform `fmt`, módulos reutilizables, estado en backend S3 Scaleway.
- **Antes de escribir plomería, mirar [§13.2](#13-2-lo-que-ya-existe)**: el catálogo de lo que ya
  existe en `platform/pkg/` y en `apps/frontend/packages/`, y qué NO se unifica y por qué. Las
  reglas de eficiencia están en [§13.3](#13-3-eficiencia) y qué prueba dejar en
  [§13.4](#13-4-pruebas). El porqué de todo ello, en el [ADR 0037](decisions.md#0037).
- Documentación de producto en **español**; identificadores en inglés. Licencia del código: **propietaria © Sytue**.

<a id="13-1-idiomas-de-producto"></a>

### 13.1 Idiomas de producto — NORMATIVO

**DECIDIDO 2026-08-11 (Daniel).** Las dos preguntas que este documento dejaba abiertas
tienen respuesta, y las dos son trabajo pendiente, no discusión:

1. **Los ~444 `WithMessage(...)` en castellano de los 8 servicios Go pasan a INGLÉS.** El
   idioma por defecto de la plataforma es el inglés; el castellano es una traducción más.
   Esto NO es cosmético: son mensajes que la interfaz enseña tal cual al usuario (la
   tarjeta del logotipo enseña el de Sytue ID sin tocarlo), así que hoy un administrador
   alemán recibe castellano. Ruta: reescribir el literal a inglés en el servicio y, donde
   el mensaje sea para una persona y no para un log, moverlo al catálogo. Hacerlo por
   servicio, no en una barrida: el gate de i18n del frontend no cubre Go.

   **Recuento real medido el 2026-08-11: 530 mensajes en ONCE servicios**, no los 444 en
   ocho que decía este documento. Por servicio: `id` 165 · `mail` 70 · `drive` 70 ·
   `rooms` 58 · `calendar` 44 · `cloud` 43 · `integrations` 36 · `docs` 18 ·
   `contacts` 13 · `hub` 10 · `ai` 3.

   Orden sugerido: **`id` primero** —es el que más tiene y el único cuyos mensajes se
   enseñan hoy tal cual en pantalla (la tarjeta del logotipo)—, y dentro de él ya está
   hecho `admin_branding.go`. Después los que tienen interfaz propia delante.
   Comando para medir el avance:

   ```sh
   for d in services/*/; do grep -rho 'WithMessage("[^"]*"' "$d" --include='*.go' |
     grep -cE '[áéíóúñ¿¡]|\b(de|la|el|los|las|un|una|que|no|con|para|se)\b'; done
   ```

2. **Se acepta SVG para el logotipo de organización — RESUELTO 2026-08-11.** Era el
   formato en el que llega la mayoría de logotipos corporativos y obligar a exportar a
   PNG era la renuncia más cara del diseño.

   **Se rasteriza en el NAVEGADOR, no en el servidor**, que es lo contrario de lo que
   este documento planeaba. El plan original —sanear el SVG en Go y rasterizarlo allí—
   exigía una dependencia nueva de rasterizado, y traer una dependencia es «riesgo B»
   de esta misma política; además dejaba a Sytue ID, que custodia el JWKS, parseando
   XML de terceros. Convirtiendo en el cliente desaparecen las dos cosas: al servicio
   NUNCA le llega un SVG, así que no hay nada que sanear ni superficie nueva que
   proteger, y el motor de encaje recibe un mapa de bits como el de cualquier otro
   logo. Cero dependencias.

   Es seguro porque un SVG cargado por `<img>` va en modo imagen: el navegador no
   ejecuta su script ni trae sus recursos externos, por especificación. Y si el fichero
   intentara cargar algo de fuera, el lienzo quedaría contaminado y `toBlob` fallaría,
   con lo que ese caso se rechaza solo. Ver `svgAPng` en `OrgLogoCard.tsx`.

   La API sigue admitiendo solo ráster, y su mensaje de error sigue explicando cómo
   exportar: quien llame al endpoint sin pasar por la consola no pierde nada.

**La plataforma habla 26 idiomas: los 24 oficiales de la UE —inglés como BASE— más noruego bokmål e
islandés, para cubrir el EEE y Suiza** (ampliado el 2026-09-14; hasta entonces eran cinco: inglés,
español, francés, alemán e italiano). No es un objetivo de una ola: es una condición de «terminado».
Una función sin todos los idiomas de su app no está acabada, y el compilador lo exige
(`AppCatalog<typeof en>`: una clave nueva en inglés rompe cada catálogo que no la tenga).

- **Los 21 idiomas nuevos se publican app a app.** El cromo compartido (`@sytue/i18n`), Sytue ID, el
  SDK de Cloud, el ERP y el escritorio van en los 26. Una app pasa a los 26 cuando tiene sus 21
  catálogos (`src/i18n/messages/<cc>.ts`); mientras tanto sirve en inglés el idioma que le falta, en
  el servidor y en la isla (`AppCatalogs` en `translate.ts`). La web pública ya sale en los 26; un
  idioma nuevo no se publica en ella sin su catálogo, porque saldría en inglés bajo su prefijo y el
  `hreflang` lo anunciaría como traducido.
- **Tratamiento y género.** Cada idioma con su convención, como los catálogos escritos a mano: de tú
  en es, it, pl, sv, da, nb, fi, is, ga, mt y nl; de usted en de, fr, pt, cs, sk, hr, sl, bg, lt, lv,
  el, ro, hu y et. Y nunca con el género del usuario ni de terceros: forma impersonal o nominal en
  vez de participio o adjetivo concordado («Logowanie powiodło się», no «Jesteś zalogowany»;
  «Norint išjungti», no «Norėdami išjungti»).
- **El inglés es el idioma base.** Las cadenas nacen en inglés en el catálogo y las demás son
  traducciones de ese original. Esto es el idioma del PRODUCTO: la convención de arriba
  —documentación y comentarios en español, identificadores en inglés— no cambia.
- **Alcance:** todo lo que ve un usuario. Interfaz de las 11 apps, correos transaccionales, mensajes
  de error de la API, salida del CLI y también las funciones que GENERAN texto.
- **Las funciones generadoras respetan la gramática de cada lengua**, no traducen palabra a palabra.
  El caso de referencia es el sugeridor de nombres de proyecto: en español, francés e italiano el
  adjetivo va detrás y concuerda en género; en inglés va delante y no concuerda; en alemán va
  delante, hay tres géneros y el adjetivo antepuesto se declina.
- **Nunca una cadena visible suelta en el código.** Va al catálogo, con todos sus idiomas, en el
  mismo commit. Lo que una ola no pueda traducir se reporta como pendiente explícito.

Motivo: una suite soberana europea que solo habla español no es vendible fuera de España, y
retro-traducir la interfaz cuesta mucho más que nacer traducida.

**Estado medido el 2026-08-07.** Las once interfaces y las pantallas del IdP cumplen: la de acceso
negocia por `Accept-Language` y sale en los cinco (`lang="de"` + «Anmelden», `lang="fr"` +
«Se connecter»). Dos gates lo sostienen: `apps/frontend/scripts/lint-i18n.sh` (trinquete sobre los
rótulos del código) y `test/e2e/tests/i18n-sin-espanol.spec.ts` (carga cada app pidiendo inglés).

Lo que NO cumple son los **mensajes de error de la API**, que esta misma sección incluye en el
alcance: **444 `WithMessage(...)` en castellano**, repartidos así —`id` 143, `mail` 67, `drive` 64,
`rooms` 45, `cloud` 42, `calendar` 42, `integrations` 29, `docs` 12—. El contrato de
`platform/pkg/httpx/errors.go` ya dice qué debería ocurrir: «los mensajes son el respaldo en inglés;
el cliente traduce por `Code`». Hoy no pasa ninguna de las dos cosas.

Hay dos caminos y conviene elegir uno antes de tocar 444 sitios:

1. **Cumplir el contrato tal cual**: pasar los respaldos al inglés y que el cliente traduzca por
   código. Barato, pero el código (`bad_request`) es mucho menos específico que el mensaje
   («client_id desconocido»), así que el usuario pierde información.
2. **Traducir en el servidor**: `Fail` ya recibe la `*http.Request`, y el IdP ya tiene catálogo de
   552 claves en cinco idiomas con negociación por `Accept-Language` (`services/id/internal/i18n`).
   Extender ese patrón conserva la especificidad a cambio de un catálogo por servicio.

Mientras tanto el daño es limitado —el frontend casi nunca pinta el `message` crudo—, pero la norma
se incumple y conviene decidirlo antes de que sean 800.

<a id="13-2-lo-que-ya-existe"></a>

### 13.2 Antes de escribir código nuevo: el catálogo de lo que ya existe — NORMATIVO

Cada producto nuevo de esta suite se ha creado copiando el anterior. Es la forma
más rápida de empezar y la más cara de mantener: lo copiado **diverge solo**, y
lo hace por donde nadie mira. Medido en este repo, todo real:

- el mismo endpoint de auditoría en Mail y en Drive, de ~490 líneas idénticas
  salvo 36 — y ya contestaba en **español en uno y en inglés en el otro**;
- doce `middleware.ts` que el fichero de referencia pedía «copiar tal cual»: para
  cuando se midieron, solo cinco seguían siendo iguales byte a byte;
- tres copias del mismo getter de access token con **márgenes de caducidad
  distintos** (60 s contra 30 s), que es como una app manda tokens ya caducados.

**La regla:** antes de escribir una pieza de plomería, búscala aquí. Si existe,
se importa. Si existe y no encaja, se discute **ampliarla**, no copiarla.

#### Go, clientes nativos — `apps/nucleo/`

Módulo aparte (`git.sytue.eu/sytue/one/apps/nucleo`): las dependencias de
cliente de escritorio —llavero, D-Bus— no entran en el go.mod de la plataforma.
Se usa con un `replace` desde `apps/fyne`, y desde el módulo raíz para `formula` y
`numfmt` (Docs exporta las hojas con sus valores): por eso los Dockerfile de los
servicios copian también `apps/nucleo/go.mod`.

| Paquete              | Para qué                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ajustes`            | La carpeta `<config>/sytue` que comparten las trece apps: tema, idioma, la marca de que la sesión ha cambiado y el cerrojo entre procesos ([ADR 0050](decisions.md#0050))                                                                                                                                                                                                                                                                                                                                                                    |
| `api`                | Cliente HTTP/WS de los servicios de la suite, producto a producto, `Elegir.Listar` (lo que el selector de la suite enseña de Drive, Docs, Calendar, Rooms o Notepads, con el token de la persona y sin BFF), los enlaces entre apps `RutaRedactar`/`LeerRedactar` (abrir el redactor de Mail con destinatario, asunto, cuerpo y adjuntos de Drive) y `RutaCompartir`/`LeerCompartir` (la pantalla de compartir de Rooms), y `Fase`: en cuál de sus seis estados está una pantalla (cargando, vacía, error, sin conexión, sin permiso, lista) |
| `auth`               | Sesión OIDC (código + PKCE por loopback en escritorio, esquema propio en móvil) y llavero del sistema. **UNA** sesión para las trece apps                                                                                                                                                                                                                                                                                                                                                                                                    |
| `buscar`             | `Plegar`, `Contiene` y `Rangos`: la regla ÚNICA de buscar sin acentos ni mayúsculas («reunion» encuentra «reunión»). Espejo de `plegar` de `@sytue/i18n` y de la función SQL `plegar(text)`; los tres leen `buscar/testdata/casos.json`                                                                                                                                                                                                                                                                                                      |
| `ficheros`           | Diálogo «Guardar» del sistema, carpeta de descargas y «enseñar en el gestor»                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `formula`            | Motor de fórmulas de Sytue Sheets: 451 funciones, atado por trazas al de la web                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `fuentes`            | Los cinco .ttf de Figtree, en crudo. Cada toolkit los convierte a lo suyo                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `htmltext`           | HTML a texto plano (previsualización de un correo, de una nota)                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `i18n`               | Catálogo de los 26 idiomas, generado desde la web                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `numfmt`             | El formato de número de una celda (presets es-ES y máscaras): el mismo texto en Calcus y en las descargas de hojas del servicio de Docs                                                                                                                                                                                                                                                                                                                                                                                                      |
| `offline`            | Registros por cuenta y espacio y caché HTTP compartida. En nativo cifra por defecto con una clave del almacén seguro del SO, migra el JSON anterior de forma reanudable y permite borrar al salir sin descifrar. No implementa por sí solo una cola de mutaciones ni E2E del backend.                                                                                                                                                                                                                                                        |
| `storage`            | Contrato puro de registros privados de aplicación: ámbito de persona/organización/producto, objeto, revisión, rol y ramas de conflicto. El registro se transporta dentro del cifrado Matrix; este paquete no implementa criptografía ni migra los datos de las apps.                                                                                                                                                                                                                                                                         |
| `matrix`             | Cliente mantenido de Matrix E2EE: dispositivos, verificación, recuperación, adjuntos y mensajes de Rooms. Las salas privadas de almacenamiento tienen dispositivos por producto y excluyen de sus claves al controlador y a cualquier otra persona.                                                                                                                                                                                                                                                                                          |
| `sistema`            | Idioma y tema del sistema operativo                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `externo`            | Bajar lo que está FUERA de la suite —la imagen de una nota de imagen, lo que alguien haya pegado como URL— con los topes que un navegador pone solo                                                                                                                                                                                                                                                                                                                                                                                          |
| `sistema/escritorio` | Lo del escritorio que Fyne no da: el número sobre el icono (`Insignia`), si alguien pinta la bandeja (`HayBandeja`), el aviso del sistema que al pulsarlo lleva a su destino (`Avisar`, hoy solo en Linux) y el atajo global de la suite (`Atajo`)                                                                                                                                                                                                                                                                                           |
| `tokens`             | Los colores de la hoja `branding/tokens/colors.css`, como `image/color.NRGBA`, `Crudo` (el token tal como lo escribe la hoja, sin el aplanado de los bordes sobre `surface`: para el borde que va sobre el cromo de tinta), y cómo se componen: `SobreSRGB` (un color translúcido sobre otro), `Aplanar` (una opacidad de grupo, `opacity-40`, contra un fondo) y `ConAlfa` (el `/60` de Tailwind)                                                                                                                                           |
| `webref`             | Leer un fichero de la web en un test, para comparar espejos                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

#### Go, interfaz nativa — `apps/fyne/ui`

Los widgets propios de Fyne (§13.6). Cada uno es el espejo de una pieza de
`packages/ui` o de algo que la web escribe igual en varias apps, con su medida
atada a producción por un guardián. **Antes de escribir un widget en el paquete de
una app, se busca aquí**; el contrato son las reglas del comentario de
paquete de `ui.go`. Al lado viven `tema` (tokens, escala, sombras; las caras de Figtree, con
`NegraCursiva` para `<b><i>`, que Fyne no engorda; `SinSombras`, el tema sin la
sombra de borde que Fyne pinta en un desplazamiento y la web no, y `SinBarras`,
además sin la barra de desplazamiento de las tiras que se deslizan de lado) e
`iconos` (los
83 de la web y las 18 marcas, como constantes tipadas).

| Familia         | Piezas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base            | `Caja` (el borde, que no es un trazo), `Aro` (anillo de foco; `Abierto()` para el control que no tapa su centro con una caja opaca), `Sombra`, `BordeSobreCromo` (un borde de la hoja compuesto sobre el cromo de tinta y no sobre `surface`: el filete de la barra y del lateral), `Texto`/`Medir`/`ColocarTexto`/`Truncar`, `Rotulo`, `Versal`, `Parrafo` (el `<p>` que se reparte en líneas y se recorta con `line-clamp`), los layouts `Fila`, `Columna` y `Pila`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| Controles       | `Boton` (cuatro variantes por cuatro tallas; `Hueco` es el `gap-*` entre glifo y rótulo cuando no es el de la talla), `BotonIcono`, `BotonFlotante` (el `MobileFab`: la acción principal en el teléfono, por debajo de 768), `Enlace` (el `<a>` con `hover:underline`, o el `<button>` con esa pinta), `Tecla` (el `<kbd>`), `Segmentado`, `Deslizador`, `MuestraColor` (la muestra redonda de la paleta de anotación de CertiPDF)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| Formulario      | `Entrada` (con la talla `CampoBusqueda` para un buscador y `Rotulo`, su nombre accesible), `AreaTexto`, `Selector`, `Eleccion` (casilla y radio), `TarjetaEleccion` (el `ChoiceCard`: una opción de un grupo de radios con su `Icono`), `GrupoCampo` (el `FieldGroup`: rótulo, control, ayuda y error), `Conmutador`, `CampoFecha`/`CampoHora`, `AyudaEnLinea`, `PistaCampo`, `EditorPlano`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Estructura      | `Armazon` (con `AlCajon` y `CerrarCajon` para el cajón del móvil), `BarraMovil` (la barra bajo la de la suite por debajo de 768), `Lateral` (con `SeccionLateral` —`AccionesAlPasar` esconde sus acciones hasta pasar el puntero—, `FilaLateral` —`Tachada` para una tarea hecha—, `RamaLateral` —con su `Pliegue`—, `AccionLateral` y `Plegador`), `BarraSuite` (se crea con `NuevaBarraSuite(t, s, iconos.<Producto>)`: lleva la marca de su app), `CabeceraApp` (su `Ancho` declara el alto cuando envuelve, como `SectionHeader`), `Pestanas`, `Migas` (con su `EstiloMigas`: cuerpo de letra, fondo al pasar, tinta del galón e `Iguales`, que pinta el segmento actual como los demás), `Pasos` (el `Stepper` de un asistente), `BarraDeLote`, `PaginaNoEncontrada` (la 404 nativa: un enlace a algo borrado o a una ruta que la app no conoce), `FranjaSoloLectura` (el `ReadOnlyBar`: la franja de solo lectura con su salida —«Editar» si el bloqueo es de la persona, «Pedir acceso de edición» si le falta el permiso—; en los editores la engancha `editor.EngancharPeticionEdicion`) |
| Datos           | `Tabla` (también con celdas que son objetos), `FilaLista`, `FilaDrive`, `Rejilla`, `Tarjeta`, `Avatar` (con `Foto`; `PresenciaDe` traduce un estado de presencia del Hub a su punto), `SelectorAvatar` (el `AvatarPicker`, con `AvatarPredefinido`: las iniciales hechas imagen para subirlas como foto), `Insignia`, `Contador`, `Medidor`, `Vacio`, `Esqueleto`, `Rueda`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Superposiciones | `Dialogo` (`Recolocar` lo vuelve a centrar cuando cambia su contenido; `PieAncho` estira el pie a todo el panel), `Confirmacion` (el `ConfirmDialog`: un `Dialogo` `sm` con el pie de cancelar y confirmar ya montado; `Ocupado` mientras trabaja y `SinConfirmar` mientras falte algo), `Velo`, `Menu` (con `Ancho` para el que la web fija; `ElementoMenu.MantieneAbierto` para el ítem que no cierra el menú y `PuntoColor` para un punto de color que es un dato), `SelectorSuite` (el `SuitePicker`: elegir un fichero de Drive, un documento, un evento, una sala o una nota como referencia), `MostrarPopover` (el `Popover` sobre un `Emergente`, que tiene `Recolocar` y `Rotulo`), `Aviso` y `PilaAvisos`, `Pista`                                                                                                                                                                                                                                                                                                                                                                      |

**Para los tests, `ui/recorrido`**: `recorrido.Comprobar(t, w, tope)` pulsa todo lo que se ve en una
ventana de pruebas —y lo que va apareciendo—, cierra con Escape cada capa que se abre y marca como error
los pánicos y las capas que no se cierran. Cada app tiene su `TestRecorridoConSesion` contra el servicio
falso de sus propios tests; una app nueva lleva el suyo.

**Y lo que se queda en su app a propósito**, porque la web lo pinta distinto
(§«Lo que NO se comparte», abajo): la fila de conversación de Mail no es
`FilaLista`; el carril del ERP mide 210 y el de la suite 260; la fila del menú de
Rooms mide 35 y la del resto 33,25; la muestra de color del editor (cuadrada) y la
del avatar (con el borde por dentro) no son `MuestraColor`; la píldora de filtro
del ERP (`text-xs`, 21 de alto, `bg-selected`) no es la del Hub (`text-sm`, con
borde y `bg-primary`); el conmutador de vista de Calendar (`text-sm` semi-negrita,
`h-7`) no es el `Segmentado` del menú de cuenta (`text-xs`, 21 de alto), ni lo es
el de lista y cuadrícula de Drive, que lleva iconos y `divide-x`; y la pestaña de
la cinta del editor mide 31 y no va en versalitas, así que no es `Pestana`; y
«Adjuntar desde Drive» de Mail no es el `SelectorSuite` en modo Drive, porque en
la web tampoco (`DrivePickerModal.tsx` frente a `SuitePicker.tsx`): lleva una
nota al pie, un error de sesión con «Volver a cargar» y las carpetas en
`font-medium`. Cada una lo explica en su comentario.

Y en `apps/fyne/sesion`, además de la sesión, la apariencia, el idioma y la
presencia de las trece apps: `Residente` (la app se queda en la bandeja del
sistema y cerrar la ventana la esconde en vez de salir), `Abierta` (si otra app de
la suite está abierta en esta sesión de usuario) y `AlNoEncontrar`/`NoEncontrado`
(quién enseña la `PaginaNoEncontrada` de cada app y cómo pedírsela), y
`Suite.Conexion(producto, privadas...)`, el cliente HTTP de una app con la caché
«red primero» de la suite y el monitor que sabe si hay red (`privadas`, las rutas
que no se guardan). En `apps/fyne/iconos`, `Dibujo.Apagar` es el `opacity-40
grayscale` de la tesela de un producto que la cuenta no puede usar. Y en `apps/fyne/editor`, lo que comparten los
tres editores: `AbrirArchivo` («Nuevo» y «Abrir un archivo» del menú Archivo,
que cada producto engancha con `Contexto.Nuevo` y `Contexto.Abrir`; su `Importa`
son las extensiones que ninguna app reclama y que la app importa igual, como
el `accept` de su portada web) y
`Autoguardado` (el `saveTimerRef` de la web: agrupa los cambios y guarda UNA vez
cuando dejan de llegar) `Servicio` (descargar, guardar una versión, el historial
y restaurar, con los avisos de la web) y `ElegirDelSistema` (el selector de
ficheros del sistema, el de `<input type="file">`).

#### Go — `platform/pkg/`

| Paquete         | Para qué                                                                                                                                                                                                                    |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `asistente`     | Llamar a Sytue AI (`/api/v1/assist`) desde un servicio, con el token de la persona, y recortar el JSON de la respuesta                                                                                                      |
| `auditlog`      | Lectura admin del log de compliance: filtro, keyset, cursor y CSV                                                                                                                                                           |
| `authn`         | Validar el JWT de Sytue ID (JWKS con caché y rotación)                                                                                                                                                                      |
| `cache`         | Cliente de la caché/pub-sub efímera                                                                                                                                                                                         |
| `callaccess`    | Cercas de acceso a llamadas de documentos: cierre síncrono en Rooms antes del commit de ACL, versión monotónica y vaciado compartido desde Docs e ID                                                                        |
| `clave`         | Contraseñas de recurso (enlaces públicos de Drive y Docs): hash Argon2id PHC y verificación en tiempo constante                                                                                                             |
| `cms`           | Firmar y verificar CMS SignedData desprendido (RFC 5652)                                                                                                                                                                    |
| `commerce`      | Contratos comerciales compartidos entre servicios                                                                                                                                                                           |
| `config`        | Configuración tipada desde el entorno                                                                                                                                                                                       |
| `correo`        | Mandar un correo de texto por el MSA de Sytue Mail (STARTTLS obligatorio con credenciales)                                                                                                                                  |
| `events`        | Bus de eventos sobre PostgreSQL                                                                                                                                                                                             |
| `guiones`       | Reglas de los scripts de usuario que no dependen del producto (ADR 0069): cotas del código, cuotas FIJAS por hora, estados de una versión, quién ejecuta cuál y el hash que avala una aprobación. Docs y Bases las importan |
| `httpx`         | Errores de API (`Error`/`Fail`/`JSON`), cookies y CSRF                                                                                                                                                                      |
| `ingressroutes` | Contrasta las rutas que un servicio monta con las que declara                                                                                                                                                               |
| `keycrypto`     | Cifrado de material sensible (claves privadas, secretos)                                                                                                                                                                    |
| `log`           | Loggers estructurados con `log/slog`                                                                                                                                                                                        |
| `logofit`       | Encajar el logo que sube una organización                                                                                                                                                                                   |
| `migcheck`      | Gate expand → contract de las migraciones                                                                                                                                                                                   |
| `miniatura`     | Miniaturas de imagen ráster                                                                                                                                                                                                 |
| `objectstore`   | Clientes S3, Google Drive, OneDrive, Dropbox y WebDAV; contrato de blobs y barrera SSRF compartidos                                                                                                                         |
| `pdfdoc`        | PDF de texto con fuentes incrustadas y maquetación por métricas reales; base del PDF etiquetado (PDF/UA)                                                                                                                    |
| `pdffuente`     | Leer TrueType y escribir su subconjunto incrustado (CIDFontType2, /W, /ToUnicode); Figtree y Liberation Sans OFL                                                                                                            |
| `pdfsimple`     | PDF de texto con solo la biblioteca estándar                                                                                                                                                                                |
| `personas`      | Quién es «@ana» en Sytue ID (`people:resolve`, ADR 0040), con el token de quien pregunta (Rooms y Cast); y `NombresOrgs`, el nombre visible de una organización de fuera (Hub, Rooms, Calendar)                             |
| `pg`            | PostgreSQL sobre pgx: pool, `InTenantTx`, RLS por tenant                                                                                                                                                                    |
| `pgtest`        | Harness de tests de integración contra Postgres real                                                                                                                                                                        |
| `portabilidad`  | «Exportar tus datos»: la entrega que escribe cada producto y junta Hub (tablas en JSON, CSV o XML)                                                                                                                          |
| `qr`            | Código QR en PNG (nivel M, sin dependencias): el enrolamiento TOTP de Sytue ID y el QR tributario de VERI*FACTU                                                                                                             |
| `sqlorg`        | La organización va EN el SQL: guardián que recorre los literales SQL de un servicio y falla si una sentencia toca una tabla con columna de organización sin filtrarla                                                       |
| `ssrfhttp`      | Barrera SSRF para peticiones salientes                                                                                                                                                                                      |
| `telemetry`     | Trazas, métricas y logs correlacionados                                                                                                                                                                                     |
| `tenancy`       | El contexto de tenant que atraviesa todo                                                                                                                                                                                    |
| `validation`    | Validación y saneado de entrada, y `CSVSafe`                                                                                                                                                                                |

#### Frontend — `apps/frontend/packages/`

| Paquete                  | Para qué                                                                                                                                                                                                                                                                                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ui`                     | **Toda** la interfaz. Nada de UI fuera de aquí (§13). La franja de solo lectura de cualquier app es `ReadOnlyBar`, con su botón de salida                                                                                                                                                                                                                                 |
| `shell`                  | La barra, el conmutador de apps, notificaciones, búsqueda global                                                                                                                                                                                                                                                                                                          |
| `auth`                   | BFF (`/server`) y navegador (`/client`): sesión, login, `getAccessToken`                                                                                                                                                                                                                                                                                                  |
| `api-clients`            | SDK tipado de los servicios Go                                                                                                                                                                                                                                                                                                                                            |
| `config`                 | Middleware de idioma y seguridad, tokens de marca, Tailwind, Astro                                                                                                                                                                                                                                                                                                        |
| `i18n`                   | Negociación de idioma, catálogos comunes y `plegar`/`contiene`/`rangos` (buscar sin acentos ni mayúsculas)                                                                                                                                                                                                                                                                |
| `ai`, `cloud`, `formula` | Asistente, PaaS y hoja de cálculo                                                                                                                                                                                                                                                                                                                                         |
| `offline`                | Almacén por cuenta, conectividad y service worker. `@sytue/offline/matrix` comparte el cliente Matrix y Rust/WASM; la app aporta sus rutas y sesiones mediante adaptador. `@sytue/offline/matrix/types` es un contrato sin SDK para los retornos SSO. No copiar este motor en otra app.                                                                                   |
| `escena`                 | La escena vectorial común (Docus, Diapos, Sytue Web): nodos, capas, conectores con anclajes, trazos a mano alzada, hit-testing, serialización versionada y SVG. Las formas, el trazado de conectores y el z-order/alinear/distribuir viven aquí, no en las apps                                                                                                           |
| `scripts`                | El runtime aislado de los scripts de usuario ([ADR 0069](decisions.md#0069)): iframe de origen opaco con su CSP, Worker que se mata por tiempo, protocolo de mensajes validado y el bucle del anfitrión que hace cumplir los límites. Cada producto (Docs, Bases) aporta su superficie `sytue.v1` como datos y su traductor de llamadas; el sandbox NO se copia en la app |

#### Lo que NO se comparte, y por qué importa distinguirlo

Duplicación no es lo mismo que **variación deliberada**. Medido también aquí:
`ValidName` existe en cuatro servicios con tres implementaciones, y las tres son
correctas — 128 bytes en Calendar/Contacts, 255 en Drive porque son nombres de
fichero, y 100 runas en Rooms, que valida **nombres de canal** y por eso no
rechaza `/` ni `..`. Unificarlas pediría un parámetro de longitud y un flag de
«seguro para rutas»: dos copias de dieciséis líneas son mejor abstracción que
eso.

**Cómo decidirlo:** si las diferencias son de _dominio_ (qué significa un nombre
aquí), quédense separadas. Si son de _accidente_ (un comentario reflowado, un
mensaje traducido, un número que alguien tocó en un sitio), es duplicación.

#### Los guardianes que lo sostienen

Esto no se mantiene con buena voluntad, porque la siguiente app se creará igual:
copiando la anterior. Hay tests que fallan cuando alguien vuelve a copiar:

- `packages/config/src/plomeria-compartida.test.ts` — busca en el CÓDIGO FUENTE
  los marcadores de la plomería copiada (el `negotiateLocale` del middleware, el
  `'/auth/token'` del getter, el montaje del BFF, el proxy del asistente).
- `packages/auth/src/server/rutas-auth-reexportan.test.ts` — exige que toda ruta
  de `/auth/*` sea un reexport de `_rutas.ts`.

Los dos comparan **por igualdad** con su lista de excepciones, no por
pertenencia: así fallan TAMBIÉN cuando una excepción se deduplica y nadie borra
su entrada. Una lista blanca que nadie poda deja de proteger en dos refactores.

> Si uno de estos tests te para, lo normal es que ya exista una función que hace
> eso. Añadir tu fichero a `excepciones` es la última opción, no la primera, y
> pide escribir **por qué** el comportamiento es de verdad distinto.

<a id="13-3-eficiencia"></a>

### 13.3 Eficiencia: las reglas que salieron de medir — NORMATIVO

No son principios generales: cada una viene de un fallo concreto de este repo.

**1. Una petición, una transacción.** Cada método del store abre la suya
(`InTenantTx`), así que encadenar cuatro llamadas son cuatro `BEGIN`/`COMMIT`.
Resolver una ruta de WebDAV llegó a costar **seis transacciones**; hoy son dos.
Si un método de servicio llama al store más de dos veces seguidas, la pregunta
es si el store debería ofrecer una sola operación que las haga juntas.

**2. Recorrer en SQL, no en Go.** Caminar una ruta tramo a tramo era una consulta
por segmento, y cada una se traía TODAS las subcarpetas del padre para quedarse
con una. Un `WITH RECURSIVE` lo deja en coste fijo, **sin crecer con la
profundidad**. Antes de escribir un bucle que consulta, mirar si Postgres puede
hacer el recorrido entero.

**3. Un INSERT, no N.** Añadir una selección de doscientas fotos a un álbum eran
doscientas sentencias, y cada una llevaba dentro un `max(position)` sobre una
tabla que crecía con cada inserción. `unnest(...) WITH ORDINALITY` + una
subconsulta escalar (que Postgres resuelve una vez, como InitPlan) lo deja en
una.

**4. Leer antes de escribir en todo `Ensure*`.** Un `INSERT ... ON CONFLICT DO
UPDATE` **escribe aunque no cambie ningún valor**: tupla muerta, WAL y un
bloqueo sobre la única fila del tenant, por la que pasan todas sus peticiones a
la vez. Si el preámbulo de cada petición pasa por ahí, se serializa el servicio
entero. La señal para comprobarlo en un test es `xmin`: si Postgres reescribe la
fila, cambia.

**5. Pausar los sondeos con la pestaña de fondo.** `useSondeo` de `@sytue/ui`
hace justo eso y consulta de inmediato al volver. Un `setInterval` pelado gasta
batería en el cliente y peticiones en el servidor para enseñárselo a nadie.

**6. Buscar texto: `plegar()` y, con volumen, una columna ya plegada.** Toda
búsqueda de texto va sin acentos ni mayúsculas con la misma regla: `plegar(text)`
en SQL, `buscar.Plegar`/`Contiene`/`Rangos` en Go y `plegar`/`contiene`/`rangos` de
`@sytue/i18n` en la web (lo vigilan dos guardianes). Y un hallazgo medido: con
RLS forzada, **Postgres no usa índices de trigramas ni FTS para el rol de
aplicación**, porque `LIKE`, `ILIKE` y `@@` no son LEAKPROOF y no pueden ir por
delante de la política; lo que acota es el tenant. En 200 000 mensajes: `body
ILIKE` 205 ms, `plegar(body) LIKE` 440 ms, columna plegada guardada 60 ms. Por eso
Rooms guarda `body_plegado` y Mail su tsvector plegado; una tabla grande nueva
con búsqueda de texto hace lo mismo.

**7. Y la que manda sobre todas: medir antes de tocar.** Dos veces en un solo
día el cuello no estaba donde se suponía. Se buscaban 700 ms de hilo principal y
resultó que **nada iba comprimido** —el middleware existía, desplegado y sin que
lo referenciara ni uno de los 49 ingresses—. Y se iba a poner una pausa de
visibilidad a un `setInterval` que **no llamaba nadie**. Definir algo no lo
aplica, y optimizar lo que no se ejecuta es trabajo perdido con aspecto de
progreso.

<a id="13-4-pruebas"></a>

### 13.4 Qué prueba se deja detrás — NORMATIVO

Lógica no trivial deja **una** comprobación ejecutable, la más pequeña que falle
si la lógica se rompe. Con dos condiciones que en este repo han costado caro:

- **Probar el SQL contra Postgres de verdad** (`-tags=integration`, `pgtest`).
  Un doble en memoria solo demuestra que el doble se parece a lo que uno _cree_
  que hace el `WITH RECURSIVE`.
- **Verificar que el guardián falla.** Un test que no se ha visto en rojo no
  protege nada: se comprueba invirtiendo a mano lo que vigila. Los de este repo
  se escribieron así, y uno cazó una regresión de posiciones (`[0 1 3 4]`) que
  no se ve en pantalla porque el orden sigue saliendo bien.

Y cuidado con el **falso verde**, que aquí ha pasado varias veces:
`grep -c "^FAIL"` da 0 tanto si todo pasó como si el comando ni se ejecutó;
`go test -run` sin coincidencias dice `ok`; y `printf … | grep -q` con
`set -o pipefail` convierte _encontrarlo_ en _no encontrarlo_ por el SIGPIPE.
Contar siempre también los que pasaron.

### 13.5 Paridad web → apps nativas — NORMATIVO (2026-09-17)

**Toda función nueva de las web apps se añade también a las apps nativas de
escritorio y móvil en la MISMA ola, o queda registrada como deuda
planificada.** No es un deseo: se gobierna con un registro y un test.

- **Registro**: [`apps/frontend/packages/config/src/paridad.ts`](../apps/frontend/packages/config/src/paridad.ts).
  El PR que introduce (o toca) una función de web la AÑADE o la ACTUALIZA ahí,
  con su estado en `escritorio` y `movil`.
- **Gate**: `paridad.test.ts` falla si algo de web queda sin estado en las dos
  superficies, si 'planificado' va sin `deuda` apuntando a una sección que
  EXISTE en roadmap, o si se usa 'no' para algo que la web tiene.
- **Y el registro no es la única red**: este de aquí se escribe a mano y por eso
  se queda corto —declara 31 funciones para 80 pantallas de web—. Lo que la web
  tiene de verdad se deriva de los ficheros en
  [§13.7](#13-7-el-gate-de-paridad).
- **Compilación y pruebas**: ci-desktop y ci-mobile corren AL CREAR UNA
  RELEASE (decisión 2026-09-17: las puertas de las nativas validan lo que se
  publica, no cada push). El gate de paridad en cambio corre en cada PR,
  dentro de ci-frontend.
- **Única excepción de estética**: la topBar global, que solo existe en la
  web; el cromo nativo la sustituye (registro `shell-topbar`).
- **Dónde vive el código nativo**: `apps/fyne` (interfaz) sobre `apps/nucleo`
  (todo lo que no es del toolkit) es la MISMA base de las builds de escritorio y
  de las móviles — una mejora, dos superficies. Ver §13.6.

La deuda del registro se revisa en cada cierre de ola; lo 'planificado' que
llega a su fase de roadmap se sube a 'si' en el mismo PR que lo implementa.

<a id="13-6-la-interfaz-nativa-es-fyne-con-todo-propio--normativo-2026-09-21"></a>

### 13.6 La interfaz nativa es Fyne, con TODO propio — NORMATIVO (2026-09-21)

**Escritorio y móvil se construyen con [Fyne](https://fyne.io), en la versión fijada en §3, y widgets,
tema y layouts PROPIOS, hasta paridad 1:1 a nivel de píxel con las web apps.**
La web no entra: `sytue.eu` sigue siendo Astro + Preact + Go (§4), y por tanto
**no hay destino WebAssembly**. El porqué, lo medido y el techo real están en el
[ADR 0048](decisions.md#0048); esta sección es lo normativo.

**No hay límite en cuánto código propio.** Si el tema de Fyne estorba, se
sustituye; si un `Layout` de serie no da la geometría exacta, se escribe uno; si
hace falta cargar la fuente a mano, se carga. Entra en la paridad **todo**:
geometría, tipografía, color, iconos, espaciados, estados, foco, animaciones y
los diálogos.

**Dos excepciones, y solo dos.** No son dejadez y no hay que volver a
preguntarlas:

1. **La `TopBar` SÍ va en Fyne, recortada.** Corregido por el dueño el
   2026-09-22; hasta entonces esta sección decía que no se replicaba, y es lo
   contrario.

   Cada app nativa lleva **arriba a la derecha** las **tres piezas
   transversales**: el **buscador** de toda la suite, las **notificaciones** y
   el **menú de cuenta** (el avatar). No es cromo de app: es la suite, y por eso
   son las mismas en las trece.

   **Lo que NO lleva** es el **selector de aplicaciones** —en nativo se cambia de
   app en el sistema, no dentro de una ventana— y por eso la barra nativa es
   **más fina** que la de la web.

   **Y es CLARA, con la esquina de marca del color del carril** (dueño,
   2026-10-04: las nativas van igual que la web). Con la forma Carril el color de
   marca vive en el menú lateral, así que la barra pinta el `topbar-bg` de la web
   y solo su **esquina de marca** —la marca de la app y su nombre— lleva el
   `sidebar-bg` del carril, con el ancho del menú desde 768 px: es el `rail` de
   `TopBar.tsx`. El filete de la barra se para en la esquina, para que el carril
   se lea de arriba abajo sin una raya que lo parta. La pieza es `ui.BarraSuite`
   (tokens `fondoBarra` y `fondoEsquina`) y los guardianes,
   `TestLaEsquinaDeMarcaContinuaElCarril` y `TestElFileteDeLaBarraNoCruzaElCarril`,
   que comparan PÍXELES de barra, esquina y lateral pintados juntos. Sustituye a
   la regla del 2026-09-23, que pintaba la barra entera del color del lateral
   cuando los dos eran el mismo cromo. **El punto rojo de la campana** sigue en
   `danger-text`, que contra el anillo del color de la barra pasa de 3:1 en los
   tres modos (guardián `TestElPuntoDeAvisoContrastaConLaBarra`).

   **Dentro del carril, cada pieza pide su token por el nombre.** En la web todo
   lo que cuelga de `bg-sidebar` recibe reasignada la familia del cromo
   (`appearance.css`); Fyne no tiene cascada, así que una pieza que vaya dentro
   del menú lateral pide `sidebar-focus`, `sidebar-indicator`, `sidebar-border`,
   `sidebar-hover-bg` o `sidebar-primary`/`sidebar-on-primary`, y no los
   `chrome-*`, `indicator` o `primary`, que son los de la barra clara y del
   contenido. Los iconos del menú llevan el relleno ámbar del carril: lo ponen
   solos `iconos.Trazo` con un token `sidebar-*` y `DeToken` en los glifos
   propios de una app (`Dibujo.EnCarril` a mano).

   **La forma es un eje también en nativo, y «Marco» es la que se ve sin elegir**
   (dueño, 2026-10-05: de golpe y en todas las apps; lo de la web está en
   [design.md](design.md#forma-marco)). Lo anterior describe **Carril**, que es la
   forma que se elige. Cuatro reglas:
   - **Los números vienen de la hoja, nunca de una constante.** `apps/nucleo/tokens`
     embebe una copia de `colors.css`, de `appearance.css` y de `theme.css` (las
     copia `scripts/sync-branding.sh`; `TestLaCopiaDeLaHojaEsLaDeBranding` falla si
     divergen) y resuelve la cascada de modo × color × app × forma como el
     navegador. Un color se pide con `tema.Color(token, v)` y una medida de forma
     con `tema.Forma("radius-control")`, `"sheet-gap"`, `"nav-pad"`, `"row-inset"`…
     (la tabla está en `theme.css`, «Tokens de forma»). Los títulos que en la web
     son `sytue-h1`–`h4` **dentro de la hoja** se piden con `ui.EscalonTitulo(n)` y
     `ui.PesoTitulo(n)`; el de un diálogo no, que cuelga fuera de ella.
   - **La hoja se pinta UNA vez, en `ui/marco.go`.** Fyne no recorta a esquinas
     redondeadas: el marco es el color continuo detrás, el papel de la hoja, el
     contenido rectangular encima, cuatro máscaras de esquina del color del marco
     sobre él y la sombra `md` en ocho teselas. Lo coloca `ui.Armazon` (la hoja
     empieza donde acaba el menú, que deja ver el marco) y, para lo que no trae
     armazón —el Hub, SyPhotos, Cast, los cuatro editores (Docus, Calcus, Diapos,
     CertiPDF), «Iniciar sesión»—, `BarraSuite.Montar`, todos con el mismo `sheet-gap`
     (10 px desde 768 px, 0 px por debajo de 768 px) para que toda la suite se vea
     uniforme entre web y Fyne. El ERP lleva hoja como los demás: su carril
     de iconos va en `Armazon.Carril` —la hoja empieza donde él acaba— y con él el menú
     de módulos es un cajón a cualquier ancho, que conserva su fondo. Por debajo de 768
     va siempre a sangre. Ninguna app pinta esquinas ni sombra de hoja por su cuenta.
   - **Lo que va sobre el menú pregunta qué tiene detrás** con `ui.TokenCarril()` o
     `ui.FondoCarril(v)` (`sidebar-bg` en Carril, `frame-from` en Marco), no con
     `sidebar-bg` a mano. En Marco la barra entera es cromo, así que el punto de la
     campana pasa a `danger` y, como el avatar de la cuenta, lleva el aro del texto
     del carril (`sidebar-fg`, 1,5 px: el `sytue-aro-carril` de la web; 3:1 o más en
     los tres colores y los tres modos, `TestElPuntoDeAvisoLlevaAroSobreLaBarraDeColor`)
     y, con el color «el de cada app», la tesela de marca va sobre placa blanca.
   - **Los guardianes del driver corren en Carril** (`tema.FormaDeLosGuardianes()` en
     el `TestMain` de cada paquete): sus cientos de medidas se tomaron así. Lo propio
     de Marco lleva tests que la eligen (`enMarco(t)` en `ui/marco_test.go`), y
     `SYTUE_FORMA=marco` saca en Marco las capturas de cualquier test. En ventana
     real, `scripts/ventana-real.sh --forma carril|marco --color …`.
   - **La ventana real NO escribe en producción.** El recorrido pulsa en coordenadas
     fijas con la cuenta de auditoría, y una coordenada desplazada acaba en cualquier
     botón: el 2026-10-05 creó una nota, y los tres editores dejaban un documento de
     plantilla en cada tirada. El guion lanza las apps con `SYTUE_SOLO_LECTURA=1`, que
     corta antes de la red toda petición que no sea leer (`sesion/conexion.go`; solo
     pasa el canje del token). Un paso que necesite escribir se marca con `!` y solo
     corre con `--escribe`, que deja dicho lo que hay que borrar después.
   - **El aro de foco se mide en la ventana, no solo en los tests.** Los dos pintores
     de Fyne no trazan igual: el de software pinta el `StrokeWidth` de un rectángulo a
     caballo del contorno y el de GL por dentro de la caja. El aro abierto de `ui.Aro`
     fue un trazo y salía bien en los tests y un píxel más adentro en la ventana real,
     a ×1 y a ×2; ahora son cuatro franjas pintadas píxel a píxel, que los dos tratan
     igual. `scripts/ventana-real.sh --aros` abre el muestrario (`pantalla.Aros`: un
     control de cada familia con el foco encendido) a ×1 y a ×2, en claro y en oscuro,
     y falla si un aro no son 2 px del token pegados a la caja y a 3:1; la misma
     medida corre con el pintor de software en `TestLosArosDeFocoSonLosDeLaWeb`. Lo
     que se dibuje con un trazo no lo protege un test: o va relleno o va a la ventana.
     Dos cosas medidas ese día que se quedan como están, para que nadie las
     redescubra. **Los aros RELLENOS** (botones sólidos, campos, casillas) están en su
     sitio, pero a ×1 salen al 84 % en la ventana por el suavizado de borde del pintor
     de GL: 3,70:1 el peor (el botón principal, en claro) y 100 % a ×2. Pasan de 3:1 y
     cambiarlos es redibujar más de cien controles: no compensa (decidido ese día).
     **Los demás trazos** —el borde translúcido de `Caja.ConTrazo`, el marco, el anillo
     y la casilla de la rejilla de Bases, las marcas de CertiPDF, la muestra de tabla
     de Calcus— los pinta el de GL por dentro de la caja, medio grosor más adentro que
     el de los tests, a ×1 y a ×2, y eso no produce nada que se vea: ni líneas dobles,
     ni huecos, ni bordes comidos (medido con las primitivas de cada sitio y, las
     tarjetas, en el Hub real). En la ventana un borde de 1 px cae donde lo pone un
     `border` de CSS; el que lo deja a caballo es el pintor de los tests. El marco de
     la celda de Calcus y los separadores son rectángulos rellenos y salen idénticos.

   La preferencia va en `ajustes.json` (`forma`, junto a `tema` y `color`) y se elige
   en el menú de la cuenta; quien no ha elegido ve la de su organización, que llega
   en el id_token (`org_forma`, `org_color`) y guarda la sesión
   (`auth.Session.Apariencia`): persona → organización → casa. La primera opción de
   los dos selectores es «Organización», que borra la elección y vuelve a seguirla;
   una cuenta personal no la ve.

   **Y lo que se abre desde ahí, se abre en el SISTEMA.** Un resultado de
   búsqueda o una notificación de OTRA app no se pinta dentro de la actual:
   lanza esa app por su esquema de URL, igual que una tesela del Hub. Es la
   misma regla de la excepción 2 y la misma maquinaria.

   **Y vale igual para el CONTENIDO, no solo para la navegación** (dueño,
   2026-09-22): un documento **se abre en su app**, no dentro de la que lo
   listaba. Un `.docx` de Drive abre **Docus**; una hoja, **Calcus**; una
   presentación, **Diapos**; un PDF, **CertiPDF**; una imagen, **SyPhotos**. Lo
   mismo para un adjunto de Sytue Mail o de Sytue Rooms.

   Consecuencia práctica, y es liberadora: **ninguna app necesita un visor de
   los formatos de las demás**. El `PreviewDialog` de Drive no se porta; lo que
   se porta es la acción de abrir. Y con el mismo mecanismo de siempre: se
   intenta el esquema `sytue-<producto>` y, si el sistema no tiene quién lo
   atienda, se cae a la web — un fichero que no hace nada al pulsarlo es peor
   que uno que abre el navegador.

   Lo que **no** cambia es la regla dura: **el documento no se corrompe nunca**.
   Abrir en otra app es pasarle una referencia, no una copia: quien guarda es la
   app dueña del formato, con su viaje de ida y vuelta demostrado.

   **CUATRO COSAS SON TRANSVERSALES** a todas las apps del sistema (dueño,
   2026-09-22). No son ajustes de una app: son de la persona, y por eso se
   eligen desde el menú de cuenta de cualquiera de las trece y valen en todas.
   **Pero no viven en el mismo sitio, y confundirlo es el error que hay que
   evitar:**

   |                | dónde vive               | por qué                                                                                                                          |
   | -------------- | ------------------------ | -------------------------------------------------------------------------------------------------------------------------------- |
   | **Sesión**     | el llavero del sistema   | es un secreto. **Se entra UNA vez**, no una por app                                                                              |
   | **Apariencia** | preferencias compartidas | no es un secreto, y el llavero pediría permiso al arrancar                                                                       |
   | **Idioma**     | preferencias compartidas | ídem                                                                                                                             |
   | **Presencia**  | **el servidor**          | **la ven los demás**; una copia local sería una segunda verdad que se queda vieja en cuanto se cambie desde la web o el teléfono |

   Y las cuatro comparten el mismo problema: **trece procesos y un cambio que
   todos tienen que ver**. Para la sesión no hay margen —cerrarla la cierra en
   todas, y en el momento—; para la apariencia y el idioma, aplicarlo en el
   siguiente arranque de las demás es aceptable **si se dice**. La presencia ya
   la empuja el servidor.

   **«Mi cuenta» es la excepción del menú**: no es una app nativa, abre la web
   de Sytue ID en el navegador. Se llegó a portar a Fyne y se retiró.

2. **Lo que no se copia es el SUBDOMINIO, no la separación.** Cada producto
   sigue siendo **su propia aplicación, también en escritorio y móvil**: un
   binario instalable por producto, no uno que los contenga a todos. Lo que no
   se replica es el despliegue web —subdominio, BFF, TLS—, porque ahí la
   paridad es de **pantallas y piezas**.

   **Y las apps se llaman entre sí.** El equivalente nativo de ir a otro
   subdominio es invocar a la otra aplicación por su esquema de URL: `.desktop`
   y xdg en Linux, `CFBundleURLTypes` en macOS, registro de protocolo en
   Windows, `intent-filter` en Android, universal links en iOS. Es una acción
   que delega en el sistema, **nunca una pantalla más del mismo binario**.

   Esta excepción estaba escrita al revés en la primera redacción de esta
   sección (2026-09-21): decía que en nativo los productos _no_ son
   aplicaciones distintas. Lo corrige el dueño el mismo día, antes de que
   nadie empaquetara nada.

**Dónde vive cada cosa:**

- **`apps/nucleo`** — lo que NO es del toolkit y usan todos los clientes:
  `ajustes`, `api`, `auth`, `ficheros`, `formula`, `htmltext`, `i18n`,
  `offline`, `sistema`, más los **tokens de color** (`tokens`, que interpreta
  `branding/tokens/colors.css`) y las **fuentes** (`fuentes`). El nombre no
  lleva el toolkit **a propósito**: se ha cambiado dos veces en un mes.
- **`apps/fyne/sesion`** — lo transversal de una app nativa en una línea:
  sesión, apariencia, idioma y presencia. No se monta otro ([ADR 0050](decisions.md#0050)).
- **`apps/fyne/tema`** — tokens, escala y el `fyne.Theme`, que existe para
  gobernar la LETRA y el fondo del lienzo, no para configurar widgets. La
  variante se pide con `tema.De`, que resuelve la elección de la persona: Fyne
  no deja forzarla (`app/settings.go:75-86`).
- **`apps/fyne/ui`** — los widgets propios. **Antes de escribir el segundo,
  leer el comentario de paquete de `ui.go` entero**: son seis reglas, cada una
  con la medición que la justifica, y saltárselas cuesta dieciséis apps.
- **`apps/gio`** — RETIRADO el 2026-09-23, con `services/gioweb` y el escritorio de
  `apps/desktop`. La copia vive en `gio-archivado/`, fuera de git.

**Hay UNA sola vía nativa: esta.** El 2026-09-21 se exploró durante unas horas
montar además Wails v3 —la web dentro de un WebView— como alternativa, y el
dueño lo canceló el mismo día para no partir el esfuerzo. No llegó código al
repositorio. Si vuelve a proponerse, que sea con la pregunta contestada de qué
resuelve que Fyne no.

**Y ningún gate lleva el nombre de un toolkit dentro.** El registro de paridad
habla de `escritorio` y `movil` a propósito: el toolkit ha cambiado dos veces
en un mes y los gates sobrevivieron porque no lo nombraban.

**Las cuatro reglas que más se incumplen** (las seis completas, en `ui.go`):

1. **Solo primitivas dentro del renderer.** `canvas.Rectangle`, `canvas.Text`,
   `canvas.Image` y otros widgets de `ui`. Nunca `widget.Button` ni
   `container.NewVBox`.
2. **Una sola cuenta de la medida.** Cada widget expone `MedidaX(...)`; su
   `MinSize()` la llama a ella y el guardián compara contra ella Y contra los
   píxeles pintados.
3. **Los decimales de la web se escriben tal cual.** No se redondean: Fyne
   cuantiza al píxel de DISPOSITIVO, y redondear en nuestro código congela a ×1
   un error que una pantalla HiDPI arregla sola.
4. **El borde nunca es `StrokeWidth`** (Fyne lo pinta centrado y sale a media
   fuerza): es `ui.Caja`.

**Qué prueba deja un widget nuevo.** Una que compare la medida DECLARADA, la que
pide a Fyne y la PINTADA, contra el número medido en producción —no contra lo
que calcule el propio widget—. Corre sin CGO y sin servidor gráfico: el pintor de
software de Fyne es Go puro, así que va en el pre-push. El ejemplo a copiar es
`apps/fyne/ui/boton_test.go`.

**Y por eso NADA se pinta con `canvas.Shader`.** El tipo existe en Fyne v2.8.1 y
es tentador —un fragment shader GLSL con sus uniformes—, pero **el pintor de
software no lo pinta**: no falla, no hace nada. Su `switch` no tiene caso para
`*canvas.Shader` (medido el 2026-09-21: un shader que devuelve rojo opaco deja
**0 píxeles rojos de 10 000**). Un widget que dependa de un shader es invisible
allí donde no haya GPU, empezando por estos guardianes. Lo que necesite un efecto
se precalcula en un `image.Image` y se pinta con `canvas.Image`, que los dos
pintores tratan igual; el ejemplo montado es `apps/fyne/ui/sombra.go`.

**Cuidado también con `canvas.Rectangle.Shadow`**, que sí pintan los dos: su
`BlurRadius` significa cosas distintas en cada uno —gaussiana con σ =
√(2·BlurRadius/escala) en software, `smoothstep` en GL—, así que la sombra
**cambia de tamaño con el DPI** (σ de 7,00 px a ×1, 5,00 a ×2 y 4,00 a ×3 con el
mismo valor). No sirve para paridad a píxel.

<a id="13-7-el-gate-de-paridad"></a>

### 13.7 El gate de paridad: lo derivado manda sobre lo declarado — NORMATIVO (2026-09-21)

**Lo que la web tiene no se declara: se DERIVA de los ficheros.** Un registro
que se actualiza a mano se queda viejo en silencio, y este ya lo hizo: §13.5
gobierna 31 funciones mientras la web tiene **80 pantallas** (`.astro` bajo
`apps/frontend/apps/*/src/pages/`, sin el sitio público) y **46 piezas**
(`apps/frontend/packages/ui/src/components/*.tsx`, sin la TopBar, que se cuenta
aparte por lo que dice §13.6). Ninguna entrada era falsa; el registro
simplemente dejó de mirar.

- **Gate**: `apps/frontend/packages/config/src/paridad-pantallas.test.ts`. Corre
  en `bun run test` (o sea, en ci-frontend) y en el pre-push, también cuando el
  push solo toca lo nativo. Doce milisegundos: solo lee ficheros.
- **Qué deriva**: el inventario de la web. Una pantalla nueva aparece sola, sin
  que nadie la anote.
- **Qué se declara**, porque no se puede leer de un árbol de ficheros: la foto
  de la deuda y las excepciones.

**Cómo se dice que algo YA está portado.** El fichero nativo lo declara él
mismo, con un marcador en el código:

```go
// paridad-web: apps/frontend/apps/mail/src/pages/index.astro
```

Vale desde cualquier superficie nativa de `apps/` (menos `apps/frontend`, que es
el origen) y **no lleva el nombre de ningún toolkit a propósito**: ha cambiado dos
veces en un mes. El
registro habla de `escritorio` y `movil`, y así se queda.

**Cómo se salda deuda** — los dos pasos van juntos, en el mismo PR:

1. Se porta la pantalla y se le pone el marcador en el fichero nativo.
2. **Se borra su línea de `DEUDA_SIN_NATIVO`.** Si no se borra, el gate se pone
   rojo diciendo «YA NO ESTÁ PENDIENTE: …». No es una molestia: saldar deuda sin
   bajar la foto deja hueco libre para la siguiente sin que nadie se entere, que
   es como se llegó a 35 funciones de 35 en 'planificado'.

**Cómo crece la deuda cuando de verdad tiene que crecer.** Una pantalla nueva de
web que no se puede portar en esta ola se añade a mano a `DEUDA_SIN_NATIVO` y se
dice en el PR cuándo se cierra. Añadir esa línea es el acto que se ve en la
revisión; el gate existe para que no se pueda evitar.

**Cómo se añade una excepción legítima — y por qué casi nunca es lo que toca.**
`SOLO_WEB` es para lo que **NUNCA** va a existir en escritorio ni en móvil, no
para lo que todavía no está: eso es deuda. **Es la última opción, no la
primera**, y pide escribir el motivo en la propia entrada. Hoy son estas:

- `TopBar.tsx` — **ya NO es excepción entera** (§13.6, corregido el
  2026-09-22): sus tres piezas transversales —buscador, notificaciones y menú de
  cuenta— **sí van** en cada app de Fyne, arriba a la derecha. Lo único que no
  se porta es el **selector de aplicaciones**, porque en nativo se cambia de app
  en el sistema. Mientras el gate razone por fichero y no por pieza, la entrada
  se queda aquí con este motivo escrito.
- `apps/frontend/apps/www/` — el sitio público es SSG, sin sesión y sin backend;
  no es un producto de la suite, y una app de escritorio no lleva dentro su
  propia web comercial.
- `apps/frontend/apps/account/src/pages/index.astro` («Mi cuenta») — solo web por
  decisión del dueño (2026-09-22): el menú de cuenta nativo abre Sytue ID en el
  navegador.
- `apps/frontend/apps/cast/src/pages/embed/[id].astro` — el reproductor para el
  `<iframe>` de una web ajena; en nativo no hay iframe ajeno.
- `apps/frontend/apps/docus/src/pages/wopi/editor.astro` — la página que aloja el
  editor externo por WOPI; escritorio la abre en el navegador (dueño, 2026-10-01).
- `apps/frontend/apps/console/` y `apps/frontend/apps/cloud-dashboard/` — Sytue
  Admin y Sytue DevX son solo web **para siempre** (dueño, 2026-10-01): no
  tendrán app nativa, así que sus pantallas no son deuda.
- `apps/frontend/apps/pages/` — Sytue Web es solo web **por ahora** (dueño,
  2026-10-02): el catálogo lo tiene como `nativa: false` y sus pantallas y sus
  catorce funciones no cuentan como deuda. Si se porta algún día, la excepción
  se quita y vuelven a la deuda.

Y no se quedan de adorno: si lo que la excepción tapaba desaparece de la web, el
gate exige quitarla.

**Lo que el gate NO permite, aunque parezca cómodo.** Cada producto es su propia
aplicación también en escritorio y en móvil —lo que no se replica de la web es el
subdominio y el despliegue, no la separación—, así que **un fichero nativo no
puede portar pantallas de dos productos**. Los productos se llaman entre sí por
deep link. Es un test y no un comentario porque la forma cómoda de portar —una
lista que sirve a Mail y a Notepads, «que son casi iguales»— fusiona dos
productos sin que nadie lo decida y no deja rastro legible en ningún diff.

**El gate se ha visto en rojo en las cuatro direcciones** (pantalla nueva sin
portar, pantalla portada sin bajar la foto, marcador apuntando a una pantalla
renombrada, y un fichero nativo sirviendo a dos productos), y en las cuatro dice
el nombre del fichero: un gate que dice «falta algo» sin decir qué se ignora al
tercer día.

<a id="14-sytue-cloud--dx-y-servicios-gestionados-vercel--supabase--google-cloud"></a>

## 14. Sytue Cloud — DX y servicios gestionados (Vercel × Supabase × Google Cloud)

DX project-centric, más simple que Google Cloud, con inmediatez de Vercel. Deploy (conectar repo →
build Kaniko → preview por PR → promover → rollback; dominios+TLS automáticos). Servicios gestionados
FOSS/UE sobre PostgreSQL: **Sytue Postgres** (con API REST automática PostgREST, pooling, backups,
branching), **DB Studio** (estilo Supabase, diseño Sytue), **Storage** (S3), **Functions** (serverless),
**Cron**, **Secrets/Env**, **Auth gestionada** (vía Sytue ID, realm por proyecto). Detalle en `docs/architecture.md#3-sytue-cloud-paas`.

<a id="15-entornos-dev-local--base-scaleway-paridad-devprod"></a>

## 15. Entornos: dev local + base Scaleway (paridad dev/prod)

- **Dev = LOCAL:** Docker Compose en un comando (`make dev-up`), `*.sytue.localhost` con TLS mkcert
  (Traefik), Postgres + MinIO + LiveKit + **todos los servicios Go** + frontends. Seed incluido.
- **Prod = SCALEWAY:** Terraform (Kapsule, RDB Postgres, Object Storage, Registry, LB, DNS, Secret
  Manager, LiveKit) + Helm (umbrella `sytue-one`, solo servicios Go + LiveKit + Traefik). Solo UE.
- Misma configuración lógica parametrizada por entorno; cambian endpoints/escala, no la arquitectura.
- **PROBAR va a un efímero de dev en Scaleway o directamente contra producción, no al
  portátil** (requisito desde 2026-08-09; alcance corregido el 2026-08-10). Construir imágenes
  o levantar servicios en la máquina de quien trabaja tiene dos problemas: le come los 8 GB que
  necesita para trabajar, y **el resultado no se parece a producción** —arquitectura, red,
  versiones—, así que un verde local no demuestra gran cosa.
  Cuenta también como prueba **verificar un build**: la excepción de `go build`/`go vet`/
  unitarios/linters («no levantan nada») era demasiado ancha y acababa en compilar en bucle en
  el portátil para ir comprobando el trabajo. La vía por defecto no es montar un efímero sino
  **empujar y dejar que lo verifique el CI (GitHub Actions), que ya ES ese entorno remoto**; el
  efímero se levanta solo para lo que el CI no da (depurar en caliente, fallos de red o de
  arquitectura) y **se elimina al terminar**. En local queda lo que el hook de pre-push
  ejecuta solo al empujar, que es parte del flujo. Y no se deja nada encendido: si al empezar
  hay un dev local corriendo, se baja.

---

<a id="16-naming-dominios-y-dns-anexo-normativo"></a>

## 16. Naming, dominios y DNS (anexo normativo)

> Documento de referencia para el **sistema de marca**, la **tabla completa de
> dominios/subdominios**, los **puertos de los servicios Go**, el **plan DNS** de
> `sytue.eu` sobre **Scaleway DNS** (incluida la configuración de correo para
> **Sytue Mail**, servidor de correo Go propio) y las **convenciones de naming** de
> recursos cloud y _labels_ de Kubernetes.
>
> Fuente de verdad del contrato: [`docs/CONVENTIONS.md`](./CONVENTIONS.md) (v4). Cualquier
> nombre, dominio, subdominio o puerto aquí descrito **debe** coincidir con el contrato
> canónico. Soberanía: **100 % UE sobre Scaleway** (solo regiones UE). Sin proveedores US
> para datos personales.
>
> **Stack único y total (v4):** Backend = **Go** (un servicio por producto, módulo
> único `git.sytue.eu/sytue/one`); Frontend = **Astro + Preact**; datastore **único** =
> **PostgreSQL** (estado + colas + caché sobre Postgres — **sin Redis/Valkey**); blobs
> grandes en **Object Storage S3** (Scaleway prod / MinIO dev). **Ningún motor de terceros
> AGPL** salvo el transporte Matrix autorizado en §0.2 (Keycloak, Nextcloud, OnlyOffice,
> Mattermost y Stalwart quedan **fuera**). **LiveKit** transporta los medios de _Sytue Rooms_.

---

<a id="161-sistema-de-marca"></a>

### 16.1 Sistema de marca

#### 1.1 Jerarquía de marca

| Nivel       | Nombre                 | Qué es                                                                            |
| ----------- | ---------------------- | --------------------------------------------------------------------------------- |
| Marca padre | **Sytue**              | La empresa/marca matriz. Es el "apellido" de todo el ecosistema.                  |
| Suite       | **Sytue One**          | El paquete unificado de productos (estilo Google Workspace) y el portal/lanzador. |
| Productos   | **Sytue \<Producto\>** | Cada superficie de la suite (Sytue Mail, Sytue Drive, …).                         |

- **Esquema de nombres:** siempre `Sytue <Producto>` (un solo espacio, `<Producto>`
  capitalizado en singular y en **inglés** salvo nombres ya en español de producto).
  Ejemplos válidos: `Sytue Mail`, `Sytue Drive`, `Sytue Calendar`, `Sytue Cloud`.
- **El portal** se llama exactamente **Sytue One** (no "Sytue Portal").
- **El IdP** se llama **Sytue ID** (no "Sytue Identity" ni "Sytue Auth"): es la pieza
  **nuclear** de la suite y un **servicio Go propio** (OIDC/OAuth2/SCIM/MFA), no un motor
  de terceros.
- Nunca se usan minúsculas para la marca en texto de producto (`sytue` en minúscula solo
  aparece como identificador técnico: dominios, _slugs_, nombres de recurso, client OIDC, etc.).

#### 1.2 Mapa marca → identificador técnico

Cada producto tiene un nombre de marca (humano) y un **slug** estable en _kebab-case_
(en inglés) que se reutiliza en dominios, clients OIDC, nombres de recurso y labels. En v4
cada producto es un **servicio Go** en `services/<slug>/` (módulo único `git.sytue.eu/sytue/one`).

| Marca (humano)               | Slug técnico     | Uso del slug                                                                                                                     |
| ---------------------------- | ---------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Sytue One                    | `one` / `portal` | subdominio `one`; app frontend `portal`; client `sytue-one`                                                                      |
| Sytue ID                     | `id`             | subdominio `id`; servicio Go `services/id`; client base de la suite                                                              |
| Sytue Drive                  | `drive`          | subdominio `drive`; servicio Go `services/drive`; app `drive`; client `sytue-drive`                                              |
| Sytue Docs / Sheets / Slides | `docs`           | subdominio `docs`; servicio Go `services/docs` (+ colab WS); client `sytue-docs`                                                 |
| Sytue Mail                   | `mail`           | subdominio `mail` + `mx`; servicio Go `services/mail`; client `sytue-mail`                                                       |
| Sytue Calendar               | `calendar`       | subdominio `calendar`; servicio Go `services/calendar`; client `sytue-calendar`                                                  |
| Sytue Contacts               | `contacts`       | subdominio `contacts`; servicio Go `services/contacts`; client `sytue-contacts`                                                  |
| Sytue Rooms                  | `rooms`          | subdominio `rooms`; servicio Go `services/rooms` (mensajería + videollamada, + LiveKit SFU en `rooms-sfu`); client `sytue-rooms` |
| Sytue Cloud                  | `cloud`          | subdominio `cloud`; servicio Go `services/cloud`; app `cloud-dashboard`; client `sytue-cloud` (+ `sytue-cloud-api`)              |
| Sytue Admin                  | `console`        | subdominio `console`; app `console`; client `sytue-console`                                                                      |
| Sytue Hub                    | `hub`            | subdominio `hub`; servicio Go `services/hub`; client `sytue-hub`                                                                 |
| Sytue Mail — host de correo  | `mx`             | subdominio `mx` (MX/SMTP/IMAP/JMAP), servido por `services/mail`                                                                 |
| Sitio público                | `www`            | apex `sytue.eu` + `www`; app `www`                                                                                               |

> **Coherencia:** el slug es la pieza estable; de él se derivan dominio, client OIDC
> (`sytue-<slug>`), servicio Go (`services/<slug>`), nombre de recurso (`sytue-<env>-<slug>`)
> y la etiqueta `sytue.eu/product=<slug>`. Ver §4.
>
> **Sin slugs de motores:** ya **no** existen `office` (OnlyOffice Doc Server), ni _realms_
> ni _temas_ de Keycloak, ni federaciones de Nextcloud/Mattermost/Stalwart. _Sytue Docs_ es
> un servicio Go con colaboración propia (CRDT/OT sobre WebSocket), sin Document Server aparte.

#### 1.3 Dominio raíz y entornos

- **Dominio de producción:** `sytue.eu` (registrado y gestionado en **Scaleway DNS**, zona UE).
- **Dominio de desarrollo (local):** `*.sytue.localhost` (Docker Compose + Traefik 3 + mkcert).
  Mismo esquema de slugs que producción → **paridad dev/prod** (p. ej. `mail.sytue.localhost`
  en dev ⇄ `mail.sytue.eu` en prod).

---

<a id="162-tabla-completa-de-dominios-y-subdominios"></a>

### 16.2 Tabla completa de dominios y subdominios

Todos los subdominios cuelgan de `sytue.eu` y resuelven (directa o indirectamente) hacia el
_ingress_ de Scaleway (Load Balancer → Traefik 3 → cert-manager 1.16 para TLS 1.3). El
backend de cada superficie es un **servicio Go propio** (no un motor de terceros).

#### 2.1 Superficies de producto (suite Sytue One)

| Subdominio (prod)   | Producto / función                          | Frontend (UX)                      | Backend (servicio Go)                                                                     | TLS                                      |
| ------------------- | ------------------------------------------- | ---------------------------------- | ----------------------------------------------------------------------------------------- | ---------------------------------------- |
| `sytue.eu` (apex)   | Sitio público / marketing                   | Astro + Preact                     | —                                                                                         | cert-manager                             |
| `www.sytue.eu`      | Sitio público (alias del apex)              | Astro + Preact                     | —                                                                                         | cert-manager                             |
| `one.sytue.eu`      | **Sytue One** — portal/lanzador             | Astro + Preact                     | Sytue Hub (`services/hub`)                                                                | cert-manager                             |
| `id.sytue.eu`       | **Sytue ID** — identidad/SSO/MFA/SCIM       | Astro + Preact                     | `services/id` (OIDC/OAuth2/SCIM Go)                                                       | cert-manager                             |
| `drive.sytue.eu`    | **Sytue Drive** — archivos/sync             | Astro + Preact                     | `services/drive` (WebDAV + REST; PG + S3)                                                 | cert-manager                             |
| `docs.sytue.eu`     | **Sytue Docs/Sheets/Slides**                | Astro + Preact (editor propio)     | `services/docs` (modelo doc + colab CRDT/OT WS; PG + S3)                                  | cert-manager                             |
| `mail.sytue.eu`     | **Sytue Mail** — correo (HTTP/JMAP)         | Astro + Preact (JMAP)              | `services/mail` (SMTP/IMAP/JMAP Go; PG + S3)                                              | cert-manager                             |
| `calendar.sytue.eu` | **Sytue Calendar** — calendario             | Astro + Preact (CalDAV)            | `services/calendar` (CalDAV + REST; PG)                                                   | cert-manager                             |
| `contacts.sytue.eu` | **Sytue Contacts** — contactos              | Astro + Preact (CardDAV)           | `services/contacts` (CardDAV + REST; PG)                                                  | cert-manager                             |
| `notes.sytue.eu`    | **Sytue Notepads** — notas y cuadernos      | Astro + Preact                     | `services/notes` (REST propio; cifrado E2E en cliente; PG)                                | cert-manager                             |
| `rooms.sytue.eu`    | **Sytue Rooms** — mensajería y videollamada | Astro + Preact (+ LiveKit web SDK) | `services/rooms` (WS + REST + salas; PG). El SFU **LiveKit** vive en `rooms-sfu.sytue.eu` | cert-manager                             |
| `cloud.sytue.eu`    | **Sytue Cloud** — PaaS (dashboard)          | Astro + Preact                     | `services/cloud` (control plane Go + PostgREST; PG + S3)                                  | cert-manager                             |
| `admin.sytue.eu`    | **Sytue Admin** — admin/tenants             | Astro + Preact                     | admin API Go (sobre Sytue ID + Hub)                                                       | cert-manager                             |
| `hub.sytue.eu`      | **Sytue Hub** — integración (shell backend) | —                                  | `services/hub` (búsqueda/notif/presencia/intents; PG)                                     | cert-manager                             |
| `mx.sytue.eu`       | Host de correo (MX/SMTP/IMAP/JMAP)          | —                                  | `services/mail` (mismo binario que `mail.sytue.eu`)                                       | cert-manager (JMAP/web) + cert SMTP/IMAP |

> **Hosts retirados (2026-09-11).** `chat.sytue.eu` y `meet.sytue.eu` (301 a
> Rooms) y `console.sytue.eu` (301 a Admin) dejaron de existir: se retiraron su
> redirección, su certificado y su registro DNS. Sus `redirect_uris` salieron de
> Sytue ID en la migración 0041 y sus destinos de cierre en la 0059. Un 301 que
> nadie sigue es un nombre más que certificar y una superficie más que vigilar.

> **Canónica por host de producto (2026-09-26, ADR 0055).** La dirección que se
> anuncia (`PUBLIC_URL`, `redirect_uri` de OIDC) de cada superficie es la de su
> SUBDOMINIO de producto con su prefijo (`mail.sytue.eu/mail`, mig. id 0099). El
> host de organización `<slug>.sytue.eu` —hoy `corporate.sytue.eu`— sigue
> sirviendo el MISMO prefijo y su login redirige al host de producto; queda en
> transición hasta que la organización pase a ser segmento de PATH, y sobrevive
> como dominio de correo. Los `redirect_uri` de ambos hosts están registrados
> (0099 y 0054); el guardián `platform/pkg/migcheck` exige los dos.

#### 2.2 Infraestructura y dominios técnicos

| Subdominio (prod)       | Función                                                                                                                                                                             |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ingress.sytue.eu`      | Punto de entrada público (Load Balancer Scaleway → Traefik). Objetivo de los `CNAME` de dominios propios del PaaS.                                                                  |
| `registry.sytue.eu`     | Alias de Scaleway Container Registry (imágenes de los servicios Go + builds PaaS).                                                                                                  |
| `status.sytue.eu`       | Página de estado/incidencias (statuspage propia, Astro).                                                                                                                            |
| `mta-sts.sytue.eu`      | Endpoint HTTPS del _policy file_ MTA-STS de Sytue Mail (ver §3.4).                                                                                                                  |
| `autoconfig.sytue.eu`   | Autoconfiguración de clientes de correo (Thunderbird/genérico). Lo sirve `services/mail` (`internal/autoconfig`) a través del Ingress de `charts/mail` (`ingress.extraSubdomains`). |
| `autodiscover.sytue.eu` | Autodescubrimiento de correo (clientes Microsoft). Mismo backend y mismo Ingress que `autoconfig`.                                                                                  |
| `imap.sytue.eu`         | Nombre público de IMAPS que anuncia la autoconfiguración (`IMAP_PUBLIC_HOST`). Alias de `mx` (ver §3.4.7).                                                                          |
| `smtp.sytue.eu`         | Nombre público de _submission_ que anuncia la autoconfiguración (`SMTP_PUBLIC_HOST`). Alias de `mx` (ver §3.4.7).                                                                   |
| `livekit.sytue.eu`      | Señalización WebSocket de LiveKit (SFU de medios de Sytue Rooms).                                                                                                                   |

#### 2.3 Dominios de ejecución del PaaS (Sytue Cloud)

Las apps desplegadas por **clientes** en Sytue Cloud viven en zonas _wildcard_ separadas del
dominio de la suite, para aislar el contenido de terceros del de Sytue:

| Patrón de dominio                    | Función                                        | Resolución                                    |
| ------------------------------------ | ---------------------------------------------- | --------------------------------------------- |
| `<app>.apps.sytue.eu`                | App de **producción** de un proyecto cliente   | wildcard `*.apps.sytue.eu`                    |
| `<proyecto>-pr-<n>.preview.sytue.eu` | **Preview** por rama/PR (efímero)              | wildcard `*.preview.sytue.eu`                 |
| `<dominio-propio-del-cliente>`       | Dominio propio del cliente conectado a una app | `CNAME` → `ingress.sytue.eu` + TLS automático |

> Ejemplos: `tienda.apps.sytue.eu` (prod), `tienda-pr-42.preview.sytue.eu` (preview del PR #42),
> `www.cliente.com` → `CNAME ingress.sytue.eu`.

#### 2.4 Dominio de desarrollo (paridad dev/prod)

Mismo esquema bajo `*.sytue.localhost` vía Traefik 3 + mkcert (ver puertos en §2.5 y en
`docs/CONVENTIONS.md` §6):

```
one.sytue.localhost          id.sytue.localhost           drive.sytue.localhost
docs.sytue.localhost         mail.sytue.localhost         calendar.sytue.localhost
contacts.sytue.localhost     rooms.sytue.localhost        rooms-sfu.sytue.localhost
cloud.sytue.localhost        admin.sytue.localhost      hub.sytue.localhost
mx.sytue.localhost           www.sytue.localhost / sytue.localhost
*.apps.sytue.localhost       *.preview.sytue.localhost
```

> **Eliminado en v4:** ya no existe `office.sytue.localhost` (OnlyOffice). _Sytue Docs_ sirve
> su colaboración (WS CRDT/OT) en el propio `docs.sytue.localhost`.

#### 2.5 Puertos de los servicios Go (dev / docker-compose)

Puertos HTTP de cada **servicio Go** y de los frontends Astro en dev (dominio `*.sytue.localhost`
vía Traefik). Fuente de verdad: `docs/CONVENTIONS.md` §6.

| Servicio Go (backend)            | Puerto                          | Frontend Astro (dev) | Puerto |
| -------------------------------- | ------------------------------- | -------------------- | ------ |
| Sytue ID                         | 8081                            | portal               | 4321   |
| Sytue Drive (+WebDAV)            | 8082                            | console              | 4322   |
| Sytue Docs (+colab WS)           | 8083                            | cloud-dashboard      | 4323   |
| Sytue Rooms (+WS)                | 8085                            | www                  | 4324   |
| Sytue Mail (HTTP/JMAP)           | 8086 (SMTP/IMAP 25/465/587/993) | mail                 | 4325   |
| Sytue Calendar (+CalDAV)         | 8087                            | drive                | 4326   |
| Sytue Contacts (+CardDAV)        | 8088                            | calendar             | 4327   |
| Sytue Notepads                   | 8089                            | contacts             | 4328   |
| Sytue Cloud API                  | 8090                            | (libre, era docs)    | 4329   |
| Sytue Cloud builder              | 8091                            | rooms                | 4330   |
| Sytue Hub                        | 8092                            | notes                | 4331   |
| Sytue Integration Gateway        | 8093                            |                      |        |
| Sytue Analytics colector         | 8094                            |                      |        |
| Sytue Analytics API              | 8095                            |                      |        |
| Sytue OCR (interno, sin ingress) | 8096                            |                      |        |

| Infra (dev)                        | Puerto                                |
| ---------------------------------- | ------------------------------------- |
| Traefik HTTP / HTTPS / dashboard   | 80 / 443 / 8080                       |
| PostgreSQL                         | 5432                                  |
| Object Storage MinIO API / consola | 9000 / 9001                           |
| LiveKit (Meet, WebRTC)             | 7880 (WS) / 7881 (TCP) / 7882 UDP mux |
| Sytue Mail SMTP / IMAP             | 25 / 465 / 587 / 993                  |

> El estado, las colas y la caché viven en **PostgreSQL** (5432). **LiveKit** usa los
> puertos de medios; **Synapse** escucha en 8008 dentro del clúster y publica únicamente
> sus rutas de cliente por HTTPS (§0.2). Su API de administración permanece interna.

---

<a id="163-plan-dns-para-sytueeu-en-scaleway-dns"></a>

### 16.3 Plan DNS para `sytue.eu` en Scaleway DNS

Gestión declarativa con **Terraform** (`provider scaleway/scaleway ~> 2.x`, módulo
`infra/terraform/modules/dns`). Tipo de zona: **gestionada por Scaleway DNS** sobre el dominio
externo `sytue.eu`. Todos los registros se versionan en el estado Terraform (backend S3
Scaleway, región UE).

> **Convenciones:**
>
> - Los registros `A`/`AAAA` apuntan al **Load Balancer** de Scaleway (IPv4 + IPv6).
> - Los subdominios de servicio apuntan vía `CNAME` al _host_ del ingress (`ingress.sytue.eu`)
>   salvo el apex y `ingress` (que llevan `A`/`AAAA` directos, porque el apex no admite `CNAME`).
> - Placeholders: `<LB_IPV4>`, `<LB_IPV6>` = IPs del Load Balancer; `<LB_IPV4_MAIL>`,
>   `<LB_IPV6_MAIL>` = IP(s) públicas del servicio de correo; `<DKIM_PUBLIC_KEY>` = clave
>   pública DKIM generada por _Sytue Mail_ (la privada **nunca** va en git ni en DNS).
> - TTL por defecto **300 s** (5 min) durante despliegue; subir a **3600 s** una vez estable.

#### 3.1 Registros base (A/AAAA del ingress y apex)

| Nombre            | Tipo   | Valor            | TTL  | Notas                                                                 |
| ----------------- | ------ | ---------------- | ---- | --------------------------------------------------------------------- |
| `sytue.eu` (apex) | `A`    | `<LB_IPV4>`      | 300  | Sitio público.                                                        |
| `sytue.eu` (apex) | `AAAA` | `<LB_IPV6>`      | 300  | Sitio público (IPv6).                                                 |
| `ingress`         | `A`    | `<LB_IPV4>`      | 300  | Punto de entrada del clúster.                                         |
| `ingress`         | `AAAA` | `<LB_IPV6>`      | 300  | Punto de entrada (IPv6).                                              |
| `mx`              | `A`    | `<LB_IPV4_MAIL>` | 3600 | IP pública del nodo/LB de Sytue Mail (SMTP/IMAP necesitan A directo). |
| `mx`              | `AAAA` | `<LB_IPV6_MAIL>` | 3600 | IPv6 del correo.                                                      |

#### 3.2 Subdominios de servicio (CNAME → ingress)

Todas las superficies web cuelgan del mismo ingress; el enrutado por host lo hace Traefik 3
hacia el servicio Go correspondiente.

| Nombre         | Tipo    | Valor               | TTL  |
| -------------- | ------- | ------------------- | ---- |
| `www`          | `CNAME` | `sytue.eu.`         | 3600 |
| `one`          | `CNAME` | `ingress.sytue.eu.` | 300  |
| `id`           | `CNAME` | `ingress.sytue.eu.` | 300  |
| `drive`        | `CNAME` | `ingress.sytue.eu.` | 300  |
| `docs`         | `CNAME` | `ingress.sytue.eu.` | 300  |
| `mail`         | `CNAME` | `ingress.sytue.eu.` | 300  |
| `calendar`     | `CNAME` | `ingress.sytue.eu.` | 300  |
| `contacts`     | `CNAME` | `ingress.sytue.eu.` | 300  |
| `rooms`        | `CNAME` | `ingress.sytue.eu.` | 300  |
| `rooms-sfu`    | `CNAME` | `ingress.sytue.eu.` | 300  |
| `livekit`      | `CNAME` | `ingress.sytue.eu.` | 300  |
| `cloud`        | `CNAME` | `ingress.sytue.eu.` | 300  |
| `console`      | `CNAME` | `ingress.sytue.eu.` | 300  |
| `hub`          | `CNAME` | `ingress.sytue.eu.` | 300  |
| `registry`     | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `status`       | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `mta-sts`      | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `autoconfig`   | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `autodiscover` | `CNAME` | `ingress.sytue.eu.` | 3600 |

> **Apex sin CNAME:** `www` apunta al apex y el apex usa `A`/`AAAA` (RFC: el apex no puede ser
> `CNAME`). Si Scaleway DNS ofreciera _alias_ a nivel apex, podría usarse en lugar de `A`/`AAAA`.
> **Eliminado:** ya no hay registro `office` (OnlyOffice fuera del stack v4).

#### 3.3 Wildcards del PaaS (`*.apps` y `*.preview`)

| Nombre      | Tipo    | Valor               | TTL | Notas                                                                                                                                                   |
| ----------- | ------- | ------------------- | --- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `*.apps`    | `CNAME` | `ingress.sytue.eu.` | 300 | Apps de producción de clientes. TLS automático por host vía cert-manager (certificado _wildcard_ `*.apps.sytue.eu` con DNS-01, o por host con HTTP-01). |
| `*.preview` | `CNAME` | `ingress.sytue.eu.` | 300 | Previews efímeros por PR. Certificado _wildcard_ `*.preview.sytue.eu` (DNS-01 contra Scaleway DNS).                                                     |

> **TLS de wildcards:** los certificados `*.apps.sytue.eu` y `*.preview.sytue.eu` se emiten con
> **cert-manager** usando el reto **DNS-01** contra la API de Scaleway DNS (un `Issuer`/`ClusterIssuer`
> con credenciales en Scaleway Secret Manager, materializadas como `Secret` de
> Kubernetes por el bootstrap — nunca en git). Los dominios
> propios de clientes se validan por host con HTTP-01 al conectarse.

#### 3.4 Correo — Sytue Mail (MX + SPF + DKIM + DMARC + MTA-STS + TLS-RPT)

El dominio de correo es `sytue.eu` y el host de correo es `mx.sytue.eu`. _Sytue Mail_ es el
**servidor de correo Go propio** (clean-room sobre `emersion/go-smtp`, `go-imap`, `go-message`
— librerías MIT — y JMAP propio): SMTP 25/465/587, IMAP 993, JMAP sobre HTTP. Esta sección
implementa la cadena completa de autenticación y políticas de correo exigida en el contrato
(`SPF+DKIM+DMARC+MTA-STS+TLS-RPT`).

##### 3.4.1 MX

| Nombre     | Tipo | Prioridad | Valor          | TTL  |
| ---------- | ---- | --------- | -------------- | ---- |
| `sytue.eu` | `MX` | `10`      | `mx.sytue.eu.` | 3600 |

> Un único MX en `mx.sytue.eu` (Sytue Mail). Un segundo MX de respaldo en la región DR (`nl-ams`)
> se añadiría como `MX 20 mx2.sytue.eu.` si se habilita alta disponibilidad de correo.

##### 3.4.2 SPF (autoriza al MX a enviar; rechaza el resto)

| Nombre     | Tipo  | Valor              | TTL  |
| ---------- | ----- | ------------------ | ---- |
| `sytue.eu` | `TXT` | `"v=spf1 mx -all"` | 3600 |

> `mx` autoriza a los hosts del registro MX; `-all` (hardfail) rechaza cualquier otro origen.
> Si más adelante se envía desde otra IP/servicio UE, se añade con `ip4:`/`ip6:`/`include:`
> antes de `-all`.

##### 3.4.3 DKIM (firma criptográfica; clave pública en DNS, privada en Sytue Mail)

Selector recomendado: `sytue` (rotable a `sytue2`, `sytue3`… al rotar claves). _Sytue Mail_
genera el par de claves; **solo la pública** se publica en DNS.

| Nombre             | Tipo  | Valor                                   | TTL  |
| ------------------ | ----- | --------------------------------------- | ---- |
| `sytue._domainkey` | `TXT` | `"v=DKIM1; k=rsa; p=<DKIM_PUBLIC_KEY>"` | 3600 |

> Placeholder `<DKIM_PUBLIC_KEY>`: clave RSA pública (base64) emitida por _Sytue Mail_. La clave
> privada permanece en el almacén de secretos del servicio (Secret Manager + Sealed Secret);
> **jamás** en git ni en DNS. Si el valor supera 255 caracteres, se divide en _strings_
> concatenadas dentro del mismo `TXT`. Ed25519 (`k=ed25519`) es una alternativa válida.

##### 3.4.4 DMARC (política de alineación + reportes)

| Nombre   | Tipo  | Valor                                                                                                                          | TTL  |
| -------- | ----- | ------------------------------------------------------------------------------------------------------------------------------ | ---- |
| `_dmarc` | `TXT` | `"v=DMARC1; p=reject; rua=mailto:dmarc-reports@sytue.eu; ruf=mailto:dmarc-forensics@sytue.eu; fo=1; adkim=s; aspf=s; pct=100"` | 3600 |

> `p=reject` rechaza correo no alineado; `adkim=s`/`aspf=s` exigen alineación estricta;
> `rua`/`ruf` reciben los reportes agregados/forenses en buzones de _Sytue Mail_.
> **Despliegue progresivo:** comenzar con `p=none` (solo monitorización), pasar a `p=quarantine`
> y finalmente a `p=reject` una vez verificados SPF/DKIM en los reportes `rua`.

##### 3.4.5 MTA-STS (fuerza TLS entre servidores de correo)

Dos partes: un registro DNS `TXT` que anuncia la política y un archivo de política servido por
**HTTPS** en `https://mta-sts.sytue.eu/.well-known/mta-sts.txt`.

| Nombre     | Tipo  | Valor                            | TTL  |
| ---------- | ----- | -------------------------------- | ---- |
| `_mta-sts` | `TXT` | `"v=STSv1; id=20260630T000000Z"` | 3600 |

> El campo `id` cambia cada vez que se actualiza el archivo de política (formato `<fechaZ>`).
> El subdominio `mta-sts.sytue.eu` está en §2.2/§3.2 (CNAME → ingress) y sirve, vía Traefik, el
> fichero estático generado por _Sytue Mail_:
>
> ```text
> # https://mta-sts.sytue.eu/.well-known/mta-sts.txt
> version: STSv1
> mode: enforce
> mx: mx.sytue.eu
> max_age: 604800
> ```
>
> `mode: enforce` rechaza la entrega si no hay TLS válido. Durante el rollout puede usarse
> `mode: testing` para recibir reportes sin bloquear.

##### 3.4.6 TLS-RPT (reportes de fallos de TLS de transporte)

| Nombre       | Tipo  | Valor                                           | TTL  |
| ------------ | ----- | ----------------------------------------------- | ---- |
| `_smtp._tls` | `TXT` | `"v=TLSRPTv1; rua=mailto:tls-reports@sytue.eu"` | 3600 |

> Los servidores remotos envían reportes de errores de negociación TLS a `tls-reports@sytue.eu`
> (buzón de _Sytue Mail_), complementando MTA-STS.

##### 3.4.7 Autodescubrimiento de clientes nativos (correo, calendario y contactos)

**Qué se persigue:** que el usuario escriba **solo su dirección de correo y su contraseña** en
Apple Mail, Outlook, Thunderbird o DAVx5 y todo lo demás se resuelva solo. Esto lo consiguen
dos piezas: los _endpoints_ HTTP que ya sirven los servicios Go (§ siguiente) y **estos
registros DNS**. Sin los registros, los endpoints existen pero nadie los encuentra.

Endpoints que respaldan esta sección (ya desplegados; ver `docs/architecture.md#5-capa-de-compatibilidad-entrante`):

| Endpoint                                                              | Servicio            | Lo consulta                          |
| --------------------------------------------------------------------- | ------------------- | ------------------------------------ |
| `/.well-known/autoconfig/mail/config-v1.1.xml`                        | `services/mail`     | Thunderbird y compatibles            |
| `/mail/config-v1.1.xml?emailaddress=…`                                | `services/mail`     | Thunderbird (host `autoconfig.`)     |
| `/autodiscover/autodiscover.xml` (y `/Autodiscover/Autodiscover.xml`) | `services/mail`     | Outlook (POX Autodiscover)           |
| `/.well-known/caldav` → `301` a `/dav/`                               | `services/calendar` | Apple Calendario, Thunderbird, DAVx5 |
| `/.well-known/carddav` → `301` a `/dav/`                              | `services/contacts` | Apple Contactos, DAVx5               |
| `/account/apple.mobileconfig` (con sesión)                            | `services/id`       | iOS/macOS (perfil de configuración)  |

###### A. Imprescindibles

Sin estos registros el autodescubrimiento **no funciona** (los nombres que anuncia el propio
servidor no resolverían, o el cliente no encontraría dónde preguntar).

| Nombre         | Tipo    | Valor               | TTL  | Para qué / quién lo usa                                                                                                                                                   |
| -------------- | ------- | ------------------- | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `imap`         | `CNAME` | `mx.sytue.eu.`      | 3600 | Nombre de servidor entrante que **anuncia** Sytue Mail (`IMAP_PUBLIC_HOST`, por defecto `imap.<dominio>`) y que lleva dentro el perfil de Apple. Todos los clientes IMAP. |
| `smtp`         | `CNAME` | `mx.sytue.eu.`      | 3600 | Ídem para el saliente (`SMTP_PUBLIC_HOST`, por defecto `smtp.<dominio>`). Todos los clientes SMTP.                                                                        |
| `autoconfig`   | `CNAME` | `ingress.sytue.eu.` | 3600 | **Thunderbird**: consulta `https://autoconfig.<dominio>/mail/config-v1.1.xml?emailaddress=…` antes de recurrir a la base de datos de Mozilla.                             |
| `autodiscover` | `CNAME` | `ingress.sytue.eu.` | 3600 | **Outlook**: `https://autodiscover.<dominio>/autodiscover/autodiscover.xml`.                                                                                              |
| `calendar`     | `CNAME` | `ingress.sytue.eu.` | 300  | Destino del descubrimiento CalDAV (ya en §3.2; se repite por su papel aquí).                                                                                              |
| `contacts`     | `CNAME` | `ingress.sytue.eu.` | 300  | Destino del descubrimiento CardDAV (ya en §3.2).                                                                                                                          |

> `imap`/`smtp` apuntan a `mx` (la IP del correo), **no** al ingress: IMAP 993 y submission 465
> no pasan por Traefik. `autoconfig`/`autodiscover` sí apuntan al ingress: son HTTPS y los
> atiende `services/mail` (Ingress de `charts/mail`, `ingress.extraSubdomains`).

###### B. Opcionales (recomendados: acortan o rescatan el descubrimiento)

Formato SRV: `<prioridad> <peso> <puerto> <destino>`. Prioridad menor gana; el peso reparte
entre iguales.

| Nombre               | Tipo  | Valor (prio peso puerto destino) | TTL  | Para qué / quién lo usa                                                                                                                |
| -------------------- | ----- | -------------------------------- | ---- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `_imaps._tcp`        | `SRV` | `0 1 993 mx.sytue.eu.`           | 3600 | IMAP sobre TLS implícito (RFC 6186). Thunderbird y clientes que hacen SRV.                                                             |
| `_submissions._tcp`  | `SRV` | `0 1 465 mx.sytue.eu.`           | 3600 | Envío con TLS implícito (RFC 8314). **Es lo que Sytue Mail anuncia** en su autoconfiguración.                                          |
| `_submission._tcp`   | `SRV` | `0 1 587 mx.sytue.eu.`           | 3600 | Envío con STARTTLS. Publicar **solo** si el despliegue sirve el 587 con certificado; Sytue Mail no lo anuncia en la autoconfiguración. |
| `_autodiscover._tcp` | `SRV` | `0 0 443 autodiscover.sytue.eu.` | 3600 | **Outlook**: último recurso si las consultas HTTPS fallan.                                                                             |
| `_caldavs._tcp`      | `SRV` | `0 1 443 calendar.sytue.eu.`     | 3600 | **CalDAV** (RFC 6764): permite descubrir el calendario desde el dominio de correo (Apple, DAVx5, Thunderbird).                         |
| `_carddavs._tcp`     | `SRV` | `0 1 443 contacts.sytue.eu.`     | 3600 | **CardDAV** (RFC 6764): ídem para contactos.                                                                                           |
| `_caldavs._tcp`      | `TXT` | `"path=/dav/"`                   | 3600 | Ruta del contexto DAV (RFC 6764 §6). Evita depender del `301` del well-known.                                                          |
| `_carddavs._tcp`     | `TXT` | `"path=/dav/"`                   | 3600 | Ídem para CardDAV.                                                                                                                     |
| `_jmap._tcp`         | `SRV` | `0 1 443 mx.sytue.eu.`           | 3600 | Descubrimiento JMAP (clientes JMAP; el webmail no lo necesita).                                                                        |

> **`_caldavs`/`_carddavs` apuntan al host de producto** (`calendar.`/`contacts.`), no a `mx`:
> ahí es donde el Ingress enruta `/.well-known/caldav` y `/dav` al servicio Go
> (`charts/calendar` y `charts/contacts`, `ingress.paths`).

###### C. Lo que NO cubrimos (honestidad de alcance)

- **El apex** (`https://sytue.eu/.well-known/autoconfig/…` y `https://sytue.eu/autodiscover/…`)
  **no** está enrutado a `services/mail`. Ambos clientes lo prueban (Outlook, de hecho, antes
  que `autodiscover.`) y siguen adelante cuando no responde, así que la configuración acaba
  saliendo por `autoconfig.`/`autodiscover.`. Cubrir el apex exigiría una regla propia en el
  Ingress de `charts/mail` (`extraSubdomains` compone `<label>.<dominio>`, no sirve para el
  apex) y que el certificado de mail incluyera el apex: se ha dejado fuera a propósito.
- **Exchange / ActiveSync**: no se implementan. Outlook se configura como cuenta **IMAP/SMTP**
  vía Autodiscover POX; no hay `Exchange Web Services` ni sincronización EAS.
- **DAVx5 y clientes Android** usan los registros `_caldavs`/`_carddavs` o directamente la URL
  `https://calendar.<dominio>/dav/` — no hay descubrimiento propietario de Google.

##### 3.4.8 CAA (restringe qué CA pueden emitir certificados — buena práctica)

| Nombre     | Tipo  | Valor                                | TTL  | Notas                                        |
| ---------- | ----- | ------------------------------------ | ---- | -------------------------------------------- |
| `sytue.eu` | `CAA` | `0 issue "letsencrypt.org"`          | 3600 | Solo Let's Encrypt (usada por cert-manager). |
| `sytue.eu` | `CAA` | `0 issuewild "letsencrypt.org"`      | 3600 | Permite _wildcards_ (PaaS) de la misma CA.   |
| `sytue.eu` | `CAA` | `0 iodef "mailto:security@sytue.eu"` | 3600 | Notificación de emisiones no autorizadas.    |

#### 3.5 Resumen del fichero de zona (referencia)

```dns
; ----- Base / ingress -----
sytue.eu.            300   IN A     <LB_IPV4>
sytue.eu.            300   IN AAAA  <LB_IPV6>
ingress.sytue.eu.    300   IN A     <LB_IPV4>
ingress.sytue.eu.    300   IN AAAA  <LB_IPV6>

; ----- Superficies de producto (CNAME → ingress → servicio Go) -----
www.sytue.eu.        3600  IN CNAME sytue.eu.
one.sytue.eu.        300   IN CNAME ingress.sytue.eu.
id.sytue.eu.         300   IN CNAME ingress.sytue.eu.
drive.sytue.eu.      300   IN CNAME ingress.sytue.eu.
docs.sytue.eu.       300   IN CNAME ingress.sytue.eu.
mail.sytue.eu.       300   IN CNAME ingress.sytue.eu.
calendar.sytue.eu.   300   IN CNAME ingress.sytue.eu.
contacts.sytue.eu.   300   IN CNAME ingress.sytue.eu.
rooms.sytue.eu.      300   IN CNAME ingress.sytue.eu.
rooms-sfu.sytue.eu.  300   IN CNAME ingress.sytue.eu.
livekit.sytue.eu.    300   IN CNAME ingress.sytue.eu.
cloud.sytue.eu.      300   IN CNAME ingress.sytue.eu.
admin.sytue.eu.    300   IN CNAME ingress.sytue.eu.
hub.sytue.eu.        300   IN CNAME ingress.sytue.eu.

; ----- Técnicos -----
registry.sytue.eu.   3600  IN CNAME ingress.sytue.eu.
status.sytue.eu.     3600  IN CNAME ingress.sytue.eu.
mta-sts.sytue.eu.    3600  IN CNAME ingress.sytue.eu.
autoconfig.sytue.eu. 3600  IN CNAME ingress.sytue.eu.
autodiscover.sytue.eu. 3600 IN CNAME ingress.sytue.eu.
; imap./smtp. van al CORREO (mx), no al ingress: 993/465 no pasan por Traefik.
imap.sytue.eu.       3600  IN CNAME mx.sytue.eu.
smtp.sytue.eu.       3600  IN CNAME mx.sytue.eu.

; ----- PaaS wildcards -----
*.apps.sytue.eu.     300   IN CNAME ingress.sytue.eu.
*.preview.sytue.eu.  300   IN CNAME ingress.sytue.eu.

; ----- Correo (Sytue Mail) -----
mx.sytue.eu.         3600  IN A     <LB_IPV4_MAIL>
mx.sytue.eu.         3600  IN AAAA  <LB_IPV6_MAIL>
sytue.eu.            3600  IN MX 10 mx.sytue.eu.
sytue.eu.            3600  IN TXT   "v=spf1 mx -all"
sytue._domainkey.sytue.eu. 3600 IN TXT "v=DKIM1; k=rsa; p=<DKIM_PUBLIC_KEY>"
_dmarc.sytue.eu.     3600  IN TXT   "v=DMARC1; p=reject; rua=mailto:dmarc-reports@sytue.eu; ruf=mailto:dmarc-forensics@sytue.eu; fo=1; adkim=s; aspf=s; pct=100"
_mta-sts.sytue.eu.   3600  IN TXT   "v=STSv1; id=20260630T000000Z"
_smtp._tls.sytue.eu. 3600  IN TXT   "v=TLSRPTv1; rua=mailto:tls-reports@sytue.eu"

; ----- Descubrimiento de clientes nativos (SRV, ver §3.4.7) -----
_imaps._tcp.sytue.eu.        3600 IN SRV 0 1 993 mx.sytue.eu.
_submissions._tcp.sytue.eu.  3600 IN SRV 0 1 465 mx.sytue.eu.
_submission._tcp.sytue.eu.   3600 IN SRV 0 1 587 mx.sytue.eu.   ; solo si se sirve el 587
_autodiscover._tcp.sytue.eu. 3600 IN SRV 0 0 443 autodiscover.sytue.eu.
_caldavs._tcp.sytue.eu.      3600 IN SRV 0 1 443 calendar.sytue.eu.
_carddavs._tcp.sytue.eu.     3600 IN SRV 0 1 443 contacts.sytue.eu.
_caldavs._tcp.sytue.eu.      3600 IN TXT "path=/dav/"
_carddavs._tcp.sytue.eu.     3600 IN TXT "path=/dav/"
_jmap._tcp.sytue.eu.         3600 IN SRV 0 1 443 mx.sytue.eu.

; ----- CAA -----
sytue.eu.            3600  IN CAA 0 issue "letsencrypt.org"
sytue.eu.            3600  IN CAA 0 issuewild "letsencrypt.org"
sytue.eu.            3600  IN CAA 0 iodef "mailto:security@sytue.eu"
```

> Este fichero es **referencia**; la fuente declarativa real es Terraform
> (`scaleway_domain_record` en `infra/terraform/modules/dns`). Los placeholders se resuelven
> desde _outputs_ de los módulos `loadbalancer`/`network` y desde secretos (DKIM) en el almacén.

---

<a id="164-convencion-de-naming-de-recursos-cloud-y-labels-de-kubernetes"></a>

### 16.4 Convención de naming de recursos cloud y labels de Kubernetes

#### 4.1 Variables comunes

| Variable    | Valores                              | Descripción                                             |
| ----------- | ------------------------------------ | ------------------------------------------------------- |
| `<env>`     | `dev` · `staging` · `prod`           | Entorno (dev = local Compose; staging/prod = Scaleway). |
| `<service>` | slug del producto/recurso (§1.2)     | p. ej. `id`, `drive`, `mail`, `hub`, `pg`.              |
| `<tenant>`  | slug de organización en _kebab-case_ | p. ej. `acme`, `cliente-uno`.                           |
| `<region>`  | `fr-par` (principal) · `nl-ams` (DR) | Solo regiones UE de Scaleway.                           |

> En v4 los slugs `<service>` son **servicios Go** (`id`, `drive`, `docs`, `mail`, `calendar`,
> `contacts`, `rooms`, `cloud`, `hub`) más recursos de infraestructura (`pg`, etc.).
> Ya **no** existen slugs de motores (`keycloak`, `nextcloud`, `mattermost`, `stalwart`,
> `onlyoffice`) ni `valkey`.

#### 4.2 Nombres de recursos cloud (Scaleway / Terraform)

Patrón canónico: **`sytue-<env>-<service>`** (todo en minúsculas, _kebab-case_). Se aplica a
Kapsule, RDB, Object Storage, Registry, Load Balancer, Secret Manager, DNS, redes, etc.

| Recurso Scaleway               | Patrón de nombre                         | Ejemplos                                                                                                                                        |
| ------------------------------ | ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Clúster Kapsule                | `sytue-<env>-k8s`                        | `sytue-prod-k8s`, `sytue-staging-k8s`                                                                                                           |
| Pool de nodos Kapsule          | `sytue-<env>-pool-<perfil>`              | `sytue-prod-pool-platform`, `sytue-prod-pool-workload`, `sytue-prod-pool-media`                                                                 |
| Base de datos RDB (PostgreSQL) | `sytue-<env>-pg-<rol>`                   | `sytue-prod-pg-suite`, `sytue-prod-pg-paas`                                                                                                     |
| Bucket Object Storage (S3)     | `sytue-<env>-<purpose>`                  | `sytue-prod-drive-blobs`, `sytue-prod-mail-blobs`, `sytue-prod-docs-blobs`, `sytue-prod-backups`, `sytue-prod-tfstate`, `sytue-prod-paas-cache` |
| Load Balancer                  | `sytue-<env>-lb`                         | `sytue-prod-lb`                                                                                                                                 |
| Private Network (VPC)          | `sytue-<env>-net`                        | `sytue-prod-net`                                                                                                                                |
| Container Registry (namespace) | `sytue-<env>-registry`                   | `sytue-prod-registry`                                                                                                                           |
| Secret (Secret Manager)        | `sytue-<env>-<service>-<clave>`          | `sytue-prod-id-signing-key`, `sytue-prod-mail-dkim`, `sytue-prod-livekit-apikey`                                                                |
| Zona DNS                       | `sytue.eu` (externa, sin prefijo de env) | —                                                                                                                                               |

> **Sin Valkey:** no existe el patrón `sytue-<env>-valkey-<service>` (eliminado en v4; estado,
> colas y caché viven en **PostgreSQL**). **Buckets por producto:** cada servicio Go que guarda
> blobs usa su propio bucket/prefijo (`sytue-<env>-drive-blobs`, `-mail-blobs`, `-docs-blobs`).
> **Buckets PaaS por proyecto/tenant:** `sytue-<env>-paas-<tenant>-<proyecto>` (o prefijos S3
> por proyecto dentro del bucket de almacenamiento del PaaS). Recursos multi-tenant del PaaS
> llevan además el slug de tenant: `sytue-<env>-pg-<tenant>-<proyecto>`.

#### 4.3 Namespaces de Kubernetes (Kapsule)

| Tipo                         | Patrón            | Ejemplos                                                    |
| ---------------------------- | ----------------- | ----------------------------------------------------------- |
| Plataforma (suite Sytue One) | `sytue-system`    | `sytue-system` (servicios Go, ingress, operadores, LiveKit) |
| Por producto (opcional)      | `sytue-<service>` | `sytue-mail`, `sytue-rooms`, `sytue-cloud`                  |
| Por tenant (multi-tenant)    | `tenant-<slug>`   | `tenant-acme`, `tenant-cliente-uno`                         |

> Cada namespace `tenant-<slug>` lleva **ResourceQuota + NetworkPolicy** (aislamiento), mapeado
> 1:1 con el grupo de _Sytue ID_ `org/<slug>` (ver `docs/CONVENTIONS.md` §9). El namespace de
> plataforma es `sytue-system`.

#### 4.4 Nombres de releases Helm y recursos k8s

- **Umbrella chart:** release `sytue-one` en `sytue-system` (solo servicios Go + LiveKit + Traefik).
- **Subcharts/releases por servicio Go:** `sytue-<service>` (p. ej. `sytue-id`, `sytue-drive`,
  `sytue-docs`, `sytue-mail`, `sytue-calendar`, `sytue-contacts`, `sytue-rooms`,
  `sytue-cloud`, `sytue-hub`). Componente de terceros: `sytue-livekit`.
- Recursos derivados (Deployment/Service/Ingress/PVC) heredan el nombre del release +
  sufijo estándar de Helm (`sytue-<service>-<componente>`).

> **Sin releases de motores:** ya no existen `sytue-keycloak`, `sytue-nextcloud`,
> `sytue-mattermost`, `sytue-stalwart`, `sytue-onlyoffice`.

#### 4.5 Labels y anotaciones de Kubernetes

Etiquetas **obligatorias** en todo objeto de la suite (estándar `app.kubernetes.io/*` + dominio
propio `sytue.eu/*`):

| Label                          | Valor                                          | Obligatorio | Ejemplo                                       |
| ------------------------------ | ---------------------------------------------- | ----------- | --------------------------------------------- |
| `app.kubernetes.io/part-of`    | `sytue-one`                                    | Sí          | `sytue-one`                                   |
| `app.kubernetes.io/name`       | slug del servicio                              | Sí          | `mail`, `id`, `hub`                           |
| `app.kubernetes.io/instance`   | nombre del release Helm                        | Sí          | `sytue-mail`                                  |
| `app.kubernetes.io/component`  | rol del componente                             | Sí          | `frontend`, `api`, `worker`, `ingress`, `sfu` |
| `app.kubernetes.io/managed-by` | `Helm` (o `Terraform`)                         | Sí          | `Helm`                                        |
| `app.kubernetes.io/version`    | versión del componente                         | Recomendado | `1.0.0`                                       |
| `sytue.eu/env`                 | `<env>`                                        | Sí          | `prod`, `staging`, `dev`                      |
| `sytue.eu/stack`               | `go` (backend) / `astro` (frontend)            | Recomendado | `go`                                          |
| `sytue.eu/product`             | slug del producto (§1.2)                       | Sí          | `mail`, `drive`, `cloud`                      |
| `sytue.eu/tenant`              | `<slug>` del tenant (solo cargas multi-tenant) | Condicional | `acme` (ausente en plataforma)                |

Ejemplo de bloque de labels (Deployment del servicio Go _Sytue Mail_ en prod):

```yaml
metadata:
  labels:
    app.kubernetes.io/part-of: sytue-one
    app.kubernetes.io/name: mail
    app.kubernetes.io/instance: sytue-mail
    app.kubernetes.io/component: api
    app.kubernetes.io/managed-by: Helm
    app.kubernetes.io/version: "1.0.0"
    sytue.eu/env: prod
    sytue.eu/stack: go
    sytue.eu/product: mail
```

#### 4.6 Tags de recursos Scaleway

Los recursos Scaleway llevan _tags_ equivalentes (Scaleway usa tags simples tipo `clave=valor`
como cadena), generados por Terraform:

```hcl
tags = [
  "part-of=sytue-one",
  "env=${var.env}",            # dev | staging | prod
  "product=${var.product}",    # mail | drive | hub | ...
  "stack=go",                  # backend Go (frontend usa stack=astro)
  "managed-by=terraform",
  "region=${var.region}",      # fr-par | nl-ams
]
```

#### 4.7 Resumen de coherencia slug → recurso

Para un producto con slug `<service>` en entorno `<env>`:

| Plano                | Valor derivado                                                               |
| -------------------- | ---------------------------------------------------------------------------- |
| Subdominio           | `<service>.sytue.eu`                                                         |
| Servicio Go          | `services/<service>/cmd/<service>/main.go` (módulo `git.sytue.eu/sytue/one`) |
| Client OIDC          | `sytue-<service>` (Sytue Cloud añade `sytue-cloud-api`)                      |
| Recurso Scaleway     | `sytue-<env>-<service>` (RDB usa `-pg-<rol>`)                                |
| Release Helm         | `sytue-<service>`                                                            |
| Namespace (opcional) | `sytue-<service>` (plataforma) / `tenant-<slug>` (tenant)                    |
| Label `name`         | `<service>`                                                                  |
| Label `product`      | `<service>`                                                                  |
| Bucket S3            | `sytue-<env>-<purpose>` (p. ej. `<service>-blobs`)                           |

<a id="pages-convenciones"></a>

## Convenciones específicas de Sytue Web

El IR de Sytue Web es cable versionado: las claves son `snake_case`, los cambios incompatibles exigen
otra versión y el espejo Go y TypeScript se actualizan en la misma entrega. El editor no serializa
ni normaliza un borrador ajeno; guarda el IR validado y conserva la concurrencia optimista. Una
traducción de IA es un parche revisable del borrador, nunca una publicación automática.

Las publicaciones y exportaciones deben usar el contrato público `/_pages/v1`; no se replica ni
se inventa una API por destino. Cualquier recurso publicado se valida contra la lista permitida y
las islas se generan en la base común. Los dominios se verifican con TXT y no se aceptan como URL
de salida arbitraria; las comprobaciones HTTPS usan el destino fijo autorizado por configuración.
