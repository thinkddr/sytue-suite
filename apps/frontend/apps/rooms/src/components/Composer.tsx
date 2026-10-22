/**
 * Composer — caja de redaccion de Sytue Chat. Enter envia, Shift+Enter salto de
 * linea; auto-crece hasta un maximo. Al escribir «@» ofrece un selector de
 * miembros (y de las menciones especiales @channel/@here/@all) para mencionar; al
 * escribir «:algo» ofrece emoji por shortcode. Un botón de emoji abre el selector
 * completo (buscable); también se puede arrastrar-soltar o pegar una imagen. Los
 * ficheros se suben a Sytue Drive (proxy /api/upload) y viajan como file_ids al
 * enviar; un mensaje puede llevar solo adjuntos. Teclado + raton. Envio optimista
 * gestionado por el padre. docs/design.md §9.
 *
 * El cromo tiene tres puertas y ningún formulario permanente (Ola 1 de paridad,
 * docs/roadmap.md): «+» abre el menú de acciones (subir fichero, fichero YA existente
 * de Sytue Drive, apps e integraciones); «!» abre la prioridad y el acuse de recibo; y
 * el reloj abre «Enviar más tarde» con atajos relativos (el control de fecha solo
 * aparece si se pide «personalizada»). Lo que se aplica se resume en un chip sobre la
 * caja. Nada de <select> ni de datetime-local a la vista.
 *
 * FORMA DE LA CAJA. El texto va ARRIBA, a todo el ancho, y los controles DEBAJO, en una
 * barra con el cromo de la casa (`BOTON_DE_BARRA`, `SEPARADOR_DE_BARRA`): los mismos
 * botones planos del ribbon de Sytue Docs y del redactor de Sytue Mail. Antes iba todo
 * en una fila —dos botones, el texto, el emoji y el partido de enviar—, y eso costaba
 * dos cosas caras: a 320 px el cromo se comía ~190 px de los 288 útiles y quedaban ~96
 * para escribir, y con el tabulador había que atravesar dos botones ANTES de llegar al
 * campo, que es lo único a lo que se viene. En dos filas el texto tiene el ancho entero
 * y es la primera parada del foco.
 */
import type { JSX, TargetedEvent } from 'preact';
import { useCallback, useEffect, useId, useRef, useState } from 'preact/hooks';
import {
  Avatar,
  Button,
  Dropdown,
  Icon,
  MenuItem,
  MenuSeparator,
  Popover,
  SEPARADOR_DE_BARRA,
  Spinner,
  cx,
  type IconName,
  useConfirm,
  Checkbox,
  Radio,
  useUltimo,
  useMobileBack,
  MASCARA_SCROLL_X,
  useScrollRestante,
} from '@sytue/ui';
import { contiene, formatTime, type Locale } from '@sytue/i18n';
import type { ChannelMember, Attachment, MessageCard } from '../client/chatApi.js';
import { uploadAttachment } from '../client/chatApi.js';
import type { T, MessageKey } from '../i18n/index.js';
import { EMOJI } from './MessageBody.js';
import { EmojiPickerButton } from './EmojiPicker.js';
import { DrivePickerModal } from './DrivePickerModal.js';
import { CardComposerModal } from './CardComposerModal.js';
import { GifPickerModal } from './GifPickerModal.js';
import {
  ClipGlyph,
  ClockGlyph,
  ImageGlyph,
  PriorityGlyph,
  SendGlyph,
  UploadGlyph,
} from './glyphs.js';
import { MAX_ATTACHMENTS } from '../lib/fileIds.js';
import { loadDraft, saveDraft, clearDraft } from '../lib/drafts.js';
import { mentionsWholeChannel } from '../lib/mentions.js';
import { aplicarFormato, marcaDeAtajo, type MarcaFormato } from '../lib/formato.js';
import { formatSchedule, schedulePresets, toLocalValue } from '../lib/schedule.js';
import {
  parseCommand,
  commandAutocompleteQuery,
  matchCommands,
  matchRemoteCommands,
  shrugBody,
  emoteBody,
  type SlashCommand,
  type RemoteCommand,
  type CommandName,
} from '../lib/commands.js';

/** Descripción y uso de un comando: viven en el catálogo, con el nombre como clave. */
function commandDescription(t: T, name: CommandName): string {
  return t(`command.${name}.description`);
}
function commandUsage(t: T, name: CommandName): string {
  return t(`command.${name}.usage`);
}

/** Opciones de la Ola 12 que acompañan a un envío desde el composer de canal:
 *  prioridad, petición de acuse y/o programación (sendAt en ISO 8601). */
export interface ComposeOptions {
  priority?: 'important' | 'urgent';
  requiresAck?: boolean;
  /** Si viene, el mensaje se PROGRAMA para este instante (ISO 8601) en vez de enviarse ya. */
  sendAt?: string;
  /** Objeto rico que acompaña al mensaje: encuesta o tarea. El cuerpo es la pregunta
   *  o el título, así que se publica por el MISMO camino que cualquier mensaje. */
  card?: MessageCard;
}

/** A partir de cuántos miembros se pide confirmación para @channel/@here/@all.
 *  Por debajo, el aviso estorba más de lo que protege: en un canal de cuatro
 *  personas mencionar a todos es lo normal. */
const UMBRAL_AVISO_MASIVO = 8;

interface Props {
  channelName: string;
  /** Bloqueo estructural (p. ej. no hay canal activo): desactiva también la edición. */
  disabled?: boolean;
  /** Sin red se puede redactar y persistir el texto, pero no ejecutar acciones remotas. */
  offline?: boolean;
  /** Miembros para el selector de menciones («@»). */
  members?: ChannelMember[];
  /** Texto guía (por defecto «Escribe en #canal»). */
  placeholder?: string;
  /** Etiqueta accesible del textarea (por defecto «Escribir en {canal}»). */
  ariaLabel?: string;
  /**
   * Muestra los controles avanzados de la Ola 12 (prioridad, pedir acuse, programar
   * envío). Solo el composer de CANAL los ofrece; el de respuestas en hilo no: sin
   * `advanced` no aparecen ni el botón «!» ni el reloj de programar.
   */
  advanced?: boolean;
  /**
   * Abre el modal de apps e integraciones (webhooks entrantes/salientes + bots) desde
   * el menú «+». El estado vive en el padre (ChatApp), que ya monta el modal. Ausente
   * = la entrada no se ofrece (composer de hilo).
   */
  onOpenIntegrations?: () => void;
  /** body = texto; mentions = subs mencionados; attachments = adjuntos de Drive ya subidos;
   *  opts = prioridad/acuse/programación (Ola 12, solo si advanced). */
  onSend: (
    body: string,
    mentions: string[],
    attachments: Attachment[],
    opts?: ComposeOptions,
  ) => void;
  /**
   * Despacha un slash command CON EFECTO (Ola 14): presencia, DM, tema, búsqueda…
   * El registro/parseo vive en lib/commands.ts; el efecto (que necesita las deps
   * reales) lo ejecuta el padre. `/shrug`, `/me` y `/help` los resuelve el propio
   * composer. Ausente = esos comandos avisan «no disponible aquí» y NO se envían.
   */
  onCommand?: (name: string, arg: string) => void;
  /**
   * Comandos de SERVIDOR registrados por la organización, para el autocompletado y
   * para reconocerlos al enviar. Vacío = solo los integrados.
   */
  remoteCommands?: RemoteCommand[];
  /**
   * Ejecuta un comando de servidor: lo llama el backend, que a su vez llama al
   * servicio del tercero y devuelve su respuesta. Ausente = se avisa «no disponible».
   */
  onRemoteCommand?: (name: string, arg: string) => void;
  /** Notifica (el padre lo emite por WS) que el usuario está escribiendo. Throttled aquí. */
  onTyping?: () => void;
  /**
   * Ámbito del borrador (canal `<channelId>` o hilo `<channelId>:thread:<rootId>`).
   * Persiste el texto sin enviar en localStorage y lo restaura al remontar. El padre
   * DEBE dar además `key={draftId}` para remontar el composer al cambiar de ámbito
   * (si no, el texto de un canal se colaría en el siguiente). Ausente = sin persistir.
   */
  draftId?: string;
  /**
   * Canal al que se publica. Los ficheros que se suban se guardan en el ÁMBITO de ese
   * canal (solo los leen sus miembros), así que es obligatorio para adjuntar.
   */
  channelId: string;
  /** Idioma activo (formato de fechas del menú «Enviar más tarde») y su traductor. */
  locale: Locale;
  t: T;
}

