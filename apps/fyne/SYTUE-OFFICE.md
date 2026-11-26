# Sytue Office — la edición local de las apps nativas

**Sytue Office** son las apps nativas de la suite (Fyne: macOS, Windows, Linux, iOS y
Android) funcionando **enteras en el dispositivo**:

- **sin cuenta**: no hay inicio de sesión con Sytue ID;
- **sin cifrado propio**: los datos están en claro, en formatos legibles, en una carpeta de
  la persona (la protección en reposo es la del cifrado de disco del sistema);
- **sin servidores de Sytue**: recién instalada y sin configurar nada, no abre ni una
  conexión de red.

Opcionalmente, y solo si la persona lo configura en **Ajustes**:

- **Nube**: Sytue Drive, cualquier WebDAV (Nextcloud…), Google Drive y OneDrive /
  Microsoft 365; y, sin configurar nada, las carpetas que ya sincroniza el sistema.
- **IA**: Anthropic (Claude), OpenAI, Google Gemini u otro servicio compatible con OpenAI
  con la clave de la persona; un modelo en el propio equipo (Ollama, LM Studio); o la IA
  del sistema operativo donde hay una puerta para otras apps.

Es una **modalidad adicional**: las apps conectadas de `apps/fyne/cmd/*` no cambian, y las
dos ediciones pueden estar instaladas a la vez sin pisarse (identificador, esquema,
carpeta de ajustes, llavero y carpeta de datos distintos). El diseño completo y sus
decisiones están en [`PLAN-EDICION-LOCAL.md`](PLAN-EDICION-LOCAL.md); este documento dice
qué hay hecho y cómo se usa. «Sytue Local» era su nombre de trabajo. La decisión está en
la [ADR 0083](../../docs/decisions.md#0083) y lo que falta, en el
[roadmap](../../docs/roadmap.md#sytue-office-lo-que-falta).

## Apps

Cada app es un binario aparte en `apps/fyne/cmd-local/<app>/` (identificador
`eu.sytue.office.<app>`, esquema `sytueoffice-<producto>`, ejecutable `sytue-office-<app>`)
y monta las MISMAS pantallas que la edición conectada. Todas se han compilado con las
etiquetas de la edición y abierto en una pantalla virtual sin pedir cuenta.

| App | Datos | Funciona | Aún no (lo que falla cerrado o se oculta) |
| --- | --- | --- | --- |
| **Docus, Calcus, Diapos** | `local/biblioteca` (un JSON por documento); ficheros del disco anclados por `file://` + huella | Edición completa, versiones, comentarios, importar y exportar (docx/xlsx/pptx, ODF, PDF, md, csv…) en el dispositivo, IA, guardar en la nube; **formato nativo** `.docus`/`.calcus`/`.diapos` (ZIP sin pérdida); abrir un fichero lo **ancla** a su documento (abrirlo otra vez da el mismo); **Guardar** (Ctrl/Cmd+S) de vuelta en el fichero —un formato ajeno pregunta la primera vez y deja `nombre (original).ext`; recortado, plantilla o formato que no se escribe: solo «Guardar como»— y **no pisa** un fichero cambiado por fuera; **Guardar como…** con el selector del sistema; papelera, «Hacer una copia», galería de plantillas, temas y bloques sin servidor; Diapos lee el JSON antiguo; «Compartir» no sale; `.docus`/`.calcus`/`.diapos` declarados a «Abrir con» (`api.AppQueAbre` con `api.FormatosOffice`, `FyneApp.toml`, tipo MIME y UTI propios, y en Linux su `shared-mime-info`, que `instalar.sh` registra); «Exportar en formatos abiertos» (Ajustes → Copia) deja cada documento en OOXML; el PDF accesible de la conectada declara el idioma de la app (O-11) | Marca de organización y plantillas de organización (no hay organización); NSSavePanel de la variante de la Mac App Store sin hacer; en el móvil, «Guardar como…» guarda en una copia de trabajo de la app que un reflejo lleva al documento elegido (Android: también tras reiniciar; iOS: mientras la app siga abierta) |
| **CertiPDF** | Los PDF donde estén (`local/archivos`), sin copiarlos | Ver, anotar, formularios, páginas, marcas de agua, fondo, numeración y Bates, encabezado y pie, proteger, comparar, adjuntos; guardar en su sitio con versión anterior; firma con certificado de fichero o del sistema (tarjeta por PKCS#11, Llavero, almacén de Windows, KeyChain), en dos pasos en el equipo; sello de tiempo de la autoridad que elija la persona (o sin sello, y se dice); verificar firmas contra los certificados de confianza de una carpeta suya, sin consultar revocación; OCR con `tesseract` si está instalado; si no, en Windows, **el OCR del sistema** (`Windows.Media.Ocr` por WinRT en Go puro, `nucleo/sistema/ocrwindows`: los idiomas de reconocimiento que ya trae Windows, con los códigos de tesseract en el selector, y las cajas de cada palabra; cualquier fallo cae a lo siguiente) y, si no, el modelo de visión de la persona; IA con aviso la primera vez (P-01 a P-10); la autoridad y las anclas también en su pestaña de Ajustes; un `content://` de Android hasta los 100 MiB del motor (`local.LeerLocalHasta`), también el que le da Drive desde la carpeta elegida del teléfono | Pedir firma a otra persona; OCR propio de Apple (Vision); el de Windows está compilado en cruzado y SIN PROBAR en un Windows |
| **Notepads** | Motor de registros en claro | Cuadernos y notas, borradores que sobreviven a cerrar, importar `.enex`/`.onex`/`.md` abriendo el fichero, IA; el aviso pulsado (o el Hub) abre su nota en su cuaderno; «Añadir al calendario» abre Calendar local con el formulario relleno (N-08); **ficheros del equipo**: «Añadir una imagen de este equipo…» (el «…» del cuaderno, `ficheros.ElegirImagenes`) o soltar una imagen sobre el cuaderno crea una **nota de imagen con la imagen dentro** (su adjunto en `data.attachments`, sin `data.url`; reducida a 2048 px si no cabe en los 5 MB de la nota) que se ve sin red (N-06); las notas de imagen leen lo mismo que Photos (`local/imagenes`): además de PNG, JPEG, GIF y WebP, **AVIF, TIFF y BMP** y **HEIC** pasada a JPEG con el sistema, y lo que no es de la web se guarda en JPEG o PNG (BMP, siempre en PNG: capturas y dibujos sin las manchas del JPEG); **la nota de imagen por URL** no se baja sola: su tarjeta dice de dónde es y ofrece «Cargar la imagen», y «Añadir una imagen de una dirección web…» (el «…» del cuaderno) la crea con la dirección que se escribe; la imagen se baja solo entonces, por el permiso puntual del portero (`red.Portero.Puntual`, propósito `imagen-remota`: solo https, solo direcciones públicas, nada con «Sin conexión»), y se guarda como adjunto de la nota con su dirección por origen, así que desde entonces se ve sin red (`notes.NewLocal` admite la nota de imagen con una `data.url` web); **«Volver a cargar la imagen»** en el «…» de la nota que ya la lleva dentro la baja otra vez por el mismo permiso puntual (mismas reglas) y SOLO si llega bien sustituye a la de dentro, que deja la nota y la memoria (la compactación del almacén la quita del disco); si llega la misma, no se escribe nada y se dice; **soltar un fichero** sobre una nota de texto o tarea le añade su enlace `file://` (donde está el cursor, si es la que se escribe) y sobre el cuaderno crea una nota con él; el enlace se abre con su app de la edición o la del sistema (N-07); el repositorio se abre con `notes.NewLocal`, que guarda esas notas de imagen y esos enlaces (el de la conectada los sigue rechazando y quitando); lo **«borrado del todo» se va del disco en el acto** (`almacen.Recursos.CompactarYa`, en segundo plano y juntando las seguidas; si otra copia lo tiene abierto, al cerrar); exportar cada cuaderno a Markdown; **diario** (notas diarias y «Este día»), **tareas** y posponer, **recordatorios** (los avisa el sistema a su hora), etiquetas con estados, mover, archivar y purgar, e «Importar notas…» desde la pantalla, en el dispositivo (`nucleo/notes`: `diario.go`, `recordatorios.go`, `importar.go`; N-03) | HEIC solo con un conversor del sistema (el de Photos); la imagen por URL se vuelve a bajar solo a mano («Volver a cargar la imagen»), nunca sola; en el móvil no hay soltar |
| **Calendar** | Motor de registros en claro; el estado de cada cuenta en `Datos/calendar/agenda/<cuenta>.json` | Calendarios y eventos con recurrencias, importar `.ics`, suscribirse a un calendario por URL (por el portero, propósito `suscripcion-ics`), recordatorios que avisa el sistema a su hora, «¿Vas a ir?» en los eventos donde el correo del perfil es invitado (la respuesta queda en su `.ics` y, en una cuenta sincronizada, sube con él: `calendar.Repository.ResponderPorUID`, A-02), un calendario de cumpleaños de solo lectura leído de Contacts (`cumpleanos.go`, A-07), exportar a la nube, asistente de eventos; `sytueoffice-calendar:/events/new?action=create&title=&description=&attendees=` (Mail, Notepads) abre el formulario relleno, con los participantes en la descripción; exportar cada calendario a ICS; **cuentas CalDAV** (iCloud, Fastmail, Nextcloud, Sytue… con contraseña de aplicación), **Google Calendar** y **Microsoft 365/Outlook** sincronizadas (`local/agenda`): bajada incremental (`sync-collection` o ctag+ETag, `syncToken`, `delta`), cola de subida con reintento, conflictos sin perder lo de aquí («(copia en conflicto)»), al abrir, al volver, cada 10 min y a mano; el menú las agrupa por cuenta y marca «Solo lectura»; se dan de alta, se vuelve a entrar (sin perder calendarios ni lo sincronizado) y se quitan en Ajustes → Cuentas (iCloud, Fastmail, Sytue, Yahoo, cualquier CalDAV/CardDAV con descubrimiento por SRV y `.well-known`, Google y Microsoft por OAuth), con un único registro por app (`l.Agenda()`); invitaciones que manda el proveedor (CalDAV con planificación, Google, Graph) y, con un CalDAV **sin planificación, por correo (iMIP, A-11)**: REQUEST, CANCEL (también a quien se quita) y el REPLY de la persona salen por la cuenta de Mail con la dirección del organizador (o de quien contesta), por `correo.Remitente`, con el `.ics` adjunto, solo si cambió lo que ven los invitados (`agenda.Cartero`, `imip.go`); sin cuenta de Mail con esa dirección o si el envío falla, «Invitaciones sin enviar»; **«Crear calendario» en la sección de cada cuenta** (el formulario de siempre; `MKCALENDAR` en CalDAV, la API en Google y Microsoft); en **Outlook**, la respuesta de quien no organiza sube con `accept`/`tentativelyAccept`/`decline`, las horas van en la zona del evento (de Windows a IANA, también la regla), las ocurrencias canceladas de una serie bajan como EXDATE y las quitadas y las excepciones hechas aquí suben a su ocurrencia; **invitar desde el formulario** en los eventos de un calendario de una cuenta (`calendario.Pantalla.Invitar`, campo opcional: la conectada no cambia; `cmd-local/calendar/invitar.go`): el campo de los invitados vuelve con el **autocompletado de la libreta de Contacts** de este equipo (`lector.Libreta`, en memoria y releída cuando cambia su diario; flechas e Intro), una pista de por dónde saldrá la invitación (Google, Outlook, el servidor CalDAV que planifica o el correo desde la cuenta de Mail con esa dirección, o cuándo saldrá si aún no la hay), y el evento se guarda con su ORGANIZER —la dirección de la persona en esa cuenta, que da el proveedor (`calendar-user-address-set`, el calendario principal de Google, `/me` de Graph) y queda en el estado— y se sincroniza en el acto; en un calendario de solo este equipo no se invita y el formulario dice dónde se puede (no está en ningún servidor: nadie vería sus cambios hechos en otro equipo y las respuestas llegarían a Mail sin volver al evento); **cola de las invitaciones por correo que no salen** (`agenda.ColaIMIP`, cada una tal cual se compuso en `Datos/calendar/agenda/imip/<id>.eml` con su ficha `.json`): se reintentan con espera creciente (30 s a 1 h) a su hora, al arrancar y, sin esperar, cuando la red vuelve; lo que espera una cuenta de Mail sale en cuanto la hay; un envío posterior a las mismas personas deja viejo el anterior; lo de más de un mes se tira; el aviso dice cuántas quedan; **ocupación de los invitados** en «Buscar un hueco» (A-13): los huecos comunes restan los ratos ocupados que da el proveedor de la cuenta del calendario —CalDAV por el buzón de salida de la planificación (VFREEBUSY, RFC 6638), Google `freeBusy`, Graph `getSchedule`— y el panel dice con quién se ha contado y de quién no se sabe; **las respuestas de los invitados que llegan por correo** (iMIP REPLY) a lo que la persona organiza en un CalDAV sin planificación: Mail las deja en su cola y Calendar las recoge al abrirse, en cada vuelta y, abierta, en cuanto llega una (la mira cada 20 s, sin red), pone el PARTSTAT del invitado en el evento (`agenda.Registro.AplicarRespuestas`: el evento con ese UID en una colección que no planifica, organizado por la persona y con ese ORGANIZER, el invitado en la lista, la SEQUENCE no anterior y, por invitado, el DTSTAMP más nuevo que el último aplicado; una respuesta a una ocurrencia, en su excepción, y si aquí no la hay, en una nueva —la copia del maestro en su hora— marcada como venida de una respuesta) y la vuelta lo sube sin mandar correo a nadie (ni el Cartero ni su cola); en Google, Outlook o un CalDAV que planifica la aplica el proveedor y se descarta; lo que aún no ha bajado espera hasta un mes (`respuestas.go`); **enlace de la reunión desde el formulario** (la otra mitad de A-13; `calendario.Pantalla.Reunion`, campo opcional: la conectada no cambia; `reunion.go`): en un evento de un calendario de Google, «Añadir Google Meet» —al guardar, el evento queda marcado (`X-SYTUE-REUNION`, `calendar.Repository.PonerExtension`, que no sale del equipo) y la subida lo pide con `conferenceData.createRequest` y `conferenceDataVersion=1`, sin otro permiso—; en uno de Microsoft, «Reunión de Teams» (`isOnlineMeeting` con `onlineMeetingProvider: teamsForBusiness`); el evento vuelve en la misma vuelta con su enlace (el sincronizador guarda lo que el proveedor devuelve tras subir, con el UID de aquí); en los demás, si hay una cuenta de Rooms con Teams o Meet, «Crear reunión de Microsoft Teams/Google Meet» la crea ya con ella (`chat.Registro`, por el portero, sin abrir Rooms; Teams con el título y las horas del evento) y pega el enlace; si no, el campo libre de siempre; **guardar el formulario no pisa lo que contestaron entre medias** (`calendar.Repository.ConservarRespuestas`, campo opcional que enciende `cmd-local/calendar`: la conectada no cambia; `calendar/ical.ConservarRespuestas`): el PARTSTAT de un invitado que cambió en el almacén desde que se abrió el formulario —la respuesta que llegó por correo, la que bajó de la cuenta— se queda, salvo de quien la persona quitó o cambió de ROLE, que queda como ella lo dejó; las excepciones (RECURRENCE-ID) llevan lo contestado a cada ocurrencia, también la que creó una respuesta mientras tanto, y quitar o añadir a alguien a la serie vale también en ellas (quien se quitó no vuelve, ni porque llegue otra vez su respuesta); la SEQUENCE solo sube si cambia lo que ven los invitados (`ical.CambiaLaInvitacion`, la misma lista que la firma del iMIP), así que la respuesta que se aplica después de guardar un recordatorio sigue valiendo; conservar una respuesta no manda invitaciones ni deja nada en la cola; **«solo este» y «este y los siguientes» tampoco pierden lo contestado** (`calendar.Repository.DividirSerie`, la opción `api.CalendarSerieData` que usa `calendario.EditarSerie`; sin ella, crear y recortar como siempre): al guardar se relee la serie y el evento nuevo lleva lo que cada invitado contestó a ESA ocurrencia (su excepción, la que creó una respuesta mientras tanto, o el maestro) o a la serie, con el organizador de la serie; «este y los siguientes» se lleva además las excepciones desde esa ocurrencia (con su UID nuevo y lo contestado en cada una) si se repite igual y a la misma hora; recortar la serie quita las excepciones de lo recortado, que se seguían viendo, y la EXDATE nueva va en la hora de la zona del evento (`ical.DividirConRespuestas`, `ical.RecortarSerie`); **lo que la persona cambia en la serie llega a sus excepciones** (`SerieALasExcepciones`, `ical.LlevarALasExcepciones`): título, lugar, descripción, estado, organizador, clase, transparencia, color, videollamada y recordatorios, a cada excepción que lo tenía como la serie —también la que creó una respuesta—; la que lo tenía cambiado a propósito lo conserva, la hora no se mueve por esto (sí al mover la serie) y la que recibe algo que ven los invitados sube su SEQUENCE; **guardar desde el formulario no reescribe lo que no cambió** (`ConservarForma`, `ical.ConservarForma`): la EXDATE en la hora de su zona, sin el STATUS, CLASS ni TRANSP que no traía, el fin con DURATION, la regla en su orden, cada invitado y el organizador con sus parámetros, los recordatorios con su texto y la SEQUENCE como entero; y **lo que ven los invitados se compara por lo que significa** (`ical.ContenidoDeLaInvitacion`: la misma hora en otra zona, el mismo conjunto de EXDATE, STATUS ausente = CONFIRMED, DTEND o DURATION, el UNTIL como la última repetición que deja…), para subir la SEQUENCE y en la firma del iMIP (`agenda`, con las firmas guardadas antes entendidas): un evento de la cuenta guardado sin cambios no manda invitación ni sube la SEQUENCE; los textos del `.ics` se leen sin escapes (un título «a\, b» de iCloud salía con la barra y se volvía a escapar en cada guardado, también en la edición conectada); **el formulario (compartido con la conectada) ya no ancla las series ni mueve la hora**: «todos» desde cualquier ocurrencia conserva el DTSTART del maestro, que se lee (la ocurrencia trae el suyo), y le aplica lo que se movió en días y hora de pared (`calendario.desplazarMaestro`), y «este y los siguientes» deja a la serie nueva lo que queda del COUNT; **mover la hora (o el día) de una serie mueve con ella sus EXDATE y sus excepciones** (antes se quedaban en la hora vieja: la ocurrencia quitada volvía a salir y las excepciones se perdían): la pantalla manda las EXDATE desplazadas igual que el DTSTART, también las que quedan en la serie nueva de «este y los siguientes» (`calendario.desplazarFechas`), y el almacén desplaza el RECURRENCE-ID de cada excepción (`ical.CarryOver`, también en el servicio), cada fecha escrita como venía (en su zona, en UTC o como fecha, `ical.ConservarForma`); la excepción que estaba a la hora de su ocurrencia se va con ella y la que estaba a otra hora a propósito se queda; los campos dicen la hora del evento en SU zona, la de «Zona horaria» (`EdicionDe`), así que abrir y guardar no mueve la hora se pinte en la zona que se pinte (lo que no se toca se queda en su instante, también la hora que se repite al volver al horario de invierno), un evento sin zona se queda sin ella, y elegir un hueco y crear la reunión van a esa hora | Que la respuesta a un evento importado o de un calendario de este equipo llegue al organizador (las invitaciones que llegan por correo se contestan en Mail, M-17); «solo este» y «este y los siguientes» siguen sacando la ocurrencia a un evento con otro UID (los invitados reciben uno nuevo); un evento cuya hora no existe en la zona del sistema (la que se salta al pasar al horario de verano, si la del evento no la salta ese día) se enseña en el formulario con una hora de diferencia, porque los campos van en la zona del sistema (guardarlo sin tocar la hora lo deja donde estaba); la ocupación por CalDAV solo con un servidor que planifica (sin buzón de salida no la da), y Google y Microsoft solo la dan de quien comparte su calendario o es de su organización; «Reunión de Teams» pide `teamsForBusiness` (una cuenta personal de Outlook puede no darla), y la sala de Meet creada con Rooms no lleva título ni horas; Graph, la ocupación, el iMIP, las respuestas por correo y Meet y Teams sin probar contra Google, Outlook ni un servidor, un IMAP o un SMTP reales (sí con `davprueba` y falsos de Google, Graph, IMAP y SMTP) |
| **Contacts** | Libreta SQLite en claro, `Datos/contacts/libreta.sqlite` (`nucleo/local/libreta`, el plan B de §5.6: índices y las MISMAS reglas del repositorio de Contacts); la primera vez pasa a ella el almacén de registros de antes, con una pantalla de avance como la de Photos (20.000 contactos: ~8 s), y lo deja sin tocar hasta que la persona lo borra en Ajustes → Datos («Borrar los datos de antes de la migración»: solo con la migración comprobada, el almacén entero y nada más); el estado de cada cuenta en `Datos/contacts/agenda/<cuenta>.json` | **20.000 contactos**: abrir en milisegundos y ~25 MiB, buscar < 1 ms, dar de alta o editar ~1,5 ms, un CSV de 2.000 filas en 1,8 s (ver «Prueba de carga»); libretas y contactos, importar vCard y CSV, exportar CSV y vCard (un contacto, una selección o la libreta: `contacts.Repository.BatchVCard`), deshacer un borrado en lote (`BatchRestore`), IA; `?clase=contacto&id=` abre la ficha; «Exportar en formatos abiertos» deja todo en vCard y CSV; **libretas CardDAV, Google Contacts y Outlook** sincronizadas como las de Calendar (las mismas cuentas), agrupadas por cuenta; los **grupos de CardDAV** (`KIND:group`, y `X-ADDRESSBOOKSERVER-KIND` de iCloud) son las etiquetas de su libreta, con su UID, en los dos sentidos (`contacts.Repository.PonerGrupoCrudo`); una **libreta de solo lectura** en su cuenta no se edita aquí (ni crear, editar, etiquetar, combinar, importar ni papelera; destacar sí), con su aviso sobre la lista; una libreta que no quiere vCard 4.0 recibe 3.0; las **etiquetas de Google** son sus grupos de contactos (`contactGroups`, sin los del sistema) y las de **Outlook**, las categorías de los contactos de la carpeta, en los dos sentidos (crear, renombrar, cambiar miembros y borrar; borrar no se lleva a nadie), con los miembros pasados de ids del proveedor a UID de aquí (`agenda.Enlaces`); una etiqueta de aquí que se llama como una de allí aún sin ver se junta con ella al subir; **«Nueva libreta» en la sección de una cuenta** CardDAV (el MKCOL extendido, RFC 5689) o de Microsoft (una carpeta de contactos), con el mismo diálogo (`contactos.GrupoCuenta.CrearLibreta`, opcional) | el almacén de registros de antes no se borra solo: lo eliminado para siempre después de pasar a la base sigue en él hasta que la persona lo borre (Ajustes → Datos); Google no tiene más libretas que una (sus etiquetas son las de esa): allí no se crean; una etiqueta sin nadie no existe en Outlook (las categorías van en cada contacto, y la lista maestra pide un permiso de buzón que no se pide): se queda aquí; sin probar contra iCloud, Google ni Outlook (sí con `davprueba` y falsos de People y Graph) |
| **Photos** | **SQLite con índices** (`nucleo/local/fototeca`, el plan B de §5.6: `Datos/photos/fototeca.sqlite`, WAL, copia en caliente) con las reglas del repositorio de la conectada como funciones puras (`photos/compartido.go`); originales copiados, derivadas y pósters tal cual por su huella en `objetos/`; los **enlazados** se quedan en su carpeta (la fototeca guarda su ruta, huella y miniaturas) | Importar fotos JPEG, PNG, GIF, **WebP, AVIF y BMP** (botón, soltar, «Abrir con», el carrete y las carpetas; BMP como `image/bmp` aunque el sistema lo llame `image/x-ms-bmp`), visor, favoritos, álbumes y su portada, papelera y vaciarla, saltar a un año, recuerdos, repetidas (mismo original), «Por aquí» (2 km), **«Exportar la fototeca»** a `AAAA/MM/`, IA con visión; **HEIC** convertida a JPEG con el sistema (Image I/O, Android 9+, `sips`), `heif-convert` o el ffmpeg del equipo (no hay decodificador Go puro con licencia admitida: libheif/libde265 son LGPL); **«Importar»**: del carrete en el móvil, **fotos y vídeos** (ahora o automático, con el motor de la copia de la conectada y destino local por huella, FO-04), «Importar una carpeta…» y **carpetas vigiladas** (`fsnotify`) en el escritorio, que **enlazan sin copiar** fotos y vídeos (FO-09, `ImportarExterno`; casilla «Copiar a la fototeca» para quien lo prefiera): si el fichero falta o cambia, la tesela y el visor lo dicen sin romper la galería, y lo movido o renombrado dentro de una vigilada se recoloca por su huella; las enlazadas se editan como las demás; **vídeos en la galería** (MP4, MOV, M4V, 3GP, WebM, MKV, AVI: lo de `carrete.TipoDe`) **por todas las puertas** (FO-09): enlazados desde una carpeta y **copiados** desde el carrete (MediaStore `Images` y `Video` con duración, lados y tamaño; PhotoKit con `duration` y lados), «Añadir fotos» (el selector ofrece también vídeos) y soltarlos en la ventana (`photos.ImportarVideo`: original retenido tal cual, sin el tope de una foto, y derivadas del póster; reconocidos por su huella, a la papelera, purgados y exportados como las fotos, y no se editan), con su duración en la tesela y póster (WebM en Go, GStreamer, ffmpeg, MediaCodec o la miniatura del sistema; lo que no se mide lo pone el carrete), que se **reproducen en el visor** con el `Reproductor` de Cast (WebM en Go; lo demás, y el WebM cuyo vídeo no saca Go —VP9 de perfil 1 o 2, AV1—, con GStreamer, Media Foundation o MediaCodec; uno copiado, desde un enlace con su extensión al original de la fototeca, `fototeca.RutaEnClaro`, que se poda al vaciar la papelera) y «Abrir en Cast» (uno enlazado, tal cual; uno copiado, solo en el escritorio, se le deja a Cast en la carpeta de sus vídeos como clon del original de la fototeca donde el sistema de ficheros clona (APFS, Btrfs, XFS: no ocupa más hasta que uno cambie) o como copia —nunca un enlace duro: reescribirlo en su sitio cambiaría el original— con su nombre, y Cast lo recoge como suyo: `copiasParaVer.paraCast`; sin Cast instalado, va al reproductor del sistema); «Abrir con» una imagen (del sistema o «Editar en Photos» desde Drive) la abre **en el editor** (FO-06); **vaciar la papelera libera el disco**: borra en el acto el original copiado y las derivadas de lo purgado si nadie más los cita (con el cerrojo de escritura de SQLite, para no pisarse con el «Traer» del Hub) y barre lo que un corte dejara sin citar; **la fototeca de antes** (motor de registros) **pasa a SQLite al abrir Photos la primera vez**, con su barra de avance: en una transacción, comparando antes de confirmar los recuentos y lo que ve la pantalla en las dos (galería, papelera, huellas, álbumes con su número y su portada), sin copiar ni un fichero (los de `objetos/` son los mismos) y con el diario viejo conservado como `registros-antes-de-sqlite.jsonl`, nunca borrado solo: lo borra la persona en Ajustes → Datos («Borrar los datos de antes de la migración», `fototeca.BorrarAntes`), solo con la migración comprobada (su marca y sus recuentos), con el Hub cerrado y nadie usando el almacén de antes (su cerrojo), y de `objetos/` —que es de las dos— solo lo que ya no cita la fototeca, con su cerrojo de escritura (lo vivo, también lo de después de migrar, se queda); con la fototeca de antes abierta en otra app no migra y lo dice; un corte antes de confirmar la deja como estaba, y uno después solo aparta el diario al abrir (50.000 fotos: 36 s y 2,6 GB de pico, lo que costaba abrirla una vez; después abre en 2 ms con 26 MiB); el mismo juego de pruebas de comportamiento pasa contra las dos (`photos/contrato`, §8.2); textos propios sin «Sytue Drive» en la galería vacía y la papelera | HEIC sin probar con un fichero real (este equipo no genera HEIC) ni en Windows; «Importar del carrete» (fotos y vídeos) sin probar en un dispositivo; el puente de PhotoKit, sin compilar aquí; en Android Sytue Office no tiene selector de ficheros y en iOS el de la fototeca (PHPicker) solo da fotos, así que en el teléfono los vídeos entran por el carrete; sin GStreamer lo que no es WebM no se reproduce aquí (se dice y se ofrece Cast); los vídeos no se editan ni se descargan como imagen; un enlazado fuera de las vigiladas no se recoloca si se mueve; en el teléfono, «Abrir en Cast» no se ofrece con uno copiado; SQLite (cgo, `mattn/go-sqlite3`, como Mail) sin probar en el móvil; migrar 50.000 fotos pide los 2,6 GB de abrir la fototeca de antes (una vez); hasta borrar los datos de antes, lo que solo citaba el diario de antes (páginas de álbumes) se queda en `objetos/` con él, y lo que se borre para siempre después ya no está para ese diario; mover la carpeta de datos y la copia no toman cerrojo de la fototeca (confían, como con Mail, en que Photos esté cerrado) |
| **Bases** | Motor de registros en claro (compactado al cerrar, §5.6); adjuntos como ficheros retenidos del almacén | Bases, tablas, campos y registros con las reglas del servicio (`apps/nucleo/bases/dominio`): validación, fórmulas (también agregados por enlaces), opciones, reordenar campos, vistas con filtro, orden y búsqueda evaluados en el dispositivo, paginación, integridad de enlaces al borrar, importar CSV, exportar CSV y JSON, adjuntos con miniatura, comentarios; borrar un adjunto suelta su fichero y la compactación al cerrar lo quita del disco (`bases.Repository.SoltarAdjuntos`); **actividad de cada base** anotada en el dispositivo (`Datos/bases/actividad.jsonl`, B-06) con las mismas acciones y frases que la traza del servicio | Compartir, formularios públicos (la vista de formulario no se ofrece, B-05) y tiempo real (no se montan) |
| **Cast** | Ficheros en `~/Sytue Office/Vídeos` + índice JSON | Biblioteca y reproductor; **conversor** a MP4, WebM, MKV, MOV, GIF, MP3 y Opus con el **ffmpeg del sistema**; sin ffmpeg, lo que no es WebM se ve con **GStreamer del sistema** si está (`video/sistema`, CA-08) o se abre con el reproductor del sistema y lo dice; **póster sin ffmpeg** (primer fotograma del WebM, fotograma de GStreamer, miniatura del sistema: caché de freedesktop y `.thumbnailer` en Linux, Quick Look en macOS) y «Usar el fotograma de ahora» con lo que enseña el reproductor (CA-06); **Colecciones** en el carril y en «Editar»; textos propios en los 26 idiomas («Importar», «Borrador», sin visualizaciones); un **WebM cuyo vídeo no saca Go** (VP9 de perfil 1 o 2 —el `mandelbrot` de ffmpeg con libvpx—, AV1) se ve y saca su póster con el descodificador del sistema: `video.Nuevo` y `video.Fotograma` prueban el primer fotograma en Go (en 4 s como mucho; medido: 0,03 s un 1080p, 0,12 s un 4K) y, si no sale, cambian (sin sistema, como siempre); recibe de Photos los vídeos copiados en su carpeta (`videoteca.CarpetaEn`) | Miniatura del sistema en Android (`MediaMetadataRetriever`) y Windows (Explorador): hechas, sin probar en un dispositivo; descodificador de Apple (diseño en `video/sistema.go`; Android con MediaCodec y Windows con Media Foundation: hechos, sin probar en un dispositivo); con GStreamer, buscar relanza y descarta (lento en vídeos largos); en móvil solo el reproductor del sistema |
| **Rooms** | Canales de notas en JSON; adjuntos de las notas en `Datos/rooms/notas/adjuntos/`; las cuentas en `chat-cuentas.json`, sin secretos (tokens, y el de refresco de Teams y Google, en el llavero) | Canales locales de notas sin configurar nada; **cuentas Matrix** (Sytue Rooms online y cualquier servidor) solo para salas **sin cifrar**, y **espacios de Slack** (app propia del espacio); en todas: **mensajes directos**, **adjuntos** (medios Matrix sin cifrar, ficheros de Slack, «Guardar en Drive» a la carpeta de Drive), **crear canal en la cuenta elegida**, **explorar** canales/salas públicas y unirse, **«Hilos»** (los que escribiste o sigues), **añadir personas**; con varias cuentas, una categoría por cuenta sin «Sin canales todavía» ni presencia inventada; **cuentas de Microsoft Teams** (RO-05, Graph con OAuth de Microsoft y PKCE en el navegador): chats 1:1, de grupo y de reunión y, si se marcan al entrar y el administrador lo consiente, los **canales de los equipos** con sus hilos (sin consentimiento, el canal lo dice y los chats siguen); enviar, responder en los hilos de canal, **editar, borrar y reaccionar en los chats**, no leídos por la vista de cada chat, **presencia** de Graph en los directos y **sondeo** con espera creciente (la vista previa del último mensaje de cada chat, los cambios de los abiertos por fecha y los canales con `delta`); **cuentas de Google Chat y Meet** (RO-06): con el cliente OAuth de la compilación se escribe (cada espacio dice cómo leerlo); con el **cliente propio del Workspace** (D-22) además se lee, con hilos, quién está y sondeo por fecha; **«Reunión de Microsoft Teams / de Google Meet»** en la cabecera (en vez de «Llamar») crea el enlace (`onlineMeetings`, Meet `spaces.create`) y lo publica en el canal —también en uno de notas o de Slack, con la primera cuenta que crea reuniones— y se entra con la app oficial (D-21); un token caducado se renueva solo, y uno rechazado deja la cuenta en **«Volver a entrar»** (aquí y en Ajustes → Cuentas); una cuenta que no contestaba al abrir trae sus canales cuando vuelve la red; en Teams y Google Chat, **directos nuevos** («Nuevo mensaje» por nombre o correo: el 1:1 que ya hay o uno nuevo, `POST /chats` en Teams; en Google, el que ya hay con el permiso de siempre y uno nuevo con `spaces.setup` si al entrar se marcó «Mensajes directos nuevos»), **adjuntos** (Teams: si al entrar se marcó «Adjuntos», se suben al OneDrive de quien manda, se comparten con los del chat y van enlazados en el mensaje, como hace Teams, y los recibidos se bajan por `shares`; las imágenes pegadas se ven sin ese permiso; Google: `media.upload` y `media.download`), **búsqueda en lo cargado** (sin acentos) y un **chat o espacio nuevo aparece en el carril sin recargar** (el sondeo lo ve y la pantalla vuelve a pedir la lista sin esqueletos); en Google Chat, el **nombre de quien escribe** (y el de los directos) sale de la People API y, si Google no lo enseña, el correo o «Persona de Google» en el idioma de la app; los tipos, las comprobaciones y los clientes de Graph, Chat, Meet y **Slack** están en `apps/nucleo/cuentas/chat`, y **el servicio de integraciones los usa en vez de su transporte propio** (RO-01) | Llamadas (no se compilan); en Slack, los espacios ya conectados deben añadir `files:read`, `files:write` y `users:read.email` a su app; Teams personal (no está en la API de Microsoft) y los canales sin consentimiento del administrador; Teams y Google probados solo contra servidores de mentira; los ficheros de Drive enlazados en Google Chat no se bajan |
| **ERP** | SQLite en claro, servido en proceso (sin sockets); adjuntos como ficheros en `Datos/erp/adjuntos/` | Asistente de alta, catálogo, CRUD de los 64 modelos del catálogo, numeración, presupuesto → pedido → factura con sus líneas y el cuadre al emitir; **Facturae, UBL, imprimir y PDF** de la factura y **leer facturas XML** (UBL, CII, Facturae: asistente e importación con deshacer), con la generación de `apps/nucleo/erp/efactura` que usa también el servidor; botón **«PDF»** en la ficha (`api.ERP.PDFFactura`): se guarda con el selector de guardado del sistema (sin él, en Descargas) y se abre en CertiPDF de la edición o con la app del sistema; **enviar la factura por correo** con el PDF adjunto por una cuenta de Sytue Mail de este equipo (`local/correo.Remitente`, por el portero de red, sin abrir el buzón de Mail): se elige la cuenta, el destinatario es la dirección de facturación del cliente y el asunto y el texto llevan número, importe y vencimiento; queda en el historial como en el servicio; sin cuenta, «Abrir en el correo» abre el redactor de Sytue Mail de este equipo con el PDF ya adjunto: lo deja en la carpeta `para-enviar` de la edición y el enlace solo lleva su identificador, de un solo uso (`nucleo/local/correo.ParaEnviar`); sin Mail o sin esa carpeta (Android), `mailto:` con el PDF guardado a la vista; **libro** (asentar, anular, balance, cierre y reapertura), **banco como tareas** con el extracto Norma 43; **obligaciones** con lo pendiente, el cálculo desde el libro, el proceso de cuatro pasos y el ICS; **TPV** (cobro con su asiento y la sesión de caja, presupuesto); tareas y documentos, exportar CSV, importar CSV con vista previa y deshacer, informes, tableros y Excel; IA con la de Ajustes (frase → informe, frase → plan de registros, pistas del banco, rellenar ficha desde texto o PDF leído con el motor PDF local); **«Comprobar en VIES»** el NIF-IVA de un contacto, en su ficha y en los contactos nuevos del plan de la IA: solo al pulsarlo, por el permiso puntual del portero (propósito `vies`), con lo que contesta el servicio de la Comisión Europea (válido, nombre y dirección registrados) al lado de lo propuesto y sin cambiar ningún campo | **Facturas SIN VERI\*FACTU** (lo dice una franja, D-29); la web y el correo que propone la IA no se comprueban en internet; aprobaciones, peticiones y conectores dicen «aún no está» (E-09); en las cuentas que no guardan solas lo enviado, la copia en «Enviados» espera a que se abra Mail; en Android, «Abrir en el correo» no adjunta el PDF (las apps no comparten carpeta) |
| **Hub** («Sytue Office», solo escritorio) | Lee los almacenes de las demás en solo lectura y sin cerrojos (`nucleo/local/indice`, `almacen.Leer`; el índice SQLite de Mail y la fototeca de Photos con `mode=ro`) | Una tesela por app de la edición instalada junto a él (`sytue-office-<app>` en su carpeta, o su `.app` en macOS), que la lanza sin caer a la web; **«Hoy»** (HU-04): eventos del día de Calendar con las repeticiones expandidas, recordatorios y tareas de Notepads y correo sin leer de Mail; lo reciente de Docus, Calcus, Diapos, Notepads y Drive; búsqueda sin red en el **contenido** de los documentos (texto, celdas, cajas y notas de diapositivas), correo, notas, contactos, eventos, fotos (nombre, descripción, etiquetas, cámara, fecha) y álbumes, bases, tablas y celdas de texto, y ficheros de Drive, que abre cada resultado en su app (`sytueoffice-<producto>:/abrir?…`); **bandeja del sistema** con lo de hoy y las apps, «Abrir al iniciar sesión» (apagado de serie) y copia única (HU-03); en **Linux**, avisa los recordatorios de Calendar y Notepads cuando su app está cerrada (HU-06); **«Traer desde Sytue»** (HU-07, `nucleo/local/traer`): el ZIP de «Tus datos» se reparte en Calendar (.ics), Contacts (.vcf/.csv), Notepads, Docus/Calcus/Diapos, Mail («Importados»), Photos y Drive, tras pedir cerrar esas apps (también Photos); lo demás queda tal cual en `Drive/Traído de Sytue <fecha>/`; **las notas** entran desde el HTML nativo de cada cuaderno, que lleva dentro el volcado íntegro, así que da igual que las tablas se pidieran en JSON, CSV o XML (`model.VolcadoDeHTML` y `notes.TraerCuaderno`, con la lectura y el saneado de `ImportNotebook`): el árbol de cuadernos, las fechas de cada nota, las tareas hechas o canceladas con su fecha, las etiquetas con estados (buscadas o creadas por su nombre), los adjuntos y las notas de imagen; **un cuaderno del diario** entra como el diario de su día, el de Notepads (`notes.TraerCuaderno`: si ya hay uno de ese día, sus notas se suman a él); **las fotos y los vídeos**: sus originales llegan en `drive/files` y se casan con las tablas de Fotos (`photos` y `albums`, en JSON, CSV o XML; el exportador del servicio escribe las de prueba) por nombre y tamaño; se dejan en Drive una sola vez y Photos los **enlaza** desde allí (`ImportarExterno`, como los de una carpeta del equipo: si faltan o cambian, lo dice), igual que desde la propia Photos: las fotos también en **AVIF** y BMP (el Hub lleva `local/imagenes`), los **vídeos** medidos con lo mismo que Photos (`local/videos`: duración, lados y póster) y las **HEIC** como el JPEG del conversor del sistema, copiado a la fototeca (la HEIC se queda en Drive); con su favorita, su descripción, sus etiquetas, su fecha de captura si el fichero no la trae (`OrigenExterno.TomadoEn`), la de **subida** si no tienen de captura, que es donde las ponía Sytue (`OrigenExterno.SubidoEn`, `photos.PonerFechas`), y sus álbumes, **reconocidos por su identificador en Sytue**, que se guarda como origen del álbum (`AlbumRow.Origin`, `CrearAlbumDe`, `AlbumesPorOrigen`): uno renombrado no se duplica, dos con el mismo título son dos y uno borrado no vuelve; lo que Photos no sabe abrir en ese equipo (una HEIC sin conversor, un formato que no lee) se queda en Drive y el resumen lo dice (y, si son HEIC, que se instale ffmpeg o libheif y se traiga otra vez); **traer otra vez la misma copia, o una posterior, no repite nada** (`nucleo/local/traer`, `repetir.go`): **los eventos** vuelven a su mismo calendario de aquí (uno por `.ics`, apuntado en la memoria de lo traído; sin ella, por su nombre interno, que sale de la ruta en la copia, o el `sytue-<uuid>` con el nombre del fichero de antes) y se casan por UID —el objeto entero, con sus excepciones (RECURRENCE-ID)—, **los contactos** a la misma libreta «Sytue» por el UID de la vCard (sin él, por la huella de la tarjeta; del CSV de Google, por el correo o el nombre), **los documentos** por su ruta en la copia (y su origen `sytue:<ruta>`) y **los correos** por su Message-ID (sin él, por la huella del .eml) entre los de «Importados» (`correo.CarpetaImportados`, `correo.MessageIDDe`); de cada uno se guarda la huella con que vino de Sytue y con la que quedó aquí (sin DTSTAMP, PRODID ni REV, que cambian sin que cambie nada): lo que cambió en Sytue y aquí no se ha tocado se pone al día (`PonerObjetoCrudo`, `PonerTarjetaCruda` con lo esperado; un documento, como **versión nueva**, con la de antes en su historial y sin repetir sus comentarios), lo que cambió también aquí no se pisa (un documento entra aparte, «<título> (de Sytue, <fecha de la copia>)», y es el que sigue a Sytue; de un evento o un contacto, el .ics o el .vcf se deja tal cual en la carpeta de lo traído), lo que la persona quitó no vuelve (salvo el calendario o la libreta enteros, que se crean otra vez), y una copia más vieja que la última no deshace nada; las notas, por el identificador que tenían en Sytue, que guarda su fila y conservan las ediciones; las fotos, por la huella de su original —la de una HEIC, por la de su JPEG—: la tirada a la papelera no vuelve y la enlazada cuyo fichero falta se recoloca; lo que se dejó en Drive, por su ruta en el ZIP y su huella, apuntadas en `Drive/.traido-de-sytue.json`: no se copia otra vez mientras siga en Drive, aunque la persona haya movido o renombrado la carpeta, que se busca por tamaño y huella, y si se borró se copia otra vez) y el resumen dice, de cada app, cuántas cosas ya estaban, cuántas se han puesto al día, cuántas habían cambiado también aquí y cuántas se quitaron aquí y no vuelven; cada resultado abre en su sitio: un correo en su conversación de Mail (`clase=hilo`), una tabla o un registro de Bases en ella (`<base>/<tabla>[/<registro>]`, `bases.Pantalla.AbrirTabla`) y un fichero que ya está en Drive en su carpeta, sin volver a subirlo; **«Exportar todo»**: lo de todas las apps en formatos abiertos de una vez, en `<carpeta elegida>/Sytue Office <fecha>/<app>/` (Docus, Calcus y Diapos en OOXML, Notepads en Markdown, Calendar en ICS, Contacts en vCard y CSV, Bases en CSV y los originales de Photos), con esas apps cerradas como en «Traer» (`local.CopiaViva` y la lista de procesos), abriendo cada almacén o biblioteca con el MISMO repositorio que su app y con las mismas funciones de su Ajustes → Copia, que viven en `nucleo/local/exportar`; lo que nunca se usó no se abre ni deja carpeta; Nube y Ajustes de la edición | Los lugares de las fotos (sin geocodificador) y quién está en cada álbum (`private_members`) no se buscan; las notas que apuntan a un fichero de Drive o a un documento de Docs no entran (su cuaderno queda en HTML en la carpeta de lo traído); sin la memoria de lo traído (`Drive/.traido-de-sytue.json`) nada se repite, pero un contacto, o un evento que ya no es igual que en la copia, deja de ponerse al día solo; de lo traído con una versión anterior a esta se reconocen los calendarios y los correos, pero no los contactos (entraron con un UID nuevo) ni los documentos (sin origen), que se repiten una vez; una HEIC entra copiada, no enlazada (no hay decodificador de HEIC en Go); en el móvil no hay Hub |
| **Drive** | Carpetas del disco (`local/archivos`, raíz `~/Sytue Office/Drive`; en iOS, `Drive` en los Documentos de la app; en Android, la de la app o una carpeta del teléfono o de la tarjeta elegida con el selector del sistema, un árbol de documentos `content://` que `archivos.Arbol` presenta como rutas sobre el sistema de ficheros abstracto `archivos.Sistema`) con `.sytue/` (índice, papelera, versiones) | Mi unidad, Recientes, Favoritos y Papelera (restaurar, borrar del todo, vaciar); subir y soltar, carpetas, renombrar, mover y tirar en lote, copiar (también carpetas), versiones, detalles, ZIP y miniaturas en el dispositivo, espacio del disco como cuota; «Nuevo → Documento/Hoja/Presentación» deja un .docx/.xlsx/.pptx y lo abre en su app de Office (`sytueoffice-<producto>:/abrir?local=…`) o, sin ella, con la del sistema; vídeo a Cast; vista previa de texto e imágenes; «Copiar a…» y «Mostrar en la carpeta»; «Resumir» con la IA de Ajustes; Combinar PDF; **etiquetas libres** en el hueco de «Clasificar», en el índice de `.sytue`, que se buscan y van y vuelven con la papelera (DR-09); **unidades de la nube** en el lateral (las cuentas de Ajustes → Nube: Sytue Drive/WebDAV, Google Drive, OneDrive): se navegan, se abren bajadas a una caché y lo guardado sube solo (`fsnotify` y al volver a la ventana) sin pisar lo que otro cambió («nombre (conflicto AAAA-MM-DD)»), y aceptan subir y soltar, crear carpetas, renombrar, mover dentro de la misma unidad, copiar y tirar a la papelera del proveedor (`nube.Gestor`; lo cambiado aquí sube antes de mover o renombrar; el «⋯» y «Nuevo» de una unidad lo ofrecen), con la versión del proveedor (ETag de WebDAV/Graph, `version` de Google) para detectar cambios, por el portero de red (DR-13); **mover entre unidades** (el disco y una nube, o dos nubes) con «Mover a…», cuyo selector sube por encima de la raíz a «Ubicaciones» (Mi unidad y cada nube): copia cada fichero, **comprueba** que llegó entero (el tamaño y la huella que da el destino —MD5 de Google, SHA-1 de OneDrive personal—, o lo relee si no la da; en el disco, su SHA-256) y solo entonces quita el origen a la papelera de su unidad; carpetas enteras (de una vez a la papelera si llegó todo; si no, cada fichero que sí), con una hoja de avance («3 de 12: …») y «Cancelar», que para antes del siguiente fichero (el que va termina: cortar una subida a medias podría dejar una copia que el servidor guardara después); en el destino no se pisa nada; lo que falla se queda en el origen tal cual (la copia a medias se retira) y una hoja dice qué no se movió y por qué; un documento de Google (sin fichero) y lo oculto del disco no viajan; la estrella y las etiquetas van con lo movido (`archivos.Remotas.Trasladar`, `EnEquipo.MoverEntreUnidades`); mover y tirar **en lote** dentro de una unidad van a su proveedor; **favoritos y etiquetas de lo que está en la nube**, guardados en este equipo (`drive-marcas-nube.json` en los ajustes) por cuenta e identificador estable (el ID de Google y OneDrive, el `oc:fileid` de Nextcloud y ownCloud; en un WebDAV sin él, la ruta, que se lleva al renombrar o mover desde aquí), así que renombrar o mover, aquí o en la web del proveedor, no los pierde; salen con los del disco en «Favoritos» (con el nombre de su unidad y abriéndose sin listar antes su carpeta), en las etiquetas que se ofrecen y en la búsqueda por etiqueta; el «destacado» de Google no se usa (sería escribir en la cuenta de la persona y solo lo tiene Google): el menú dice «Añadir a favoritos de este equipo»; **subir o soltar con un nombre que ya está** en una carpeta de la nube pregunta, como Google Drive, «Sustituir» o «Guardar los dos» (el nuevo, «nombre (n).ext», el primero libre) y, con dos o más que se llaman igual, «Sustituir» no se puede pulsar hasta elegir cuál (cada uno con el nombre con que se ve, su tamaño y su fecha) y va encima de ese por su identificador; lo que se sube dentro de una carpeta entera no se pregunta ni se adivina: con uno que se llama igual lo sustituye por su id, con varios no sube (`EnEquipo.Homonimos`, `Remotas.Homonimos`); **dos documentos o carpetas que se llaman igual** en una carpeta de Google Drive (o con el mismo `displayname` en un WebDAV; OneDrive no los admite) se ven todos: el más antiguo con su nombre y los demás «nombre (2).ext», «nombre (3).ext»… (el primero libre), solo en la vista y en la caché, sin renombrar nada en la nube; abrir, copiar fuera y en la nube, renombrar, mover y tirar van al que es por su identificador, y lo guardado sube encima del suyo por su identificador (`nube.Reemplazador`), no por el nombre; si ya no está y su nombre es de otro, sube aparte como conflicto; miniaturas en caché de disco; papelera con textos del equipo; en Android, **«Carpeta de Drive»** en Ajustes (DR-12): elegir una carpeta del teléfono o de la tarjeta con el selector del sistema (`ACTION_OPEN_DOCUMENT_TREE`), con permiso de lectura y escritura guardado; Drive se cambia a ella en el acto (`Archivos.CambiarRaiz`), vuelve a ella al abrir (sin permiso o sin la tarjeta, abre la de la app y lo dice) y allí hace lo mismo que en el disco —subir, carpetas, renombrar, mover, copiar, papelera, versiones, etiquetas, ZIP, miniaturas— por `DocumentsContract` (`ficheros.Arbol` y la clase propia `ArbolDocumentos`, sin tocar el fork de Fyne); un documento de ahí se abre en **Docus, Calcus, Diapos o CertiPDF de Sytue Office** con un `ACTION_VIEW` explícito a su paquete, con el tipo que declara su manifiesto y permiso de leerlo y escribirlo (`ArbolDocumentos.verEn`; sin esa app instalada, u otro fichero, con la app del sistema, que recibe permiso para guardarlo en su sitio), y se comparte con la hoja del sistema; en el teléfono, **«Copiar a…» y «Subir carpeta»** eligen el destino o el origen con el mismo selector (la copia, `archivos.CopiarFueraEn` sobre un `archivos.Arbol`; la subida lee cada fichero al subirlo, sin lo oculto) y sueltan su permiso al acabar, salvo el de la carpeta de Drive; «Usar la carpeta de la app» suelta el permiso | Compartir, equipos, «Documentos» y nubes de la organización no se montan; arrastrar a OTRA unidad no (Drive solo deja soltar sobre una carpeta de la vista, que es de la misma unidad: entre unidades se usa «Mover a…»); mover entre unidades, los favoritos de la nube y los homónimos al subir, sin probar contra Google, OneDrive ni un Nextcloud de verdad; la carpeta elegida en Android, sin probar en un teléfono (ver «Pendiente») |
| **Mail** | `.eml` en `~/Sytue Office/Correo/<cuenta>/` + índice SQLite | Cuentas IMAP/SMTP (Gmail, Outlook por OAuth, iCloud, Yahoo, Sytue Mail, cualquier IMAP) probadas al darlas de alta; sincronización por UID con CONDSTORE si el servidor lo anuncia; **IDLE** en la entrada de cada cuenta (el correo llega en el acto, con aviso del sistema e insignia; lo que ya estaba en la primera vuelta de una carpeta no avisa, y en una que estaba vacía, lo que llega después sí); leer sin conexión, hilos, búsqueda; redactar y enviar con bandeja de salida y deshacer; borradores subidos a «Drafts» (APPEND); carpetas creadas, renombradas y borradas en la cuenta elegida; cuota (QUOTA); **lector con formato** (HTML saneado, imágenes remotas bloqueadas y cargables por mensaje o remitente); **invitaciones** con Sí/Quizá/No (REPLY de iTIP por correo y el evento a Calendar local); **las respuestas de los invitados** (REPLY de iTIP que llega nuevo a la entrada) a lo que organiza la persona —el organizador es la dirección de una cuenta de Mail y quien contesta es quien manda el correo (From, Sender, Reply-To o el SENT-BY del asistente)— van a Calendar sin abrir su almacén: Mail deja el `.ics` tal cual en una cola compartida (`<Datos>/Correo/respuestas-imip/`, `correo.RespuestasPorAplicar`; lo que nadie recoge en un mes se tira), marca el mensaje como procesado en su índice (también si vuelve con otro UID) y el lector no lo enseña como invitación; «Crear evento» en Calendar local; autocompletar y «Elegir de Contactos» con la libreta de este equipo, la MISMA de Calendar (`lector.Libreta` de `nucleo/contacts/lector`, K-04: en solo lectura sobre la base SQLite de Contacts, abierta en modo WAL mientras Contacts escribe: cada letra es una consulta con índice que ve lo último guardado —0,7 ms con 20.000 contactos—, sin copia en memoria ni relectura; con Contacts aún en su almacén de antes, en memoria y releída detrás como hasta ahora); **copia en «Enviados» de lo que envía otra app** (el ERP, por `correo.Remitente`) en las cuentas cuyo servidor no la guarda solo: el `Remitente` la deja en una cola compartida (`<Datos>/Correo/enviados-por-subir/<cuenta>/`) y Mail la recoge al abrirse y en cada vuelta y la sube con APPEND con su índice, sin duplicarla; baja de listas de un clic o por correo; posponer y silenciar en local; etiquetas y reglas locales; IA en el redactor; `mailto:` y `sytueoffice-mail:/redactar?…` por los argumentos o `SetOnOpenURL` (`l.AlAbrirEnlace`), y el de Sytue Office con los ADJUNTOS que deja otra app (`adjunto=<id>`: lo que dejó en `para-enviar`, recogido una sola vez y borrado al copiarlo al borrador; nunca una ruta, ni por `mailto:`), y el empaquetado la ofrece como programa de correo (`esquemas_extra = "mailto"`); las imágenes remotas y la baja de un clic salen por el permiso puntual del portero (`red.Portero.Puntual`); «Buscar los servidores» de un dominio (autoconfig y SRV); sus cuentas también desde Ajustes → Cuentas de cualquier app; el índice SQLite entra en la copia en caliente; **respuesta automática por cuenta** (M-10) donde el proveedor la ofrece, en su servidor ManageSieve (RFC 5804, cliente propio en Go puro `nucleo/cuentas/sieve`: STARTTLS obligatorio y sin cifrar solo con localhost, SASL PLAIN con la contraseña de la cuenta u OAUTHBEARER/XOAUTH2 con su token si se anuncian, literales `{n+}`/`{n}`, topes a lo que llega del servidor): se pregunta SOLO al abrir los Ajustes de Mail, con un plazo corto de 8 s, a las cuentas de un IMAP cualquiera (el host de IMAP en el 4190, o el que se escriba en «Servidores (avanzado)» → «Respuesta automática (Sieve)»), y ese servidor entra en el portero como uno más de la cuenta (`local/cuentas`); Gmail, Outlook, iCloud, Yahoo y Sytue Mail no tienen ManageSieve y la fila «Vacaciones» no aparece (`api.MailRespuestas`, interfaz opcional: la conectada sigue igual). El diálogo de siempre con la cuenta, el asunto, el mensaje, cada cuántos días a la misma persona y desde/hasta si el servidor tiene `date` y `relational` (si no, sin fechas, y lo dice); el estado se lee siempre del servidor. **No toca las reglas de la persona**: escribe un script propio, «sytue-office», validado con CHECKSCRIPT; si hay otro activo, el propio lo incluye DETRÁS de la respuesta (`include :personal :optional`, RFC 6609) y lo apunta en un comentario, y al apagarla vuelve a quedar activo exactamente lo de antes; si el servidor no sabe incluir, o ese script (o lo que incluye) ya responde, rechaza correo o incluye scripts globales —Pigeonhole falla entero con dos `vacation`, comprobado con `sieve-test`—, no se toca nada y el diálogo lo explica. Probado con un ManageSieve de mentira en proceso (`cuentas/sieve/sieveprueba`), contra Dovecot 2.3 con Pigeonhole en este equipo (`go test -tags integration`; `sieve-test` ejecuta la respuesta y luego las reglas incluidas) y bajo Xvfb | Sin cifrar solo con localhost; QRESYNC sin VANISHED (el cliente IMAP no lo lee: se usa CONDSTORE); en iOS, ser el programa de correo pide el derecho `com.apple.developer.mail-client` de Apple (el esquema ya va en el Info.plist); alias, buzones compartidos y S/MIME; la respuesta automática de Gmail y Outlook (no tienen ManageSieve; sus API propias no se usan) y sin probar contra un proveedor real con ManageSieve; Outlook con OAuth sin probar contra el servicio real |


## Dónde está cada cosa

| Qué | Dónde (escritorio) | En claro |
| --- | --- | --- |
| Documentos de Docus, Calcus y Diapos | `~/Sytue Office/Documentos/<producto>/<id>.json` (+ `<id>.versiones/`) | sí, JSON legible |
| Datos de las demás apps | `~/Sytue Office/Datos/<producto>/` (`registros.jsonl` + `objetos/`) | sí |
| Contactos | `~/Sytue Office/Datos/contacts/libreta.sqlite` (SQLite en modo WAL, solo de la persona: 0600); el almacén de antes (`registros.jsonl`), si lo había, se queda al lado sin tocar | sí, SQLite |
| Ficheros bajados de la nube | `~/Sytue Office/Nube/<cuenta>/` | sí |
| Perfil, apariencia, IA, cuentas (sin secretos) | `<config>/sytue-office/*.json` (`cuentas.json` junta las de todas las apps; `red.json`, el interruptor «Sin conexión»; `avisos-<app>.json`, lo programado en el sistema) | sí |
| Copias de seguridad | `~/Sytue Office/Copias/sytue-office-<fecha>.zip`: `datos/`, `ajustes/` (sin `secretos/`) y `manifiesto.json` | sí, ZIP |
| Claves de API, contraseñas y tokens | llavero del sistema, servicio `eu.sytue.office`; sin llavero, `<config>/sytue-office/secretos/` con permisos 0600 (Ajustes → Datos lo dice) | no se escriben en claro en ningún JSON |

La carpeta de datos se cambia en **Ajustes → Datos**, o con `SYTUE_OFFICE_DATOS`. Desde
**Ajustes → Nube** se puede poner dentro de una carpeta que ya sincroniza el sistema
(OneDrive, Google Drive para escritorio, iCloud Drive, Sytue Drive): así todo queda en la
nube sin que la app hable con ningún servidor. En iOS y Android va dentro del contenedor
de cada app.

**Cambiarla se lleva lo que había** (`nucleo/local/traslado`): con las demás apps de la
edición cerradas (`local.Abiertas`: su enchufe de copia única o su proceso; donde el sistema
no deja verlo, se pide) y los almacenes tomados con su cerrojo EXCLUSIVO (`almacen.CerrojoExclusivo`:
ninguna los abre mientras dura), copia TODO a la nueva —con fechas y permisos, los enlaces como
enlaces—, vuelve a leer cada copia y compara tamaño y SHA-256 con lo leído (y que el original no
cambió), pone al día lo que guarda rutas absolutas (el índice y las versiones de Drive, los
vídeos copiados de Cast, los documentos anclados a un fichero de Drive) y solo entonces la apunta
en el perfil. Si la nueva no está vacía, no mezcla: si se elige una carpeta con cosas, los datos
van a una `Sytue Office` dentro, y si esa también las tiene, se dice. Con progreso (contar,
copiar, comprobar) y cancelable; si falla o se cancela a medias, la nueva queda como estaba (una
marca `.sytue-office-moviendo` deja retomar lo cortado por un apagón) y nada cambia. La vieja no se
borra sola: se ofrece «Borrar la carpeta de antes», que quita exactamente lo copiado. Como al
restaurar una copia, la app cierra lo suyo antes y se cierra al terminar. **Ajustes → Datos**
dice también si el disco de la carpeta está **cifrado** (X-10, `nucleo/sistema/discocifrado`, sin
privilegios ni red): Linux por `/proc/self/mountinfo` y `/sys` (dm-crypt/LUKS, también bajo LVM;
fscrypt por `statx`; eCryptfs; ZFS; gocryptfs y otros FUSE que cifran; si no hay `/sys`, `lsblk -s`),
macOS por `fdesetup isactive` (disco de arranque) o `diskutil info -plist` (otro volumen), Windows
por la propiedad `System.Volume.BitLockerProtection` del Explorador (la que se lee sin
administrador; `manage-bde` y WMI lo piden), y en Android e iOS, cifrado siempre. Sin cifrar o no se
sabe: «tus datos van en claro y quien acceda al equipo los lee».

## Red, cuentas, avisos y copia

- **Red** (`nucleo/local/red`, §5.8): el portero es la única puerta. `local.Abrir` pone
  `http.DefaultTransport`, `http.DefaultClient` y `api.Transporte` a un transporte que lo
  deniega todo; la IA (`l.IA.HTTP`), la nube (`l.Nube.HTTP`), el correo
  (`correo.Red{Marcar: l.Red.Marcador(red.Correo), HTTP: l.Red.Cliente(red.OAuth)}`) y la
  mensajería (`reg.Cliente = l.Red.Cliente(red.Chat)`) salen por él. La lista blanca sale de
  la configuración (proveedor de IA, cuentas de nube, de correo y de chat), solo TLS salvo
  `localhost` o lo marcado a mano, y las altas abren su servidor solo mientras duran
  (`l.Red.Permitir`). Lo que la persona pide en el momento y no sale de la configuración (ver
  las imágenes de un correo, darse de baja de una lista, buscar los servidores de un dominio)
  va por `l.Red.Puntual(propósito, motivo)`: solo https, solo direcciones públicas, sin proxy,
  y cada petición abierta como temporal mientras dura. **Ajustes → Red** enseña la lista y
  por qué, con «Sin conexión».
- **Cuentas** (`nucleo/local/cuentas`, §5.10, D-17): el modelo común junta, sin tocarlas, las
  de Mail (`<Datos>/Correo/cuentas.json`), Rooms (`chat-cuentas.json`) y la nube
  (`nube.json`), y lo escribe en `cuentas.json`. **Ajustes → Cuentas** las enseña con su
  estado y deja añadir, volver a entrar y quitar: las de nube en el acto; las de correo y
  mensajería con su app (`l.GestionarCuentas` en Mail y Rooms; desde otra app, su enlace
  `sytueoffice-<app>:/cuentas?accion=anadir|reconectar|quitar&id=…`, que pregunta antes de
  quitar).
- **Ajustes por app**: `l.AjustesDeLaApp(título, crear)` añade una pestaña propia (CertiPDF:
  autoridad de sellado y anclas; Cast: el ffmpeg).
- **Avisos** (`nucleo/local/avisos`, §5.12): Calendar (los de los próximos 30 días, también de
  cada repetición) y Notepads (el próximo de cada nota) programan sus recordatorios con
  `l.Recordar`. macOS e iOS
  (`UNUserNotificationCenter`) y Android (`AlarmManager`) por el `ScheduleNotification` del
  fork de Fyne; Windows por `escritorio.ProgramarToast` (`ScheduledToastNotification` con el
  texto escapado: el de Fyne no lo escapa); Linux, un reloj del proceso que avisa por
  `escritorio.Avisar` mientras la app está abierta y, cerrada, el Hub residente (HU-06). Pulsarlo abre el evento
  (`?clase=evento&id=…`).
- **Copia de seguridad** (`nucleo/local/copia`, §5.13): **Ajustes → Copia de seguridad** crea el
  ZIP, lo lista y restaura (antes guarda una copia de lo que hay, comprueba rutas y
  manifiesto, y cierra la app). Las bases SQLite que la app tiene abiertas se copian en
  caliente si se apuntan con `l.Copia.BaseAbierta` (Mail y ERP, con `VACUUM INTO`).
  **Exportar en formatos abiertos** (`l.AlExportarTodo`, en Ajustes → Copia): Docus, Calcus y
  Diapos a OOXML, Notepads a Markdown, Calendar a ICS, Contacts a vCard y CSV, Bases a CSV por
  tabla y Photos los originales en `AAAA/MM`; cada app lo suyo (Mail ya guarda `.eml` y Cast
  los vídeos tal cual en la carpeta de datos).
- **Enlaces entre apps** (§5.11): `local.AbrirEn(a, producto, ruta)` abre un fichero en su app
  por el esquema `sytueoffice-<producto>` (si no hay quien lo atienda, con la del sistema;
  nunca la web) y `l.AlAbrirElemento(func(clase, id))` recibe
  `sytueoffice-<producto>:/abrir?clase=…&id=…`: Docus, Calcus y Diapos (documento), Calendar
  (evento), Notepads (nota), Contacts (contacto), Photos (foto, álbum), Bases (base,
  registro `<base>/<tabla>/<registro>`), Mail (hilo), Drive (carpeta, fichero por su ruta),
  CertiPDF (fichero), Cast (vídeo) y ERP (vista). Cualquier otro enlace (`mailto:`,
  `sytueoffice-mail:/redactar?…`, `sytueoffice-calendar:/events/new?action=create…`) llega a
  `l.AlAbrirEnlace(func(*url.URL))`, por `SetOnOpenURL` o por los argumentos; los que llegan
  antes de que la app arranque esperan a que haya ventana. Un segundo esquema por app se
  declara con `esquemas_extra` en su `FyneApp.toml` y `empaquetar.sh` lo registra como
  candidato en los cinco sistemas (en Windows por `Capabilities\URLAssociations`, sin quitarle
  `mailto` al programa de correo de la persona). **Copia única** (escritorio): abrir una app que
  ya está abierta le pasa a esa copia los enlaces de los argumentos y la trae al frente, por un
  enchufe local de la persona (`local/instancia.go`), en vez de poner un segundo proceso sobre el
  mismo almacén; el Hub sabe así qué apps están abiertas (`local.CopiaViva`).

## Nube

| Proveedor | Cómo se conecta | Qué hace falta |
| --- | --- | --- |
| Carpetas sincronizadas | Se detectan solas (`nube.Sincronizadas`) | El cliente del proveedor instalado |
| **Sytue Drive** | WebDAV en `https://drive.sytue.eu/dav` con una **contraseña de aplicación** | Crearla en Sytue ID → Seguridad. La app no inicia sesión con Sytue ID |
| WebDAV (Nextcloud, ownCloud…) | WebDAV con usuario y contraseña de aplicación, solo `https` | — |
| **Google Drive** | OAuth de cliente público con PKCE y vuelta a `127.0.0.1`; en iOS y Android, por el esquema de la app (abajo) | Un identificador de cliente «App de escritorio»: el de la compilación o uno propio que se pega en Ajustes |
| **OneDrive / Microsoft 365** | Igual, con Microsoft Graph (`Files.ReadWrite`, `offline_access`) | Un identificador de cliente de Microsoft Entra («aplicación cliente pública», redirección `http://localhost` y, para el móvil, `sytueoffice-<producto>:/oauth`) |

**OAuth en el móvil** (§5.10): en iOS y Android el navegador vuelve por el esquema de la app,
`redirect_uri` = `sytueoffice-<producto>:/oauth` (`nube.Vuelta`, puesta por la cáscara con
`nube.UsarVuelta`; vale para todas las altas, porque nube, Calendar, Contacts, Mail y Rooms van por
`nube.OAuth.Conectar`). El enlace llega por `SetOnOpenURL` y la cáscara se lo da a la vuelta antes
que a `l.AlAbrirEnlace`: solo se acepta el de un alta en marcha, por su `state`, una sola vez y antes
de que caduque (10 minutos); el código se canjea con el verificador PKCE. En el escritorio sigue
`127.0.0.1`. El dueño tiene que registrar ese `redirect_uri` en el cliente: Microsoft Entra lo
admite en «Aplicaciones móviles y de escritorio»; Google, en sus clientes de iOS y Android, solo
admite el esquema del identificador del paquete (`eu.sytue.office.<app>:/…`), así que con Google
hace falta un cliente por app y declarar ese esquema en `empaquetar.sh` (no está). Probado con un
servidor OAuth de mentira y el enlace de vuelta simulado; **sin probar en un teléfono**.

Los identificadores de cliente de la compilación (`nube.ClienteGoogle`, `ClienteGoogleSecreto`
y `ClienteMicrosoft`, que usan también Calendar, Mail y Rooms) los pone `empaquetar.sh` con
`-ldflags -X` a partir de `SYTUE_OFFICE_GOOGLE_CLIENT_ID`, `SYTUE_OFFICE_GOOGLE_CLIENT_SECRET` y
`SYTUE_OFFICE_MICROSOFT_CLIENT_ID`, que en el CI son secretos de Actions; sin ellos se compila sin
ellos y Ajustes pide los de la persona. En el `.ipa` de iOS no entran (la CLI de Fyne pisa el
`-ldflags`). Registrar la app en Google y en Microsoft es del dueño (PLAN §11). El permiso de Drive
completo de Google es «restringido»: para distribuirlo, Google exige verificar la app.

En cada app, **Nube → Abrir de la nube…** baja el fichero y lo abre como uno del equipo;
**Guardar en la nube…** sube el documento abierto (Docus en `.docx`, Calcus en `.xlsx`,
Diapos en `.pptx`).

## IA

Apagada por defecto. Sin IA, los botones de IA no aparecen. Encendida, solo se envía el
texto sobre el que se pide ayuda y solo al proveedor elegido (Ajustes lo dice). El prompt
es el mismo que el del servicio de IA de la suite —el contexto va como datos delimitados,
nunca como órdenes—, porque el paquete se ha movido de `services/ai/internal/provider` a
`apps/nucleo/ia/proveedor` y lo usan los dos.

| Opción | Detalle |
| --- | --- |
| Modelo en este equipo | Cualquier servidor compatible con OpenAI en `localhost` (Ollama: `http://localhost:11434/v1`). Sin Internet y sin clave |
| Anthropic | API de Messages, con la clave de la persona |
| OpenAI | API de Responses, con la clave de la persona |
| Google Gemini | Extremo compatible con OpenAI de Gemini, con la clave de la persona |
| Otro compatible con OpenAI | Mistral, Scaleway, OpenRouter… (solo `https`) |
| **IA del sistema — macOS** | Apple Intelligence a través de **Atajos**: crea un atajo llamado **«Sytue Office IA»** que reciba texto, lo pase a la acción **«Usar modelo»** (modelo del dispositivo o Private Cloud Compute) y devuelva el resultado. La app lo ejecuta con la orden `shortcuts`. Apple no ofrece su modelo a apps que no sean Swift de otra forma |
| **IA del sistema — Windows** | **Foundry Local** (`winget install Microsoft.FoundryLocal`): el runtime de modelos en el dispositivo de Microsoft, que usa la NPU de los PC Copilot+. La app encuentra su servidor con `foundry service status`. La app de Copilot no tiene API para terceros |
| **IA del sistema — iOS y iPadOS 26** | **Foundation Models** (Apple Intelligence en el dispositivo) por `SytueIA.framework` (`xcode/ia-sistema`, Swift con `@_cdecl`), que la app abre con `dlopen`; Ajustes dice, en el idioma de la app (el núcleo da el estado, `asistente.EstadoDelSistema`), si el dispositivo no es compatible, si Apple Intelligence está apagada o si el modelo aún se prepara |
| **IA del sistema — Android** | **Gemini Nano** por la Prompt API de ML Kit GenAI (AICore), con el puente `eu.sytue.nativo.IASistema` por reflexión; si AICore puede bajar el modelo, se le pide (`download` con su aviso). Con `SYTUE_MLKIT=si` la APK de Sytue Office **lleva ML Kit** (`com.google.mlkit:genai-prompt` 1.0.0-beta5 y su árbol, 64 bibliotecas fijadas con su sha256 en `android/mlkit/dependencias.tsv`, dexadas detrás del dex propio, sin Gradle: `scripts/android-mlkit.py`), con el permiso y la consulta de AICore y el arranque de ML Kit, pero **sin el canal de métricas de Google** (el transporte `datatransport` no tiene declarado a quién enviar). Sin probar en un teléfono con AICore. **Por defecto no la lleva** (siete de esas bibliotecas son propietarias de Google y la Prompt API está en beta: lo decide el dueño), y Ajustes dice que esta compilación no trae el puente |
| IA del sistema — Linux | No hay: usa un modelo en el equipo |

Los nombres de modelo de cada proveedor son solo el punto de partida; se cambian en Ajustes.

## Compilar, probar y empaquetar

```bash
make fyne-local-check   # las 15 apps con goolm,edicionlocal, vet y tests -race de local/, cmd-local/
                        # (guardianes y arranque de cada app) y ui/; sin la etiqueta, que local no
                        # arranca; el guardián de textos y, del núcleo, local/, offline/ e ia/
cd apps/fyne && golangci-lint run --build-tags goolm,edicionlocal --config ../../.golangci.yml ./local/... ./cmd-local/...
# Prueba de carga del almacén (§5.6, R-MEDIR; no corre sin SYTUE_CARGA): ver «Pendiente»
cd ../nucleo && SYTUE_CARGA=1 go test -tags goolm -run Carga -v -timeout 0 ./notes/ ./photos/ ./contacts/ ./local/biblioteca/ ./local/fototeca/
```

Lo mismo corre el pre-push cuando el empujón toca `apps/fyne` o `apps/nucleo` (además de su
pasada con `goolm`, y con el lint con la etiqueta), un paso por módulo de `ci-desktop.yml` y
`ci-mobile.yml`, y `go-licenses` sobre `cmd-local` y `nucleo/local` en `security.yml`
([operaciones](../../docs/operations.md#sytue-office)).

Los iconos (`cmd-local/<app>/Icon.png`) son los de la conectada con la marca de la edición: una
insignia abajo a la derecha del glifo con los colores del símbolo de la casa (disco negro
`#0a0a0a`, aro blanco) y el glifo `device` del catálogo (una pantalla: la app vive en este equipo),
dentro del círculo seguro de la maskable. Se regeneran desde la raíz, después de los de la conectada
(lo hace `scripts/sync-branding.sh`):

```bash
scripts/gen-iconos-fyne.sh            # los de apps/fyne/cmd (ImageMagick 7, de las marcas SVG)
scripts/gen-iconos-fyne.sh office     # los de apps/fyne/cmd-local, desde esos (vale el `convert` del 6)
```

Empaquetar (el mismo `empaquetar.sh`, con el árbol de la edición):

```bash
cd apps/fyne/scripts
SYTUE_ARBOL=cmd-local ./empaquetar.sh linux        # bundle «office»: sytue-office-linux-amd64.tar.xz
SYTUE_ARBOL=cmd-local SYTUE_LINUX_ARCH=arm64 ./empaquetar.sh linux   # …-linux-arm64.tar.xz, en ARM64
SYTUE_ARBOL=cmd-local ./empaquetar.sh paquetes     # qué apps lleva
SYTUE_OFFICE_GOOGLE_CLIENT_ID=… SYTUE_OFFICE_MICROSOFT_CLIENT_ID=… SYTUE_ARBOL=cmd-local ./empaquetar.sh linux
```

La etiqueta `edicionlocal` deja fuera de los binarios los medios de las llamadas (LiveKit y
pion): `apps/fyne/llamada` queda en su API vacía. Con `SYTUE_ARBOL=cmd-local` el script la
pone solo, los ejecutables se llaman `sytue-office-<app>` y el bundle de
escritorio es `office`. Sin la variable, el script hace exactamente lo de siempre. Desde un
`vendor/` (con `GOFLAGS=-mod=vendor`) no pide `GOPRIVATE` y toma de ahí el Java del fork de Fyne
para el dex de Android.

Publicar: Linux y Windows con `release-office.yml`; Android con `release-mobile.yml` y
`edicion=local` (los AAB firmados con la clave de subida, a la release del tag; a Google Play
todavía no). El DMG de macOS y las tiendas de Apple, aún no (ver Pendiente).

Se publica también en abierto, dentro de `github.com/thinkddr/sytue-suite` (Apache-2.0): llega
con la sincronización de la suite, `./sytue-deploy --mode office` lo compila e instala en el
equipo y cada release de la suite adjunta sus paquetes. Ver
[operaciones](../../docs/operations.md#sytue-office-publico).

## Guardianes

- `apps/fyne/local/guardian_test.go`: ningún binario de `cmd-local` enlaza el cifrado de
  la suite (`apps/fyne/cifrado`), `sesion`, el cliente de Matrix, las llamadas, LiveKit ni
  libolm (Rooms, excepción documentada: su pantalla arrastra `cifrado` y este `sesion`);
  tampoco `nucleo/auth` (login OIDC, almacenes de sesión, llavero de la suite), en ninguno
  de los sistemas de la edición (Linux, Windows, macOS con y sin `tienda`, iOS, Android),
  salvo Rooms por lo mismo (`TestAuthNoEntraEnLasApps`, que dice quién lo importa);
  y ningún `main` local importa `sesion`, `auth` ni `cifrado`.
- Textos propios (C-12): cada paquete con un `i18n.Propios` prueba que `Faltan()` está vacío
  (las mismas claves y huecos en los 26 idiomas; `local/textos_test.go` para las tres tablas
  de la cáscara); y `apps/fyne/catalogos/propios_test.go` recorre por AST `cmd-local`,
  `local`, `editor/enfichero`, los `edicionlocal*.go`, `apps/fyne/apps` y
  `apps/nucleo/local`: ninguna tabla `map[string][2]string` de textos fuera de un `Propios`
  con su mapa de traducciones en el mismo paquete, ni una elección de idioma a mano
  (`== "es"`, `strings.HasPrefix(idioma, "es")`, `case "en"`, un mapa con `"es"` y `"en"`);
  las tablas que son datos van, con su motivo, en `datosQueNoSonTextos`, comparada por
  igualdad. Las traducciones usan el trato de los catálogos de la web de cada idioma: de
  usted en bg, cs, de, el, et, fr, hr, hu, lt, lv, pt, ro, sk y sl; de tú en los demás.
- `apps/nucleo/local/red`, `cuentas`, `copia`, `avisos`: lista blanca vacía sin configurar,
  solo TLS, redirecciones, «Sin conexión»; copia sin secretos y restauración que rechaza
  rutas fuera de su carpeta; avisos dentro de la ventana de 30 días.
- `apps/nucleo/local/almacen/almacen_test.go`: el repositorio de notas de la edición
  conectada, sin tocarlo, corre sobre el almacén en claro y sobrevive a reabrir y a un
  corte a mitad de escritura (también con lo que se escribe después pegado a la línea
  cortada: `leerLinea`), y dos instancias sobre el mismo almacén se ven y la segunda escribe
  sobre lo de la primera (`TestDosInstanciasVenLoQueEscribeLaOtra`, `Puerto.seguir`).
- `apps/nucleo/local/biblioteca`, `asistente`, `nube`, `secretos`: pruebas de cada pieza,
  sin red (servidores `httptest`).

Los de §8.1 del plan (cada uno visto en rojo invirtiendo a mano lo que vigila; el «VISTO EN
ROJO» de su comentario dice cómo):

- **`TestRecienInstaladaNoAbreNingunaConexion`** (R-RED, R-CONVIVE), en
  `cmd-local/<app>/arranque_test.go` con la etiqueta de la edición: monta el `main` DE VERDAD
  de cada app en el driver de pruebas de Fyne (`local/prueba.Arrancar`; los `main` piden la
  app a `local.NuevaApp`) con una carpeta personal vacía y falla si se intenta una sola
  conexión —`red.VigilarDePrueba` ve cada intento por el portero o por `red.Denegar`, se
  permita o no—, si `http.DefaultTransport`/`api.Transporte` no son `red.Denegar`, si la
  ventana no pinta su pantalla inicial, si escribe en `<config>/sytue` o `<caché>/sytue` (las
  de la conectada) o si guarda algún secreto. Con `SYTUE_ARRANQUE_CAPTURAS=<carpeta>` deja la
  captura de cada una. Las quince apps; `appsSinPruebaDeArranque` (por igualdad) está vacía.
- **`TestTodoClienteDeRedPasaPorElPortero`** (`nucleo/local/red/todos_test.go`, estático con
  `go/ast`): en `nucleo/local/...` (con `cuentas`), `fyne/local/...` y `cmd-local/...`, fuera
  del portero, ni `http.Client{}` sin `Transport`, ni `http.Transport` propio, ni
  `http.DefaultClient`/`DefaultTransport`, `http.Get`/`Post`…, `net.Dial*`, `net.Dialer`,
  `tls.Dial*`, el resolvedor de nombres ni el `Dial` de una biblioteca; las excepciones, con
  su motivo y por igualdad (el enchufe Unix de la copia única, la cáscara poniendo
  `red.Denegar`, Foundry Local en 127.0.0.1, el Socket Mode de Slack con el cliente del
  portero). Destapó que los
  clientes de respaldo de `nube`, `asistente`, `chat` y `correo` salían a la red sin portero
  si nadie les inyectaba uno (y `correo` marcaba TCP directo): ahora fallan cerrados
  (`red.Denegar`, `red.ErrDenegado`) y las pruebas inyectan el suyo. Las consultas SRV y TXT
  del alta de correo y de CalDAV/CardDAV, que eran excepciones (al resolvedor del sistema),
  van ya por el portero (`red.Portero.Resolvedor`, propósito `descubrimiento`: con «Sin
  conexión» no se pregunta nada, la prueba de arranque las ve y Ajustes → Red las enseña
  como temporales mientras duran; `TestElDNSDelAltaPasaPorElPortero`, visto en rojo), y sin
  él no se miran los SRV.
  `TestLasPuertasDePruebaSoloEnPruebas`: `red.VigilarDePrueba` y `local.AppDePrueba` solo los
  nombran su definición y `local/prueba`, que ningún binario enlaza (`guardian_test.go`).
- **`TestCadaAppLocalDeclaraSuIdentidad`** y **`TestLaIdentidadLocalNoChocaConLaConectada`**
  (`local/identidad_test.go`): las quince apps con su ID `eu.sytue.office.<app>`, su esquema
  `sytueoffice-<producto>` (el mismo producto que su `local.Abrir`), su instalador `office`,
  su nombre «Sytue … Office», `AlAbrirFichero` si declaran extensiones y el icono de su
  marca; e identificadores, esquemas, ejecutables (`empaquetar.sh`), nombres e instaladores
  disjuntos de los de `cmd/*`, sin un esquema `sytue-…`. `TestCadaAppLocalSePruebaAlArrancar`
  exige el `arranque_test.go` de cada una.
- **`TestSinEtiquetaNoArranca`** (D-02): `local.Abrir` devuelve `ErrSinEtiqueta` si el binario
  no lleva `edicionlocal` (`escritorio.EdicionLocal`); corre en las dos compilaciones.
- **`TestElEspacioLocalNoEsElDeLaSuite`**: ajustes en `<config>/sytue-office`, caché en
  `<caché>/sytue-office`, datos en `~/Sytue Office` y llavero `eu.sytue.office`, distintos de
  los de la conectada (`<config>/sytue`, `eu.sytue.one`). El diseño cambió respecto a §5.3: los
  paquetes compartidos no leen la etiqueta; la edición tiene sus rutas y la prueba de arranque
  comprueba que no escribe en las otras.
- **`TestLosSecretosNoEstanEnLosAjustes`** (`local/correo`): tras dar de alta por el código de
  verdad una cuenta de correo (prueba IMAP y SMTP en proceso), una WebDAV (que se lista con la
  contraseña guardada) y una clave de IA, ningún fichero de la carpeta personal salvo
  `<ajustes>/secretos` contiene ninguno (ni en base64).
- **`TestLosDatosEstanEnClaro`**: una nota (por `l.Almacen`) y un documento de Docus (por
  `l.Biblioteca`) se leen sin clave en la carpeta de datos. El documento va tal cual; la nota,
  en base64 dentro del JSON del diario (no es cifrado, pero tampoco legible a simple vista).
- **`TestLaBarraDeLaEdicionUsaLosBotonesDeUI`** (píxel, §8.3): la barra de `Montar` lleva el
  fantasma atenuado de `ui` (`local.BotonDeBarra`: `text-muted`, `bg-subtle` con el puntero,
  32 px) y no botones de Fyne; la del Hub, también. Y su menú «Nube» es el `Menu` de `ui`
  (Dropdown.tsx) colgado del botón y alineado con su borde derecho, con sus `ElementoMenu`
  («Abrir de la nube…», «Guardar en la nube…» solo si la app exporta, y «Ajustes…»; `text` y
  `text-muted`, `menu-surface` y `menu-hover` con el puntero), y no un diálogo con botones de
  Fyne.
- **`TestElIconoDeLaEdicionLlevaSuMarca`** (§5.2, R-CONVIVE; `local/icono_edicion_test.go`): el
  `Icon.png` de cada app de `cmd-local` (del que `fyne package` saca el `.icns`, el `.ico`, el PNG
  del `.desktop`, el mipmap de Android y el catálogo de iOS) existe a 1024 × 1024 y opaco, NO es el
  de `cmd` y lo que cambia es la marca de la edición: fuera del círculo seguro de la maskable (r =
  40 %) es el de la conectada píxel a píxel, la insignia es la misma en las quince (con 3:1 o más
  entre su aro y la tesela) y, reducido a 16, 32 y 64 px, sigue siendo un punto oscuro que el de la
  conectada no tiene. Lee las medidas de `scripts/gen-iconos-fyne.sh`.
- **Contrato (§8.2)**: Notepads, Photos, Contacts, Calendar y Bases corren cada uno el MISMO
  juego de comportamiento sobre dos puertos. `nucleo/notes/contrato_test.go`, el de Notepads
  sobre el puerto en memoria de la conectada y sobre el almacén en claro;
  `nucleo/photos/contrato` (`TestContratoDeLaFototeca`), el de Photos sobre el repositorio de la conectada en el almacén
  en claro y sobre la fototeca de SQLite (importar y reabrir, la línea de tiempo y su cursor,
  buscar, años, recuerdos, repetidas, «Por aquí», álbumes, editar, enlazados, vídeos, borrar
  para siempre, lo traído de Sytue y exportar); `nucleo/local/libreta/contrato_test.go`, el de
  Contacts sobre el almacén y su libreta SQLite; y `nucleo/calendar/contrato_test.go` y
  `nucleo/bases/contrato_test.go` (`TestContratoDeCalendar*`, `TestContratoDeBases*`), los de
  Calendar y Bases por la fachada de las pantallas sobre el puerto en memoria del motor
  (`storage/storagetest.Memoria`: el publicador de la conectada, con el historial de lo más
  nuevo a lo más viejo y por páginas, y sin el sello ni la caché de lo comprobado que solo da el
  almacén) y sobre el almacén en disco como lo abren `cmd-local/calendar` (con sus tres
  opciones) y `cmd-local/bases` (con la actividad local y SoltarAdjuntos). Calendar:
  calendarios, eventos, avisos, buscar (con filtro y por páginas), exportar, series con
  excepciones y EXDATE, lo cambiado en la serie que llega a sus excepciones, «solo este» y
  «este y los siguientes», respuestas (con y sin ConservarRespuestas), suscripciones y la
  papelera. Bases: bases, tablas, campos, vistas, registros y su celda con conflicto, fórmulas y
  agregados por enlace, la integridad de los enlaces al borrar, adjuntos retenidos (y su
  fichero soltado), comentarios con lápida, actividad, CSV. Los dos: reabrir, dos procesos sobre
  el mismo almacén, un corte a mitad de escritura (la respuesta perdida, en los dos; la línea a
  medias, en disco) y uno que compara lo que enseña la pantalla en los dos puertos. Destaparon
  dos fallos del almacén en disco, arreglados (ver el guardián del almacén, arriba): dos
  procesos no se veían hasta reabrir y el segundo que cambiaba lo mismo dejaba el objeto con dos
  cabezas y la app sin abrir, y una escritura cortada se pegaba a la siguiente y el diario ya no
  abría. El de Bases destapó además, en los dos puertos y en el código que comparte la conectada
  (`nucleo/bases`), que repetir tras un corte la misma operación no daba lo mismo, arreglado
  (`TestContratoDeBasesRepetirTrasUnCorte`): un alta (base, tabla, campo, vista, registro,
  comentario) o un adjunto repetidos creaban otra fila, porque el id nuevo entraba en la
  operación del motor (ahora la operación es lo que se pide, el id sale al prepararla y la
  repetición retoma la pendiente y da la misma fila; el adjunto, con `MutateWithContent` como
  las fotos de Photos: la ficha queda en el diario antes de subir el fichero); la celda repetida
  devolvía un registro vacío (ahora el escrito, con `Replayed` como el servicio); renombrar
  otra vez lo que un corte dejó a medias retomaba lo cortado y perdía el nombre nuevo (la
  operación lleva ya el nombre); y lo repetido no se anotaba en la actividad. Queda: una alta
  cortada que nadie repite sigue pendiente en el diario, y la misma alta pedida después la
  retoma (da esa fila, ya visible, en vez de otra), como en Contacts. Diferencias aceptadas,
  con su motivo en cada contrato: la línea a medias solo se simula
  en disco, y solo el disco libera el fichero de un adjunto soltado (una sala no borra nunca lo
  publicado).
- Los tests de `apps/fyne/ui` compilan y pasan también con `-tags goolm,edicionlocal`: los que
  abren una sesión de verdad llevan `//go:build !edicionlocal`.

## Pendiente

- **macOS (DMG) e iOS**: `empaquetar.sh macos` firma con Developer ID solo los App ID de la
  conectada (`derechos_sesion` y un perfil Developer ID por app), y Apple no tiene aún los
  `eu.sytue.office.<app>` (PLAN §11, punto 5). Sin cuenta ni App Group en macOS, la edición podría
  firmarse sin perfil (como `demo`), pero no se ha probado en un Mac; cuando esté, el DMG irá por el
  job `macos` de `release-desktop.yml` con una entrada de edición (es el único flujo con los
  secretos de Apple). El `.ipa` de `fyne package --release` no admite los identificadores OAuth de
  la compilación.
- **`auth` en Rooms**: ni `sesion` (C-04: `ui/suite.go` y `apps/fyne/anfitrion`) ni
  `nucleo/auth` entran ya en las apps (lo que el cliente `api`, `offline`, las nubes y las
  pantallas compartidas usaban de `auth` vive en la hoja `nucleo/autorizacion`, con alias en
  `auth`; la caché cifrada de `offline` no tiene llavero con `edicionlocal`), salvo en Rooms:
  su pantalla compartida enlaza `cifrado`, `sesion` y el cliente de Matrix, y con ellos
  `auth` (excepción documentada en el guardián); sacarlos pide partir esa pantalla.
- **Rutas de `api` sin camino local** (Anexo A del plan): las que quedan son las de la columna
  «No está» de cada app; cada una sería una interfaz opcional más su implementación.
- **Notepads**: la nota de imagen por URL se carga a petición, por el permiso puntual del
  portero, y queda dentro de la nota; si cambia en su sitio se vuelve a bajar a mano
  («Volver a cargar la imagen», en el «…» de la nota), nunca sola. La vieja se suelta de la
  nota y la quita del disco la compactación al cerrar (no en el acto, como «Eliminar para
  siempre»). El éxito de volver a cargarla está probado con una bajada de mentira y la prueba
  visual solo ve el fallo (este equipo no sale a Internet). HEIC necesita un conversor del
  sistema (el de Photos: Image I/O, `sips`, `heif-convert` o el ffmpeg de la persona), sin
  probar con una HEIC real (este equipo no la genera). Un enlace `file://` apunta a la ruta de
  ESTE equipo (si el fichero se mueve, el enlace se rompe; en otro equipo no lleva a nada).
- **Compactar el almacén** (§5.6, `nucleo/local/almacen/compactar.go`): reescribe `registros.jsonl`
  con la única cabeza de cada objeto (sin padres: el motor la acepta como raíz) y lo que cita
  un contenido vivo, y barre `objetos/`; atómico y con cerrojo del sistema entre instancias (no
  compacta si otra tiene el almacén abierto ni con trabajo a medias). Photos ya no lo usa (su
  fototeca es SQLite y vaciar la papelera borra en el acto); Notepads, al «Eliminar para siempre» (en el acto) y al cerrar; Bases y
  Calendar, al cerrar (Contacts tampoco usa el almacén: su libreta SQLite sobrescribe con ceros
  lo eliminado para siempre y vacía su diario en el acto). **Bases**: borrar un
  adjunto suelta su fichero (`bases.Repository.SoltarAdjuntos`) y la compactación lo libera.
  Las miniaturas de Photos no llevan caché aparte: ya están en disco en `objetos/` y se leen
  en 2–5 ms (medido con una foto de 12 MP), así que una segunda copia no compensa (FO-08). **Hace falta** (medido, abajo): un historial de 1.000.102
  registros (las 5.000 notas guardadas cien veces, 1,5 GiB de diario) tarda 60 s en abrir y pide
  4,2 GB; compactarlo lleva 29 s (queda en 10.102 líneas) y después abre en 0,5 s con 90 MiB.
- **Prueba de carga del almacén** (§5.6, R-MEDIR; 2026-11-25): `*/carga_test.go` en `notes`,
  `photos`, `contacts`, `local/biblioteca` y `local/fototeca` (la de Photos sobre SQLite, la
  misma carga), con `nucleo/local/almacen/cargaprueba` (genera cada
  carga una vez con el repositorio de verdad, en tmpfs para no pagar el fsync de cada línea, y la
  copia al disco; abre en frío en un proceso nuevo, mide memoria y repite: mediana y mín–máx).
  `SYTUE_CARGA_DIR=<carpeta>` guarda las cargas entre ejecuciones (ocupan 4,5 GiB),
  `SYTUE_CARGA_FOTOS`/`SYTUE_CARGA_VERSIONES` cambian el tamaño, `SYTUE_CARGA_PERFIL_MEMORIA`
  da el perfil de memoria con todo abierto. Medido en este equipo, no en un runner del CI: 4 CPU
  compartidas con otros agentes (carga 13–16), caché de disco caliente:

  | Carga | Abrir en frío | Memoria (montón / pico RSS) | Operación | Umbral |
  | --- | --- | --- | --- | --- |
  | 5.000 notas, 50 cuadernos | 0,53 s | 39 / 90 MiB | guardar 70–82 ms (45–239) | abrir sí; **guardar no** (< 50 ms) |
  | 50.000 fotos, 200 álbumes (motor de registros, antes) | **26,7 s** | 880 MiB / **2,7 GB** | página siguiente < 0,1 ms; la primera 1,3 s; álbumes 1,6 s | página sí; **abrir y memoria no** |
  | 50.000 fotos, 200 álbumes (**SQLite**, plan B) | **1,7 ms** (1,1–1,8); con años y recuerdos, 4,5 ms | 4 / **26 MiB** | página 0,9 ms; álbumes 25 ms; buscar 12 ms; repetidas 47 ms; importar 3,7 ms | **sí** |
  | 50.000 fotos: migrar a SQLite (una vez) | 36 s (35–36) | 5 MiB / 2,6 GB | abrir la migrada 1,7 ms | — |
  | 5.000 fotos, 20 álbumes (motor de registros) | 3,3 s | 96 / 308 MiB | página < 0,1 ms | sí |
  | 20.000 contactos | **6,8 s** | 508 MiB / 1,0 GB | buscar 32 ms | buscar sí; **abrir no** (< 2 s) |
  | 20.000 contactos, libreta SQLite (plan B, abajo) | 2,2 ms | 4 / 25 MiB | buscar 0,7 ms | sí |
  | 2.000 documentos × 50 versiones | 0,15 s | 31 / 53 MiB | guardar 3,1 ms | sí |
  | 1.000.102 registros de historial | 60 s; compactado, 0,5 s | 4,2 GB; compactado, 90 MiB | compactar 29 s | hace falta compactar |

  Arreglado (lo que el perfil señaló, con pruebas de que el resultado es el mismo): **el motor ya
  no vuelve a comprobar lo comprobado** en el almacén local (`records.VerifiedContentCacher`, por
  generaciones: el puerto la sube al releer su diario, y entonces todo se comprueba de nuevo; la
  edición conectada no cambia) ni descodifica en cada `Selected` la cabecera de cada cabeza;
  **Notepads** guarda las relaciones ya leídas de cada nota (guardar descodificaba las 5.000):
  guardar baja de 157 ms (130–258) a 70–82 ms con el equipo más cargado, y abrir otro cuaderno de
  175 a 95–111 ms; **Photos** proyectaba la línea de tiempo una vez por revisión del catálogo
  (`photos/linea.go`, como Contacts): cada página recorría la galería entera y la siguiente
  salía en menos de 0,1 ms; y **Docus**: `SaveDocumentChecked`
  devolvía como base un reloj que no era el guardado, y el segundo autoguardado de una ventana
  daba conflicto (`guardar_encadenado_test.go`). Pendiente, grande: (1) **cada escritura recarga
  el catálogo entero** del motor (`storage.LoadAtomicCatalogFrom`, O(objetos): dos tercios de
  guardar una nota); recargar solo lo añadido desde el sello del puerto pide cambiar el motor;
  (2) **el puerto tiene el diario entero en memoria** (el 80 % de los 880 MiB de las 50.000
  fotos; 684 MiB de diario): dejar el contenido en el disco y leerlo por su desplazamiento;
  (3) **abrir valida el catálogo entero** cada vez (15 de los 25 s de Photos, casi todo Contacts,
  que además descodifica cada contacto dos veces): o validación incremental guardada, o el plan B
  de §5.6 (su `*Data` sobre SQLite), que ya tienen Photos y Contacts (abajo). **Photos ya tiene su plan B**
  (`nucleo/local/fototeca`, 2026-11-25): su `*Data` sobre SQLite con índices (la línea por
  fecha e id, favoritas, papelera, años, recuerdos por mes y día, huella, latitud, enlazados,
  miembros de álbum por posición) y los ficheros por su huella como antes; abre en 1,7 ms y
  26 MiB con 50.000 fotos frente a 26,7 s y 2,7 GB, y la lista de álbumes baja de 1,6 s a 25 ms.
  Migrar la fototeca de antes cuesta, una sola vez, lo que costaba abrirla (36 s y 2,6 GB con
  50.000: casi todo es el motor validando el almacén al abrirlo). Importar una foto en disco son ~40 escrituras sincronizadas
  (150 ms en este equipo; 7 ms en memoria). **Umbrales** (se ajustan con esta medición): los de
  Notepads, documentos y la página de Photos valen; «guardar» debería medirse en el runner del
  CI, porque aquí el fsync va de 0,2 ms a cientos; la memoria, como pico RSS; y con el motor de
  hoy ni Photos ni Contacts lo usan: los dos tienen su plan B sobre SQLite y cumplen con 50.000
  fotos y 20.000 contactos.
- **Contacts sobre SQLite** (plan B de §5.6, 2026-11-25; `nucleo/local/libreta`): la libreta es
  `Datos/contacts/libreta.sqlite` (WAL, un escritor, 0600). `fila` guarda cada fila de Contacts
  (libreta, tarjeta con la vCard entera, contacto, correo, teléfono, etiqueta, miembro) con su JSON
  tal cual y lo que la relaciona en columnas, con índice por clase e id, por contacto y por
  etiqueta; `libreta`, `contacto` y `grupo`, lo que se ve de cada uno (su JSON de `api`) con sus
  claves de orden, el texto plegado de la búsqueda y si está destacado, en la papelera, en una
  libreta borrada o tiene cumpleaños, e índices parciales para la lista en orden, la búsqueda
  (cubriente: se recorre en orden y para al tener los que pide), la página de una libreta, la
  papelera, los favoritos, los cumpleaños y el UID (con y sin `urn:uuid:`, para CardDAV y
  «Traer»); `correo`, por libreta (lo que mira importar un CSV); y `favorito`, el índice de
  favoritos por UID. Cada escritura lee por índice la parte de la libreta que mira y toca, prepara
  el cambio con las MISMAS funciones del repositorio de Contacts (`contacts/operaciones.go`,
  sacadas sin cambiar lo que hacen), lo compone y valida igual y guarda la diferencia; las
  consultas no descodifican ninguna vCard (`TestLasConsultasVanPorIndice` mira el plan de cada
  una), y la página de una libreta lleva la huella de su versión, no la de la libreta entera.
  **Migración**: la primera vez que se abre (Contacts, o «Traer» del Hub, con las apps de
  `traer.Escriben` cerradas) con un almacén de registros al lado, lo lee con el repositorio de
  Contacts y lo pasa en UNA transacción, comprobando las filas de cada clase, las libretas,
  contactos, etiquetas y favoritos y la huella de todas las filas; solo entonces pone la marca
  (`meta.migrada`). Un corte a mitad no deja nada y se repite; el almacén viejo no se toca ni se
  borra solo. Mientras migra, la ventana enseña cómo va, como Photos («Poniendo al día tus
  contactos»: la barra sin medida mientras lee el almacén y con el porcentaje al pasarlo;
  `libreta.HayQueMigrar`, `libreta.AbrirCon` con `Opciones.Avance`; textos en los 26 idiomas);
  si falla lo dice en la ventana y se repite al abrir. **Contrato §8.2** (`local/libreta/contrato_test.go`): el mismo juego contra el
  repositorio sobre el almacén y contra la libreta, más uno que hace lo mismo en las dos y compara
  todo lo que sale; las pruebas de las cuentas (`local/agenda`: CardDAV con `davprueba`, Google,
  Graph, grupos y etiquetas) y de «Traer» corren ya contra la libreta. La leen y escriben
  `cmd-local/contacts`, `local/agenda` (por `contacts.LibretasExternas`), `local/traer`, la
  búsqueda del Hub (`local/indice`, que ya no enseña la papelera), `local/exportar` (que llevaba al
  `.vcf` solo los 500 primeros), la copia en caliente, los cumpleaños de Calendar y Mail
  (`contacts/lector`, en solo lectura; sin base aún, el almacén de antes). Medido en el mismo
  equipo y a la vez, con los 20.000 contactos de `libretaprueba` (carga 2–6 en 4 CPU):

  | 20.000 contactos | Almacén de registros | Libreta SQLite | Umbral |
  | --- | --- | --- | --- |
  | abrir en frío y primera pantalla | 5,1 s (5,0–5,2) | 2,2 ms (1,5–2,5) | < 2 s: cumple |
  | memoria: montón / pico RSS | 509 / 991 MiB | 4 / 25 MiB | |
  | buscar (50 resultados) | 51 ms (26–121) | 0,7 ms (0,4–4,2); sin nada que case, 3,7 ms | < 100 ms: cumple |
  | dar de alta un contacto | 0,66 s | 1,6 ms (1,4–3,1) | |
  | editar un contacto | (como el alta) | 1,3 ms (1,1–2,5) | |
  | importar un CSV de 2.000 filas | 36 s | 1,8 s (1,7–1,9) | |
  | la libreta entera en la pantalla (`InBook`) | — | 148 ms | |
  | Mail: releer la libreta tras un cambio | 4,9 s (detrás; la letra contestaba con lo viejo) | no hay relectura: la letra siguiente ya lo ve, 6,5 ms | |
  | Mail: una letra con la libreta abierta | 0,2 ms | 0,7 ms | |
  | Calendar: los cumpleaños | releer la libreta (4,9 s) | 48 ms | |
  | pasar el almacén a la base (una vez) | | 8,4 s (8,3–8,5) | |

  Se repite con `SYTUE_CARGA=1 go test -tags goolm -run 'CargaLibretaSQLite|CargaContactos$' -v
  -timeout 0 ./contacts/` (y lo de antes, con `-bench 'Alta/20000|ImportarCSV'` y el de
  `contacts/lector`). Una etiqueta de 3.000 miembros se renombra en 0,4 s: resolver los miembros
  era cuadrático también en el repositorio de Contacts (5 s; `contacts/groups.go` los indexa ya
  una vez por escritura). **Borrar los datos de antes de la migración** (2026-11-25), en Ajustes →
  Datos de cualquier app (`local/ajustes_antes.go`): una fila por app (Photos y Contacts) SOLO si
  su migración está hecha y comprobada (la marca: en Contacts, `Origen` «registros» con la huella
  de las filas; en Photos, la fecha con los recuentos de la comprobación; una base que nació sin
  almacén no la tiene) y queda algo, con lo que ocupa; pide cerrar las demás apps que usan esos
  datos (`Abiertas`: Contacts, Hub, Mail y Calendar; Photos y Hub; la propia no, porque lo vivo
  de su base no se toca), pide confirmarlo y dice lo que liberó (o lo que quedó, que se puede
  volver a borrar). Borra solo lo del almacén viejo (`almacen.Restos`: el diario, el diario
  transitorio y su clave, el origen, los temporales y, al final, el cerrojo), con su cerrojo
  exclusivo (una copia de antes que lo tenga abierto lo impide), más: en Contacts, `objetos/`
  entero (la libreta no guarda ficheros) y nunca `agenda/`; en Photos, el diario conservado y, de
  `objetos/`, solo lo que ya no cita la fototeca, decidido y borrado dentro de una transacción de
  escritura (`fototeca.BorrarAntes`), sin tocar las subidas en curso. Probado: que lo vivo se
  queda (cada original y derivada citados, lo importado, editado y purgado después de migrar, la
  base, `agenda/`, lo ajeno) y se ve igual con la app abierta y al reabrir, que lo viejo se va, que
  sin migración (o con el almacén abierto) no se ofrece ni se borra nada
  (`fototeca/antes_test.go`, `libreta/antes_test.go`, `local/ajustes_antes_test.go`), y bajo Xvfb
  con 20.000 contactos (133 MB liberados) y una fototeca de 40 fotos (solo sobraron los 5 objetos
  que ya no citaba; los 121 vivos, todos).
- **Móvil (F11)**: **Android compila y se empaqueta** (2026-11-25, SDK de `accion.sh android-sdk`:
  NDK 28.2.13676358, build-tools 36.0.0, API 36): `SYTUE_ARBOL=cmd-local empaquetar.sh android
  docus notes` saca sus AAB, sin el broker de sesión, los widgets, el proveedor de documentos ni el
  servicio de avisos de la conectada (PLAN §7.2; `verificar_office` lo comprueba en el AAB), y con
  ML Kit GenAI dentro (ver «IA del sistema — Android»: +4,1 MiB en la APK universal de Docus, de
  33,1 a 37,2 MiB; el `.so` de Go son 73 MiB sin comprimir). `anfitrion` y
  `nucleo/local/asistente` pasan `go vet` y `go build` con el clang del NDK. Notepads no
  empaquetaba en ningún sistema (`.zip` sin tipo MIME). **Sin probar en un dispositivo ni en el
  emulador** (aquí no hay `/dev/kvm`): falta ver la IA del sistema en un teléfono con AICore (Pixel 9
  o posterior), y las otras trece apps en Android. Las bibliotecas de ML Kit y de Play Services
  son de licencia propietaria de Google (ML Kit Terms of Service, Android SDK License) y la Prompt
  API solo existe en beta: decisión del dueño antes de publicar (ver «ML Kit GenAI» en
  `docs/operations.md`). En iOS falta probar `xcode/ia-sistema/construir.sh` en `sytue-macos`;
  el destino de un «Guardar como» de iOS no sobrevive a cerrar la app (falta un marcador de
  seguridad); en iOS los ajustes de toda la edición (perfil, IA, cuentas, secretos) ya van
  al App Group `group.eu.sytue.office` (`espacio.AbrirMovil`), pero el dueño tiene que registrar ese
  grupo en Apple Developer y asociarlo a los App ID `eu.sytue.office.*` (sin él, cada app tiene los
  suyos, como hasta ahora); en Android, compartir los ajustes entre apps pide un ContentProvider con
  permiso de firma (`protectionLevel="signature"`), que `empaquetar.sh` aún no declara.
- **Cast** (§6.10): descodificador del sistema de Apple (AVFoundation) con la interfaz
  `video.Sistema` (no se puede probar aquí); GStreamer por proceso no sabe empezar en un instante
  (buscar relanza y descarta) y toma el instante de cada fotograma de la cadencia. **Android: hecho,
  sin probar en un dispositivo** (aquí no hay emulador). El descodificador es `MediaExtractor` +
  `MediaCodec` sin superficie (`video/sistema/mediacodec_android.go` y la clase
  `eu.sytue.nativo.VideoSistema`): un extractor por pista, el fotograma en I420 ya derecho (con el
  giro de la pista), el sonido en PCM mono a 48 kHz, buscar con `seekTo`; lo que el teléfono no sabe
  descodificar falla al abrir y se queda como hoy. La miniatura es `MediaMetadataRetriever`
  (`videoteca/miniatura_android.go`). Probado en la JVM contra un `android.media` de mentira
  (`VideoSistemaPrueba`), con `go vet`/`go build` del NDK y dentro del dex de la APK de Cast; falta
  verlo en un teléfono (el ritmo de un 4K y los descodificadores de fabricante). **Windows: hecho y
  SIN PROBAR en el sistema** (compilado en cruzado; probado en Linux solo lo que no es COM): el
  descodificador con Media Foundation (`video/sistema/mediafoundation`, Go puro por
  `nucleo/sistema/com`, registrado por `sistema.Registrar`) y la miniatura del Explorador
  (`IShellItemImageFactory`, `local/videoteca`); falta verlo en un Windows de verdad (qué mirar, en
  la documentación del paquete), y no aplica la relación de aspecto de los píxeles no cuadrados ni
  la rotación. Un WebM que Go no descodifica pasa al sistema solo si falla (o tarda) el PRIMER
  fotograma; si Go se atasca más adelante (un fotograma que no sabe a mitad), no se cambia.
- **Rooms** (§6.14): Teams y Google Chat (RO-05, RO-06) están probados solo contra servidores de
  mentira (`httptest` y la prueba visual): faltan los clientes OAuth de la compilación (registrar la
  app en Microsoft Entra y en Google Cloud, §11), declarando también los permisos opcionales
  (`Files.ReadWrite.All` de los adjuntos de Teams y `chat.spaces.create` de los directos nuevos de
  Google, que es sensible), y, con el de Sytue, leer Google Chat pide la verificación de un permiso
  restringido (D-22: hoy, solo con el cliente propio del Workspace); sin leer, Google Chat tampoco
  deja bajar ficheros ni nombrar los directos que ya había. Que la People API dé el nombre de quien
  escribe con el permiso `profile` depende de la organización (si no, «Persona de Google»). La
  búsqueda de Teams y Google Chat es en lo cargado en la sesión (Microsoft Search pide otros
  permisos y Chat no busca con la autenticación de la persona); en un canal de Teams, los adjuntos
  se comparten con un enlace para la organización (Graph no da sus miembros con los permisos que se
  piden); los chats de Teams no tienen hilos (responder con cita no está). RO-01: los tests de
  integración del servicio (`-tags integration`) pasan contra PostgreSQL 18.6 (2026-11-25), igual
  que los de `services/drive` y `services/cast`, cuyas migraciones usan `pg_c_utf8`. En Matrix, invitar
  es invitar (la persona entra al aceptar) y un servidor que cifre por su cuenta las salas
  privadas las deja ilegibles aquí; «Hilos» solo ve los hilos de lo ya cargado (Slack no tiene una
  lista de hilos).
- **Avisos**: en macOS sin empaquetar Fyne cae a su reloj del proceso; Windows y Apple sin
  probar en su sistema (compilados con `GOOS=windows go vet`). **Copia**: «Exportar en formatos
  abiertos» de todas las apps a la vez ya está en el Hub (ver «Hub»). **Cuentas**: el
  autodescubrimiento no consulta la base de proveedores de Thunderbird (ISPDB), que es un
  servidor de terceros.
- **Sincronización de Calendar y Contacts** (§6.11): hechos iMIP (A-11) con su cola de reintentos,
  invitar desde el formulario en los calendarios de una cuenta (con la libreta de Contacts y el
  ORGANIZER de la cuenta), la ocupación de los invitados en «Buscar un hueco» (A-13: VFREEBUSY de
  CalDAV, `freeBusy`, `getSchedule`), las respuestas de los invitados que llegan por correo a un
  CalDAV sin planificación (Mail las deja en su cola `respuestas-imip` y Calendar las pone en el
  evento y las sube), el enlace de Meet o Teams desde el formulario (A-13: Google
  `conferenceData.createRequest`, Graph `isOnlineMeeting`, o la cuenta de Rooms que la cree), las
  etiquetas de Google (`contactGroups`) y de Outlook (categorías), crear libretas en la cuenta
  (MKCOL extendido; carpetas de Outlook), las respuestas, zonas, ocurrencias y excepciones de Graph,
  `MKCALENDAR`, los grupos `KIND:group`, la libreta de solo lectura, vCard 3.0 y S-02; Mail y
  Calendar autocompletan con la misma libreta de Contacts (`lector.Libreta`, sobre su base SQLite). La
  respuesta a una sola ocurrencia que aquí no tiene su excepción la crea (copia del maestro en su
  hora, con RECURRENCE-ID) marcada como venida de una respuesta: la vuelta la sube sin mandar
  ninguna invitación, y un cambio posterior de la persona sí sale, con ella; y la que llega a una
  entrada que estaba vacía en la primera vuelta de la cuenta ya cuenta como nueva (la carpeta
  queda `estrenada`). El SEQUENCE de las respuestas y de lo que sale por correo se leía como 0. Guardar
  el formulario ya no pisa la respuesta que llega mientras se edita (ni la de una ocurrencia), y la
  SEQUENCE no sube por lo que los invitados no ven; tampoco «solo este» y «este y los siguientes»
  (`DividirSerie`: el evento nuevo lleva lo contestado a esa ocurrencia o a la serie, y las
  excepciones desde ella), lo cambiado en la serie llega a sus excepciones y un evento de la
  cuenta escrito de otra forma (EXDATE con zona, sin STATUS) se guarda como estaba y no cuenta
  como cambio (la comparación y la firma del iMIP son por lo que significa cada propiedad).
  Falta: nada de esto probado contra iCloud, Google, Outlook ni un IMAP o un SMTP reales (sí con
  `davprueba`, que ya da la ocupación y crea libretas, y falsos de Google, Graph, IMAP y SMTP).
- **CertiPDF**: el token del sello de tiempo se autentica contra las raíces del sistema y las de
  la carpeta de confianza (sin probar aquí contra la autoridad real de Izenpe: este equipo no
  llega a ella); los preajustes de autoridades cualificadas
  son solo Izenpe (la persona puede añadir más); el OCR de Vision (Apple). El de Windows
  (`Windows.Media.Ocr`, `nucleo/sistema/ocrwindows`) está hecho y SIN PROBAR en un Windows: Go puro
  por WinRT (`RoGetActivationFactory`, un `SoftwareBitmap` BGRA8 copiado de un `IBuffer`, la
  operación asíncrona esperada preguntando su estado), compilado en cruzado (`GOOS=windows
  CGO_ENABLED=0 go vet`) y probado en Linux solo lo que no es WinRT (reducir la página al lado
  máximo del motor, BGRA8, las cajas de vuelta a la escala original, los idiomas de tesseract
  contra los de Windows); qué mirar la primera vez, en la documentación del paquete. Reconoce un
  idioma cada vez (el primero de «spa+eng» que esté instalado).
- **Drive** (DR-12, DR-13): la carpeta de Android elegida con el selector del sistema (SAF) está
  hecha SIN cambiar el fork de Fyne: como «Guardar como…», lo hace una clase propia de la APK
  (`eu.sytue.nativo.ArbolDocumentos`, con `ficheros.ElegirArbol`/`ficheros.Arbol`), que abre
  `ACTION_OPEN_DOCUMENT_TREE`, guarda el permiso de lectura Y escritura y crea, renombra, mueve, copia
  y borra con `DocumentsContract`; `local/archivos` trabaja sobre un sistema de ficheros abstracto
  (`archivos.Sistema`: `DelDisco`, el de siempre, o un `archivos.Arbol`). Probado en la JVM contra un
  Android de mentira (`android/prueba/arbol`, también al empaquetar) y en Go con proveedores falsos:
  las pruebas de `archivos` pasan enteras también sobre un árbol. Compila y se empaqueta (AAB), pero
  SIN PROBAR en un teléfono. Abrir un documento de esa carpeta lo da a Docus, Calcus, Diapos o
  CertiPDF de Sytue Office (un `ACTION_VIEW` explícito a su paquete con permiso de leer y escribir,
  `ArbolDocumentos.verEn`, probado en la JVM; el tipo es el que su manifiesto declara, y una prueba lo
  compara con `empaquetar.sh tipos`), pero esas apps lo traen como documento NUEVO (Docus, Calcus y
  Diapos lo importan; CertiPDF lo copia a su carpeta «PDF»): guardar no vuelve al `content://`. «Copiar
  a…» y «Subir carpeta» ya eligen con el selector en Android (en iOS siguen sin selector de
  carpetas); la cuota solo se sabe del
  almacenamiento del teléfono y de las tarjetas; en el árbol, sustituir un fichero no es atómico (se
  borra y se renombra: un corte entre medias deja el temporal y la versión anterior en `.sytue`, no un
  fichero a medias); un proveedor que no sabe mover mueve copiando y borrando. Los documentos de una
  carpeta de nube que se llaman igual se ven todos («nombre (2).ext») y se abren, guardan, renombran,
  mueven y tiran por su identificador: probado con una API de Google Drive de mentira, WebDAV con
  `displayname` repetido (también en la app, bajo Xvfb) y un Graph de mentira; sin probar contra Google ni un
  WebDAV reales. **Mover entre unidades** (copiar, comprobar y quitar el origen), los **favoritos y
  etiquetas de la nube** y la **pregunta de los homónimos al subir** están probados con la API de Google
  Drive de mentira (con su MD5), nubes de rutas en memoria con fallos a propósito (una subida que se
  corta, una copia que llega distinta, una bajada que falla, cancelar a mitad), el disco y, en la app bajo
  Xvfb, un WebDAV local (`golang.org/x/net/webdav` con `displayname` repetido): sin probar contra Google,
  OneDrive ni un Nextcloud reales (el `oc:fileid` y la papelera de Nextcloud son los de su
  documentación). Lo que se mueve se lee entero en memoria, hasta los 100 MiB que admite una subida a la
  nube (lo que pasa de ahí no se mueve y se dice); OneDrive de empresa solo da `quickXorHash`, que no se
  calcula: su copia se relee. La estrella o las etiquetas de algo que se tiró desde la web del
  proveedor (o que estaba dentro de una carpeta tirada, salvo en WebDAV, donde van por la ruta) siguen en
  «Favoritos» hasta que se quitan (abrirlo dice que no está); en un WebDAV sin `oc:fileid`, renombrar o
  mover desde su web las pierde. Arrastrar no lleva a otra
  unidad: en la vista solo hay carpetas de la misma.
- **Photos** (FO-09): los vídeos entran ya por todas las puertas (carpeta enlazada; carrete, «Añadir
  fotos» y soltar, copiados), pero el carrete con vídeos (MediaStore con duración y lados, probado en la
  JVM contra un MediaStore de mentira, `CarretePrueba`; PhotoKit, sin compilar aquí) está SIN PROBAR en
  un teléfono. En el móvil «Añadir fotos» abre el selector del sistema con fotos y vídeos (Android, el de
  `apps/fyne/anfitrion` que ahora pone también la edición; iOS, el PHPicker con vídeos por
  `ficheros.ElegirFotosYVideos`, sin compilar aquí). Un vídeo grande se lee varias veces: al enlazarlo
  (la huella del motor de importación, la del destino y la del repositorio) y al copiarlo (la huella, el
  diario sellado y el almacén); si la caché no puede enlazar al almacén (otro disco y sin enlaces
  simbólicos, como Windows sin modo de desarrollador), verlo copiado lo copia una vez a la caché. Un WebM
  cuyo vídeo no saca Go (VP9 de perfil 1 o 2, AV1) va ya por el descodificador del sistema (probado con
  GStreamer y el `mandelbrot` de ffmpeg, en Photos y en Cast); sin descodificador del sistema (GStreamer,
  Media Foundation o MediaCodec), ni eso ni lo que no es WebM se reproduce en el visor (Cast lo
  convierte). En Android el visor reproduce con MediaCodec y el póster sale de él o de
  `MediaMetadataRetriever` (hecho, sin probar en un dispositivo); en el TELÉFONO, con uno copiado no se
  ofrece «Abrir en Cast»: en el escritorio se le deja en la carpeta de vídeos de Cast (clon o
  copia), pero en el móvil cada app tiene su carpeta de datos y Photos no alcanza la de Cast; haría
  falta darle el vídeo por un `content://` con permiso de lectura (un `FileProvider` que `empaquetar.sh`
  aún no declara) en Android, o por el App Group de iOS. En un sistema de ficheros que no clona (ext4,
  NTFS) el vídeo ocupa dos veces. En Windows, como en Cast, hecho y SIN PROBAR en el
  sistema: el visor reproduce con Media Foundation y el póster de un vídeo cae, sin ffmpeg, a la
  miniatura del Explorador.
- **ERP** (§6.13): VERI*FACTU, tras la revisión legal (D-29, E-07). La factura enviada por correo por una cuenta
  cuyo servidor no guarda la copia (iCloud, IMAP genérico) llega a «Enviados» cuando Mail se abre (la recoge de la
  cola compartida `enviados-por-subir`); con Mail sin abrir, no está en el servidor. «Abrir en el
  correo» lleva el PDF adjunto al redactor de Sytue Mail por la carpeta `para-enviar` (en el escritorio,
  `<caché>/sytue-office/para-enviar`; en iOS, la del App Group, sin probar en un iPhone); en Android las
  apps no comparten ninguna carpeta (cada una en su contenedor; haría falta un `FileProvider` con permiso
  por URI) y cae a `mailto:` con el PDF en Descargas, igual que en la variante de la tienda de macOS
  (sandbox sin grupo). Que Mail esté instalada se pregunta al sistema en Linux (`xdg-mime`) y Windows (el
  registro) y en macOS falla `open`; en iOS no se sabe: sin Mail, el enlace no abre nada y el PDF se barre
  al día. El botón sigue saliendo solo sin cuenta en Mail (con cuenta se envía desde la ficha), así que
  Mail abre el redactor sobre su bienvenida y para enviarlo hay que añadir antes una cuenta. La conectada
  (`cmd/erp`) tendría PDF y enviar con `p.Salida = erp.SalidaFactura{PDF: true, Enviar: true}` (rutas del
  servicio); no se enciende sin decidirlo. De lo que propone la IA, el NIF-IVA ya se
  comprueba en VIES al pulsarlo (`local/erp.VIES`, propósito `vies` del portero); la web y el
  correo, no (cada uno pediría su propósito y un servicio que no hay). `ec.europa.eu` consta como
  `blocked` en `providers.tsv` hasta acreditar desde dónde se sirve (aquí no hay DNS ni RDAP para
  mirarlo); si es la red de la Comisión, pasa a `eu-approved` y sale de
  `conexiones-del-dispositivo.tsv`. Los vectores dorados pasan en
  PostgreSQL 18.6 (2026-11-25, la imagen `postgres:18.6-alpine` del CI, fijada por su digest), con la
  colación `pg_c_utf8` nativa, y dan lo mismo que el SQLite de la edición (`local/erp`); pasan también
  todos los tests de integración de `services/syerp`. Producción corre en el 17 de Scaleway (la
  brecha 17/18 es la de `docs/operations.md`).
- **Ofimática** (O-04): `GuardarComo` con NSSavePanel en la variante de la tienda de macOS (`ficheros.GuardarEnElMovil` de iOS
  y Android ya está: ver «Móvil»).
- **Iconos de la edición** (§5.2, 2026-11-25): hechos y vistos en el `.desktop` del bundle de
  Linux (el PNG de `share/pixmaps` es el de `cmd-local`); el `.icns`, el `.ico`, el mipmap de
  Android y el catálogo de iOS salen del mismo `Icon.png` con `fyne package`, pero no se han visto
  en un Mac, un Windows ni un teléfono. Android usa el icono de siempre (no adaptativo: la app no
  trae `Icon-foreground.png`), así que el lanzador lo encoge entero dentro de su forma; si algún día
  se le dan capas adaptativas, la insignia ya está dentro del círculo seguro. Generados aquí con el
  `convert` de ImageMagick 6 (este equipo no tiene el 7); con `magick` salen los mismos píxeles
  salvo el suavizado. En Notepads la insignia tapa la esquina doblada de su glifo (va donde la de
  las quince). **El explorador de la nube** (`ExplorarNube`, lo que abre «Abrir de la nube…» y
  «Guardar en la nube…») sigue con botones de Fyne dentro de su `ui.Dialogo`.
- Registrar los clientes OAuth de Google y Microsoft y pasar la verificación de Google.
- **Ajustes → Datos y OAuth en el móvil**: el cifrado del disco de macOS (`fdesetup`, `diskutil`) y
  de Windows (`System.Volume.BitLockerProtection` por PowerShell) está probado con salidas grabadas,
  no en su sistema; aquí solo se ha visto Linux (ext4 sin dm-crypt: «sin cifrar»). Mover la carpeta
  de datos, probado en Linux (pruebas y Xvfb), no en Windows ni macOS. No se reescriben las rutas
  absolutas que van DENTRO del almacén del motor (un enlace `file://` de una nota o una carpeta
  enlazada de Photos que estuviera dentro de la carpeta de datos), porque sus registros van por su
  huella. La vuelta OAuth por el esquema de la app, sin probar en un teléfono; con Google hace falta
  un cliente de iOS o Android por app y declarar en `empaquetar.sh` el esquema del identificador del
  paquete (`eu.sytue.office.<app>`), y si iOS cierra la app mientras la persona está en el navegador,
  el alta se pierde y hay que repetirla.
- **Mail**: en iOS, ser el programa de correo predeterminado pide a Apple el derecho
  `com.apple.developer.mail-client` (el esquema `mailto` ya va en el Info.plist). La libreta de
  Contacts, medida con 20.000 contactos realistas (`nucleo/contacts/lector/libreta_bench_test.go`, en
  un Xeon de 2,3 GHz con 4 núcleos): releerla tardaba **245 s** (cada página de 500 rehacía la
  instantánea entera: 128 GB asignados), luego **11,5 s** (una instantánea y una proyección,
  `Snapshot.ContactosVisibles`) y ahora **4,8 s** (lo que valida al abrir es ya la instantánea, sin
  leerla dos veces, y el motor no valida dos veces cada índice); ocupa 17,4 MiB y 2,7 MiB más de
  textos de búsqueda. Como sigue
  pasando de 200 ms, se relee DETRÁS: la letra que llega tras un cambio contesta en 9 µs con la lista
  de antes (antes esperaba la relectura entera) y una sola relectura sirve a todas; Mail la lee al
  abrirse. Una letra del autocompletado bajó de 16–21 ms (98 ms y 170 MB de basura si nada casa) a
  0,18 ms (0,9 ms sin basura), plegando cada contacto una vez por lectura (`contacts.TextosBuscables`);
  en Calendar, de 17 a 0,19 ms. Ninguna llamada corre en el hilo de la interfaz. Releer sigue siendo
  leer el almacén entero (el lector abre su vista cada vez), pero ya solo mientras Contacts no se
  haya abierto con su libreta SQLite: con ella no hay relectura ni copia en memoria, y cada letra
  es una consulta con índice a la base (ver «Contacts sobre SQLite»). **En Contacts ya no se relee**
  (`nucleo/contacts/foto.go` y `vistas.go`): la instantánea se guarda con la revisión del catálogo
  del motor (`records.Repository.CatalogRevision`, que sube con cada cabeza nueva y con el sello del
  almacén, `records.CatalogStamper`); tras una escritura propia, la guardada es la que ya se compuso
  y validó para escribir, si el motor confirma que nada se cruzó (`PublishedSince`); las vistas se
  proyectan una vez por instantánea y la siguiente reutiliza lo que no cambia; y el motor no
  recarga un almacén que no ha cambiado ni revalida lo que ya validó
  (`storage.LoadAtomicCatalogFrom`). Medido con `nucleo/contacts/rapidez_test.go`: dar de alta un
  contacto con 300, 2.000 y 20.000 en la libreta pasa de 215 ms, 1,4 s y 16,6 s a 6–8 ms, 29–36 ms y
  0,39–0,45 s; importar un CSV de 2.000 filas en una libreta vacía, de 15 min 48 s (568 GB
  asignados) a 29–33 s; leer la libreta sin cambios (20.000), de 5,0 s a 2 µs. Sigue creciendo con
  la libreta (~20 µs por contacto en cada alta: copiar y validar en memoria el grafo entero,
  proyectarlo y recargar el índice del motor): bajarlo más pediría estructuras incrementales en el
  modelo de Contacts y en el motor (en Sytue Office, la libreta SQLite: 1,6 ms con 20.000). Lo que deja otra
  app en la cola de «Enviados» de una cuenta que Mail aún no ha sincronizado nunca espera a la primera
  sincronización. **Respuesta automática (M-10)**: probada contra el ManageSieve de mentira y contra
  Dovecot 2.3 con Pigeonhole en este equipo, no contra un proveedor real; el script «sytue-office» lo
  escribe la app y lo que se cambie en él a mano se pierde al guardar (lo dice su cabecera); con varias
  cuentas, la insignia de la fila es la de la primera que responde; las reglas en el servidor (la fase
  posterior de M-09) siguen siendo locales.
- **Hub**: Windows y macOS sin probar en su sistema (la bandeja y el arranque
  al iniciar sesión son los de `nucleo/sistema/escritorio`, los mismos del Hub conectado).
  «Exportar todo» abre los almacenes con el cerrojo compartido de siempre, tras comprobar que sus
  apps están cerradas: una app que se abra a mitad no se bloquea (como en «Traer»). Mail y Cast no
  exportan nada aparte (ya guardan `.eml` y los vídeos tal cual) y Drive son ficheros.
  «Traer» enlaza cada foto y cada vídeo a su original en `Drive/Traído de Sytue <fecha>/Drive/`:
  como esa carpeta no es una vigilada de Photos, si se mueve o se renombra desde Drive, Photos
  dice que falta hasta que se trae otra vez la copia (entonces la encuentra donde esté, por
  tamaño y huella, y recoloca allí sus enlazadas sin copiar nada; si se borró, la vuelve a
  dejar). Una HEIC no se puede enlazar: entra copiada como el JPEG del conversor del sistema, y
  sin conversor se queda en Drive. La tabla de Drive no dice qué fichero es el original de cada
  foto (se casa por nombre y tamaño, prefiriendo `Fotos/`). Lo dejado en Drive se recuerda por
  su ruta en el ZIP y su huella (`Drive/.traido-de-sytue.json`): un fichero que cambió en Sytue
  entre dos copias se deja otra vez, como nuevo. Las notas de fichero, de Drive y de documento
  de la copia no tienen a qué apuntar aquí y no entran.

- **Textos (C-12)**: los catálogos de la web en alemán mezclan el tuteo (Drive, Rooms,
  Calendar, Photos) con el usted (el resto, la mayoría, que es lo que sigue la edición):
  unificarlos es cosa de la web.
