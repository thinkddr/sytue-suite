/**
 * MessageView — historico de un canal de Sytue Chat, estilo Mattermost: agrupa
 * mensajes consecutivos del mismo autor, separadores de dia en pildora, acciones
 * al pasar el cursor (reacciones rapidas), fila de reacciones/indicador de hilo y
 * auto-scroll al final cuando llega un mensaje nuevo y el usuario ya estaba
 * abajo. docs/design.md §2, §7.
 *
 * Virtualizado con <VirtualList> (@sytue/ui): solo se pintan las filas visibles
 * + overscan (obligatorio a decenas de miles de mensajes). `stickToBottom`
 * arranca abajo y mantiene el fondo estable; al acercarse al principio se pide
 * una pagina mas de historial (cursor `before` en ChatApp).
 */
import type { JSX, RefObject } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Avatar,
  Skeleton,
  EmptyState,
  Icon,
  Button,
  Toolbar,
  VirtualList,
  cx,
  useToast,
  ConfirmDialog,
  Modal,
  Spinner,
  Illustration,
  Glifo,
} from '@sytue/ui';
import type { VirtualListHandle } from '@sytue/ui';
import { formatDate, formatDateTime, formatTime, type Locale } from '@sytue/i18n';
import { appUrl, orgActual } from '@sytue/config';
import {
  messageLink,
  messageReaders,
  type ChatMessage,
  type MessageReader,
} from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { callEndedSeconds, formatCallDuration } from '../lib/call.js';
import { copyToClipboard } from '../lib/clipboard.js';
import { revelaPorToque } from '../lib/popover.js';
import { reactedByLabel } from '../lib/reactions.js';
import {
  PinGlyph,
  BookmarkGlyph,
  CopyGlyph,
  LinkGlyph,
  UnreadGlyph,
  ForwardGlyph,
  EditGlyph,
  TrashGlyph,
  ThreadsGlyph,
  SeenGlyph,
  NoteGlyph,
} from './glyphs.js';
import { MessageMenu, type MenuEntry } from './MessageMenu.js';
import { BlockedFold } from './Moderation.js';
import { MessageBody, EmojiToken, extractLinks } from './MessageBody.js';
import { LinkPreview, MeetingLinkCard } from './LinkPreview.js';
import { proveedorDeReunion } from '../lib/enlacesExternos.js';
import { Attachments } from './Attachments.js';
import { CardView } from './CardView.js';
import { EmojiPickerButton } from './EmojiPicker.js';

