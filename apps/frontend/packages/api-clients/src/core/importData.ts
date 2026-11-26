/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ASISTENTE DE IMPORTACIÓN — traerse uno sus propios datos de Google o Microsoft 365.
 *
 * El motor de migración vive en el Sytue Integration Gateway y lo tenía SOLO la consola
 * de administración: quien llegaba a Sytue y abría Mail con el buzón vacío no encontraba
 * por dónde traerse su correo. Esto es el lado servidor de la puerta que ahora vive
 * dentro de cada producto — Mail, Drive, Calendar y Contacts—, que es donde se busca.
 *
 * Vive en un paquete compartido porque las cuatro apps necesitan exactamente lo mismo y
 * cada una tiene su propio BFF: sin esto habría cuatro copias del mismo proxy
 * divergiendo. Lo que cambia por app es UN dato —qué contenido trae— y viaja como
 * parámetro. Hasta el propio endpoint (`importEndpoint`) vive aquí: las cuatro rutas
 * `/api/import` eran ya copias con la misma validación escrita cuatro veces.
 *
 * El token de sesión NUNCA sale del servidor: estas funciones se llaman desde el
 * endpoint BFF de cada app, no desde el navegador.
 */

/** Contenido que se puede traer. Coincide con `MigrationDomain` del gateway. */
export type ImportDomain = 'mail' | 'drive' | 'calendar' | 'contacts';

/** SaaS del que se importa. */
export type ImportSource = 'google' | 'm365';

/**
 * Si se puede lanzar: `unavailable` es una conexión que existe pero no puede leer
 * (caducó, se revocó o nunca se autorizó para datos). Se enseña apagada con su motivo:
 * un botón que siempre falla no explica nada.
 */
export type ImportOriginState = 'ready' | 'unavailable';

/** Por qué falló una importación, cuando tiene arreglo en manos de alguien. */
export type ImportErrorCode = 'connection_expired' | 'needs_admin';

/**
 * Un origen disponible: UNA cuenta del proveedor que la organización ya autorizó, aunque
 * la haya conectado varias veces.
 */
export interface ImportOrigin {
  /** La conexión con la que se lanza: la mejor de las de esa cuenta. */
  instanceId: string;
  /** Todas las conexiones de la cuenta, la elegida primero. */
  instanceIds: string[];
  /** Nombre que le puso la organización a la conexión. */
  name: string;
  source: ImportSource;
  /** Identidad estable de la cuenta («google:acme.com»). Vacía si no se sabe. */
  accountKey: string;
  /** Cómo se le enseña a una persona («acme.com»). Vacío si no se sabe. */
  accountLabel: string;
  state: ImportOriginState;
}

/** Estado de una importación en curso o terminada. */
export interface ImportJob {
  id: string;
  /** Conexión con la que corre; agrupa cuando no se sabe la cuenta. */
  instanceId: string;
  domain: ImportDomain;
  source: ImportSource;
  status: 'queued' | 'running' | 'done' | 'error' | 'canceled';
  /** Lo traído y lo que se quedó fuera, ACUMULADO de todas sus pasadas. */
  migrated: number;
  skipped: number;
  errors: number;
  lastError: string;
  /** Vacío: el fallo no tiene más arreglo que reintentar. */
  errorCode: ImportErrorCode | '';
  accountKey: string;
  accountLabel: string;
  createdAt: string;
  finishedAt?: string;
}

interface WireOrigin {
  instance_id: string;
  instance_ids?: string[];
  name: string;
  source: string;
  account_key?: string;
  account_label?: string;
  state?: string;
}

interface WireInstance {
  id: string;
  name: string;
  template_key: string;
  status: string;
  has_secrets?: boolean;
  config?: Record<string, unknown> | null;
}

interface WireJob {
  id: string;
  instance_id?: string;
  domain: string;
  source: string;
  status: string;
  counts?: { migrated?: number; skipped?: number; errors?: number };
  last_error?: string;
  error_code?: string;
  account_key?: string;
  account_label?: string;
  created_at: string;
  finished_at?: string;
}

/**
 * Qué plantilla del catálogo es cada origen migrable. Son EXACTAMENTE las de
 * `sourceForTemplate` del gateway, que es quien decide; aquí solo se usan si el gateway
 * es anterior a `/migrations/self/origins`.
 *
 * Antes se reconocía por prefijo («google…», «microsoft…») y eso era el duplicado que se
 * veía: la misma cuenta conectada para Drive, Meet, Chat, Teams o el correo de Outlook
 * salía como otro «Traerlo de…», y todos menos uno fallaban al pulsarlos.
 */
