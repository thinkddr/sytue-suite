/**
 * Icon — iconografia de interfaz (grid 24, trazo de 1,5 px y relleno de acento:
 * ver `Glifo`) para el cromo de la suite. Distinta de los iconos de producto
 * (branding/icons/*.svg). Geometria europea moderna, coherente con docs/design.md §1.
 *
 * ENCUADRE (identidad de la suite): todos los glifos se dibujan en un grid de
 * 24 con caja optica consistente — contenido centrado en (12,12), padding ~2-3u por
 * lado, geometria de rejilla (esquinas cuadradas de radio minimo). Ningun glifo
 * "flota" pequeno ni descentrado: llenan su cuadro de forma uniforme.
 */
import type { JSX } from 'preact';
import { Glifo, type GlifoProps } from './Glifo.js';

const PATHS = {
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-3.8-3.8',
  mail: 'M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2zM22 6l-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 6',
  bell: 'M18 8.5a6 6 0 1 0-12 0c0 5.5-2.2 7-2.2 7h16.4s-2.2-1.5-2.2-7ZM10.3 20a2 2 0 0 0 3.4 0',
  close: 'M18.5 5.5 5.5 18.5M5.5 5.5l13 13',
  check: 'M20 6.5 9.5 17.5 4 12',
  chevronDown: 'M5 8.5l7 7 7-7',
  chevronUp: 'M5 15.5l7-7 7 7',
  chevronRight: 'M8.5 5l7 7-7 7',
  chevronLeft: 'M15.5 5l-7 7 7 7',
  menu: 'M3.5 6.5h17M3.5 12h17M3.5 17.5h17',
  grid: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z',
  user: 'M12 3.5a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM4 20.5a8 8 0 0 1 16 0',
  settings:
    'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6Z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z',
  logout: 'M9 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h3M16 16.5l4.5-4.5L16 7.5M20.5 12H9',
  info: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM12 11v5M12 7.5h.01',
  warning:
    'M10.3 4.2 2.2 18a2 2 0 0 0 1.7 3h16.2a2 2 0 0 0 1.7-3L13.7 4.2a2 2 0 0 0-3.4 0ZM12 9.5v4.5M12 17.5h.01',
  danger: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM12 8v4.5M12 16h.01',
  plus: 'M12 4.5v15M4.5 12h15',
  /* Administración e infraestructura (Console / Cloud). */
  building:
    'M3.5 21V6a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2v15M14.5 21V10.5h3a2 2 0 0 1 2 2V21M2.5 21h19M7 8h3.5M7 11.5h3.5M7 15h3.5',
  users:
    'M16 21v-1.5a4.5 4.5 0 0 0-4.5-4.5h-4A4.5 4.5 0 0 0 3 19.5V21M9.5 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM21 21v-1.5a4.5 4.5 0 0 0-3.4-4.36M16 3.13a4 4 0 0 1 0 7.74',
  database:
    'M12 3c-4.4 0-8 1.3-8 3s3.6 3 8 3 8-1.3 8-3-3.6-3-8-3ZM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
  bucket:
    'M12 3.5c-4.4 0-8 1.1-8 2.5s3.6 2.5 8 2.5 8-1.1 8-2.5-3.6-2.5-8-2.5ZM4.3 6.2l1.6 14a1 1 0 0 0 1 .8h10.2a1 1 0 0 0 1-.8l1.6-14',
  rocket:
    'M12 3.8c2.6 2.2 4 5.2 4 8.2V16H8v-4c0-3 1.4-6 4-8.2ZM12 9.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3ZM8 14l-3 2.7V20l3-1.4M16 14l3 2.7V20l-3-1.4M12 16v4',
  cloud: 'M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z',
  audit:
    'M15 4.5h2A1.5 1.5 0 0 1 18.5 6v13.5A1.5 1.5 0 0 1 17 21H7a1.5 1.5 0 0 1-1.5-1.5V6A1.5 1.5 0 0 1 7 4.5h2M9 3.5h6a1 1 0 0 1 1 1V6a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1ZM9 13.5l2 2 3.5-4',
  /* Semántica adicional (secretos / dominios / datos / overflow / código). */
  lock: 'M8 10V7a4 4 0 0 1 8 0v3M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8ZM12 14.5v2.5',
  globe: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM3.5 12h17M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18Z',
  table:
    'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM3 9.5h18M9.5 5.5V18.5M14.5 5.5V18.5',
  // Dos destellos: el asistente de IA. VIVE AQUÍ Y NO EN CADA APP porque lo
  // usan ya Mail, Calendar, Contactos, Fotos y Drive, y un glifo copiado seis
  // veces es un glifo que acaba dibujando seis cosas distintas.
  ai: 'M12 3.5 13.7 8.3 18.5 10 13.7 11.7 12 16.5 10.3 11.7 5.5 10 10.3 8.3ZM18 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8Z',
  ellipsis: 'M6 12h.01M12 12h.01M18 12h.01',
  code: 'M9 8l-4.5 4 4.5 4M15 8l4.5 4-4.5 4M13.5 5.5l-3 13',
  // Marco + prompt: la terminal NO puede compartir glifo con `code`, que ya usa
  // «Funciones». Dos entradas del mismo menú con el mismo icono dejan de ser
  // reconocibles de un vistazo, que es lo único que aporta un icono en un menú.
  terminal:
    'M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM7 10l2.5 2L7 14M12.5 15H17',
  chart: 'M4 4v16h16M8 20v-4M12 20v-8M16 20v-11',
  // Esfera + manecillas: lo programado (tareas cron y jobs). No puede ser `audit`
  // ni `rocket` —ya son «Registro» y «Despliegues» en el mismo menú—, y lo que
  // distingue a una tarea de todo lo demás es CUÁNDO ocurre, no qué ejecuta.
  clock: 'M12 3a9 9 0 1 1 0 18 9 9 0 0 1 0-18ZM12 7.5V12l3 2',
  receipt:
    'M6 4.5a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1V20.5l-2-1.2-2 1.2-2-1.2-2 1.2-2-1.2-2 1.2ZM9 8h6M9 11.5h6',
  /*
   * Identidad y gobierno (Sytue Admin). Mismo motivo que `terminal`, pero peor:
   * el menú de administración llevaba SIETE entradas con el candado —roles, acceso
   * privilegiado, SSO, acceso condicional, dispositivos, gobernanza y etiquetas—,
   * así que el icono no distinguía ninguna. El candado se queda solo para el
   * acceso privilegiado, que es el que de verdad restringe.
   */
  // Llave vertical: anilla, vástago y dos dientes. Credencial → inicio de sesión.
  key: 'M12 3.5a3.75 3.75 0 1 0 0 7.5 3.75 3.75 0 0 0 0-7.5ZM12 11v9.5M12 15h3.5M12 18h2.5',
  // Escudo: protección y política (gobernanza).
  shield: 'M12 3.5 4.5 6.6v5.4c0 4.3 3 7.5 7.5 9 4.5-1.5 7.5-4.7 7.5-9V6.6L12 3.5Z',
  // Portátil: pantalla sobre la línea de la base.
  device: 'M5.5 5.5h13a1 1 0 0 1 1 1v8.5h-15V6.5a1 1 0 0 1 1-1ZM2.5 18.5h19',
  // Silueta con visto: a quién se le concede qué (roles).
  userCheck:
    'M9.5 3.5a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM2 20.5a7.5 7.5 0 0 1 15 0M16.5 13l1.8 1.8L22 11',
  // Etiqueta con su ojal, en diagonal como una marca colgada.
  tag: 'M11.7 3.5H19a1.5 1.5 0 0 1 1.5 1.5v7.3a1.5 1.5 0 0 1-.44 1.06l-6.7 6.7a1.5 1.5 0 0 1-2.12 0l-7.3-7.3a1.5 1.5 0 0 1 0-2.12l6.7-6.7A1.5 1.5 0 0 1 11.7 3.5ZM16.3 8.2h.01',
  /*
   * Anotación de documento (Sytue PDF). La barra de herramientas pintaba sus
   * catorce herramientas con EMOJI —✋ 🖍 💬 ✏️ 📐 🏷— y dos apaños tipográficos
   * con diacrítico (`U̲`, `S̶`): a todo color sobre marfil y grafito, contra la
   * identidad de la casa, y con el avance tipográfico de cada sistema, así que
   * la fila medía distinto en cada plataforma. La iconografía vive aquí.
   */
  // Mano: tres dedos y el pulgar cerrando la palma. Arrastrar el documento.
  hand: 'M15 12V7a1.5 1.5 0 0 1 3 0v8a6 6 0 0 1-6 6h-1a5 5 0 0 1-3.5-1.5l-3.2-3.2a1.5 1.5 0 0 1 2.1-2.1L9 15V6a1.5 1.5 0 0 1 3 0v6M12 12V4.5a1.5 1.5 0 0 1 3 0V12',
  // Rotulador de punta biselada sobre su trazo.
  highlighter: 'M13.5 2.5 20.5 9.5 10.5 19.5H4v-6.5ZM4 22h16',
  // La letra y su regla: subrayar y tachar se distinguen por DÓNDE cae la línea.
  underline: 'M7 4v6a5 5 0 0 0 10 0V4M5.5 20.5h13',
  strikethrough:
    'M4.5 12h15M8 7.5A3.5 3.5 0 0 1 11.5 4.5h1a3.5 3.5 0 0 1 3.4 2.6M16 16.5a3.5 3.5 0 0 1-3.5 3h-1a3.5 3.5 0 0 1-3.4-2.6',
  // Bocadillo rectangular con rabillo: nota al margen. Rectangular y no ovalado
  // para no confundirlo con el `cloud` de infraestructura a 16px.
  comment:
    'M4.5 4.5h15a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H11l-5 4v-4H4.5a1 1 0 0 1-1-1v-10a1 1 0 0 1 1-1Z',
  // Lápiz con su virola: dibujo a mano alzada.
  pen: 'M4 20.5h4l11-11a2.1 2.1 0 0 0-3-3L5 17.5v3ZM14.5 6.5l3 3',
  square: 'M4.5 5.5h15a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1v-11a1 1 0 0 1 1-1Z',
  circle: 'M12 5.5c4.7 0 8.5 2.9 8.5 6.5s-3.8 6.5-8.5 6.5S3.5 15.6 3.5 12 7.3 5.5 12 5.5Z',
  line: 'M4.5 19.5 19.5 4.5',
  arrowUpRight: 'M5 19 19 5M9.5 5H19v9.5',
  // Escuadra con sus marcas: medir distancias sobre el documento.
  ruler:
    'M2.9 16.2 16.2 2.9a1.5 1.5 0 0 1 2.1 0l2.8 2.8a1.5 1.5 0 0 1 0 2.1L7.8 21.1a1.5 1.5 0 0 1-2.1 0l-2.8-2.8a1.5 1.5 0 0 1 0-2.1ZM7 12l2 2M10 9l2 2M13 6l2 2',
  // La T mayúscula con su base: insertar texto.
  text: 'M5 4.5h14M12 4.5v15M9 19.5h6',
  bold: 'M7 4.5h6.5a3.75 3.75 0 0 1 0 7.5H7zM7 12h7.5a3.75 3.75 0 0 1 0 7.5H7z',
  italic: 'M10 4.5h8M6 19.5h8M14.5 4.5l-5 15',
  link: 'M10 13.5a4.5 4.5 0 0 0 6.36.36l2.5-2.5a4.5 4.5 0 0 0-6.36-6.36l-1.25 1.25M14 10.5a4.5 4.5 0 0 0-6.36-.36l-2.5 2.5a4.5 4.5 0 0 0 6.36 6.36l1.25-1.25',
  quote: 'M17.5 6.5h-14M20.5 12h-13M20.5 17.5h-13M3.5 12v5.5',
  list: 'M4 6.5h.01M4 12h.01M4 17.5h.01M8.5 6.5h11.5M8.5 12h11.5M8.5 17.5h11.5',
  // Sello de mano sobre su base.
  stamp:
    'M4.5 20.5h15M6.5 17.5h11v-1.8a2 2 0 0 0-1.5-1.9l-2.2-.6a1.3 1.3 0 0 1-.9-1.5l.5-2.9A3.4 3.4 0 0 0 12 3.5a3.4 3.4 0 0 0-3.4 4.3l.5 2.9a1.3 1.3 0 0 1-.9 1.5l-2.2.6a2 2 0 0 0-1.5 1.9Z',
  // Campo de formulario: la caja vacía con el cursor dentro. Es lo que se
  // dibuja sobre un impreso para que otro pueda rellenarlo.
  formField: 'M3.5 7.5h17v9h-17ZM8 10.5h4M10 10.5v3',
  // Censura: la banda que tapa una línea entre dos que se siguen leyendo. Es lo
  // único de la barra que DESTRUYE contenido, así que no comparte signo con nada.
  redact: 'M4.5 5.5h15M4.5 18.5h9M3.5 10h17v4h-17Z',
  /* Zoom y ajuste del lienzo. */
  minus: 'M4.5 12h15',
  fitWidth: 'M3.5 12h17M7 8.5 3.5 12 7 15.5M17 8.5 20.5 12 17 15.5',
  fitPage: 'M14.5 3.5H20v5.5M20 3.5l-6.5 6.5M9.5 20.5H4V15M4 20.5 10.5 14',
  dosPaginas: 'M4 4.5h6.5v15H4zM13.5 4.5H20v15h-6.5z',
  camera:
    'M4 8.5A1.5 1.5 0 0 1 5.5 7H8l1.4-2h5.2L16 7h2.5A1.5 1.5 0 0 1 20 8.5V17a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 17ZM12 16a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z',
  // Los tres de videollamada. `microfonoOff` lleva la barra DIBUJADA y no
  // superpuesta con CSS: el silencio tiene que leerse de un vistazo y sin
  // depender del color, que en una barra oscura sobre vídeo no siempre contrasta.
  microfono:
    'M12 4a3 3 0 0 1 3 3v5a3 3 0 0 1-6 0V7a3 3 0 0 1 3-3ZM6 11.5a6 6 0 0 0 12 0M12 17.5V20',
  microfonoOff:
    'M9 7a3 3 0 0 1 6 0v3M15 13.2a3 3 0 0 1-6-1.2M6 11.5a6 6 0 0 0 9.3 5M18 11.5a6 6 0 0 1-.4 2.2M12 17.5V20M4 4l16 16',
  // Pantalla compartida: un monitor con la flecha de salida. Sin flecha se
  // confunde con «cambiar de vista».
  pantalla: 'M3.5 5.5h17v10h-17zM8.5 19.5h7M12 15.5v4M12 12V7.5M9.5 10 12 7.5 14.5 10',
  // Corazón para «favorita». Se dibuja con trazo, como el resto; relleno se
  // consigue pasándole `fill="currentColor"`, que es lo que distingue marcada
  // de sin marcar sin depender del color.
  corazon:
    'M20.8 5.7a5.5 5.5 0 0 0-7.8 0l-1 1-1-1a5.5 5.5 0 0 0-7.8 7.8l8.8 8.7 8.8-8.7a5.5 5.5 0 0 0 0-7.8Z',
  image:
    'M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2ZM8.5 10.5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3ZM20 15l-4.5-4.5L7 19',
  album:
    'M8 8V6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2M4 10a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2ZM16 20l-4-4-4 4',
  rotate: 'M20 12a8 8 0 1 1-2.3-5.7M20 4v4.5h-4.5',
  flipH: 'M12 3v18M4 8l4 4-4 4V8ZM20 8l-4 4 4 4V8Z',
  flipV: 'M3 12h18M8 4l4 4 4-4H8ZM8 20l4-4 4 4H8Z',
  undo: 'M9 14 4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3',
  // Disquete: guardar el trabajo actual. La etiqueta superior y la ventana
  // inferior lo distinguen de un documento a 16px.
  save: 'M5 3h11l3 3v15H5zM8 3v6h8V3M8 21v-7h8v7',
  download: 'M12 4v12M7 11l5 5 5-5M4 20h16',
  upload: 'M12 16V4M7 9l5-5 5 5M4 20h16',
  // Dos flechas, una que sube y otra que baja: datos en los dos sentidos (DataFlow).
  swap: 'M8 20V5M4 9l4-4 4 4M16 4v15M12 15l4 4 4-4',
  calendar: 'M4 6h16v14H4zM4 10h16M8 3v4M16 3v4',
  copies: 'M8 8h12v12H8zM4 16V4h12',
  trash: 'M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3',
  recorte: 'M7 2.5v14.5h14.5M2.5 7H17v14.5',
  voz: 'M4 9.5h3.5L12 5.5v13L7.5 14.5H4zM16 9a4 4 0 0 1 0 6M18.5 6.5a7.5 7.5 0 0 1 0 11',
  /* Reproductor de vídeo (Sytue Cast). Trazo como el resto: `play` relleno se
     consigue con `fill="currentColor"`, igual que el corazón. */
  play: 'M7 4.8v14.4a.8.8 0 0 0 1.2.7l11.4-7.2a.8.8 0 0 0 0-1.4L8.2 4.1A.8.8 0 0 0 7 4.8Z',
  pause: 'M7 5h3v14H7zM14 5h3v14h-3z',
  // El altavoz de `voz` con la cruz del silencio en vez de las ondas.
  vozOff: 'M4 9.5h3.5L12 5.5v13L7.5 14.5H4zM16 9.5l5 5M21 9.5l-5 5',
  // Las cuatro esquinas hacia fuera (entrar) y hacia dentro (salir).
  maximizar: 'M4 9V4h5M15 4h5v5M20 15v5h-5M9 20H4v-5',
  minimizar: 'M9 4v5H4M20 9h-5V4M15 20v-5h5M4 15h5v5',
  // Imagen dentro de imagen: el marco y la ventanita flotante abajo a la derecha.
  pip: 'M3.5 5.5h17v13h-17zM12.5 12h6v4.5h-6z',
  home: 'M4 10.5 12 4l8 6.5V20h-5.5v-6h-5v6H4Z',
  // Brújula: explorar lo público.
  explorar: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18ZM15.5 8.5l-2 5-5 2 2-5z',
  // Emitir: el punto central con dos pares de ondas. Es la misma idea que el
  // glifo de marca de Cast, en trazo de interfaz.
  emitir:
    'M12 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3ZM8.5 8.5a5 5 0 0 0 0 7M15.5 8.5a5 5 0 0 1 0 7M5.6 5.6a9 9 0 0 0 0 12.8M18.4 5.6a9 9 0 0 1 0 12.8',
  video: 'M3.5 6.5h12v11h-12zM15.5 10.5l5-3v9l-5-3',
  ojo: 'M2.5 12s3.5-6.5 9.5-6.5 9.5 6.5 9.5 6.5-3.5 6.5-9.5 6.5S2.5 12 2.5 12ZM12 9.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5Z',
  ojoOff:
    'M10 5.7A9.5 9.5 0 0 1 12 5.5c6 0 9.5 6.5 9.5 6.5a17 17 0 0 1-2.4 3.2M6.3 6.8C3.9 8.4 2.5 12 2.5 12s3.5 6.5 9.5 6.5a9 9 0 0 0 4.7-1.3M10.2 10.2a2.5 2.5 0 0 0 3.6 3.6M4 4l16 16',
} as const;

export type IconName = keyof typeof PATHS;

/** Lo de `Glifo` (tamaño, etiqueta, atributos del `<svg>`) con el dibujo por NOMBRE. */
export interface IconProps extends Omit<GlifoProps, 'd'> {
  name: IconName;
}

export function Icon({ name, ...rest }: IconProps): JSX.Element {
  return <Glifo d={PATHS[name]} {...rest} />;
}
