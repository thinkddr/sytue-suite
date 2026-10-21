/**
 * Cliente REST del lado servidor para el servicio Go propio Sytue Chat.
 *
 * Habla con la API REST del servicio Go (dev :8085, prod rooms.sytue.eu) usando el
 * access token de Sytue ID (Authorization: Bearer) que solo vive en el servidor.
 * El BFF (src/pages/api/*) reenvia las peticiones del navegador a traves de este
 * cliente, de modo que el token nunca toca el cliente. El tiempo real (entrega de
 * mensajes nuevos) va por WebSocket directo navegador<->servicio Go autorizado con
 * un ticket de corta duracion (ver src/pages/api/ws-ticket.ts).
 *
 * Reimplementacion clean-room sobre API REST propia; sin dependencias AGPL.
 * docs/CONVENTIONS.md §1.
 */
import { serverEnv } from '@sytue/auth/server';
import type { MatrixReference, MatrixFile } from '../client/matrix/types.js';
import { ChatClient as SharedChatClient, bearer } from '@sytue/api-clients';

export interface Channel {
  id: string;
  /** Nombre legible (sin '#'). En DMs, el nombre de los OTROS participantes (lo
   *  resuelve el backend per-visor en ListChannels): en un 1:1 el del peer; en un
   *  grupo, «Ana, Beto, Caro». */
  name: string;
  /** DMs (kind==='direct'): nombres de los OTROS miembros, per-visor. Distingue un DM
   *  de grupo (varios) de un 1:1 (uno) y permite componer la cabecera. */
  peers?: string[];
  /** DMs: SUBJECTS de esos mismos OTROS miembros, ALINEADOS índice a índice con `peers`
   *  (mapea `peer_subs` del servicio Go). Fuente EXACTA para la presencia/receipts del
   *  peer (no el frágil match por nombre). */
  peerSubs?: string[];
  topic?: string;
  /** «De qué va este canal»: el texto que se lee al EXPLORAR canales, distinto del
   *  tema (el estado del día bajo el nombre). Se fija con «/purpose». */
  purpose?: string;
  /** 'public', 'private' o 'direct' (DM 1:1). Coincide con domain.ChannelKind del
   *  servicio Go: NO usar 'dm'/'channel' (el sidebar filtra por 'direct'). */
  kind: 'public' | 'private' | 'direct';
  /** Numero de mensajes sin leer para el usuario actual. */
  unread?: number;
  /** Subconjunto de `unread` que menciona (@) al usuario actual: la señal de
   *  "te han mencionado aquí" que la sidebar resalta aparte del no leído. */
  mentions?: number;
  /** Favorito del usuario actual (per-usuario): agrupa el canal arriba en el sidebar. */
  favorite?: boolean;
  /** Silenciado por el usuario actual: se atenúa y no cuenta para notificaciones. Es la
   *  PROYECCIÓN visual de notifyLevel==='none' (fuente de verdad), no un flag aparte. */
  muted?: boolean;
  /** Nivel de notificación del canal (Ola 13): 'all' (todos), 'mentions' (solo
   *  @menciones/palabras clave) o 'none' (nada, ⟺ muted). Fuente de verdad del aviso. */
  notifyLevel?: 'all' | 'mentions' | 'none';
  /** Categoría del sidebar (per-usuario) a la que el usuario asignó el canal; ausente
   *  = categoría por defecto según kind (Favoritos si favorito, DMs si directo, si no
   *  Canales). La resuelve el backend en listChannels (Ola 11). */
  categoryId?: string;
  /** Orden del canal dentro de su categoría (per-usuario, Ola 11). */
  sortOrder?: number;
  /** Marca de tiempo del ultimo mensaje (ISO 8601). */
  lastMessageAt?: string;
  /** Conversación de OTRA organización en la que el usuario está como persona de
   *  fuera (ADR 0040). Vive allí, así que no es de ningún equipo suyo: el sidebar la
   *  marca y no ofrece lo que solo tiene sentido en la propia organización. */
  external?: boolean;
}

/** Alguien de Sytue One encontrado por su @usuario o su correo (people:resolve):
 *  lo mínimo para escribirle y pintarle. */
export interface Persona {
  sub: string;
  name: string;
  username?: string;
}

/** Categoría del sidebar de un usuario (Ola 11, estilo Mattermost/Slack): grupo
 *  personalizado y colapsable de canales. Estado per-usuario. */
export interface Category {
  id: string;
  name: string;
  /** Orden de la categoría en el sidebar (menor primero). */
  sortOrder: number;
  /** ¿Está colapsada (persistido)? */
  collapsed: boolean;
}

/** Adjunto de un mensaje: referencia a un fichero de Sytue Drive con sus
 *  metadatos denormalizados (para pintar la miniatura/tarjeta sin llamar a Drive). */
export interface Attachment {
  matrixFile?: MatrixFile;
  objectUrl?: string;
  /** Id del fichero en Sytue Drive (se sirve vía /api/attachment?fileId=). */
  fileId: string;
  name: string;
  /** MIME del contenido (image/* se previsualiza como miniatura). */
  contentType: string;
  sizeBytes: number;
}

/** Emoji personalizado del tenant (Ola 14): un shortcode `:name:` ligado a una
 *  imagen de Sytue Drive (fileId), servida por el proxy de adjuntos (nosniff). */
export interface CustomEmoji {
  name: string;
  fileId: string;
  /** Tipo MIME de la imagen (lo denormaliza el backend). Es lo que distingue un GIF
   *  animado de un PNG dentro del MISMO catálogo de imágenes del tenant. */
  contentType?: string;
}

/** Webhook ENTRANTE (Ola 15): token opaco ligado a un canal; POST /hooks/{token}
 *  publica en él. El token en claro NO viaja aquí (solo al crear/rotar). */
export interface IncomingWebhook {
  id: string;
  channelId: string;
  name: string;
  icon?: string;
  createdAt: string;
}

/**
 * Lo que vuelve UNA vez al crear o rotar un entrante: el token y la URL en claro y el
 * dominio del correo a canal si el servicio lo tiene encendido. Sin dominio no se
 * ofrece la dirección de correo: una que rebota es peor que ninguna.
 */
export interface IncomingSecret {
  webhook: IncomingWebhook;
  token: string;
  url: string;
  channelMailDomain?: string;
}

/** Webhook SALIENTE (Ola 15): dispara un POST SSRF-safe a callbackUrl cuando un
 *  mensaje casa (canal y/o palabra disparadora). `token` es el de verificación
 *  (se envía en la cabecera del POST; se puede mostrar). */
/**
 * Formato en que Rooms habla con el destino de un webhook saliente. Son los mismos tres
 * valores que persiste la columna `dialect` (migración 0017) y que traduce
 * `services/rooms/internal/compat`: mantenerlos en sincronía es lo que hace que un bot
 * escrito para Slack funcione contra Rooms sin tocarlo.
 */
export type OutgoingDialect = 'sytue' | 'slack' | 'teams';

export interface OutgoingWebhook {
  id: string;
  channelId?: string;
  triggerWords?: string[];
  callbackUrl: string;
  token?: string;
  /** Ausente en webhooks creados antes de la migración 0017 ⇒ 'sytue'. */
  dialect?: OutgoingDialect;
  /**
   * Secreto con el que se firma cada entrega. Es lo que hay que pegar en el bot
   * (`SLACK_SIGNING_SECRET` o la clave de seguridad de Teams) para que valide el
   * origen: a diferencia de `token`, este valor NO viaja en las entregas.
   */
  signingSecret?: string;
  createdAt: string;
}

/** Plataforma de un PUENTE de canal (migración 0033 de Rooms). */
export type BridgePlatform = 'slack' | 'teams' | 'googlechat';

/**
 * PUENTE: lo que se escribe en el canal se repite en Slack, Teams o Google Chat por
 * el webhook entrante del otro lado. La URL es un secreto y el servicio no la
 * devuelve nunca; `target` es su forma enmascarada, para reconocerla en la lista.
 */
export interface Bridge {
  id: string;
  channelId: string;
  platform: BridgePlatform;
  target: string;
  /** Último fallo de entrega (webhook revocado, flujo borrado). Ausente = entregando. */
  lastError?: string;
  lastErrorAt?: string;
  createdAt: string;
}

/**
 * Número de WHATSAPP BUSINESS conectado a un equipo (migración 0034 de Rooms): cada
 * cliente que escribe tiene su canal en ese equipo. El token de Meta y el secreto de
 * la app no vuelven nunca del servidor.
 */
export interface WhatsAppAccount {
  id: string;
  teamId: string;
  phoneNumberId: string;
  displayPhone: string;
  verifiedName?: string;
  createdAt: string;
}

/** BOT (Ola 15): identidad del tenant con token Bearer propio (prefijo bot_) que
 *  publica por la API en los canales a los que se le añade. */
export interface Bot {
  id: string;
  name: string;
  createdAt: string;
}

/** Alguien que ha visto un mensaje, y cuándo lo vio. */
export interface MessageReader {
  subject: string;
  name?: string;
  read_at: string;
}

/**
 * Voto de una tarjeta: quién ha marcado un valor —la opción de la encuesta ('0'..'9')
 * o el 'done' de una tarea—. `mine` lo resuelve el cliente contra el sub de la sesión,
 * igual que en las reacciones.
 */
export interface CardVote {
  value: string;
  count: number;
  mine: boolean;
  subs?: string[];
}

/**
 * Objeto rico del mensaje: encuesta o tarea. El TEXTO del mensaje sigue siendo la
 * pregunta o el título —la tarjeta solo lleva lo que el texto no puede—, así que un
 * cliente antiguo que no la entienda sigue enseñando algo legible.
 */
