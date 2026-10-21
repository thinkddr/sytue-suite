/**
 * Cliente de navegador para Sytue Mail: habla con los endpoints BFF same-origin
 * (cookie de sesion). El servidor reenvia al servicio Go Sytue Mail via el SDK
 * JMAP. Asi el
 * access token nunca toca el navegador para el correo (solo la shell lo usa para
 * el Hub). Reexporta los tipos del SDK @sytue/api-clients para coherencia.
 */
import type { Mailbox, EmailSummary, EmailAddress, MessageBody } from '@sytue/api-clients';
import type { MailFilter, MailFilterInput } from '@sytue/api-clients/mail';
/*
 * `withBase` en la vuelta al login: la app se sirve bajo su prefijo en el dominio
 * de organización (`<org>.sytue.eu/mail`, `/calendar`…), y ahí `/auth/login` a pelo
 * es la raíz del host, que devuelve el 404 de Traefik. O sea: al caducar la sesión
 * se acababa en una página de error en vez de en el formulario. El shim de `fetch`
 * (packages/ui/src/basePath.ts) no llega hasta aquí porque esto no es un fetch.
 */
import { withBase } from '@sytue/ui';

export type { Mailbox, EmailSummary, EmailAddress, MessageBody, MailFilter, MailFilterInput };

/** Adjunto saliente tal como viaja del navegador al BFF (bytes en base64). */
export interface OutgoingAttachmentInput {
  name: string;
  type: string;
  contentBase64: string;
  /** Con `contentId`, la imagen se INCRUSTA en el cuerpo en vez de adjuntarse. */
  contentId?: string;
}

/** Metadatos de un adjunto de un correo leído (para listar en el lector). */
export interface AttachmentMeta {
  name: string;
  type: string;
  size: number;
  /** `Content-ID` sin ángulos: enlaza la parte con el `cid:` del cuerpo HTML. */
  cid?: string;
  /** Mandada para verse dentro del mensaje (`Content-Disposition: inline`). */
  inline?: boolean;
}

async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: 'include', headers: { accept: 'application/json' } });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  if (!res.ok) throw new Error(`Error ${res.status}`);
  return (await res.json()) as T;
}

/**
 * Error de una llamada al BFF que CONSERVA el porqué: el estado y el `code`
 * estable del servicio. Sin esto, `new Error('Error 502')` borraba el motivo
 * antes de que ninguna pantalla pudiera contarlo, y todos los rechazos —los que
 * tienen arreglo y los que no— acababan en el mismo mensaje genérico.
 */
export class MailHttpError extends Error {
  constructor(
    readonly status: number,
    readonly code?: string,
  ) {
    super(`Error ${status}${code ? ` (${code})` : ''}`);
    this.name = 'MailHttpError';
  }
}

async function postJson<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const cuerpo = (await res.json().catch(() => ({}))) as { code?: string };
    throw new MailHttpError(res.status, cuerpo.code);
  }
  return (await res.json()) as T;
}

export function listMailboxes(): Promise<{ mailboxes: Mailbox[] }> {
  return getJson('/api/mailboxes');
}

/**
 * Desenlace de crear, renombrar o borrar una carpeta. Los rechazos que la
 * pantalla SABE explicar viajan como valor, no como excepción: solo el fallo de
 * red se lanza, porque es el único ante el que no hay nada que corregir.
 */
export type FolderResult = 'ok' | 'exists' | 'invalid' | 'system' | 'notEmpty';

async function postFolder(body: unknown): Promise<FolderResult> {
  const res = await fetch('/api/mailboxes', {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(body),
  });
  if (res.ok) return 'ok';
  // El 409 lo comparten «ya existe» y «todavía tiene correo dentro»: el motivo
  // solo está en el cuerpo, y son dos cosas que se arreglan de forma distinta.
  const { error } = (await res.json().catch(() => ({}))) as { error?: string };
  if (error === 'mailboxHasEmail') return 'notEmpty';
  if (error === 'forbidden') return 'system';
  if (res.status === 409) return 'exists';
  if (res.status === 400 || res.status === 404) return 'invalid';
  throw new Error(`Error ${res.status}`);
}

