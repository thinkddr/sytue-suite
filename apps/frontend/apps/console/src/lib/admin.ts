/*
 * Proveedor de datos de administración de Sytue Admin (SOLO servidor).
 *
 * Cablea las páginas/endpoints SSR a la API de administración de Sytue ID
 * (identidad/SCIM) y a Sytue Hub (auditoría) vía @sytue/api-clients con el
 * access token de la sesión (rol admin/owner). Usa un HttpClient autenticado del
 * propio SDK para las superficies
 * admin/SCIM/auditoría que aún no tienen método dedicado. Degrada con elegancia
 * al seed de demostracion (data.ts) cuando NO hay backend vivo. NUNCA expone
 * tokens ni secretos al cliente. docs/CONVENTIONS.md §8, §9, §11.
 */
import { HttpClient, HttpError, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { adminBaseUrl } from './auth.js';
import * as seed from './data.js';

/**
 * ¿Ha CONTESTADO el backend? Un 4xx es una respuesta —«no puedes» o «no hay»— y
 * ante eso no se degrada al seed: enseñar «ACME Europa» con 42 usuarios a quien
 * Sytue ID acaba de responder 403 es contarle una mentira con aspecto de panel.
 * El seed queda para lo que era: la instalación sin backend vivo (red, 5xx).
 */
function contestoElBackend(e: unknown): boolean {
  return e instanceof HttpError && e.status < 500;
}
import type {
  AccessAction,
  AccessPolicy,
  AccessPolicyInput,
  AdminGroup,
  AdminGroupMember,
  AuditEvent,
  ConsoleMetric,
  CustomRole,
  CustomRoleInput,
  CustomRoleMember,
  Device,
  DirectoryUser,
  Group,
  MembershipRule,
  OidcApp,
  OidcAppInput,
  PasswordPolicy,
  PimActivateInput,
  PimActivation,
  PimEligibility,
  PimEligibilityInput,
  SamlSp,
  SensitivityLabel,
  SensitivityLabelInput,
  SsoProvider,
  SsoProviderInput,
  SytueRole,
  Tenant,
} from './types.js';
import { contiene } from '@sytue/i18n';

/** Default de la política de contraseñas (espeja el default de la migración). */
const DEFAULT_PASSWORD_POLICY: PasswordPolicy = { min_length: 12, require_complexity: false };

export interface Sourced<T> {
  live: boolean;
  data: T;
}

/**
 * Código HTTP a devolver por el BFF cuando una acción admin contra Sytue ID
 * falla: propaga el status del servicio (403/404/409…) para que la UI muestre el
 * motivo real; cualquier otro fallo (red, etc.) es un 502 genérico.
 */
export function proxyStatus(e: unknown): number {
  return e instanceof HttpError ? e.status : 502;
}

/**
 * Cuerpo de error que el BFF devuelve cuando Sytue ID rechaza una acción,
 * CONSERVANDO el `code` y el `message` del servicio.
 *
 * Los 4xx de esta API responden a lo que el admin acaba de hacer —«Solo un owner
 * puede asignar el rol owner» distingue el rechazo de «No autorizado en esta
 * organización», y los dos son `code: forbidden`—, así que aplanarlos a «error»
 * deja probando a ciegas justo donde hay algo concreto que corregir. Es el mismo
 * criterio que ya sigue el proxy de plataforma.
 *
 * El sobre de httpx va ANIDADO ({"error":{"code","message"},"request_id"}), y
 * `HttpError.code` lee la forma plana: por eso se desanida aquí a mano, igual
 * que en lib/platform.ts.
 */
export function proxyError(e: unknown): { error: string; code?: string; message?: string } {
  if (!(e instanceof HttpError) || !e.body) return { error: 'upstream' };
  try {
    const body = JSON.parse(e.body) as {
      error?: { code?: string; message?: string };
      code?: string;
      message?: string;
    };
    const err = body.error ?? body;
    return {
      error: 'upstream',
      ...(typeof err.code === 'string' ? { code: err.code } : {}),
      ...(typeof err.message === 'string' ? { message: err.message } : {}),
    };
  } catch {
    // Cuerpo que no es JSON (un 502 de la malla, una página HTML): no hay motivo
    // que propagar y el status ya lo lleva proxyStatus.
    return { error: 'upstream' };
  }
}

/** HttpClient autenticado a la API de administración de Sytue ID (/api/v1/admin). */
function adminHttp(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${adminBaseUrl().replace(/\/$/, '')}/api/v1/admin`,
    auth: bearer(session.tokens.accessToken),
  });
}

/** HttpClient autenticado a la API self-service de Sytue ID (/api/v1). */
function meHttp(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${adminBaseUrl().replace(/\/$/, '')}/api/v1`,
    auth: bearer(session.tokens.accessToken),
  });
}

// ---- Tenants (organizaciones) ----

/** Página del listado de organizaciones; nextCursor '' cuando no hay más. */
export interface TenantsPage {
  items: Tenant[];
  nextCursor: string;
}

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/tenants). */
interface AdminTenantWire {
  id: string;
  slug: string;
  name: string;
  status: 'active' | 'suspended';
  user_count: number;
  namespace: string;
  created_at: string;
  /** Rol del SOLICITANTE en esta org (owner|admin); lo emite tenantWire en Go. */
  role?: string;
}

/**
 * Mapea la fila del servicio Go al modelo de la Console. `mailDomain` y `plan` NO
 * están en el modelo de identidad de Sytue ID (son conceptos de correo/facturación
 * de otros servicios): se rellenan con valores de presentación por defecto hasta
 * que exista ese modelo. Slug, nombre, estado, namespace y recuento sí son reales.
 */
function toTenant(w: AdminTenantWire): Tenant {
  return {
    slug: w.slug,
    name: w.name,
    mailDomain: `${w.slug}.sytue.eu`,
    plan: 'starter',
    userCount: typeof w.user_count === 'number' ? w.user_count : 0,
    namespace: w.namespace || `tenant-${w.slug}`,
    createdAt: w.created_at,
    status: w.status === 'suspended' ? 'suspended' : 'active',
    // Fail-closed: solo 'owner' explícito concede la capacidad de nombrar owners.
    // Un servicio viejo que aún no mande el campo cae en 'admin', que es la
    // opción que NO ofrece nada que el backend vaya a rechazar.
    role: w.role === 'owner' ? 'owner' : 'admin',
  };
}

/**
 * Página de organizaciones que el admin GESTIONA (owner/admin), con paginación
 * keyset: `cursor` es el token opaco devuelto como nextCursor por la página
 * anterior (omitir en la primera). Degrada al seed si no hay backend vivo.
 */
export async function loadTenantsPage(
  session: SytueSession,
  opts: { cursor?: string; limit?: number } = {},
): Promise<Sourced<TenantsPage>> {
  try {
    const page = await adminHttp(session).json<{ items: AdminTenantWire[]; nextCursor?: string }>({
      path: '/tenants',
      query: { cursor: opts.cursor || undefined, limit: opts.limit },
    });
    if (!Array.isArray(page?.items) || page.items.length === 0) throw new Error('empty');
    return {
      live: true,
      data: {
        items: page.items.map(toTenant),
        nextCursor: typeof page.nextCursor === 'string' ? page.nextCursor : '',
      },
    };
  } catch (e) {
    if (contestoElBackend(e)) return { live: false, data: { items: [], nextCursor: '' } };
    return { live: false, data: { items: seed.tenants, nextCursor: '' } };
  }
}

/** Organizaciones (primera página) para consumidores que solo necesitan la lista. */
export async function loadTenants(session: SytueSession): Promise<Sourced<Tenant[]>> {
  const { live, data } = await loadTenantsPage(session, { limit: 200 });
  return { live, data: data.items };
}

export async function loadTenant(
  session: SytueSession,
  slug: string,
): Promise<Sourced<Tenant> | undefined> {
  const { live, data } = await loadTenants(session);
  const match = data.find((t) => t.slug === slug);
  return match ? { live, data: match } : undefined;
}

/** Crea una organización; el creador queda como owner. */
export async function createTenant(
  session: SytueSession,
  input: { slug: string; name: string },
): Promise<Tenant> {
  const w = await adminHttp(session).postJson<AdminTenantWire>('/tenants', input);
  return toTenant(w);
}

/**
 * Edita los ajustes de una organización (nombre/estado). Sytue ID exige que el
 * solicitante sea owner/admin de ESA org (cross-org ⇒ 404). El slug es inmutable.
 */
export async function updateTenant(
  session: SytueSession,
  slug: string,
  patch: { name?: string; status?: 'active' | 'suspended' },
): Promise<Tenant> {
  const w = await adminHttp(session).json<AdminTenantWire>({
    method: 'PATCH',
    path: `/tenants/${encodeURIComponent(slug)}`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(patch),
  });
  return toTenant(w);
}

/**
 * MULTI-PERTENENCIA: añade a una persona QUE YA EXISTE a otra organización con
 * un rol (POST /api/v1/orgs/{slug}/members). Es el caso de quien trabaja en dos
 * organizaciones del mismo despliegue: una sola identidad, dos membresías.
 *
 * No da de alta a nadie —para eso está la invitación—: si no hay cuenta con ese
 * correo, Sytue ID responde 404. Cuelga de /api/v1 (no de /api/v1/admin), de ahí
 * meHttp. La autorización la decide ENTERA el servicio: owner/admin de ESA org
 * (verificado contra la BD, no por el claim del token) y solo un owner puede
 * conceder el rol owner. Lanza HttpError; el BFF propaga status y motivo.
 */
export async function addOrgMember(
  session: SytueSession,
  slug: string,
  input: { email: string; role: SytueRole },
): Promise<{ org: string; user_id: string; role: string }> {
  return meHttp(session).postJson(`/orgs/${encodeURIComponent(slug)}/members`, input);
}

// ---- Usuarios ----

/** Página del directorio de usuarios; nextCursor '' cuando no hay más. */
export interface UsersPage {
  items: DirectoryUser[];
  nextCursor: string;
}

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/users). */
interface AdminUserWire {
  id: string;
  email: string;
  email_verified: boolean;
  name: string;
  mfa_enabled: boolean;
  status: 'active' | 'suspended' | 'deleted';
  role: string;
  created_at: string;
  department?: string;
  title?: string;
  location?: string;
}

