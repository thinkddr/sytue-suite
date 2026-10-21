/*
 * Proveedor de datos de ADMINISTRACIÓN DE PLATAFORMA (SOLO servidor).
 *
 * Habla con /api/v1/platform de Sytue ID con el access token httpOnly de la
 * sesión. A diferencia del resto de `lib/`, aquí NO hay degradación a datos de
 * demostración: esta consola decide suspensiones, descuentos y fusiones, y unos
 * números inventados de un seed serían indistinguibles de los reales. Si el
 * backend no responde, se dice.
 *
 * La autorización NO se decide aquí: la impone Sytue ID (allowlist
 * `platform_admins` + segundo factor). Este módulo solo propaga el 403.
 */
import type { SytueSession } from '@sytue/auth';
import { adminBaseUrl } from './auth.js';

export interface PlatformOrg {
  id: string;
  slug: string;
  name: string;
  status: 'active' | 'suspended' | 'deleted' | 'merged';
  namespace: string;
  active_users: number;
  seats_purchased: number;
  seats_granted: number;
  seats_effective: number;
  over_capacity: boolean;
  subscription_status: string;
  owner_emails: string[];
  education?: boolean;
  created_at: string;
  merged_into_org_id?: string;
  merged_at?: string;
  seat_grants?: SeatGrant[];
  /**
   * La organización que OPERA la plataforma (`corporate`). No se suspende, no se
   * fusiona y no se elimina: sus miembros son los administradores globales y una
   * organización no activa no admite inicios de sesión, así que hacerlo dejaría
   * fuera a todo el mundo, incluido quien tendría que deshacerlo. Lo deniega el
   * servidor; esto es para no ofrecer el botón.
   */
  is_platform?: boolean;
}

