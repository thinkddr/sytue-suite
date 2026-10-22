/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Nucleo HTTP compartido por los SDKs a los servicios Go de Sytue One.
 * Autorizacion Bearer con el access token de Sytue ID (JWT validado por cada
 * servicio via JWKS; lo provee la app/BFF, el SDK no gestiona credenciales).
 * Maneja errores tipados y parseo de cuerpos JSON / texto / XML, y construye
 * URLs para canales en tiempo real (WebSocket / SSE).
 */

export interface AuthProvider {
  /** Devuelve el access token vigente (puede refrescarse de forma transparente). */
  getAccessToken: () => string | Promise<string>;
}

export interface HttpClientOptions {
  /** URL base del servicio (p.ej. https://drive.sytue.eu). */
  baseUrl: string;
  auth?: AuthProvider | undefined;
  /** Cabeceras por defecto. */
  defaultHeaders?: Record<string, string>;
  /** fetch inyectable (tests / entornos server). */
  fetchImpl?: typeof fetch | undefined;
}

export class HttpError extends Error {
  constructor(
    public readonly status: number,
    public readonly statusText: string,
    public readonly url: string,
    public readonly body?: string,
  ) {
    super(`HTTP ${status} ${statusText} en ${url}`);
    this.name = 'HttpError';
  }

  /**
   * `code` estable del error de API (httpx.Error en el backend). Es la clave por
   * la que el cliente traduce y decide, en vez de mirar el texto del mensaje o
   * solo el estado HTTP.
   *
   * `httpx.Fail` lo envuelve —`{"error":{"code":…}}`— y el proxy del BFF pasa ese
   * cuerpo tal cual; otros puntos lo ponen en la raíz. Se leen las dos formas: con
   * solo la raíz, ningún error de un servicio llegaba con su `code`.
   */
  get code(): string | undefined {
    return cuerpoDeError(this.body).code;
  }

  /** Provider cooldown, bounded to one day; never a reason to replay a write. */
  get retryAfterMs(): number {
    const params = cuerpoDeError(this.body).params;
    const seconds = params?.retry_after_seconds;
    return typeof seconds === 'number' && Number.isFinite(seconds) && seconds > 0
      ? Math.min(seconds, 86400) * 1000
      : 0;
  }

  /**
   * La sesión no tiene organización con derecho de uso: la suscripción de esa
   * organización no está activa todavía.
   *
   * Se distingue del 403 genérico porque la salida es distinta. No es un permiso
   * que pedirle a un administrador: es completar la suscripción. Mostrarlo como
   * un error de carga cualquiera —«no se pudieron cargar las carpetas»— dejaba
   * al usuario sin ninguna pista de qué le pasaba ni adónde ir, y ocurre a la
   * vez en las diez superficies.
   */
  get noActiveOrg(): boolean {
    return this.status === 403 && this.code === 'no_active_organization';
  }
}

/** A parsed response exceeded the caller's byte budget before JSON materialization. */
export class ResponseTooLargeError extends Error {
  constructor(public readonly maxBytes: number) {
    super(`HTTP response exceeds ${maxBytes} bytes`);
    this.name = 'ResponseTooLargeError';
  }
}

async function responseText(res: Response, maxBytes?: number): Promise<string> {
  if (maxBytes === undefined) return res.text();
  const encoding = res.headers.get('content-encoding');
  const advertised = res.headers.get('content-length')?.trim();
  // Fetch exposes decompressed bytes; a compressed Content-Length measures a different body.
  if (
    (!encoding || encoding.toLowerCase() === 'identity') &&
    advertised &&
    /^\d+$/.test(advertised) &&
    Number(advertised) > maxBytes
  ) {
    await res.body?.cancel().catch(() => undefined);
    throw new ResponseTooLargeError(maxBytes);
  }
  if (!res.body) return '';
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  const chunks: string[] = [];
  let bytes = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (value.byteLength > maxBytes - bytes) {
        throw new ResponseTooLargeError(maxBytes);
      }
      bytes += value.byteLength;
      chunks.push(decoder.decode(value, { stream: true }));
    }
    chunks.push(decoder.decode());
    return chunks.join('');
  } catch (error) {
    await reader.cancel(error).catch(() => undefined);
    throw error;
  } finally {
    reader.releaseLock();
  }
}

