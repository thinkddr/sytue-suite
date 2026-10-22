/**
 * Cliente de navegador de Sytue Chat.
 *
 * REST: habla con los endpoints BFF same-origin (cookie); el servidor reenvia al
 * servicio Go Sytue Chat con el token federado, asi el access token no toca el
 * navegador para los datos.
 *
 * Tiempo real: abre un WebSocket directo contra el servicio Go usando un ticket
 * de corta duracion que entrega el BFF (/api/ws-ticket). Reconecta con backoff.
 *
 * Reexporta los tipos de dominio desde src/lib/chat.ts.
 */
import { roomsMatrix } from './matrix/client.js';
import {
  decryptPayload,
  decryptMessage,
  knownMessage,
  encryptedFiles,
  rememberUpload,
  captureMatrixScope,
} from './matrix/messages.js';
import { MatrixSecurityError } from './matrix/types.js';
import type {
  Channel,
  ChatMessage,
  ChannelMember,
  Attachment,
  ThreadSummary,
  Category,
  ScheduledMessage,
  Draft,
  NotifyPrefs,
  CustomEmoji,
  IncomingWebhook,
  IncomingSecret,
  OutgoingWebhook,
  OutgoingDialect,
  Bot,
  Bridge,
  BridgePlatform,
  WhatsAppAccount,
  ServerCommand,
  LinkPreview,
  Receipt,
  MessageCard,
  CardVote,
  Persona,
} from '../lib/chat.js';
import { toAttachment, ChatHttpError } from '../lib/chat.js';
import type { Invitacion } from '../lib/chat.js';
/*
 * `withBase` en la vuelta al login: la app se sirve bajo su prefijo en el dominio
 * de organización (`<org>.sytue.eu/mail`, `/calendar`…), y ahí `/auth/login` a pelo
 * es la raíz del host, que devuelve el 404 de Traefik. O sea: al caducar la sesión
 * se acababa en una página de error en vez de en el formulario. El shim de `fetch`
 * (packages/ui/src/basePath.ts) no llega hasta aquí porque esto no es un fetch.
 */
import { withBase } from '@sytue/ui';
import { appUrl, orgActual } from '@sytue/config';
import type { FreeGif, FreeGifPage } from '@sytue/api-clients';

export { ChatHttpError };

export type { Invitacion } from '../lib/chat.js';
export type {
  Channel,
  ChatMessage,
  ChannelMember,
  Attachment,
  ThreadSummary,
  Category,
  ScheduledMessage,
  Draft,
  NotifyPrefs,
  CustomEmoji,
  IncomingWebhook,
  IncomingSecret,
  OutgoingWebhook,
  OutgoingDialect,
  Bot,
  Bridge,
  BridgePlatform,
  WhatsAppAccount,
  ServerCommand,
  LinkPreview,
  Receipt,
  MessageCard,
  CardVote,
  Persona,
};

async function getJson<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, {
    credentials: 'include',
    headers: { accept: 'application/json' },
    ...(signal ? { signal } : {}),
  });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  // ChatHttpError lleva el código de estado (el 4xx que el BFF preserva del servicio Go),
  // así el llamador puede distinguir p. ej. un 409 de nombre duplicado de un fallo
  // genérico. El mensaje es el mismo de antes: los `catch` existentes no cambian.
  if (!res.ok) throw new ChatHttpError(res.status, `Error ${res.status}`);
  return decryptPayload((await res.json()) as T);
}

async function postJson<T>(url: string, body: unknown, signal?: AbortSignal): Promise<T> {
  if (
    (url.startsWith('/api/scheduled') && !(body && typeof body === 'object' && 'cancel' in body)) ||
    url === '/api/command' ||
    url === '/api/card-vote' ||
    url === '/api/forward'
  )
    throw new MatrixSecurityError('unsupported');
  const res = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(body),
    ...(signal ? { signal } : {}),
  });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  if (!res.ok) throw new ChatHttpError(res.status, `Error ${res.status}`);
  return decryptPayload((await res.json()) as T);
}

export function listChannels(): Promise<{ channels: Channel[] }> {
  return getJson('/api/channels');
}

/** Invitaciones de otras organizaciones, pendientes de aceptar (aceptar antes de entrar). */
export function listInvitations(): Promise<{ invitations: Invitacion[] }> {
  return getJson('/api/invitations');
}

/** Acepta o rechaza una invitación. */
export function answerInvitation(channelId: string, aceptar: boolean): Promise<{ ok: boolean }> {
  return postJson('/api/invitations', { channelId, action: aceptar ? 'accept' : 'reject' });
}

export function listMessages(
  channelId: string,
  before?: string,
): Promise<{ messages: ChatMessage[] }> {
  const params = new URLSearchParams({ channelId });
  if (before) params.set('before', before);
  return getJson(`/api/messages?${params.toString()}`);
}

/** Ventana de mensajes centrada en `messageId` (permalink / scroll-to-message, Ola 16),
 *  en orden cronológico ascendente. No marca el canal como leído. */
export function messagesAround(
  channelId: string,
  messageId: string,
  limit?: number,
): Promise<{ messages: ChatMessage[]; threadRootId?: string }> {
  const params = new URLSearchParams({ channelId, message: messageId });
  if (limit) params.set('limit', String(limit));
  return getJson(`/api/around?${params.toString()}`);
}

/** Acuse de lectura de los otros miembros de un DM («Visto», Ola 16). Vacío si no es DM. */
export function getReceipts(channelId: string): Promise<{ receipts: Receipt[] }> {
  return getJson(`/api/receipts?channelId=${encodeURIComponent(channelId)}`);
}

/** Vista previa (OpenGraph) de una URL http/https, cacheada + SSRF-safe en el backend. */
export function unfurlLink(url: string): Promise<{ unfurl: LinkPreview }> {
  void url;
  return Promise.reject(new MatrixSecurityError('unsupported'));
}

