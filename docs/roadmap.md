<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

# Estado y hoja de ruta — Sytue Suite

Resumen público de la madurez arquitectónica de **Sytue Suite** y las líneas de evolución previstas:

1. **[Estado del proyecto](#1-estado-del-proyecto)** — arquitectura y componentes de la suite.
2. **[Roadmap por fases](#2-roadmap-por-fases)** — hitos completados (F0–F6).
3. **[Madurez por producto](#3-paridad-panorama-y-backlog-transversal)** — capacidades actuales por servicio.
4. **[Sytue Rooms](#4-rooms--gaps-al-detalle-vs-mattermost--slack--teams)** — mensajería, canales, videollamadas y cifrado E2EE.
5. **[Ofimática nativa y clientes Fyne](#5-docs--paridad-onlyoffice-y-apps-nativas)** — Docus, Calcus, Diapos, CertiPDF y apps nativas.
6. **[Sytue Cloud](#6-sytue-cloud--vision-y-workstreams)** — plataforma PaaS y servicios gestionados.

> Documentación de arquitectura: [`architecture.md`](architecture.md). Decisiones técnicas: [`decisions.md`](decisions.md).

---

<a id="1-estado-del-proyecto"></a>

## 1. Estado del proyecto

<a id="resumen-ejecutivo"></a>

### Resumen ejecutivo

**Sytue Suite** es una suite soberana de productividad, colaboración, gestión empresarial (ERP) y plataforma cloud (PaaS) desarrollada íntegramente desde cero (*clean-room*, licencia **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)**) sobre un stack unificado:

- **Backend**: **Go** (`go.mod` único, 16 microservicios independientes bajo `services/<svc>/cmd/<svc>` y librerías compartidas en `platform/pkg/*`).
- **Frontend Web**: **Astro + Preact (Bun)** en `apps/frontend/` (22 aplicaciones web y 10 paquetes compartidos `@sytue/*`, con diseño accesible WCAG AA, modo claro/oscuro y patrón BFF sin tokens expuestos en el navegador).
- **Clientes Nativos**: **Fyne (Go)** en `apps/fyne/` para escritorio (Linux, macOS, Windows) y móvil (Android, iOS/iPadOS) con 15 aplicaciones nativas sin WebView.
- **Almacenamiento**: **PostgreSQL 18** con aislamiento multi-tenant mediante *Row-Level Security* (`ENABLE` + `FORCE ROW LEVEL SECURITY`), **Dragonfly** (estado efímero y *fan-out* en tiempo real) y **Object Storage compatible con S3** (MinIO, SeaweedFS o S3 gestionado).

<a id="puertas-de-calidad-verificadas-reproducibles"></a>

### Verificación continua y puertas de calidad

| Verificación | Comando | Propósito |
| --- | --- | --- |
| Compilación y análisis Go | `go build ./... && go vet ./...` | Compilación limpia de los 16 servicios y paquetes de plataforma. |
| Tests unitarios Go | `go test -race ./...` | Validación de lógica de dominio, criptografía, OIDC/PKCE, parsers y políticas. |
| Tests de integración | `go test -tags=integration ./...` | Pruebas contra PostgreSQL real verificando migraciones y aislamiento RLS. |
| Compilación y tipado Web | `cd apps/frontend && bun run build && bun run typecheck` | Construcción de paquetes `@sytue/*` y verificación estricta de tipos en las 22 apps. |
| Despliegue guiado | `./sytue-deploy` / `make deploy` | Asistente paso a paso para Docker Compose y Kubernetes (Helm). |
| Smoke test E2E | `scripts/e2e/smoke.sh` | Comprobación de salud (`/healthz`, `/readyz`), OIDC discovery y flujo PKCE. |

<a id="componentes"></a>

### Componentes principales

- **`platform/pkg/`** (Go): `httpx` (router HTTP, cabeceras CSP/HSTS/CSRF, rate-limiting y sondas de salud), `authn` (verificación JWT/JWKS RS256), `pg` (pool `pgx`, migraciones embebidas y transacciones con RLS por organización), `events` (`LISTEN/NOTIFY` y outbox transaccional), `objectstore` (cliente S3), `crypto` (AES-256-GCM), `telemetry` (OpenTelemetry) y `tenancy`.
- **`services/`** (16 servicios Go):
  - `id`: Proveedor de identidad nuclear (OIDC/OAuth 2.1 con PKCE S256 obligatorio, MFA TOTP y WebAuthn/Passkeys, SCIM 2.0, Acceso Condicional, PIM y rotación JWKS).
  - `hub`: Agregador de actividad transversal, notificaciones en tiempo real (SSE), búsqueda global e intents entre aplicaciones.
  - `mail`: Servidor y cliente de correo electrónico (JMAP, IMAP, SMTP, firma DKIM, filtros Sieve y retención legal).
  - `drive`: Almacenamiento en la nube con API REST y WebDAV, versionado, enlaces compartidos con contraseña/caducidad y papelera unificada.
  - `docs`: Motor colaborativo para documentos (**SyDocus**), hojas de cálculo (**SyCalcus**), presentaciones (**SyDiapos**), bases de datos relacionales (**SyBases**) y edición/firma de PDF (**SyCertiPDF**).
  - `photos`: Galería fotográfica (**SyPhotos**) con álbumes, línea temporal y miniaturas.
  - `notes`: Cuadernos y notas rápidas (**SyNotepads**) con sincronización instantánea.
  - `calendar`: Calendarios y eventos con soporte CalDAV, invitaciones iMIP y programación de reuniones.
  - `contacts`: Agenda de contactos y grupos con soporte CardDAV.
  - `rooms`: Mensajería instantánea en tiempo real por canales e hilos, cifrado de extremo a extremo (Matrix/Olm) y videollamadas WebRTC mediante **LiveKit**.
  - `cast`: Plataforma de vídeo bajo demanda y emisiones en directo (HLS) con portada pública opcional.
  - `pages`: Creador visual de sitios web y tiendas online con publicación estática inmutable y exportación de código.
  - `syerp`: Sistema ERP modular (facturación, contabilidad, inventario, TPV, RR. HH., proyectos, gastos y encadenamiento criptográfico VeriFactu).
  - `cloud`: Plataforma PaaS para desplegar aplicaciones desde Git y aprovisionar bases de datos gestionadas sobre Kubernetes.
  - `analytics`: Analítica web de primera parte sin cookies ni rastreadores.
  - `integrations`: Pasarela de conectores salientes (BYO-SaaS, SSRF guard, auditoría de egress y asistentes de migración).
  - `ai` y `ocr`: Asistente de inteligencia artificial transversal con control de cuota y servicio de reconocimiento óptico de caracteres (Tesseract).

---

<a id="2-roadmap-por-fases"></a>

## 2. Roadmap por fases

| Fase | Alcance | Estado |
| --- | --- | --- |
| **F0 — Fundaciones y contrato canónico** | Estructura del monorepo, `CONVENTIONS.md`, librerías `platform/pkg/*` y pipeline CI. | Completada |
| **F1 — Identidad nuclear (Sytue ID) + Shell/Hub** | OIDC/OAuth 2.1 + PKCE, MFA (TOTP/WebAuthn), SCIM 2.0, `@sytue/shell` y `services/hub`. | Completada |
| **F2 — Núcleo de colaboración y productividad** | Sytue Drive, Sytue Mail, Sytue Calendar, Sytue Contacts y Sytue Rooms (chat + vídeo LiveKit). | Completada |
| **F3 — Ofimática nativa y PDF** | Editores nativos para documentos (Docus), hojas de cálculo (Calcus), presentaciones (Diapos) y CertiPDF con firma eIDAS (PAdES). | Completada |
| **F4 — Gestión empresarial y creación digital** | SyERP, Sytue Bases, Sytue Web (`services/pages`), Sytue Cast, Sytue Photos y Sytue Notepads. | Completada |
| **F5 — Sytue Cloud (PaaS) e Interoperabilidad** | Despliegue de aplicaciones en Kubernetes, Integration Gateway, capa de compatibilidad Slack/Teams y gobernanza enterprise. | Completada |
| **F6 — Clientes nativos multiplataforma** | Suite de 15 aplicaciones nativas en Fyne para Linux, macOS, Windows, Android e iOS/iPadOS. | Completada |

---

<a id="3-paridad-panorama-y-backlog-transversal"></a>

## 3. Madurez por producto y evolución continua

| Producto | Estado actual | Próximas mejoras en hoja de ruta |
| --- | --- | --- |
| **Sytue ID & Admin** | OIDC/PKCE, MFA/Passkeys, SCIM 2.0, Acceso Condicional, PIM, grupos dinámicos, auditoría SIEM. | Exportación SIEM en tiempo real (streaming syslog/CEF) y federación OIDC multi-emisor avanzada. |
| **Sytue Mail** | JMAP/SMTP/IMAP, DKIM, hilos, etiquetas, programación de envíos, reglas Sieve, plantillas y eDiscovery. | Búsqueda fonética avanzada y sincronización offline extendida en clientes móviles. |
| **Sytue Drive & Photos** | Árbol de carpetas, WebDAV, subida multiparte S3, enlaces públicos protegidos, versiones y galería fotográfica. | Escritura completa en protocolo WOPI (`Lock`/`PutFile`) para integraciones de terceros. |
| **SyDocus / SyCalcus / SyDiapos** | Coedición colaborativa, importación/exportación OOXML (`.docx`, `.xlsx`, `.pptx`) y ODF (`.odt`, `.ods`, `.odp`), exportación PDF/UA-1 y scripts aislados. | Ampliación del catálogo de funciones estadísticas/financieras en Calcus y transiciones avanzadas en Diapos. |
| **SyCertiPDF** | Visor vectorial SVG, anotaciones, combinación/división de páginas, OCR con capa de texto y firma digital avanzada PAdES. | Validación de lotes de firmas masivas por CLI y plantillas de formularios AcroForm interactivos. |
| **Sytue Rooms** | Canales públicos/privados, mensajes directos, hilos, reacciones, compartición de pantalla, salas WebRTC y cifrado E2EE. | Transcripción automática opcional de reuniones mediante `services/ai`. |
| **SyERP** | Facturación, contabilidad, inventario, TPV, proyectos, gastos, conciliación bancaria (Norma 43) y VeriFactu. | Conectores adicionales de banca abierta europea (PSD2) y localización fiscal para más países de la UE. |
| **Sytue Cloud** | Despliegue desde Git, builds aislados en Kubernetes, bases de datos PostgreSQL por proyecto, dominios con TLS automático y analítica web. | Autoescalado horizontal basado en métricas personalizadas y regiones múltiples en clústeres federados. |

---

<a id="4-rooms--gaps-al-detalle-vs-mattermost--slack--teams"></a>

## 4. Sytue Rooms — Arquitectura de comunicación en tiempo real

`services/rooms` unifica la mensajería persistente por canales y las videollamadas en una única base de datos y servicio:

- **Mensajería en tiempo real**: WebSockets con *fan-out* sobre PostgreSQL `LISTEN/NOTIFY` y Dragonfly para despliegues multi-réplica.
- **Videollamadas integradas**: Integración nativa con **LiveKit** (Apache-2.0) para audio, vídeo y pantalla compartida con generación de tokens firmados en el backend.
- **Compatibilidad de webhooks**: Adaptadores entrantes y salientes compatibles con el formato de webhooks de Slack y Microsoft Teams (`services/rooms/internal/compat`).
- **Seguridad y moderación**: Cifrado en dispositivos mediante Matrix, bloqueo de usuarios en servidor y sistema de denuncias para administradores de la organización.

---

<a id="5-docs--paridad-onlyoffice-y-apps-nativas"></a>

## 5. Ofimática nativa y aplicaciones de escritorio y móvil

La suite ofimática se organiza en aplicaciones especializadas que comparten el motor colaborativo de `services/docs` y almacenan los documentos directamente en el **Sytue Drive** del usuario:

- **SyDocus** (`apps/frontend/apps/docs`): Procesador de textos paginado con estilos, tablas, control de cambios, comentarios, notas al pie, tabla de contenido y combinación de correspondencia.
- **SyCalcus** (`apps/frontend/apps/sheets`): Hoja de cálculo con motor de fórmulas propio, formatos numéricos, filtros por condición, tablas dinámicas y gráficos.
- **SyDiapos** (`apps/frontend/apps/slides`): Editor de presentaciones con temas, patrones de diapositivas, notas del orador y modo presentación.
- **SyBases** (`apps/frontend/apps/bases`): Bases de datos relacionales colaborativas con vistas de tabla, kanban, calendario y formulario.
- **Aplicaciones nativas (`apps/fyne`)**: Quince aplicaciones compiladas de forma nativa en Go + Fyne que comparten inicio de sesión único mediante el llavero del sistema operativo ([ADR 0050](decisions.md#0050)).

---

<a id="6-sytue-cloud--vision-y-workstreams"></a>

## 6. Sytue Cloud — Plataforma PaaS y servicios gestionados

`services/cloud` proporciona una experiencia de despliegue de aplicaciones integrada con el resto de la suite:

- **Plano de control (`cloud-api`)**: Gestión de proyectos, entornos, variables de entorno cifradas con AES-256-GCM (`CLOUD_SECRET_ENC_KEY`), despliegues, dominios personalizados y límites de gasto opcionales.
- **Plano de ejecución (`KubeRunner`)**: Creación declarativa de `Namespace`, `Deployment`, `Service`, `Ingress` y `NetworkPolicy` de aislamiento por tenant en Kubernetes.
- **Servicios gestionados**: Aprovisionamiento de bases de datos PostgreSQL dedicadas, caché e inyección automática de credenciales en las aplicaciones desplegadas.