const SYTUE_ROLES: readonly SytueRole[] = ['owner', 'admin', 'member', 'billing'];

/** Mapea la fila del servicio Go al modelo de la Console. */
function toDirectoryUser(w: AdminUserWire, tenant: string): DirectoryUser {
  return {
    id: w.id,
    username: w.email.split('@')[0] ?? w.email,
    email: w.email,
    fullName: w.name || w.email,
    enabled: w.status === 'active',
    emailVerified: w.email_verified,
    roles: [SYTUE_ROLES.includes(w.role as SytueRole) ? (w.role as SytueRole) : 'member'],
    groups: tenant ? [`org/${tenant}`] : [],
    tenant,
    mfaEnabled: w.mfa_enabled,
    createdAt: w.created_at,
    // Sytue ID no expone (aún) estado SCIM por servicio; un alta viva está sincronizada.
    provisioning: { drive: 'synced', chat: 'synced', mail: 'synced' },
    department: w.department ?? '',
    jobTitle: w.title ?? '',
    location: w.location ?? '',
  };
}

/**
 * Directorio de usuarios de la organización activa con paginación keyset:
 * `cursor` es el token opaco devuelto como nextCursor por la página anterior
 * (omitir en la primera). Degrada al seed (sin cursor) si no hay backend vivo.
 *
 * `q` busca en el SERVIDOR por correo o nombre. Filtrar lo ya cargado daba la
 * misma respuesta —«sin resultados»— tanto si alguien no está en la
 * organización como si está en la página doce.
 */
export async function loadUsers(
  session: SytueSession,
  tenant?: string,
  opts: { cursor?: string; limit?: number; q?: string } = {},
): Promise<Sourced<UsersPage>> {
  try {
    // `consulta` y no `json`: va por QUERY si el servicio lo entiende, y si no
    // por GET, recordando el respaldo. Lo que se teclea aquí son nombres y
    // correos de personas concretas; en la URL quedan escritos en el log de
    // accesos de cada salto y en el `Referer`, y en el cuerpo no.
    const page = await adminHttp(session).consulta<{
      items: AdminUserWire[];
      nextCursor?: string;
    }>({
      path: '/users',
      query: { cursor: opts.cursor || undefined, limit: opts.limit, q: opts.q || undefined },
    });
    if (!Array.isArray(page?.items)) throw new Error('bad_shape');
    const org = session.user.tenant ?? '';
    let items = page.items.map((w) => toDirectoryUser(w, org));
    // El servicio acota al tenant activo del token; si la vista pide otro, filtrar.
    if (tenant) items = items.filter((u) => u.tenant === tenant);
    return {
      live: true,
      data: { items, nextCursor: typeof page.nextCursor === 'string' ? page.nextCursor : '' },
    };
  } catch {
    // Sin backend vivo la búsqueda se aplica al seed: si no, escribir en la caja
    // de un entorno de demostración devolvería siempre el directorio entero.
    const todos = tenant ? seed.usersForTenant(tenant) : seed.users;
    const q = opts.q?.trim().toLowerCase() ?? '';
    const items = q ? todos.filter((u) => contiene(u.fullName, q) || contiene(u.email, q)) : todos;
    return { live: false, data: { items, nextCursor: '' } };
  }
}

// ---- Grupos y roles (jerarquía org/<slug> con mapeos; modelo de presentación) ----

/**
 * Jerarquía de grupos org/<slug> con sus mapeos a recursos de los servicios Sytue
 * (concepto distinto de los grupos gestionables AdminGroup, ver más abajo). La
 * consume el detalle de organización (TenantDetailPanel). El backend aún no expone
 * esta jerarquía con mapeos, así que degrada al seed de presentación.
 */
export async function loadGroups(
  session: SytueSession,
  tenant?: string,
): Promise<Sourced<Group[]>> {
  try {
    const list = await adminHttp(session).json<Group[]>({
      path: '/groups',
      ...(tenant ? { query: { tenant } } : {}),
    });
    if (!Array.isArray(list)) throw new Error('bad_shape');
    return { live: true, data: list };
  } catch {
    const data = tenant ? seed.groupsForTenant(tenant) : seed.groups;
    return { live: false, data };
  }
}

// ---- Grupos gestionables (directorio admin, contrato /admin/groups) ----

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/groups). */
interface AdminGroupWire {
  id: string;
  name: string;
  description?: string;
  member_count?: number;
  dynamic?: boolean;
  membership_rule?: MembershipRule;
}

/** Miembro devuelto por GET /api/v1/admin/groups/:id. */
interface AdminGroupMemberWire {
  user_id: string;
  email: string;
  name?: string;
}

function toAdminGroup(w: AdminGroupWire): AdminGroup {
  return {
    id: w.id,
    name: w.name || w.id,
    description: w.description ?? '',
    memberCount: typeof w.member_count === 'number' ? w.member_count : 0,
    dynamic: Boolean(w.dynamic),
    ...(w.membership_rule ? { membershipRule: w.membership_rule } : {}),
  };
}

function toAdminGroupMember(w: AdminGroupMemberWire): AdminGroupMember {
  return { userId: w.user_id, email: w.email, name: w.name || w.email };
}

/**
 * Directorio de grupos gestionables del tenant activo (Sytue ID acota por RLS al
 * tenant del token). Degrada al seed si no hay backend vivo.
 */
export async function loadAdminGroups(session: SytueSession): Promise<Sourced<AdminGroup[]>> {
  try {
    const res = await adminHttp(session).json<{ groups?: AdminGroupWire[] }>({ path: '/groups' });
    if (!Array.isArray(res?.groups)) throw new Error('bad_shape');
    return { live: true, data: res.groups.map(toAdminGroup) };
  } catch {
    return { live: false, data: seed.adminGroups };
  }
}

/** Detalle de un grupo con sus miembros (GET /api/v1/admin/groups/:id). */
export async function loadGroupDetail(
  session: SytueSession,
  id: string,
): Promise<{ group: AdminGroup; members: AdminGroupMember[] }> {
  const res = await adminHttp(session).json<{
    group: AdminGroupWire;
    members?: AdminGroupMemberWire[];
  }>({ path: `/groups/${encodeURIComponent(id)}` });
  return {
    group: toAdminGroup(res.group),
    members: Array.isArray(res.members) ? res.members.map(toAdminGroupMember) : [],
  };
}

/** Miembro devuelto por la previsualización de una regla dinámica. */
export interface DynamicPreview {
  count: number;
  members: AdminGroupMember[];
}

/**
 * Coerción mínima de un cuerpo JSON no confiable a MembershipRule para el reenvío del
 * BFF: exige la forma básica (match + conditions[]) y descarta lo demás. La validación
 * REAL (operadores, atributos del catálogo, límites) la aplica Sytue ID (400 propagado).
 */
export function coerceRule(v: unknown): MembershipRule | undefined {
  if (!v || typeof v !== 'object') return undefined;
  const o = v as Record<string, unknown>;
  const match = o.match === 'any' ? 'any' : 'all';
  if (!Array.isArray(o.conditions)) return undefined;
  const conditions = o.conditions
    .filter((c): c is Record<string, unknown> => Boolean(c) && typeof c === 'object')
    .map((c) => ({
      attribute: typeof c.attribute === 'string' ? c.attribute : '',
      op: typeof c.op === 'string' ? c.op : '',
      ...(typeof c.value === 'string' ? { value: c.value } : {}),
      ...(Array.isArray(c.values)
        ? { values: c.values.filter((x): x is string => typeof x === 'string') }
        : {}),
    })) as MembershipRule['conditions'];
  return { match, conditions };
}

/**
 * Crea un grupo en el tenant activo. Con `membership_rule` lo crea DINÁMICO: Sytue ID
 * calcula y materializa su membresía a partir de la regla.
 */
export async function createGroup(
  session: SytueSession,
  input: { name: string; description?: string; membership_rule?: MembershipRule },
): Promise<AdminGroup> {
  return toAdminGroup(await adminHttp(session).postJson<AdminGroupWire>('/groups', input));
}

/**
 * Previsualiza a quién incluiría una regla ANTES de guardarla (no persiste). Sytue ID
 * evalúa la regla contra los usuarios del tenant y devuelve el total + una muestra.
 */
export async function previewDynamicMembers(
  session: SytueSession,
  rule: MembershipRule,
): Promise<DynamicPreview> {
  const res = await adminHttp(session).postJson<{
    count?: number;
    members?: AdminGroupMemberWire[];
  }>('/groups/preview', { membership_rule: rule });
  return {
    count: typeof res.count === 'number' ? res.count : 0,
    members: Array.isArray(res.members) ? res.members.map(toAdminGroupMember) : [],
  };
}

/** Efecto REAL de un recálculo de membresía dinámica (contrato /groups/:id/recompute). */
export interface RecomputeResult {
  added: number;
  removed: number;
  memberCount: number;
}

/**
 * Recalcula la membresía de un grupo DINÁMICO contra el estado actual de los usuarios y
 * devuelve cuántos entraron y cuántos salieron. Repara la deriva (regla creada antes que
 * los usuarios, atributos cambiados por otra vía) sin esperar al barrido periódico de
 * Sytue ID.
 */
export async function recomputeDynamicGroup(
  session: SytueSession,
  id: string,
): Promise<RecomputeResult> {
  const res = await adminHttp(session).postJson<{
    added?: number;
    removed?: number;
    member_count?: number;
  }>(`/groups/${encodeURIComponent(id)}/recompute`, {});
  return {
    added: typeof res.added === 'number' ? res.added : 0,
    removed: typeof res.removed === 'number' ? res.removed : 0,
    memberCount: typeof res.member_count === 'number' ? res.member_count : 0,
  };
}

/**
 * Lo mismo para TODOS los grupos dinámicos de la org de una vez. Tras cambiar
 * atributos de perfil en lote (una reorganización, una importación) la deriva es
 * de la org entera, no de un grupo: ir uno a uno no es una opción.
 */