export async function sendMessage(input: {
  channelId: string;
  body: string;
  clientId: string;
  mentions?: string[];
  fileIds?: string[];
  priority?: 'important' | 'urgent';
  requiresAck?: boolean;
  /** Objeto rico: encuesta o tarea. El `body` es la pregunta / el título. */
  card?: MessageCard;
}): Promise<{ message: ChatMessage }> {
  const files = input.fileIds?.length ? encryptedFiles(input.channelId, input.fileIds) : undefined;
  const matrix_event = await roomsMatrix().send(
    input.channelId,
    {
      body: input.body,
      ...(input.mentions ? { mentions: input.mentions } : {}),
      ...(files ? { attachments: files.attachments } : {}),
      ...(input.card ? { card: input.card } : {}),
      ...(input.priority ? { priority: input.priority } : {}),
      ...(input.requiresAck ? { requiresAck: true } : {}),
    },
    input.clientId,
    undefined,
    files?.room,
  );
  return postJson('/api/send', {
    channelId: input.channelId,
    body: '',
    clientId: input.clientId,
    matrix_event,
  });
}

/** Registra el acuse de recibo del usuario sobre un mensaje que lo pide (Ola 12);
 *  devuelve el nuevo total «Confirmado por N». */
export function ackMessage(input: { messageId: string }): Promise<{ count: number }> {
  return postJson('/api/ack', input);
}

/** Alguien que ha visto un mensaje, y cuándo lo vio. */
export interface MessageReader {
  subject: string;
  name?: string;
  read_at: string;
}

/**
 * Quién ha visto un mensaje. `total` es cuánta gente hay en el canal aparte del
 * autor, para poder decir «visto por 3 de 12» — un número suelto no dice si eso
 * es mucho o poco.
 */
export function messageReaders(
  messageId: string,
): Promise<{ readers: MessageReader[]; total: number }> {
  return getJson(`/api/readers?messageId=${encodeURIComponent(messageId)}`);
}

/** Programa un mensaje para enviarse en `sendAt` (ISO 8601 futuro). */
export function scheduleMessage(input: {
  channelId: string;
  body: string;
  sendAt: string;
  mentions?: string[];
  fileIds?: string[];
}): Promise<{ scheduled: ScheduledMessage }> {
  return postJson('/api/scheduled', input);
}

/** Mis borradores guardados en el servidor (cross-canal, del más reciente al más antiguo). */
export function listDrafts(): Promise<{ drafts: Draft[] }> {
  return roomsMatrix()
    .drafts()
    .then((drafts) => ({ drafts }));
}

/** Guarda el borrador de un ámbito de redacción; un cuerpo en blanco lo BORRA. */
export function putDraft(
  channelId: string,
  parentId: string | undefined,
  body: string,
): Promise<{ ok: boolean }> {
  return roomsMatrix()
    .saveDraft(channelId, parentId, body)
    .then(() => ({ ok: true }));
}

/** Mis mensajes programados (opcionalmente por canal), por send_at ascendente. */
export function listScheduled(channelId?: string): Promise<{ scheduled: ScheduledMessage[] }> {
  const qs = channelId ? `?channelId=${encodeURIComponent(channelId)}` : '';
  return getJson(`/api/scheduled${qs}`);
}

/** Edita un programado (cuerpo / instante / adjuntos); devuelve el actualizado. */
export function updateScheduled(
  id: string,
  patch: { body?: string; sendAt?: string; fileIds?: string[] },
): Promise<{ scheduled: ScheduledMessage }> {
  return postJson('/api/scheduled', { id, ...patch });
}

/** Cancela un programado (borrado vía POST {cancel:true} para no disparar el CSRF). */
export function cancelScheduled(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/scheduled', { id, cancel: true });
}

/**
 * Publica una tarjeta de «llamada iniciada» en el timeline del canal: un mensaje de
 * sistema type=call con cuerpo vacío. El resto de clientes (y este) la reciben por
 * WebSocket, de modo que la tarjeta aparece sin insertar nada de forma optimista.
 */
export function postCallMessage(channelId: string): Promise<{ message: ChatMessage }> {
  return postJson('/api/send', { channelId, type: 'call' });
}

/**
 * Sube un fichero a Sytue Drive vía el proxy BFF /api/upload y devuelve el adjunto
 * (fileId + metadatos) para adjuntarlo a un mensaje. Usa XHR (no fetch) por el
 * evento de progreso de subida; los bytes van como octet-stream con ?name=&type=
 * (esquiva el CSRF de Astro). Rechaza con Error cuyo message es un código:
 * 'too_large' | 'aborted' | 'upload_failed'.
 */
export async function uploadAttachment(
  file: File,
  opts?: { onProgress?: (fraction: number) => void; signal?: AbortSignal; channelId?: string },
): Promise<Attachment> {
  if (opts?.signal?.aborted) throw new Error('aborted');
  if (opts?.channelId) {
    const scope = captureMatrixScope();
    const uploaded = rememberUpload(
      scope,
      opts.channelId,
      await scope.client.upload(opts.channelId, file, opts.signal),
    );
    opts.onProgress?.(1);
    return uploaded;
  }
  // Tenant emoji and imported media must not silently bypass the encrypted uploader.
  throw new MatrixSecurityError('unsupported');
}

/**
 * URL same-origin para servir un adjunto (proxy BFF /api/attachment). Con `download`
 * fuerza descarga; si no, imágenes/PDF/texto se muestran inline. Con `thumb` pide la
 * miniatura que genera Drive en vez del original completo.
 *
 * `channelId` es la VERJA: los adjuntos viven en el ámbito de su canal y Drive exige
 * ser miembro para servirlos, así que hay que decirle de qué canal es el mensaje.
 */
export function attachmentUrl(
  a: { fileId: string; name?: string; objectUrl?: string; matrixFile?: Attachment['matrixFile'] },
  opts?: { download?: boolean; channelId?: string; thumb?: boolean },
): string {
  if (a.matrixFile) return a.objectUrl ?? '';
  const qs = new URLSearchParams({ fileId: a.fileId });
  if (a.name) qs.set('name', a.name);
  if (opts?.download) qs.set('download', '1');
  if (opts?.channelId) qs.set('channel', opts.channelId);
  if (opts?.thumb) qs.set('thumb', '1');
  // CON withBase: esto acaba en un `href`, no en un `fetch`, así que el shim de
  // `basePathScript` —que prefija las rutas `/api/` de fetch— no lo alcanza. Sin
  // el prefijo, en `<organizacion>.sytue.eu` la petición sale a la RAÍZ del host,
  // donde el ingress del tenant no enruta `/api`: 404 de Traefik. Medido.
  return withBase(import.meta.env.BASE_URL, `/api/attachment?${qs.toString()}`);
}

