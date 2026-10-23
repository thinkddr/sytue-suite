/**
 * ChatApp — isla raiz de Sytue Chat. Monta la Sytue Shell y, debajo, el AppShell
 * canonico: sidebar de canales (publicos / DMs) + vista del canal seleccionado
 * (cabecera + historico) + redaccion. Carga inicial por REST (BFF) y entrega de
 * mensajes nuevos por WebSocket (ticket del BFF). Envio optimista con reconciliacion.
 * Estetica Sytue, modo claro/oscuro. docs/CONVENTIONS.md §1, §5.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'preact/hooks';
import { lazy, Suspense } from 'preact/compat';
import { nombreDeOrganizacion, SytueShell } from '@sytue/shell';
import { useConnectivity } from '@sytue/offline/preact';
import {
  Sidebar,
  SidebarSection,
  SidebarAction,
  Avatar,
  Badge,
  Button,
  Icon,
  Modal,
  Spinner,
  EmptyState,
  cx,
  useToast,
  useNavDrawer,
  useSondeo,
  ConfirmDialog,
  useConfirm,
  usePrompt,
  Illustration,
  ShortcutsModal,
  useUltimo,
  useAtajo,
  Glifo,
} from '@sytue/ui';
import type { VirtualListHandle } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import { MARCA_ORG, orgActual, RE_SLUG_ORG, rutaConOrg } from '@sytue/config';
import { formatDateTime, type Locale } from '@sytue/i18n';
import { roomsT } from '../i18n/index.js';
import {
  listChannels,
  listMessages,
  messagesAround,
  getReceipts,
  listMembers,
  getMe,
  sendMessage,
  ChatHttpError,
  listBlocks,
  blockPerson,
  unblockPerson,
  type BlockedPerson,
  type ReportTarget,
  reactMessage,
  editMessage,
  deleteMessage,
  markRead,
  markUnread,
  getThread,
  postReply,
  followThread,
  unfollowThread,
  listThreads,
  listPins,
  pinMessage,
  unpinMessage,
  saveMessage,
  unsaveMessage,
  listSaved,
  setChannelPrefs,
  updateChannel,
  openDirect,
  leaveChannel,
  deleteChannel,
  hideDirect,
  inviteMember,
  removeTeamMember,
  listChannelMembers,
  addChannelMember,
  removeChannelMember,
  listInvitations,
  answerInvitation,
  setChannelMemberRole,
  listCustomEmoji,
  postCallMessage,
  connectRealtime,
  messageLink,
  listCategories,
  createCategory,
  updateCategory,
  deleteCategory,
  setChannelCategory,
  ackMessage,
  voteCard,
  scheduleMessage,
  listScheduled,
  updateScheduled,
  cancelScheduled,
  listDrafts,
  getNotifyPrefs,
  type Channel,
  type ChatMessage,
  type ChatEvent,
  type ChannelMember,
  type Persona,
  type Attachment,
  type ThreadSummary,
  type Category,
  type ScheduledMessage,
  type NotifyPrefs,
  type Receipt,
  type CardVote,
  type Invitacion,
} from '../client/chatApi.js';
import {
  groupChannels,
  DEFAULT_FAVORITES,
  DEFAULT_CHANNELS,
  DEFAULT_DMS,
  type SidebarGroup,
} from '../lib/categories.js';
import {
  mentionsUser,
  mentionNotificationBody,
  shouldNotifyNewMessage,
  matchesKeyword,
  inDndWindow,
  localMinutesNow,
} from '../lib/mentions.js';
import { unreadDividerIndex } from '../lib/unread.js';
import { applyCardVotes, toggleMyCardVote } from '../lib/cards.js';
import type { RemoteCommand } from '../lib/commands.js';
import type { PreJoinChoice } from './PreJoin.js';
import { TextoDelAsistente } from './TextoDelAsistente.js';
import { IncomingCall } from './IncomingCall.js';
import { sonarTimbre } from '../lib/timbre.js';
import { avisoDeLlamada } from '../lib/llamadaEntrante.js';
import { upsertMember } from '../lib/roster.js';
import { forwardedBody } from '../lib/forward.js';
import { getAccessToken } from '@sytue/auth/client';
import {
  startCall,
  getActiveCall,
  listMeetings,
  createMeeting,
  joinMeeting,
  meetingDetail,
  closeMeeting,
  removeMeetingParticipant,
  type ChannelCall,
  type Meeting,
  type MeetingParticipant,
  type MeetingRecording,
  knockChannelCall,
  knockMeeting,
  LlamadaError,
} from '../client/meetApi.js';
import { MeetingsPanel } from './MeetingsPanel.js';
import { AbrirEnLaApp } from './AbrirEnLaApp.js';
import { abrirReunionEnLaApp, queHacer } from '../lib/enLaApp.js';
import {
  queryPresence,
  sendHeartbeat,
  toPresenceStatus,
  typingLabel,
  STATE_KEY,
  type PresenceState,
  type Presence,
} from '../client/presenceApi.js';
import { setCustomEmoji } from '../lib/customEmoji.js';
import {
  listLocalDrafts,
  subscribeDrafts,
  hydrateDrafts,
  clearDraft,
  draftId as draftScopeId,
  type LocalDraft,
} from '../lib/drafts.js';
import { loadCustomStatus, saveCustomStatus, storedExpiry } from '../lib/status.js';
import { useDmNotifications } from '../lib/useDmNotifications.js';
import { MessageView } from './MessageView.js';
import { Composer, type ComposeOptions } from './Composer.js';
import { ThreadPanel } from './ThreadPanel.js';
import { ThreadsPanel } from './ThreadsPanel.js';
import { MessageListPanel } from './MessageListPanel.js';
import { ScheduledPanel } from './ScheduledPanel.js';
import { DraftsPanel } from './DraftsPanel.js';
import { SearchPanel } from './SearchPanel.js';
import { MembersPanel } from './MembersPanel.js';
import { ChannelRow } from './ChannelRow.js';
import { CategorySection } from './CategorySection.js';
import { BrowseChannelsModal } from './BrowseChannelsModal.js';
import { CreateChannelModal } from './CreateChannelModal.js';
import { NewDmModal } from './NewDmModal.js';
import { NotifySettingsModal } from './NotifySettingsModal.js';
import { BlockedModal, ReportModal, ReportsModal } from './Moderation.js';
import { ExternalConversations } from './ExternalConversations.js';
import { captureMatrixScope, assertMatrixScope } from '../client/matrix/messages.js';
import { IntegrationsModal } from './IntegrationsModal.js';
import { ForwardMessageModal } from './ForwardMessageModal.js';
import { StatusMenu } from './StatusMenu.js';
import { QuickSwitcher } from './QuickSwitcher.js';
import { MessageMenu, type MenuEntry } from './MessageMenu.js';
import {
  PinGlyph,
  BookmarkGlyph,
  ClipGlyph,
  StarGlyph,
  ArchiveGlyph,
  EditGlyph,
  TrashGlyph,
  ThreadsGlyph,
  ClockGlyph,
} from './glyphs.js';

// La sala de vídeo arrastra livekit-client; se carga en un chunk aparte solo al
// abrir una llamada, para no lastrar el bundle de quien únicamente chatea.
const VideoRoom = lazy(() => import('./VideoRoom.js').then((m) => ({ default: m.VideoRoom })));
// La pantalla previa NO va en ese chunk: se enseña antes de conectar, así que
// cargarla junto a livekit-client sería esperar al SFU para elegir el micrófono.
const PreJoin = lazy(() => import('./PreJoin.js').then((m) => ({ default: m.PreJoin })));

/** Icono de vídeo (24-grid, envoltorio `Glifo`) para el botón «Llamar»: @sytue/ui aún no
 *  expone un glifo de cámara, así que va inline como el resto de glyphs de Chat. */
function VideoGlyph(): JSX.Element {
  return (
    <Glifo size={16}>
      <path d="M23 7 16 12l7 5V7Z" />
      <rect x="1" y="5" width="15" height="14" rx="2" />
    </Glifo>
  );
}

/** Icono de «personas» (24-grid, envoltorio `Glifo`) para el avatar de un DM de grupo. */
function GroupGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
      <circle cx="9" cy="7" r="4" />
      <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
      <path d="M16 3.13a4 4 0 0 1 0 7.75" />
    </Glifo>
  );
}

/** Avatar de un DM de grupo: un círculo neutro con el glifo de personas (no hay una
 *  presencia única que mostrar, a diferencia del 1:1). Iguala la huella del Avatar. */
function GroupAvatar({ px }: { px: number }): JSX.Element {
  return (
    <span
      class="bg-bg-subtle text-text-muted grid shrink-0 place-items-center rounded-full"
      style={{ width: px, height: px }}
      aria-hidden="true"
    >
      <GroupGlyph size={Math.round(px * 0.58)} />
    </span>
  );
}

/** ¿Es un DM de grupo (más de un peer)? Un 1:1 tiene un único peer. El backend puebla
 *  `peers` (los otros miembros) per-visor solo al listar los canales del usuario. */
function isGroupDm(c: Channel): boolean {
  return c.kind === 'direct' && (c.peers?.length ?? 0) > 1;
}

/** Devuelve el canal con su categoryId fijado, u OMITIDO si es null. Con
 *  exactOptionalPropertyTypes no se puede poner la clave opcional a undefined: hay que
 *  quitarla del objeto. */
function withCategory(c: Channel, categoryId: string | null): Channel {
  const { categoryId: _drop, ...rest } = c;
  return categoryId ? { ...rest, categoryId } : rest;
}

interface Props {
  user: SytueUser;
  hubBaseUrl: string;
  /**
   * Idioma ya negociado en el servidor (`Astro.locals.locale`), que llega por props
   * desde el `.astro`. NO se detecta aquí: el servidor ya pintó el `<html lang>` con
   * el idioma negociado y detectarlo en cliente provocaría desajuste de hidratación.
   */
  locale: Locale;
}

