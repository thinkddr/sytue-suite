/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Mail — SDK JMAP (RFC 8620 / 8621 Mail) contra el servidor Go propio
 * (mail.sytue.eu, dev :8086; SMTP/IMAP 25/465/587/993). Implementacion
 * clean-room sobre el estandar JMAP (NO Stalwart). El cliente propio Astro+Preact
 * tipo webmail consume este SDK. Maneja la sesion JMAP, mailboxes,
 * busqueda/lectura de Emails y envio (EmailSubmission). Estado en PostgreSQL,
 * blobs en S3. Autorizacion Bearer (JWT de Sytue ID).
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

const CORE = 'urn:ietf:params:jmap:core';
const MAIL = 'urn:ietf:params:jmap:mail';

export interface JmapOptions {
  /** URL del endpoint de sesion JMAP, p.ej. https://mail.sytue.eu/.well-known/jmap */
  sessionUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

export interface JmapSession {
  apiUrl: string;
  downloadUrl: string;
  uploadUrl: string;
  /** accountId primario para correo. */
  primaryAccountId: string;
  accounts: Record<string, { name: string }>;
}

export interface Mailbox {
  id: string;
  name: string;
  role?: string | undefined;
  totalEmails: number;
  unreadEmails: number;
  parentId?: string | null;
}

/**
 * Motivo por el que el servidor NO aplicó un Mailbox/set (`null` = aplicado).
 *
 * Se devuelve en vez de lanzar porque los tres primeros no son averías sino
 * respuestas legítimas que la pantalla tiene que explicar con palabras distintas
 * («esa carpeta es del sistema», «todavía tiene correo», «ya tienes una así»).
 * Un `throw` genérico los aplanaría todos a «no se pudo».
 */
export type MailboxSetError =
  | null
  | 'forbidden'
  | 'mailboxHasEmail'
  | 'alreadyExists'
  | 'invalidProperties'
  | 'notFound'
  | 'serverFail';

/** Lee el tipo de error que Mailbox/set dejó en notUpdated/notDestroyed. */
function setErrorType(bucket: unknown, id: string): MailboxSetError {
  const entry = (bucket as Record<string, { type?: string }> | undefined)?.[id];
  if (!entry) return null;
  return (entry.type as MailboxSetError) ?? 'serverFail';
}

export interface EmailAddress {
  name?: string;
  email: string;
}

export interface EmailSummary {
  id: string;
  threadId: string;
  subject: string;
  from: EmailAddress[];
  to: EmailAddress[];
  cc: EmailAddress[];
  preview: string;
  receivedAt: string;
  /** Keywords JMAP ($seen, $flagged, ...). */
  keywords: Record<string, boolean>;
  hasAttachment: boolean;
  /** Mensajes del hilo en TODA la cuenta, no solo en esta carpeta: es lo que abre
   *  el lector al pulsar la fila. Solo lo rellena el listado de bandeja. */
  threadCount?: number;
}

/** Adjunto saliente: se codifica base64 inline en el MIME multipart/mixed. */
export interface OutgoingAttachment {
  name: string;
  /** MIME type (p.ej. text/plain, application/pdf). */
  type: string;
  content: Uint8Array;
  /**
   * Identificador para incrustar la parte en el cuerpo (`<img src="cid:…">`).
   *
   * Con él, el mensaje se compone como `multipart/related` y la imagen se ve
   * DENTRO del texto; sin él va como fichero adjunto. La misma foto puede ir de
   * las dos maneras, y quien lo decide es el compositor al insertarla — no su
   * tipo MIME.
   */
  contentId?: string;
}

export interface OutgoingEmail {
  from: EmailAddress;
  to: EmailAddress[];
  cc?: EmailAddress[];
  subject: string;
  textBody?: string;
  htmlBody?: string;
  /** Adjuntos: si hay, el cuerpo se compone como multipart/mixed. */
  attachments?: OutgoingAttachment[];
}

/** Adjunto extraído de un correo leído (metadatos + bytes decodificados). */
export interface ParsedAttachment {
  name: string;
  type: string;
  size: number;
  content: Uint8Array;
  /**
   * `Content-ID` sin los ángulos, si la parte lo trae.
   *
   * Es lo que enlaza una imagen EN LÍNEA con el `<img src="cid:…">` del cuerpo
   * (RFC 2392). Sin él, el lector no tiene forma de saber qué parte pinta cada
   * referencia y la imagen queda rota: se compone con `cid:` al ENVIAR desde el
   * primer día, pero al LEER no lo resolvía nadie.
   */
  cid?: string;
  /**
   * La parte se mandó para VERSE dentro del mensaje, no para descargarla
   * (`Content-Disposition: inline`).
   *
   * No es lo mismo que traer `cid`. El iPhone manda las fotos así: un
   * `multipart/mixed` con el texto y las imágenes sueltas, marcadas `inline`,
   * SIN parte HTML y SIN `Content-ID` que las cite desde ningún sitio. Sin esta
   * marca no hay forma de distinguirlas de un fichero adjunto cualquiera, y el
   * mensaje se ve como un «Holaaa» a secas con tres ficheros al pie.
   */
  inline: boolean;
}

/** Cuerpo completo de un correo leído (el sobre indexado solo trae un extracto). */
export interface MessageBody {
  /** Cuerpo text/plain (LF), vacío si el mensaje solo trae HTML. */
  text: string;
  /** Cuerpo text/html crudo (el consumidor decide cómo aislarlo/sanearlo). */
  html: string;
}

/** Contenido editable de un borrador, para rehidratar el compositor al reabrirlo. */
export interface DraftContent {
  /** Cabecera To cruda ("Nombre <a@b>, c@d"), lista para el campo Para. */
  to: string;
  /** Cabecera Cc cruda. */
  cc: string;
  subject: string;
  textBody: string;
  /** Cuerpo text/html crudo, si el borrador lo tenía (formato enriquecido). */
  htmlBody: string;
  /** Ficheros e imágenes incrustadas del borrador (bytes + metadatos). */
  attachments: ParsedAttachment[];
}

/** Regla de filtrado del lado servidor (auto-mover/marcar el correo entrante). */
export interface MailFilter {
  id: string;
  name: string;
  enabled: boolean;
  priority: number;
  /** Subcadena a buscar en el remitente (dirección o nombre); vacío = no filtra. */
  matchFrom: string;
  /** Subcadena a buscar en el asunto; vacío = no filtra. */
  matchSubject: string;
  /** Id del buzón destino (mover) o null. */
  moveTo: string | null;
  markRead: boolean;
  markFlagged: boolean;
}

/** Campos para crear/actualizar una regla (los ausentes toman su valor por defecto). */
export interface MailFilterInput {
  name?: string;
  enabled?: boolean;
  priority?: number;
  matchFrom?: string;
  matchSubject?: string;
  moveTo?: string | null;
  markRead?: boolean;
  markFlagged?: boolean;
}

/** Tokens estructurados de una consulta de búsqueda (ver parseSearchQuery). */
export interface SearchTokens {
  /** Texto libre restante (FTS). */
  text: string;
  from?: string;
  subject?: string;
  hasAttachment?: boolean;
  /** Keyword requerido ($seen, $flagged) — is:read / is:starred. */
  hasKeyword?: string;
  /** Keyword excluido ($seen) — is:unread. */
  notKeyword?: string;
}

/**
 * Traduce la consulta del buscador a tokens de filtro Email/query. Operadores
 * soportados (estilo Gmail): `from:`, `subject:`, `has:attachment`,
 * `is:unread|read|starred|flagged`. El valor puede ir entre comillas para incluir
 * espacios (`subject:"pago pendiente"`). Todo lo demás es texto libre. Función
 * pura (sin red), testeable de forma unitaria.
 */
export function parseSearchQuery(q: string): SearchTokens {
  const out: SearchTokens = { text: '' };
  const free: string[] = [];
  let i = 0;
  while (i < q.length) {
    while (i < q.length && /\s/.test(q[i]!)) i++;
    if (i === q.length) break;

    const inicio = i;
    let finClave = i;
    while (
      (q.charCodeAt(finClave) >= 48 && q.charCodeAt(finClave) <= 57) ||
      (q.charCodeAt(finClave) >= 65 && q.charCodeAt(finClave) <= 90) ||
      (q.charCodeAt(finClave) >= 97 && q.charCodeAt(finClave) <= 122) ||
      q.charCodeAt(finClave) === 95
    ) {
      finClave++;
    }
    const clave =
      finClave > inicio && q[finClave] === ':'
        ? q.slice(inicio, finClave).toLowerCase()
        : undefined;
    if (clave !== undefined) i = finClave + 1;

    if (q[i] === '"') {
      const cierre = q.indexOf('"', i + 1);
      if (cierre >= 0) {
        const valor = q.slice(i + 1, cierre);
        if (clave !== undefined) applySearchOp(out, free, clave, valor);
        else free.push(valor);
        i = cierre + 1;
        continue;
      }
      if (clave === undefined) {
        i = inicio;
        while (i < q.length && !/\s/.test(q[i]!)) i++;
        free.push(q.slice(inicio, i));
        continue;
      }
    }

    const inicioValor = i;
    while (i < q.length && !/\s/.test(q[i]!)) i++;
    const valor = q.slice(inicioValor, i);
    if (clave !== undefined && valor.length > 0) applySearchOp(out, free, clave, valor);
    else free.push(q.slice(inicio, i));
  }
  out.text = free.join(' ').trim();
  return out;
}

// applySearchOp mapea un operador a su token; los desconocidos vuelven a texto libre.
function applySearchOp(out: SearchTokens, free: string[], key: string, val: string): void {
  switch (key) {
    case 'from':
      out.from = val;
      return;
    case 'subject':
      out.subject = val;
      return;
    case 'has':
      if (/^attachments?$/i.test(val)) out.hasAttachment = true;
      else free.push(`${key}:${val}`);
      return;
    case 'is':
      // ponytail: hasKeyword/notKeyword son un único valor cada uno; si se
      // combinan operadores is: en conflicto, gana el último. Suficiente hoy.
      if (/^unread$/i.test(val)) out.notKeyword = '$seen';
      else if (/^read$/i.test(val)) out.hasKeyword = '$seen';
      else if (/^(starred|flagged)$/i.test(val)) out.hasKeyword = '$flagged';
      else free.push(`${key}:${val}`);
      return;
    default:
      free.push(`${key}:${val}`);
  }
}

type JmapMethodCall = [string, Record<string, unknown>, string];

interface JmapResponse {
  methodResponses: JmapMethodCall[];
}

/**
 * Lo más largo que va DENTRO del JSON de /send y /drafts. El servicio corta el cuerpo
 * en 1 MiB y el mensaje viaja escapado (cada CRLF son cuatro caracteres): 768 KiB deja
 * margen de sobra. Lo que pase de aquí se sube aparte (`rawPayload`).
 */
export const MAX_RAW_EN_LINEA = 768 * 1024;

export class JmapClient {
  private readonly http: HttpClient;
  private session: JmapSession | null = null;

