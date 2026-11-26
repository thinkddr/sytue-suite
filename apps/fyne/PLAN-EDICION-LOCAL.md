# Plan — Edición local de la suite (solo apps nativas, sin servidores de Sytue)

Estado: **en ejecución** con el nombre **Sytue Office**. La decisión está en el
[ADR 0083](../../docs/decisions.md#0083) y lo pendiente, en el
[roadmap](../../docs/roadmap.md#sytue-office-lo-que-falta) (2026-11-24). Lo hecho, por app, y el
detalle de lo pendiente están en [`SYTUE-OFFICE.md`](SYTUE-OFFICE.md). Redactado el 2026-11-18 sobre
`origin/main` en `32aa6708da`.

> **Desviaciones decididas al ejecutar (2026-11-23).** Docus, Calcus y Diapos no usan el motor de
> registros (D-06) sino `nucleo/local/biblioteca`, un JSON en claro por documento: el motor exige
> un diario sellado y un estado de migración que en una edición sin servidor son accidente. Las
> demás apps sí usan el motor, sobre `nucleo/local/almacen` (registros en JSON por líneas, no
> SQLite como decía D-07). Además de lo previsto: nube opcional (Sytue Drive por WebDAV, Google
> Drive, OneDrive y carpetas sincronizadas) e IA del sistema (Apple Intelligence por Atajos en
> macOS, Foundry Local en Windows). Cast convierte con el ffmpeg instalado en el sistema.

Este documento es el plan completo para añadir una **segunda modalidad** de las apps nativas de Fyne:
funcionan enteras en el dispositivo, sin cuenta de Sytue, sin cifrado propio y sin ningún servidor de
Sytue. La edición conectada actual no cambia.

Vive aquí, junto a `BACKLOG-FYNE.md`, porque `docs/` está cerrado a diez ficheros. Cuando se apruebe,
la decisión entra como **ADR 0083** en `docs/decisions.md` (el último es el 0082) y los pendientes, en
`docs/roadmap.md` (§12 de este plan).

**De dónde sale cada afirmación.** Todo lo que se dice del código viene de leerlo, no de ejecutarlo:
ocho exploraciones de solo lectura más las comprobaciones directas que se listan en §14. Nada se ha
compilado ni probado. Lo que no se pudo confirmar está en §14 y cada fase empieza resolviendo lo suyo.

## Índice

1. [Qué se pide](#1-qué-se-pide)
2. [Reglas duras](#2-reglas-duras)
3. [Lo que hay hoy y condiciona el diseño](#3-lo-que-hay-hoy-y-condiciona-el-diseño)
4. [Decisiones tomadas](#4-decisiones-tomadas)
5. [Arquitectura](#5-arquitectura)
6. [Plan por producto](#6-plan-por-producto)
7. [Empaquetado, CI y distribución](#7-empaquetado-ci-y-distribución)
8. [Pruebas y guardianes](#8-pruebas-y-guardianes)
9. [Fases, orden y tamaño](#9-fases-orden-y-tamaño)
10. [Riesgos](#10-riesgos)
11. [Lo que solo puede hacer el dueño](#11-lo-que-solo-puede-hacer-el-dueño)
12. [Documentación que hay que tocar](#12-documentación-que-hay-que-tocar)
13. [Fuera de alcance](#13-fuera-de-alcance)
14. [Lo comprobado y lo que falta por comprobar](#14-lo-comprobado-y-lo-que-falta-por-comprobar)
15. [Anexo A — Métodos sin camino local hoy](#anexo-a--métodos-sin-camino-local-hoy)
16. [Anexo B — Destinos de red permitidos](#anexo-b--destinos-de-red-permitidos)

---

## 1. Qué se pide

Requisitos del dueño, tal como los dio (2026-11-18):

| # | Requisito |
| --- | --- |
| R1 | Una versión **solo de apps Fyne** para macOS, iOS, Windows, Linux y Android. |
| R2 | **Sin login ni OIDC**, y **sin cifrado**. |
| R3 | **Claves de API opcionales** para conectar IA y similares. |
| R4 | Solo las apps nativas y **todas sus funciones sin conexión**; **sin conexiones a web ni HTTPS**. Solo las versiones «pesadas» de los productos. |
| R5 | **Mail** es un cliente de correo al que se conectan cuentas externas (Outlook y demás). |
| R6 | **Calendar** igual: cliente de calendarios externos. **Rooms**: cliente de Rooms online, Microsoft Teams, Google Meet y Chat, y Slack. |
| R7 | Es una **modalidad adicional**; no sobrescribe lo existente. |
| R8 | **Cast** almacena y publica **en local**, como una galería de vídeos privada local. |
| R9 | Las decisiones abiertas se resuelven con lo que sea lógico para una versión **100 % funcional sin servidores propios**. |

**Cómo se concilian R4 y R5–R6.** «Sin conexiones a web» y «conectar Outlook» solo son compatibles con
esta lectura, que es la que adopta el plan: **la app nunca habla con Sytue ni con ningún servidor por
su cuenta; solo se conecta a los servicios que la persona ha configurado a mano** (una cuenta de
correo, un calendario, un espacio de Slack, un proveedor de IA). Recién instalada y sin configurar
nada, no abre ni una conexión. Es la regla R-RED de §2.

**Nombre de trabajo:** «Sytue Local». Se define en un solo sitio (§5.2) y cambiarlo es barato.

---

## 2. Reglas duras

Son las que no se negocian durante la ejecución. Cada una tiene un guardián en §8.

| Regla | Enunciado |
| --- | --- |
| **R-ADITIVA** | Los 16 binarios de `apps/fyne/cmd/*` no cambian de comportamiento. Todo cambio en código compartido es una **adición** (un método, una interfaz opcional, una opción) o una **extracción que no cambia lo que hace**, demostrada por los tests que ya existen. |
| **R-RED** | Un binario local no contacta con ningún host que la persona no haya configurado. Sin cuentas ni claves, cero conexiones. No hay telemetría, actualizador propio ni «caer a la web». |
| **R-SIN-SYTUE** | Un binario local no enlaza el login OIDC, el cifrado de la suite ni Matrix. Única excepción: el proveedor «Sytue Rooms online» de Rooms, que es una cuenta externa más y solo existe en ese binario (§6.14). |
| **R-CLARO** | Los datos de la persona se guardan **en claro** y en formatos legibles. Los **secretos** (contraseñas de cuentas, tokens, claves de API) van al llavero del sistema; donde no hay llavero, a un fichero con permisos 0600, y se dice en pantalla. |
| **R-CONVIVE** | Las dos ediciones pueden estar instaladas a la vez sin compartir ni pisar nada: identificador, esquema de URL, carpeta de ajustes, caché, llavero y carpeta de datos son distintos. |
| **R-NO-COPIAR** | La plomería se importa (`docs/CONVENTIONS.md` §13.2). Lo que hoy solo vive en el servidor y hace falta en el cliente **se mueve** a `apps/nucleo` y el servidor lo importa desde ahí; no se duplica. |
| **R-NO-CORROMPE** | Un fichero de la persona no se estropea nunca: escritura a temporal y renombrado atómico, versión anterior guardada, y nunca se guarda de vuelta un documento importado con pérdida sin avisar. |
| **R-MEDIR** | Antes de optimizar o de dar por buena una pieza de rendimiento, se mide (AGENTS.md, «medir antes de tocar»). El almacén local empieza con una prueba de carga (§5.6). |

---

## 3. Lo que hay hoy y condiciona el diseño

### 3.1 Cómo arranca hoy una app

Las 16 apps hacen lo mismo en `apps/fyne/cmd/<app>/main.go`:

1. `sesion.Abrir(a, "")` — construye el cliente OIDC, lee la sesión del llavero y crea el cliente del Hub.
2. `cifrado.NewPrivateApplication{Build, Mount}` — exige sesión iniciada y un dispositivo Matrix
   verificado; construye un «repositorio privado» cifrado.
3. `api.NewX(...).WithPrivateData(repo)` — entrega a la pantalla una fachada que ya no hace HTTP.
4. `ui.NuevaBarraSuite(t, s, icono).Montar(...)` — buscador, campana y cuenta, los tres contra el servidor.

Excepciones: `drive` monta su propio controlador, `rooms` no tiene repositorio privado, `photos` monta
dos, y `demo` no tiene sesión.

### 3.2 Las costuras que existen

| Costura | Dónde | Sirve para |
| --- | --- | --- |
| Interfaces `api.*Data` detrás de `WithPrivateData` | `apps/nucleo/api/*_data.go` | 12 productos. De 4 a 32 métodos, sin tipos de Matrix, cifrado ni HTTP. Solo piden un `storage.Scope` válido. |
| Puerto `records.Crypto` bajo el motor de registros | `apps/nucleo/storage/records/repository.go:39-67` | Lo que el motor pide a Matrix. Los tests ya lo satisfacen en memoria (`hub/private_operations_test.go`, `notes/repository_test.go`, `mail/mail_test.go`). |
| Interfaz `mail.Servicio` (51 métodos) | `apps/fyne/apps/mail/buzon.go:47-84` | La pantalla de Mail no depende de `*api.Mail`. |
| Interfaz `rooms.Servicio` (63 métodos) + capacidades opcionales | `apps/fyne/apps/rooms/sala.go:47-115` | La pantalla de Rooms oculta sola lo que el servicio no implementa. |
| `api.New(baseURL, nil, httpc)` | `apps/nucleo/api/client.go:42-49` | Sesión nula y cliente HTTP inyectable: un transporte en proceso puede servir un API entero. |
| IA como función | `func(ctx, api.PeticionIA) (string, error)` | Docus, Diapos, Calcus, Mail, Notes, Rooms y Calendar ya la reciben inyectada. |

### 3.3 Lo que no se puede reutilizar

- **`cifrado.PrivateApplication`**: atado a `*sesion.Suite`, `*mx.Client` y Rooms. Su fábrica lo dice:
  «There is no local, anonymous or nil-lease fallback».
- **`sesion.Suite` tal cual**: `Abrir` crea OIDC, lee el llavero `eu.sytue.one` y escribe en la carpeta
  compartida `sytue`.
- **El «repositorio privado» como almacén**: hoy **no es un almacén local**. El catálogo se
  reconstruye por red desde el historial de Matrix en cada apertura; en disco solo hay un diario de
  escrituras pendientes, cifrado.

### 3.4 Lo que ya es local y no se usa como tal

- Conversión ofimática completa, en Go, en `apps/nucleo/office/model` (importar y exportar docx, odt,
  xlsx, ods, pptx, odp, pdf, iWork, md, html, csv). Pero **abrir un fichero del disco** pasa hoy por
  `POST /api/v1/import` (`apps/fyne/sesion/sesion.go:574`).
- Motor PDF propio (`apps/nucleo/pdf`, unas 55 000 líneas). Tiene escritas funciones que el enrutador
  local no ofrece: comparar, verificar firmas, extraer adjuntos, marca de agua, numeración y firma en
  dos pasos.
- iCalendar, recurrencias, huecos libres y alarmas (`apps/nucleo/calendar/{ical,event,alarm}`); vCard
  y CSV (`apps/nucleo/contacts/card`); MIME (`apps/nucleo/eml`); exportadores e importadores de notas
  (`apps/nucleo/notes/model`); derivadas, recetas y EXIF de fotos (`apps/nucleo/photos/{render,exif}`).

### 3.5 Lo que solo existe en el servidor

| Qué | Dónde | Consecuencia |
| --- | --- | --- |
| Clientes IMAP, SMTP, POP3, Gmail, Graph | `services/integrations/internal/mailbox` | Hay que moverlos a `apps/nucleo` (R-NO-COPIAR). Les falta búsqueda, IDLE, carpetas, borradores y etiquetas. |
| Clientes CalDAV/CardDAV, Google Calendar/People, Graph | `services/integrations/internal/linkedservices` | Igual. El de DAV está limitado a Yahoo y no borra ni sincroniza por cambios. |
| Slack, Teams, Google Chat | `services/integrations/internal/http/external_chat_*.go` | Igual. Sin tiempo real. |
| Proveedores de IA y construcción del prompt | `services/ai/internal/provider` | Igual. |
| Catálogo de plantillas de la ofimática | `services/docs/internal/plantillas` | Igual. |
| Lógica de Bases (fórmulas, validación, CSV) | `services/bases/internal/domain` | Paquetes puros: se mueven. Filtros y orden son SQL: se reescriben en memoria. |
| Lógica del ERP | `services/syerp/internal/{catalogo,domain,verifactu,banca,obligaciones}` (puros) y `store` (Postgres) | Los puros se mueven; el `store` se reescribe sobre SQLite. Es lo más caro del plan. |
| Conversión de vídeo (ffmpeg) | `services/cast/internal/transcoder` | El reproductor nativo solo decodifica WebM con VP9 y Opus. |

El patrón de «mover al núcleo y que el servidor importe» ya existe: `services/bases/.../formula.go`
importa `apps/nucleo/formula`, y `platform/pkg/guiones` reexporta `apps/nucleo/guiones`.

### 3.6 Lo que condiciona el empaquetado

- `scripts/empaquetar.sh` recorre `apps/fyne/cmd/*/FyneApp.toml` a fuego y fija las etiquetas de Go.
  No admite otro árbol, sufijo de identificador ni etiquetas extra.
- Unos veinte guardianes cuentan **exactamente 15 apps** en `apps/fyne/cmd/*` (lista en §7.1).
- La única variante de compilación es la etiqueta `tienda` (Mac App Store).
- Los datos no van por identificador de app: ajustes en `<config>/sytue`, caché en `<caché>/sytue`,
  llavero `eu.sytue.one`.

---

## 4. Decisiones tomadas

Resueltas con el criterio de R9. «Descartado» dice qué alternativa se miró y por qué no.

### 4.1 Forma de la modalidad

| # | Decisión | Motivo | Descartado |
| --- | --- | --- | --- |
| D-01 | Las apps locales son **binarios aparte**, con sus puntos de entrada en **`apps/fyne/cmd-local/<app>/`**. | Los veinte guardianes que cuentan 15 apps solo miran `cmd/*`; un árbol hermano no los toca (R-ADITIVA). | Directorios nuevos en `cmd/` (rompe los guardianes). Variante por etiqueta dentro del mismo `main.go` (los ficheros etiquetados no los compila, prueba ni analiza ningún gate, y el guardián que recorre el AST de `cmd/*` vería dos `main`). |
| D-02 | Etiqueta de compilación **`edicionlocal`**, obligatoria en `cmd-local`, que fija en compilación el espacio de datos y el llavero. | Garantía en compilación de R-CONVIVE: un binario local no puede abrir los datos de la edición conectada ni por error. Precedente: `tienda`. | Elegir el espacio de datos en tiempo de ejecución (una llamada olvidada lo deja escribiendo en `sytue`). |
| D-03 | Mismo módulo (`apps/fyne`) y mismo núcleo (`apps/nucleo`); no hay tercer módulo. | Las pantallas se comparten y los gates de pre-push ya cubren ambos módulos. | Módulo `apps/local` propio (aísla más, pero queda fuera del hook y del CI, y obliga a duplicar el cableado de gates). |
| D-04 | **Las pantallas son las mismas.** Lo que cambia es lo que se les enchufa. | R-NO-COPIAR y «una mejora, dos superficies» (§13.6). | Copiar `apps/fyne/apps/*`. |
| D-05 | Identidad propia: `eu.sytue.local.<app>`, esquema `sytuelocal-<clave>`, ejecutable `sytue-local-<app>`. | R-CONVIVE. El esquema no empieza por `sytue-` a propósito: `api/abrir.go:317` y `sesion/widgets.go:66` deciden por ese prefijo. El guion del ejecutable hace que `escritorio.AppDeEjecutable` no lo tome por una app hermana. | `sytue-local-<clave>` como esquema (cae en los `HasPrefix("sytue-")`). |

### 4.2 Datos

| # | Decisión | Motivo | Descartado |
| --- | --- | --- | --- |
| D-06 | Los repositorios de dominio (`office`, `notes`, `contacts`, `calendar`, `photos`, `bases`, `hub`) se **reutilizan enteros** sobre un **puerto local** que implementa `records.Crypto` en disco y en claro. | Cero cambios en el motor y en los repositorios; los tests ya demuestran que el motor corre sobre un puerto que no es Matrix. Solo `office.Repository` son 1.858 líneas de operaciones, versiones y comentarios que no hay que reescribir. | Interfaz nueva por encima del motor (obliga a tocar nueve repositorios y a emular 19 métodos con tipos del motor). Implementaciones nuevas de cada `*Data` (duplica la lógica de dominio). Un interruptor «en claro» dentro del motor (un fallo ahí haría que la edición conectada escribiera sin cifrar). |
| D-07 | El puerto local guarda en **SQLite** (registros, JSON legible) y en **ficheros en claro** (binarios). | `mattn/go-sqlite3` ya es dependencia de `apps/nucleo` (`go.mod:33`) y compila en las cinco plataformas con el cliente Matrix. Transacciones, acceso aleatorio y resistencia a cortes. | JSON por líneas (sin transacciones entre filas). Un fichero por registro (decenas de miles de ficheros). Una dependencia nueva (bbolt, SQLite en Go puro). |
| D-08 | El diario transitorio del motor sigue sellado, con su clave en un fichero 0600 del espacio local, inyectada con `offline.OpenWithSecrets`. | El motor exige `Journal.Encrypted()` y esa exigencia protege a la edición conectada; así no se toca. El diario solo retiene datos unos instantes, porque el puerto local publica en el acto. Los **datos** quedan en claro (R-CLARO). | Depender del llavero (no existe en un Linux sin Secret Service). Un diario que diga estar sellado y no lo esté. |
| D-09 | **Drive, Cast y Mail no usan el motor**: llevan implementación local propia. | Su unidad natural es un fichero de la persona (un documento, un vídeo de varios GB, un `.eml`), que debe quedar con su nombre y en una carpeta visible, no como un binario dentro de un almacén. Es variación de dominio, no de accidente. | Guardar vídeos y carpetas enteras como binarios del motor. |
| D-10 | El **ERP local se sirve con un transporte en proceso** que implementa `/api/v1` sobre su almacén; `api.ERP` y la pantalla no cambian. | Unos 60 métodos del cliente no tienen camino local; añadirlos uno a uno a `ERPData` es más superficie que servir las rutas. Los tests de la pantalla ya corren contra un servicio HTTP falso. No abre ningún socket, así que no rompe R-RED. | Ampliar `ERPData` con 60 métodos. |
| D-11 | Una sola **carpeta de datos** visible y elegible (por defecto `~/Sytue Local`), con todo lo de la persona dentro. | Una carpeta que copiar, mover o sincronizar con lo que la persona quiera. Sin servidor, la copia de seguridad es suya. | Repartir los datos por las carpetas de aplicación de cada sistema. |
| D-12 | **Sin cuadernos cifrados** en Notepads y sin bloqueo biométrico de sesión. | R2. Se pierde: contraseña por cuaderno, desbloqueo con huella y leer cuadernos cifrados en la web. La protección en reposo queda en el cifrado de disco del sistema, y se documenta. | Mantener el cifrado opcional (contradice R2 y arrastra código de cifrado al binario). |

### 4.3 Red, cuentas e IA

| # | Decisión | Motivo | Descartado |
| --- | --- | --- | --- |
| D-13 | **Portero de red**: todo cliente de red de un binario local se crea a través de un paquete que solo deja pasar los destinos derivados de la configuración de la persona. `http.DefaultTransport` y `api.Transporte` se sustituyen por uno que lo deniega todo. | Hoy no hay un único punto de paso del HTTP, así que la garantía de R-RED se construye en tres capas: no enlazar lo prohibido, denegar por defecto y pasar por el portero. | Confiar en `SYTUE_SOLO_LECTURA` (no cubre los clientes de `api`). |
| D-14 | **Sin actualizador propio.** Las actualizaciones llegan por las tiendas o por descarga manual. | Un actualizador necesita un servidor propio (R9) y una conexión no pedida (R-RED). | Consultar un manifiesto firmado en una URL fija. |
| D-15 | **IA con clave propia**: el cliente habla directamente con el proveedor. Tres familias: compatible con OpenAI (cubre OpenAI, Mistral, Scaleway, OpenRouter y **modelos locales** con Ollama o LM Studio), Anthropic y Google Gemini por su extremo compatible. | R3. Los modelos locales permiten IA sin ninguna conexión a Internet. El orden de los preajustes pone primero lo local y lo europeo (§0.5 de CONVENTIONS). | Pasar por el servicio de IA de Sytue. |
| D-16 | **Cuentas externas sincronizadas a un almacén local**, no consultadas en vivo: se lee y se escribe en local y un sincronizador reconcilia. | R4: todo debe funcionar sin conexión. Con la red caída se lee el correo descargado, se redacta, se edita el calendario; al volver, se envía. | Cliente en vivo como el diálogo actual de cuentas externas (sin red no hay nada). |
| D-17 | Las **cuentas son de toda la edición**, no de cada app: una cuenta de Microsoft da correo, calendario, contactos y Teams. En escritorio se configuran una vez. | Evita pedir la misma cuenta cuatro veces. | Cuentas por app. En **móvil**, la primera entrega sí es por app (compartir entre apps separadas pide App Group en Apple y un proveedor firmado en Android: §9, fase F11). |
| D-18 | **OAuth de cliente público con PKCE y redirección a `127.0.0.1`**, reutilizando `auth.WaitLoopback`. Los identificadores de cliente son configuración de compilación; si faltan, esa opción no se ofrece. Siempre existe la vía avanzada «usar mi propio identificador de cliente». | No hay servidor que guarde un secreto de cliente. Los registros en Microsoft, Google y Slack son del dueño (§11). | Cliente confidencial (necesita servidor). |
| D-19 | **Outlook y Microsoft 365** por OAuth desde el primer día (IMAP y SMTP con XOAUTH2). **Gmail**, primero con contraseña de aplicación; OAuth cuando Google verifique la app. | Microsoft retiró la autenticación básica, así que sin OAuth no hay Outlook, que es el ejemplo de R5. El permiso de correo de Google es «restringido» y exige una auditoría de seguridad externa para distribuir la app. | Empezar solo con IMAP y contraseña (deja fuera Outlook). |
| D-20 | **Slack** con «app propia del espacio»: la persona instala en su espacio una app interna con un manifiesto que damos; tiempo real por Socket Mode. | Desde el 2025-05-29 Slack limita `conversations.history` y `conversations.replies` a **1 petición por minuto y 15 mensajes** para apps distribuidas fuera de su Marketplace (comprobado en su documentación, §14). Una app interna del espacio no tiene ese límite. | Una app de Sytue distribuida (inservible con ese límite salvo que entre en el Marketplace: decisión del dueño, §11). |
| D-21 | Las **reuniones de Teams y Meet** se abren en su aplicación oficial; desde la app se crean y se listan los enlaces. | Ninguno de los dos ofrece a terceros un punto de conexión de medios; no hay cliente posible. | Un cliente propio de medios. |
| D-22 | **Google Chat**: enviar siempre; **leer solo con cliente OAuth propio** de la organización de Google Workspace. | Leer exige un permiso restringido con auditoría externa; una app «interna» de un Workspace está exenta. | Prometer lectura a cualquier cuenta. |

### 4.4 Productos

| # | Decisión | Motivo | Descartado |
| --- | --- | --- | --- |
| D-23 | Los editores trabajan con **biblioteca local** (lo que ya enseña la portada) **y con ficheros**: abrir del disco, «Guardar como…», guardar de vuelta en el fichero y un **formato nativo sin pérdida** (`.docus`, `.calcus`, `.diapos`). | Una ofimática de escritorio sin «Guardar como» no es funcional. El formato nativo existe porque ir y volver por docx u odt puede perder lo que el modelo no representa (R-NO-CORROMPE). | Solo biblioteca (obliga a exportar para sacar cualquier documento). |
| D-24 | **CertiPDF edita el PDF en su sitio**, con versión anterior guardada. | Editar un PDF no es una conversión con pérdida, y copiar cada PDF a una carpeta interna no tiene sentido sin Drive en la nube. | Copiarlo a una carpeta «PDF» como hoy. |
| D-25 | **Drive es un gestor de ficheros** sobre carpetas del disco, con papelera, versiones, favoritos y recientes en una carpeta auxiliar `.sytue`. | La interfaz `DriveData` ya tiene esa forma. | Un almacén opaco de ficheros. |
| D-26 | **Cast**: los vídeos son ficheros en una carpeta; «publicar» es un estado local que los enseña en la portada «Explorar» **de esa app y en ese equipo**. No se sirve a la red local. | R8 («privada») y R-RED. | Servir la galería por HTTP o DLNA en la red de casa (sería un servidor; queda fuera, §13). |
| D-27 | **Vídeo con los decodificadores del sistema operativo**; mientras no estén, lo que no sea WebM se abre con el reproductor del sistema. | El reproductor propio solo entiende WebM con VP9 y Opus, y no hay decodificador de H.264 ni de AAC en el repo. Usar los del sistema evita además las patentes de H.264 y HEVC. Precedentes: `llamada/camara` ya usa esos marcos para capturar, y Diapos ya abre medios con el reproductor del sistema. | Empaquetar ffmpeg (licencia GPL o LGPL, fuera de lo que admite CONVENTIONS §12). Convertir todo a WebM al importar (lento, y necesita igualmente decodificar). |
| D-28 | **La barra de arriba cambia**: marca, buscador de la propia app y un menú de **Ajustes** donde hoy está la cuenta. Sin campana global. | No hay cuenta, ni Hub, ni búsqueda federada. Es una excepción nueva a CONVENTIONS §13.6, que va escrita en el ADR. | Dejar los tres controles inertes. |
| D-29 | **VERI\*FACTU va al final y con revisión legal previa**; hasta entonces el ERP local emite facturas sin él y lo dice. | La cadena y el envío a la AEAT son obligaciones legales con declaración responsable del fabricante; un programa local con la base de datos en claro es un sistema distinto del que hoy se declara. | Portarlo tal cual. |
| D-30 | **Perfil local**: un nombre y, si se quiere, un correo, que firman comentarios, cambios y anotaciones. | Varias pantallas piden «quién soy» y hoy lo sacan del token. | Dejarlo vacío. |

---

## 5. Arquitectura

### 5.1 Qué se añade y dónde

```
apps/
├── nucleo/                              (módulo existente)
│   ├── local/                           ← NUEVO: lo local que no es del toolkit
│   │   ├── espacio/                     rutas de la edición (ajustes, caché, datos), perfil
│   │   ├── almacen/                     el puerto local de records.Crypto (SQLite + ficheros)
│   │   ├── sqlite/                      abrir, WAL, migraciones versionadas
│   │   ├── red/                         el portero: clientes HTTP y marcadores TCP con lista blanca
│   │   ├── secretos/                    llavero `eu.sytue.local` con reserva a fichero 0600
│   │   ├── archivos/                    api.DriveData sobre el sistema de ficheros (Drive, CertiPDF)
│   │   ├── videoteca/                   api.CastData sobre carpeta + índice
│   │   ├── correo/                      mail.Servicio: índice, caché .eml, bandeja de salida, hilos
│   │   ├── agenda/                      sincronizador de calendarios y contactos externos
│   │   ├── chat/                        rooms.Servicio agregado por cuenta
│   │   ├── erp/                         almacén SQLite del ERP y su transporte en proceso
│   │   ├── avisos/                      avisos programados (recordatorios)
│   │   └── copia/                       copia de seguridad y restauración
│   ├── cuentas/                         ← NUEVO, MOVIDO del servidor (lo importan los dos)
│   │   ├── modelo/                      cuenta, servicio, capacidades, estado
│   │   ├── oauth/                       cliente público PKCE + loopback
│   │   ├── correo/                      IMAP, SMTP, POP3, Gmail, Graph  (de integrations/mailbox)
│   │   ├── dav/                         CalDAV, CardDAV                 (de linkedservices/dav.go)
│   │   ├── nube/                        Google Calendar/People, Graph   (de linkedservices)
│   │   └── chat/                        Slack, Teams, Google Chat       (de http/external_chat_*)
│   ├── ia/proveedor/                    ← MOVIDO de services/ai/internal/provider
│   ├── office/plantillas/               ← MOVIDO de services/docs/internal/plantillas
│   ├── bases/dominio/                   ← MOVIDO de services/bases/internal/domain (lo puro)
│   └── erp/{catalogo,dominio,verifactu,banca,obligaciones}/   ← MOVIDO de services/syerp/internal
└── fyne/                                (módulo existente)
    ├── anfitrion/                       ← NUEVO, EXTRAÍDO de sesion: lo que no es sesión
    ├── local/                           ← NUEVO: la cáscara de una app local
    │   ├── local.go                     local.Abrir: el sustituto de sesion.Abrir
    │   ├── montaje/                     abrir el almacén y montar la pantalla (sustituye a cifrado)
    │   ├── ajustes/                     pantallas: apariencia, perfil, cuentas, IA, datos, copia
    │   └── catalogo/                    textos propios de la edición, 26 idiomas
    ├── video/sistema/                   ← NUEVO: decodificadores del sistema por plataforma
    └── cmd-local/<app>/                 ← NUEVO: 15 puntos de entrada + FyneApp.toml + Icon.png
```

Nada de `apps/fyne/cifrado`, `apps/nucleo/matrix`, `apps/nucleo/auth` (OIDC) ni `apps/fyne/llamada`
entra en un binario local, salvo en Rooms (§6.14).

### 5.2 Identidad de la edición

Una tabla, en `apps/nucleo/local/espacio/edicion.go`, es la única fuente:

| Dato | Valor |
| --- | --- |
| Nombre de la edición | `Local` (lo que se añade al nombre de cada producto) |
| Prefijo de identificador | `eu.sytue.local.` |
| Prefijo de esquema | `sytuelocal-` |
| Prefijo de ejecutable | `sytue-local-` |
| Carpeta de ajustes y caché | `sytue-local` |
| Servicio de llavero | `eu.sytue.local` |
| Grupo de apps de Apple (fase F11) | `group.eu.sytue.local` |
| Clave de instalador en Windows | `eu.sytue.local.bundle.<bundle>` |
| Bundle de escritorio | `local` → instalador «Sytue Local» con las 15 apps |

Las quince apps (todas menos `demo`):

| `cmd-local/` | Nombre | Identificador | Esquema | Extensiones que abre |
| --- | --- | --- | --- | --- |
| `hub` | Sytue Local | `eu.sytue.local.hub` | `sytuelocal-one` | — |
| `docus` | Sytue Docus Local | `eu.sytue.local.docus` | `sytuelocal-docs` | docus, docx, dotx, odt, ott, pages, txt, md, html |
| `calcus` | Sytue Calcus Local | `eu.sytue.local.calcus` | `sytuelocal-sheets` | calcus, xlsx, xltx, ods, ots, numbers, csv, tsv |
| `diapos` | Sytue Diapos Local | `eu.sytue.local.diapos` | `sytuelocal-slides` | diapos, pptx, potx, odp, otp, key |
| `certipdf` | Sytue CertiPDF Local | `eu.sytue.local.certipdf` | `sytuelocal-pdf` | pdf |
| `drive` | Sytue Drive Local | `eu.sytue.local.drive` | `sytuelocal-drive` | — |
| `notes` | Sytue Notepads Local | `eu.sytue.local.notes` | `sytuelocal-notes` | onex, enex |
| `photos` | Sytue Photos Local | `eu.sytue.local.photos` | `sytuelocal-photos` | jpg, jpeg, png, gif, webp, avif, heic |
| `cast` | Sytue Cast Local | `eu.sytue.local.cast` | `sytuelocal-cast` | webm, mkv, mp4, m4v, mov |
| `mail` | Sytue Mail Local | `eu.sytue.local.mail` | `sytuelocal-mail` (+ `mailto:`, a elección de la persona) | eml |
| `calendar` | Sytue Calendar Local | `eu.sytue.local.calendar` | `sytuelocal-calendar` | ics |
| `contacts` | Sytue Contacts Local | `eu.sytue.local.contacts` | `sytuelocal-contacts` | vcf |
| `rooms` | Sytue Rooms Local | `eu.sytue.local.rooms` | `sytuelocal-rooms` | — |
| `bases` | Sytue Bases Local | `eu.sytue.local.bases` | `sytuelocal-bases` | — |
| `erp` | Sytue ERP Local | `eu.sytue.local.erp` | `sytuelocal-erp` | — |

Las apps locales **no se hacen predeterminadas solas** para ninguna extensión (la conectada lo hace una
vez con `predeterminarUnaVez`): lo ofrecen una vez y solo si la persona acepta. Así instalar la edición
local no le quita los `.docx` a la conectada.

Los iconos son los de cada producto con una marca de edición. Se generan con
`scripts/gen-iconos-fyne.sh`, ampliado con un parámetro.

### 5.3 Etiqueta de compilación y espacio de datos

La etiqueta `edicionlocal` elige en compilación una sola constante por paquete, con el patrón de
`tienda.go` / `tienda_no.go`:

| Paquete | Fichero nuevo | Qué fija con la etiqueta |
| --- | --- | --- |
| `nucleo/sistema/escritorio` | `edicion_local.go` / `edicion_local_no.go` | `const EdicionLocal = true / false` |
| `nucleo/ajustes` | `espacio_local.go` / `espacio_local_no.go` | carpeta `sytue-local` en vez de `sytue`; en Apple, sin grupo `group.eu.sytue.suite` |
| `nucleo/auth` (solo llavero genérico) | `servicio_local.go` / `servicio_local_no.go` | servicio `eu.sytue.local`; alias de Android `eu.sytue.local.*` |
| `fyne/sesion` (rutas de caché y socket) | por `ajustes`, sin fichero propio | `<caché>/sytue-local/…` |

Es **defensa en profundidad**. Los paquetes nuevos de `local/` no leen estos globales: reciben sus
rutas inyectadas desde `espacio.Abrir()`, así que sus tests corren sin la etiqueta. La etiqueta protege
al código compartido heredado que sí los lee.

`local.Abrir` falla al arrancar si `!escritorio.EdicionLocal`: un `cmd-local` compilado sin la etiqueta
no llega a pintar nada.

**Dónde queda cada cosa** (escritorio; en móvil, dentro del contenedor de la app):

| Qué | Ruta | En claro |
| --- | --- | --- |
| Ajustes de apariencia e idioma | `<config>/sytue-local/ajustes.json` | sí |
| Perfil, carpeta de datos elegida | `<config>/sytue-local/perfil.json` | sí |
| Cuentas (sin secretos) | `<config>/sytue-local/cuentas.json` | sí |
| IA (proveedor, modelo; sin la clave) | `<config>/sytue-local/ia.json` | sí |
| Secretos | llavero `eu.sytue.local`; si no hay, `<config>/sytue-local/secretos/` (0600) | llavero / fichero 0600 |
| Clave del diario transitorio | `<config>/sytue-local/diario.clave` (0600) | — |
| Cachés (miniaturas, derivadas, diario) | `<caché>/sytue-local/` | sí; se puede borrar |
| Socket de instancia única | `<caché>/sytue-local/<id>.sock` | — |
| **Datos de la persona** | `~/Sytue Local/` (elegible) | sí |

Carpeta de datos:

```
~/Sytue Local/
├── Drive/                      ficheros de la persona, tal cual
│   └── .sytue/                 indice.sqlite, papelera/, versiones/
├── Vídeos/                     vídeos de Cast, con su nombre
│   └── .sytue/                 videoteca.sqlite, pósteres/
├── Correo/<cuenta>/            mensajes descargados (.eml) + indice.sqlite
├── Datos/<producto>/           docs, sheets, slides, notes, photos, contacts, calendar, bases, hub, erp
│   ├── registros.sqlite        los registros del producto, JSON legible
│   └── objetos/ab/cd…          binarios en claro (originales de fotos, adjuntos, contenidos grandes)
└── Copias/                     copias de seguridad
```

### 5.4 El anfitrión: lo que sustituye a `sesion.Suite`

`sesion.Suite` mezcla dos cosas. Se separan sin cambiar lo que hace:

- **Lo que es sesión** (OIDC, llavero, Hub, presencia, widgets con datos de la cuenta, actualizador,
  bloqueo, consentimientos, Drive en el sistema): se queda en `sesion`.
- **Lo que no** (instancia única y reenvío de URL, recibir URL y ficheros, soltar ficheros, ciclo de
  vida y escena, apariencia e idioma sobre `ajustes`, bandeja, pantalla encendida, teclado, imagen
  dentro de imagen): se **extrae** a `apps/fyne/anfitrion`, y `sesion.Suite` pasa a delegar en él.

Regla de la extracción: una pieza se mueve solo si los tests de `sesion` siguen en verde sin tocarlos.
Si una pieza no se puede mover sin cambiar el comportamiento de la conectada —porque lleva dentro el
consentimiento de subida o la caída a la web—, la cáscara local lleva la suya, y se anota como
variación de dominio con su motivo.

Superficie de la cáscara local (`apps/fyne/local`), con los nombres de `sesion` donde hacen lo mismo
para que los `main.go` locales se parezcan a los actuales:

```go
// Abrir prepara una app local. No habla con la red, no lee ningún llavero de sesión.
func Abrir(a fyne.App) *Aplicacion

func (l *Aplicacion) Idioma() i18n.Locale
func (l *Aplicacion) Poner(cambio func(*ajustes.Ajustes)) error   // apariencia e idioma
func (l *Aplicacion) Perfil() Perfil                               // nombre, correo, id de instalación
func (l *Aplicacion) Espacio() espacio.Rutas                       // ajustes, caché, datos
func (l *Aplicacion) Alcance(producto string) storage.Scope        // Tenant "local", Subject = id de instalación

func (l *Aplicacion) AlAbrirURL(f func(*url.URL))
func (l *Aplicacion) RutaPropia(f func(*url.URL) bool)
func (l *Aplicacion) AlAbrirFichero(producto string, cargar func(context.Context, api.Apertura) error)
func (l *Aplicacion) AceptarFicheros(w fyne.Window, titulo, texto string)
func (l *Aplicacion) CuandoHayaEscena(f func())
func (l *Aplicacion) AlFondo(f func())
func (l *Aplicacion) AlVolver(f func())
func (l *Aplicacion) Arrancar(ctx context.Context)

func (l *Aplicacion) Asistir() func(context.Context, api.PeticionIA) (string, error) // nil sin clave
func (l *Aplicacion) Cuentas() *cuentas.Registro
func (l *Aplicacion) Red() *red.Portero
func (l *Aplicacion) AbrirEn(producto, ruta string) error          // esquema sytuelocal-, sin caída a la web
```

Y el sustituto de `cifrado.NewPrivateApplication` (`local/montaje`), que no tiene estados de bloqueo
ni de verificación porque no hay nada que desbloquear:

```go
type Config[R any] struct {
    Aplicacion *local.Aplicacion
    Producto   string
    Abrir      func(context.Context, almacen.Recursos) (R, error)       // construye el repositorio
    Montar     func(context.Context, R) (Vista, error)                  // construye la pantalla
}
type Vista struct { Objeto fyne.CanvasObject; Soltar func(); Esperar func(context.Context) error }
```

Estados: abriendo → listo, o fallo con «Reintentar». Al cerrar la ventana: `Soltar`, vaciar el diario
y cerrar el almacén.

### 5.5 Barra local y Ajustes

`ui.BarraSuite` no tiene hoy forma de quitar buscador, campana o cuenta. Se le añade, sin tocar la
actual, un segundo constructor que reutiliza las mismas piezas de marca y de geometría:

```go
// NuevaBarraLocal: marca de la app + buscador de la propia app (opcional) + botón de Ajustes.
func NuevaBarraLocal(t *i18n.Translator, a Anfitrion, p iconos.Producto) *BarraSuite
```

`ui.Anfitrion` es la interfaz mínima que la barra necesita (leer y poner apariencia e idioma, saber
cuándo cambian). `*sesion.Suite` y `*local.Aplicacion` la cumplen las dos.

- **Buscador:** llama a un `AlBuscar` de la propia app. La búsqueda entre todas las apps locales es
  una pieza posterior del Hub (§6.15).
- **Campana:** no hay. Los recordatorios de Calendar y Notepads salen como avisos del sistema.
- **Ajustes** (engranaje, donde hoy va el avatar): apariencia, color, forma, idioma, y «Ajustes de
  Sytue Local…».

Los guardianes de píxel de la barra actual no se tocan. La barra local lleva los suyos, con el patrón
de `ui/boton_test.go`.

**Pantallas de Ajustes** (`apps/fyne/local/ajustes`). Es un paquete compartido: van en el Hub y, en
móvil, en cada app, porque allí se instalan por separado.

| Sección | Contenido |
| --- | --- |
| Apariencia e idioma | Tema, color, forma, idioma. Lo que ya existe, sin la opción «Organización». |
| Perfil | Nombre y correo que firman comentarios, cambios y anotaciones. |
| Cuentas | Lista de cuentas externas, estado de cada una, añadir, reconectar, quitar; qué servicios usa cada app. |
| Inteligencia artificial | Proveedor, dirección, modelo, clave, «Probar». Aviso claro de a quién se envía el texto. Apagada por defecto. |
| Datos | Carpeta de datos (ver, cambiar con traslado), espacio ocupado por app, vaciar cachés. |
| Copia de seguridad | Crear copia, restaurar, exportar todo en formatos abiertos. |
| Red | Lista de todos los destinos a los que la app puede conectarse ahora mismo y por qué (sale del portero). Interruptor «Sin conexión»: corta todo. |
| Acerca de | Versión, licencias de terceros, dónde están los datos. |

### 5.6 El almacén local

**Qué es.** Un paquete, `apps/nucleo/local/almacen`, que implementa `records.Crypto` (y con él
`storage.RecordStore`) guardando en disco y en claro. El motor `storage/records` y los repositorios de
dominio no se modifican.

**Piezas que entrega** (`almacen.Recursos`), una por cada cosa que `records.Config` exige:

| Campo de `records.Config` | Hoy lo da | En local lo da |
| --- | --- | --- |
| `Crypto` | `*matrix.Client` | `almacen.Puerto` |
| `Gateway *api.Drive` | el Drive cifrado | `&api.Drive{}` (el motor solo se lo pasa a `Crypto`; así lo hacen ya los tests) |
| `Source` | la migración del servicio (HTTP) | `almacen.SinOrigen`: dice «migrado y vacío» sin red |
| `Journal` | `offline.Store` con clave del llavero | `offline.OpenWithSecrets(dir, almacen.ClaveEnFichero)` (D-08) |
| `Scope` | inquilino y sujeto del token | `{Tenant: "local", Subject: <id de instalación>, Product}` |
| `WorkLease`, `BindPublicationContext`, `Context` | la fábrica de `cifrado` | un arrendamiento local de vida igual a la ventana |

**Esquema del puerto** (`Datos/<producto>/registros.sqlite`, modo WAL, un escritor):

```sql
CREATE TABLE registro (
  seq       INTEGER PRIMARY KEY AUTOINCREMENT,
  evento    TEXT NOT NULL UNIQUE,   -- referencia sintética "$<uuid>"
  objeto    TEXT NOT NULL,          -- id del objeto del dominio
  rol       TEXT NOT NULL,
  carga     TEXT NOT NULL,          -- el registro, JSON legible
  creado    TEXT NOT NULL
);
CREATE INDEX registro_objeto ON registro(objeto, seq);
CREATE TABLE enlace  (objeto TEXT PRIMARY KEY, evento TEXT NOT NULL);   -- StorageBindings
CREATE TABLE fichero (evento TEXT PRIMARY KEY, sha256 TEXT NOT NULL, tamano INTEGER NOT NULL,
                      tipo TEXT NOT NULL, nombre TEXT NOT NULL);
CREATE TABLE meta    (clave TEXT PRIMARY KEY, valor TEXT NOT NULL);     -- versión de esquema
```

- Las referencias que el motor exige con forma de Matrix (`!sala`, `$evento`) son sintéticas:
  `!local:<producto>` y `$<uuid>`.
- Los binarios (`UploadStorageFileAt`, y el JSON de más de 16 KiB) se escriben en
  `objetos/<2>/<2>/<sha256>` con temporal y renombrado, y se sirven con `io.ReaderAt`. Sin cifrar.
- `StorageObjectHistory` pagina `registro` por `seq`. Es la fuente del catálogo en cada apertura.

**Prueba de carga antes de construir encima (R-MEDIR).** El motor reconstruye el catálogo entero en
memoria al abrir y los repositorios lo recorren. Se mide, en un runner del CI y con el puerto real:

| Carga | Qué se mide | Umbral para seguir |
| --- | --- | --- |
| 5.000 notas en 50 cuadernos | apertura en frío, memoria, guardar una nota | abrir < 1,5 s; guardar < 50 ms |
| 50.000 fotos, 200 álbumes | apertura, memoria, una página de la línea de tiempo | abrir < 4 s; página < 100 ms; memoria < 400 MB |
| 20.000 contactos | apertura, búsqueda | abrir < 2 s; buscar < 100 ms |
| 2.000 documentos con 50 versiones | apertura, guardar | abrir < 2 s; guardar < 80 ms |
| historial de 1.000.000 de registros | apertura | define si hace falta compactar |

Los umbrales son la propuesta de partida; se ajustan con la primera medición, y se anota.

**Si no se cumplen** hay dos salidas, por este orden:

1. **Compactar** el historial al cerrar: dejar solo la última versión de cada objeto y las que el
   dominio enseña como historial. Solo vale si el motor acepta un historial sin los padres antiguos;
   es lo primero que comprueba la prueba.
2. **Plan B, solo para el producto que falle:** implementación propia de su `*Data` sobre SQLite con
   índices, reutilizando sus paquetes puros (`photos/render`, `notes/model`…). Es lo que D-09 ya hace
   para Drive, Cast y Mail.

**Semántica que el puerto debe respetar.** Sale de los tests de contrato, que corren el mismo juego
contra el puerto en memoria de los tests actuales y contra el puerto local (§8): escrituras idempotentes
por identificador, lectura de lo recién escrito, orden total por `seq`, resistencia a un corte a mitad
de escritura y cierre limpio con el diario vacío.

### 5.7 Lo que hay que añadir a `api`

Muchas llamadas que las pantallas hacen **no tienen camino local** y hoy fallan cerradas con la
fachada privada. La lista completa por producto está en el [Anexo A](#anexo-a--métodos-sin-camino-local-hoy).

Patrón único, el que `api` ya usa (`DriveFileReplacer`, `NotesViewsData`, `PhotosAlbumData`):

1. Una **interfaz opcional** nueva por grupo de funciones (`DocsTrashData`, `NotesTasksData`…).
2. En el método de `api.X`, una rama: si `data` la cumple, se delega; si no, sigue fallando cerrado
   como hoy.
3. La implementación va en el **repositorio de dominio** (`office.Repository`, `notes.Repository`…),
   así que la edición conectada **también gana** la función. Es una adición.

Excepciones: Mail implementa `mail.Servicio` directamente, Rooms `rooms.Servicio`, y el ERP va por
transporte en proceso (D-10).

Cuatro consumidores reciben hoy un `*api.IA` concreto (CertiPDF, Photos, Contacts, Drive). Se cambia
el tipo del campo por una interfaz de un método, que `*api.IA` ya cumple:

```go
type Asistente interface { Asistir(context.Context, PeticionIA) (string, error) }
```

### 5.8 Red: el portero y sus guardianes

`apps/nucleo/local/red`:

```go
type Proposito string // "correo", "calendario", "contactos", "chat", "ia", "oauth",
                      // "sello-de-tiempo", "suscripcion-ics", "imagen-remota", "aeat", "baja-de-lista"

func (p *Portero) Cliente(pr Proposito) *http.Client          // HTTP(S) con lista blanca
func (p *Portero) Marcar(ctx, pr, red, direccion) (net.Conn, error) // IMAP, SMTP, POP3
func (p *Portero) Destinos() []Destino                         // lo que enseña Ajustes → Red
func (p *Portero) SinConexion(si bool)                         // el interruptor
```

- La lista blanca **se deriva de la configuración**: los hosts de cada cuenta, del proveedor de IA
  elegido, de la autoridad de sellado si se puso una, y los de inicio de sesión del proveedor
  mientras dura el alta. No hay ningún host fijo que valga sin configuración.
- Solo TLS, salvo `localhost` (modelos locales) y lo que la persona marque a mano (un servidor IMAP
  de casa).
- `local.Abrir` sustituye `http.DefaultTransport` y `api.Transporte` por uno que **lo deniega todo**.
  Cualquier cliente heredado que no pase por el portero falla cerrado.
- **Enlaces que la persona pulsa** (una web en un correo, el mapa de una foto) se abren en el
  navegador del sistema: la app no hace la petición.
- **Imágenes remotas** (correo, notas de imagen): bloqueadas por defecto; se cargan por mensaje o por
  remitente, a petición.

Los destinos previstos por función están en el [Anexo B](#anexo-b--destinos-de-red-permitidos).

### 5.9 IA con clave propia

**Mover, no copiar.** `services/ai/internal/provider` (los adaptadores `openai.go`, `anthropic.go`,
`responses.go`, y la construcción del prompt `SystemPrompt` y `BuildUserMessage`) pasa a
`apps/nucleo/ia/proveedor`; el servicio lo importa. Sin cambio de comportamiento en el servidor,
demostrado por sus tests.

**El adaptador local:**

```go
// Local traduce una PeticionIA al prompt de siempre y la envía al proveedor de la persona.
type Local struct { Proveedor proveedor.Cliente; Modelo string; Vision bool }
func (l *Local) Asistir(ctx context.Context, q api.PeticionIA) (string, error)
```

Mismo contrato que `api.IA.Asistir`: una petición, un texto. Sin flujo, como hoy.

| Preajuste (en este orden) | Familia | Dirección por defecto | Nota |
| --- | --- | --- | --- |
| Modelo en este equipo (Ollama, LM Studio) | compatible OpenAI | `http://localhost:11434/v1` | sin Internet y sin clave |
| Mistral | compatible OpenAI | `https://api.mistral.ai/v1` | europeo |
| Scaleway | compatible OpenAI | la de Scaleway | europeo |
| OpenAI | compatible OpenAI | `https://api.openai.com/v1` | |
| Anthropic | Anthropic | `https://api.anthropic.com` | |
| Google Gemini | compatible OpenAI | la de Google | |
| Otro compatible | compatible OpenAI | la que escriba la persona | |

- La **clave** va al llavero. La dirección y el modelo, a `ia.json`.
- **Visión** (describir, preguntar y editar fotos; transcribir un PDF escaneado) solo se ofrece si el
  modelo elegido la declara.
- **Primera vez por app:** un aviso que dice qué se envía y a quién. CertiPDF tiene hoy una barrera
  (`CheckRemoteProcessing`) que bloquea la IA con datos privados; en local se sustituye por ese aviso.
- **Sin clave, la IA no aparece**: todas las pantallas ya ocultan sus botones con la función a `nil`.
- La IA del ERP (informes, planes, rellenar fichas) pasa hoy por el servicio del ERP con
  `platform/pkg/asistente`; se porta con el ERP (§6.13).

### 5.10 Cuentas externas

**Modelo** (`apps/nucleo/cuentas/modelo`):

```go
type Cuenta struct {
    ID        string            // uuid local
    Proveedor string            // microsoft | google | icloud | yahoo | imap | caldav | carddav | slack | sytue
    Rotulo    string            // lo que ve la persona
    Direccion string            // correo o usuario
    Servicios []Servicio        // correo, calendario, contactos, chat, reuniones
    Servidores map[Servicio]Servidor // host, puerto, seguridad; vacío si el proveedor los fija
    Estado    string            // ok | reconectar | sin-servicio
}
```

`cuentas.json` guarda esto. Los secretos, en el llavero con la cuenta `cuenta-<ID>-<clase>` (contraseña,
token de refresco).

**Alta de una cuenta:**

1. Elegir proveedor. Con un correo escrito se intenta el autodescubrimiento estándar (registros SRV y
   `.well-known` de CalDAV y CardDAV; RFC 6186 y RFC 6764). Es red hacia el dominio de la cuenta,
   pedida por la persona.
2. OAuth (Microsoft, Google, Slack) o usuario y contraseña (IMAP, DAV, iCloud con contraseña de app).
3. Prueba de conexión de cada servicio y elección de cuáles se usan.

**OAuth** (`cuentas/oauth`): código de autorización con PKCE (S256), navegador del sistema,
redirección a `http://127.0.0.1:<puerto libre>/…` con `auth.WaitLoopback`. En móvil, redirección por
el esquema propio de la app. La lógica PKCE de `apps/nucleo/auth/oidc.go` está atada a Sytue ID; se
generaliza lo común en vez de escribirla otra vez.

**Mover del servidor** (R-NO-COPIAR), en tres pasos por paquete:

1. Mover a `apps/nucleo/cuentas/…` **sin cambiar comportamiento**. Lo que hoy marca conexiones con
   `platform/pkg/ssrfhttp` (que el núcleo no puede importar) pasa a recibir el marcador inyectado: el
   servidor inyecta el suyo con sus defensas, el cliente inyecta el portero.
2. El servidor importa desde `apps/nucleo` y sus tests siguen en verde.
3. Ampliar con lo que falta (§6.9, §6.10, §6.11, §6.14).

**Dependencias nuevas en `apps/nucleo/go.mod`:** `go-imap/v2`, `go-message`, `go-sasl`, `go-smtp`,
`go-webdav`. Todas MIT, todas ya en el módulo raíz y todas nombradas como pila admitida en
CONVENTIONS (líneas 152-153). `go-imap/v2` sigue en beta: la raíz fija `v2.0.0-beta.8` y la última
publicada es `v2.0.0-beta.9` (2026-11-14, comprobado). La norma del repo prohíbe betas, pero la raíz
ya lleva esta; se usa **la misma versión que la raíz**, se sube en los dos sitios a la vez y la
excepción queda escrita en el ADR.

**Sincronización** (D-16). Un sincronizador por cuenta y servicio, con el mismo esqueleto:

- **Bajada:** incremental con el mecanismo del proveedor (UID y `CONDSTORE` en IMAP;
  `sync-collection` o `ctag`+`etag` en DAV; `syncToken` en Google; `delta` en Graph; historial en Gmail).
- **Subida:** cola de cambios pendientes en el almacén local, reintento con espera creciente.
- **Conflictos:** el servidor manda en lo que no se tocó en local; si los dos cambiaron lo mismo, se
  conservan los dos y se avisa. Nunca se pierde lo escrito en local.
- **Cuándo:** al abrir, al volver a primer plano, cada N minutos con la app abierta y a mano. En
  móvil, además en el refresco en segundo plano del sistema, que es de «mejor esfuerzo».
- **Sin avisos instantáneos con la app cerrada en móvil.** El aviso «push» necesita un servidor que
  lo envíe. Es un límite de no tener servidores, y se dice.

### 5.11 Abrir entre apps y ficheros del disco

Hoy `api.RutaDeApertura` y `api.LeerApertura` llevan una referencia de Drive (`file=`, `path=`). Se
añade un parámetro que lleva una ruta local y rellena el campo que ya existe, `Apertura.Local`:

```
sytuelocal-docs:/abrir?local=file:///Users/…/informe.docx&volver=<carpeta>
```

- `local.AbrirEn` intenta el esquema `sytuelocal-<producto>` y **no cae a la web**. Si no hay quien lo
  atienda, abre con la app predeterminada del sistema (`ficheros.Abrir`) y, si tampoco, lo dice.
- Los receptores usan el camino que ya existe para «Abrir con», soltar en la ventana y argumentos
  (`AlAbrirFichero` → `Apertura{Local}`), **sin** la pregunta de consentimiento de subida, que en
  local no significa nada.
- **Photos** hoy ignora los ficheros locales (solo mira `Ref.ID`); se le añade el camino local.
- La tabla extensión → producto (`api.AppQueAbre`) se amplía con las extensiones nativas
  (`.docus`, `.calcus`, `.diapos`) y con vídeo → `cast`.

### 5.12 Avisos programados

Calendar y Notepads tienen recordatorios que hoy entrega el servidor. Se añade
`apps/nucleo/local/avisos`, con una implementación por sistema:

| Sistema | Mecanismo | Con la app cerrada |
| --- | --- | --- |
| macOS, iOS | avisos locales programados del sistema (`UNUserNotificationCenter`) | sí |
| Android | `AlarmManager` + aviso local | sí |
| Windows | avisos programados del sistema (`ScheduledToastNotification`) | sí |
| Linux | temporizador en el proceso residente (la bandeja del Hub) | solo con el Hub residente abierto |

Las horas salen del código que ya existe (`calendar/alarm.Triggers`). Al abrir se reprograma una
ventana de los próximos 30 días. Al pulsar el aviso se abre la app en el evento o la nota, por su
esquema.

### 5.13 Copia de seguridad, exportación e importación

- **Copia:** un ZIP en `Copias/` con la carpeta de datos (las bases SQLite se copian con la API de
  copia en caliente de SQLite, no con el fichero abierto) más los ajustes. Sin secretos.
- **Restaurar:** elegir un ZIP, comprobarlo y sustituir, con copia previa de lo que hay.
- **Exportar en formatos abiertos:** cada app ya tiene los suyos (notas a Markdown y HTML, contactos a
  vCard y CSV, calendario a ICS, documentos a ODF y OOXML, bases a CSV); «Exportar todo» los encadena.
- **Traer desde la edición conectada:** importar el ZIP de «Tus datos» que genera el Hub. Es un
  fichero, no una conexión. Va en una fase posterior (§9, F6).

### 5.14 Textos e idiomas

- Las pantallas compartidas siguen con sus catálogos, generados desde la web (26 idiomas).
- Los textos **propios** de la edición (Ajustes, cuentas, IA, copia, «Guardar como», mensajes de
  importación) van en `apps/fyne/local/catalogo`. Va **fuera** de `apps/fyne/catalogos` a propósito:
  el guardián `catalogos_test.go:88` exige que cada directorio de ahí tenga exactamente las claves de
  un `en.ts` de la web, y este no tiene web.
- La fuente son `es` y `en`, escritos a mano. Los otros 24 se generan y se revisan. Un test propio
  exige las mismas claves en los 26.
- El guardián `TestLasClavesQueElCodigoPideExisten` recorre todo `apps/fyne`. Se le **da de alta** el
  catálogo local para los prefijos `local/` y `cmd-local/` en su tabla de prefijos. No es una
  excepción: es registrar un catálogo más.

---

## 6. Plan por producto

Para cada producto: qué es en local, qué se monta, qué no, las tareas y sus pruebas. Las tareas llevan
un identificador para poder citarlas en los PR.

### 6.1 Cimientos (comunes a todas)

| Tarea | Qué | Dónde |
| --- | --- | --- |
| C-01 | ADR 0083 y la excepción de la barra en CONVENTIONS §13.6. | `docs/decisions.md`, `docs/CONVENTIONS.md` |
| C-02 | Tabla de identidad y rutas (`espacio`), perfil e id de instalación. | `apps/nucleo/local/espacio` |
| C-03 | Etiqueta `edicionlocal` y las tres constantes (§5.3), con su guardián de espacio. | `nucleo/sistema/escritorio`, `nucleo/ajustes`, `nucleo/auth` |
| C-04 | Extraer `anfitrion` de `sesion`, pieza a pieza, con los tests de `sesion` en verde. | `apps/fyne/anfitrion`, `apps/fyne/sesion` |
| C-05 | `local.Abrir` y `local/montaje`. | `apps/fyne/local` |
| C-06 | `ui.Anfitrion`, `ui.NuevaBarraLocal` y sus guardianes de píxel. | `apps/fyne/ui` |
| C-07 | `local/sqlite` (abrir, WAL, migraciones). | `apps/nucleo/local/sqlite` |
| C-08 | Puerto local, `SinOrigen`, `ClaveEnFichero`; tests de contrato; prueba de carga (§5.6). | `apps/nucleo/local/almacen` |
| C-09 | Portero de red y transporte que deniega. | `apps/nucleo/local/red` |
| C-10 | Secretos: llavero `eu.sytue.local` con reserva a fichero. | `apps/nucleo/local/secretos` |
| C-11 | Pantallas de Ajustes (apariencia, perfil, datos, red, acerca de). | `apps/fyne/local/ajustes` |
| C-12 | Catálogo local en 26 idiomas y alta en el guardián de claves. | `apps/fyne/local/catalogo` |
| C-13 | `Apertura.Local` en los esquemas y `local.AbrirEn` (§5.11). | `apps/nucleo/api/abrir.go` |
| C-14 | Guardianes de la edición (§8.1). | varios |
| C-15 | Empaquetado parametrizado y CI (§7). | `scripts/empaquetar.sh`, `.github/workflows` |
| C-16 | **Primera app de punta a punta:** Notepads en modo mínimo (un cuaderno, una nota), que ejercita todo lo anterior. No se crea una app de demostración aparte. | `apps/fyne/cmd-local/notes` |

### 6.2 Docus, Calcus y Diapos

**En local son** una ofimática de escritorio: biblioteca propia y ficheros del disco.

**Se monta tal cual** (ya es local con la fachada privada): edición completa, fórmulas, gráficos,
tablas dinámicas, hipótesis, formato condicional, control de cambios, comentarios, versiones manuales,
exportar (PDF accesible y simple, docx, xlsx, pptx, odt, ods, odp, html, markdown, plantillas),
imprimir, macros, combinación de correspondencia, presentar con vista de moderador.

**No se monta:** compartir y enlaces públicos, coedición y presencia, peticiones de edición, registro
de actividad, menciones a personas, directorio, «Presentar con Cast» y el incrustado de Power BI.

| Tarea | Qué | Notas |
| --- | --- | --- |
| O-01 | Puntos de entrada `cmd-local/{docus,calcus,diapos}`: `office.Repository` sobre el puerto local; `EnVivo=false`; `EnlaceDoc`, `EnlacePublico`, `Directorio`, `Cast` a `nil`; `Reclamos` desde el perfil; `editor.Personal` con una persona fija «local» para que las macros se guarden. | Diapos necesita que el mismo dato cumpla `SlidesData` y `DocsData`; `office.Repository` ya lo hace. |
| O-02 | **Ocultar** los botones de compartir de la cabecera y de la cinta cuando no hay enlace. Hoy quedan visibles e inertes (`editor/cabecera.go:159`, `editor/colaboracion.go:110`). | Cambio aditivo: una comprobación de `nil`. |
| O-03 | **Abrir del disco sin servidor.** El gancho `Importar` del punto de entrada llama a la conversión de la fachada, que con datos privados ya es local (`api/docs.go:304-305`), en vez de a `sesion.importar`. | Mismos límites de hoy: 25 MiB de entrada, 900.000 caracteres de contenido. |
| O-04 | **«Guardar como…»**: selector de guardado del sistema, nuevo en `apps/nucleo/ficheros` (`GuardarComo(nombre, tipos)`), para las cinco plataformas. | `ficheros` tiene hoy `Elegir` y `Guardar(carpeta, …)`, pero no un selector de guardado. |
| O-05 | **Formato nativo** `.docus` / `.calcus` / `.diapos`: un ZIP con `mimetype`, `meta.json` (tipo, título, versión del formato), `contenido` (el mismo registro que guarda la biblioteca: HTML saneado o JSON del modelo) y `comentarios.json`. Lectura y escritura en `apps/nucleo/office/model/nativo.go`. | Sin pérdida por construcción: es el modelo, no una conversión. |
| O-06 | **Documento anclado a un fichero.** Se reutiliza el anclaje que ya existe (`CreateDocumentFromSource`, `DocumentBySource`) con `fuente = file://…` más la huella del fichero. Abrir el mismo fichero dos veces abre el mismo documento. | |
| O-07 | **Guardar en el fichero** (Ctrl/Cmd+S con el documento anclado). Fichero nativo: directo. Formato ajeno (docx, odt…): la primera vez se pregunta, se explica que es una conversión y se guarda una copia `nombre (original).ext`. **Bloqueado** si la importación recortó contenido o venía de una plantilla; entonces solo «Guardar como». Siempre temporal + renombrado. | R-NO-CORROMPE. |
| O-08 | Detectar que el fichero cambió por fuera (huella distinta al guardar): preguntar, no pisar. | |
| O-09 | Papelera, «Hacer una copia», plantillas, temas y bloques: interfaces opcionales nuevas en `api` e implementación en `office.Repository` (Anexo A). | Los temas ya están en el cliente (`model.CatalogoTemas`, `AplicarTema`, `PrevisualizarTema`). |
| O-10 | Mover el catálogo de plantillas y bloques de `services/docs/internal/plantillas` a `apps/nucleo/office/plantillas`. | R-NO-COPIAR. |
| O-11 | La exportación a PDF accesible lleva el idioma a fuego (`"es"`, `office/repository.go:1345`): usar el de la app. | Arreglo que también gana la conectada. |
| O-12 | IA: `Asistir` desde el anfitrión (`nil` sin clave). | |
| O-13 | Diapos: el modo de fichero JSON que ya existe (`cmd/diapos/main.go:260-278`) se sustituye por el formato nativo; se mantiene la lectura del JSON antiguo. | |
| O-14 | Lista de la portada: orden por título y paginación, que la lista privada ignora hoy (`office/repository.go:588-602`). | Con muchos documentos locales deja de ser un detalle. |

**Pruebas:** ida y vuelta del formato nativo byte a byte del contenido; anclaje (mismo fichero, mismo
documento); guardado bloqueado tras importación recortada; corte a mitad de guardado no deja fichero a
medias; los guardianes de píxel actuales de `editor/…` no cambian.

**Fuera:** `.doc`, `.xls`, `.ppt` y `.rtf` (no hay conversor en el repo); corrector ortográfico y
dictado (no existen en los editores).

### 6.3 CertiPDF

**En local es** un editor de PDF sobre los ficheros del disco.

**Se monta tal cual:** ver, buscar, seleccionar, anotar y comentar, formularios, editar texto e
imágenes, páginas (extraer, borrar, girar, reordenar, combinar), proteger y permisos, redactar,
comprimir, PDF/A, escanear fotos a PDF, firma dibujada, firma con fichero de certificado, leer en voz
alta, imprimir.

**No se monta:** peticiones de firma a otras personas.

| Tarea | Qué | Notas |
| --- | --- | --- |
| P-01 | `cmd-local/certipdf` sobre `local/archivos` (§6.4) como `api.DriveData`, con **ficheros sueltos**: un PDF de cualquier carpeta se abre por su ruta absoluta, sin copiarlo. | D-24. |
| P-02 | **Guardar en su sitio** con `DriveFileReplacer.ReplaceFile`: temporal + renombrado, y la versión anterior a `.sytue/versiones`. El panel «Versiones» funciona sobre eso. | |
| P-03 | **Enrutar en `localMotor` lo que ya está escrito:** `/pdf/compare` (`CompararPDF`, `CompararVisual`), `/pdf/verify` (`VerificarFirmas`), `/pdf/attachment` (`ExtraerAdjunto`), las capas de `/pdf/pages` (`MarcaDeAguaPDF`, `FondoPDF`, `NumerarPaginasPDF`, `NumeracionBatesPDF`, `EncabezadoPiePDF`) y `/pdf/sign/prepare` y `/complete` (`PrepararFirmaPDF`, `CompletarFirmaPDF`). | Trabajo de conexión. La conectada también lo gana. |
| P-04 | **Firma con el certificado del sistema** (tarjeta, llavero, almacén de Windows, KeyChain de Android), en local gracias a P-03. `Almacen.Firmar` recibe hoy un tipo del API; se construye desde `localpdf.FirmaPreparada`. | |
| P-05 | **Sello de tiempo opcional**: la persona escribe la dirección de una autoridad de sellado (se ofrecen preajustes de autoridades cualificadas europeas). Sin configurar, la firma no lleva sello y se dice. | Es red, pedida por la persona (propósito `sello-de-tiempo`). |
| P-06 | Verificación de firmas: usa solo las anclas configuradas y no consulta revocación (ya es así). En local, las anclas se cargan de ficheros que la persona añade. | Se dice en pantalla que no se comprueba revocación. |
| P-07 | **OCR.** Primera entrega: transcripción con el modelo de visión de la persona, si lo hay. Después: el OCR del sistema (Vision en Apple, el de Windows; en Linux, `tesseract` si está instalado), y la capa de texto con `CapaDeTextoPDF`, que ya es local. | Hoy el reconocimiento es un servicio aparte con Tesseract. |
| P-08 | `p.IA` pasa a la interfaz `api.Asistente` (§5.7) y la barrera `CheckRemoteProcessing` se sustituye por el aviso de primera vez. | |
| P-09 | Autor de las anotaciones: el perfil, no `Account.Me`. | |
| P-10 | Subir el tope de lectura local de 25 MiB al del motor (100 MiB, `PDFMaxBytes`). | Hoy `sesion.LeerLocal` corta antes. |
| P-11 | La portada enseña los PDF recientes (ficheros sueltos abiertos) y la carpeta que la persona elija. | |

**Pruebas:** guardar en su sitio conserva la versión anterior; un PDF abierto fuera de la carpeta de
datos no se copia; cada ruta nueva de `localMotor` con sus PDF de prueba; la firma sin autoridad
configurada no abre conexiones.

### 6.4 Drive

**En local es** un gestor de ficheros sobre carpetas del disco.

`apps/nucleo/local/archivos` implementa `api.DriveData` (25 métodos), `DriveFileReplacer` y las
interfaces nuevas del Anexo A.

| Aspecto | Diseño |
| --- | --- |
| Raíz | `~/Sytue Local/Drive` por defecto. Se pueden añadir **más ubicaciones** (otra carpeta, otro disco), que salen en el lateral donde hoy salen las nubes. |
| Identificador | Opaco: ubicación + ruta relativa. Renombrar o mover desde la app actualiza el índice. Un cambio hecho por fuera se reconcilia al refrescar; lo que ya no existe se suelta. |
| Índice | `.sytue/indice.sqlite` en cada ubicación: favoritos, recientes, etiquetas, papelera y versiones. |
| Papelera | Propia: `.sytue/papelera/<uuid>` + ruta original y fecha. Restaurar, borrar del todo, vaciar. Retención configurable (30 días por defecto). Igual en las cinco plataformas. |
| Versiones | Al sobrescribir **desde la suite** se guarda la anterior en `.sytue/versiones`, con tope por número y por tamaño. Lo que cambie otro programa no se versiona, y se dice. |
| Miniaturas | `apps/nucleo/miniatura`, con caché en `<caché>/sytue-local/miniaturas`. |
| ZIP | En el dispositivo, en flujo. |
| Cuota | El espacio libre del disco, donde hoy va la cuota. |
| Búsqueda | Por nombre, recorriendo el árbol, como hoy. |
| Refresco | Al volver a la ventana y al tirar para refrescar. Vigilar el disco en vivo, solo si se mide que hace falta. |

**No se monta:** compartir y enlaces, «Compartido conmigo», unidades de equipo, etiquetas de
sensibilidad de la organización, exportar vistas de Bases, «Traer mis datos», adjuntar a correo por
referencia, y las integraciones con el sistema (Cloud Files, File Provider, WebDAV, SAF): con el Drive
ya en una carpeta del disco, no tienen qué proyectar.

| Tarea | Qué |
| --- | --- |
| DR-01 | `local/archivos`: los 25 métodos de `DriveData` sobre el sistema de ficheros, con escritura atómica. |
| DR-02 | Índice, papelera y versiones en `.sytue`. |
| DR-03 | Interfaces nuevas: `Purge`, `CopyFolder`, `BatchMove`, `BatchTrash`, `Quota` (Anexo A). |
| DR-04 | `cmd-local/drive`: `drive.NuevaPantalla` con la fachada local; `Directorio`, `Grupos`, `integraciones` y `Cuenta` a `nil`. |
| DR-05 | **Ocultar** las secciones y acciones de compartir, equipos y nubes. Hoy no se ocultan solas: llaman y fallan. |
| DR-06 | «Nuevo → Documento, Hoja, Presentación» crea un **fichero nativo** en la carpeta y lo abre en su app local por `sytuelocal-…?local=`. Hoy crea un documento en el servicio de Docs y abre la web. |
| DR-07 | Abrir en su app: `local.AbrirEn` con ruta local (§5.11). Audio y vídeo: Cast local si es vídeo; si no, el reproductor del sistema, directamente sobre la ruta, sin descargar. |
| DR-08 | «Descargar» pasa a ser «Copiar a…» y «Mostrar en la carpeta». |
| DR-09 | Etiquetas libres locales en el hueco de «Clasificar». |
| DR-10 | «Resumir» con la IA de la persona; `IA` pasa a `api.Asistente`. |
| DR-11 | Vista previa de imagen: JPEG, PNG y GIF también en «Mi unidad» (hoy solo en «Compartido conmigo»). |
| DR-12 | Móvil: la raíz es la carpeta de documentos de la app, visible en «Archivos» de iOS; en Android, una carpeta elegida con el selector del sistema. |
| DR-13 | **Fase posterior — ubicaciones remotas**: WebDAV (sirve para Nextcloud y un NAS) y después Google Drive, OneDrive y Dropbox, moviendo `platform/pkg/objectstore/externo_*`. Es el equivalente en Drive de las cuentas externas de Mail. |

**Pruebas:** juego de contrato de `DriveData` contra el sistema de ficheros real en un directorio
temporal; renombrado por fuera y reconciliación; nombres con caracteres raros y rutas largas en
Windows; papelera y versiones; ningún fichero a medias tras un corte.

### 6.5 Notepads

**En local son** cuadernos y notas en el almacén local.

**Se monta tal cual:** árbol de cuadernos, las cuatro disposiciones, editor con formato, tablas,
adjuntos, búsqueda, etiquetas de texto, fijadas, enlaces entre notas y exportar a Markdown, texto,
HTML, PDF y `.onex`.

**No se monta:** cuadernos cifrados (D-12), compartir, enlace público, «Compartido conmigo», menciones,
compartir en Rooms, páginas de servicios vinculados, la vista previa de documento de Docs y las notas
de tipo sala.

| Tarea | Qué | Notas |
| --- | --- | --- |
| N-01 | `cmd-local/notes` con `notes.NuevaPantalla` (no la «privada») sobre `notes.Repository` y el puerto local. | Con `work == nil` el asistente de IA sí se ofrece. |
| N-02 | **Primero, trazar el camino de guardado del editor sin diario** (`Almacen == nil`): no quedó comprobado de punta a punta (§14). Si el editor exige el diario cifrado, se le da el diario sellado local de D-08. | Es el acoplamiento más delicado con el cifrado. |
| N-03 | Implementar en `notes.Repository` todo lo que no tiene camino local (Anexo A): diario y «este día», tareas y posponer, etiquetas con estados, recordatorios, mover nota y cambiar de cuaderno, archivar, borrar del todo, y **persistir la importación** (el análisis ya es local). | Lo gana también la conectada. |
| N-04 | Recordatorios con `local/avisos` (§5.12). | |
| N-05 | Ocultar la creación de cuadernos cifrados; rechazar con un mensaje claro la importación de volcados cifrados. | |
| N-06 | Nota de imagen desde un **fichero** incrustado; la de URL queda tras el bloqueo de imágenes remotas. | Hoy solo admite `http(s)`. |
| N-07 | Enlaces a ficheros locales desde una nota (se abren con su app). | Sustituye a los enlaces a Drive. |
| N-08 | Eventos desde una nota: abrir Calendar local con `?action=create…`. | |
| N-09 | IA: `Asistir` desde el anfitrión. | |

**Pruebas:** contrato de `NotesData` y de las interfaces nuevas contra los dos puertos; importar
Markdown, Obsidian y Evernote de punta a punta sin red; recordatorio programado y cancelado.

### 6.6 Contacts

**En local es** una libreta de direcciones local, y cliente de CardDAV, Google y Microsoft.

**Se monta tal cual:** libretas, lista, ficha, formulario, etiquetas, favoritos, cumpleaños, papelera,
lote, importar y exportar vCard y CSV, duplicados y combinar, foto.

**No se monta:** la libreta «Organización», el directorio, «Trae tus contactos» y el panel de
servicios vinculados (lo sustituyen las cuentas).

| Tarea | Qué |
| --- | --- |
| K-01 | `cmd-local/contacts` sobre `contacts.Repository` y el puerto local; `Directorio` a `nil`. |
| K-02 | `BatchVCard` y `BatchRestore` con camino local (Anexo A). Sin ellos no hay exportación a vCard ni deshacer el borrado. |
| K-03 | IA «Rellenar» con `api.Asistente`. |
| K-04 | **Lector entre apps**: Mail (autocompletar, «Elegir de Contactos») y Calendar (cumpleaños) leen el almacén de Contacts en solo lectura. |
| K-05 | **Sincronización** (fase F8): cada libreta externa es una libreta local con origen. CardDAV (`addressbook-query`, `sync-collection`, PUT con `If-Match`, borrado, grupos como vCard `KIND:group`), Google People y Graph. |
| K-06 | En la lista de libretas, agrupar por cuenta. |

### 6.7 Calendar

**En local es** calendarios locales, y cliente de CalDAV, Google y Microsoft.

**Se monta tal cual:** vistas (mes, semana, día, agenda), crear, editar, arrastrar, recurrencias con
«este, los siguientes, todos», recordatorios por evento, buscar hueco con la ocupación propia,
exportar ICS, enlaces profundos.

**No se monta:** compartir calendarios de Sytue, recursos y salas, notas de evento contra Notes en la
nube, «Trae tus calendarios» y crear salas de Rooms de Sytue.

| Tarea | Qué | Notas |
| --- | --- | --- |
| A-01 | `cmd-local/calendar` sobre `calendar.Repository` y el puerto local. | |
| A-02 | `ResponderPorUID` y `SearchEventsFiltrada` con camino local (Anexo A). | |
| A-03 | Quitar los fallos a propósito del repositorio para lo que en local sí tiene sentido: `Subscribe` y `RefreshSubscription` (suscripción a una URL ICS, con `ical.DecodeICSFeed`). | La URL es red pedida por la persona (propósito `suscripcion-ics`). |
| A-04 | **Importar un `.ics`** desde fichero. Nuevo: no existe en la interfaz nativa. | |
| A-05 | Centro de recordatorios: calculado en local con `alarm.Triggers`, más los avisos del sistema de §5.12. | Hoy no hay aviso del sistema en este paquete. |
| A-06 | **Campo libre «Enlace de reunión»** en el formulario (`ConferenceURL` ya existe en el evento) y botón «Unirse», que abre el enlace. | Hoy solo se puede crear una sala de Rooms. |
| A-07 | Calendario virtual de **cumpleaños**, de solo lectura, desde Contacts (K-04). Nuevo. | |
| A-08 | Persistir preferencias de vista (`App.Almacen`), que hoy no se cablea. | |
| A-09 | IA: frase → propuesta de evento (`App.Asistente`), que hoy no se cablea. | |
| A-10 | **Sincronización** (fase F8): CalDAV (`calendar-query` con ventana, `sync-collection`, PUT y DELETE con `ETag`, `MKCALENDAR`), Google Calendar y Graph. La expansión de recurrencias es local (`event.ExpandEvents`). | |
| A-11 | **Invitaciones**: enviar y responder con el mecanismo del proveedor (planificación de CalDAV; nativo en Google y Graph) y, si no lo hay, por correo (iMIP) con la cuenta de Mail de la misma cuenta. | Calendar enlaza solo el envío de `cuentas/correo`. |
| A-12 | Lista de calendarios agrupada por cuenta; los de solo lectura, marcados. | |
| A-13 | Fase posterior: ocupación de los invitados (CalDAV `free-busy`, Graph `getSchedule`) y crear enlaces de Meet o Teams desde el formulario. | |

### 6.8 Photos

**En local es** una fototeca local.

**Se monta tal cual:** línea de tiempo, favoritos, búsqueda por nombre, descripción y etiquetas,
álbumes, visor, editor no destructivo (recorte, giro, espejo, exposición, contraste, saturación,
temperatura), exportar JPEG y PNG, lote, papelera y guardar en la galería del sistema.

**No se monta:** enlace público de álbum, compartir con personas, «Compartido conmigo», «Parecidas» y
sugerencias de álbum (dependen de vectores de imagen que calcula el servidor), y el ajuste de excluir
a la organización de la IA.

| Tarea | Qué | Notas |
| --- | --- | --- |
| FO-01 | `cmd-local/photos` sobre `photos.Repository` y el puerto local, **un solo repositorio**: los originales entran por la ingesta directa (`ImportarFoto`), sin el segundo repositorio de Drive que monta la conectada. | Los originales quedan en `Datos/photos/objetos`, en claro pero con nombre de huella. |
| FO-02 | Calcular en local lo que hoy calcula el servidor (Anexo A): saltar a un año, años de la línea de tiempo, recuerdos («un día como hoy»), duplicados por huella, «Por aquí» con la distancia entre coordenadas, vaciar papelera, portada de álbum. | |
| FO-03 | **Más formatos de entrada.** WebP y AVIF en Go puro en las cinco plataformas (los decodificadores ya son dependencia: los usa la vista previa de Drive). HEIC: con el sistema en Apple y Android; en Windows, con el códec del sistema si está; en Linux se conserva el original y se enseña sin vista previa. | El límite a JPEG, PNG y GIF venía del servidor. |
| FO-04 | «Copia de seguridad» pasa a ser **«Importar del carrete»**, con importación automática opcional. La lógica de `copia.go` (una vez cada elemento, por huella) se conserva. | |
| FO-05 | «Exportar la fototeca»: saca los originales con su nombre a `AAAA/MM/`. | Compensa que el almacén los guarde por huella. |
| FO-06 | Abrir un fichero de imagen desde Drive o desde el sistema directamente en el editor (§5.11). | Hoy Photos ignora los ficheros locales. |
| FO-07 | IA con visión: describir, preguntar, «editar con IA» y ampliar la búsqueda; `IA` pasa a `api.Asistente`. | |
| FO-08 | Caché en disco de miniaturas y derivadas (hoy solo viven en memoria). | Con R-MEDIR. |
| FO-09 | Fase posterior: **carpetas vigiladas** (enseñar fotos sin copiarlas) y **vídeos** en la galería con el reproductor de Cast. | |

### 6.9 Mail

**En local es** un cliente de correo para cuentas externas. No hay buzón de Sytue.

**Arquitectura.** `apps/nucleo/local/correo` implementa `mail.Servicio` entero, encima de:

- un **índice local** por cuenta (`Correo/<cuenta>/indice.sqlite`): sobres, marcas, carpetas, hilos,
  etiquetas, reglas, firma, bandeja de salida;
- una **caché de mensajes** en `.eml`, en claro (`Correo/<cuenta>/<carpeta>/<uid>.eml`);
- un **motor de sincronización por cuenta**, con los clientes de `apps/nucleo/cuentas/correo`.

| Proveedor | Lectura y acciones | Envío | Acceso |
| --- | --- | --- | --- |
| Outlook y Microsoft 365 | IMAP con XOAUTH2 | SMTP con XOAUTH2 | OAuth (D-19) |
| Gmail | IMAP (con sus extensiones de hilo y etiquetas) | SMTP | contraseña de aplicación; OAuth cuando esté verificada |
| iCloud, Yahoo, Fastmail y cualquier IMAP | IMAP | SMTP | contraseña o contraseña de aplicación |
| POP3 | solo lectura | SMTP | contraseña |

**Varias cuentas en una pantalla pensada para una.** Los identificadores de carpeta llevan la cuenta
delante. El árbol tiene un nodo por cuenta (el campo `Mailbox.ParentID` ya existe) y, arriba, las
bandejas unificadas (todas las entradas, todos los enviados, todos los borradores), cuyo dibujo ya
está hecho para las cuentas externas (`vista_externa.go`, `buzon_externo.go`). Cada cuenta es además
una identidad de «De».

**Se monta:** árbol de carpetas y contadores, lista con desplazamiento infinito, filtros rápidos,
conversación, adjuntos, descargar el `.eml`, redactar con el editor con formato, adjuntos e imágenes
en línea, firma, borradores, enviar, responder y reenviar, mover, archivar, papelera, correo no
deseado, leído y destacado, selección múltiple, crear, renombrar y borrar carpetas, búsqueda, atajos.

**No se monta:** alias, buzones compartidos y delegación, S/MIME, logos de remitente, guardar y
adjuntar de Drive en la nube, compartir en Rooms, «Trae tu correo», el medidor de cuota de Sytue y
«Clientes de correo».

| Tarea | Qué | Notas |
| --- | --- | --- |
| M-01 | Mover `services/integrations/internal/mailbox` a `apps/nucleo/cuentas/correo`, con el marcador inyectable. El servidor importa de ahí. | Sin cambio de comportamiento. |
| M-02 | Ampliar el cliente IMAP: `SEARCH`, `IDLE`, `CONDSTORE`/`QRESYNC`, crear, renombrar y borrar carpetas, `APPEND` de borradores, palabras clave, borrado con `UID EXPUNGE`, `QUOTA`, conexiones mantenidas. | Hoy abre una conexión por llamada y solo lee, mueve y marca. |
| M-03 | Índice local, caché `.eml` y descarga: primero sobres, cuerpos bajo demanda, y «tener sin conexión» los últimos N días por carpeta (ajustable). | |
| M-04 | **Hilos** en local: identificadores nativos donde existen (Gmail, Graph); si no, índice por `Message-ID`, `In-Reply-To` y `References`. `ThreadCount` y la conversación entre carpetas salen del índice. | Hoy los asigna el servidor. |
| M-05 | **Búsqueda**: sobre el índice local (sobres y cuerpos descargados, sin acentos, con `apps/nucleo/buscar`), más `SEARCH` en el servidor para lo no descargado. Operadores de la gramática actual. | Índice de texto completo de SQLite, solo si se mide que hace falta. |
| M-06 | **Bandeja de salida local**: enviar sin conexión, deshacer envío (retraso local) y envío programado. | El programado solo sale con la app abierta, o al siguiente arranque; se avisa al programarlo. |
| M-07 | Correo nuevo: `IDLE` en la entrada de cada cuenta y sondeo del resto; insignia de no leídos; aviso del sistema. | En móvil con la app cerrada, solo el refresco del sistema (§5.10). |
| M-08 | **Etiquetas**: las de Gmail, las palabras clave de IMAP donde el servidor las admite y, si no, etiquetas solo locales (se dice que no se sincronizan). | |
| M-09 | **Reglas**: locales, aplicadas al llegar el correo con la app abierta. Fase posterior: reglas en el servidor (Sieve). | |
| M-10 | Respuesta automática: solo donde el proveedor la ofrece (Sieve); si no, la sección no aparece. | |
| M-11 | Filtrar la página de Ajustes de Mail: su lista es una variable fija que siempre enseña alias, compartidos, S/MIME y clientes. | Cambio aditivo en `ajustes.go`. |
| M-12 | Alta de cuentas: el asistente que ya existe (`correo_externo.go`), contra el registro de cuentas local. | |
| M-13 | Autocompletar y «Elegir de Contactos» desde el almacén de Contacts (K-04). | |
| M-14 | «Crear evento» desde un correo: abrir Calendar local con `?action=create…`. | Hoy pasa por el Hub. |
| M-15 | IA en el redactor: `Asistir` desde el anfitrión. | |
| M-16 | **Lector con formato.** Hoy el lector enseña solo texto, o el HTML aplanado. Para Outlook y Gmail, donde casi todo es HTML, se pinta un subconjunto saneado con el modelo de Notepads (`notes.Leer`, `notes.NuevaProsa`) y el saneador de correo (`mail.SanearHTML`); imágenes remotas bloqueadas por defecto. | Falta comprobar que el HTML real de un correo sobrevive a ese modelo (§14). La primera entrega sale con el lector de texto actual. |
| M-17 | Invitaciones en el lector: detectar `text/calendar`, enseñar el evento y responder. | Hoy no hay. |
| M-18 | Fase posterior: darse de baja de listas (cabeceras y un clic), posponer y silenciar en local, API de Gmail y de Graph como alternativa a IMAP. | |

**Pruebas:** el cliente IMAP contra el servidor en memoria de la propia biblioteca
(`imapmemserver`); SMTP contra un servidor de prueba en proceso; hilos con un corpus de cabeceras
reales anonimizadas; bandeja de salida con la red cortada; nada sale del portero. Humo manual con
cuentas reales de Outlook, Gmail e iCloud (§11).

### 6.10 Cast

**En local es** una videoteca privada en el equipo (R8).

`apps/nucleo/local/videoteca` implementa `api.CastData`, `CastMediaData` y las interfaces nuevas.

| Concepto de hoy | En local |
| --- | --- |
| Subir | **Importar**: copiar el vídeo a `Vídeos/` (o dejarlo donde está y solo apuntarlo, para ficheros muy grandes). |
| Visibilidad | Dos estados: **borrador** y **publicado**. |
| Explorar (portada pública) | Los vídeos **publicados**, en este equipo. |
| Canal de la organización | **Colecciones** locales, para ordenar la portada. |
| Gestionar | Título, descripción, póster, colección, publicar o retirar, borrar. Con todos los permisos. |
| Calidades | No hay: se reproduce el original. |
| Reanudar, barra de visto, PiP, velocidad | Como hoy; ya eran locales. |

**No se monta:** directo y emisión, chat, comentarios, compartir e incrustar, personas, moderación y
denuncias, «Subir desde Drive», estimación de calidades y cuota.

**Reproducción** (D-27):

| Entrega | Qué se reproduce dentro de la app | El resto |
| --- | --- | --- |
| V1 | WebM y Matroska con VP9 u **VP8** y Opus (el decodificador VP8 ya viene en la misma biblioteca). | «Abrir con el reproductor del sistema». |
| V2 | Además MP4, MOV y M4V (H.264, HEVC, AAC) en **macOS, iOS y Android**, con los decodificadores del sistema. | — |
| V3 | Lo mismo en **Windows**. En **Linux**, con el decodificador del sistema si está instalado; si no, reproductor del sistema. | — |

`apps/fyne/video/sistema` entrega fotogramas e instantes de audio con la misma forma que el
reproductor actual, de modo que `visor.go` no cambia:

| Sistema | Marco |
| --- | --- |
| macOS, iOS | AVFoundation (`AVAssetReader`) |
| Android | `MediaExtractor` + `MediaCodec`, por el puente JNI que ya se usa |
| Windows | Media Foundation (lector de origen) |
| Linux | el decodificador del sistema, detectado al arrancar |

| Tarea | Qué |
| --- | --- |
| CA-01 | `local/videoteca`: índice SQLite + carpeta; `CastData`, `CastMediaData` (un `os.File` ya cumple la fuente del reproductor). |
| CA-02 | Interfaces nuevas: importar, actualizar, borrar, póster, colecciones, publicar (Anexo A). |
| CA-03 | `cmd-local/cast`: arranca en **Inicio** («Mis vídeos», «Seguir viendo», recientes). Hoy el montaje privado aterriza en un «Gestionar» vacío y la pantalla de inicio falla al pedir los vídeos de la organización. |
| CA-04 | Reescribir la carga de Inicio, Explorar y Canal para pedir solo lo local; ocultar las secciones de organización, directo y compartidos. |
| CA-05 | Leer duración y dimensiones del fichero: del WebM con el lector actual; del MP4, con un lector de cajas en Go (`mvhd`, `tkhd`), pequeño y nuevo. |
| CA-06 | **Póster**: en WebM, el primer fotograma (el reproductor ya lo entrega, `AlFotograma`); en el resto, la miniatura del sistema (Quick Look, la del Explorador, `MediaMetadataRetriever`); siempre se puede elegir una imagen. «Usar este fotograma» funciona donde haya decodificador. |
| CA-07 | VP8 en el reproductor propio. |
| CA-08 | Decodificadores del sistema: V2 (Apple y Android) y V3 (Windows y Linux). |
| CA-09 | Diapos: «Presentar con Cast» no se monta (necesita directo). |

### 6.11 Sincronización de Calendar y Contacts

Va descrita en §5.10 (esqueleto), §6.6 (K-05) y §6.7 (A-10, A-11). Tareas comunes:

| Tarea | Qué |
| --- | --- |
| S-01 | Mover `linkedservices/dav.go` y los clientes de Google y Graph a `apps/nucleo/cuentas/{dav,nube}`, con marcador inyectable. |
| S-02 | Quitar del cliente DAV el bloqueo a hosts de Yahoo (pasa a ser una política que inyecta el servidor) y añadirle borrado, grupos, planificación y `sync-collection`. |
| S-03 | `apps/nucleo/local/agenda`: el sincronizador, la cola de pendientes y la resolución de conflictos. |
| S-04 | Autodescubrimiento (SRV y `.well-known`). |

**Pruebas:** contra los servidores en memoria de `go-webdav`; conflictos (cambio en los dos lados);
cola de pendientes con la red cortada; recurrencias con excepciones de ida y vuelta.

### 6.12 Bases

**En local es** tablas relacionales en el almacén local, para una persona.

**No se monta:** compartir y roles, formularios públicos, coedición y presencia en vivo, transferir la
propiedad.

| Tarea | Qué | Notas |
| --- | --- | --- |
| B-01 | Mover lo puro de `services/bases/internal/domain` a `apps/nucleo/bases/dominio`: fórmulas (`AnalizarFormula`, `Comprobar`, `EvaluarFormulas`), validación de valores, configuración de vistas, lectura de CSV e inferencia de tipos. | El motor de fórmulas ya está en el cliente. |
| B-02 | **Evaluador en memoria** de filtros, orden y búsqueda. En el servidor son SQL (`store/filtros.go`). | Para que no diverjan, **los mismos vectores de prueba** corren contra el SQL del servidor y contra el evaluador. |
| B-03 | Completar `bases.Repository`, que hoy ignora vista, cursor, validación, conflictos y recálculo: paginación, validación al escribir, recálculo de fórmulas (también a través de enlaces), integridad de enlaces al borrar, posición y moneda de los campos. | |
| B-04 | Camino local para lo que no lo tiene (Anexo A): fórmula y opciones de un campo, reordenar campos, actualizar vista, búsqueda y orden puntuales, adjuntos y sus miniaturas, importar CSV, exportar CSV y JSON, comentarios (como notas del registro). | |
| B-05 | `cmd-local/bases`; ocultar compartir, formularios y actividad. | |
| B-06 | Registro de actividad local, opcional. | Fase posterior. |

### 6.13 ERP

**En local es** la gestión de una empresa en un equipo, para una persona. Es **lo más caro del plan** y
va lo último.

**Por qué es caro.** Toda la lógica está en el servidor. Los paquetes **puros** se mueven. La que vive
en el almacén de Postgres (numeración, libro, cierre, ventas, banco, TPV, importación, informes) hay
que **reescribirla sobre SQLite**. El repositorio privado actual del ERP devuelve un catálogo vacío.

| Tarea | Qué | Notas |
| --- | --- | --- |
| E-01 | Mover a `apps/nucleo/erp/…`: `catalogo` (módulos y términos embebidos, 26 idiomas), `domain` (validación, etiquetas, estados, **asiento**, **desglose de IVA**, NIF), `verifactu` (huella, QR, registro), `banca` (Norma 43, emparejar) y `obligaciones` (31 países). | `domain/nif.go` importa `platform/pkg/validation` y `verifactu/certificado.go` importa `platform/pkg/pkcs12`: hay que mover también esas dos piezas o invertir la dependencia. |
| E-02 | `apps/nucleo/local/erp`: esquema SQLite de un solo inquilino y las operaciones del almacén. | |
| E-03 | **Transporte en proceso** (D-10): un `http.RoundTripper` que atiende `/api/v1/…` sobre E-02. `api.NewERP(url, nil, &http.Client{Transport: …}, idioma)`. | Las rutas y las formas JSON son las que el cliente ya define. |
| E-04 | Entrega 1 — **fichas**: catálogo, listas con búsqueda, orden, estados y papelera, crear, editar, duplicar, **numeración**, conversión presupuesto → pedido → factura, tareas, documentos adjuntos, historial, paleta, imprimir, Facturae, UBL, exportar CSV. | |
| E-05 | Entrega 2 — **contabilidad**: asentar, libro, balance, anular, cierre y reapertura, resumen, banco como tareas, importar extracto Norma 43 (es un fichero, no una conexión al banco). | |
| E-06 | Entrega 3 — obligaciones (calendario, cálculo, ICS), asistente de arranque, TPV, importación con simulacro y deshacer, informes, exportar a Excel, tableros. | |
| E-07 | Entrega 4 — **VERI\*FACTU** (D-29), tras la revisión legal: cadena, QR y registros en local; envío a la AEAT con el certificado de la persona. | El envío es red a la AEAT, exigida por la norma y configurada por la persona (propósito `aeat`). |
| E-08 | IA del ERP (frase → informe, frase → plan de registros, rellenar ficha desde texto o PDF, pistas del banco) con la clave de la persona; mover lo necesario de `services/syerp/internal/ia` y `platform/pkg/asistente`. | |
| E-09 | **No se monta:** aprobaciones y umbrales, peticiones de edición, el equipo como responsables de tareas, el proceso de obligaciones compartido, conectores. | Son de varias personas. |

**Para que no diverja del servidor.** Vectores dorados compartidos: mismas entradas, mismos asientos,
mismo desglose, misma numeración y mismas huellas en el almacén de Postgres y en el de SQLite. Es la
condición de salida de cada entrega.

### 6.14 Rooms

**En local es** un cliente de varios servicios de mensajería. No hay salas propias.

**Arquitectura.** `apps/nucleo/local/chat` implementa `rooms.Servicio` como **agregador**: cada cuenta
o espacio conectado es un **equipo** (`RoomsTeam`), con los identificadores prefijados por cuenta. Las
capacidades opcionales (hilos, reacciones, editar, adjuntos…) las declara cada proveedor, y la pantalla
ya oculta lo que el servicio no implementa.

**Cambio de interfaz necesario.** La pantalla usa hoy **solo el primer equipo**
(`lista{equipo: equipos[0].ID}`, `sala.go:620`). Hace falta un **selector de espacio** en el carril,
con sus no leídos, y que `Sala` recuerde el elegido. Es una adición: con un solo equipo no se ve.

| Proveedor | Leer | Escribir | Tiempo real | Reuniones | Acceso |
| --- | --- | --- | --- | --- | --- |
| **Slack** | canales, directos, hilos | enviar, editar, borrar, reaccionar, adjuntos | Socket Mode | — | app propia del espacio (D-20) |
| **Microsoft Teams** | chats; canales si el administrador de la organización lo consiente | enviar, responder; editar y borrar en chats | sondeo (las notificaciones de cambio piden un servidor) | crear enlace; unirse en la app oficial | OAuth de Microsoft |
| **Google Chat** | solo con cliente OAuth propio del Workspace (D-22) | enviar | sondeo | — | OAuth de Google |
| **Google Meet** | — | — | — | crear enlace; unirse en la app oficial | OAuth de Google |
| **Sytue Rooms online** | todo | todo | el actual | llamadas dentro de la app | cuenta de Sytue (ver abajo) |

**Sytue Rooms online es la excepción de R-SIN-SYTUE.** Conectarlo exige iniciar sesión en Sytue ID y
el cifrado de Matrix, porque así funciona el servicio (ADR 0077). Se trata como una cuenta externa
más: opcional, solo en el binario de Rooms, con su sesión en el llavero de la edición local (no en el
de la conectada) y reutilizando `mx.NewRoomsService` y `rooms.NuevoCifrado` tal como están. Va en la
última entrega de Rooms.

**Límites que son de los proveedores:**

- Teams personal (no de empresa) no está disponible por la API de Microsoft.
- Leer canales de Teams pide consentimiento del administrador de la organización.
- No hay llamadas de Teams ni de Meet dentro de la app (D-21).
- En móvil con la app cerrada no llegan avisos (§5.10).

| Tarea | Qué |
| --- | --- |
| RO-01 | Mover `services/integrations/internal/http/external_chat_{slack,teams,google}.go` a `apps/nucleo/cuentas/chat`, con marcador inyectable. |
| RO-02 | Agregador `local/chat` y mapeo de cada proveedor al modelo `api.Rooms*` (canales, mensajes, hilos, reacciones, miembros, no leídos). |
| RO-03 | Selector de espacio en el carril. |
| RO-04 | Slack: manifiesto de la app del espacio, alta guiada, Socket Mode, adjuntos, reacciones, búsqueda. |
| RO-05 | Teams: chats y canales con Graph, sondeo incremental, crear reunión. |
| RO-06 | Google Chat (enviar; leer con cliente propio) y Meet (crear enlace). |
| RO-07 | Caché local de los últimos mensajes por canal, para leer sin conexión. |
| RO-08 | «Ponme al día» con la IA de la persona. En local es posible: el contenido no va cifrado por nosotros. |
| RO-09 | Preferencias y avisos en local (hoy contra el Hub); presencia, donde el proveedor la dé. |
| RO-10 | Proveedor Sytue Rooms online. |
| RO-11 | **No se monta:** «Apps e integraciones», «Conectar con otras Apps» (puentes del servidor), moderación de la organización, GIF y emoji de la organización, WhatsApp. |

**Pruebas:** cada proveedor contra un servidor HTTP falso con respuestas grabadas; el agregador con
dos proveedores a la vez; capacidades ausentes que ocultan su control; nada sale del portero.

### 6.15 Hub

**En local es** el lanzador y el sitio de los Ajustes.

| Tarea | Qué | Notas |
| --- | --- | --- |
| HU-01 | `cmd-local/hub`: rejilla de las apps locales **instaladas**; abrir por `sytuelocal-…`, sin caída a la web. | En móvil, saber si otra app está instalada pide declararlo (esquemas consultables en iOS, visibilidad de paquetes en Android). |
| HU-02 | Los Ajustes completos (§5.5), incluidas cuentas, IA, datos y copia. | |
| HU-03 | Bandeja, «Abrir al iniciar sesión» y atajo global: la mecánica ya va por identificador de app y es local. | |
| HU-04 | Tarjeta «Hoy»: agenda de Calendar local y no leídos de Mail local, leyendo sus almacenes. | Fase posterior. |
| HU-05 | **Búsqueda en todas las apps locales**, consultando el índice de cada una. | Fase posterior. |
| HU-06 | Avisos propios del equipo (copias hechas, sincronizaciones con error), con `nucleo/hub` sobre el puerto local. | |
| HU-07 | «Traer desde Sytue»: importar el ZIP de «Tus datos». | Fase posterior. |
| HU-08 | **No se monta:** actividad, búsqueda federada del servidor, «Tus datos», descargas y actualizador, dispositivos y clientes externos, alta guiada, integraciones, presencia. | |

---

## 7. Empaquetado, CI y distribución

### 7.1 Por qué `cmd-local` y no otra cosa

Guardianes que cuentan o listan `apps/fyne/cmd/*`. Ninguno ve un árbol hermano:

| Guardián | Qué exige |
| --- | --- |
| `sesion/esquemas_test.go` | 15 esquemas, extensiones, color del icono, alta del esquema en Sytue ID |
| `sesion/widgets_plomeria_test.go` | exactamente 15 `main.go` con el publicador de widgets |
| `sesion/fondo_plomeria_test.go` | exactamente 15 que llaman a `sesion.Abrir` |
| `sesion/despertar_test.go` | los identificadores igualan `bundlesDespertar` |
| `catalogos/deproducto_test.go` | exactamente 15 en la tabla de catálogos |
| `iconos/iconos_test.go` | nombre = marca del producto; exactamente 15 |
| `nucleo/widgets/fuentes/registradas_test.go` | los esquemas igualan las fuentes registradas |
| `platform/pkg/hubnotify/cambios_test.go` | los esquemas igualan los productos con datos |
| `apps/frontend/…/openInApp.test.ts`, `apps.test.ts`, `downloads.test.ts` | tablas de la web iguales a `fyne/cmd` |
| `scripts/empaquetar-test.sh` | listas exactas de 15 apps y de dos bundles |
| `scripts/android-sesion.py` | 15 paquetes en `session-signers.json` |
| `xcode/generar.py --comprobar` | el proyecto generado iguala el árbol |

Lo que **sí** alcanza a `cmd-local` y hay que atender:

- `TestLasClavesQueElCodigoPideExisten` recorre todo `apps/fyne` (§5.14).
- `make fyne-cmd-check` compila `./...` para `js/wasm`: los `cmd-local` llevan `//go:build
  edicionlocal`, así que no entran. Se añade su propia comprobación (§7.3).
- El hook de pre-push excluye `/cmd/` de los tests, pero **no** `/cmd-local/`: sus tests corren en
  local. Se mantienen ligeros.

### 7.2 `empaquetar.sh`

Hoy la ruta `"$FYNE"/cmd` está escrita en cuatro sitios y las etiquetas, en seis. Cambio: una variable
de edición, que por defecto deja todo **exactamente** como está.

| Qué | Edición por defecto | `SYTUE_EDICION=local` |
| --- | --- | --- |
| Árbol de apps | `apps/fyne/cmd` | `apps/fyne/cmd-local` |
| Etiquetas de Go | `goolm` (+ `tienda`) | `goolm,edicionlocal` (+ `tienda`) |
| Identificador válido | `^eu\.sytue\.[a-z]+$` | `^eu\.sytue\.local\.[a-z]+$` |
| Ejecutable | `sytue-<app>` | `sytue-local-<app>` |
| Bundle de escritorio | `one`, `drive` | `local` |
| Clave de instalador en Windows | `eu.sytue.bundle.<b>` | `eu.sytue.local.bundle.<b>` |
| Piezas de sesión de Android (`android-sesion.py`) | sí | **no** |
| Widgets, avisos remotos, File Provider, derechos de sesión | según la app | **no** |
| Servicio en primer plano del Hub en Android | sí | **no** |

Los casos especiales por nombre de app que el script lleva a fuego (`drive`, `hub`, `demo`) se revisan
uno a uno para la edición local.

**Dos comprobaciones de convivencia con el actualizador de la conectada:**

- En Windows, `escritorio.Hermanas()` toma por hermana toda clave de desinstalación que empiece por
  `eu.sytue.`. La clave local empieza así. Se le añade a esa función que ignore `eu.sytue.local.`
  (una adición, con su test), y **antes** se comprueba qué hace hoy el actualizador con una clave que
  no conoce (§14).
- En macOS y Linux, `AppDeEjecutable` solo acepta letras y números tras `sytue-`; `sytue-local-docus`
  lleva un guion y queda fuera. Se fija con un test.

`scripts/empaquetar-test.sh` gana una sección para la edición local, con sus listas exactas (15 apps,
un bundle). Las de la edición actual no se tocan.

### 7.3 CI

AGENTS.md manda: las pruebas no van en el portátil, el CI es el entorno remoto, y los flujos se
despachan a mano.

| Flujo | Cambio |
| --- | --- |
| `ci-desktop.yml`, `ci-mobile.yml` | Un paso más por módulo con la etiqueta: `go build`, `go vet` y `go test -race -tags goolm,edicionlocal` sobre `./local/... ./cmd-local/...`, y los guardianes de §8.1. |
| `release-desktop.yml`, `release-mobile.yml` | Entrada `edicion` (`conectada` por defecto, `local`), que pasa `SYTUE_EDICION`. |
| `security.yml` | `go-licenses` sobre los árboles nuevos. `tools/eusupply`: añadir a su lista los ficheros con hosts de proveedores (preajustes de IA, extremos de OAuth). |
| `Makefile` | `make fyne-local-check` (compilar `cmd-local` con la etiqueta), dentro de `make gates`. |
| `.githooks/pre-push` | Compilar también con `-tags ci,goolm,edicionlocal`. |

No hace falta entorno efímero: no hay servidor que levantar. La verificación en ventana real
(`scripts/ventana-real.sh`) se amplía para lanzar apps locales; aquí no hay producción que proteger.

### 7.4 Distribución

Sin servidor propio de descargas (R9):

| Plataforma | Vía |
| --- | --- |
| macOS | DMG notarizado; después, Mac App Store |
| Windows | instalador firmado |
| Linux | `tar.xz` con `instalar.sh`, como hoy |
| iOS | App Store |
| Android | Google Play; AAB y APK para instalar a mano |

Los ficheros de escritorio se publican como adjuntos de la release del repositorio. La Mac App Store
exige caja de arena: las carpetas que elige la persona (datos, ubicaciones de Drive) necesitan
marcadores de seguridad persistentes, que hoy no existen. Por eso va después del DMG.

---

## 8. Pruebas y guardianes

Regla del repo que se aplica a todos: **un guardián que no se ha visto en rojo no protege nada**
(CONVENTIONS §13.4). Cada uno se comprueba invirtiendo a mano lo que vigila.

### 8.1 Guardianes de la edición

| Guardián | Regla | Cómo |
| --- | --- | --- |
| `TestUnBinarioLocalNoEnlazaLaSuite` | R-SIN-SYTUE | `go list -deps -tags edicionlocal ./cmd-local/<app>` no contiene `apps/fyne/cifrado`, `apps/nucleo/matrix`, `apps/fyne/llamada`, el OIDC de `apps/nucleo/auth`, `mautrix`, `livekit` ni `pion`. La lista de excepciones (solo `rooms`) se compara **por igualdad**. |
| `TestRecienInstaladaNoAbreNingunaConexion` | R-RED | Cada app arranca con un portero que falla el test ante cualquier marcado y pinta su pantalla inicial. |
| `TestTodoClienteDeRedPasaPorElPortero` | R-RED | En los paquetes de `local/` y `cuentas/` no hay `http.Client{}`, `http.DefaultClient`, `net.Dial` ni `tls.Dial` fuera de `local/red`. |
| `TestElEspacioLocalNoEsElDeLaSuite` | R-CONVIVE | Con la etiqueta, la carpeta de ajustes acaba en `sytue-local` y el servicio de llavero es `eu.sytue.local`; sin ella, los de siempre. |
| `TestLaIdentidadLocalNoChocaConLaConectada` | R-CONVIVE | Identificadores, esquemas y ejecutables de `cmd-local` son disjuntos de los de `cmd`, y ningún esquema local empieza por `sytue-`. |
| `TestCadaAppLocalDeclaraSuIdentidad` | — | El espejo de `esquemas_test.go` para `cmd-local`: 15 apps, su esquema, sus extensiones, su icono. |
| `TestSinEtiquetaNoArranca` | D-02 | `local.Abrir` falla si `!escritorio.EdicionLocal`. |
| `TestLosSecretosNoEstanEnLosAjustes` | R-CLARO | Tras dar de alta una cuenta y una clave de IA de prueba, ningún fichero de `<config>/sytue-local` (salvo `secretos/`) contiene el secreto. |
| `TestLosDatosEstanEnClaro` | R-CLARO | Tras crear una nota, su texto se encuentra tal cual en `registros.sqlite`. |
| `TestLaEdicionConectadaNoCambia` | R-ADITIVA | Los guardianes actuales de `cmd/*` (los de §7.1) siguen pasando sin tocar; es la condición de fusión de cada pieza. |

### 8.2 Tests de contrato

Un mismo juego de pruebas por producto que corre contra **dos** puertos: el que ya usan los tests en
memoria y el puerto local en disco. Demuestra que los repositorios de dominio se comportan igual sobre
los dos. Es lo que hace segura la decisión D-06.

### 8.3 Por tipo de pieza

| Pieza | Prueba |
| --- | --- |
| Movimientos del servidor al núcleo | Los tests del servicio, sin tocar, antes y después. |
| Almacén local | Contrato, corte a mitad de escritura, dos procesos, prueba de carga (§5.6). |
| Clientes de protocolo | Contra servidores en proceso de las propias bibliotecas (IMAP, WebDAV) y servidores HTTP falsos con respuestas grabadas (Graph, Google, Slack). |
| Sincronización | Conflictos, cola de pendientes con la red cortada, reanudación. |
| Formatos | Ida y vuelta del formato nativo; importaciones con sus ficheros de prueba. |
| Lógica portada (Bases, ERP) | Vectores compartidos con el servidor. |
| Pantallas | Los guardianes de píxel actuales, que no cambian, más los de la barra local y Ajustes. |
| Empaquetado | La sección local de `empaquetar-test.sh`. |
| Humo con cuentas reales | Lista manual por proveedor, con las cuentas de prueba de §11. |

---

## 9. Fases, orden y tamaño

Los tamaños son **semanas-persona estimadas leyendo el código, sin haber medido nada**. Sirven para
ordenar y comparar, no para comprometer fechas. Cada fase termina con sus pruebas en verde en el CI.

| Fase | Contenido | Depende de | Tamaño |
| --- | --- | --- | --- |
| **F0 — Cimientos** | C-01 a C-16: ADR, etiqueta y espacio, anfitrión, barra y Ajustes, almacén local con su prueba de carga, portero, guardianes, empaquetado y CI, Notepads mínimo de punta a punta. | — | 7–10 |
| **F1 — Ofimática y PDF** | §6.2 y §6.3. | F0, DR-01 y DR-02 de Drive (`local/archivos`) | 7–9 |
| **F2 — Drive** | §6.4 (sin DR-13). | F0 | 4–6 |
| **F3 — IA con clave propia** | §5.9 y el cableado en cada app ya hecha. | F0 | 2–3 |
| **F4 — Notepads, Contacts y Calendar en local** | §6.5, §6.6 (sin K-05), §6.7 (sin A-10 a A-13), avisos programados. | F0 | 9–12 |
| **F5 — Photos y Cast** | §6.8 (sin FO-09) y §6.10 con la reproducción V1. | F0 | 6–8 |
| **F5b — Vídeo con el sistema** | Decodificadores V2 y V3. | F5 | 8–12 |
| **F6 — Hub, copia de seguridad, traer desde Sytue** | §6.15 y §5.13. | F0 | 3–5 |
| **F7 — Cuentas y Mail** | §5.10 (modelo, OAuth, mover clientes) y §6.9 (sin M-16 a M-18). | F0, F4 (Contacts) | 12–16 |
| **F7b — Lector con formato e invitaciones** | M-16, M-17. | F7 | 3–5 |
| **F8 — Sincronización de Calendar y Contacts** | §6.11, K-05, A-10 a A-12. | F4, F7 (cuentas) | 6–8 |
| **F9 — Rooms** | §6.14. | F7 (cuentas) | 12–18 |
| **F10 — Bases** | §6.12. | F0 | 6–8 |
| **F11 — Móvil y tiendas** | Android e iOS de todo lo anterior: selectores, permisos, segundo plano, cuentas compartidas entre apps, fichas de tienda, caja de arena de la Mac App Store. | cada fase aporta su parte | 8–12 |
| **F12 — ERP** | §6.13, en cuatro entregas. | F0, F3 | 16–24 |

Total aproximado: **entre 110 y 155 semanas-persona**. F3, F4, F5, F6 y F10 pueden ir en paralelo
tras F0.

**Hitos**

| Hito | Fases | Qué puede hacer una persona |
| --- | --- | --- |
| **H1 — Escritorio ofimático** | F0, F1, F2, F3 | Escribir, calcular, presentar, editar y firmar PDF y gestionar sus ficheros, sin cuenta y sin red, en macOS, Windows y Linux. Con IA si pone una clave. |
| **H2 — Suite personal** | F4, F5, F6 | Notas, contactos, calendario, fotos y vídeos, todo en local, con copia de seguridad. |
| **H3 — Clientes** | F7, F7b, F8, F9 | Leer y enviar el correo de Outlook, Gmail y cualquier IMAP; calendarios y contactos externos; Slack y Teams. |
| **H4 — Gestión** | F10, F12 | Bases y ERP en local. |
| **H5 — Móvil y tiendas** | F11 | Lo mismo en iOS y Android, publicado. |

**Orden dentro de F0**, porque todo lo demás cuelga de él:

1. C-01 (ADR) y C-02, C-03 (identidad, etiqueta) con sus guardianes en rojo y luego en verde.
2. C-08: el puerto local y **la prueba de carga**. Si falla, se decide entre compactar y el plan B
   antes de seguir.
3. C-04, C-05, C-06: anfitrión, cáscara y barra.
4. C-09, C-10: portero y secretos.
5. C-16: Notepads mínimo de punta a punta, que valida la cadena entera.
6. C-15: empaquetarlo en las tres plataformas de escritorio y abrirlo en una ventana real.
7. C-11, C-12, C-13, C-14.

**Forma de trabajo** (lo que ya manda el repo):

- Rama de la línea: `edicion-local`, cortada de `sytue-native`, en su **propio árbol de trabajo**
  (`git worktree add ../sytue-one-local edicion-local`).
- Piezas pequeñas, como las `pieza/PEND-…` del historial, cada una con su guardián.
- Los movimientos del servidor al núcleo van **en PR aparte y antes** de usarlos, para que un fallo
  del servidor no se mezcle con código nuevo.
- Gates del hook antes de empujar; `make ci` y `security.yml` despachados después; `make ci-check`
  en verde antes de dar la pieza por cerrada.

---

## 10. Riesgos

| # | Riesgo | Efecto | Qué se hace |
| --- | --- | --- | --- |
| X-01 | El motor de registros no rinde con bibliotecas grandes: reconstruye el catálogo en memoria al abrir. | Photos o Notepads lentos. | Prueba de carga al principio de F0; compactar o plan B por producto (§5.6). |
| X-02 | La extracción de `anfitrion` cambia sin querer la edición conectada. | Regresión en 15 apps. | Pieza a pieza, tests de `sesion` sin tocar; si una pieza no sale limpia, no se mueve. |
| X-03 | Mover los clientes de protocolo rompe el servicio de integraciones. | Regresión en producción. | Movimiento sin cambio de comportamiento, en PR propio, con los tests del servicio. |
| X-04 | Los registros OAuth no llegan o Google no verifica la app. | Sin Outlook (bloquea R5) o sin Gmail por OAuth. | Microsoft es condición de entrada de F7. Gmail sale con contraseña de aplicación; siempre queda «mi propio cliente». |
| X-05 | Slack cambia otra vez sus condiciones, o un espacio no deja instalar apps internas. | Slack degradado o imposible en ese espacio. | Modo de app propia del espacio; se dice en el alta qué permisos pide. |
| X-06 | Teams sin consentimiento del administrador. | Solo chats, sin canales. | Se detecta y se dice; no se promete más. |
| X-07 | Los decodificadores del sistema son cuatro implementaciones nativas distintas. | F5b se alarga. | V1 sale útil con WebM y reproductor del sistema; V2 y V3 son incrementales. |
| X-08 | El ERP local diverge del servidor. | Cuentas distintas con los mismos datos. | Vectores dorados compartidos como condición de salida. |
| X-09 | VERI\*FACTU en un programa local no cumple la norma. | Riesgo legal para quien factura. | D-29: revisión legal antes; mientras, se dice en pantalla que no lo lleva. |
| X-10 | Datos en claro en un equipo sin cifrado de disco. | Quien acceda al equipo lo lee todo. | Es R2, pedido. Se documenta, y Ajustes → Datos dice si el disco está cifrado donde el sistema lo deja saber. |
| X-11 | Guardar de vuelta en docx u odt pierde contenido. | Documento empobrecido. | Formato nativo por defecto, copia del original, aviso, bloqueo tras importación recortada (O-07). |
| X-12 | `go-imap/v2` sigue en beta. | Cambios de API entre betas. | Una sola versión en el repo, la de la raíz; subida coordinada. |
| X-13 | En móvil, las apps se instalan por separado y no comparten cuentas. | Configurar la cuenta en cada app. | Primera entrega así; compartir llega en F11. |
| X-14 | El lector de correo con formato no aguanta el HTML real. | Mail se queda en texto. | La primera entrega ya es de texto, como hoy; M-16 se valida con un corpus antes de comprometerse. |
| X-15 | El tamaño del plan: son más de dos años-persona. | Se queda a medias. | Los hitos son entregables por sí solos; H1 ya es un producto. |

---

## 11. Lo que solo puede hacer el dueño

| # | Qué | Para qué | Cuándo hace falta |
| --- | --- | --- | --- |
| 1 | **Nombre de la edición** y de las apps («Sytue Local» es de trabajo). | Identidad, iconos, fichas de tienda. | Antes de publicar; barato de cambiar. |
| 2 | Registrar una **app pública en Microsoft Entra** (cuentas de empresa y personales, redirección a `http://localhost`, permisos de IMAP, SMTP, calendario, contactos y Teams) y verificar el editor. | Outlook, Microsoft 365, Teams. | Entrada de F7. |
| 3 | Registrar un **cliente OAuth de escritorio en Google Cloud** y pedir la verificación; el permiso de correo exige una auditoría de seguridad externa. | Gmail por OAuth, Google Calendar, contactos, Chat, Meet. | F7 y F8 (Gmail sale antes con contraseña de aplicación). |
| 4 | Decidir si Sytue distribuye una **app de Slack** y la presenta a su Marketplace. | Slack sin que la persona cree su app. | F9; no bloquea (D-20). |
| 5 | **Identificadores de app, grupo de apps y perfiles** en Apple para las 15 apps locales. | iOS y Mac App Store. | F11. |
| 6 | **Fichas en Google Play y App Store** para las 15 apps. | Publicar. | F11. |
| 7 | **Cuentas de prueba**: Outlook personal, un inquilino de Microsoft 365, Gmail, un Workspace de Google, iCloud, un espacio de Slack, un servidor CalDAV y CardDAV. | Pruebas de humo reales. | F7, F8, F9. |
| 8 | **Revisión legal de VERI\*FACTU** en un programa local, y si la declaración responsable lo cubre. | Entrega 4 del ERP. | Antes de E-07. |
| 9 | Confirmar que **conectar directamente desde el dispositivo** a Microsoft, Google y Slack, con los tokens en el equipo, entra en lo autorizado. El ADR 0077 autoriza el intercambio con esos proveedores pasando por la pasarela del servidor; este caso no está escrito en ninguna parte. | ADR 0083. | Antes de F7. |
| 10 | Aceptar la **excepción a la barra** de CONVENTIONS §13.6 (sin campana ni cuenta). | ADR 0083. | F0. |

---

## 12. Documentación que hay que tocar

`docs/` son diez ficheros; todo entra como sección.

| Documento | Qué entra |
| --- | --- |
| `docs/decisions.md` | **ADR 0083 — Edición local**: las reglas de §2 y las decisiones de §4, con lo descartado. Incluye la excepción de la beta de `go-imap/v2` y la de la barra. |
| `docs/CONVENTIONS.md` | §13.2: los paquetes nuevos en el catálogo (`local/*`, `cuentas/*`, `ia/proveedor`, `anfitrion`). §13.6: la excepción de la barra y dónde vive el código local. §3: las dependencias nuevas del núcleo. |
| `docs/architecture.md` | La edición local como pieza: árbol, espacio de datos, portero, almacén. |
| `docs/roadmap.md` | Las fases de §9 y lo que queda fuera. |
| `docs/security.md` | Modelo de amenazas de la edición local: datos en claro, secretos en el llavero, lista blanca de red, lo que se pierde sin cifrado. |
| `docs/operations.md` | Cómo empaquetar y publicar la edición; lista de humo por proveedor. |
| `compliance/supply-chain/` | Filas para los flujos nuevos: proveedores a los que se conecta **el dispositivo de la persona por elección suya** (correo, calendario, chat, IA). Es una categoría distinta de la actual, que supone la pasarela. |
| `apps/fyne/BACKLOG-FYNE.md` | Una línea que apunte a este plan. |
| Este fichero | Se mantiene como estado de la línea hasta que cada parte pase a `docs/`. |

---

## 13. Fuera de alcance

- **Servir nada a la red local** (galería de Cast, Drive por la red de casa): sería un servidor.
- **Sincronizar entre los dispositivos de la persona.** Sin servidor no hay. La carpeta de datos se
  puede sincronizar con la herramienta que la persona quiera, bajo su responsabilidad; las bases
  SQLite no aguantan que dos equipos escriban a la vez, y se avisa.
- **Avisos instantáneos con la app cerrada en móvil.**
- **Llamadas de Teams y de Meet dentro de la app.**
- **Colaboración**: compartir, coedición, comentarios entre personas, aprobaciones.
- **`.doc`, `.xls`, `.ppt`, `.rtf`.**
- **Cifrado propio**: cuadernos con contraseña, S/MIME, bloqueo biométrico.
- **Personas, caras y lugares en Photos**: no existen tampoco en la edición conectada.
- **«Parecidas» y sugerencias de álbum** en Photos.
- **Destino web** (`js/wasm`): la edición local es solo nativa.
- **Migrar automáticamente** de la edición conectada a la local, más allá de importar el ZIP de «Tus datos».

---

## 14. Lo comprobado y lo que falta por comprobar

**Comprobado directamente al redactar** (2026-11-18):

- El árbol está limpio, en `32aa6708da`; no hay ADR ni entrada de roadmap sobre una edición local; el
  último ADR es el 0082.
- Los repositorios de dominio usan del motor un subconjunto pequeño (`Mutate`, `Selected`, `ReadJSON`,
  `ReadRaw`, `ReadBinary`, `Catalog`, `Start` y el ciclo de vida; `notes`, `photos` y `cast` añaden
  `MutateWithContent`, ficheros retenidos y contenido pendiente).
- `records.newRepository` exige `Crypto`, `Gateway`, `Source` y `Journal.Encrypted()`
  (`storage/records/repository.go:103`).
- `mattn/go-sqlite3 v1.14.52` es dependencia de `apps/nucleo`; no hay IMAP, WebDAV ni OAuth en los
  módulos de cliente.
- Los productos válidos de un alcance (`storage/products.txt`) incluyen los quince que hacen falta.
- Rooms usa solo el primer equipo (`apps/fyne/apps/rooms/sala.go:620`).
- `api.Esquema` es `"sytue-" + producto`, y hay dos comprobaciones por ese prefijo.
- `go-imap/v2`: la última publicada es `v2.0.0-beta.9` (2026-11-14) y sigue marcada como preliminar.
- El límite de Slack para apps fuera de su Marketplace (1 petición por minuto, 15 mensajes, desde el
  2025-05-29) figura en su documentación.

**De memoria, sin contrastar hoy** (se comprueban al empezar su fase): la retirada de la
autenticación básica de Microsoft; que el permiso de correo de Google es restringido y exige
auditoría; que Socket Mode y una app interna de Slack no tienen el límite anterior; los permisos de
Graph para Teams y cuáles piden consentimiento de administrador; los plazos legales de VERI\*FACTU.

**No verificado en el código** (cada uno es la primera tarea de su fase):

| Qué | Fase |
| --- | --- |
| Que el motor acepte un historial compactado, sin los padres antiguos. | F0 |
| Que `office.Repository` se comporte bien sobre un puerto que no es Matrix: no tiene test de repositorio; el precedente es el de `hub`. | F0 |
| Cuánto de `sesion` se puede extraer sin cambiar comportamiento. | F0 |
| Qué hace el actualizador de la conectada con una clave de desinstalación de Windows que no conoce. | F0 |
| Las partes de `empaquetar.sh` que no se leyeron enteras (aproximadamente las líneas 560-660, 700-1230, 1260-1560, 1730-1795, 1830-1905, 2010-2290 y 2700-2955). | F0 |
| El camino de guardado del editor de Notepads cuando no hay diario. | F4 |
| Si el publicador de widgets nativos puede funcionar sin sesión (hoy vive en `sesion`). No se monta en la primera entrega. | F11 |
| Si las ayudas de `nucleo/auth` (loopback, llavero) están libres de supuestos de Sytue ID. | F7 |
| Si el modelo de texto de Notepads pinta de forma aceptable el HTML real de un correo. | F7b |
| Qué formatos de imagen admite de verdad `ImagenesAPDF` (el comentario dice JPEG y PNG). | F1 |
| El comportamiento del ERP conectado con el catálogo privado vacío. | F12 |
| Las filas marcadas «por búsqueda» en los inventarios: se mapearon por nombre de fichero y de llamada, sin leer cada cuerpo. | cada fase |

---

## Anexo A — Métodos sin camino local hoy

Llamadas que las pantallas hacen y que, con la fachada privada, fallan cerradas. Cada grupo es una
interfaz opcional nueva en `apps/nucleo/api` y su implementación (§5.7). «No se monta» significa que la
función se oculta en local y el método no se implementa.

**Docs, Calcus, Diapos** (`api.Docs`)

| Grupo | Métodos | En local |
| --- | --- | --- |
| Papelera | `Papelera`, `Restaurar`, `Purgar`, `Vaciar` | se implementa |
| Copia | `CreateCopy` | se implementa |
| Plantillas | `Templates`, `CreateFromTemplate` | se implementa (O-10) |
| Temas y marca | `Themes`, `ApplyTheme`, `PreviewTheme`, `SaveBrand` | se implementa |
| Bloques | `Blocks`, `Block` | se implementa |
| Actividad | `Actividad` | no se monta |
| Peticiones, coedición | `Peticiones`, `EdicionPedida`, `Coeditar`, `CoeditarConCursor`, `CreateShare` | no se monta |

**CertiPDF** (rutas de `localMotor`)

| Ruta | Código local que ya existe | En local |
| --- | --- | --- |
| `/pdf/compare` | `CompararPDF`, `CompararVisual`, `CompararVisualPagina` | se enruta |
| `/pdf/verify` | `VerificarFirmas` | se enruta |
| `/pdf/attachment` | `ExtraerAdjunto` | se enruta |
| `/pdf/pages` con capas | `MarcaDeAguaPDF`, `FondoPDF`, `NumerarPaginasPDF`, `NumeracionBatesPDF`, `EncabezadoPiePDF` | se enruta |
| `/pdf/sign/prepare`, `/complete` | `PrepararFirmaPDF`, `CompletarFirmaPDF` | se enruta |
| `/pdf/ocr` | solo `CapaDeTextoPDF` | P-07 |
| `/pdf/signature-requests…` | — | no se monta |

**Drive** (`api.Drive`)

| Métodos | En local |
| --- | --- |
| `Purge`, `CopyFolder`, `BatchMove`, `BatchTrash`, `Quota` | se implementa |
| `Shares`, `ShareLink`, `ShareWith`, `Revoke`, `Peticiones`, `SharedWithMe`, `SharedList`, `SharedContent`, `SharedUpload` | no se monta |
| `TeamDrive*`, `ExternalStorage*`, `ConnectNAS`, `NASStatus`, `ExportBasesView` | no se monta (ubicaciones remotas: DR-13, fase posterior) |

**Photos** (`api.Photos`)

| Métodos | En local |
| --- | --- |
| `TimelineDesde`, `TimelineYears`, `Purgar`, `PonerPortada`, `Memories`, `Duplicates`, `Nearby` | se implementa |
| `Similar`, `SimilarMany`, `Themes` | no se monta |
| `ShareAlbum`, `UnshareAlbum`, `PublicAlbum`, `PublicVariant`, `PersonasDelAlbum`, `AlbumesCompartidos`, `AlbumCompartidoConmigo`, `VarianteCompartida`, `Settings`, `SetExcludeAIBatch` | no se monta |

**Notes** (`api.Notes`)

| Grupo | Métodos | En local |
| --- | --- | --- |
| Diario | `DailyNotebook`, `DailyRange`, `ThisDay` | se implementa |
| Tareas | `ListTasks`, `PostponeTask` | se implementa |
| Etiquetas con estados | `SaveTag`, `DeleteTag`, `SetNoteState` | se implementa |
| Recordatorios | `SetReminder`, `DeleteReminder` | se implementa |
| Mover y archivar | `MoveNote`, `MoveNoteToNotebook`, `ArchiveNotebook`, `PurgeNote` | se implementa |
| Importar | `PreviewNotebook`, `ImportNotebook` | se implementa |
| Compartir | `Compartidas`, `PersonasDeCuaderno`, `PersonasDeNota`, `Peticiones` | no se monta |
| Cifrado | `MigrateEncryptedContent` | no se monta |

**Calendar** (`api.Calendars`)

| Métodos | En local |
| --- | --- |
| `ResponderPorUID`, `SearchEventsFiltrada` (con filtro) | se implementa |
| `Subscribe`, `RefreshSubscription`, `RespondInvitation`, `SetPartStat` (hoy fallan a propósito en el repositorio) | se implementa |
| `Peticiones`, `SharedWithMe`, `Share`, `RevokeShare`, `Resources`, `CreateResource`, `UpdateResource`, `DeleteResource`, `BookResource` | no se monta |

**Contacts** (`api.Contacts`)

| Métodos | En local |
| --- | --- |
| `BatchVCard`, `BatchRestore` | se implementa |

**Bases** (`api.Bases`)

| Métodos | En local |
| --- | --- |
| `SetFieldFormula`, `SetFieldChoices`, `ReorderFields`, `UpdateView`, `ListRecordsCon` con búsqueda u orden, `ImportCSV`, `Export` | se implementa |
| `UploadAttachment`, `DownloadAttachment`, `DownloadThumb`, `DeleteAttachment` | se implementa |
| `ListComments`, `CreateComment`, `DeleteComment` | se implementa (notas del registro) |
| `ListAudit` | fase posterior |
| `Events`, `Heartbeat`, permisos, formularios públicos, `TransferBase`, `MyGroups` | no se monta |

**Cast** (`api.Cast`)

| Métodos | En local |
| --- | --- |
| `Create` + `Upload` (como importar), `Update`, `Delete`, `UploadPoster`, `PosterFrame`, `Explore` y `Channel` (sobre lo local) | se implementa |
| `Shared`, `People`, `AddPerson`, `RemovePerson`, `Quota`, `DriveFile`, `FromDrive`, `Estimate`, `Qualities`, `SetQualities` | no se monta |
| `StreamKey`, `RotateStreamKey`, `ScheduleLive`, `PublishToken`, `EndLive`, comentarios, chat, diapositivas, denuncias y bloqueos | no se monta |

**ERP** (`api.ERP`): unos 60 métodos sin camino local (banco, libro, balance, cierre, obligaciones,
TPV, informes, adjuntos, actividades, importación, VERI\*FACTU). No se amplía la interfaz: se sirven
por el transporte en proceso (D-10), en las cuatro entregas de §6.13.

**Mail, Rooms, Hub:** no aplica. Mail y Rooms implementan sus interfaces de pantalla; el Hub local no
usa el cliente del Hub.

---

## Anexo B — Destinos de red permitidos

Ninguno vale sin que la persona lo haya configurado. Ajustes → Red enseña en cada momento la lista
real, calculada por el portero.

| Propósito | Cuándo se permite | Destinos |
| --- | --- | --- |
| `correo` | hay una cuenta de correo | los servidores IMAP, SMTP o POP3 de esa cuenta |
| `calendario`, `contactos` | hay una cuenta con ese servicio | su servidor CalDAV o CardDAV; las API de Google o de Microsoft Graph |
| `chat` | hay una cuenta de chat | la API y el canal en tiempo real de Slack; Microsoft Graph; la API de Google Chat y de Meet |
| `oauth` | durante el alta o la renovación de una cuenta | el servidor de inicio de sesión y de tokens de ese proveedor |
| `ia` | hay un proveedor de IA configurado | la dirección de ese proveedor (o `localhost` con un modelo local) |
| `sello-de-tiempo` | hay una autoridad configurada en CertiPDF | esa autoridad |
| `suscripcion-ics` | hay una suscripción de calendario | la dirección de ese calendario |
| `imagen-remota` | la persona pide cargar las imágenes de un mensaje o de una nota | el host de esas imágenes, solo para esa carga |
| `baja-de-lista` | la persona pulsa «Darse de baja» | el host que indica la cabecera del mensaje |
| `aeat` | VERI\*FACTU activado en el ERP | los extremos de la AEAT |
| descubrimiento | durante el alta de una cuenta | el dominio de la dirección escrita (registros SRV y `.well-known`) |

Y lo que **nunca** se contacta: `*.sytue.eu` y `*.sytue.com` (salvo el proveedor «Sytue Rooms online»
en Rooms, si la persona lo conecta), servidores de actualización, de telemetría o de fuentes, mapas y
cualquier otro contenido remoto que no esté en la tabla.
