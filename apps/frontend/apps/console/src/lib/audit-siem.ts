/*
 * Auditoría unificada + export SIEM de Sytue Admin (SOLO servidor).
 *
 * Agrega la traza de auditoría de TODOS los servicios de la suite en una sola vista
 * y un solo export, normalizada a un esquema apto para ingesta SIEM:
 *   { ts, service, actor, action, target, detail }
 *
 * Fuentes (una llamada por servicio, BEST-EFFORT — si una cae, se reporta su estado
 * sin romper el resto; mismo patrón que lib/compliance.ts):
 *   - id            → Sytue ID: eventos de identidad/admin (GET /api/v1/admin/audit).
 *   - mail          → Sytue Mail: traza de compliance (GET /api/v1/admin/audit-log).
 *   - drive         → Sytue Drive: traza de compliance (GET /api/v1/admin/audit-log).
 *   - notes         → Sytue Notepads: traza de compliance (GET /api/v1/admin/audit-log).
 *   - integrations  → Integration Gateway: egress (GET /admin/egress-audit) + decisiones
 *                     de política (GET /admin/policy-decisions).
 *
 * El rango de fechas y el tope se EMPUJAN a cada backend (acotan el volumen); el
 * filtro por servicio/acción/actor se aplica de forma UNIFORME aquí sobre el conjunto
 * ya normalizado (las semánticas de filtro difieren por servicio). Cada backend exige
 * rol owner/admin y aísla por org (RLS del tenant del token). El token httpOnly nunca
 * sale del servidor. docs/CONVENTIONS.md §8, §11.
 *
 * FOLLOW-UP (streaming SIEM en tiempo real): esto es el primer corte HONESTO —export
 * *pull* + formato normalizado. El push en tiempo real (forwarder que emita cada
 * evento a un endpoint SIEM/syslog configurado, p. ej. HEC de Splunk o un colector
 * OTLP) es una ola posterior: requiere un emisor persistente por-servicio, no cabe en
 * el BFF. El esquema normalizado de aquí es exactamente lo que ese forwarder emitiría.
 */