  constructor(private readonly options: JmapOptions) {
    this.http = new HttpClient({ baseUrl: '', auth: options.auth, fetchImpl: options.fetchImpl });
  }

  /** Obtiene (y cachea) la sesion JMAP. */
  async getSession(): Promise<JmapSession> {
    if (this.session) return this.session;
    const raw = await this.http.json<RawSession>({ method: 'GET', url: this.options.sessionUrl });
    this.session = {
      apiUrl: raw.apiUrl,
      downloadUrl: raw.downloadUrl,
      uploadUrl: raw.uploadUrl,
      primaryAccountId: raw.primaryAccounts[MAIL] ?? Object.keys(raw.accounts)[0] ?? '',
      accounts: raw.accounts,
    };
    return this.session;
  }

  /** Lista los mailboxes (carpetas) de la cuenta. */
  async listMailboxes(): Promise<Mailbox[]> {
    const session = await this.getSession();
    const res = await this.call(
      [['Mailbox/get', { accountId: session.primaryAccountId, ids: null }, '0']],
      [CORE, MAIL],
    );
    const list = (res.methodResponses[0]?.[1]?.list ?? []) as RawMailbox[];
    return list.map((m) => ({
      id: m.id,
      name: m.name,
      role: m.role ?? undefined,
      totalEmails: m.totalEmails ?? 0,
      unreadEmails: m.unreadEmails ?? 0,
      parentId: m.parentId ?? null,
    }));
  }

