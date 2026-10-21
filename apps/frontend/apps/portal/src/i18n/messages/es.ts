/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ESPAÑOL del portal Sytue One. Traducción del original inglés
 * (`en.ts`), que manda: el tipo exige aquí exactamente sus mismas claves.
 *
 * Tuteo en todo el catálogo, como el resto de la suite («comprueba tu conexión»,
 * no «compruebe su conexión»). Los NOMBRES de producto (Mail, Drive, Docs,
 * Rooms, Cloud…) son marca y no se traducen; las etiquetas literales del panel
 * de Cloudflare tampoco, porque el usuario las va a leer en inglés en su
 * pantalla.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en';

export const es: AppCatalog<typeof en> = {
  // --- Títulos de página -------------------------------------------------
  'page.home': 'Sytue One — Inicio',
  'page.search': 'Buscar en Sytue One',
  'page.searchQuery': '«{query}» — Buscar en Sytue One',
  'page.devices': 'Conecta tus dispositivos — Sytue One',
  'page.data': 'Tus datos — Sytue One',
  'page.downloads': 'Descargas — Sytue One',
  'downloads.title': 'Descarga las aplicaciones',
  'downloads.lead':
    'Aplicaciones nativas para tu equipo. Se instalan, viven en el Dock o en el menú de inicio y te avisan con las notificaciones del sistema.',
  'downloads.version': 'Versión {version}',
  'downloads.get': 'Descargar',
  'downloads.forYou': 'Para tu {platform}',
  'downloads.others': 'Otras plataformas',
  'downloads.all': 'Todas las plataformas',
  'downloads.empty': 'Todavía no hay instaladores publicados',
  'downloads.emptyNoRelease':
    'Ninguna versión ha publicado aún sus instaladores. Aparecerán aquí en cuanto lo haga; por tu parte no hay nada que hacer.',
  'downloads.emptyUnconfigured':
    'Este despliegue no sabe dónde buscar los instaladores. Lo configura un operador.',
  // En la app nativa la lista la pide al Hub, y eso sí puede fallar.
  'downloads.failedTitle': 'No se pudieron cargar las descargas',
  'downloads.failedBody': 'El Hub no responde ahora mismo. Vuelve a intentarlo en unos segundos.',
  'downloads.savedTitle': 'Descarga terminada',
  'downloads.saveFailedTitle': 'No se pudo descargar el instalador',
  'page.activity': 'Actividad reciente — Sytue One',
  'page.welcome': 'Te damos la bienvenida a Sytue',
  // --- Actividad reciente (Sytue Hub) ------------------------------------
  'activity.overline': 'Actividad',
  'activity.heading': 'Actividad reciente',
  'activity.intro':
    'Lo que ha pasado últimamente en tu correo, archivos, documentos, eventos y salas.',
  'activity.today': 'Hoy',
  'activity.yesterday': 'Ayer',
  'activity.empty':
    'Todavía no hay nada reciente. A medida que uses Sytue, lo que toques aparecerá aquí.',
  'activity.failedTitle': 'No se pudo cargar la actividad',
  'activity.failedBody': 'El Hub no responde ahora mismo. Vuelve a intentarlo en unos segundos.',
  'activity.retry': 'Volver a intentarlo',
  'activity.all': 'Todo',
  'activity.filterBy': 'Filtrar por producto',
  'activity.count': { one: '{count} elemento', other: '{count} elementos' },
  'page.inbox': 'Notificaciones — Sytue One',
  // --- Bandeja de notificaciones (Sytue Hub) -----------------------------
  'inbox.overline': 'Notificaciones',
  'inbox.heading': 'Notificaciones',
  'inbox.intro': 'Todo lo que la suite tiene que contarte, en un solo sitio.',
  'inbox.all': 'Todas',
  'inbox.unreadFilter': { one: '{count} sin leer', other: '{count} sin leer' },
  'inbox.filterBy': 'Filtrar notificaciones',
  'inbox.markAll': 'Marcar todas como leídas',
  'inbox.loading': 'Cargando notificaciones…',
  'inbox.unreadDot': 'Sin leer',
  'inbox.errorTitle': 'No se pudieron cargar las notificaciones',
  'inbox.errorBody': 'El Hub no responde ahora mismo. Vuelve a intentarlo en unos segundos.',
  'inbox.retry': 'Volver a intentarlo',
  'inbox.emptyTitle': 'Nada que leer',
  'inbox.emptyBody': 'Cuando algo requiera tu atención, aparecerá aquí.',
  'inbox.emptyUnreadTitle': 'Estás al día',
  'inbox.emptyUnreadBody': 'Lo has leído todo. Cambia a «Todas» para ver las anteriores.',
  'inbox.prefsTitle': 'Qué quieres recibir',
  'inbox.prefsBody':
    'Si apagas un producto, deja de avisarte aquí y por correo. Puedes volver a encenderlo cuando quieras.',

  // --- Lanzador ----------------------------------------------------------
  'launcher.heading': 'Tus aplicaciones',
  'launcher.greeting.morning': 'Buenos días',
  'launcher.greeting.afternoon': 'Buenas tardes',
  'launcher.greeting.evening': 'Buenas noches',
  'launcher.more': 'Más',
  'launcher.apps': 'Aplicaciones',
  'launcher.connected': 'Conectado',
  'launcher.appAria': '{name} — {description}',
  'launcher.servedBy': '{description} · {app}',
  'launcher.disabled': 'Desactivado',

  'product.mail': 'Correo electrónico',
  'product.drive': 'Archivos y colaboración',
  'product.calendar': 'Calendario',
  'product.contacts': 'Contactos',
  'product.docs': 'Documentos, hojas de cálculo y presentaciones',
  'product.rooms': 'Canales, mensajes y llamadas',
  'product.cloud': 'Plataforma de aplicaciones (PaaS)',
  'product.console': 'Administración y organizaciones',
  'product.id': 'Identidad, perfil y seguridad',

  'quick.heading': 'Acciones rápidas',
  'quick.search': 'Buscar en toda la suite',
  'quick.compose': 'Escribir un correo',
  'quick.newDoc': 'Nuevo documento',
  'quick.upload': 'Subir a Drive',
  'quick.newEvent': 'Crear un evento',
  'quick.download': 'Descargar la aplicación',
  'quick.devices': 'Conecta tus dispositivos',
  'quick.data': 'Tus datos',

  'storage.connectTitle': 'Conectar almacenamiento externo',
  'storage.connectDescription': 'Google Drive, OneDrive, Dropbox, WebDAV y más.',

  'recent.heading': 'Reciente en toda la suite',
  'recent.viewAll': 'Ver todo',
  'recent.loading': 'Cargando tu actividad reciente…',
  'recent.errorTitle': 'No hemos podido cargar tu actividad',
  'recent.errorBody':
    'El Hub no responde ahora mismo. Tu trabajo está a salvo; vuelve a intentarlo.',
  'recent.retry': 'Reintentar',
  'recent.emptyTitle': 'No hay actividad reciente',
  'recent.emptyBody': 'Cuando trabajes en Mail, Drive, Calendar o Rooms, lo verás aquí.',
  'recent.emptyCta': 'Empezar en Drive',

  // --- Búsqueda cross-app ------------------------------------------------
  'search.overline': 'Búsqueda',
  'search.heading': 'Busca en toda la suite',
  'search.placeholder': 'Busca en Mail, Drive, Docs, Calendar, Contacts, Rooms…',
  'search.aria': 'Buscar en Sytue One',
  'search.submit': 'Buscar',
  'search.hint':
    'Escribe aquí arriba para buscar a la vez en tu correo, tus archivos, tus documentos, tus eventos, tus contactos y tus salas. Los resultados se abren directamente en su producto.',
  'search.failedTitle': 'No se ha podido completar la búsqueda',
  'search.failedBody': 'El Hub no responde ahora mismo. Inténtalo de nuevo en unos segundos.',
  'search.retry': 'Volver a intentarlo',
  'search.empty': 'No hay resultados para «{query}». Prueba con otras palabras.',
  'search.count': { one: '{count} resultado', other: '{count} resultados' },

  // --- Bienvenida --------------------------------------------------------
  'welcome.heading': 'Te damos la bienvenida a Sytue',
  'welcome.lead':
    'En unos minutos lo tendrás todo en marcha. Vamos paso a paso; si lo dejas a medias, retomamos donde lo dejaste cuando vuelvas.',

  'data.title': 'Tus datos',
  'data.lead':
    'Descarga una copia de todo lo que tienes en Sytue One, cuando quieras y sin pedírselo a nadie.',
  'data.export.section': 'Descargar una copia',
  'data.export.body':
    'Un único archivo ZIP con tu correo, archivos, calendarios, contactos, notas, documentos, fotos y mensajes. Lo que ya tiene un formato estándar va tal cual: los correos en .eml, los calendarios en .ics, los contactos en .vcf y los archivos, tal y como los subiste.',
  'data.export.formatLegend': 'Formato de las listas',
  'data.export.formatHelp':
    'Las listas describen lo que hay en la copia: tus mensajes, eventos, contactos o archivos, uno por fila.',
  'data.export.orgHelp':
    'Cada copia es de una organización. Si eliges otra, entras en ella y la descarga empieza sola.',
  'data.export.json': 'Para otras aplicaciones y programas.',
  'data.export.csv': 'Para abrirlo en Excel, Numbers o Google Sheets.',
  'data.export.xml': 'Para sistemas que piden XML.',
  'data.export.action': 'Descargar copia',
  'data.export.startedTitle': 'La descarga ha empezado',
  'data.export.startedBody':
    'Tu navegador la sigue guardando aunque cierres esta página. Si tienes muchos archivos, tardará un rato.',
  'data.export.scope':
    'La copia incluye los datos de la organización con la que has entrado. El archivo de resumen que va dentro dice qué se ha incluido de cada aplicación.',
  'data.move.section': 'Llevarlo a otro servicio',
  'data.move.body':
    'Para pasarlo directamente a Google Workspace o a Microsoft 365, sin descargar nada: si tu cuenta es personal o administras tu organización, lánzalo tú desde Sytue Admin, en Migraciones; si no, pídeselo a quien la administra.',
  'data.move.action': 'Ir a Migraciones',

  // --- Conectar tus dispositivos ----------------------------------------
  'devices.title': 'Conecta tus dispositivos',
  'devices.lead':
    'Llévate el correo, el calendario, los contactos y los archivos de Sytue al móvil o al ordenador. Te guiamos paso a paso.',
  'devices.leadShort':
    'Llévate el correo, el calendario y los contactos de Sytue al móvil o al ordenador.',
  'devices.unavailable.section': 'Ahora mismo no podemos enseñarte los datos',
  'devices.unavailable.title': 'No hemos podido obtener la configuración',
  'devices.unavailable.body':
    'Inténtalo de nuevo en un momento. Si sigue igual, escribe a quien administre Sytue en tu organización.',
  'devices.retry': 'Reintentar',
  'devices.copy': 'Copiar',
  'devices.copyFailTitle': 'No se ha podido copiar',
  'devices.copyFailBody': 'Selecciónalo y cópialo a mano.',
  'devices.copied.address': 'Tu dirección, copiada',
  'devices.copied.imap': 'Servidor de entrada copiado',
  'devices.copied.port': 'Puerto copiado',
  'devices.copied.smtp': 'Servidor de salida copiado',
  'devices.copied.url': 'Dirección copiada',
  'devices.copied.drive': 'Dirección de Drive copiada',
  'devices.oneClick.thunderbird': 'Configurar Thunderbird',
  'devices.oneClick.outlook': 'Configurar Outlook',
  'devices.oneClick.android': 'Configurar en Android',
  'devices.oneClick.body':
    'Crea una contraseña de aplicación solo para este programa, la copia y te la enseña una vez.',
  'devices.oneClick.failTitle': 'No se ha podido crear la contraseña de aplicación',
  'devices.oneClick.failBody':
    'Inténtalo de nuevo en un momento. También puedes crear una en Mi cuenta.',
  'devices.oneClick.onceTitle': 'Copiada. Solo se enseña esta vez',
  'devices.oneClick.onceBody':
    'Pégala donde el programa pida la contraseña. Si la pierdes, revócala en Mi cuenta y crea otra.',
  'devices.oneClick.passwordLabel': 'Contraseña de aplicación',
  'devices.copied.password': 'Contraseña de aplicación copiada',
  'devices.oneClick.notOpened':
    'No hemos podido abrir {program} en este equipo: ábrelo tú y sigue estos pasos.',
  'devices.oneClick.next.thunderbird':
    'En el asistente de cuenta nueva de Thunderbird, escribe tu dirección y pega la contraseña: los servidores los encuentra solo. Calendarios y contactos: añade un calendario en la red y una libreta CardDAV con la misma dirección y contraseña (si te pide la ubicación, cópiala de abajo).',
  'devices.oneClick.next.outlook':
    'En «Agregar cuenta» de Outlook, escribe tu dirección y pega la contraseña: Outlook encuentra los servidores solo.',
  'devices.oneClick.next.android':
    'En tu app de correo, añade una cuenta «Otra (IMAP)» con tu dirección y pega la contraseña. Calendarios y contactos: DAVx5 con las direcciones de abajo y la misma contraseña.',
  'devices.android.davx5Calendars': 'Calendarios en DAVx5',
  'devices.android.davx5Contacts': 'Contactos en DAVx5',
  'devices.android.davx5Install': 'Instalar DAVx5 desde F-Droid',

  'devices.platform.iphone.title': 'iPhone o iPad',
  'devices.platform.iphone.body': 'Instalas un perfil y ya tienes correo, calendario y contactos.',
  'devices.platform.iphone.note': 'Lo más rápido: un archivo y tu contraseña.',
  'devices.platform.mac.title': 'Mac',
  'devices.platform.mac.body':
    'El mismo perfil que en el iPhone, instalado desde Ajustes del Sistema.',
  'devices.platform.mac.note': 'Apple Mail, Calendario y Contactos.',
  'devices.platform.outlook.title': 'Windows (Outlook)',
  'devices.platform.outlook.body':
    'Escribes tu dirección y tu contraseña; Outlook encuentra el resto solo.',
  'devices.platform.outlook.note': 'Solo correo. El calendario y los contactos van aparte.',
  'devices.platform.thunderbird.title': 'Thunderbird',
  'devices.platform.thunderbird.body':
    'Se configura solo con tu dirección; el calendario y los contactos se añaden después.',
  'devices.platform.thunderbird.note': 'Windows, Mac o Linux.',
  'devices.platform.android.title': 'Android',
  'devices.platform.android.body':
    'Tu aplicación de correo para el buzón y DAVx5 para el calendario y los contactos.',
  'devices.platform.android.note': 'No hay aplicación de Sytue: se usan las del sistema.',
  // Compatibilidad drop-in: Drive, Calendar y Contacts hablan las APIs de Google y
  // de Microsoft Graph. Estaba construido y no se decia en ninguna parte.
  'devices.platform.apis.title': 'Una herramienta que habla Google o Microsoft',
  'devices.platform.apis.body':
    'Apunta a Sytue una integración escrita para Google Drive, Google Calendar, Google Contacts o Microsoft Graph, sin tocar su código.',
  'devices.platform.apis.note': 'Solo donde puedas cambiar la dirección base.',
  'devices.apis.step1.title': 'Cambia la dirección base',
  'devices.apis.step1.body':
    'En la herramienta, sustituye la dirección a la que llama por la del producto y el dialecto que necesites. El resto de su configuración se queda igual.',
  'devices.apis.step2.title': 'Usa tus credenciales de Sytue',
  'devices.apis.step2.body':
    'La autenticación es tu token de Sytue ID, no una cuenta de Google ni de Microsoft: los permisos son los mismos que ya tienes aquí.',
  'devices.apis.identityGoogle': 'Identidad · OAuth y directorio de Google',
  'devices.apis.step3.title': 'Una app de terceros, como si fuera Google',
  'devices.apis.step3.body':
    'Si la herramienta inicia sesión con OAuth de Google, la administración de la organización la registra en Sytue Admin con los permisos de Google que necesite. La app pedirá esos permisos y recibirá los datos de Sytue; Google no interviene ni ve nada.',
  'devices.apis.driveGoogle': 'Drive · API de Google Drive',
  'devices.apis.driveGraph': 'Drive · Microsoft Graph',
  'devices.apis.calendarGoogle': 'Calendar · API de Google Calendar',
  'devices.apis.calendarGraph': 'Calendar · Microsoft Graph',
  'devices.apis.contactsGoogle': 'Contacts · API People de Google',
  'devices.apis.contactsGraph': 'Contacts · Microsoft Graph',
  'devices.apis.expectTitle': 'Qué está cubierto',
  'devices.apis.expectBody':
    'Listar, leer, crear, editar y borrar. Lo que no está cubierto responde con un error claro, nunca con un éxito vacío.',
  'devices.copied.apiBase': 'Dirección base copiada',
  'devices.check.apis.label': 'La herramienta',
  'devices.check.apis.value': 'Lista tu contenido de Sytue como si fuera el servicio original.',
  'devices.platform.drive.title': 'Explorador de archivos (Drive)',
  'devices.platform.drive.body':
    'Ves tus archivos de Sytue Drive como una unidad más del ordenador.',
  'devices.platform.drive.note': 'Finder en Mac, Explorador de archivos en Windows.',

  'devices.mail.address': 'Tu dirección (usuario)',
  'devices.mail.imapHost': 'Servidor de entrada (IMAP)',
  'devices.mail.imapPort': 'Puerto de entrada',
  'devices.mail.smtpHost': 'Servidor de salida (SMTP)',
  'devices.mail.smtpPort': 'Puerto de salida',
  'devices.mail.hint':
    'Cifrado SSL/TLS en los dos, y la misma contraseña de Sytue con la que inicias sesión en la web.',
  'devices.dav.caldav': 'Calendario (CalDAV)',
  'devices.dav.carddav': 'Contactos (CardDAV)',
  'devices.dav.user': 'Usuario',

  'devices.apple.download': 'Descargar el perfil de Sytue',
  'devices.apple.step1.title': 'Descarga el perfil',
  'devices.apple.step1.body':
    'Pulsa el botón de aquí arriba. Se descarga un archivo pequeño con la configuración de tu cuenta.',
  'devices.apple.step2.iphone.title': 'Instálalo desde Ajustes',
  'devices.apple.step2.iphone.body':
    'Abre Ajustes. Justo arriba del todo aparece «Perfil descargado»: tócalo y pulsa Instalar.',
  'devices.apple.step2.mac.title': 'Instálalo desde Ajustes del Sistema',
  'devices.apple.step2.mac.body':
    'Abre Ajustes del Sistema › Privacidad y seguridad › Perfiles, haz doble clic en el perfil descargado y pulsa Instalar.',
  'devices.apple.step3.title': 'No tienes que escribir ninguna contraseña',
  'devices.apple.step3.body':
    'El perfil lleva su propia contraseña de aplicación, creada solo para este dispositivo. Puedes revocarla cuando quieras en Mi cuenta.',
  'devices.apple.step4.title': 'Listo',
  'devices.apple.step4.body':
    'El correo, el calendario y los contactos se añaden de una vez. No hay ningún servidor que escribir.',
  'devices.apple.manualTitle': 'Si prefieres hacerlo a mano',
  'devices.apple.manualBody': 'También puedes crear la cuenta manualmente con estos datos.',
  'devices.manualValues': 'Ver los valores manuales',

  'devices.desktop.step1.title': 'Añade una cuenta de correo',
  'devices.desktop.step1.outlook': 'En Outlook: Archivo › Agregar cuenta.',
  'devices.desktop.step1.thunderbird':
    'En Thunderbird: Cuentas › Configurar una cuenta › Correo electrónico.',
  'devices.desktop.step2.title': 'Escribe solo tu dirección y tu contraseña',
  'devices.desktop.step2.outlook':
    'Nada más. Outlook le pide la configuración a Sytue y la rellena por ti.',
  'devices.desktop.step2.thunderbird':
    'Nada más. Thunderbird le pide la configuración a Sytue y la rellena por ti.',
  'devices.desktop.step3.title': 'Confirma',
  'devices.desktop.step3.body':
    'Cuando te muestre los servidores, acepta. Se conecta como una cuenta IMAP.',
  'devices.desktop.emailLabel': 'Tu dirección de correo',
  'devices.desktop.notFoundTitle': '¿No lo ha encontrado solo?',
  'devices.desktop.notFoundBody': 'Elige la configuración manual y usa estos valores.',
  'devices.thunderbird.davTitle': 'Calendario y contactos',
  'devices.thunderbird.davBody': 'En Thunderbird se añaden aparte, con estas direcciones.',
  'devices.outlook.davTitle': 'Calendario y contactos en Outlook',
  'devices.outlook.davBody':
    'Outlook para Windows no admite CalDAV/CardDAV, así que el calendario y los contactos de Sytue no se sincronizan ahí. Úsalos desde la web de Sytue, o desde un cliente que sí hable CalDAV/CardDAV (Thunderbird, Calendario de Apple).',

  'devices.android.mailTitle': 'Correo',
  'devices.android.mailBody':
    'Con la aplicación de correo que ya usas (Gmail, Samsung Mail, K-9…).',
  'devices.android.mail1.title': 'Añade una cuenta',
  'devices.android.mail1.body': 'Elige «Otra» o «Personal (IMAP)», no «Google».',
  'devices.android.mail2.title': 'Escribe tu dirección y tu contraseña',
  'devices.android.mail2.body': 'Si te pide los servidores, cópialos de aquí abajo.',
  'devices.android.davTitle': 'Calendario y contactos',
  'devices.android.davBody':
    'Android no trae CalDAV/CardDAV: necesitas DAVx5, una aplicación gratuita que los añade al calendario y a la agenda del propio teléfono.',
  'devices.android.dav1.title': 'Instala DAVx5',
  'devices.android.dav1.body':
    'Está en Google Play y en F-Droid. Sytue ni la publica ni la mantiene.',
  'devices.android.dav2.title': 'Añade una cuenta con «Iniciar sesión con URL y nombre de usuario»',
  'devices.android.dav2.body':
    'Pega la dirección de abajo y usa tu correo y tu contraseña de Sytue.',
  'devices.android.dav3.title': 'Elige qué sincronizar',
  'devices.android.dav3.body': 'Marca tus calendarios y tu agenda.',

  'devices.drive.step1.title': 'Abre la conexión a un servidor',
  'devices.drive.step1.body':
    'En Mac: Finder › Ir › Conectarse al servidor (⌘K). En Windows: Explorador de archivos › Este equipo › Conectar a unidad de red, y marca «Conectar con otras credenciales».',
  'devices.drive.step2.title': 'Pega esta dirección',
  'devices.drive.step2.body': 'Es la carpeta con tus archivos de Sytue Drive.',
  'devices.drive.step3.title': 'Inicia sesión con tu cuenta de Sytue',
  'devices.drive.step3.body': 'Usuario: tu dirección de correo. Contraseña: la de Sytue.',
  'devices.drive.urlLabel': 'Dirección de Drive (WebDAV)',
  'devices.drive.expectTitle': 'Qué vas a encontrarte',
  'devices.drive.windowsExpectBody':
    'En Windows, Drive aparece en el Explorador de archivos como la carpeta «Sytue Drive». Los archivos se descargan al abrirlos; con clic derecho › «Mantener siempre en este dispositivo» los tienes sin conexión. Lo que guardes o crees ahí se sube a Drive.',
  'devices.drive.expectBody':
    'Los archivos se abren desde el servidor, no se copian al disco. Para trabajar sin conexión, descarga antes lo que necesites.',

  'devices.check.mail.label': 'El correo',
  'devices.check.mail.value': 'Llegan los mensajes nuevos y puedes enviarte uno a ti mismo.',
  'devices.check.dav.label': 'Calendario y contactos',
  'devices.check.dav.value':
    'Un evento creado en la web de Sytue aparece en el dispositivo en unos minutos.',
  'devices.check.profile.label': 'El perfil',
  'devices.check.profile.value': '«Sytue One» aparece en la lista de perfiles instalados.',
  'devices.check.drive.label': 'La unidad',
  'devices.check.drive.value': 'Ves tus carpetas de Drive y puedes abrir un archivo.',
  'devices.check.write.label': 'La escritura',
  'devices.check.write.value':
    'Creas una carpeta y también aparece en Sytue Drive desde el navegador.',

  'devices.step1.title': 'Qué usas',
  'devices.step1.heading': '¿Dónde quieres tu cuenta de Sytue?',
  'devices.step1.description': 'Elige uno. Si tienes varios, repite el asistente con cada uno.',
  'devices.step2.title': 'Cómo se hace',
  'devices.step2.heading': 'Configurar {platform}',
  'devices.step2.headingFallback': 'Cómo se hace',
  'devices.step2.description':
    'Sigue estos pasos. Todo lo que tengas que escribir se copia con un botón.',
  'devices.step2.next': 'Ya está',
  'devices.step3.title': 'Comprobación',
  'devices.step3.heading': 'Cómo saber que ha funcionado',
  'devices.step3.description': 'Si esto se cumple, está bien conectado.',
  'devices.step3.helpTitle': '¿Algo no cuadra?',
  'devices.step3.helpBody':
    'La contraseña suele ser la culpable: usa la misma con la que inicias sesión en Sytue desde el navegador. Si tienes verificación en dos pasos, puede que tu organización exija una contraseña de aplicación; pregunta a quien administre Sytue.',
  'devices.step3.again': 'Conectar otro dispositivo',

  // --- Alta guiada (onboarding) -----------------------------------------
  'ob.error.generic': 'No hemos podido completar la operación. Inténtalo de nuevo.',
  'ob.failedTitle': 'No hemos podido cargar tu alta',
  'ob.failedBody':
    'El servicio de identidad no responde ahora mismo. No se ha perdido nada: inténtalo de nuevo dentro de un minuto.',
  'ob.retry': 'Reintentar',
  'ob.preparing': 'Preparando tu alta…',
  'ob.lead':
    'Unos minutos y lo tienes en marcha. Vamos paso a paso; si lo dejas a medias, al volver seguimos donde estábamos.',
  'ob.saveFailedTitle': 'No se ha podido guardar',
  'ob.saveFailedBody': 'Inténtalo de nuevo en un momento.',
  'ob.copy': 'Copiar',
  'ob.copyFailTitle': 'No se ha podido copiar',
  'ob.copyFailBody': 'Selecciónalo con el ratón y cópialo a mano.',
  'ob.copied.recordType': 'Tipo de registro copiado',
  'ob.copied.recordName': 'Nombre del registro copiado',
  'ob.copied.recordValue': 'Valor del registro copiado',

  'ob.roleOption.member': 'Miembro — usa la suite, no administra nada',
  'ob.roleOption.admin': 'Administrador — puede gestionar toda la organización',
  'ob.roleOption.billing': 'Facturación — se encarga de las facturas',
  'ob.roleName.owner': 'Propietario',
  'ob.roleName.admin': 'Administrador',
  'ob.roleName.member': 'Miembro',
  'ob.roleName.billing': 'Facturación',

  'ob.mode.title': 'Empezar',
  'ob.mode.heading': '¿Para quién es Sytue?',
  'ob.mode.description': 'Esto nos dice qué preguntarte. Podrás cambiarlo más adelante.',
  'ob.mode.individual.title': 'Solo para mí',
  'ob.mode.individual.body':
    'Tu correo, tus archivos y tu calendario, con tu propia dirección. Si algún día necesitas invitar a alguien, podrás.',
  'ob.mode.org.title': 'Para mi organización',
  'ob.mode.org.body':
    'Das de alta a tu equipo, repartes quién puede hacer qué y lo administras todo desde un solo sitio.',
  'ob.mode.org.badge': 'Lo más habitual',

  'ob.identity.titleOrg': 'Tu organización',
  'ob.identity.titleMe': 'Tu cuenta',
  'ob.identity.headingOrg': '¿Cómo se llama tu organización?',
  'ob.identity.headingMe': '¿Cómo quieres que se llame tu cuenta?',
  'ob.identity.descriptionCreated':
    'Ya está creada. Puedes cambiar el nombre; la dirección se queda como está.',
  'ob.identity.description': 'Con esto creamos tu espacio y tu dirección en Sytue.',
  'ob.identity.nextCreated': 'Continuar',
  'ob.identity.nextOwnDomain': 'Crear y ver qué añadir a mi dominio',
  'ob.identity.next': 'Crear y continuar',
  'ob.identity.nameLabel': 'Nombre',
  'ob.identity.namePlaceholderOrg': 'ACME Europe',
  'ob.identity.namePlaceholderMe': 'Marta Ruiz',
  'ob.identity.nameHintOrg':
    'Es lo que ve tu equipo cuando inicia sesión. Puedes cambiarlo cuando quieras.',
  'ob.identity.nameHintMe': 'Solo lo ves tú. Puedes cambiarlo cuando quieras.',
  'ob.identity.slugLabel': 'Tu dirección en Sytue',
  'ob.identity.slugPlaceholder': 'acme',
  'ob.identity.slugCreated':
    'Tu dirección es {subdomain}. No se puede cambiar: de ella cuelgan tu correo y tus archivos.',
  'ob.identity.slugEmpty': 'Elige un nombre corto. Será tu dirección: acme.{suite}',
  'ob.identity.slugChecking': 'Comprobando si está libre…',
  'ob.identity.slugFree': 'Libre. Tu dirección será {subdomain}',
  'ob.identity.slugTaken': 'No se puede usar.',
  'ob.identity.firstTitle': 'Primero creamos tu espacio',
  'ob.identity.firstBody':
    'Cuando pulses el botón de abajo creamos tu dirección de Sytue y te enseñamos aquí mismo el registro exacto que tienes que añadir a tu dominio. No hace falta que lo tengas listo ahora.',

  'ob.domain.legend': '¿Y tu propio dominio?',
  'ob.domain.keep.title': 'Me quedo con {host}',
  'ob.domain.keep.placeholder': 'mi-nombre',
  'ob.domain.keep.body':
    'No tienes que tocar nada: publicarlo corre de nuestra cuenta. Es lo que recomendamos para empezar.',
  'ob.domain.own.title': 'Ya tengo un dominio y quiero usarlo (acme.com)',
  'ob.domain.own.body':
    'Si está en Cloudflare, te lo configuramos nosotros. Si no, te decimos el registro exacto que hay que añadir y lo comprobamos aquí mismo.',
  'ob.domain.search.title': 'Todavía no tengo dominio',
  'ob.domain.search.body':
    'Comprobamos si el nombre que quieres está libre y cuánto cuesta al año. Es solo una consulta: aquí no se compra nada.',
  'ob.domain.hostLabel': 'Tu dominio',
  'ob.domain.hostPlaceholder': 'acme.com',
  'ob.domain.hostHint': 'Escríbelo tal cual: acme.com, sin «https://» y sin «www».',
  'ob.domain.verified': '{host} verificado',

  'ob.dns.legend': '¿Dónde está {domain}?',
  'ob.dns.fallback': 'tu dominio',
  'ob.dns.cf.title': 'En Cloudflare — que lo configure Sytue',
  'ob.dns.cf.body':
    'Nos das permiso temporal en tu cuenta y creamos los registros nosotros. Antes de tocar nada te enseñamos qué vamos a cambiar.',
  'ob.dns.cf.badge': 'Sin tocar el DNS',
  'ob.dns.manual.title': 'Con otro proveedor — lo hago yo',
  'ob.dns.manual.body':
    'Te damos el registro exacto para copiarlo en tu panel de DNS y lo comprobamos aquí en cuanto lo tengas.',
  'ob.dns.manualSummary': 'Prefiero añadir los registros yo',

  'ob.people.title': 'Tu equipo',
  'ob.people.heading': '¿A quién invitas primero?',
  'ob.people.description':
    'Cada persona recibe un correo con un enlace personal para crear su contraseña e iniciar sesión. Puedes saltarte este paso y hacerlo más tarde.',
  'ob.people.next': 'Continuar',
  'ob.people.emailsLabel': 'Direcciones de correo',
  'ob.people.emailsPlaceholder': 'ana@acme.com, bruno@acme.com',
  'ob.people.emailsHint': 'Puedes pegar varias de una vez, separadas por comas o saltos de línea.',
  'ob.people.roleLabel': 'Con qué rol entran',
  'ob.people.roleHint': 'Se puede cambiar en cualquier momento desde Sytue Admin.',
  'ob.people.send': 'Enviar invitaciones',
  'ob.people.sendNote': 'No se envía nada hasta que pulses aquí.',
  'ob.people.pendingTitle': 'Invitaciones pendientes de aceptar',
  'ob.people.emptyTitle': 'Todavía no has invitado a nadie',
  'ob.people.emptyBody':
    'Puedes hacerlo ahora o más tarde, desde Sytue Admin. Tu organización funciona igual.',
  'ob.invite.headline': '{email} — {status}',
  'ob.invite.sent': 'Invitación enviada',
  'ob.invite.already': 'Ya está en tu organización',
  'ob.invite.invalid': 'Eso no parece una dirección de correo',
  'ob.invite.notSent': 'No se ha podido enviar el correo',
  'ob.invite.failed': 'No se les ha podido invitar',
  'ob.invite.default':
    'Recibirán un correo con un enlace personal para crear su contraseña e iniciar sesión.',

  'ob.billing.title': 'Facturación',
  'ob.billing.heading': '¿A nombre de quién van las facturas?',
  'ob.billing.description':
    'Añade los datos de factura y elige los puestos de pago. El acceso solo se activa cuando Mollie confirma el cobro.',
  'ob.billing.next': 'Continuar',
  'ob.billing.pay': 'Pagar de forma segura con Mollie',
  'ob.billing.verify': 'Verificar el pago',
  'ob.billing.legalLabel': 'Razón social',
  'ob.billing.legalPlaceholder': 'ACME Europe S.L.',
  'ob.billing.legalHint': 'El nombre legal de la empresa, tal y como aparece en tus facturas.',
  'ob.billing.taxLabel': 'Identificación fiscal',
  'ob.billing.taxPlaceholder': 'B12345678',
  'ob.billing.taxHint': 'El número fiscal o de IVA de tu país.',
  'ob.billing.addressLabel': 'Dirección',
  'ob.billing.addressPlaceholder': 'Calle Mayor 1, 28013 Madrid, España',
  'ob.billing.addressHint': 'El domicilio fiscal que va en la factura.',
  'ob.billing.emailLabel': 'Correo para las facturas',
  'ob.billing.emailPlaceholder': 'facturas@acme.com',
  'ob.billing.emailHint':
    'Ahí es donde enviamos las facturas. Por defecto, el correo de tu cuenta.',
  'ob.billing.note':
    'Solo te pedimos lo imprescindible para facturarte. Nada de esto se comparte con nadie.',
  'ob.billing.seatsLabel': 'Puestos de pago',
  'ob.billing.seatsHint': '{seats} puesto(s) × {price}/Usuario/Mes · total {total}/Mes.',
  'ob.billing.active': 'Suscripción activa',
  'ob.billing.activeBody': 'Mollie ha confirmado el pago. Ya tienes acceso a Sytue One.',
  'ob.billing.pending': 'Estamos verificando el pago',
  'ob.billing.pendingBody':
    'Mollie aún no ha confirmado el pago. Espera unos segundos y vuelve a verificar; no se te cobrará dos veces.',

  'ob.roles.title': 'Permisos',
  'ob.roles.heading': '¿Quieres repartir las tareas de administración?',
  'ob.roles.description':
    'Son roles ya hechos: eliges los que quieras y los creamos. Después se los asignas a quien tú digas desde Sytue Admin. Puedes saltarte este paso.',
  'ob.roles.next': 'Continuar',
  'ob.roles.already': 'Ya creado',
  'ob.roles.seePermissions': 'Ver los permisos exactos',
  'ob.roles.helpTitle': 'El control sigue siendo tuyo',
  'ob.roles.helpBody':
    'Tu cuenta es la propietaria: estos roles no le quitan nada. Puedes editarlos o borrarlos cuando quieras en Sytue Admin → Roles.',

  'ob.done.title': 'Listo',
  'ob.done.headingCompleted': 'Todo listo',
  'ob.done.heading': 'Esto es lo que has configurado',
  'ob.done.description': 'Viene de tu cuenta real, no de lo que has escrito aquí.',
  'ob.done.next': 'Entrar en Sytue',
  'ob.done.warnTitle': '{host} todavía no está verificado',
  'ob.done.warnBody':
    'Mientras tanto Sytue funciona igual, con {subdomain}. Cuando añadas el registro a tu dominio, vuelve al paso «{step}» y pulsa «Comprobar ahora».',
  'ob.done.nextTitle': 'Por dónde seguir',
  'ob.next.devices.title': 'Conecta tu móvil y tu ordenador',
  'ob.next.devices.body':
    'Deja funcionando el correo, el calendario y los contactos en las aplicaciones de siempre.',
  'ob.next.migrations.title': 'Trae tu contenido de Microsoft 365 o de Google',
  'ob.next.migrations.body':
    'Copiamos el correo, los archivos y los calendarios sin que pierdas nada por el camino.',
  'ob.next.apps.title': 'Conecta tus aplicaciones',
  'ob.next.apps.body': 'Las herramientas que ya usa tu equipo, enlazadas con tu cuenta de Sytue.',
  'ob.next.users.title': 'Sigue dando de alta a tu equipo',
  'ob.next.users.body': 'Invita a quien falte y revisa quién tiene cada rol.',

  'ob.sum.org': 'Organización',
  'ob.sum.account': 'Tu cuenta',
  'ob.sum.address': 'Dirección',
  'ob.sum.addressLive': '{host} · publicada',
  'ob.sum.addressPending': '{host} · pendiente de publicar',
  'ob.sum.ownDomain': 'Dominio propio',
  'ob.sum.domainVerified': '{host} · verificado',
  'ob.sum.domainPending': '{host} · esperando a que aparezca el registro',
  'ob.sum.noDomain': 'Sin dominio propio',
  'ob.sum.invitations': 'Invitaciones',
  'ob.sum.invitationsPending': {
    one: '1 pendiente de aceptar',
    other: '{count} pendientes de aceptar',
  },
  'ob.sum.noInvitations': 'Ninguna todavía',
  'ob.sum.billing': 'Facturación',
  'ob.sum.billingEmpty': 'Sin rellenar (todavía no hace falta)',
  'ob.sum.roles': 'Roles creados',
  'ob.sum.edit': 'Editar',
  'ob.sum.noRoles': 'Ninguno',

  'ob.sub.liveTitle': 'Tu dirección ya está publicada',
  'ob.sub.liveBody': '{host} resuelve en internet. No tienes que hacer nada más.',
  'ob.sub.failedTitle': 'Tu dirección todavía no está publicada',
  'ob.sub.failedDefault': 'No hemos podido publicarla en este momento.',
  'ob.sub.failedTail': 'Tu cuenta está creada: sigue con el alta y vuelve dentro de un rato.',
  'ob.sub.reservedTitle': 'Tu dirección está reservada',
  'ob.sub.reservedBody':
    '{host} es tuya. Publicarla en internet corre de nuestra cuenta: no tienes que hacer nada.',

  'ob.search.label': 'El dominio que te gustaría',
  'ob.search.placeholder': 'acme.com',
  'ob.search.hint': 'Escríbelo entero, con su extensión: acme.com, acme.es, acme.eu…',
  'ob.search.check': 'Comprobar disponibilidad',
  'ob.search.useOwn': 'Ya lo he registrado: conéctalo',
  'ob.search.note':
    'En Sytue no se venden dominios: aquí solo te decimos si está libre y cuánto cuesta. Regístralo donde prefieras y vuelve a este paso, a «Ya tengo un dominio», para conectarlo con tu cuenta.',
  'ob.avail.freeTitle': '{domain} está libre',
  'ob.avail.freePrice':
    'Cuesta {registration} el primer año y {renewal} en cada renovación. Es el precio de hoy en Cloudflare; donde lo registres puede ser otro.',
  'ob.avail.freeNoPrice': 'Se puede registrar. El registrador donde lo compres te dirá el precio.',
  'ob.avail.takenTitle': '{domain} ya está registrado',
  'ob.avail.takenBody':
    'Alguien lo tiene. Prueba con otro nombre, o con otra extensión (.es, .eu, .net…). Si el dominio es tuyo, elige arriba «Ya tengo un dominio» y lo conectamos.',
  'ob.avail.unsupportedTitle': 'No podemos consultar esa extensión',
  'ob.avail.unsupportedBody':
    'No podemos decirte si está libre ni cuánto cuesta: compruébalo en tu registrador. Cuando lo tengas, vuelve aquí y elige «Ya tengo un dominio».',
  'ob.avail.invalidTitle': 'Eso no parece un dominio',
  'ob.avail.invalidBody':
    'Escríbelo como acme.com: un nombre, un punto y la extensión. Sin «https://» y sin «www».',
  'ob.avail.unconfiguredTitle': 'Aquí no consultamos precios de dominios',
  'ob.avail.unconfiguredBody':
    'Búscalo en el registrador que prefieras. Cuando lo tengas, vuelve a este paso y elige «Ya tengo un dominio» para conectarlo.',
  'ob.avail.unknownTitle': 'No hemos podido consultarlo ahora mismo',
  'ob.avail.unknownBody':
    'No es culpa de lo que has escrito: no hemos recibido respuesta. Inténtalo de nuevo en un momento.',

  'ob.dnsRec.title': 'Añade este registro donde tengas registrado tu dominio',
  'ob.dnsRec.body':
    'Busca la sección «DNS» o «Registros» en tu proveedor, crea un registro TXT y copia estos dos valores. El procedimiento es el mismo en todas partes.',
  'ob.dnsRec.type': 'Tipo',
  'ob.dnsRec.name': 'Nombre',
  'ob.dnsRec.value': 'Valor',
  'ob.dnsRec.verifiedTitle': 'Dominio verificado',
  'ob.dnsRec.verifiedBody': 'Hemos encontrado el registro: {host} es tuyo y así queda registrado.',
  'ob.dnsRec.mismatchTitle': 'Hay un registro, pero no es este',
  'ob.dnsRec.mismatchBody':
    'En {name} hemos encontrado «{found}». Comprueba que copiaste el valor entero y que no se ha quedado ninguno antiguo.',
  'ob.dnsRec.unreachableTitle': 'No hemos podido consultar tu DNS',
  'ob.dnsRec.unreachableBody':
    'No es culpa de tu registro: el servidor de nombres no ha contestado. Inténtalo de nuevo en un momento.',
  'ob.dnsRec.pendingTitle': 'Todavía no vemos el registro',
  'ob.dnsRec.pendingBody':
    'Si acabas de crearlo, es normal: los cambios de DNS tardan entre unos minutos y unas horas en aparecer. Puedes seguir con el alta y volver a comprobarlo cuando quieras.',
  'ob.dnsRec.uncheckedTitle': 'Sin comprobar todavía',
  'ob.dnsRec.uncheckedBody':
    'Cuando hayas añadido el registro en tu proveedor, pulsa «Comprobar ahora» y miramos tu DNS al momento.',
  'ob.dnsRec.empty': 'Cuando pulses «Guardar y comprobar» te damos el registro exacto que añadir.',
  'ob.dnsRec.check': 'Comprobar ahora',
  'ob.dnsRec.save': 'Guardar y comprobar',

  'ob.cf.state.ok': 'Ya estaba',
  'ob.cf.state.created': 'Creado',
  'ob.cf.state.replaced': 'Sustituido',
  'ob.cf.state.missing': 'Lo añadimos',
  'ob.cf.state.conflict': 'Ya hay algo',
  'ob.cf.state.failed': 'No se ha podido',
  'ob.cf.state.pending': 'Todavía no',
  'ob.cf.permTitle': 'Danos permiso solo para tu DNS',
  'ob.cf.step1': 'Abre tus tokens de API de Cloudflare y pulsa «Create token».',
  'ob.cf.step1Link': 'Cloudflare → My profile → API tokens',
  'ob.cf.step2': 'Elige «Create custom token» y marca un único permiso: Zone · DNS · Edit.',
  'ob.cf.step3': 'En «Zone resources» elige {domain}, crea el token y pégalo aquí.',
  'ob.cf.tokenLabel': 'Token de Cloudflare',
  'ob.cf.tokenPlaceholder': 'El token que acabas de crear',
  'ob.cf.tokenHint':
    'Solo sirve para el DNS de este dominio. Lo usamos y lo descartamos: no se guarda salvo que nos lo pidas aquí abajo, y nunca sale de nuestros servidores.',
  'ob.cf.mailLabel': 'Recibir también en Sytue Mail el correo de @{domain}',
  'ob.cf.mailHintOn':
    'Esto cambia a dónde llega el correo de tu dominio. Si hoy lo recibes en otro sitio, te avisamos antes de sustituir nada.',
  'ob.cf.mailHintOff': 'Actívalo solo si quieres que tus buzones de @tu-dominio se muden a Sytue.',
  'ob.cf.rememberLabel': 'Recordar este permiso para futuros cambios',
  'ob.cf.rememberHint':
    'Si lo dejas sin marcar, tendrás que volver a pegarlo la próxima vez. Marcarlo lo guarda cifrado; desmarcarlo lo borra.',
  'ob.cf.preview': 'Ver qué vamos a cambiar',
  'ob.cf.previewAgain': 'Volver a mirar',
  'ob.cf.nothing': 'No queda nada por hacer',
  'ob.cf.apply': { one: 'Aplicar 1 cambio', other: 'Aplicar {count} cambios' },
  'ob.cf.zone': 'Zona encontrada',
  'ob.cf.token': 'Tu token',
  'ob.cf.tokenStored': 'Guardado cifrado para futuros cambios',
  'ob.cf.tokenDiscarded': 'Usado y descartado (no lo hemos guardado)',
  'ob.cf.domainState': 'Estado del dominio',
  'ob.cf.verified': 'Verificado',
  'ob.cf.pending': 'Pendiente de verificar',
  'ob.cf.nowThere': 'Ahora mismo hay: {current}',
  'ob.cf.replaceLabel': 'Sustituirlo por el de Sytue',
  'ob.cf.failTitle': 'Algo se ha quedado sin configurar',
  'ob.cf.failBody':
    'El resto está en su sitio. Aquí arriba tienes el motivo de cada registro que ha fallado y su valor, para añadirlo a mano desde Cloudflare.',
  'ob.cf.conflictTitle': 'Hay registros tuyos que no vamos a tocar',
  'ob.cf.conflictBody':
    'Nunca sustituimos nada que ya tengas salvo que lo marques. Si prefieres combinarlos a mano, ahí arriba tienes el valor actual y el nuestro.',
  'ob.cf.okTitle': 'Dominio verificado',
  'ob.cf.okBody':
    'Hemos visto el registro en tu propia zona: {host} es tuyo y así queda registrado.',
  'ob.cf.todoTitle': 'Todavía hay que aplicar los cambios',
  'ob.cf.todoBody':
    'Aún no hemos tocado tu zona. Pulsa «Aplicar» cuando lo de arriba te parezca bien.',
  'ob.cf.someFailed': {
    one: '1 registro se ha quedado sin configurar',
    other: '{count} registros se han quedado sin configurar',
  },
  'ob.cf.someFailedBody': 'Abajo tienes el motivo de cada uno y los valores para hacerlo a mano.',
  'ob.cf.configured': '{host} configurado y verificado',
  'ob.cf.created': 'Registros creados en tu zona',
  'today.title': 'Hoy',
  'today.openCalendar': 'Ver el calendario',
  'today.noEvents': 'Nada en la agenda para hoy.',
  'today.unavailable': 'La agenda no está disponible ahora mismo.',
  'today.allDay': 'Todo el día',
  'today.untitled': '(sin título)',
  'today.unread': { one: '1 correo sin leer', other: '{count} correos sin leer' },
  'today.inboxZero': 'Bandeja de entrada al día',
  'launcher.quickActions': 'Accesos rápidos',
  'launcher.recentAll': 'Reciente en toda la suite',
  'launcher.activityError': 'No pudimos cargar tu actividad',
  'launcher.noActivity': 'Sin actividad reciente',
  'ob.dns.verified': 'Dominio verificado',
  'ob.dns.mismatch': 'Hay un registro, pero no es este',
  'ob.dns.unreachable': 'No hemos podido consultar tu DNS',
  'ob.dns.notYet': 'Todavía no vemos el registro',
  'ob.dns.unchecked': 'Sin comprobar todavía',

  // --- Onboarding: rótulos que estaban en duro ---
  'ob.dns.type': 'Tipo',
  'ob.dns.name': 'Nombre',
  'ob.dns.value': 'Valor',
  'ob.cf.tokenField': 'Token de Cloudflare',
  'ob.cf.remember': 'Recordar este permiso para futuros cambios',
  'ob.cf.replaceWithSytue': 'Sustituirlo por el de Sytue',
  'recent.hubDown':
    'El Hub no responde ahora mismo. Tu trabajo sigue a salvo; vuelve a intentarlo.',
  'search.partial':
    '{sources} no ha respondido, así que pueden faltar resultados. Vuelve a intentarlo en un momento.',
  'search.partialJoin': ' y ',
  'search.showing': 'Se muestran {count} de {total} resultados.',
  'privacy.notice':
    'No vendemos tus datos ni los cedemos a terceros. No los usamos para perfilarte ni para publicidad, ni para entrenar o mejorar nuestros servicios o los de nadie. Van cifrados en tránsito y en reposo, y solo salen de nuestra infraestructura europea en lo imprescindible para cobrarte y en las integraciones que tú actives, en las que el tercero te pedirá aprobación sobre qué datos usará.',
};