export async function recomputeAllDynamicGroups(
  session: SytueSession,
): Promise<{ groups: number; added: number; removed: number }> {
  const res = await adminHttp(session).postJson<{
    groups?: number;
    added?: number;
    removed?: number;
  }>('/groups/recompute', {});
  return {
    groups: typeof res.groups === 'number' ? res.groups : 0,
    added: typeof res.added === 'number' ? res.added : 0,
    removed: typeof res.removed === 'number' ? res.removed : 0,
  };
}

/** Edita nombre/descripción de un grupo y, opcionalmente, su regla dinámica (recomputa). */
export async function updateGroup(
  session: SytueSession,
  id: string,
  patch: { name?: string; description?: string; membership_rule?: MembershipRule },
): Promise<AdminGroup> {
  return toAdminGroup(
    await adminHttp(session).json<AdminGroupWire>({
      method: 'PATCH',
      path: `/groups/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  );
}

/** Borra un grupo (Sytue ID aplica authz por tenant; membership cae en cascada). */
export function deleteGroup(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({ method: 'DELETE', path: `/groups/${encodeURIComponent(id)}` });
}

/** Añade un usuario del tenant como miembro del grupo. */
export function addGroupMember(
  session: SytueSession,
  id: string,
  userId: string,
): Promise<unknown> {
  return adminHttp(session).postJson(`/groups/${encodeURIComponent(id)}/members`, {
    user_id: userId,
  });
}

/** Quita a un usuario del grupo. */
export function removeGroupMember(
  session: SytueSession,
  id: string,
  userId: string,
): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/groups/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`,
  });
}

// ---- Auditoría ----

/** Filtros server-side del log de auditoría (los aplica el servicio Go). */
export interface AuditQuery {
  /** Prefijo de acción, p. ej. "auth." o "user.". */
  action?: string | undefined;
  /** UUID del actor. */
  actor?: string | undefined;
  /** Desde (RFC3339 o AAAA-MM-DD, inclusive). */
  since?: string | undefined;
  /** Hasta (RFC3339 o AAAA-MM-DD, inclusive del día). */
  until?: string | undefined;
  /** Cursor keyset opaco de la página anterior. */
  cursor?: string | undefined;
  limit?: number | undefined;
}

/** Página del log; nextCursor '' cuando no hay más. */
export interface AuditPageData {
  items: AuditEvent[];
  nextCursor: string;
}

/** Fila del endpoint admin de auditoría de Sytue ID (GET /api/v1/admin/audit). */
interface AdminAuditWire {
  id: string;
  occurred_at: string;
  actor_id: string;
  actor_email: string;
  action: string;
  target: string;
  ip: string;
  detail?: unknown;
}

// El log de identidad registra hechos; marcamos como "error" las acciones cuyo
// nombre denota un fallo (auth.mfa.failed, binding_mismatch, reuse detectado…)
// para que la columna Resultado sea significativa, sin un campo ok explícito en
// el backend. Ojo: NO incluir "revoked" (revocar sesiones es una acción normal).
const AUDIT_FAILURE = /fail|denied|mismatch|error|reuse|invalid/i;

function toAuditEvent(w: AdminAuditWire, tenant: string): AuditEvent {
  // Campos opcionales solo si tienen valor (exactOptionalPropertyTypes: no
  // asignar undefined explícito a target?/tenant?).
  const ev: AuditEvent = {
    id: w.id,
    source: 'id',
    type: w.action,
    actor: w.actor_email || w.actor_id || 'sistema',
    at: w.occurred_at,
    ok: !AUDIT_FAILURE.test(w.action),
  };
  if (w.target) ev.target = w.target;
  if (tenant) ev.tenant = tenant;
  return ev;
}

/**
 * Página del log de auditoría de la organización activa (Sytue ID), con filtros
 * server-side y paginación keyset. El servicio valida cursor/filtros; aquí solo
 * se reenvían. Degrada al seed (sin cursor) si no hay backend vivo.
 */
export async function loadAudit(
  session: SytueSession,
  q: AuditQuery = {},
): Promise<Sourced<AuditPageData>> {
  try {
    const page = await adminHttp(session).json<{ items: AdminAuditWire[]; nextCursor?: string }>({
      path: '/audit',
      query: {
        action: q.action || undefined,
        actor: q.actor || undefined,
        since: q.since || undefined,
        until: q.until || undefined,
        cursor: q.cursor || undefined,
        limit: q.limit,
      },
    });
    if (!Array.isArray(page?.items)) throw new Error('bad_shape');
    const tenant = session.user.tenant ?? '';
    return {
      live: true,
      data: {
        items: page.items.map((w) => toAuditEvent(w, tenant)),
        nextCursor: typeof page.nextCursor === 'string' ? page.nextCursor : '',
      },
    };
  } catch (e) {
    if (contestoElBackend(e)) return { live: false, data: { items: [], nextCursor: '' } };
    return { live: false, data: { items: seed.auditEvents, nextCursor: '' } };
  }
}

// El export CSV del log se unificó en `siem-audit` (UnifiedAuditPanel): agrega
// las fuentes de todos los servicios, no solo el log de Sytue ID. El helper que
// hablaba con /api/v1/admin/audit/export.csv y su ruta BFF se fueron con él.

// ---- Acciones admin sobre un usuario (ciclo de vida y sesiones) ----

/** Sesión SSO de un usuario tal como la expone Sytue ID al admin. */
export interface AdminSessionWire {
  id: string;
  ip: string;
  user_agent: string;
  device: string;
  amr: string[];
  auth_time: string;
  expires_at: string;
}

/** Suspende o reactiva una cuenta. Suspender revoca además sus sesiones. */
export function setUserStatus(
  session: SytueSession,
  userId: string,
  status: 'suspended' | 'active',
): Promise<{ id: string; status: string; sessions_revoked?: number }> {
  const verb = status === 'suspended' ? 'suspend' : 'reactivate';
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/${verb}`, {});
}

/** Cierre forzado de todas las sesiones del usuario (sin cambiar su estado). */
export function revokeUserSessions(
  session: SytueSession,
  userId: string,
): Promise<{ id: string; sessions_revoked: number }> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/sessions/revoke`, {});
}

/** Qué pasó con UN usuario del lote. `code` es estable y lo traduce la pantalla. */
export interface BulkResult {
  id: string;
  ok: boolean;
  code?: string;
  sessions_revoked?: number;
}

/**
 * La misma acción sobre VARIOS usuarios. Sytue ID responde 200 con el detalle por
 * usuario aunque fallen todos —en un lote de treinta, dos fallan por motivos
 * legítimos y abortar los veintiocho restantes lo dejaría inservible—, así que
 * quien llama mira `applied` contra el largo de `results`, no el status.
 */
export function bulkUsers(
  session: SytueSession,
  action: 'suspend' | 'reactivate' | 'revoke_sessions' | 'set_role',
  userIds: string[],
  role = '',
): Promise<{ results: BulkResult[]; applied: number }> {
  return adminHttp(session).postJson('/users/bulk', {
    action,
    user_ids: userIds,
    ...(role ? { role } : {}),
  });
}

/**
 * Da de alta a alguien en la organización activa. Por defecto va por invitación:
 * Sytue ID emite un token de un solo uso y envía el enlace, y la contraseña la
 * elige quien la va a usar sin que viaje por ningún canal.
 *
 * Con `password` se da de alta DIRECTAMENTE, sin enlace y sin correo. Es la
 * salida cuando el correo no sale: sin ella, un alta depende de un mensaje que
 * puede no llegar nunca y deja a alguien esperando mientras quien la hizo cree
 * que ya está dentro. La respuesta trae `invited` para que la pantalla no diga
 * «le hemos enviado un correo» cuando no se ha enviado ninguno.
 */
export function inviteUser(
  session: SytueSession,
  email: string,
  role: SytueRole,
  mailboxLocal = '',
  password = '',
): Promise<{ email: string; role: string; expires_at?: string; invited?: boolean }> {
  return adminHttp(session).postJson('/invitations', {
    email,
    role,
    mailbox_local: mailboxLocal,
    ...(password ? { password } : {}),
  });
}

/** Una invitación emitida y todavía sin aceptar. */
export interface PendingInvitation {
  email: string;
  role: SytueRole;
  expires_at: string;
}

/**
 * Las invitaciones PENDIENTES de la organización activa.
 *
 * Sin esto la consola sabía emitir invitaciones y no sabía cuáles había emitido:
 * una enviada a la dirección equivocada no se podía ni ver ni quitar, solo
 * esperar a que caducara.
 */
export async function listInvitations(session: SytueSession): Promise<PendingInvitation[]> {
  const out = await adminHttp(session).json<{ items?: PendingInvitation[] }>({
    path: '/invitations',
  });
  return Array.isArray(out?.items) ? out.items : [];
}

/**
 * Revoca la invitación pendiente de un correo. Idempotente en el servicio: si ya
 * no había ninguna devuelve `revoked: false` en vez de fallar, así que reintentar
 * es seguro y la pantalla no tiene que distinguir «no estaba» de «ha fallado».
 */
export function revokeInvitation(
  session: SytueSession,
  email: string,
): Promise<{ revoked: boolean }> {
  return adminHttp(session).postJson('/invitations:revoke', { email });
}

// ---- Direcciones de correo de la organización ----

/** Una dirección que entrega en el buzón de alguien: la principal o un alias. */
export interface OrgMailbox {
  id: string;
  user_id: string;
  address: string;
  primary: boolean;
}

/**
 * Direcciones de la organización activa (todas, o las de una persona con userId).
 * `mail_domain` es el dominio en el que se reparten: lo devuelve Sytue ID para que
 * la consola no tenga que repetir la regla del dominio propio verificado.
 */
export function listMailboxes(
  session: SytueSession,
  userId = '',
): Promise<{ mailboxes: OrgMailbox[]; mail_domain: string }> {
  const q = userId ? `?user_id=${encodeURIComponent(userId)}` : '';
  return adminHttp(session).json({ path: `/mailboxes${q}` });
}