  /**
   * Renombra una carpeta propia (Mailbox/set update). El servidor rechaza las de
   * sistema (Recibidos, Enviados…) y los nombres repetidos o inválidos.
   */
  async renameMailbox(id: string, name: string): Promise<MailboxSetError> {
    const session = await this.getSession();
    const res = await this.call(
      [['Mailbox/set', { accountId: session.primaryAccountId, update: { [id]: { name } } }, '0']],
      [CORE, MAIL],
    );
    return setErrorType(res.methodResponses[0]?.[1]?.notUpdated, id);
  }

  /**
   * Borra una carpeta propia y VACÍA (Mailbox/set destroy). El servidor niega las
   * de sistema y las que aún tienen correo: borrarla arrastraría los mensajes.
   */
  async deleteMailbox(id: string): Promise<MailboxSetError> {
    const session = await this.getSession();
    const res = await this.call(
      [['Mailbox/set', { accountId: session.primaryAccountId, destroy: [id] }, '0']],
      [CORE, MAIL],
    );
    return setErrorType(res.methodResponses[0]?.[1]?.notDestroyed, id);
  }

  /**
   * Ejecuta Email/query con el filtro dado y luego Email/get de los ids
   * resultantes. Base comun de listEmails y search.
   *
   * Dos viajes (no back-reference `#ids` en una sola peticion) porque el servidor
   * Go no resuelve ResultReference (RFC 8620 §3.7): Email/get necesita los ids
   * literales, asi que primero leemos los ids de Email/query y luego los pasamos.
   */
  private async queryEmails(
    filter: Record<string, unknown>,
    options?: { limit?: number; position?: number },
  ): Promise<EmailSummary[]> {
    const session = await this.getSession();
    const queryRes = await this.call(
      [
        [
          'Email/query',
          {
            accountId: session.primaryAccountId,
            filter,
            sort: [{ property: 'receivedAt', isAscending: false }],
            position: options?.position ?? 0,
            limit: options?.limit ?? 50,
          },
          '0',
        ],
      ],
      [CORE, MAIL],
    );
    const ids = (queryRes.methodResponses[0]?.[1]?.ids ?? []) as string[];
    if (ids.length === 0) return [];

    const getRes = await this.call(
      [
        [
          'Email/get',
          {
            accountId: session.primaryAccountId,
            ids,
            properties: [
              'id',
              'threadId',
              'subject',
              'from',
              'to',
              'cc',
              'preview',
              'receivedAt',
              'keywords',
              'hasAttachment',
            ],
          },
          '0',
        ],
      ],
      [CORE, MAIL],
    );
    const list = (getRes.methodResponses[0]?.[1]?.list ?? []) as RawEmail[];
    return list.map(mapEmail);
  }

  /**
   * Lista correos de un mailbox (Email/query filter.inMailbox). position/limit
   * para paginacion.
   */
  async listEmails(
    mailboxId: string,
    options?: { limit?: number; position?: number },
  ): Promise<EmailSummary[]> {
    return this.queryEmails({ inMailbox: mailboxId }, options);
  }

  /**
   * Busca correos. La consulta admite operadores estilo Gmail (from:, subject:,
   * has:attachment, is:unread|read|starred; valores con espacios entre comillas);
   * `parseSearchQuery` los traduce a un filtro Email/query estructurado y el resto
   * va como texto libre (FTS del servidor sobre asunto + remitente + cuerpo, RFC
   * 8621 §4.4.1). Opcionalmente restringe a un mailbox. Sobres más recientes primero.
   */
  async search(
    query: string,
    options?: { limit?: number; position?: number; mailboxId?: string },
  ): Promise<EmailSummary[]> {
    const t = parseSearchQuery(query);
    const filter: Record<string, unknown> = {};
    if (t.text) filter.text = t.text;
    if (t.from) filter.from = t.from;
    if (t.subject) filter.subject = t.subject;
    if (t.hasAttachment) filter.hasAttachment = true;
    if (t.hasKeyword) filter.hasKeyword = t.hasKeyword;
    if (t.notKeyword) filter.notKeyword = t.notKeyword;
    // Sin ningún criterio (p. ej. query solo de puntuación): cae a texto libre,
    // así nunca se manda un filtro vacío que devolvería toda la cuenta.
    if (Object.keys(filter).length === 0) filter.text = query.trim();
    if (options?.mailboxId) filter.inMailbox = options.mailboxId;
    return this.queryEmails(filter, options);
  }