export interface SeatGrant {
  id: string;
  org_id: string;
  seats: number;
  reason: string;
  kind: 'goodwill' | 'employee' | 'trial' | 'migration';
  granted_by: string;
  expires_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface PlatformUsage {
  orgs: number;
  orgs_active: number;
  orgs_suspended: number;
  orgs_merged: number;
  users: number;
  users_active: number;
  users_active_30d: number;
  seats_purchased: number;
  seats_granted: number;
  subscriptions_active: number;
}

export interface OrgMerge {
  id: string;
  source_org_id: string;
  target_org_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'aborted';
  services: Record<string, string>;
  error: string;
  requested_by: string;
  created_at: string;
}

export interface PlatformAdmin {
  user_id: string;
  email: string;
  name: string;
  granted_by: string;
  reason: string;
  created_at: string;
  /** Última revisión del acceso (mig 0047). Nunca vacía: el alta cuenta como una. */
  reviewed_at: string;
  reviewed_by: string;
  /** Lo calcula el servidor: lleva más del periodo sin que nadie lo mire. */
  review_due: boolean;
}

/**
 * Elevación temporal del alcance global (PIM, mig 0043). Estar en
 * `platform_admins` solo dice quién PUEDE elevarse; esto dice quién lo está.
 */
export interface PlatformActivation {
  id: string;
  user_id: string;
  user_email?: string;
  user_name?: string;
  status: 'active' | 'revoked';
  /** Con la caducidad ya aplicada: una fila 'active' vencida vale 'expired'. */
  effective_status: 'active' | 'revoked' | 'expired';
  justification: string;
  duration_minutes: number;
  activated_at: string;
  expires_at: string;
  revoked_at?: string;
  remaining_seconds?: number;
}

/**
 * Resultado de una llamada: o datos, o el motivo por el que no los hay. El
 * `code` viaja con el fallo porque los tres 403 de esta API significan cosas
 * distintas —no eres admin global, lo eres sin segundo factor, lo eres sin
 * elevar— y solo el último tiene arreglo desde la propia pantalla.
 */
export type PlatformResult<T> =
  | { ok: true; data: T }
  | {
      ok: false;
      status: number;
      code?: string | undefined;
      message?: string | undefined;
      /** Los datos que interpola el mensaje, para componerlo traducido. */
      params?: Record<string, unknown> | undefined;
    };

/**
 * Una persona de una organización, como la ve la consola de plataforma: el mismo
 * directorio que su consola (`responderUsuariosDeOrg` en Sytue ID).
 */
export interface PlatformMember {
  id: string;
  email: string;
  name: string;
  role: 'owner' | 'admin' | 'member' | 'billing';
  status: 'active' | 'suspended' | 'deleted';
  mfa_enabled: boolean;
}

/**
 * platformFetch reenvía a Sytue ID conservando método, cuerpo y STATUS. El
 * status importa: 403 con code `mfa_required` es «vuelve a entrar con segundo
 * factor», y aplanarlo a un error genérico dejaría al admin sin saber qué hacer.
 */
export async function platformFetch<T>(
  session: SytueSession,
  path: string,
  init?: { method?: string; body?: unknown },
): Promise<PlatformResult<T>> {
  const url = `${adminBaseUrl()}/api/v1/platform${path}`;
  try {
    const res = await fetch(url, {
      method: init?.method ?? 'GET',
      headers: {
        authorization: `Bearer ${session.tokens.accessToken}`,
        ...(init?.body ? { 'content-type': 'application/json' } : {}),
      },
      ...(init?.body ? { body: JSON.stringify(init.body) } : {}),
    });
    if (!res.ok) {
      // El error viaja ANIDADO: {"error":{"code","message"},"request_id"} — el
      // sobre canónico de platform/pkg/httpx. Leerlo de la raíz devolvía siempre
      // `undefined`, y con el código perdido los tres 403 de esta API se
      // aplanaban al mismo callejón sin salida: un admin global sin elevar veía
      // «no puedes acceder» en vez del botón para elevarse.
      //
      // Se acepta también la forma plana porque el BFF reenvía así sus propios
      // errores, y este módulo lo usan los dos caminos.
      const body = (await res.json().catch(() => ({}))) as {
        error?: { code?: string; message?: string; params?: Record<string, unknown> };
        code?: string;
        message?: string;
        params?: Record<string, unknown>;
      };
      const err = body.error ?? body;
      return {
        ok: false,
        status: res.status,
        code: err.code,
        message: err.message,
        params: err.params,
      };
    }
    if (res.status === 204) return { ok: true, data: undefined as T };
    return { ok: true, data: (await res.json()) as T };
  } catch {
    // El servicio no respondió. 503 y no 500: es indisponibilidad, no un fallo
    // de la petición, y la consola lo distingue al mostrarlo.
    return { ok: false, status: 503 };
  }
}

export async function loadPlatformUsage(s: SytueSession): Promise<PlatformResult<PlatformUsage>> {
  return platformFetch<PlatformUsage>(s, '/usage');
}

export async function loadPlatformOrgs(
  s: SytueSession,
  q = '',
): Promise<PlatformResult<{ items: PlatformOrg[] }>> {
  const suffix = q ? `?q=${encodeURIComponent(q)}` : '';
  return platformFetch<{ items: PlatformOrg[] }>(s, `/orgs${suffix}`);
}

/**
 * Detalle de una organización, con sus plazas regaladas. Va aparte del listado
 * a propósito: son una por concesión y solo interesan cuando se mira UNA, así
 * que traerlas en la lista sería cargar cientos de filas que nadie va a leer.
 */
export async function loadPlatformOrg(
  s: SytueSession,
  id: string,
): Promise<PlatformResult<PlatformOrg>> {
  return platformFetch<PlatformOrg>(s, `/orgs/${encodeURIComponent(id)}`);
}

/**
 * Un problema del registro: el mismo mensaje del mismo servicio, con cuántas
 * veces ha pasado y desde cuándo. Se agrupa en el servidor porque mil líneas
 * iguales no son mil cosas que mirar, son una que lleva pasando mil veces.
 */
export interface LogGroup {
  level: string;
  service: string;
  message: string;
  count: number;
  first: string;
  last: string;
  sample?: Record<string, string>;
}

export interface LogStats {
  errors_24h: number;
  warns_24h: number;
  errors_1h: number;
  services_affected: number;
  by_hour: { hour: string; errors: number; warns: number }[];
}

export async function loadPlatformLogs(
  s: SytueSession,
  q: { level?: string; service?: string; q?: string; hours?: number } = {},
): Promise<PlatformResult<{ items: LogGroup[] }>> {
  const p = new URLSearchParams();
  if (q.level) p.set('level', q.level);
  if (q.service) p.set('service', q.service);
  if (q.q) p.set('q', q.q);
  if (q.hours) p.set('hours', String(q.hours));
  const qs = p.toString();
  return platformFetch<{ items: LogGroup[] }>(s, `/logs${qs ? `?${qs}` : ''}`);
}

export async function loadPlatformLogStats(s: SytueSession): Promise<PlatformResult<LogStats>> {
  return platformFetch<LogStats>(s, '/logs/stats');
}

/**
 * Versión que corre UNA pieza de la plataforma. `ok` false ⇒ no contestó, y
 * entonces `release`/`commit` van vacíos: no se rellenan con la del vecino.
 */
export interface ComponentVersion {
  name: string;
  /** `service` (Go) o `surface` (superficie web Astro). */
  kind: string;
  release: string;
  commit: string;
  ok: boolean;
  error?: string;
}

export interface PlatformVersions {
  items: ComponentVersion[];
  /** La release que declara la mayoría. Vacía si no hay mayoría clara. */
  expected: string;
}

export async function loadPlatformVersions(
  s: SytueSession,
): Promise<PlatformResult<PlatformVersions>> {
  return platformFetch<PlatformVersions>(s, '/versions');
}

export async function loadPlatformMerges(
  s: SytueSession,
): Promise<PlatformResult<{ items: OrgMerge[] }>> {
  return platformFetch<{ items: OrgMerge[] }>(s, '/merges');
}

export async function loadPlatformAdmins(
  s: SytueSession,
): Promise<PlatformResult<{ items: PlatformAdmin[]; review_days: number }>> {
  return platformFetch<{ items: PlatformAdmin[]; review_days: number }>(s, '/admins');
}

/**
 * La elevación propia. Esta ruta NO está detrás del gate de plataforma —pedirla
 * desde detrás sería imposible: el gate exige justo lo que se va a pedir—, así
 * que responde a cualquier elegible y es lo que distingue «no eres admin global»
 * de «lo eres pero no estás elevado».
 */
export async function loadPlatformActivation(
  s: SytueSession,
): Promise<PlatformResult<{ activation: PlatformActivation | null }>> {
  return platformFetch<{ activation: PlatformActivation | null }>(s, '/activation');
}

export async function loadPlatformActivations(
  s: SytueSession,
): Promise<PlatformResult<{ items: PlatformActivation[] }>> {
  return platformFetch<{ items: PlatformActivation[] }>(s, '/activations');
}

/** Qué puede ver quien mira: su elevación, o el motivo por el que no hay datos. */
export interface AccesoPlataforma {
  /** La elevación viva de quien mira. `null` si no la tiene. */
  activation: PlatformActivation | null;
  /** Status que impide enseñar datos; `undefined` ⇒ se pueden pedir. */
  problema?: number | undefined;
  /** Código que separa los tres motivos del 403. */
  motivo?: string | undefined;
}

/**
 * accesoDePlataforma traduce la respuesta de `/platform/activation` a si hay algo
 * que pedir, y si no, a por qué. Es lo que evita disparar las siete llamadas de
 * la consola de plataforma cuando ya se sabe que las siete van a responder 403.
 *
 * Se decide con ESTA llamada y no con la de los datos porque es la única que no
 * está detrás del gate: preguntar por lo de detrás para enterarse de que no
 * puedes pasar deja un aviso por llamada en el registro de plataforma —el mismo
 * que se lee en la pestaña de problemas de esta consola—, y siete falsos
 * problemas por visita entierran los de verdad.
 */
export function accesoDePlataforma(
  r: PlatformResult<{ activation: PlatformActivation | null }>,
): AccesoPlataforma {
  // 403 con su código (fuera de la allowlist, o sin segundo factor) y cualquier
  // otro fallo (Sytue ID caído) se propagan tal cual: cada uno tiene su pantalla.
  if (!r.ok) return { activation: null, problema: r.status, motivo: r.code };
  // Elegible pero sin elevar: el caso NORMAL al abrir la pantalla. Tiene salida
  // aquí mismo, y por eso lleva código propio.
  if (!r.data.activation) return { activation: null, problema: 403, motivo: 'activation_required' };
  return { activation: r.data.activation };
}

/**
 * Solicitud de una operación irreversible pendiente de una segunda persona
 * (mig 0046). `payload` lleva los parámetros de la acción: para `org.suspend`,
 * la organización y el motivo; para `org.merge`, origen y destino.
 */
export interface PlatformApproval {
  id: string;
  action: 'org.suspend' | 'org.delete' | 'org.merge' | 'platform.admin.grant';
  payload?: Record<string, string>;
  reason: string;
  requested_by: string;
  requester_email: string;
  status: 'pending' | 'executed' | 'rejected';
  effective_status: 'pending' | 'executed' | 'rejected' | 'expired';
  decided_by?: string;
  decider_email?: string;
  decided_at?: string;
  result?: string;
  created_at: string;
  expires_at: string;
}

export async function loadPlatformApprovals(
  s: SytueSession,
): Promise<PlatformResult<{ items: PlatformApproval[] }>> {
  return platformFetch<{ items: PlatformApproval[] }>(s, '/approvals');
}

/**
 * Marca o desmarca a una organización como institución educativa (50% de descuento en Pro).
 */
export async function setPlatformOrgEducation(
  s: SytueSession,
  id: string,
  education: boolean,
  reason: string,
): Promise<PlatformResult<{ id: string; slug: string; education: boolean }>> {
  return platformFetch<{ id: string; slug: string; education: boolean }>(
    s,
    `/orgs/${encodeURIComponent(id)}/education`,
    {
      method: 'POST',
      body: { education, reason },
    },
  );
}

/**
 * Consulta el detalle de una fusión por identificador.
 */
export async function loadPlatformMerge(
  s: SytueSession,
  id: string,
): Promise<PlatformResult<OrgMerge>> {
  return platformFetch<OrgMerge>(s, `/merges/${encodeURIComponent(id)}`);
}

/**
 * Retira el alcance de administrador global a una cuenta.
 */
export async function revokePlatformAdmin(
  s: SytueSession,
  id: string,
): Promise<PlatformResult<void>> {
  return platformFetch<void>(s, `/admins/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

/** Una entrada del registro de plataforma. `org_slug` vacío ⇒ no fue sobre ninguna. */
export interface PlatformAuditRow {
  id: string;
  occurred_at: string;
  actor_id: string;
  actor_email: string;
  action: string;
  target: string;
  org_slug: string;
  ip: string;
}

/**
 * Lo que han hecho los admin globales, de todas las organizaciones a la vez.
 * Se escribía desde el principio y no se leía en ninguna parte: la mitad con
 * org_id solo se veía desde la consola de esa organización y la otra mitad no
 * se veía nunca.
 */
export async function loadPlatformAudit(
  s: SytueSession,
  opts: { action?: string; cursor?: string; limit?: number } = {},
): Promise<PlatformResult<{ items: PlatformAuditRow[]; nextCursor: string }>> {
  const p = new URLSearchParams();
  if (opts.action) p.set('action', opts.action);
  if (opts.cursor) p.set('cursor', opts.cursor);
  if (opts.limit) p.set('limit', String(opts.limit));
  const qs = p.toString();
  return platformFetch<{ items: PlatformAuditRow[]; nextCursor: string }>(
    s,
    `/audit${qs ? `?${qs}` : ''}`,
  );
}