/**
 * Cambia la dirección principal de un miembro. La anterior NO se pierde: Sytue ID
 * la conserva como alias para que el correo que ya iba a ella siga entregándose.
 */
export function setUserMailbox(
  session: SytueSession,
  userId: string,
  address: string,
): Promise<{ address: string }> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/mailbox`, { address });
}

/** Añade una dirección más que entrega en el buzón de esa persona. */
export function addUserAlias(
  session: SytueSession,
  userId: string,
  address: string,
): Promise<OrgMailbox> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/aliases`, { address });
}

/** Quita un alias. Sytue ID se niega a borrar una dirección principal. */
export async function deleteMailboxAlias(session: SytueSession, id: string): Promise<void> {
  await adminHttp(session).request({
    method: 'DELETE',
    path: `/mailboxes/${encodeURIComponent(id)}`,
  });
}

/**
 * Cambia el rol del usuario en la org activa. Una degradación revoca sus sesiones.
 * Sytue ID aplica el guard del último admin y la autorización por tenant.
 */
export function setMemberRole(
  session: SytueSession,
  userId: string,
  role: SytueRole,
): Promise<{ id: string; role: string; sessions_revoked?: number }> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/role`, { role });
}

/**
 * Departamento, puesto y ubicación de un miembro: los tres atributos con los que
 * se escriben las reglas de los grupos dinámicos.
 *
 * Sytue ID los reevalúa en la MISMA transacción, así que quien cumpla ahora la
 * regla de «Ventas» entra en el grupo al guardar, sin recalcular nada a mano.
 * Sin esta llamada las reglas por departamento/puesto/ubicación no podían casar
 * con nadie: no existía ninguna pantalla —ni aquí ni en el resto de la suite—
 * capaz de escribir esos campos, y SCIM, que es la otra vía, está apagado.
 */
export function setUserAttributes(
  session: SytueSession,
  userId: string,
  attrs: { department: string; title: string; location: string },
): Promise<{ id: string; department: string; title: string; location: string }> {
  return adminHttp(session).json({
    method: 'PATCH',
    path: `/users/${encodeURIComponent(userId)}/profile`,
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(attrs),
  });
}

/**
 * Restablece el MFA del usuario (limpia TOTP + WebAuthn) para que re-enrole.
 * Destructivo: hasta que vuelva a configurar 2FA su cuenta queda sin segundo
 * factor. Sytue ID valida que el objetivo sea del mismo tenant (404 sin fuga).
 */
export function resetUserMfa(session: SytueSession, userId: string): Promise<{ ok: boolean }> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/reset-mfa`, {});
}

/** Quita al usuario de la org activa y revoca sus sesiones. */
export function removeMember(
  session: SytueSession,
  userId: string,
): Promise<{ id: string; sessions_revoked: number }> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/users/${encodeURIComponent(userId)}/membership`,
  });
}

/**
 * Dispara el reseteo de contraseña por email: el usuario recibe un enlace para
 * elegir su nueva contraseña (el admin no la ve ni la fija). Alternativa a
 * resetUserPassword (fijar una contraseña dada).
 */
export function sendPasswordResetLink(
  session: SytueSession,
  userId: string,
): Promise<{ id: string; sent: boolean }> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/reset-password`, {});
}

/**
 * Reset de contraseña por admin: Sytue ID fija la contraseña dada y revoca las
 * sesiones del usuario. La contraseña viaja solo servidor→servidor (BFF con el
 * access token httpOnly); nunca se registra.
 */
export function resetUserPassword(
  session: SytueSession,
  userId: string,
  newPassword: string,
): Promise<{ id: string; sessions_revoked: number }> {
  return adminHttp(session).postJson(`/users/${encodeURIComponent(userId)}/password`, {
    new_password: newPassword,
  });
}

/** Sesiones SSO vigentes de un usuario (visibilidad de dispositivos). */
export async function loadUserSessions(
  session: SytueSession,
  userId: string,
): Promise<AdminSessionWire[]> {
  const res = await adminHttp(session).json<{ sessions?: AdminSessionWire[] }>({
    path: `/users/${encodeURIComponent(userId)}/sessions`,
  });
  return Array.isArray(res?.sessions) ? res.sessions : [];
}

/**
 * Qué pasa de verdad si se toca a este usuario. Lo pide la ficha ANTES de
 * enseñar una confirmación: «¿seguro?» a secas no dice nada que quien lo pulsa
 * no supiera ya; lo que hace dudar a tiempo es «se cerrarán 4 sesiones» o «es el
 * único propietario», y esos números solo los tiene el servidor.
 */
export interface UserImpact {
  active_sessions: number;
  groups: number;
  last_owner: boolean;
}

export async function loadUserImpact(session: SytueSession, userId: string): Promise<UserImpact> {
  const res = await adminHttp(session).json<Partial<UserImpact>>({
    path: `/users/${encodeURIComponent(userId)}/impact`,
  });
  return {
    active_sessions: Number(res?.active_sessions ?? 0),
    groups: Number(res?.groups ?? 0),
    last_owner: res?.last_owner === true,
  };
}

/**
 * Qué hay que mirar HOY en esta organización. La portada enseñaba recuentos
 * totales y las últimas filas de auditoría: informan de que la organización
 * existe, no de que haya algo que hacer.
 *
 * Los calcula el servidor sobre la organización entera. Deducirlos de la primera
 * página del directorio daría un «3 sin MFA» que en veinte mil personas es una
 * respuesta inventada.
 */
export interface OrgInsights {
  without_mfa: number;
  suspended: number;
  unverified_email: number;
  invitations_expired: number;
  invitations_pending: number;
}

const SIN_INSIGHTS: OrgInsights = {
  without_mfa: 0,
  suspended: 0,
  unverified_email: 0,
  invitations_expired: 0,
  invitations_pending: 0,
};

/**
 * Nunca lanza: la portada tiene que pintarse aunque esto falle. Con ceros no se
 * enseña ningún aviso, que es lo correcto — inventar uno sería peor que callar.
 */
export async function loadInsights(session: SytueSession): Promise<OrgInsights> {
  try {
    const res = await adminHttp(session).json<Partial<OrgInsights>>({ path: '/insights' });
    return { ...SIN_INSIGHTS, ...res };
  } catch {
    return SIN_INSIGHTS;
  }
}

/**
 * Destino SIEM de la auditoría. El servicio empuja los eventos a Splunk/Sentinel/
 * Elastic; esto es su configuración.
 *
 * Los secretos NO viajan de vuelta: solo si HAY uno guardado. Un token del SIEM
 * que se puede releer desde el navegador es un token que se filtra por la
 * primera captura de pantalla.
 */
export interface AuditSink {
  url: string;
  has_auth_header: boolean;
  has_signing_secret: boolean;
  enabled: boolean;
  last_sent_id: number;
  last_error: string;
  last_sent_at?: string;
}

/** null = no hay destino configurado (404 del servicio), que no es un error. */
export async function loadAuditSink(session: SytueSession): Promise<AuditSink | null> {
  try {
    return await adminHttp(session).json<AuditSink>({ path: '/audit/sink' });
  } catch {
    return null;
  }
}

/**
 * Guarda el destino. Los secretos vacíos CONSERVAN los que hubiera: el admin no
 * puede releerlos, así que exigirlos en cada edición obligaría a rotar el token
 * del SIEM para cambiar una coma de la URL.
 */
export async function saveAuditSink(
  session: SytueSession,
  body: { url: string; auth_header?: string; signing_secret?: string; enabled?: boolean },
): Promise<AuditSink> {
  return adminHttp(session).json<AuditSink>({
    path: '/audit/sink',
    method: 'PUT',
    body: JSON.stringify(body),
  });
}

export async function deleteAuditSink(session: SytueSession): Promise<void> {
  await adminHttp(session).json({ path: '/audit/sink', method: 'DELETE' });
}

/**
 * Token de acceso inicial del registro dinámico (RFC 7591). Sirve para que una
 * aplicación se registre SOLA —un despliegue automatizado, un cliente que se
 * instala— en vez de que alguien rellene el formulario a mano.
 *
 * Vuelve UNA vez: Sytue ID guarda solo su hash. Quien lo reciba tiene que
 * copiarlo en ese momento, y por eso la pantalla lo dice antes.
 */
export interface DCRToken {
  initial_access_token: string;
  registration_endpoint: string;
  expires_at: string;
}

export async function createDCRToken(session: SytueSession): Promise<DCRToken> {
  return adminHttp(session).json<DCRToken>({
    path: '/apps/dcr-tokens',
    method: 'POST',
    body: '{}',
  });
}

// ---- Política de contraseñas de la organización ----

function toPolicy(w: Partial<PasswordPolicy> | null | undefined): PasswordPolicy {
  return {
    min_length:
      typeof w?.min_length === 'number' ? w.min_length : DEFAULT_PASSWORD_POLICY.min_length,
    require_complexity: Boolean(w?.require_complexity),
  };
}

/**
 * Política de contraseñas de la organización activa (Sytue ID acota por el
 * tenant del token). Degrada al default de la migración si no hay backend vivo.
 */
export async function loadPasswordPolicy(session: SytueSession): Promise<Sourced<PasswordPolicy>> {
  try {
    const w = await adminHttp(session).json<Partial<PasswordPolicy>>({
      path: '/org/password-policy',
    });
    return { live: true, data: toPolicy(w) };
  } catch {
    return { live: false, data: DEFAULT_PASSWORD_POLICY };
  }
}

/** Actualiza la política de contraseñas de la org activa (admin/owner). */
export async function updatePasswordPolicy(
  session: SytueSession,
  patch: Partial<PasswordPolicy>,
): Promise<PasswordPolicy> {
  const w = await adminHttp(session).json<Partial<PasswordPolicy>>({
    method: 'PATCH',
    path: '/org/password-policy',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(patch),
  });
  return toPolicy(w);
}

// ---- Dominio de correo de la organización ----

