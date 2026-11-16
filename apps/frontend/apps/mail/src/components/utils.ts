/**
 * Utilidades de presentacion para Sytue Mail. Todo lo que sale a pantalla —fechas,
 * tamaños, nombres de carpeta— depende del idioma, así que estas funciones reciben
 * el `locale` ya negociado (o la `t` de la app) en vez de fijar un formato.
 */
import { formatDate, formatDateTime, formatTime, intlTag, type Locale } from '@sytue/i18n';
import { PALETA_DE_USUARIO, appUrl, colorDeUsuario } from '@sytue/config';
import type { GlyphName } from './glyphs.js';
import type { MailMessageKey, T } from '../i18n/index.js';
import type { EmailAddress, EmailChange, EmailSummary } from '../client/mailApi.js';

/** Fecha relativa compacta para la lista (hoy -> hora; este año -> dia/mes; resto -> con año). */
export function shortDate(locale: Locale, iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return formatTime(locale, d);
  return d.getFullYear() === now.getFullYear()
    ? formatDate(locale, d, { day: '2-digit', month: 'short' })
    : formatDate(locale, d, { day: '2-digit', month: 'short', year: 'numeric' });
}

export function fullDate(locale: Locale, iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? ''
    : formatDateTime(locale, d, { dateStyle: 'long', timeStyle: 'short' });
}

/**
 * Formatea una fecha para un <input type="datetime-local"> (hora LOCAL, sin zona,
 * «YYYY-MM-DDTHH:mm»): compensa el offset de zona antes de recortar el ISO (UTC).
 * Se usa para el valor por defecto y el `min` del selector de programar envío.
 */