/**
 * Menciones especiales: notifican a todo el canal (o a los presentes, @here). La
 * pista es una clave de catálogo; el token («channel», «here», «all») NO se traduce
 * porque es lo que se teclea.
 */
const SPECIAL_MENTIONS: { token: string; hint: MessageKey }[] = [
  { token: 'channel', hint: 'mention.notifyChannel' },
  { token: 'here', hint: 'mention.notifyHere' },
  { token: 'all', hint: 'mention.notifyChannel' },
];

/** Una entrada del popover de autocompletado. */
type Suggestion =
  | { kind: 'member'; member: ChannelMember }
  | { kind: 'special'; token: string; hint: MessageKey }
  | { kind: 'emoji'; name: string; glyph: string }
  | { kind: 'command'; command: SlashCommand }
  // Comando de SERVIDOR: su nombre y su ayuda vienen del backend (los escribió quien
  // lo registró), así que no pasan por el catálogo de traducciones.
  | { kind: 'remote'; command: RemoteCommand };

/** Disparador activo: «/» (comando, al principio), «@» (menciones) o «:» (emoji). */
type Trigger =
  | { type: 'mention'; query: string }
  | { type: 'emoji'; query: string }
  | { type: 'command'; query: string }
  | null;

const MENTION_AT = /(^|\s)@([^\s@]*)$/;
const EMOJI_AT = /(^|\s):([a-z0-9_+-]{2,})$/;

/**
 * Botón del cromo del composer («+», «!», reloj, emoji): el de la casa.
 *
 * Era una copia a mano —32×32, `rounded-md`— que el selector de emoji volvía a copiar
 * en su `triggerClass`, y las dos habían perdido lo mismo por el camino: el objetivo
 * TÁCTIL. 32 px se falla con la yema, y aquí los botones van pegados unos a otros y al
 * de enviar, así que fallar no es no acertar: es pulsar el de al lado o enviar sin
 * querer. `BOTON_DE_BARRA` trae los 44 px con el dedo (`pointer-coarse`) y la forma
 * plana del ribbon —sin caja, porque veinte cajas en una barra compiten con lo que se
 * está escribiendo, que es lo que importa—. `shrink-0` porque la barra es flex.
 */
// Los mismos estados que BOTON_DE_BARRA (cromoDeEditor.ts) pero a 32 px y con el
// radio compartido: la barra de un editor va a 28 px porque son veinte controles en fila;
// aquí son cinco bajo un campo de 40 px, y a 28 px se leían apretados y pequeños.
const BOTON_DEL_COMPOSER =
  'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-control text-text-muted ' +
  'transition-colors duration-150 hover:bg-bg-subtle hover:text-text ' +
  'aria-pressed:bg-selected aria-pressed:text-text disabled:pointer-events-none disabled:opacity-40 ' +
  'pointer-coarse:min-h-[max(44px,2.75rem)] pointer-coarse:min-w-[max(44px,2.75rem)] sytue-focus-ring';

/** Recuerda si la barra de formato está desplegada (por dispositivo, como en Mail). */
const FORMATO_KEY = 'sytue-rooms-formato';

/** Botones de la barra de formato: marca, icono del set y rótulo. */
const MARCAS_DE_BARRA: ReadonlyArray<readonly [MarcaFormato, IconName, MessageKey]> = [
  ['bold', 'bold', 'composer.formatBold'],
  ['italic', 'italic', 'composer.formatItalic'],
  ['strike', 'strikethrough', 'composer.formatStrike'],
  ['code', 'code', 'composer.formatCode'],
  ['link', 'link', 'composer.formatLink'],
  ['quote', 'quote', 'composer.formatQuote'],
  ['list', 'list', 'composer.formatList'],
];

/** Entrada de un popover del composer (atajo de programación / fecha personalizada). */
const POPOVER_ITEM =
  'flex w-full items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-sm text-text transition-colors duration-150 hover:bg-bg-subtle focus-visible:bg-bg-subtle focus-visible:outline-none';

/** Fila de opción con control nativo dentro de un popover (radio / casilla). */
const POPOVER_OPTION =
  'flex cursor-pointer items-center gap-2 rounded-md px-1.5 py-1.5 text-sm text-text transition-colors duration-150 hover:bg-bg-subtle';

/** Rótulo de sección dentro de un popover del composer. */
const POPOVER_LABEL = 'mb-1 px-1.5 sytue-overline text-text-muted';

/** Límite de subida por fichero: el mismo que declara el BFF (`pages/api/upload.ts`),
 *  para avisar aquí en vez de gastar la subida entera y recibir un 413.
 *  Por defecto 2 GiB (requisito de la suite); configurable mediante variable/política de Sytue Admin. */
const DEFAULT_MAX_ATTACHMENT_MB = 2048;
const MAX_UPLOAD_MB =
  typeof window !== 'undefined' &&
  typeof (window as unknown as { __SYTUE_MAX_ATTACHMENT_MB__?: number })
    .__SYTUE_MAX_ATTACHMENT_MB__ === 'number'
    ? (window as unknown as { __SYTUE_MAX_ATTACHMENT_MB__: number }).__SYTUE_MAX_ATTACHMENT_MB__
    : DEFAULT_MAX_ATTACHMENT_MB;
const MAX_UPLOAD_BYTES = MAX_UPLOAD_MB * 1024 * 1024;

/** Un adjunto en curso/listo dentro del composer (antes de enviar el mensaje). */
interface Pending {
  localId: string;
  name: string;
  status: 'uploading' | 'done' | 'error';
  /** Fracción subida 0..1 (mientras status === 'uploading'). */
  progress: number;
  /** Adjunto resuelto (fileId + metadatos) cuando status === 'done'. */
  att?: Attachment;
  controller: AbortController;
}

/** Disparador de autocompletado pegado al cursor (o null). */
function triggerAt(text: string, caret: number): Trigger {
  const before = text.slice(0, caret);
  const e = EMOJI_AT.exec(before);
  if (e) return { type: 'emoji', query: e[2]! };
  const m = MENTION_AT.exec(before);
  if (m) return { type: 'mention', query: m[2]! };
  return null;
}