  /** Lista las reglas de filtrado de la cuenta (gestión). */
  async listFilters(): Promise<MailFilter[]> {
    const origin = new URL(this.options.sessionUrl).origin;
    const res = await this.http.json<{ filters: RawFilter[] }>({
      method: 'GET',
      url: `${origin}/api/v1/filters`,
    });
    return (res.filters ?? []).map(mapFilter);
  }

  /** Crea una regla de filtrado; devuelve la persistida (con id). */
  async createFilter(input: MailFilterInput): Promise<MailFilter> {
    const origin = new URL(this.options.sessionUrl).origin;
    const res = await this.http.json<RawFilter>({
      method: 'POST',
      url: `${origin}/api/v1/filters`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(filterBody(input)),
    });
    return mapFilter(res);
  }

  /** Actualiza una regla existente (reemplazo de campos mutables). */
  async updateFilter(id: string, input: MailFilterInput): Promise<void> {
    const origin = new URL(this.options.sessionUrl).origin;
    await this.http.request({
      method: 'PUT',
      url: `${origin}/api/v1/filters/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(filterBody(input)),
    });
  }

  /** Borra una regla de filtrado. */
  async deleteFilter(id: string): Promise<void> {
    const origin = new URL(this.options.sessionUrl).origin;
    await this.http.request({
      method: 'DELETE',
      url: `${origin}/api/v1/filters/${encodeURIComponent(id)}`,
    });
  }

  /** Marca un correo como leido/no leido ($seen). */
  async setSeen(emailId: string, seen: boolean): Promise<void> {
    const session = await this.getSession();
    await this.call(
      [
        [
          'Email/set',
          {
            accountId: session.primaryAccountId,
            update: { [emailId]: { 'keywords/$seen': seen ? true : null } },
          },
          '0',
        ],
      ],
      [CORE, MAIL],
    );
  }

  /**
   * Destaca/quita la estrella de un correo ($flagged ↔ \Flagged) por el endpoint
   * REST del servicio (POST /api/v1/messages/{id}/flags), que espera el flag en
   * formato IMAP.
   */
  async setFlagged(emailId: string, flagged: boolean): Promise<void> {
    const origin = new URL(this.options.sessionUrl).origin;
    await this.http.request({
      method: 'POST',
      url: `${origin}/api/v1/messages/${encodeURIComponent(emailId)}/flags`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ flag: '\\Flagged', add: flagged }),
    });
  }

  /**
   * Mueve un correo a otra carpeta (POST /api/v1/messages/{id}/move). Cubre
   * archivar (role=archive), papelera (role=trash) y «Mover a…».
   */
  async moveMessage(emailId: string, mailboxId: string): Promise<void> {
    const origin = new URL(this.options.sessionUrl).origin;
    await this.http.request({
      method: 'POST',
      url: `${origin}/api/v1/messages/${encodeURIComponent(emailId)}/move`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ mailbox_id: mailboxId }),
    });
  }

  /**
   * Envia un correo por el endpoint REST del servicio (POST /api/v1/send con el
   * mensaje RFC 5322 crudo): el servidor firma DKIM y lo encola. El servicio Go
   * no implementa EmailSubmission/set JMAP en esta versión (decisión explícita);
   * este SDK compone el raw. Con adjuntos el cuerpo se compone como
   * multipart/mixed (bytes base64 inline): el endpoint /send solo acepta raw, no
   * referencia blobIds (auditado), así que el adjunto viaja en el propio mensaje.
   */
  async send(email: OutgoingEmail): Promise<void> {
    const origin = new URL(this.options.sessionUrl).origin;
    const recipients = [...email.to, ...(email.cc ?? [])].map((a) => a.email);
    await this.http.request({
      method: 'POST',
      url: `${origin}/api/v1/send`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        from: email.from.email,
        recipients,
        ...(await this.rawPayload(buildRfc5322(email))),
      }),
    });
  }

  /**
   * El mensaje para /send y /drafts: dentro del JSON (`raw`) si cabe en el MiB que el
   * servicio admite (`httpx.DecodeJSON`), o subido antes por /jmap/upload y
   * referenciado (`raw_blob_id`). Un adjunto de 800 KB en base64 ya no cabía y el
   * envío fallaba con `body_too_large`, aunque el compositor admitiera 2 GB.
   */
  async rawPayload(raw: string): Promise<{ raw: string } | { raw_blob_id: string }> {
    if (raw.length <= MAX_RAW_EN_LINEA) return { raw };
    const origin = new URL(this.options.sessionUrl).origin;
    const { primaryAccountId } = await this.getSession();
    const res = await this.http.json<{ blobId: string }>({
      method: 'POST',
      url: `${origin}/jmap/upload/${encodeURIComponent(primaryAccountId)}`,
      headers: { 'content-type': 'message/rfc822' },
      body: raw,
    });
    return { raw_blob_id: res.blobId };
  }

  /**
   * Guarda un borrador: compone el mismo RFC 5322 que `send` pero lo entrega al
   * endpoint REST POST /api/v1/drafts, que lo materializa en la carpeta Drafts
   * (\Draft) SIN encolar el envío. Devuelve el id del mensaje creado.
   */
  async saveDraft(email: OutgoingEmail): Promise<{ id: string }> {
    const origin = new URL(this.options.sessionUrl).origin;
    const recipients = [...email.to, ...(email.cc ?? [])].map((a) => a.email);
    const res = await this.http.json<{ id: string }>({
      method: 'POST',
      url: `${origin}/api/v1/drafts`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        from: email.from.email,
        recipients,
        ...(await this.rawPayload(buildRfc5322(email))),
      }),
    });
    return { id: res.id };
  }

  /**
   * Actualiza un borrador (PUT /api/v1/drafts/{id}). El servidor reemplaza el
   * mensaje: el id cambia y se devuelve el nuevo, que el cliente debe adoptar.
   */
  async updateDraft(id: string, email: OutgoingEmail): Promise<{ id: string }> {
    const origin = new URL(this.options.sessionUrl).origin;
    const recipients = [...email.to, ...(email.cc ?? [])].map((a) => a.email);
    const res = await this.http.json<{ id: string }>({
      method: 'PUT',
      url: `${origin}/api/v1/drafts/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        from: email.from.email,
        recipients,
        ...(await this.rawPayload(buildRfc5322(email))),
      }),
    });
    return { id: res.id };
  }

  /** Descarta un borrador (DELETE /api/v1/drafts/{id}; soft-delete en el servidor). */
  async deleteDraft(id: string): Promise<void> {
    const origin = new URL(this.options.sessionUrl).origin;
    await this.http.request({
      method: 'DELETE',
      url: `${origin}/api/v1/drafts/${encodeURIComponent(id)}`,
    });
  }

  /**
   * Lee el contenido editable de un borrador para reabrir el compositor: descarga
   * el .eml crudo (GET /api/v1/messages/{id}/raw) y extrae destinatarios, asunto y
   * cuerpo de texto. El sobre indexado (Email/get) solo trae un extracto truncado
   * del cuerpo, así que aquí se parsea el mensaje completo para no perder texto.
   */
  async readDraft(id: string): Promise<DraftContent> {
    const origin = new URL(this.options.sessionUrl).origin;
    const res = await this.http.request({
      method: 'GET',
      url: `${origin}/api/v1/messages/${encodeURIComponent(id)}/raw`,
    });
    return parseDraftContent(new Uint8Array(await res.arrayBuffer()));
  }

  /**
   * Lee el cuerpo completo (texto y HTML) de un correo. El sobre de Email/get
   * solo trae `preview` (extracto truncado por el servidor Go), así que se
   * descarga el .eml crudo (GET /api/v1/messages/{id}/raw) y se extraen las
   * partes text/plain y text/html del MIME.
   */
  async getMessageBody(emailId: string): Promise<MessageBody> {
    const origin = new URL(this.options.sessionUrl).origin;
    const res = await this.http.request({
      method: 'GET',
      url: `${origin}/api/v1/messages/${encodeURIComponent(emailId)}/raw`,
    });
    return parseMessageBody(new Uint8Array(await res.arrayBuffer()));
  }

  /**
   * Extrae los adjuntos de un correo leído. El servidor Go solo persiste el .eml
   * crudo y un flag has_attachment (no expone adjuntos como blobs por parte, ni el
   * download por blobId sirve para ellos —auditado), así que descargamos el .eml
   * (GET /api/v1/messages/{id}/raw) y parseamos el MIME multipart aquí.
   */
  async getAttachments(emailId: string): Promise<ParsedAttachment[]> {
    const origin = new URL(this.options.sessionUrl).origin;
    const res = await this.http.request({
      method: 'GET',
      url: `${origin}/api/v1/messages/${encodeURIComponent(emailId)}/raw`,
    });
    return parseAttachments(new Uint8Array(await res.arrayBuffer()));
  }

  private async call(methodCalls: JmapMethodCall[], using: string[]): Promise<JmapResponse> {
    const session = await this.getSession();
    return this.http.json<JmapResponse>({
      method: 'POST',
      url: session.apiUrl,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ using, methodCalls }),
    });
  }
}

