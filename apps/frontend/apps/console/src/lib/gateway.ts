/*
 * Proveedor de datos del Sytue Integration Gateway para la Console (SOLO servidor).
 *
 * Cablea la página de Integraciones y sus endpoints BFF a la API del gateway Go
 * (services/integrations) con el access token de la sesión (rol admin/owner). El
 * gateway aísla por org (RLS) según el claim tenant del token. NUNCA expone secretos
 * al cliente: los secretos se escriben write-only y jamás vuelven. Degrada con
 * elegancia (catálogo/instancias vacíos) cuando el servicio no está vivo, igual que
 * lib/admin.ts. docs/CONVENTIONS.md §8, §11.
 */
import { HttpClient, HttpError, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { gatewayBaseUrl } from './auth.js';
import type { DesenlaceConsentimiento } from './consentimiento.js';

// ---- Gobernanza L5 (DLP/política de egress + auditoría) ----------------------
// Tipos + loaders + mutaciones para la página de Gobernanza. Todo pasa por el
// gateway Go, aislado por org (RLS). El BFF solo reenvía.

/** Veredicto de una regla sobre un egress. */
export type PolicyAction = 'allow' | 'warn' | 'block';

/** Condición de match de una regla (campos opcionales AND-combinados; vacío = todo).
 *  Las claves SON el contrato de la API (snake_case): se envían tal cual al gateway. */
export interface PolicyCondition {
  api_family?: string;
  host?: string;
  method?: string;
  path_prefix?: string;
  body_contains?: string;
}

/** Regla de política de egress. */
export interface PolicyRule {
  id: string;
  name: string;
  condition: PolicyCondition;
  action: PolicyAction;
  enabled: boolean;
}

/** Fila del registro de egress (solo metadatos; jamás payloads ni tokens). */
export interface EgressAuditRow {
  id: string;
  occurredAt: string;
  instanceId: string;
  host: string;
  method: string;
  status: number;
  reqBytes: number;
  respBytes: number;
  latencyMs: number;
  auth: string;
  error: string;
}

/** Decisión de política (block/warn) registrada en el egress. */
export interface PolicyDecisionRow {
  id: string;
  occurredAt: string;
  instanceId: string;
  policyName: string;
  action: 'warn' | 'block';
  requestId: string;
  host: string;
  method: string;
}

/** Página con cursor keyset opaco ('' = no hay más). */
export interface Page<T> {
  items: T[];
  nextCursor: string;
}

export interface EgressQuery {
  host?: string | undefined;
  status?: string | undefined;
  since?: string | undefined;
  until?: string | undefined;
  cursor?: string | undefined;
  limit?: number | undefined;
}

export interface DecisionQuery {
  action?: string | undefined;
  cursor?: string | undefined;
  limit?: number | undefined;
}

/** Cuerpo de alta/edición de una regla. */
export interface PolicyInput {
  name: string;
  condition: PolicyCondition;
  action: PolicyAction;
  enabled: boolean;
}

interface PolicyWire {
  id: string;
  name: string;
  condition?: PolicyCondition | null;
  action: PolicyAction;
  enabled: boolean;
}

interface EgressWire {
  id: string;
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
  id: string;
  occurred_at: string;
  instance_id: string;
  policy_name: string;
  action: 'warn' | 'block';
  request_id: string;
  host: string;
  method: string;
}

function toPolicyRule(w: PolicyWire): PolicyRule {
  return {
    id: w.id,
    name: w.name,
    condition: w.condition ?? {},
    action: w.action,
    enabled: Boolean(w.enabled),
  };
}

function toEgressRow(w: EgressWire): EgressAuditRow {
  return {
    id: w.id,
    occurredAt: w.occurred_at,
    instanceId: w.instance_id,
    host: w.host,
    method: w.method,
    status: w.status,
    reqBytes: w.req_bytes,
    respBytes: w.resp_bytes,
    latencyMs: w.latency_ms,
    auth: w.auth,
    error: w.error,
  };
}

function toDecisionRow(w: DecisionWire): PolicyDecisionRow {
  return {
    id: w.id,
    occurredAt: w.occurred_at,
    instanceId: w.instance_id,
    policyName: w.policy_name,
    action: w.action,
    requestId: w.request_id,
    host: w.host,
    method: w.method,
  };
}

/** Reglas de política de la org (GET /admin/policies). */
export async function loadPolicies(session: SytueSession): Promise<Sourced<PolicyRule[]>> {
  try {
    const res = await gatewayHttp(session).json<{ policies?: PolicyWire[] }>({
      path: '/admin/policies',
    });
    if (!Array.isArray(res?.policies)) throw new Error('bad_shape');
    return { live: true, data: res.policies.map(toPolicyRule) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Página del registro de egress (GET /admin/egress-audit). */
export async function loadEgressPage(
  session: SytueSession,
  q: EgressQuery = {},
): Promise<Sourced<Page<EgressAuditRow>>> {
  try {
    const page = await gatewayHttp(session).json<{ items: EgressWire[]; nextCursor?: string }>({
      path: '/admin/egress-audit',
      query: {
        host: q.host || undefined,
        status: q.status || undefined,
        since: q.since || undefined,
        until: q.until || undefined,
        cursor: q.cursor || undefined,
        limit: q.limit,
      },
    });
    if (!Array.isArray(page?.items)) throw new Error('bad_shape');
    return {
      live: true,
      data: {
        items: page.items.map(toEgressRow),
        nextCursor: typeof page.nextCursor === 'string' ? page.nextCursor : '',
      },
    };
  } catch {
    return { live: false, data: { items: [], nextCursor: '' } };
  }
}

/** Página de decisiones de política (GET /admin/policy-decisions). */
export async function loadDecisionsPage(
  session: SytueSession,
  q: DecisionQuery = {},
): Promise<Sourced<Page<PolicyDecisionRow>>> {
  try {
    const page = await gatewayHttp(session).json<{ items: DecisionWire[]; nextCursor?: string }>({
      path: '/admin/policy-decisions',
      query: { action: q.action || undefined, cursor: q.cursor || undefined, limit: q.limit },
    });
    if (!Array.isArray(page?.items)) throw new Error('bad_shape');
    return {
      live: true,
      data: {
        items: page.items.map(toDecisionRow),
        nextCursor: typeof page.nextCursor === 'string' ? page.nextCursor : '',
      },
    };
  } catch {
    return { live: false, data: { items: [], nextCursor: '' } };
  }
}

/** Claves de condición reconocidas (el resto se descarta al sanear la entrada). */
const CONDITION_KEYS = ['api_family', 'host', 'method', 'path_prefix', 'body_contains'] as const;

/**
 * Sanea el cuerpo de un alta/edición de regla a un PolicyInput, o null si falta
 * name o la acción no es válida. La validación dura la aplica el gateway; esto
 * es el espejo mínimo del BFF (recorta a claves conocidas de condición).
 */
export function sanitizePolicyInput(body: unknown): PolicyInput | null {
  const b = (body ?? {}) as Record<string, unknown>;
  const name = typeof b.name === 'string' ? b.name.trim() : '';
  const action = b.action;
  if (!name || (action !== 'allow' && action !== 'warn' && action !== 'block')) return null;
  const rawCond = (b.condition ?? {}) as Record<string, unknown>;
  const condition: PolicyCondition = {};
  for (const k of CONDITION_KEYS) {
    const v = rawCond[k];
    if (typeof v === 'string' && v.trim()) condition[k] = v.trim();
  }
  return { name, condition, action, enabled: typeof b.enabled === 'boolean' ? b.enabled : true };
}

/** Crea una regla y la devuelve mapeada. */
export async function createPolicy(session: SytueSession, input: PolicyInput): Promise<PolicyRule> {
  return toPolicyRule(await gatewayHttp(session).postJson<PolicyWire>('/admin/policies', input));
}

/** Reemplaza una regla (estado deseado completo) y la devuelve mapeada. */
export async function updatePolicy(
  session: SytueSession,
  id: string,
  input: PolicyInput,
): Promise<PolicyRule> {
  const w = await gatewayHttp(session).json<PolicyWire>({
    method: 'PATCH',
    path: `/admin/policies/${encodeURIComponent(id)}`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(input),
  });
  return toPolicyRule(w);
}

/** Borra una regla. */
export function deletePolicy(session: SytueSession, id: string): Promise<unknown> {
  return gatewayHttp(session).json({
    method: 'DELETE',
    path: `/admin/policies/${encodeURIComponent(id)}`,
  });
}

/**
 * Respuesta cruda del export CSV del registro de egress (streaming) para reenviar
 * por el BFF. fetch directo con el token de sesión (el HttpClient consumiría el
 * cuerpo como JSON), devolviendo el Response para hacer passthrough del stream.
 */
export function egressExportResponse(session: SytueSession, q: EgressQuery): Promise<Response> {
  const url = new URL(`${gatewayBaseUrl().replace(/\/$/, '')}/api/v1/admin/egress-audit`);
  url.searchParams.set('format', 'csv');
  for (const [k, v] of Object.entries({
    host: q.host,
    status: q.status,
    since: q.since,
    until: q.until,
  })) {
    if (v) url.searchParams.set(k, String(v));
  }
  return fetch(url, {
    headers: { authorization: `Bearer ${session.tokens.accessToken}`, accept: 'text/csv' },
  });
}

// ---- Migración de contenido desde Microsoft 365 / Google Workspace -----------
// El contenido aterriza en la cuenta de CADA persona: el gateway canjea un token
// acotado a ella (RFC 8693) y escribe con él. Un trabajo por persona y tipo de
// contenido, así que el fallo de uno no arrastra a los demás.

/** Qué contenido trae un trabajo. Correo y calendario aún no se migran. */
export type MigrationDomain =
  'drive' | 'contacts' | 'repos' | 'gists' | 'teams' | 'team_files' | 'mail' | 'calendar' | 'cloud';

/** SaaS del que se importa contenido. */
export type MigrationSource = 'm365' | 'google' | 'github';

export type MigrationStatus = 'queued' | 'running' | 'done' | 'error' | 'canceled';

/** Trabajo de migración tal y como lo devuelve el gateway. */
export interface MigrationJob {
  id: string;
  instanceId: string;
  /** Persona de Sytue en cuya cuenta aterriza el contenido. */
  targetUser: string;
  /** Cuenta del origen de la que se copia. */
  sourceUser: string;
  source: MigrationSource;
  domain: MigrationDomain;
  status: MigrationStatus;
  counts: { migrated: number; skipped: number; errors: number; bytesCopied: number };
  lastError: string;
  createdAt: string;
  finishedAt?: string;
}

/** Entidad NO copiada, con su motivo (la mitad honesta del informe). */
export interface MigrationSkipped {
  sourceId: string;
  kind: string;
  path: string;
  reason: string;
}

/** Alta de una tanda: personas × tipos de contenido. */
export type MigrationDirection = 'in' | 'out';

export interface MigrationInput {
  instance_id: string;
  source: MigrationSource;
  domains: MigrationDomain[];
  targets: { source_user: string; target_user: string }[];
  /** 'in' trae el contenido a Sytue; 'out' se lo lleva al tercero. */
  direction?: MigrationDirection;
}

interface MigrationWire {
  id: string;
  instance_id: string;
  target_user: string;
  source_user: string;
  source: MigrationSource;
  domain: MigrationDomain;
  status: MigrationStatus;
  counts?: { migrated: number; skipped: number; errors: number; bytes_copied: number };
  last_error?: string;
  created_at: string;
  finished_at?: string;
}

function toMigrationJob(w: MigrationWire): MigrationJob {
  const job: MigrationJob = {
    id: w.id,
    instanceId: w.instance_id,
    targetUser: w.target_user,
    sourceUser: w.source_user,
    source: w.source,
    domain: w.domain,
    status: w.status,
    counts: {
      migrated: w.counts?.migrated ?? 0,
      skipped: w.counts?.skipped ?? 0,
      errors: w.counts?.errors ?? 0,
      bytesCopied: w.counts?.bytes_copied ?? 0,
    },
    lastError: w.last_error ?? '',
    createdAt: w.created_at,
  };
  if (w.finished_at) job.finishedAt = w.finished_at;
  return job;
}

/** Trabajos de migración de la org (GET /migrations). */
export async function loadMigrations(session: SytueSession): Promise<Sourced<MigrationJob[]>> {
  try {
    const res = await gatewayHttp(session).json<{ migrations?: MigrationWire[] }>({
      path: '/migrations',
      query: { limit: 200 },
    });
    if (!Array.isArray(res?.migrations)) throw new Error('bad_shape');
    return { live: true, data: res.migrations.map(toMigrationJob) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Da de alta la tanda y devuelve los trabajos creados (POST /migrations). */
export async function createMigrations(
  session: SytueSession,
  input: MigrationInput,
): Promise<MigrationJob[]> {
  const res = await gatewayHttp(session).postJson<{ migrations?: MigrationWire[] }>(
    '/migrations',
    input,
  );
  return (res.migrations ?? []).map(toMigrationJob);
}

/** Progreso + informe de un trabajo (GET /migrations/{id}). */
export async function loadMigration(
  session: SytueSession,
  id: string,
): Promise<{ migration: MigrationJob; skipped: MigrationSkipped[] }> {
  const res = await gatewayHttp(session).json<{
    migration: MigrationWire;
    skipped?: { source_id: string; kind?: string; path?: string; skip_reason?: string }[];
  }>({ path: `/migrations/${encodeURIComponent(id)}` });
  return {
    migration: toMigrationJob(res.migration),
    skipped: (res.skipped ?? []).map((s) => ({
      sourceId: s.source_id,
      kind: s.kind ?? '',
      path: s.path ?? '',
      reason: s.skip_reason ?? '',
    })),
  };
}

/** cancel | resume sobre un trabajo (POST /migrations/{id}/{action}). */
export function migrationAction(
  session: SytueSession,
  id: string,
  action: 'cancel' | 'resume',
): Promise<unknown> {
  return gatewayHttp(session).postJson(`/migrations/${encodeURIComponent(id)}/${action}`, {});
}

/**
 * Plan del asistente AUTOMÁTICO: qué se traería con la conexión ya autorizada, para
 * quién, y qué cuentas del origen no tienen equivalente en Sytue. No copia nada.
 */
export interface MigrationPlan {
  source: MigrationSource;
  instanceId: string;
  /** Cuenta del origen que autorizó la conexión. */
  connectedAs: string;
  /** 'user' = lo de esa cuenta; 'organization' = todo el personal del origen. */
  scope: 'user' | 'organization';
  /** El origen permite ampliar a toda la organización, pero aún falta autorizarlo. */
  canExpandToOrg: boolean;
  /** Enlace directo a la pantalla del proveedor donde se concede esa ampliación. */
  delegationSetupUrl: string;
  items: { domain: MigrationDomain; destination: string; count: number }[];
  people: { sourceUser: string; targetUser: string; name: string }[];
  unmatched: string[];
  domains: MigrationDomain[];
  jobs: number;
}

interface MigrationPlanWire {
  source: MigrationSource;
  instance_id: string;
  connected_as?: string;
  scope: 'user' | 'organization';
  can_expand_to_org?: boolean;
  delegation_setup_url?: string;
  items?: { domain: MigrationDomain; destination: string; count: number }[];
  people?: { source_user: string; target_user: string; name?: string }[];
  unmatched?: string[];
  domains?: MigrationDomain[];
  jobs?: number;
}

/** Descubre el plan de una conexión ya autorizada (GET /instances/{id}/migration-plan). */
export async function loadMigrationPlan(
  session: SytueSession,
  instanceId: string,
): Promise<MigrationPlan> {
  const res = await gatewayHttp(session).json<{ plan: MigrationPlanWire }>({
    path: `/instances/${encodeURIComponent(instanceId)}/migration-plan`,
  });
  const p = res.plan;
  return {
    source: p.source,
    instanceId: p.instance_id,
    connectedAs: p.connected_as ?? '',
    scope: p.scope,
    canExpandToOrg: p.can_expand_to_org ?? false,
    delegationSetupUrl: p.delegation_setup_url ?? '',
    items: p.items ?? [],
    people: (p.people ?? []).map((x) => ({
      sourceUser: x.source_user,
      targetUser: x.target_user,
      name: x.name ?? '',
    })),
    unmatched: p.unmatched ?? [],
    domains: p.domains ?? [],
    jobs: p.jobs ?? 0,
  };
}

/** Lanza el plan entero (POST /migrations/auto). */
export async function autoMigrate(
  session: SytueSession,
  instanceId: string,
): Promise<MigrationJob[]> {
  const res = await gatewayHttp(session).postJson<{ migrations?: MigrationWire[] }>(
    '/migrations/auto',
    { instance_id: instanceId },
  );
  return (res.migrations ?? []).map(toMigrationJob);
}

/** Lanza (o continúa) la salida de lo propio (POST /migrations/self/out). */
export async function selfOutbound(
  session: SytueSession,
  instanceId: string,
  domains: MigrationDomain[],
): Promise<MigrationJob[]> {
  const res = await gatewayHttp(session).postJson<{ migrations?: MigrationWire[] }>(
    '/migrations/self/out',
    { instance_id: instanceId, domains },
  );
  return (res.migrations ?? []).map(toMigrationJob);
}

/** Sanea el cuerpo de un alta de migración, o null si le falta algo esencial. */
export function sanitizeMigrationInput(body: unknown): MigrationInput | null {
  const b = (body ?? {}) as Record<string, unknown>;
  const instance = typeof b.instance_id === 'string' ? b.instance_id.trim() : '';
  const source: MigrationSource | null =
    b.source === 'm365' || b.source === 'google' || b.source === 'github' ? b.source : null;
  const domains = Array.isArray(b.domains)
    ? b.domains.filter(
        (d): d is MigrationDomain =>
          d === 'drive' ||
          d === 'contacts' ||
          d === 'repos' ||
          d === 'gists' ||
          d === 'teams' ||
          d === 'team_files' ||
          d === 'mail' ||
          d === 'calendar' ||
          d === 'cloud',
      )
    : [];
  const rawTargets = Array.isArray(b.targets) ? b.targets : [];
  const targets = rawTargets
    .map((t) => (t ?? {}) as Record<string, unknown>)
    .map((t) => ({
      source_user: typeof t.source_user === 'string' ? t.source_user.trim() : '',
      target_user: typeof t.target_user === 'string' ? t.target_user.trim() : '',
    }))
    .filter((t) => t.source_user && t.target_user);
  const direction: MigrationDirection = b.direction === 'out' ? 'out' : 'in';
  if (!instance || !source || domains.length === 0 || targets.length === 0) return null;
  return { instance_id: instance, source, domains, targets, direction };
}

export interface Sourced<T> {
  live: boolean;
  data: T;
}

/** Código HTTP a propagar por el BFF cuando una acción contra el gateway falla. */
export function proxyStatus(e: unknown): number {
  return e instanceof HttpError ? e.status : 502;
}

/** HttpClient autenticado a la API del gateway (/api/v1). */
function gatewayHttp(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${gatewayBaseUrl().replace(/\/$/, '')}/api/v1`,
    auth: bearer(session.tokens.accessToken),
  });
}

// ---- Tipos de presentación (derivados del manifiesto declarativo) ----

export interface CatalogField {
  key: string;
  label: string;
  placeholder?: string;
  help?: string;
  required?: boolean;
}

export interface CatalogApp {
  key: string;
  name: string;
  description: string;
  category?: string;
  color: string;
  iconUrl: string;
  identity: 'saml' | 'oidc' | 'none';
  provisioning: 'scim' | 'none';
  hasData: boolean;
  fields: CatalogField[];
  secret?: { label: string; help?: string };
  /** Presente ⇒ la app se conecta por CONSENTIMIENTO: un botón, no un token pegado.
   *  Los scopes se enseñan antes de redirigir: un consentimiento que no se entiende no
   *  es consentimiento. */
  oauth?: { scopes: string[] };
  tier: string;
}

export interface ConnectedInstance {
  id: string;
  templateKey: string;
  name: string;
  status: 'draft' | 'active' | 'error' | 'disabled';
  hasSecrets: boolean;
  launchUrl?: string;
}

export interface SyncRun {
  id: string;
  status: 'running' | 'ok' | 'error';
  counts: { created: number; updated: number; deactivated: number; errors: number };
  startedAt: string;
  finishedAt?: string;
}

// ---- Forma en el cable (gateway Go) ----

interface ManifestWire {
  name: string;
  description: string;
  category?: string;
  color: string;
  icon: string;
  identity?: { kind?: string };
  provisioning?: string;
  data?: string;
  fields?: CatalogField[];
  secret?: { label: string; help?: string };
  oauth?: { scopes?: string[] };
}

interface TemplateWire {
  key: string;
  manifest: ManifestWire;
  tier: string;
  enabled: boolean;
}

interface InstanceWire {
  id: string;
  template_key: string;
  name: string;
  status: ConnectedInstance['status'];
  has_secrets: boolean;
  config?: Record<string, string> | null;
}

/** SVG inline curado → data URI para <img>. encodeURIComponent escapa '#' de colores. */
function svgToDataUri(svg: string): string {
  return svg ? `data:image/svg+xml,${encodeURIComponent(svg)}` : '';
}

function toCatalogApp(t: TemplateWire): CatalogApp {
  const m = t.manifest;
  const identity = (m.identity?.kind ?? 'none') as CatalogApp['identity'];
  const app: CatalogApp = {
    key: t.key,
    name: m.name,
    description: m.description,
    color: m.color,
    iconUrl: svgToDataUri(m.icon),
    identity: identity === 'saml' || identity === 'oidc' ? identity : 'none',
    provisioning: m.provisioning === 'scim' ? 'scim' : 'none',
    hasData: Boolean(m.data),
    fields: Array.isArray(m.fields) ? m.fields : [],
    tier: t.tier,
  };
  if (m.category) app.category = m.category;
  if (m.secret && m.secret.label) app.secret = m.secret;
  if (m.oauth && Array.isArray(m.oauth.scopes)) app.oauth = { scopes: m.oauth.scopes };
  return app;
}

function toConnectedInstance(w: InstanceWire): ConnectedInstance {
  const inst: ConnectedInstance = {
    id: w.id,
    templateKey: w.template_key,
    name: w.name,
    status: w.status,
    hasSecrets: Boolean(w.has_secrets),
  };
  const launch = w.config?.['_launch_url'];
  if (launch) inst.launchUrl = launch;
  return inst;
}

// ---- SSR: catálogo + instancias de la org ----

/** Catálogo de plantillas (GET /templates) mapeado a tiles de presentación. */
export async function loadCatalog(session: SytueSession): Promise<Sourced<CatalogApp[]>> {
  try {
    const res = await gatewayHttp(session).json<{ templates?: TemplateWire[] }>({
      path: '/templates',
    });
    if (!Array.isArray(res?.templates)) throw new Error('bad_shape');
    return { live: true, data: res.templates.map(toCatalogApp) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Instancias configuradas de la org (GET /instances). */
export async function loadConnected(session: SytueSession): Promise<Sourced<ConnectedInstance[]>> {
  try {
    const res = await gatewayHttp(session).json<{ instances?: InstanceWire[] }>({
      path: '/instances',
    });
    if (!Array.isArray(res?.instances)) throw new Error('bad_shape');
    return { live: true, data: res.instances.map(toConnectedInstance) };
  } catch {
    return { live: false, data: [] };
  }
}

// ---- Mutaciones (las llaman los endpoints BFF; puro passthrough) ----

/** Crea una instancia (estado 'draft'). */
export function createInstance(
  session: SytueSession,
  body: { template_key: string; name: string; config?: Record<string, string> },
): Promise<unknown> {
  return gatewayHttp(session).postJson('/instances', body);
}

/** Borra una instancia (y su secreto en cascada). */
export function deleteInstance(session: SytueSession, id: string): Promise<unknown> {
  return gatewayHttp(session).json({
    method: 'DELETE',
    path: `/instances/${encodeURIComponent(id)}`,
  });
}

/** Acciones sobre una instancia. `action` es el segmento literal de la API del gateway
 *  (activate|deactivate|test|sync|sync-runs|secrets): el BFF solo reenvía. */
export function instanceAction(
  session: SytueSession,
  id: string,
  action: string,
  method: 'GET' | 'POST' | 'PUT' | 'PATCH',
  body?: unknown,
): Promise<unknown> {
  const path = `/instances/${encodeURIComponent(id)}/${action}`;
  if (method === 'GET') return gatewayHttp(session).json({ path });
  return gatewayHttp(session).json({
    method,
    path,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  });
}

/**
 * Termina el consentimiento OAuth de una conexión con la sesión de quien vuelve del
 * proveedor. El servicio solo canjea el código si es la persona que lo empezó
 * (NEG-10) y responde siempre con el desenlace (ver lib/consentimiento.ts).
 */
export function finishOAuth(
  session: SytueSession,
  vuelta: { state: string; code: string; error: string },
): Promise<DesenlaceConsentimiento> {
  return gatewayHttp(session).json<DesenlaceConsentimiento>({
    method: 'POST',
    path: '/oauth/finish',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(vuelta),
  });
}

// ---- Libertad de elección: quién sirve cada producto de la suite ----------

/** Una alternativa capaz de servir un producto de la suite. */
export interface SubstitutionOption {
  instanceId: string;
  name: string;
  iconUrl: string;
  color: string;
  openUrl: string;
  /** Sabe además abrir UN documento concreto, no solo su portada. */
  hasDocumentHandoff: boolean;
}

/** El estado de un producto: quién lo sirve hoy y quién podría. */
export interface ProductChoice {
  product: string;
  /** Vacío ⇒ lo sirve el producto nativo de Sytue. */
  currentInstanceId: string;
  options: SubstitutionOption[];
}

interface ChoiceWire {
  product?: unknown;
  current_instance_id?: unknown;
  options?: unknown;
}

export function toOption(raw: unknown): SubstitutionOption | null {
  const o = raw as Record<string, unknown> | null;
  if (!o || typeof o.instance_id !== 'string') return null;
  return {
    instanceId: o.instance_id,
    name: typeof o.name === 'string' ? o.name : o.instance_id,
    // El servicio manda el SVG del manifiesto, no una URL: se pinta como imagen.
    iconUrl: svgToDataUri(typeof o.iconUrl === 'string' ? o.iconUrl : ''),
    color: typeof o.color === 'string' ? o.color : 'var(--sytue-primary-500)',
    openUrl: typeof o.open_url === 'string' ? o.open_url : '',
    hasDocumentHandoff: o.has_document_handoff === true,
  };
}

/** Estado completo de la elección por producto (GET /substitutions). */
export async function loadProductChoices(session: SytueSession): Promise<Sourced<ProductChoice[]>> {
  try {
    const res = await gatewayHttp(session).json<{ products?: ChoiceWire[] }>({
      path: '/substitutions',
    });
    if (!Array.isArray(res?.products)) throw new Error('bad_shape');
    return {
      live: true,
      data: res.products.map((p) => ({
        product: typeof p.product === 'string' ? p.product : '',
        currentInstanceId: typeof p.current_instance_id === 'string' ? p.current_instance_id : '',
        options: Array.isArray(p.options)
          ? p.options.map(toOption).filter((o): o is SubstitutionOption => o !== null)
          : [],
      })),
    };
  } catch {
    return { live: false, data: [] };
  }
}

/** Elige quién sirve un producto. instanceId vacío devuelve el producto a Sytue. */
export function setProductChoice(
  session: SytueSession,
  product: string,
  instanceId: string,
): Promise<unknown> {
  return gatewayHttp(session).json({
    method: 'PUT',
    path: `/substitutions/${encodeURIComponent(product)}`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ instance_id: instanceId }),
  });
}

/** Integración de serie que cada persona conecta desde la app; Admin solo la apaga. */
export interface BuiltinIntegration {
  provider: string;
  enabled: boolean;
}

export async function loadBuiltinIntegrations(
  session: SytueSession,
): Promise<Sourced<BuiltinIntegration[]>> {
  try {
    const res = await gatewayHttp(session).json<{ items?: BuiltinIntegration[] }>({
      path: '/admin/integrations',
    });
    if (!Array.isArray(res?.items)) throw new Error('bad_shape');
    return { live: true, data: res.items };
  } catch {
    return { live: false, data: [] };
  }
}

export function setBuiltinIntegration(
  session: SytueSession,
  provider: string,
  enabled: boolean,
): Promise<unknown> {
  return gatewayHttp(session).json({
    method: 'PUT',
    path: `/admin/integrations/${encodeURIComponent(provider)}`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ enabled }),
  });
}