/** Crea una carpeta propia. */
export function createMailbox(name: string): Promise<FolderResult> {
  return postFolder({ name });
}

/** Renombra una carpeta propia (las de sistema las rechaza el servicio). */
export function renameMailbox(id: string, name: string): Promise<FolderResult> {
  return postFolder({ id, name });
}

/** Borra una carpeta propia y vacía (el servicio niega el resto). */
export function deleteMailbox(id: string): Promise<FolderResult> {
  return postFolder({ id, delete: true });
}

/**
 * Lista una página de una carpeta por KEYSET: `cursor` es el `nextCursor` de la
 * página previa (vacío = primera). Devuelve `nextCursor` para pedir la siguiente
 * («» = no hay más), soportando scroll infinito con coste por página constante.
 */
export function listEmails(
  mailboxId: string,
  opts?: { limit?: number; cursor?: string },
): Promise<{ emails: EmailSummary[]; nextCursor: string }> {
  const params = new URLSearchParams({ mailboxId });
  if (opts?.limit) params.set('limit', String(opts.limit));
  if (opts?.cursor) params.set('cursor', opts.cursor);
  return getJson(`/api/emails?${params.toString()}`);
}

/** Busca correos; `position` pagina por offset (la búsqueda no expone keyset). */
export function searchEmails(
  query: string,
  opts?: { mailboxId?: string; limit?: number; position?: number },
): Promise<{ emails: EmailSummary[] }> {
  const params = new URLSearchParams({ q: query });
  if (opts?.mailboxId) params.set('mailboxId', opts.mailboxId);
  if (opts?.limit) params.set('limit', String(opts.limit));
  if (opts?.position) params.set('position', String(opts.position));
  return getJson(`/api/search?${params.toString()}`);
}

/** Sugerencia de contacto para el autocompletado de Para/CC/CCO. */
export interface ContactSuggestion {
  name: string;
  email: string;
}

/**
 * Autocompletado de contactos (proxy BFF a Sytue Contacts). Best-effort: si el
 * servicio no responde, devuelve lista vacía (la redacción sigue como texto libre).
 */
export async function searchContacts(query: string): Promise<ContactSuggestion[]> {
  const q = query.trim();
  if (!q) return [];
  try {
    const res = await fetch(`/api/contacts?q=${encodeURIComponent(q)}`, {
      credentials: 'include',
      headers: { accept: 'application/json' },
    });
    if (!res.ok) return [];
    const data = (await res.json()) as { contacts?: ContactSuggestion[] };
    return data.contacts ?? [];
  } catch {
    return [];
  }
}

export function setSeen(emailId: string, seen: boolean): Promise<{ ok: boolean }> {
  return postJson('/api/seen', { emailId, seen });
}

/** Destaca/quita la estrella ($flagged) de un correo. */
export function setFlagged(emailId: string, flagged: boolean): Promise<{ ok: boolean }> {
  return postJson('/api/message-actions', { emailId, flagged });
}

/**
 * Mueve un correo a otra carpeta: archivar (role=archive), papelera (role=trash)
 * o cualquier destino del menú «Mover a…». Va por POST (CSRF de Astro).
 */
export function moveEmail(emailId: string, mailboxId: string): Promise<{ ok: boolean }> {
  return postJson('/api/message-actions', { emailId, moveTo: mailboxId });
}

/**
 * Mueve un LOTE de correos a otra carpeta en UNA operación set-based del servicio
 * (id = ANY), no N llamadas: archivar/papelera/spam/«Mover a…» de una
 * multiselección o de un hilo entero. La autorización por dueño la impone el
 * servicio (owner + RLS). Devuelve cuántos se movieron.
 */
export function batchMove(ids: string[], mailboxId: string): Promise<{ moved: number }> {
  return postJson('/api/batch', { ids, moveTo: mailboxId });
}

/**
 * Marca leído/no leído (o destaca) un LOTE de correos en UNA operación. `keyword`
 * es el keyword JMAP ($seen/$flagged); el servicio lo traduce al flag IMAP.
 */