function newClientId(): string {
  return (
    globalThis.crypto?.randomUUID?.() ?? `c-${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
}

/** Página del histórico: limit por defecto del SDK/BFF (lib/chat.ts). */
const PAGE_SIZE = 50;
/**
 * Tope de mensajes vivos en memoria (~200): al llegar mensajes nuevos con el
 * usuario abajo se recorta por arriba; el historial vuelve con `loadOlder`.
 * ponytail: paginando hacia atrás la lista sí crece por encima del tope (no hay
 * cursor `after` para recuperar lo recortado por abajo); la virtualización
 * mantiene el DOM pequeño y el estado se resetea al cambiar de canal.
 */
const HISTORY_CAP = 200;

/** Entrada de nivel superior del sidebar (Hilos, Reuniones, Apps): las que abren un panel
 *  o un modal, no un canal. El anillo de foco es `.sytue-focus-ring-chrome`, el blanco
 *  que el cromo declara para lo que vive encima de él (WCAG 1.4.11). */
const SIDEBAR_ITEM =
  'flex w-full items-center gap-2 rounded-nav px-[var(--sytue-nav-pad)] py-2 text-left text-base ' +
  'transition-colors duration-150 sytue-focus-ring-chrome ' +
  'focus-visible:outline-none';

/** Estado visual de una entrada del sidebar según esté su panel abierto. */
const sidebarItemState = (on: boolean): string =>
  on
    ? 'bg-sidebar-active text-sidebar-active-fg [font-weight:var(--sytue-nav-active-weight)]'
    : 'font-medium text-sidebar-fg hover:bg-sidebar-hover';

/** Clave de localStorage del colapso de las secciones POR DEFECTO del sidebar
 *  (Favoritos/Canales/DMs). Las categorías personalizadas lo persisten en el backend. */
const COLLAPSED_KEY = 'sytue-rooms-collapsed-sections';

/** Lee el conjunto de secciones por defecto colapsadas (persistido en localStorage). */
function readCollapsedDefaults(): Set<string> {
  try {
    const raw = localStorage.getItem(COLLAPSED_KEY);
    return new Set(raw ? (JSON.parse(raw) as string[]) : []);
  } catch {
    return new Set();
  }
}

/** Añade/quita un sub de la lista de reactores de un chip, SOLO si el chip ya la
 *  trae: si el servicio no envió `subs`, no se fabrica una lista parcial (el chip
 *  cae a la etiqueta de solo conteo en vez de mentir sobre quién reaccionó). */
function withSub(r: { subs?: string[] }, sub: string, added: boolean): { subs?: string[] } {
  if (!r.subs) return {};
  return { subs: added ? [...r.subs, sub] : r.subs.filter((s) => s !== sub) };
}

/** Alterna la reaccion del usuario de la sesion sobre un mensaje (optimista). */
function toggleMyReaction(m: ChatMessage, emoji: string, mySub: string): ChatMessage {
  const reactions = [...(m.reactions ?? [])];
  const idx = reactions.findIndex((r) => r.emoji === emoji);
  if (idx === -1) {
    reactions.push({ emoji, count: 1, mine: true, subs: [mySub] });
  } else {
    const r = reactions[idx]!;
    if (r.mine) {
      const next = { ...r, count: r.count - 1, mine: false, ...withSub(r, mySub, false) };
      if (next.count <= 0) reactions.splice(idx, 1);
      else reactions[idx] = next;
    } else {
      reactions[idx] = { ...r, count: r.count + 1, mine: true, ...withSub(r, mySub, true) };
    }
  }
  return { ...m, reactions };
}

/** Aplica el delta de una reaccion AJENA (added/removed); no toca `mine`. */
function applyReactionDelta(
  m: ChatMessage,
  emoji: string,
  added: boolean,
  subject: string,
): ChatMessage {
  const reactions = [...(m.reactions ?? [])];
  const idx = reactions.findIndex((r) => r.emoji === emoji);
  if (added) {
    if (idx === -1) reactions.push({ emoji, count: 1, mine: false, subs: [subject] });
    else {
      const r = reactions[idx]!;
      reactions[idx] = { ...r, count: r.count + 1, ...withSub(r, subject, true) };
    }
  } else if (idx !== -1) {
    const r = reactions[idx]!;
    const next = { ...r, count: r.count - 1, ...withSub(r, subject, false) };
    if (next.count <= 0) reactions.splice(idx, 1);
    else reactions[idx] = next;
  }
  return { ...m, reactions };
}

/** Aplica una edicion a un mensaje: solo cuerpo + marca editedAt (conserva reacciones,
 *  replyCount, etc., que el frame/respuesta de edicion no incluye). */
function mergeEdit(m: ChatMessage, edited: ChatMessage): ChatMessage {
  return { ...m, body: edited.body, ...(edited.editedAt ? { editedAt: edited.editedAt } : {}) };
}

export function ChatApp({ user, hubBaseUrl, locale }: Props): JSX.Element {
  const offline = useConnectivity() === 'offline';
  // `t` atada al idioma que llega por props. Memoizada porque viaja por props a las
  // islas hijas y entra en listas de dependencias: recrearla en cada render las
  // invalidaría todas (el idioma solo cambia al recargar).
  const t = useMemo(() => roomsT(locale), [locale]);
  const tRef = useUltimo(t);
  const toast = useToast();
  // Comandos de servidor de la organización, para el autocompletado del compositor y
  // para reconocerlos al enviar. Se piden una vez: cambian cuando un admin los toca,
  // no mientras se escribe.
  const [remoteCommands, setRemoteCommands] = useState<RemoteCommand[]>([]);
  const [channels, setChannels] = useState<Channel[]>([]);
  // Ola 11: categorías del sidebar (per-usuario). El colapso de las secciones POR
  // DEFECTO vive en localStorage; el de las personalizadas, en Category.collapsed.
  const [categories, setCategories] = useState<Category[]>([]);
  const [collapsedDefaults, setCollapsedDefaults] = useState<Set<string>>(readCollapsedDefaults);
  // DnD del sidebar: qué se arrastra ahora (un canal o una categoría), para decidir el
  // soltar. null = nada en curso.
  const [drag, setDrag] = useState<{ kind: 'channel' | 'category'; id: string } | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  // Id del primer mensaje no leído al ABRIR el canal (separador «Nuevos mensajes»).
  // Se calcula del contador de no leídos capturado antes de markRead; null = ninguno.
  const [unreadDividerId, setUnreadDividerId] = useState<string | null>(null);
  // Permalink / scroll-to-message (Ola 16): mensaje a resaltar tras cargar la ventana,
  // y bandera de «viendo historial» (se saltó a un mensaje antiguo: cargamos una ventana
  // centrada, no el final). jumpTargetRef porta el objetivo del próximo (re)cargado.
  const [highlightId, setHighlightId] = useState<string | null>(null);
  const [viewingHistory, setViewingHistory] = useState(false);
  const viewingHistoryRef = useRef(false);
  viewingHistoryRef.current = viewingHistory;
  const jumpTargetRef = useRef<{ channelId: string; messageId: string } | null>(null);
  const highlightTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // «Visto» en DMs (Ola 16): last_read_at de los otros miembros del canal activo.
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [loadingChannels, setLoadingChannels] = useState(true);
  const [loadingMessages, setLoadingMessages] = useState(false);
  // Errores de carga con reintento: el "tick" fuerza a re-ejecutar el efecto.
  const [channelsError, setChannelsError] = useState(false);
  const [channelsTick, setChannelsTick] = useState(0);
  const [messagesError, setMessagesError] = useState(false);
  const [messagesTick, setMessagesTick] = useState(0);
  useEffect(() => {
    const verified = (): void => setMessagesTick((value) => value + 1);
    window.addEventListener('sytue-matrix-verified', verified);
    return () => window.removeEventListener('sytue-matrix-verified', verified);
  }, []);

  const [status, setStatus] = useState<'connecting' | 'open' | 'closed'>('connecting');

  // --- Ola 4: presencia y estado ---
  // Presencia propia. El estado (online/away/dnd) es efímero (TTL del Hub) y arranca
  // "En línea"; el estado personalizado SÍ se restaura de localStorage al montar, ya
  // caducado si su expiración («Borrar después de») venció mientras Rooms estaba cerrado.
  const [myState, setMyState] = useState<PresenceState>('online');
  const [myStatus, setMyStatus] = useState<string>(() => loadCustomStatus());
  // Presencia de OTROS (sub -> Presence), poblada en batch desde el Hub.
  const [presences, setPresences] = useState<Record<string, Presence>>({});
  // "Escribiendo…" por canal: channelId -> (sub -> nombre); cada entrada expira a los ~5 s.
  const [typing, setTyping] = useState<Record<string, Record<string, string>>>({});

  // Paginación hacia atrás: ¿quedan mensajes más antiguos? + página en vuelo.
  const [hasOlder, setHasOlder] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const loadingOlderRef = useRef(false);
  // Handle de la lista virtualizada (isAtBottom para el cap del histórico).
  const listRef = useRef<VirtualListHandle>(null);

  /** Recorta a HISTORY_CAP por arriba, solo con el usuario abajo: recortar
   *  mientras lee historial antiguo le movería el contenido bajo el viewport. */
  function capAtBottom(next: ChatMessage[]): ChatMessage[] {
    if (next.length <= HISTORY_CAP) return next;
    if (!(listRef.current?.isAtBottom() ?? true)) return next;
    // Lo recortado sigue persistido en el servidor: reabre la paginación aunque
    // el historial estuviera «agotado» (p. ej. canal corto que crece en vivo).
    setHasOlder(true);
    return next.slice(-HISTORY_CAP);
  }

  // Miembros (para el selector de menciones) y su mapa sub -> nombre (resaltado).
  const [members, setMembers] = useState<ChannelMember[]>([]);
  const memberNames = useMemo(
    () => Object.fromEntries(members.map((m) => [m.sub, m.name])),
    [members],
  );
  // Nombre -> sub, para resolver un usuario ESCRITO A MANO (p. ej. «/msg @nombre»). La
  // presencia/receipts de un DM NO usan esto: van por `peerSubs` del canal (sub exacto).
  const membersByName = useMemo(() => new Map(members.map((m) => [m.name, m.sub])), [members]);
  // ¿Es el usuario admin del team? Gobierna las acciones de admin del cliente
  // (archivar, editar tema). El backend valida igualmente; esto es solo la UI.
  // ponytail: rol de TEAM; un admin solo-de-canal (no del team) no verá el botón
  // pero el backend seguiría autorizándolo — cubrir con rol por canal si hace falta.
  const isTeamAdmin = useMemo(
    () => members.some((m) => m.sub === user.id && m.role === 'admin'),
    [members, user.id],
  );
  // channelId -> nombre, para etiquetar el origen de cada guardado (cross-canal).
  const channelNames = useMemo(
    () => Object.fromEntries(channels.map((c) => [c.id, c.name])),
    [channels],
  );

  // Panel de hilo: mensaje raíz abierto (null = cerrado) + respuestas y estado.
  const [threadRoot, setThreadRoot] = useState<ChatMessage | null>(null);
  const [threadReplies, setThreadReplies] = useState<ChatMessage[]>([]);
  const [threadLoading, setThreadLoading] = useState(false);
  const [threadError, setThreadError] = useState(false);
  // ¿Sigo el hilo abierto? Alimenta el botón Seguir/Siguiendo del ThreadPanel (Ola 9).
  const [threadFollowing, setThreadFollowing] = useState(false);
  // Seguimientos ya CONOCIDOS por respuesta del servidor: lo que devolvió getThread al
  // abrir un hilo y lo que el usuario ha alternado en esta sesión. Se aplica sobre la
  // página cargada de «Hilos» para marcar «Siguiendo» en el feed (Ola 3B).
  const [followOverrides, setFollowOverrides] = useState<Record<string, boolean>>({});

  // Panel RHS auxiliar (excluyente con el hilo): 'pinned' (fijados del canal),
  // 'saved' (mis guardados, cross-canal), 'search' (búsqueda global cross-team),
  // 'threads' (mis hilos seguidos, cross-canal — Ola 9), 'scheduled' (mis mensajes
  // programados, cross-canal — Ola 12) o 'members' (roster del canal — Ola 2C).
  const [rhsView, setRhsView] = useState<
    | 'pinned'
    | 'saved'
    | 'search'
    | 'threads'
    | 'scheduled'
    | 'members'
    | 'meetings'
    | 'drafts'
    | null
  >(null);

  // Borradores del usuario (lo escrito y no enviado, cross-canal). Se leen del
  // almacenamiento local —que lib/drafts.ts sincroniza con el servidor— y se
  // recomponen con cada guardado: así el contador del sidebar reacciona a la misma
  // pulsación que escribe el texto, sin una petición por letra.
  const [drafts, setDrafts] = useState<LocalDraft[]>([]);
  // Sube al hidratar con lo que había en el servidor: forma parte de la `key` del
  // composer para que remonte y recoja el borrador que acaba de llegar de otro
  // dispositivo (si no, seguiría mostrando el vacío con el que se montó).
  const [draftsEpoch, setDraftsEpoch] = useState(0);
  // Semilla del panel de búsqueda cuando se abre por el slash command «/search».
  const [searchSeed, setSearchSeed] = useState('');

  // Mensajes programados del usuario (vista global «Programados», cross-canal). Se
  // carga al abrir el panel; es una foto (un programado que venza mientras está
  // abierto llega al canal por WS y se retira al recargar).
  const [scheduledList, setScheduledList] = useState<ScheduledMessage[]>([]);
  const [scheduledLoading, setScheduledLoading] = useState(false);
  const [scheduledError, setScheduledError] = useState(false);

  // Hilos que sigo (vista global «Hilos», cross-canal, keyset). Se hidrata al montar
  // (para el contador del sidebar) y al abrir el panel; el WS lo mantiene en vivo.
  const [threadsList, setThreadsList] = useState<ThreadSummary[]>([]);
  const [threadsNext, setThreadsNext] = useState<string | undefined>(undefined);
  const [threadsLoading, setThreadsLoading] = useState(false);
  const [threadsLoadingMore, setThreadsLoadingMore] = useState(false);
  const [threadsError, setThreadsError] = useState(false);
  // Nº de hilos seguidos con respuestas no leídas (badge del sidebar). Acotado a la
  // página cargada (los hilos con actividad reciente encabezan la lista).
  // ponytail: sin endpoint de conteo dedicado; a escala el badge cubre tus hilos más
  // activos (los que tienen no-leídos), que son justo los del principio de la lista.
  const threadsUnreadCount = useMemo(
    () => threadsList.filter((th) => th.unreadReplies > 0).length,
    [threadsList],
  );
  // Mensajes cuyo hilo SÉ que sigo: los de la página cargada de «Hilos» + los
  // seguimientos confirmados por el servidor en esta sesión (followOverrides).
  // ponytail: conocimiento POSITIVO y acotado — el mensaje del feed no trae
  // `following`, así que un hilo seguido MÁS ANTIGUO que la página cargada no muestra
  // «Siguiendo» hasta que se abre (getThread lo confirma). Se ofrece la acción igual
  // (seguir es idempotente en el servidor) en vez de fingir el estado. Upgrade: flag
  // `following` en domain.Message o endpoint de consulta en bloque.
  const followedIds = useMemo(() => {
    const ids = new Set(threadsList.map((th) => th.root.id));
    for (const [id, following] of Object.entries(followOverrides)) {
      if (following) ids.add(id);
      else ids.delete(id);
    }
    return ids;
  }, [threadsList, followOverrides]);

  // Fijados del canal activo (cargados al abrir el canal; el evento WS pin los
  // mantiene en vivo). pinnedIds alimenta el toggle Fijar/Desfijar del menú.
  const [pins, setPins] = useState<ChatMessage[]>([]);
  const [pinsLoading, setPinsLoading] = useState(false);
  const [pinsError, setPinsError] = useState(false);
  const [pinsTick, setPinsTick] = useState(0);
  const pinnedIds = useMemo(() => new Set(pins.map((p) => p.id)), [pins]);

  // Roster del canal activo (Ola 2C): alimenta el facepile de la cabecera y el panel
  // «Miembros», y lo mantienen en vivo los frames channel_member_added/removed. Un DM
  // no tiene roster gestionable (el backend responde 403) así que no se pide.
  // rosterTick fuerza la recarga tras un error del panel.
  const [roster, setRoster] = useState<ChannelMember[]>([]);
  const [rosterLoading, setRosterLoading] = useState(false);
  const [rosterError, setRosterError] = useState(false);
  const [rosterTick, setRosterTick] = useState(0);

  // Guardados del usuario (cross-canal, keyset). savedIds alimenta el toggle
  // Guardar/Quitar del menú (se hidrata al abrir el panel + con cada toggle).
  const [savedList, setSavedList] = useState<ChatMessage[]>([]);
  const [savedIds, setSavedIds] = useState<Set<string>>(new Set());
  const [savedNext, setSavedNext] = useState<string | undefined>(undefined);
  const [savedLoading, setSavedLoading] = useState(false);
  const [savedLoadingMore, setSavedLoadingMore] = useState(false);
  const [savedError, setSavedError] = useState(false);

  // Llamada del canal (Sytue Rooms): overlay de vídeo sobre el panel. `call` es la
  // conexión LiveKit activa (null = sin llamada); callBusy evita el doble clic y
  // callError avisa si el arranque falla.
  const [call, setCall] = useState<ChannelCall | null>(null);
  // Lo que se eligió en la pantalla previa. Mientras es `null` se muestra ESA
  // pantalla y no se ha conectado nada: la sala no se monta —y por tanto no se
  // abre ni el micrófono ni la cámara— hasta que el usuario dice con qué entra.
  const [callChoice, setCallChoice] = useState<PreJoinChoice | null>(null);
  // Canal en el que se abrió la llamada. El chat de la reunión es el de ESE canal,
  // y desde dentro se puede seguir cambiando de canal por la barra lateral: sin
  // esto, el panel enseñaría la conversación de otro sitio.
  const [callChannelId, setCallChannelId] = useState<string | null>(null);
  // --- Reuniones (salas sin canal). Foto al abrir el panel; se recarga a mano. ---
  const [meetings, setMeetings] = useState<Meeting[]>([]);
  const [meetingsLoading, setMeetingsLoading] = useState(false);
  const [meetingsError, setMeetingsError] = useState(false);
  const [meetingDetailFor, setMeetingDetailFor] = useState<string | null>(null);
  const [meetingParticipants, setMeetingParticipants] = useState<MeetingParticipant[]>([]);
  const [meetingRecordings, setMeetingRecordings] = useState<MeetingRecording[]>([]);
  const [meetingDetailLoading, setMeetingDetailLoading] = useState(false);
  const [callBusy, setCallBusy] = useState(false);
  const [callError, setCallError] = useState(false);
  // La sala está cerrada: no es un fallo, es una respuesta. Guarda el canal para
  // poder llamar a la puerta, y si ya se llamó, que no se ofrezca dos veces.
  const [puertaCerrada, setPuertaCerrada] = useState<{
    channelId: string;
    llamado: boolean;
  } | null>(null);
  // Participantes presentes en la llamada EN CURSO del canal activo (0 = sin llamada).
  // Alimenta el banner «Llamada en curso» de la cabecera; se sondea ligero (~25 s).
  const [activeCallCount, setActiveCallCount] = useState(0);
  // Canales con llamada viva, MIREN O NO al canal abierto. El sondeo de arriba
  // solo cubre el canal activo, así que sin esto la barra lateral no puede decir
  // dónde hay gente hablando. Se alimenta del WebSocket, que ya lo anunciaba.
  const [llamadasVivas, setLlamadasVivas] = useState<Set<string>>(new Set());
  // Llamada entrante en un canal que NO estás mirando. null = nadie llamando.
  const [entrante, setEntrante] = useState<{
    channelId: string;
    label: string;
    direct: boolean;
  } | null>(null);

  // Ola 3: modales de «Explorar canales» y «Nuevo mensaje» (DM); edición de tema
  // en la cabecera del canal (solo admin).
  const [browseOpen, setBrowseOpen] = useState(false);
  const [newDmOpen, setNewDmOpen] = useState(false);
  // Ola 2B: modal «Crear canal» (el «+» de la cabecera de «Canales» y el CTA del
  // estado vacío, la salida del callejón de un tenant recién creado).
  const [createOpen, setCreateOpen] = useState(false);
  // Ola 8: mensaje que se está reenviando (abre el selector de canal destino).
  const [forwarding, setForwarding] = useState<ChatMessage | null>(null);
  const [editingTopic, setEditingTopic] = useState(false);
  const [topicDraft, setTopicDraft] = useState('');
  // «Ponme al día»: el resumen de lo hablado en el canal, que lo escribe Sytue
  // AI a partir de los últimos mensajes. Vive aquí y no en un panel porque se
  // lee una vez y se cierra: guardarlo sería inventarle una vida que no tiene.
  const [resumen, setResumen] = useState<string | null>(null);
  const [resumiendo, setResumiendo] = useState(false);

  // Ref al canal activo para que el handler de WebSocket lea el valor vigente.
  const activeRef = useRef<string | null>(null);
  activeRef.current = activeId;
  // Ref al hilo abierto: el handler de WebSocket enruta las respuestas en vivo.
  const threadRootRef = useRef<ChatMessage | null>(null);
  threadRootRef.current = threadRoot;
  // Ref a los canales: el handler de WebSocket (creado una sola vez) resuelve el
  // nombre del canal de una mención sin recrearse cada vez que cambia la lista.
  const channelsRef = useRef<Channel[]>([]);
  channelsRef.current = channels;
  // Ref a la llamada en curso: el handler del WebSocket la consulta para no
  // anunciar una entrante encima de la que ya estás.
  const callRef = useRef<ChannelCall | null>(null);
  callRef.current = call;

  // Refs de presencia propia: el bucle de heartbeat (montado una vez) lee siempre
  // el valor vigente sin re-suscribirse. Se sincronizan en cada render y también, de
  // forma síncrona, al cambiar el estado (para que el heartbeat inmediato lo lleve).
  const myStateRef = useRef(myState);
  myStateRef.current = myState;
  const myStatusRef = useRef(myStatus);
  myStatusRef.current = myStatus;
  // Dispara un heartbeat inmediato (al cambiar estado o al ocultarse la pestaña).
  const heartbeatNowRef = useRef<(() => void) | null>(null);
  // Temporizadores de expiración de "escribiendo…" por (canal:sub).
  const typingTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  // The Hub account rule is shared with Fyne; browser permission stays separate.
  const dmAlerts = useDmNotifications({ id: user.id, tenant: user.tenant ?? '' });
  const dmNotifyRef = useUltimo(dmAlerts.browserEnabled);

  // Preferencias de notificación (Ola 13): palabras clave + ventana «No molestar». Se
  // cargan una vez y viven en una ref para que el handler de WebSocket (montado una vez)
  // consulte siempre el valor vigente sin re-suscribirse. El modal las edita/guarda.
  const [notifyPrefs, setNotifyPrefs] = useState<NotifyPrefs>({
    keywords: [],
    dndStart: 0,
    dndEnd: 0,
  });
  const notifyPrefsRef = useRef(notifyPrefs);
  notifyPrefsRef.current = notifyPrefs;
  const [notifySettingsOpen, setNotifySettingsOpen] = useState(false);
  // Integraciones (Ola 15): webhooks entrantes/salientes + bots (modal de gestión).
  const [integrationsOpen, setIntegrationsOpen] = useState(
    () =>
      typeof window !== 'undefined' &&
      new URLSearchParams(window.location.search).get('connect-app') === '1',
  );

  const [externalOpen, setExternalOpen] = useState(
    () =>
      typeof window !== 'undefined' &&
      new URLSearchParams(window.location.search).get('connect-external') === '1',
  );

  // Denunciar y bloquear (ADR 0073). El bloqueo lo APLICA el servidor (no entrega
  // directos ni avisos); aquí la lista sirve para plegar los mensajes de esas
  // personas en los canales compartidos y para callar el aviso de escritorio.
  const [blocks, setBlocks] = useState<BlockedPerson[]>([]);
  const blockedSubs = useMemo(() => new Set(blocks.map((b) => b.sub)), [blocks]);
  const blockedRef = useRef(blockedSubs);
  useEffect(() => {
    blockedRef.current = blockedSubs;
  }, [blockedSubs]);
  const [blockedOpen, setBlockedOpen] = useState(false);
  // La cola de denuncias se abre también desde el aviso del Hub (`?moderation=1`).
  const [reportsOpen, setReportsOpen] = useState(false);
  useEffect(() => {
    if (new URLSearchParams(window.location.search).get('moderation') === '1') {
      setReportsOpen(true);
    }
  }, []);
  const [reporting, setReporting] = useState<{ target: ReportTarget; name: string } | null>(null);
  useEffect(() => {
    const ac = new AbortController();
    void listBlocks(ac.signal)
      .then((d) => setBlocks(d.blocks))
      .catch(() => undefined); // degrada: nada plegado; el servidor sigue aplicándolo
    return () => ac.abort();
  }, []);

  useEffect(() => {
    let live = true;
    void getNotifyPrefs()
      .then(({ prefs }) => live && setNotifyPrefs(prefs))
      .catch(() => undefined); // degrada: sin palabras clave ni DND
    return () => {
      live = false;
    };
  }, []);

  // Borradores: se traen los del SERVIDOR una vez al arrancar (lo empezado en otro
  // dispositivo) y a partir de ahí la vista vive de la copia local, que se recompone
  // con cada guardado del composer.
  useEffect(() => {
    const refresh = (): void => setDrafts(listLocalDrafts(user.id));
    refresh();
    const unsubscribe = subscribeDrafts(refresh);
    let live = true;
    void listDrafts()
      .then(({ drafts: remote }) => {
        if (!live) return;
        const escritos = hydrateDrafts(
          user.id,
          remote.map((d) => ({
            channelId: d.channelId,
            body: d.body,
            ...(d.parentId ? { parentId: d.parentId } : {}),
          })),
        );
        if (escritos > 0) setDraftsEpoch((n) => n + 1);
      })
      .catch(() => undefined); // sin red: quedan los locales
    return () => {
      live = false;
      unsubscribe();
    };
  }, [user.id]);

  // ¿Administro el WORKSPACE? Ojo, NO es `isTeamAdmin`, que mira el roster del primer
  // team y sirve para moderar ese team (reuniones, canales). Esto es «admin de ALGÚN
  // team del tenant», que es lo que exige el backend para las integraciones: con varios
  // teams, `isTeamAdmin` daría un falso negativo y escondería la opción a quien sí puede.
  // Se resuelve una vez al montar y por defecto NO: si la respuesta tarda o falla, es
  // preferible que la opción aparezca al llegar el dato a que se ofrezca y luego la
  // rechace el servidor.
  const [canManageIntegrations, setCanManageIntegrations] = useState(false);
  useEffect(() => {
    let live = true;
    void getMe()
      .then(({ workspaceAdmin }) => live && setCanManageIntegrations(workspaceAdmin))
      .catch(() => undefined); // degrada: sin el dato, no se ofrece administración
    return () => {
      live = false;
    };
  }, []);

  // Permiso de notificaciones del navegador (best-effort, una vez): permite
  // avisar de una mención o un DM aunque la pestaña esté en segundo plano. Sin gesto
  // algunos navegadores lo posponen; si se deniega, la mención solo sube el badge.
  useEffect(() => {
    if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
      void Notification.requestPermission().catch(() => undefined);
    }
  }, []);

  /** ¿Estoy dentro de mi ventana «No molestar» ahora (hora local)? Suprime avisos. */
  function inQuietHours(): boolean {
    const { dndStart, dndEnd } = notifyPrefsRef.current;
    return inDndWindow(localMinutesNow(), dndStart, dndEnd);
  }

  /** Lanza una notificación del navegador por una mención o palabra clave (fire-and-forget). */
  function notifyMention(msg: ChatMessage): void {
    if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
    if (blockedRef.current.has(msg.author.id)) return; // persona bloqueada: sin aviso
    const channel = channelsRef.current.find((c) => c.id === msg.channelId);
    // Canal en 'none' (⟺ muted): no notifica; el no-leído/mención se sigue mostrando,
    // atenuado. En 'mentions' o 'all' sí avisa (una mención/palabra clave es prioritaria).
    if (channel?.muted) return;
    if (inQuietHours()) return; // ventana No molestar
    // Plantilla COMPLETA por caso (con y sin canal): no se pega « en #canal » al
    // final, porque el orden cambia entre lenguas.
    const title = channel
      ? t('notification.mentionedIn', { name: msg.author.name, channel: channel.name })
      : t('notification.mentioned', { name: msg.author.name });
    try {
      new Notification(title, {
        body: mentionNotificationBody(msg.body),
        tag: `sytue-chat-mention-${msg.channelId}`,
      });
    } catch {
      // Algunos navegadores exigen Service Worker o gesto de usuario: se ignora.
    }
  }

  /** Notifica un mensaje nuevo si la ventana no tiene foco: un DM, o un canal en el que
   *  el usuario pidió «Todas las notificaciones». El resto de condiciones —silenciado,
   *  propio, respuesta en hilo, ajuste global— las decide shouldNotifyNewMessage. */
  function notifyDirectMessage(msg: ChatMessage): void {
    if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
    if (blockedRef.current.has(msg.author.id)) return; // persona bloqueada: sin aviso
    const channel = channelsRef.current.find((c) => c.id === msg.channelId);
    const focused = typeof document !== 'undefined' ? document.hasFocus() : true;
    if (
      !shouldNotifyNewMessage(msg, channel, {
        windowFocused: focused,
        enabled: channel?.kind === 'direct' ? dmNotifyRef.current : true,
      })
    )
      return;
    if (inQuietHours()) return; // ventana No molestar
    try {
      // En un DM basta el nombre de quien escribe; en un canal hace falta decir cuál,
      // o el aviso no se distingue de otro canal cualquiera.
      const titulo =
        channel?.kind === 'direct'
          ? msg.author.name
          : `${msg.author.name} · ${channel?.name ?? ''}`;
      new Notification(titulo, {
        body: mentionNotificationBody(msg.body),
        tag: `sytue-chat-dm-${msg.channelId}`,
      });
    } catch {
      // Algunos navegadores exigen Service Worker o gesto de usuario: se ignora.
    }
  }

  // El WebSocket se conecta una sola vez; las notificaciones deben leer las
  // preferencias, el idioma y los canales de la renderización más reciente.
  const notificationHandlersRef = useRef({ notifyMention, notifyDirectMessage });
  notificationHandlersRef.current = { notifyMention, notifyDirectMessage };

  // Miembros del team (una vez): alimentan el selector de menciones. El nombre del
  // propio usuario lo rellena el BFF desde la sesión (el backend puede no tenerlo).
  useEffect(() => {
    let live = true;
    void listMembers()
      .then(({ members: list }) => live && setMembers(list))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  // Emoji personalizados del tenant (Ola 14), una vez: rellenan el catálogo de módulo
  // que consultan MessageBody (render de `:nombre:`), las reacciones y el EmojiPicker
  // —sin enhebrar la tabla por props—. Degrada en silencio si el endpoint falla.
  useEffect(() => {
    void listCustomEmoji()
      .then(({ emoji }) => setCustomEmoji(emoji))
      .catch(() => undefined);
  }, []);

  // Comandos de servidor de la organización, una vez: alimentan el autocompletado del
  // compositor y hacen que «/loquesea» se reconozca en vez de avisar «no existe». Degrada
  // en silencio: sin ellos, los comandos integrados siguen funcionando.
  useEffect(() => {
    let live = true;
    void fetch('/api/commands', { credentials: 'include', headers: { accept: 'application/json' } })
      .then((r) => (r.ok ? r.json() : { commands: [] }))
      .then((d: { commands?: { name: string; description?: string; usageHint?: string }[] }) => {
        if (live) setRemoteCommands(d.commands ?? []);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  // Hidrata la vista global «Hilos» al montar, solo para el contador de no-leídos del
  // sidebar (el panel la recarga al abrirse). Degrada en silencio si el endpoint falla.
  useEffect(() => {
    let live = true;
    void listThreads()
      .then(({ threads, nextBefore }) => {
        if (!live) return;
        setThreadsList(threads);
        setThreadsNext(nextBefore);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  // Categorías del sidebar (Ola 11), una vez. Degrada en silencio: sin categorías, el
  // sidebar cae a las secciones por defecto (Favoritos/Canales/DMs).
  useEffect(() => {
    let live = true;
    void listCategories()
      .then(({ categories: list }) => live && setCategories(list))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  // Invitaciones de otras organizaciones: la gente de fuera acepta antes de entrar
  // (decidido por el dueño el 2026-10-16). Se piden con los canales.
  const [invitaciones, setInvitaciones] = useState<Invitacion[]>([]);
  // El aviso de la campana (`?invitations=1`, COM-15) abre la lista con las invitaciones a
  // la vista, sin entrar en ningún canal: en el teléfono, entrar la tapaba.
  const [aInvitaciones] = useState(
    () => new URLSearchParams(window.location.search).get('invitations') === '1',
  );
  const invitacionesRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (aInvitaciones && invitaciones.length > 0) {
      invitacionesRef.current?.scrollIntoView({ block: 'nearest' });
    }
  }, [aInvitaciones, invitaciones.length]);
  useEffect(() => {
    let live = true;
    void listInvitations()
      .then(({ invitations }) => live && setInvitaciones(invitations))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [channelsTick]);

  /** Acepta (y entra en la conversación) o rechaza una invitación. */
  async function responderInvitacion(inv: Invitacion, aceptar: boolean): Promise<void> {
    try {
      await answerInvitation(inv.channel_id, aceptar);
      setInvitaciones((prev) => prev.filter((i) => i.channel_id !== inv.channel_id));
      if (aceptar) {
        setActiveId(inv.channel_id);
        setChannelsTick((n) => n + 1);
      }
    } catch {
      toast.notify({ tone: 'danger', title: t('invitations.failed') });
    }
  }

  // Carga de canales (channelsTick reintenta tras un error).
  useEffect(() => {
    let live = true;
    setLoadingChannels(true);
    setChannelsError(false);
    void listChannels()
      .then(({ channels: list }) => {
        if (!live) return;
        setChannels(list);
        if (!aInvitaciones) setActiveId((prev) => prev ?? list[0]?.id ?? null);
      })
      .catch(() => live && setChannelsError(true))
      .finally(() => live && setLoadingChannels(false));
    return () => {
      live = false;
    };
  }, [channelsTick, aInvitaciones]);

  // Carga de mensajes al cambiar de canal (messagesTick reintenta tras un error).
  // Si hay un objetivo de salto pendiente para este canal (permalink / búsqueda / deep-
  // link del Hub), carga una VENTANA centrada en él (messagesAround) en vez del final,
  // lo resalta y entra en modo «viendo historial»; si no, carga el final y marca leído.
  useEffect(() => {
    if (!activeId) return;
    let live = true;
    // Consume el objetivo de salto pendiente (si es para este canal).
    const jump = jumpTargetRef.current;
    const jumping = jump !== null && jump.channelId === activeId;
    if (jumping) jumpTargetRef.current = null;

    setLoadingMessages(true);
    setMessagesError(false);
    setMessages([]);
    setHasOlder(false);
    setUnreadDividerId(null);
    setHighlightId(null);
    setViewingHistory(jumping);
    // Captura el nº de no leídos ANTES de ponerlo a cero (markRead lo borra en el
    // servidor y el setChannels de abajo, localmente): con él se ancla el separador.
    // Vía channelsRef (valor vigente) para no meter `channels` en las deps del
    // efecto: si no, cada cambio de contador recargaría el histórico.
    const openUnread = channelsRef.current.find((c) => c.id === activeId)?.unread ?? 0;
    // El tipo va ANOTADO: solo una de las dos ramas devuelve `threadRootId`, y sin
    // esto TS infiere del ternario el tipo de la otra y la desestructuración de
    // abajo no compila.
    const loader: Promise<{ messages: ChatMessage[]; threadRootId?: string }> = jumping
      ? messagesAround(activeId, jump.messageId)
      : listMessages(activeId);
    loader
      .then(({ messages: list, threadRootId }) => {
        if (!live) return;
        setMessages(list);
        if (jumping) {
          // Ventana centrada: hay historial a ambos lados (paginar hacia atrás sigue
          // funcionando por keyset). Resalta el objetivo (MessageView hace el scroll).
          setHasOlder(list.length > 0);
          if (threadRootId) {
            // El objetivo era una RESPUESTA: el feed viene anclado en la raíz, así que
            // se abre su hilo y se resalta dentro. Sin esto el salto dejaba al usuario
            // mirando la raíz sin ver el mensaje que había pedido.
            const root = list.find((m) => m.id === threadRootId);
            if (root) setThreadRoot(root);
            highlightMessage(threadRootId);
          } else {
            highlightMessage(jump.messageId);
          }
        } else {
          // Página completa: asume que hay historial anterior (heurística keyset).
          setHasOlder(list.length >= PAGE_SIZE);
          // Separador «Nuevos mensajes» ante el primer no leído (solo esta apertura).
          const idx = unreadDividerIndex(openUnread, list.length);
          setUnreadDividerId(idx === null ? null : (list[idx]?.id ?? null));
        }
      })
      .catch(() => {
        if (!live) return;
        setMessages([]);
        setMessagesError(true);
      })
      .finally(() => live && setLoadingMessages(false));
    // Saltar a un mensaje antiguo NO marca el canal como leído (no borrar los no leídos
    // posteriores a la ventana). Abrir el final del canal sí: limpia contadores y persiste.
    if (!jumping) {
      setChannels((prev) =>
        prev.map((c) => (c.id === activeId ? { ...c, unread: 0, mentions: 0 } : c)),
      );
      void markRead(activeId).catch(() => undefined);
    }
    return () => {
      live = false;
    };
  }, [activeId, messagesTick]);

  // Fijados del canal activo (para el contador de cabecera, el panel y el toggle
  // del menú). pinsTick fuerza la recarga tras un error del panel.
  useEffect(() => {
    if (!activeId) {
      setPins([]);
      return;
    }
    let live = true;
    setPins([]);
    setPinsLoading(true);
    setPinsError(false);
    listPins(activeId)
      .then(({ pins: list }) => live && setPins(list))
      .catch(() => live && setPinsError(true))
      .finally(() => live && setPinsLoading(false));
    return () => {
      live = false;
    };
  }, [activeId, pinsTick]);

  // Indicador de «llamada en curso» del canal activo: sondea by-source + presentes
  // al abrir el canal y cada ~60 s. Ligero (dos lecturas, cero escrituras); degrada
  // a 0 si Meet no responde. Se detiene al cambiar de canal (sin fugas).
  //
  // Antes iba cada 25 s porque ERA la única forma de enterarse de una llamada. Ya
  // no: el frame `call_started` del WebSocket lo anuncia al instante, y a este
  // sondeo solo le queda ajustar CUÁNTA gente hay dentro. Para eso, 25 segundos
  // por cliente y canal era gastar por nada.
  const refrescarLlamadaActiva = useCallback((): void => {
    const chId = activeRef.current;
    if (!chId) return;
    getActiveCall(chId)
      .then((r) => activeRef.current === chId && setActiveCallCount(r.participants))
      .catch(() => undefined); // degradación limpia: sin banner
  }, []);

  useEffect(() => {
    setActiveCallCount(0);
    if (activeId) refrescarLlamadaActiva();
  }, [activeId, refrescarLlamadaActiva]);

  // Con la pestaña de fondo el banner no lo ve nadie: eran 60 consultas a la hora
  // por canal abierto. Al volver se consulta de inmediato, que es cuando importa.
  useSondeo(refrescarLlamadaActiva, 60_000, Boolean(activeId));

  // «Visto» en DMs (Ola 16): sondea el last_read_at de los otros miembros del DM activo
  // al abrirlo y cada ~15 s (una consulta agregada, sin N+1, y SOLO en DMs). El backend
  // devuelve vacío en canales grandes. Degrada en silencio; se detiene al salir del DM.
  // ponytail: sondeo ligero acotado al DM abierto; sin evento WS de lectura, es la vía
  // más simple. Upgrade: emitir un evento al marcar leído y refrescar en vivo.
  const enDM = channelsRef.current.find((c) => c.id === activeId)?.kind === 'direct';

  const refrescarReceipts = useCallback((): void => {
    const chId = activeRef.current;
    if (!chId) return;
    getReceipts(chId)
      .then(({ receipts: r }) => activeRef.current === chId && setReceipts(r))
      .catch(() => undefined); // degradación limpia: sin «Visto»
  }, []);

  useEffect(() => {
    setReceipts([]);
    if (activeId && enDM) refrescarReceipts();
  }, [activeId, enDM, refrescarReceipts]);

  // El que más pesaba de los tres: cuatro consultas por minuto, 240 a la hora, por
  // un «Visto» que con la pestaña de fondo no se está leyendo.
  useSondeo(refrescarReceipts, 15_000, Boolean(activeId) && enDM);

  // Ref al handle de tiempo real para (re)suscribir canales desde otro efecto.
  const realtimeRef = useRef<ReturnType<typeof connectRealtime> | null>(null);

  // Conexion de tiempo real (una sola, viva mientras la pestana exista).
  useEffect(() => {
    const handle = connectRealtime({
      userId: user.id,
      onStatus: setStatus,
      onEvent: (ev: ChatEvent) => {
        if (ev.type === 'message') {
          const msg = ev.message;
          // Aviso personal: el mensaje me menciona (@) o contiene una palabra clave mía
          // (Ola 13). El badge de menciones solo lo sube una @mención real; una palabra
          // clave solo dispara el aviso de escritorio. Si NO es personal, valora la
          // notificación de DM (mensaje directo con la ventana sin foco); shouldNotifyDirect
          // filtra no-DM/silenciado/nivel/propio/hilo/ajuste, sin solaparse con el aviso
          // personal. notifyMention/notifyDirectMessage suprimen dentro de la ventana DND.
          const isMention = mentionsUser(msg, user.id);
          const isPersonal = isMention || matchesKeyword(msg.body, notifyPrefsRef.current.keywords);
          if (isMention && msg.channelId !== activeRef.current) {
            setChannels((prev) =>
              prev.map((c) =>
                c.id === msg.channelId ? { ...c, mentions: (c.mentions ?? 0) + 1 } : c,
              ),
            );
          }
          if (isPersonal && msg.channelId !== activeRef.current) {
            notificationHandlersRef.current.notifyMention(msg);
          } else if (!isPersonal) {
            notificationHandlersRef.current.notifyDirectMessage(msg);
          }
          // Respuesta en hilo: NO va al histórico del canal; sube el contador del
          // padre y, si su hilo está abierto, se anexa. El eco propio ya se
          // gestionó de forma optimista (+ REST), así que se ignora (dedupe).
          if (msg.parentId) {
            if (msg.mine) return;
            setMessages((prev) =>
              prev.map((m) =>
                m.id === msg.parentId ? { ...m, replyCount: (m.replyCount ?? 0) + 1 } : m,
              ),
            );
            const openThreadId = threadRootRef.current?.id;
            if (openThreadId === msg.parentId) {
              setThreadReplies((prev) =>
                prev.some((r) => r.id === msg.id) ? prev : [...prev, msg],
              );
              setThreadRoot((prev) =>
                prev && prev.id === msg.parentId
                  ? { ...prev, replyCount: (prev.replyCount ?? 0) + 1 }
                  : prev,
              );
            }
            // Vista global «Hilos» en vivo: si sigo el hilo (está en la lista cargada),
            // sube su actividad + no leídos (salvo que lo esté viendo ahora) y reordena.
            setThreadsList((prev) => {
              if (!prev.some((th) => th.root.id === msg.parentId)) return prev;
              const bumped = prev.map((th) =>
                th.root.id === msg.parentId
                  ? {
                      ...th,
                      lastActivity: msg.createdAt,
                      root: { ...th.root, replyCount: (th.root.replyCount ?? 0) + 1 },
                      unreadReplies:
                        msg.parentId === openThreadId ? th.unreadReplies : th.unreadReplies + 1,
                    }
                  : th,
              );
              return [...bumped].sort((a, b) => (a.lastActivity < b.lastActivity ? 1 : -1));
            });
            return;
          }
          if (msg.channelId === activeRef.current && !viewingHistoryRef.current) {
            setMessages((prev) => {
              // Reconcilia el envio optimista por id; evita duplicados.
              if (prev.some((m) => m.id === msg.id)) return prev;
              const withoutPending = prev.filter(
                (m) => !(m.pending && m.body === msg.body && m.mine),
              );
              return capAtBottom([...withoutPending, msg]);
            });
          } else {
            // Canal inactivo, o el activo mientras se ve historial (ventana centrada, no el
            // final): NO se anexa —crearía un hueco—; solo sube el no leído del sidebar. El
            // banner «Ir a mensajes recientes» lleva al final, que marca leído.
            setChannels((prev) =>
              prev.map((c) => (c.id === msg.channelId ? { ...c, unread: (c.unread ?? 0) + 1 } : c)),
            );
          }
        } else if (ev.type === 'reaction') {
          // La reaccion propia ya se pinto optimista al hacer clic (evita doble
          // conteo); aplica solo las de OTROS usuarios sobre el canal abierto.
          if (ev.subject === user.id || ev.channelId !== activeRef.current) return;
          setMessages((prev) =>
            prev.map((m) =>
              m.id === ev.messageId ? applyReactionDelta(m, ev.emoji, ev.added, ev.subject) : m,
            ),
          );
        } else if (ev.type === 'card') {
          // Voto de encuesta o tarea marcada: el frame trae el recuento COMPLETO, así
          // que aplicarlo es idempotente (también para el eco del propio voto, que ya
          // se pintó optimista) y no hay deltas que cruzar. Se aplica también en el
          // hilo abierto: la encuesta puede estar ahí y no en el feed.
          setMessages((prev) => prev.map((m) => applyCardVotes(m, ev.messageId, ev.votes)));
          setThreadReplies((prev) => prev.map((m) => applyCardVotes(m, ev.messageId, ev.votes)));
          setThreadRoot((prev) => (prev ? applyCardVotes(prev, ev.messageId, ev.votes) : prev));
        } else if (ev.type === 'message.edited') {
          // Idempotente para el eco propio (mismo cuerpo/marca). El frame no trae
          // reacciones/replyCount: fusiona solo cuerpo + editedAt para no perderlos.
          setMessages((prev) =>
            prev.map((m) => (m.id === ev.message.id ? mergeEdit(m, ev.message) : m)),
          );
          setThreadReplies((prev) =>
            prev.map((m) => (m.id === ev.message.id ? mergeEdit(m, ev.message) : m)),
          );
          setThreadRoot((prev) =>
            prev && prev.id === ev.message.id ? mergeEdit(prev, ev.message) : prev,
          );
        } else if (ev.type === 'message.deleted') {
          // Idempotente para el eco propio (ya retirado). Cierra el hilo si borran su raíz.
          setMessages((prev) => prev.filter((m) => m.id !== ev.messageId));
          setThreadReplies((prev) => prev.filter((m) => m.id !== ev.messageId));
          setThreadRoot((prev) => (prev && prev.id === ev.messageId ? null : prev));
          setPins((prev) => prev.filter((m) => m.id !== ev.messageId));
        } else if (ev.type === 'pin') {
          // Fijado/desfijado en el canal abierto: mantiene el panel/contador en vivo.
          // Idempotente para el eco propio (ya aplicado de forma optimista).
          if (ev.channelId !== activeRef.current) return;
          if (ev.pinned) {
            const pinnedMsg = ev.message;
            if (pinnedMsg) {
              setPins((prev) =>
                prev.some((m) => m.id === ev.messageId) ? prev : [pinnedMsg, ...prev],
              );
            }
          } else {
            setPins((prev) => prev.filter((m) => m.id !== ev.messageId));
          }
        } else if (ev.type === 'channel.member') {
          // Roster en vivo del canal abierto. Idempotente para el eco propio (la
          // respuesta REST ya lo aplicó): el alta es un UPSERT por subject y cubre
          // también el cambio de rol.
          // ponytail: TECHO CONOCIDO del backend — la difusión solo alcanza a quien YA
          // está suscrito a la sala del canal, así que la sesión del RECIÉN AÑADIDO no
          // recibe nada y ve el canal al recargar. Se resuelve enrutando por subject en
          // el Hub del servicio (services/chat), no aquí.
          // Si el que sale soy yo (otra pestaña, o un admin me ha sacado), el canal
          // deja de ser mío: fuera de la barra y, si lo tenía abierto, se cierra.
          if (!ev.added && ev.subject === user.id) {
            const fuera = ev.channelId;
            setChannels((prev) => prev.filter((c) => c.id !== fuera));
            setActiveId((id) => (id === fuera ? null : id));
            return;
          }
          if (ev.channelId !== activeRef.current) return;
          if (ev.added) {
            const m = ev.member;
            if (m) setRoster((prev) => upsertMember(prev, m));
          } else {
            setRoster((prev) => prev.filter((m) => m.sub !== ev.subject));
          }
        } else if (ev.type === 'channel.deleted') {
          // Borrado por un admin: fuera de la barra de todos, y cerrado si estaba abierto.
          const borrado = ev.channelId;
          setChannels((prev) => prev.filter((c) => c.id !== borrado));
          setActiveId((id) => (id === borrado ? null : id));
        } else if (ev.type === 'call') {
          // La llamada se anuncia AHORA, no en el próximo sondeo. El contador
          // exacto de participantes sigue viniendo del sondeo —el frame solo dice
          // que empezó o terminó—, así que al empezar se pinta el banner con 1 y
          // el siguiente refresco lo ajusta; al terminar se apaga en seco.
          const suyo = ev.channelId;
          setLlamadasVivas((prev) => {
            const next = new Set(prev);
            if (ev.started) next.add(suyo);
            else next.delete(suyo);
            return next;
          });
          if (suyo === activeRef.current) {
            setActiveCallCount((prev) => (ev.started ? Math.max(prev, 1) : 0));
            return;
          }
          // Aquí estaba el agujero: el frame se descartaba por no ser el canal
          // abierto, así que a quien te llamaba por un directo no lo veías hasta
          // abrir esa conversación. Ahora es justo el caso que hay que avisar.
          if (!ev.started) {
            setEntrante((prev) => (prev?.channelId === suyo ? null : prev));
            return;
          }
          const aviso = avisoDeLlamada({
            channelId: suyo,
            activeId: activeRef.current,
            enLlamada: callRef.current !== null,
            canales: channelsRef.current,
          });
          if (aviso) setEntrante(aviso);
        } else if (ev.type === 'typing') {
          // "Escribiendo…": ignora el propio eco (otras pestañas del mismo usuario);
          // refresca la entrada y (re)programa su expiración a los 5 s sin más eventos.
          if (ev.subject === user.id) return;
          const { channelId, subject, name } = ev;
          setTyping((prev) => ({
            ...prev,
            [channelId]: { ...prev[channelId], [subject]: name },
          }));
          const tkey = `${channelId}:${subject}`;
          const pending = typingTimers.current.get(tkey);
          if (pending) clearTimeout(pending);
          typingTimers.current.set(
            tkey,
            setTimeout(() => {
              typingTimers.current.delete(tkey);
              setTyping((prev) => {
                const ch = prev[channelId];
                if (!ch || !(subject in ch)) return prev;
                const nextCh = { ...ch };
                delete nextCh[subject];
                const next = { ...prev };
                if (Object.keys(nextCh).length === 0) delete next[channelId];
                else next[channelId] = nextCh;
                return next;
              });
            }, 5000),
          );
        }
      },
    });
    realtimeRef.current = handle;
    const timers = typingTimers.current;
    return () => {
      handle.close();
      realtimeRef.current = null;
      for (const t of timers.values()) clearTimeout(t);
      timers.clear();
    };
  }, [user.id]);

  // Suscribe la conexion en vivo a cada canal conocido: sin la suscripcion el
  // servidor no difunde sus mensajes/reacciones. Idempotente y reenviada tras
  // reconectar (el handle recuerda los canales deseados).
  useEffect(() => {
    for (const c of channels) realtimeRef.current?.subscribe(c.id);
  }, [channels]);

  const active = useMemo(
    () => channels.find((c) => c.id === activeId) ?? null,
    [channels, activeId],
  );

  // Roster del canal activo. Se pide al abrir el canal (el facepile de la cabecera
  // necesita el conteo antes de abrir el panel) y solo para canales, no para DM.
  const activeIsChannel = active !== null && active.kind !== 'direct';
  useEffect(() => {
    if (!activeId || !activeIsChannel) {
      setRoster([]);
      return;
    }
    let live = true;
    setRoster([]);
    setRosterLoading(true);
    setRosterError(false);
    listChannelMembers(activeId)
      .then(({ members: list }) => live && setRoster(list))
      .catch(() => live && setRosterError(true))
      .finally(() => live && setRosterLoading(false));
    return () => {
      live = false;
    };
  }, [activeId, activeIsChannel, rosterTick]);

  // ¿Puedo administrar el roster? Admin DEL CANAL o del team, igual que la verja del
  // backend (requireChannelAdmin cae al rol de team si no hay fila de canal).
  // En una conversación de OTRA organización ser admin del equipo propio no cuenta:
  // allí solo manda el rol en ese canal (ADR 0040).
  const canManageMembers = useMemo(
    () =>
      (isTeamAdmin && !active?.external) ||
      roster.some((m) => m.sub === user.id && m.role === 'admin'),
    [isTeamAdmin, active?.external, roster, user.id],
  );

  // «Visto» del último mensaje propio en un DM (Ola 16): id del mensaje + etiqueta
  // accesible cuando el/los peers ya lo han leído (su last_read_at >= created_at del
  // mensaje). null si no es un DM, no tengo mensaje propio, o nadie lo ha visto aún.
  const seenReceipt = useMemo(() => {
    if (!active || active.kind !== 'direct') return null;
    let mine: ChatMessage | undefined;
    for (let i = messages.length - 1; i >= 0; i--) {
      const m = messages[i]!;
      if (m.mine && !m.pending && !m.failed) {
        mine = m;
        break;
      }
    }
    if (!mine) return null;
    const sentAt = new Date(mine.createdAt).getTime();
    const seenBy = receipts.filter(
      (r) => r.lastReadAt && new Date(r.lastReadAt).getTime() >= sentAt,
    ).length;
    if (seenBy === 0) return null;
    const total = active.peers?.length ?? 1;
    return {
      messageId: mine.id,
      label: total > 1 ? t('message.seenBy', { count: seenBy }) : t('message.seen'),
    };
  }, [active, messages, receipts, t]);

  // Al cambiar de canal, sal de la edición de tema (no arrastrar el borrador).
  useEffect(() => {
    setEditingTopic(false);
  }, [activeId]);

  // --- Ola 4: heartbeat de presencia propia (mientras Rooms está abierto) ---
  // Bucle auto-reprogramado: renueva la presencia cada ~TTL/2 (lo sugiere el Hub).
  // Con la pestaña oculta y estado "online" degrada a "away" (auto-ausencia); un
  // estado fijado a mano (ausente/no molestar) se respeta. Degradación limpia: si
  // el Hub no responde, reintenta al siguiente tick.
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const effective = (): PresenceState =>
      // En una llamada se está ocupado, se hubiera dicho o no: quien te busca en
      // la lista necesita saber que estás reunido antes de escribirte.
      callRef.current
        ? 'busy'
        : document.hidden && myStateRef.current === 'online'
          ? 'away'
          : myStateRef.current;
    // Lo que se ANUNCIA mientras dura la llamada. NO se toca `myStatus` ni lo que
    // hay en localStorage: el estado que la persona escribió sigue ahí y vuelve
    // solo al colgar. Si el navegador se cierra a mitad de la llamada, la
    // presencia caduca por TTL y al volver se anuncia el suyo, sin nada que
    // limpiar.
    const anuncio = (): string =>
      callRef.current ? tRef.current('presence.inCall') : myStatusRef.current;
    async function beat(): Promise<void> {
      if (stopped) return;
      let secs = 25; // fallback ~TTL/2 (TTL del Hub por defecto 45 s). Ajustar si cambia el TTL.
      try {
        const r = await sendHeartbeat(effective(), anuncio());
        if (r.heartbeat_seconds) secs = Math.min(Math.max(r.heartbeat_seconds, 15), 60);
      } catch {
        // Hub inaccesible: se reintenta en el siguiente tick (misma cadencia).
      }
      if (!stopped) timer = setTimeout(() => void beat(), secs * 1000);
    }
    const beatNow = (): void => {
      if (timer) clearTimeout(timer);
      void beat();
    };
    heartbeatNowRef.current = beatNow;
    void beat(); // inmediato al montar
    document.addEventListener('visibilitychange', beatNow);
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
      document.removeEventListener('visibilitychange', beatNow);
      heartbeatNowRef.current = null;
    };
  }, [tRef]);

  /** Cambia el estado/estado-personalizado propio y hace heartbeat inmediato con el
   *  nuevo valor (sincroniza los refs antes de disparar: setState es asíncrono).
   *  `expiresAt` (epoch ms) fija la caducidad del estado personalizado; null = no caduca;
   *  ausente = conserva la vigente (p. ej. al solo cambiar de presencia). status vacío
   *  borra el estado persistido. */
  function changeMyPresence(state: PresenceState, status: string, expiresAt?: number | null): void {
    setMyState(state);
    setMyStatus(status);
    myStateRef.current = state;
    myStatusRef.current = status;
    if (status === '') {
      saveCustomStatus('', null);
    } else {
      saveCustomStatus(status, expiresAt !== undefined ? expiresAt : storedExpiry());
    }
    heartbeatNowRef.current?.();
  }

  // Entrar o salir de una llamada cambia la presencia YA, sin esperar al siguiente
  // latido: veinticinco segundos anunciando que estás libre mientras hablas es
  // exactamente lo que esto viene a evitar.
  useEffect(() => {
    heartbeatNowRef.current?.();
  }, [call]);

  // Expiración del estado personalizado: si tiene fecha de caducidad, prográmate para
  // limpiarlo al vencer (y reenviar el heartbeat sin status vía changeMyPresence).
  // Se reprograma cuando cambia myStatus (nueva caducidad, o borrado). Al montar, si ya
  // venció, loadCustomStatus lo dejó en '' — aquí no hay fecha y no se programa nada.
  useEffect(() => {
    const expiresAt = storedExpiry();
    if (expiresAt == null) return;
    const ms = expiresAt - Date.now();
    const clear = (): void => changeMyPresence(myStateRef.current, '');
    if (ms <= 0) {
      clear();
      return;
    }
    const id = setTimeout(clear, ms);
    return () => clearTimeout(id);
  }, [myStatus]);

  /** Sub del otro participante de un DM 1:1 (o undefined si es grupo / no es DM). Usa
   *  `peerSubs` del backend (per-visor, alineado con `peers`): mapea el punto de
   *  presencia al peer EXACTO, sin el frágil match por nombre. */
  function dmPeerSub(c: Channel): string | undefined {
    return c.kind === 'direct' && c.peerSubs?.length === 1 ? c.peerSubs[0] : undefined;
  }

  // Subs de TODOS los peers de los DMs visibles (1:1 y grupos) y, con el panel de
  // miembros abierto, los del canal: la presencia de otros se consulta en BATCH (una
  // GET por cada 200, el tope del Hub), no N peticiones. Por SUB, no por nombre. La
  // clave (string, ordenada) evita re-consultar por identidad de array.
  const presenceSubs = useMemo(() => {
    const subs = new Set<string>();
    for (const c of channels) {
      if (c.kind === 'direct') for (const s of c.peerSubs ?? []) subs.add(s);
    }
    if (rhsView === 'members') for (const m of roster) subs.add(m.sub);
    return [...subs].sort();
  }, [channels, rhsView, roster]);
  const presenceSubsKey = presenceSubs.join(',');

  // Consulta la presencia de los DMs visibles al cargar y la refresca cada 60 s.
  const refrescarPresencia = useCallback((): void => {
    if (presenceSubsKey === '') return;
    queryPresence(presenceSubsKey.split(','))
      .then((map) => setPresences((prev) => ({ ...prev, ...map })))
      .catch(() => undefined); // degradación limpia: conserva lo último conocido
  }, [presenceSubsKey]);

  useEffect(() => {
    refrescarPresencia();
  }, [refrescarPresencia]);

  // Los puntos de color de una lista que no se está mirando no hacen falta; al
  // volver se consultan de golpe y salen al día.
  useSondeo(refrescarPresencia, 60_000, presenceSubsKey !== '');

  // Presencia del peer del DM activo (para el punto y el subtítulo de la cabecera).
  const activePeer = active ? dmPeerSub(active) : undefined;
  const activePeerPresence = activePeer ? presences[activePeer] : undefined;
  // "Escribiendo…" del canal activo (nombres de quienes escriben ahora mismo).
  const typingNames = activeId ? Object.values(typing[activeId] ?? {}) : [];

  function handleSend(
    body: string,
    mentions: string[] = [],
    attachments: Attachment[] = [],
    opts?: ComposeOptions,
  ): void {
    if (!activeId) return;

    // Enviar mientras se ve historial (ventana centrada de un permalink): vuelve al modo
    // normal — el mensaje se anexa al histórico visible y los WS posteriores dejan de
    // suprimirse. Se sincroniza el ref además del estado (el efecto WS lo lee por ref).
    if (viewingHistoryRef.current) {
      viewingHistoryRef.current = false;
      setViewingHistory(false);
    }

    // Programado (Ola 12): no entra en el feed; el worker lo publica al vencer y
    // llega por WS. Confirmación por toast; si el panel «Programados» está abierto,
    // se refleja al instante.
    if (opts?.sendAt) {
      const channelId = activeId;
      scheduleMessage({
        channelId,
        body,
        sendAt: opts.sendAt,
        ...(mentions.length ? { mentions } : {}),
        ...(attachments.length ? { fileIds: attachments.map((a) => a.fileId) } : {}),
      })
        .then(({ scheduled }) => {
          toast.notify({
            tone: 'success',
            title: t('schedule.done'),
            description: t('schedule.doneDetail', {
              when: formatDateTime(locale, scheduled.sendAt, {
                dateStyle: 'long',
                timeStyle: 'short',
              }),
            }),
          });
          setScheduledList((prev) =>
            [...prev, scheduled].sort((a, b) => (a.sendAt < b.sendAt ? -1 : 1)),
          );
        })
        .catch(() => toast.notify({ tone: 'danger', title: t('schedule.failed') }));
      return;
    }

    const clientId = newClientId();
    const optimistic: ChatMessage = {
      id: `pending-${clientId}`,
      channelId: activeId,
      author: { id: user.id, name: user.name },
      body,
      createdAt: new Date().toISOString(),
      mine: true,
      pending: true,
      ...(mentions.length ? { mentions } : {}),
      // Adjuntos ya subidos a Drive: se pintan al instante (URLs válidas) y el
      // servidor los reconcilia con los metadatos resueltos.
      ...(attachments.length ? { attachments } : {}),
      ...(opts?.priority ? { priority: opts.priority } : {}),
      ...(opts?.requiresAck ? { requiresAck: true } : {}),
      // La tarjeta se pinta ya (sin votos): el mensaje optimista de una encuesta debe
      // enseñar sus opciones, no solo la pregunta suelta.
      ...(opts?.card ? { card: opts.card } : {}),
    };
    setMessages((prev) => capAtBottom([...prev, optimistic]));

    sendMessage({
      channelId: activeId,
      body,
      clientId,
      ...(mentions.length ? { mentions } : {}),
      ...(attachments.length ? { fileIds: attachments.map((a) => a.fileId) } : {}),
      ...(opts?.priority ? { priority: opts.priority } : {}),
      ...(opts?.requiresAck ? { requiresAck: true } : {}),
      ...(opts?.card ? { card: opts.card } : {}),
    })
      .then(({ message }) =>
        setMessages((prev) => prev.map((m) => (m.id === optimistic.id ? message : m))),
      )
      .catch((err: unknown) => {
        // Marca el optimista como fallido; MessageView ofrece «Reintentar».
        setMessages((prev) =>
          prev.map((m) => (m.id === optimistic.id ? { ...m, pending: false, failed: true } : m)),
        );
        // Un 403 en un directo es un bloqueo entre las dos personas: reintentar no
        // lo arregla, así que se dice por qué.
        if (
          err instanceof ChatHttpError &&
          err.status === 403 &&
          channelsRef.current.find((c) => c.id === activeId)?.kind === 'direct'
        ) {
          toast.notify({ tone: 'danger', title: t('block.cannotSend') });
        }
      });
  }

  /** Confirma el acuse de recibo de un mensaje (Ola 12): optimista (marca acked +
   *  sube el conteo en el feed y en el hilo) y reconcilia con el total del servidor;
   *  revierte si falla. Idempotente en el backend. */
  function handleAck(message: ChatMessage): void {
    if (message.acked) return;
    const apply = (patch: (m: ChatMessage) => ChatMessage): void => {
      setMessages((prev) => prev.map((m) => (m.id === message.id ? patch(m) : m)));
      setThreadReplies((prev) => prev.map((m) => (m.id === message.id ? patch(m) : m)));
      setThreadRoot((prev) => (prev && prev.id === message.id ? patch(prev) : prev));
    };
    apply((m) => ({ ...m, acked: true, ackCount: (m.ackCount ?? 0) + 1 }));
    ackMessage({ messageId: message.id })
      .then(({ count }) => apply((m) => ({ ...m, acked: true, ackCount: count })))
      .catch(() =>
        apply((m) => ({ ...m, acked: false, ackCount: Math.max(0, (m.ackCount ?? 1) - 1) })),
      );
  }

  /** Reintenta un envio fallido: retira el mensaje fallido y lo reenvia (conserva
   *  sus adjuntos, ya subidos a Drive). */
  function handleResend(message: ChatMessage): void {
    setMessages((prev) => prev.filter((m) => m.id !== message.id));
    handleSend(message.body, message.mentions ?? [], message.attachments ?? []);
  }

  /**
   * Página anterior del histórico (scroll cerca del principio): cursor keyset
   * `before` = createdAt del mensaje más antiguo cargado. Un solo vuelo a la
   * vez; si la página llega corta, no hay más historial.
   */
  function loadOlder(): void {
    const oldest = messages[0];
    if (!activeId || !oldest || !hasOlder || loadingOlderRef.current || loadingMessages) return;
    loadingOlderRef.current = true;
    setLoadingOlder(true);
    const channelId = activeId;
    listMessages(channelId, oldest.createdAt)
      .then(({ messages: older }) => {
        if (activeRef.current !== channelId) return; // cambió de canal en vuelo
        if (older.length < PAGE_SIZE) setHasOlder(false);
        if (older.length === 0) return;
        setMessages((prev) => {
          // Dedupe defensivo (empates de createdAt en el cursor estricto):
          // claves repetidas romperían el anclaje por clave del virtualizador.
          const known = new Set(prev.map((m) => m.id));
          return [...older.filter((o) => !known.has(o.id)), ...prev];
        });
      })
      .catch(() => undefined) // reintento natural: el siguiente scroll arriba vuelve a pedirla
      .finally(() => {
        loadingOlderRef.current = false;
        setLoadingOlder(false);
      });
  }

  /** Resalta un mensaje ~2,5 s (el scroll a él lo hace MessageView por su índice).
   *  Se reprograma si se salta a otro antes de expirar. */
  function highlightMessage(messageId: string): void {
    if (highlightTimer.current) clearTimeout(highlightTimer.current);
    setHighlightId(messageId);
    highlightTimer.current = setTimeout(() => {
      setHighlightId(null);
      highlightTimer.current = null;
    }, 2500);
  }

  /**
   * Navega a un mensaje concreto (permalink «Copiar enlace», resultado de búsqueda o
   * deep-link `?message=`): abre su canal si hace falta y carga una ventana centrada en
   * él (el efecto de carga consume jumpTargetRef, hace scroll y lo resalta). Cierra los
   * paneles RHS para que el histórico quede en primer plano.
   */
  function goToMessage(channelId: string, messageId: string): void {
    jumpTargetRef.current = { channelId, messageId };
    setRhsView(null);
    closeThread();
    if (activeRef.current === channelId) {
      setMessagesTick((n) => n + 1); // mismo canal: refuerza el efecto de carga
    } else {
      setActiveId(channelId);
    }
  }

  /** Vuelve al final del canal desde el modo «viendo historial»: recarga la última
   *  página y marca leído (el efecto de carga, sin objetivo de salto). */
  function jumpToPresent(): void {
    setViewingHistory(false);
    setMessagesTick((n) => n + 1);
  }

  /** Abre el panel de hilo de un mensaje raíz y carga raíz + respuestas. */
  function openThread(message: ChatMessage): void {
    if (message.pending || message.failed) return; // solo mensajes persistidos
    setRhsView(null); // el hilo y los paneles auxiliares son excluyentes
    setThreadRoot(message);
    setThreadReplies([]);
    setThreadError(false);
    setThreadLoading(true);
    setThreadFollowing(false);
    // Abrir el hilo lo marca visto en el servidor (Ola 9): baja su no leído a 0 en la
    // vista global «Hilos» localmente (el badge se reconcilia al recargar).
    setThreadsList((prev) =>
      prev.map((th) => (th.root.id === message.id ? { ...th, unreadReplies: 0 } : th)),
    );
    getThread(message.id)
      .then(({ root, replies, following }) => {
        setThreadFollowing(following);
        // Dato autoritativo per-mensaje: alimenta «Siguiendo» en el feed.
        setFollowOverrides((prev) => ({ ...prev, [root.id]: following }));
        // Reconciliación no destructiva: si el usuario respondió mientras cargaba
        // el hilo, conserva su respuesta optimista y su +1 en el contador (Math.max),
        // en lugar de pisarlos con la foto del servidor (carrera carga vs. envío).
        const serverIds = new Set(replies.map((r) => r.id));
        setThreadReplies((prev) => [
          ...replies,
          ...prev.filter((r) => (r.pending || r.failed) && !serverIds.has(r.id)),
        ]);
        setThreadRoot((prev) =>
          prev && prev.id === root.id
            ? { ...root, replyCount: Math.max(root.replyCount ?? 0, prev.replyCount ?? 0) }
            : root,
        );
        setMessages((prev) =>
          prev.map((m) =>
            m.id === root.id
              ? { ...m, replyCount: Math.max(root.replyCount ?? 0, m.replyCount ?? 0) }
              : m,
          ),
        );
      })
      .catch(() => setThreadError(true))
      .finally(() => setThreadLoading(false));
  }

  function closeThread(): void {
    setThreadRoot(null);
    setThreadReplies([]);
    setThreadError(false);
  }

  /**
   * Alterna seguir/dejar de seguir el hilo de un mensaje raíz (optimista; revierte si
   * falla). Lo usan el botón de la cabecera del hilo y «Seguir mensaje» del menú «…»
   * del feed. Refresca la vista global «Hilos» para reflejar la alta/baja y su contador.
   */
  function toggleFollow(messageId: string, following: boolean): void {
    const next = !following;
    setFollowOverrides((prev) => ({ ...prev, [messageId]: next }));
    if (threadRoot?.id === messageId) setThreadFollowing(next);
    const call = next ? followThread({ messageId }) : unfollowThread({ messageId });
    call
      .then(() => loadThreads())
      .catch(() => {
        setFollowOverrides((prev) => ({ ...prev, [messageId]: following }));
        if (threadRoot?.id === messageId) setThreadFollowing(following);
      });
  }

  /** Alterna el seguimiento del hilo ABIERTO (botón de la cabecera del ThreadPanel). */
  function toggleThreadFollow(): void {
    if (threadRoot) toggleFollow(threadRoot.id, threadFollowing);
  }

  /** Publica una respuesta en el hilo abierto (optimista + reconciliación). */
  function handleSendReply(
    body: string,
    mentions: string[] = [],
    attachments: Attachment[] = [],
  ): void {
    const root = threadRoot;
    if (!root) return;
    const clientId = newClientId();
    const optimistic: ChatMessage = {
      id: `pending-${clientId}`,
      channelId: root.channelId,
      parentId: root.id,
      author: { id: user.id, name: user.name },
      body,
      createdAt: new Date().toISOString(),
      mine: true,
      pending: true,
      ...(mentions.length ? { mentions } : {}),
      ...(attachments.length ? { attachments } : {}),
    };
    setThreadReplies((prev) => [...prev, optimistic]);
    // Sube el contador de forma optimista (histórico + raíz del panel).
    setMessages((prev) =>
      prev.map((m) => (m.id === root.id ? { ...m, replyCount: (m.replyCount ?? 0) + 1 } : m)),
    );
    setThreadRoot((prev) =>
      prev && prev.id === root.id ? { ...prev, replyCount: (prev.replyCount ?? 0) + 1 } : prev,
    );

    postReply({
      messageId: root.id,
      body,
      ...(mentions.length ? { mentions } : {}),
      ...(attachments.length ? { fileIds: attachments.map((a) => a.fileId) } : {}),
    })
      .then(({ message }) => {
        setThreadReplies((prev) => prev.map((m) => (m.id === optimistic.id ? message : m)));
        // Responder auto-sigue el hilo en el backend (Ola 9): refleja «Siguiendo».
        setThreadFollowing(true);
      })
      .catch(() => {
        // Fallo: marca la respuesta como fallida y revierte el contador optimista.
        setThreadReplies((prev) =>
          prev.map((m) => (m.id === optimistic.id ? { ...m, pending: false, failed: true } : m)),
        );
        setMessages((prev) =>
          prev.map((m) =>
            m.id === root.id ? { ...m, replyCount: Math.max(0, (m.replyCount ?? 1) - 1) } : m,
          ),
        );
        setThreadRoot((prev) =>
          prev && prev.id === root.id
            ? { ...prev, replyCount: Math.max(0, (prev.replyCount ?? 1) - 1) }
            : prev,
        );
      });
  }

  /**
   * Alterna una reaccion del usuario sobre un mensaje: pinta el toggle optimista,
   * llama al BFF (PUT/DELETE en el servicio Go) y revierte si la mutacion falla.
   * El resto de clientes lo recibe por WebSocket (evento `reaction`).
   */
  function handleToggleReaction(messageId: string, emoji: string): void {
    const msg = messages.find((m) => m.id === messageId);
    if (!msg || msg.pending || msg.failed) return; // no reaccionar a optimistas/fallidos
    const mine = msg.reactions?.find((r) => r.emoji === emoji)?.mine ?? false;

    setMessages((prev) =>
      prev.map((m) => (m.id === messageId ? toggleMyReaction(m, emoji, user.id) : m)),
    );

    reactMessage({ messageId, emoji, add: !mine }).catch(() =>
      // Fallo de la mutacion: deshaz el toggle (es su propio inverso).
      setMessages((prev) =>
        prev.map((m) => (m.id === messageId ? toggleMyReaction(m, emoji, user.id) : m)),
      ),
    );
  }

  /**
   * Marca o retira un valor de la tarjeta de un mensaje (opción de encuesta, tarea
   * hecha): pinta el cambio optimista, llama al BFF y RECONCILIA con el recuento que
   * devuelve el servidor —que es la verdad, incluidos los votos ajenos que llegaran
   * entretanto—. Si falla, se restaura el estado previo.
   */
  function handleCardVote(messageId: string, value: string, add: boolean): void {
    // La tarjeta puede estar en el feed o en el hilo abierto (una encuesta se responde
    // igual de bien desde el panel de hilo).
    const before = [...messages, ...threadReplies, threadRoot].find(
      (m): m is ChatMessage => Boolean(m) && m!.id === messageId,
    );
    if (!before?.card || before.pending || before.failed) return;
    const optimistic = toggleMyCardVote(before.card, value, add, user.id);
    const apply = (votes: CardVote[]): void => {
      setMessages((prev) => prev.map((m) => applyCardVotes(m, messageId, votes)));
      setThreadReplies((prev) => prev.map((m) => applyCardVotes(m, messageId, votes)));
      setThreadRoot((prev) => (prev ? applyCardVotes(prev, messageId, votes) : prev));
    };
    apply(optimistic);
    voteCard({ messageId, value, add })
      .then(({ votes }) => apply(votes))
      .catch(() => apply(before.card?.votes ?? []));
  }

  /**
   * Edita un mensaje propio: pinta el nuevo cuerpo + marca «(editado)» optimista,
   * llama al BFF (PATCH) y reconcilia con la respuesta; revierte si falla. El resto
   * de clientes lo recibe por WebSocket (evento `message.edited`).
   */
  function handleEdit(messageId: string, body: string): void {
    const prev = messages.find((m) => m.id === messageId);
    if (!prev || prev.pending || prev.failed || body === prev.body) return;
    const optimistic: ChatMessage = { ...prev, body, editedAt: new Date().toISOString() };
    setMessages((ms) => ms.map((m) => (m.id === messageId ? optimistic : m)));

    editMessage({ messageId, body })
      .then(({ message }) =>
        setMessages((ms) => ms.map((m) => (m.id === messageId ? mergeEdit(m, message) : m))),
      )
      .catch(() =>
        // Fallo: restaura el mensaje previo (cuerpo y marca de edición originales).
        setMessages((ms) => ms.map((m) => (m.id === messageId ? prev : m))),
      );
  }

  /**
   * Borra un mensaje propio: confirma el servidor (borrado suave) y lo retira del
   * histórico. El resto de clientes lo recibe por WebSocket (`message.deleted`).
   */
  function handleDelete(messageId: string): void {
    deleteMessage({ messageId })
      .then(() => setMessages((prev) => prev.filter((m) => m.id !== messageId)))
      .catch(() => undefined);
  }

  /** Alterna el fijado de un mensaje (optimista; revierte si la mutación falla).
   *  El resto de clientes lo recibe por WebSocket (evento pin). */
  function togglePin(message: ChatMessage, pinned: boolean): void {
    if (pinned) {
      setPins((prev) => prev.filter((p) => p.id !== message.id));
      unpinMessage({ messageId: message.id }).catch(() =>
        setPins((prev) => (prev.some((p) => p.id === message.id) ? prev : [message, ...prev])),
      );
    } else {
      setPins((prev) => (prev.some((p) => p.id === message.id) ? prev : [message, ...prev]));
      pinMessage({ messageId: message.id })
        .then(({ message: pinnedMsg }) =>
          setPins((prev) => prev.map((p) => (p.id === message.id ? pinnedMsg : p))),
        )
        .catch(() => setPins((prev) => prev.filter((p) => p.id !== message.id)));
    }
  }

  /** Alterna el guardado de un mensaje (per-usuario; optimista + reversión). */
  function toggleSave(message: ChatMessage, saved: boolean): void {
    const withId = (add: boolean) =>
      setSavedIds((prev) => {
        const next = new Set(prev);
        if (add) next.add(message.id);
        else next.delete(message.id);
        return next;
      });
    if (saved) {
      withId(false);
      setSavedList((prev) => prev.filter((m) => m.id !== message.id));
      unsaveMessage({ messageId: message.id }).catch(() => withId(true));
    } else {
      withId(true);
      setSavedList((prev) => (prev.some((m) => m.id === message.id) ? prev : [message, ...prev]));
      saveMessage({ messageId: message.id }).catch(() => {
        withId(false);
        setSavedList((prev) => prev.filter((m) => m.id !== message.id));
      });
    }
  }

  /** Marca el canal como no leído a partir de un mensaje: sube el badge del canal
   *  (cuenta local = mensajes desde el objetivo que no son míos) y persiste; el
   *  contador exacto se reconcilia al recargar los canales.
   *  ponytail: si marcas no leído en el canal que estás viendo, el badge persiste
   *  hasta que cambias de canal y vuelves (markRead lo limpia), como en Mattermost. */
  function markMessageUnread(message: ChatMessage): void {
    const chId = message.channelId;
    const idx = messages.findIndex((m) => m.id === message.id);
    const count = idx === -1 ? 1 : messages.slice(idx).filter((m) => !m.mine).length;
    const prevUnread = channels.find((c) => c.id === chId)?.unread ?? 0;
    setChannels((prev) =>
      prev.map((c) => (c.id === chId ? { ...c, unread: Math.max(prevUnread, count) } : c)),
    );
    markUnread({ messageId: message.id }).catch(() =>
      setChannels((prev) => prev.map((c) => (c.id === chId ? { ...c, unread: prevUnread } : c))),
    );
  }

  // --- Ola 8: reenviar un mensaje a otro canal ---

  /** Abre el selector de canal destino para reenviar un mensaje. */
  function openForward(message: ChatMessage): void {
    setForwarding(message);
  }

  /**
   * Reenvía el mensaje seleccionado al canal destino: publica una cita del original
   * (cuerpo citado + atribución al autor + enlace canónico) reusando el envío del
   * SDK. No arrastra menciones (no re-notifica a los @mencionados). Si el destino es
   * el canal abierto, el mensaje aparece por el eco de WebSocket; si no, estará allí
   * al visitarlo. Devuelve la promesa para que el modal muestre el toast / el error.
   */
  async function handleForward(target: Channel): Promise<void> {
    const msg = forwarding;
    if (!msg) return;
    const body = forwardedBody(msg.body, msg.author.name, messageLink(msg.channelId, msg.id));
    await sendMessage({ channelId: target.id, body, clientId: newClientId() });
  }

  // --- Ola 3: favoritos / silenciar / archivar / tema / DM ---

  /** Alterna el favorito del canal (per-usuario; optimista + reversión). */
  function toggleFavorite(c: Channel): void {
    const next = !c.favorite;
    setChannels((prev) => prev.map((x) => (x.id === c.id ? { ...x, favorite: next } : x)));
    setChannelPrefs(c.id, { favorite: next }).catch(() =>
      setChannels((prev) =>
        prev.map((x) => (x.id === c.id ? { ...x, favorite: c.favorite ?? false } : x)),
      ),
    );
  }

  /** Fija el nivel de notificación del canal (per-usuario; optimista + reversión). El
   *  atenuado del sidebar (muted) se deriva de 'none' (fuente de verdad: notify_level). */
  function setNotifyLevel(c: Channel, level: 'all' | 'mentions' | 'none'): void {
    if ((c.notifyLevel ?? 'all') === level) return;
    const prevLevel = c.notifyLevel ?? 'all';
    const prevMuted = c.muted ?? false;
    setChannels((prev) =>
      prev.map((x) => (x.id === c.id ? { ...x, notifyLevel: level, muted: level === 'none' } : x)),
    );
    setChannelPrefs(c.id, { notifyLevel: level }).catch(() =>
      setChannels((prev) =>
        prev.map((x) => (x.id === c.id ? { ...x, notifyLevel: prevLevel, muted: prevMuted } : x)),
      ),
    );
  }

  /** Archiva un canal (solo admin; el backend valida). Confirma, lo saca del sidebar
   *  y deselecciona; revierte si la mutación falla.
   */
  // Archivar un canal es destructivo para quien lo usa y lo confirmaba el diálogo
  // del navegador: botones en el idioma del sistema operativo y sin sitio donde
  // decir qué implica. El propio comentario de arriba lo tenía anotado como deuda.
  const [archivando, setArchivando] = useState<Channel | null>(null);
  // Para las confirmaciones sueltas: cada una era un `window.confirm`.
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();

  function archiveChannel(c: Channel): void {
    setArchivando(null);
    const snapshot = channels;
    setChannels((prev) => prev.filter((x) => x.id !== c.id));
    setActiveId((id) => (id === c.id ? null : id));
    updateChannel(c.id, { archived: true }).catch(() => setChannels(snapshot));
  }

  /** Saca un canal de la barra ya (y lo cierra si estaba abierto). Devuelve con qué
   *  deshacerlo si el servicio dice que no. Se vuelve a meter por id, no con una foto
   *  de `channels`: entre medias puede haber llegado un mensaje o un canal nuevo. */
  function quitarDeLaBarra(c: Channel): () => void {
    const eraActivo = activeRef.current === c.id;
    setChannels((prev) => prev.filter((x) => x.id !== c.id));
    setActiveId((id) => (id === c.id ? null : id));
    return () => {
      setChannels((prev) => (prev.some((x) => x.id === c.id) ? prev : [...prev, c]));
      if (eraActivo) setActiveId((id) => id ?? c.id);
    };
  }

  /** Sale de un canal: optimista y, si falla, vuelve con su aviso (409 = eres el
   *  último admin de un canal privado). Lo comparten «/leave» y «Salir del canal». */
  function salirDelCanal(c: Channel): void {
    const deshacer = quitarDeLaBarra(c);
    leaveChannel(c.id).catch((err: unknown) => {
      deshacer();
      const ultimoAdmin = err instanceof ChatHttpError && err.status === 409;
      toast.notify({
        tone: 'danger',
        title: ultimoAdmin ? t('channel.lastAdmin') : t('command.leaveFailed'),
      });
    });
  }

  /** «Salir del canal» de los menús: lo mismo que «/leave», tras confirmarlo. */
  async function pedirSalir(c: Channel): Promise<void> {
    const ok = await confirmar({
      mensaje: t('channel.confirmLeave', { name: c.name }),
      confirmLabel: t('channel.leave'),
      tone: 'danger',
    });
    if (ok) salirDelCanal(c);
  }

  /** ¿Puedo borrar este canal? Admin del equipo (no en conversaciones de otra
   *  organización) o admin DEL CANAL —que solo se sabe del abierto, por su roster—.
   *  Un directo nunca: eso es «Borrar chat», que solo lo quita para ti. */
  function puedeBorrar(c: Channel): boolean {
    if (c.kind === 'direct') return false;
    return (isTeamAdmin && !c.external) || (c.id === activeId && canManageMembers);
  }

  /** Borra un canal para todos o, en un directo, «borra el chat» solo para ti. Pide
   *  confirmación, sale de la barra ya y vuelve con aviso si el servicio dice que no. */
  async function pedirBorrar(c: Channel): Promise<void> {
    const directo = c.kind === 'direct';
    const ok = await confirmar({
      mensaje: directo
        ? t('dm.confirmDelete', { name: c.name })
        : t('channel.confirmDelete', { name: c.name }),
      confirmLabel: directo ? t('dm.delete') : t('channel.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    const deshacer = quitarDeLaBarra(c);
    (directo ? hideDirect(c.id) : deleteChannel(c.id)).catch(() => {
      deshacer();
      toast.notify({
        tone: 'danger',
        title: directo ? t('dm.deleteFailed') : t('channel.deleteFailed'),
      });
    });
  }

  // --- Ola 11: categorías del sidebar ---

  /** Colapsa/expande una sección: personalizada → persiste en el backend; por defecto →
   *  en localStorage. Optimista + reversión (personalizadas). */
  function toggleGroupCollapse(group: SidebarGroup): void {
    const cat = group.category;
    if (cat) {
      const next = !cat.collapsed;
      setCategories((prev) => prev.map((c) => (c.id === cat.id ? { ...c, collapsed: next } : c)));
      updateCategory(cat.id, { collapsed: next }).catch(() =>
        setCategories((prev) =>
          prev.map((c) => (c.id === cat.id ? { ...c, collapsed: !next } : c)),
        ),
      );
      return;
    }
    setCollapsedDefaults((prev) => {
      const next = new Set(prev);
      if (next.has(group.key)) next.delete(group.key);
      else next.add(group.key);
      try {
        localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...next]));
      } catch {
        // almacenamiento no disponible: el colapso vale solo para esta sesión.
      }
      return next;
    });
  }

  /** ¿La sección está colapsada? (personalizada: backend; por defecto: localStorage). */
  function isGroupCollapsed(group: SidebarGroup): boolean {
    return group.category ? group.category.collapsed : collapsedDefaults.has(group.key);
  }

  /** Crea una categoría (nombre por prompt nativo, como el confirm() de archivar).
   */
  async function createNewCategory(): Promise<void> {
    const name = (await preguntar({ mensaje: t('category.newPrompt') }))?.trim();
    if (!name) return;
    try {
      const { category } = await createCategory(name);
      setCategories((prev) => [...prev, category]);
    } catch {
      // el backend valida el nombre; si falla, no se añade (se reconcilia al recargar).
    }
  }

  /** Renombra una categoría (optimista + reversión). */
  async function renameCategory(cat: Category): Promise<void> {
    const name = (
      await preguntar({ mensaje: t('category.renamePrompt'), valor: cat.name })
    )?.trim();
    if (!name || name === cat.name) return;
    setCategories((prev) => prev.map((c) => (c.id === cat.id ? { ...c, name } : c)));
    updateCategory(cat.id, { name }).catch(() =>
      setCategories((prev) => prev.map((c) => (c.id === cat.id ? { ...c, name: cat.name } : c))),
    );
  }

  /** Bloquea (con confirmación) o desbloquea a alguien con quien se comparte un canal. */
  async function toggleBlock(
    channelId: string,
    sub: string,
    name: string,
    blocked: boolean,
  ): Promise<void> {
    if (blocked) {
      try {
        await unblockPerson(sub);
        setBlocks((prev) => prev.filter((b) => b.sub !== sub));
        toast.notify({ tone: 'success', title: t('block.unblocked', { name }) });
      } catch {
        toast.notify({ tone: 'danger', title: t('block.failed') });
      }
      return;
    }
    // El nombre, una vez: en el título. Iba en el título, el mensaje y el botón
    // («Bloquear a Pablo Iglesias Fernández», 258 px), que partía el pie en dos filas.
    const ok = await confirmar({
      titulo: t('block.confirm', { name }),
      mensaje: t('block.explain'),
      confirmLabel: t('block.blockShort'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      const { block } = await blockPerson(channelId, sub);
      setBlocks((prev) => [block, ...prev.filter((b) => b.sub !== sub)]);
      toast.notify({ tone: 'success', title: t('block.blocked', { name }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('block.failed') });
    }
  }

  /** Borra una categoría; sus canales recaen a su sección por defecto (optimista). */
  async function removeCategory(cat: Category): Promise<void> {
    const ok = await confirmar({
      mensaje: t('category.confirmDelete', { name: cat.name }),
      confirmLabel: t('category.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    const snapCats = categories;
    const snapChans = channels;
    setCategories((prev) => prev.filter((c) => c.id !== cat.id));
    setChannels((prev) => prev.map((c) => (c.categoryId === cat.id ? withCategory(c, null) : c)));
    deleteCategory(cat.id).catch(() => {
      setCategories(snapCats);
      setChannels(snapChans);
    });
  }

  /** Reordena una categoría personalizada: la reinserta ante `targetId` y renumera el
   *  sortOrder de todas, persistiendo las que cambian (optimista + reversión). */
  function reorderCategoryBefore(draggedId: string, targetId: string): void {
    if (draggedId === targetId) return;
    const ordered = [...categories].sort(
      (a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name),
    );
    const from = ordered.findIndex((c) => c.id === draggedId);
    const to = ordered.findIndex((c) => c.id === targetId);
    if (from === -1 || to === -1) return;
    const [moved] = ordered.splice(from, 1);
    ordered.splice(to, 0, moved!);
    const snapshot = categories;
    const renumbered = ordered.map((c, i) => ({ ...c, sortOrder: i }));
    setCategories(renumbered);
    const changed = renumbered.filter(
      (c) => snapshot.find((s) => s.id === c.id)?.sortOrder !== c.sortOrder,
    );
    Promise.all(changed.map((c) => updateCategory(c.id, { sortOrder: c.sortOrder }))).catch(() =>
      setCategories(snapshot),
    );
  }

  /** Sube/baja una categoría un puesto (camino accesible de reordenar, sin arrastrar). */
  function moveCategory(cat: Category, dir: -1 | 1): void {
    const ordered = [...categories].sort(
      (a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name),
    );
    const idx = ordered.findIndex((c) => c.id === cat.id);
    const neighbor = ordered[idx + dir];
    if (neighbor) reorderCategoryBefore(cat.id, neighbor.id);
  }

  /** Mueve un canal a una categoría (categoryId null = su sección por defecto). Optimista.
   *  Con exactOptionalPropertyTypes hay que OMITIR la clave (no ponerla a undefined): lo
   *  hace withCategory. */
  function moveChannelToCategory(channelId: string, categoryId: string | null): void {
    const c = channels.find((x) => x.id === channelId);
    if (!c || (c.categoryId ?? null) === categoryId) return;
    const prev = c.categoryId ?? null;
    setChannels((list) => list.map((x) => (x.id === channelId ? withCategory(x, categoryId) : x)));
    setChannelCategory(channelId, categoryId).catch(() =>
      setChannels((list) => list.map((x) => (x.id === channelId ? withCategory(x, prev) : x))),
    );
  }

  /** Suelta el canal arrastrado en una sección: personalizada → la asigna; por defecto →
   *  quita la categoría (y, en Favoritos, marca favorito). */
  function dropChannelOnGroup(group: SidebarGroup, channelId: string): void {
    if (group.category) {
      moveChannelToCategory(channelId, group.category.id);
      return;
    }
    moveChannelToCategory(channelId, null);
    if (group.key === DEFAULT_FAVORITES) {
      const c = channels.find((x) => x.id === channelId);
      if (c && !c.favorite) toggleFavorite(c);
    }
  }

  /** Ítems del menú «…» de cabecera de una categoría (renombrar/reordenar/borrar). */
  function categoryMenu(cat: Category): MenuEntry[] {
    return [
      {
        key: 'rename',
        label: t('category.rename'),
        icon: <EditGlyph size={16} />,
        onSelect: () => void renameCategory(cat),
      },
      { key: 'up', label: t('category.moveUp'), onSelect: () => moveCategory(cat, -1) },
      { key: 'down', label: t('category.moveDown'), onSelect: () => moveCategory(cat, 1) },
      'separator',
      {
        key: 'delete',
        label: t('category.delete'),
        icon: <TrashGlyph size={16} />,
        danger: true,
        onSelect: () => void removeCategory(cat),
      },
    ];
  }

  /** Pide el resumen del canal activo. No cachea: lo interesante es lo de ahora. */
  async function ponmeAlDia(): Promise<void> {
    const canal = activeRef.current;
    if (!canal || resumiendo) return;
    setResumiendo(true);
    try {
      const res = await fetch('/api/resumen', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ channelId: canal }),
      });
      const j = (await res.json()) as { texto?: string; error?: string };
      setResumen(
        res.ok && j.texto
          ? j.texto
          : j.error === 'ai_not_configured'
            ? t('channel.summaryOff')
            : t('channel.summaryError'),
      );
    } catch {
      setResumen(t('channel.summaryError'));
    } finally {
      setResumiendo(false);
    }
  }

  /** Guarda el tema editado en la cabecera (admin; optimista + reversión). */
  function commitTopic(): void {
    setEditingTopic(false);
    if (!active) return;
    const next = topicDraft.trim();
    if (next === (active.topic ?? '')) return;
    const prevChannel = active;
    setChannels((prev) => prev.map((c) => (c.id === prevChannel.id ? { ...c, topic: next } : c)));
    updateChannel(prevChannel.id, { topic: next }).catch(() =>
      setChannels((prev) => prev.map((c) => (c.id === prevChannel.id ? prevChannel : c))),
    );
  }

  /** Entra en modo edición del tema del canal activo (admin). */
  function startEditTopic(): void {
    if (!active) return;
    setTopicDraft(active.topic ?? '');
    setEditingTopic(true);
  }

  /** Refresca el sidebar tras unirse a un canal desde «Explorar canales». */
  function handleJoined(channel: Channel): void {
    setChannels((prev) => (prev.some((c) => c.id === channel.id) ? prev : [...prev, channel]));
    realtimeRef.current?.subscribe(channel.id);
  }

  /** Añade al sidebar el canal recién creado, cierra el modal y ENTRA en él. */
  function handleChannelCreated(channel: Channel): void {
    setChannels((prev) => (prev.some((c) => c.id === channel.id) ? prev : [...prev, channel]));
    setActiveId(channel.id);
    setCreateOpen(false);
    realtimeRef.current?.subscribe(channel.id);
  }

  /** Abre/crea un DM desde «Nuevo mensaje» y lo selecciona en la vista. */
  function handleDmOpened(channel: Channel): void {
    setChannels((prev) =>
      prev.some((c) => c.id === channel.id)
        ? prev.map((c) => (c.id === channel.id ? channel : c))
        : [...prev, channel],
    );
    setActiveId(channel.id);
    setNewDmOpen(false);
    realtimeRef.current?.subscribe(channel.id);
  }

  /**
   * Ejecuta un comando de SERVIDOR: el BFF llama al servicio Go, que firma la petición
   * al tercero y devuelve su respuesta.
   *
   * Una respuesta `in_channel` YA se publicó (llega por WS como un mensaje más), así que
   * aquí solo se enseña la efímera —que no la ve nadie más— y los fallos. Enseñar las dos
   * duplicaría el texto en pantalla.
   */
  function runRemoteCommand(name: string, arg: string): void {
    void name;
    void arg;
    toast.notify({ tone: 'info', title: t('matrix.unsupported') });
  }

  /**
   * Despacha un slash command CON EFECTO desde el composer (Ola 14). El registro y
   * el parseo viven en lib/commands.ts; aquí están las dependencias reales
   * (presencia, canal activo, roster, paneles…). `/shrug`, `/me` y `/help` los
   * resuelve el propio composer; aquí llegan solo los de efecto. El nombre es
   * CANÓNICO (los alias ya se resolvieron: /dm→msg, /header→topic).
   */
  function runCommand(name: string, arg: string): void {
    switch (name) {
      case 'away':
        changeMyPresence('away', myStatus);
        break;
      case 'online':
        changeMyPresence('online', myStatus);
        break;
      case 'dnd':
        // Presencia del Hub: «No molestar» ≈ ocupado (busy).
        changeMyPresence('busy', myStatus);
        break;
      case 'status':
        changeMyPresence(myState, arg);
        break;
      case 'topic': {
        if (!activeId) break;
        const target = activeId;
        const topic = arg;
        const snapshot = channels;
        setChannels((prev) => prev.map((c) => (c.id === target ? { ...c, topic } : c)));
        updateChannel(target, { topic }).catch(() => {
          setChannels(snapshot);
          toast.notify({ tone: 'danger', title: t('command.topicFailed') });
        });
        break;
      }
      case 'purpose': {
        // El PROPÓSITO no es el tema: es «de qué va este canal», lo que se lee al
        // explorarlos. Mismo camino optimista que /topic.
        if (!activeId) break;
        const target = activeId;
        const purpose = arg;
        const snapshot = channels;
        setChannels((prev) => prev.map((c) => (c.id === target ? { ...c, purpose } : c)));
        updateChannel(target, { purpose }).catch(() => {
          setChannels(snapshot);
          toast.notify({ tone: 'danger', title: t('command.purposeFailed') });
        });
        break;
      }
      case 'msg': {
        const who = arg.replace(/^@/, '').trim();
        const sub = who ? membersByName.get(who) : undefined;
        if (!sub) {
          // La clave ya estaba escrita en los cinco idiomas y sin usar: aquí había un
          // literal en castellano, así que a un usuario en inglés «/msg @nadie» le
          // respondía en español.
          toast.notify({ tone: 'danger', title: t('command.notFound', { name: who || '…' }) });
          break;
        }
        openDirect([sub])
          .then(({ channel }) => handleDmOpened(channel))
          .catch(() => toast.notify({ tone: 'danger', title: t('command.dmFailed') }));
        break;
      }
      case 'search':
        setThreadRoot(null);
        setSearchSeed(arg);
        setRhsView('search');
        break;
      case 'leave': {
        const target = channels.find((c) => c.id === activeId);
        if (target) salirDelCanal(target);
        break;
      }
      case 'invite': {
        const who = arg.replace(/^@/, '').trim();
        const member = who ? members.find((m) => m.name === who) : undefined;
        if (!member) {
          toast.notify({ tone: 'danger', title: t('command.noSuchMember', { name: who || '…' }) });
          break;
        }
        inviteMember({ subject: member.sub, name: member.name })
          .then(() =>
            toast.notify({ tone: 'success', title: t('command.invited', { name: member.name }) }),
          )
          .catch(() => toast.notify({ tone: 'danger', title: t('command.inviteFailed') }));
        break;
      }
      // Simétrico de «invite»: la baja del TEAM, no la de un canal. Sin él, sacar a
      // alguien de un canal lo dejaba en el team (canales públicos + menciones).
      case 'remove': {
        const who = arg.replace(/^@/, '').trim();
        const member = who ? members.find((m) => m.name === who) : undefined;
        if (!member) {
          toast.notify({ tone: 'danger', title: t('command.noSuchMember', { name: who || '…' }) });
          break;
        }
        removeTeamMember(member.sub)
          .then(() => {
            setMembers((prev) => prev.filter((m) => m.sub !== member.sub));
            toast.notify({ tone: 'success', title: t('command.removed', { name: member.name }) });
          })
          .catch(() => toast.notify({ tone: 'danger', title: t('command.removeFailed') }));
        break;
      }
    }
  }

  /** Icono principal de una fila del sidebar: mención (@) > avatar (DM) > candado
   *  (privado) > #. Sin el candado un canal privado era indistinguible de un público.
   *  `enPanel`: el mismo icono sobre la superficie clara de un diálogo («Ir a un
   *  canal»), donde el tono del carril no se veía. */
  function channelLeading(c: Channel, enPanel = false): JSX.Element {
    const tenue = enPanel ? 'text-text-subtle' : 'text-sidebar-fg-muted';
    if (c.mentions) {
      return (
        <span
          role="img"
          aria-label={t('channel.mentions', { count: c.mentions })}
          title={t('channel.mentioned')}
          class="bg-danger grid h-6 w-6 place-items-center rounded-full text-xs leading-none font-bold text-white"
        >
          @
        </span>
      );
    }
    if (c.kind === 'direct') {
      // Group DM: avatar de grupo (sin presencia única). 1:1: punto de presencia del
      // peer sobre el avatar (accesible: Avatar añade aria-label/title con "En
      // línea"/"Ausente"/… — nunca solo color).
      if (isGroupDm(c)) return <GroupAvatar px={24} />;
      const sub = dmPeerSub(c);
      const p = sub ? presences[sub] : undefined;
      return (
        <Avatar name={c.name} size="xs" {...(p ? { presence: toPresenceStatus(p.state) } : {})} />
      );
    }
    if (c.kind === 'private') {
      // No solo el glifo: se anuncia con palabras (mismo patrón que la mención).
      return (
        <span
          role="img"
          aria-label={t('channel.private')}
          title={t('channel.private')}
          class={`${tenue} grid h-6 w-6 place-items-center`}
        >
          <Icon name="lock" size={14} />
        </span>
      );
    }
    return (
      <span class={`${tenue} text-md leading-none`} aria-hidden="true">
        #
      </span>
    );
  }

  /**
   * Acciones del canal ACTIVO para el menú de su cabecera. Las que ya vivían en el «…»
   * de la fila del sidebar siguen ahí; esto añade las que no tenían puerta en ninguna
   * parte —miembros, fijados, integraciones, tema— al sitio donde Slack, Teams y
   * Mattermost las ponen: pulsar el nombre del canal. Hasta ahora ese nombre era un
   * <h1> inerte, así que funciones ya construidas parecían no existir.
   */
  function channelHeaderMenuItems(c: Channel): MenuEntry[] {
    if (c.kind === 'direct') {
      return [
        {
          key: 'delete-dm',
          label: t('dm.delete'),
          icon: <TrashGlyph size={16} />,
          danger: true,
          onSelect: () => void pedirBorrar(c),
        },
      ];
    }
    const items: MenuEntry[] = [
      {
        key: 'members',
        label: t('channel.members'),
        icon: <Icon name="users" size={16} />,
        onSelect: toggleMembersPanel,
      },
      {
        key: 'pinned',
        label: t('panel.pinned'),
        onSelect: togglePinnedPanel,
      },
      // Guardados y Programados viven también aquí porque en <768px sus botones
      // se retiran de la cabecera: seis iconos junto al nombre del canal se
      // salían 98px de un iPhone SE. Quien navega desde el móvil los encuentra
      // en este menú, no se pierde ninguna de las dos vistas.
      {
        key: 'saved',
        label: t('channel.saved'),
        icon: <BookmarkGlyph size={16} />,
        onSelect: toggleSavedPanel,
      },
      {
        key: 'scheduled',
        label: t('channel.scheduledMessages'),
        icon: <ClockGlyph size={16} />,
        onSelect: toggleScheduledPanel,
      },
      {
        key: 'notify',
        label: t('notify.openSettings'),
        icon: <Icon name="bell" size={16} />,
        onSelect: () => setNotifySettingsOpen(true),
      },
    ];
    items.push({
      key: 'blocked',
      label: t('block.list'),
      onSelect: () => setBlockedOpen(true),
    });
    // La cola de denuncias: para quien administra un equipo o la organización. A los
    // demás el servicio les devolvería una cola vacía; no se les enseña la puerta.
    if (canManageIntegrations || user.roles.some((r) => r === 'owner' || r === 'admin')) {
      items.push({
        key: 'reports',
        label: t('moderation.title'),
        icon: <Icon name="warning" size={16} />,
        onSelect: () => setReportsOpen(true),
      });
    }
    // Integraciones: hasta ahora su ÚNICA puerta era el menú de presencia
    // («En línea / Ausente»), que es donde nadie busca un webhook o un bot.
    // La condición es canManageIntegrations y NO isTeamAdmin: la API exige ser admin de
    // ALGÚN team y esto lo abre el mismo modal que la entrada del sidebar. Con dos
    // criterios distintos para la misma acción, a un admin de un segundo team le salía
    // por una puerta y no por la otra.
    if (canManageIntegrations) {
      items.push({
        key: 'integrations',
        label: t('integrations.open'),
        icon: <Icon name="code" size={16} />,
        onSelect: () => setIntegrationsOpen(true),
      });
    }
    if (isTeamAdmin && !c.external) {
      items.push('separator', {
        key: 'topic',
        label: t('channel.editTopic'),
        onSelect: startEditTopic,
      });
    }
    // Salir y borrar, también aquí: en un teléfono el nombre del canal es el camino
    // a todo lo del canal (la fila del sidebar queda detrás del cajón).
    items.push('separator', {
      key: 'leave',
      label: t('channel.leave'),
      onSelect: () => void pedirSalir(c),
    });
    if (puedeBorrar(c)) {
      items.push({
        key: 'delete',
        label: t('channel.delete'),
        icon: <TrashGlyph size={16} />,
        danger: true,
        onSelect: () => void pedirBorrar(c),
      });
    }
    return items;
  }

  /** Fila de canal/DM con sus acciones (favorito, nivel de notificación y —admin— archivar). */
  function renderChannelRow(c: Channel): JSX.Element {
    // Nivel de notificación (Ola 13): el activo lleva un check; los demás, un hueco del
    // mismo ancho para alinear la columna de icono (WCAG: la elección no es solo color).
    const level = c.notifyLevel ?? 'all';
    const levelItem = (lvl: 'all' | 'mentions' | 'none', label: string): MenuEntry => ({
      key: `notify-${lvl}`,
      label,
      icon:
        level === lvl ? (
          <Icon name="check" size={16} />
        ) : (
          <span aria-hidden="true" style={{ display: 'inline-block', width: 16, height: 16 }} />
        ),
      onSelect: () => setNotifyLevel(c, lvl),
    });
    const items: MenuEntry[] = [
      {
        key: 'fav',
        label: c.favorite ? t('channel.unfavourite') : t('channel.favourite'),
        icon: <StarGlyph size={16} filled={c.favorite ?? false} />,
        onSelect: () => toggleFavorite(c),
      },
      'separator',
      levelItem('all', t('notify.levelAll')),
      levelItem('mentions', t('notify.levelMentions')),
      levelItem('none', t('notify.levelNone')),
    ];
    // «Mover a categoría…» — camino ACCESIBLE (teclado) de mover el canal, alternativa al
    // arrastrar-soltar (WCAG AA: el drag no es el único camino). Lista las categorías del
    // usuario (salvo la actual) + «Quitar de la categoría» si está en una personalizada.
    const orderedCats = [...categories].sort(
      (a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name),
    );
    const moveTargets = orderedCats.filter((cat) => cat.id !== c.categoryId);
    // Las categorías son de la organización propia: una conversación de otra no se
    // puede colocar en ellas (ADR 0040).
    if (!c.external && (moveTargets.length > 0 || c.categoryId)) {
      items.push('separator');
      for (const cat of moveTargets) {
        items.push({
          key: `move-${cat.id}`,
          label: t('channel.moveTo', { name: cat.name }),
          onSelect: () => moveChannelToCategory(c.id, cat.id),
        });
      }
      if (c.categoryId) {
        items.push({
          key: 'move-default',
          label: t('channel.removeFromCategory'),
          onSelect: () => moveChannelToCategory(c.id, null),
        });
      }
    }
    // Lo que saca la conversación de tu barra, al final y separado: salir, archivar y
    // borrar en un canal; «Borrar chat» (solo para ti) en un directo.
    const salida: MenuEntry[] = [];
    if (c.kind === 'direct') {
      salida.push({
        key: 'delete-dm',
        label: t('dm.delete'),
        icon: <TrashGlyph size={16} />,
        danger: true,
        onSelect: () => void pedirBorrar(c),
      });
    } else {
      salida.push({ key: 'leave', label: t('channel.leave'), onSelect: () => void pedirSalir(c) });
      if (isTeamAdmin && !c.external) {
        salida.push({
          key: 'archive',
          label: t('channel.archive'),
          icon: <ArchiveGlyph size={16} />,
          danger: true,
          onSelect: () => setArchivando(c),
        });
      }
      if (puedeBorrar(c)) {
        salida.push({
          key: 'delete',
          label: t('channel.delete'),
          icon: <TrashGlyph size={16} />,
          danger: true,
          onSelect: () => void pedirBorrar(c),
        });
      }
    }
    items.push('separator', ...salida);
    return (
      <ChannelRow
        key={c.id}
        leading={channelLeading(c)}
        label={c.name}
        active={activeId === c.id}
        unread={c.unread ?? 0}
        muted={c.muted ?? false}
        favorite={c.favorite ?? false}
        inCall={llamadasVivas.has(c.id)}
        external={c.external ?? false}
        onSelect={() => setActiveId(c.id)}
        onToggleFavorite={() => toggleFavorite(c)}
        menuItems={items}
        menuLabel={t('channel.actions', { name: c.name })}
        t={t}
        draggable={!c.external}
        onDragStart={() => setDrag({ kind: 'channel', id: c.id })}
        onDragEnd={() => setDrag(null)}
      />
    );
  }

  // El timbre vive atado al aviso, no a cada sitio que lo apaga: contestar,
  // rechazar, que cuelguen y que se agote son cuatro caminos distintos, y con el
  // efecto solo hay uno que pararlo. Solo suena en un DIRECTO —una llamada de
  // canal se ve, pero no interrumpe a los treinta que están en él—.
  useEffect(() => {
    if (!entrante?.direct) return;
    return sonarTimbre();
  }, [entrante]);

  /** Contesta la llamada entrante: abre esa conversación y entra. */
  function contestarEntrante(): void {
    const id = entrante?.channelId;
    setEntrante(null);
    if (!id) return;
    const canal = channelsRef.current.find((c) => c.id === id);
    setActiveId(id);
    if (canal) startCallFor(canal);
  }

  /** Abre/cierra el panel de Fijados del canal (excluyente con el hilo). */
  /** «Archivos del canal»: los adjuntos de este canal, de más nuevo a más viejo.
   *
   *  No hay panel propio ni consulta nueva: la búsqueda YA sabe filtrar por canal
   *  (`in:`) y por adjunto (`has:file`), así que esto siembra esa consulta y abre
   *  el panel de resultados. Un panel aparte sería otra lista que mantener, con su
   *  paginación y su estado vacío, para enseñar exactamente lo mismo.
   *
   *  ponytail: sin galería de miniaturas —eso pide previsualización y rejilla; se
   *  añade si alguien la pide de verdad. */
  function openChannelFiles(): void {
    const canal = channels.find((c) => c.id === activeId);
    if (!canal) return;
    setThreadRoot(null);
    // `in:` casa contra el NOMBRE del canal, que es lo que el parser entiende.
    setSearchSeed(`in:#${canal.name} has:file `);
    setRhsView('search');
  }

  function togglePinnedPanel(): void {
    setThreadRoot(null);
    setRhsView((v) => (v === 'pinned' ? null : 'pinned'));
  }

  /** Abre/cierra el panel «Miembros» (excluyente con el hilo). El roster ya está
   *  cargado por el canal activo: abrir no vuelve a pedirlo. */
  function toggleMembersPanel(): void {
    setThreadRoot(null);
    setRhsView((v) => (v === 'members' ? null : 'members'));
  }

  // Acciones del roster. Aplican la respuesta del servicio (que trae el miembro
  // exacto) en vez de fiarse del eco por WS; el frame que vuelve es un upsert
  // idempotente. Devuelven la promesa para que el panel pinte su error/ocupado.

  async function addMemberToChannel(m: ChannelMember): Promise<void> {
    if (!activeId) throw new Error('sin canal activo');
    const chId = activeId;
    const { member } = await addChannelMember(chId, m.sub, m.name);
    // De fuera: es una invitación, no entra en el roster hasta que la acepte.
    if (member.pending) return;
    if (activeRef.current === chId) setRoster((prev) => upsertMember(prev, member));
  }

  async function removeMemberFromChannel(m: ChannelMember): Promise<void> {
    if (!activeId) throw new Error('sin canal activo');
    const chId = activeId;
    await removeChannelMember(chId, m.sub);
    if (activeRef.current === chId) setRoster((prev) => prev.filter((x) => x.sub !== m.sub));
  }

  async function changeMemberRole(m: ChannelMember, role: 'admin' | 'member'): Promise<void> {
    if (!activeId) throw new Error('sin canal activo');
    const chId = activeId;
    const { member } = await setChannelMemberRole(chId, m.sub, role);
    if (activeRef.current === chId) setRoster((prev) => upsertMember(prev, member));
  }

  /** Abre/cierra el panel de Búsqueda global (excluyente con el hilo). Abrir desde el
   *  botón parte de query vacía (la semilla es solo del slash command «/search»). */
  function toggleSearchPanel(): void {
    setThreadRoot(null);
    setSearchSeed('');
    setRhsView((v) => (v === 'search' ? null : 'search'));
  }

  /** Carga la primera página de mis guardados e hidrata savedIds. */
  function loadSaved(): void {
    setSavedLoading(true);
    setSavedError(false);
    listSaved()
      .then(({ messages: list, nextBefore }) => {
        setSavedList(list);
        setSavedNext(nextBefore);
        setSavedIds((prev) => new Set([...prev, ...list.map((m) => m.id)]));
      })
      .catch(() => setSavedError(true))
      .finally(() => setSavedLoading(false));
  }

  /** Página siguiente de guardados (keyset por nextBefore). */
  function loadMoreSaved(): void {
    if (!savedNext || savedLoadingMore) return;
    setSavedLoadingMore(true);
    listSaved(savedNext)
      .then(({ messages: list, nextBefore }) => {
        setSavedList((prev) => [...prev, ...list.filter((m) => !prev.some((p) => p.id === m.id))]);
        setSavedNext(nextBefore);
        setSavedIds((prev) => new Set([...prev, ...list.map((m) => m.id)]));
      })
      .catch(() => undefined)
      .finally(() => setSavedLoadingMore(false));
  }

  /** Abre/cierra el panel de Guardados (excluyente con el hilo). */
  function toggleSavedPanel(): void {
    setThreadRoot(null);
    if (rhsView === 'saved') {
      setRhsView(null);
      return;
    }
    setRhsView('saved');
    loadSaved();
  }

  /** Carga la primera página de mis hilos seguidos (vista global «Hilos»). */
  function loadThreads(): void {
    setThreadsLoading(true);
    setThreadsError(false);
    listThreads()
      .then(({ threads, nextBefore }) => {
        setThreadsList(threads);
        setThreadsNext(nextBefore);
      })
      .catch(() => setThreadsError(true))
      .finally(() => setThreadsLoading(false));
  }

  /** Página siguiente de hilos seguidos (keyset por última actividad). */
  function loadMoreThreads(): void {
    if (!threadsNext || threadsLoadingMore) return;
    setThreadsLoadingMore(true);
    listThreads(threadsNext)
      .then(({ threads, nextBefore }) => {
        setThreadsList((prev) => [
          ...prev,
          ...threads.filter((th) => !prev.some((p) => p.root.id === th.root.id)),
        ]);
        setThreadsNext(nextBefore);
      })
      .catch(() => undefined)
      .finally(() => setThreadsLoadingMore(false));
  }

  /** Abre/cierra el panel «Hilos» (excluyente con el hilo abierto). */
  function toggleThreadsPanel(): void {
    setThreadRoot(null);
    if (rhsView === 'threads') {
      setRhsView(null);
      return;
    }
    setRhsView('threads');
    loadThreads();
  }

  // --- Ola 12: mensajes programados ---

  /** Carga mis mensajes programados (cross-canal). */
  function loadScheduled(): void {
    setScheduledLoading(true);
    setScheduledError(false);
    listScheduled()
      .then(({ scheduled }) => setScheduledList(scheduled))
      .catch(() => setScheduledError(true))
      .finally(() => setScheduledLoading(false));
  }

  /** Abre/cierra el panel «Programados» (excluyente con el hilo abierto). */
  function toggleScheduledPanel(): void {
    setThreadRoot(null);
    if (rhsView === 'scheduled') {
      setRhsView(null);
      return;
    }
    setRhsView('scheduled');
    loadScheduled();
  }

  /** Cancela un programado (optimista; reconcilia recargando si falla). */
  function cancelScheduledMsg(m: ScheduledMessage): void {
    setScheduledList((prev) => prev.filter((s) => s.id !== m.id));
    cancelScheduled(m.id).catch(() => loadScheduled());
  }

  /** «Enviar ahora»: cancela el programado y publica el mensaje al momento
   *  (reusa el envío normal). Si el canal está abierto llega por WS; si no, estará
   *  allí al visitarlo. */
  function sendScheduledNow(m: ScheduledMessage): void {
    setScheduledList((prev) => prev.filter((s) => s.id !== m.id));
    cancelScheduled(m.id)
      .then(() =>
        sendMessage({
          channelId: m.channelId,
          body: m.body,
          clientId: newClientId(),
          ...(m.fileIds?.length ? { fileIds: m.fileIds } : {}),
        }),
      )
      .then(() => toast.notify({ tone: 'success', title: t('message.sent') }))
      .catch(() => {
        toast.notify({ tone: 'danger', title: t('message.sendFailed') });
        loadScheduled();
      });
  }

  /** Reprograma un programado a un nuevo instante (optimista; reconcilia si falla). */
  function rescheduleMsg(m: ScheduledMessage, sendAtISO: string): void {
    setScheduledList((prev) =>
      prev
        .map((s) => (s.id === m.id ? { ...s, sendAt: sendAtISO } : s))
        .sort((a, b) => (a.sendAt < b.sendAt ? -1 : 1)),
    );
    updateScheduled(m.id, { sendAt: sendAtISO }).catch(() => loadScheduled());
  }

  /** Inicia (o reutiliza) la llamada de un canal concreto y abre la sala de vídeo. La
   *  decisión de reusar/crear la sala vive en el servicio Go; aquí solo se pide. */
  function startCallFor(channel: Channel): void {
    if (callBusy) return;
    setCallBusy(true);
    setCallError(false);
    // Solo se limpia si veníamos de OTRA puerta: el reintento automático pasa por
    // aquí cada ocho segundos, y borrarlo haría desaparecer el «esperando» y
    // reaparecer el botón de llamar en cada vuelta.
    setPuertaCerrada((prev) => (prev && prev.channelId === channel.id ? prev : null));
    startCall(channel.id, channel.name)
      .then((c) => {
        setCall(c);
        setCallChannelId(channel.id);
        // Anota la llamada en el timeline (mensaje de sistema type=call). Fire-and-
        // forget: la tarjeta llega a todos (incluido este cliente) por WebSocket, así
        // que no insertamos nada optimista; si el post falla, la llamada sigue.
        // ponytail: se publica en CADA inicio/unión (también al pulsar «Unirse»), así
        // que varias personas pueden generar tarjetas duplicadas. Un webhook meet→chat
        // (una tarjeta por sala + «finalizada»/duración automáticas, y cubriendo las
        // llamadas iniciadas por otro cliente) es el follow-up robusto; hoy lo publica
        // el cliente que inicia. Ver docs de la ola.
        void postCallMessage(channel.id).catch(() => undefined);
      })
      .catch((err: unknown) => {
        // «Bloqueada» tiene respuesta —llamar a la puerta— y «no se pudo» no. Un
        // único mensaje de error para las dos cosas deja a la gente recargando.
        if (err instanceof LlamadaError && err.code === 'room_locked') {
          // Si ya se había llamado, se conserva: esto es un reintento, no una
          // llamada nueva, y volver a poner `llamado: false` reabriría el botón.
          setPuertaCerrada((prev) =>
            prev?.channelId === channel.id ? prev : { channelId: channel.id, llamado: false },
          );
        } else {
          setCallError(true);
        }
      })
      .finally(() => setCallBusy(false));
  }

  // Mientras se espera en la puerta, se reintenta entrar solo. Sin esto, el
  // anfitrión abre y al otro lado no pasa nada: la persona se queda mirando
  // «esperando» hasta que se le ocurre volver a pulsar «Llamar», que es la mitad
  // de la función sin hacer. El reintento es el propio join —la señal de que te
  // han abierto es que entras— y se rinde a los cinco minutos para no dejar un
  // temporizador latiendo en una pestaña olvidada.
  useEffect(() => {
    if (!puertaCerrada?.llamado) return;
    const ch = puertaCerrada.channelId;
    let vivo = true;
    let intentos = 0;
    const id = window.setInterval(() => {
      if (!vivo) return;
      if (++intentos > 38) {
        window.clearInterval(id);
        return;
      }
      const canal = channelsRef.current.find((c) => c.id === ch);
      if (canal) startCallForRef.current(canal);
    }, 8000);
    return () => {
      vivo = false;
      window.clearInterval(id);
    };
  }, [puertaCerrada]);

  /** Llama a la puerta de la sala cerrada del canal. */
  function llamarALaPuerta(): void {
    const ch = puertaCerrada?.channelId;
    if (!ch) return;
    setPuertaCerrada({ channelId: ch, llamado: true });
    // Best-effort declarado: si el aviso no sale, el botón vuelve a estar
    // disponible en vez de dejar a alguien esperando una respuesta que nadie
    // llegó a pedir.
    void knockChannelCall(ch).catch(() => setPuertaCerrada({ channelId: ch, llamado: false }));
  }

  // ---------------------------------------------------------------------------
  // Reuniones sueltas: convocar, entrar por enlace, moderar. La sala NO cuelga de un
  // canal, así que aquí no se publica ninguna tarjeta en el timeline (no hay dónde).
  // ---------------------------------------------------------------------------

  function loadMeetings(): void {
    setMeetingsLoading(true);
    setMeetingsError(false);
    listMeetings('active')
      .then((r) => setMeetings(r.meetings))
      .catch(() => setMeetingsError(true))
      .finally(() => setMeetingsLoading(false));
  }

  /** Abre (o cierra) la vista «Borradores». No pide nada: la lista es local. */
  function toggleDraftsPanel(): void {
    setRhsView((v) => (v === 'drafts' ? null : 'drafts'));
  }

  /** Lleva al sitio donde se estaba redactando: el canal, o el hilo si era de uno.
   *  El composer restaura el texto solo (lee su borrador al montarse). */
  function goToDraft(d: LocalDraft): void {
    setRhsView(null);
    if (!d.parentId) {
      closeThread();
      if (activeRef.current !== d.channelId) setActiveId(d.channelId);
      return;
    }
    // ponytail: openThread necesita el mensaje raíz para pintarlo, y del borrador
    // solo se conoce su id; si no está en la ventana cargada se pide. El coste es una
    // lectura extra en ese caso, a cambio de no duplicar el flujo de apertura.
    if (activeRef.current !== d.channelId) setActiveId(d.channelId);
    const cargado = messages.find((m) => m.id === d.parentId);
    if (cargado) {
      openThread(cargado);
      return;
    }
    getThread(d.parentId)
      .then(({ root }) => openThread(root))
      .catch(() => toast.notify({ tone: 'danger', title: t('thread.loadFailed') }));
  }

  /** Descarta un borrador desde la vista (no toca el canal abierto). */
  function discardDraft(d: LocalDraft): void {
    clearDraft(draftScopeId(user.id, d));
  }

  /** Abre el panel de reuniones y trae la lista (una vez por apertura). */
  function openMeetings(): void {
    if (rhsView === 'meetings') {
      setRhsView(null);
      return;
    }
    setRhsView('meetings');
    loadMeetings();
  }

  /** Convoca y entra: una sola acción, como «Nueva reunión» de Meet. Con invitados
   *  —también de otras organizaciones, ADR 0040— se llama en la conversación con ellos. */
  async function onCreateMeeting(title: string, people: Persona[]): Promise<void> {
    if (people.length > 0) {
      try {
        const { channel } = await openDirect(people.map((p) => p.sub));
        handleDmOpened(channel);
        setRhsView(null);
        startCallFor(channel);
      } catch {
        toast.notify({ tone: 'danger', title: t('meetings.createFailed') });
      }
      return;
    }
    try {
      const created = await createMeeting(title);
      // La sala ocupa la pantalla: dejar el panel abierto detrás hace que el
      // siguiente clic en «Reuniones» lo CIERRE en vez de volver a la lista.
      setRhsView(null);
      // El objeto entero, no campo a campo: `createMeeting` ya devuelve todo lo
      // que la sala necesita —incluidos el id y el anfitrión, que hacen falta
      // para moderar— y copiarlo a mano se queda corto en cuanto crece.
      setCall(created);
      setMeetings((prev) => [created.meeting, ...prev.filter((m) => m.id !== created.meeting.id)]);
    } catch {
      // Por toast y no por `callError`: ese aviso vive en la cabecera del CANAL, que
      // con el panel de reuniones abierto está oculta en móvil y no existe si aún no
      // hay ningún canal — y nadie lo apagaba nunca, así que se quedaba clavado.
      toast.notify({ tone: 'danger', title: t('meetings.createFailed') });
    }
  }

  /** Entra en una reunión existente (de la lista o pegando su enlace, con su clave). */
  async function onJoinMeeting(id: string, clave = ''): Promise<void> {
    try {
      const c = await joinMeeting(id, clave);
      setRhsView(null);
      setCall(c);
    } catch (err: unknown) {
      // Una sala con la puerta echada NO es una avería: tiene respuesta, y llamar a
      // ella estaba cableado de punta a punta (`knockMeeting` → BFF → servicio) sin
      // que ninguna pantalla lo llamara. Quien pegaba el enlace de una reunión
      // bloqueada solo veía «no se pudo iniciar la llamada» y se quedaba fuera.
      if (err instanceof LlamadaError && err.code === 'room_locked') {
        toast.notify({
          tone: 'info',
          title: t('meet.roomLockedNotice'),
          action: {
            label: t('meet.knock'),
            onClick: () => {
              void knockMeeting(id, clave)
                .then(() => toast.notify({ tone: 'success', title: t('meet.knocked') }))
                .catch(() => toast.notify({ tone: 'danger', title: t('meetings.joinFailed') }));
            },
          },
        });
        return;
      }
      // Una reunión suelta solo admite a quien la convocó, a quien ya estuvo dentro y a
      // quien trae su enlace: un enlace viejo, sin clave, llega aquí.
      if (err instanceof LlamadaError && err.status === 403) {
        toast.notify({ tone: 'danger', title: t('meetings.needLink') });
        return;
      }
      toast.notify({ tone: 'danger', title: t('meetings.joinFailed') });
    }
  }

  /** Detalle plegable: quién está y qué se grabó. Volver a pulsar lo cierra. */
  function onOpenMeetingDetail(id: string): void {
    if (meetingDetailFor === id) {
      setMeetingDetailFor(null);
      return;
    }
    setMeetingDetailFor(id);
    setMeetingDetailLoading(true);
    meetingDetail(id)
      .then((d) => {
        setMeetingParticipants(d.participants);
        setMeetingRecordings(d.recordings);
      })
      .catch(() => {
        setMeetingParticipants([]);
        setMeetingRecordings([]);
      })
      .finally(() => setMeetingDetailLoading(false));
  }

  async function onCloseMeeting(id: string): Promise<void> {
    const ok = await confirmar({
      mensaje: t('meetings.closeConfirm'),
      confirmLabel: t('meetings.close'),
      tone: 'danger',
    });
    if (!ok) return;
    closeMeeting(id)
      .then(() => setMeetings((prev) => prev.filter((m) => m.id !== id)))
      .catch(() => setMeetingsError(true));
  }

  async function onRemoveMeetingParticipant(
    id: string,
    identity: string,
    name: string,
  ): Promise<void> {
    const ok = await confirmar({
      mensaje: t('meetings.removeConfirm', { name }),
      confirmLabel: t('meetings.remove'),
      tone: 'danger',
    });
    if (!ok) return;
    removeMeetingParticipant(id, identity)
      .then(() => setMeetingParticipants((prev) => prev.filter((p) => p.identity !== identity)))
      .catch(() => undefined);
  }

  // --- La barra de direcciones converge a la canónica (ADR 0056) -----------
  // La vuelta del login aterriza en la forma que el middleware sirve POR DENTRO
  // (`/rooms/channels/x?__org=acme`): sirve, pero la marca `__org` es plomería
  // del middleware y no debe quedarse en la barra —ni copiarse dentro de un
  // enlace—. Aquí se reescribe UNA vez a la forma con org (`/rooms/acme/...`),
  // conservando query y hash, y a partir de ahí todo el pushState de abajo ya
  // compone canónico.
  useEffect(() => {
    const marca = new URLSearchParams(window.location.search).get(MARCA_ORG);
    if (!marca || !RE_SLUG_ORG.test(marca)) return;
    const base = import.meta.env.BASE_URL.replace(/\/+$/, '');
    if (!window.location.pathname.startsWith(`${base}/`)) return;
    const query = new URLSearchParams(window.location.search);
    query.delete(MARCA_ORG);
    const qs = query.toString();
    window.history.replaceState(
      null,
      '',
      rutaConOrg(base, marca, window.location.pathname.slice(base.length) || '/') +
        (qs ? `?${qs}` : '') +
        window.location.hash,
    );
  }, []);

  // Deep-link de reunión: `…/?rooms=<roomId>` (`enlaceReunion`) es lo que se copia con
  // «Copiar el enlace» y lo que se manda por correo o chat para convocar. Entra UNA vez
  // y limpia el parámetro. No depende de los canales: quien recibe el enlace puede no
  // tener ninguno. El parámetro antiguo, `meet`, lo redirige a este el servidor (`index.astro`).
  // La reunión del enlace, abierta en la app o a la espera de abrirla (AbrirEnLaApp).
  const [enLaApp, setEnLaApp] = useState<{ id: string; clave: string; abierta: boolean } | null>(
    null,
  );
  const meetDeepLinkHandled = useRef(false);
  // El efecto entra UNA vez; la llamada, con la versión más reciente del manejador.
  const onJoinMeetingRef = useUltimo(onJoinMeeting);
  useEffect(() => {
    if (meetDeepLinkHandled.current) return;
    const params = new URLSearchParams(window.location.search);
    const meetId = params.get('rooms');
    if (!meetId) return;
    meetDeepLinkHandled.current = true;
    // La clave del enlace (reunión suelta): abre la puerta a quien no la convocó.
    const clave = params.get('clave') ?? '';
    params.delete('rooms');
    params.delete('clave');
    const qs = params.toString();
    window.history.replaceState(
      null,
      '',
      window.location.pathname + (qs ? `?${qs}` : '') + window.location.hash,
    );
    // Primero, la APP: con Rooms instalada, el enlace entra en la llamada nativa
    // (lib/enLaApp.ts). Si no se la llevó, se entra aquí como siempre, sin aviso.
    const accion = queHacer();
    if (accion === 'ofrecer') {
      setEnLaApp({ id: meetId, clave, abierta: false });
      return;
    }
    if (accion === 'nada') {
      void onJoinMeetingRef.current(meetId, clave);
      return;
    }
    void abrirReunionEnLaApp(meetId, clave).then((abierta) => {
      if (abierta) setEnLaApp({ id: meetId, clave, abierta: true });
      else void onJoinMeetingRef.current(meetId, clave);
    });
  }, [onJoinMeetingRef]);

  /** Llama al canal activo (botón de la cabecera). */
  function startChannelCall(): void {
    if (active) startCallFor(active);
  }

  // Deep-link de Sytue Rooms: el intent `call-channel` del Hub compone
  // `…/?call=<channelId>`. Una vez cargados los canales, auto-inicia la videollamada
  // de ese canal UNA sola vez y limpia el parámetro (para no re-arrancar en refresh o
  // re-render). Si el canal no está en la lista del usuario, degrada en silencio.
  const callDeepLinkHandled = useRef(false);
  const startCallForRef = useRef(startCallFor);
  startCallForRef.current = startCallFor;
  useEffect(() => {
    if (callDeepLinkHandled.current || loadingChannels || channels.length === 0) return;
    callDeepLinkHandled.current = true;
    const params = new URLSearchParams(window.location.search);
    const callId = params.get('call');
    if (!callId) return;
    // Retira `call` conservando el resto de la query y el hash (no re-dispara).
    params.delete('call');
    const qs = params.toString();
    window.history.replaceState(
      null,
      '',
      window.location.pathname + (qs ? `?${qs}` : '') + window.location.hash,
    );
    const target = channels.find((c) => c.id === callId);
    if (!target) return; // canal ajeno al usuario: no arranca (silencioso)
    setActiveId(target.id);
    startCallForRef.current(target);
  }, [loadingChannels, channels]);

  // Deep-link a un MENSAJE (Ola 16): `…/channels/<id>?message=<messageId>` (formato de
  // «Copiar enlace» y del deep-link del Hub). Una vez cargados los canales, salta al
  // mensaje UNA sola vez (ventana centrada + resalte) y limpia el parámetro. El canal
  // sale de la ruta `/channels/:id` o del parámetro `?channel=`. Canal ajeno ⇒ silencio.
  const messageDeepLinkHandled = useRef(false);
  const goToMessageRef = useRef(goToMessage);
  goToMessageRef.current = goToMessage;
  useEffect(() => {
    if (messageDeepLinkHandled.current || loadingChannels || channels.length === 0) return;
    const params = new URLSearchParams(window.location.search);
    const messageId = params.get('message');
    if (!messageId) return;
    messageDeepLinkHandled.current = true;
    const fromPath = window.location.pathname.match(/\/channels\/([^/?#]+)/);
    const channelId = fromPath ? decodeURIComponent(fromPath[1]!) : params.get('channel');
    // Retira `message` (y `channel`) conservando el resto de la query y el hash.
    params.delete('message');
    params.delete('channel');
    const qs = params.toString();
    window.history.replaceState(
      null,
      '',
      window.location.pathname + (qs ? `?${qs}` : '') + window.location.hash,
    );
    if (!channelId || !channels.some((c) => c.id === channelId)) return; // canal ajeno
    goToMessageRef.current(channelId, messageId);
  }, [loadingChannels, channels]);

  // --- El canal abierto TIENE dirección propia -----------------------------
  // Hasta ahora `setActiveId` movía el canal solo en memoria: la barra de
  // direcciones se quedaba en `/` con `#General` abierto. Medido en producción
  // el 2026-08-10. Eso deja sin compartir un canal, sin abrirlo en otra pestaña,
  // sin «atrás» del navegador y perdiendo el sitio en cada recarga — y la ruta
  // `/channels/[channelId]` ya existía, solo que únicamente la usaban los
  // enlaces ENTRANTES.
  //
  // `pushState` y no `replaceState`: con replace no hay historial que recorrer y
  // el botón «atrás» se sale de Rooms en vez de volver al canal anterior. El
  // `popstate` cierra el círculo al navegar hacia atrás.
  useEffect(() => {
    if (!activeId) return;
    // CON el prefijo de la app: bajo el host de la organización Rooms vive en
    // `/rooms`, y `/channels/<id>` a pelo es un 404 al recargar o al pegar el
    // enlace en otra pestaña (medido el 2026-09-09: la barra decía
    // `corporate.sytue.eu/channels/…` y recargar daba «404 page not found»).
    // Y CON la organización (ADR 0056): la canónica la lleva en el path, así
    // que el enlace a un canal se comparte ya en forma canónica. La org es la
    // de la URL vista (`orgActual`, que entiende también la marca del rewrite):
    // quien abrió un enlace de otra organización sigue donde está en vez de
    // mandar la barra a la org de su sesión. Y SIN org en la sesión (alta a
    // medias) no hay segmento que conservar: la forma sin org, que el
    // middleware deja pasar.
    const destino = rutaConOrg(
      import.meta.env.BASE_URL,
      user.tenant ? orgActual(import.meta.env.BASE_URL) : null,
      `/channels/${encodeURIComponent(activeId)}`,
    );
    if (window.location.pathname === destino) return;
    window.history.pushState({ canal: activeId }, '', destino + window.location.search);
  }, [activeId, user.tenant]);

  useEffect(() => {
    const alVolver = (): void => {
      const m = window.location.pathname.match(/\/channels\/([^/?#]+)/);
      const id = m ? decodeURIComponent(m[1]!) : null;
      // Sin canal en la ruta (se volvió a `/`) no se fuerza nada: dejar el que
      // haya evita un parpadeo al canal por defecto.
      if (id) setActiveId(id);
    };
    window.addEventListener('popstate', alVolver);
    return () => window.removeEventListener('popstate', alVolver);
  }, []);

  // Limpia el temporizador del resalte al desmontar (evita fugas si se cierra Rooms).
  useEffect(
    () => () => {
      if (highlightTimer.current) clearTimeout(highlightTimer.current);
    },
    [],
  );

  // --- Ola 18: sidebar como drawer en móvil (<768px) ---
  // En ≥768px la sidebar va en flujo, como siempre; en <768px es un drawer sobre overlay
  // (hamburguesa en la cabecera del canal). La mecánica —foco, trampa, Esc y cierre al
  // ensanchar— es la de @sytue/ui, la misma que AdminShell y AppShell.
  const [drawerOpen, setDrawerOpen] = useState(false);
  const drawerId = useId();
  const menuBtnId = useId();
  useNavDrawer(drawerOpen, drawerId, menuBtnId, () => setDrawerOpen(false));

  // Cambiar de canal cierra el drawer: en móvil el canal pasa a pantalla completa.
  useEffect(() => {
    setDrawerOpen(false);
  }, [activeId]);

  // Ola 11: agrupación del sidebar en secciones (Favoritos/Canales/DMs por defecto +
  // categorías personalizadas). Un canal en una categoría personalizada conocida va solo
  // en ella; si no, cae a su sección por defecto según favorito/kind. Ver lib/categories.
  const groups = groupChannels(channels, categories);

  // Estado de conexion en tiempo real: nunca solo color (WCAG AA) — el punto se
  // acompana de texto; con la conexion sana el texto queda para lectores de
  // pantalla (sr-only) y solo aflora cuando algo va mal.
  const statusLabel =
    status === 'open'
      ? t('connection.online')
      : status === 'connecting'
        ? t('connection.connecting')
        : t('connection.offline');

  // «Marcar todo como leído»: contadores a cero en el acto y una llamada por canal
  // con algo pendiente. Si alguna falla, el siguiente sondeo repone su contador.
  const marcarTodoLeido = useCallback(() => {
    const pendientes = channels.filter((c) => (c.unread ?? 0) > 0 || (c.mentions ?? 0) > 0);
    setChannels((prev) => prev.map((c) => ({ ...c, unread: 0, mentions: 0 })));
    void Promise.allSettled(pendientes.map((c) => markRead(c.id))).then(() =>
      toast.notify({
        tone: 'success',
        title: t('status.markedAllRead', { count: pendientes.length }),
      }),
    );
  }, [channels, t, toast]);

  // Conmutador rápido de canales (Ctrl+G; Ctrl+K es la búsqueda de la suite).
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const atajoIrACanal = useAtajo('Ctrl+G');
  // Hoja de atajos («?»): lo que el compositor solo contaba al lector de pantalla.
  const [atajosOpen, setAtajosOpen] = useState(false);
  useEffect(() => {
    const alPulsar = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() === 'g' && (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey) {
        e.preventDefault();
        setSwitcherOpen(true);
        return;
      }
      // «?» abre la hoja de atajos, salvo mientras se escribe: en el compositor
      // el signo es texto, no un mando.
      if (e.key !== '?' || e.metaKey || e.ctrlKey || e.altKey) return;
      const donde = document.activeElement;
      if (
        donde instanceof HTMLElement &&
        (donde.isContentEditable ||
          donde.tagName === 'INPUT' ||
          donde.tagName === 'TEXTAREA' ||
          donde.closest('[role="dialog"]'))
      ) {
        return;
      }
      e.preventDefault();
      setAtajosOpen(true);
    };
    document.addEventListener('keydown', alPulsar);
    return () => document.removeEventListener('keydown', alPulsar);
  }, []);

  const sidebar = (
    <Sidebar
      header={
        // px-2.5 alinea la cabecera con el overline «Canales» y los items. Sin
        // «Sytue Rooms»: el selector de apps de la barra superior ya lo dice y la
        // barra lateral lo repetía justo debajo (informado el 2026-09-09). Lo que
        // queda es lo que solo vive aquí: tu presencia.
        <div class="flex items-center gap-2 px-2.5">
          {/* Estado de la CONEXIÓN en tiempo real, solo para lectores de
              pantalla: lo VISIBLE lo lleva el menú de presencia, que ya se pone en
              «Sin conexión» cuando cae el socket. */}
          <span class="sr-only" role="status" aria-live="polite">
            {statusLabel}
          </span>
          {/* Tu presencia: selector En línea / Ausente / No molestar + estado
              personalizado, y el ajuste de notificación de mensajes directos. */}
          <StatusMenu
            // Con el socket caído tu presencia no es la que elegiste: el Hub ya no
            // la recibe. «En línea» al lado de «Sin conexión» era contradictorio;
            // se enseña desconectado hasta que vuelva.
            state={status === 'open' ? myState : 'offline'}
            status={myStatus}
            onChange={changeMyPresence}
            dmNotify={dmAlerts.enabled}
            dmLoading={dmAlerts.loading}
            dmSaving={dmAlerts.saving}
            dmError={dmAlerts.error}
            dmLegacy={dmAlerts.hasLegacy}
            onRefreshDmNotify={dmAlerts.refresh}
            onDiscardLegacyDmNotify={dmAlerts.discardLegacy}
            onToggleDmNotify={dmAlerts.toggle}
            onOpenNotifySettings={() => setNotifySettingsOpen(true)}
            onOpenIntegrations={() => setIntegrationsOpen(true)}
            onMarkAllRead={marcarTodoLeido}
            canManageIntegrations={canManageIntegrations}
            t={t}
          />
        </div>
      }
    >
      {/* «Ir a un canal…»: la caja de arriba de Slack. Un botón con pinta de campo,
          porque lo que abre es el conmutador, no un filtro de la lista. */}
      <button
        type="button"
        onClick={() => setSwitcherOpen(true)}
        title={`${t('switcher.open')} (${atajoIrACanal || 'Ctrl+G'})`}
        class="border-sidebar-fg-muted/40 text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg sytue-focus-ring-chrome rounded-control mx-2.5 mb-2 flex h-8 w-[calc(100%-1.25rem)] items-center gap-2 border px-2.5 text-sm transition-colors duration-150 focus-visible:outline-none"
      >
        <Icon name="search" size={14} />
        <span class="min-w-0 flex-1 truncate text-left">{t('switcher.open')}</span>
        <kbd class="text-2xs border-sidebar-fg-muted/40 rounded border px-1 font-mono">
          {atajoIrACanal}
        </kbd>
      </button>
      {/* «Hilos» (Ola 9): vista global de hilos seguidos, con el nº de hilos con no
          leídos. Toggle de panel RHS (aria-pressed, como los toggles de la cabecera). */}
      <SidebarSection>
        <li>
          <button
            type="button"
            aria-pressed={rhsView === 'threads'}
            title={t('sidebar.threadsFollowed')}
            onClick={toggleThreadsPanel}
            class={cx(SIDEBAR_ITEM, sidebarItemState(rhsView === 'threads'))}
          >
            <span class="grid h-6 w-6 shrink-0 place-items-center">
              <ThreadsGlyph size={16} />
            </span>
            <span class="min-w-0 flex-1 truncate">{t('sidebar.threads')}</span>
            {threadsUnreadCount > 0 ? <Badge count>{threadsUnreadCount}</Badge> : null}
          </button>
        </li>
        {/* «Reuniones»: la puerta a las salas que NO cuelgan de un canal — convocar
            una, entrar por enlace, ver quién está y qué se grabó. Antes de esto, la
            única forma de hacer una videollamada era estar dentro de un canal. */}
        <li>
          <button
            type="button"
            aria-pressed={rhsView === 'meetings'}
            title={t('meetings.title')}
            onClick={openMeetings}
            class={cx(SIDEBAR_ITEM, sidebarItemState(rhsView === 'meetings'))}
          >
            <span class="grid h-6 w-6 shrink-0 place-items-center">
              <VideoGlyph />
            </span>
            <span class="min-w-0 flex-1 truncate">{t('meetings.open')}</span>
          </button>
        </li>
        {/* «Borradores»: lo escrito y no enviado, en cualquier canal o hilo. Antes solo
            existía si te acordabas de volver al canal donde lo dejaste —y solo en ESE
            dispositivo—; ahora vive en el servidor y se ve de un vistazo, como en
            Slack, Mattermost y Teams. La insignia lleva cuántos hay. */}
        {drafts.length > 0 ? (
          <li>
            <button
              type="button"
              aria-pressed={rhsView === 'drafts'}
              title={t('drafts.title')}
              onClick={toggleDraftsPanel}
              class={cx(SIDEBAR_ITEM, sidebarItemState(rhsView === 'drafts'))}
            >
              <span class="grid h-6 w-6 shrink-0 place-items-center">
                <EditGlyph size={16} />
              </span>
              <span class="min-w-0 flex-1 truncate">{t('drafts.title')}</span>
              <Badge count>{drafts.length}</Badge>
            </button>
          </li>
        ) : null}
        <li>
          <button
            type="button"
            onClick={() => setExternalOpen(true)}
            class={cx(SIDEBAR_ITEM, sidebarItemState(externalOpen))}
          >
            <span class="grid h-6 w-6 shrink-0 place-items-center">
              <Icon name="link" size={16} />
            </span>
            <span class="min-w-0 flex-1 truncate">{t('external.title')}</span>
          </button>
        </li>
        {/* «Apps e integraciones»: hasta ahora la ÚNICA puerta era el desplegable del
            avatar, donde nadie busca apps — el síntoma que reportaba el usuario era
            justamente no encontrar dónde añadir un bot. Aquí está a la vista, junto a
            las demás secciones de nivel superior. Solo para quien las administra: ver
            canManageIntegrations. */}
        {canManageIntegrations ? (
          <li>
            <button
              type="button"
              title={t('integrations.title')}
              onClick={() => setIntegrationsOpen(true)}
              class={cx(SIDEBAR_ITEM, sidebarItemState(integrationsOpen))}
            >
              <span class="grid h-6 w-6 shrink-0 place-items-center">
                <Icon name="grid" size={16} />
              </span>
              <span class="min-w-0 flex-1 truncate">{t('integrations.apps')}</span>
            </button>
          </li>
        ) : null}
      </SidebarSection>

      {invitaciones.length > 0 ? (
        <div ref={invitacionesRef}>
          <SidebarSection title={t('invitations.title')}>
            {invitaciones.map((inv) => (
              <li
                key={inv.channel_id}
                class="text-sidebar-fg flex flex-col gap-1.5 px-2.5 py-1.5 text-sm"
              >
                <span class="min-w-0">
                  {inv.kind === 'direct'
                    ? t('invitations.direct', {
                        name: inv.invited_by_name,
                        org: nombreDeOrganizacion(inv),
                      })
                    : t('invitations.channel', {
                        name: inv.invited_by_name,
                        org: nombreDeOrganizacion(inv),
                        channel: inv.name,
                      })}
                </span>
                <span class="flex gap-1.5">
                  <Button size="sm" onClick={() => void responderInvitacion(inv, true)}>
                    {t('invitations.accept')}
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => void responderInvitacion(inv, false)}
                  >
                    {t('invitations.reject')}
                  </Button>
                </span>
              </li>
            ))}
          </SidebarSection>
        </div>
      ) : null}

      {loadingChannels ? (
        <SidebarSection title={t('sidebar.channels')}>
          {/* Esqueletos con la geometría real del item (36px) — sin salto de layout. */}
          <li aria-busy="true" class="flex flex-col gap-1 px-0.5">
            <span role="status" class="sr-only">
              {t('sidebar.loadingChannels')}
            </span>
            {[0, 1, 2].map((i) => (
              <span
                key={i}
                aria-hidden="true"
                class="bg-sidebar-hover block h-9 animate-pulse rounded-md"
              />
            ))}
          </li>
        </SidebarSection>
      ) : channelsError ? (
        <SidebarSection title={t('sidebar.channels')}>
          {/* Error con acción de reintento también aquí (no solo en el panel). */}
          <li class="text-sidebar-fg-muted flex flex-wrap items-center gap-x-1.5 px-2.5 py-1.5 text-xs">
            <span>{t('sidebar.loadFailed')}</span>
            <button
              type="button"
              class="text-sidebar-fg hover:text-sidebar-active-fg sytue-focus-ring-chrome rounded-sm font-semibold underline underline-offset-2 transition-colors duration-150 focus-visible:outline-none"
              onClick={() => setChannelsTick((n) => n + 1)}
            >
              {t('action.retry')}
            </button>
          </li>
        </SidebarSection>
      ) : (
        <>
          {groups.map((group) => {
            const isCustom = Boolean(group.category);
            // ¿Puede soltarse aquí lo que se arrastra? Un canal cae en cualquier sección;
            // una categoría se reordena soltándola sobre OTRA categoría personalizada.
            const dropActive =
              drag?.kind === 'channel' ||
              (drag?.kind === 'category' && isCustom && drag.id !== group.category!.id);
            // Personalizada: el nombre que escribió el usuario. Por defecto: catálogo.
            const groupTitle = group.title ?? (group.titleKey ? t(group.titleKey) : '');
            return (
              <CategorySection
                key={group.key}
                bodyId={`cat-body-${group.key}`}
                title={groupTitle}
                unread={group.unread}
                collapsed={isGroupCollapsed(group)}
                onToggleCollapse={() => toggleGroupCollapse(group)}
                {...(isCustom
                  ? {
                      menuItems: categoryMenu(group.category!),
                      menuLabel: t('category.actions', { name: groupTitle }),
                    }
                  : {})}
                {...(group.key === DEFAULT_CHANNELS
                  ? {
                      // «+» de la cabecera de «Canales»: la puerta principal para crear
                      // un canal (paridad Slack/Mattermost/Teams). Siempre visible y con
                      // title: la acción del carril de @sytue/ui, que es la que mide los
                      // 24 px de WCAG 2.5.8 (la copia en `h-6` se quedaba en 21).
                      headerAction: (
                        <SidebarAction
                          label={t('sidebar.createChannel')}
                          icon={<Icon name="plus" size={16} />}
                          onClick={() => setCreateOpen(true)}
                          class="shrink-0"
                        />
                      ),
                    }
                  : {})}
                draggable={isCustom}
                onDragStart={() =>
                  group.category && setDrag({ kind: 'category', id: group.category.id })
                }
                onDragEnd={() => setDrag(null)}
                dropActive={dropActive}
                t={t}
                onDrop={() => {
                  if (!drag) return;
                  if (drag.kind === 'channel') dropChannelOnGroup(group, drag.id);
                  else if (group.category) reorderCategoryBefore(drag.id, group.category.id);
                  setDrag(null);
                }}
              >
                {group.channels.map(renderChannelRow)}
                {group.channels.length === 0 && group.key === DEFAULT_CHANNELS ? (
                  <li class="text-sidebar-fg-muted px-2.5 py-1.5 text-xs">
                    {t('sidebar.noChannelsYet')}
                  </li>
                ) : null}
                {group.channels.length === 0 && isCustom ? (
                  <li class="text-sidebar-fg-muted px-2.5 py-1.5 text-xs">
                    {t('sidebar.dropHint')}
                  </li>
                ) : null}
                {group.key === DEFAULT_CHANNELS ? (
                  <li>
                    <button
                      type="button"
                      onClick={() => setBrowseOpen(true)}
                      class="text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg sytue-focus-ring-chrome rounded-nav flex w-full items-center gap-2 px-[var(--sytue-nav-pad)] py-2 text-left text-base font-medium transition-colors duration-150 focus-visible:outline-none"
                    >
                      <span class="grid h-6 w-6 shrink-0 place-items-center">
                        <Icon name="search" size={16} />
                      </span>
                      <span class="min-w-0 flex-1 truncate">{t('sidebar.browseChannels')}</span>
                    </button>
                  </li>
                ) : null}
                {group.key === DEFAULT_DMS ? (
                  <li>
                    <button
                      type="button"
                      onClick={() => setNewDmOpen(true)}
                      class="text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg sytue-focus-ring-chrome rounded-nav flex w-full items-center gap-2 px-[var(--sytue-nav-pad)] py-2 text-left text-base font-medium transition-colors duration-150 focus-visible:outline-none"
                    >
                      <span class="grid h-6 w-6 shrink-0 place-items-center">
                        <Icon name="plus" size={16} />
                      </span>
                      <span class="min-w-0 flex-1 truncate">{t('sidebar.newMessage')}</span>
                    </button>
                  </li>
                ) : null}
              </CategorySection>
            );
          })}
          {/* Crear una categoría personalizada (nombre por prompt nativo). */}
          <button
            type="button"
            onClick={() => void createNewCategory()}
            class="text-sidebar-fg-muted hover:bg-sidebar-hover hover:text-sidebar-fg sytue-focus-ring-chrome rounded-nav mb-4 flex w-full items-center gap-2 px-[var(--sytue-nav-pad)] py-2 text-left text-base font-medium transition-colors duration-150 focus-visible:outline-none"
          >
            <span class="grid h-6 w-6 shrink-0 place-items-center">
              <Icon name="plus" size={16} />
            </span>
            <span class="min-w-0 flex-1 truncate">{t('sidebar.newCategory')}</span>
          </button>
        </>
      )}
    </Sidebar>
  );

  const channelPanel = active ? (
    <div class="bg-surface flex h-full min-w-0 flex-1 flex-col">
      {/* Cabecera de canal en la banda de cabecera (`bg-header`): un paso de tinta
          bajo la topbar, así se lee como cabecera sin borde. docs/design.md §2. */}
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center gap-3 px-4">
        {/* Móvil (<768px): hamburguesa que abre el drawer de canales (equivale a «atrás»
            a la lista). En ≥768px la sidebar está siempre visible, así que se oculta. */}
        <button
          id={menuBtnId}
          type="button"
          class="text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control -ml-1 inline-flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150 md:hidden"
          aria-label={t('sidebar.openChannels')}
          aria-expanded={drawerOpen}
          aria-controls={drawerId}
          onClick={() => setDrawerOpen(true)}
        >
          <Icon name="menu" size={16} />
        </button>
        {active.kind === 'direct' ? (
          isGroupDm(active) ? (
            <GroupAvatar px={32} />
          ) : (
            <Avatar
              name={active.name}
              size="sm"
              {...(activePeerPresence
                ? { presence: toPresenceStatus(activePeerPresence.state) }
                : {})}
            />
          )
        ) : (
          <span
            class="bg-chrome-hover text-md text-header-fg grid h-8 w-8 shrink-0 place-items-center rounded-md font-semibold"
            aria-hidden="true"
          >
            #
          </span>
        )}
        <div class="flex min-w-0 flex-1 flex-col justify-center">
          <div class="flex min-w-0 items-center gap-2">
            {/* El nombre del canal ES el disparador de su menú (Slack, Teams y
                Mattermost lo hacen así). Sigue siendo el <h1> de la vista para los
                lectores de pantalla: el botón va dentro, no en su lugar. También
                permite borrar un chat privado desde la conversación abierta. */}
            {/* El margen negativo que alinea el nombre con el borde va en el h1 y
                NO en el botón: un margen negativo dentro resta de la anchura que
                el h1 calcula para sí, y el botón —con `max-w-full`— quedaba 12px
                más estrecho que su texto: «Aleat…» con media cabecera libre. */}
            <h1 class="-mx-1.5 min-w-0 truncate">
              <MessageMenu
                label={t('channel.menuAria', { name: active.name })}
                items={channelHeaderMenuItems(active)}
                // `min-h-[24px]`: con `py-0.5` el disparador medía 21px de
                // alto, por debajo del mínimo de WCAG 2.5.8 (AA) para lo que
                // se toca. Y `min-h-6` no lo arreglaba: son 1,5 rem, que con la
                // raíz a 14 vuelven a ser 21 (medido en producción, 2026-09-21). Es el nombre del canal, que en un teléfono es el
                // camino a ajustes, miembros y salir del canal.
                triggerClass="text-md text-header-fg hover:bg-chrome-hover active:bg-chrome-active inline-flex min-h-[24px] max-w-full items-center gap-1 truncate rounded-control px-1.5 py-0.5 leading-tight font-semibold transition-colors duration-150 sytue-focus-ring"
                trigger={
                  <>
                    <span class="truncate">{active.name}</span>
                    <Icon name="chevronDown" size={14} />
                  </>
                }
              />
            </h1>
            {active.kind === 'private' ? (
              <span class="rounded-pill border-chrome-border text-header-fg shrink-0 border px-2 py-0.5 text-xs font-medium">
                {t('channel.privateBadge')}
              </span>
            ) : null}
          </div>
          {/* DM: presencia del peer (estado + estado personalizado) bajo el nombre.
              Canal: tema, visible siempre y editable en línea para admins. Enter
              guarda, Esc cancela, blur confirma. */}
          {active.kind === 'direct' ? (
            isGroupDm(active) ? (
              // Group DM: nº total de participantes (tú incluido); el h1 (active.name)
              // ya lista a los OTROS miembros. title= muestra la lista completa al pasar.
              <p
                class="text-header-fg/80 truncate text-xs leading-tight"
                title={t('dm.youAnd', { name: active.name })}
              >
                {t('dm.peopleCount', { count: (active.peers?.length ?? 0) + 1 })}
              </p>
            ) : (
              <p class="text-header-fg/80 truncate text-xs leading-tight">
                {t(STATE_KEY[activePeerPresence?.state ?? 'offline'])}
                {activePeerPresence?.status ? ` · ${activePeerPresence.status}` : ''}
              </p>
            )
          ) : editingTopic ? (
            <input
              type="text"
              value={topicDraft}
              // eslint-disable-next-line jsx-a11y/no-autofocus -- al editar el tema, el input toma el foco directamente
              autoFocus
              maxLength={280}
              placeholder={t('channel.topicPlaceholder')}
              onInput={(e) => setTopicDraft(e.currentTarget.value)}
              onBlur={commitTopic}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  commitTopic();
                } else if (e.key === 'Escape') {
                  e.preventDefault();
                  setEditingTopic(false);
                }
              }}
              aria-label={t('channel.topicAria')}
              // Marcador al 60 % y no al 50: sobre la tinta de la cabecera el 50 % daba
              // 4,45:1, por debajo del 4,5 de WCAG 1.4.3 (el 60 %, 5,7).
              class="border-header-fg/40 text-header-fg placeholder:text-header-fg/60 w-full border-b bg-transparent text-xs leading-tight focus:outline-none"
            />
          ) : active.topic ? (
            isTeamAdmin && !active.external ? (
              <button
                type="button"
                onClick={startEditTopic}
                title={t('channel.editTopic')}
                class="text-header-fg/80 hover:text-header-fg sytue-focus-ring truncate text-left text-xs leading-tight hover:underline"
              >
                {active.topic}
              </button>
            ) : (
              <p class="text-header-fg/80 truncate text-xs leading-tight" title={active.topic}>
                {active.topic}
              </p>
            )
          ) : isTeamAdmin && !active.external ? (
            <button
              type="button"
              onClick={startEditTopic}
              class="text-header-fg/60 hover:text-header-fg sytue-focus-ring inline-flex items-center gap-1 self-start text-xs leading-tight"
            >
              <EditGlyph size={12} />
              {t('channel.addTopic')}
            </button>
          ) : null}
        </div>

        {/* Acciones de canal: Fijados (contador) y Guardados (cross-canal), + la
            llamada de vídeo. Botones sobre la cabecera sólida (bg-header).
            En <768px se quedan Buscar y Llamar: los seis juntos se salían 98px de
            un iPhone SE. Los retirados (miembros, fijados, guardados,
            programados) están todos en el menú del nombre del canal. */}
        <div class="flex shrink-0 items-center gap-1">
          {/* Facepile con contador: la puerta al roster que Slack/Teams/Mattermost
              ponen en la cabecera. Solo en canales (un DM no tiene roster gestionable).
              Los avatares son decorativos (aria-hidden): el botón ya se anuncia con el
              número de miembros. Si la carga falló se pinta solo el icono —sin contador
              inventado— para que el panel, que trae su «Reintentar», siga alcanzable. */}
          <button
            type="button"
            title={t('channel.summarize')}
            aria-label={t('channel.summarize')}
            disabled={resumiendo}
            onClick={() => void ponmeAlDia()}
            class="text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control flex h-8 w-8 shrink-0 items-center justify-center disabled:opacity-50"
          >
            <Icon name="info" size={16} />
          </button>
          {activeIsChannel && !rosterLoading ? (
            <button
              type="button"
              aria-pressed={rhsView === 'members'}
              aria-label={
                roster.length > 0
                  ? t('channel.membersCount', { count: roster.length })
                  : t('channel.members')
              }
              title={t('channel.members')}
              onClick={toggleMembersPanel}
              class={cx(
                'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 items-center gap-1.5 px-2 transition-colors duration-150 max-md:hidden',
                rhsView === 'members' && 'bg-chrome-active',
              )}
            >
              {roster.length > 0 ? (
                <>
                  <span class="flex items-center" aria-hidden="true">
                    {roster.slice(0, 3).map((m, i) => (
                      <Avatar
                        key={m.sub}
                        name={m.name}
                        size="xs"
                        // El anillo del color de la cabecera separa los avatares
                        // apilados sin sombras decorativas (el envoltorio mide lo mismo
                        // que el avatar, así que basta redondearlo igual).
                        class={cx(
                          'rounded-md ring-2 ring-[var(--sytue-header-bg)]',
                          i > 0 && '-ml-1.5',
                        )}
                      />
                    ))}
                  </span>
                  <span class="text-xs font-semibold tabular-nums">{roster.length}</span>
                </>
              ) : (
                <Icon name="users" size={16} />
              )}
            </button>
          ) : null}
          <button
            type="button"
            aria-pressed={rhsView === 'search'}
            aria-label={t('channel.searchMessages')}
            title={t('channel.search')}
            onClick={toggleSearchPanel}
            class={cx(
              'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 w-8 items-center justify-center transition-colors duration-150',
              rhsView === 'search' && 'bg-chrome-active',
            )}
          >
            <Icon name="search" size={16} />
          </button>
          <button
            type="button"
            aria-pressed={rhsView === 'pinned'}
            aria-label={t('channel.pinnedCount', { count: pins.length })}
            title={t('channel.pinned')}
            onClick={togglePinnedPanel}
            class={cx(
              'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 items-center gap-1 px-2 transition-colors duration-150 max-md:hidden',
              rhsView === 'pinned' && 'bg-chrome-active',
            )}
          >
            <PinGlyph size={16} />
            {pins.length > 0 ? (
              <span class="text-xs font-semibold tabular-nums">{pins.length}</span>
            ) : null}
          </button>
          <button
            type="button"
            aria-label={t('channel.files')}
            title={t('channel.files')}
            onClick={openChannelFiles}
            class="text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 w-8 items-center justify-center transition-colors duration-150 max-md:hidden"
          >
            <ClipGlyph size={16} />
          </button>
          <button
            type="button"
            aria-pressed={rhsView === 'saved'}
            aria-label={t('channel.saved')}
            title={t('channel.saved')}
            onClick={toggleSavedPanel}
            class={cx(
              'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 w-8 items-center justify-center transition-colors duration-150 max-md:hidden',
              rhsView === 'saved' && 'bg-chrome-active',
            )}
          >
            <BookmarkGlyph size={16} />
          </button>
          <button
            type="button"
            aria-pressed={rhsView === 'scheduled'}
            aria-label={t('channel.scheduledMessages')}
            title={t('channel.scheduled')}
            onClick={toggleScheduledPanel}
            class={cx(
              'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 w-8 items-center justify-center transition-colors duration-150 max-md:hidden',
              rhsView === 'scheduled' && 'bg-chrome-active',
            )}
          >
            <ClockGlyph size={16} />
          </button>
          <span class="bg-border mx-1 h-5 w-px max-md:hidden" aria-hidden="true" />
          {callError ? (
            <span class="text-header-fg/80 text-xs" role="status">
              {t('call.startFailed')}
            </span>
          ) : null}
          {/* La puerta cerrada NO es un fallo: tiene respuesta. Se dice qué pasa y
              se ofrece llamar; una vez llamado, el botón desaparece y queda el
              aviso de que el anfitrión ya lo sabe. */}
          {puertaCerrada && puertaCerrada.channelId === activeId ? (
            <span class="text-header-fg/80 flex items-center gap-2 text-xs" role="status">
              {puertaCerrada.llamado ? t('meet.knocked') : t('meet.roomLockedNotice')}
              {!puertaCerrada.llamado ? (
                <Button size="sm" variant="secondary" onClick={llamarALaPuerta}>
                  {t('meet.knock')}
                </Button>
              ) : null}
            </span>
          ) : null}
          <Button
            size="sm"
            variant="primary"
            leadingIcon={<VideoGlyph />}
            loading={callBusy}
            onClick={startChannelCall}
          >
            {callBusy ? t('connection.connecting') : t('call.call')}
          </Button>
        </div>
      </header>

      {/* Banner discreto de «llamada en curso» bajo la cabecera: solo si hay presentes
          y no estás ya dentro de la llamada. Accesible (role=status; acción real, no
          solo color). El sondeo lo mantiene fresco (~25 s). */}
      {activeCallCount > 0 && !call ? (
        <div
          role="status"
          class="border-border bg-selected text-primary-text flex shrink-0 items-center gap-2 border-b px-4 py-1.5 text-sm"
        >
          <VideoGlyph />
          <span class="min-w-0 flex-1 truncate">
            {t('call.inProgress', { count: activeCallCount })}
          </span>
          <Button size="sm" variant="secondary" loading={callBusy} onClick={startChannelCall}>
            {t('action.join')}
          </Button>
        </div>
      ) : null}

      <MessageView
        messages={messages}
        loading={loadingMessages}
        error={messagesError}
        channelName={active.name}
        memberNames={memberNames}
        onReact={handleToggleReaction}
        blockedSubs={blockedSubs}
        onReport={(m) => setReporting({ target: { messageId: m.id }, name: m.author.name })}
        onToggleBlock={(m, blocked) =>
          void toggleBlock(m.channelId, m.author.id, m.author.name, blocked)
        }
        onRetry={() => setMessagesTick((n) => n + 1)}
        onResend={handleResend}
        onOpenThread={openThread}
        onEdit={handleEdit}
        onDelete={handleDelete}
        pinnedIds={pinnedIds}
        savedIds={savedIds}
        onTogglePin={togglePin}
        onToggleSave={toggleSave}
        followedIds={followedIds}
        onToggleFollow={(m, following) => toggleFollow(m.id, following)}
        onMarkUnread={markMessageUnread}
        onForward={openForward}
        onAck={handleAck}
        onCardVote={handleCardVote}
        unreadDividerId={unreadDividerId}
        highlightId={highlightId}
        seen={seenReceipt}
        onJoinCall={startChannelCall}
        onLoadOlder={loadOlder}
        loadingOlder={loadingOlder}
        handleRef={listRef}
        locale={locale}
        t={t}
      />

      {/* «Ir a mensajes recientes» (Ola 16): al saltar a un mensaje antiguo (permalink /
          búsqueda) se muestra una ventana centrada; esta barra vuelve al final del canal
          (y lo marca leído). aria-live la anuncia sin robar el foco. */}
      {viewingHistory ? (
        <div
          class="border-border bg-bg-subtle shrink-0 border-t px-4 py-1.5"
          role="status"
          aria-live="polite"
        >
          <button
            type="button"
            onClick={jumpToPresent}
            class="text-primary-text hover:bg-selected sytue-focus-ring rounded-control flex w-full items-center justify-center gap-1.5 py-1 text-sm font-medium transition-colors duration-150"
          >
            <Glifo d="M12 5v14M6 13l6 6 6-6" size={14} />
            {t('message.goToRecent')}
          </button>
        </div>
      ) : null}

      {/* Indicador "escribiendo…" del canal activo: reserva altura para no saltar el
          layout; aria-live polite lo anuncia sin robar el foco. */}
      <div class="sytue-meta h-5 shrink-0 truncate px-4 italic" aria-live="polite">
        {typingNames.length > 0 ? typingLabel(t, typingNames) : ''}
      </div>

      <Composer
        // key por canal: remonta el composer al cambiar de canal → restaura su
        // borrador y NO arrastra el texto de un canal al siguiente (Ola 8). El epoch
        // lo remonta UNA vez más al llegar los borradores del servidor, para recoger
        // el que se empezó en otro dispositivo.
        key={`${active.id}:${draftsEpoch}`}
        channelName={active.name}
        disabled={!active}
        offline={offline}
        members={members}
        draftId={`${user.id}:${active.id}`}
        channelId={active.id}
        advanced
        onSend={handleSend}
        onCommand={runCommand}
        remoteCommands={remoteCommands}
        onRemoteCommand={runRemoteCommand}
        // Entrada «Apps e integraciones» del menú «+» (antes solo desde el menú de
        // presencia): mismo modal, otra puerta donde el admin la busca.
        onOpenIntegrations={() => setIntegrationsOpen(true)}
        onTyping={() => {
          if (activeId) realtimeRef.current?.sendTyping(activeId);
        }}
        locale={locale}
        t={t}
      />
    </div>
  ) : channelsError ? (
    <EmptyState
      class="h-full flex-1"
      title={t('empty.channelsError')}
      description={t('error.checkConnection')}
      illustration={<Icon name="danger" size={32} class="text-danger-text" />}
      action={
        <Button size="sm" variant="secondary" onClick={() => setChannelsTick((n) => n + 1)}>
          {t('action.retry')}
        </Button>
      }
    />
  ) : loadingChannels ? (
    <EmptyState
      class="h-full flex-1"
      title={t('empty.loadingChannels')}
      description={t('empty.loadingChannelsHelp')}
      illustration={<Spinner size={24} />}
    />
  ) : channels.length === 0 ? (
    // Un tenant recién creado empieza aquí, con 0 canales: sin este CTA no hay salida.
    <EmptyState
      illustration={<Illustration name="emptyChat" />}
      class="h-full flex-1"
      title={t('empty.noChannels')}
      description={t('empty.noChannelsHelp')}
      action={
        <div class="flex flex-wrap items-center justify-center gap-2">
          <Button size="sm" variant="primary" onClick={() => setCreateOpen(true)}>
            {t('sidebar.createChannel')}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setBrowseOpen(true)}>
            {t('sidebar.browseChannels')}
          </Button>
        </div>
      }
    />
  ) : (
    <EmptyState
      illustration={<Illustration name="emptyChat" />}
      class="h-full flex-1"
      title={t('empty.pickChannel')}
      description={t('empty.pickChannelHelp')}
    />
  );

  return (
    <SytueShell
      currentApp="rooms"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      locale={locale}
    >
      {/* Sidebar oscura de canales + vista de canal (layout canonico bajo la
          topbar de la shell; misma geometria que AppShell). En <768px la sidebar es un
          drawer sobre overlay (Ola 18); en ≥768px va en flujo, como siempre. */}
      {/* `sytue-frame` y `sytue-sheet` (en el `main`): el marco y la hoja de la forma
          «Marco», como en AppShell. En «Carril» no hacen nada. */}
      <div class="sytue-frame sytue-safe-workspace bg-bg text-text flex h-[calc(100dvh-var(--spacing-topbar))] min-h-0">
        {drawerOpen ? (
          // EMPIEZA BAJO EL TOPBAR: con `inset-0` el velo cubría la barra
          // superior de la Sytue Shell —buscador, selector de apps y cuenta—,
          // porque `--z-overlay` (1050) va por encima de `--z-topbar` (1040).
          // Acotarlo por arriba deja la escala de z como está.
          <div
            class="fixed inset-x-0 bottom-0 md:hidden"
            style={{
              top: 'var(--spacing-topbar)',
              zIndex: 'var(--z-overlay)',
              background: 'var(--sytue-overlay)',
            }}
            onClick={() => setDrawerOpen(false)}
            aria-hidden="true"
          />
        ) : null}
        <aside
          id={drawerId}
          tabIndex={-1}
          class={cx(
            'sytue-nav-drawer shrink-0 focus-visible:outline-none',
            // Por debajo del topbar, igual que el velo: arranca a la altura de
            // la barra en vez de en el borde de la pantalla.
            'max-md:fixed max-md:top-[var(--spacing-topbar)] max-md:bottom-0 max-md:left-0',
            // El z de modal es SOLO para el drawer móvil, que sí debe tapar el
            // contenido. Aplicarlo también en escritorio dejaba la sidebar (1060)
            // por encima de la topbar (1040): como la topbar crea contexto de
            // apilamiento, el selector de apps no podía escapar de ella y se veía
            // POR DEBAJO de la lista de canales. Mismo caso que AdminShell.
            'z-[var(--z-sidebar)] max-md:z-[var(--z-modal)]',
            drawerOpen ? 'sytue-drawer-in' : 'max-md:hidden',
          )}
        >
          {sidebar}
        </aside>
        {/* `relative` ancla el overlay de la llamada, que cubre el panel del canal
            (no la sidebar: los canales siguen accesibles durante la llamada). */}
        <main
          id="contenido"
          tabIndex={-1}
          class="sytue-sheet relative flex min-w-0 flex-1 focus-visible:outline-none"
        >
          {/* En móvil, con un panel RHS abierto, el canal se oculta para que el RHS ocupe
              la pantalla completa; en ≥768px conviven lado a lado como siempre. */}
          {/* LAS ESQUINAS DE LA HOJA: la hoja no recorta, así que las redondea lo de
              dentro (ver AppShell). Aquí son dos columnas de alto fijo con su propio
              desplazamiento, y cada una recorta a su caja: la del canal toca las cuatro
              esquinas, o solo las de la izquierda con un panel abierto a su lado. */}
          <div
            class={cx(
              'sytue-sheet-corners marco:overflow-clip flex min-w-0 flex-1',
              (threadRoot || rhsView) &&
                'max-md:hidden md:[--sytue-sheet-br:0px] md:[--sytue-sheet-tr:0px]',
            )}
          >
            {channelPanel}
          </div>
          {threadRoot ? (
            <ThreadPanel
              root={threadRoot}
              replies={threadReplies}
              loading={threadLoading}
              error={threadError}
              members={members}
              memberNames={memberNames}
              blockedSubs={blockedSubs}
              // El hilo puede abrirse desde la vista global «Hilos» (otro canal): el
              // nombre sale del canal de la raíz, no del canal activo.
              channelName={channelNames[threadRoot.channelId] ?? active?.name ?? ''}
              following={threadFollowing}
              onToggleFollow={toggleThreadFollow}
              onClose={closeThread}
              onRetry={() => openThread(threadRoot)}
              onSendReply={handleSendReply}
              onCardVote={handleCardVote}
              ownerId={user.id}
              offline={offline}
              locale={locale}
              t={t}
            />
          ) : rhsView === 'meetings' ? (
            <MeetingsPanel
              meetings={meetings}
              loading={meetingsLoading}
              error={meetingsError}
              detailFor={meetingDetailFor ?? undefined}
              participants={meetingParticipants}
              recordings={meetingRecordings}
              detailLoading={meetingDetailLoading}
              canModerate={isTeamAdmin}
              onCreate={onCreateMeeting}
              onJoin={onJoinMeeting}
              onOpenDetail={onOpenMeetingDetail}
              onClose={(id) => void onCloseMeeting(id)}
              onRemoveParticipant={(id, identity, name) =>
                void onRemoveMeetingParticipant(id, identity, name)
              }
              onRefresh={loadMeetings}
              onDismiss={() => setRhsView(null)}
              locale={locale}
              t={t}
            />
          ) : rhsView === 'pinned' ? (
            <MessageListPanel
              title={t('panel.pinned')}
              {...(active ? { subtitle: `#${active.name}` } : {})}
              messages={pins}
              loading={pinsLoading}
              error={pinsError}
              memberNames={memberNames}
              emptyText={t('panel.pinnedEmpty')}
              removeLabel={t('panel.unpin')}
              onRemove={(m) => togglePin(m, true)}
              onSelect={(m) => goToMessage(m.channelId, m.id)}
              onClose={() => setRhsView(null)}
              onRetry={() => setPinsTick((n) => n + 1)}
              locale={locale}
              t={t}
            />
          ) : rhsView === 'saved' ? (
            <MessageListPanel
              title={t('panel.saved')}
              messages={savedList}
              loading={savedLoading}
              error={savedError}
              memberNames={memberNames}
              channelNames={channelNames}
              emptyText={t('panel.savedEmpty')}
              removeLabel={t('panel.unsave')}
              onRemove={(m) => toggleSave(m, true)}
              onSelect={(m) => goToMessage(m.channelId, m.id)}
              onClose={() => setRhsView(null)}
              onRetry={loadSaved}
              {...(savedNext ? { onLoadMore: loadMoreSaved, loadingMore: savedLoadingMore } : {})}
              locale={locale}
              t={t}
            />
          ) : rhsView === 'search' ? (
            <SearchPanel
              channelNames={channelNames}
              memberNames={memberNames}
              initialQuery={searchSeed}
              onSelect={(channelId, messageId) => goToMessage(channelId, messageId)}
              onClose={() => setRhsView(null)}
              locale={locale}
              t={t}
            />
          ) : rhsView === 'threads' ? (
            <ThreadsPanel
              threads={threadsList}
              loading={threadsLoading}
              error={threadsError}
              memberNames={memberNames}
              channelNames={channelNames}
              onSelect={(th) => openThread(th.root)}
              onClose={() => setRhsView(null)}
              onRetry={loadThreads}
              {...(threadsNext
                ? { onLoadMore: loadMoreThreads, loadingMore: threadsLoadingMore }
                : {})}
              locale={locale}
              t={t}
            />
          ) : rhsView === 'members' && active ? (
            <MembersPanel
              channelName={active.name}
              members={roster}
              teamMembers={members}
              loading={rosterLoading}
              error={rosterError}
              canManage={canManageMembers}
              selfSub={user.id}
              onAdd={addMemberToChannel}
              onRemove={removeMemberFromChannel}
              onSetRole={changeMemberRole}
              blockedSubs={blockedSubs}
              onReport={(m) =>
                setReporting({ target: { channelId: active.id, subject: m.sub }, name: m.name })
              }
              onToggleBlock={(m, blocked) => void toggleBlock(active.id, m.sub, m.name, blocked)}
              onRetry={() => setRosterTick((n) => n + 1)}
              onClose={() => setRhsView(null)}
              presences={presences}
              t={t}
            />
          ) : rhsView === 'drafts' ? (
            <DraftsPanel
              drafts={drafts}
              channelNames={channelNames}
              onSelect={goToDraft}
              onDiscard={discardDraft}
              onClose={() => setRhsView(null)}
              t={t}
            />
          ) : rhsView === 'scheduled' ? (
            <ScheduledPanel
              scheduled={scheduledList}
              loading={scheduledLoading}
              error={scheduledError}
              channelNames={channelNames}
              onCancel={cancelScheduledMsg}
              onSendNow={sendScheduledNow}
              onReschedule={rescheduleMsg}
              onClose={() => setRhsView(null)}
              onRetry={loadScheduled}
              locale={locale}
              t={t}
            />
          ) : null}
          {enLaApp ? (
            <AbrirEnLaApp
              abierta={enLaApp.abierta}
              t={t}
              onAbrir={() => {
                const { id, clave } = enLaApp;
                void abrirReunionEnLaApp(id, clave).then((abierta) => {
                  if (abierta) setEnLaApp({ id, clave, abierta: true });
                });
              }}
              onAqui={() => {
                const { id, clave } = enLaApp;
                setEnLaApp(null);
                void onJoinMeeting(id, clave);
              }}
            />
          ) : null}
          {/* Llamada entrante en OTRO canal. Fuera del overlay de vídeo a
              propósito: si ya estás dentro de una llamada no se anuncia. */}
          {entrante ? (
            <IncomingCall
              label={entrante.label}
              direct={entrante.direct}
              onAccept={contestarEntrante}
              onDismiss={() => setEntrante(null)}
              t={t}
            />
          ) : null}

          {call ? (
            <div class="sytue-sheet-corners bg-neutral-1000 marco:overflow-clip absolute inset-0 z-40">
              <Suspense
                fallback={
                  <div class="bg-neutral-1000 grid h-full place-items-center">
                    <Spinner size={28} />
                  </div>
                }
              >
                {callChoice ? (
                  <VideoRoom
                    connection={call.connection}
                    roomTitle={call.roomTitle}
                    onLeave={() => {
                      setCall(null);
                      setCallChoice(null);
                    }}
                    t={t}
                    choice={callChoice}
                    roomId={call.roomId}
                    hostIdentity={call.hostIdentity}
                    orgAdmin={user.roles.includes('admin') || user.roles.includes('owner')}
                    locked={call.locked}
                    {...(callChannelId ? { channelId: callChannelId } : {})}
                    {...(callChannelId && callChannelId === activeId
                      ? {
                          chat: {
                            // Los últimos, que es lo que se lee en una llamada; el
                            // historial entero está en el canal, a un clic de salir.
                            mensajes: messages.slice(-40).map((m) => ({
                              id: m.id,
                              autor: m.author.name,
                              texto: m.body,
                              mio: m.mine,
                            })),
                            // El MISMO envío que el composer del canal: optimista,
                            // con menciones y con lo que venga después. Un segundo
                            // camino para mandar un mensaje acabaría divergiendo.
                            enviar: (texto: string) => handleSend(texto),
                          },
                        }
                      : {})}
                  />
                ) : (
                  <PreJoin
                    roomTitle={call.roomTitle}
                    onJoin={setCallChoice}
                    onCancel={() => setCall(null)}
                    t={t}
                  />
                )}
              </Suspense>
            </div>
          ) : null}
        </main>
      </div>

      {/* Modales de la Ola 3 (portales de @sytue/ui): explorar canales y nuevo DM. */}
      <BrowseChannelsModal
        open={browseOpen}
        onClose={() => setBrowseOpen(false)}
        onJoined={handleJoined}
        t={t}
      />
      {/* Ola 2B de paridad: crear canal (público/privado + tema). */}
      <CreateChannelModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={handleChannelCreated}
        t={t}
      />
      <Modal open={resumen !== null} onClose={() => setResumen(null)} title={t('channel.summary')}>
        <TextoDelAsistente texto={resumen ?? ''} />
      </Modal>
      <ShortcutsModal
        open={atajosOpen}
        title={t('shortcuts.title')}
        groups={[
          {
            titulo: t('shortcuts.group.move'),
            atajos: [
              { teclas: ['Ctrl', 'G'], descripcion: t('shortcuts.switcher') },
              { teclas: ['Esc'], descripcion: t('shortcuts.close') },
              { teclas: ['?'], descripcion: t('shortcuts.help') },
            ],
          },
          {
            titulo: t('shortcuts.group.write'),
            atajos: [
              { teclas: [t('key.enter')], descripcion: t('composer.hintSend') },
              { teclas: ['@'], descripcion: t('composer.hintMention') },
              { teclas: [':'], descripcion: t('composer.hintEmoji') },
              { teclas: ['/'], descripcion: t('composer.hintCommands') },
            ],
          },
        ]}
        onClose={() => setAtajosOpen(false)}
      />

      <QuickSwitcher
        open={switcherOpen}
        channels={channels}
        activeId={activeId}
        leading={(c) => channelLeading(c, true)}
        onClose={() => setSwitcherOpen(false)}
        onPick={(c) => setActiveId(c.id)}
        t={t}
      />
      <NewDmModal
        open={newDmOpen}
        onClose={() => setNewDmOpen(false)}
        members={members}
        selfId={user.id}
        onOpened={handleDmOpened}
        t={t}
      />
      {/* Ola 8: reenviar el mensaje seleccionado a otro canal (selector + toast). */}
      <ForwardMessageModal
        open={forwarding !== null}
        onClose={() => setForwarding(null)}
        channels={channels}
        message={forwarding}
        onForward={handleForward}
        t={t}
      />
      {/* Ola 13: preferencias de notificación (palabras clave + No molestar). */}
      <NotifySettingsModal
        open={notifySettingsOpen}
        onClose={() => setNotifySettingsOpen(false)}
        onSaved={setNotifyPrefs}
        t={t}
      />
      <ReportModal
        target={reporting?.target ?? null}
        name={reporting?.name ?? ''}
        onClose={() => setReporting(null)}
        t={t}
      />
      <BlockedModal
        open={blockedOpen}
        blocks={blocks}
        onUnblock={(b) => void toggleBlock('', b.sub, b.name, true)}
        onClose={() => setBlockedOpen(false)}
        t={t}
      />
      <ReportsModal open={reportsOpen} onClose={() => setReportsOpen(false)} t={t} />
      {/* Ola 15: integraciones (webhooks entrantes/salientes + bots). */}
      {externalOpen ? (
        <ExternalConversations
          onClose={() => setExternalOpen(false)}
          t={t}
          meetingChannels={channels}
          activeChannelId={active?.id}
          captureMeetingScope={() => {
            const scope = captureMatrixScope();
            return {
              assertCurrent: () => assertMatrixScope(scope),
              share: async (channelId, url, clientId) => {
                assertMatrixScope(scope);
                await sendMessage({ channelId, body: url, clientId });
                assertMatrixScope(scope);
              },
            };
          }}
        />
      ) : null}
      <IntegrationsModal
        isOrgAdmin={user.roles.some((r) => r === 'owner' || r === 'admin')}
        open={integrationsOpen}
        onClose={() => setIntegrationsOpen(false)}
        channels={channels}
        {...(active ? { activeChannelId: active.id } : {})}
        t={t}
      />
      <ConfirmDialog
        open={archivando !== null}
        onClose={() => setArchivando(null)}
        onConfirm={() => {
          if (archivando) archiveChannel(archivando);
        }}
        title={t('channel.archive')}
        confirmLabel={t('channel.archive')}
        tone="danger"
      >
        <p class="text-text text-sm leading-relaxed">
          {archivando ? t('channel.confirmArchive', { name: archivando.name }) : ''}
        </p>
      </ConfirmDialog>
      {dialogoConfirmar}
      {dialogoPreguntar}
    </SytueShell>
  );
}