/** Lista de sugerencias para el disparador vigente (acotada). */
function suggestionsFor(
  trigger: Trigger,
  roster: ChannelMember[],
  remote: RemoteCommand[],
): Suggestion[] {
  if (trigger?.type === 'command') {
    return [
      ...matchCommands(trigger.query).map((command): Suggestion => ({ kind: 'command', command })),
      // Detrás de los integrados: son los que la gente ya conoce.
      ...matchRemoteCommands(trigger.query, remote).map((command): Suggestion => ({
        kind: 'remote',
        command,
      })),
    ];
  }
  if (trigger?.type === 'mention') {
    const q = trigger.query.toLowerCase();
    const specials: Suggestion[] = SPECIAL_MENTIONS.filter((s) => s.token.startsWith(q)).map(
      (s) => ({
        kind: 'special',
        token: s.token,
        hint: s.hint,
      }),
    );
    const members: Suggestion[] = roster
      .filter((m) => contiene(m.name, q))
      .slice(0, 8)
      .map((m) => ({ kind: 'member', member: m }));
    return [...specials, ...members].slice(0, 10);
  }
  if (trigger?.type === 'emoji') {
    const q = trigger.query.toLowerCase();
    return Object.keys(EMOJI)
      .filter((n) => contiene(n, q))
      .slice(0, 8)
      .map((n) => ({ kind: 'emoji', name: n, glyph: EMOJI[n]! }));
  }
  return [];
}

function suggestionKey(s: Suggestion): string {
  if (s.kind === 'member') return `m:${s.member.sub}`;
  if (s.kind === 'special') return `s:${s.token}`;
  if (s.kind === 'command') return `c:${s.command.name}`;
  if (s.kind === 'remote') return `r:${s.command.name}`;
  return `e:${s.name}`;
}

/** Prioridad de un mensaje: 'normal' no viaja en ComposeOptions (es el defecto). */
type Priority = 'normal' | 'important' | 'urgent';

/** Opciones del popover de prioridad, en orden (Mattermost «Message Priority»). */
const PRIORITY_CHOICES: readonly Priority[] = ['normal', 'important', 'urgent'];

/** Tono del chip de prioridad (solo las que se señalan; 'normal' no tiene chip). */
const PRIORITY_TONE: Record<Exclude<Priority, 'normal'>, string> = {
  important: 'bg-warning-bg text-warning-text',
  urgent: 'bg-danger-bg text-danger-text',
};

/**
 * Chip compacto sobre la caja de texto: resume una opción ACTIVA del envío
 * (prioridad, acuse, programación) y la desactiva con su «✕». Sustituye al formulario
 * permanente: el estado se ve, pero los controles viven tras su puerta.
 */
function OptionChip({
  tone,
  label,
  clearLabel,
  onClear,
  disabled,
}: {
  tone: string;
  label: string;
  clearLabel: string;
  onClear: () => void;
  disabled?: boolean;
}): JSX.Element {
  return (
    <span
      class={cx(
        'inline-flex items-center gap-1 rounded-md pr-0.5 pl-2 text-xs font-semibold',
        tone,
      )}
    >
      {label}
      <button
        type="button"
        disabled={disabled}
        aria-label={clearLabel}
        title={clearLabel}
        onClick={onClear}
        class="sytue-focus-ring grid min-h-[24px] min-w-[24px] place-items-center rounded transition-opacity duration-150 hover:opacity-70 disabled:cursor-not-allowed"
      >
        <Icon name="close" size={12} />
      </button>
    </span>
  );
}