export function batchSetFlag(
  ids: string[],
  keyword: '$seen' | '$flagged',
  add: boolean,
): Promise<{ updated: number }> {
  return postJson('/api/batch', { ids, flag: keyword, add });
}

/**
 * Borra los mensajes DEFINITIVAMENTE (no van a la papelera: dejan de existir) y
 * libera el espacio que ocupaban. Sin vuelta atrás: quien llame ha de haberlo
 * confirmado antes.
 */
export function purgeEmails(ids: string[]): Promise<{ deleted: number }> {
  return postJson('/api/purge', { ids });
}

/** Un cambio del delta incremental: el sobre + su carpeta (para ubicarlo en la vista). */
export interface EmailChange extends EmailSummary {
  mailboxId: string;
}

/**
 * Lee el hilo COMPLETO atravesando carpetas. La lista de una carpeta solo trae
 * lo que vive en ella, así que la conversación se quedaba coja: tu respuesta
 * está en Enviados y lo viejo en Archivo. Cada sobre trae `mailboxId` para que
 * la vista decida qué mezclar (Papelera/Spam fuera salvo estando en ellas).
 * Owner-scoped en el servicio: un hilo ajeno devuelve lista vacía.
 */
export function listThreadEmails(threadId: string): Promise<{ emails: EmailChange[] }> {
  return getJson(`/api/thread?threadId=${encodeURIComponent(threadId)}`);
}

/**
 * Delta incremental de la bandeja (modelo JMAP Email/changes): dado el `state`
 * previo (vacío = arranque, solo fija el estado actual), devuelve SOLO lo
 * cambiado/borrado y el nuevo `state`, sin recargar toda la bandeja. El cliente lo
 * sondea con backoff y pausa en pestaña oculta. Owner-scoped en el servicio.
 */
export function fetchChanges(
  state: string,
): Promise<{ changed: EmailChange[]; destroyed: string[]; state: string; hasMore: boolean }> {
  const qs = state ? `?since=${encodeURIComponent(state)}` : '';
  return getJson(`/api/changes${qs}`);
}

/** Métodos de baja que anuncia un correo (List-Unsubscribe / RFC 8058 one-click). */
export interface UnsubscribeInfo {
  /** El remitente admite baja en un clic (RFC 8058): el backend hace el POST https. */
  oneClick: boolean;
  /** Hay una dirección mailto de baja: el backend envía el correo de baja por ti. */
  mailto: boolean;
}

/** Consulta si un correo ofrece darse de baja (y por qué método), sin exponer URLs. */
export function getUnsubscribe(emailId: string): Promise<UnsubscribeInfo> {
  return getJson(`/api/unsubscribe?emailId=${encodeURIComponent(emailId)}`);
}

/**
 * Se da de baja de la lista de un correo. El servidor elige el método SEGURO
 * (one-click https SSRF-safe, o enviar el correo al mailto) SIN exponer la
 * dirección/URL al navegador. Devuelve el resultado para avisar al usuario.
 */
export function unsubscribe(emailId: string): Promise<{ status: 'ok' | 'sent' | 'failed' }> {
  return postJson('/api/unsubscribe', { emailId });
}

export interface SendInput {
  to: EmailAddress[];
  cc?: EmailAddress[] | undefined;
  /** Copia oculta (CCO/BCC): recibe el correo pero no aparece en las cabeceras. */
  bcc?: EmailAddress[] | undefined;
  /**
   * Dirección «De» (enviar-como): la primaria o un alias verificado del usuario.
   * Ausente ⇒ la primaria. El servidor valida la propiedad (rechaza una ajena).
   */
  from?: string | undefined;
  subject: string;
  textBody?: string | undefined;
  htmlBody?: string | undefined;
  attachments?: OutgoingAttachmentInput[] | undefined;
  draftMailboxId: string;
  /** Programar: hora de envío en ISO 8601 (el servidor lo retiene hasta entonces). */
  sendAt?: string | undefined;
  /** Deshacer: ventana de gracia en segundos antes del despacho real. */
  undoSeconds?: number | undefined;
}