import { HttpClient, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { adminBaseUrl, mailBaseUrl, driveBaseUrl, notesBaseUrl, gatewayBaseUrl } from './auth.js';
import { contiene } from '@sytue/i18n';

/** Fuente (servicio) de un registro de auditoría. */
export type AuditSource = 'id' | 'mail' | 'drive' | 'notes' | 'integrations';

export const ALL_SOURCES: readonly AuditSource[] = ['id', 'mail', 'drive', 'notes', 'integrations'];

/** Etiqueta de presentación de cada fuente. */
export const SOURCE_LABEL: Record<AuditSource, string> = {
  id: 'Identidad',
  mail: 'Correo',
  drive: 'Archivos',
  notes: 'Notas',
  integrations: 'Integraciones',
};

/** Registro de auditoría NORMALIZADO (esquema SIEM). detail es estructurado ({} si no hay). */
export interface SiemRecord {
  ts: string; // ISO 8601 (UTC)
  service: AuditSource;
  actor: string;
  action: string;
  target: string;
  detail: Record<string, unknown>;
}

/** Estado por-fuente tras el fan-out (para avisar de una fuente caída sin romper el resto). */
export interface SourceStatus {
  source: AuditSource;
  ok: boolean;
  count: number;
}

/** Resultado agregado: registros combinados (orden fecha DESC) + estado por-fuente. */
export interface UnifiedAudit {
  records: SiemRecord[];
  sources: SourceStatus[];
}

/** Filtros de la vista unificada. Fecha/tope se empujan a los backends; el resto es uniforme. */
export interface AuditFilters {
  /** Fuentes a incluir; vacío/ausente = todas. */
  services?: AuditSource[] | undefined;
  /** Subcadena sobre actor u objetivo (case-insensitive). */
  actor?: string | undefined;
  /** Subcadena sobre la acción (case-insensitive). */
  action?: string | undefined;
  /** Rango de fechas AAAA-MM-DD (vacío = sin límite). */
  since?: string | undefined;
  until?: string | undefined;
  /** Tope por-fuente y del conjunto (1..500, 200 por defecto). */
  limit?: number | undefined;
}

/** Ventana empujada a cada backend. */
interface AuditWindow {
  since?: string | undefined;
  until?: string | undefined;
  limit: number;
}

// ---- Forma en el cable de cada backend ---------------------------------------

interface IdAuditWire {
  occurred_at: string;
  actor_id?: string;
  actor_email?: string;
  action: string;
  target?: string;
  detail?: unknown;
}

interface ComplianceAuditWire {
  occurred_at: string;
  actor?: string;
  action: string;
  target?: string;
  detail?: unknown;
}

interface EgressWire {
  occurred_at: string;
  instance_id: string;
  host: string;
  method: string;
  status: number;
  req_bytes: number;
  resp_bytes: number;
  latency_ms: number;
  auth: string;
  error: string;
}

interface DecisionWire {
  occurred_at: string;
  instance_id: string;
  policy_name: string;
  action: 'warn' | 'block';
  request_id: string;
  host: string;
  method: string;
}

/** detail siempre estructurado: pasa el objeto tal cual o {} si no es un objeto. */
function asDetail(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

function http(session: SytueSession, base: string): HttpClient {
  return new HttpClient({
    baseUrl: `${base.replace(/\/$/, '')}/api/v1/admin`,
    auth: bearer(session.tokens.accessToken),
  });
}

// ---- Cargadores por-fuente (mapean al esquema normalizado; lanzan si la forma es mala) ----

async function loadId(session: SytueSession, w: AuditWindow): Promise<SiemRecord[]> {
  const res = await http(session, adminBaseUrl()).json<{ items?: IdAuditWire[] }>({
    path: '/audit',
    query: { since: w.since || undefined, until: w.until || undefined, limit: w.limit },
  });
  if (!Array.isArray(res?.items)) throw new Error('bad_shape');
  return res.items.map((a) => ({
    ts: a.occurred_at,
    service: 'id' as const,
    actor: a.actor_email || a.actor_id || '',
    action: a.action,
    target: a.target ?? '',
    detail: asDetail(a.detail),
  }));
}

async function loadCompliance(
  session: SytueSession,
  base: string,
  service: 'mail' | 'drive' | 'notes',
  w: AuditWindow,
): Promise<SiemRecord[]> {
  const res = await http(session, base).json<{ items?: ComplianceAuditWire[] }>({
    path: '/audit-log',
    query: { since: w.since || undefined, until: w.until || undefined, limit: w.limit },
  });
  if (!Array.isArray(res?.items)) throw new Error('bad_shape');
  return res.items.map((a) => ({
    ts: a.occurred_at,
    service,
    actor: a.actor ?? '',
    action: a.action,
    target: a.target ?? '',
    detail: asDetail(a.detail),
  }));
}

async function loadEgress(session: SytueSession, w: AuditWindow): Promise<SiemRecord[]> {
  const res = await http(session, gatewayBaseUrl()).json<{ items?: EgressWire[] }>({
    path: '/egress-audit',
    query: { since: w.since || undefined, until: w.until || undefined, limit: w.limit },
  });
  if (!Array.isArray(res?.items)) throw new Error('bad_shape');
  return res.items.map((e) => ({
    ts: e.occurred_at,
    service: 'integrations' as const,
    actor: e.instance_id,
    action: 'integrations.egress',
    target: `${e.method} ${e.host}`.trim(),
    detail: {
      status: e.status,
      latency_ms: e.latency_ms,
      req_bytes: e.req_bytes,
      resp_bytes: e.resp_bytes,
      auth: e.auth,
      error: e.error,
    },
  }));
}

async function loadDecisions(session: SytueSession, w: AuditWindow): Promise<SiemRecord[]> {
  // policy-decisions no acota por fecha en el gateway; solo se le pasa el tope.
  const res = await http(session, gatewayBaseUrl()).json<{ items?: DecisionWire[] }>({
    path: '/policy-decisions',
    query: { limit: w.limit },
  });
  if (!Array.isArray(res?.items)) throw new Error('bad_shape');
  return res.items.map((d) => ({
    ts: d.occurred_at,
    service: 'integrations' as const,
    actor: d.instance_id,
    action: `policy.${d.action}`,
    target: `${d.method} ${d.host}`.trim(),
    detail: { policy_name: d.policy_name, request_id: d.request_id },
  }));
}

/** Ejecuta un cargador como resultado best-effort (nunca lanza). */
async function attempt(fn: Promise<SiemRecord[]>): Promise<{ ok: boolean; data: SiemRecord[] }> {
  try {
    return { ok: true, data: await fn };
  } catch {
    return { ok: false, data: [] };
  }
}

function clampLimit(n: number | undefined): number {
  if (typeof n !== 'number' || !Number.isFinite(n)) return 200;
  return Math.min(500, Math.max(1, Math.trunc(n)));
}

/** Aplica los filtros uniformes (servicio/acción/actor) sobre el conjunto normalizado. */
function applyFilters(records: SiemRecord[], f: AuditFilters): SiemRecord[] {
  const services = f.services && f.services.length > 0 ? new Set(f.services) : null;
  const action = f.action?.trim().toLowerCase() ?? '';
  const actor = f.actor?.trim().toLowerCase() ?? '';
  return records.filter((r) => {
    if (services && !services.has(r.service)) return false;
    if (action && !contiene(r.action, action)) return false;
    if (actor && !contiene(r.actor, actor) && !contiene(r.target, actor)) {
      return false;
    }
    return true;
  });
}

/**
 * Auditoría unificada: fan-out best-effort a las 5 fuentes (6 llamadas — Integraciones
 * agrega egress + decisiones), normaliza, filtra de forma uniforme, ordena por fecha
 * DESC y topa a `limit`. Reporta el estado por-fuente para avisar de una fuente caída.
 */
export async function loadUnifiedAudit(
  session: SytueSession,
  filters: AuditFilters = {},
): Promise<UnifiedAudit> {
  const w: AuditWindow = {
    since: filters.since,
    until: filters.until,
    limit: clampLimit(filters.limit),
  };
  const [id, mail, drive, notes, egress, decisions] = await Promise.all([
    attempt(loadId(session, w)),
    attempt(loadCompliance(session, mailBaseUrl(), 'mail', w)),
    attempt(loadCompliance(session, driveBaseUrl(), 'drive', w)),
    attempt(loadCompliance(session, notesBaseUrl(), 'notes', w)),
    attempt(loadEgress(session, w)),
    attempt(loadDecisions(session, w)),
  ]);
  const sources: SourceStatus[] = [
    { source: 'id', ok: id.ok, count: id.data.length },
    { source: 'mail', ok: mail.ok, count: mail.data.length },
    { source: 'drive', ok: drive.ok, count: drive.data.length },
    { source: 'notes', ok: notes.ok, count: notes.data.length },
    // Integraciones = egress + decisiones: viva si CUALQUIERA respondió.
    {
      source: 'integrations',
      ok: egress.ok || decisions.ok,
      count: egress.data.length + decisions.data.length,
    },
  ];
  let records = [
    ...id.data,
    ...mail.data,
    ...drive.data,
    ...notes.data,
    ...egress.data,
    ...decisions.data,
  ];
  records = applyFilters(records, filters);
  records.sort((a, b) => (a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : 0));
  if (records.length > w.limit) records = records.slice(0, w.limit);
  return { records, sources };
}

/** Extrae los filtros de la query del BFF (repetible: ?service=id&service=mail). */
export function parseAuditFilters(p: URLSearchParams): AuditFilters {
  const services = p
    .getAll('service')
    .filter((s): s is AuditSource => (ALL_SOURCES as readonly string[]).includes(s));
  const limit = Number.parseInt(p.get('limit') ?? '', 10);
  return {
    services: services.length > 0 ? services : undefined,
    actor: p.get('actor')?.trim() || undefined,
    action: p.get('action')?.trim() || undefined,
    since: p.get('since')?.trim() || undefined,
    until: p.get('until')?.trim() || undefined,
    limit: Number.isFinite(limit) && limit > 0 ? limit : undefined,
  };
}

// ---- Serialización del export (esquema SIEM) ---------------------------------

const SIEM_COLUMNS = ['ts', 'service', 'actor', 'action', 'target', 'detail'] as const;

/** Neutraliza la inyección de fórmulas CSV (CWE-1236) y entrecomilla si hace falta. */
function csvCell(s: string): string {
  let v = s;
  if (v && '=+-@\t\r'.includes(v[0]!)) v = `'${v}`; // fuerza texto en Excel/Sheets
  if (/[",\r\n]/.test(v)) v = `"${v.replace(/"/g, '""')}"`;
  return v;
}

/** CSV del conjunto agregado (cabecera normalizada + una fila por registro). */
export function toCsv(records: SiemRecord[]): string {
  const lines = [SIEM_COLUMNS.join(',')];
  for (const r of records) {
    lines.push(
      [r.ts, r.service, r.actor, r.action, r.target, JSON.stringify(r.detail)]
        .map(csvCell)
        .join(','),
    );
  }
  return lines.join('\r\n') + '\r\n';
}

/** JSON del conjunto agregado (array de registros normalizados; apto para ingesta SIEM). */
export function toJson(records: SiemRecord[]): string {
  return JSON.stringify(records, null, 2);
}