/** Miembros del canal/team, para el selector de menciones del composer. */
export function listMembers(): Promise<{ members: ChannelMember[] }> {
  return getJson('/api/members');
}

/** ¿Administra el usuario el workspace? Decide qué entradas de administración se
 *  ofrecen; la autorización real la sigue haciendo el backend en cada llamada. */
export function getMe(): Promise<{ workspaceAdmin: boolean }> {
  return getJson('/api/me');
}

/** Hilo de un mensaje raíz: { root, replies, following } ya en el shape del UI. */
export function getThread(
  messageId: string,
): Promise<{ root: ChatMessage; replies: ChatMessage[]; following: boolean }> {
  return getJson(`/api/thread?messageId=${encodeURIComponent(messageId)}`);
}

/** Sigue el hilo de un mensaje raíz (para verlo con sus no-leídos en «Hilos»). */
export function followThread(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/follow', input);
}

/** Deja de seguir un hilo (borrado vía POST {delete:true} para no disparar el CSRF). */
export function unfollowThread(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/follow', { messageId: input.messageId, delete: true });
}

/** Mis hilos seguidos (vista global), con cursor keyset `before` por última actividad. */
export function listThreads(
  before?: string,
): Promise<{ threads: ThreadSummary[]; nextBefore?: string }> {
  const qs = before ? `?before=${encodeURIComponent(before)}` : '';
  return getJson(`/api/threads${qs}`);
}

/** Publica una respuesta dentro del hilo de un mensaje raíz. */
export async function postReply(input: {
  messageId: string;
  body: string;
  mentions?: string[];
  fileIds?: string[];
}): Promise<{ message: ChatMessage }> {
  const root = knownMessage(input.messageId);
  const files = input.fileIds?.length ? encryptedFiles(root.channelId, input.fileIds) : undefined;
  const matrix_event = await roomsMatrix().send(
    root.channelId,
    {
      body: input.body,
      ...(input.mentions ? { mentions: input.mentions } : {}),
      ...(files ? { attachments: files.attachments } : {}),
    },
    crypto.randomUUID(),
    root.matrix_event ? { thread: root.matrix_event } : undefined,
    files?.room,
  );
  return postJson('/api/reply', { messageId: input.messageId, body: '', matrix_event });
}

/** Edita el cuerpo de un mensaje propio; devuelve el mensaje con `editedAt`. */
export async function editMessage(input: {
  messageId: string;
  body: string;
}): Promise<{ message: ChatMessage }> {
  const original = knownMessage(input.messageId);
  if (!original.matrix_event) throw new MatrixSecurityError('unsupported');
  const matrix_event = await roomsMatrix().send(
    original.channelId,
    { body: input.body, ...(original.attachments ? { attachments: original.attachments } : {}) },
    crypto.randomUUID(),
    { replaces: original.matrix_event },
    original.matrix_event.room_id,
  );
  return postJson('/api/message', { messageId: input.messageId, body: '', matrix_event });
}

/** Borra un mensaje propio (borrado suave en el backend). */
export function deleteMessage(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/message', { messageId: input.messageId, delete: true });
}

/** Añade (add=true) o retira una reacción de un mensaje vía el BFF -> servicio Go. */
export function reactMessage(input: {
  messageId: string;
  emoji: string;
  add: boolean;
}): Promise<{ ok: boolean }> {
  return postJson('/api/reactions', input);
}

/**
 * Marca (add=true) o retira un valor de la tarjeta de un mensaje: votar una opción de
 * la encuesta o dar por hecha la tarea. Devuelve el recuento ya agregado por el
 * backend; el resto de clientes lo recibe por WebSocket (evento `card`).
 */
export function voteCard(input: {
  messageId: string;
  value: string;
  add: boolean;
}): Promise<{ votes: CardVote[] }> {
  return postJson('/api/card-vote', input);
}

/** Marca un canal como leído (pone a cero no leídos y menciones durables). */
export function markRead(channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/read', { channelId });
}

// ---- Ola 3: favoritos / silenciar / explorar / archivar / tema / DM ----

/** Fija favorito, silenciado y/o nivel de notificación del canal; devuelve el canal. */
export function setChannelPrefs(
  channelId: string,
  prefs: { favorite?: boolean; muted?: boolean; notifyLevel?: 'all' | 'mentions' | 'none' },
): Promise<{ channel: Channel }> {
  return postJson('/api/prefs', { channelId, ...prefs });
}

/** Mis preferencias de notificación (palabras clave + «No molestar»). */
export function getNotifyPrefs(): Promise<{ prefs: NotifyPrefs }> {
  return getJson('/api/notify-prefs');
}

/** Guarda mis preferencias de notificación (POST al BFF → PUT al servicio; el backend
 *  las sanea). Devuelve las resultantes. */
export function putNotifyPrefs(prefs: NotifyPrefs): Promise<{ prefs: NotifyPrefs }> {
  return postJson('/api/notify-prefs', prefs);
}

// ---- Ola 18: Web Push ----

/** Clave pública VAPID (applicationServerKey). Vacía = push no configurado en el servidor. */
export function getPushKey(): Promise<{ key: string }> {
  return getJson('/api/push');
}

/** Registra la suscripción Web Push del navegador. */
export function subscribePush(sub: {
  endpoint: string;
  keys: { p256dh: string; auth: string };
}): Promise<{ ok: boolean }> {
  return postJson('/api/push', { action: 'subscribe', ...sub });
}

/** Da de baja la suscripción Web Push (por endpoint). */
export function unsubscribePush(endpoint: string): Promise<{ ok: boolean }> {
  return postJson('/api/push', { action: 'unsubscribe', endpoint });
}