/**
 * Envía un correo. Sin sendAt/undoSeconds va inmediato; con ellos queda DIFERIDO
 * en la cola (programado o ventana de deshacer) y devuelve el id del diferido +
 * la hora de liberación, para cancelarlo/deshacerlo antes de que salga.
 */
export function sendEmail(
  input: SendInput,
): Promise<{ ok: boolean; id?: string; releaseAt?: string; status?: string }> {
  return postJson('/api/send', input);
}

/** Un envío PROGRAMADO pendiente (vista «Programados»). */
export interface ScheduledItem {
  id: string;
  from: string;
  recipients: string[];
  subject: string;
  releaseAt: string;
  createdAt: string;
}

/** Lista una página (keyset) de los programados pendientes del usuario. */
export function listScheduled(
  cursor?: string,
): Promise<{ scheduled: ScheduledItem[]; nextCursor: string }> {
  const qs = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
  return getJson(`/api/scheduled${qs}`);
}

/**
 * Cancela un diferido (programado o deshacer): lo saca de la cola antes de que
 * salga. `kind` distingue undo/scheduled. El contenido para editar lo reabre el
 * cliente (deshacer, desde su estado). POST con {action:'cancel'} por el CSRF.
 */
export function cancelScheduled(id: string): Promise<{ canceled: boolean; kind?: string }> {
  return postJson('/api/scheduled', { action: 'cancel', id });
}

/** Cambia la hora de un programado (sendAt en ISO 8601). */
export function rescheduleScheduled(
  id: string,
  sendAt: string,
): Promise<{ id: string; releaseAt: string }> {
  return postJson('/api/scheduled', { action: 'reschedule', id, sendAt });
}

export interface DraftInput {
  /** Presente al actualizar un borrador ya guardado (reemplazo). */
  id?: string | undefined;
  to: EmailAddress[];
  cc?: EmailAddress[] | undefined;
  subject: string;
  textBody: string;
  /** Formato enriquecido del cuerpo (negrita, listas, imágenes incrustadas…). */
  htmlBody?: string | undefined;
  attachments?: OutgoingAttachmentInput[] | undefined;
}

/** Guarda (o actualiza, si trae id) un borrador; devuelve el id del mensaje. */
export function saveDraft(input: DraftInput): Promise<{ id: string }> {
  return postJson('/api/drafts', input);
}

/**
 * Lo que trae de vuelta reabrir un borrador guardado: campos + adjuntos en
 * BASE64 (a diferencia de `DraftContent` del SDK, que trae los bytes crudos —
 * pensado para el servidor, no para viajar por `fetch().json()`). Mismo shape
 * que `OutgoingAttachmentInput`: lo que sale de aquí se puede volver a mandar
 * tal cual en el siguiente `saveDraft`/`sendEmail`.
 */
export interface DraftReadResult {
  to: string;
  cc: string;
  subject: string;
  textBody: string;
  htmlBody: string;
  attachments: OutgoingAttachmentInput[];
}

/** Lee el contenido editable de un borrador para reabrir el compositor. */
export function readDraft(id: string): Promise<DraftReadResult> {
  return getJson(`/api/drafts?id=${encodeURIComponent(id)}`);
}

/**
 * Descarta un borrador (p. ej. tras enviarlo). Va por POST con {delete:true}
 * (no DELETE): el checkOrigin CSRF de Astro rechaza el DELETE del navegador con 403.
 */
export async function deleteDraft(id: string): Promise<void> {
  await postJson('/api/drafts', { id, delete: true });
}

/** Cuerpo completo (texto y HTML) de un correo leído; el sobre solo trae extracto. */
export function getMessageBody(emailId: string): Promise<MessageBody> {
  return getJson(`/api/body?emailId=${encodeURIComponent(emailId)}`);
}

/** Lista los metadatos de los adjuntos de un correo leído. */
export function listAttachments(emailId: string): Promise<{ attachments: AttachmentMeta[] }> {
  return getJson(`/api/attachments?emailId=${encodeURIComponent(emailId)}`);
}

/**
 * URL (same-origin, cookie) de un adjunto por índice.
 *
 * `enLinea` la pide para PINTARLA en el cuerpo (`<img src>`) en vez de para
 * descargarla. El BFF solo lo concede a imágenes rasterizadas; para el resto
 * sigue mandando `Content-Disposition: attachment`.
 */
