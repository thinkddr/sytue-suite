/**
 * MessageBody — cuerpo de un mensaje de Sytue Chat: Markdown ligero + menciones
 * + emoji por :shortcode:.
 *
 * Parser clean-room mínimo, sin dependencias. Inline: **negrita**, _cursiva_,
 * ~~tachado~~, `código`, bloques ``` y autolinks http(s). Bloque: encabezados
 * (#/##/###), listas con viñetas (-/*) y numeradas (1.), y citas (>). El render
 * es 100 % JSX (nunca dangerouslySetInnerHTML), así el cuerpo no puede inyectar
 * HTML → sin XSS; los `< > &` del usuario los escapa Preact.
 *
 * El cuerpo vive dentro de un <p> (MessageView/ThreadPanel): los bloques se
 * pintan con <span class="block"> (nunca <ul>/<blockquote>/<h1>, que cerrarían el
 * <p> y romperían el SSR), con role=list/listitem para la semántica de lector.
 *
 * Las menciones @Nombre (de message.mentions) y las especiales @channel/@here/
 * @all se resaltan como chip dentro de texto, negrita, cursiva, listas y citas;
 * nunca dentro de código o enlaces. Solo se resaltan los subs realmente
 * mencionados, así una coincidencia casual de «@algo» no se pinta.
 * docs/design.md §1.6.
 */
import { Fragment } from 'preact';
import type { JSX } from 'preact';
import { parseEmote } from '../lib/commands.js';
import { customEmojiMap, customEmojiSrc } from '../lib/customEmoji.js';

import { copyToClipboard } from '../lib/clipboard.js';
import type { T } from '../i18n/index.js';

interface Props {
  body: string;
  /** Nombres a resaltar (los display de los subs mencionados en este mensaje). */
  mentionNames?: string[];
  /** Autor del mensaje: para pintar el emote «* Autor acción» (/me). */
  authorName?: string;
  /**
   * Emoji personalizados del tenant (nombre → fileId de Drive). `:nombre:` de esta
   * tabla se pinta como `<img class=emoji>` servida por el proxy de adjuntos (nosniff);
   * un nombre ausente queda literal (Ola 14).
   */
  customEmoji?: Record<string, string>;
  /** Traductor, para los rótulos de la cabecera del bloque de código. */
  t?: T;
}

/** Contexto de render inline: regex de menciones + tabla de emoji personalizados. */
interface RenderCtx {
  re: RegExp | null;
  custom?: Record<string, string>;
  /**
   * Traductor. Va en el contexto y no en un prop porque quien lo necesita está
   * hondo —la cabecera del bloque de código— y enhebrarlo por cada nodo sería
   * pasar el mismo valor por siete firmas. Opcional para no romper a los llamantes
   * directos de los tests, que no pintan bloques con cabecera.
   */
  t?: T;
}

/** Token de emoji personalizado :nombre: (minúsculas/dígitos/_/-, como el backend). */
const CUSTOM_EMOJI_RE = /:([a-z0-9_-]+):/g;

/** Estilo de la imagen de un emoji personalizado dentro del texto: un poco más
 *  alta que la línea (como Slack/Mattermost), alineada a la base. Se sirve por el
 *  proxy de adjuntos, que aplica X-Content-Type-Options: nosniff + CSP sandbox. */
const EMOJI_IMG_CLASS = 'mx-px inline-block h-[1.375em] w-[1.375em] object-contain align-[-0.3em]';

/** Un `:nombre:` de emoji personalizado como <img>; nombre ausente → texto literal.
 *  Reutilizable para las reacciones (píldoras), no solo el cuerpo del mensaje. */
export function EmojiToken({ token }: { token: string }): JSX.Element {
  const m = /^:([a-z0-9_-]+):$/.exec(token);
  const fileId = m ? customEmojiMap()[m[1]!] : undefined;
  if (fileId === undefined) return <>{token}</>;
  return (
    <img
      src={customEmojiSrc(fileId)}
      alt={token}
      title={token}
      class={EMOJI_IMG_CLASS}
      loading="lazy"
      decoding="async"
    />
  );
}

/**
 * Nodo del Markdown ligero. Las marcas inline (text/bold/italic/strike/code/
 * block/link) no se anidan entre sí; los bloques (heading/quote/ul/ol) llevan
 * hijos inline (parseados con parseInline), no otros bloques.
 */