// ---- Ola 11: categorías del sidebar (per-usuario) ----

/** Mis categorías del sidebar (ordenadas). */
export function listCategories(): Promise<{ categories: Category[] }> {
  return getJson('/api/categories');
}

/** Crea una categoría del sidebar; devuelve la creada. */
export function createCategory(name: string): Promise<{ category: Category }> {
  return postJson('/api/categories', { name });
}

/** Renombra, colapsa/expande y/o reordena una categoría; devuelve la actualizada. */
export function updateCategory(
  id: string,
  patch: { name?: string; sortOrder?: number; collapsed?: boolean },
): Promise<{ category: Category }> {
  return postJson('/api/categories', { id, ...patch });
}

/** Borra una categoría (sus canales recaen a la por defecto; DELETE vía POST {delete}). */
export function deleteCategory(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/categories', { id, delete: true });
}

/** Asigna el canal a una categoría del sidebar (categoryId null = por defecto) y,
 *  opcionalmente, fija su orden. Devuelve el canal actualizado. */
export function setChannelCategory(
  channelId: string,
  categoryId: string | null,
  sortOrder?: number,
): Promise<{ channel: Channel }> {
  return postJson('/api/channel-category', {
    channelId,
    categoryId,
    ...(sortOrder !== undefined ? { sortOrder } : {}),
  });
}

/** Crea un canal (público o privado) en el team del usuario y devuelve el canal creado.
 *  Lanza ChatHttpError con status 409 si ya existe un canal con ese nombre. */
export function createChannel(input: {
  name: string;
  kind: 'public' | 'private';
  topic?: string;
}): Promise<{ channel: Channel }> {
  return postJson('/api/channels', input);
}

/** Explorar: canales públicos del team a los que NO perteneces (?q= filtra). */
export function browseChannels(query = ''): Promise<{ channels: Channel[] }> {
  const qs = query.trim() ? `?q=${encodeURIComponent(query.trim())}` : '';
  return getJson(`/api/browse${qs}`);
}

/** Une al usuario a un canal público; devuelve el canal. */
export function joinChannel(channelId: string): Promise<{ channel: Channel }> {
  return postJson('/api/join', { channelId });
}

/** Retematiza o archiva un canal (solo admin); devuelve el canal actualizado. */
export function updateChannel(
  channelId: string,
  patch: { topic?: string; purpose?: string; archived?: boolean; name?: string },
): Promise<{ channel: Channel }> {
  return postJson('/api/channel', { channelId, ...patch });
}

/** Abre (o reutiliza) un DM (1:1 o de grupo) con otros miembros del team; `subjects`
 *  son los subs de los OTROS participantes. Devuelve el canal directo. */
export function openDirect(subjects: string[]): Promise<{ channel: Channel }> {
  return postJson('/api/dm', { subjects });
}

/** Encuentra a alguien de CUALQUIER organización por su @usuario o su correo (ADR 0040).
 *  404 si nadie en Sytue One lo usa; 429 si se busca demasiado seguido. */
export function resolvePerson(query: string): Promise<{ person: Persona }> {
  return postJson('/api/people', { query });
}

/** Abandona el canal (deja de ser miembro): el slash command «/leave» y «Salir del
 *  canal» de los menús. 409 si eres el último admin de un canal privado. */
export function leaveChannel(channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/leave', { channelId });
}

/** Borra un canal público o privado para todos (admin del canal o del equipo). */
export function deleteChannel(channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/channel-delete', { channelId });
}

/** «Borra el chat» de un DM solo para ti: sale de tu barra y se te esconde lo
 *  anterior; los demás lo conservan. Si alguien vuelve a escribir, reaparece. */
export function hideDirect(channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/channel-delete', { channelId, direct: true });
}

// ---- Ola 2C: roster del canal («Miembros») ----

/** Miembros del canal (rol de CANAL, no de team), ordenados por nombre. */
export function listChannelMembers(channelId: string): Promise<{ members: ChannelMember[] }> {
  return getJson(`/api/channel-members?channelId=${encodeURIComponent(channelId)}`);
}

/** Añade al canal a alguien del team. 403 si no pertenece al team o no eres admin;
 *  409 si el canal está archivado (ChatHttpError lleva el estado). */
export function addChannelMember(
  channelId: string,
  subject: string,
  name?: string,
): Promise<{ member: ChannelMember }> {
  return postJson('/api/channel-members', { channelId, subject, ...(name ? { name } : {}) });
}

/** Saca a alguien del canal (o a ti mismo: el backend admite autoservicio). */
export function removeChannelMember(channelId: string, subject: string): Promise<{ ok: boolean }> {
  return postJson('/api/channel-members', { channelId, subject, remove: true });
}

/** Da o quita el rol de admin DEL CANAL a un miembro. */
export function setChannelMemberRole(
  channelId: string,
  subject: string,
  role: 'admin' | 'member',
): Promise<{ member: ChannelMember }> {
  return postJson('/api/channel-members', { channelId, subject, role });
}

/** Añade a un miembro al team del usuario (invitar); lo despacha «/invite @usuario». */
export function inviteMember(input: { subject: string; name: string }): Promise<{ ok: boolean }> {
  return postJson('/api/invite', input);
}

/** Da de BAJA a alguien del team; lo despacha «/remove @usuario». Solo admin de team
 *  (el servicio Go responde 403 si no). Es la simétrica de inviteMember: sin ella,
 *  quitar a alguien de un canal lo dejaba igualmente en el team. */
export function removeTeamMember(subject: string): Promise<{ ok: boolean }> {
  return postJson('/api/team-members', { subject, remove: true });
}

// ---- Ola 14: emoji personalizados del tenant ----

/** Emoji personalizados del tenant (nombre + fileId de Drive). */
export function listCustomEmoji(signal?: AbortSignal): Promise<{ emoji: CustomEmoji[] }> {
  return getJson('/api/emoji', signal);
}

export type { FreeGif, FreeGifPage } from '@sytue/api-clients';

