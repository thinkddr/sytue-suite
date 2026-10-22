/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Hub — SDK al servicio Go de integracion (hub.sytue.eu, dev :8092):
 * busqueda federada (Drive/Mail/Calendar/Contacts/Chat), notificaciones
 * unificadas (SSE), presencia e intents cross-app. Lo consume @sytue/shell
 * (topbar global) y las apps. Autorizacion Bearer (client `sytue-hub`, JWT de
 * Sytue ID). docs/CONVENTIONS.md §11.
 *
 * Base API: /api/v1
 */
import { HttpClient, type AuthProvider } from '../core/http.js';
import type { ProductKey } from '@sytue/config';

export interface HubOptions {
  /** URL base, p.ej. https://hub.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/** Resultado de busqueda global federada. */
export interface SearchResult {
  id: string;
  /** Producto origen del resultado. */
  source: ProductKey;
  title: string;
  snippet?: string;
  /**
   * Tamaño de un fichero, en bytes. Si viene, se enseña formateado en el idioma de
   * quien busca en vez del `snippet`, que el Hub escribe siempre igual («11.7 KiB»).
   */
  sizeBytes?: number;
  /** URL profunda al recurso (deep link cross-app). */
  url: string;
  /** Epoch ms para ordenar por recencia. */
  timestamp?: number;
  icon?: string;
}

export interface SearchResponse {
  query: string;
  results: SearchResult[];
  /**
   * Cuántos resultados hay en total ANTES de recortar la página, y si quedan más.
   *
   * El Hub los manda desde siempre y el SDK los tiraba, así que la interfaz no
   * podía decir «23 resultados» ni ofrecer «ver más»: enseñaba la primera tanda y
   * callaba, sin manera de saber que había otras.
   */
  total: number;
  hasMore: boolean;
  /**
   * Orígenes que NO han contestado, con su motivo.
   *
   * Es lo que separa «no hay nada» de «Mail no responde». La búsqueda federada
   * degrada con elegancia a propósito —un origen caído no rompe el resto—, pero
   * si el SDK descarta ese aviso, la degradación se vuelve una MENTIRA: quien
   * busca da por perdido un correo que sí existe y deja de buscarlo.
   */
  fallidos: { source: ProductKey; error: string }[];
  /**
   * Qué entendió la IA de la frase, cuando se le pidió que la interpretara.
   *
   * Ausente = se buscó literalmente, que es lo de siempre. La interfaz TIENE que
   * enseñarlo: una búsqueda que ha filtrado por un rango de fechas que nadie ha
   * visto devuelve cero resultados sin explicar por qué, y quien busca concluye
   * que no existe lo que está buscando.
   */
  interpretado?: Interpretado;
}

/** Los filtros que la IA dedujo de la frase. */
export interface Interpretado {
  /** Lo que quedó como término literal tras sacar los filtros. */
  text: string;
  sources?: ProductKey[];
  kinds?: string[];
  /** Epoch ms, como `SearchResult.timestamp`. */
  from?: number;
  to?: number;
}

/** Forma canonica en el cable del Hub (GET /search). Interna al SDK. */
interface HubSearchWire {
  query: string;
  hits?: {
    source: ProductKey;
    id: string;
    title: string;
    snippet?: string;
    size_bytes?: number;
    url: string;
    icon?: string;
    /** RFC3339 (Go *time.Time). */
    timestamp?: string;
  }[];
  /** Un bloque por origen, con su latencia y su error parcial si lo hubo. */
  sources?: { source: ProductKey; took_ms?: number; error?: string }[];
  total?: number;
  has_more?: boolean;
  interpreted?: {
    text: string;
    sources?: ProductKey[];
    kinds?: string[];
    /** RFC3339 (Go *time.Time). */
    from?: string;
    to?: string;
  };
}

/** Los orígenes que fallaron, del bloque `sources` del Hub. */
function fallidosDe(sources: HubSearchWire['sources']): { source: ProductKey; error: string }[] {
  return (sources ?? [])
    .filter((s) => Boolean(s.error))
    .map((s) => ({ source: s.source, error: s.error as string }));
}

/**
 * Regla de silencio de avisos. `kind` vacio u omitido silencia el ORIGEN
 * entero; con `kind`, solo ese tipo dentro del origen. La regla mas especifica
 * gana (domain.Muted del Hub).
 */
export interface NotificationPref {
  source: ProductKey;
  kind?: string;
  muted: boolean;
}

/** Aplana los hits del cable al modelo del front. Lo comparten /search y /recent. */
function hitsToResults(hits: HubSearchWire['hits']): SearchResult[] {
  return (hits ?? []).map((h) => ({
    id: h.id,
    source: h.source,
    title: h.title,
    url: h.url,
    ...(h.snippet !== undefined ? { snippet: h.snippet } : {}),
    ...(h.size_bytes !== undefined ? { sizeBytes: h.size_bytes } : {}),
    ...(h.icon !== undefined ? { icon: h.icon } : {}),
    ...(h.timestamp ? { timestamp: Date.parse(h.timestamp) } : {}),
  }));
}

/** Pasa lo interpretado del cable al modelo del front (RFC3339 → epoch ms). */
function interpretadoDe(w: NonNullable<HubSearchWire['interpreted']>): Interpretado {
  return {
    text: w.text,
    ...(w.sources?.length ? { sources: w.sources } : {}),
    ...(w.kinds?.length ? { kinds: w.kinds } : {}),
    ...(w.from ? { from: Date.parse(w.from) } : {}),
    ...(w.to ? { to: Date.parse(w.to) } : {}),
  };
}

export type PresenceState = 'online' | 'away' | 'dnd' | 'offline';

export interface Presence {
  userId: string;
  state: PresenceState;
  /** Texto de estado opcional. */
  message?: string;
}

export interface Notification {
  id: string;
  source: ProductKey;
  /** Tipo del emisor (`docs.comment`…): con `params`, el título en cada idioma. */
  type?: string;
  /** Texto de respaldo, en el idioma del emisor. */
  title: string;
  body?: string;
  url?: string;
  /** Variables del título (`actor`, `name`…) para `notifications.kind.<type>`. */
  params?: Record<string, string>;
  /** Instante de creación (RFC3339). El Hub lo emite en snake_case. */
  created_at: string;
  read: boolean;
  /**
   * La organización que lo mandó, solo cuando NO es la de la sesión (ADR 0053): un
   * aviso de fuera se ve como tal.
   */
  organization?: string;
  /**
   * Su nombre visible («Acme Ingeniería S.L.»), si Sytue ID lo da al Hub; sin él se
   * enseña `organization`.
   */
  organization_name?: string;
  /**
   * Reconstruido de lo que ya había, de antes de que existiera el registro de avisos:
   * nace leído y la campana lo marca como tal.
   */
  historic?: boolean;
}

/** Datos para emitir una notificación (POST /notifications del Hub). */
export interface NewNotification {
  source: ProductKey;
  /** Tipo/evento del emisor, p.ej. "share.doc" (no vacío, <=100). */
  type: string;
  title: string;
  body?: string;
  url?: string;
  level?: 'info' | 'success' | 'warning' | 'error';
  /** Destinatario. Omitido = el propio usuario (no requiere scope). */
  subject?: string;
}

/**
 * Intent cross-app. Los cuatro que el Hub sabe resolver; no es una cadena libre
 * porque un valor que no esté aquí lo rechaza el servidor, y descubrirlo en
 * tiempo de ejecución es peor que no compilar.
 */
export type IntentType =
  'attach-drive-to-mail' | 'create-event-from-mail' | 'share-to-chat' | 'call-channel';

/**
 * Petición de intent, EN LA FORMA QUE ESPERA EL HUB.
 *
 * Este tipo decía `{action, target, payload}` y el servidor lee `{type, params,
 * ids}` (services/hub/internal/domain: IntentRequest). Con eso, `dispatchIntent`
 * no podía funcionar nunca: al Hub le llegaba un `type` vacío y respondía
 * «intent desconocido». Nadie lo notó porque las dos superficies que sí usan
 * intents —Mail y Drive— se compusieron su propia llamada en su BFF con la forma
 * correcta, y este cliente se quedó de adorno.
 */
export interface Intent {
  type: IntentType;
  /** Parámetros por tipo (p. ej. `message_id`, `subject`, `to`, `text`). */
  params?: Record<string, string>;
  /** Identificadores múltiples, p. ej. varios ficheros de Drive a adjuntar. */
  ids?: string[];
}

/** Respuesta: el deep-link que materializa el intent, más lo que el shell pinta. */
export interface IntentResult {
  type: IntentType;
  /** Deep-link de cara al usuario. Es lo único imprescindible. */
  url: string;
  /** Superficie destino (`mail`, `calendar`, `chat`…). */
  target?: string;
  /** Texto sugerido para el botón. */
  label?: string;
}

export class HubClient {
  private readonly http: HttpClient;
  private readonly base: string;
  private readonly auth: AuthProvider;