export type MdNode =
  | { kind: 'text'; text: string }
  | { kind: 'bold'; text: string }
  | { kind: 'italic'; text: string }
  | { kind: 'strike'; text: string }
  | { kind: 'code'; text: string }
  | { kind: 'block'; text: string; lang?: string }
  | { kind: 'link'; href: string; text?: string }
  | { kind: 'heading'; level: number; children: MdNode[] }
  | { kind: 'quote'; children: MdNode[] }
  | { kind: 'ul'; items: MdNode[][] }
  | { kind: 'ol'; start: number; items: MdNode[][] };

// Bloque cercado ```…```. La etiqueta de lenguaje se CONSERVA: quien escribe
// ```go lo escribe para que se sepa qué es, y tirarla convertía ese esfuerzo en
// nada. No se usa para colorear —eso pide un resaltador y es otra ola—, sino para
// rotularlo, que es la mitad del valor y no cuesta una dependencia.
const FENCE_RE = /```([\w+-]*)\n?([\s\S]*?)```/g;
// Inline en una pasada; el orden de alternativas fija la precedencia (código gana).
// El lookbehind/lookahead de _cursiva_ evita falsos positivos en snake_case.
// ponytail: parser plano, sin anidamiento (enlace dentro de negrita, etc.); pasar a
// parser recursivo solo si algún producto lo pide.
const INLINE_RE =
  /`([^`\n]+)`|\*\*([^\n]+?)\*\*|~~([^~\n]+?)~~|(?<![\w_])_([^_\n]+)_(?!\w)|\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)|https?:\/\/[^\s]+/g;
// Puntuación colgante tras un autolink («mira https://x.eu.») que no es parte de la URL.
const TRAIL_RE = /[.,;:!?…»)\]]+$/;

// Construcciones de bloque, por línea (tras extraer los bloques cercados).
const HEADING_RE = /^(#{1,3})\s+(.+?)\s*$/;
const BULLET_RE = /^\s*[-*]\s+(.+)$/;
const NUMBER_RE = /^\s*(\d{1,9})\.\s+(.+)$/;
const QUOTE_RE = /^\s*>\s?(.*)$/;

function parseInline(text: string, out: MdNode[]): void {
  let last = 0;
  for (const m of text.matchAll(INLINE_RE)) {
    const idx = m.index ?? 0;
    if (idx > last) out.push({ kind: 'text', text: text.slice(last, idx) });
    last = idx + m[0].length;
    if (m[1] !== undefined) out.push({ kind: 'code', text: m[1] });
    else if (m[2] !== undefined) out.push({ kind: 'bold', text: m[2] });
    else if (m[3] !== undefined) out.push({ kind: 'strike', text: m[3] });
    else if (m[4] !== undefined) out.push({ kind: 'italic', text: m[4] });
    else if (m[5] !== undefined && m[6] !== undefined) {
      // Enlace con texto, `[texto](url)`. El patrón solo admite http/https: un
      // `[pincha](javascript:alert(1))` no casa y se queda como texto plano.
      out.push({ kind: 'link', href: m[6], text: m[5] });
    } else {
      const href = m[0].replace(TRAIL_RE, '');
      if (/^https?:\/\/$/.test(href)) {
        out.push({ kind: 'text', text: m[0] }); // protocolo pelado: no es un enlace
      } else {
        out.push({ kind: 'link', href });
        last = idx + href.length; // la puntuación colgante vuelve al flujo de texto
      }
    }
  }
  if (last < text.length) out.push({ kind: 'text', text: text.slice(last) });
}

/** Nodos inline de un fragmento de texto (hijos de un bloque). */
function inlineNodes(text: string): MdNode[] {
  const nodes: MdNode[] = [];
  parseInline(text, nodes);
  return nodes;
}

/**
 * Parsea las construcciones de bloque (encabezados, listas, citas) línea a línea;
 * las líneas normales se agrupan en párrafos y pasan por parseInline (mismo
 * resultado plano que antes para texto sin bloques).
 */
function parseBlocks(text: string, out: MdNode[]): void {
  const lines = text.split('\n');
  let para: string[] = [];
  const flush = (): void => {
    if (para.length === 0) return;
    parseInline(para.join('\n'), out);
    para = [];
  };
  for (let i = 0; i < lines.length;) {
    const line = lines[i]!;
    let m: RegExpExecArray | null;
    if ((m = HEADING_RE.exec(line))) {
      flush();
      out.push({ kind: 'heading', level: m[1]!.length, children: inlineNodes(m[2]!) });
      i++;
    } else if (BULLET_RE.test(line)) {
      flush();
      const items: MdNode[][] = [];
      while (i < lines.length && (m = BULLET_RE.exec(lines[i]!))) {
        items.push(inlineNodes(m[1]!));
        i++;
      }
      out.push({ kind: 'ul', items });
    } else if (NUMBER_RE.test(line)) {
      flush();
      const items: MdNode[][] = [];
      let start = 1;
      while (i < lines.length && (m = NUMBER_RE.exec(lines[i]!))) {
        if (items.length === 0) start = Number.parseInt(m[1]!, 10);
        items.push(inlineNodes(m[2]!));
        i++;
      }
      out.push({ kind: 'ol', start, items });
    } else if (QUOTE_RE.test(line)) {
      flush();
      const quoted: string[] = [];
      while (i < lines.length && (m = QUOTE_RE.exec(lines[i]!))) {
        quoted.push(m[1]!);
        i++;
      }
      out.push({ kind: 'quote', children: inlineNodes(quoted.join('\n')) });
    } else {
      para.push(line);
      i++;
    }
  }
  flush();
}

/** Parsea el Markdown ligero de un mensaje a una lista de nodos. Puro, testeable. */
export function parseMarkdown(body: string): MdNode[] {
  const out: MdNode[] = [];
  let last = 0;
  for (const m of body.matchAll(FENCE_RE)) {
    const idx = m.index ?? 0;
    // El salto de línea pegado a la valla ya lo aporta el bloque (display block).
    if (idx > last) parseBlocks(body.slice(last, idx).replace(/\n$/, ''), out);
    out.push({
      kind: 'block',
      text: (m[2] ?? '').replace(/\n$/, ''),
      ...(m[1] ? { lang: m[1] } : {}),
    });
    last = idx + m[0].length;
    if (body[last] === '\n') last++;
  }
  if (last < body.length) parseBlocks(body.slice(last), out);
  return out;
}

/**
 * Extrae hasta `max` URLs http(s) del cuerpo, reusando el MISMO parser (parseMarkdown):
 * solo los autolinks reales (nodos `link`) cuentan, así una URL dentro de `código` o
 * de un bloque ``` NO se previsualiza. Deduplicado, en orden de aparición. Alimenta el
 * unfurl de enlaces (Ola 16): el cliente pide la vista previa de estas URLs.
 */
