/*
 * Proveedor de datos de Cumplimiento de Sytue Admin (SOLO servidor).
 *
 * Cierra el bucle de los backends de compliance (legal hold + eDiscovery) de Sytue
 * Mail (correo) y Sytue Drive (ficheros), que exponen los MISMOS endpoints admin
 * bajo /api/v1/admin. Una retención o una búsqueda pueden abarcar AMBOS servicios:
 * el BFF llama a los dos y reporta por-servicio en modo BEST-EFFORT (si uno falla,
 * se informa su estado sin romper el otro).
 *
 * ACCESO PRIVILEGIADO Y AUDITADO: cada crear/liberar hold y cada búsqueda/export es
 * acceso a datos AJENOS; el servicio Go exige rol owner/admin, aísla por org (RLS) y
 * AUDITA en modo fail-closed. El token de sesión (httpOnly) nunca sale del servidor.
 * NUNCA se baja el contenido aquí: el export streamea .mbox (correo) / .zip (ficheros).
 * docs/CONVENTIONS.md §8, §11.
 */
import { HttpClient, HttpError, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { mailBaseUrl, driveBaseUrl, notesBaseUrl } from './auth.js';

/**
 * `notes` SOLO cubre retención legal (legal hold), no eDiscovery: el servicio de
 * Notepads monta `/admin/legal-holds` con las MISMAS rutas que Drive (comentario
 * en `services/notes/internal/http/server.go`), pero no expone
 * `/admin/ediscovery/*`. Las funciones de eDiscovery de este fichero siguen
 * limitadas a mail/drive a propósito; `holdCapableServices` es la lista que sí
 * incluye a los tres.
 */
export type ComplianceService = 'mail' | 'drive' | 'notes';

/** Etiqueta de presentación de cada servicio (fuente de un hold/hit). */
export const SERVICE_LABEL: Record<ComplianceService, string> = {
  mail: 'Correo',
  drive: 'Archivos',
  notes: 'Notepads',
};

/** Servicios con retención legal (los tres); eDiscovery sigue siendo solo mail/drive. */
export const HOLD_SERVICES: ComplianceService[] = ['mail', 'drive', 'notes'];

/** Retención legal (paridad M365 Purview / Google Vault), etiquetada por servicio. */
export interface LegalHold {
  id: string;
  service: ComplianceService;
  /** sub de Sytue ID del sujeto; '' = toda la organización. */
  subject: string;
  reason: string;
  case: string;
  createdBy: string;
  createdAt: string;
  releasedAt: string;
  releasedBy: string;
  orgWide: boolean;
  active: boolean;
}

/** Resultado de eDiscovery unificado (correo o fichero) para una tabla combinada. */
export interface EDiscoveryHit {
  id: string;
  service: ComplianceService;
  /** asunto (correo) / nombre (fichero). */
  title: string;
  /** remitente (correo) / tipo MIME (fichero). */
  subtitle: string;
  /** recibido (correo) / actualizado (fichero), ISO. */
  at: string;
  size: number;
  /** retenido: borrado por el usuario pero conservado para el caso. */
  preserved: boolean;
}

/** Resultado por-servicio: si el servicio respondió (ok), con qué status, y su carga. */
export interface ServiceResult<T> {
  ok: boolean;
  status: number;
  data: T;
}

/** Ambos servicios reportados por separado (best-effort). */
export type BothServices<T> = Record<ComplianceService, ServiceResult<T>>;

/** Código HTTP a propagar por el BFF cuando una acción de un solo servicio falla. */
export function proxyStatus(e: unknown): number {
  return e instanceof HttpError ? e.status : 502;
}

/** Servicios cubiertos por un ámbito ('mail' | 'drive' | 'both'); [] si es inválido. */
export function scopeServices(scope: unknown): ComplianceService[] {
  if (scope === 'mail') return ['mail'];
  if (scope === 'drive') return ['drive'];
  // 'notes' es EXCLUSIVO de retención legal: eDiscovery nunca manda este ámbito
  // (su propio selector no lo ofrece), así que añadirlo aquí no cambia nada del
  // camino de búsqueda/export.
  if (scope === 'notes') return ['notes'];
  if (scope === 'both') return ['mail', 'drive'];
  if (scope === 'all') return ['mail', 'drive', 'notes'];
  return [];
}

function baseUrlFor(service: ComplianceService): string {
  if (service === 'mail') return mailBaseUrl();
  if (service === 'drive') return driveBaseUrl();
  return notesBaseUrl();
}

/** HttpClient autenticado a la API admin de compliance del servicio (/api/v1/admin). */
function complianceHttp(session: SytueSession, service: ComplianceService): HttpClient {
  return new HttpClient({
    baseUrl: `${baseUrlFor(service).replace(/\/$/, '')}/api/v1/admin`,
    auth: bearer(session.tokens.accessToken),
  });
}

/** Ejecuta una llamada al servicio como ServiceResult best-effort (nunca lanza). */
async function attempt<T>(fallback: T, fn: () => Promise<T>): Promise<ServiceResult<T>> {
  try {
    return { ok: true, status: 200, data: await fn() };
  } catch (e) {
    return { ok: false, status: e instanceof HttpError ? e.status : 502, data: fallback };
  }
}

// ---- Legal holds -------------------------------------------------------------

interface HoldWire {
  id: string;
  subject?: string;
  reason?: string;
  case?: string;
  created_by?: string;
  created_at: string;
  released_at?: string;
  released_by?: string;
}

function toHold(w: HoldWire, service: ComplianceService): LegalHold {
  const subject = w.subject ?? '';
  return {
    id: w.id,
    service,
    subject,
    reason: w.reason ?? '',
    case: w.case ?? '',
    createdBy: w.created_by ?? '',
    createdAt: w.created_at,
    releasedAt: w.released_at ?? '',
    releasedBy: w.released_by ?? '',
    orgWide: subject === '',
    active: !w.released_at,
  };
}

function loadServiceHolds(
  session: SytueSession,
  service: ComplianceService,
  includeReleased: boolean,
): Promise<ServiceResult<LegalHold[]>> {
  return attempt<LegalHold[]>([], async () => {
    const res = await complianceHttp(session, service).json<{ holds?: HoldWire[] }>({
      path: '/legal-holds',
      query: includeReleased ? { include_released: 'true' } : {},
    });
    if (!Array.isArray(res?.holds)) throw new Error('bad_shape');
    return res.holds.map((h) => toHold(h, service));
  });
}

/** Holds de correo, ficheros y cuadernos, reportados por-servicio (best-effort). */
export async function loadHolds(
  session: SytueSession,
  includeReleased: boolean,
): Promise<BothServices<LegalHold[]>> {
  const [mail, drive, notes] = await Promise.all([
    loadServiceHolds(session, 'mail', includeReleased),
    loadServiceHolds(session, 'drive', includeReleased),
    loadServiceHolds(session, 'notes', includeReleased),
  ]);
  return { mail, drive, notes };
}

export interface CreateHoldInput {
  orgWide: boolean;
  /** sub de Sytue ID (obligatorio para Archivos si no es org-wide). */
  subject: string;
  /** email del sujeto (Correo resuelve la cuenta); vacío si org-wide. */
  email: string;
  reason: string;
  case: string;
}

/** Crea un hold en un servicio (best-effort; devuelve el hold creado o el status del fallo). */
export function createServiceHold(
  session: SytueSession,
  service: ComplianceService,
  input: CreateHoldInput,
): Promise<ServiceResult<LegalHold | null>> {
  // Correo resuelve por email (su vía documentada); Archivos exige el sub. Org-wide
  // no lleva sujeto. El servicio valida y AUDITA; aquí solo se arma el cuerpo.
  const body: Record<string, unknown> = input.orgWide
    ? { org_wide: true }
    : service === 'mail' && input.email
      ? { email: input.email }
      : { subject: input.subject };
  if (input.reason) body.reason = input.reason;
  if (input.case) body.case = input.case;
  return attempt<LegalHold | null>(null, async () => {
    const w = await complianceHttp(session, service).postJson<HoldWire>('/legal-holds', body);
    return toHold(w, service);
  });
}

/** Libera un hold de un servicio (lanza HttpError; el BFF propaga el status). */
export function releaseHold(
  session: SytueSession,
  service: ComplianceService,
  id: string,
): Promise<unknown> {
  return complianceHttp(session, service).postJson(
    `/legal-holds/${encodeURIComponent(id)}/release`,
    {},
  );
}

// ---- eDiscovery --------------------------------------------------------------

export interface SearchInput {
  /** email del sujeto (Correo lo resuelve). */
  email: string;
  /** sub del sujeto (Archivos lo exige). */
  subject: string;
  /** rango de fechas YYYY-MM-DD (vacío = sin límite). */
  from: string;
  to: string;
  /** texto libre: asunto/remitente/cuerpo (Correo) o nombre de fichero (Archivos). */
  query: string;
  /** referencia de caso (opcional); el servicio la registra en la auditoría. */
  case: string;
}

interface MailMsgWire {
  id: string;
  subject?: string;
  from?: string;
  received_at: string;
  size?: number;
  preserved?: boolean;
}

interface DriveFileWire {
  id: string;
  name?: string;
  content_type?: string;
  updated_at: string;
  size_bytes?: number;
  preserved?: boolean;
}

/** Búsqueda de eDiscovery en un servicio (best-effort). */
export function searchService(
  session: SytueSession,
  service: ComplianceService,
  input: SearchInput,
): Promise<ServiceResult<EDiscoveryHit[]>> {
  return attempt<EDiscoveryHit[]>([], async () => {
    const http = complianceHttp(session, service);
    if (service === 'mail') {
      const res = await http.postJson<{ messages?: MailMsgWire[] }>('/ediscovery/search', {
        ...(input.email ? { email: input.email } : { subject: input.subject }),
        from: input.from || undefined,
        to: input.to || undefined,
        query: input.query || undefined,
        case: input.case || undefined,
      });
      return (res.messages ?? []).map((m) => ({
        id: m.id,
        service: 'mail' as const,
        title: m.subject || '(sin asunto)',
        subtitle: m.from || '',
        at: m.received_at,
        size: typeof m.size === 'number' ? m.size : 0,
        preserved: Boolean(m.preserved),
      }));
    }
    const res = await http.postJson<{ files?: DriveFileWire[] }>('/ediscovery/search', {
      subject: input.subject,
      from: input.from || undefined,
      to: input.to || undefined,
      name: input.query || undefined,
      case: input.case || undefined,
    });
    return (res.files ?? []).map((f) => ({
      id: f.id,
      service: 'drive' as const,
      title: f.name || '(sin nombre)',
      subtitle: f.content_type || '',
      at: f.updated_at,
      size: typeof f.size_bytes === 'number' ? f.size_bytes : 0,
      preserved: Boolean(f.preserved),
    }));
  });
}

/**
 * Respuesta cruda del export de eDiscovery (streaming) para reenviar por el BFF.
 * Correo → .mbox; Archivos → .zip. fetch directo con el token (no HttpClient, que
 * consumiría el cuerpo como JSON): devuelve el Response para hacer passthrough del
 * stream. El servicio AUDITA el export (fail-closed) antes de emitir los bytes.
 */
export function ediscoveryExportResponse(
  session: SytueSession,
  service: ComplianceService,
  input: SearchInput,
): Promise<Response> {
  const url = new URL(`${baseUrlFor(service).replace(/\/$/, '')}/api/v1/admin/ediscovery/export`);
  const q = url.searchParams;
  if (service === 'mail') {
    if (input.email) q.set('email', input.email);
    else q.set('subject', input.subject);
    if (input.query) q.set('query', input.query);
  } else {
    q.set('subject', input.subject);
    if (input.query) q.set('name', input.query);
  }
  if (input.from) q.set('from', input.from);
  if (input.to) q.set('to', input.to);
  if (input.case) q.set('case', input.case);
  return fetch(url, {
    headers: {
      authorization: `Bearer ${session.tokens.accessToken}`,
      accept: service === 'mail' ? 'application/mbox' : 'application/zip',
    },
  });
}