export function attachmentDownloadUrl(emailId: string, index: number, enLinea = false): string {
  // CON withBase: esto acaba en un `href`, no en un `fetch`, así que el shim de
  // `basePathScript` —que prefija las rutas `/api/` de fetch— no lo alcanza. Sin
  // el prefijo, en `<organizacion>.sytue.eu` la petición sale a la RAÍZ del host,
  // donde el ingress del tenant no enruta `/api`: 404 de Traefik. Medido.
  return withBase(
    import.meta.env.BASE_URL,
    `/api/attachments?emailId=${encodeURIComponent(emailId)}&index=${index}${enLinea ? '&inline=1' : ''}`,
  );
}

/**
 * URL de descarga del mensaje ORIGINAL (.eml). `subject` solo bautiza el fichero
 * —el BFF lo desinfecta— para que no acabe en Descargas con nombre de UUID.
 */
export function rawDownloadUrl(emailId: string, subject: string): string {
  // CON withBase: esto acaba en un `href`, no en un `fetch`, así que el shim de
  // `basePathScript` —que prefija las rutas `/api/` de fetch— no lo alcanza. Sin
  // el prefijo, en `<organizacion>.sytue.eu` la petición sale a la RAÍZ del host,
  // donde el ingress del tenant no enruta `/api`: 404 de Traefik. Medido.
  return withBase(
    import.meta.env.BASE_URL,
    `/api/raw?emailId=${encodeURIComponent(emailId)}&name=${encodeURIComponent(subject)}`,
  );
}

/** Petición de intent cross-app al Hub (forma canónica {type, params, ids}). */
export interface IntentRequest {
  type: string;
  params?: Record<string, string>;
  ids?: string[];
}

/** Resultado de un intent: deep-link primario + metadatos (docs/CONVENTIONS.md §11). */
export interface IntentResult {
  type: string;
  url: string;
  target: string;
  label: string;
  links?: { rel: string; source: string; url: string }[];
}

/**
 * Resuelve un intent cross-app via el Hub (p. ej. crear evento desde un correo):
 * devuelve el deep-link que el cliente abre. El Hub NO ejecuta la acción; el
 * servicio destino aplica su autorización al abrir el enlace.
 */
export function resolveIntent(req: IntentRequest): Promise<IntentResult> {
  return postJson('/api/intent', req);
}

// ---- Reglas/filtros de servidor ----

/** Lista las reglas de filtrado de la cuenta. */
export function listFilters(): Promise<{ filters: MailFilter[] }> {
  return getJson('/api/filters');
}

/**
 * Crea (sin id) o actualiza (con id) una regla. Siempre POST: el checkOrigin CSRF
 * de Astro rechaza PUT desde el navegador; el BFF traduce a PUT hacia el servicio.
 */
export function saveFilter(
  input: MailFilterInput & { id?: string },
): Promise<{ filter?: MailFilter; ok?: boolean }> {
  return postJson('/api/filters', input);
}

/** Borra una regla (POST con {delete:true}, no DELETE, por el CSRF de Astro). */
export function deleteFilter(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/filters', { id, delete: true });
}

// ---- Etiquetas (labels) ----

/**
 * Etiqueta de usuario. Se modela como keyword JMAP/IMAP en el backend (el token
 * en messages.flags es su id), así que un mensaje lleva N etiquetas; aquí solo
 * los metadatos: nombre, color (token de paleta) y contadores para el sidebar.
 */
export interface Label {
  id: string;
  name: string;
  color: string;
  totalEmails: number;
  unreadEmails: number;
}

/** Lista las etiquetas de la cuenta con contadores total/no-leído. */
export function listLabels(): Promise<{ labels: Label[] }> {
  return getJson('/api/labels');
}

/**
 * Lista una página (KEYSET) de los mensajes de una etiqueta —across carpetas,
 * como la vista de etiqueta de Gmail—. Mismo contrato que listEmails.
 */