export function extractLinks(body: string, max = 2): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  const walk = (nodes: MdNode[]): void => {
    for (const n of nodes) {
      if (out.length >= max) return;
      if (n.kind === 'link') {
        if (!seen.has(n.href)) {
          seen.add(n.href);
          out.push(n.href);
        }
      } else if (n.kind === 'heading' || n.kind === 'quote') {
        walk(n.children);
      } else if (n.kind === 'ul' || n.kind === 'ol') {
        for (const item of n.items) walk(item);
      }
    }
  };
  walk(parseMarkdown(body));
  return out;
}

// ---------------------------------------------------------------------------
// Emoji por :shortcode:
// ---------------------------------------------------------------------------

/**
 * Mapa clean-room de shortcodes de emoji frecuentes → Unicode (sin dependencia).
 * No pretende ser exhaustivo (~130 de uso común); un shortcode ausente se deja
 * literal. Exportado para que el Composer autocomplete con el mismo mapa.
 */
export const EMOJI: Record<string, string> = {
  smile: '😄',
  smiley: '😃',
  grin: '😁',
  laughing: '😆',
  sweat_smile: '😅',
  joy: '😂',
  rofl: '🤣',
  relaxed: '☺️',
  blush: '😊',
  innocent: '😇',
  slight_smile: '🙂',
  upside_down: '🙃',
  wink: '😉',
  relieved: '😌',
  heart_eyes: '😍',
  kissing_heart: '😘',
  yum: '😋',
  stuck_out_tongue: '😛',
  stuck_out_tongue_winking_eye: '😜',
  zany: '🤪',
  sunglasses: '😎',
  nerd: '🤓',
  thinking: '🤔',
  hushed: '😯',
  neutral_face: '😐',
  expressionless: '😑',
  no_mouth: '😶',
  rolling_eyes: '🙄',
  smirk: '😏',
  unamused: '😒',
  sweat: '😓',
  pensive: '😔',
  confused: '😕',
  worried: '😟',
  frowning: '😦',
  anguished: '😧',
  cry: '😢',
  sob: '😭',
  weary: '😩',
  tired_face: '😫',
  triumph: '😤',
  angry: '😠',
  rage: '😡',
  exploding_head: '🤯',
  flushed: '😳',
  scream: '😱',
  fearful: '😨',
  cold_sweat: '😰',
  disappointed: '😞',
  astonished: '😲',
  yawning: '🥱',
  sleeping: '😴',
  sleepy: '😪',
  mask: '😷',
  nauseated: '🤢',
  sneezing: '🤧',
  hot: '🥵',
  cold: '🥶',
  dizzy_face: '😵',
  shushing: '🤫',
  hugging: '🤗',
  money_mouth: '🤑',
  cowboy: '🤠',
  partying: '🥳',
  woozy: '🥴',
  pleading: '🥺',
  star_struck: '🤩',
  shrug: '🤷',
  facepalm: '🤦',
  thumbsup: '👍',
  '+1': '👍',
  thumbsdown: '👎',
  '-1': '👎',
  ok_hand: '👌',
  punch: '👊',
  fist: '✊',
  wave: '👋',
  raised_hands: '🙌',
  pray: '🙏',
  clap: '👏',
  muscle: '💪',
  point_up: '☝️',
  point_down: '👇',
  point_left: '👈',
  point_right: '👉',
  v: '✌️',
  crossed_fingers: '🤞',
  handshake: '🤝',
  writing_hand: '✍️',
  heart: '❤️',
  orange_heart: '🧡',
  yellow_heart: '💛',
  green_heart: '💚',
  blue_heart: '💙',
  purple_heart: '💜',
  black_heart: '🖤',
  white_heart: '🤍',
  broken_heart: '💔',
  sparkling_heart: '💖',
  two_hearts: '💕',
  heartbeat: '💓',
  fire: '🔥',
  star: '⭐',
  star2: '🌟',
  sparkles: '✨',
  boom: '💥',
  zap: '⚡',
  tada: '🎉',
  confetti_ball: '🎊',
  balloon: '🎈',
  gift: '🎁',
  trophy: '🏆',
  medal: '🏅',
  crown: '👑',
  rocket: '🚀',
  '100': '💯',
  check: '✅',
  white_check_mark: '✅',
  heavy_check_mark: '✔️',
  x: '❌',
  warning: '⚠️',
  question: '❓',
  exclamation: '❗',
  bell: '🔔',
  eyes: '👀',
  bulb: '💡',
  lock: '🔒',
  key: '🔑',
  hourglass: '⏳',
  alarm: '⏰',
  calendar: '📅',
  pushpin: '📌',
  paperclip: '📎',
  memo: '📝',
  pencil: '✏️',
  book: '📖',
  books: '📚',
  mag: '🔍',
  bar_chart: '📊',
  chart: '📈',
  chart_down: '📉',
  email: '📧',
  envelope: '✉️',
  phone: '📱',
  computer: '💻',
  bug: '🐛',
  wrench: '🔧',
  hammer: '🔨',
  gear: '⚙️',
  link: '🔗',
  coffee: '☕',
  tea: '🍵',
  beer: '🍺',
  pizza: '🍕',
  cake: '🍰',
  birthday: '🎂',
  apple: '🍎',
  sun: '☀️',
  cloud: '☁️',
  rainbow: '🌈',
  snowflake: '❄️',
  dog: '🐶',
  cat: '🐱',
  unicorn: '🦄',
  ghost: '👻',
  robot: '🤖',
  alien: '👽',
  poop: '💩',
  skull: '💀',
  wave_hand: '👋',
  clap_hands: '👏',
  ok: '🆗',
  new: '🆕',
  up: '🆙',
  cool: '🆒',
  free: '🆓',
  musical_note: '🎵',
  notes: '🎶',
};
const SHORTCODE_RE = /:([a-z0-9_+-]+):/g;