/**
 * El `code` y el `message` de un cuerpo de error de API.
 *
 * `httpx.Fail` los envuelve —`{"error":{"code":…,"message":…}}`— y otros puntos los
 * ponen en la raíz; un `error` que es TEXTO no es el sobre, y entonces manda la raíz.
 * Lo que no sea JSON, o no traiga cadenas, se queda sin nada en vez de lanzar.
 *
 * Existe aparte de `HttpError` para el BFF que reenvía la respuesta de un servicio sin
 * pasar por `HttpClient`: el de CertiPDF leía solo la raíz y, con el sobre, perdía el
 * código —un PDF con contraseña enseñaba «pdf_failed» en vez de pedirla—.
 */
export function cuerpoDeError(body?: string): {
  code?: string | undefined;
  message?: string | undefined;
  params?: Record<string, unknown> | undefined;
} {
  if (!body) return {};
  try {
    const raiz = JSON.parse(body) as Record<string, unknown> | null;
    if (!raiz || typeof raiz !== 'object') return {};
    const sobre =
      raiz.error && typeof raiz.error === 'object' ? (raiz.error as Record<string, unknown>) : raiz;
    const texto = (v: unknown): string | undefined => (typeof v === 'string' ? v : undefined);
    const params =
      sobre.params && typeof sobre.params === 'object'
        ? (sobre.params as Record<string, unknown>)
        : undefined;
    return { code: texto(sobre.code), message: texto(sobre.message), params };
  } catch {
    return {};
  }
}

/**
 * El estado con el que un BFF contesta por un error del servicio: su 4xx TAL CUAL
 * (401, 403, 404, 409, 412…) y 502 solo para lo demás (5xx, red, cuerpo roto).
 *
 * Convertir un 403 en 502 hacía que la web tomase «ya no tienes acceso» (una
 * compartición retirada) por una caída y lo enseñara como error genérico.
 */
export function upstreamStatus(err: unknown): number {
  return err instanceof HttpError && err.status >= 400 && err.status < 500 ? err.status : 502;
}

export interface RequestOptions {
  method?: string;
  path?: string;
  /** URL absoluta (ignora baseUrl/path si se indica). */
  url?: string;
  headers?: Record<string, string>;
  query?: Record<string, string | number | boolean | undefined>;
  body?: BodyInit | null;
  /** Si se omite la autorizacion (endpoints publicos). */
  anonymous?: boolean;
  /** AbortSignal para cancelar la peticion. */
  signal?: AbortSignal;
  /** Byte budget for json/text and HTTP error bodies; raw request() responses remain caller-owned. */
  maxResponseBytes?: number;
  /**
   * Sobrevive a que la página se descargue a mitad de la petición (logout,
   * cierre de pestaña): el navegador la sigue entregando aunque el documento ya
   * no exista. Sin esto, una petición disparada justo antes de navegar puede
   * cancelarse a medio camino sin que nadie se entere.
   */
  keepalive?: boolean;
}

export class HttpClient {
  private readonly fetchImpl: typeof fetch;

  constructor(private readonly options: HttpClientOptions) {
    this.fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
  }

  get baseUrl(): string {
    return this.options.baseUrl.replace(/\/$/, '');
  }

  /** Token vigente (o undefined si no hay AuthProvider). */
  async accessToken(): Promise<string | undefined> {
    return this.options.auth ? this.options.auth.getAccessToken() : undefined;
  }