export interface MessageCard {
  kind: 'poll' | 'task';
  /** Opciones de la encuesta, en orden; el voto referencia su ÍNDICE. */
  options?: string[];
  /** La encuesta admite varias respuestas por persona (casillas, no radios). */
  multi?: boolean;
  /** Responsable de la tarea (opcional). */
  assigneeSub?: string;
  assigneeName?: string;
  /** Fecha límite de la tarea (ISO 8601, opcional). */
  due?: string;
  /** Estado agregado por valor (votos de la encuesta, 'done' de la tarea). */
  votes?: CardVote[];
}

export interface ChatMessage {
  matrix_event?: MatrixReference;
  encryption?: 'm.megolm.v1.aes-sha2' | 'legacy';
  decryptionFailed?: boolean;
  identityVerified?: boolean;
  authenticityVerified?: boolean;
  id: string;
  channelId: string;
  author: { id: string; name: string };
  /** Texto del mensaje (Markdown ligero). */
  body: string;
  /** Adjuntos del mensaje (ficheros de Sytue Drive). */
  attachments?: Attachment[];
  /** Fecha de creacion (ISO 8601). */
  createdAt: string;
  /** Fecha de la ultima edicion (ISO 8601); presente si el mensaje fue editado. */
  editedAt?: string;
  /** True si el autor es el usuario de la sesion. */
  mine?: boolean;
  /** Estado de envio optimista (solo en cliente). */
  pending?: boolean;
  /** Envio fallido, reintentable (solo en cliente). */
  failed?: boolean;
  /** Reacciones agregadas del mensaje (emoji + conteo + si reacciono el usuario). */
  reactions?: MessageReaction[];
  /** Numero de respuestas en hilo (si el mensaje abre un hilo). */
  replyCount?: number;
  /** Id del mensaje raiz si este mensaje es una respuesta en hilo. */
  parentId?: string;
  /** Subs de usuarios mencionados en el mensaje (para resaltar @Nombre). */
  mentions?: string[];
  /** El mensaje menciona a TODO el canal (@channel/@here/@all): sube el badge de
   *  @menciones al instante por WS aunque no lleve tu sub (ver lib/mentions.ts). */
  mentionsAll?: boolean;
  /** Tipo de mensaje: 'call' = tarjeta de sistema de «llamada iniciada» (Sytue
   *  Rooms); ausente o 'user' = mensaje normal. */
  type?: 'user' | 'call';
  /** Mensaje publicado por una integración (webhook entrante/saliente o bot, Ola 15):
   *  pinta el badge «BOT» junto al autor. Lo deriva el backend del prefijo del autor
   *  sintético (hook:/bot:), no de una columna. */
  isBot?: boolean;
  /** Prioridad (Ola 12); ausente = 'normal'. El cliente pinta un badge con TEXTO
   *  (nunca solo color, WCAG AA). */
  priority?: 'important' | 'urgent';
  /** El autor pide acuse de recibo explícito a los lectores (Ola 12). */
  requiresAck?: boolean;
  /** Nº de lectores que confirmaron (agregado, sin N+1). Solo si requiresAck. */
  ackCount?: number;
  /** True si el usuario de la sesión ya confirmó este mensaje (per-visor). */
  acked?: boolean;
  /** Objeto rico del mensaje (encuesta o tarea); ausente = mensaje normal. */
  card?: MessageCard;
}

/** Mensaje PROGRAMADO (Ola 12): redactado para publicarse en sendAt; un worker lo
 *  envía al vencer. Alimenta el panel «Programados» (ver/editar/cancelar/enviar-ahora). */
export interface ScheduledMessage {
  id: string;
  channelId: string;
  parentId?: string;
  author: { id: string; name: string };
  body: string;
  /** Ids de ficheros de Sytue Drive a adjuntar al enviar. */
  fileIds?: string[];
  /** Instante de envío programado (ISO 8601). */
  sendAt: string;
  createdAt: string;
}

/**
 * Borrador de redacción del usuario, guardado en el SERVIDOR: lo escrito y no
 * enviado en el composer de un canal (sin parentId) o de un hilo. Es lo que hace que
 * el texto empezado en el portátil siga ahí en el móvil, y lo que alimenta la vista
 * «Borradores» del sidebar.
 */
export interface Draft {
  channelId: string;
  parentId?: string;
  body: string;
  /** Última escritura (ISO 8601), para ordenar la vista. */
  updatedAt: string;
}

/** Vista previa (OpenGraph) de una URL http/https que aparece en un mensaje (Ola 16).
 *  Campos vacíos = la URL no dio metadatos; el cliente degrada sin tarjeta. */
export interface LinkPreview {
  url: string;
  title?: string;
  description?: string;
  /** og:image; el cliente la pinta con referrerpolicy=no-referrer (sin filtrar el Referer). */
  imageUrl?: string;
  siteName?: string;
}

/** Acuse de lectura de otro miembro de un DM (Ola 16 · «Visto»): hasta cuándo ha leído
 *  el canal. lastReadAt ausente = nunca lo abrió. */
export interface Receipt {
  subject: string;
  name: string;
  /** ISO 8601; ausente si el miembro nunca abrió el canal. */
  lastReadAt?: string;
}

/** Miembro de un canal/team, para el selector de menciones del composer y de DMs. */
export interface ChannelMember {
  sub: string;
  /** Nombre legible (nunca vacio: cae al sub si el backend no lo tiene). */
  name: string;
  /** Rol en el team ('admin' | 'member'); gobierna acciones de admin en el cliente. */
  role?: 'admin' | 'member';
  /** Es de OTRA organización (ADR 0040). */
  external?: boolean;
  /** Su @usuario de Sytue One, cuando se le encontró por él. */
  username?: string;
  /** Invitación aún sin aceptar (gente de fuera): no es miembro todavía. */
  pending?: boolean;
}

/** Invitación de otra organización a un canal, un DM o un grupo (aceptar antes de entrar). */
export interface Invitacion {
  organization: string;
  /** Su nombre visible, si Sytue ID lo da; si no, se enseña `organization`. */
  organization_name?: string;
  channel_id: string;
  name: string;
  kind: 'public' | 'private' | 'direct';
  invited_by_name: string;
}

export interface MessageReaction {
  emoji: string;
  count: number;
  /** True si el usuario de la sesion ya reacciono con este emoji. */
  mine?: boolean;
  /** Subs que reaccionaron, tal y como los envía el servicio: alimenta el tooltip
   *  «Ana, Luis y 3 más reaccionaron con 👍» (lib/reactions.ts). Ausente si el
   *  servicio no los manda (entonces el chip solo muestra el conteo). */
  subs?: string[];
}

/** Resumen de un hilo SEGUIDO para la vista global «Hilos» (Ola 9): el mensaje raíz
 *  (su replyCount es el total de respuestas), la última actividad y el nº de
 *  respuestas que el usuario aún no ha leído. */
export interface ThreadSummary {
  root: ChatMessage;
  /** Fecha de la última respuesta (ISO 8601): orden de la vista y cursor keyset. */
  lastActivity: string;
  /** Respuestas de otros autores posteriores a la última vez que abriste el hilo. */
  unreadReplies: number;
}

export interface SendMessageInput {
  matrix_event?: MatrixReference;
  channelId: string;
  body: string;
  /** Idempotencia ante reintentos (lo genera el cliente). */
  clientId?: string;
  /** Subs mencionados en el mensaje (el backend los persiste). */
  mentions?: string[];
  /** Ids de ficheros de Sytue Drive a adjuntar (ya subidos vía /api/upload). */
  fileIds?: string[];
  /** 'call' publica una tarjeta de llamada de sistema (cuerpo vacío permitido). */
  type?: 'call';
  /** Prioridad del mensaje (Ola 12; por defecto 'normal'). */
  priority?: 'normal' | 'important' | 'urgent';
  /** Pide acuse de recibo explícito a los lectores (Ola 12). */
  requiresAck?: boolean;
  /** Publica el mensaje como objeto rico: encuesta o tarea. El `body` sigue siendo
   *  obligatorio: es la pregunta o el título. */
  card?: MessageCard;
}

interface ChatClientDeps {
  accessToken: string;
  /** Identificador del usuario de la sesion, para marcar `mine`. */
  userId: string;
}

function apiBase(): string {
  const env = serverEnv(import.meta.env);
  // Por el Service INTERNO: esto corre en un pod y el nombre público no
  // resuelve desde dentro (`getaddrinfo ETIMEOUT`, medido el 2026-09-10). Sale
  // como 502 —«la llamada no llegó a ser HTTP»—. Mismo patrón que
  // `SYTUE_MAIL_INTERNAL_URL`; la pública queda de respaldo para un despliegue
  // que aún no tenga la variable nueva.
  return (
    env.SYTUE_ROOMS_INTERNAL_URL ??
    env.SYTUE_ROOMS_API_URL ??
    env.SYTUE_CHAT_API_URL ??
    'https://rooms.sytue.eu'
  );
}

/** Mapea un adjunto crudo del servicio Go (snake_case) al Attachment del UI. */
export function toAttachment(a: {
  file_id: string;
  name?: string;
  content_type?: string;
  size_bytes?: number;
}): Attachment {
  return {
    fileId: a.file_id,
    name: a.name || 'archivo',
    contentType: a.content_type || 'application/octet-stream',
    sizeBytes: a.size_bytes ?? 0,
  };
}

/** Canal crudo del servicio Go: como Channel pero con category_id/sort_order/notify_level/
 *  peer_subs en snake. */