// ---- formas crudas JMAP ----

interface RawSession {
  apiUrl: string;
  downloadUrl: string;
  uploadUrl: string;
  primaryAccounts: Record<string, string>;
  accounts: Record<string, { name: string }>;
}

interface RawMailbox {
  id: string;
  name: string;
  role?: string | null;
  totalEmails?: number;
  unreadEmails?: number;
  parentId?: string | null;
}

/**
 * Forma cruda de un mensaje en la respuesta de Email/get del servidor Go, que
 * serializa su domain.Message (REST-ish, no el objeto Email JMAP): direcciones
 * como cadenas simples, flags IMAP (`\Seen`) en vez de keywords JMAP, y claves
 * snake_case. mapEmail lo normaliza al EmailSummary del SDK.
 */
interface RawEmail {
  id: string;
  thread_id?: string;
  subject?: string;
  from?: string;
  from_name?: string;
  to?: string[];
  cc?: string[];
  preview?: string;
  received_at?: string;
  keywords?: string[];
  has_attachment?: boolean;
}

/** Flag IMAP (\Seen) → keyword JMAP ($seen). El resto se pasa en minúsculas. */
const FLAG_TO_KEYWORD: Record<string, string> = {
  '\\Seen': '$seen',
  '\\Flagged': '$flagged',
  '\\Draft': '$draft',
  '\\Answered': '$answered',
  '\\Deleted': '$deleted',
};