  async request(opts: RequestOptions): Promise<Response> {
    if (
      opts.maxResponseBytes !== undefined &&
      (!Number.isSafeInteger(opts.maxResponseBytes) || opts.maxResponseBytes <= 0)
    )
      throw new RangeError('maxResponseBytes must be a positive safe integer');
    const url = this.buildUrl(opts);
    const headers = new Headers({ ...this.options.defaultHeaders, ...opts.headers });
    if (!opts.anonymous && this.options.auth) {
      const token = await this.options.auth.getAccessToken();
      headers.set('authorization', `Bearer ${token}`);
    }
    const res = await this.fetchImpl(url, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body ?? null,
      signal: opts.signal ?? null,
      ...(opts.keepalive ? { keepalive: true } : {}),
    });
    if (!res.ok) {
      const text = await responseText(res, opts.maxResponseBytes).catch((error: unknown) => {
        if (error instanceof ResponseTooLargeError) throw error;
        if (
          opts.maxResponseBytes !== undefined &&
          (opts.signal?.aborted ||
            (typeof error === 'object' &&
              error !== null &&
              'name' in error &&
              error.name === 'AbortError'))
        )
          throw error;
        return undefined;
      });
      throw new HttpError(res.status, res.statusText, url, text);
    }
    return res;
  }

  async json<T>(opts: RequestOptions): Promise<T> {
    const res = await this.request({
      ...opts,
      headers: { accept: 'application/json', ...opts.headers },
    });
    if (res.status === 204) return undefined as T;
    return opts.maxResponseBytes === undefined
      ? ((await res.json()) as T)
      : (JSON.parse(await responseText(res, opts.maxResponseBytes)) as T);
  }

  async text(opts: RequestOptions): Promise<string> {
    const res = await this.request(opts);
    return responseText(res, opts.maxResponseBytes);
  }

  /**
   * Servidores que ya han dicho que NO entienden QUERY.
   *
   * Compartido entre instancias a propósito: el soporte es del SERVIDOR, no del
   * objeto que lo llama, y crear un cliente nuevo por pantalla no debería volver
   * a pagar el viaje de descubrirlo. Solo crece —un servidor no deja de soportar
   * QUERY a mitad de sesión— así que no hay que invalidarlo.
   */
  private static readonly sinQuery = new Set<string>();

  /**
   * Una consulta: QUERY si el servidor lo entiende, GET si no.
   *
   * QUERY (RFC 10008) es seguro e idempotente como GET, pero lleva los
   * parámetros en el CUERPO. Se prefiere por dos razones que no se ven hasta que
   * fallan: una URL con una docena de identificadores roza los límites reales de
   * proxies y servidores —y al cruzarlos no hay error claro, hay truncado—, y lo
   * que alguien escribe para buscar deja de quedar escrito en el log de accesos
   * de cada salto y en el `Referer`.
   *
   * EL RESPALDO SE RECUERDA. Si el servidor contesta 405, 501 o 404, se apunta
   * y a partir de ahí se va directo por GET: probar en cada llamada costaría un
   * viaje de ida y vuelta por consulta contra un servidor viejo, que es
   * exactamente a quien menos conviene cargarle trabajo de más.
   *
   * EL 404 ENTRA EN LA LISTA POR ASTRO, que a un método sin handler le contesta
   * 404 y no 405 (`dist/runtime/server/endpoint.js`: `mod[method] ?? mod.ALL`).
   * Sin él, una consulta contra un BFF que todavía no hable QUERY no caía a GET
   * sino que fallaba en la cara de quien buscaba. Como el 404 también es «esa
   * ruta no existe», no se apunta a ciegas: decide el respaldo.
   *
   * Cualquier otro error se propaga tal cual: un 401 o un 500 no dicen nada
   * sobre si QUERY existe, y tragárselos para reintentar por GET convertiría un
   * fallo real en dos peticiones y el mismo fallo.
   */
  async consulta<T>(opts: RequestOptions): Promise<T> {
    const query = opts.query;
    if (query && !HttpClient.sinQuery.has(this.baseUrl)) {
      const cuerpo = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) {
        if (v !== undefined) cuerpo.append(k, String(v));
      }
      try {
        // La clave `query` se OMITE, no se pone a `undefined`: con
        // `exactOptionalPropertyTypes` no son lo mismo, y dejarla explícita no
        // compila.
        const { query: _enElCuerpo, ...resto } = opts;
        return await this.json<T>({
          ...resto,
          method: 'QUERY',
          body: cuerpo.toString(),
          headers: { 'content-type': 'application/x-www-form-urlencoded', ...opts.headers },
        });
      } catch (e) {
        if (!(e instanceof HttpError) || ![404, 405, 501].includes(e.status)) throw e;
        // El respaldo primero y la marca después: si el GET también falla, su
        // error es el que se propaga —era una ruta que no existe, no un QUERY
        // que no se entiende— y no se apunta nada.
        const respuesta = await this.json<T>(opts);
        HttpClient.sinQuery.add(this.baseUrl);
        return respuesta;
      }
    }
    return this.json<T>(opts);
  }

  /** Atajo JSON->JSON: serializa el cuerpo y fija content-type. */
  postJson<T>(
    path: string,
    body: unknown,
    opts?: Omit<RequestOptions, 'path' | 'body' | 'method'>,
  ): Promise<T> {
    return this.json<T>({
      ...opts,
      method: 'POST',
      path,
      headers: { 'content-type': 'application/json', ...opts?.headers },
      body: JSON.stringify(body),
    });
  }

  buildUrl(opts: Pick<RequestOptions, 'url' | 'path' | 'query'>): string {
    const raw = opts.url ?? `${this.baseUrl}${opts.path ?? ''}`;
    if (!opts.query) return raw;
    const url = new URL(raw);
    for (const [key, value] of Object.entries(opts.query)) {
      if (value !== undefined) url.searchParams.set(key, String(value));
    }
    return url.toString();
  }

  /** Convierte http(s):// en ws(s):// para endpoints WebSocket del mismo origen. */
  wsUrl(opts: Pick<RequestOptions, 'path' | 'query'>): string {
    return this.buildUrl(opts).replace(/^http/, 'ws');
  }
}

