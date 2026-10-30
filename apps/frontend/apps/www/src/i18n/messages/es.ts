/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo ESPAÑOL del sitio público. Traducción del inglés (`en.ts`), que es la
 * fuente de verdad: el tipo se deriva de él y una clave de más o de menos rompe
 * la compilación (docs/CONVENTIONS.md §13.1).
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const es: AppCatalog<typeof en> = {
  // --- Cromo del sitio ---------------------------------------------------
  'skip.content': 'Saltar al contenido',
  'nav.products': 'Productos',
  'nav.sovereignty': 'Soberanía',
  'nav.standards': 'Estándares abiertos',
  'nav.pricing': 'Precios',
  'footer.pricing': 'Precios',
  'home.pricing.title': 'Tres planes, toda la suite',
  'home.pricing.body':
    'Los mismos productos en los tres: lo que cambia es el espacio por usuario. Sin ediciones recortadas.',
  'home.pricing.amount': '5 €',
  'home.pricing.tax': 'sin IVA',
  'home.pricing.per': '/Usuario/Mes',
  'home.pricing.cta': 'Hablar con ventas',
  'home.pricing.commitment.title': 'Compromiso de por vida',
  'home.pricing.commitment.body':
    'El precio por usuario sube 0,50 € cada cuatro años, y nada más: de por vida. Lo sabes el día que entras y no cambia.',
  'home.pricing.included.title': 'Una sola edición',
  'home.pricing.included.body':
    'No hay planes por función, ni niveles, ni versiones recortadas: la suite completa entra en el precio por usuario, con las mismas funciones para todo el mundo.',
  'home.pricing.metered.title': 'Lo que se cobra aparte',
  'home.pricing.metered.body': 'El almacenamiento adicional: por uso o en bloques.',
  'home.pricing.storage.title': 'El almacenamiento se comparte',
  'home.pricing.storage.body':
    'La cuota de cada persona se suma a la de su organización: lo que una no gasta lo aprovechan las demás. Y la cuota por usuario se puede ajustar desde la organización.',
  'nav.downloads': 'Descargas',
  'nav.aria': 'Navegación del sitio',
  'nav.ariaMobile': 'Navegación móvil',
  'nav.home': 'Sytue One — Inicio',
  'nav.openMenu': 'Abrir el menú',
  'nav.closeMenu': 'Cerrar el menú',
  'theme.toLight': 'Cambiar a modo claro',
  'theme.toDark': 'Cambiar a modo oscuro',
  'lang.label': 'Idioma',
  'cta.start': 'Empezar gratis',
  'cta.signIn': 'Acceder',
  'cta.seeSuite': 'Ver la suite',

  // --- Portada -----------------------------------------------------------
  'meta.home.title': 'Sytue One — La suite de productividad soberana europea',
  'meta.home.description':
    'Sytue One: la suite de productividad soberana europea. Identidad, correo, archivos, documentos, calendario y chat, 100 % en la UE y sobre estándares abiertos.',
  'home.hero.lead': 'Tu productividad,',
  'home.hero.accent': 'soberana y europea',
  'home.hero.subtitle':
    'Correo, archivos, documentos, calendario, chat y videollamadas, integrados como una sola aplicación y alojados 100 % en la UE.',
  'home.hero.note': 'Sin tarjeta. Datos en Scaleway (UE). Migración asistida.',
  'home.privacy.title': 'Tu trabajo es tuyo. Tu privacidad también.',
  'home.privacy.body': 'No vendemos tus datos ni los usamos para crear perfiles publicitarios.',
  'home.privacy.anonymity':
    'Sytue Analytics protege tu privacidad con estadísticas agregadas, sin cookies de seguimiento y con identificadores de visita seudónimos que cambian a diario y están separados por sitio.',
  'home.privacy.account':
    'Para usar la suite necesitamos datos de cuenta y registros de acceso que nos permitan prestar el servicio y mantenerlo seguro.',
  'home.privacy.link': 'Cómo protegemos tu privacidad',

  'home.stats.aria': 'Huella europea',
  'home.stats.regions': 'Regiones UE (fr-par · nl-ams · on-premise Madrid)',
  'home.stats.apps': 'Aplicaciones con una sola identidad',
  'home.stats.standards': 'Estándares abiertos: OIDC, JMAP, CalDAV, CardDAV y WebDAV',

  'home.products.title': 'Una suite, una experiencia',
  'home.products.subtitle':
    'Cada producto comparte diseño, identidad y shell. Se ven y se comportan como una sola aplicación.',

  'home.integration.title': 'La integración que se le supone a una suite',
  'home.integration.subtitle':
    'Un alta da acceso a todo. Aprovisionamiento automático (SCIM) desde Sytue ID.',
  'home.integration.sso.title': 'Inicio de sesión único',
  'home.integration.sso.body': 'Una identidad (Sytue ID) para toda la suite. SSO + MFA/WebAuthn.',
  'home.integration.shell.title': 'Shell común',
  'home.integration.shell.body':
    'Topbar, conmutador de apps, búsqueda y notificaciones globales en cada producto.',
  'home.integration.cross.title': 'Acciones cruzadas',
  'home.integration.cross.body':
    'Adjunta desde Drive en Mail, convierte un correo en un evento de Calendar y compártelo en Rooms sin salir del flujo.',

  'home.sovereignty.title': 'Soberanía, de verdad',
  'home.sovereignty.body':
    'No es una promesa de marketing: es la arquitectura. La infraestructura vive solo en regiones europeas y todo se construye sobre protocolos abiertos y auditables, sin puertas traseras.',
  'home.sovereignty.link': 'Por qué estándares abiertos',
  'home.sovereignty.eu.title': '100 % en la UE',
  'home.sovereignty.eu.body':
    'Toda la infraestructura en la UE: Scaleway (fr-par / nl-ams) y una zona on-premise en Madrid. Sin proveedores estadounidenses para datos personales.',
  'home.sovereignty.gdpr.title': 'Cumplimiento RGPD',
  'home.sovereignty.gdpr.body':
    'DPA, registro de actividades, runbook de derechos (DSR) y retención. Cifrado en tránsito (TLS 1.3) y en reposo.',
  'home.sovereignty.lockin.title': 'Sin dependencia',
  'home.sovereignty.lockin.body':
    'Construido sobre estándares abiertos: OIDC, JMAP, CalDAV, CardDAV y WebDAV. Tus datos, siempre portables.',

  'home.standards.overline': 'Obra original de Sytue',
  'home.standards.title': 'Construido sobre estándares abiertos',
  'home.standards.body':
    'Cada producto de Sytue es obra original, escrita en Go y Astro + Preact sobre protocolos abiertos (OIDC, JMAP, CalDAV, CardDAV, WebDAV). Sin motores de terceros: tus datos son portables y tu suite, soberana.',

  'home.cta.title': 'Recupera el control de tu trabajo',
  'home.cta.body':
    'Empieza con tu organización hoy. Migración asistida desde Google Workspace o Microsoft 365.',
  'home.cta.primary': 'Crear mi organización',
  'home.cta.secondary': 'Hablar con ventas',

  // --- Rejilla de productos ----------------------------------------------
  'grid.discover': 'Descubrir',
  'product.mail.blurb': 'Correo privado y europeo sobre JMAP, sin rastreadores ni publicidad.',
  'product.drive.blurb': 'Archivos y colaboración, con la marca de tu organización.',
  'product.docs.blurb': 'Documentos, hojas y presentaciones con edición colaborativa en directo.',
  'product.calendar.blurb': 'Calendario CalDAV integrado con el resto de la suite.',
  'product.contacts.blurb': 'Contactos CardDAV, compartidos en tu organización.',
  'product.notes.blurb':
    'Cuadernos que guardan cualquier cosa: texto, ficheros, eventos, documentos enteros.',
  'product.photos.blurb':
    'Galería y editor de fotos: tus originales en Drive, tus ajustes como receta.',
  'product.rooms.blurb': 'Canales, mensajes y videollamadas de equipo en un solo sitio.',
  'product.cast.blurb': 'Vídeo bajo demanda y directo: sube, emite y comparte, en la UE.',
  'product.pages.blurb':
    'Webs y tiendas de tu empresa, diseñadas a tu medida y publicadas en la UE.',
  'product.bases.blurb':
    'Tablas relacionales colaborativas para clientes, pedidos o existencias, en la UE.',
  'product.cloud.blurb': 'Plataforma de despliegue y datos gestionados, en la UE.',
  'product.id.blurb': 'Una identidad única (SSO + MFA) con Sytue ID.',
  'product.one.blurb':
    'Tu portada de la suite: aplicaciones, accesos rápidos, actividad reciente y avisos.',
  'product.sheets.blurb':
    'Hojas de cálculo con fórmulas, datos externos y gráficos, editadas a varias manos.',
  'product.slides.blurb': 'Presentaciones con tu marca, temas y vista del moderador.',
  'product.pdf.blurb': 'Lee, anota, firma y ordena tus PDF sin que salgan de Europa.',
  'product.console.blurb':
    'Administración de la organización: personas, acceso, cumplimiento y auditoría.',
  'product.erp.blurb':
    'Veinticuatro módulos y un libro por partida doble que la base de datos no deja descuadrar.',

  // --- Fichas de producto -------------------------------------------------
  'productPage.back': 'Toda la suite',
  'productPage.features': 'Todo lo que hace {name}',
  'productPage.featuresPlanned': 'Lo que hará {name}',
  'productPage.builtOn': 'Construido sobre {standard}',
  'productPage.fromScratch':
    'El editor de Docus, Calcus, Diapos y CertiPDF está escrito desde cero: motor propio de Sytue, no una integración de terceros.',
  'productPage.shot': 'La pantalla principal de {name}, tal cual se ve al entrar.',
  'productPage.shotFeatures': '{name} en marcha, con sus funciones a la vista.',
  'productPage.open': 'Abrir {name}',
  'productPage.enDesarrollo': 'Este producto está en desarrollo: todavía no está disponible.',
  'productPage.included':
    'Incluido en Sytue One: una sola identidad (Sytue ID) y un alta dan acceso a todos los productos. Datos 100 % en la UE (Scaleway) y portables sobre estándares abiertos.',

  'product.mail.tagline': 'Correo privado y europeo, sin rastreadores ni publicidad.',
  'product.mail.standard': 'JMAP (estándar abierto)',
  'product.mail.p1': 'Bandeja rápida con hilos, etiquetas, filtros y búsqueda.',
  'product.mail.p2': 'Adjunta archivos directamente desde Sytue Drive.',
  'product.mail.p3': 'Convierte un correo en un evento de Calendar.',
  'product.mail.p4':
    'Cifrado en tránsito (TLS 1.3). La infraestructura de Sytue está en la UE. Los servicios externos siguen la ubicación de su proveedor.',

  'product.drive.tagline': 'Tus archivos y tu colaboración, con la marca de tu organización.',
  'product.drive.standard': 'WebDAV (estándar abierto)',
  'product.drive.p1': 'Sube, organiza y comparte con enlaces controlados (permiso y caducidad).',
  'product.drive.p2': 'Vista previa en línea de documentos e imágenes.',
  'product.drive.p3': 'Adjunta a Mail sin duplicar el archivo.',
  'product.drive.p4': 'Almacenamiento respaldado por S3 en regiones europeas.',

  'product.docs.tagline':
    'Documentos, hojas y presentaciones con edición colaborativa en tiempo real.',
  'product.docs.standard': 'Edición colaborativa en tiempo real',
  'product.docs.p1': 'Escribe a varias manos, con presencia y cambios al instante.',
  'product.docs.p2': 'Hojas de cálculo con fórmulas y formato condicional.',
  'product.docs.p3': 'Presentaciones con la identidad de tu marca.',
  'product.docs.p4': 'Todo guardado en tu Drive europeo.',

  'product.calendar.tagline': 'Tu calendario, integrado con el resto de la suite.',
  'product.calendar.standard': 'CalDAV (estándar abierto)',
  'product.calendar.p1': 'Vistas de día, semana y mes.',
  'product.calendar.p2': 'Eventos recurrentes y recordatorios.',
  'product.calendar.p3': 'Crea eventos desde un correo de Mail.',
  'product.calendar.p4': 'Compatible con cualquier cliente CalDAV.',

  'product.contacts.tagline': 'Los contactos de tu organización, compartidos y portables.',
  'product.contacts.standard': 'CardDAV (estándar abierto)',
  'product.contacts.p1': 'Libreta compartida en tu organización.',
  'product.contacts.p2': 'Importa y exporta en vCard.',
  'product.contacts.p3': 'Fusiona duplicados sin perder datos.',
  'product.contacts.p4': 'Compatible con cualquier cliente CardDAV.',
  'product.notes.tagline': 'Todo lo que quieres conservar, en un sitio y en tus manos.',
  'product.notes.standard': 'Modelo propio sobre estándares abiertos (API REST propia)',
  'product.notes.p1': 'Cuadernos anidados, en lienzo libre, en columnas o en lista.',
  'product.notes.p2':
    'Notas de cualquier tipo: texto, tareas, enlaces, ficheros, eventos de Calendar, documentos de Docs.',
  'product.notes.p3': 'Cifrado extremo a extremo: la contraseña no sale de tu navegador.',
  'product.notes.p4': 'Tú decides qué entra en la búsqueda, y qué no entra nunca.',

  'product.rooms.tagline': 'Canales, mensajes y videollamadas de equipo, unificados.',
  'product.rooms.standard': 'Mensajería y llamadas en tiempo real',
  'product.rooms.p1': 'Canales por equipo o proyecto.',
  'product.rooms.p2': 'Videollamada embebida en cada canal.',
  'product.rooms.p3': 'Menciones y notificaciones unificadas en la suite.',
  'product.rooms.p4': 'Historial buscable desde toda Sytue One.',

  'product.cloud.p4': 'Dominios propios con TLS automático.',

  'product.cloud.p3': 'Almacenamiento S3 y funciones gestionadas.',
  'product.cloud.p2': 'Base de datos Postgres con API REST automática.',
  'product.cloud.p1': 'Preview por cada PR y rollback instantáneo.',
  'product.cloud.standard': 'PaaS Kubernetes-native',
  'product.cloud.tagline': 'Tu código desplegado y tus datos gestionados, en la Unión Europea.',
  'product.id.tagline': 'Una identidad para toda la suite. SSO + MFA.',
  'product.id.standard': 'OpenID Connect / OAuth 2.0 (estándar abierto)',
  'product.id.p1': 'Inicio de sesión único en todos los productos.',
  'product.id.p2': 'MFA y llaves de acceso (WebAuthn).',
  'product.id.p3': 'Aprovisionamiento automático (SCIM) desde tu directorio.',
  'product.id.p4': 'Perfil y seguridad en un solo sitio.',
  'product.one.tagline': 'Todo Sytue One a un clic, desde la misma puerta.',
  'product.one.standard': 'Un solo inicio de sesión (Sytue ID) para toda la suite',
  'product.one.p1': 'Lanzador con todas tus aplicaciones y las que tu organización conecta.',
  'product.one.p2': 'Búsqueda en toda la suite (⌘K) desde cualquier pantalla.',
  'product.one.p3':
    'Tu agenda de hoy y el correo sin leer en la portada, junto a lo reciente en toda la suite y las notificaciones.',
  'product.one.p4': 'Conecta tus dispositivos y descarga las aplicaciones nativas.',
  'product.sheets.tagline': 'Números que se entienden, en una hoja que trabaja contigo.',
  'product.sheets.standard': 'Formato ODF/OOXML y edición colaborativa en tiempo real',
  'product.sheets.p1': 'Fórmulas, formato condicional y validación de datos.',
  'product.sheets.p2': 'Trae datos de fuera y consúltalos sin salir de la hoja.',
  'product.sheets.p3': 'Gráficos, minigráficos, segmentaciones y tablas de datos.',
  'product.sheets.p4': 'Exporta a ODS y XLSX; abre ficheros de Excel.',
  'product.slides.tagline': 'Cuenta la historia; la diapositiva acompaña.',
  'product.slides.standard': 'Formato ODF/OOXML y edición colaborativa en tiempo real',
  'product.slides.p1': 'Temas de presentación con la identidad de tu organización.',
  'product.slides.p2': 'Vista del moderador con notas y siguiente diapositiva.',
  'product.slides.p3': 'Imágenes, formas, gráficos y SmartArt.',
  'product.slides.p4': 'Exporta a ODP, PPTX y PDF; abre ficheros de PowerPoint.',
  'product.pdf.tagline': 'El PDF, de principio a fin, en tu navegador.',
  'product.pdf.standard': 'PDF (ISO 32000) con anotaciones y formularios estándar',
  'product.pdf.p1': 'Resaltar, subrayar, comentar y dibujar sobre el documento.',
  'product.pdf.p2': 'Formularios: rellena campos o crea los tuyos.',
  'product.pdf.p3': 'Combinar, extraer, girar, numerar y poner marcas de agua.',
  'product.pdf.p4': 'Panel de firmas, censura definitiva y documentos protegidos con contraseña.',
  'product.console.tagline': 'Gobierna la suite sin salir de una pantalla.',
  'product.console.standard': 'SCIM, SAML y OpenID Connect (estándares abiertos)',
  'product.console.p1': 'Personas, grupos (también dinámicos) y roles.',
  'product.console.p2': 'SSO con tu proveedor y aprovisionamiento SCIM.',
  'product.console.p3':
    'Reglas de acceso condicional, dispositivos y acceso privilegiado con aprobación.',
  'product.console.p4': 'Auditoría exportable a tu SIEM y retenciones legales.',
  'product.erp.tagline': 'El ERP entero, y el catálogo de modelos es un fichero de datos.',
  'product.erp.standard': 'Partida doble, catálogo de modelos en JSON y una sola API REST',
  'product.erp.p1':
    'Veinticuatro módulos y treinta y seis modelos: contactos, ventas, compras, inventario, fabricación, facturación, contabilidad, proyectos y RRHH.',
  'product.erp.p2':
    'Añadir un módulo es escribir un JSON: sin migración, sin ruta nueva y sin tocar la interfaz.',
  'product.erp.p3':
    'El dinero va en céntimos enteros, nunca en coma flotante, y el cuadre lo impone la base de datos.',
  'product.erp.p4':
    'Lo asentado no se toca: se corrige con otro asiento, que es lo que separa un libro de una hoja de cálculo.',
  'product.one.f1':
    'Lanzador con las aplicaciones de la suite y las de terceros que tu organización conecta.',
  'product.one.f2':
    'Accesos rápidos: escribir un correo, nuevo documento, subir a Drive, crear un evento.',
  'product.one.f3':
    'Reciente en toda la suite: lo último que tocaste en Drive, Docus, Mail o Rooms.',
  'product.one.f4': 'Búsqueda global con ⌘K o Ctrl K, con resultados de todas las aplicaciones.',
  'product.one.f5': 'Centro de notificaciones y presencia compartidos por toda la suite.',
  'product.one.f6':
    'Conecta tus dispositivos: perfil para iPhone y Mac, Android y escritorio, en dos pasos.',
  'product.one.f7': 'Descargas de las aplicaciones nativas de escritorio y móvil.',
  'product.one.f8': 'Asistente de bienvenida que deja la organización lista sin manuales.',
  'product.mail.f1': 'Hilos de conversación con lectura, respuesta y reenvío en un clic.',
  'product.mail.f2':
    'Carpetas, etiquetas de color y búsqueda con operadores (from:, has:attachment, is:unread…).',
  'product.mail.f3': 'Reglas automáticas que archivan, etiquetan o mueven el correo al llegar.',
  'product.mail.f4': 'Envío programado y «Deshacer envío».',
  'product.mail.f5': 'Firmas, alias de dirección y respuesta automática de vacaciones.',
  'product.mail.f6': 'Buzones compartidos y delegados con permisos de lectura y envío.',
  'product.mail.f7':
    'Redactor con formato enriquecido y adjuntos desde Sytue Drive sin duplicar ficheros.',
  'product.mail.f8': 'Contenido remoto bloqueado por defecto: nadie sabe cuándo abres un correo.',
  'product.mail.f9': 'Correo firmado (PGP y S/MIME) reconocido y señalado; TLS en tránsito.',
  'product.mail.f10':
    'Acceso también desde Thunderbird, Outlook, Apple Mail o el móvil, con contraseñas de aplicación.',
  'product.mail.f11':
    'Direcciones en fichas con la libreta de Contacts a un clic, y aviso si dices «adjunto» sin adjuntar.',
  'product.mail.f12':
    'Reglas desde el propio correo, vaciar la papelera de un golpe y atajos de teclado con «?».',
  'product.drive.f1':
    'Mi unidad, unidades de equipo, compartido conmigo, recientes, favoritos y papelera.',
  'product.drive.f2': 'Sube arrastrando; carpetas, mover, copiar, renombrar y selección múltiple.',
  'product.drive.f3':
    'Enlaces de compartición con dos niveles (mirar o editar), contraseña y caducidad.',
  'product.drive.f4': 'Historial de versiones con restauración.',
  'product.drive.f5': 'Vista previa de documentos, imágenes y PDF sin descargar.',
  'product.drive.f6':
    'Abre y edita documentos de oficina en Docus, Calcus y Diapos; los PDF, en CertiPDF.',
  'product.drive.f7': 'Etiquetas de confidencialidad que impiden enlaces públicos donde no toca.',
  'product.drive.f8': 'Comparte a un canal de Rooms o adjunta a Mail sin duplicar el archivo.',
  'product.drive.f9':
    'Acceso WebDAV y cliente de escritorio para ver tus archivos en el ordenador.',
  'product.drive.f10': 'Almacenamiento S3 en regiones europeas, con la cuota siempre visible.',
  'product.docs.f1': 'Edición colaborativa en tiempo real, con presencia y sin pisarse.',
  'product.docs.f2':
    'Cinta por pestañas: Inicio, Insertar, Referencias, Correspondencia y Revisión.',
  'product.docs.f3': 'Tablas, imágenes, formas, SmartArt, gráficos, símbolos y marcas de agua.',
  'product.docs.f4': 'Referencias: índice, notas al pie, citas y bibliografía.',
  'product.docs.f5': 'Correspondencia: combinación de datos para cartas y etiquetas.',
  'product.docs.f6': 'Comentarios en hilo, revisión de cambios y versiones.',
  'product.docs.f7':
    'Asistente de IA opcional para reescribir, traducir o resumir, solo si tu organización lo activa.',
  'product.docs.f8': 'Exporta a ODT, DOCX y PDF; abre ficheros de Word desde Drive.',
  'product.docs.f9': 'Encabezados, pies de página, numeración y bloques reutilizables.',
  'product.docs.f10': 'Guardado automático en tu Drive europeo, con historial.',
  'product.docs.f11':
    'Descarga desde la portada en PDF, HTML, Markdown, OpenDocument o Microsoft Office.',
  'product.sheets.f1': 'Fórmulas con ayuda contextual y recálculo instantáneo.',
  'product.sheets.f2': 'Formato condicional, validación de datos y formatos numéricos por región.',
  'product.sheets.f3': 'Gráficos de varios tipos, minigráficos y segmentaciones.',
  'product.sheets.f4': 'Tablas de datos con totales y ordenación; tabla dinámica.',
  'product.sheets.f5': 'Datos externos: conecta una fuente y consúltala desde la hoja.',
  'product.sheets.f6': 'Solver para encontrar el valor que cumple un objetivo.',
  'product.sheets.f7': 'Macros para automatizar tareas repetidas.',
  'product.sheets.f8': 'Colaboración en tiempo real con presencia y comentarios.',
  'product.sheets.f9': 'Filtros, segmentaciones y búsqueda dentro de la hoja.',
  'product.sheets.f10': 'Exporta a ODS, XLSX y PDF; abre ficheros de Excel desde Drive.',
  'product.slides.f1': 'Temas y paletas coherentes con la marca de tu organización.',
  'product.slides.f2': 'Lienzo con formas, imágenes, gráficos, SmartArt y conectores.',
  'product.slides.f3': 'Presenta a pantalla completa con vista del moderador y notas.',
  'product.slides.f4': 'Colaboración en tiempo real con presencia y comentarios.',
  'product.slides.f5': 'Diapositivas con fondo, pósteres y disposiciones.',
  'product.slides.f6': 'Exporta a ODP, PPTX y PDF.',
  'product.slides.f7': 'Abre ficheros de PowerPoint desde Drive.',
  'product.slides.f8': 'Guardado automático con versiones.',
  'product.pdf.f1': 'Visor rápido con índice, miniaturas, búsqueda y zoom a página o a anchura.',
  'product.pdf.f2':
    'Anotaciones: resaltar, subrayar, tachar, comentar, dibujar, formas, flechas y sellos.',
  'product.pdf.f3': 'Cuadros de texto, medidas y recortes de zona.',
  'product.pdf.f4': 'Formularios: rellena los campos existentes o añade los tuyos.',
  'product.pdf.f5': 'Panel de firmas del documento.',
  'product.pdf.f6': 'Censura definitiva: lo marcado desaparece del fichero, no solo de la vista.',
  'product.pdf.f7':
    'Páginas: combinar, extraer, eliminar, girar, numerar (también Bates), encabezados y pies.',
  'product.pdf.f8': 'Marca de agua, fondo y metadatos editables.',
  'product.pdf.f9': 'Documentos protegidos con contraseña y permisos.',
  'product.pdf.f10': 'Lectura en voz alta y comentarios en hilo.',
  'product.calendar.f1': 'Vistas de mes, semana, día y agenda, con minicalendario y «Hoy».',
  'product.calendar.f2':
    'Repetición diaria, semanal o mensual con fin; edita solo este, este y los siguientes, o todos.',
  'product.calendar.f3': 'Recordatorios por evento y notificaciones dentro de la suite.',
  'product.calendar.f4': 'Invitaciones con respuesta (asistiré, quizá, no) y seguimiento.',
  'product.calendar.f5': '«Buscar un hueco» libre para todas las personas invitadas.',
  'product.calendar.f6': 'Salas y recursos reservables con disponibilidad.',
  'product.calendar.f7': 'Calendarios compartidos con permisos, y los que otros comparten contigo.',
  'product.calendar.f8': 'Suscripción a calendarios externos (ICS) y varios calendarios con color.',
  'product.calendar.f9': 'Crea un evento desde un correo de Mail o desde una nota.',
  'product.calendar.f10':
    'Compatible con cualquier cliente CalDAV: Apple Calendar, Thunderbird y más.',
  'product.calendar.f11':
    'Duplica un evento o cópialo a tu calendario, y descárgalo como .ics para quien no esté en Sytue.',
  'product.contacts.f1': 'Libretas de direcciones, incluida la compartida de tu organización.',
  'product.contacts.f2':
    'Ficha completa: teléfonos, correos, direcciones, organización, cumpleaños, notas y foto.',
  'product.contacts.f3': 'Etiquetas y favoritos para agrupar y encontrar.',
  'product.contacts.f4': 'Detección y fusión de duplicados sin perder datos.',
  'product.contacts.f5': 'Importa y exporta vCard y CSV; exporta un contacto suelto.',
  'product.contacts.f6': 'Acciones en lote y papelera con recuperación.',
  'product.contacts.f7': 'Búsqueda instantánea y autocompletado al escribir en Mail.',
  'product.contacts.f8': 'Compatible con cualquier cliente CardDAV.',
  'product.contacts.f9': 'Próximos cumpleaños de tu libreta, con aviso de los de este mes.',
  'product.contacts.f10':
    'Crea una reunión con un contacto o copia su correo sin salir de su ficha.',
  'product.notes.f1':
    'Cuadernos anidados con icono y color; tres disposiciones: libre, columnas y lista.',
  'product.notes.f2':
    'Doce tipos de nota: texto, tareas, enlaces, color, imagen y referencias a Drive, Docus, Calendar, Rooms y otras notas.',
  'product.notes.f3': 'Editor con formato (negrita, listas, enlaces) que guarda al salir.',
  'product.notes.f4':
    'Etiquetas, fijar, duplicar, mover entre cuadernos, agrupar y arrastrar en el lienzo.',
  'product.notes.f5': 'Visibilidad por cuaderno: privado, personal, organización o enlace público.',
  'product.notes.f6': 'Cifrado en tu navegador para cuadernos sensibles: Sytue no puede leerlos.',
  'product.notes.f7': 'Tú decides qué cuadernos entran en la búsqueda de la suite.',
  'product.notes.f8': 'Retroenlaces: ve qué notas apuntan a esta.',
  'product.notes.f9': 'Exporta un cuaderno entero a HTML, PDF, Markdown y más; importa copias.',
  'product.notes.f10': 'Comparte una nota a un canal de Rooms; suelta o pega para crear notas.',
  'product.notes.f11':
    'Tablas en el editor y atajos de Markdown al escribir: «- », «# », «> » y «```».',
  'product.notes.f12':
    'Mueve una nota a otro cuaderno, exporta una sola nota y ordena el cuaderno como quieras.',
  'product.photos.tagline': 'Tus fotos, ordenadas y a tu gusto, sin salir de Europa.',
  'product.photos.standard': 'Originales en Sytue Drive; motor de edición en tu navegador',
  'product.photos.p1': 'Línea de tiempo por meses con búsqueda por nombre.',
  'product.photos.p2': 'Álbumes con portada; visor a pantalla completa.',
  'product.photos.p3': 'Editor con luz, color, giro, espejo y recorte, en tiempo real.',
  'product.photos.p4': 'La edición es una receta: el original no se toca nunca.',
  'product.photos.f1':
    'Línea de tiempo agrupada por meses, con búsqueda por nombre y miniaturas nítidas.',
  'product.photos.f2': 'Álbumes: crear, renombrar, elegir portada, añadir desde el visor y quitar.',
  'product.photos.f3': 'Visor a pantalla completa con navegación por teclado.',
  'product.photos.f4':
    'Editor con cuatro deslizadores (exposición, contraste, saturación, temperatura), giro y espejo.',
  'product.photos.f5':
    'Recorte por arrastre y atajos de teclado; vista previa en la GPU, con respaldo en CPU.',
  'product.photos.f6':
    'La edición es una receta guardada aparte: el original de Drive no se modifica nunca.',
  'product.photos.f7':
    'Exportación a resolución completa, calculada en el servidor con las mismas fórmulas que ves.',
  'product.photos.f8': 'Papelera con restaurar.',
  'product.photos.f9':
    'Los originales viven en tu Sytue Drive; las derivadas son caché regenerable.',
  'product.photos.f10': 'Datos EXIF leídos del propio fichero; todo en la Unión Europea.',
  'product.photos.f11':
    'Selección múltiple por mes: a un álbum nuevo, a la papelera o descargadas de una vez.',
  'product.photos.f12':
    'Pase automático, tamaño real, giro desde el visor y recorte con proporción fija.',
  'product.rooms.f1': 'Canales públicos y privados en categorías; mensajes directos y grupos.',
  'product.rooms.f2': 'Hilos, reacciones, menciones, fijados, guardados y borradores.',
  'product.rooms.f3':
    'Formato con barra «Aa» y atajos: negrita, cursiva, código, citas, listas y enlaces.',
  'product.rooms.f4': 'Adjuntos, imágenes, GIF y emoji personalizados; ficheros desde Drive.',
  'product.rooms.f5': 'Envío programado, prioridad, acuse de recibo y «visto por».',
  'product.rooms.f6': 'Comandos con «/» y tarjetas: encuestas y tareas asignables.',
  'product.rooms.f7':
    'Videollamadas en el propio canal: cámara, pantalla, mano levantada, chat y grabación.',
  'product.rooms.f8': 'Reuniones programadas y enlaces para invitar.',
  'product.rooms.f9': 'Bots, webhooks e integraciones con otras herramientas.',
  'product.rooms.f10': 'Presencia y estado personalizado; avisos por canal a tu medida.',
  'product.rooms.f11': 'Búsqueda en el historial y desde toda la suite.',
  'product.rooms.f12': 'Tiempo real por WebSocket, con reconexión sin perder mensajes.',
  'product.rooms.f13':
    'Conmutador rápido de canales con Ctrl+G, marcar todo como leído y atajos con «?».',
  'product.cast.tagline':
    'Tus vídeos y tus directos, bajo demanda y en tiempo real, sin salir de Europa.',
  'product.cast.standard':
    'Reproducción HLS en el navegador; ingesta RTMP desde cualquier programa de emisión',
  'product.cast.p1':
    'Sube un vídeo y se transcodifica en varias calidades para cualquier pantalla.',
  'product.cast.p2':
    'Emite en directo desde un programa de emisión, desde el navegador o desde la app.',
  'product.cast.p3':
    'Cuatro niveles de visibilidad: público, oculto, organización o personas concretas.',
  'product.cast.p4': 'Chat en directo y comentarios, con moderación básica de quien publica.',
  'product.cast.f1': 'Sube vídeos de hasta 8 GiB; el original se conserva.',
  'product.cast.f2': 'Reproducción adaptativa en tres calidades: el navegador elige la mejor.',
  'product.cast.f3':
    'Directo desde un programa de emisión por RTMP, con una clave de emisión por persona.',
  'product.cast.f4':
    'Directo desde el navegador o la app: cámara y micrófono. Compartir pantalla en navegadores, sistemas de escritorio y Android compatibles; no disponible en la app de iOS/iPadOS.',
  'product.cast.f5': 'Las grabaciones de los directos se convierten en vídeos bajo demanda solas.',
  'product.cast.f6': 'Portada pública y un canal por organización, indexables.',
  'product.cast.f7':
    'Enlaces ocultos, vídeos solo para la organización o acceso a personas concretas por @usuario.',
  'product.cast.f8':
    'Chat en directo y comentarios en hilo, moderados por quien publica y por la administración de la organización.',
  'product.cast.f9': 'Reproductor incrustable y oEmbed para los vídeos públicos y ocultos.',
  'product.cast.f10':
    'El almacenamiento cuenta en tu cuota de Sytue Drive; todo en la Unión Europea.',
  'product.cast.f11':
    '«Subir a Cast» desde Sytue Drive: una copia del vídeo pasa a Cast y el fichero se queda en Drive.',
  'product.pages.tagline':
    'La web de tu empresa y su tienda, diseñadas en un editor visual y publicadas sin salir de Europa.',
  'product.pages.standard':
    'HTML y CSS estáticos y accesibles, sin cookies de terceros; exportación a Astro',
  'product.pages.p1':
    'Editor visual con marcos que se adaptan solos a escritorio, tableta y móvil.',
  'product.pages.p2':
    'Diez plantillas por sector, con textos de verdad y fotos de dominio público.',
  'product.pages.p3': 'Tienda con catálogo, carrito y cobro en la cuenta de tu empresa.',
  'product.pages.p4':
    'Tu dominio propio con certificado automático, o una dirección en sites.sytue.eu.',
  'product.pages.f1':
    'Editor visual por marcos con distribución automática, componentes reutilizables y secciones listas.',
  'product.pages.f2':
    'Tres puntos de corte (escritorio, tableta y móvil), cada uno con sus propios ajustes.',
  'product.pages.f3': 'Analítica de visitas sin cookies, dentro del panel del sitio.',
  'product.pages.f4':
    'Webs en varios idiomas, con direcciones por idioma, hreflang y traducción asistida por Sytue AI.',
  'product.pages.f5':
    'Imágenes desde tu equipo, Sytue Drive o Sytue Photos, con texto alternativo al insertarlas.',
  'product.pages.f6':
    'Tienda con variantes, existencias, IVA por producto y pago con Mollie, Adyen, Redsys o Stripe.',
  'product.pages.f7':
    'Pedidos con factura simplificada emitida por Sytue ERP y desistimiento desde la propia web.',
  'product.pages.f8':
    'Formularios con consentimiento, bandeja de mensajes y avisos a las personas de tu organización.',
  'product.pages.f9':
    'Versiones con nombre, vista previa con enlace secreto y vuelta a cualquier publicación anterior.',
  'product.pages.f10':
    'Exportación del código a Astro, React, Next.js y apps nativas, y del catálogo a Shopify, WooCommerce y otras plataformas.',
  'product.bases.tagline':
    'Tablas enlazadas para llevar clientes, pedidos o existencias, con permisos y registro de cada cambio.',
  'product.bases.standard':
    'CSV y JSON abiertos; esquema y relaciones con la integridad garantizada por la propia base de datos',
  'product.bases.p1':
    'Clientes, pedidos o existencias en tablas enlazadas, sin saber nada de bases de datos.',
  'product.bases.p2': 'Se comparte con tu organización con el rol que decidas.',
  'product.bases.p3': 'Cada cambio queda registrado: quién lo hizo y qué valía antes.',
  'product.bases.p4': 'Tus datos se quedan en la UE y los exportas cuando quieras.',
  'product.bases.f1':
    'Tablas con campos de texto, número, fecha, casilla, elección única y elección múltiple.',
  'product.bases.f2':
    'Relaciones entre tablas: enlaza un pedido con su cliente y la base de datos no deja romper el vínculo.',
  'product.bases.f3':
    'Rejilla editable con teclado, legible con lector de pantalla y usable desde el móvil.',
  'product.bases.f4': 'Permisos por base y por tabla: propietario, editor, comentarista y lector.',
  'product.bases.f5': 'Importa un CSV como tabla nueva y exporta a CSV o JSON.',
  'product.bases.f6': 'Registro de cada cambio: quién, qué registro y campo, antes y después.',
  'product.cloud.f12': 'Uso, costes y facturación europea en la misma consola.',
  'product.cloud.f11': 'Personas, tokens de API y auditoría exportable.',
  'product.cloud.f10': 'Analítica web respetuosa con la privacidad, sin cookies.',
  'product.cloud.f9': 'Tareas programadas y plantillas de proyecto.',
  'product.cloud.f8': 'Terminal en el contenedor; reinicia o pausa el servicio.',
  'product.cloud.f7': 'Registros en directo, métricas y monitorización con alertas.',
  'product.cloud.f6': 'Variables de entorno y secretos por entorno.',
  'product.cloud.f5': 'Dominios propios con TLS automático; redirecciones y cabeceras.',
  'product.cloud.f4': 'Almacenamiento S3 y funciones gestionadas.',
  'product.cloud.f3': 'Bases de datos Postgres con API REST automática y editor SQL.',
  'product.cloud.f2': 'Vista previa por cada PR y vuelta atrás a cualquier despliegue en un clic.',
  'product.cloud.f1':
    'Proyectos desde tu repositorio (GitHub, GitLab o Git propio) con despliegue en cada push.',
  'product.console.f1': 'Panel con el estado de la organización y lo que merece un vistazo hoy.',
  'product.console.f2':
    'Cuentas: alta, suspensión, restablecer contraseña y MFA, sesiones, alias y buzón.',
  'product.console.f3': 'Grupos estáticos y dinámicos por reglas; roles con permisos.',
  'product.console.f4': 'Acceso privilegiado (PIM): activación temporal con aprobación.',
  'product.console.f5':
    'SSO con proveedores SAML u OIDC, con prueba de conexión; aprovisionamiento SCIM.',
  'product.console.f6': 'Reglas de acceso condicional e inventario de dispositivos de confianza.',
  'product.console.f7':
    'Integraciones: clientes OIDC, SAML y conexiones con terceros; sustitución de productos.',
  'product.console.f8': 'Traer contenido: migración desde otros proveedores.',
  'product.console.f9': 'Editor de documentos y canales de Rooms administrables.',
  'product.console.f10':
    'Gobernanza: retenciones legales, etiquetas de confidencialidad y política de contraseñas.',
  'product.console.f11': 'Auditoría con exportación CSV y envío a tu SIEM; registros de servicio.',
  'product.console.f12':
    'Dominio y marca propios: logotipo de la organización y dominio de correo.',
  'product.id.f1':
    'Inicio de sesión único (OpenID Connect y OAuth 2.0 con PKCE) para la suite y tus aplicaciones.',
  'product.id.f2': 'Llaves de acceso (WebAuthn) y MFA por TOTP con códigos de recuperación.',
  'product.id.f3': 'Contraseñas de aplicación para IMAP y clientes que no hablan OIDC.',
  'product.id.f4': 'Sesiones y dispositivos: revoca una o todas las demás.',
  'product.id.f5': 'Federación SAML con tu IdP, y Sytue como IdP SAML para otras aplicaciones.',
  'product.id.f6': 'Aprovisionamiento SCIM desde tu directorio.',
  'product.id.f7': 'Invitaciones, verificación de correo y recuperación de contraseña.',
  'product.id.f8': 'Perfil para iPhone y Mac (.mobileconfig) en un toque.',
  'product.id.f9': 'Políticas de contraseña y reglas de acceso definidas por la organización.',
  'product.id.f10': 'Registro de clientes dinámico (DCR) y descubrimiento estándar.',
  'product.erp.f1':
    'Veinticuatro módulos y treinta y seis modelos en un solo servicio y una sola base de datos.',
  'product.erp.f2':
    'El catálogo de modelos es dato: cada módulo declara sus campos, sus estados y sus columnas de lista.',
  'product.erp.f3':
    'Un módulo nuevo es un fichero JSON — sin DDL, sin migración y sin bloquear una tabla.',
  'product.erp.f4':
    'Una sola API para los treinta y seis modelos, y una sola interfaz generada del catálogo.',
  'product.erp.f5':
    'Contabilidad por partida doble con importes en céntimos enteros; ni un decimal en coma flotante.',
  'product.erp.f6':
    'El cuadre lo comprueba la base de datos al confirmar la transacción, no el código de la aplicación.',
  'product.erp.f7':
    'Los asientos cerrados son inmutables: no se modifican, no se borran y no admiten apuntes nuevos.',
  'product.erp.f8':
    'Un documento, un asiento: un reintento o dos pestañas no pueden duplicar un ingreso.',
  'product.erp.f9':
    'Los rótulos comunes están en los 26 idiomas, y un campo sin traducir impide el arranque.',
  'product.erp.f10':
    'Listados sobre índice: la primera página de veinte mil filas se sirve en menos de un milisegundo.',

  // --- Descargas ----------------------------------------------------------
  'meta.downloads.title': 'Descargas — Sytue One para escritorio y móvil',
  'meta.downloads.description':
    'Apps nativas de Sytue One para macOS, Windows, Linux, iOS y Android. Toda la suite en tu equipo, con las integraciones de cada sistema.',
  'downloads.overline': 'Apps',
  'downloads.h1': 'Sytue One en tu equipo',
  'downloads.intro':
    'Aplicaciones de escritorio y móvil de verdad: se instalan, viven en el Dock o en el menú de inicio, avisan con las notificaciones del sistema y abren los archivos desde tu equipo.',
  'downloads.desktop.title': 'Escritorio',
  'downloads.desktop.body':
    'Una aplicación nativa por producto, que dibuja su propia interfaz con la tarjeta gráfica: sin navegador dentro, arranca rápido y consume como una app del sistema.',
  'downloads.apps.title': 'Solo lo que necesitas',
  'downloads.apps.body':
    'Sytue One trae la jornada entera. Si solo quieres el disco, Sytue Drive se instala por su cuenta.',
  'downloads.includes': 'Incluye:',
  'downloads.mobile.title': 'Móvil',
  'downloads.mobile.body':
    'Aplicación nativa de iOS y Android, con notificaciones del sistema, compartir y acceso a archivos y cámara.',
  'downloads.selfhost.title': '¿Tu organización se aloja en su propio dominio?',
  'downloads.selfhost.body':
    'Las aplicaciones nativas se conectan a sytue.eu. Si tu organización tiene una instalación propia, entra con el navegador en su dominio: la suite funciona igual en la web.',
  'downloads.yourSystem': 'Tu sistema',
  'downloads.signInToGet': 'Inicia sesión para descargar',
  'downloads.store': 'Ver en {format}',
  'downloads.soon': 'Próximamente en {format}',
  'downloads.browser.text':
    '¿Prefieres el navegador? Toda la suite funciona igual en la web, sin instalar nada:',
  'downloads.browser.link': 'abrir Sytue One',
  'platform.macos.req': 'macOS 11 o posterior · Apple Silicon e Intel',
  'platform.windows.req': 'Windows 10 o posterior · 64 bits',
  'platform.linux.req': 'OpenGL 2.1 · 64 bits',
  'platform.ios.req': 'iOS 14 o posterior',
  'platform.android.req': 'Android 6.0 o posterior',

  // --- Pie ----------------------------------------------------------------
  'footer.tagline':
    'Cada aplicación de la suite es un servicio propio, construido sobre protocolos abiertos y publicados.',
  'footer.products': 'Productos',
  'footer.resources': 'Recursos',
  'footer.legal': 'Legal',
  'footer.suite': 'La suite',
  'footer.downloads': 'Descargar las apps',
  'footer.sales': 'Hablar con ventas',
  'footer.privacy': 'Privacidad y RGPD',
  'footer.accessibility': 'Accesibilidad',
  'footer.openSource': 'Código abierto',
  'footer.legalNotice': 'Aviso legal',
  'footer.terms': 'Términos',
  'footer.copyright':
    '© {year} Sytue · Datos alojados en la UE (Scaleway fr-par / nl-ams · on-premise Madrid).',
  'footer.privacyNotice':
    'No vendemos tus datos ni los cedemos a terceros. No los usamos para perfilarte ni para publicidad, ni para entrenar o mejorar nuestros servicios o los de nadie. Van cifrados en tránsito y en reposo, y solo salen de nuestra infraestructura europea en lo imprescindible para cobrarte, para avisarte con la app cerrada (el servicio de push solo ve tu aparato y la hora, nunca el contenido) y en las integraciones que tú actives, en las que el tercero te pedirá aprobación sobre qué datos usará.',

  // --- 404 ----------------------------------------------------------------
  'meta.notFound.title': 'Página no encontrada — Sytue One',
  'notFound.overline': 'Error 404',
  'notFound.title': 'No encontramos esta página',
  'notFound.body': 'Puede que el enlace haya cambiado. Vuelve al inicio o explora la suite.',
  'notFound.home': 'Volver al inicio',

  // --- Legal: privacidad ---------------------------------------------------
  'meta.legal.title': 'Aviso legal — Sytue One',
  'meta.legal.description':
    'Datos identificativos del titular de www.sytue.eu y del servicio Sytue One: Sytue Projects, S.L.',
  'aviso.title': 'Aviso legal',
  'aviso.intro':
    'En cumplimiento del artículo 10 de la Ley 34/2002, de 11 de julio, de servicios de la sociedad de la información y de comercio electrónico (LSSI), estos son los datos identificativos del titular de este sitio web y del servicio Sytue One.',
  'aviso.review.body':
    'Los datos registrales de este aviso están comprobados en el Registro Mercantil. El resto del texto está en revisión legal y puede cambiar de redacción sin que cambie lo que describe.',
  'aviso.owner.title': 'Titular',
  'aviso.owner.name': 'Denominación social',
  'aviso.owner.form': 'Forma jurídica',
  'aviso.owner.formValue': 'Sociedad de responsabilidad limitada unipersonal',
  'aviso.owner.nif': 'NIF',
  'aviso.owner.address': 'Domicilio social',
  'aviso.owner.registry': 'Inscripción registral',
  'aviso.owner.registryValue':
    'Registro Mercantil de Madrid, sección 8, hoja M-887153, inscripción 1.ª',
  'aviso.owner.email': 'Correo electrónico',
  'aviso.owner.web': 'Sitio web',
  'aviso.scope.title': 'Objeto',
  'aviso.scope.body':
    'Este sitio web informa sobre la suite Sytue One y da acceso a sus aplicaciones. El uso del servicio se rige por los Términos del servicio y el tratamiento de datos personales, por la Política de privacidad, ambos enlazados al pie de cada página.',
  'aviso.ip.title': 'Propiedad intelectual e industrial',
  'aviso.ip.body':
    'Los contenidos de este sitio (textos, diseño, código, marcas y logotipos de Sytue y Sytue One) son de Sytue Projects, S.L. o se usan con licencia. Los nombres de productos de terceros que aparecen como referencia de compatibilidad son marcas de sus titulares.',
  'aviso.liability.title': 'Responsabilidad',
  'aviso.liability.body':
    'Sytue Projects, S.L. procura que este sitio esté disponible y actualizado, sin garantizar la ausencia de interrupciones o errores. No responde de los contenidos de sitios de terceros a los que se enlaza.',
  'aviso.law.title': 'Legislación aplicable',
  'aviso.law.body':
    'Este aviso se rige por la legislación española. Si eres consumidor, conservas los derechos y el fuero que te reconoce la normativa de tu país de residencia en la Unión Europea.',
  'aviso.updated': 'Última actualización: septiembre de 2026.',
  'meta.accessibility.title': 'Declaración de accesibilidad — Sytue One',
  'meta.accessibility.description':
    'Qué tan accesibles son la suite Sytue One y esta web, qué medimos, qué falta todavía y cómo avisarnos.',
  'a11y.title': 'Declaración de accesibilidad',
  'a11y.intro':
    'Sytue quiere que cualquier persona pueda usar Sytue One y esta web, con la tecnología de apoyo que necesite. Esta declaración cubre www.sytue.eu y las aplicaciones web de la suite.',
  'a11y.standard.title': 'Norma con la que trabajamos',
  'a11y.standard.body':
    'Diseñamos y construimos siguiendo las WCAG 2.1 nivel AA, el nivel al que remiten la norma europea EN 301 549 y la Ley Europea de Accesibilidad (Directiva (UE) 2019/882).',
  'a11y.status.title': 'Situación de conformidad',
  'a11y.status.body':
    'Parcialmente conforme: la mayor parte de la suite cumple la norma y las partes que se enumeran más abajo aún no la cumplen del todo. Todavía no hemos encargado una auditoría independiente.',
  'a11y.measures.title': 'Qué hacemos para conseguirlo',
  'a11y.measures.design':
    'Un solo sistema de diseño para toda la suite: colores de texto con el contraste comprobado, anillo de foco visible en todos los controles y manejo por teclado de menús, diálogos y listas.',
  'a11y.measures.ci':
    'Auditorías automáticas de accesibilidad (Lighthouse) en la cadena de integración con cada cambio de la web pública.',
  'a11y.measures.system':
    'La interfaz sigue los ajustes de tu sistema: tema oscuro o claro, movimiento reducido y tamaño del texto, sin tamaños fijos en píxeles en los campos de formulario.',
  'a11y.measures.text':
    'Toda imagen, icono y estado vacío lleva una alternativa textual, y cada página declara su idioma para que los lectores de pantalla lo pronuncien bien.',
  'a11y.limits.title': 'Limitaciones conocidas',
  'a11y.limits.body':
    'La rejilla de la hoja de cálculo, el lienzo de las presentaciones y la anotación de PDF son editores complejos a los que aún les falta trabajo para manejarse del todo con teclado y lector de pantalla. Los documentos y ficheros que suben las personas se muestran tal cual, sin que los corrijamos.',
  'a11y.feedback.title': 'Dinos qué no funciona',
  'a11y.feedback.body':
    'Si algo de la suite o de esta web te resulta difícil o imposible de usar, escríbenos: lo revisamos y te contestamos.',
  'a11y.enforcement.title': 'Si no quedas conforme',
  'a11y.enforcement.body':
    'Si has comunicado una barrera y la respuesta no te satisface, puedes dirigirte a la autoridad responsable de accesibilidad de tu país conforme a la Ley Europea de Accesibilidad.',
  'a11y.updated':
    'Esta declaración se redactó en septiembre de 2026 y se revisa cuando la suite cambia de forma sustancial.',
  'meta.openSource.title': 'Código abierto — Sytue One',
  'meta.openSource.description':
    'Sytue construye casi todo desde cero, pero no reinventa lo que ya funciona bien. Los proyectos de código abierto sobre los que se apoya la suite.',
  'oss.title': 'Código abierto',
  'oss.intro':
    'Sytue construye casi todo desde cero: el correo, los editores, el calendario. Pero no reinventa lo que ya funciona bien. Estos son los proyectos de código abierto sobre los que se apoyan algunas piezas de la suite.',
  'oss.livekit.body':
    'El audio y el vídeo en tiempo real de las llamadas de Sytue Rooms corren sobre LiveKit.',
  'oss.postgres.body':
    'Cada servicio de la suite guarda sus datos en PostgreSQL, una base por organización.',
  'meta.privacy.title': 'Privacidad y RGPD — Sytue One',
  'meta.privacy.description':
    'Cómo trata Sytue One los datos personales: responsable, datos, bases legales, conservación, encargados, seguridad, transferencias internacionales y tus derechos RGPD.',
  'legal.overline': 'Legal',
  'legal.updated': 'Última actualización: julio de 2026.',
  'legal.review.title': 'En revisión legal.',
  'privacy.title': 'Privacidad y RGPD',
  'privacy.review.body':
    'Esta política describe con exactitud cómo tratamos los datos hoy. Los plazos concretos de conservación se completan con la versión final, revisada por nuestro equipo jurídico.',
  'privacy.controller.title': 'Responsable del tratamiento',
  'privacy.controller.body':
    'Sytue Projects, S.L. (NIF B88729900; Calle Atenas, 10, 28822 Coslada, Madrid; inscrita en el Registro Mercantil de Madrid, sección 8, hoja M-887153), como proveedora de la suite Sytue One, es la responsable del tratamiento de los datos de las cuentas y organizaciones del servicio. Contacto en materia de protección de datos:',
  'privacy.data.title': 'Qué datos tratamos',
  'privacy.data.account.label': 'Datos de cuenta:',
  'privacy.data.account.body':
    'nombre, dirección de correo y organización, gestionados con Sytue ID (SSO + MFA).',
  'privacy.data.content.label': 'Contenido de tu organización:',
  'privacy.data.content.body':
    'correo, archivos, documentos, calendario y mensajes que tu organización aloja en la suite. Ese contenido es de tu organización.',
  'privacy.data.technical.label': 'Datos técnicos de operación:',
  'privacy.data.technical.body':
    'registros de acceso y datos de conexión necesarios para la seguridad, la prevención de abuso y el funcionamiento del servicio.',
  'privacy.data.note':
    'No vendemos datos ni los usamos para publicidad. La infraestructura y el almacenamiento gestionados por Sytue están en la Unión Europea (Scaleway, regiones fr-par y nl-ams, y zona on-premise en Madrid), con cifrado en tránsito (TLS 1.3) y en reposo. Las conexiones con servicios externos envían los datos que autorizas a su proveedor, sujetos a su ubicación y condiciones.',
  'privacy.basis.title': 'Base legal',
  'privacy.basis.body':
    'Los tratamientos se apoyan en las bases del artículo 6 del RGPD: la ejecución del contrato de servicio con tu organización, el interés legítimo en la seguridad y la prevención de abuso, el cumplimiento de obligaciones legales y, cuando se solicite expresamente, tu consentimiento. El detalle base por base, tratamiento a tratamiento, formará parte del texto definitivo.',
  'privacy.retention.title': 'Conservación',
  'privacy.retention.body':
    'Conservamos los datos de cuenta y el contenido mientras tu organización mantenga activo el servicio. Cuando una cuenta u organización se da de baja, los datos se eliminan o se anonimizan en un plazo razonable, salvo lo que debamos conservar por obligación legal (por ejemplo, facturación). Los registros técnicos de operación se purgan de forma periódica. Los plazos concretos por categoría se detallan en la versión final. Las denuncias de contenido se conservan mientras están abiertas y se borran doce meses después de resolverse.',
  'privacy.processors.title': 'Encargados y subencargados',
  'privacy.processors.body':
    'Nuestro proveedor de infraestructura es Scaleway (regiones fr-par y nl-ams), que actúa como encargado del tratamiento y opera exclusivamente en la Unión Europea; además operamos una zona on-premise en Madrid (España), en instalaciones propias. Para el cobro de las suscripciones recurrimos a Mollie B.V. (Países Bajos), que trata únicamente los datos necesarios para el pago. Para enviar el SMS con el código que verifica el móvil al crear una cuenta recurrimos a Sweego (Mindbaz SAS, Francia), que recibe únicamente ese número y el código. No recurrimos a subencargados fuera del EEE. La lista de subencargados se publicará y se mantendrá actualizada, con aviso previo de cualquier alta.',
  'privacy.security.title': 'Seguridad',
  'privacy.security.body':
    'Aplicamos medidas técnicas y organizativas acordes al RGPD: cifrado en tránsito (TLS 1.3) y en reposo, aislamiento entre organizaciones (multi-tenant con seguridad a nivel de fila), autenticación reforzada (MFA/WebAuthn), registros de acceso y principio de mínimo privilegio. Ningún sistema es infalible: si se produjera una brecha que te afecte, lo notificaremos conforme a los plazos del Reglamento.',
  'privacy.cookies.title': 'Cookies',
  'privacy.cookies.body':
    'Usamos únicamente cookies técnicas necesarias para el inicio de sesión único y la seguridad de la sesión. No usamos cookies de publicidad ni de seguimiento de terceros, por lo que no necesitamos mostrarte un muro de consentimiento para rastreo.',
  'privacy.transfers.title': 'Transferencias internacionales',
  'privacy.transfers.body':
    'El tratamiento y el alojamiento gestionados por Sytue ocurren en regiones de la Unión Europea. Las conexiones externas activadas por el usuario envían los datos autorizados a su proveedor; su ubicación depende del proveedor y de su configuración. Los avisos que te llegan con la app o el navegador cerrados también pueden usar servicios externos. El servicio de push de tu aparato (Google, Mozilla o Apple, según tu navegador o tu móvil) ve un identificador anónimo del aparato y la hora de cada aviso; nunca el contenido, que viaja cifrado de extremo a extremo y en iOS va directamente vacío. Puedes apagar los avisos, y entonces tampoco sale eso.',
  'privacy.rights.title': 'Tus derechos',
  'privacy.rights.body1':
    'El RGPD te reconoce los derechos de acceso, rectificación, supresión, oposición, limitación del tratamiento y portabilidad. Puedes ejercerlos escribiendo a',
  'privacy.rights.body2':
    '; respondemos en los plazos que marca el Reglamento. También tienes derecho a reclamar ante',
  'privacy.rights.authority': 'tu autoridad de control',
  'privacy.dpo.title': 'Delegado de protección de datos (DPO)',
  'privacy.dpo.body':
    'Para cualquier consulta sobre protección de datos o para contactar con nuestro delegado de protección de datos:',
  'privacy.minors.title': 'Menores',
  'privacy.minors.body':
    'Sytue One es un servicio para organizaciones y las personas que trabajan en ellas; no está dirigido a menores ni recopilamos datos de menores de forma consciente.',
  'privacy.changes.title': 'Cambios en esta política',
  'privacy.changes.body':
    'Si actualizamos esta política, cambiaremos la fecha de «última actualización» y, cuando los cambios sean relevantes, te lo comunicaremos por los canales del servicio antes de que surtan efecto.',

  // --- Legal: términos -----------------------------------------------------
  'meta.terms.title': 'Términos del servicio — Sytue One',
  'meta.terms.description':
    'Términos del servicio de Sytue One: prestador, objeto, cuentas y organizaciones, uso aceptable, disponibilidad, tus datos, propiedad intelectual y ley aplicable.',
  'terms.title': 'Términos del servicio',
  'terms.review.body':
    'Estas condiciones describen con exactitud el servicio y su uso. Las cláusulas contractuales concretas (nivel de servicio, responsabilidad y jurisdicción) se completan con la versión final, revisada por nuestro equipo jurídico.',
  'terms.provider.title': 'Quién presta el servicio',
  'terms.provider.body':
    'Sytue One lo presta Sytue Projects, S.L. (NIF B88729900; Calle Atenas, 10, 28822 Coslada, Madrid; inscrita en el Registro Mercantil de Madrid, sección 8, hoja M-887153) desde infraestructura ubicada exclusivamente en la Unión Europea (Scaleway, regiones fr-par y nl-ams, y zona on-premise en Madrid), y se rige por el derecho de la UE.',
  'terms.scope.title': 'Objeto del servicio',
  'terms.scope.body':
    'Sytue One es una suite de productividad para organizaciones: identidad (Sytue ID), correo, archivos, documentos, calendario, contactos y mensajería con videollamadas. El servicio se contrata por organización.',
  'terms.accounts.title': 'Cuentas y organizaciones',
  'terms.accounts.body':
    'Cada organización administra sus cuentas, grupos y permisos a través de Sytue ID (SSO + MFA). La administración de la organización es responsable de las altas, bajas y del uso que se hace del servicio con esas cuentas.',
  'terms.use.title': 'Uso aceptable',
  'terms.use.body':
    'Te comprometes a usar el servicio conforme a la ley y a no emplearlo para actividades ilícitas, envío de spam, distribución de malware, vulneración de derechos de terceros o cualquier uso que comprometa la seguridad o la disponibilidad del servicio para otras organizaciones.',
  'terms.availability.title': 'Disponibilidad y soporte',
  'terms.availability.body1':
    'Trabajamos para ofrecer una alta disponibilidad y realizamos mantenimientos y mejoras de forma continua. Los compromisos concretos de nivel de servicio (SLA), si aplican a tu plan, se fijan en el acuerdo de servicio. El soporte se atiende a través de',
  'terms.data.title': 'Tus datos',
  'terms.data.body1':
    'El contenido que tu organización aloja en la suite es de tu organización. Está construido sobre estándares abiertos (OIDC, JMAP, CalDAV, CardDAV, WebDAV), de modo que tus datos son siempre portables y puedes llevártelos. El tratamiento de datos personales se describe en la página de',
  'terms.ip.title': 'Propiedad intelectual',
  'terms.ip.body':
    'El software de Sytue One es obra original de Sytue y está protegido por los derechos de propiedad intelectual correspondientes. Conservas la titularidad del contenido que tu organización aloja en la suite; nosotros solo lo tratamos para prestarte el servicio.',
  'terms.liability.title': 'Responsabilidad',
  'terms.liability.body':
    'Ponemos todo nuestro empeño en un servicio fiable y seguro. Las limitaciones y garantías de responsabilidad aplicables se fijan en el acuerdo de servicio; nada en estas condiciones excluye responsabilidades que la ley no permita excluir.',
  'terms.contact.title': 'Contratación y contacto',
  'terms.contact.body1':
    'Para condiciones de contratación, precios por organización o cualquier consulta sobre el servicio, escríbenos a',
  'terms.changes.title': 'Modificaciones',
  'terms.changes.body':
    'Podemos actualizar estas condiciones para reflejar cambios en el servicio o en la normativa. Cuando los cambios sean relevantes, te avisaremos por los canales del servicio antes de que surtan efecto, actualizando la fecha de «última actualización».',
  'terms.law.title': 'Ley aplicable',
  'terms.law.body':
    'El servicio se rige por el derecho de la Unión Europea y la normativa del país del prestador. El fuero concreto para la resolución de conflictos se fija en el texto definitivo. Con independencia de ello, conservas tu derecho a reclamar ante tu autoridad de control en materia de protección de datos.',

  // --- Datos estructurados (JSON-LD) --------------------------------------
  'seo.org.description':
    'Sytue One: la suite de productividad soberana europea. Identidad, correo, archivos, documentos, calendario, chat, videollamadas y una PaaS, 100 % en la UE.',
  'seo.org.slogan': 'Tu productividad, soberana y europea.',
  'seo.software.description':
    'Suite de productividad soberana europea: correo (JMAP), archivos (WebDAV), documentos colaborativos, calendario (CalDAV), contactos (CardDAV), chat, videollamadas y PaaS. SSO con Sytue ID.',
  'seo.software.f1': 'Inicio de sesión único (SSO) + MFA/WebAuthn',
  'seo.software.f2': 'Correo, archivos, documentos, calendario, contactos, chat y videollamadas',
  'seo.software.f3': 'Plataforma de despliegue y datos gestionados',
  'seo.software.f4': 'Datos 100 % en la UE (Scaleway)',
  'seo.offer.description': 'Prueba gratuita sin tarjeta.',
  'seo.downloads.description':
    'Apps nativas de la suite Sytue One para escritorio (macOS, Windows, Linux) y móvil (iOS, Android).',

  // --- Precios: cuenta personal ---
  'home.pricing.personal.title': 'Gratis',
  'home.pricing.personal.freeStorage': '5 GB, para una persona',
  'home.pricing.personal.freeBody':
    'Correo, archivos, fotos, documentos, calendario, contactos, notas y salas. Sin tarjeta y sin límite de tiempo.',
  'home.pricing.personal.note':
    'Tu organización es tu correo: sin administración de organización ni ERP, y sí todo lo demás, incluido Sytue Admin para tus integraciones y migraciones.',
  'home.pricing.personal.cta': 'Crear cuenta gratis',
  'grid.orgOnly': 'Solo para organizaciones',
  'grid.enDesarrollo': 'En desarrollo',
  'home.pricing.pro.title': 'Pro',
  'home.pricing.pro.storage': '200 GB por usuario',
  'home.pricing.paidBody':
    'Para organizaciones y también para una persona sola. Toda la suite, con Sytue ERP y Sytue Admin para administrar tu organización.',
  'home.pricing.thirdParty':
    'Todos los planes permiten conectar tu propio almacenamiento: Google Drive, Microsoft OneDrive, un bucket S3 (Scaleway, OVH, Hetzner, Wasabi, MinIO), un WebDAV como Nextcloud o Internxt, o tu NAS en tus instalaciones —sin abrir puertos, por un túnel de Sytue—. Las copias de seguridad y la recuperación de los datos guardados en un almacenamiento conectado quedan bajo la responsabilidad del cliente (usted), y Sytue® queda liberada de toda responsabilidad por la pérdida de datos en los almacenamientos conectados a Sytue®.',
  'home.pricing.soon': 'Próximamente',
  'home.pricing.enterprise.title': 'Enterprise',
  'home.pricing.enterprise.amount': '25 €',
  'home.pricing.enterprise.storage': '1 TB por usuario',
  'home.pricing.enterprise.support': 'Con Soporte Premium.',
  'home.pricing.education': 'Instituciones educativas: 50 % de descuento en Pro.',
};