export function listLabelEmails(
  labelId: string,
  opts?: { limit?: number; cursor?: string },
): Promise<{ emails: EmailSummary[]; nextCursor: string }> {
  const params = new URLSearchParams({ labelId });
  if (opts?.limit) params.set('limit', String(opts.limit));
  if (opts?.cursor) params.set('cursor', opts.cursor);
  return getJson(`/api/label-emails?${params.toString()}`);
}

/** Crea una etiqueta (nombre + token de color de la paleta). */
export function createLabel(name: string, color: string): Promise<{ label: Label }> {
  return postJson('/api/labels', { name, color });
}

/** Renombra/recolorea una etiqueta (no toca los mensajes: el keyword es su id). */
export function updateLabel(id: string, name: string, color: string): Promise<{ ok: boolean }> {
  return postJson('/api/labels', { id, name, color });
}

/** Borra una etiqueta (POST con {delete:true} por el CSRF). No borra mensajes. */
export function deleteLabel(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/labels', { id, delete: true });
}

/**
 * Aplica (add=true) o quita (add=false) una etiqueta a un LOTE de mensajes (un
 * hilo entero) en una sola llamada. El backend valida dueño de etiqueta y mensajes.
 */
export function applyLabel(
  labelId: string,
  messageIds: string[],
  add: boolean,
): Promise<{ ok: boolean }> {
  return postJson('/api/labels', { id: labelId, messageIds, add });
}

// ---- Alias de correo (enviar-como) ----

/** Un alias: otra dirección que entrega a la misma cuenta. */
export interface Alias {
  id: string;
  address: string;
}

/** Lista los alias de la cuenta + la dirección primaria (para el selector «De»). */
export function listAliases(): Promise<{ aliases: Alias[]; primary: string }> {
  return getJson('/api/aliases');
}

/** Crea un alias (dirección de un dominio del servicio). */
export function createAlias(address: string): Promise<{ alias: Alias }> {
  return postJson('/api/aliases', { address });
}

/** Borra un alias (POST con {delete:true} por el CSRF de Astro). */
export function deleteAlias(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/aliases', { id, delete: true });
}

// ---- Autorespuesta / vacaciones ----

/** Config de fuera-de-oficina. Fechas en ISO 8601 (vacías = sin límite). */
export interface Vacation {
  enabled: boolean;
  subject: string;
  body: string;
  startsAt?: string | undefined;
  endsAt?: string | undefined;
}

/** Lee la config de vacaciones de la cuenta. */
export function getVacation(): Promise<Vacation> {
  return getJson('/api/vacation');
}

// ---- Cuota de almacenamiento ----

/** Uso de almacenamiento de la cuenta: bytes usados y cuota total. */
export interface StorageQuota {
  usedBytes: number;
  quotaBytes: number;
}

/**
 * Lee la cuota de almacenamiento del propio usuario (indicador del sidebar). El
 * uso es el contador agregado que el servicio mantiene (sin N+1); solo lectura y
 * owner-scoped en el backend.
 */
export function getStorageQuota(): Promise<StorageQuota> {
  return getJson('/api/quota');
}

/** Guarda (reemplaza) la config de vacaciones. */
export function saveVacation(v: Vacation): Promise<{ ok: boolean }> {
  return postJson('/api/vacation', v);
}

// ---- Firma de correo ----

/**
 * Lee la firma del usuario. Vive en el SERVIDOR (por cuenta), no en el
 * navegador: la misma en el portátil, en el móvil y tras vaciar el
 * almacenamiento. Sin firma devuelve cadena vacía.
 */
export function getSignature(): Promise<{ signature: string }> {
  return getJson('/api/signature');
}

/**
 * Guarda (reemplaza) la firma. Cadena vacía la borra. Lanza si el servidor la
 * rechaza —es demasiado larga, o no hay red—: quien llame DEBE avisar, porque
 * dar por guardada una firma que no se guardó es perder lo que el usuario
 * escribió.
 */
export function saveSignature(signature: string): Promise<{ ok: boolean }> {
  return postJson('/api/signature', { signature });
}

// ---- Buzones compartidos + delegación ----

/** Nivel de acceso a un buzón compartido/delegado (escalera: read < send < manage). */
export type MailPermission = 'read' | 'send' | 'manage';