function flagsToKeywords(flags?: string[]): Record<string, boolean> {
  const kw: Record<string, boolean> = {};
  for (const f of flags ?? []) kw[FLAG_TO_KEYWORD[f] ?? f.toLowerCase()] = true;
  return kw;
}

/** Forma cruda de una regla en la respuesta del servidor Go (domain.Filter). */
interface RawFilter {
  id: string;
  name?: string;
  enabled?: boolean;
  priority?: number;
  match_from?: string;
  match_subject?: string;
  move_to?: string | null;
  mark_read?: boolean;
  mark_flagged?: boolean;
}

/** Normaliza una regla cruda (snake_case del Go) a MailFilter (camelCase). */
function mapFilter(f: RawFilter): MailFilter {
  return {
    id: f.id,
    name: f.name ?? '',
    enabled: f.enabled ?? true,
    priority: f.priority ?? 0,
    matchFrom: f.match_from ?? '',
    matchSubject: f.match_subject ?? '',
    moveTo: f.move_to ?? null,
    markRead: Boolean(f.mark_read),
    markFlagged: Boolean(f.mark_flagged),
  };
}

/** Cuerpo JSON (snake_case) para crear/actualizar una regla en el servicio Go. */
function filterBody(f: MailFilterInput): Record<string, unknown> {
  return {
    name: f.name ?? '',
    enabled: f.enabled ?? true,
    priority: f.priority ?? 0,
    match_from: f.matchFrom ?? '',
    match_subject: f.matchSubject ?? '',
    move_to: f.moveTo ?? null,
    mark_read: f.markRead ?? false,
    mark_flagged: f.markFlagged ?? false,
  };
}

/** Normaliza un mensaje crudo de Email/get (domain.Message del Go) a EmailSummary. */
function mapEmail(e: RawEmail): EmailSummary {
  return {
    id: e.id,
    threadId: e.thread_id ?? '',
    subject: e.subject ?? '(Sin asunto)',
    from: e.from ? [{ ...(e.from_name ? { name: e.from_name } : {}), email: e.from }] : [],
    to: (e.to ?? []).map((email) => ({ email })),
    cc: (e.cc ?? []).map((email) => ({ email })),
    preview: e.preview ?? '',
    receivedAt: e.received_at ?? '',
    keywords: flagsToKeywords(e.keywords),
    hasAttachment: Boolean(e.has_attachment),
  };
}

/**
 * Dirección para cabecera: `Nombre <a@b>`, con el display-name ENTRECOMILLADO en
 * cuanto lleva un carácter especial de RFC 5322.
 *
 * Sin las comillas, un nombre que es a su vez una dirección —lo que pasa cuando
 * la cuenta no tiene nombre y cae a su correo— salía como
 * `From: ana@uno.example <ana@dos.example>`. Esa cabecera no es válida, el
 * receptor la descarta ENTERA y el correo aparece en su bandeja sin remitente:
 * «Desconocido», sin búsqueda por remitente y sin Para al responder.
 *
 * Los saltos de línea se quitan antes que nada: son inyección de cabeceras, no
 * un problema de formato.
 */
