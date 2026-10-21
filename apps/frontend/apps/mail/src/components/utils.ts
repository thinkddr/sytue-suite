/**
 * Utilidades de presentacion para Sytue Mail. Todo lo que sale a pantalla —fechas,
 * tamaños, nombres de carpeta— depende del idioma, así que estas funciones reciben
 * el `locale` ya negociado (o la `t` de la app) en vez de fijar un formato.
 */
import { formatDate, formatDateTime, formatTime, type Locale } from '@sytue/i18n';
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
    b.receivedAt.localeCompare(a.receivedAt),
  );
  return groupByThread(recientesPrimero)[0] ?? null;
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
} as const satisfies Record<string, MailMessageKey>;

export function mailboxLabel(t: T, name: string, role?: string): string {
  const key = role ? ROLE_KEYS[role as keyof typeof ROLE_KEYS] : undefined;
  return key ? t(key) : name;
}

/**
 * Paleta sobria de etiquetas: TOKEN → color del PUNTO. El chip
 * en sí es neutro (tokens del design system, contraste AA); el punto solo
 * desambigua y el NOMBRE siempre acompaña, así el color nunca es el único
 * indicador (WCAG AA). Debe coincidir con domain.LabelColors del backend.
 */
export const LABEL_COLORS: Record<string, string> = {
  slate: '#5b6b7b',
  brass: '#9a7b3f',
  sage: '#5f7a5a',
  clay: '#a4623f',
  blue: '#3f6699',
  plum: '#7a5480',
  teal: '#3f7d7a',
  rose: '#9c5470',
};

/** Lista ordenada de tokens de color (para el selector de la etiqueta). */
export const LABEL_COLOR_TOKENS = Object.keys(LABEL_COLORS);

/** Color del punto de una etiqueta (token → hex); vacío/desconocido = neutro. */
export function labelDot(color: string): string {
  return LABEL_COLORS[color] ?? 'var(--sytue-border-strong, #94a3b8)';
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