/**
 * Un buzón (compartido de la organización, kind='shared', o personal delegado,
 * kind='user') al que tengo acceso por ACL, con mi permiso. Alimenta el sidebar y
 * el selector «De» del redactor (enviar-como / enviar-en-nombre-de).
 */
export interface SharedMailbox {
  accountId: string;
  address: string;
  displayName: string;
  kind: 'shared' | 'user';
  permission: MailPermission;
  /** En `read`, ya ha pedido acceso (`FranjaPeticion`). */
  editRequested?: boolean;
}

/** Una entrada de ACL (quién tiene acceso y con qué permiso). */
export interface MailGrant {
  grantee: string;
  permission: MailPermission;
}

/** Lista los buzones compartidos/delegados accesibles (además del propio). */
export function listSharedMailboxes(): Promise<{ sharedMailboxes: SharedMailbox[] }> {
  return getJson('/api/shared-mailboxes');
}

/** Crea un buzón compartido de la organización (el creador queda como `manage`). */
export function createSharedMailbox(
  address: string,
  displayName: string,
): Promise<{ mailbox: SharedMailbox }> {
  return postJson('/api/shared-mailboxes', { address, displayName });
}

/** Lista las carpetas de un buzón compartido/delegado (requiere permiso de lectura). */
export function listSharedFolders(accountId: string): Promise<{ mailboxes: Mailbox[] }> {
  return getJson(`/api/shared-folders?accountId=${encodeURIComponent(accountId)}`);
}

/** Lista (keyset) los mensajes de una carpeta de un buzón compartido/delegado. */
export function listSharedEmails(
  accountId: string,
  mailboxId: string,
  opts?: { limit?: number; cursor?: string },
): Promise<{ emails: EmailSummary[]; nextCursor: string }> {
  const params = new URLSearchParams({ accountId, mailboxId });
  if (opts?.limit) params.set('limit', String(opts.limit));
  if (opts?.cursor) params.set('cursor', opts.cursor);
  return getJson(`/api/shared-emails?${params.toString()}`);
}

/** Lista el ACL de un buzón compartido (requiere `manage`). */
export function listGrants(accountId: string): Promise<{ grants: MailGrant[] }> {
  return getJson(`/api/shared-grants?accountId=${encodeURIComponent(accountId)}`);
}

/** Concede/actualiza acceso a un buzón compartido (requiere `manage`). */
export function grantAccess(
  accountId: string,
  grantee: string,
  permission: MailPermission,
): Promise<{ ok: boolean }> {
  return postJson('/api/shared-grants', { accountId, grantee, permission });
}

/** Revoca acceso a un buzón compartido (POST con delete por el CSRF de Astro). */
export function revokeAccess(accountId: string, grantee: string): Promise<{ ok: boolean }> {
  return postJson('/api/shared-grants', { accountId, grantee, delete: true });
}

/** Lista a quién he delegado MI buzón. */
export function listDelegation(): Promise<{ grants: MailGrant[] }> {
  return getJson('/api/delegation');
}

/** Delega MI buzón en otra persona (lectura o enviar-en-mi-nombre). */
export function delegateAccess(
  grantee: string,
  permission: MailPermission,
): Promise<{ ok: boolean }> {
  return postJson('/api/delegation', { grantee, permission });
}

/** Retira una delegación de MI buzón. */
export function revokeDelegation(grantee: string): Promise<{ ok: boolean }> {
  return postJson('/api/delegation', { grantee, delete: true });
}

export interface EntradaDeLibreta {
  id: string;
  name: string;
  emails: string[];
}

/** La libreta de Contacts con correo (icono de Contactos del redactor). Lanza si no hay libreta. */
export async function cargarLibreta(): Promise<EntradaDeLibreta[]> {
  const res = await fetch('/api/libreta', {
    credentials: 'include',
    headers: { accept: 'application/json' },
  });
  if (!res.ok) throw new Error(`libreta: ${res.status}`);
  const data = (await res.json()) as { contacts?: EntradaDeLibreta[] };
  return data.contacts ?? [];
}