type RawChannel = Channel & {
  category_id?: string;
  sort_order?: number;
  notify_level?: string;
  peer_subs?: string[];
};

/** Mapea un canal crudo (category_id/sort_order/notify_level/peer_subs snake) al Channel
 *  del UI. El resto de campos son de una sola palabra y casan directamente. */
export function toChannel(raw: RawChannel): Channel {
  const { category_id, sort_order, notify_level, peer_subs, ...c } = raw;
  return {
    ...c,
    ...(category_id ? { categoryId: category_id } : {}),
    ...(peer_subs?.length ? { peerSubs: peer_subs } : {}),
    sortOrder: sort_order ?? 0,
    // notify_level es la fuente de verdad; el backend deja `muted` en sync (⟺ 'none').
    notifyLevel: notify_level === 'mentions' || notify_level === 'none' ? notify_level : 'all',
  };
}

/** Preferencias de notificación por usuario (Ola 13): palabras clave que disparan aviso
 *  además de las @menciones, y ventana «No molestar» en minutos del día [0..1439]
 *  (dndStart===dndEnd = desactivado; dndStart>dndEnd cruza medianoche). */
export interface NotifyPrefs {
  keywords: string[];
  dndStart: number;
  dndEnd: number;
}

/** Prefs crudas del servicio Go (snake_case). */
interface RawNotifyPrefs {
  keywords?: string[];
  dnd_start?: number;
  dnd_end?: number;
}

/** Mapea las prefs crudas del servicio Go a NotifyPrefs del UI. */
export function toNotifyPrefs(p: RawNotifyPrefs): NotifyPrefs {
  return { keywords: p.keywords ?? [], dndStart: p.dnd_start ?? 0, dndEnd: p.dnd_end ?? 0 };
}

/** Categoría cruda del servicio Go (snake_case). */
interface RawCategory {
  id: string;
  name: string;
  sort_order?: number;
  collapsed?: boolean;
}

/** Mapea una categoría cruda al Category del UI. */
export function toCategory(c: RawCategory): Category {
  return { id: c.id, name: c.name, sortOrder: c.sort_order ?? 0, collapsed: Boolean(c.collapsed) };
}

/** Miembro tal y como lo emite el servicio Go (domain.Member: subject/name/role/joined_at). */
interface RawMember {
  subject: string;
  name?: string;
  role?: string;
  external?: boolean;
  pending?: boolean;
}

/** Mapea un miembro crudo (de team o de canal) al ChannelMember del UI. */
function toMember(m: RawMember): ChannelMember {
  return {
    sub: m.subject,
    name: m.name || m.subject,
    ...(m.role === 'admin' || m.role === 'member' ? { role: m.role } : {}),
    ...(m.external ? { external: true } : {}),
    ...(m.pending ? { pending: true } : {}),
  };
}

/** Error de una llamada HTTP de Sytue Chat, con su código de estado. Lo usa el BFF para
 *  PRESERVAR los 4xx del servicio Go (p. ej. 409 «ya existe un canal con ese nombre»)
 *  en vez de colapsarlos a 502, y el cliente del navegador para distinguirlos de un
 *  fallo genérico. El mensaje es el de siempre: los `catch` existentes no cambian. */
/** Motivos de una denuncia: la misma lista cerrada que valida el servicio. */
export const REPORT_REASONS = ['harassment', 'sexual', 'violence', 'spam', 'other'] as const;
export type ReportReason = (typeof REPORT_REASONS)[number];
/** Acción que cierra una denuncia (es también el estado en que queda). */
export type ReportAction = 'dismissed' | 'message_deleted' | 'member_removed';

/** Persona bloqueada, para la lista de ajustes. */
export interface BlockedPerson {
  sub: string;
  name: string;
}

/** Denuncia vista por quien modera. No lleva quién denunció. */
export interface ModerationReport {
  id: string;
  channelId: string;
  channelName: string;
  channelKind: string;
  targetSub: string;
  targetName: string;
  messageId?: string;
  /** Texto del mensaje, leído en vivo: vacío si no hay mensaje o ya se borró. */
  messageBody: string;
  reason: string;
  note: string;
  createdAt: string;
}

interface RawBlock {
  subject: string;
  name: string;
}
interface RawReport {
  id: string;
  channel_id: string;
  channel_name: string;
  channel_kind: string;
  target_sub: string;
  target_name: string;
  message_id?: string;
  message_body?: string;
  reason: string;
  note?: string;
  created_at: string;
}
function toBlock(b: RawBlock): BlockedPerson {
  return { sub: b.subject, name: b.name || b.subject };
}
function toReport(r: RawReport): ModerationReport {
  return {
    id: r.id,
    channelId: r.channel_id,
    channelName: r.channel_name,
    channelKind: r.channel_kind,
    targetSub: r.target_sub,
    targetName: r.target_name || r.target_sub,
    ...(r.message_id ? { messageId: r.message_id } : {}),
    messageBody: r.message_body ?? '',
    reason: r.reason,
    note: r.note ?? '',
    createdAt: r.created_at,
  };
}

export class ChatHttpError extends Error {
  constructor(
    readonly status: number,
    message: string,
    /** Explicación del servicio en un 422: qué respondió el otro lado de un puente. */
    readonly detail?: string,
  ) {
    super(message);
    this.name = 'ChatHttpError';
  }
}

/**
 * Respuesta de una ruta BFF cuando el servicio falla: los 4xx se PRESERVAN (403 sin
 * permiso, 404 no existe, 409 conflicto —p. ej. último admin de un canal privado—)
 * para que la isla elija su aviso; el resto colapsa a 502. Era la misma función
 * copiada en cada ruta que la necesitaba.
 */