const ORIGENES: Record<string, ImportSource> = {
  'google-workspace': 'google',
  microsoft365: 'm365',
};

// `Record<string, string>` y no `HeadersInit`: este objeto se esparce con `...` en la
// línea de abajo, y `HeadersInit` admite además `Headers` y `string[][]` —formas cuyo
// spread produce índices numéricos y dejaría la petición SIN cabecera `authorization`.
function auth(accessToken: string): Record<string, string> {
  return { authorization: `Bearer ${accessToken}`, accept: 'application/json' };
}

function api(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/$/, '')}/api/v1${path}`;
}

function texto(v: unknown): string {
  return typeof v === 'string' ? v.trim() : '';
}

function toOrigin(w: WireOrigin): ImportOrigin | null {
  const source = w.source === 'google' || w.source === 'm365' ? w.source : null;
  if (!source || !w.instance_id) return null;
  const ids = Array.isArray(w.instance_ids) ? w.instance_ids.filter(Boolean) : [];
  return {
    instanceId: w.instance_id,
    instanceIds: ids.includes(w.instance_id) ? ids : [w.instance_id, ...ids],
    name: w.name ?? '',
    source,
    accountKey: texto(w.account_key).toLowerCase(),
    accountLabel: texto(w.account_label),
    state: w.state === 'unavailable' ? 'unavailable' : 'ready',
  };
}

/**
 * Orígenes a partir de `/instances`, para un gateway anterior al listado de orígenes. Es
 * la misma regla que el servidor, en pequeño: plantillas exactas, ni borradores ni
 * desactivadas, y la cuenta del dominio o el inquilino que diga la configuración.
 */
function fromInstances(instances: WireInstance[]): ImportOrigin[] {
  return instances.flatMap((i): ImportOrigin[] => {
    const source = ORIGENES[i.template_key];
    if (!source || (i.status !== 'active' && i.status !== 'error')) return [];
    const cfg = i.config ?? {};
    let accountKey = texto(cfg._import_account).toLowerCase();
    let accountLabel = texto(cfg._import_account_label);
    if (!accountKey) {
      const declarada = texto(source === 'google' ? cfg.domain : cfg.tenant).toLowerCase();
      if (declarada) [accountKey, accountLabel] = [`${source}:${declarada}`, declarada];
    }
    return [
      {
        instanceId: i.id,
        instanceIds: [i.id],
        name: i.name,
        source,
        accountKey,
        accountLabel: accountLabel || accountKey.slice(accountKey.indexOf(':') + 1),
        state: i.status === 'active' && i.has_secrets !== false ? 'ready' : 'unavailable',
      },
    ];
  });
}

/**
 * UN origen por (proveedor, cuenta). El servidor ya lo hace; esto es la red por si un
 * gateway viejo, o uno con un fallo, devolviera la misma cuenta dos veces. Se queda la
 * primera (el servidor las da con la mejor delante), salvo que una posterior pueda
 * leer y ella no, y se juntan las conexiones de todas.
 *
 * Una conexión de cuenta desconocida se suma a la ÚNICA cuenta conocida de su
 * proveedor, como en el servidor: casi siempre es la misma conectada antes de que la
 * cuenta se recordara.
 */
export function unifyOrigins(origins: ImportOrigin[]): ImportOrigin[] {
  const conocidas = new Map<ImportSource, Set<string>>();
  for (const o of origins) {
    if (!o.accountKey) continue;
    const s = conocidas.get(o.source) ?? new Set<string>();
    s.add(o.accountKey);
    conocidas.set(o.source, s);
  }
  const porCuenta = new Map<string, ImportOrigin>();
  for (const o of origins) {
    const unicas = conocidas.get(o.source);
    const cuenta = o.accountKey || (unicas?.size === 1 ? [...unicas][0]! : '');
    const clave = `${o.source}|${cuenta}`;
    const antes = porCuenta.get(clave);
    if (!antes) {
      porCuenta.set(clave, { ...o, accountKey: cuenta, instanceIds: [...o.instanceIds] });
      continue;
    }
    const gana = antes.state !== 'ready' && o.state === 'ready' ? o : antes;
    const ids = [...gana.instanceIds];
    for (const id of [...antes.instanceIds, ...o.instanceIds]) if (!ids.includes(id)) ids.push(id);
    porCuenta.set(clave, {
      ...gana,
      accountKey: cuenta,
      accountLabel: gana.accountLabel || antes.accountLabel || o.accountLabel,
      instanceIds: ids,
    });
  }
  return [...porCuenta.values()];
}

/**
 * Orígenes de los que esta persona puede traerse datos: UNA entrada por cuenta de
 * Google o Microsoft que su organización tenga conectada.
 *
 * Degrada a lista vacía ante cualquier fallo: sin orígenes la interfaz no ofrece nada, y
 * eso es correcto en una organización que no ha conectado ninguno. Enseñar un error donde
 * la respuesta honesta es «no hay» solo asustaría.
 */
export async function listImportOrigins(
  baseUrl: string,
  accessToken: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ImportOrigin[]> {
  try {
    const res = await fetchImpl(api(baseUrl, '/migrations/self/origins'), {
      headers: auth(accessToken),
    });
    if (res.ok) {
      const { origins = [] } = (await res.json()) as { origins?: WireOrigin[] };
      return unifyOrigins(origins.flatMap((o) => toOrigin(o) ?? []));
    }
    // Un gateway anterior al listado de orígenes: se deduce de las conexiones.
    if (res.status !== 404) return [];
    const viejo = await fetchImpl(api(baseUrl, '/instances'), { headers: auth(accessToken) });
    if (!viejo.ok) return [];
    const { instances = [] } = (await viejo.json()) as { instances?: WireInstance[] };
    return unifyOrigins(fromInstances(instances));
  } catch {
    return [];
  }
}

function toJob(w: WireJob): ImportJob {
  const job: ImportJob = {
    id: w.id,
    instanceId: w.instance_id ?? '',
    domain: w.domain as ImportDomain,
    source: w.source as ImportSource,
    status: w.status as ImportJob['status'],
    migrated: w.counts?.migrated ?? 0,
    skipped: w.counts?.skipped ?? 0,
    errors: w.counts?.errors ?? 0,
    lastError: w.last_error ?? '',
    errorCode:
      w.error_code === 'connection_expired' || w.error_code === 'needs_admin' ? w.error_code : '',
    accountKey: texto(w.account_key).toLowerCase(),
    accountLabel: texto(w.account_label),
    createdAt: w.created_at,
  };
  if (w.finished_at) job.finishedAt = w.finished_at;
  return job;
}

/**
 * La importación VIGENTE de cada cuenta y contenido: la más reciente.
 *
 * Una cuenta importada dos veces —dos conexiones de la misma cuenta, o trabajos de antes
 * de que relanzar reutilizara el anterior— dejaba dos trabajos, y el resumen los SUMABA:
 * «traídos 2 400» cuando eran 1 200 copiados dos veces. Los contadores de un trabajo ya
 * son acumulados de todas sus pasadas, así que el último basta. Sin cuenta conocida, se
 * agrupa por conexión.
 */
export function latestImports(jobs: ImportJob[]): ImportJob[] {
  const orden = [...jobs].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  const vistas = new Set<string>();
  return orden.filter((j) => {
    const clave = `${j.accountKey || `instance:${j.instanceId || j.id}`}|${j.domain}`;
    if (vistas.has(clave)) return false;
    vistas.add(clave);
    return true;
  });
}

/** Resultado de lanzar: los trabajos, o el código con el que el gateway lo rechazó. */
export interface StartImportResult {
  ok: boolean;
  status: number;
  /** Código estable del gateway (p. ej. `import_connection_unavailable`); vacío si no lo hay. */
  code: string;
  jobs: ImportJob[];
}

/**
 * Lanza la importación. El gateway ignora a quién se le diga que migrar: destino y
 * origen son siempre quien llama, así que aquí no viaja ninguna identidad. Y es
 * idempotente: lanzar otra vez la misma cuenta y contenido devuelve el trabajo que ya
 * había —en marcha, o reanudado donde se quedó— en vez de copiarlo todo de nuevo.
 */
export async function startImport(
  baseUrl: string,
  accessToken: string,
  input: { instanceId: string; source: ImportSource; domains: ImportDomain[] },
  fetchImpl: typeof fetch = fetch,
): Promise<StartImportResult> {
  const res = await fetchImpl(api(baseUrl, '/migrations/self'), {
    method: 'POST',
    headers: { ...auth(accessToken), 'content-type': 'application/json' },
    body: JSON.stringify({
      instance_id: input.instanceId,
      source: input.source,
      domains: input.domains,
    }),
  });
  if (!res.ok) {
    const cuerpo = (await res.json().catch(() => null)) as { error?: { code?: unknown } } | null;
    return { ok: false, status: res.status, code: texto(cuerpo?.error?.code), jobs: [] };
  }
  const { migrations = [] } = (await res.json()) as { migrations?: WireJob[] };
  return { ok: true, status: res.status, code: '', jobs: migrations.map(toJob) };
}

/**
 * Importaciones de esta persona, filtradas al contenido que le interesa a ESTA app y
 * reducidas a la vigente de cada cuenta y contenido (`latestImports`).
 *
 * Se filtra aquí y no en el gateway porque el mismo trabajo le importa a dos sitios: una
 * migración de contactos la enseñan Contacts y Mail, cada uno con sus palabras.
 */
export async function listMyImports(
  baseUrl: string,
  accessToken: string,
  domains: ImportDomain[],
  fetchImpl: typeof fetch = fetch,
): Promise<ImportJob[]> {
  try {
    const res = await fetchImpl(api(baseUrl, '/migrations/self'), {
      headers: auth(accessToken),
    });
    if (!res.ok) return [];
    const { migrations = [] } = (await res.json()) as { migrations?: WireJob[] };
    const quiero = new Set<string>(domains);
    return latestImports(migrations.filter((m) => quiero.has(m.domain)).map(toJob));
  } catch {
    return [];
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Lo que este producto trae de lo que pida el cliente; sin petición, todo lo suyo. */
function pedidos(propios: ImportDomain[], raw: string | null): ImportDomain[] {
  const piden = new Set((raw ?? '').split(',').map((d) => d.trim()));
  const out = propios.filter((d) => piden.has(d));
  return out.length > 0 ? out : propios;
}

/**
 * El endpoint `/api/import` de cada producto, entero:
 *
 *   GET  /api/import?domains=…  → { origins, jobs }  orígenes conectados + mi progreso
 *   POST /api/import { instanceId, source, domains } → lanza la importación
 *
 * `domains` es lo que ESE producto sabe traer: lo que pida el navegador se acota a eso,
 * para que «traer mi correo» no se convierta en «traer todo lo mío». Un rechazo del
 * gateway viaja con su código (`code`) para que la interfaz diga qué pasó —una conexión
 * caducada no se arregla «intentándolo dentro de un momento»—.
 *
 * Sin token (sin sesión) responde 401 sin tocar el gateway.
 */
export async function importEndpoint(
  request: Request,
  opts: {
    baseUrl: string;
    accessToken: string | null | undefined;
    domains: ImportDomain[];
    fetchImpl?: typeof fetch;
  },
): Promise<Response> {
  const { baseUrl, accessToken, domains: propios, fetchImpl = fetch } = opts;
  if (!accessToken) return json({ error: 'unauthenticated' }, 401);

  if (request.method === 'GET') {
    const domains = pedidos(propios, new URL(request.url).searchParams.get('domains'));
    const [origins, jobs] = await Promise.all([
      listImportOrigins(baseUrl, accessToken, fetchImpl),
      listMyImports(baseUrl, accessToken, domains, fetchImpl),
    ]);
    return json({ origins, jobs });
  }
  if (request.method !== 'POST') return json({ error: 'method_not_allowed' }, 405);

  let p: { instanceId?: unknown; source?: unknown; domains?: unknown };
  try {
    p = (await request.json()) as typeof p;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const instanceId = typeof p.instanceId === 'string' ? p.instanceId : '';
  const source = p.source === 'google' || p.source === 'm365' ? p.source : null;
  if (!instanceId || !source) return json({ error: 'missing_fields' }, 400);

  const piden = Array.isArray(p.domains) ? p.domains.map(String) : [];
  const domains = propios.filter((d) => piden.includes(d));
  if (domains.length === 0) return json({ error: 'missing_domains' }, 400);

  try {
    const res = await startImport(baseUrl, accessToken, { instanceId, source, domains }, fetchImpl);
    if (!res.ok) {
      return json({ error: 'import_failed', code: res.code }, res.status >= 500 ? 502 : res.status);
    }
    return json({ jobs: res.jobs }, 202);
  } catch {
    return json({ error: 'import_failed', code: '' }, 502);
  }
}