/**
 * En qué dominio vive el correo de la organización.
 *
 * Son DOS y no uno: `base_domain` es el subdominio de la suite —`<slug>.sytue.eu`—,
 * que existe siempre y del que cuelga el cableado interno; `mail_domain` es el que
 * se usa hoy para las direcciones nuevas, que coincide con el base mientras no haya
 * un dominio propio verificado. Conectar el propio NO apaga el base.
 */
export interface OrgDomainState {
  mail_domain: string;
  base_domain: string;
  /** null si aún no se ha reclamado ningún dominio propio. */
  domain: OrgOwnDomain | null;
}

export interface OrgOwnDomain {
  host: string;
  /** Nombre y valor EXACTOS del registro TXT que hay que crear. */
  record_name: string;
  record_value: string;
  verified: boolean;
  verified_at?: string;
  /** verified | pending | mismatch | unreachable — solo mientras no esté verificado. */
  status?: string;
  /** Lo que hay publicado hoy en ese nombre, cuando no coincide con lo nuestro. */
  found?: string[];
}

// ---- LOS dominios de correo, en plural (mig 0052) ----
//
// Una organización puede tener varios: dos marcas, o el .es y el .com de la
// misma. Antes cabía uno y conectar el segundo significaba desconectar el
// primero, con las direcciones que ya usaba su plantilla dentro.

/** Un dominio de la organización. `primary` es el que estrenan las altas nuevas. */
export interface OrgMailDomain extends OrgOwnDomain {
  id: string;
  primary: boolean;
}

export interface OrgMailDomainsState {
  items: OrgMailDomain[];
  /** El dominio en el que nacen hoy las direcciones nuevas. */
  mail_domain: string;
  /** El subdominio de la suite, que existe siempre y no se puede quitar. */
  base_domain: string;
}

export async function loadOrgMailDomains(
  session: SytueSession,
): Promise<Sourced<OrgMailDomainsState | null>> {
  try {
    return {
      live: true,
      data: await adminHttp(session).json<OrgMailDomainsState>({ path: '/mail-domains' }),
    };
  } catch {
    // Sin backend vivo NO se inventa nada: un registro TXT falso lleva a alguien a
    // publicar en su zona un valor que no sirve para verificar nada.
    return { live: false, data: null };
  }
}

/** Añade el dominio y lo comprueba. El mismo endpoint sirve para recomprobarlo. */
export function claimOrgMailDomain(session: SytueSession, host: string): Promise<OrgMailDomain> {
  return adminHttp(session).json<OrgMailDomain>({
    method: 'POST',
    path: '/mail-domains',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ domain: host }),
  });
}

/** Elige en qué dominio nacen las direcciones nuevas; muda a toda la plantilla. */
export function setPrimaryOrgMailDomain(session: SytueSession, id: string): Promise<OrgMailDomain> {
  return adminHttp(session).json<OrgMailDomain>({
    method: 'POST',
    path: `/mail-domains/${encodeURIComponent(id)}/primary`,
  });
}

/** Desconecta un dominio. Las direcciones que ya existen en él no se tocan. */
export function deleteOrgMailDomain(session: SytueSession, id: string): Promise<void> {
  return adminHttp(session).json<void>({
    method: 'DELETE',
    path: `/mail-domains/${encodeURIComponent(id)}`,
  });
}

// ---- Verificación del propio email (self-service) ----

/**
 * Envía (o reenvía) el enlace de verificación a la dirección de la cuenta del
 * usuario autenticado. Sytue ID emite un token de un solo uso y lo manda por
 * email; si ya está verificado responde {sent:false}. Rate-limitado en el servicio.
 */
export function resendEmailVerification(
  session: SytueSession,
): Promise<{ verified: boolean; sent: boolean }> {
  return meHttp(session).postJson('/me/email/verify/send', {});
}

/** Perfil propio devuelto por Sytue ID. ponytail: solo el display name es editable. */
export interface MeProfile {
  name: string;
}

/**
 * Actualiza el perfil del propio usuario (solo display name). Sytue ID valida y
 * sanea el nombre y actualiza SOLO al usuario del token (email/rol/tenant fuera).
 */