export async function searchFreeGifs(
  query: string,
  cursor = '',
  signal?: AbortSignal,
): Promise<FreeGifPage> {
  const response = await fetch('/api/gifs', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ query, cursor }),
    ...(signal ? { signal } : {}),
  });
  if (!response.ok) throw new ChatHttpError(response.status, 'gif_catalog_unavailable');
  return (await response.json()) as FreeGifPage;
}

/** Solo se baja el original elegido; después se reutiliza el catálogo del tenant. */
export async function importFreeGif(gif: FreeGif, signal?: AbortSignal): Promise<CustomEmoji> {
  signal?.throwIfAborted();
  const name = `commons-${gif.id}`;
  const existing = (await listCustomEmoji(signal)).emoji.find((e) => e.name === name);
  signal?.throwIfAborted();
  if (existing) return existing;
  const response = await fetch(`/api/gifs?id=${gif.id}`, signal ? { signal } : {});
  if (!response.ok) throw new ChatHttpError(response.status, 'gif_unavailable');
  const data = await response.blob();
  signal?.throwIfAborted();
  // Drive sustituye por nombre: este archivo pertenece solo a esta importación.
  const file = new File([data], `${name}-${crypto.randomUUID()}.gif`, { type: 'image/gif' });
  let uploaded: Attachment;
  try {
    uploaded = await uploadAttachment(file, signal ? { signal } : undefined);
  } catch (err) {
    signal?.throwIfAborted();
    const concurrent = (await listCustomEmoji(signal)).emoji.find((e) => e.name === name);
    signal?.throwIfAborted();
    if (concurrent) return concurrent;
    throw err;
  }
  signal?.throwIfAborted();
  try {
    const result = await createCustomEmoji({ name, fileId: uploaded.fileId }, signal);
    signal?.throwIfAborted();
    return result.emoji;
  } catch (err) {
    signal?.throwIfAborted();
    // Otra persona puede haber importado el mismo GIF mientras se subía.
    const concurrent = (await listCustomEmoji(signal)).emoji.find((e) => e.name === name);
    signal?.throwIfAborted();
    if (concurrent) {
      if (concurrent.fileId !== uploaded.fileId) {
        // Solo retiramos la subida nueva perdedora; nunca el archivo reutilizado.
        await fetch(`/api/upload?fileId=${encodeURIComponent(uploaded.fileId)}`, {
          method: 'DELETE',
          ...(signal ? { signal } : {}),
        }).catch(() => undefined);
      }
      signal?.throwIfAborted();
      return concurrent;
    }
    throw err;
  }
}

/** Crea un emoji personalizado ligando `name` a un fichero de Drive ya subido. */
export function createCustomEmoji(
  input: {
    name: string;
    fileId: string;
  },
  signal?: AbortSignal,
): Promise<{ emoji: CustomEmoji }> {
  return postJson('/api/emoji', input, signal);
}

/** Borra un emoji personalizado por nombre (borrado vía POST {delete:true}). */
export function deleteCustomEmoji(name: string): Promise<{ ok: boolean }> {
  return postJson('/api/emoji', { name, delete: true });
}

// ---- Ola 15: integraciones (webhooks entrantes/salientes + bots) ----

/** Estado de integraciones de un canal (incoming) + del tenant (outgoing/bots). */
export interface IntegrationsData {
  incoming: IncomingWebhook[];
  outgoing: OutgoingWebhook[];
  bots: Bot[];
  /** Puentes del canal elegido (vacío si no se eligió canal). */
  bridges: Bridge[];
  /** Números de WhatsApp Business conectados y los equipos donde se pueden atender. */
  whatsapp: WhatsAppAccount[];
  teams: { id: string; name: string }[];
}

/** Slash commands de terceros registrados por la organización. */
export async function listCommands(): Promise<ServerCommand[]> {
  const d = await getJson<{ commands: ServerCommand[] }>('/api/commands');
  return d.commands ?? [];
}

/** Registra un comando (admin). Devuelve también sus secretos. */
export function createCommand(input: {
  name: string;
  requestUrl: string;
  description?: string;
  usageHint?: string;
}): Promise<ServerCommand> {
  return postJson('/api/commands', { action: 'create', ...input });
}

/** Borra un comando (admin). */
export function deleteCommand(id: string): Promise<void> {
  return postJson('/api/commands', { action: 'delete', id });
}

/** Carga las integraciones para el modal de gestión (incoming acotado al canal). */
export function listIntegrations(channelId?: string): Promise<IntegrationsData> {
  const qs = channelId ? `?channelId=${encodeURIComponent(channelId)}` : '';
  return getJson(`/api/integrations${qs}`);
}

/** Crea un webhook entrante; devuelve el token y la URL en CLARO una única vez. */
export function createIncomingWebhook(
  channelId: string,
  name: string,
  icon?: string,
): Promise<IncomingSecret> {
  return postJson('/api/integrations', {
    action: 'create-incoming',
    channelId,
    name,
    ...(icon ? { icon } : {}),
  });
}

/** Rota el token de un webhook entrante; devuelve el nuevo token y URL en CLARO. */
export function rotateIncomingWebhook(id: string): Promise<IncomingSecret> {
  return postJson('/api/integrations', { action: 'rotate-incoming', id });
}

/** Borra un webhook entrante. */
export function deleteIncomingWebhook(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'delete-incoming', id });
}

/** Crea un webhook saliente (canal y/o palabras disparadoras; al menos uno). */
export function createOutgoingWebhook(input: {
  channelId?: string;
  callbackUrl: string;
  triggerWords?: string[];
  /** Formato del POST: 'slack' y 'teams' hacen que un bot escrito para ellos funcione. */
  dialect?: OutgoingDialect;
}): Promise<{ webhook: OutgoingWebhook }> {
  return postJson('/api/integrations', { action: 'create-outgoing', ...input });
}

/** Borra un webhook saliente. */
export function deleteOutgoingWebhook(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'delete-outgoing', id });
}

/** Desconecta un número de WhatsApp Business. */
export function disconnectWhatsApp(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'disconnect-whatsapp', id });
}

