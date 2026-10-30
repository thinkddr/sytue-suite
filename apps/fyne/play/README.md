# Google Play: las fichas de las quince apps

Todo lo que Play Console pide de cada app y se puede tener en el repositorio. Lo sube
`apps/fyne/scripts/play-publicar.sh` (workflow `play-publicar.yml`, y el paso opcional
de `release-mobile.yml`); el porqué y el funcionamiento, en «Google Play: ficha y subida
por la API» de `docs/operations.md`.

```
<app>/<idioma de Play>/title.txt               ≤ 30 caracteres (el Name de FyneApp.toml)
<app>/<idioma de Play>/short_description.txt   ≤ 80
<app>/<idioma de Play>/full_description.txt    ≤ 4000
<app>/graficos/icono.png                       512 × 512, RGBA de 8 bits
<app>/graficos/destacado.png                   1024 × 500
<app>/graficos/telefono-N.png                  1080 × 1920, de 2 a 8
<app>/seguridad-datos.csv                      «Seguridad de los datos», formato de exportación de Play
```

`<app>` es el directorio de `apps/fyne/cmd/`; el paquete sale de su `FyneApp.toml`
(`eu.sytue.<app>`, y `eu.sytue.one` para el Hub).

## Cómo se regenera

- **Textos y CSV**: `bun apps/fyne/play/generar.ts` desde la raíz. Los textos salen del
  catálogo de www (`product.<clave>.*`), que ya está en los 26 idiomas: la ficha dice lo
  mismo que la web y no hay otra traducción que mantener. Lo que la app del teléfono no
  hace se deja fuera clave a clave, en `FICHAS`, con el motivo al lado.
  **24 idiomas, no 26**: Play Console no tiene irlandés (`ga`) ni maltés (`mt`).
  El idioma por defecto es `es-ES`: las imágenes solo se suben a él, y Play las enseña en
  los demás.
- **Icono y gráfico destacado**: `bash apps/fyne/play/graficos.sh` (necesita `magick`, así
  que en el Mac). El icono es el `Icon.png` de la app; el destacado, la marca y el nombre
  sobre el papel de la suite (`--sytue-neutral-50`, tinta `--sytue-ink`, Figtree Bold).