export function Composer({
  channelName,
  disabled = false,
  offline = false,
  members,
  placeholder,
  ariaLabel,
  advanced = false,
  onSend,
  onCommand,
  remoteCommands,
  onRemoteCommand,
  onTyping,
  onOpenIntegrations,
  draftId,
  channelId,
  locale,
  t,
}: Props): JSX.Element {
  const networkDisabled = disabled || offline;
  // Restaura el borrador de este ámbito al montar (lazy). Como el padre remonta el
  // composer con key={draftId} al cambiar de canal/hilo, este init corre de nuevo
  // por ámbito: nunca se arrastra el texto de un canal a otro.
  const [value, setValue] = useState(() => (draftId ? loadDraft(draftId) : ''));
  const [trigger, setTrigger] = useState<Trigger>(null);
  const [index, setIndex] = useState(0);
  const [pending, setPending] = useState<Pending[]>([]);
  const pendingRef = useUltimo(pending);
  useEffect(
    () => () => {
      for (const upload of pendingRef.current) upload.controller.abort();
    },
    [pendingRef],
  );
  const [uploadError, setUploadError] = useState<string | null>(null);
  // Aviso de un slash command no reconocido / no disponible (no se envía el texto).
  const [cmdNotice, setCmdNotice] = useState<string | null>(null);
  // Confirmación de @channel/@here/@all: el diálogo va montado al final del árbol.
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const [dragOver, setDragOver] = useState(false);
  // Ola 12: prioridad, petición de acuse y programación (solo con `advanced`).
  const [priority, setPriority] = useState<Priority>('normal');
  const [requiresAck, setRequiresAck] = useState(false);
  // scheduleAt es el valor del <input datetime-local> (hora local, «» = enviar ya).
  // Lo fijan tanto los atajos relativos del menú como la fecha personalizada.
  const [scheduleAt, setScheduleAt] = useState('');
  // Revela el control de fecha DENTRO del popover de «Enviar más tarde» (nunca antes).
  const [customDate, setCustomDate] = useState(false);
  // Mínimo del selector de fecha, fijado al abrirlo: el render no puede leer el reloj.
  const [minFecha, setMinFecha] = useState('');
  // Selector de un fichero YA existente de Sytue Drive (menú «+»).
  const [drivePickerOpen, setDrivePickerOpen] = useState(false);
  /** Objeto rico en curso desde el menú «+»: encuesta, tarea o ninguno. */
  const [cardKind, setCardKind] = useState<'poll' | 'task' | null>(null);
  const [gifPickerOpen, setGifPickerOpen] = useState(false);
  const ref = useRef<HTMLTextAreaElement>(null);

  // Barra de formato bajo demanda («Aa», como en Mail): oculta por defecto y
  // recordada por dispositivo. Los atajos (Cmd/Ctrl+B/I/K, Mayús+X) valen siempre.
  const [formatoVisible, setFormatoVisibleEstado] = useState(
    () => typeof localStorage !== 'undefined' && localStorage.getItem(FORMATO_KEY) === '1',
  );
  const { ref: accionesRef, queda } = useScrollRestante<HTMLDivElement>(
    (formatoVisible ? 7 : 0) + (advanced ? 2 : 0),
  );
  const setFormatoVisible = (n: boolean): void => {
    setFormatoVisibleEstado(n);
    if (typeof localStorage !== 'undefined') localStorage.setItem(FORMATO_KEY, n ? '1' : '0');
  };
  const aplicarMarca = (marca: MarcaFormato): void => {
    const ta = ref.current;
    if (!ta) return;
    const r = aplicarFormato(ta.value, ta.selectionStart ?? 0, ta.selectionEnd ?? 0, marca);
    setValue(r.value);
    // El cursor se coloca tras el repintado: si no, Preact lo manda al final y
    // «negrita» acaba escribiéndose fuera de las marcas que acabas de poner.
    requestAnimationFrame(() => {
      ta.setSelectionRange(r.selStart, r.selEnd);
      ta.focus();
    });
  };
  const fileInputRef = useRef<HTMLInputElement>(null);
  // Throttle del indicador "escribiendo…": como mucho un evento cada 3 s mientras
  // se teclea (el receptor lo expira a los ~5 s). Evita un frame por pulsación.
  const lastTypingRef = useRef(0);
  // A11y: liga el textarea (combobox) con el popover de sugerencias (listbox) y su
  // opción activa (aria-activedescendant), para lectores de pantalla.
  const listboxId = useId();
  const optionId = (i: number): string => `${listboxId}-opt-${i}`;
  // Pistas de atajos ligadas al textarea (aria-describedby): las anuncia el lector
  // al enfocar. Visualmente son un pie discreto, no una tira de onboarding.
  const hintsId = useId();
  // Base de ids de las opciones de envío (grupo de radios de prioridad y control de
  // fecha): única por instancia — canal e hilo conviven y no deben compartir grupo.
  const optsId = useId();

  function notifyTyping(text: string): void {
    if (!onTyping || networkDisabled || !text.trim()) return;
    const now = Date.now();
    if (now - lastTypingRef.current < 3000) return;
    lastTypingRef.current = now;
    onTyping();
  }

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
  }, [value]);

  // --- Borrador (Ola 8): persiste el texto sin enviar por ámbito -------------
  // Último valor por ref, para volcarlo al desmontar (cambio de canal remonta el
  // composer y, sin este volcado, se perderían las últimas pulsaciones).
  const valueRef = useUltimo(value);
  // Lectura del ÚLTIMO texto en el momento de volcar. `valueRef` no apunta a un nodo del
  // DOM sino al valor más reciente, así que leerlo al limpiar es justo lo que se quiere.
  const textoActual = useCallback(() => valueRef.current, [valueRef]);
  // Persiste con debounce mientras se teclea (evita un write por pulsación).
  useEffect(() => {
    if (!draftId) return;
    const t = setTimeout(() => saveDraft(draftId, value), 400);
    return () => clearTimeout(t);
  }, [draftId, value]);
  // Volcado del último valor al desmontar (o cambiar de ámbito).
  useEffect(() => {
    if (!draftId) return;
    return () => saveDraft(draftId, textoActual());
  }, [draftId, textoActual]);

  const roster = members ?? [];
  const matches = suggestionsFor(trigger, roster, remoteCommands ?? []);
  const open = matches.length > 0;

  useMobileBack(open, () => setTrigger(null));
  // Índice de selección acotado al rango vigente (la lista cambia al teclear).
  const sel = Math.min(index, Math.max(0, matches.length - 1));

  const doneAttachments = pending.filter((p) => p.status === 'done' && p.att).map((p) => p.att!);
  const uploading = pending.some((p) => p.status === 'uploading');
  const canSend =
    !networkDisabled && !uploading && (value.trim().length > 0 || doneAttachments.length > 0);

  function refreshTrigger(el: HTMLTextAreaElement): void {
    // «/comando» al principio absoluto (aún sin espacio) tiene prioridad sobre @/:.
    const cmd = commandAutocompleteQuery(el.value);
    setTrigger(
      cmd !== null
        ? { type: 'command', query: cmd }
        : triggerAt(el.value, el.selectionStart ?? el.value.length),
    );
    setIndex(0);
  }

  // --- Adjuntos ---------------------------------------------------------------

  /** Sube cada fichero a Drive (con progreso); rechaza los que exceden el límite. */
  function addFiles(files: File[]): void {
    if (files.length === 0 || networkDisabled) return;
    setUploadError(null);
    const room = MAX_ATTACHMENTS - pending.length;
    if (room <= 0) {
      setUploadError(t('composer.maxAttachments', { count: MAX_ATTACHMENTS }));
      return;
    }
    const accepted = files.slice(0, room);
    if (files.length > accepted.length)
      setUploadError(t('composer.maxAttachments', { count: MAX_ATTACHMENTS }));
    for (const file of accepted) {
      if (file.size > MAX_UPLOAD_BYTES) {
        setUploadError(t('composer.fileTooLarge', { name: file.name, max: MAX_UPLOAD_MB }));
        continue;
      }
      const localId = `u-${Date.now()}-${Math.random().toString(36).slice(2)}`;
      const controller = new AbortController();
      setPending((prev) => [
        ...prev,
        { localId, name: file.name, status: 'uploading', progress: 0, controller },
      ]);
      uploadAttachment(file, {
        // El canal es parte de la subida: Drive guarda el fichero en su ámbito y no
        // en «Mi unidad» (que toda la organización puede leer).
        channelId,
        signal: controller.signal,
        onProgress: (f) =>
          setPending((prev) =>
            prev.map((p) => (p.localId === localId ? { ...p, progress: f } : p)),
          ),
      })
        .then((att) =>
          setPending((prev) =>
            prev.map((p) =>
              p.localId === localId
                ? { ...p, status: 'done', progress: 1, att, name: att.name }
                : p,
            ),
          ),
        )
        .catch((err: unknown) => {
          if (err instanceof Error && err.message === 'aborted') return; // lo quitó el usuario
          if (err instanceof Error && err.message === 'too_large') {
            setUploadError(t('composer.fileTooLarge', { name: file.name, max: MAX_UPLOAD_MB }));
          }
          setPending((prev) =>
            prev.map((p) => (p.localId === localId ? { ...p, status: 'error' } : p)),
          );
        });
    }
  }

  /**
   * Adjunta un fichero YA existente de Sytue Drive por REFERENCIA: entra directo como
   * adjunto resuelto (status 'done'), sin subir bytes de nuevo. Respeta el tope por
   * mensaje y evita duplicar el mismo fichero.
   */
  function addDriveAttachment(att: Attachment): void {
    setDrivePickerOpen(false);
    setUploadError(null);
    setPending((prev) => {
      if (prev.some((p) => p.att?.fileId === att.fileId)) return prev;
      if (prev.length >= MAX_ATTACHMENTS) {
        setUploadError(t('composer.maxAttachments', { count: MAX_ATTACHMENTS }));
        return prev;
      }
      return [
        ...prev,
        {
          localId: `d-${att.fileId}`,
          name: att.name,
          status: 'done',
          progress: 1,
          att,
          // Adjunto por referencia: no hay subida que abortar, pero `Pending` exige un
          // controlador; se crea uno inerte para no ramificar el tipo.
          controller: new AbortController(),
        },
      ];
    });
  }

  /** Quita un adjunto (cancela la subida en vuelo). */
  function removeAttachment(localId: string): void {
    setPending((prev) => {
      const e = prev.find((p) => p.localId === localId);
      if (e && e.status === 'uploading') e.controller.abort();
      return prev.filter((p) => p.localId !== localId);
    });
  }

  function onFilesPicked(e: TargetedEvent<HTMLInputElement>): void {
    const input = e.currentTarget;
    addFiles(Array.from(input.files ?? []));
    input.value = ''; // permite re-seleccionar el mismo fichero
  }

  function onDrop(e: DragEvent): void {
    e.preventDefault();
    setDragOver(false);
    addFiles(Array.from(e.dataTransfer?.files ?? []));
  }

  function onPaste(e: ClipboardEvent): void {
    const items = e.clipboardData?.items;
    if (!items) return;
    const files: File[] = [];
    for (const it of items) {
      if (it.kind === 'file') {
        const f = it.getAsFile();
        if (f) files.push(f);
      }
    }
    if (files.length > 0) {
      e.preventDefault(); // pega imagen/fichero, no su ruta como texto
      addFiles(files);
    }
  }

  // --- Envío ------------------------------------------------------------------

  /** Limpia el composer tras un envío/comando con éxito (texto, adjuntos, opciones). */
  function resetAfterSend(): void {
    setValue('');
    setTrigger(null);
    setPending([]);
    setUploadError(null);
    setCmdNotice(null);
    setPriority('normal');
    setRequiresAck(false);
    setScheduleAt('');
    setCustomDate(false);
    // Enviado con éxito: descarta el borrador de este ámbito (el volcado al
    // desmontar vería '' y también lo borraría, pero esto es explícito e inmediato).
    if (draftId) clearDraft(draftId);
  }

  /** Envía un cuerpo ya resuelto: calcula menciones + opciones (Ola 12) y limpia.
   *
   *  Es `async` por la confirmación de @channel/@here/@all: sin freno, cualquiera
   *  puede despertar a un canal entero por error —y en un canal grande eso es una
   *  notificación a cada persona, de noche incluida—. */
  async function sendBody(body: string): Promise<void> {
    if (mentionsWholeChannel(body) && roster.length >= UMBRAL_AVISO_MASIVO) {
      const ok = await confirmar({
        mensaje: t('mention.confirmAll', { count: roster.length, channel: channelName }),
        confirmLabel: t('mention.confirmAllAction'),
      });
      if (!ok) return;
    }
    // Menciones = subs cuyos nombres aparecen literalmente como «@Nombre» en el
    // texto. Los tokens especiales (@channel/@here/@all) los expande el backend
    // desde el cuerpo, no viajan en este array.
    const mentions = [
      ...new Set(roster.filter((m) => body.includes(`@${m.name}`)).map((m) => m.sub)),
    ];
    let opts: ComposeOptions | undefined;
    if (advanced) {
      opts = {
        ...(priority !== 'normal' ? { priority } : {}),
        ...(requiresAck ? { requiresAck: true } : {}),
        // datetime-local es hora LOCAL; new Date lo interpreta como tal → ISO en UTC.
        ...(scheduleAt ? { sendAt: new Date(scheduleAt).toISOString() } : {}),
      };
      if (Object.keys(opts).length === 0) opts = undefined;
    }
    onSend(body, mentions, doneAttachments, opts);
    resetAfterSend();
  }

  function submit(): void {
    if (networkDisabled || uploading) return;
    setCmdNotice(null);

    // Slash command (Ola 14): despacha la acción EN VEZ de enviar el texto. Parsea
    // el valor CRUDO (el «/» inicial importa). Desconocido → avisa y NO envía.
    const parsed = parseCommand(value, remoteCommands ?? []);
    if (parsed?.kind === 'unknown') {
      setCmdNotice(t('command.unknown', { name: parsed.name }));
      return;
    }
    if (parsed?.kind === 'remote') {
      // Comando de un tercero: lo resuelve el backend, que firma la llamada y trae la
      // respuesta. Aquí solo se despacha y se limpia el composer.
      if (!onRemoteCommand) {
        setCmdNotice(t('command.unavailable', { name: parsed.name }));
        return;
      }
      onRemoteCommand(parsed.name, parsed.arg.trim());
      resetAfterSend();
      return;
    }
    if (parsed?.kind === 'command') {
      if (parsed.name === 'help') {
        // Reutiliza el autocompletado como ayuda: muestra todos los comandos.
        setValue('/');
        setTrigger({ type: 'command', query: '' });
        setIndex(0);
        return;
      }
      if (parsed.name === 'shrug') {
        void sendBody(shrugBody(parsed.arg));
        return;
      }
      if (parsed.name === 'me') {
        void sendBody(emoteBody(parsed.arg));
        return;
      }
      // Comandos con efecto (presencia, DM, tema, búsqueda…): los despacha el padre.
      if (!onCommand) {
        setCmdNotice(t('command.unavailable', { name: parsed.name }));
        return;
      }
      onCommand(parsed.name, parsed.arg.trim());
      resetAfterSend();
      return;
    }

    // Mensaje normal.
    const body = value.trim();
    if (!body && doneAttachments.length === 0) return;
    void sendBody(body);
  }

  /** Inserta un glyph de emoji en el cursor (desde el selector completo). */
  function insertGlyph(glyph: string): void {
    const el = ref.current;
    const caret = el?.selectionStart ?? value.length;
    const before = value.slice(0, caret) + glyph;
    const next = before + value.slice(caret);
    setValue(next);
    requestAnimationFrame(() => {
      if (!el) return;
      el.focus();
      el.setSelectionRange(before.length, before.length);
    });
  }

  /** Sustituye el «@query»/«:query» pegado al cursor por la sugerencia y cierra.
   *  Para un comando, rellena «/nombre » (el valor entero) listo para escribir el arg. */
  function pick(s: Suggestion): void {
    const el = ref.current;
    if (s.kind === 'command' || s.kind === 'remote') {
      const next = `/${s.command.name} `;
      setValue(next);
      setTrigger(null);
      requestAnimationFrame(() => {
        if (!el) return;
        el.focus();
        el.setSelectionRange(next.length, next.length);
      });
      return;
    }
    const caret = el?.selectionStart ?? value.length;
    const head = value.slice(0, caret);
    let before: string;
    if (s.kind === 'member') before = head.replace(MENTION_AT, `$1@${s.member.name} `);
    else if (s.kind === 'special') before = head.replace(MENTION_AT, `$1@${s.token} `);
    else before = head.replace(EMOJI_AT, `$1${s.glyph} `);
    const next = before + value.slice(caret);
    setValue(next);
    setTrigger(null);
    // Recolocar el cursor tras el texto insertado (en el próximo frame).
    requestAnimationFrame(() => {
      if (!el) return;
      el.focus();
      el.setSelectionRange(before.length, before.length);
    });
  }

  function onKeyDown(e: TargetedEvent<HTMLTextAreaElement, KeyboardEvent>): void {
    // Formato antes que nada: Cmd/Ctrl+B, +I y +K envuelven la selección. Va aquí
    // arriba porque son atajos del NAVEGADOR (Cmd+B abre favoritos, Cmd+K la barra
    // de búsqueda) y hay que quedárselos antes de que llegue el autocompletado.
    const marca = marcaDeAtajo(e);
    if (marca) {
      e.preventDefault();
      aplicarMarca(marca);
      return;
    }
    if (open) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setIndex((i) => (i + 1) % matches.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setIndex((i) => (i - 1 + matches.length) % matches.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        pick(matches[sel]!);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setTrigger(null);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  }

  return (
    <div
      class="border-border bg-surface relative border-t px-4 py-3"
      onDragOver={(e) => {
        if (networkDisabled) return;
        e.preventDefault();
        setDragOver(true);
      }}
      onDragLeave={(e) => {
        // Solo cierra al salir del contenedor (no al cruzar entre hijos).
        if (e.currentTarget === e.target) setDragOver(false);
      }}
      onDrop={onDrop}
    >
      {open ? (
        <ul
          id={listboxId}
          // `left-4`/`right-4` y no `left-3`/`right-3`: el contenedor lleva `px-4`, así
          // que con 12 px el panel se quedaba 4 px fuera de la caja a la que pertenece
          // por cada lado, desalineado con el texto que lo dispara.
          class="sytue-menu-surface border-border absolute right-4 bottom-full left-4 z-20 mb-1 max-h-56 overflow-y-auto rounded-lg border py-1 shadow-[var(--shadow-md)]"
          role="listbox"
          aria-label={
            trigger?.type === 'command'
              ? t('composer.commands')
              : trigger?.type === 'emoji'
                ? t('composer.emojiToInsert')
                : t('composer.mentionSuggestions')
          }
        >
          {matches.map((s, i) => (
            <li key={suggestionKey(s)} id={optionId(i)} role="option" aria-selected={i === sel}>
              <button
                type="button"
                // onMouseDown (no onClick): evita que el blur del textarea cierre
                // el selector antes de insertar la sugerencia.
                onMouseDown={(ev) => {
                  ev.preventDefault();
                  pick(s);
                }}
                onMouseEnter={() => setIndex(i)}
                class={cx(
                  'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors duration-150',
                  i === sel ? 'bg-selected text-primary-text' : 'text-text hover:bg-bg-subtle',
                )}
              >
                {s.kind === 'member' ? (
                  <>
                    <Avatar name={s.member.name} size="xs" />
                    <span class="min-w-0 truncate">{s.member.name}</span>
                  </>
                ) : s.kind === 'special' ? (
                  <>
                    <span
                      aria-hidden="true"
                      class="bg-warning-bg text-warning-text grid h-5 w-5 shrink-0 place-items-center rounded text-xs font-bold"
                    >
                      @
                    </span>
                    <span class="min-w-0 truncate font-medium">@{s.token}</span>
                    <span class="sytue-meta ml-auto shrink-0 truncate">{t(s.hint)}</span>
                  </>
                ) : s.kind === 'command' || s.kind === 'remote' ? (
                  <>
                    <span
                      aria-hidden="true"
                      class="bg-bg-subtle text-text-muted grid h-5 w-5 shrink-0 place-items-center rounded font-mono text-xs font-bold"
                    >
                      /
                    </span>
                    <span class="shrink-0 font-medium">/{s.command.name}</span>
                    <span class="sytue-meta min-w-0 flex-1 truncate">
                      {s.kind === 'remote'
                        ? (s.command.description ?? '')
                        : commandDescription(t, s.command.name)}
                    </span>
                    <span class="sytue-meta hidden shrink-0 font-mono sm:inline">
                      {s.kind === 'remote'
                        ? (s.command.usageHint ?? '')
                        : commandUsage(t, s.command.name)}
                    </span>
                  </>
                ) : (
                  <>
                    <span
                      aria-hidden="true"
                      class="grid h-5 w-5 shrink-0 place-items-center text-base"
                    >
                      {s.glyph}
                    </span>
                    <span class="text-text-muted min-w-0 truncate">:{s.name}:</span>
                  </>
                )}
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      {/* Superposición de arrastrar-soltar sobre el composer. */}
      {dragOver ? (
        <div
          class="border-primary bg-selected/80 text-primary-text pointer-events-none absolute inset-1 z-30 flex items-center justify-center rounded-lg border-2 border-dashed text-sm font-medium"
          aria-hidden="true"
        >
          {t('composer.dropToAttach')}
        </div>
      ) : null}

      {/* Adjuntos en curso / listos, con progreso y quitar. */}
      {pending.length > 0 ? (
        <ul class="mb-2 flex flex-wrap gap-2" aria-label={t('composer.attachments')}>
          {pending.map((p) => (
            <li
              key={p.localId}
              class="border-border bg-bg flex items-center gap-2 rounded-md border px-2 py-1.5 text-xs"
            >
              <span class="bg-bg-subtle text-text-muted grid h-6 w-6 shrink-0 place-items-center rounded">
                {p.status === 'uploading' ? (
                  <Spinner size={12} />
                ) : p.status === 'error' ? (
                  <Icon name="warning" size={12} class="text-danger-text" />
                ) : (
                  <ClipGlyph size={12} />
                )}
              </span>
              <span class="text-text max-w-[10rem] truncate font-medium" title={p.name}>
                {p.name}
              </span>
              {p.status === 'uploading' ? (
                <span class="text-text-subtle shrink-0 tabular-nums">
                  {Math.round(p.progress * 100)}%
                </span>
              ) : p.status === 'error' ? (
                <span class="text-danger-text shrink-0 font-medium">{t('state.error')}</span>
              ) : null}
              <button
                type="button"
                aria-label={t('action.removeNamed', { name: p.name })}
                title={t('action.remove')}
                onClick={() => removeAttachment(p.localId)}
                class="text-text-subtle hover:bg-bg-subtle hover:text-text sytue-focus-ring grid min-h-[24px] min-w-[24px] shrink-0 place-items-center rounded transition-colors duration-150"
              >
                <Icon name="close" size={12} />
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      {uploadError ? (
        <p class="text-danger-text mb-2 flex items-center gap-1.5 text-xs" role="alert">
          <Icon name="warning" size={12} />
          {uploadError}
        </p>
      ) : null}

      {cmdNotice ? (
        <p class="text-danger-text mb-2 flex items-center gap-1.5 text-xs" role="alert">
          <Icon name="warning" size={12} />
          {cmdNotice}
        </p>
      ) : null}

      {/* Ola 12 tras su puerta (Ola 1 de paridad): la prioridad, el acuse y la
          programación ya no son un formulario permanente. Solo se ven cuando están
          ACTIVOS, como chips que se desactivan con su «✕». */}
      {advanced && (priority !== 'normal' || requiresAck || scheduleAt) ? (
        <ul class="mb-2 flex flex-wrap items-center gap-2" aria-label={t('composer.sendOptions')}>
          {priority !== 'normal' ? (
            <li class="flex">
              <OptionChip
                tone={cx('uppercase', PRIORITY_TONE[priority])}
                label={t(`priority.${priority}`)}
                clearLabel={t('composer.clearPriority')}
                onClear={() => setPriority('normal')}
                disabled={disabled}
              />
            </li>
          ) : null}
          {requiresAck ? (
            <li class="flex">
              <OptionChip
                tone="bg-bg-subtle text-text-muted"
                label={t('composer.requestAck')}
                clearLabel={t('composer.clearAck')}
                onClear={() => setRequiresAck(false)}
                disabled={disabled}
              />
            </li>
          ) : null}
          {scheduleAt ? (
            <li class="flex">
              <OptionChip
                tone="bg-bg-subtle text-text-muted"
                label={t('scheduled.sendsAt', { when: formatSchedule(locale, scheduleAt) })}
                clearLabel={t('composer.clearSchedule')}
                onClear={() => {
                  setScheduleAt('');
                  setCustomDate(false);
                }}
                disabled={disabled}
              />
            </li>
          ) : null}
        </ul>
      ) : null}

      <div
        class={cx(
          'border-border bg-surface rounded-lg border px-1.5 pt-1 pb-1.5 shadow-[var(--shadow-sm)] transition-[border-color,box-shadow,opacity] duration-150',
          disabled
            ? 'opacity-60'
            : // La CAJA no se enciende. El foco lo pinta el área de texto y solo ella
              // (ver el `textarea` de abajo): mientras lo pintaban los dos salían dos
              // rectángulos dorados —uno alrededor de todo, incluida la barra de
              // iconos, y otro alrededor del texto—, que es lo que el propietario
              // pidió quitar el 2026-09-09. Aquí queda el hover de cualquier campo.
              'hover:border-border-strong',
        )}
      >
        {/* Input file oculto (operable por teclado): lo dispara «Subir archivo» del
            menú «+». El clip suelto desapareció para no ofrecer dos veces lo mismo. */}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          class="sr-only"
          tabIndex={-1}
          aria-hidden="true"
          onChange={onFilesPicked}
        />

        {/* El texto, PRIMERO en el DOM y a todo el ancho. Primero porque es a lo que se
            viene: quien tabula desde la lista de mensajes aterriza aquí y no en el cromo.
            Crece hasta 200 px (el efecto de arriba mide `scrollHeight`) y a partir de ahí
            se desplaza por dentro, con la barra siempre debajo y a la vista. */}
        <textarea
          ref={ref}
          rows={1}
          value={value}
          disabled={disabled}
          aria-label={ariaLabel ?? t('composer.writeInAria', { channel: channelName })}
          placeholder={placeholder ?? t('composer.writeIn', { channel: channelName })}
          role="combobox"
          aria-expanded={open}
          aria-controls={open ? listboxId : undefined}
          aria-activedescendant={open ? optionId(sel) : undefined}
          aria-autocomplete="list"
          aria-describedby={hintsId}
          class="text-text placeholder:text-text-subtle block max-h-[200px] min-h-[2.5rem] w-full resize-none rounded-md bg-transparent px-2.5 pt-2 pb-1 text-base leading-6 transition-shadow duration-150 outline-none disabled:cursor-not-allowed"
          onInput={(e: TargetedEvent<HTMLTextAreaElement>) => {
            setValue(e.currentTarget.value);
            refreshTrigger(e.currentTarget);
            notifyTyping(e.currentTarget.value);
            if (cmdNotice) setCmdNotice(null);
          }}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          onBlur={() => setTrigger(null)}
        />

        {/* La barra: los controles DEBAJO del texto y no a sus lados. Con el cromo de la
            casa (`gap-0.5` como `BARRA_DE_EDICION`), pero sin su fondo ni su pelo: esta
            barra vive DENTRO de la caja de escribir, que ya se separa sola del resto. */}
        <div class="flex min-w-0 items-center gap-1 px-0.5 pt-0.5">
          {/* El ancho útil también puede ser el de un canal entre dos paneles:
              desplazar solo las opciones reserva siempre el sitio de Enviar. */}
          <div
            ref={accionesRef}
            data-scroll-x
            class={cx(
              'flex min-w-0 flex-1 items-center gap-1 overflow-x-auto overscroll-x-contain p-0.5 [&>*]:shrink-0',
              queda && MASCARA_SCROLL_X,
            )}
          >
            {/* Menú «+» de acciones: puerta única a lo que ya existe (subir, Drive, apps).
              Reusa el Dropdown de @sytue/ui → role=menu, flechas, Esc y foco gestionado. */}
            <Dropdown
              align="start"
              side="top"
              label={t('composer.messageActions')}
              trigger={({ onClick, ...aria }) => (
                <button
                  type="button"
                  disabled={networkDisabled}
                  onClick={onClick}
                  {...aria}
                  aria-label={t('composer.addToMessage')}
                  title={t('composer.addToMessage')}
                  class={BOTON_DEL_COMPOSER}
                >
                  <Icon name="plus" size={18} />
                </button>
              )}
            >
              {/* Cada entrada recoge el menú antes de abrir su diálogo (el del sistema para
              subir, el selector de Drive o el de integraciones). */}
              {(close) => (
                <>
                  <MenuItem
                    size="sm"
                    icon={<UploadGlyph size={16} />}
                    onClick={() => {
                      close();
                      fileInputRef.current?.click();
                    }}
                  >
                    {t('composer.uploadFile')}
                  </MenuItem>
                  {/* Nube = el fichero YA vive en Drive (frente a la flecha de «subir»). */}
                  <MenuItem
                    size="sm"
                    icon={<Icon name="cloud" size={16} />}
                    onClick={() => {
                      close();
                      setDrivePickerOpen(true);
                    }}
                  >
                    {t('drive.pickTitle')}
                  </MenuItem>
                  {/* Objetos ricos. Van tras un separador porque no son «adjuntar algo»:
                  crean un mensaje con estructura propia (paridad Slack/Teams, donde
                  encuesta y GIF cuelgan de este mismo menú).

                  Encuesta y tarea SOLO en el compositor de canal (`advanced`): el de
                  respuestas entrega (body, mentions, adjuntos) sin `opts`, así que
                  ofrecerlas ahí publicaría la pregunta y tiraría la tarjeta en
                  silencio. ponytail: para permitirlas en un hilo hay que pasar `opts`
                  por onSendReply, no ampliar este menú. */}
                  {advanced ? (
                    <>
                      <MenuSeparator />
                      <MenuItem
                        size="sm"
                        icon={<Icon name="chart" size={16} />}
                        onClick={() => {
                          close();
                          setCardKind('poll');
                        }}
                      >
                        {t('card.newPoll')}
                      </MenuItem>
                      <MenuItem
                        size="sm"
                        icon={<Icon name="check" size={16} />}
                        onClick={() => {
                          close();
                          setCardKind('task');
                        }}
                      >
                        {t('card.newTask')}
                      </MenuItem>
                    </>
                  ) : null}
                  <MenuItem
                    size="sm"
                    icon={<ImageGlyph size={16} />}
                    onClick={() => {
                      close();
                      setGifPickerOpen(true);
                    }}
                  >
                    {t('gif.title')}
                  </MenuItem>
                  {onOpenIntegrations ? (
                    <>
                      <MenuSeparator />
                      <MenuItem
                        size="sm"
                        icon={<Icon name="grid" size={16} />}
                        onClick={() => {
                          close();
                          onOpenIntegrations();
                        }}
                      >
                        {t('integrations.apps')}
                      </MenuItem>
                    </>
                  ) : null}
                </>
              )}
            </Dropdown>

            {/* El emoji, pegado al «+»: los dos AÑADEN algo al mensaje. Deshabilitado con
              el resto del cromo — antes se quedaba vivo con el compositor bloqueado y
              dejaba insertar en un campo que no admitía escritura. */}
            <EmojiPickerButton
              onPick={insertGlyph}
              t={t}
              disabled={networkDisabled}
              label={t('composer.insertEmoji')}
              title={t('emoji.label')}
              triggerClass={BOTON_DEL_COMPOSER}
            />

            {/* Formato («Aa»): las opciones que no caben se alcanzan deslizando,
              también con el teclado del móvil abierto. */}
            <button
              type="button"
              disabled={networkDisabled}
              onClick={() => setFormatoVisible(!formatoVisible)}
              aria-pressed={formatoVisible}
              aria-label={t('composer.format')}
              title={t('composer.format')}
              class={cx(BOTON_DEL_COMPOSER, formatoVisible && 'bg-selected text-text')}
            >
              <Icon name="text" size={18} />
            </button>
            {formatoVisible ? (
              <div class="flex items-center gap-0.5" role="group" aria-label={t('composer.format')}>
                {MARCAS_DE_BARRA.map(([marca, icono, clave]) => (
                  <button
                    key={marca}
                    type="button"
                    disabled={networkDisabled}
                    onClick={() => aplicarMarca(marca)}
                    aria-label={t(clave)}
                    title={t(clave)}
                    class={BOTON_DEL_COMPOSER}
                  >
                    <Icon name={icono} size={16} />
                  </button>
                ))}
              </div>
            ) : null}

            {/* El pelo de la casa: a la izquierda lo que se AÑADE al mensaje, a la derecha
              CÓMO se envía (prioridad, programación). Sin `advanced` no hay segundo
              grupo, así que tampoco corte que insinuar. */}
            {advanced ? <span class={SEPARADOR_DE_BARRA} aria-hidden="true" /> : null}

            {/* Prioridad y acuse de recibo tras un «!» (Mattermost «Message Priority»,
              Teams «Opciones de envío»). Popover (role=dialog) porque lleva radios y
              casilla: el Dropdown se quedaría con las flechas del grupo de radios. */}
            {advanced ? (
              <Popover
                side="top"
                align="start"
                label={t('composer.priorityAndAck')}
                trigger={({ onClick, ...aria }) => (
                  <button
                    type="button"
                    disabled={networkDisabled}
                    onClick={onClick}
                    {...aria}
                    aria-label={t('composer.priorityAndAck')}
                    title={t('composer.priorityAndAck')}
                    // Sin tinta de estado. Pintaba el icono de rojo o de ámbar con una
                    // clase `text-*` que compite con la del propio botón —dos utilidades
                    // del mismo tipo las resuelve el orden de la HOJA generada, no el de
                    // la cadena—, y encima no lo anunciaba a nadie. La prioridad ya se ve
                    // dicha con todas las letras en el chip de encima de la caja.
                    class={BOTON_DEL_COMPOSER}
                  >
                    <PriorityGlyph size={18} />
                  </button>
                )}
              >
                <div class="w-56">
                  {/* ponytail: se aplica EN VIVO, sin pareja Cancelar/Aplicar: el chip que
                  aparece sobre la caja ya confirma el estado y su «✕» lo deshace. */}
                  <fieldset>
                    <legend class={POPOVER_LABEL}>{t('composer.priority')}</legend>
                    {PRIORITY_CHOICES.map((c) => (
                      <label key={c} class={POPOVER_OPTION}>
                        <Radio
                          name={`${optsId}-priority`}
                          value={c}
                          checked={priority === c}
                          onChange={() => setPriority(c)}
                          class="accent-primary h-4 w-4 shrink-0"
                        />
                        {t(`priority.${c}`)}
                      </label>
                    ))}
                  </fieldset>

                  <div class="bg-border-subtle my-1.5 h-px" role="separator" />

                  <label class={POPOVER_OPTION}>
                    <Checkbox
                      checked={requiresAck}
                      onChange={(e) => setRequiresAck(e.currentTarget.checked)}
                      class="accent-primary h-4 w-4 shrink-0"
                    />
                    {t('composer.requestAckLong')}
                  </label>
                </div>
              </Popover>
            ) : null}

            {/* «Enviar más tarde». Era el CHEVRON de un botón partido pegado a Enviar, y
              ese partido salía medio encendido: sin nada escrito, la mitad de enviar
              iba deshabilitada al 50 % y el chevron seguía a todo color, así que el
              control leía como roto justo en el estado en el que más se ve —vacío—. Y
              programar es una OPCIÓN de envío, exactamente igual que la prioridad, así
              que vive donde su hermana y con su misma forma. */}
            {advanced ? (
              <Popover
                side="top"
                align="start"
                label={t('composer.sendLater')}
                trigger={({ onClick, ...aria }) => (
                  <button
                    type="button"
                    disabled={networkDisabled}
                    onClick={onClick}
                    {...aria}
                    aria-label={t('composer.sendLater')}
                    title={t('composer.sendLater')}
                    class={BOTON_DEL_COMPOSER}
                  >
                    <ClockGlyph size={18} />
                  </button>
                )}
              >
                {(close) => (
                  <div class="w-60">
                    <p class={POPOVER_LABEL}>{t('composer.sendLater')}</p>
                    {/* Atajos RELATIVOS calculados en cliente: nada de calendario del
                      sistema mientras no se pida expresamente. */}
                    {schedulePresets().map((p) => (
                      <button
                        key={p.value}
                        type="button"
                        onClick={() => {
                          setScheduleAt(p.value);
                          setCustomDate(false);
                          close();
                        }}
                        class={POPOVER_ITEM}
                      >
                        <ClockGlyph size={16} />
                        {t(p.key, { time: formatTime(locale, p.at) })}
                      </button>
                    ))}
                    {customDate ? (
                      // Revelado SOLO al pedirlo: el control nativo de fecha nunca vive en
                      // el cromo, y aquí va etiquetado y estilado con los tokens.
                      <div class="mt-1.5 px-1.5">
                        <label
                          class="text-text-muted mb-1 block text-xs font-medium"
                          for={`${optsId}-when`}
                        >
                          {t('composer.dateAndTime')}
                        </label>
                        <input
                          id={`${optsId}-when`}
                          type="datetime-local"
                          value={scheduleAt}
                          min={minFecha}
                          onInput={(e) => setScheduleAt(e.currentTarget.value)}
                          class="border-border bg-bg text-text sytue-focus-ring w-full rounded-md border px-2 py-1.5 text-sm outline-none"
                        />
                        <div class="mt-2 flex justify-end">
                          <Button size="sm" disabled={!scheduleAt} onClick={close}>
                            {t('action.done')}
                          </Button>
                        </div>
                      </div>
                    ) : (
                      <button
                        type="button"
                        onClick={() => {
                          setMinFecha(toLocalValue(Date.now()));
                          setCustomDate(true);
                        }}
                        class={POPOVER_ITEM}
                      >
                        <Icon name="ellipsis" size={16} class="text-text-muted" />
                        {t('composer.customDateTime')}
                      </button>
                    )}
                  </div>
                )}
              </Popover>
            ) : null}
          </div>

          {/* Enviar: el ÚNICO control con caja de toda la barra, y por eso se lee como
              la acción principal sin que nadie lo explique.
              `ghost` mientras no hay nada que enviar: un relleno de acento encendido en
              cada canal y en cada hilo con la caja vacía es lo más ruidoso del panel, y
              en una sala lo que importa son los mensajes. Se enciende al haber texto o
              adjunto listo. Misma talla en las dos variantes: no salta nada. */}
          <Button
            variant={canSend ? 'primary' : 'ghost'}
            size="sm"
            iconOnly
            disabled={!canSend}
            aria-label={scheduleAt ? t('composer.schedule') : t('composer.send')}
            title={scheduleAt ? t('composer.schedule') : t('composer.sendHint')}
            onClick={submit}
            class="shrink-0"
          >
            <SendGlyph size={18} />
          </Button>
        </div>
      </div>
      {/* Las pistas de atajos, SOLO para el lector de pantalla (`aria-describedby` del
          textarea, que las anuncia al enfocar). Eran una tira encendida de cuatro pares
          tecla+texto bajo CADA compositor —canal e hilo—: a 320 px se partía en tres
          líneas y le robaba ese alto a los mensajes para enseñar algo que hace falta una
          vez. El «Intro» sigue estando a la vista donde se busca: en el `title` del
          botón de enviar («Enviar (Intro)»). */}
      <p id={hintsId} class="sr-only">
        {t('key.enter')} {t('composer.hintSend')}, @ {t('composer.hintMention')}, :{' '}
        {t('composer.hintEmoji')}, / {t('composer.hintCommands')}
      </p>

      {/* Selector de un fichero YA existente de Drive (menú «+»): adjunta por
          referencia (fileId), sin volver a subir los bytes. */}
      {drivePickerOpen ? (
        <DrivePickerModal
          onClose={() => setDrivePickerOpen(false)}
          onPick={addDriveAttachment}
          t={t}
          locale={locale}
        />
      ) : null}

      {/* Encuesta / tarea (menú «+»): el modal solo COMPONE la tarjeta; publicar es el
          `onSend` de siempre, así que hereda el envío optimista y el fan-out por WS. */}
      <CardComposerModal
        kind={cardKind}
        onClose={() => setCardKind(null)}
        onSubmit={(body, card) => {
          setCardKind(null);
          onSend(body, [], [], { card });
        }}
        {...(members?.length
          ? { members: members.map((m) => ({ subject: m.sub, name: m.name })) }
          : {})}
        t={t}
      />

      {/* GIF: catálogo de imágenes animadas de la organización. Entra como atajo
          `:nombre:` en el texto (no como adjunto): el catálogo es del tenant y se sirve
          igual en todos los canales, mientras que un adjunto vive en uno solo. */}
      {gifPickerOpen ? (
        <GifPickerModal
          onClose={() => setGifPickerOpen(false)}
          onPick={(shortcode) => {
            setGifPickerOpen(false);
            insertGlyph(shortcode);
          }}
          t={t}
        />
      ) : null}
      {dialogoConfirmar}
    </div>
  );
}