export function chatFail(err: unknown): Response {
  const status =
    err instanceof ChatHttpError && err.status >= 400 && err.status < 500 ? err.status : 502;
  return new Response(JSON.stringify({ error: 'chat_error' }), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Las funciones nuevas de Rooms usan el SDK común, también para streaming. */
export function chatSdk(accessToken: string): SharedChatClient {
  return new SharedChatClient({ baseUrl: apiBase(), auth: bearer(accessToken) });
}

export function chatClient({ accessToken, userId }: ChatClientDeps) {
  const base = apiBase().replace(/\/$/, '');

  async function call<T>(
    method: string,
    path: string,
    body?: unknown,
    signal?: AbortSignal,
  ): Promise<T> {
    const res = await fetch(`${base}/api/v1${path}`, {
      method,
      headers: {
        authorization: `Bearer ${accessToken}`,
        accept: 'application/json',
        ...(body !== undefined ? { 'content-type': 'application/json' } : {}),
      },
      body: body !== undefined ? JSON.stringify(body) : null,
      ...(signal ? { signal } : {}),
    });
    if (!res.ok) {
      // Solo el 422 trae una explicación que merece la pena enseñar; el resto de
      // errores sigue siendo un código.
      const detail =
        res.status === 422
          ? await res
              .json()
              .then((d: { error?: { message?: string } }) => d.error?.message)
              .catch(() => undefined)
          : undefined;
      throw new ChatHttpError(res.status, `Sytue Chat ${method} ${path} -> ${res.status}`, detail);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  /** Mensaje tal y como lo emite el servicio Go (snake_case, autor plano). */
  interface RawMessage {
    matrix_event?: MatrixReference;
    encryption?: 'm.megolm.v1.aes-sha2' | 'legacy';
    id: string;
    channel_id: string;
    parent_id?: string;
    author_sub: string;
    author_name?: string;
    body: string;
    type?: string;
    mentions?: string[];
    mentions_all?: boolean;
    attachments?: { file_id: string; name?: string; content_type?: string; size_bytes?: number }[];
    reactions?: { emoji: string; count: number; subs?: string[] }[];
    reply_count?: number;
    edited_at?: string;
    created_at: string;
    priority?: string;
    requires_ack?: boolean;
    ack_count?: number;
    acked?: boolean;
    is_bot?: boolean;
    card?: RawCard;
  }

  /** Tarjeta tal y como la emite el servicio Go (snake_case). */
  interface RawCard {
    kind?: string;
    options?: string[];
    multi?: boolean;
    assignee_sub?: string;
    assignee_name?: string;
    due?: string;
    votes?: { value: string; count: number; subs?: string[] }[];
  }

  /** Mapea la tarjeta del servicio Go a la del UI, resolviendo `mine` por sub. */
  function toCard(c: RawCard): MessageCard {
    return {
      kind: c.kind === 'task' ? 'task' : 'poll',
      ...(c.options?.length ? { options: c.options } : {}),
      ...(c.multi ? { multi: true } : {}),
      ...(c.assignee_sub ? { assigneeSub: c.assignee_sub } : {}),
      ...(c.assignee_name ? { assigneeName: c.assignee_name } : {}),
      ...(c.due ? { due: c.due } : {}),
      ...(c.votes?.length
        ? {
            votes: c.votes.map((v) => ({
              value: v.value,
              count: v.count,
              mine: Boolean(v.subs?.includes(userId)),
              ...(v.subs?.length ? { subs: v.subs } : {}),
            })),
          }
        : {}),
    };
  }

  /** Mapea el shape del servicio Go al ChatMessage del UI (camelCase, mine). */
  function toChatMessage(m: RawMessage): ChatMessage {
    return {
      id: m.id,
      channelId: m.channel_id,
      author: { id: m.author_sub, name: m.author_name || m.author_sub },
      body: m.body,
      ...(m.matrix_event ? { matrix_event: m.matrix_event } : {}),
      ...(m.encryption ? { encryption: m.encryption } : {}),
      createdAt: m.created_at,
      mine: m.author_sub === userId,
      ...(m.is_bot ? { isBot: true } : {}),
      ...(m.type === 'call' ? { type: 'call' as const } : {}),
      ...(m.priority === 'important' || m.priority === 'urgent' ? { priority: m.priority } : {}),
      ...(m.requires_ack ? { requiresAck: true } : {}),
      ...(m.ack_count ? { ackCount: m.ack_count } : {}),
      ...(m.acked ? { acked: true } : {}),
      ...(m.edited_at ? { editedAt: m.edited_at } : {}),
      ...(m.parent_id ? { parentId: m.parent_id } : {}),
      ...(m.mentions && m.mentions.length ? { mentions: m.mentions } : {}),
      ...(m.mentions_all ? { mentionsAll: true } : {}),
      ...(m.attachments && m.attachments.length
        ? { attachments: m.attachments.map(toAttachment) }
        : {}),
      ...(m.reply_count ? { replyCount: m.reply_count } : {}),
      ...(m.card?.kind ? { card: toCard(m.card) } : {}),
      ...(m.reactions
        ? {
            reactions: m.reactions.map((r) => ({
              emoji: r.emoji,
              count: r.count,
              mine: Boolean(r.subs?.includes(userId)),
              // `subs` se CONSERVA (antes se tiraba): es el «quién reaccionó».
              ...(r.subs?.length ? { subs: r.subs } : {}),
            })),
          }
        : {}),
    };
  }

  /** Resumen de hilo tal y como lo emite el servicio Go (snake_case). */
  interface RawThreadSummary {
    root: RawMessage;
    last_activity: string;
    unread_replies?: number;
  }

  /** Mapea el resumen de hilo del servicio Go al ThreadSummary del UI. */
  function toThreadSummary(t: RawThreadSummary): ThreadSummary {
    return {
      root: toChatMessage(t.root),
      lastActivity: t.last_activity,
      unreadReplies: t.unread_replies ?? 0,
    };
  }

  /** Programado tal y como lo emite el servicio Go (snake_case, autor plano). */
  interface RawScheduled {
    id: string;
    channel_id: string;
    parent_id?: string;
    author_sub: string;
    author_name?: string;
    body: string;
    file_ids?: string[];
    send_at: string;
    created_at: string;
  }

  /** Mapea el programado del servicio Go al ScheduledMessage del UI. */
  function toScheduled(s: RawScheduled): ScheduledMessage {
    return {
      id: s.id,
      channelId: s.channel_id,
      author: { id: s.author_sub, name: s.author_name || s.author_sub },
      body: s.body,
      sendAt: s.send_at,
      createdAt: s.created_at,
      ...(s.parent_id ? { parentId: s.parent_id } : {}),
      ...(s.file_ids && s.file_ids.length ? { fileIds: s.file_ids } : {}),
    };
  }

  return {
    async listChannels(): Promise<Channel[]> {
      // Los canales cuelgan de un team (backend: /teams/{id}/channels); no hay
      // lista top-level. Se resuelve primero el team del usuario. Sin teams -> [].
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) return [];
      const team = encodeURIComponent(first.id);
      // Canales + contadores durables (no leídos / menciones) en paralelo. Los
      // contadores degradan a vacío si su endpoint falla: los canales cargan igual.
      const [data, unread, deFuera] = await Promise.all([
        call<{ channels: RawChannel[] }>('GET', `/teams/${team}/channels`),
        call<{ channels: { channel_id: string; unread?: number; mentions?: number }[] }>(
          'GET',
          `/teams/${team}/unread`,
        ).catch(() => ({
          channels: [] as { channel_id: string; unread?: number; mentions?: number }[],
        })),
        // Las conversaciones de OTRAS organizaciones donde está como persona de fuera
        // (ADR 0040), con sus no leídos. Degradan a ninguna si fallan: las suyas cargan.
        call<{
          channels: RawChannel[];
          unread: { channel_id: string; unread?: number; mentions?: number }[];
        }>('GET', '/me/external-channels').catch(() => ({ channels: [], unread: [] })),
      ]);
      const counts = new Map([...unread.channels, ...deFuera.unread].map((s) => [s.channel_id, s]));
      const conContadores = (channel: Channel): Channel => {
        const s = counts.get(channel.id);
        return s ? { ...channel, unread: s.unread ?? 0, mentions: s.mentions ?? 0 } : channel;
      };
      // `external` ya viene puesto por el servicio en las de fuera.
      return [...data.channels, ...deFuera.channels].map((raw) => conContadores(toChannel(raw)));
    },

    /** Invitaciones de otras organizaciones, pendientes de aceptar. */
    async invitations(): Promise<Invitacion[]> {
      const r = await call<{ invitations?: Invitacion[] }>('GET', '/me/invitations');
      return r.invitations ?? [];
    },

    /** Acepta (entra) o rechaza una invitación. */
    async answerInvitation(channelId: string, aceptar: boolean): Promise<void> {
      const ruta = `/me/invitations/${encodeURIComponent(channelId)}`;
      await call<void>(aceptar ? 'POST' : 'DELETE', aceptar ? `${ruta}/accept` : ruta);
    },

    /** Encuentra a alguien de cualquier organización por su @usuario o su correo. */
    async resolvePerson(query: string): Promise<Persona> {
      const p = await call<{ user_id: string; name?: string; username?: string }>(
        'POST',
        '/people:resolve',
        { query },
      );
      return {
        sub: p.user_id,
        name: p.name || (p.username ? `@${p.username}` : p.user_id),
        ...(p.username ? { username: p.username } : {}),
      };
    },

    async listMessages(
      channelId: string,
      opts?: { before?: string; limit?: number },
    ): Promise<ChatMessage[]> {
      const params = new URLSearchParams();
      if (opts?.before) params.set('before', opts.before);
      params.set('limit', String(opts?.limit ?? 50));
      const data = await call<{ messages: RawMessage[] }>(
        'GET',
        `/channels/${encodeURIComponent(channelId)}/messages?${params.toString()}`,
      );
      // El servicio Go pagina newest-first (ORDER BY created_at DESC); el UI pinta
      // cronologico (viejo arriba, nuevo abajo). Se invierte aqui, el unico punto
      // por el que pasan la carga inicial y la paginacion `before` (messages[0]
      // queda como el mas antiguo cargado = cursor de la pagina anterior).
      return data.messages.map(toChatMessage).reverse();
    },

    /** Ventana de mensajes CENTRADA en messageId (permalink / scroll-to-message, Ola
     *  16). El servicio ya devuelve orden cronológico ASCENDENTE (no se invierte).
     *
     *  `threadRootId` viene cuando el mensaje pedido era una RESPUESTA de hilo: el
     *  feed del canal solo pinta raíces, así que la ventana se ancla en la raíz y hay
     *  que abrir ese hilo para enseñar el mensaje que se pidió de verdad. */
    async messagesAround(
      channelId: string,
      messageId: string,
      limit = 50,
    ): Promise<{ messages: ChatMessage[]; threadRootId?: string }> {
      const params = new URLSearchParams({ message: messageId, limit: String(limit) });
      const data = await call<{ messages: RawMessage[]; thread_root_id?: string }>(
        'GET',
        `/channels/${encodeURIComponent(channelId)}/messages/around?${params.toString()}`,
      );
      // La clave se OMITE cuando no hay raíz, en vez de ponerla a `undefined`:
      // con `exactOptionalPropertyTypes` un `?:` no acepta el undefined explícito.
      return {
        messages: (data.messages ?? []).map(toChatMessage),
        ...(data.thread_root_id ? { threadRootId: data.thread_root_id } : {}),
      };
    },

    /** Acuse de lectura de los otros miembros de un DM («Visto», Ola 16). Vacío si el
     *  canal no es un DM (el backend solo calcula receipts para directos). */
    async dmReceipts(channelId: string): Promise<Receipt[]> {
      const data = await call<{
        receipts: { subject: string; name?: string; last_read_at?: string }[];
      }>('GET', `/channels/${encodeURIComponent(channelId)}/receipts`);
      return (data.receipts ?? []).map((r) => ({
        subject: r.subject,
        name: r.name || r.subject,
        ...(r.last_read_at ? { lastReadAt: r.last_read_at } : {}),
      }));
    },

    /** Vista previa (OpenGraph) de una URL http/https, cacheada + SSRF-safe en el
     *  backend (Ola 16). Devuelve campos vacíos si la URL no da metadatos. */
    async unfurl(url: string): Promise<LinkPreview> {
      const data = await call<{
        unfurl: {
          url: string;
          title?: string;
          description?: string;
          image_url?: string;
          site_name?: string;
        };
      }>('GET', `/unfurl?url=${encodeURIComponent(url)}`);
      const u = data.unfurl ?? { url };
      return {
        url: u.url,
        ...(u.title ? { title: u.title } : {}),
        ...(u.description ? { description: u.description } : {}),
        ...(u.image_url ? { imageUrl: u.image_url } : {}),
        ...(u.site_name ? { siteName: u.site_name } : {}),
      };
    },

    async sendMessage(input: SendMessageInput): Promise<ChatMessage> {
      // Al servicio va {body, mentions?, file_ids?}: clientId es un artefacto de
      // reconciliación optimista de cliente y el servicio rechaza campos desconocidos.
      const msg = await call<RawMessage>(
        'POST',
        `/channels/${encodeURIComponent(input.channelId)}/messages`,
        {
          body: input.body,
          ...(input.matrix_event ? { matrix_event: input.matrix_event } : {}),
          ...(input.type ? { type: input.type } : {}),
          ...(input.mentions?.length ? { mentions: input.mentions } : {}),
          ...(input.fileIds?.length ? { file_ids: input.fileIds } : {}),
          ...(input.priority && input.priority !== 'normal' ? { priority: input.priority } : {}),
          ...(input.requiresAck ? { requires_ack: true } : {}),
          ...(input.card
            ? {
                card: {
                  kind: input.card.kind,
                  ...(input.card.options?.length ? { options: input.card.options } : {}),
                  ...(input.card.multi ? { multi: true } : {}),
                  ...(input.card.assigneeSub ? { assignee_sub: input.card.assigneeSub } : {}),
                  ...(input.card.assigneeName ? { assignee_name: input.card.assigneeName } : {}),
                  ...(input.card.due ? { due: input.card.due } : {}),
                },
              }
            : {}),
        },
      );
      return toChatMessage(msg);
    },

    /** Marca (add) o retira un valor de la tarjeta de un mensaje: votar una opción de
     *  la encuesta o dar por hecha la tarea. Devuelve el recuento ya agregado, para
     *  pintar el resultado sin esperar al evento WS del propio voto. */
    async voteCard(messageID: string, value: string, add: boolean): Promise<CardVote[]> {
      const data = await call<{ votes?: { value: string; count: number; subs?: string[] }[] }>(
        add ? 'PUT' : 'DELETE',
        `/messages/${encodeURIComponent(messageID)}/card/${encodeURIComponent(value)}`,
        add ? {} : undefined,
      );
      return (data.votes ?? []).map((v) => ({
        value: v.value,
        count: v.count,
        mine: Boolean(v.subs?.includes(userId)),
        ...(v.subs?.length ? { subs: v.subs } : {}),
      }));
    },

    /** Registra el acuse de recibo del usuario sobre un mensaje que lo pide;
     *  devuelve el nuevo total «Confirmado por N». Idempotente. */
    async ackMessage(messageID: string): Promise<number> {
      const data = await call<{ count?: number }>(
        'POST',
        `/messages/${encodeURIComponent(messageID)}/ack`,
        {},
      );
      return data.count ?? 0;
    },

    /** Programa un mensaje para enviarse en sendAt (RFC3339 futuro). Devuelve el
     *  programado creado (el servicio lo publicará al vencer). */
    async scheduleMessage(input: {
      channelId: string;
      body: string;
      sendAt: string;
      mentions?: string[];
      fileIds?: string[];
    }): Promise<ScheduledMessage> {
      const sm = await call<RawScheduled>(
        'POST',
        `/channels/${encodeURIComponent(input.channelId)}/messages`,
        {
          body: input.body,
          send_at: input.sendAt,
          ...(input.mentions?.length ? { mentions: input.mentions } : {}),
          ...(input.fileIds?.length ? { file_ids: input.fileIds } : {}),
        },
      );
      return toScheduled(sm);
    },

    /** Mis borradores (cross-canal), del más reciente al más antiguo. */
    async listDrafts(): Promise<Draft[]> {
      const data = await call<{
        drafts?: { channel_id: string; parent_id?: string; body: string; updated_at: string }[];
      }>('GET', '/me/drafts');
      return (data.drafts ?? []).map((d) => ({
        channelId: d.channel_id,
        body: d.body,
        updatedAt: d.updated_at,
        ...(d.parent_id ? { parentId: d.parent_id } : {}),
      }));
    },

    /** Guarda el borrador de un ámbito de redacción; cuerpo en blanco lo BORRA. */
    async putDraft(channelId: string, parentId: string | undefined, body: string): Promise<void> {
      await call<unknown>('PUT', '/me/drafts', {
        channel_id: channelId,
        body,
        ...(parentId ? { parent_id: parentId } : {}),
      });
    },

    /** Mis programados (opcionalmente por canal), por send_at ascendente. */
    async listScheduled(channelId?: string): Promise<ScheduledMessage[]> {
      const qs = channelId ? `?channel_id=${encodeURIComponent(channelId)}` : '';
      const data = await call<{ scheduled?: RawScheduled[] }>('GET', `/me/scheduled${qs}`);
      return (data.scheduled ?? []).map(toScheduled);
    },

    /** Edita un programado (cuerpo / instante / adjuntos); devuelve el actualizado. */
    async updateScheduled(
      id: string,
      patch: { body?: string; sendAt?: string; fileIds?: string[] },
    ): Promise<ScheduledMessage> {
      return toScheduled(
        await call<RawScheduled>('PATCH', `/scheduled/${encodeURIComponent(id)}`, {
          ...(patch.body !== undefined ? { body: patch.body } : {}),
          ...(patch.sendAt !== undefined ? { send_at: patch.sendAt } : {}),
          ...(patch.fileIds !== undefined ? { file_ids: patch.fileIds } : {}),
        }),
      );
    },

    /** Cancela (borra) un programado del usuario. */
    async cancelScheduled(id: string): Promise<void> {
      await call<void>('DELETE', `/scheduled/${encodeURIComponent(id)}`);
    },

    /** Hilo de un mensaje raíz: {root, replies, following}, ya en el shape del UI.
     *  `following` respalda el botón Seguir/Siguiendo; abrir el hilo lo marca visto. */
    async getThread(
      rootId: string,
    ): Promise<{ root: ChatMessage; replies: ChatMessage[]; following: boolean }> {
      const data = await call<{ root: RawMessage; replies: RawMessage[]; following?: boolean }>(
        'GET',
        `/messages/${encodeURIComponent(rootId)}/thread`,
      );
      return {
        root: toChatMessage(data.root),
        replies: (data.replies ?? []).map(toChatMessage),
        following: Boolean(data.following),
      };
    },

    /** Sigue el hilo del mensaje raíz (para recibir sus no-leídos en «Hilos»). */
    async followThread(rootId: string): Promise<void> {
      await call<void>('POST', `/messages/${encodeURIComponent(rootId)}/follow`, {});
    },

    /** Deja de seguir el hilo del mensaje raíz. */
    async unfollowThread(rootId: string): Promise<void> {
      await call<void>('DELETE', `/messages/${encodeURIComponent(rootId)}/follow`);
    },

    /** Hilos que sigo (vista global), keyset por última actividad. Cada uno con su
     *  raíz, total de respuestas (root.replyCount) y respuestas no leídas. */
    async listThreads(opts?: {
      before?: string;
      limit?: number;
    }): Promise<{ threads: ThreadSummary[]; nextBefore?: string }> {
      const params = new URLSearchParams();
      if (opts?.before) params.set('before', opts.before);
      params.set('limit', String(opts?.limit ?? 50));
      const data = await call<{ threads: RawThreadSummary[]; next_before?: string }>(
        'GET',
        `/me/threads?${params.toString()}`,
      );
      return {
        threads: (data.threads ?? []).map(toThreadSummary),
        ...(data.next_before ? { nextBefore: data.next_before } : {}),
      };
    },

    /** Publica una respuesta dentro del hilo de un mensaje raíz. */
    async postReply(
      rootId: string,
      input: {
        body: string;
        matrix_event?: MatrixReference;
        mentions?: string[];
        fileIds?: string[];
      },
    ): Promise<ChatMessage> {
      const msg = await call<RawMessage>(
        'POST',
        `/messages/${encodeURIComponent(rootId)}/replies`,
        {
          body: input.body,
          ...(input.matrix_event ? { matrix_event: input.matrix_event } : {}),
          ...(input.mentions?.length ? { mentions: input.mentions } : {}),
          ...(input.fileIds?.length ? { file_ids: input.fileIds } : {}),
        },
      );
      return toChatMessage(msg);
    },

    /** Edita el cuerpo de un mensaje propio (PATCH); devuelve el mensaje con editedAt. */
    async editMessage(
      messageID: string,
      body: string,
      matrix_event?: MatrixReference,
    ): Promise<ChatMessage> {
      const msg = await call<RawMessage>('PATCH', `/messages/${encodeURIComponent(messageID)}`, {
        body,
        ...(matrix_event ? { matrix_event } : {}),
      });
      return toChatMessage(msg);
    },

    /** Borra un mensaje propio (DELETE, borrado suave en el backend). */
    async deleteMessage(messageID: string): Promise<void> {
      await call<void>('DELETE', `/messages/${encodeURIComponent(messageID)}`);
    },

    /**
     * Quién ha visto un mensaje y cuándo, más cuánta gente hay en el canal aparte
     * del autor. El backend lo saca de «leído hasta aquí» (`read_state`), no de
     * una fila por mensaje y persona.
     */
    async messageReaders(messageID: string): Promise<{ readers: MessageReader[]; total: number }> {
      const r = await call<{ readers?: MessageReader[]; total?: number }>(
        'GET',
        `/messages/${encodeURIComponent(messageID)}/readers`,
      );
      return { readers: r.readers ?? [], total: r.total ?? 0 };
    },

    /** ¿Puede el usuario administrar recursos de todo el workspace (integraciones,
     *  emoji, bots)? Es la misma definición que aplica el backend en isWorkspaceAdmin:
     *  ser admin de ALGÚN team del tenant. Se pregunta para no ofrecer lo que el
     *  servidor va a denegar. Ante un fallo de red devuelve false: esconder una opción
     *  de administración es mejor error que enseñar una que acabará en «no se pudo
     *  cargar». */
    async isWorkspaceAdmin(): Promise<boolean> {
      const { teams } = await call<{ teams: { role?: string }[] }>('GET', '/teams').catch(() => ({
        teams: [] as { role?: string }[],
      }));
      return teams.some((t) => t.role === 'admin');
    },

    /** Miembros del team del usuario (para el selector de menciones y de DMs). */
    async listMembers(): Promise<ChannelMember[]> {
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) return [];
      const data = await call<{ members: RawMember[] }>(
        'GET',
        `/teams/${encodeURIComponent(first.id)}/members`,
      );
      return data.members.map(toMember);
    },

    /** Fija favorito y/o nivel de notificación del canal para el usuario actual; devuelve
     *  el canal. `muted` (legado) lo traduce el backend a notify_level='none'/'all'. */
    async setChannelPrefs(
      channelId: string,
      prefs: { favorite?: boolean; muted?: boolean; notifyLevel?: 'all' | 'mentions' | 'none' },
    ): Promise<Channel> {
      const { notifyLevel, ...rest } = prefs;
      return toChannel(
        await call<RawChannel>('POST', `/channels/${encodeURIComponent(channelId)}/prefs`, {
          ...rest,
          ...(notifyLevel !== undefined ? { notify_level: notifyLevel } : {}),
        }),
      );
    },

    /** Mis preferencias de notificación (palabras clave + «No molestar»). */
    async getNotifyPrefs(): Promise<NotifyPrefs> {
      return toNotifyPrefs(await call<RawNotifyPrefs>('GET', '/me/notify-prefs'));
    },

    /** Fija mis preferencias de notificación (el backend las sanea); devuelve las
     *  resultantes. PUT server-to-server (sin CSRF de navegador). */
    async putNotifyPrefs(prefs: NotifyPrefs): Promise<NotifyPrefs> {
      return toNotifyPrefs(
        await call<RawNotifyPrefs>('PUT', '/me/notify-prefs', {
          keywords: prefs.keywords,
          dnd_start: prefs.dndStart,
          dnd_end: prefs.dndEnd,
        }),
      );
    },

    // ---- Ola 18: Web Push ----

    /** Clave pública VAPID (applicationServerKey del navegador). Vacía = push
     *  no configurado en el servidor (el cliente oculta el toggle). */
    async getPushKey(): Promise<string> {
      const d = await call<{ key?: string }>('GET', '/me/push/key');
      return d.key ?? '';
    },

    /** Registra (upsert) la suscripción Web Push del navegador del usuario. */
    async subscribePush(sub: {
      endpoint: string;
      keys: { p256dh: string; auth: string };
    }): Promise<void> {
      await call<void>('POST', '/me/push/subscribe', sub);
    },

    /** Da de baja la suscripción Web Push (por endpoint). */
    async unsubscribePush(endpoint: string): Promise<void> {
      await call<void>('POST', '/me/push/unsubscribe', { endpoint });
    },

    // ---- Ola 11: categorías del sidebar (per-usuario) ----

    /** Mis categorías del sidebar (ordenadas). */
    async listCategories(): Promise<Category[]> {
      const data = await call<{ categories: RawCategory[] }>('GET', '/me/categories');
      return (data.categories ?? []).map(toCategory);
    },

    /** Crea una categoría del sidebar (al final del orden); devuelve la creada. */
    async createCategory(name: string): Promise<Category> {
      return toCategory(await call<RawCategory>('POST', '/me/categories', { name }));
    },

    /** Renombra, colapsa/expande y/o reordena una categoría; devuelve la actualizada. */
    async updateCategory(
      id: string,
      patch: { name?: string; sortOrder?: number; collapsed?: boolean },
    ): Promise<Category> {
      return toCategory(
        await call<RawCategory>('PATCH', `/me/categories/${encodeURIComponent(id)}`, {
          ...(patch.name !== undefined ? { name: patch.name } : {}),
          ...(patch.sortOrder !== undefined ? { sort_order: patch.sortOrder } : {}),
          ...(patch.collapsed !== undefined ? { collapsed: patch.collapsed } : {}),
        }),
      );
    },

    /** Borra una categoría; sus canales recaen a la categoría por defecto. */
    async deleteCategory(id: string): Promise<void> {
      await call<void>('DELETE', `/me/categories/${encodeURIComponent(id)}`);
    },

    /** Asigna el canal a una categoría del sidebar (categoryId null = por defecto) y,
     *  opcionalmente, fija su orden. Devuelve el canal. */
    async setChannelCategory(
      channelId: string,
      categoryId: string | null,
      sortOrder?: number,
    ): Promise<Channel> {
      return toChannel(
        await call<RawChannel>('POST', `/channels/${encodeURIComponent(channelId)}/category`, {
          category_id: categoryId ?? '',
          ...(sortOrder !== undefined ? { sort_order: sortOrder } : {}),
        }),
      );
    },

    /** Crea un canal en el team del usuario. El creador queda como admin del canal (lo
     *  siembra el backend). Un 409 significa que ya hay un canal con ese nombre.
     *  ponytail: sin `members[]` (el backend lo acepta) — no hay UI de miembros de canal
     *  todavía; se añade cuando exista el panel «Miembros». */
    async createChannel(input: {
      name: string;
      kind: 'public' | 'private';
      topic?: string;
    }): Promise<Channel> {
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) throw new Error('Sin team para crear un canal');
      return toChannel(
        await call<RawChannel>('POST', `/teams/${encodeURIComponent(first.id)}/channels`, {
          name: input.name,
          kind: input.kind,
          ...(input.topic ? { topic: input.topic } : {}),
        }),
      );
    },

    /** Canales públicos del team a los que el usuario NO pertenece (explorar). */
    async browseChannels(query: string): Promise<Channel[]> {
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) return [];
      const qs = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : '';
      const data = await call<{ channels: Channel[] }>(
        'GET',
        `/teams/${encodeURIComponent(first.id)}/channels/browse${qs}`,
      );
      return data.channels ?? [];
    },

    /** Une al usuario a un canal público; devuelve el canal. */
    async joinChannel(channelId: string): Promise<Channel> {
      return call<Channel>('POST', `/channels/${encodeURIComponent(channelId)}/join`, {});
    },

    /** Actualiza un canal (tema / propósito / archivar / renombrar; solo admin). */
    async updateChannel(
      channelId: string,
      patch: { name?: string; topic?: string; purpose?: string; archived?: boolean },
    ): Promise<Channel> {
      return call<Channel>('PATCH', `/channels/${encodeURIComponent(channelId)}`, patch);
    },

    /** Abre (o reutiliza) un DM (1:1 o de grupo) con otros miembros del team; devuelve
     *  el canal. `subjects` son los subs de los OTROS participantes (idempotente por
     *  conjunto en el backend). */
    async openDirect(subjects: string[]): Promise<Channel> {
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) throw new Error('Sin team para abrir un DM');
      return call<Channel>('POST', `/teams/${encodeURIComponent(first.id)}/dms`, { subjects });
    },

    async markRead(channelId: string): Promise<void> {
      await call<void>('POST', `/channels/${encodeURIComponent(channelId)}/read`, {});
    },

    async addReaction(messageID: string, emoji: string): Promise<void> {
      await call<void>(
        'PUT',
        `/messages/${encodeURIComponent(messageID)}/reactions/${encodeURIComponent(emoji)}`,
      );
    },

    async removeReaction(messageID: string, emoji: string): Promise<void> {
      await call<void>(
        'DELETE',
        `/messages/${encodeURIComponent(messageID)}/reactions/${encodeURIComponent(emoji)}`,
      );
    },

    // ---- Fijados (pinned) / Guardados (saved) / Marcar como no leído ----

    /** Fija un mensaje; devuelve el mensaje fijado (para el panel/optimista). */
    async pinMessage(messageID: string): Promise<ChatMessage> {
      const msg = await call<RawMessage>(
        'POST',
        `/messages/${encodeURIComponent(messageID)}/pin`,
        {},
      );
      return toChatMessage(msg);
    },

    /** Desfija un mensaje. */
    async unpinMessage(messageID: string): Promise<void> {
      await call<void>('DELETE', `/messages/${encodeURIComponent(messageID)}/pin`);
    },

    /** Mensajes fijados de un canal (más reciente primero). */
    async listPins(channelId: string): Promise<ChatMessage[]> {
      const data = await call<{ pins: RawMessage[] }>(
        'GET',
        `/channels/${encodeURIComponent(channelId)}/pins`,
      );
      return (data.pins ?? []).map(toChatMessage);
    },

    /** Guarda un mensaje (marcador por-usuario). */
    async saveMessage(messageID: string): Promise<void> {
      await call<void>('POST', `/messages/${encodeURIComponent(messageID)}/save`, {});
    },

    /** Quita un mensaje de guardados. */
    async unsaveMessage(messageID: string): Promise<void> {
      await call<void>('DELETE', `/messages/${encodeURIComponent(messageID)}/save`);
    },

    /** Mis guardados, cross-canal, con paginación keyset por fecha de guardado. */
    async listSaved(opts?: {
      before?: string;
      limit?: number;
    }): Promise<{ messages: ChatMessage[]; nextBefore?: string }> {
      const params = new URLSearchParams();
      if (opts?.before) params.set('before', opts.before);
      params.set('limit', String(opts?.limit ?? 50));
      const data = await call<{ messages: RawMessage[]; next_before?: string }>(
        'GET',
        `/me/saved?${params.toString()}`,
      );
      return {
        messages: (data.messages ?? []).map(toChatMessage),
        ...(data.next_before ? { nextBefore: data.next_before } : {}),
      };
    },

    /** Deja el canal del mensaje como no leído a partir de ese mensaje. */
    async markUnread(messageID: string): Promise<void> {
      await call<void>('POST', `/messages/${encodeURIComponent(messageID)}/unread`, {});
    },

    /**
     * Búsqueda global, cross-team: mensajes de todos los canales accesibles al
     * usuario (el backend respeta el acceso y acota por LIMIT). La query puede
     * llevar operadores from:/in:/before:/after:, que se pasan tal cual (el servicio
     * Go los parsea). El nombre del canal NO viaja en el mensaje: se resuelve en el
     * cliente contra la lista de canales.
     */
    async searchAll(query: string, limit = 30): Promise<ChatMessage[]> {
      const params = new URLSearchParams({ q: query, limit: String(limit) });
      const data = await call<{ messages: RawMessage[] }>('GET', `/search?${params.toString()}`);
      return (data.messages ?? []).map(toChatMessage);
    },

    // ---- Ola 14: emoji personalizados + abandonar canal + invitar al team ----

    /** Emoji personalizados del tenant (nombre + fileId de Drive). */
    async listCustomEmoji(signal?: AbortSignal): Promise<CustomEmoji[]> {
      const data = await call<{
        emoji: { name: string; file_id: string; content_type?: string }[];
      }>('GET', '/emoji', undefined, signal);
      return (data.emoji ?? []).map((e) => ({
        name: e.name,
        fileId: e.file_id,
        ...(e.content_type ? { contentType: e.content_type } : {}),
      }));
    },

    /** Crea un emoji personalizado ligando el shortcode `name` a un fichero de Drive. */
    async createCustomEmoji(
      name: string,
      fileId: string,
      signal?: AbortSignal,
    ): Promise<CustomEmoji> {
      const e = await call<{ name: string; file_id: string; content_type?: string }>(
        'POST',
        '/emoji',
        { name, file_id: fileId },
        signal,
      );
      return {
        name: e.name,
        fileId: e.file_id,
        ...(e.content_type ? { contentType: e.content_type } : {}),
      };
    },

    /** Borra un emoji personalizado por nombre (autor o admin del workspace). */
    async deleteCustomEmoji(name: string): Promise<void> {
      await call<void>('DELETE', `/emoji/${encodeURIComponent(name)}`);
    },

    /** Abandona un canal (el usuario deja de ser miembro). */
    async leaveChannel(channelId: string): Promise<void> {
      await call<void>('POST', `/channels/${encodeURIComponent(channelId)}/leave`, {});
    },

    /** Borra un canal público o privado (admin del canal o del equipo; 403 en un DM). */
    async deleteChannel(channelId: string): Promise<void> {
      await call<void>('DELETE', `/channels/${encodeURIComponent(channelId)}`);
    },

    /** «Borra el chat» de un DM solo para quien lo pide: sale de su barra y se le
     *  esconde lo anterior; los demás lo conservan. Reaparece si alguien escribe. */
    async hideDirect(channelId: string): Promise<void> {
      await call<void>('POST', `/channels/${encodeURIComponent(channelId)}/hide`, {});
    },

    // ---- Roster del canal (Ola 2C): el `role` de estos ChannelMember es el rol DEL
    // CANAL (admin de canal), no el del team. Un DM no tiene roster gestionable (403)
    // y un canal archivado responde 409.

    /** Miembros del canal, ya ordenados por nombre en el backend. */
    async listChannelMembers(channelId: string): Promise<ChannelMember[]> {
      const data = await call<{ members: RawMember[] }>(
        'GET',
        `/channels/${encodeURIComponent(channelId)}/members`,
      );
      return (data.members ?? []).map(toMember);
    },

    /** Añade al canal a alguien que YA pertenezca al team (si no, el servicio da 403).
     *  Exige ser admin del canal o del team. */
    async addChannelMember(
      channelId: string,
      subject: string,
      name?: string,
    ): Promise<ChannelMember> {
      return toMember(
        await call<RawMember>('POST', `/channels/${encodeURIComponent(channelId)}/members`, {
          subject,
          ...(name ? { name } : {}),
        }),
      );
    },

    // ---- Denunciar y bloquear (ADR 0073) ----

    /** Denuncia un mensaje (con `messageId`) o a una persona en un canal. */
    async report(input: {
      messageId?: string;
      channelId?: string;
      subject?: string;
      reason: string;
      note?: string;
    }): Promise<void> {
      const path = input.messageId
        ? `/messages/${encodeURIComponent(input.messageId)}/report`
        : `/channels/${encodeURIComponent(input.channelId ?? '')}/members/${encodeURIComponent(input.subject ?? '')}/report`;
      await call<unknown>('POST', path, { reason: input.reason, note: input.note ?? '' });
    },

    /** Bloquea a alguien con quien se comparte el canal. */
    async blockMember(channelId: string, subject: string): Promise<BlockedPerson> {
      const b = await call<RawBlock>(
        'PUT',
        `/channels/${encodeURIComponent(channelId)}/members/${encodeURIComponent(subject)}/block`,
      );
      return toBlock(b);
    },

    /** A quién tiene bloqueado quien llama. */
    async listBlocks(): Promise<BlockedPerson[]> {
      const data = await call<{ items: RawBlock[] }>('GET', '/me/blocks');
      return (data.items ?? []).map(toBlock);
    },

    async unblock(subject: string): Promise<void> {
      await call<void>('DELETE', `/me/blocks/${encodeURIComponent(subject)}`);
    },

    /** Cola de denuncias que quien llama puede moderar. */
    async listReports(): Promise<ModerationReport[]> {
      const data = await call<{ items: RawReport[] }>('GET', '/moderation/reports');
      return (data.items ?? []).map(toReport);
    },

    async resolveReport(reportId: string, action: ReportAction): Promise<void> {
      await call<unknown>('POST', `/moderation/reports/${encodeURIComponent(reportId)}/resolve`, {
        action,
      });
    },

    /** Saca a alguien del canal. Admin del canal o del team; o uno mismo. */
    async removeChannelMember(channelId: string, subject: string): Promise<void> {
      await call<void>(
        'DELETE',
        `/channels/${encodeURIComponent(channelId)}/members/${encodeURIComponent(subject)}`,
      );
    },

    /** Fija el rol de canal de un miembro (admin de canal ↔ miembro). */
    async setChannelMemberRole(
      channelId: string,
      subject: string,
      role: 'admin' | 'member',
    ): Promise<ChannelMember> {
      return toMember(
        await call<RawMember>(
          'PATCH',
          `/channels/${encodeURIComponent(channelId)}/members/${encodeURIComponent(subject)}`,
          { role },
        ),
      );
    },

    /** Añade a un miembro al team del usuario (invitar). Resuelve el team primero,
     *  igual que openDirect; el backend exige rol admin. */
    async addTeamMember(subject: string, name: string): Promise<void> {
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) throw new Error('Sin team para invitar');
      await call<void>('POST', `/teams/${encodeURIComponent(first.id)}/members`, { subject, name });
    },

    /** Da de BAJA a alguien del team (no de un canal suelto): deja de ver los canales
     *  públicos y sale del directorio de menciones. Resuelve el team igual que
     *  addTeamMember; el backend exige rol admin. */
    async removeTeamMember(subject: string): Promise<void> {
      const { teams } = await call<{ teams: { id: string }[] }>('GET', '/teams');
      const first = teams[0];
      if (!first) throw new Error('Sin team del que dar de baja');
      await call<void>(
        'DELETE',
        `/teams/${encodeURIComponent(first.id)}/members/${encodeURIComponent(subject)}`,
      );
    },

    // ---- Ola 15: integraciones (webhooks entrantes/salientes + bots) ----

    /** Webhooks entrantes de un canal (admin del canal). */
    async listIncomingWebhooks(channelId: string): Promise<IncomingWebhook[]> {
      const data = await call<{ webhooks: RawIncoming[] }>(
        'GET',
        `/channels/${encodeURIComponent(channelId)}/webhooks/incoming`,
      );
      return (data.webhooks ?? []).map(toIncoming);
    },

    /**
     * Crea un webhook entrante; devuelve el token y la URL en CLARO una única vez y, si
     * el correo a canal está encendido en el servicio, su dominio.
     */
    async createIncomingWebhook(
      channelId: string,
      name: string,
      icon?: string,
    ): Promise<IncomingSecret> {
      const data = await call<RawIncomingSecret>(
        'POST',
        `/channels/${encodeURIComponent(channelId)}/webhooks/incoming`,
        { name, ...(icon ? { icon } : {}) },
      );
      return toIncomingSecret(data);
    },

    /** Borra un webhook entrante (admin de su canal). */
    async deleteIncomingWebhook(id: string): Promise<void> {
      await call<void>('DELETE', `/webhooks/incoming/${encodeURIComponent(id)}`);
    },

    /** Rota el token de un webhook entrante; lo mismo que al crearlo, con el token nuevo. */
    async rotateIncomingWebhook(id: string): Promise<IncomingSecret> {
      const data = await call<RawIncomingSecret>(
        'POST',
        `/webhooks/incoming/${encodeURIComponent(id)}/rotate`,
        {},
      );
      return toIncomingSecret(data);
    },

    /** Webhooks salientes gestionables por el actor. */
    async listOutgoingWebhooks(): Promise<OutgoingWebhook[]> {
      const data = await call<{ webhooks: RawOutgoing[] }>('GET', '/webhooks/outgoing');
      return (data.webhooks ?? []).map(toOutgoing);
    },

    /** Crea un webhook saliente (canal y/o palabras disparadoras; una al menos). */
    async createOutgoingWebhook(input: {
      channelId?: string;
      callbackUrl: string;
      triggerWords?: string[];
      /**
       * Formato del POST y de la respuesta esperada. El servicio lo persiste desde la
       * migración 0017 y `compat.NormalizeOutgoing` lo aplica; sin pasarlo aquí, TODO
       * saliente salía en dialecto nativo y un bot escrito para Slack no lo entendía.
       */
      dialect?: OutgoingDialect;
    }): Promise<OutgoingWebhook> {
      const data = await call<{ webhook: RawOutgoing }>('POST', '/webhooks/outgoing', {
        callback_url: input.callbackUrl,
        ...(input.channelId ? { channel_id: input.channelId } : {}),
        ...(input.triggerWords?.length ? { trigger_words: input.triggerWords } : {}),
        // 'sytue' es el valor por defecto del servicio: no hace falta enviarlo.
        ...(input.dialect && input.dialect !== 'sytue' ? { dialect: input.dialect } : {}),
      });
      return toOutgoing(data.webhook);
    },

    /** Borra un webhook saliente. */
    async deleteOutgoingWebhook(id: string): Promise<void> {
      await call<void>('DELETE', `/webhooks/outgoing/${encodeURIComponent(id)}`);
    },

    /** Puentes del canal (admin del canal). */
    async listBridges(channelId: string): Promise<Bridge[]> {
      const data = await call<{ bridges: RawBridge[] }>(
        'GET',
        `/channels/${encodeURIComponent(channelId)}/bridges`,
      );
      return (data.bridges ?? []).map(toBridge);
    },

    /** Desconecta un puente. */
    async deleteBridge(id: string): Promise<void> {
      await call<void>('DELETE', `/bridges/${encodeURIComponent(id)}`);
    },

    /** Equipos del usuario, con su nombre (para elegir dónde atender WhatsApp). */
    async listTeamNames(): Promise<{ id: string; name: string }[]> {
      const { teams } = await call<{ teams: { id: string; name: string }[] }>('GET', '/teams');
      return (teams ?? []).map((t) => ({ id: t.id, name: t.name }));
    },

    /** Números de WhatsApp conectados (admin del workspace). */
    async listWhatsApp(): Promise<WhatsAppAccount[]> {
      const data = await call<{ accounts: RawWhatsApp[] }>('GET', '/whatsapp');
      return (data.accounts ?? []).map(toWhatsApp);
    },

    /** Desconecta un número. */
    async disconnectWhatsApp(id: string): Promise<void> {
      await call<void>('DELETE', `/whatsapp/${encodeURIComponent(id)}`);
    },

    /** Bots del tenant (admin del workspace). */
    async listBots(): Promise<Bot[]> {
      const data = await call<{ bots: RawBot[] }>('GET', '/bots');
      return (data.bots ?? []).map(toBot);
    },

    /** Crea un bot; devuelve su token Bearer en CLARO una única vez. */
    async createBot(name: string): Promise<{ bot: Bot; token: string }> {
      const data = await call<{ bot: RawBot; token: string }>('POST', '/bots', { name });
      return { bot: toBot(data.bot), token: data.token };
    },

    /** Borra un bot (su dueño o admin del workspace). */
    async deleteBot(id: string): Promise<void> {
      await call<void>('DELETE', `/bots/${encodeURIComponent(id)}`);
    },

    /** Añade un bot a un canal para que pueda publicar (admin del canal). */
    async addBotToChannel(botId: string, channelId: string): Promise<void> {
      await call<void>(
        'POST',
        `/bots/${encodeURIComponent(botId)}/channels/${encodeURIComponent(channelId)}`,
        {},
      );
    },

    /** Quita un bot de un canal. */
    async removeBotFromChannel(botId: string, channelId: string): Promise<void> {
      await call<void>(
        'DELETE',
        `/bots/${encodeURIComponent(botId)}/channels/${encodeURIComponent(channelId)}`,
      );
    },

    // --- Slash commands de terceros ---------------------------------------

    /** Comandos registrados por la organización. Los secretos solo llegan a un admin. */
    async listCommands(): Promise<ServerCommand[]> {
      const data = await call<{ commands: RawCommand[] }>('GET', '/commands');
      return (data.commands ?? []).map(toCommand);
    },

    /** Registra un comando (admin). Devuelve también los secretos, una sola vez visibles. */
    async createCommand(input: {
      name: string;
      requestUrl: string;
      description?: string;
      usageHint?: string;
    }): Promise<ServerCommand> {
      const data = await call<{ command: RawCommand }>('POST', '/commands', {
        name: input.name,
        request_url: input.requestUrl,
        ...(input.description ? { description: input.description } : {}),
        ...(input.usageHint ? { usage_hint: input.usageHint } : {}),
      });
      return toCommand(data.command);
    },

    /** Borra un comando (admin). */
    async deleteCommand(id: string): Promise<void> {
      await call<void>('DELETE', `/commands/${encodeURIComponent(id)}`);
    },

    /**
     * Ejecuta un comando: el servicio llama al tercero (firmando la petición) y trae
     * su respuesta. `input` es la línea entera —«/desplegar prod»— porque separar el
     * nombre de los argumentos es regla del servicio, no del cliente.
     */
    async executeCommand(channelId: string, input: string): Promise<CommandResult> {
      return call<CommandResult>('POST', '/commands/execute', {
        channel_id: channelId,
        input,
      });
    },
  };
}