/** Helper para crear un AuthProvider a partir de un token estatico o getter. */
export function bearer(token: string | (() => string | Promise<string>)): AuthProvider {
  return { getAccessToken: typeof token === 'function' ? token : () => token };
}

/**
 * Resuelve `ref` contra el servicio configurado (`baseUrl`) y EXIGE que el
 * resultado quede en el MISMO origen. Lanza si no.
 *
 * POR QUÉ. Los SDK de CalDAV/CardDAV reciben la URL del recurso (calendario,
 * libreta, evento, contacto) desde el BFF, y el BFF la lee de la consulta del
 * navegador (`?calendarUrl=`, `?addressBookUrl=`, `href` del cuerpo). Cada
 * petición del SDK lleva `Authorization: Bearer <token de la sesión>`. Si se
 * aceptara un destino ABSOLUTO a otro host, ese token viajaría a él: robo de
 * token de acceso (y SSRF a la red interna). Con `SameSite=Lax` la variante GET
 * se dispara desde otra web con un clic. Un `ref` relativo (`/dav/...`) o uno
 * absoluto del propio servicio pasan; cualquier otro origen —incluido el
 * protocolo‑relativo `//otro`— se rechaza.
 */
export function sameOriginUrl(baseUrl: string, ref: string): string {
  const base = baseUrl.replace(/\/$/, '');
  const esperado = new URL(`${base}/`);
  let resuelto: URL;
  try {
    resuelto = new URL(ref, esperado);
  } catch {
    throw new Error('@sytue/api-clients: URL de recurso inválida.');
  }
  if (resuelto.origin !== esperado.origin) {
    throw new Error('@sytue/api-clients: el destino está fuera del servicio configurado.');
  }
  return resuelto.toString();
}