function formatAddr(a: EmailAddress): string {
  const nombre = a.name?.replace(/[\r\n]+/g, ' ').trim();
  if (!nombre) return a.email;
  if (!/[()<>[\]:;@\\,."]/.test(nombre)) return `${nombre} <${a.email}>`;
  return `"${nombre.replace(/(["\\])/g, '\\$1')}" <${a.email}>`;
}

/** Codifica una cabecera con no-ASCII como encoded-word UTF-8 (RFC 2047). */
function headerText(v: string): string {
  if (/^[\x20-\x7e]*$/.test(v)) return v;
  const bytes = new TextEncoder().encode(v);
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return `=?utf-8?B?${btoa(bin)}?=`;
}

/**
 * Compone un mensaje RFC 5322 (html o texto, utf-8 8bit). Con adjuntos usa
 * multipart/mixed: el cuerpo como primera parte y cada adjunto como parte base64.
 */
function buildRfc5322(email: OutgoingEmail): string {
  const isHtml = email.htmlBody != null;
  const body = (isHtml ? email.htmlBody : email.textBody) ?? '';
  const bodyType = isHtml ? 'text/html' : 'text/plain';
  const normBody = body.replace(/\r?\n/g, '\r\n');
  const baseHeaders = [
    `From: ${formatAddr(email.from)}`,
    `To: ${email.to.map(formatAddr).join(', ')}`,
    ...(email.cc?.length ? [`Cc: ${email.cc.map(formatAddr).join(', ')}`] : []),
    `Subject: ${headerText(email.subject)}`,
    `Date: ${new Date().toUTCString()}`,
    `Message-ID: <${crypto.randomUUID()}@mail.sytue>`,
    'MIME-Version: 1.0',
  ];

  const atts = email.attachments ?? [];
  if (atts.length === 0) {
    return `${[
      ...baseHeaders,
      `Content-Type: ${bodyType}; charset=utf-8`,
      'Content-Transfer-Encoding: 8bit',
    ].join('\r\n')}\r\n\r\n${normBody}`;
  }

  const bnd = `=_sytue_${crypto.randomUUID()}`;
  const parts = [
    `--${bnd}`,
    `Content-Type: ${bodyType}; charset=utf-8`,
    'Content-Transfer-Encoding: 8bit',
    '',
    normBody,
  ];
  for (const a of atts) {
    const name = headerText(a.name.replace(/[\r\n"]/g, ''));
    parts.push(
      `--${bnd}`,
      `Content-Type: ${sanitizeMimeType(a.type)}; name="${name}"`,
      'Content-Transfer-Encoding: base64',
      `Content-Disposition: attachment; filename="${name}"`,
      '',
      base64Wrap(a.content),
    );
  }
  parts.push(`--${bnd}--`, '');
  return `${[...baseHeaders, `Content-Type: multipart/mixed; boundary="${bnd}"`].join('\r\n')}\r\n\r\n${parts.join('\r\n')}`;
}

/** MIME type saneado (sin CR/LF ni parámetros), con fallback octet-stream. */
function sanitizeMimeType(t: string): string {
  const clean = (t || '')
    .split(';')[0]!
    .replace(/[\r\n]/g, '')
    .trim();
  return /^[\w.+-]+\/[\w.+-]+$/.test(clean) ? clean : 'application/octet-stream';
}

/** Uint8Array → cadena binaria (un char por byte), para btoa/atob. */
function bytesToBinary(bytes: Uint8Array): string {
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return s;
}

/** Cadena binaria → Uint8Array. */
function binaryToBytes(bin: string): Uint8Array {
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i) & 0xff;
  return out;
}

/** base64 de unos bytes, envuelto a 76 columnas (RFC 2045). */
function base64Wrap(bytes: Uint8Array): string {
  const b64 = btoa(bytesToBinary(bytes));
  return b64.replace(/(.{76})/g, '$1\r\n');
}

// ---- parser MIME de adjuntos (lectura) ----
//
// ponytail: parser mínimo para multipart/mixed|related|alternative anidado con
// CTE base64/quoted-printable/8bit y nombres RFC 2047. No cubre RFC 2231
// (filename*=) ni message/rfc822 anidado; si aparecen, súbelo a un parser real.

/**
 * Extrae los adjuntos de un mensaje RFC 5322 crudo. Un adjunto es cualquier parte
 * hoja con Content-Disposition: attachment o con nombre de fichero.
 */
export function parseAttachments(raw: Uint8Array | string): ParsedAttachment[] {
  const bin = typeof raw === 'string' ? raw : bytesToBinary(raw);
  return collectAttachments(bin);
}

/**
 * Extrae el contenido editable (To/Cc/Asunto/cuerpo, formato y adjuntos) de un
 * borrador RFC 5322 crudo para rehidratar el compositor. Reutiliza el MISMO
 * walker MIME que ya usan `parseAttachments`/`parseMessageBody` para un correo
 * LEÍDO: un borrador guardado por `saveDraft`/`updateDraft` (con `htmlBody` +
 * `attachments`, ver DraftInput del BFF de Mail) es, de puertas para adentro, el
 * mismo tipo de mensaje MIME — no hace falta un camino de lectura aparte.
 *
 * Antes solo se leía el text/plain: reabrir un borrador con formato o imágenes
 * incrustadas los mostraba como si nunca hubieran existido, y el siguiente
 * autoguardado los borraba DE VERDAD (sobrescribía el borrador con lo que había
 * en pantalla, que ya no los tenía).
 */
export function parseDraftContent(raw: Uint8Array | string): DraftContent {
  const bin = typeof raw === 'string' ? raw : bytesToBinary(raw);
  const { headers, body } = splitHeaderBody(bin);
  return {
    to: decodeEncodedWords(headers['to'] ?? ''),
    cc: decodeEncodedWords(headers['cc'] ?? ''),
    subject: decodeEncodedWords(headers['subject'] ?? ''),
    // Normaliza CRLF→LF: el <textarea> del compositor trabaja con \n.
    textBody: extractTextBody(headers, body).replace(/\r\n/g, '\n'),
    htmlBody: extractBody(headers, body, 'text/html'),
    attachments: collectAttachments(bin),
  };
}

/**
 * Extrae los cuerpos text/plain y text/html de un mensaje RFC 5322 crudo,
 * recorriendo multipart anidado (alternative/related/mixed) y saltando las
 * partes marcadas como adjunto. Honra el charset declarado de cada parte.
 */
export function parseMessageBody(raw: Uint8Array | string): MessageBody {
  const bin = typeof raw === 'string' ? raw : bytesToBinary(raw);
  const { headers, body } = splitHeaderBody(bin);
  return {
    // El lector y el compositor trabajan con LF.
    text: extractBody(headers, body, 'text/plain').replace(/\r\n/g, '\n'),
    html: extractBody(headers, body, 'text/html'),
  };
}

/** Primer cuerpo text/plain de un mensaje (recorre multipart anidado). */
function extractTextBody(headers: Record<string, string>, body: string): string {
  return extractBody(headers, body, 'text/plain');
}

/** Primera parte hoja no-adjunto del tipo pedido (recorre multipart anidado). */
function extractBody(headers: Record<string, string>, body: string, mime: string): string {
  const ct = headers['content-type'] ?? 'text/plain';
  const boundary = mimeParam(ct, 'boundary');
  if (/^multipart\//i.test(ct.trim()) && boundary) {
    for (const part of splitParts(body, boundary)) {
      const { headers: ph, body: pb } = splitHeaderBody(part);
      const nested = extractBody(ph, pb, mime);
      if (nested) return nested;
    }
    return '';
  }
  if (!ct.trim().toLowerCase().startsWith(mime)) return '';
  // Una parte text/* con disposición attachment es un fichero, no el cuerpo.
  if (/^\s*attachment/i.test(headers['content-disposition'] ?? '')) return '';
  const cte = (headers['content-transfer-encoding'] ?? '7bit').trim().toLowerCase();
  return decodeText(decodePartBody(body, cte), mimeParam(ct, 'charset'));
}

/** Bytes → texto honrando el charset declarado (fallback utf-8). */
function decodeText(bytes: Uint8Array, charset: string): string {
  try {
    return new TextDecoder(charset || 'utf-8').decode(bytes);
  } catch {
    return new TextDecoder('utf-8').decode(bytes);
  }
}

function collectAttachments(section: string): ParsedAttachment[] {
  const { headers, body } = splitHeaderBody(section);
  const ct = headers['content-type'] ?? 'text/plain';
  const boundary = mimeParam(ct, 'boundary');
  if (/^multipart\//i.test(ct.trim()) && boundary) {
    return splitParts(body, boundary).flatMap(collectAttachments);
  }
  const disp = headers['content-disposition'] ?? '';
  const filename = mimeParam(disp, 'filename') || mimeParam(ct, 'name');
  // `Content-ID` entre ángulos, que es como viaja (RFC 2392).
  const cid = (headers['content-id'] ?? '').trim().replace(/^<|>$/g, '');
  // UNA PARTE CON `Content-ID` CUENTA AUNQUE NO TENGA NOMBRE DE FICHERO. Muchos
  // clientes mandan la imagen en línea con `Content-Disposition: inline` y sin
  // `filename`: solo el cid. Antes se descartaba ahí mismo, así que el cuerpo
  // apuntaba con `cid:` a una parte que el lector ni siquiera había recogido.
  const isAttachment = /^\s*attachment/i.test(disp) || Boolean(filename) || Boolean(cid);
  if (!isAttachment) return [];
  const inline = /^\s*inline/i.test(disp);
  const cte = (headers['content-transfer-encoding'] ?? '7bit').trim().toLowerCase();
  const content = decodePartBody(body, cte);
  return [
    {
      name: decodeEncodedWords(filename || 'adjunto'),
      type:
        ct
          .split(';')[0]!
          .replace(/[\r\n]/g, '')
          .trim() || 'application/octet-stream',
      size: content.length,
      content,
      inline,
      ...(cid ? { cid } : {}),
    },
  ];
}

/** Separa cabeceras y cuerpo por la primera línea en blanco; desdobla cabeceras. */
function splitHeaderBody(section: string): { headers: Record<string, string>; body: string } {
  const m = /\r?\n\r?\n/.exec(section);
  const rawHeaders = m ? section.slice(0, m.index) : section;
  const body = m ? section.slice(m.index + m[0].length) : '';
  const headers: Record<string, string> = {};
  // Desdoblado RFC 5322: una línea que empieza por espacio/tab continúa la previa.
  const unfolded = rawHeaders.replace(/\r?\n[ \t]+/g, ' ');
  for (const line of unfolded.split(/\r?\n/)) {
    const c = line.indexOf(':');
    if (c > 0) headers[line.slice(0, c).trim().toLowerCase()] = line.slice(c + 1).trim();
  }
  return { headers, body };
}

/** Divide el cuerpo multipart por el boundary, descartando preámbulo y epílogo. */
function splitParts(body: string, boundary: string): string[] {
  const segs = body.split('--' + boundary);
  const out: string[] = [];
  for (let i = 1; i < segs.length; i++) {
    let s = segs[i]!;
    if (s.startsWith('--')) break; // delimitador de cierre → fin
    s = s.replace(/^\r?\n/, '').replace(/\r?\n$/, '');
    out.push(s);
  }
  return out;
}

/** Valor de un parámetro MIME (boundary, filename, name), con o sin comillas. */
function mimeParam(header: string, name: string): string {
  const re = new RegExp(name + '\\s*=\\s*(?:"([^"]*)"|([^;\\s]+))', 'i');
  const m = re.exec(header);
  return m ? (m[1] ?? m[2] ?? '') : '';
}

/** Decodifica el cuerpo de una parte según su Content-Transfer-Encoding. */
function decodePartBody(body: string, cte: string): Uint8Array {
  if (cte === 'base64') return binaryToBytes(atob(body.replace(/[^A-Za-z0-9+/=]/g, '')));
  if (cte === 'quoted-printable') return binaryToBytes(decodeQuotedPrintable(body));
  return binaryToBytes(body); // 7bit / 8bit / binary / desconocido
}

/** Quoted-printable (RFC 2045 §6.7): =XX y saltos blandos (=\r\n). */
function decodeQuotedPrintable(s: string): string {
  return s
    .replace(/=\r?\n/g, '')
    .replace(/=([0-9A-Fa-f]{2})/g, (_, hex) => String.fromCharCode(parseInt(String(hex), 16)));
}

/** Decodifica encoded-words RFC 2047 (=?charset?B|Q?texto?=) a texto Unicode. */
function decodeEncodedWords(s: string): string {
  return s.replace(/=\?([^?]+)\?([BbQq])\?([^?]*)\?=/g, (_, charset, enc, text) => {
    try {
      const bin =
        String(enc).toUpperCase() === 'B'
          ? atob(String(text))
          : decodeQuotedPrintable(String(text).replace(/_/g, ' '));
      return new TextDecoder(String(charset).trim() || 'utf-8').decode(binaryToBytes(bin));
    } catch {
      return String(text);
    }
  });
}