- **Capturas**: el recorrido de cada app (`TestRecorridoConSesion`) contra su servicio
  falso, con `SYTUE_CAPTURAS_PLAY=<dir>`: en vez de comprobar, pone la ventana a
  360 × 640 a ×3 (1080 × 1920, disposición de teléfono) y deja una captura de cada
  pantalla y cada capa a la que se llega pulsando. Las de la ficha se eligen de ahí y se
  copian con `graficos.sh <dir>`, que lee la lista `CAPTURAS` de ese guion. La vía habitual
  es el CI, que importa esa selección con `graficos.sh --seleccion <app>`:

  ```sh
  gh workflow run ci-desktop.yml --ref <rama> -F empaquetado_solo=true \
    -F captura_fichas=true -f apps_fichas=todas -f clase_fichas=android
  ```

  `apps_fichas` también acepta una lista separada por espacios. El trabajo conserva el
  índice completo y solo los PNG seleccionados, como se describe en
  [`../appstore/README.md`](../appstore/README.md). Hay que volver a mirar las imágenes
  tras regenerarlas: los números del recorrido cambian al cambiar la interfaz.

  **Revisión visual completada el 2026-10-14: 45 PNG de teléfono de las quince apps**,
  de 1080 × 1920, RGB sin alfa. Se revisó
  [la regeneración completa de Android](https://github.com/thinkddr/sytue-one/actions/runs/38608561176)
  y se sustituyeron Rooms y Drive por las imágenes de
  [la regeneración dirigida](https://github.com/thinkddr/sytue-one/actions/runs/38613047745),
  también revisadas. De las 54 iniciales se retiraron dos de Calcus, una de Calendar y
  una de Docus con avisos de error del servicio falso, una de Drive y dos de Rooms,
  y las vistas opcionales de Photos y Diapos con controles recortados. `graficos.sh`
  conserva la selección final; los PNG conservan el render original.
  La revisión es de capturas del driver de software a medida de teléfono. La validación
  en dispositivos físicos sigue pendiente y las imágenes no acreditan el funcionamiento
  completo de las apps en Android.

## Lo que hace la API y lo que no

| Paso                                                                                            | ¿Por la API?                                                                      | Quién                                          |
| ----------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- | ---------------------------------------------- |
| Crear la app en Play Console                                                                    | No                                                                                | El dueño (trece el 25/09; **Cast, pendiente**) |
| Ficha (título, descripciones) en 24 idiomas                                                     | Sí, `edits.listings.update`                                                       | `play-publicar.sh`                             |
| Icono, destacado y capturas                                                                     | Sí, `edits.images.upload`                                                         | `play-publicar.sh`                             |
| Sitio web de contacto (`https://sytue.eu`) e idioma por defecto                                 | Sí, `edits.details.patch`                                                         | `play-publicar.sh`                             |
| Correo de contacto                                                                              | Sí, pero **no nos lo inventamos**                                                 | DECISIÓN del dueño                             |
| Política de privacidad (`https://sytue.eu/privacidad`)                                          | **No**: `AppDetails` no la tiene                                                  | El dueño, en _Contenido de la app_             |
| Seguridad de los datos                                                                          | Sí, `applications.dataSafety` (fuera de la edición)                               | `play-publicar.sh`                             |
| AAB a «internal» (como borrador)                                                                | Sí, `edits.bundles.upload` + `edits.tracks.update`                                | `play-publicar.sh --aab`                       |
| Testers de «internal»                                                                           | Solo **grupos de Google** (`edits.testers`); las listas de correos, en la consola | El dueño                                       |
| Países (30: UE + EEE + CH, sin Francia)                                                         | **No**: `countryAvailability` solo se lee                                         | El dueño; DECISIÓN abajo                       |
| Clasificación de contenido, público objetivo, anuncios, acceso, declaraciones, ID de publicidad | **No**                                                                            | El dueño, con las respuestas de abajo          |
| Enviar a revisión y a producción                                                                | No se hace                                                                        | El dueño                                       |

## Respuestas para _Contenido de la app_, pregunta a pregunta

Valen para las quince salvo donde se dice otra cosa. Salen del código y del inventario de
lo que sale a terceros (2026-09-14): sin rastreadores, sin SDK de terceros, sin anuncios,
sin analítica en las apps; los avisos de Android llegan por el SSE del Hub (sin Firebase).

### Política de privacidad

`https://sytue.eu/privacidad`.

### Acceso a la app

«**Toda o parte de la funcionalidad está restringida**». Todas piden iniciar sesión con
Sytue ID (se abre el navegador del sistema en `id.sytue.eu`). Instrucciones para el
revisor, que **escribe el dueño** con una cuenta de prueba suya (nadie más la tiene ni debe
tenerla en el repositorio):

> Pulse «Iniciar sesión»: se abre el navegador en id.sytue.eu. Usuario: `<la del dueño>`.
> Contraseña: `<la del dueño>`. La cuenta pertenece a una organización de demostración con
> correo, archivos, calendario y canales de ejemplo. No pide segundo factor.

La cuenta tiene que ir **sin segundo factor ni passkey obligatoria**, o el revisor se queda
en la puerta y Play rechaza la app. Una sola cuenta vale para las quince.

### Anuncios

«**No, mi app no contiene anuncios**».

### ID de publicidad

«**No**»: ninguna app lo usa (no hay SDK de anuncios ni de analítica, ni `AD_ID` en el
manifiesto).

### Clasificación de contenido (IARC)

- Correo de contacto: el del dueño.
- Categoría: «**Todas las demás tipos de apps**» (ni juego ni tienda).
- Violencia, sexualidad, lenguaje soez, sustancias, apuestas, contenido sensible: **No** a todo, en lo que la app trae
  de fábrica. **Sytue Cast es la excepción** en la pregunta de si los usuarios pueden compartir públicamente desnudos o
  violencia: ahí se declara **Sí** (decisión del dueño, 2026-10-06; el porqué y lo que haría falta para un «No», más
  abajo en «Bloquear y denunciar»).
- ¿Los usuarios pueden interactuar o intercambiar contenido? **Sí** en las quince: todas
  comparten dentro de la organización (Mail, Rooms y Cast además con otras; Cast, con
  portada pública). No cambia la edad
  (sale PEGI 3 / USK 0 / IARC 3+) y solo añade el aviso «Los usuarios interactúan».
- ¿Comparte la ubicación del usuario? **No**. ¿Compras digitales dentro de la app? **No**.
- ¿Es un navegador web o un buscador? **No**.

### Bloquear y denunciar (contenido generado por usuarios)

La clasificación pregunta si la app permite **bloquear** usuarios o contenido y **denunciar**
usuarios o contenido, y la política de contenido generado por usuarios lo exige donde el
contenido principal es de usuarios.

**Sytue Rooms** ([ADR 0073](../../../docs/decisions.md#0073)). Lo que se puede declarar, y dónde
está en la app:

| Pregunta de la consola                         | Respuesta | Dónde, en la app                                                                                                                                                                                |
| ---------------------------------------------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ¿Se puede bloquear a otros usuarios?           | **Sí**    | «…» de un mensaje → «Bloquear a…»; panel «Personas» → «…» de la persona. Se deshace en el menú del canal → «Personas bloqueadas»                                                                |
| ¿Se puede denunciar a otros usuarios?          | **Sí**    | Panel «Personas» → «…» de la persona → «Denunciar a…»                                                                                                                                           |
| ¿Se puede denunciar contenido?                 | **Sí**    | «…» de un mensaje → «Denunciar mensaje», con motivo (acoso, contenido sexual, violencia, spam, otro) y nota opcional                                                                            |
| ¿Hay moderación del contenido de los usuarios? | **Sí**    | Los administradores del equipo y de la organización reciben la denuncia y pueden descartarla, borrar el mensaje o expulsar del canal; y el operador de la plataforma (Sytue), en su propia cola |

- **Qué hace bloquear**, por si lo pide la revisión: quien bloquea deja de recibir los
  mensajes directos y los avisos de esa persona —lo aplica el servidor, no la app— y ve
  plegados sus mensajes en los canales que comparten. No se borra nada.
- **Condición para declararlo.** Es verdad cuando la versión que se sube a Play y el servicio
  desplegado llevan los dos esta función: la app sin el servicio enseña las entradas y
  falla al usarlas, y el servicio sin la app no tiene puerta. Antes de marcarlo, comprobar
  las dos cosas en la versión que se envía.

**Sytue Cast** ([ADR 0074](../../../docs/decisions.md#0074)). Lo que se puede declarar, y dónde
está en la app:

| Pregunta de la consola                         | Respuesta | Dónde, en la app                                                                                                                                                                                                                                                                                                                                                 |
| ---------------------------------------------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ¿Se puede bloquear a otros usuarios?           | **Sí**    | «Denunciar» junto al vídeo → «Bloquear a…»; «Bloquear a…» bajo cada comentario; «…» de un mensaje del chat. Se deshace en «Denuncias y bloqueos»                                                                                                                                                                                                                 |
| ¿Se puede denunciar a otros usuarios?          | **Sí**    | «Denunciar» junto al vídeo → «Denunciar a…»                                                                                                                                                                                                                                                                                                                      |
| ¿Se puede denunciar contenido?                 | **Sí**    | «Denunciar» junto al vídeo, bajo cada comentario, en el «…» de cada mensaje del chat y en la cabecera de un canal; con motivo (acoso, contenido sexual o desnudos, violencia, spam o engaño, derechos de autor, otro) y texto opcional. **También sin cuenta**, en lo público                                                                                    |
| ¿Hay moderación del contenido de los usuarios? | **Sí**    | Quienes administran la organización dueña del contenido ven la cola en «Denuncias y bloqueos» y pueden descartar, borrar el comentario o el mensaje, retirar el vídeo de la portada pública, despublicarlo o bloquear a la persona; nadie ve las denuncias contra sí mismo. Y el operador de la plataforma (Sytue) tiene su propia cola, con las mismas acciones |

- **Qué hace bloquear**, por si lo pide la revisión: quien bloquea deja de recibir los comentarios, los mensajes del chat
  y los avisos de directo de esa persona, y esa persona deja de poder comentar y escribir en los vídeos de quien la
  bloqueó. Lo aplica el servidor. No se borra nada.
- **Condición para declararlo**, la misma que en Rooms: que la versión que se sube a Play y el servicio desplegado lleven
  los dos esta función. Además, la denuncia sin cuenta pasa por la web de Cast solo si esa web está desplegada con ella.
- **La cola del operador** ([ADR 0075](../../../docs/decisions.md#0075)) es de la consola de plataforma, solo web: la
  app no cambia. Para contarla en la revisión hace falta que estén desplegados a la vez Sytue ID (la ruta
  `platform/moderation/access`), Cast (migraciones 0010-0012) y Rooms (migración 0039), y la consola de plataforma; y
  que el dueño haya revisado las condiciones y el DPA, porque es acceso de Sytue al contenido de los clientes.

**Desnudos y violencia en Cast: hoy se declara que SÍ pueden compartirse públicamente.** Es la decisión del dueño
(2026-10-06) y sigue siendo la respuesta verdadera: nada lo impide técnicamente, cualquiera con una cuenta puede publicar
un vídeo y aparece en la portada sin revisión. Para poder responder «**No**» harían falta, todas (estado al 2026-10-12):

1. **Condiciones de uso que lo prohíban**, aceptadas por quien publica, y enlazadas desde la ficha y desde la app.
   **Falta**: es texto legal, decisión del dueño.
2. **Retirada tras denuncia hecha por Sytue**, no solo por la organización que publica. **Hecho en código, sin
   desplegar**: la cola del operador ([ADR 0075](../../../docs/decisions.md#0075)) recibe todas las denuncias —también
   las que son contra quien administra, que esa persona ya no ve— y puede retirar de la portada, despublicar, borrar y
   bloquear. **Falta** comprometer un plazo de respuesta y que alguien mire la cola: el operador no recibe avisos.
3. **Capacidad de actuar contra reincidentes.** **A medias**: suspender una organización ya existe en la consola de
   plataforma (con segunda persona), y el bloqueo en la organización cierra la puerta a una persona dentro de ella.
   **No existe** suspender a una persona en toda la plataforma, ni quitarle a una cuenta u organización la publicación
   en abierto sin suspenderla entera.
4. **Una decisión sobre la portada**: que un vídeo no llegue a «Explorar» sin algún filtro (revisión previa, antigüedad
   de la cuenta o detección automática), o aceptar por escrito que la retirada es posterior a la publicación.
   **Falta**: decisión del dueño.
5. **Contacto de abuso publicado** para quien no usa la app (un correo), atendido. **Falta.** (La denuncia sin cuenta
   desde la web pública ya existe y llega a las dos colas.)

Con 1, 2 —desplegado y con plazo— y 5 se puede sostener «no está permitido y se retira». Hasta entonces la respuesta
honrada sigue siendo «sí».

**Qué se puede declarar hoy, en una línea.** Bloquear usuarios, denunciar usuarios y contenido y moderación: **Sí** en
Rooms y en Cast, en cuanto la versión que se sube y los servicios desplegados lleven la función (la cola del operador
añade moderación de Sytue cuando esté desplegada). Desnudos y violencia en Cast: **Sí** pueden compartirse.

### Público objetivo y contenido

- Edad del público objetivo: **18 años o más en las quince apps**. Decisión explícita
  del dueño, confirmada el **2026-10-15** para Google Play.
- ¿Podría atraer a niños sin querer? **No**.

### Seguridad de los datos

Se sube por la API desde `seguridad-datos.csv`. Resumen de lo declarado:

- Se **recoge** (va a nuestros servidores) y **no se comparte** con nadie: Scaleway, Mollie
  y Sweego son encargados, que Play no cuenta como «compartir».
- Todo cifrado en tránsito (TLS 1.3). Se puede pedir la eliminación de los datos
  (`PSL_SUPPORT_DATA_DELETION_BY_USER` = sí).
- En todas: nombre, correo, ID de usuario (la cuenta: funcionalidad y gestión de la cuenta;
  obligatorio) e historial de búsqueda en la app (efímero).
- Además: Hub → teléfono (opcional; el alta guiada lo verifica por SMS); Mail → correos y
  ficheros; Drive → ficheros, fotos y vídeos; Docus, Calcus, Diapos y CertiPDF → ficheros
  y documentos; Calendar → calendario; Contacts → contactos; Notepads → contenido creado
  por el usuario y ficheros; Photos → fotos; Rooms → otros mensajes, audio y vídeos (la
  voz y la cámara o la pantalla de una llamada; no efímeros, porque la reunión se puede
  grabar desde el teléfono), fotos y ficheros; Cast → vídeos y su audio (la grabación de
  un directo se queda como vídeo), fotos (el póster propio), contenido creado por el
  usuario (título y descripción) y otros mensajes (chat y comentarios), todo opcional, e
  interacciones con la app (quién está viendo un directo, guardado mientras dura); ERP → «otra información financiera»
  (los libros de la organización); Bases → contenido creado por el usuario (lo que la
  organización escribe en sus tablas; los adjuntos no se suben desde la app).
- **Alta de la cuenta**: en Sytue ID, con **usuario y contraseña** (`PSL_ACM_USER_ID_PASSWORD`);
  el SMS del móvil y las invitaciones de la organización no cuentan como otro método. Sin
  esta respuesta la API rechaza el CSV entero («Response missing for
  PSL_SUPPORTED_ACCOUNT_CREATION_METHODS»).
- **Eliminar la cuenta**: el enlace web que pide Play es `https://id.sytue.eu/account/delete`
  (`PSL_ACCOUNT_DELETION_URL`), en las quince. Tiene que funcionar antes de enviar a revisión:
  Play rechaza la app por «Invalid account/data deletion link» si no carga.
- **Eliminar datos sin eliminar la cuenta** (`PSL_DATA_DELETION_URL`): la política de
  privacidad, `https://sytue.eu/privacidad`, que explica cómo pedir la supresión. Sin esta
  respuesta la API también rechaza el CSV («Response missing for PSL_DATA_DELETION_URL»).

### Apps gubernamentales

«**No**» en las quince.

### Funciones financieras

- Trece apps: «**Mi app no ofrece ninguna función financiera**».
- **ERP**: contabilidad por partida doble, facturación y VERI*FACTU para la organización; no
  mueve dinero, no da préstamos ni guarda tarjetas. **DECISIÓN**: declararla en
  «Servicios de soporte y gestión» (herramientas de gestión financiera/contable) por
  transparencia, o «ninguna» si la consola no tiene esa casilla. Si algún día se conecta al
  banco (PSD2, pendiente del dueño), la respuesta cambia.

### Apps de salud

«**No**» en las quince (ninguna trata datos de salud).

### Apps de noticias

«**No**» en las quince.

### Permisos

- Todas: `INTERNET`, `POST_NOTIFICATIONS` (avisos, Android 13+) y `USE_BIOMETRIC` (el
  bloqueo de la sesión). Ninguno necesita declaración aparte.
- Hub: `FOREGROUND_SERVICE` y `FOREGROUND_SERVICE_SPECIAL_USE` (mantener abierto el flujo
  de avisos sin Firebase).
- Rooms y Cast (`llamadas = "si"` en su `FyneApp.toml`): además `RECORD_AUDIO`,
  `MODIFY_AUDIO_SETTINGS`, `CAMERA`, `FOREGROUND_SERVICE` y
  `FOREGROUND_SERVICE_MEDIA_PROJECTION` (la voz y el vídeo de una llamada en Rooms; emitir
  en directo con cámara, micrófono o pantalla en Cast). Se piden al usarlos, no al
  instalar. Ninguna pide la ubicación, SMS, registro de llamadas ni «todos los ficheros».
- **Servicios en primer plano** (_Contenido de la app → Permisos de servicios en primer
  plano_): Play pide declararlos a quien apunta a Android 14+. Rooms y Cast, «Proyección
  multimedia» (compartir la pantalla en una llamada o emitirla en directo; lo empieza la
  persona y lo ve en la notificación); Hub, «Uso especial» (recibir los avisos de la
  organización sin Firebase). Cada declaración pide un vídeo corto del uso: lo graba el
  dueño.

## Por app: categoría y lo que queda

**Categoría** (DECISIÓN; en la consola, _Presencia en tienda → Configuración de la tienda_):

| App               | Paquete             | Categoría propuesta               | Etiquetas            |
| ----------------- | ------------------- | --------------------------------- | -------------------- |
| Sytue Hub (`hub`) | `eu.sytue.one`      | Productividad                     | —                    |
| Sytue Mail        | `eu.sytue.mail`     | Comunicación                      | Correo               |
| Sytue Rooms       | `eu.sytue.rooms`    | Comunicación                      | Mensajería de equipo |
| Sytue Contacts    | `eu.sytue.contacts` | Comunicación                      | Contactos            |
| Sytue Calendar    | `eu.sytue.calendar` | Productividad                     | Calendario           |
| Sytue Drive       | `eu.sytue.drive`    | Productividad                     | Almacenamiento       |
| Sytue Docus       | `eu.sytue.docus`    | Productividad                     | Procesador de textos |
| Sytue Calcus      | `eu.sytue.calcus`   | Productividad                     | Hoja de cálculo      |
| Sytue Diapos      | `eu.sytue.diapos`   | Productividad                     | Presentaciones       |
| Sytue CertiPDF    | `eu.sytue.certipdf` | Productividad                     | PDF                  |
| Sytue Notepads    | `eu.sytue.notes`    | Productividad                     | Notas                |
| Sytue Photos      | `eu.sytue.photos`   | Fotografía                        | Galería              |
| Sytue Cast        | `eu.sytue.cast`     | Reproductores y editores de vídeo | Vídeo                |
| Sytue ERP         | `eu.sytue.erp`      | Empresa                           | Contabilidad         |
| Sytue Bases       | `eu.sytue.bases`    | Productividad                     | Bases de datos       |

**Países** (DECISIÓN): **30**, los 26 de la UE sin Francia, Islandia, Liechtenstein,
Noruega y Suiza. Se conserva la exclusión de Francia decidida por el titular el
**2026-10-14**, posterior a la lista de 31 de septiembre. La disponibilidad de Play
se fija por esa decisión de distribución del titular.

El **2026-10-25** quedaron guardados esos 30 países en las **quince apps** de Play
Console, sin enviar los cambios a revisión ni publicar. Se configuran en _Prueba y
publicación → Producción → Países o regiones_; en «internal» no hace falta.

**Lo que le queda al dueño en CADA una de las quince**, en este orden:

0. Las quince apps, incluidas Cast y Bases, **existen y permiten editar con la cuenta de
   servicio**, confirmado el 2026-10-14 por
   [el sondeo de Play](https://github.com/thinkddr/sytue-one/actions/runs/38608638019).
   `modo=produccion` exige en las quince el `versionCode` exacto del `tag` solicitado en
   `internal`; una versión diferente detiene la preparación antes de modificar releases.
   **Bases, además, no está vista en un teléfono**: sus capturas son del recorrido a 360 × 640 contra el
   servicio falso (la cabecera y los diálogos se ajustaron ese día; la rejilla se
   desplaza de lado). Antes de promoverla a producción, abrirla en un teléfono de verdad.

1. _Contenido de la app_: política de privacidad, acceso (con la cuenta de prueba),
   anuncios, ID de publicidad, clasificación, público objetivo, gubernamental, financieras,
   salud y noticias, con las respuestas de arriba.
2. _Presencia en tienda_: categoría, etiquetas y correo de contacto.
3. Comprobar que `https://id.sytue.eu/account/delete` responde: el CSV ya lleva el enlace.
4. Lanzar el workflow `play-publicar.yml` con `modo=sondear`, luego `modo=ficha`, y
   `modo=todo` con el tag cuando haya AAB firmados (Codemagic `android-fyne` +
   `release-mobile.yml`).
5. En «internal»: la lista de testers y **publicar el borrador** que deja la API.
6. Producción: preparar los borradores con `play-publicar.yml`, `modo=produccion`,
   `apps` vacío, `tag=vX.Y.Z`, `estado_produccion=draft` y la confirmación exacta
   `PUBLICAR_15_APPS`. Reutiliza los AAB ya firmados de ese tag en `internal`, sin
   volver a subirlos ni cambiar países o ficha, y deja borradores sin revisión ni
   publicación pública. Comprueba las quince antes de modificar la primera; conserva
   cualquier otra release de producción y repetir el mismo borrador no lo sobrescribe.
   **Las quince apps siguen en estado Draft y sus releases de internal son borradores**:
   no repetir `completed` en ese estado. La
   [primera publicación se completa en Play Console](https://developers.google.com/android-publisher/edits),
   con sus formularios y consentimientos; si se empieza por pruebas internas, también
   hay que activar su release en Console. Solo cuando el estado de la app permita
   publicar por la API y estén completas la ficha, cuenta demo, países y borrado,
   `estado_produccion=completed` podrá solicitar revisión del mismo AAB. Si una app ya
   está en revisión, falla sin cancelarla; un error de Google no provoca otro estado
   automático. Un borrador preparado no acredita que Play haya publicado o revisado la app.