interface Props {
  messages: ChatMessage[];
  loading: boolean;
  /** Error al cargar el historico (muestra estado de error con reintento). */
  error?: boolean;
  /** Nombre del canal activo (para el estado vacio). */
  channelName?: string;
  /** sub -> nombre legible, para resaltar @menciones en el cuerpo. */
  memberNames?: Record<string, string>;
  /** Alterna una reaccion del usuario sobre un mensaje. */
  onReact?: (messageId: string, emoji: string) => void;
  /** Subs de las personas bloqueadas: sus mensajes se ven PLEGADOS (ADR 0073). */
  blockedSubs?: ReadonlySet<string>;
  /** Abre el diálogo de denuncia del mensaje. */
  onReport?: (m: ChatMessage) => void;
  /** Bloquea o desbloquea al autor del mensaje. */
  onToggleBlock?: (m: ChatMessage, blocked: boolean) => void;
  /** Envia un mensaje (CTA del estado vacio). */
  /** Reintenta la carga del historico tras un error. */
  onRetry?: () => void;
  /** Reintenta el envio de un mensaje fallido. */
  onResend?: (message: ChatMessage) => void;
  /** Abre el panel de hilo de un mensaje raíz. */
  onOpenThread?: (message: ChatMessage) => void;
  /** Guarda la edición del cuerpo de un mensaje propio. */
  onEdit?: (messageId: string, body: string) => void;
  /** Borra un mensaje propio. */
  onDelete?: (messageId: string) => void;
  /** IDs de mensajes fijados en el canal activo (para alternar Fijar/Desfijar). */
  pinnedIds?: Set<string>;
  /** IDs de mensajes guardados por el usuario (para alternar Guardar/Quitar). */
  savedIds?: Set<string>;
  /** Alterna el fijado de un mensaje (pinned = estado actual). */
  onTogglePin?: (message: ChatMessage, pinned: boolean) => void;
  /** Alterna el guardado de un mensaje (saved = estado actual). */
  onToggleSave?: (message: ChatMessage, saved: boolean) => void;
  /** Ids de mensajes cuyo hilo SIGUE el usuario, de los que se sabe (ver followedIds
   *  en ChatApp): pinta «Siguiendo» y alterna la entrada del menú. */
  followedIds?: Set<string>;
  /** Alterna seguir/dejar de seguir el hilo de un mensaje (following = estado actual). */
  onToggleFollow?: (message: ChatMessage, following: boolean) => void;
  /** Deja el canal como no leído a partir de este mensaje. */
  onMarkUnread?: (message: ChatMessage) => void;
  /** Confirma el acuse de recibo de un mensaje que lo pide (Ola 12). */
  onAck?: (message: ChatMessage) => void;
  /** Marca o retira un valor de la tarjeta de un mensaje (votar una opción de la
   *  encuesta, dar por hecha la tarea). Ausente = tarjetas de solo lectura. */
  onCardVote?: (messageId: string, value: string, add: boolean) => void;
  /** Reenvía el mensaje a otro canal (abre el selector de destino en el padre). */
  onForward?: (message: ChatMessage) => void;
  /** Id del primer mensaje no leído: pinta el separador «Nuevos mensajes» ante él. */
  unreadDividerId?: string | null;
  /** Id del mensaje a RESALTAR brevemente (permalink / scroll-to-message, Ola 16). */
  highlightId?: string | null;
  /** Indicador «Visto» de un DM (Ola 16): id de TU último mensaje + etiqueta accesible
   *  («Visto» / «Visto por N»). El padre lo calcula del last_read_at de los peers. */
  seen?: { messageId: string; label: string } | null;
  /** Unirse a la llamada anotada por una tarjeta type=call del timeline. */
  onJoinCall?: (message: ChatMessage) => void;
  /** Pide una pagina mas de historial (el usuario se acerca al principio). */
  onLoadOlder?: () => void;
  /** Pagina anterior en vuelo: pildora «Cargando…» sobre la lista. */
  loadingOlder?: boolean;
  /** Handle imperativo de la lista (ChatApp lo usa para el cap con isAtBottom). */
  handleRef?: RefObject<VirtualListHandle>;
  /** Idioma activo (fechas y reloj) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

/** Reacciones rapidas de la barra de acciones (paridad Mattermost). */
const QUICK_REACTIONS = ['👍', '❤️', '😄', '🎉'] as const;

/** Altura estimada de una fila de mensaje (px); la real se mide del DOM. */
const ESTIMATED_ROW_PX = 56;

/** A menos de este scrollTop del principio se pide la pagina anterior. */
const LOAD_OLDER_PX = 300;

/**
 * Etiqueta del separador de día. «Hoy»/«Ayer» salen del catálogo; el resto es una
 * fecha larga formateada por `Intl` en el idioma del usuario. La mayúscula inicial
 * NO se fuerza: en alemán e inglés el día ya va en mayúscula y en español/francés/
 * italiano va en minúscula a propósito (capitalizarla a mano sería un error de
 * ortografía en tres de las cinco lenguas).
 */
function dayLabel(locale: Locale, t: T, iso: string): string {
  const d = new Date(iso);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return t('day.today');
  if (d.toDateString() === yesterday.toDateString()) return t('day.yesterday');
  return formatDate(locale, d, { weekday: 'long', day: 'numeric', month: 'long' });
}

function timeLabel(locale: Locale, iso: string): string {
  return formatTime(locale, iso, { hour: '2-digit', minute: '2-digit' });
}

/** Fecha completa para el title de los timestamps truncados. */
function fullDateLabel(locale: Locale, iso: string): string {
  return formatDateTime(locale, iso, { dateStyle: 'full', timeStyle: 'short' });
}

/** Separador de día en píldora (compartido por la fila normal y la tarjeta de llamada). */
function dayDivider(locale: Locale, t: T, iso: string): JSX.Element {
  return (
    <div class="my-4 flex items-center gap-3" role="separator">
      <span class="bg-border h-px flex-1" />
      <span class="rounded-pill border-border bg-surface text-text-muted border px-3 py-0.5 text-xs font-semibold">
        {dayLabel(locale, t, iso)}
      </span>
      <span class="bg-border h-px flex-1" />
    </div>
  );
}

/** Badge de prioridad (Ola 12): acento + TEXTO (nunca solo color, WCAG AA). El
 *  texto («Importante»/«Urgente») lleva el significado; el color solo lo refuerza. */
function priorityBadge(t: T, priority: 'important' | 'urgent'): JSX.Element {
  const urgent = priority === 'urgent';
  return (
    <span
      class={cx(
        'rounded-pill sytue-overline inline-flex items-center border px-2 py-0.5',
        urgent
          ? 'border-danger bg-danger-bg text-danger-text'
          : 'border-border bg-warning-bg text-warning-text',
      )}
    >
      {urgent ? t('priority.urgent') : t('priority.important')}
    </span>
  );
}

/** Separador «Nuevos mensajes» (Ola 8): línea de acento ante el primer no leído
 *  al abrir el canal. Solo marcador visual de esta apertura (no re-marca no leído). */
function unreadDivider(t: T): JSX.Element {
  return (
    <div
      class="my-3 flex items-center gap-3"
      role="separator"
      aria-label={t('message.newMessages')}
    >
      <span class="bg-primary h-px flex-1" />
      <span class="rounded-pill bg-primary px-3 py-0.5 text-xs font-semibold text-[var(--sytue-on-primary)]">
        {t('message.newMessages')}
      </span>
      <span class="bg-primary h-px flex-1" />
    </div>
  );
}

export function MessageView({
  messages,
  loading,
  error = false,
  channelName,
  memberNames,
  onReact,
  blockedSubs,
  onReport,
  onToggleBlock,
  onRetry,
  onResend,
  onOpenThread,
  onEdit,
  onDelete,
  pinnedIds,
  savedIds,
  onTogglePin,
  onToggleSave,
  followedIds,
  onToggleFollow,
  onMarkUnread,
  onForward,
  onAck,
  onCardVote,
  unreadDividerId,
  highlightId,
  seen,
  onJoinCall,
  onLoadOlder,
  loadingOlder = false,
  handleRef,
  locale,
  t,
}: Props): JSX.Element {
  // Handle propio si el padre no pide uno (el stick de abajo lo necesita).
  const ownHandle = useRef<VirtualListHandle>(null);
  const listHandle = handleRef ?? ownHandle;
  const toast = useToast();

  async function copyAndToast(text: string, okTitle: string): Promise<void> {
    const ok = await copyToClipboard(text);
    toast.notify(
      ok ? { tone: 'success', title: okTitle } : { tone: 'danger', title: t('message.copyFailed') },
    );
  }

  /** Construye las entradas del menú «…» de un mensaje (según estado y permisos). */
  function menuItems(m: ChatMessage): MenuEntry[] {
    const pinned = pinnedIds?.has(m.id) ?? false;
    const saved = savedIds?.has(m.id) ?? false;
    const items: MenuEntry[] = [];
    if (onTogglePin) {
      items.push({
        key: 'pin',
        label: pinned ? t('message.unpin') : t('message.pin'),
        icon: <PinGlyph size={16} />,
        onSelect: () => onTogglePin(m, pinned),
      });
    }
    if (onToggleSave) {
      items.push({
        key: 'save',
        label: saved ? t('message.unsave') : t('action.save'),
        icon: <BookmarkGlyph size={16} />,
        onSelect: () => onToggleSave(m, saved),
      });
    }
    // «Seguir mensaje» (paridad Mattermost): solo en mensajes RAÍZ — el servicio
    // rechaza seguir una respuesta (exige parent_id IS NULL).
    if (onToggleFollow && !m.parentId) {
      const following = followedIds?.has(m.id) ?? false;
      items.push({
        key: 'follow',
        label: following ? t('thread.unfollowShort') : t('message.follow'),
        icon: <ThreadsGlyph size={16} />,
        onSelect: () => onToggleFollow(m, following),
      });
    }
    items.push({
      key: 'copy',
      label: t('message.copyText'),
      icon: <CopyGlyph size={16} />,
      onSelect: () => void copyAndToast(m.body, t('message.copied')),
    });
    items.push({
      key: 'link',
      label: t('message.copyLink'),
      icon: <LinkGlyph size={16} />,
      onSelect: () => void copyAndToast(messageLink(m.channelId, m.id), t('message.linkCopied')),
    });
    // «Guardar en Notes»: abre Sytue Notepads con una nota nueva ya vinculada a este
    // mensaje. NO se copia el texto al otro lado — se guarda la REFERENCIA, así
    // que quien abra la nota sigue pasando por el permiso del canal.
    //
    // El host sale de `appDomains` (@sytue/config), que ya lo deriva del emisor
    // OIDC del entorno: es lo mismo que hace el conmutador de aplicaciones, y
    // ahorra bajar una URL más por props hasta aquí. La organización (ADR 0056)
    // es la de la URL vista (`orgActual`): la referencia apunta al mensaje de
    // ESTA organización, y en el navegador el evento siempre la tiene a mano.
    items.push({
      key: 'to-notes',
      label: t('message.saveToNotes'),
      icon: <NoteGlyph size={16} />,
      onSelect: () => {
        const rotulo = m.body.trim().slice(0, 120);
        window.open(
          `${appUrl('notes', '', orgActual('/rooms'))}/?link=room_message:${encodeURIComponent(m.id)}&label=${encodeURIComponent(rotulo)}`,
          '_blank',
          'noopener,noreferrer',
        );
      },
    });
    if (onForward) {
      items.push({
        key: 'forward',
        label: t('message.forward'),
        icon: <ForwardGlyph size={16} />,
        onSelect: () => onForward(m),
      });
    }
    if (onMarkUnread) {
      items.push({
        key: 'unread',
        label: t('message.markUnread'),
        icon: <UnreadGlyph size={16} />,
        onSelect: () => onMarkUnread(m),
      });
    }
    // «Visto por» solo en los PROPIOS: saber quién ha leído lo que escribió otro
    // no ayuda a nadie a trabajar y sí convierte el chat en un panel de control
    // sobre los compañeros.
    if (m.mine) {
      items.push({
        key: 'readers',
        label: t('message.readersTitle'),
        icon: <SeenGlyph size={16} />,
        onSelect: () => void abrirLectores(m.id),
      });
    }
    if (m.mine && (onEdit || onDelete)) {
      items.push('separator');
      if (onEdit) {
        items.push({
          key: 'edit',
          label: t('action.edit'),
          icon: <EditGlyph size={16} />,
          onSelect: () => startEdit(m),
        });
      }
      if (onDelete) {
        items.push({
          key: 'delete',
          label: t('action.delete'),
          icon: <TrashGlyph size={16} />,
          danger: true,
          onSelect: () => confirmDelete(m.id),
        });
      }
    }
    // Denunciar y bloquear: solo sobre lo AJENO y de una persona (un bot no se
    // bloquea: se quita la integración).
    if (!m.mine && !m.isBot && (onReport || onToggleBlock)) {
      const blocked = blockedSubs?.has(m.author.id) ?? false;
      items.push('separator');
      if (onReport) {
        items.push({
          key: 'report',
          label: t('report.message'),
          icon: <Icon name="warning" size={16} />,
          danger: true,
          onSelect: () => onReport(m),
        });
      }
      if (onToggleBlock) {
        items.push({
          key: 'block',
          label: t(blocked ? 'block.unblock' : 'block.block', { name: m.author.name }),
          danger: !blocked,
          onSelect: () => onToggleBlock(m, blocked),
        });
      }
    }
    return items;
  }
  // Mensajes de personas bloqueadas que se han abierto con «Mostrar».
  const [mostrados, setMostrados] = useState<ReadonlySet<string>>(new Set());
  // Quién ha visto un mensaje. Se pide al abrirlo y no con el histórico: en una
  // conversación larga serían cientos de consultas para un dato que casi nunca se
  // mira, y el que importa —el del último mensaje propio— se ve igual de bien.
  const [lectoresDe, setLectoresDe] = useState<string | null>(null);
  const [lectores, setLectores] = useState<{ readers: MessageReader[]; total: number } | null>(
    null,
  );

  async function abrirLectores(messageId: string): Promise<void> {
    setLectoresDe(messageId);
    setLectores(null);
    try {
      setLectores(await messageReaders(messageId));
    } catch {
      // Sin datos se enseña el estado vacío: decir «no se pudo» en un panel que
      // se abre de un menú es ruido, y volver a abrirlo reintenta.
      setLectores({ readers: [], total: 0 });
    }
  }

  // Edición inline: id del mensaje en edición (null = ninguno) y borrador del cuerpo.
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState('');

  // Mensaje cuya barra de acciones está desplegada POR TOQUE (null = ninguno).
  //
  // En un teléfono no existe «pasar el cursor», y Tailwind 4 mete `group-hover`
  // dentro de `@media (hover: hover)`: con el dedo esa consulta es falsa SIEMPRE, así
  // que la barra se quedaba en `opacity-0 pointer-events-none` de forma permanente y
  // reaccionar, responder en hilo, editar, borrar, fijar, guardar, copiar el enlace,
  // reenviar, marcar no leído y «visto por» sencillamente no existían en el móvil.
  // El gesto equivalente es tocar el mensaje —lo que hacen Teams y Slack—: abre la
  // barra de ESE mensaje y solo ese, y volver a tocarlo la cierra.
  const [tocado, setTocado] = useState<string | null>(null);
  // Con qué se pulsó lo último. El `click` de Preact se tipa como MouseEvent y ahí ya
  // no se puede preguntar el `pointerType`, así que se anota en el `pointerdown` que
  // siempre lo precede (mismo reparto que el selector de emoji). Con RATÓN no se toca
  // nada: en escritorio sigue mandando el hover de siempre.
  const ultimoPuntero = useRef('');

  function startEdit(m: ChatMessage): void {
    setEditingId(m.id);
    setDraft(m.body);
  }
  function cancelEdit(): void {
    setEditingId(null);
    setDraft('');
  }
  function saveEdit(original: string): void {
    const body = draft.trim();
    if (body && body !== original) onEdit?.(editingId!, body);
    cancelEdit();
  }
  // Borrar un mensaje se confirma con el diálogo de la casa, no con el del
  // navegador: aquel pinta sus botones en el idioma del sistema operativo.
  const [borrando, setBorrando] = useState<string | null>(null);
  function confirmDelete(messageId: string): void {
    setBorrando(messageId);
  }

  // Sigue los mensajes nuevos si el usuario ya estaba abajo (umbral 160px del
  // handle, como la versión sin virtualizar: se evalúa tras pintar el mensaje).
  // Cubre también los appends con la lista capada (mismo count: el core no los
  // detecta como append y no los sigue solo). Excepción: un prepend puro de
  // historial (cambia el primer id, el último no) NO tira al fondo — en
  // viewports muy altos «arriba» y «abajo» caben en el umbral a la vez y el
  // scroll saltaría justo tras cargar la página anterior.
  const edgeIdsRef = useRef<{ first: string | undefined; last: string | undefined }>({
    first: undefined,
    last: undefined,
  });
  useEffect(() => {
    const first = messages[0]?.id;
    const last = messages[messages.length - 1]?.id;
    const prevEdges = edgeIdsRef.current;
    edgeIdsRef.current = { first, last };
    // Mientras hay un mensaje resaltado (permalink / scroll-to-message), NO tires al
    // fondo: el foco es el mensaje enlazado, no el final de la ventana.
    if (highlightId) return;
    if (first !== prevEdges.first && last === prevEdges.last) return; // prepend
    const h = listHandle.current;
    if (h?.isAtBottom()) h.scrollToBottom();
  }, [messages, listHandle, highlightId]);

  // Scroll-to-message (Ola 16): al fijarse un highlightId NUEVO, centra su fila en el
  // viewport (una sola vez por objetivo). El resalte visual lo pinta renderRow; el padre
  // limpia highlightId tras ~2,5 s. rAF: espera al primer pintado de la ventana cargada.
  const lastHighlightRef = useRef<string | null>(null);
  useEffect(() => {
    if (!highlightId) {
      lastHighlightRef.current = null;
      return;
    }
    if (highlightId === lastHighlightRef.current) return;
    const idx = messages.findIndex((m) => m.id === highlightId);
    if (idx < 0) return;
    lastHighlightRef.current = highlightId;
    const raf = requestAnimationFrame(() =>
      listHandle.current?.scrollToIndex(idx, { align: 'center' }),
    );
    return () => cancelAnimationFrame(raf);
  }, [highlightId, messages, listHandle]);

  // Clave estable por índice: imprescindible para prepend de historial sin saltos.
  const getItemKey = useMemo(() => (i: number) => messages[i]?.id ?? i, [messages]);

  // «Saltar a no leído»: índice del primer no leído (o -1). El botón flotante se
  // muestra mientras haya separador y no se haya usado; se reactiva al abrir otro
  // canal (cambia unreadDividerId). ponytail: se oculta al pulsarlo, no al llegar
  // el separador al viewport (haría falta rastrear la visibilidad de la fila).
  const [jumpUsed, setJumpUsed] = useState(false);
  useEffect(() => setJumpUsed(false), [unreadDividerId]);
  const unreadIndex = useMemo(
    () => (unreadDividerId ? messages.findIndex((m) => m.id === unreadDividerId) : -1),
    [messages, unreadDividerId],
  );
  const showJump = unreadIndex >= 0 && !jumpUsed;
  function jumpToUnread(): void {
    if (unreadIndex < 0) return;
    listHandle.current?.scrollToIndex(unreadIndex, { align: 'start' });
    setJumpUsed(true);
  }

  if (loading) {
    // Carga: esqueletos con la geometria real de una fila de mensaje (avatar +
    // autor + cuerpo), no un spinner suelto — evita salto de layout (§3.7).
    return (
      <div class="min-h-0 flex-1 overflow-y-auto px-4 py-3" aria-busy="true">
        <span role="status" class="sr-only">
          {t('message.loading')}
        </span>
        {[70, 48, 62, 40, 55].map((w, i) => (
          <div key={i} class="mt-5 flex gap-3">
            <Skeleton width="2rem" height="2rem" class="shrink-0" />
            <div class="flex min-w-0 flex-1 flex-col gap-2 pt-1">
              <Skeleton width="7rem" height="0.75rem" />
              <Skeleton width={`${w}%`} height="0.75rem" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <EmptyState
        class="h-full flex-1"
        title={t('message.loadFailed')}
        description={t('error.checkConnection')}
        illustration={<Icon name="danger" size={32} class="text-danger-text" />}
        action={
          onRetry ? (
            <Button size="sm" variant="secondary" onClick={onRetry}>
              {t('action.retry')}
            </Button>
          ) : undefined
        }
      />
    );
  }

  if (messages.length === 0) {
    return (
      <EmptyState
        illustration={<Illustration name="emptyChat" />}
        class="h-full flex-1"
        title={
          channelName ? t('message.channelStart', { channel: channelName }) : t('message.noneYet')
        }
        description={t('message.writeFirst')}
      />
    );
  }

  /** Una fila del histórico: separador de día (si cambia) + mensaje. El estado
   *  «agrupado» se deriva del mensaje anterior por índice (las filas se pintan
   *  sueltas al virtualizar, no en una pasada secuencial). */
  // Solo la ÚLTIMA tarjeta de llamada, y solo si es un «inició», ofrece
  // «Unirse». Antes lo ofrecía cada «inició» del histórico: una llamada de hace
  // tres días, con su «finalizada» justo debajo, seguía invitando a entrar, y
  // quien pulsaba aterrizaba en una sala vacía.
  let ultimaLlamadaId: string | null = null;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i]?.type === 'call') {
      ultimaLlamadaId = messages[i]!.id;
      break;
    }
  }

  function renderRow(m: ChatMessage, index: number): JSX.Element {
    const prev = index > 0 ? messages[index - 1] : undefined;
    const day = new Date(m.createdAt).toDateString();
    const showDay = !prev || new Date(prev.createdAt).toDateString() !== day;

    // Tarjeta de sistema de llamada (Sytue Rooms): fila centrada, sin avatar ni
    // agrupación. Cuerpo VACÍO = «inició una llamada» (acción «Unirse»); cuerpo con la
    // duración en segundos = «Llamada finalizada · N» (sin «Unirse», la sala ya cerró).
    if (m.type === 'call') {
      const endedSec = callEndedSeconds(m.body);
      const ended = endedSec !== null;
      // La única que importa es la que se puede unir: la última y aún abierta.
      const destacada = !ended && m.id === ultimaLlamadaId;
      return (
        <div class={cx(index === 0 && 'pt-3', index === messages.length - 1 && 'pb-3')}>
          {showDay ? dayDivider(locale, t, m.createdAt) : null}
          {m.id === unreadDividerId ? unreadDivider(t) : null}
          {/* Solo la llamada ABIERTA es una tarjeta; las terminadas Y los «inició» de
              días pasados son una línea sin caja: un canal con reuniones diarias era
              una pila de cajas iguales en la que la única que importa —la que se
              puede unir— no destacaba. */}
          <div
            class={cx(
              'mx-2 mt-2 flex items-center gap-3 rounded-md px-3',
              destacada ? 'border-border bg-bg-subtle border py-2' : 'text-text-muted py-1',
            )}
          >
            <span
              class={cx(
                'grid shrink-0 place-items-center rounded-full',
                destacada
                  ? 'bg-selected text-primary-text h-8 w-8'
                  : 'bg-border text-text-muted h-6 w-6',
              )}
              aria-hidden="true"
            >
              {ended ? (
                // Cámara con barra: la llamada terminó.
                <Glifo size={16}>
                  <path d="m1 1 22 22" />
                  <path d="M23 7 16 12l3.5 2.5V7Z" />
                  <path d="M14 5h-9a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h9" />
                </Glifo>
              ) : (
                <Glifo size={16}>
                  <path d="M23 7 16 12l7 5V7Z" />
                  <rect x="1" y="5" width="15" height="14" rx="2" />
                </Glifo>
              )}
            </span>
            <div class="min-w-0 flex-1">
              <p class={cx('truncate text-sm', destacada ? 'text-text' : 'text-text-muted')}>
                {ended
                  ? t('call.ended', { duration: formatCallDuration(t, endedSec) })
                  : t('call.started', { name: m.author.name })}
              </p>
              <span class="sytue-meta" title={fullDateLabel(locale, m.createdAt)}>
                {timeLabel(locale, m.createdAt)}
              </span>
            </div>
            {!ended && onJoinCall && m.id === ultimaLlamadaId ? (
              <Button size="sm" variant="secondary" onClick={() => onJoinCall(m)}>
                {t('action.join')}
              </Button>
            ) : null}
          </div>
        </div>
      );
    }

    // Una tarjeta de llamada NO ancla un grupo: es un aviso del sistema, sin
    // avatar ni nombre, y el mensaje que venía detrás del mismo autor salía
    // huérfano —solo el texto, sin quién ni cuándo— porque «continuaba» a la
    // tarjeta.
    const grouped = !showDay && prev?.author.id === m.author.id && prev?.type !== 'call';

    return (
      // pt/pb sustituyen al py-3 del contenedor: dentro de la fila medida, el
      // virtualizador calcula los offsets exactos (padding del scroller no).
      <div class={cx(index === 0 && 'pt-3', index === messages.length - 1 && 'pb-3')}>
        {showDay ? dayDivider(locale, t, m.createdAt) : null}
        {m.id === unreadDividerId ? unreadDivider(t) : null}

        <div
          class={cx(
            'group hover:bg-bg-subtle relative flex gap-3 rounded-md px-2 transition-colors duration-150',
            grouped ? 'py-0.5' : 'mt-2 py-1',
            // Resalte transitorio del mensaje enlazado (permalink / búsqueda): anillo +
            // tinte, se desvanece cuando el padre limpia highlightId (~2,5 s).
            m.id === highlightId && 'bg-selected ring-primary ring-2',
          )}
        >
          {m.id === highlightId ? <span class="sr-only">{t('message.fromLink')}</span> : null}
          {/* Barra de acciones al pasar el cursor: reacciones rapidas +
              responder en hilo. Visible tambien al foco por teclado (opacity,
              no display:none, para que los botones sigan siendo tabulables — AA)
              y, con el dedo, al tocar el mensaje (ver `tocado`). */}
          {!m.pending && !m.failed ? (
            <Toolbar
              class={cx(
                'sytue-popover-surface border-border absolute -top-3.5 right-2 z-10 flex items-center gap-0.5 rounded-md border p-0.5 shadow-[var(--shadow-sm)] transition-opacity duration-150 group-hover:pointer-events-auto group-hover:opacity-100 focus-within:pointer-events-auto focus-within:opacity-100',
                // Desplegada por toque: sin `opacity-0`, para no depender de qué
                // utilidad gana el orden de la hoja entre `opacity-0` y `opacity-100`.
                m.id === tocado ? 'opacity-100' : 'pointer-events-none opacity-0',
              )}
              aria-label={t('composer.messageActions')}
            >
              {onReact
                ? QUICK_REACTIONS.map((emoji) => (
                    <button
                      key={emoji}
                      type="button"
                      aria-label={t('reaction.with', { emoji })}
                      onClick={() => onReact(m.id, emoji)}
                      class="hover:bg-bg-subtle flex h-7 w-7 items-center justify-center rounded text-sm transition-colors duration-150"
                    >
                      {emoji}
                    </button>
                  ))
                : null}
              {/* Selector de emoji completo: reaccionar con cualquiera, no solo el set rápido. */}
              {onReact ? (
                <EmojiPickerButton
                  onPick={(glyph) => onReact(m.id, glyph)}
                  t={t}
                  label={t('reaction.withAny')}
                  title={t('reaction.react')}
                  size={16}
                  triggerClass="flex h-7 w-7 items-center justify-center rounded text-text-muted transition-colors duration-150 hover:bg-bg-subtle hover:text-text sytue-focus-ring"
                />
              ) : null}
              {onOpenThread ? (
                <button
                  type="button"
                  aria-label={t('message.replyInThread')}
                  title={t('message.replyInThread')}
                  onClick={() => onOpenThread(m)}
                  class="text-text-muted hover:bg-bg-subtle hover:text-text sytue-focus-ring flex h-7 w-7 items-center justify-center rounded transition-colors duration-150"
                >
                  <Glifo
                    d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"
                    size={16}
                  />
                </button>
              ) : null}
              {/* Resto de acciones (fijar, guardar, copiar, marcar no leído y —en
                  mensajes propios— editar/borrar) en un menú «…» accesible. Se
                  renderiza en un portal para no recortarse en el scroller. */}
              <MessageMenu label={t('message.moreActions')} items={menuItems(m)} />
            </Toolbar>
          ) : null}

          <div class="w-9 shrink-0 pt-0.5">
            {!grouped ? (
              <Avatar name={m.author.name} size="sm" />
            ) : (
              <span
                // `pointer-coarse`: con el dedo el hover no llega nunca, y la hora de
                // un mensaje agrupado es justo lo que se busca al releer una conversación.
                class="sytue-meta block pt-1 text-right leading-4 opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100"
                title={fullDateLabel(locale, m.createdAt)}
              >
                {timeLabel(locale, m.createdAt)}
              </span>
            )}
          </div>
          {/* Tocar el mensaje despliega su barra de acciones (ver `tocado`). No lleva
              manejador de teclado a propósito: quien navega con Tab ya la abre con
              `focus-within`, y esto es el sustituto TÁCTIL del hover, no un control. */}
          {/* oxlint-disable-next-line jsx-a11y/no-static-element-interactions, jsx-a11y/click-events-have-key-events */}
          <div
            class="min-w-0 flex-1"
            onPointerDown={(e) => {
              ultimoPuntero.current = e.pointerType;
            }}
            onClick={() => {
              if (!revelaPorToque(ultimoPuntero.current) || editingId === m.id) return;
              setTocado((prev) => (prev === m.id ? null : m.id));
            }}
          >
            {!grouped ? (
              <div class="flex min-w-0 items-baseline gap-2">
                <span
                  class="text-text min-w-0 truncate text-sm font-semibold"
                  title={m.author.name}
                >
                  {m.author.name}
                </span>
                {/* Badge «BOT» (Ola 15): TEXTO, no solo color (WCAG AA). Marca los
                    mensajes de webhooks/bots (integraciones). */}
                {m.isBot ? (
                  <span
                    class="border-border bg-bg-subtle text-text-muted text-2xs shrink-0 rounded-sm border px-1 font-semibold tracking-wide uppercase"
                    aria-label={t('message.botAria')}
                  >
                    {t('message.bot')}
                  </span>
                ) : null}
                <span class="sytue-meta shrink-0" title={fullDateLabel(locale, m.createdAt)}>
                  {timeLabel(locale, m.createdAt)}
                </span>
              </div>
            ) : null}
            {editingId === m.id ? (
              <div class="mt-0.5">
                <textarea
                  aria-label={t('message.edit')}
                  value={draft}
                  autofocus
                  rows={Math.min(6, draft.split('\n').length)}
                  onInput={(e) => setDraft((e.target as HTMLTextAreaElement).value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      saveEdit(m.body);
                    } else if (e.key === 'Escape') {
                      e.preventDefault();
                      cancelEdit();
                    }
                  }}
                  class="border-border bg-surface text-text sytue-focus-ring w-full resize-none rounded-md border px-2.5 py-1.5 text-base outline-none"
                />
                <div class="mt-1 flex items-center gap-2">
                  <Button size="sm" onClick={() => saveEdit(m.body)}>
                    {t('action.save')}
                  </Button>
                  <Button size="sm" variant="secondary" onClick={cancelEdit}>
                    {t('action.cancel')}
                  </Button>
                  <span class="sytue-meta">{t('message.editHint')}</span>
                </div>
              </div>
            ) : blockedSubs?.has(m.author.id) && !mostrados.has(m.id) ? (
              <BlockedFold t={t} onShow={() => setMostrados((prev) => new Set(prev).add(m.id))} />
            ) : (
              <>
                {m.authenticityVerified === false && !m.decryptionFailed ? (
                  <p class="text-text-muted mb-1 text-xs">{t('matrix.senderUnverified')}</p>
                ) : m.matrix_event && !m.decryptionFailed && m.identityVerified === false ? (
                  <p class="text-text-muted mb-1 text-xs">{t('matrix.identityUnverified')}</p>
                ) : null}
                {m.encryption === 'legacy' ? (
                  <p class="text-text-muted mb-1 text-xs">{t('matrix.legacy')}</p>
                ) : null}
                {m.decryptionFailed ? (
                  <p class="text-text-muted text-sm">{t('matrix.unreadable')}</p>
                ) : null}
                {m.priority ? <div class="mb-0.5">{priorityBadge(t, m.priority)}</div> : null}
                {m.body ? (
                  <p
                    class={cx(
                      'text-base break-words whitespace-pre-wrap',
                      m.pending ? 'text-text-muted' : 'text-text',
                    )}
                  >
                    <MessageBody
                      t={t}
                      body={m.body}
                      authorName={m.author.name}
                      mentionNames={(m.mentions ?? []).map((s) => memberNames?.[s] ?? s)}
                    />
                    {m.editedAt ? (
                      <span class="sytue-meta ml-1" title={t('message.editedTitle')}>
                        {t('message.edited')}
                      </span>
                    ) : null}
                    {m.pending ? <span class="sytue-meta ml-1">{t('message.sending')}</span> : null}
                  </p>
                ) : null}
                <Attachments
                  attachments={m.attachments}
                  channelId={m.channelId}
                  t={t}
                  locale={locale}
                />
                {/* Objeto rico (encuesta/tarea): la pregunta o el título son el cuerpo
                    de arriba; aquí van las opciones votables o la casilla de «hecha». */}
                {m.card ? (
                  <CardView
                    card={m.card}
                    messageId={m.id}
                    {...(onCardVote && !m.pending && !m.failed ? { onVote: onCardVote } : {})}
                    locale={locale}
                    t={t}
                  />
                ) : null}
                {/* Unfurl (Ola 16): tarjeta(s) de vista previa de hasta 2 URLs del cuerpo.
                    Solo en mensajes ya persistidos (no optimistas/fallidos). */}
                {m.encryption === 'legacy' && m.body && !m.pending && !m.failed
                  ? extractLinks(m.body, 2).map((url) =>
                      proveedorDeReunion(url) ? (
                        <MeetingLinkCard key={url} url={url} t={t} />
                      ) : (
                        <LinkPreview key={url} url={url} t={t} />
                      ),
                    )
                  : null}
                {!m.body && m.pending ? (
                  <span class="sytue-meta">{t('message.sending')}</span>
                ) : null}
              </>
            )}

            {/* Envio fallido: aviso con icono + reintento (nunca solo color). */}
            {m.failed ? (
              <p class="text-danger-text mt-0.5 flex items-center gap-1.5 text-xs">
                <Icon name="warning" size={12} />
                <span>{t('message.sendFailed')}</span>
                {onResend ? (
                  <button
                    type="button"
                    class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2 transition-colors duration-150"
                    onClick={() => onResend(m)}
                  >
                    {t('action.retry')}
                  </button>
                ) : null}
              </p>
            ) : null}

            {/* Fila de reacciones (chips) + indicador de hilo + «Siguiendo». */}
            {(m.reactions && m.reactions.length > 0) ||
            (m.replyCount ?? 0) > 0 ||
            followedIds?.has(m.id) ? (
              <div class="mt-1 flex flex-wrap items-center gap-1">
                {(m.reactions ?? []).map((r) => {
                  // «Quién reaccionó»: nombres de los reactores (tooltip nativo + etiqueta
                  // accesible). Sin `subs` del servicio cae al emoji + conteo de siempre.
                  const who = reactedByLabel(locale, t, r.emoji, r.subs, memberNames);
                  return (
                    <button
                      key={r.emoji}
                      type="button"
                      aria-pressed={r.mine ? 'true' : 'false'}
                      aria-label={t(r.mine ? 'reaction.removeMine' : 'reaction.add', {
                        what: who ?? `${r.emoji} ${r.count}`,
                      })}
                      {...(who ? { title: who } : {})}
                      onClick={() => onReact?.(m.id, r.emoji)}
                      class={cx(
                        'rounded-control sytue-focus-ring inline-flex items-center gap-1 border px-2 py-0.5 text-xs font-medium transition-colors duration-150',
                        r.mine
                          ? // Texto coloreado con el rol -text (AA sobre su tinte, §1.6);
                            // el hover del borde tambien remapea bien en oscuro.
                            'border-primary bg-selected text-primary-text hover:border-primary-text'
                          : 'border-border bg-surface text-text-muted hover:border-border-strong hover:bg-bg-subtle',
                      )}
                    >
                      <span aria-hidden="true">
                        <EmojiToken token={r.emoji} />
                      </span>
                      <span>{r.count}</span>
                    </button>
                  );
                })}
                {/* «+» al final de las reacciones: añadir otra con cualquier emoji. */}
                {onReact && (m.reactions?.length ?? 0) > 0 ? (
                  <EmojiPickerButton
                    onPick={(glyph) => onReact(m.id, glyph)}
                    t={t}
                    label={t('reaction.addOne')}
                    title={t('reaction.addOne')}
                    triggerClass="inline-flex h-6 items-center justify-center rounded-control border border-border bg-surface px-2 text-text-muted transition-colors duration-150 hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring"
                  >
                    <span class="text-sm leading-none font-semibold" aria-hidden="true">
                      +
                    </span>
                  </EmojiPickerButton>
                ) : null}
                {(m.replyCount ?? 0) > 0 ? (
                  <button
                    type="button"
                    onClick={() => onOpenThread?.(m)}
                    class="rounded-control text-primary-text hover:bg-selected sytue-focus-ring inline-flex items-center gap-1 px-1.5 py-0.5 text-xs font-medium transition-colors duration-150"
                  >
                    <Icon name="chevronRight" size={12} />
                    {t('thread.replyCountInThread', { count: m.replyCount ?? 0 })}
                  </button>
                ) : null}
                {/* «Siguiendo» (paridad Mattermost): TEXTO + icono, nunca solo color. */}
                {followedIds?.has(m.id) ? (
                  <span
                    class="sytue-meta inline-flex items-center gap-1 px-1.5 py-0.5 font-medium"
                    title={t('thread.followingTitle')}
                  >
                    <ThreadsGlyph size={12} />
                    {t('thread.following')}
                  </span>
                ) : null}
              </div>
            ) : null}

            {/* Acuse de recibo (Ola 12): botón «Confirmar» (o estado «Confirmado») +
                «Confirmado por N». Solo si el mensaje pide acuse (requires_ack). */}
            {m.requiresAck ? (
              <div class="mt-1.5 flex flex-wrap items-center gap-2">
                {m.acked ? (
                  <span class="rounded-pill bg-success-bg text-success-text inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium">
                    {t('ack.done')}
                  </span>
                ) : onAck ? (
                  <Button size="sm" variant="secondary" onClick={() => onAck(m)}>
                    {t('ack.confirm')}
                  </Button>
                ) : null}
                {(m.ackCount ?? 0) > 0 ? (
                  <span class="sytue-meta">{t('ack.byCount', { count: m.ackCount ?? 0 })}</span>
                ) : null}
              </div>
            ) : null}

            {/* «Visto» en DMs (Ola 16): bajo TU último mensaje cuando el/los peers ya lo
                han leído. Texto accesible (nunca solo un icono). */}
            {seen && seen.messageId === m.id ? (
              <div class="sytue-meta mt-0.5 text-right">{seen.label}</div>
            ) : null}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div class="relative flex min-h-0 flex-1 flex-col">
      {/* Pagina anterior en vuelo: pildora de estado sobre la lista (aria-live
          via role=status), sin desplazar el contenido. */}
      {loadingOlder ? (
        <div
          class="pointer-events-none absolute inset-x-0 top-2 z-10 flex justify-center"
          role="status"
        >
          <span class="rounded-pill border-border bg-surface text-text-muted border px-3 py-0.5 text-xs font-medium shadow-[var(--shadow-sm)]">
            {t('message.loadingOlder')}
          </span>
        </div>
      ) : null}
      {/* «Saltar a no leído»: lleva al separador «Nuevos mensajes» (scroll a la fila).
          Cede el hueco superior a la píldora de «cargando anteriores» si coinciden. */}
      {showJump && !loadingOlder ? (
        <div class="pointer-events-none absolute inset-x-0 top-2 z-10 flex justify-center">
          <Button
            size="sm"
            variant="primary"
            class="pointer-events-auto shadow-[var(--shadow-sm)]"
            leadingIcon={<Glifo d="M12 19V5M6 11l6-6 6 6" size={14} />}
            onClick={jumpToUnread}
          >
            {t('message.jumpToUnread')}
          </Button>
        </div>
      ) : null}
      <VirtualList
        items={messages}
        estimateSize={ESTIMATED_ROW_PX}
        stickToBottom
        getItemKey={getItemKey}
        handleRef={listHandle}
        class="min-h-0 flex-1 px-4"
        aria-label={t('message.listAria')}
        onScroll={(e) => {
          // Cerca del principio: pide la pagina anterior (ChatApp la deduplica,
          // guarda el cursor `before` y evita vuelos concurrentes).
          if (e.currentTarget.scrollTop < LOAD_OLDER_PX) onLoadOlder?.();
        }}
        renderItem={renderRow}
      />
      <ConfirmDialog
        open={borrando !== null}
        onClose={() => setBorrando(null)}
        onConfirm={() => {
          if (borrando) onDelete?.(borrando);
          setBorrando(null);
        }}
        title={t('action.delete')}
        confirmLabel={t('action.delete')}
        tone="danger"
      >
        <p class="text-text text-sm leading-relaxed">{t('message.confirmDelete')}</p>
      </ConfirmDialog>

      {/* Quién ha visto este mensaje. Con el nombre de cada persona y la hora a la
          que llegó hasta aquí — «visto por 3» sin decir quiénes obliga a
          preguntarlo igual, que es justo lo que esto viene a evitar. */}
      <Modal
        open={lectoresDe !== null}
        onClose={() => setLectoresDe(null)}
        title={t('message.readersTitle')}
        size="sm"
        footer={<Button onClick={() => setLectoresDe(null)}>{t('action.close')}</Button>}
      >
        {lectores === null ? (
          <div class="grid h-20 place-items-center" aria-busy="true">
            <Spinner size={18} />
          </div>
        ) : lectores.readers.length === 0 ? (
          <p class="text-text-muted text-sm">{t('message.seenByNobody')}</p>
        ) : (
          <>
            <p class="sytue-meta mb-2">
              {t('message.seenByCount', {
                count: lectores.readers.length,
                total: lectores.total,
              })}
            </p>
            <ul class="divide-border/60 border-border divide-y rounded-md border">
              {lectores.readers.map((l) => (
                <li key={l.subject} class="flex items-center justify-between gap-3 px-3 py-2">
                  <span class="text-text truncate text-sm">{l.name || l.subject}</span>
                  <span class="sytue-meta shrink-0">
                    {formatDateTime(locale, l.read_at, { dateStyle: 'medium', timeStyle: 'short' })}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </Modal>
    </div>
  );
}
