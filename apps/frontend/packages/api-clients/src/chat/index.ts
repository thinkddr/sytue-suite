/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Chat — SDK al servicio Go propio (rooms.sytue.eu, dev :8085). Mensajeria
 * en tiempo real: API REST JSON + canal WebSocket (RFC 6455) para eventos en
 * vivo. Estado en PostgreSQL (sin Redis; presencia y fan-out via LISTEN/NOTIFY).
 * Implementacion clean-room (NO Mattermost). Autorizacion Bearer (JWT de Sytue ID).
 *
 * REST base:  /api/v1
 * WS:         /api/v1/ws  (ws://)
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

/** Server-authenticated mapping. No message keys or Matrix credentials cross the BFF. */
export interface MatrixBootstrapMetadata {
  actor: string;
  tenant: string;
  homeserver_url: string;
  sso_provider_id: string;
  matrix_user_id: string;
  channels: { channel_id: string; matrix_room_id: string }[];
  encryption: 'm.megolm.v1.aes-sha2';
  draft_room_id?: string;
  draft_room_ids?: string[];
  source_url?: string;
}

export interface ChatOptions {
  /** URL base, p.ej. https://rooms.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/** GIF animado cuya licencia CC0 ha comprobado el servicio de Rooms. */
export interface FreeGif {
  id: number;
  title: string;
  source: string;
  license: string;
  width: number;
  height: number;
  size: number;
}

export interface FreeGifPage {
  items: FreeGif[];
  next?: string;
}

export type ChannelType = 'public' | 'private' | 'direct' | 'group';

export interface Channel {
  id: string;
  type: ChannelType;
  name: string;
  topic?: string;
  /** No leidos del usuario actual. */
  unread: number;
  lastMessageAt?: string;
}

export interface Message {
  id: string;
  channelId: string;
  authorId: string;
  body: string;
  createdAt: string;
  editedAt?: string;
  /** IDs de adjuntos en Sytue Drive (intents cross-app). */
  attachments?: string[];
}

export interface SendMessageInput {
  channelId: string;
  body: string;
  attachments?: string[];
  /** Idempotencia: evita duplicados en reintentos. */
  clientId?: string;
}

/** Evento entrante por el WebSocket. */
export type ChatEvent =
  | { type: 'message'; message: Message }
  | { type: 'message.edited'; message: Message }
  | { type: 'message.deleted'; channelId: string; messageId: string }
  | { type: 'typing'; channelId: string; userId: string }
  | { type: 'presence'; userId: string; state: 'online' | 'away' | 'offline' };

/** Lo que devuelve el servicio, tal cual: `snake_case` y `kind` en vez de `type`. */
interface CanalCrudo {
  id: string;
  kind?: string;
  name: string;
  topic?: string;
  unread?: number;
  last_message_at?: string;
}

interface MensajeCrudo {
  id: string;
  channel_id: string;
  author_sub?: string;
  author_name?: string;
  body: string;
  created_at?: string;
  edited_at?: string;
  attachments?: { file_id?: string }[];
}

const TIPOS: readonly string[] = ['public', 'private', 'direct', 'group'];

function canalACanal(c: CanalCrudo): Channel {
  return {
    id: c.id,
    type: (TIPOS.includes(c.kind ?? '') ? c.kind : 'public') as ChannelType,
    name: c.name,
    ...(c.topic ? { topic: c.topic } : {}),
    unread: c.unread ?? 0,
    ...(c.last_message_at ? { lastMessageAt: c.last_message_at } : {}),
  };
}

function mensajeAMensaje(m: MensajeCrudo): Message {
  return {
    id: m.id,
    channelId: m.channel_id,
    authorId: m.author_sub ?? '',
    body: m.body,
    createdAt: m.created_at ?? '',
    ...(m.edited_at ? { editedAt: m.edited_at } : {}),
    ...(m.attachments?.length
      ? { attachments: m.attachments.map((a) => a.file_id ?? '').filter(Boolean) }
      : {}),
  };
}

export class ChatClient {
  private readonly http: HttpClient;

  constructor(options: ChatOptions) {
    this.http = new HttpClient({
      baseUrl: `${options.baseUrl.replace(/\/$/, '')}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /** Authenticated Matrix identity and channel mappings; never returns credentials. */
  matrixBootstrap(signal?: AbortSignal): Promise<MatrixBootstrapMetadata> {
    return this.http.json({ path: '/matrix/bootstrap', ...(signal ? { signal } : {}) });
  }

  startMatrixNative(input: {
    code_challenge: string;
    code_challenge_method: 'S256';
  }): Promise<{ authorize_url: string; state: string }> {
    return this.http.postJson('/matrix/native/start', input);
  }
  previewMatrixNative(
    state: string,
  ): Promise<{ authorize_url: string; state: string; actor: string; tenant: string }> {
    return this.http.json({ path: '/matrix/native/preview', query: { state } });
  }
  acceptMatrixNative(input: {
    state: string;
    login_token: string;
  }): Promise<{ ticket: string; redirect_url: string }> {
    return this.http.postJson('/matrix/native/accept', input);
  }
  finishMatrixNative(input: {
    state: string;
    ticket: string;
    code_verifier: string;
  }): Promise<{ login_token: string }> {
    return this.http.postJson('/matrix/native/finish', input);
  }

  /** Activates a real Meta number only after registration and webhook subscription succeed. */
  connectWhatsAppConsent(input: {
    team_id: string;
    instance_id: string;
    phone_number_id: string;
    pin?: string;
  }): Promise<{ account: { id: string } }> {
    return this.http.postJson('/whatsapp/consent', input);
  }

  /** Authorized destinations for an OAuth account. Rooms checks the workspace role. */
  integrationDestinations(instanceId: string): Promise<{ items: { id: string; name: string }[] }> {
    return this.http.json({ path: '/integrations/destinations', query: { instance: instanceId } });
  }

  /** The service checks channel administration and sends a test before persisting. */
  connectOAuthBridge(
    channelId: string,
    instanceId: string,
    remoteId: string,
    platform: 'slack' | 'teams' | 'googlechat',
  ): Promise<{ bridge: unknown }> {
    return this.http.postJson(`/channels/${encodeURIComponent(channelId)}/bridges`, {
      instance_id: instanceId,
      remote_id: remoteId,
      platform,
    });
  }

  /** Búsqueda paginada en el catálogo CC0, consultado exclusivamente por Rooms. */
  searchFreeGifs(query: string, cursor = '', signal?: AbortSignal): Promise<FreeGifPage> {
    return this.http.postJson('/gifs/search', { query, cursor }, signal ? { signal } : {});
  }

  /** Original animado o miniatura estática, con identidad y licencia comprobadas. */
  freeGifContent(id: number, preview = false, signal?: AbortSignal): Promise<Response> {
    return this.http.request({
      path: `/gifs/${id}`,
      query: { preview: preview ? 1 : 0 },
      ...(signal ? { signal } : {}),
    });
  }

  /** Equipos del usuario. Los canales cuelgan de uno: no hay lista de raíz. */
  private async equipos(): Promise<string[]> {
    const r = await this.http.json<{ teams?: { id: string }[] }>({ path: '/teams' });
    return (r.teams ?? []).map((t) => t.id);
  }

  /**
   * Canales del usuario actual, de todos sus equipos.
   *
   * Pedía `GET /channels`, que NO EXISTE: en el servicio los canales cuelgan
   * SIEMPRE de un equipo (`/teams/{id}/channels`), porque el equipo es lo que
   * decide quién ve qué. Devolvía 404, y con él se caía la única lista que
   * tiene el selector «compartir en Rooms» de Sytue Notepads: sin canales que
   * elegir, la acción entera es inalcanzable.
   *
   * Un equipo que falle no tumba a los demás: se devuelve lo que sí contestó.
   */
  async listChannels(): Promise<Channel[]> {
    const equipos = await this.equipos();
    const tandas = await Promise.allSettled(
      equipos.map((id) =>
        this.http.json<{ channels?: CanalCrudo[] }>({
          path: `/teams/${encodeURIComponent(id)}/channels`,
        }),
      ),
    );
    return tandas
      .flatMap((r) => (r.status === 'fulfilled' ? (r.value.channels ?? []) : []))
      .map(canalACanal);
  }

  /**
   * Crea un canal en un equipo. Sin `teamId`, en el primero del usuario.
   *
   * El servicio llama `kind` a lo que aquí es `type`, y crea bajo el equipo:
   * `POST /channels` no existe.
   */
  async createChannel(input: {
    type: ChannelType;
    name?: string;
    topic?: string;
    teamId?: string;
  }): Promise<Channel> {
    const equipo = input.teamId ?? (await this.equipos())[0];
    if (!equipo) throw new Error('El usuario no pertenece a ningún equipo de Rooms.');
    const crudo = await this.http.postJson<CanalCrudo>(
      `/teams/${encodeURIComponent(equipo)}/channels`,
      { kind: input.type, name: input.name, ...(input.topic ? { topic: input.topic } : {}) },
    );
    return canalACanal(crudo);
  }

  /** Historico de mensajes de un canal (paginado hacia atras por `before`). */
  async listMessages(
    channelId: string,
    options?: { before?: string; limit?: number },
  ): Promise<Message[]> {
    // El servicio contesta {messages: […]}, no un array pelado: casteado como
    // array, un .map() en el consumidor revienta con «not a function».
    const r = await this.http.json<{ messages?: MensajeCrudo[] }>({
      path: `/channels/${encodeURIComponent(channelId)}/messages`,
      query: { before: options?.before, limit: options?.limit ?? 50 },
    });
    return (r.messages ?? []).map(mensajeAMensaje);
  }

  /** Envia un mensaje. */
  async send(input: SendMessageInput): Promise<Message> {
    const crudo = await this.http.postJson<MensajeCrudo>(
      `/channels/${encodeURIComponent(input.channelId)}/messages`,
      { body: input.body, attachments: input.attachments, clientId: input.clientId },
    );
    return mensajeAMensaje(crudo);
  }

  /**
   * Marca un canal como leído hasta un mensaje, o hasta el último si no se dice.
   *
   * El campo se llama `message_id`; con `messageId` el servicio no encontraba
   * nada que leer y marcaba hasta el final, que es justo lo contrario de pedir
   * un tope.
   */
  async markRead(channelId: string, messageId?: string): Promise<void> {
    await this.http.request({
      method: 'POST',
      path: `/channels/${encodeURIComponent(channelId)}/read`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(messageId ? { message_id: messageId } : {}),
    });
  }

  /** Total de no leidos (para la shell). */
  async unreadTotal(): Promise<number> {
    const channels = await this.listChannels();
    return channels.reduce((sum, c) => sum + (c.unread ?? 0), 0);
  }

  /**
   * URL del WebSocket de eventos en vivo. El token va por query porque el
   * WebSocket no admite cabeceras; el servicio Go lo valida via JWKS de Sytue ID.
   */
  async wsUrl(): Promise<string> {
    const token = await this.http.accessToken();
    return this.http.wsUrl({ path: '/ws', query: { access_token: token } });
  }

  /** Abre el WebSocket de eventos (azucar sobre wsUrl). */
  async connect(): Promise<WebSocket> {
    return new WebSocket(await this.wsUrl());
  }
}