/** Un comando de servidor tal como lo ve la interfaz. */
export interface ServerCommand {
  id: string;
  name: string;
  description?: string;
  usageHint?: string;
  requestUrl: string;
  /** Solo presentes para un admin: el servicio los recorta para el resto. */
  token?: string;
  signingSecret?: string;
  createdAt: string;
}

/** Respuesta de ejecutar un comando. */
export interface CommandResult {
  /** Texto de la respuesta. Vacío = el comando aceptó y contestará por su cuenta. */
  text: string;
  /** 'in_channel' = ya se publicó en el canal; 'ephemeral' = solo lo ve quien ejecutó. */
  response_type: string;
  message_id?: string;
}

interface RawCommand {
  id: string;
  name: string;
  description?: string;
  usage_hint?: string;
  request_url: string;
  token?: string;
  signing_secret?: string;
  created_at: string;
}

function toCommand(c: RawCommand): ServerCommand {
  return {
    id: c.id,
    name: c.name,
    requestUrl: c.request_url,
    createdAt: c.created_at,
    ...(c.description ? { description: c.description } : {}),
    ...(c.usage_hint ? { usageHint: c.usage_hint } : {}),
    ...(c.token ? { token: c.token } : {}),
    ...(c.signing_secret ? { signingSecret: c.signing_secret } : {}),
  };
}