/** Desconecta un puente. */
export function deleteBridge(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'delete-bridge', id });
}

/** Crea un bot; devuelve su token Bearer en CLARO una única vez. */
export function createBot(name: string): Promise<{ bot: Bot; token: string }> {
  return postJson('/api/integrations', { action: 'create-bot', name });
}

/** Borra un bot. */
export function deleteBot(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'delete-bot', id });
}

/** Añade un bot al canal indicado (para que pueda publicar en él). */
export function addBotToChannel(botId: string, channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'add-bot-channel', botId, channelId });
}

/** Quita un bot de un canal. */
export function removeBotFromChannel(botId: string, channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/integrations', { action: 'remove-bot-channel', botId, channelId });
}

// ---- Fijados (pinned) / Guardados (saved) / Marcar como no leído ----

/** Lista los mensajes fijados de un canal. */
export function listPins(channelId: string): Promise<{ pins: ChatMessage[] }> {
  return getJson(`/api/pins?channelId=${encodeURIComponent(channelId)}`);
}

/** Fija un mensaje; devuelve el mensaje fijado. */
export function pinMessage(input: { messageId: string }): Promise<{ message: ChatMessage }> {
  return postJson('/api/pin', input);
}

/** Desfija un mensaje (borrado vía POST {delete:true} para no disparar el CSRF). */
export function unpinMessage(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/pin', { messageId: input.messageId, delete: true });
}

/** Guarda un mensaje para el usuario. */
export function saveMessage(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/save', input);
}

/** Quita un mensaje de guardados (borrado vía POST {delete:true}). */
export function unsaveMessage(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/save', { messageId: input.messageId, delete: true });
}

/** Mis guardados, cross-canal, con cursor keyset `before`. */
export function listSaved(
  before?: string,
): Promise<{ messages: ChatMessage[]; nextBefore?: string }> {
  const qs = before ? `?before=${encodeURIComponent(before)}` : '';
  return getJson(`/api/saved${qs}`);
}

/** Deja el canal del mensaje como no leído a partir de ese mensaje. */
export function markUnread(input: { messageId: string }): Promise<{ ok: boolean }> {
  return postJson('/api/unread', input);
}

/**
 * Búsqueda global de Rooms (cross-team) vía el proxy BFF. `query` se pasa tal cual,
 * incluidos los operadores from:/in:/before:/after:; el backend los parsea. Los
 * resultados no traen el nombre del canal (se resuelve en el cliente).
 */
export function searchMessages(query: string): Promise<{ messages: ChatMessage[] }> {
  void query;
  return Promise.reject(new MatrixSecurityError('unsupported'));
}

/**
 * URL canónica a un mensaje, con el MISMO formato del deep-link del Hub:
 * `{rooms}/rooms/<org>/channels/{channelId}?message={messageId}`. Al abrirla, Rooms
 * carga una ventana centrada en el mensaje (fetch-around por keyset), hace scroll a él
 * y lo resalta (Ola 16). La ruta la resuelve src/pages/channels/[channelId].astro.
 *
 * `base` es host Y prefijo: se componía con `location.origin` a secas, y
 * `rooms.sytue.eu/channels/…` es el 404 de Traefik —solo la raíz se redirige al
 * prefijo—, así que «Copiar enlace» y el reenvío dejaban un enlace muerto.
 *
 * Y CON la organización (ADR 0056): el enlace canónico a un contenido dice a qué
 * organización apunta, así que «Copiar enlace» y el reenvío salen ya en forma
 * canónica y aterrizan sin redirección. La org es la de la URL VISTA
 * (`orgActual`, que entiende también la marca del rewrite): quien reenvía desde
 * un enlace de otra organización comparte el enlace a ESA, no a la de su
 * sesión. En SSR no hay URL de navegador y sale sin org — el middleware de la
 * app la añade con un 302.
 */
export function messageLink(
  channelId: string,
  messageId: string,
  base: string = appUrl('rooms', '', orgActual('/rooms')),
): string {
  return `${base}/channels/${encodeURIComponent(channelId)}?message=${encodeURIComponent(messageId)}`;
}

// ---- Tiempo real (WebSocket) ----

/** Eventos que el servicio Go empuja por el WebSocket, ya en el shape del UI. */
export type ChatEvent =
  | { type: 'message'; message: ChatMessage }
  | { type: 'message.edited'; message: ChatMessage }
  | { type: 'message.deleted'; channelId: string; messageId: string }
  | {
      type: 'reaction';
      channelId: string;
      messageId: string;
      emoji: string;
      subject: string;
      added: boolean;
    }
  | { type: 'pin'; channelId: string; messageId: string; pinned: boolean; message?: ChatMessage }
  // Tarjeta (encuesta/tarea): llega el recuento COMPLETO, no el delta, así que
  // aplicarlo es idempotente aunque se crucen dos votos simultáneos.
  | { type: 'card'; channelId: string; messageId: string; votes: CardVote[] }
  // Roster del canal: un solo evento del UI para los dos frames del servicio, porque
  // el alta hace de UPSERT por subject (también cubre el cambio de rol).
  | {
      type: 'channel.member';
      channelId: string;
      subject: string;
      added: boolean;
      member?: ChannelMember;
    }
  // El canal se ha borrado: sale de la barra y se cierra si estaba abierto.
  | { type: 'channel.deleted'; channelId: string }
  | { type: 'typing'; channelId: string; subject: string; name: string }
  // Llamada del canal. Antes esto se descubría SONDEANDO cada ~25 s: quien
  // llamaba esperaba media conversación a que al otro le saliera el aviso.
  | { type: 'call'; channelId: string; started: boolean };

export interface RealtimeHandlers {
  /** Sub del usuario de la sesión (para marcar `mine` en mensajes/reacciones). */
  userId: string;
  onEvent: (event: ChatEvent) => void;
  onStatus?: (status: 'connecting' | 'open' | 'closed') => void;
}