export async function updateProfile(session: SytueSession, name: string): Promise<MeProfile> {
  const w = await meHttp(session).json<Partial<MeProfile>>({
    method: 'PATCH',
    path: '/me',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  return { name: typeof w.name === 'string' ? w.name : name };
}

// ---- SSO / federación OIDC upstream ----

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/sso/providers). */
interface SsoProviderWire {
  id: string;
  type?: string;
  name: string;
  slug: string;
  issuer: string;
  client_id: string;
  has_secret: boolean;
  scopes: string;
  attribute_mapping?: Record<string, string> | null;
  allowed_email_domains?: string[] | null;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint: string;
  jwks_uri: string;
  pkce: boolean;
  enabled: boolean;
  configured: boolean;
  redirect_uri: string;
  created_at: string;
  updated_at: string;
  // SAML
  idp_entity_id?: string;
  idp_sso_url?: string;
  idp_certificate?: string;
  sp_entity_id?: string;
  saml_binding?: string;
  name_id_format?: string;
  acs_url?: string;
  metadata_url?: string;
}

function toSsoProvider(w: SsoProviderWire): SsoProvider {
  return {
    id: w.id,
    type: w.type === 'saml' ? 'saml' : 'oidc',
    name: w.name,
    slug: w.slug,
    issuer: w.issuer,
    clientId: w.client_id,
    hasSecret: Boolean(w.has_secret),
    scopes: w.scopes || 'openid email profile',
    attributeMapping: w.attribute_mapping ?? {},
    allowedEmailDomains: Array.isArray(w.allowed_email_domains) ? w.allowed_email_domains : [],
    authorizationEndpoint: w.authorization_endpoint || '',
    tokenEndpoint: w.token_endpoint || '',
    userinfoEndpoint: w.userinfo_endpoint || '',
    jwksUri: w.jwks_uri || '',
    pkce: Boolean(w.pkce),
    enabled: Boolean(w.enabled),
    configured: Boolean(w.configured),
    redirectUri: w.redirect_uri || '',
    createdAt: w.created_at,
    updatedAt: w.updated_at,
    idpEntityId: w.idp_entity_id || '',
    idpSsoUrl: w.idp_sso_url || '',
    idpCertificate: w.idp_certificate || '',
    spEntityId: w.sp_entity_id || '',
    samlBinding: w.saml_binding === 'post' ? 'post' : 'redirect',
    nameIdFormat: w.name_id_format || '',
    acsUrl: w.acs_url || '',
    metadataUrl: w.metadata_url || '',
  };
}

/**
 * Proveedores SSO de la organización activa (Sytue ID acota por el tenant del
 * token). El secreto nunca viaja. Degrada a lista vacía si no hay backend vivo.
 */
export async function loadSsoProviders(session: SytueSession): Promise<Sourced<SsoProvider[]>> {
  try {
    const res = await adminHttp(session).json<{ items?: SsoProviderWire[] }>({
      path: '/sso/providers',
    });
    if (!Array.isArray(res?.items)) throw new Error('bad_shape');
    return { live: true, data: res.items.map(toSsoProvider) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Crea un proveedor SSO; Sytue ID resuelve el discovery del issuer (SSRF-safe). */
export async function createSsoProvider(
  session: SytueSession,
  input: SsoProviderInput,
): Promise<SsoProvider> {
  return toSsoProvider(await adminHttp(session).postJson<SsoProviderWire>('/sso/providers', input));
}

/** Edita un proveedor SSO (el secreto solo se re-escribe si viene en el payload). */
export async function updateSsoProvider(
  session: SytueSession,
  id: string,
  patch: Partial<SsoProviderInput>,
): Promise<SsoProvider> {
  return toSsoProvider(
    await adminHttp(session).json<SsoProviderWire>({
      method: 'PATCH',
      path: `/sso/providers/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  );
}

/** Re-ejecuta el discovery del issuer y actualiza los endpoints cacheados. */
export async function testSsoProvider(
  session: SytueSession,
  id: string,
): Promise<{ ok: boolean; pkce: boolean; provider: SsoProvider }> {
  const res = await adminHttp(session).postJson<{
    ok: boolean;
    pkce: boolean;
    provider: SsoProviderWire;
  }>(`/sso/providers/${encodeURIComponent(id)}/test`, {});
  return { ok: Boolean(res.ok), pkce: Boolean(res.pkce), provider: toSsoProvider(res.provider) };
}

/** Borra un proveedor SSO (cascada: enlaces federados y estados en curso). */
export function deleteSsoProvider(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/sso/providers/${encodeURIComponent(id)}`,
  });
}

// ---- Aplicaciones que entran CON Sytue (Sytue ID como proveedor OIDC) ----

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/apps/oidc-clients). */
interface OidcAppWire {
  client_id: string;
  name: string;
  type?: string;
  redirect_uris?: string[] | null;
  post_logout_redirect_uris?: string[] | null;
  scopes?: string[] | null;
  has_secret?: boolean;
  created_at: string;
  client_secret?: string;
  issuer: string;
  discovery_url: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint: string;
  jwks_uri: string;
  end_session_endpoint: string;
}

function toOidcApp(w: OidcAppWire): OidcApp {
  return {
    clientId: w.client_id,
    name: w.name,
    type: w.type === 'public' ? 'public' : 'confidential',
    redirectUris: Array.isArray(w.redirect_uris) ? w.redirect_uris : [],
    postLogoutRedirectUris: Array.isArray(w.post_logout_redirect_uris)
      ? w.post_logout_redirect_uris
      : [],
    scopes: Array.isArray(w.scopes) ? w.scopes : [],
    hasSecret: Boolean(w.has_secret),
    createdAt: w.created_at,
    ...(w.client_secret ? { secret: w.client_secret } : {}),
    issuer: w.issuer || '',
    discoveryUrl: w.discovery_url || '',
    authorizationEndpoint: w.authorization_endpoint || '',
    tokenEndpoint: w.token_endpoint || '',
    userinfoEndpoint: w.userinfo_endpoint || '',
    jwksUri: w.jwks_uri || '',
    endSessionEndpoint: w.end_session_endpoint || '',
  };
}

/** Aplicaciones OIDC de la organización activa. Nunca traen el secreto. */
export async function loadOidcApps(session: SytueSession): Promise<Sourced<OidcApp[]>> {
  try {
    const res = await adminHttp(session).json<{ items?: OidcAppWire[] }>({
      path: '/apps/oidc-clients',
    });
    if (!Array.isArray(res?.items)) throw new Error('bad_shape');
    return { live: true, data: res.items.map(toOidcApp) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Registra una aplicación; la respuesta trae el secreto UNA sola vez. */
export async function createOidcApp(session: SytueSession, input: OidcAppInput): Promise<OidcApp> {
  return toOidcApp(await adminHttp(session).postJson<OidcAppWire>('/apps/oidc-clients', input));
}

/** Rota el secreto: devuelve el nuevo (única vez) e invalida el anterior. */
export async function rotateOidcAppSecret(
  session: SytueSession,
  clientId: string,
): Promise<OidcApp> {
  return toOidcApp(
    await adminHttp(session).postJson<OidcAppWire>(
      `/apps/oidc-clients/${encodeURIComponent(clientId)}/rotate-secret`,
      {},
    ),
  );
}

// ---- Proveedores de servicio SAML (Sytue ID hace de IdP para una app de terceros) ----

/** Fila de GET /api/v1/admin/apps/saml-sps. */
interface SamlSpWire {
  id: string;
  name: string;
  entity_id: string;
  acs_url: string;
  name_id_format?: string;
  idp_initiated?: boolean;
  enabled?: boolean;
  configured?: boolean;
  created_at: string;
  idp_entity_id?: string;
  idp_metadata_url?: string;
  idp_sso_url?: string;
  idp_slo_url?: string;
  initiate_url?: string;
}

function toSamlSp(w: SamlSpWire): SamlSp {
  return {
    id: w.id,
    name: w.name,
    entityId: w.entity_id,
    acsUrl: w.acs_url,
    nameIdFormat: w.name_id_format ?? '',
    idpInitiated: Boolean(w.idp_initiated),
    enabled: Boolean(w.enabled),
    configured: Boolean(w.configured),
    createdAt: w.created_at,
    idpEntityId: w.idp_entity_id ?? '',
    idpMetadataUrl: w.idp_metadata_url ?? '',
    idpSsoUrl: w.idp_sso_url ?? '',
    idpSloUrl: w.idp_slo_url ?? '',
    initiateUrl: w.initiate_url ?? '',
  };
}

/**
 * SP SAML de la organización activa. Los crea `integrations` al activar una plantilla
 * del catálogo, así que hasta ahora existían sin que nadie pudiera verlos.
 */
export async function loadSamlSps(session: SytueSession): Promise<Sourced<SamlSp[]>> {
  try {
    const res = await adminHttp(session).json<{ items?: SamlSpWire[] }>({
      path: '/apps/saml-sps',
    });
    if (!Array.isArray(res?.items)) throw new Error('bad_shape');
    return { live: true, data: res.items.map(toSamlSp) };
  } catch {
    return { live: false, data: [] };
  }
}

/**
 * Estado del aprovisionamiento SCIM de esta organización, MEDIDO.
 *
 * Aquí hubo un KPI «Provisioning: OK» y un distintivo «SCIM activo» escritos en
 * el JSX, que anunciaban que el aprovisionamiento —y sobre todo el
 * DESaprovisionamiento— del IdP corporativo funcionaba mientras nadie revocaba
 * ninguna cuenta al salir de la empresa. Los dos se retiraron. Esto los
 * sustituye con lo que Sytue ID dice de verdad, y `live:false` cuando no
 * contesta: no saberlo se pinta como no saberlo.
 */
export interface ScimStatus {
  configured: boolean;
  baseUrl: string;
  usersProvisioned: number;
  lastEventAt: string;
  tokens: ScimToken[];
}

/** Un token de aprovisionamiento vivo. El secreto NO está aquí: solo se ve al crearlo. */
export interface ScimToken {
  id: string;
  label: string;
  created_at: string;
  last_used_at: string | null;
}

/**
 * Crea el token de aprovisionamiento de esta organización. El secreto vuelve UNA
 * vez y no se puede releer: quien lo pierde crea otro y revoca el anterior.
 */
export async function createScimToken(
  session: SytueSession,
  label: string,
): Promise<{ id: string; token: string; base_url: string }> {
  return adminHttp(session).json<{ id: string; token: string; base_url: string }>({
    method: 'POST',
    path: '/scim/tokens',
    body: JSON.stringify({ label }),
  });
}

/** Corta un token: el aprovisionamiento deja de funcionar en el acto. */
export async function revokeScimToken(session: SytueSession, id: string): Promise<void> {
  await adminHttp(session).json<unknown>({
    method: 'DELETE',
    path: `/scim/tokens/${encodeURIComponent(id)}`,
  });
}

export async function loadScimStatus(session: SytueSession): Promise<Sourced<ScimStatus | null>> {
  try {
    const res = await adminHttp(session).json<{
      configured?: boolean;
      base_url?: string;
      users_provisioned?: number;
      last_event_at?: string;
      tokens?: ScimToken[] | null;
    }>({ path: '/scim/status' });
    if (typeof res?.configured !== 'boolean') throw new Error('bad_shape');
    return {
      live: true,
      data: {
        configured: res.configured,
        baseUrl: typeof res.base_url === 'string' ? res.base_url : '',
        usersProvisioned: typeof res.users_provisioned === 'number' ? res.users_provisioned : 0,
        lastEventAt: typeof res.last_event_at === 'string' ? res.last_event_at : '',
        tokens: Array.isArray(res.tokens) ? res.tokens : [],
      },
    };
  } catch {
    return { live: false, data: null };
  }
}

/** Enciende/apaga la federación con ese SP sin borrar su configuración. */
export async function setSamlSpEnabled(
  session: SytueSession,
  id: string,
  enabled: boolean,
): Promise<SamlSp> {
  return toSamlSp(
    await adminHttp(session).json<SamlSpWire>({
      method: 'PATCH',
      path: `/apps/saml-sps/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ enabled }),
    }),
  );
}

/** Borra el SP: la app de terceros deja de poder federar con esta organización. */
export function deleteSamlSp(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/apps/saml-sps/${encodeURIComponent(id)}`,
  });
}

/** Borra la aplicación: deja de poder iniciar sesión al instante. */
export function deleteOidcApp(session: SytueSession, clientId: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/apps/oidc-clients/${encodeURIComponent(clientId)}`,
  });
}

// ---- Acceso Condicional ----

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/access-policies). */
interface AccessPolicyWire {
  id: string;
  name: string;
  enabled: boolean;
  priority: number;
  action: string;
  include_users?: string[] | null;
  include_groups?: string[] | null;
  exclude_users?: string[] | null;
  exclude_groups?: string[] | null;
  ip_ranges?: string[] | null;
  ip_negate?: boolean;
  client_ids?: string[] | null;
  require_trusted_device?: boolean;
  days_mask?: number;
  start_minute?: number;
  end_minute?: number;
  created_at: string;
  updated_at: string;
}

function strArr(v: string[] | null | undefined): string[] {
  return Array.isArray(v) ? v : [];
}

function toAccessPolicy(w: AccessPolicyWire): AccessPolicy {
  const action =
    w.action === 'allow' ? 'allow' : w.action === 'require_mfa' ? 'require_mfa' : 'block';
  return {
    id: w.id,
    name: w.name,
    enabled: Boolean(w.enabled),
    priority: typeof w.priority === 'number' ? w.priority : 100,
    action,
    includeUsers: strArr(w.include_users),
    includeGroups: strArr(w.include_groups),
    excludeUsers: strArr(w.exclude_users),
    excludeGroups: strArr(w.exclude_groups),
    ipRanges: strArr(w.ip_ranges),
    ipNegate: Boolean(w.ip_negate),
    clientIds: strArr(w.client_ids),
    requireTrustedDevice: Boolean(w.require_trusted_device),
    daysMask: typeof w.days_mask === 'number' ? w.days_mask : 0,
    startMinute: typeof w.start_minute === 'number' ? w.start_minute : -1,
    endMinute: typeof w.end_minute === 'number' ? w.end_minute : -1,
    createdAt: w.created_at,
    updatedAt: w.updated_at,
  };
}

/**
 * Políticas de Acceso Condicional de la organización activa (Sytue ID acota por el
 * tenant del token). Degrada a lista vacía si no hay backend vivo.
 */
export async function loadAccessPolicies(session: SytueSession): Promise<Sourced<AccessPolicy[]>> {
  try {
    const res = await adminHttp(session).json<{ items?: AccessPolicyWire[] }>({
      path: '/access-policies',
    });
    if (!Array.isArray(res?.items)) throw new Error('bad_shape');
    return { live: true, data: res.items.map(toAccessPolicy) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Crea una política de Acceso Condicional (name y action obligatorios). */
export async function createAccessPolicy(
  session: SytueSession,
  input: AccessPolicyInput,
): Promise<AccessPolicy> {
  return toAccessPolicy(
    await adminHttp(session).postJson<AccessPolicyWire>('/access-policies', input),
  );
}

/** Edita una política de Acceso Condicional (solo los campos presentes). */
export async function updateAccessPolicy(
  session: SytueSession,
  id: string,
  patch: Partial<AccessPolicyInput>,
): Promise<AccessPolicy> {
  return toAccessPolicy(
    await adminHttp(session).json<AccessPolicyWire>({
      method: 'PATCH',
      path: `/access-policies/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  );
}

/** Borra una política de Acceso Condicional (aislada por tenant en Sytue ID). */
export function deleteAccessPolicy(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/access-policies/${encodeURIComponent(id)}`,
  });
}

const ACCESS_ACTIONS = new Set<AccessAction>(['allow', 'require_mfa', 'block']);
const ACCESS_STR_LISTS = [
  'include_users',
  'include_groups',
  'exclude_users',
  'exclude_groups',
  'ip_ranges',
  'client_ids',
] as const;

/**
 * Sanea el cuerpo de create/patch de una política (espejo mínimo para el UX; la
 * validación real —UUIDs, CIDRs, aislamiento por org— la hace Sytue ID). Devuelve solo
 * los campos presentes con el tipo correcto: los ausentes NO se tocan (semántica PATCH).
 */
export function sanitizeAccessPolicyInput(body: unknown): Partial<AccessPolicyInput> {
  const b = (body ?? {}) as Record<string, unknown>;
  const out: Partial<AccessPolicyInput> = {};
  if (typeof b.name === 'string') out.name = b.name.trim();
  if (typeof b.action === 'string' && ACCESS_ACTIONS.has(b.action as AccessAction)) {
    out.action = b.action as AccessAction;
  }
  if (typeof b.enabled === 'boolean') out.enabled = b.enabled;
  if (typeof b.priority === 'number' && Number.isFinite(b.priority)) {
    out.priority = Math.trunc(b.priority);
  }
  for (const k of ACCESS_STR_LISTS) {
    if (Array.isArray(b[k])) {
      (out as Record<string, string[]>)[k] = (b[k] as unknown[])
        .map((v) => String(v).trim())
        .filter(Boolean);
    }
  }
  if (typeof b.ip_negate === 'boolean') out.ip_negate = b.ip_negate;
  if (typeof b.require_trusted_device === 'boolean') {
    out.require_trusted_device = b.require_trusted_device;
  }
  if (typeof b.days_mask === 'number' && Number.isFinite(b.days_mask)) {
    out.days_mask = Math.trunc(b.days_mask) & 0x7f;
  }
  if (typeof b.start_minute === 'number' && Number.isFinite(b.start_minute)) {
    out.start_minute = Math.trunc(b.start_minute);
  }
  if (typeof b.end_minute === 'number' && Number.isFinite(b.end_minute)) {
    out.end_minute = Math.trunc(b.end_minute);
  }
  return out;
}

// ---- Etiquetas de sensibilidad (paridad Purview / Google Drive labels) ----

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/sensitivity-labels). */
interface SensitivityLabelWire {
  id: string;
  name: string;
  rank?: number;
  color?: string;
  description?: string;
  block_external_share?: boolean;
  created_at: string;
  updated_at: string;
}

function toSensitivityLabel(w: SensitivityLabelWire): SensitivityLabel {
  return {
    id: w.id,
    name: w.name,
    rank: typeof w.rank === 'number' ? w.rank : 0,
    color: typeof w.color === 'string' ? w.color : '',
    description: typeof w.description === 'string' ? w.description : '',
    blockExternalShare: Boolean(w.block_external_share),
    createdAt: w.created_at,
    updatedAt: w.updated_at,
  };
}

/**
 * Taxonomía de etiquetas de la organización activa, de menos a más sensible (el orden
 * lo da Sytue ID). Degrada a lista vacía si no hay backend vivo.
 */
export async function loadSensitivityLabels(
  session: SytueSession,
): Promise<Sourced<SensitivityLabel[]>> {
  try {
    const res = await adminHttp(session).json<{ items?: SensitivityLabelWire[] }>({
      path: '/sensitivity-labels',
    });
    if (!Array.isArray(res?.items)) throw new Error('bad_shape');
    return { live: true, data: res.items.map(toSensitivityLabel) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Crea una etiqueta de sensibilidad (name obligatorio). */
export async function createSensitivityLabel(
  session: SytueSession,
  input: SensitivityLabelInput,
): Promise<SensitivityLabel> {
  return toSensitivityLabel(
    await adminHttp(session).postJson<SensitivityLabelWire>('/sensitivity-labels', input),
  );
}

/** Edita una etiqueta (solo los campos presentes). NO reescribe lo ya etiquetado. */
export async function updateSensitivityLabel(
  session: SytueSession,
  id: string,
  patch: Partial<SensitivityLabelInput>,
): Promise<SensitivityLabel> {
  return toSensitivityLabel(
    await adminHttp(session).json<SensitivityLabelWire>({
      method: 'PATCH',
      path: `/sensitivity-labels/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  );
}

/** Borra una etiqueta de la taxonomía (aislada por org en Sytue ID). */
export function deleteSensitivityLabel(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/sensitivity-labels/${encodeURIComponent(id)}`,
  });
}

/**
 * Sanea el cuerpo de create/patch de una etiqueta (espejo mínimo para el UX; la
 * validación real la hace Sytue ID). Solo los campos presentes viajan (semántica PATCH).
 */
export function sanitizeSensitivityLabelInput(body: unknown): Partial<SensitivityLabelInput> {
  const b = (body ?? {}) as Record<string, unknown>;
  const out: Partial<SensitivityLabelInput> = {};
  if (typeof b.name === 'string') out.name = b.name.trim();
  if (typeof b.description === 'string') out.description = b.description.trim();
  if (typeof b.color === 'string') out.color = b.color.trim().toLowerCase();
  if (typeof b.rank === 'number' && Number.isFinite(b.rank)) out.rank = Math.trunc(b.rank);
  if (typeof b.block_external_share === 'boolean') {
    out.block_external_share = b.block_external_share;
  }
  return out;
}

// ---- Dispositivos de confianza (paridad Entra device-based CA) ----

/** Fila del endpoint de dispositivos de Sytue ID (self-service y admin). */
interface DeviceWire {
  id: string;
  name: string;
  user_agent: string;
  trusted: boolean;
  first_seen: string;
  last_seen: string;
  last_ip: string;
  current?: boolean;
  // Solo en la vista admin (dueño del dispositivo).
  user_id?: string;
  user_email?: string;
  user_name?: string;
}

function toDevice(w: DeviceWire): Device {
  const d: Device = {
    id: w.id,
    name: w.name || 'Dispositivo',
    userAgent: w.user_agent || '',
    trusted: Boolean(w.trusted),
    firstSeen: w.first_seen,
    lastSeen: w.last_seen,
    lastIp: w.last_ip || '',
    current: Boolean(w.current),
  };
  // exactOptionalPropertyTypes: solo asignar las opcionales cuando llegan con valor.
  if (w.user_id) d.userId = w.user_id;
  if (w.user_email) d.userEmail = w.user_email;
  if (w.user_name) d.userName = w.user_name;
  return d;
}

/** Mis dispositivos (self-service), con el actual marcado si se reenvía la cookie. */
export async function loadMyDevices(
  session: SytueSession,
  deviceCookie: Record<string, string> = {},
): Promise<Sourced<Device[]>> {
  try {
    const res = await meHttp(session).json<{ devices?: DeviceWire[] }>({
      path: '/me/devices',
      headers: deviceCookie,
    });
    if (!Array.isArray(res?.devices)) throw new Error('bad_shape');
    return { live: true, data: res.devices.map(toDevice) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Marca de confianza el dispositivo actual (exige reenviar la cookie de dispositivo). */
export async function trustMyDevice(
  session: SytueSession,
  id: string,
  deviceCookie: Record<string, string> = {},
): Promise<Device> {
  return toDevice(
    await meHttp(session).json<DeviceWire>({
      method: 'POST',
      path: `/me/devices/${encodeURIComponent(id)}/trust`,
      headers: deviceCookie,
      body: '{}',
    }),
  );
}

/** Revoca (borra) uno de mis dispositivos. */
export function revokeMyDevice(session: SytueSession, id: string): Promise<unknown> {
  return meHttp(session).json({ method: 'DELETE', path: `/me/devices/${encodeURIComponent(id)}` });
}

/** Dispositivos de la organización activa (vista admin). Degrada a lista vacía sin backend. */
export async function loadOrgDevices(session: SytueSession): Promise<Sourced<Device[]>> {
  try {
    const res = await adminHttp(session).json<{ devices?: DeviceWire[] }>({ path: '/devices' });
    if (!Array.isArray(res?.devices)) throw new Error('bad_shape');
    return { live: true, data: res.devices.map(toDevice) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Revoca (borra) un dispositivo de la org (admin, aislado por tenant en Sytue ID). */
export function adminRevokeDevice(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/devices/${encodeURIComponent(id)}`,
  });
}

// ---- Roles personalizados / admin delegada ----

/** Fila del endpoint admin de Sytue ID (GET /api/v1/admin/roles). */
interface CustomRoleWire {
  id: string;
  name: string;
  permissions?: string[] | null;
  member_count?: number;
}

/** Usuario asignado devuelto por GET /api/v1/admin/roles/:id. */
interface CustomRoleMemberWire {
  user_id: string;
  email: string;
  name?: string;
}

function toCustomRole(w: CustomRoleWire): CustomRole {
  return {
    id: w.id,
    name: w.name || w.id,
    permissions: Array.isArray(w.permissions) ? w.permissions : [],
    memberCount: typeof w.member_count === 'number' ? w.member_count : 0,
  };
}

function toCustomRoleMember(w: CustomRoleMemberWire): CustomRoleMember {
  return { userId: w.user_id, email: w.email, name: w.name || w.email };
}

/**
 * Roles personalizados del tenant activo (Sytue ID acota por el tenant del token).
 * Degrada a lista vacía si no hay backend vivo, como loadSsoProviders.
 */
export async function loadCustomRoles(session: SytueSession): Promise<Sourced<CustomRole[]>> {
  try {
    const res = await adminHttp(session).json<{ roles?: CustomRoleWire[] }>({ path: '/roles' });
    if (!Array.isArray(res?.roles)) throw new Error('bad_shape');
    return { live: true, data: res.roles.map(toCustomRole) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Detalle de un rol con sus usuarios asignados (GET /api/v1/admin/roles/:id). */
export async function loadRoleDetail(
  session: SytueSession,
  id: string,
): Promise<{ role: CustomRole; members: CustomRoleMember[] }> {
  const res = await adminHttp(session).json<{
    role: CustomRoleWire;
    members?: CustomRoleMemberWire[];
  }>({ path: `/roles/${encodeURIComponent(id)}` });
  return {
    role: toCustomRole(res.role),
    members: Array.isArray(res.members) ? res.members.map(toCustomRoleMember) : [],
  };
}

/** Crea un rol en el tenant activo (name obligatorio; permisos validados por Sytue ID). */
export async function createCustomRole(
  session: SytueSession,
  input: CustomRoleInput,
): Promise<CustomRole> {
  return toCustomRole(await adminHttp(session).postJson<CustomRoleWire>('/roles', input));
}

/** Edita nombre/permisos de un rol (PATCH: solo los campos presentes). */
export async function updateCustomRole(
  session: SytueSession,
  id: string,
  patch: CustomRoleInput,
): Promise<CustomRole> {
  return toCustomRole(
    await adminHttp(session).json<CustomRoleWire>({
      method: 'PATCH',
      path: `/roles/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  );
}

/** Borra un rol (sus asignaciones caen en cascada en Sytue ID). */
export function deleteCustomRole(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({ method: 'DELETE', path: `/roles/${encodeURIComponent(id)}` });
}

/** Asigna el rol a un usuario del tenant. */
export function assignCustomRole(
  session: SytueSession,
  id: string,
  userId: string,
): Promise<unknown> {
  return adminHttp(session).postJson(`/roles/${encodeURIComponent(id)}/members`, {
    user_id: userId,
  });
}

/** Quita el rol a un usuario. */
export function unassignCustomRole(
  session: SytueSession,
  id: string,
  userId: string,
): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/roles/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`,
  });
}

// ---- PIM (Privileged Identity Management) ----

interface PimEligibilityWire {
  id: string;
  user_id: string;
  user_email: string;
  user_name: string;
  role_id: string;
  builtin_role: string;
  role_name: string;
  requires_approval: boolean;
  max_duration_minutes: number;
}

interface PimActivationWire {
  id: string;
  user_id: string;
  user_email: string;
  user_name: string;
  role_id: string;
  builtin_role: string;
  role_name: string;
  status: string;
  effective_status: string;
  justification: string;
  duration_minutes: number;
  requested_at: string;
  activated_at?: string;
  expires_at?: string;
  remaining_seconds?: number;
  approved_by_email?: string;
}

function toPimEligibility(w: PimEligibilityWire): PimEligibility {
  return {
    id: w.id,
    userId: w.user_id,
    userEmail: w.user_email || w.user_id,
    userName: w.user_name || w.user_email || w.user_id,
    roleId: w.role_id || '',
    builtinRole: w.builtin_role || '',
    roleName: w.role_name || w.builtin_role || w.role_id,
    requiresApproval: Boolean(w.requires_approval),
    maxDurationMinutes: typeof w.max_duration_minutes === 'number' ? w.max_duration_minutes : 60,
  };
}

function toPimActivation(w: PimActivationWire): PimActivation {
  // exactOptionalPropertyTypes: solo asignar las opcionales cuando llegan con valor.
  const a: PimActivation = {
    id: w.id,
    userId: w.user_id,
    userEmail: w.user_email || w.user_id,
    userName: w.user_name || w.user_email || w.user_id,
    roleId: w.role_id || '',
    builtinRole: w.builtin_role || '',
    roleName: w.role_name || w.builtin_role || w.role_id,
    status: w.status,
    effectiveStatus: w.effective_status || w.status,
    justification: w.justification || '',
    durationMinutes: typeof w.duration_minutes === 'number' ? w.duration_minutes : 0,
    requestedAt: w.requested_at,
  };
  if (w.activated_at) a.activatedAt = w.activated_at;
  if (w.expires_at) a.expiresAt = w.expires_at;
  if (typeof w.remaining_seconds === 'number') a.remainingSeconds = w.remaining_seconds;
  if (w.approved_by_email) a.approvedByEmail = w.approved_by_email;
  return a;
}

/** Elegibilidades PIM de la org activa (vista admin). Degrada a lista vacía sin backend. */
export async function loadPimEligibilities(
  session: SytueSession,
): Promise<Sourced<PimEligibility[]>> {
  try {
    const res = await adminHttp(session).json<{ eligibilities?: PimEligibilityWire[] }>({
      path: '/pim/eligibilities',
    });
    if (!Array.isArray(res?.eligibilities)) throw new Error('bad_shape');
    return { live: true, data: res.eligibilities.map(toPimEligibility) };
  } catch {
    return { live: false, data: [] };
  }
}

/** Activaciones PIM de la org (admin), opcionalmente filtradas por estado. */
export async function loadPimActivations(
  session: SytueSession,
  status?: string,
): Promise<Sourced<PimActivation[]>> {
  try {
    const res = await adminHttp(session).json<{ activations?: PimActivationWire[] }>({
      path: '/pim/activations',
      query: status ? { status } : {},
    });
    if (!Array.isArray(res?.activations)) throw new Error('bad_shape');
    return { live: true, data: res.activations.map(toPimActivation) };
  } catch {
    return { live: false, data: [] };
  }
}

// ---- Mis organizaciones (selector multi-org, self-service) ----

/** Una de MIS organizaciones, tal y como la devuelve `GET /api/v1/me`. */
export interface MyOrganization {
  id: string;
  slug: string;
  name: string;
  role: SytueRole;
  active: boolean;
}

interface MeWire {
  active_org?: string;
  memberships?: { org_id?: string; slug?: string; name?: string; role?: string; status?: string }[];
}

/**
 * Mis organizaciones, con cuál está activa. Solo las ACTIVAS: una suspendida no
 * se puede elegir (Sytue ID la rechaza con 403), y ofrecerla sería un botón que
 * solo sabe fallar.
 */
export async function loadMyOrganizations(
  session: SytueSession,
): Promise<Sourced<MyOrganization[]>> {
  try {
    const res = await meHttp(session).json<MeWire>({ path: '/me' });
    if (!Array.isArray(res?.memberships)) throw new Error('bad_shape');
    const activo = typeof res.active_org === 'string' ? res.active_org : '';
    return {
      live: true,
      data: res.memberships
        .filter((m) => m.status === 'active' && m.org_id && m.slug)
        .map((m) => ({
          id: String(m.org_id),
          slug: String(m.slug),
          name: m.name ? String(m.name) : String(m.slug),
          role: (m.role ?? 'member') as SytueRole,
          active: m.slug === activo,
        })),
    };
  } catch {
    return { live: false, data: [] };
  }
}

/**
 * Cambia mi organización activa. Devuelve el slug de destino.
 *
 * Lo que NO hace es cambiar el token que ya tiene el navegador: el claim
 * `tenant` va firmado dentro. Quien llame tiene que rehacer el login SSO
 * después (una redirección, no un login: la cookie de sesión sigue viva) o la
 * pantalla seguiría enseñando la organización anterior.
 */
export async function setMyActiveOrganization(
  session: SytueSession,
  orgId: string,
): Promise<{ slug: string }> {
  const res = await meHttp(session).json<{ slug?: string }>({
    method: 'PUT',
    path: '/me/active-organization',
    body: JSON.stringify({ org_id: orgId }),
  });
  return { slug: typeof res?.slug === 'string' ? res.slug : '' };
}

/** Mi PIM (self-service): mis elegibilidades y mis activaciones en la org activa. */
export async function loadMyPim(
  session: SytueSession,
): Promise<Sourced<{ eligibilities: PimEligibility[]; activations: PimActivation[] }>> {
  try {
    const res = await meHttp(session).json<{
      eligibilities?: PimEligibilityWire[];
      activations?: PimActivationWire[];
    }>({ path: '/me/pim' });
    if (!Array.isArray(res?.eligibilities) || !Array.isArray(res?.activations)) {
      throw new Error('bad_shape');
    }
    return {
      live: true,
      data: {
        eligibilities: res.eligibilities.map(toPimEligibility),
        activations: res.activations.map(toPimActivation),
      },
    };
  } catch {
    return { live: false, data: { eligibilities: [], activations: [] } };
  }
}

/** Declara a un usuario elegible para un rol (admin). */
export async function createPimEligibility(
  session: SytueSession,
  input: PimEligibilityInput,
): Promise<PimEligibility> {
  return toPimEligibility(
    await adminHttp(session).postJson<PimEligibilityWire>('/pim/eligibilities', input),
  );
}

/** Retira una elegibilidad (admin). */
export function deletePimEligibility(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).json({
    method: 'DELETE',
    path: `/pim/eligibilities/${encodeURIComponent(id)}`,
  });
}

/** Aprueba una solicitud de activación pendiente (admin). */
export async function approvePimActivation(
  session: SytueSession,
  id: string,
): Promise<PimActivation> {
  return toPimActivation(
    await adminHttp(session).postJson<PimActivationWire>(
      `/pim/activations/${encodeURIComponent(id)}/approve`,
      {},
    ),
  );
}

/** Deniega una solicitud de activación pendiente (admin). */
export function denyPimActivation(session: SytueSession, id: string): Promise<unknown> {
  return adminHttp(session).postJson(`/pim/activations/${encodeURIComponent(id)}/deny`, {});
}

/** Activa un rol elegible (self-service). Queda pending o active según la política. */
export async function activatePim(
  session: SytueSession,
  input: PimActivateInput,
): Promise<PimActivation> {
  return toPimActivation(
    await meHttp(session).postJson<PimActivationWire>('/me/pim/activate', input),
  );
}

/** Desactiva una activación activa propia (self-service). */
export function deactivatePim(session: SytueSession, activationId: string): Promise<unknown> {
  return meHttp(session).postJson('/me/pim/deactivate', { activation_id: activationId });
}

// ---- Métricas del panel ----

/**
 * Métricas del panel. Derivadas de los datos ya cargados (tenants/usuarios) para
 * mantener coherencia entre vistas; si no hay backend, quedan sobre el seed.
 */
export function buildMetrics(tenants: Tenant[], users: DirectoryUser[]): ConsoleMetric[] {
  const mfa = users.filter((u) => u.mfaEnabled).length;
  return [
    {
      labelKey: 'dashboard.metric.tenants',
      value: String(tenants.length),
      hintKey: 'dashboard.metric.tenantsHint',
    },
    {
      labelKey: 'dashboard.metric.users',
      value: String(users.length),
      hintKey: 'dashboard.metric.usersHint',
    },
    {
      labelKey: 'dashboard.metric.mfa',
      value: `${mfa}/${users.length}`,
      hintKey: 'dashboard.metric.mfaHint',
    },
  ];
}