/** Formas crudas (snake_case) de las integraciones tal y como las emite el servicio Go. */
interface RawIncoming {
  id: string;
  channel_id: string;
  name: string;
  icon?: string;
  created_at: string;
}
interface RawIncomingSecret {
  webhook: RawIncoming;
  token: string;
  url: string;
  channel_mail_domain?: string;
}
interface RawOutgoing {
  id: string;
  channel_id?: string;
  trigger_words?: string[];
  callback_url: string;
  token?: string;
  dialect?: string;
  signing_secret?: string;
  created_at: string;
}
interface RawBot {
  id: string;
  name: string;
  created_at: string;
}
interface RawWhatsApp {
  id: string;
  team_id: string;
  phone_number_id: string;
  display_phone: string;
  verified_name?: string;
  created_at: string;
}
interface RawBridge {
  id: string;
  channel_id: string;
  platform: BridgePlatform;
  target: string;
  last_error?: string;
  last_error_at?: string;
  created_at: string;
}

function toIncoming(w: RawIncoming): IncomingWebhook {
  return {
    id: w.id,
    channelId: w.channel_id,
    name: w.name,
    createdAt: w.created_at,
    ...(w.icon ? { icon: w.icon } : {}),
  };
}
function toIncomingSecret(s: RawIncomingSecret): IncomingSecret {
  return {
    webhook: toIncoming(s.webhook),
    token: s.token,
    url: s.url,
    ...(s.channel_mail_domain ? { channelMailDomain: s.channel_mail_domain } : {}),
  };
}
function toOutgoing(w: RawOutgoing): OutgoingWebhook {
  return {
    id: w.id,
    callbackUrl: w.callback_url,
    createdAt: w.created_at,
    ...(w.channel_id ? { channelId: w.channel_id } : {}),
    ...(w.trigger_words?.length ? { triggerWords: w.trigger_words } : {}),
    ...(w.token ? { token: w.token } : {}),
    ...(w.signing_secret ? { signingSecret: w.signing_secret } : {}),
    // Un valor que no reconozcamos se trata como nativo en vez de propagarse: la UI
    // pinta una etiqueta a partir de esto y no debe enseñar una cadena del servidor.
    ...(w.dialect === 'slack' || w.dialect === 'teams' ? { dialect: w.dialect } : {}),
  };
}
function toWhatsApp(a: RawWhatsApp): WhatsAppAccount {
  return {
    id: a.id,
    teamId: a.team_id,
    phoneNumberId: a.phone_number_id,
    displayPhone: a.display_phone,
    createdAt: a.created_at,
    ...(a.verified_name ? { verifiedName: a.verified_name } : {}),
  };
}
function toBridge(b: RawBridge): Bridge {
  return {
    id: b.id,
    channelId: b.channel_id,
    platform: b.platform,
    target: b.target,
    createdAt: b.created_at,
    ...(b.last_error ? { lastError: b.last_error } : {}),
    ...(b.last_error_at ? { lastErrorAt: b.last_error_at } : {}),
  };
}
function toBot(b: RawBot): Bot {
  return { id: b.id, name: b.name, createdAt: b.created_at };
}

export type ChatClient = ReturnType<typeof chatClient>;