  constructor(options: HubOptions) {
    this.base = options.baseUrl.replace(/\/$/, '');
    this.auth = options.auth;
    this.http = new HttpClient({
      baseUrl: `${this.base}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /**
   * Busqueda global federada. El Hub responde el modelo canonico
   * {query, hits:[{source,title,snippet,url,icon,timestamp}], sources, took_ms};
   * lo aplanamos a {query, results} para el shell (timestamp RFC3339 -> epoch ms).
   */
  async search(
    query: string,
    sources?: ProductKey[],
    page?: { limit?: number; offset?: number; ia?: boolean },
  ): Promise<SearchResponse> {
    // `consulta` y no `json`: va por QUERY si el Hub lo entiende, y lo que
    // escribe la persona viaja en el cuerpo en vez de quedar en el log de
    // accesos de cada salto y en el `Referer`. Si el Hub es anterior a QUERY,
    // cae a GET solo y no vuelve a intentarlo.
    //
    // `/recent` se queda con `json`: sus parámetros son triviales y no llevan
    // nada de quien busca.
    const r = await this.http.consulta<HubSearchWire>({
      path: '/search',
      query: {
        q: query,
        ...(sources ? { sources: sources.join(',') } : {}),
        ...(page?.limit !== undefined ? { limit: page.limit } : {}),
        ...(page?.offset !== undefined ? { offset: page.offset } : {}),
        // `ia=1` pide al Hub que entienda la frase antes de buscarla. Si el
        // servicio no está o falla, el Hub busca literal y no manda
        // `interpreted`: aquí no hay nada que atrapar.
        ...(page?.ia === true ? { ia: 1 } : {}),
      },
    });
    return {
      query: r.query,
      results: hitsToResults(r.hits),
      total: r.total ?? (r.hits ?? []).length,
      hasMore: r.has_more === true,
      fallidos: fallidosDe(r.sources),
      ...(r.interpreted ? { interpretado: interpretadoDe(r.interpreted) } : {}),
    };
  }

  /**
   * Actividad reciente entre productos: el MISMO fan-out federado que la
   * busqueda, sin termino, ordenado por recencia. El Hub responde el modelo
   * canonico de busqueda, asi que se aplana igual.
   *
   * `limit` es POR ORIGEN, no total: el Hub lo acota a su maximo configurado.
   */
  async recent(options?: { sources?: ProductKey[]; limit?: number }): Promise<SearchResult[]> {
    const r = await this.http.json<HubSearchWire>({
      path: '/recent',
      query: {
        ...(options?.sources ? { sources: options.sources.join(',') } : {}),
        ...(options?.limit !== undefined ? { limit: options.limit } : {}),
      },
    });
    return hitsToResults(r.hits);
  }

  /**
   * Notificaciones unificadas (paginadas). El Hub responde {notifications:[...]}. Con
   * `antes` —el último de la página anterior—, las de detrás: por (fecha, id), porque
   * con la fecha sola se perdían las del mismo instante en el corte.
   */
  async notifications(options?: {
    unreadOnly?: boolean;
    limit?: number;
    antes?: Pick<Notification, 'id' | 'created_at'>;
  }): Promise<Notification[]> {
    const r = await this.http.json<{ notifications: Notification[] }>({
      path: '/notifications',
      query: {
        unread: options?.unreadOnly ?? false,
        limit: options?.limit ?? 50,
        ...(options?.antes
          ? { before: options.antes.created_at, before_id: options.antes.id }
          : {}),
      },
    });
    return r.notifications ?? [];
  }

  /**
   * Cuantas hay sin leer, contadas EN LA BASE. Va aparte de `notifications()`
   * porque contar los `!read` de una pagina de 30 miente en cuanto hay 31: si
   * las 30 mas recientes estan leidas, la campana diria 0 con decenas detras.
   * Y, si Sytue Mail contesta, los correos sin leer (`mailUnread`, `null` si no se
   * sabe): el correo que ya esperaba antes de que Mail avisara es estado, no aviso,
   * y la campana lo enseña en las trece apps.
   */
  async contadores(): Promise<{ unread: number; mailUnread: number | null }> {
    const r = await this.http.json<{ unread?: number; mail_unread?: number }>({
      path: '/notifications/unread_count',
    });
    return { unread: r.unread ?? 0, mailUnread: r.mail_unread ?? null };
  }

  /**
   * Marca una notificacion como leida. Con `keepalive`: pulsar un aviso navega a su
   * elemento, y sin él la navegación cancelaba la petición y el aviso volvía sin leer.
   */
  async markRead(id: string): Promise<void> {
    await this.http.request({
      method: 'POST',
      path: `/notifications/${encodeURIComponent(id)}/read`,
      keepalive: true,
    });
  }

  /** Marca TODAS como leidas de una vez. */
  async markAllRead(): Promise<void> {
    await this.http.request({ method: 'POST', path: '/notifications/read_all' });
  }

  /**
   * Reglas de silencio del PROPIO usuario. El Hub no expone forma de leer ni
   * escribir las de otro, ni con scope: por eso no hay parametro de sujeto.
   */
  async notificationPrefs(): Promise<NotificationPref[]> {
    const r = await this.http.json<{ prefs: NotificationPref[] }>({
      path: '/notifications/prefs',
    });
    return r.prefs ?? [];
  }

  /**
   * Silencia —o vuelve a permitir— un origen entero, o un tipo dentro de el.
   * La regla MAS ESPECIFICA gana: `{source:'mail', kind:'share'}` manda sobre
   * `{source:'mail'}`, asi que silenciar todo el correo y reactivar un tipo
   * concreto es dos llamadas, no una imposible.
   */
  async setNotificationPref(pref: NotificationPref): Promise<NotificationPref> {
    return this.http.json<NotificationPref>({
      method: 'PUT',
      path: '/notifications/prefs',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ source: pref.source, kind: pref.kind ?? '', muted: pref.muted }),
    });
  }

  /**
   * Emite una notificacion (POST /notifications). Sin `subject` va al PROPIO
   * usuario y no requiere scope; el Hub la persiste y la empuja por SSE. Uso
   * server-side (BFF): notificar a terceros exige el scope notifications:write, y
   * entonces el Hub contesta 202 SIN cuerpo (ADR 0053: devolverla diría si esa
   * persona la tiene silenciada), así que el valor solo vale para uno mismo.
   */
  createNotification(input: NewNotification): Promise<Notification> {
    return this.http.json<Notification>({
      method: 'POST',
      path: '/notifications',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(input),
    });
  }

  /** Presencia de un conjunto de usuarios (Hub: GET /presence?subjects=a,b,c → {presence:[...]}). */
  async presence(userIds: string[]): Promise<Presence[]> {
    const subjects = encodeURIComponent(userIds.join(','));
    const r = await this.http.json<{ presence: Presence[] }>({
      method: 'GET',
      path: `/presence?subjects=${subjects}`,
    });
    return r.presence ?? [];
  }

  /** Publica la presencia propia (Hub: PUT /presence, cuerpo {state, status}). */
  async setPresence(state: PresenceState, message?: string): Promise<void> {
    await this.http.request({
      method: 'PUT',
      path: '/presence',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ state, status: message }),
    });
  }

  /**
   * Marca la presencia propia como desconectada AHORA (Hub: DELETE /presence),
   * en vez de esperar a que expire el TTL del último latido. Para logout y
   * cierre de pestaña/navegador — los dos momentos en que se SABE que la
   * sesión termina, a diferencia de una pérdida de red, donde expirar por TTL
   * sigue siendo lo correcto (podría volver).
   */
  async offline(): Promise<void> {
    // `keepalive`: la llamada suele salir justo antes de navegar (logout) o
    // durante `pagehide` (cierre de pestaña) — sin él, el navegador puede
    // cortar la petición a media entrega en cuanto el documento se descarga.
    await this.http.request({ method: 'DELETE', path: '/presence', keepalive: true });
  }

  /**
   * Resuelve un intent cross-app y devuelve el deep-link al que navegar. El Hub
   * NO ejecuta la acción: compone el enlace al compositor del producto destino
   * con los parámetros prerellenados, y quien autoriza y ejecuta sigue siendo ese
   * producto.
   */
  async dispatchIntent(intent: Intent): Promise<IntentResult> {
    return this.http.json<IntentResult>({
      method: 'POST',
      path: '/intents',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(intent),
    });
  }

  /**
   * Stream de notificaciones en tiempo real via SSE. Devuelve el EventSource
   * (el llamador escucha "message" y "close"). El token se pasa por query
   * porque EventSource no admite cabeceras; el Hub lo valida via JWKS de Sytue ID.
   */
  async streamNotifications(): Promise<EventSource> {
    const token = await this.auth.getAccessToken();
    const url = new URL(`${this.base}/api/v1/notifications/stream`);
    if (token) url.searchParams.set('access_token', token);
    return new EventSource(url.toString(), { withCredentials: true });
  }
}