/** Handle de la conexión en vivo: cerrarla y (re)suscribir canales. */
export interface RealtimeHandle {
  close: () => void;
  /** Suscribe la conexión a la difusión en vivo de un canal (idempotente). */
  subscribe: (channelId: string) => void;
  /** Emite el indicador "escribiendo…" al canal (efímero; el llamante lo throttlea). */
  sendTyping: (channelId: string) => void;
}

/** Reacción agregada tal y como la emite el servicio Go (subs, no `mine`). */
interface RawReaction {
  emoji: string;
  count: number;
  subs?: string[];
}

/** Mensaje tal y como lo empuja el servicio Go por el WS (snake_case). */
interface RawMessage {
  matrix_event?: ChatMessage['matrix_event'];
  encryption?: ChatMessage['encryption'];
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
  reactions?: RawReaction[];
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

/** Tarjeta (encuesta/tarea) tal y como la emite el servicio Go (snake_case). */
interface RawCard {
  kind?: string;
  options?: string[];
  multi?: boolean;
  assignee_sub?: string;
  assignee_name?: string;
  due?: string;
  votes?: { value: string; count: number; subs?: string[] }[];
}

/** Mapea los votos crudos resolviendo `mine` contra el sub de la sesión. */
function mapVotes(votes: RawCard['votes'], userId: string): CardVote[] {
  return (votes ?? []).map((v) => ({
    value: v.value,
    count: v.count,
    mine: Boolean(v.subs?.includes(userId)),
    ...(v.subs?.length ? { subs: v.subs } : {}),
  }));
}

/** Mapea la tarjeta cruda del servicio Go a la del UI. */
function mapRawCard(c: RawCard, userId: string): MessageCard {
  const votes = mapVotes(c.votes, userId);
  return {
    kind: c.kind === 'task' ? 'task' : 'poll',
    ...(c.options?.length ? { options: c.options } : {}),
    ...(c.multi ? { multi: true } : {}),
    ...(c.assignee_sub ? { assigneeSub: c.assignee_sub } : {}),
    ...(c.assignee_name ? { assigneeName: c.assignee_name } : {}),
    ...(c.due ? { due: c.due } : {}),
    ...(votes.length ? { votes } : {}),
  };
}

/**
 * Mapea el mensaje crudo del servicio Go al ChatMessage del UI.
 * ponytail: espejo puro (~10 líneas) de toChatMessage en lib/chat.ts (capa
 * servidor); duplicarlo evita acoplar el cliente de navegador al cliente REST de
 * servidor. Si cambia el shape de domain.Message, tocar ambos.
 */
function mapRawMessage(m: RawMessage, userId: string): ChatMessage {
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
    ...(m.card?.kind ? { card: mapRawCard(m.card, userId) } : {}),
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

/** Frame crudo del servidor WS: {type, channel_id?, subject?, name?, payload?}. */
export interface WireFrame {
  type: string;
  channel_id?: string;
  subject?: string;
  name?: string;
  payload?: unknown;
}

/**
 * Traduce un frame del servicio Go al ChatEvent del UI (o null si se ignora).
 *
 * Se exporta para poder comprobarla: es el traductor del protocolo en tiempo
 * real —de ella depende que un mensaje aparezca o que suene una llamada— y
 * probarla a través del WebSocket exigiría un servidor de por medio.
 */
export function toChatEvent(f: WireFrame, userId: string): ChatEvent | null {
  switch (f.type) {
    case 'message': {
      const p = f.payload as RawMessage | undefined;
      if (!p?.id) return null;
      return { type: 'message', message: mapRawMessage(p, userId) };
    }
    case 'call_started':
    case 'call_ended': {
      // El canal viene en el sobre (no en el payload), como en los demás frames
      // del servicio. Sin canal no hay a qué banner aplicarlo.
      const canal = f.channel_id ?? '';
      if (!canal) return null;
      return { type: 'call', channelId: canal, started: f.type === 'call_started' };
    }
    case 'message_edited': {
      const p = f.payload as RawMessage | undefined;
      if (!p?.id) return null;
      return { type: 'message.edited', message: mapRawMessage(p, userId) };
    }
    case 'message_deleted': {
      const p = f.payload as { id?: string; channel_id?: string } | undefined;
      if (!p?.id) return null;
      return {
        type: 'message.deleted',
        channelId: p.channel_id ?? f.channel_id ?? '',
        messageId: p.id,
      };
    }
    case 'reaction': {
      const p = f.payload as
        | {
            action?: string;
            message_id?: string;
            channel_id?: string;
            subject?: string;
            emoji?: string;
          }
        | undefined;
      if (!p?.message_id || !p.emoji) return null;
      return {
        type: 'reaction',
        channelId: p.channel_id ?? f.channel_id ?? '',
        messageId: p.message_id,
        emoji: p.emoji,
        subject: p.subject ?? '',
        added: p.action === 'added',
      };
    }
    case 'card': {
      const p = f.payload as
        { message_id?: string; channel_id?: string; votes?: RawCard['votes'] } | undefined;
      if (!p?.message_id) return null;
      return {
        type: 'card',
        channelId: p.channel_id ?? f.channel_id ?? '',
        messageId: p.message_id,
        votes: mapVotes(p.votes, userId),
      };
    }
    case 'pin': {
      const p = f.payload as
        | { action?: string; message_id?: string; channel_id?: string; message?: RawMessage }
        | undefined;
      if (!p?.message_id) return null;
      return {
        type: 'pin',
        channelId: p.channel_id ?? f.channel_id ?? '',
        messageId: p.message_id,
        pinned: p.action === 'pinned',
        ...(p.message ? { message: mapRawMessage(p.message, userId) } : {}),
      };
    }
    case 'channel_member_added':
    case 'channel_member_removed': {
      // {action, channel_id, actor, subject, member?}. `member` viene en el alta y en
      // el cambio de rol; la baja solo trae el subject.
      const p = f.payload as
        | {
            action?: string;
            channel_id?: string;
            subject?: string;
            member?: { subject: string; name?: string; role?: string };
          }
        | undefined;
      if (!p?.subject) return null;
      const m = p.member;
      return {
        type: 'channel.member',
        channelId: p.channel_id ?? f.channel_id ?? '',
        subject: p.subject,
        added: f.type === 'channel_member_added',
        ...(m
          ? {
              member: {
                sub: m.subject,
                name: m.name || m.subject,
                ...(m.role === 'admin' || m.role === 'member' ? { role: m.role } : {}),
              },
            }
          : {}),
      };
    }
    case 'channel_deleted': {
      const p = f.payload as { channel_id?: string } | undefined;
      const canal = p?.channel_id ?? f.channel_id ?? '';
      if (!canal) return null;
      return { type: 'channel.deleted', channelId: canal };
    }
    case 'typing': {
      // "escribiendo…": subject + name viajan como campos de primer nivel del frame
      // (no en payload), igual que los emite ws.ServerFrame del servicio Go.
      if (!f.channel_id || !f.subject) return null;
      return {
        type: 'typing',
        channelId: f.channel_id,
        subject: f.subject,
        name: f.name || f.subject,
      };
    }
    default:
      // ready/presence/pong/error: fuera del alcance actual.
      return null;
  }
}

/**
 * Abre y mantiene la conexion de tiempo real. Devuelve un handle para cerrarla y
 * para suscribir canales. Pide un ticket nuevo en cada (re)conexion; reconecta
 * con backoff exponencial acotado mientras la pestana siga viva. Las
 * suscripciones se recuerdan y se reenvían tras cada reconexión.
 */
export function connectRealtime(handlers: RealtimeHandlers): RealtimeHandle {
  let ws: WebSocket | null = null;
  let closedByUser = false;
  let attempt = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  // Canales deseados: se (re)envían como frames `subscribe` al abrir el socket.
  const wanted = new Set<string>();

  function sendSubscribe(channelId: string): void {
    ws?.send(JSON.stringify({ type: 'subscribe', channel_id: channelId }));
  }

  async function open(): Promise<void> {
    if (closedByUser) return;
    handlers.onStatus?.('connecting');
    let ticketUrl: string;
    try {
      const { ticket, wsUrl } = await postJson<{ ticket: string; wsUrl: string }>(
        '/api/ws-ticket',
        {},
      );
      const u = new URL(wsUrl);
      // El servicio Go valida el token en el upgrade de /ws vía ?access_token=.
      u.searchParams.set('access_token', ticket);
      ticketUrl = u.toString();
    } catch {
      scheduleReconnect();
      return;
    }

    ws = new WebSocket(ticketUrl);
    ws.addEventListener('open', () => {
      attempt = 0;
      handlers.onStatus?.('open');
      // (Re)suscribe los canales deseados: sin el frame `subscribe` el servidor no
      // mete la conexión en la sala del canal y NO llega ningún evento en vivo.
      for (const id of wanted) sendSubscribe(id);
    });
    ws.addEventListener('message', (ev) => {
      let frame: WireFrame;
      try {
        frame = JSON.parse(typeof ev.data === 'string' ? ev.data : '') as WireFrame;
      } catch {
        return; // ignora frames no-JSON
      }
      const event = toChatEvent(frame, handlers.userId);
      if (event?.type === 'message' || event?.type === 'message.edited') {
        void decryptMessage(event.message).then((message) =>
          handlers.onEvent({ ...event, message }),
        );
      } else if (event) handlers.onEvent(event);
    });
    ws.addEventListener('close', () => {
      handlers.onStatus?.('closed');
      scheduleReconnect();
    });
    ws.addEventListener('error', () => ws?.close());
  }

  function scheduleReconnect(): void {
    if (closedByUser || reconnectTimer) return;
    attempt += 1;
    const delay = Math.min(1000 * 2 ** attempt, 15_000);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      void open();
    }, delay);
  }