export function toLocalInput(d: Date): string {
  const local = new Date(d.getTime() - d.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

/** El tamaño es el `formatBytes` de la suite; se reexporta para no tocar a quien lo importa de aquí. */
export { formatBytes } from '@sytue/ui';

/** Nombre visible del primer remitente (o su correo). */
export function displayName(t: T, addrs: EmailAddress[]): string {
  const a = addrs[0];
  if (!a) return t('reader.unknownSender');
  return a.name?.trim() || a.email;
}

export function joinAddresses(addrs: EmailAddress[]): string {
  return addrs.map((a) => a.name?.trim() || a.email).join(', ');
}

/** Dirección formateada para el campo Para/CC del compositor: "Nombre <a@b>" o "a@b". */
export function addressToField(a: EmailAddress): string {
  return a.name?.trim() ? `${a.name.trim()} <${a.email}>` : a.email;
}

/** Borrador que rellena el compositor (Responder/Reenviar o redacción nueva). */
export interface ComposeDraft {
  to?: string;
  cc?: string;
  /** CCO inicial: solo la trae un borrador reabierto (al responder no hay). */
  bcc?: string;
  subject?: string;
  body?: string;
  /** Formato enriquecido del cuerpo, si el borrador reabierto lo traía. Sin él
   *  se convierte `body` (texto plano) a HTML, como hasta ahora. */
  htmlBody?: string;
  /** Ficheros del borrador reabierto, ya en base64 (mismo shape que se manda
   *  al guardar). Ausente = el compositor arranca sin adjuntos, como hasta ahora. */
  attachments?: { name: string; type: string; contentBase64: string; contentId?: string }[];
  /**
   * «De» inicial (enviar-como / en-nombre-de): al responder desde un buzón
   * compartido/delegado, se preselecciona esa dirección en el selector. Solo surte
   * efecto si está entre las opciones «De» disponibles; el servidor la verifica.
   */
  from?: string;
  /**
   * El correo de Sytue al que se contesta («Responder» / «Responder a todos»). Con él,
   * cuando el envío sale de verdad, el original se marca `$answered` (`\Answered` en
   * IMAP): así Thunderbird o el iPhone también lo ven respondido. Un reenvío no lo lleva.
   */
  answers?: string;
}

/**
 * Conversación (hilo): los mensajes de un mismo `threadId` de la lista visible,
 * más recientes primero. El backend ya agrupa por hilo (columna thread_id); aquí
 * se pliega la lista plana en conversaciones para la vista estilo Gmail.
 */
export interface Conversation {
  threadId: string;
  /** Mensajes del hilo, más recientes primero (orden de la lista). */
  messages: EmailSummary[];
  /** Mensaje más reciente: define remitente/asunto/fecha de la fila y las acciones. */
  latest: EmailSummary;
  /**
   * Mensajes del hilo ENTERO (el listado los cuenta en toda la cuenta), que es lo
   * que va a abrir el lector. Nunca menos que los que ya se ven plegados en esta
   * carpeta: si el servidor no lo manda —lecturas que no son el listado—, cae a
   * `messages.length` y la fila sigue diciendo algo cierto.
   */
  total: number;
  /** Algún mensaje sin leer / destacado / con adjunto (estado agregado del hilo). */
  unread: boolean;
  flagged: boolean;
  hasAttachment: boolean;
  /**
   * El mensaje más reciente está respondido (`$answered`). Es del ÚLTIMO y no de
   * cualquiera, como la flecha de Apple Mail: si después de contestar llegó otro, el
   * hilo vuelve a estar pendiente de respuesta.
   */
  answered: boolean;
}

/**
 * Agrupa una lista plana (más recientes primero) en conversaciones por threadId,
 * preservando el orden de aparición (el hilo se sitúa por su mensaje más reciente,
 * como Gmail). Los mensajes sin threadId se tratan como hilos de uno (clave = id)
 * para no fusionarlos todos bajo una clave vacía.
 */
export function groupByThread(emails: EmailSummary[]): Conversation[] {
  const byThread = new Map<string, Conversation>();
  const order: string[] = [];
  for (const e of emails) {
    const key = e.threadId || `msg:${e.id}`;
    let conv = byThread.get(key);
    if (!conv) {
      conv = {
        threadId: key,
        messages: [],
        latest: e,
        total: 0,
        unread: false,
        flagged: false,
        hasAttachment: false,
        answered: Boolean(e.keywords.$answered),
      };
      byThread.set(key, conv);
      order.push(key);
    }
    conv.messages.push(e);
    conv.total = Math.max(conv.total, conv.messages.length, e.threadCount ?? 0);
    if (!e.keywords.$seen) conv.unread = true;
    if (e.keywords.$flagged) conv.flagged = true;
    if (e.hasAttachment) conv.hasAttachment = true;
  }
  return order.map((k) => byThread.get(k)!);
}

/**
 * Funde el hilo COMPLETO que devuelve el servidor (atraviesa carpetas) con la
 * conversación que la vista ya tiene plegada de la carpeta actual. Devuelve los
 * mensajes a PINTAR, en el mismo orden reciente→antiguo que llega del servidor.
 *
 * Dos reglas, y las dos importan:
 *  - de los mensajes que ya están en la vista se conserva la copia LOCAL, que
 *    lleva las marcas optimistas (leído, destacado, etiqueta) que el servidor
 *    todavía no ha visto; si no, abrir un hilo revertiría lo que acabas de hacer;
 *  - los que viven en una carpeta `hidden` (Papelera, Spam) NO se traen: lo que
 *    tiraste no reaparece dentro de un hilo de Recibidos. Si ya estaba en la
 *    vista, se queda: significa que estás EN esa carpeta.
 */
export function mergeThread(
  conv: Conversation,
  thread: EmailChange[],
  hidden: ReadonlySet<string>,
): EmailSummary[] {
  const local = new Map(conv.messages.map((m) => [m.id, m]));
  return thread
    .filter((m) => local.has(m.id) || !hidden.has(m.mailboxId))
    .map((m) => local.get(m.id) ?? m);
}

/**
 * La conversación que pinta el lector. Casi siempre es una fila de la lista; pero
 * quien llega por ENLACE —un correo encontrado en el buscador global— puede abrir un
 * hilo que no está en la carpeta visible, y entonces se arma con el hilo completo que
 * ya se pidió al servicio. Sin esto, abrir un correo desde el buscador no abría nada.
 */
export function conversacionAbierta(
  conversations: Conversation[],
  threadId: string | null,
  hilo: { id: string; messages: EmailChange[] } | null,
): Conversation | null {
  if (!threadId) return null;
  const enLista = conversations.find((c) => c.threadId === threadId);
  if (enLista || hilo?.id !== threadId || hilo.messages.length === 0) return enLista ?? null;
  // La conversación va de más reciente a más antiguo, como la lista.
  const recientesPrimero = [...hilo.messages].sort((a, b) =>
    fechaDeOrden(b).localeCompare(fechaDeOrden(a)),
  );
  return groupByThread(recientesPrimero)[0] ?? null;
}

/**
 * Los mensajes de la conversación que están en Recibidos (`entradaId`): lo que
 * silenciar archiva, y por tanto lo único que su «Deshacer» debe devolver a la
 * Entrada. Una conversación abierta desde una búsqueda o desde el hilo completo trae
 * también lo enviado y lo archivado de antes, y meterlo en la Entrada sería inventar.
 * Sin `mailboxId` (lecturas que no lo traen) vale la carpeta vista: `enEntrada`.
 */
export function idsDeLaEntrada(
  conv: Pick<Conversation, 'messages'>,
  entradaId: string,
  enEntrada: boolean,
): string[] {
  return conv.messages
    .filter((m) => (m.mailboxId ? m.mailboxId === entradaId : enEntrada))
    .map((m) => m.id);
}

/**
 * «Deshacer» un movimiento: los mensajes movidos agrupados por la carpeta de la que
 * SALIERON, para devolver cada uno a la suya en una llamada por carpeta.
 *
 * Volver todos a la carpeta de la vista no vale: en una búsqueda o en una etiqueta
 * la vista cruza carpetas, y una conversación tiene a la vez mensajes en Recibidos y
 * en Enviados. Lo que manda es el `mailboxId` de cada mensaje; sin él (lecturas que
 * no lo traen) se cae a la carpeta que se estaba mirando, que en una carpeta es
 * exactamente de donde salió. Quedan fuera los que ya estaban en el destino —no se
 * movieron— y los que no tienen origen conocido: devolverlos a ciegas es inventar.
 */
export function origenesDeMovimiento(
  convs: Conversation[],
  carpetaVista: string | null,
  destino: string,
): Map<string, string[]> {
  const origenes = new Map<string, string[]>();
  for (const c of convs) {
    for (const m of c.messages) {
      const origen = m.mailboxId || carpetaVista;
      if (!origen || origen === destino) continue;
      const ids = origenes.get(origen) ?? [];
      ids.push(m.id);
      origenes.set(origen, ids);
    }
  }
  return origenes;
}

/** Filtros rápidos de la lista: todas, sin leer, destacadas o con adjuntos. */
export const FILTROS_RAPIDOS = ['todas', 'noLeidas', 'destacadas', 'conAdjuntos'] as const;
export type FiltroRapido = (typeof FILTROS_RAPIDOS)[number];

/**
 * Las conversaciones que deja ver un filtro rápido. Se filtra lo YA cargado —la
 * lista sigue pidiendo páginas al bajar—, sin otra consulta al servicio.
 *
 * La conversación ABIERTA se queda aunque deje de cumplirlo: abrir una sin leer la
 * marca leída, y que desaparezca de «No leídos» bajo el cursor —con j/k y la
 * selección contando por posición— es perder el sitio. Sale al abrir otra.
 */
export function filtrarConversaciones(
  convs: Conversation[],
  filtro: FiltroRapido,
  abierta: string | null,
): Conversation[] {
  if (filtro === 'todas') return convs;
  const cumple = (c: Conversation): boolean =>
    filtro === 'noLeidas' ? c.unread : filtro === 'destacadas' ? c.flagged : c.hasAttachment;
  return convs.filter((c) => c.threadId === abierta || cumple(c));
}

/** Las páginas que un filtro sin coincidencias pide por su cuenta antes de parar. */
export const TOPE_PAGINAS_FILTRO = 5;

/**
 * Qué hace la lista con un filtro rápido puesto, según lo que deja ver: `reiniciar`
 * la cuenta si ya enseña algo (o no queda nada que pedir); `esperar` si hay una
 * página en camino —no se pide otra ni se cuenta: contar repintados gastaría el
 * tope sin pedir—; `pedir` la siguiente; o `tope` tras TOPE_PAGINAS_FILTRO
 * seguidas, y el vacío ofrece «Cargar más». Es el final de `pintarLista` en Fyne.
 */
export function pasoDelFiltroVacio(ctx: {
  filtroSinNada: boolean;
  cargandoMas: boolean;
  paginas: number;
}): 'reiniciar' | 'esperar' | 'pedir' | 'tope' {
  if (!ctx.filtroSinNada) return 'reiniciar';
  if (ctx.cargandoMas) return 'esperar';
  return ctx.paginas >= TOPE_PAGINAS_FILTRO ? 'tope' : 'pedir';
}

/**
 * Los campos del panel «Opciones de búsqueda». `desde` y `hasta` son AAAA-MM-DD y
 * los DOS inclusivos, que es lo que entiende quien rellena «Hasta el 28»; vacíos,
 * sin acotar.
 *
 * POR QUÉ EXISTE: el servicio entiende `from:`, `to:`, `subject:`, `has:attachment`,
 * `is:unread`, `after:` y `before:` (services/mail/internal/domain/searchquery.go),
 * pero eso solo lo decía un `title` en inglés técnico, que no llega ni a la pantalla
 * táctil ni al lector de pantalla. El panel pregunta en lenguaje llano y escribe la
 * consulta; los operadores no se traducen porque son los que entiende el servicio.
 */
export interface OpcionesBusqueda {
  de: string;
  para: string;
  asunto: string;
  contiene: string;
  conAdjuntos: boolean;
  sinLeer: boolean;
  desde: string;
  hasta: string;
}

export const OPCIONES_BUSQUEDA_VACIAS: OpcionesBusqueda = {
  de: '',
  para: '',
  asunto: '',
  contiene: '',
  conAdjuntos: false,
  sinLeer: false,
  desde: '',
  hasta: '',
};

/** Palabras sueltas: sin espacios de más, como `strings.Fields` en Fyne. */
function palabras(s: string): string {
  return s.split(/\s+/).filter(Boolean).join(' ');
}

/** Lo escrito en un campo, listo para ir detrás de `op:`. */
function valorDeOperador(v: string): string {
  const limpio = palabras(v.replaceAll('"', ''));
  return limpio.includes(' ') ? `"${limpio}"` : limpio;
}

/** AAAA-MM-DD con el día de verdad (`2026-02-30` no vale), desplazado `dias`. */
function leerDia(s: string, dias = 0): string | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s.trim());
  if (!m) return null;
  const [a, mes, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
  // `setUTCFullYear` y no `Date.UTC`, que lleva los años 0–99 al siglo XX.
  const fecha = new Date(0);
  fecha.setUTCFullYear(a, mes - 1, d);
  if (fecha.getUTCFullYear() !== a || fecha.getUTCMonth() !== mes - 1 || fecha.getUTCDate() !== d) {
    return null;
  }
  fecha.setUTCDate(fecha.getUTCDate() + dias);
  return fecha.toISOString().slice(0, 10);
}

/**
 * La consulta que entiende el servicio a partir del panel. Es `ComponerConsulta` de
 * `apps/fyne/apps/mail/consulta.go`; los casos, en
 * `apps/fyne/apps/mail/testdata/consulta-busqueda.json`.
 *
 * - Un valor con espacios va entre comillas (`subject:"informe anual"`), y las que
 *   traiga dentro se quitan: el servicio no tiene cómo escaparlas.
 * - `before:` EXCLUYE su día, así que «Hasta el 28 de febrero» es
 *   `before:2026-03-01`: escrito tal cual, ese día se quedaría fuera.
 * - Una fecha que no sea AAAA-MM-DD válida no se escribe: el servicio la tomaría por
 *   texto y buscaría la palabra.
 */
export function componerConsulta(o: OpcionesBusqueda): string {
  const partes: string[] = [];
  for (const [op, valor] of [
    ['from', o.de],
    ['to', o.para],
    ['subject', o.asunto],
  ] as const) {
    const v = valorDeOperador(valor);
    if (v) partes.push(`${op}:${v}`);
  }
  if (o.conAdjuntos) partes.push('has:attachment');
  if (o.sinLeer) partes.push('is:unread');
  const desde = leerDia(o.desde);
  if (desde) partes.push(`after:${desde}`);
  const hasta = leerDia(o.hasta, 1);
  if (hasta) partes.push(`before:${hasta}`);
  const texto = palabras(o.contiene);
  if (texto) partes.push(texto);
  return partes.join(' ');
}

/**
 * Parte la consulta como `tokenizeQuery` del servicio: por espacios, tabuladores y
 * saltos fuera de las comillas, que se conservan; una comilla sin cerrar llega hasta
 * el final. `entrecomillado` es la frase que venía ENTERA entre comillas: texto,
 * aunque lleve dos puntos.
 */
function trocearConsulta(q: string): { texto: string; entrecomillado: boolean }[] {
  const out: { texto: string; entrecomillado: boolean }[] = [];
  let cur = '';
  let enComillas = false;
  let empezoConComilla = false;
  const soltar = (): void => {
    if (cur) out.push({ texto: cur, entrecomillado: empezoConComilla });
    cur = '';
    empezoConComilla = false;
  };
  for (const c of q) {
    if (c === '"') {
      if (!enComillas && !cur) empezoConComilla = true;
      enComillas = !enComillas;
      cur += c;
    } else if ((c === ' ' || c === '\t' || c === '\n') && !enComillas) {
      soltar();
    } else {
      cur += c;
    }
  }
  soltar();
  return out;
}

/**
 * Lo contrario de componerConsulta: al reabrir el panel con una consulta escrita,
 * cada operador vuelve a su campo. Lee como `ParseSearchQuery` —sin distinguir
 * mayúsculas en el operador, con sus sinónimos y ganando el último repetido— y lo que
 * el panel no tiene (`is:starred`, un operador que no existe, una fecha mal escrita)
 * se queda en «Contiene» tal cual, para que aplicar el panel no lo borre.
 */
export function desmontarConsulta(q: string): OpcionesBusqueda {
  const o: OpcionesBusqueda = { ...OPCIONES_BUSQUEDA_VACIAS };
  const libre: string[] = [];
  for (const tok of trocearConsulta(q)) {
    const i = tok.texto.indexOf(':');
    const op = tok.texto.slice(0, i).toLowerCase();
    const val = tok.texto.slice(i + 1).replace(/^"+|"+$/g, '');
    if (i < 0 || tok.entrecomillado || i === tok.texto.length - 1) {
      libre.push(tok.texto);
      continue;
    }
    let entendido = true;
    if (op === 'from') o.de = val;
    else if (op === 'to') o.para = val;
    else if (op === 'subject') o.asunto = val;
    else if (op === 'has') {
      entendido = /^attachments?$/i.test(val);
      o.conAdjuntos ||= entendido;
    } else if (op === 'is') {
      entendido = /^(unread|unseen)$/i.test(val);
      o.sinLeer ||= entendido;
    } else if (op === 'after' || op === 'since') {
      const d = leerDia(val);
      entendido = d !== null;
      if (d) o.desde = d;
    } else if (op === 'before' || op === 'until') {
      const d = leerDia(val, -1);
      entendido = d !== null;
      if (d) o.hasta = d;
    } else entendido = false;
    if (!entendido) libre.push(tok.texto);
  }
  o.contiene = libre.join(' ');
  return o;
}

/**
 * La fecha por la que se ordena la lista, como en el servicio: lo que volvió de
 * «Pospuestos» al vencer va a la hora de VUELTA (arriba de Recibidos, como en Gmail);
 * lo demás, a la de llegada. Es también la que enseña la fila.
 */
export function fechaDeOrden(e: Pick<EmailSummary, 'receivedAt' | 'unsnoozedAt'>): string {
  return e.unsnoozedAt ?? e.receivedAt;
}

/** Orden de la lista: lo más reciente arriba; a igual fecha, por id (estable). */
export function masReciente(a: EmailSummary, b: EmailSummary): number {
  return fechaDeOrden(b).localeCompare(fechaDeOrden(a)) || b.id.localeCompare(a.id);
}

/**
 * La lista tras pedir otra vez su primera página («tirar para actualizar»). Si solo
 * se había cargado esa página, manda la fresca entera. Si ya se habían bajado más
 * (scroll infinito), lo más viejo que la página fresca no alcanza se conserva con su
 * cursor: cambiarlo por la primera página sola tiraría lo leído al bajar y cerraría
 * la conversación abierta si estaba ahí abajo. Lo que la página fresca cubre lo
 * dice ella: lo borrado o movido desaparece y lo nuevo entra arriba.
 */
export function fusionarPrimeraPagina(
  previa: { emails: EmailSummary[]; cursor: string },
  fresca: { emails: EmailSummary[]; cursor: string },
): { emails: EmailSummary[]; cursor: string } {
  const ultima = fresca.emails[fresca.emails.length - 1];
  // Sin más páginas, la fresca es la carpeta entera; vacía, no hay borde que comparar.
  if (!fresca.cursor || !ultima) return fresca;
  const ids = new Set(fresca.emails.map((e) => e.id));
  const cola = previa.emails.filter((e) => !ids.has(e.id) && masReciente(ultima, e) < 0);
  if (cola.length === 0) return fresca;
  return { emails: [...fresca.emails, ...cola].sort(masReciente), cursor: previa.cursor };
}

/** Lo que hace un atajo de la selección: marcar una fila o vaciarla. */
export type AccionSeleccion =
  { tipo: 'marcar'; threadId: string; indice: number } | { tipo: 'vaciar' } | null;

/**
 * «x» y Escape sobre la lista. «x» marca la conversación del cursor: la abierta —y
 * si la abierta no está en la lista (abierta desde un aviso), nada— o la fila donde
 * se quedó j/k. Escape vacía la selección, y SOLO si hay algo marcado: sin selección
 * la tecla sigue siendo de quien la escuche. Es `marcarCursor`/`TeclaSuelta` de Fyne.
 */
export function atajoDeSeleccion(
  tecla: string,
  ctx: {
    conversaciones: Conversation[];
    abierta: string | null;
    cursor: number;
    soloLectura: boolean;
    marcadas: number;
  },
): AccionSeleccion {
  if (tecla === 'Escape') return ctx.marcadas > 0 ? { tipo: 'vaciar' } : null;
  if (tecla !== 'x' || ctx.soloLectura || ctx.conversaciones.length === 0) return null;
  const indice = ctx.abierta
    ? ctx.conversaciones.findIndex((c) => c.threadId === ctx.abierta)
    : Math.min(ctx.cursor, ctx.conversaciones.length - 1);
  const fila = ctx.conversaciones[indice];
  return fila ? { tipo: 'marcar', threadId: fila.threadId, indice } : null;
}

/** Lo que hace un atajo de triaje de Gmail: leída, no leída, spam o deshacer. */
export type AccionTriaje =
  { tipo: 'leida' | 'noLeida' | 'spam'; conversacion: Conversation } | { tipo: 'deshacer' } | null;

/**
 * `Mayús+I` (marcar leída), `Mayús+U` (marcar no leída), «!» (spam) y «z» (deshacer el
 * último triaje, el «Deshacer» del aviso). Las tres primeras actúan sobre la abierta o,
 * sin nada abierto, sobre la fila donde se quedó j/k, como «x». Marcar leída la que ya
 * lo está —o no leída la que no— no es un atajo: la tecla sigue su camino en vez de
 * cerrar el hilo o pedir al servidor lo que ya hay. En solo lectura no hay triaje. Es
 * `atajoDeTriaje` de Fyne (atajos.go).
 */
export function atajoDeTriaje(
  tecla: string,
  ctx: {
    conversaciones: Conversation[];
    abierta: Conversation | null;
    cursor: number;
    soloLectura: boolean;
    hayDeshacer: boolean;
  },
): AccionTriaje {
  if (ctx.soloLectura) return null;
  if (tecla === 'z') return ctx.hayDeshacer ? { tipo: 'deshacer' } : null;
  const tipo = tecla === 'I' ? 'leida' : tecla === 'U' ? 'noLeida' : tecla === '!' ? 'spam' : null;
  if (!tipo) return null;
  const n = ctx.conversaciones.length;
  const conversacion =
    ctx.abierta ?? (n > 0 ? ctx.conversaciones[Math.min(ctx.cursor, n - 1)] : null);
  if (!conversacion) return null;
  if (tipo === 'leida' && !conversacion.unread) return null;
  if (tipo === 'noLeida' && conversacion.unread) return null;
  return { tipo, conversacion };
}

/**
 * «Deshacer» del aviso de triaje: una llamada en lote por carpeta de origen
 * (`origenesDeMovimiento`) y, salga bien o mal, se piden de nuevo la carpeta, la
 * búsqueda y los contadores. Las dos listas perdieron los movidos de forma
 * optimista; si solo se pidiera la carpeta, en una búsqueda lo devuelto no volvía
 * hasta repetirla (la búsqueda vacía no consulta, así que pedirla siempre es gratis).
 * Devuelve si todas las carpetas aceptaron el movimiento.
 */
export async function deshacerMovimiento(
  origenes: Map<string, string[]>,
  mover: (ids: string[], carpeta: string) => Promise<unknown>,
  refrescar: { carpeta: () => void; busqueda: () => void; contadores: () => void },
): Promise<boolean> {
  try {
    await Promise.all(Array.from(origenes, ([carpeta, ids]) => mover(ids, carpeta)));
    return true;
  } catch {
    return false;
  } finally {
    refrescar.carpeta();
    refrescar.busqueda();
    refrescar.contadores();
  }
}

/**
 * Abre en otra pestaña una URL que solo se conoce tras una petición (el deep-link de
 * un intent del Hub). La pestaña se abre YA, en el mismo clic, y se le da la URL al
 * llegar: después del `await` el navegador puede haber retirado el permiso del clic,
 * y Safari —o Firefox si el Hub tarda— bloquea la ventana sin avisar (con `noopener`
 * `window.open` siempre devuelve null, así que el bloqueo ni se detectaba).
 *
 * Si aun así la bloquea, se navega en esta pestaña, como hace Drive: perder el
 * correo es mejor que un botón que no hace nada. Sin URL o con error, la pestaña en
 * blanco se cierra. Solo se navega a http(s): la pestaña vacía es de nuestro origen,
 * y un `javascript:` en ella correría con él. Devuelve si se navegó.
 */
export async function abrirTrasPeticion(
  pedir: () => Promise<string | null | undefined>,
  win: Pick<Window, 'open' | 'location'> = window,
): Promise<boolean> {
  const pestana = win.open('', '_blank');
  if (pestana) pestana.opener = null;
  try {
    const url = await pedir();
    const destino = url ? new URL(url, win.location.href) : null;
    if (!destino || (destino.protocol !== 'https:' && destino.protocol !== 'http:')) {
      pestana?.close();
      return false;
    }
    if (pestana) pestana.location.href = destino.href;
    else win.location.assign(destino.href);
    return true;
  } catch (e) {
    pestana?.close();
    throw e;
  }
}

/**
 * El mensaje que se propone en Sytue Rooms al compartir una conversación: el asunto y
 * el ENLACE canónico del correo (`/messages/{id}`, que abre su hilo), nunca el cuerpo
 * —la sala puede tener gente que no debería leerlo, y quien pulse el enlace pasa por
 * la autorización de Mail—. Con la organización de la ruta (ADR 0056) el enlace
 * aterriza sin el salto del middleware.
 */
export function textoParaRooms(asunto: string, id: string, org: string | null): string {
  return `${asunto}\n${appUrl('mail', `/messages/${encodeURIComponent(id)}`, org)}`;
}

export type ReplyMode = 'reply' | 'replyAll' | 'forward';

const RE_PREFIX = /^\s*re:/i;
const FWD_PREFIX = /^\s*(fwd?|rv):/i;

/** Cabecera de atribución + cuerpo original citado con "> " por línea. */
function quotedBody(t: T, locale: Locale, email: EmailSummary, fullText?: string): string {
  // Cita el cuerpo completo si el lector ya lo cargó (GET /api/body); si aún no
  // llegó (o falló), degrada al extracto del sobre (~280 chars del servidor).
  // La cabecera es UNA cadena del catálogo: el orden «fecha / quién / escribió»
  // cambia entre lenguas y concatenarla a trozos la rompería.
  const header = t('reader.quoteHeader', {
    date: fullDate(locale, email.receivedAt),
    name: displayName(t, email.from),
  });
  const cited = (fullText || email.preview || '')
    .split('\n')
    .map((line) => `> ${line}`)
    .join('\n');
  return `\n\n${header}\n${cited}`;
}

/**
 * Las cabeceras de atribución de `reader.quoteHeader` en los 26 idiomas de Mail.
 *
 * Están aquí, y no se leen del catálogo, porque el navegador baja SOLO el catálogo de
 * su idioma, y un correo citado en sueco llega igual a quien lee en castellano. Que no
 * se desvíen lo guarda `utils.test.ts`, que las compara con los 26 catálogos.
 */
export const ATRIBUCIONES_DE_CITA: readonly string[] = [
  'On {date}, {name} wrote:',
  'На {date} {name} написа:',
  '{date}, {name} píše:',
  'Den {date} skrev {name}:',
  'Am {date} schrieb {name}:',
  'Στις {date}, {name} έγραψε:',
  'El {date}, {name} escribió:',
  '{date} kirjutas {name}:',
  '{name} kirjoitti {date}:',
  'Le {date}, {name} a écrit :',
  'Ar {date}, scríobh {name}:',
  '{date}, {name} piše:',
  '{name} ezt írta ({date}):',
  '{date} skrifaði {name}:',
  'Il {date}, {name} ha scritto:',
  '{date}, {name} rašė:',
  '{date} {name} rakstīja:',
  'Fl-{date}, messaġġ minn {name}:',
  '{date} skrev {name}:',
  'Op {date} schreef {name}:',
  '{date}, {name} pisze:',
  'Em {date}, {name} escreveu:',
  'Pe {date}, {name} a scris:',
];

/*
 * Cada plantilla se vuelve una expresión que acepta lo que de verdad escriben los
 * clientes: los huecos son «cualquier cosa», los espacios pueden faltar o sobrar y la
 * coma es opcional —Gmail escribe «On Mon, Jan 5, 2026 at 10:00 Ana <a@b.eu> wrote:»,
 * sin la coma de la plantilla—. La misma traducción hace `cita.go`.
 */
function expresionDeAtribucion(plantilla: string): RegExp {
  const partes = plantilla.split(/\{(?:date|name)\}/);
  // Se escapa primero: ni el espacio ni la coma son metacaracteres, así que los
  // reemplazos de después no tocan nada escapado.
  const literal = (s: string) =>
    s
      .replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      .replace(/\s+/gu, ' ?')
      .replace(/,/g, ',?');
  return new RegExp(`^${partes.map(literal).join('.+')}$`, 'iu');
}

const EXPRESIONES_DE_ATRIBUCION = ATRIBUCIONES_DE_CITA.map(expresionDeAtribucion);

/** El separador con que Outlook encabeza el mensaje respondido. */
const SEPARADOR_ORIGINAL = /^-{2,}\s*original message\s*-{2,}$/i;

/*
 * Una atribución de verdad cabe en un par de líneas (Gmail parte la suya a las 78
 * columnas). El tope no es estético: con los huecos `.+` seguidos, una línea larga que
 * no casa —un enlace o un párrafo sin saltos justo encima de la cita— hace que la
 * expresión pruebe todos los repartos posibles, y con 8 000 caracteres la lectura del
 * correo se colgaba segundos. `cita.go` aplica el mismo tope, contado igual (en unidades UTF-16).
 */
const LARGO_MAX_ATRIBUCION = 256;

const esLineaCitada = (l: string) => l.trimStart().startsWith('>');
const esAtribucion = (l: string) => {
  const s = l.trim();
  if (s.length > LARGO_MAX_ATRIBUCION) return false;
  // Los espacios se juntan en uno antes de probar, y la plantilla los pide como « ?»:
  // con `\s*` entre dos huecos `.+`, que también se comen espacios, una racha de
  // espacios de 250 caracteres aún tardaba más de un segundo dentro del tope.
  const junta = s.replace(/\s+/gu, ' ');
  return EXPRESIONES_DE_ATRIBUCION.some((e) => e.test(junta));
};

/**
 * Separa lo nuevo de una respuesta del historial que arrastra, como el «•••» de Gmail.
 *
 * La cita es lo que va desde el separador de Outlook, o el BLOQUE FINAL de líneas `>`
 * con su línea de atribución delante (que puede venir partida en dos, como la parte
 * Gmail al pasar de 78 columnas). Solo el bloque final: las respuestas intercaladas
 * entre trozos citados son la respuesta, y plegarlas escondería lo que se contesta.
 *
 * Si no queda nada nuevo —un mensaje que es solo cita—, no se pliega: esconderlo
 * todo tras un botón dejaría la tarjeta vacía. Gemela de `separarCita` en
 * `apps/fyne/apps/mail/cita.go`; los casos, en `apps/fyne/apps/mail/testdata/cita.json`.
 */
export function separarCita(texto: string): { cuerpo: string; cita: string } {
  const lineas = texto.replace(/\r\n/g, '\n').split('\n');
  let corte = lineas.findIndex((l) => SEPARADOR_ORIGINAL.test(l.trim()));
  if (corte < 0) corte = lineas.length;

  let i = lineas.length - 1;
  while (i >= 0 && lineas[i]!.trim() === '') i--;
  if (i >= 0 && esLineaCitada(lineas[i]!)) {
    let inicio = i;
    for (let j = i; j >= 0 && (esLineaCitada(lineas[j]!) || lineas[j]!.trim() === ''); j--) {
      if (esLineaCitada(lineas[j]!)) inicio = j;
    }
    let a = inicio - 1;
    while (a >= 0 && lineas[a]!.trim() === '') a--;
    if (a >= 0 && esAtribucion(lineas[a]!)) inicio = a;
    else if (a >= 1 && esAtribucion(`${lineas[a - 1]!} ${lineas[a]!}`)) inicio = a - 1;
    corte = Math.min(corte, inicio);
  }

  const cuerpo = lineas.slice(0, corte).join('\n').trimEnd();
  if (cuerpo.trim() === '') return { cuerpo: texto, cita: '' };
  return { cuerpo, cita: lineas.slice(corte).join('\n') };
}

/**
 * Construye el borrador para Responder / Responder a todos / Reenviar a partir del
 * correo leído: destinatarios, asunto con prefijo (Re:/Fwd:, sin duplicar) y cuerpo
 * citado. `selfEmail` es la dirección del usuario, para no incluirse en el CC al
 * responder a todos. No hay backend nuevo: alimenta el flujo /api/send existente.
 */
export function buildReplyDraft(
  t: T,
  locale: Locale,
  email: EmailSummary,
  mode: ReplyMode,
  selfEmail?: string,
  fullText?: string,
): ComposeDraft {
  const subject = email.subject ?? '';
  // «Re:» y «Fwd:» NO se traducen: son los prefijos de asunto que entienden todos
  // los clientes de correo (y con los que agrupan el hilo).
  if (mode === 'forward') {
    return {
      to: '',
      subject: FWD_PREFIX.test(subject) ? subject : `Fwd: ${subject}`,
      body: quotedBody(t, locale, email, fullText),
    };
  }
  const draft: ComposeDraft = {
    to: email.from.map(addressToField).join(', '),
    subject: RE_PREFIX.test(subject) ? subject : `Re: ${subject}`,
    body: quotedBody(t, locale, email, fullText),
    answers: email.id,
  };
  if (mode === 'replyAll') {
    const self = (selfEmail ?? '').trim().toLowerCase();
    const senders = new Set(email.from.map((a) => a.email.toLowerCase()));
    // CC = resto de destinatarios originales (To + Cc) menos yo y el remitente,
    // sin duplicar. Así "Responder a todos" mantiene a los demás en copia.
    const seen = new Set<string>();
    const cc = [...email.to, ...email.cc]
      .filter((a) => {
        const e = a.email.toLowerCase();
        if (e === self || senders.has(e) || seen.has(e)) return false;
        seen.add(e);
        return true;
      })
      .map(addressToField)
      .join(', ');
    if (cc) draft.cc = cc;
  }
  return draft;
}

/** Parsea una lista de direcciones "Nombre <a@b>, c@d" a EmailAddress[]. */
export function parseAddresses(raw: string): EmailAddress[] {
  return raw
    .split(/[,;]/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((token): EmailAddress => {
      const match = token.match(/^(.*?)\s*<([^>]+)>$/);
      if (match?.[2]) {
        const name = (match[1] ?? '').trim();
        const email = match[2].trim();
        return name ? { name, email } : { email };
      }
      return { email: token };
    });
}

/**
 * Nombre amigable de carpeta segun su role JMAP (o el nombre crudo). Las carpetas
 * con role son las del sistema y se traducen; una carpeta creada por el usuario
 * conserva el nombre que le puso.
 */
const ROLE_KEYS = {
  inbox: 'folder.inbox',
  sent: 'folder.sent',
  drafts: 'folder.drafts',
  trash: 'folder.trash',
  junk: 'folder.junk',
  archive: 'folder.archive',
  outbox: 'folder.outbox',
  snoozed: 'folder.snoozed',
} as const satisfies Record<string, MailMessageKey>;

export function mailboxLabel(t: T, name: string, role?: string): string {
  const key = role ? ROLE_KEYS[role as keyof typeof ROLE_KEYS] : undefined;
  return key ? t(key) : name;
}

/**
 * Orden de prioridad de carpetas por role JMAP (resto, alfabético). Es también el de
 * los buzones unificados del lateral (`arbolDeBuzones`).
 */
export const ROLE_ORDER = ['inbox', 'drafts', 'sent', 'archive', 'junk', 'trash'] as const;

/**
 * Carpetas en el orden del lateral: las del sistema por su role y las demás por
 * nombre. Vale igual para las de Sytue y para las de una cuenta externa, que llegan
 * en el orden del proveedor —Gmail da la Papelera antes que Recibidos— y con el mismo
 * vocabulario de roles (`services/integrations/internal/mailbox`). Los casos que
 * comparte con Fyne (`ordenarCarpetas`) están en
 * `apps/fyne/apps/mail/testdata/carpetas-externas.json`.
 */
export function sortMailboxes<M extends { name: string; role?: string | undefined }>(
  locale: Locale,
  boxes: readonly M[],
): M[] {
  const rank = (m: M): number => {
    // «Pospuestos» justo debajo de Recibidos, como en Gmail: es la Entrada en
    // espera. Va aparte de ROLE_ORDER porque no es un buzón que se unifique (solo
    // Sytue lo tiene) ni existe en ninguna cuenta externa.
    if (m.role === 'snoozed') return 0.5;
    const i = m.role ? ROLE_ORDER.indexOf(m.role as (typeof ROLE_ORDER)[number]) : -1;
    return i === -1 ? 99 : i;
  };
  return [...boxes].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      // Las carpetas propias se ordenan con el collator del idioma del usuario, no
      // con uno fijo: la Ä alemana y la Ñ española no se colocan igual.
      a.name.localeCompare(b.name, intlTag(locale)),
  );
}

/**
 * Los colores de una etiqueta: la paleta de colores de usuario de la suite
 * (`PALETA_DE_USUARIO` de @sytue/config). Se guarda el TOKEN (el id del tono),
 * no el hex, y el chip en sí es neutro (tokens del design system, contraste
 * AA): el punto solo desambigua y el NOMBRE siempre acompaña, así el color nunca
 * es el único indicador (WCAG AA). Los ids tienen que coincidir con
 * domain.LabelColors del backend; las ocho etiquetas de antes guardan los
 * mismos tokens y se pintan con el tono nuevo.
 */
export const LABEL_COLOR_TOKENS: readonly string[] = PALETA_DE_USUARIO.map((c) => c.id);

/** Color del punto de una etiqueta (token → hex); vacío/desconocido = neutro. */
export function labelDot(color: string): string {
  return colorDeUsuario(color) ?? 'var(--sytue-border-strong, #94a3b8)';
}

/**
 * Etiquetas del registro aplicadas a una conversación: unión de las de sus
 * mensajes (un keyword = id de etiqueta presente en `keywords`). Devuelve las
 * Label completas del registro, en su orden, para pintar chips con nombre+color.
 */
export function conversationLabels<T extends { id: string }>(
  conv: Conversation,
  registry: T[],
): T[] {
  const applied = new Set<string>();
  for (const m of conv.messages) {
    for (const k in m.keywords) if (m.keywords[k]) applied.add(k);
  }
  return registry.filter((l) => applied.has(l.id));
}

/** Etiquetas aplicadas a un solo mensaje (para chips de fila de lista). */
export function messageLabels<T extends { id: string }>(email: EmailSummary, registry: T[]): T[] {
  return registry.filter((l) => email.keywords[l.id]);
}

/**
 * Glifo de cada carpeta por su role JMAP.
 *
 * Antes salían del set `Icon` de @sytue/ui, que no tiene sobre, archivador ni
 * papelera, así que cada carpeta llevaba el icono menos malo que había: una
 * CAMPANA en Recibidos, un ASPA en la Papelera —que se lee «cerrar»—, una MARCA
 * DE VERIFICACIÓN en Archivo y una REJILLA en cualquier carpeta propia. Los
 * glifos del triaje de esta misma app sí son los correctos y ya se pintan en los
 * botones de al lado, así que ahora la carpeta y la acción que lleva a ella
 * comparten dibujo.
 *
 * Borradores se queda en `folder`: no hay glifo de borrador y una carpeta
 * neutra miente menos que las tres rayas del menú.
 */
const ROLE_GLYPHS: Record<string, GlyphName> = {
  inbox: 'unread',
  sent: 'forward',
  trash: 'trash',
  junk: 'spam',
  archive: 'archive',
  outbox: 'outbox',
  snoozed: 'clock',
};

export function mailboxGlyph(role?: string): GlyphName {
  return (role && ROLE_GLYPHS[role]) || 'folder';
}

/**
 * ¿El cuerpo HTML pide algo a un servidor de fuera? Sirve para avisar de que el
 * mensaje trae contenido remoto BLOQUEADO (la CSP del visor no lo deja salir):
 * si no hay nada que desbloquear, no se enseña el aviso.
 *
 * Cubre las dos formas en que llega un píxel de rastreo: un atributo que apunta
 * a una URL absoluta (`src`, `srcset`, `background`, `poster`…) y una URL dentro
 * de CSS en línea (`background-image: url(...)`). No pretende ser un analizador
 * de HTML: quien decide de verdad es la CSP, esto solo decide si aparece la
 * barra. Un falso positivo enseña un aviso de más; un falso negativo NO expone
 * nada, porque la petición sigue bloqueada.
 */
export function hasRemoteContent(html: string): boolean {
  // `href` queda fuera a propósito: un enlace no descarga nada hasta que se pulsa.
  return (
    /\b(?:src|srcset|background|poster)\s*=\s*["']?\s*https?:\/\//i.test(html) ||
    /url\(\s*["']?\s*https?:\/\//i.test(html)
  );
}