/** Sustituye :shortcode: conocidos por su Unicode; los desconocidos quedan literales. */
export function emojify(text: string): string {
  if (!text.includes(':')) return text;
  return text.replace(SHORTCODE_RE, (whole, name: string) => EMOJI[name] ?? whole);
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/** Menciones especiales de canal (@channel/@here/@all) como token de palabra. */
const CHANNEL_MENTION_RE = /(?<![\w@])@(?:channel|here|all)\b/g;

/** Trocea texto plano resaltando las menciones @Nombre (chip AA sobre tinte). */
function withMentions(text: string, re: RegExp | null): (string | JSX.Element)[] {
  if (!re) return [text];
  const parts: (string | JSX.Element)[] = [];
  let last = 0;
  for (const match of text.matchAll(re)) {
    const idx = match.index ?? 0;
    if (idx > last) parts.push(text.slice(last, idx));
    parts.push(
      <span
        key={`u${idx}`}
        data-mention=""
        class="bg-selected text-primary-text rounded px-1 font-medium"
      >
        {match[0]}
      </span>,
    );
    last = idx + match[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return parts;
}

/**
 * Render de un fragmento de texto: emoji Unicode por shortcode + chips de mención.
 * Las menciones especiales (@channel/@here/@all) llevan chip propio (tinte de
 * aviso), distinto del chip de @usuario; el resto pasa por withMentions.
 */
function withUnicodeMentions(text: string, re: RegExp | null): (string | JSX.Element)[] {
  const t = emojify(text);
  const parts: (string | JSX.Element)[] = [];
  let last = 0;
  let k = 0;
  for (const m of t.matchAll(CHANNEL_MENTION_RE)) {
    const idx = m.index ?? 0;
    if (idx > last) parts.push(...withMentions(t.slice(last, idx), re));
    parts.push(
      <span
        key={`c${k++}`}
        data-mention="channel"
        class="bg-warning-bg text-warning-text rounded px-1 font-medium"
      >
        {m[0]}
      </span>,
    );
    last = idx + m[0].length;
  }
  if (last < t.length) parts.push(...withMentions(t.slice(last), re));
  return parts;
}

/**
 * Render de un fragmento de texto (Ola 14): primero resuelve los emoji
 * PERSONALIZADOS del tenant `:nombre:` → <img> (tienen prioridad sobre los Unicode
 * del mismo nombre); los tramos restantes pasan por Unicode + menciones. Un
 * `:nombre:` que no esté en el catálogo queda literal y lo trata el paso Unicode.
 */
function withInline(text: string, ctx: RenderCtx): (string | JSX.Element)[] {
  const custom = ctx.custom;
  if (!custom || !text.includes(':')) return withUnicodeMentions(text, ctx.re);
  const parts: (string | JSX.Element)[] = [];
  let last = 0;
  let k = 0;
  for (const m of text.matchAll(CUSTOM_EMOJI_RE)) {
    const name = m[1]!;
    const fileId = custom[name];
    if (fileId === undefined) continue; // no es custom: lo resuelve el paso Unicode
    const idx = m.index ?? 0;
    if (idx > last) parts.push(...withUnicodeMentions(text.slice(last, idx), ctx.re));
    parts.push(
      <img
        key={`ce${k++}`}
        src={customEmojiSrc(fileId)}
        alt={`:${name}:`}
        title={`:${name}:`}
        class={EMOJI_IMG_CLASS}
        loading="lazy"
        decoding="async"
      />,
    );
    last = idx + m[0].length;
  }
  if (last < text.length) parts.push(...withUnicodeMentions(text.slice(last), ctx.re));
  return parts;
}

/** Tamaño del encabezado por nivel (sobrio: sin saltos enormes). */
const HEADING_CLASS: Record<number, string> = { 1: 'text-lg', 2: 'text-md', 3: 'text-base' };

function renderChildren(nodes: MdNode[], ctx: RenderCtx): JSX.Element[] {
  return nodes.map((n, i) => renderNode(n, i, ctx));
}

function renderNode(node: MdNode, key: number | string, ctx: RenderCtx): JSX.Element {
  switch (node.kind) {
    case 'bold':
      return (
        <strong key={key} class="font-semibold">
          {withInline(node.text, ctx)}
        </strong>
      );
    case 'italic':
      return <em key={key}>{withInline(node.text, ctx)}</em>;
    case 'strike':
      return (
        <span key={key} class="line-through">
          {withInline(node.text, ctx)}
        </span>
      );
    case 'code':
      return (
        <code
          key={key}
          class="sytue-code border-border bg-surface-sunken rounded border px-1 py-px"
        >
          {node.text}
        </code>
      );
    case 'block':
      // span block y no <pre>: el cuerpo vive dentro de un <p> y un <pre>
      // anidado rompería el marcado en SSR.
      return (
        <span key={key} class="border-border bg-surface-sunken my-1 block rounded-md border">
          {/* Cabecera con el lenguaje y COPIAR. Copiar un bloque de código es la
              acción que más se hace sobre él en un chat de trabajo, y hasta ahora
              había que seleccionarlo a mano con el ratón — con el riesgo de
              llevarse media línea de menos. */}
          <span class="border-border flex items-center justify-between gap-2 border-b px-2.5 py-1">
            <span class="sytue-meta">{node.lang || ctx.t?.('message.code') || 'código'}</span>
            <button
              type="button"
              class="text-text-muted hover:text-text sytue-focus-ring rounded px-1 text-xs"
              onClick={() => void copyToClipboard(node.text)}
            >
              {ctx.t?.('action.copy') ?? ''}
            </button>
          </span>
          <span class="sytue-code block overflow-x-auto px-2.5 py-1.5 whitespace-pre">
            {node.text}
          </span>
        </span>
      );
    case 'link':
      return (
        <a
          key={key}
          href={node.href}
          target="_blank"
          rel="noopener noreferrer"
          class="text-primary-text sytue-focus-ring break-all underline underline-offset-2 focus-visible:rounded-sm"
        >
          {node.text ?? node.href}
        </a>
      );
    case 'heading':
      return (
        <span
          key={key}
          class={`text-text mt-1.5 mb-0.5 block font-semibold ${HEADING_CLASS[node.level] ?? 'text-base'}`}
        >
          {renderChildren(node.children, ctx)}
        </span>
      );
    case 'quote':
      return (
        <span key={key} class="border-border text-text-muted my-1 block border-l-2 pl-3">
          {renderChildren(node.children, ctx)}
        </span>
      );
    case 'ul':
      return (
        <span key={key} role="list" class="my-1 block">
          {node.items.map((it, i) => (
            <span key={i} role="listitem" class="block pl-1">
              <span aria-hidden="true">• </span>
              {renderChildren(it, ctx)}
            </span>
          ))}
        </span>
      );
    case 'ol':
      return (
        <span key={key} role="list" class="my-1 block">
          {node.items.map((it, i) => (
            <span key={i} role="listitem" class="block pl-1">
              {`${node.start + i}. `}
              {renderChildren(it, ctx)}
            </span>
          ))}
        </span>
      );
    default:
      return <Fragment key={key}>{withInline(node.text, ctx)}</Fragment>;
  }
}

export function MessageBody({
  body,
  mentionNames,
  authorName,
  customEmoji,
  t,
}: Props): JSX.Element {
  const names = [...new Set((mentionNames ?? []).filter((n) => n.trim() !== ''))];
  // Más largos primero: evita que «@Ana» tape a «@Ana María» (prefijo común).
  const re =
    names.length > 0
      ? new RegExp(
          `@(?:${names
            .sort((a, b) => b.length - a.length)
            .map(escapeRe)
            .join('|')})`,
          'g',
        )
      : null;
  // Emoji personalizados: el prop gana (tests / callers directos); si no, el catálogo
  // de módulo del tenant (rellenado una vez al cargar Rooms) — sin enhebrar la tabla.
  const ctx: RenderCtx = { re, custom: customEmoji ?? customEmojiMap(), ...(t ? { t } : {}) };

  // Un mensaje cuyo cuerpo es SOLO una imagen del catálogo del tenant (`:nombre:`) se
  // pinta EN GRANDE: así es como se manda un GIF (selector «GIF» del compositor), y es
  // lo que hacen Slack y Teams con el mensaje de un solo emoji. Con texto alrededor
  // sigue yendo a tamaño de emoji, que es lo que se espera dentro de una frase.
  //
  // ponytail: la regla es «token solo»; un GIF con pie de foto se pinta pequeño. Si
  // hiciera falta, la alternativa es marcar el mensaje, no adivinar por el texto.
  const loneEmoji = /^:([a-z0-9_-]{1,64}):$/.exec(body.trim());
  const loneFileId = loneEmoji ? ctx.custom?.[loneEmoji[1]!] : undefined;
  if (loneFileId !== undefined) {
    return (
      <img
        src={customEmojiSrc(loneFileId)}
        alt={`:${loneEmoji![1]}:`}
        title={`:${loneEmoji![1]}:`}
        class="max-h-48 max-w-full rounded-md"
        loading="lazy"
        decoding="async"
      />
    );
  }

  // Emote «/me acción» (Ola 14): se pinta en cursiva como «* Autor acción». La acción
  // conserva emoji + menciones (withInline); un render que no lo entienda ve «/me …».
  const emote = parseEmote(body);
  if (emote !== null) {
    return (
      <em class="text-text-muted">
        <span aria-hidden="true">* </span>
        {authorName ? <span class="text-text font-medium">{authorName} </span> : null}
        {withInline(emote, ctx)}
      </em>
    );
  }

  return <>{parseMarkdown(body).map((node, i) => renderNode(node, i, ctx))}</>;
}