  void open();

  return {
    close(): void {
      closedByUser = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    },
    subscribe(channelId: string): void {
      if (!channelId || wanted.has(channelId)) return;
      wanted.add(channelId);
      if (ws?.readyState === WebSocket.OPEN) sendSubscribe(channelId);
    },
    sendTyping(channelId: string): void {
      if (channelId && ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'typing', channel_id: channelId }));
      }
    },
  };
}

/** Guarda en «Mi Drive» una copia del adjunto. `channelId` es obligatorio: la
 *  autorización es la membresía del canal, no el permiso de «Mi unidad». */
export async function saveAttachmentToDrive(
  fileId: string,
  channelId: string,
): Promise<{ id: string; name: string }> {
  return postJson('/api/save-to-drive', { fileId, channelId });
}

// ---- Denunciar y bloquear (ADR 0073) ----

export { REPORT_REASONS } from '../lib/chat.js';
export type { BlockedPerson, ModerationReport, ReportAction, ReportReason } from '../lib/chat.js';
import type { BlockedPerson, ModerationReport, ReportAction, ReportReason } from '../lib/chat.js';

/** A quién se denuncia: un mensaje, o una persona en el canal que se comparte. */
export type ReportTarget = { messageId: string } | { channelId: string; subject: string };

/** Denuncia un mensaje o a una persona. 429 = tope diario de denuncias. */
export function sendReport(
  target: ReportTarget,
  reason: ReportReason,
  note: string,
): Promise<{ ok: boolean }> {
  return postJson('/api/reports', { ...target, reason, note });
}

/** Cola de denuncias que el usuario puede moderar (vacía si no modera nada). */
export function listReports(signal?: AbortSignal): Promise<{ reports: ModerationReport[] }> {
  return getJson('/api/reports', signal);
}

export function resolveReport(reportId: string, action: ReportAction): Promise<{ ok: boolean }> {
  return postJson('/api/reports', { reportId, action });
}

export function listBlocks(signal?: AbortSignal): Promise<{ blocks: BlockedPerson[] }> {
  return getJson('/api/blocks', signal);
}

/** Bloquea a alguien con quien se comparte `channelId`. */
export function blockPerson(channelId: string, subject: string): Promise<{ block: BlockedPerson }> {
  return postJson('/api/blocks', { channelId, subject });
}

export function unblockPerson(subject: string): Promise<{ ok: boolean }> {
  return postJson('/api/blocks', { subject, delete: true });
}
