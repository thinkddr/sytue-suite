/*
 * Proveedor de datos del control-plane de Sytue DevX (SOLO servidor).
 *
 * Cablea las paginas/endpoints SSR al PaaS real (cloud.sytue.eu) via el SDK
 * @sytue/api-clients (CloudClient) con el access token de la sesion. Mapea los
 * recursos del control-plane a los modelos de vista de esta app (types.ts) y
 * degrada con elegancia al seed de demostracion (data.ts) cuando NO hay backend
 * vivo (dev/local, error de red o 5xx): la UI nunca se queda vacia por un fallo
 * de red. NUNCA expone tokens ni secretos al cliente. docs/CONVENTIONS.md §8, §14.
 */
import {
  CloudClient,
  HttpClient,
  HttpError,
  bearer,
  type Deployment as SdkDeployment,
  type ManagedService as SdkService,
  type ManagedServiceKind as SdkServiceKind,
  type Project as SdkProject,
  type EnvVar as SdkEnvVar,
  type EnvScope as SdkEnvScope,
  type CloudDomain as SdkDomain,
  type DomainDnsInstructions,
  type CloudOrganization as SdkOrg,
  type UsageAggregate as SdkUsage,
  type UsageRecord as SdkUsageRecord,
  type UptimeCheck as SdkUptimeCheck,
  type AlertPolicy as SdkAlertPolicy,
  type InstanceSize as SdkInstanceSize,
  type SlugLanguageOptions,
  type SlugSuggestion,
  type SlugCheck,
  type TemplateApplied as SdkTemplateApplied,
  type SizingAdvice,
  type AppCatalog as SdkAppCatalog,
  type RepoInspection,
} from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import { contiene, formatDate } from '@sytue/i18n';
import { formatBytes } from '@sytue/ui';
import { cloudBaseUrl } from './auth.js';
import { appT, metricLabel } from '../i18n/index.js';
import * as seed from './data.js';
import { formatNumber } from './format.js';
import { METRICS, SUITE_SEAT_PRICE_EUR, SUITE_PRODUCTS } from './pricing.js';
import type {
  DbApiInfo,
  DbRows,
  DbTable,
  Deployment,
  Domain,
  EnvVar,
  Framework,
  LogLine,
  ManagedService,
  Project,
  ServiceKind,
  SqlResult,
  UsageMetric,
  Organization,
  UptimeCheck,
  UptimeStatus,
  AlertPolicy,
  AlertEvent,
} from './types.js';

/**
 * Resultado de una carga: origen + payload + (si lo hubo) el error REAL.
 *
 * `live:false` significaba dos cosas muy distintas y las presentaba igual: «no hay
 * backend conectado» (dev, y entonces los datos de ejemplo tienen sentido) y «el
 * control-plane respondió 403/500» (y entonces enseñar datos de ejemplo plausibles
 * es mentir: el usuario cree estar viendo sus recursos). `error` separa los casos.
 */
export interface Sourced<T> {
  live: boolean;
  data: T;
  /** Presente solo si el control-plane RESPONDIÓ con error. No es modo demostración. */
  error?: { status: number; code: string };
}

/**
 * Degrada una carga fallida distinguiendo la causa:
 *  - El backend RESPONDIÓ con error (403, 404, 500…): NO se inventan datos. Las
 *    listas quedan vacías y se adjunta el error para que la UI lo diga; devolver el
 *    ejemplo aquí es lo que convertía un «no tienes permiso» en cinco proyectos
 *    falsos con pinta de reales.
 *  - No hubo respuesta (fetch rechazado, sin control-plane): modo demostración con
 *    los datos de ejemplo, que es para lo que se diseñó.
 */
function degrade<T>(err: unknown, demo: T): Sourced<T> {
  const info = httpErrorInfo(err);
  if (info === null) return { live: false, data: demo };
  return { live: false, data: (Array.isArray(demo) ? [] : demo) as T, error: info };
}

function client(session: SytueSession): CloudClient {
  return new CloudClient({ baseUrl: cloudBaseUrl(), auth: bearer(session.tokens.accessToken) });
}

/** HttpClient directo al control-plane para superficies sin metodo dedicado en el SDK. */
function http(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${cloudBaseUrl().replace(/\/$/, '')}/api/v1`,
    auth: bearer(session.tokens.accessToken),
  });
}

/**
 * Distingue en una MUTACION un error REAL del control-plane (respondio con un status
 * HTTP de error: quota_exceeded 402, conflict 409, forbidden 403, invalid_state...)
 * de un fallo de "no hay backend" (fetch rechazado / red). Solo el primero debe
 * llegar a la UI como error real; el segundo degrada a modo demostracion (live:false),
 * igual que las CARGAS. Devuelve {status, code} si es un HttpError; null en otro caso.
 * Lo consumen las rutas BFF (/api/*) para devolver el status y codigo reales.
 */
export function httpErrorInfo(err: unknown): { status: number; code: string } | null {
  if (!(err instanceof HttpError)) return null;
  let code = err.statusText || 'backend_error';
  try {
    // Envelope canónico Go (platform/pkg/httpx): {"error": {"code","message"}}.
    // Tolera también la forma plana {"error": "<code>"}.
    const e = (JSON.parse(err.body ?? '') as { error?: unknown }).error;
    if (typeof e === 'string' && e) {
      code = e;
    } else if (e && typeof e === 'object') {
      const c = (e as { code?: unknown }).code;
      if (typeof c === 'string' && c) code = c;
    }
  } catch {
    // Cuerpo no-JSON: se conserva statusText como codigo legible.
  }
  return { status: err.status, code };
}

// ---- Mapeos SDK -> modelos de vista ----

const FRAMEWORKS: Framework[] = ['astro', 'next', 'preact', 'static', 'node', 'sveltekit'];
/**
 * Los tipos que el control-plane emite de verdad (`domain.ServiceKind`) más el
 * `cache` de los datos de demostración.
 *
 * Faltaban `app` y `job`, y lo que hace este filtro con lo que no reconoce es
 * llamarlo `postgres`: TODA app del catálogo llegaba al panel disfrazada de base
 * de datos, así que la lista de «apps instaladas» salía siempre vacía y el botón
 * de instalar no sabía que ya lo estaba —se podía pedir dos veces la misma app y
 * el 409 llegaba del servidor sin que la pantalla lo hubiera podido evitar—.
 */
const SERVICE_KINDS: ServiceKind[] = [
  'postgres',
  'storage',
  'cache',
  'functions',
  'cron',
  'job',
  'worker',
  'auth',
  'app',
];

function normalizeFramework(value: unknown): Framework {
  return FRAMEWORKS.includes(value as Framework) ? (value as Framework) : 'static';
}

function normalizeServiceKind(value: unknown): ServiceKind {
  return SERVICE_KINDS.includes(value as ServiceKind) ? (value as ServiceKind) : 'postgres';
}

// Respuestas antiguas pueden seguir vivas en una caché offline con nombres
// camelCase que el control-plane ya no emite. Esta frontera valida esos alias
// sin contaminar el código actual con propiedades SDK marcadas como deprecated.
function legacyString(value: object, key: string): string | undefined {
  const field = (value as Record<string, unknown>)[key];
  return typeof field === 'string' ? field : undefined;
}

function legacyNumber(value: object, key: string): number | undefined {
  const field = (value as Record<string, unknown>)[key];
  return typeof field === 'number' ? field : undefined;
}

function legacyBoolean(value: object, key: string): boolean | undefined {
  const field = (value as Record<string, unknown>)[key];
  return typeof field === 'boolean' ? field : undefined;
}

function slugFromSdk(p: SdkProject): string {
  return p.slug || p.id;
}

function projectName(p: SdkProject): string {
  return p.name ?? legacyString(p, 'displayName') ?? slugFromSdk(p);
}

function projectRepo(p: SdkProject): string {
  const raw = p.repo_url ?? legacyString(p, 'repoUrl') ?? '';
  // Se recorta CUALQUIER host, no solo github.com. Antes la expresión nombraba
  // a GitHub y solo a GitHub, así que un repositorio de github salía bonito
  // —`org/repo`— y uno de nuestra propia forja salía con la URL entera. En un
  // producto soberano, el caso que peor se ve no puede ser el de casa.
  return raw.replace(/^https?:\/\/(www\.)?[^/]+\//, '').replace(/\.git$/, '');
}

/** Resuelve el UUID del proyecto a partir del slug del tenant. */
async function resolveProjectId(session: SytueSession, slug: string): Promise<string | undefined> {
  const projects = await client(session).listProjects();
  return projects.find((p) => slugFromSdk(p) === slug)?.id;
}

export function mapProject(p: SdkProject, services: SdkService[]): Project {
  const slug = slugFromSdk(p);
  return {
    id: p.id,
    slug,
    // El control-plane deriva la URL pública de SU configuración (AppsDomain) y no
    // la expone en el recurso proyecto —solo en la del deployment—, así que aquí no
    // queda más que recomponerla. ponytail: si algún día `Project` trae `url`, esta
    // línea sobra.
    productionUrl: `https://${slug}.apps.sytue.eu`,
    name: projectName(p),
    framework: normalizeFramework(p.framework),
    repo: projectRepo(p),
    productionRef: p.production_ref || 'main',
    region: 'fr-par',
    tenant: p.organization?.replace(/^org\//, '') ?? '',
    createdAt: p.created_at ?? legacyString(p, 'createdAt') ?? new Date().toISOString(),
    services: services.map((s) => normalizeServiceKind(s.kind)),
    // Solo vienen al CREAR (y en el detalle de un admin/owner): la lista no los
    // trae, así que en el resto de vistas quedan ausentes en vez de vacíos.
    ...(p.webhook_url ? { webhookUrl: p.webhook_url } : {}),
    ...(p.webhook_secret ? { webhookSecret: p.webhook_secret } : {}),
    runtime: {
      // Los defectos se replican aquí porque la API los OMITE cuando valen su
      // cero (omitempty): sin esto el formulario mostraría 0 instancias y puerto
      // 0, que no es lo que la app está corriendo. Deben coincidir con
      // domain.DefaultInstanceSizeName / DefaultAppPort / DefaultHealthPath.
      instanceSize: p.instance_size || 'basic',
      instanceCount: p.instance_count || 1,
      appPort: p.app_port || 8080,
      healthPath: p.health_path || '/',
      releaseCommand: p.release_command ?? '',
    },
    previewProtected: p.preview_protected === true,
    paused: p.paused === true,
    ...(p.http_rules ? { httpRules: p.http_rules } : {}),
  };
}

const SDK_STATUS_TO_VIEW: Record<SdkDeployment['status'], Deployment['status']> = {
  queued: 'queued',
  building: 'building',
  deploying: 'building',
  ready: 'ready',
  error: 'error',
  canceled: 'canceled',
};

// Los nombres del control-plane son snake_case; los camelCase que había aquí NO
// EXISTEN en su respuesta, así que este mapeador leía `undefined` en casi todo sin
// que nada fallara: TODO despliegue salía como preview (d.env nunca es
// 'production'), sin rama, sin commit y con una fecha inválida. Los alias legacy
// se conservan como respaldo mientras queden respuestas viejas en caché.
function mapDeployment(d: SdkDeployment, projectSlug: string): Deployment {
  const target = d.target ?? legacyString(d, 'env');
  const pr = d.pr_number ?? legacyNumber(d, 'prNumber');
  const sha = d.commit_sha ?? legacyString(d, 'commitSha') ?? '';
  const createdAt = d.created_at ?? legacyString(d, 'createdAt') ?? '';
  // No hay «finished»: un deployment listo sigue vivo. Lo que marca el final de la
  // puesta en marcha es ready_at.
  const readyAt = d.ready_at;
  return {
    id: d.id,
    project: projectSlug,
    status: SDK_STATUS_TO_VIEW[d.status] ?? 'queued',
    target: target === 'production' ? 'production' : 'preview',
    branch: d.git_ref || (pr ? `pr-${pr}` : 'main'),
    ...(pr ? { pr } : {}),
    commitSha: sha ? sha.slice(0, 7) : '',
    // Vacío es un valor legítimo, no un hueco que rellenar aquí: lo que NO nace
    // de un push (redeploy, rollback, plantilla, BYO) no tiene mensaje, y quien
    // pinta ya tiene el traductor para decirlo. Este mapeador construía
    // «Despliegue abc1234», que ni estaba traducido ni decía nada que no
    // dijera ya la columna del SHA, justo al lado.
    commitMessage: d.commit_message ?? '',
    author: d.created_by ?? '',
    url: d.url ?? `https://${projectSlug}.apps.sytue.eu`,
    createdAt,
    ...(d.updated_at ? { updatedAt: d.updated_at } : {}),
    ...(readyAt && createdAt
      ? {
          durationSec: Math.max(
            0,
            Math.round((Date.parse(readyAt) - Date.parse(createdAt)) / 1000),
          ),
        }
      : {}),
  };
}

const SERVICE_STATUS_TO_VIEW: Record<SdkService['status'], ManagedService['status']> = {
  provisioning: 'provisioning',
  ready: 'running',
  error: 'paused',
};

function mapService(s: SdkService): ManagedService {
  const kind = normalizeServiceKind(s.kind);
  const createdAt = s.created_at;
  // `config` es la parte NO sensible del servicio (el control-plane no manda ahí
  // credenciales). De ella salen las dos cosas que la pantalla necesita y el
  // nombre no dice: qué app del catálogo es, y si tiene URL pública.
  const appId = s.config?.app_id;
  const webUrl = s.config?.web_url;
  // Lo que el aprovisionamiento DEVUELVE de una función (url/host/state) y lo que
  // el usuario configuró de un cron/job (schedule, image, url). Va en `config`
  // porque no es sensible; sin subirlo aquí, el panel podía listar una función y
  // no decir a qué dirección responde.
  const url = s.config?.url;
  const schedule = s.config?.schedule;
  const image = s.config?.image;
  const targetUrl = kind === 'cron' ? s.config?.url : undefined;
  return {
    ...(s.id ? { id: s.id } : {}),
    kind,
    name: s.name || seedServiceName(kind),
    status: SERVICE_STATUS_TO_VIEW[s.status] ?? 'provisioning',
    ...(createdAt ? { createdAt } : {}),
    ...(appId ? { appId } : {}),
    ...(webUrl ? { webUrl } : {}),
    // Un cron HTTP guarda en `url` la dirección que LLAMA, no una suya pública:
    // enseñarla como «la URL del servicio» diría que el cron está publicado en
    // internet, que es lo contrario de lo que pasa.
    ...(url && kind !== 'cron' ? { url } : {}),
    ...(targetUrl ? { targetUrl } : {}),
    ...(schedule ? { schedule } : {}),
    ...(image ? { image } : {}),
  };
}

function seedServiceName(kind: ServiceKind): string {
  const found = seed.services.find((s) => s.kind === kind);
  return found?.name ?? `Sytue ${kind}`;
}

/**
 * Ámbitos de VISTA de una variable a partir del `scope` que devuelve la API.
 * `all` (el defecto del control-plane) significa «en los dos», que es lo que la
 * tabla estaba escribiendo a mano para TODAS —y por eso una credencial acotada a
 * preview se anunciaba como si también estuviera en producción—.
 */
function mapEnvScopes(scope: string | undefined): EnvVar['scopes'] {
  if (scope === 'production') return ['production'];
  if (scope === 'preview') return ['preview'];
  return ['production', 'preview'];
}

function mapEnvVar(e: SdkEnvVar): EnvVar {
  // `is_secret` es el nombre real del campo en la API; `secret` es el alias que
  // el SDK arrastraba y que nunca llegó con datos vivos.
  const secret = e.is_secret ?? legacyBoolean(e, 'secret') ?? false;
  return {
    key: e.key,
    preview: e.value ?? (secret ? '••••••••••••' : ''),
    secret,
    scopes: mapEnvScopes(e.scope),
    source: 'manual',
    updatedAt: new Date().toISOString(),
  };
}

/**
 * Compone una tile de uso EN EL IDIOMA de la peticion: la etiqueta sale del
 * catalogo (`metric.*`) y la cantidad se formatea por locale, asi que ni el
 * mapeo del control-plane ni el seed guardan texto de interfaz. La unidad de
 * display la fija la tabla de metricas (pricing.ts) y, si la metrica es nueva,
 * la que declare el backend. Conserva metric/quantity/unit para que usage.astro
 * estime el coste; una metrica desconocida se muestra con su clave cruda antes
 * que romper la vista.
 */
function usageTile(locale: Locale, m: seed.UsageSample): UsageMetric {
  const unit = METRICS[m.metric]?.unit ?? m.unit;
  return {
    label: metricLabel(appT(locale), m.metric),
    value: `${formatNumber(locale, m.quantity)}${unit ? ` ${unit}` : ''}`,
    metric: m.metric,
    quantity: m.quantity,
    unit,
    ...(m.delta !== undefined ? { delta: m.delta } : {}),
    ...(m.trend !== undefined ? { trend: m.trend } : {}),
  };
}

/**
 * Nombre del registro TXT de verificación de un dominio.
 *
 * La forma la fija el control-plane (handlers.AddDomain) y solo la devuelve en la
 * RESPUESTA DE ALTA; el listado trae el token pero no el nombre del registro. Se
 * recompone aquí para que un dominio pendiente siga diciendo qué hay que crear
 * después de recargar la página —si no, las instrucciones se pierden con el toast
 * y el dominio se queda sin verificar para siempre—.
 * ponytail: prefijo espejado del backend; si ListDomains llegara a devolver `dns`,
 * bórrese esta función y úsese el valor de la API.
 */
function txtRecordFor(hostname: string): string {
  return `_sytue-challenge.${hostname}`;
}

function mapDomain(d: SdkDomain, cname: string): Domain {
  const verified = d.status === 'verified' || d.status === 'active';
  const tls: Domain['tls'] =
    d.status === 'active' ? 'active' : d.status === 'failed' ? 'error' : 'pending';
  return {
    id: d.id,
    host: d.hostname,
    kind: 'custom',
    verified,
    tls,
    cname,
    // El token solo importa mientras el dominio esté sin verificar; una vez
    // verificado enseñarlo sería ruido (y un secreto vivo de más en el HTML).
    ...(!verified && d.verify_token
      ? { txtRecord: txtRecordFor(d.hostname), txtValue: d.verify_token }
      : {}),
    primary: d.is_primary,
  };
}

function mapOrg(o: SdkOrg): Organization {
  return { name: o.name, slug: o.slug, plan: o.plan, region: o.region };
}

// ---- Cargas de alto nivel (con fallback a seed) ----

/** Lista de proyectos del usuario/tenant. */
export async function loadProjects(session: SytueSession): Promise<Sourced<Project[]>> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const mapped = await Promise.all(
      sdkProjects.map(async (p) => {
        const services = await cloud.listServices(p.id).catch(() => [] as SdkService[]);
        return mapProject(p, services);
      }),
    );
    return { live: true, data: mapped };
  } catch (e) {
    return degrade(e, seed.projects);
  }
}

/** Detalle de un proyecto por slug (+ servicios). Undefined si no existe en live ni seed. */
export async function loadProject(
  session: SytueSession,
  slug: string,
): Promise<Sourced<Project> | undefined> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const match = sdkProjects.find((p) => slugFromSdk(p) === slug);
    if (!match) throw new Error('not_found_live');
    const services = await cloud.listServices(match.id).catch(() => [] as SdkService[]);
    return { live: true, data: mapProject(match, services) };
  } catch (e) {
    const p = seed.getProject(slug);
    // `degrade` en vez de tragarse el error: si el control-plane RESPONDIÓ 403 no es
    // modo demostración, y sin el error las subpáginas del proyecto anunciaban el
    // proyecto de ejemplo como una demo en vez de decir que no hay acceso.
    return p ? degrade(e, p) : undefined;
  }
}

/** Despliegues de un proyecto (mas recientes primero). */
export async function loadDeployments(
  session: SytueSession,
  slug: string,
): Promise<Sourced<Deployment[]>> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const match = sdkProjects.find((p) => slugFromSdk(p) === slug);
    if (!match) throw new Error('not_found_live');
    const list = await cloud.listDeployments(match.id, { limit: 30 });
    return { live: true, data: list.map((d) => mapDeployment(d, slug)) };
  } catch (e) {
    return degrade(e, seed.deploymentsFor(slug));
  }
}

/**
 * Despliegue cuyo contenedor está sirviendo producción: el que los logs en vivo y
 * la terminal deben atacar. `current` solo llega en el seed de demostración, así
 * que con datos reales se toma el producción más reciente que esté listo (la
 * lista viene ordenada de más nuevo a más viejo). Undefined si no hay ninguno:
 * las vistas lo dicen en vez de inventarse un destino.
 */
export function pickRuntimeDeployment(deployments: Deployment[]): Deployment | undefined {
  return (
    deployments.find((d) => d.current && d.status === 'ready') ??
    deployments.find((d) => d.target === 'production' && d.status === 'ready')
  );
}

/**
 * Promueve un despliegue previo a produccion (rollback / re-desplegar). El
 * control-plane expone la accion por deployment id (POST /deployments/{id}/rollback,
 * el tenant sale del JWT), asi que no hace falta resolver el proyecto. Devuelve
 * false si no hay backend vivo: la accion queda en demostracion, misma degradacion
 * que las cargas de este fichero. NUNCA expone tokens.
 */
/**
 * Cancela un despliegue en curso (`DELETE /deployments/{id}`).
 *
 * El control-plane sabía cancelar desde el principio y no había ningún botón: un
 * despliegue atascado se quedaba girando para siempre, sin más salida que esperar a que
 * el propio control-plane se rindiera.
 */
export async function cancelDeployment(
  session: SytueSession,
  deploymentId: string,
): Promise<boolean> {
  try {
    await http(session).text({
      method: 'DELETE',
      path: `/deployments/${encodeURIComponent(deploymentId)}`,
    });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/**
 * Publica en producción la imagen de una PREVIEW (sin recompilar).
 *
 * Es un endpoint distinto del de abajo a propósito: `rollback` exige que el
 * despliegue ya sea de producción, así que llamarlo con una preview devolvía 409
 * — el menú del panel ofrecía «promover» sobre previews y no funcionaba nunca.
 */
export async function promotePreviewDeployment(
  session: SytueSession,
  deploymentId: string,
): Promise<boolean> {
  try {
    await http(session).json<unknown>({
      method: 'POST',
      path: `/deployments/${encodeURIComponent(deploymentId)}/promote`,
      headers: { 'content-type': 'application/json' },
      body: '{}',
    });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/** Vuelve a publicar una producción anterior (rollback). */
export async function promoteDeployment(
  session: SytueSession,
  deploymentId: string,
): Promise<boolean> {
  try {
    await http(session).json<unknown>({
      method: 'POST',
      path: `/deployments/${encodeURIComponent(deploymentId)}/rollback`,
      headers: { 'content-type': 'application/json' },
      body: '{}',
    });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/**
 * Dispara un despliegue manual de producción (re-build del ref principal).
 * Devuelve false si no hay backend vivo.
 */
export async function triggerDeploy(session: SytueSession, slug: string): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    await client(session).createDeployment(projectId, { env: 'production' });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/**
 * Reinicia las instancias de la producción activa (mismo commit, misma imagen).
 *
 * Devuelve cuántas se reiniciaron, o undefined si no hay control-plane vivo: el
 * panel necesita distinguir «cero instancias corriendo» de «esto es una
 * demostración», y un 0 fingido diría lo primero cuando pasa lo segundo.
 */
export async function restartApp(session: SytueSession, slug: string): Promise<number | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return undefined;
    const res = await http(session).json<{ restarted: number }>({
      method: 'POST',
      path: `/projects/${encodeURIComponent(projectId)}/restart`,
    });
    return res.restarted;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane (409 pausado, 404…)
    return undefined; // sin backend vivo
  }
}

/**
 * Crea un proyecto conectado a un repositorio. Además de los campos básicos,
 * acepta los ajustes de compilación pre-rellenados por una plantilla de arranque
 * (build_strategy / build_command / install_command / output_dir); el
 * control-plane los persiste al crear el proyecto (POST /projects). Se envían por
 * el HttpClient crudo (no el método tipado del SDK) para poder incluir estos
 * campos opcionales. `framework` es string libre: el catálogo de plantillas puede
 * sugerir stacks que la vista aún no enumera (go, static, detect…).
 */
export interface CreateProjectInput {
  slug: string;
  name: string;
  repoUrl: string;
  framework?: string;
  buildStrategy?: string;
  buildCommand?: string;
  installCommand?: string;
  outputDir?: string;
  /**
   * Rama que se publica en producción. Vacío = el defecto del control-plane
   * (`main`). Sin esto no se puede desplegar un repo cuya rama principal se llame
   * de otra forma.
   */
  productionRef?: string;
  /**
   * Subdirectorio del repo donde vive la app (monorepo). Vacío = la raíz. Sin esto
   * un monorepo se construye entero desde arriba y falla.
   */
  rootDir?: string;
}

export async function createProject(
  session: SytueSession,
  input: CreateProjectInput,
): Promise<Sourced<Project>> {
  try {
    const body: Record<string, string> = {
      slug: input.slug,
      name: input.name,
      repo_url: input.repoUrl,
    };
    if (input.framework && input.framework !== 'auto') body.framework = input.framework;
    if (input.buildStrategy) body.build_strategy = input.buildStrategy;
    if (input.buildCommand) body.build_command = input.buildCommand;
    if (input.installCommand) body.install_command = input.installCommand;
    if (input.outputDir) body.output_dir = input.outputDir;
    if (input.productionRef) body.production_ref = input.productionRef;
    if (input.rootDir) body.root_dir = input.rootDir;
    const p = await http(session).json<SdkProject>({
      method: 'POST',
      path: '/projects',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
    return { live: true, data: mapProject(p, []) };
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    const demo: Project = {
      slug: input.slug,
      name: input.name,
      framework: normalizeFramework(input.framework),
      repo: input.repoUrl.replace(/^https?:\/\/(www\.)?github\.com\//, '').replace(/\.git$/, ''),
      productionRef: input.productionRef || 'main',
      productionUrl: `https://${input.slug}.apps.sytue.eu`,
      region: 'fr-par',
      tenant: 'demo',
      createdAt: new Date().toISOString(),
      services: [],
      // Los mismos defectos con los que el control-plane crearía el proyecto.
      runtime: {
        instanceSize: 'basic',
        instanceCount: 1,
        appPort: 8080,
        healthPath: '/',
        releaseCommand: '',
      },
    };
    return { live: false, data: demo };
  }
}

// ---- Plantillas de arranque (Wave 14) ----
//
// Catálogo curado de manifiestos declarativos por stack. El control-plane los
// APLICA enteros en una transacción (POST /projects:from-template): proyecto,
// servicios gestionados, variables y el primer despliegue. El catálogo es estático
// (GET /api/v1/templates) y el BFF solo proxya (invariante), snake_case →
// camelCase. Sin backend vivo: catálogo VACÍO (estado vacío honesto, nunca
// plantillas inventadas) y, por tanto, tampoco hay asistente que enseñar.

/** De dónde sale el valor de una variable declarada por la plantilla. */
export type TemplateEnvOrigin = 'value' | 'ask' | 'generate' | 'from_service';

/**
 * Variable declarada por la plantilla. `origin` es lo único que decide si hay que
 * preguntar algo: solo `ask` lleva `prompt` y solo `ask` es cosa del usuario.
 * `generate` lo crea y cifra el servidor (nadie inventa contraseñas) y
 * `from_service` lo inyecta el controlador desde el servicio gestionado.
 */
export interface StarterTemplateEnv {
  key: string;
  origin: TemplateEnvOrigin;
  /** Etiqueta que lee el usuario. Solo con origin=ask. */
  prompt?: string;
  /** El valor se cifra en reposo y NO se devuelve por la API. */
  secret: boolean;
}

const ENV_ORIGINS: TemplateEnvOrigin[] = ['value', 'ask', 'generate', 'from_service'];

/** Manifiesto de arranque del catálogo. */
export interface StarterTemplate {
  id: string;
  name: string;
  description: string;
  /** Stack sugerido (string libre: puede ser go, static, detect… fuera de Framework). */
  framework: string;
  /** '' = auto | dockerfile | buildpacks. */
  buildStrategy: string;
  buildCommand: string;
  installCommand: string;
  outputDir: string;
  /** Servicios gestionados que la plantilla APROVISIONA de verdad al aplicarla. */
  suggestedServices: ServiceKind[];
  tags: string[];
  /** Licencia del stack que se acaba ejecutando (allowlist permisiva del catálogo). */
  license: string;
  /** Revisión del manifiesto (semver): queda registrada en el proyecto creado. */
  version: string;
  /** Quién responde por la plantilla. */
  maintainer: string;
  /** Variables declaradas: de aquí salen las preguntas del asistente (y solo de aquí). */
  env: StarterTemplateEnv[];
  /** Documentación oficial del framework (opcional); nunca un repo clonable. */
  docsUrl?: string;
  /**
   * `true` si esta plantilla trae ESQUELETO: entonces Sytue DevX puede crear el
   * repositorio del usuario copiándolo, y no hace falta que traiga uno suyo.
   *
   * Viaja como booleano y no como «owner/repo» a propósito: el panel solo
   * necesita saber si ofrecer la opción, y el nombre del repositorio interno de
   * la plataforma no le aporta nada a quien lo lee.
   */
  hasStarter: boolean;
}

/** Catálogo de plantillas de arranque. Sin backend vivo: lista vacía (no seed). */
export async function loadTemplates(session: SytueSession): Promise<Sourced<StarterTemplate[]>> {
  try {
    const r = await http(session).json<{
      templates?: Array<{
        id: string;
        name: string;
        description?: string;
        framework?: string;
        build?: { strategy?: string; install?: string; build?: string; output_dir?: string };
        services?: string[];
        tags?: string[];
        docs_url?: string;
        starter_repo?: string;
        license?: string;
        version?: string;
        maintainer?: string;
        env?: Array<{ key?: string; origin?: string; prompt?: string; secret?: boolean }>;
      }>;
    }>({ path: '/templates' });
    const list = (r.templates ?? []).map((t): StarterTemplate => ({
      id: t.id,
      name: t.name,
      description: t.description ?? '',
      framework: t.framework ?? '',
      buildStrategy: t.build?.strategy ?? '',
      buildCommand: t.build?.build ?? '',
      installCommand: t.build?.install ?? '',
      outputDir: t.build?.output_dir ?? '',
      suggestedServices: (t.services ?? [])
        .filter((k) => SERVICE_KINDS.includes(k as ServiceKind))
        .map((k) => k as ServiceKind),
      tags: t.tags ?? [],
      license: t.license ?? '',
      version: t.version ?? '',
      maintainer: t.maintainer ?? '',
      hasStarter: Boolean(t.starter_repo),
      // Una variable sin clave u origen reconocible se descarta: preguntar por algo
      // que el control-plane no declara es un 400 garantizado al crear.
      env: (t.env ?? [])
        .filter((e) => e.key && ENV_ORIGINS.includes(e.origin as TemplateEnvOrigin))
        .map((e) => ({
          key: e.key!,
          origin: e.origin as TemplateEnvOrigin,
          secret: e.secret === true,
          ...(e.prompt ? { prompt: e.prompt } : {}),
        })),
      ...(t.docs_url ? { docsUrl: t.docs_url } : {}),
    }));
    return { live: true, data: list };
  } catch (e) {
    return degrade(e, []);
  }
}

/**
 * Alta COMPLETA desde plantilla: proyecto + servicios + variables + primer
 * despliegue, en una sola transacción del control-plane. Igual que el resto de
 * mutaciones: un error REAL se propaga con su status y su código, y la ausencia de
 * backend degrada a `live:false` SIN inventarse un proyecto (que es lo que haría
 * creer al usuario que tiene una app desplegada que no existe).
 */
export async function createProjectFromTemplate(
  session: SytueSession,
  input: {
    templateId: string;
    slug: string;
    name: string;
    repoUrl: string;
    answers: Record<string, string>;
  },
): Promise<{ live: boolean; result: SdkTemplateApplied | null }> {
  try {
    const result = await client(session).createProjectFromTemplate({
      template_id: input.templateId,
      slug: input.slug,
      name: input.name,
      repo_url: input.repoUrl,
      answers: input.answers,
    });
    return { live: true, result };
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return { live: false, result: null }; // sin backend vivo: no se ha creado nada
  }
}

/** Organización del tenant. */
export async function loadOrg(session: SytueSession): Promise<Sourced<Organization>> {
  try {
    const org = await client(session).getOrg();
    return { live: true, data: mapOrg(org) };
  } catch (e) {
    return degrade(e, { name: 'sytue', slug: 'sytue', plan: 'pro', region: 'fr-par' });
  }
}

/** Servicios gestionados de un proyecto (para overview). */
export async function loadServices(
  session: SytueSession,
  slug: string,
): Promise<Sourced<ManagedService[]>> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const match = sdkProjects.find((p) => slugFromSdk(p) === slug);
    if (!match) throw new Error('not_found_live');
    const list = await cloud.listServices(match.id);
    return { live: true, data: list.map(mapService) };
  } catch (e) {
    return degrade(e, seed.services);
  }
}

// ---- Backups + restore de Sytue Postgres (Wave 16) ----
//
// El control-plane expone los backups y el restore por SERVICE id (kind=postgres).
// Proxy PURO (snake_case → camelCase). Sin backend vivo: lista de backups VACÍA
// (estado vacío honesto) y el restore queda en demostración (live:false), misma
// degradación que el resto de mutaciones. NUNCA expone tokens.

/** Un backup disponible de un servicio Postgres gestionado. */
export interface ServiceBackup {
  key: string;
  /** Tamaño del objeto en bytes. */
  size: number;
  /** ISO de creación del backup. */
  lastModified: string;
}

/**
 * Resuelve el servicio Postgres gestionado de un proyecto (id + nombre) para las
 * acciones de backup/restore, que van por service id. El restore SOLO aplica a
 * kind=postgres. undefined si no hay backend vivo o el proyecto no tiene Postgres.
 */
export async function loadPostgresService(
  session: SytueSession,
  slug: string,
): Promise<{ id: string; name: string; backupSchedule: string } | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return undefined;
    const pg = (await client(session).listServices(projectId)).find((s) => s.kind === 'postgres');
    return pg
      ? {
          id: pg.id,
          name: pg.name || 'Sytue Postgres',
          // Vacío = el defecto del producto (3:00); la pantalla lo enseña
          // resuelto para que nadie tenga que adivinar qué significa el hueco.
          backupSchedule: pg.config?.backup_schedule ?? '',
        }
      : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Backups de un servicio Postgres (más reciente primero, tal como los ordena el
 * control-plane). Sin backend vivo: lista vacía (estado vacío honesto).
 */
export async function loadServiceBackups(
  session: SytueSession,
  serviceId: string,
): Promise<Sourced<ServiceBackup[]>> {
  try {
    const r = await http(session).json<{
      backups?: Array<{ key: string; size: number; last_modified: string }>;
    }>({ path: `/services/${encodeURIComponent(serviceId)}/backups` });
    const list = (r.backups ?? []).map((b) => ({
      key: b.key,
      size: b.size,
      lastModified: b.last_modified,
    }));
    return { live: true, data: list };
  } catch (e) {
    return degrade(e, []);
  }
}

/**
 * Restaura un servicio Postgres a un backup. DESTRUCTIVO: sobrescribe la base de
 * datos actual. false = sin backend vivo (queda en demostración). Error REAL del
 * control-plane (403 sin rol admin/owner, 409 estado inválido…) se propaga a la UI.
 */
export async function restoreService(
  session: SytueSession,
  serviceId: string,
  backup: string,
): Promise<boolean> {
  try {
    await http(session).json<unknown>({
      method: 'POST',
      path: `/services/${encodeURIComponent(serviceId)}/restore`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ backup }),
    });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/** Variables de entorno / secretos (valores secretos enmascarados por la API). */
export async function loadEnv(session: SytueSession, slug: string): Promise<Sourced<EnvVar[]>> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const match = sdkProjects.find((p) => slugFromSdk(p) === slug);
    if (!match) throw new Error('not_found_live');
    const list = await cloud.listEnv(match.id);
    return { live: true, data: list.map(mapEnvVar) };
  } catch (e) {
    return degrade(e, seed.envVars);
  }
}

/**
 * Crea o actualiza una variable/secreto (PUT upsert en el control-plane).
 * Devuelve false si no hay backend vivo: la mutacion queda en demostracion,
 * misma degradacion que las cargas de este fichero.
 */
export async function saveEnv(
  session: SytueSession,
  slug: string,
  input: { key: string; value: string; secret: boolean; scope?: SdkEnvScope },
): Promise<boolean> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const match = sdkProjects.find((p) => slugFromSdk(p) === slug);
    if (!match) throw new Error('not_found_live');
    await cloud.setEnv(match.id, input);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/** Elimina una variable/secreto del proyecto. false = sin backend vivo (demo). */
export async function deleteEnv(
  session: SytueSession,
  slug: string,
  key: string,
): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    await client(session).deleteEnv(projectId, key);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/**
 * Dominios del proyecto vía control-plane real.
 */
export async function loadDomains(session: SytueSession, slug: string): Promise<Sourced<Domain[]>> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const list = await client(session).listDomains(projectId);
    const cname = 'ingress.sytue.eu';
    return { live: true, data: list.map((d) => mapDomain(d, cname)) };
  } catch (e) {
    return degrade(e, seed.domains);
  }
}

/** Registros DNS que hay que crear en la zona del cliente, más el id del dominio. */
export interface AddedDomain {
  id?: string;
  cname?: string;
  txtRecord?: string;
  txtValue?: string;
}

/**
 * Añade un dominio personalizado y DEVUELVE sus instrucciones DNS.
 *
 * Devolvía un booleano, así que el CNAME real y —sobre todo— el TXT de
 * verificación se tiraban: la pantalla prometía un token que nunca entregaba y
 * ningún dominio propio podía verificarse. `live:false` = sin backend vivo
 * (queda en demostración), y entonces no hay registros que enseñar.
 */
export async function addDomain(
  session: SytueSession,
  slug: string,
  hostname: string,
): Promise<{ live: boolean; domain: AddedDomain }> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const res = await client(session).addDomain(projectId, hostname);
    return { live: true, domain: mapDns(res.domain?.id, res.dns) };
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return { live: false, domain: {} }; // sin backend vivo: degrada a demostración
  }
}

/** Copia solo los registros que la API entregó de verdad (nunca un hueco vacío). */
function mapDns(id: string | undefined, dns: DomainDnsInstructions | undefined): AddedDomain {
  return {
    ...(id ? { id } : {}),
    ...(dns?.cname_target ? { cname: dns.cname_target } : {}),
    ...(dns?.txt_record ? { txtRecord: dns.txt_record } : {}),
    ...(dns?.txt_value ? { txtValue: dns.txt_value } : {}),
  };
}

/**
 * Aprovisiona un servicio gestionado (Postgres, Storage, Functions, Cron) en un
 * proyecto. Sin esto el SDK tenía el método y NADIE lo llamaba: la sección de
 * «Base de datos» aparecía en el menú de un proyecto que no tenía ninguna y no
 * había forma de crearla desde la web. false = sin backend vivo (demostración).
 */
export async function createService(
  session: SytueSession,
  slug: string,
  // `kind` es el del SDK: el tipo impide aquí los que el control-plane NO
  // aprovisiona, en vez de dejar que la API los rechace con un 400.
  // `config` lleva lo específico del tipo (para `app`, el id del catálogo).
  input: { kind: SdkServiceKind; name: string; config?: Record<string, string> },
): Promise<{ live: boolean; service: ManagedService | null }> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const created = await client(session).createService(projectId, input);
    return { live: true, service: mapService(created) };
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return { live: false, service: null }; // sin backend vivo: degrada a demostración
  }
}

/**
 * Cambia la configuración de un servicio gestionado (PATCH /services/{id}).
 *
 * El `config` se mezcla en el servidor, así que aquí solo viajan las claves que
 * se editan. false = sin backend vivo (demostración).
 */
export async function updateManagedService(
  session: SytueSession,
  serviceId: string,
  patch: { name?: string; config?: Record<string, string> },
): Promise<boolean> {
  try {
    await client(session).updateService(serviceId, patch);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return false;
  }
}

/**
 * Da de baja un servicio gestionado (DELETE /services/{id}).
 *
 * Es DESTRUCTIVO —el reconciliador retira sus objetos del clúster, y en Postgres
 * eso incluye el volumen—, así que la confirmación es cosa de la interfaz; aquí
 * solo se propaga el error real (403 sin rol, 404 ya retirado). false = sin
 * backend vivo, la baja queda en demostración.
 */
export async function deleteManagedService(
  session: SytueSession,
  serviceId: string,
): Promise<boolean> {
  try {
    await client(session).deleteService(serviceId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/**
 * URL temporal para descargar un backup. undefined sin backend vivo (en
 * demostración no hay fichero que bajar, y devolver una URL falsa daría un error
 * del navegador en vez de una explicación).
 */
export async function serviceBackupDownloadURL(
  session: SytueSession,
  serviceId: string,
  backup: string,
): Promise<string | undefined> {
  try {
    const res = await http(session).json<{ url: string }>({
      method: 'GET',
      path: `/services/${encodeURIComponent(serviceId)}/backups/download?backup=${encodeURIComponent(backup)}`,
    });
    return res.url;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return undefined; // sin backend vivo
  }
}

/**
 * Ejecuta ahora una tarea programada o puntual. Devuelve false sin backend vivo.
 */
export async function runManagedService(
  session: SytueSession,
  serviceId: string,
): Promise<boolean> {
  try {
    await http(session).json<{ status: string }>({
      method: 'POST',
      path: `/services/${encodeURIComponent(serviceId)}/run`,
    });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/**
 * Rota el secreto del webhook del proyecto y devuelve el nuevo.
 *
 * Devuelve undefined sin backend vivo: en demostración no hay credencial que
 * emitir, y enseñar una inventada sería peor que no hacer nada —alguien la
 * pegaría en su proveedor git y los push fallarían sin explicación—.
 */
export async function rotateWebhookSecret(
  session: SytueSession,
  slug: string,
): Promise<{ secret: string; url: string } | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const res = await client(session).rotateWebhookSecret(projectId);
    return { secret: res.webhook_secret, url: res.webhook_url };
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return undefined;
  }
}

/**
 * Pide una copia de seguridad inmediata del Postgres del proyecto.
 *
 * No espera a que termine: el control-plane responde 202 y el Job corre en el
 * clúster. false = sin backend vivo (demostración).
 */
export async function backupServiceNow(session: SytueSession, serviceId: string): Promise<boolean> {
  try {
    await client(session).backupServiceNow(serviceId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return false;
  }
}

/** Una línea del registro de un servicio gestionado. */
export interface ServiceLogLine {
  ts: string;
  text: string;
}

/**
 * Registro reciente de un servicio gestionado (función, cron, job, app).
 *
 * Lista vacía sin backend vivo y también cuando el servicio no ha escrito nada:
 * son lo mismo de cara a la pantalla —no hay nada que enseñar— y la diferencia se
 * cuenta con `live`.
 */
export async function loadServiceLogs(
  session: SytueSession,
  serviceId: string,
  lines = 200,
): Promise<Sourced<ServiceLogLine[]>> {
  try {
    const raw = await http(session).json<{ lines?: Array<{ ts: string; text: string }> }>({
      path: `/services/${encodeURIComponent(serviceId)}/logs`,
      query: { lines: String(lines) },
    });
    return { live: true, data: (raw.lines ?? []).map((l) => ({ ts: l.ts, text: l.text })) };
  } catch (e) {
    return degrade(e, []);
  }
}

/**
 * Cierra (o rota) la contraseña de las previews del proyecto.
 *
 * Devuelve la credencial en claro UNA vez; undefined sin backend vivo, porque una
 * contraseña inventada aquí sería la que alguien intentaría usar en una preview
 * que no la pide.
 */
export async function protectPreviews(
  session: SytueSession,
  slug: string,
): Promise<{ username: string; password: string } | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    return await client(session).protectPreviews(projectId);
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return undefined;
  }
}

/** Vuelve a dejar las previews abiertas. false = sin backend vivo. */
export async function unprotectPreviews(session: SytueSession, slug: string): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    await client(session).unprotectPreviews(projectId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return false;
  }
}

/** Uso de la organización frente a los límites de su plan. */
export interface QuotaReport {
  plan: string;
  projects: { used: number; max: number };
  services: { used: number; max: number };
  postgresPvcSize: string;
  backupRetentionDays: number;
}

/**
 * Cuánto queda del plan (GET /quota).
 *
 * Sin backend vivo devuelve `undefined` y la tarjeta no se pinta: enseñar «0 de
 * 10 proyectos» en demostración sería una cifra inventada sobre el dinero de
 * alguien.
 */
export async function loadQuota(session: SytueSession): Promise<QuotaReport | undefined> {
  try {
    const raw = await http(session).json<{
      plan?: string;
      projects?: { used?: number; max?: number };
      services?: { used?: number; max?: number };
      postgres_pvc_size?: string;
      backup_retention_days?: number;
    }>({ path: '/quota' });
    return {
      plan: raw.plan ?? 'free',
      projects: { used: raw.projects?.used ?? 0, max: raw.projects?.max ?? 0 },
      services: { used: raw.services?.used ?? 0, max: raw.services?.max ?? 0 },
      postgresPvcSize: raw.postgres_pvc_size ?? '',
      backupRetentionDays: raw.backup_retention_days ?? 0,
    };
  } catch {
    return undefined;
  }
}

/** Consumo observado del proyecto, con el techo contra el que se lee. */
export interface ProjectMetrics {
  points: Array<{ at: string; cpuMilli: number; memoryBytes: number }>;
  memoryLimitBytes: number;
  cpuMilliLimit: number;
  instanceSize: string;
  windowHours: number;
}

/**
 * Serie de consumo real del proyecto (GET /projects/{id}/metrics).
 *
 * Sin backend vivo devuelve la serie VACÍA y no un ejemplo: una gráfica inventada
 * en la pantalla que justifica un cambio de tamaño sería la peor forma posible de
 * perder la confianza del cliente. La pantalla dice «aún no hay medidas».
 */
export async function loadProjectMetrics(
  session: SytueSession,
  slug: string,
  hours = 24,
): Promise<Sourced<ProjectMetrics>> {
  const vacio: ProjectMetrics = {
    points: [],
    memoryLimitBytes: 0,
    cpuMilliLimit: 0,
    instanceSize: '',
    windowHours: hours,
  };
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const raw = await http(session).json<{
      points?: Array<{ at: string; cpu_milli: number; memory_bytes: number }>;
      memory_limit_bytes?: number;
      cpu_milli_limit?: number;
      instance_size?: string;
      window_hours?: number;
    }>({
      path: `/projects/${encodeURIComponent(projectId)}/metrics`,
      query: { hours: String(hours) },
    });
    return {
      live: true,
      data: {
        points: (raw.points ?? []).map((p) => ({
          at: p.at,
          cpuMilli: p.cpu_milli ?? 0,
          memoryBytes: p.memory_bytes ?? 0,
        })),
        memoryLimitBytes: raw.memory_limit_bytes ?? 0,
        cpuMilliLimit: raw.cpu_milli_limit ?? 0,
        instanceSize: raw.instance_size ?? '',
        windowHours: raw.window_hours ?? hours,
      },
    };
  } catch (e) {
    return degrade(e, vacio);
  }
}

/** Lo que hace falta para que un push despliegue: dónde apuntar y con qué firmar. */
export interface PushDeployInfo {
  webhookUrl?: string;
  webhookSecret?: string;
}

/**
 * Datos de conexión del push-to-deploy de un proyecto.
 *
 * El control-plane los devuelve en `GET /projects/{id}` **solo a un humano
 * admin/owner** (nunca a un token de API ni a un miembro raso). El panel los
 * enseñaba únicamente en el toast del alta: cerrabas la pestaña y el secreto no
 * se volvía a ver, así que el despliegue automático se quedaba sin configurar y
 * no había forma de recuperarlo salvo recrear el proyecto.
 *
 * Devuelve el objeto VACÍO cuando el usuario no tiene rol para verlo: no es un
 * error, es la respuesta correcta, y la pantalla lo dice con esas palabras.
 */
export async function loadPushDeploy(
  session: SytueSession,
  slug: string,
): Promise<Sourced<PushDeployInfo>> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const p = await client(session).getProject(projectId);
    return {
      live: true,
      data: {
        ...(p.webhook_url ? { webhookUrl: p.webhook_url } : {}),
        ...(p.webhook_secret ? { webhookSecret: p.webhook_secret } : {}),
      },
    };
  } catch (e) {
    return degrade(e, {});
  }
}

/**
 * Da de baja un proyecto entero (DELETE /projects/{id}).
 *
 * Lo más destructivo que ofrece el panel: se lleva por delante los despliegues,
 * los servicios gestionados y los tokens acotados a ese proyecto. La confirmación
 * —escribir el nombre— es cosa de la interfaz; aquí solo se propaga el error real
 * (403 si no eres admin/owner). false = sin backend vivo (demostración).
 */
export async function deleteProject(session: SytueSession, slug: string): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    await client(session).deleteProject(projectId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/**
 * Cadena de conexión COMPLETA del Postgres del proyecto (con contraseña).
 *
 * undefined si no hay control-plane vivo: en demostración no existe esa base, y
 * devolver una cadena inventada llevaría a intentar conectarse a la nada.
 */
export async function dbConnectionString(
  session: SytueSession,
  slug: string,
): Promise<string | undefined> {
  try {
    // Por SLUG: estas rutas de BD aceptan la referencia del proyecto tal cual, sin
    // resolverla antes a UUID (un viaje menos).
    const res = await http(session).json<{ connection_string: string }>({
      method: 'GET',
      path: `/projects/${encodeURIComponent(slug)}/db/credentials`,
    });
    return res.connection_string;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // 403 sin rol, 404 sin base
    return undefined; // sin backend vivo
  }
}

/**
 * Clona un proyecto (configuración + variables) con otro slug.
 *
 * Devuelve cuántas variables se copiaron, o undefined si no hay backend vivo: en
 * demostración no se crea nada, y decir «clonado» sería mentira.
 */
export async function cloneProject(
  session: SytueSession,
  slug: string,
  destino: string,
): Promise<number | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return undefined;
    const res = await http(session).json<{ env_vars_copied: number }>({
      method: 'POST',
      path: `/projects/${encodeURIComponent(projectId)}/clone`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ slug: destino }),
    });
    return res.env_vars_copied;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // 409 slug cogido, 402 sin cuota…
    return undefined; // sin backend vivo
  }
}

/** Una imagen ya construida del proyecto, para levantar con ella una función/job. */
export interface BuiltImage {
  /** Referencia OCI por digest (lo que espera el control-plane en `config.image`). */
  ref: string;
  /** Commit del que salió, corto: es lo que identifica la imagen para un humano. */
  commit: string;
  gitRef: string;
  createdAt: string;
}

/**
 * Imágenes que el proyecto YA ha construido, de la más reciente a la más antigua.
 *
 * Existe para que crear una función no empiece por «pega aquí una referencia OCI»:
 * el usuario elige un commit suyo y el panel manda el digest. Solo entran las
 * builds terminadas con éxito y con imagen —las demás no se pueden desplegar—, y
 * sin backend vivo la lista sale VACÍA: una imagen inventada se traduciría en un
 * servicio que nunca arranca.
 */
export async function loadBuiltImages(
  session: SytueSession,
  slug: string,
  limit = 20,
): Promise<Sourced<BuiltImage[]>> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const builds = await client(session).listBuilds(projectId, limit);
    return {
      live: true,
      data: builds
        .filter((b) => b.status === 'succeeded' && !!b.image_ref)
        .map((b) => ({
          ref: b.image_ref as string,
          commit: (b.commit_sha || '').slice(0, 7),
          gitRef: b.git_ref || '',
          createdAt: b.created_at,
        })),
    };
  } catch (e) {
    return degrade(e, []);
  }
}

/**
 * Catálogo de apps open-source (GET /app-catalog).
 *
 * Sin backend vivo devuelve el catálogo VACÍO y no un ejemplo: aquí un dato
 * inventado sería una app que el usuario intentaría crear y que el control-plane
 * rechazaría, que es peor que no ofrecer nada.
 */
export async function loadAppCatalog(session: SytueSession): Promise<Sourced<SdkAppCatalog>> {
  const vacio: SdkAppCatalog = { apps: [], pending: [], allowed_licenses: [] };
  try {
    return { live: true, data: await client(session).appCatalog() };
  } catch (e) {
    return degrade(e, vacio);
  }
}

/** Elimina un dominio por UUID. false = modo demostración. */
export async function removeDomain(session: SytueSession, domainId: string): Promise<boolean> {
  try {
    await client(session).deleteDomain(domainId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/** Marca un dominio como principal. false = modo demostración. */
export async function setDomainPrimary(session: SytueSession, domainId: string): Promise<boolean> {
  try {
    await client(session).setPrimaryDomain(domainId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

// ---- Monitorización: uptime + alertas (Wave 2) ----
//
// A diferencia del resto del fichero, cuando NO hay backend vivo estas cargas
// degradan a lista VACÍA (live:false), nunca a datos seed inventados: la UI
// muestra su estado vacío honesto. Las mutaciones honran httpErrorInfo (error
// real del control-plane -> a la UI; sin backend -> demostración).

function mapUptimeCheck(c: SdkUptimeCheck): UptimeCheck {
  const status: UptimeStatus =
    c.last_status === 'up' ? 'up' : c.last_status === 'down' ? 'down' : 'unknown';
  return {
    id: c.id,
    name: c.name,
    targetUrl: c.target_url,
    intervalSeconds: c.interval_seconds,
    enabled: c.enabled,
    status,
    latencyMs: c.last_latency_ms,
    ...(c.last_checked_at ? { lastCheckedAt: c.last_checked_at } : {}),
  };
}

function mapAlertPolicy(p: SdkAlertPolicy): AlertPolicy {
  return {
    id: p.id,
    event: p.event,
    channelUrl: p.channel_url,
    enabled: p.enabled,
    orgScope: !p.project_id,
  };
}

/**
 * Catálogo de tamaños de instancia servido por el control-plane.
 *
 * Sin backend vivo hay que enseñar ALGO o el formulario de escala queda inservible
 * en modo demostración, así que se cae a una copia mínima —marcada `live: false`,
 * como el resto de la demo—. La copia solo lleva nombre y etiqueta: los recursos y
 * la memoria facturable no se replican para que nadie los lea de aquí creyendo que
 * son los de verdad.
 */
export async function loadInstanceSizes(
  session: SytueSession,
): Promise<Sourced<{ sizes: SdkInstanceSize[]; maxCount: number }>> {
  try {
    const cat = await client(session).listInstanceSizes();
    return { live: true, data: { sizes: cat.instance_sizes, maxCount: cat.max_count } };
  } catch (err) {
    return degrade(err, { sizes: DEMO_SIZES, maxCount: 50 });
  }
}

/**
 * Solo para el modo demostración (ver loadInstanceSizes).
 *
 * Los `label` van en castellano A PROPÓSITO: imitan lo que devuelve el backend
 * —`services/cloud/internal/domain/instancesize.go` los tiene así—, y el panel
 * ya no los pinta: traduce por `name` con `runtime.size.*` y deja el label del
 * servidor como respaldo para un tamaño que no conozca. Traducirlos aquí daría
 * una demo que no se parece a la realidad.
 */
const DEMO_SIZES: SdkInstanceSize[] = [
  { name: 'basic', label: 'Básico' },
  { name: 'standard', label: 'Estándar' },
  { name: 'pro', label: 'Pro' },
  { name: 'pro-xl', label: 'Pro XL' },
].map((s) => ({
  ...s,
  cpu_request: '',
  memory_request: '',
  cpu_limit: '',
  memory_limit: '',
  memory_gib: 0,
}));

/**
 * Cambia la escala y el contrato de ejecución de un proyecto. Devuelve false si no
 * hay control-plane vivo (modo demostración); un error REAL del backend se propaga
 * para que la UI muestre el motivo en vez de fingir que se guardó.
 */
export async function updateProjectRuntime(
  session: SytueSession,
  slug: string,
  patch: {
    instance_size?: string;
    instance_count?: number;
    app_port?: number;
    health_path?: string;
    // Repositorio y rama: la fuente del código. Se editan por el MISMO camino
    // que la escala —`PATCH /projects/{id}`— porque son el mismo recurso; un
    // verbo aparte solo duplicaría la validación y la auditoría.
    repo_url?: string;
    production_ref?: string;
    release_command?: string;
    paused?: boolean;
    http_rules?: {
      redirects?: Array<{ from: string; to: string; status?: number }>;
      headers?: Record<string, string>;
    };
  },
): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('project_not_found');
    await client(session).updateProject(projectId, patch);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/** Lo que devuelve el control-plane al sondear las rutas de salud de una app. */
export interface HealthProbeReport {
  url: string;
  results: Array<{ path: string; status: number; healthy: boolean; latency_ms: number }>;
  recommended: string;
  weak: boolean;
  reachable: boolean;
}

/**
 * Sondea las rutas de salud de convención contra la app del proyecto.
 *
 * Devuelve undefined si no hay control-plane vivo. Aquí NO se degrada a un informe
 * de mentira: el sentido de esto es que lo que se enseñe esté MEDIDO, y un informe
 * inventado llevaría a configurar una ruta que la app no sirve —o sea, a que
 * Kubernetes la reinicie en bucle—.
 */
export async function probeHealthPaths(
  session: SytueSession,
  slug: string,
): Promise<HealthProbeReport | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return undefined;
    return await http(session).json<HealthProbeReport>({
      method: 'POST',
      path: `/projects/${encodeURIComponent(projectId)}/probe-health`,
    });
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo
    return undefined; // sin backend vivo
  }
}

/**
 * Nombre libre sugerido para un proyecto nuevo.
 *
 * Sin control-plane vivo devuelve undefined en vez de inventarse uno: el valor de
 * la sugerencia es que está COMPROBADA contra el índice global, y una generada aquí
 * llevaría derecha al 409 que viene a evitar.
 */
export async function suggestProjectSlug(
  session: SytueSession,
  lang?: SlugLanguageOptions,
): Promise<SlugSuggestion | undefined> {
  try {
    return await client(session).suggestProjectSlug(lang);
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return undefined;
  }
}

/**
 * Disponibilidad del nombre que el usuario está escribiendo, con alternativas
 * libres si ya está cogido.
 *
 * Sin control-plane vivo devuelve undefined, NUNCA un «disponible» inventado: solo
 * el índice global sabe si un slug está libre, y afirmarlo desde aquí sería mentirle
 * al usuario justo en el dato por el que preguntó.
 */
export async function checkProjectSlug(
  session: SytueSession,
  slug: string,
  lang?: SlugLanguageOptions,
): Promise<SlugCheck | undefined> {
  try {
    return await client(session).checkProjectSlug(slug, lang);
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return undefined;
  }
}

/**
 * Empieza el alta creando el repositorio del usuario en Sytue Git.
 *
 * Devuelve a dónde mandarlo a autorizar, no un proyecto: el repositorio se crea
 * EN SU CUENTA de la forja, así que primero tiene que consentirlo. `undefined` =
 * sin control-plane vivo, y entonces no hay nada que autorizar.
 */
export async function startScaffold(
  session: SytueSession,
  body: { template_id: string; slug: string; name: string; repo_name: string },
): Promise<string | undefined> {
  const res = await http(session).postJson<{ authorize_url?: string }>('/projects:scaffold', body);
  return res.authorize_url;
}

/**
 * Deduce cómo desplegar un repositorio, para que el alta no lo pregunte.
 *
 * Devuelve `undefined` cuando no hay control-plane vivo, y `{detected:false}`
 * cuando lo hay pero no se pudo mirar el repositorio. Los dos casos acaban igual
 * en la interfaz —se enseñan los campos— pero se distinguen aquí porque solo el
 * segundo trae un motivo que se le puede decir al usuario.
 */
export async function inspectRepo(
  session: SytueSession,
  repoUrl: string,
  rootDir = '',
): Promise<RepoInspection | undefined> {
  try {
    return await client(session).inspectRepo(repoUrl, rootDir);
  } catch {
    // Sin distinguir causa: mirar el repositorio es una comodidad, y un fallo
    // suyo no puede impedir crear el proyecto.
    return undefined;
  }
}

/** Sondas de uptime de un proyecto. Sin backend vivo: lista vacía (no seed). */
export async function loadUptimeChecks(
  session: SytueSession,
  slug: string,
): Promise<Sourced<UptimeCheck[]>> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const list = await client(session).listUptimeChecks(projectId);
    return { live: true, data: list.map(mapUptimeCheck) };
  } catch (e) {
    return degrade(e, []);
  }
}

/** Políticas de alerta del tenant. Sin backend vivo: lista vacía (no seed). */
export async function loadAlertPolicies(session: SytueSession): Promise<Sourced<AlertPolicy[]>> {
  try {
    const list = await client(session).listAlertPolicies();
    return { live: true, data: list.map(mapAlertPolicy) };
  } catch (e) {
    return degrade(e, []);
  }
}

/**
 * Crea una sonda de uptime en un proyecto. Devuelve la sonda creada (con su id
 * real del control-plane) para que la UI pinte una fila accionable al instante.
 * check=null + live=false = sin backend vivo (la UI sintetiza una fila demo).
 */
export async function createUptimeCheck(
  session: SytueSession,
  slug: string,
  input: { name: string; targetUrl: string; intervalSeconds: number },
): Promise<{ live: boolean; check: UptimeCheck | null }> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const created = await client(session).createUptimeCheck(projectId, {
      name: input.name,
      target_url: input.targetUrl,
      interval_seconds: input.intervalSeconds,
    });
    return { live: true, check: mapUptimeCheck(created) };
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return { live: false, check: null }; // sin backend vivo: degrada a demostración
  }
}

/** Actualiza una sonda por ID (parcial). false = modo demostración. */
export async function updateUptimeCheck(
  session: SytueSession,
  checkId: string,
  input: { name?: string; targetUrl?: string; intervalSeconds?: number; enabled?: boolean },
): Promise<boolean> {
  try {
    // `enabled` es lo que deja pausar una sonda en un mantenimiento programado:
    // el sondeador solo recoge las activas, así que pausarla calla también sus
    // alertas de caída sin perder su configuración.
    const patch: {
      name?: string;
      target_url?: string;
      interval_seconds?: number;
      enabled?: boolean;
    } = {};
    if (input.name !== undefined) patch.name = input.name;
    if (input.targetUrl !== undefined) patch.target_url = input.targetUrl;
    if (input.intervalSeconds !== undefined) patch.interval_seconds = input.intervalSeconds;
    if (input.enabled !== undefined) patch.enabled = input.enabled;
    await client(session).updateUptimeCheck(checkId, patch);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/** Elimina una sonda por ID. false = modo demostración. */
export async function deleteUptimeCheck(session: SytueSession, checkId: string): Promise<boolean> {
  try {
    await client(session).deleteUptimeCheck(checkId);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/**
 * Crea una política de alerta de alcance ORGANIZACIÓN (casa cualquier proyecto
 * del tenant): las alertas de despliegue/uptime son un enrutado a webhook a nivel
 * de organización. false = modo demostración.
 */
export async function createAlertPolicy(
  session: SytueSession,
  input: { event: AlertEvent; channelUrl: string },
): Promise<{ live: boolean; policy: AlertPolicy | null }> {
  try {
    const created = await client(session).createAlertPolicy({
      event: input.event,
      channel_url: input.channelUrl,
    });
    return { live: true, policy: mapAlertPolicy(created) };
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return { live: false, policy: null }; // sin backend vivo: degrada a demostración
  }
}

/** Elimina una política de alerta por ID. false = modo demostración. */
export async function deleteAlertPolicy(session: SytueSession, id: string): Promise<boolean> {
  try {
    await client(session).deleteAlertPolicy(id);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

export interface StorageView {
  bucket: string;
  endpoint: string;
  accessKey: string;
  region: string;
  objects: Array<{ key: string; size: string; modified: string }>;
}

/** Storage gestionado del proyecto. */
export async function loadStorage(
  session: SytueSession,
  locale: Locale,
  slug: string,
): Promise<Sourced<StorageView>> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const res = await client(session).listStorageObjects(projectId);
    return {
      live: true,
      data: {
        bucket: res.storage.bucket,
        endpoint: res.storage.endpoint,
        accessKey: res.storage.access_key,
        region: res.storage.region,
        objects: res.objects.map((o) => ({
          key: o.key,
          size: formatBytes(locale, o.size),
          // La fecha va POR LOCALE: clavada en es-ES, quien navega en alemán veía
          // sus objetos fechados a la española.
          modified: formatDate(locale, o.last_modified, { day: 'numeric', month: 'short' }),
        })),
      },
    };
  } catch {
    return {
      live: false,
      data: {
        bucket: `${slug}-assets`,
        endpoint: 'https://s3.fr-par.sytue.eu',
        accessKey: 'SCW••••',
        region: 'fr-par',
        objects: [],
      },
    };
  }
}

/**
 * Sube un objeto al bucket del proyecto.
 *
 * El fichero pasa POR EL SERVIDOR: se pide una URL prefirmada al control-plane y
 * el PUT lo hace este proceso, no el navegador. Subirlo directo sería mejor —no
 * ocupa memoria aquí— pero exige CORS en el bucket, y los buckets ya
 * aprovisionados no lo tienen: la subida fallaría en el navegador con un error de
 * origen cruzado que además no dice nada.
 *
 * ponytail: tope de 25 MiB por fichero para que el BFF no cargue en memoria lo
 * que le echen. Cuando el provisioner ponga CORS en el bucket (y un paso
 * autorreparador para los existentes), esto se sustituye por el PUT directo desde
 * el navegador y el tope desaparece.
 */
export const MAX_UPLOAD_BYTES = 25 * 1024 * 1024;

export async function uploadStorageObject(
  session: SytueSession,
  slug: string,
  key: string,
  body: ArrayBuffer,
  contentType: string,
): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    const url = await client(session).presignStorage(projectId, key, 'PUT');
    const res = await fetch(url, {
      method: 'PUT',
      body,
      headers: { 'content-type': contentType || 'application/octet-stream' },
    });
    if (!res.ok) throw new Error(`s3_put_${res.status}`);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo: degrada a demostración
  }
}

/**
 * URL prefirmada de DESCARGA de un objeto (válida 15 minutos).
 *
 * Se devuelve la URL en vez de servir el fichero para que la descarga vaya
 * directa del almacenamiento al navegador: pasar por aquí un objeto de gigabytes
 * no aporta nada y ocuparía el servidor mientras dura.
 */
export async function presignDownload(
  session: SytueSession,
  slug: string,
  key: string,
): Promise<string | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    return await client(session).presignStorage(projectId, key, 'GET');
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return undefined;
  }
}

/** Borra un objeto del bucket del proyecto (destructivo; exige admin/owner). */
export async function deleteStorageObject(
  session: SytueSession,
  slug: string,
  key: string,
): Promise<boolean> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) throw new Error('not_found_live');
    await client(session).deleteStorageObject(projectId, key);
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err;
    return false;
  }
}

// ---- Sytue DB Studio ----

/** Metadatos de tablas del Postgres gestionado (introspeccion via control-plane). */
export async function loadDbTables(
  session: SytueSession,
  slug: string,
): Promise<Sourced<DbTable[]>> {
  try {
    const list = await http(session).json<DbTable[]>({
      path: `/projects/${encodeURIComponent(slug)}/db/tables`,
    });
    return { live: true, data: Array.isArray(list) && list.length > 0 ? list : seed.dbTables };
  } catch (e) {
    return degrade(e, seed.dbTables);
  }
}

/** Primeras filas de una tabla (paginado por el control-plane). */
export async function loadDbRows(
  session: SytueSession,
  slug: string,
  schema: string,
  name: string,
  limit = 50,
): Promise<Sourced<DbRows>> {
  try {
    const rows = await http(session).json<DbRows>({
      path: `/projects/${encodeURIComponent(slug)}/db/tables/${encodeURIComponent(schema)}.${encodeURIComponent(name)}/rows`,
      query: { limit },
    });
    if (!rows || !Array.isArray(rows.columns)) throw new Error('bad_shape');
    return { live: true, data: rows };
  } catch (e) {
    return degrade(e, seed.tableRows(schema, name));
  }
}

/** Info de la API auto-generada (PostgREST) y credenciales enmascaradas. */
export async function loadDbApiInfo(
  session: SytueSession,
  slug: string,
): Promise<Sourced<DbApiInfo>> {
  try {
    const info = await http(session).json<DbApiInfo>({
      path: `/projects/${encodeURIComponent(slug)}/db/api`,
    });
    if (!info || typeof info.restUrl !== 'string') throw new Error('bad_shape');
    return { live: true, data: info };
  } catch (e) {
    return degrade(e, seed.dbApiInfo);
  }
}

/**
 * Ejecuta SQL contra el Postgres del proyecto (rol acotado, solo lectura por
 * defecto) via el control-plane. Si no hay backend, evalua SELECT en el seed
 * como demostracion. Las escrituras solo tienen efecto contra el backend real.
 */
export async function runSql(
  session: SytueSession,
  slug: string,
  sql: string,
  locale: Locale,
): Promise<SqlResult> {
  try {
    const res = await http(session).json<SqlResult>({
      method: 'POST',
      path: `/projects/${encodeURIComponent(slug)}/db/query`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ sql }),
    });
    if (!res || !Array.isArray(res.columns)) throw new Error('bad_shape');
    return res;
  } catch {
    return seed.runDemoSql(sql, appT(locale));
  }
}

/**
 * Comienzo del mes en curso (UTC, ISO), que es la ventana que se pide siempre.
 *
 * Sin `from`, el control-plane devuelve los ÚLTIMOS 30 DÍAS RODANTES; pero el
 * límite de gasto es mensual, las facturas van por mes natural y
 * `GET /billing/account` calcula el gasto *month-to-date*. Aceptar el defecto
 * ponía dos importes distintos bajo la misma palabra —«el período»— en dos
 * pantallas contiguas, y el que el cliente compara con su factura es el del mes.
 */
function monthStartISO(now: Date = new Date()): string {
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1)).toISOString();
}

/** Métricas de uso del tenant en el mes en curso (GET /usage). Fallback al seed. */
export async function loadUsage(
  session: SytueSession,
  locale: Locale,
  _slug?: string,
): Promise<Sourced<UsageMetric[]>> {
  const demo = (): UsageMetric[] => seed.usage.map((m) => usageTile(locale, m));
  try {
    const list = await client(session).getUsage({ from: monthStartISO() });
    // CERO filas es un dato VÁLIDO —una organización recién creada no ha consumido
    // nada—, no una ausencia de datos. Devolver aquí el seed con `live: true` hacía
    // que `DemoBanner` (que solo mira `live`) callara y la organización viera 128 min
    // de build, 342 GB·h y 18 GB inventados como su consumo real, alimentando además
    // el estimador de coste de /usage y /billing. El seed solo vale para el modo
    // demostración de verdad, que llega por `degrade()` con `live: false`.
    return {
      live: true,
      data: list.map((r: SdkUsage) =>
        usageTile(locale, { metric: r.metric, quantity: r.total, unit: r.unit }),
      ),
    };
  } catch (e) {
    return degrade(e, demo());
  }
}

/** Consumo de UN proyecto, ya sumado por métrica. `slug` vacío = de la organización. */
export interface ProjectUsage {
  projectId: string;
  name: string;
  slug: string;
  metrics: Array<{ metric: string; total: number; unit: string }>;
}

/**
 * Consumo del periodo repartido POR PROYECTO (control-plane GET /usage/detail).
 *
 * El agregado por métrica dice cuánto se ha gastado; este dice quién lo gasta, que es
 * la pregunta que se hace de verdad cuando la factura sube. Los registros llegan sin
 * agregar y se suman aquí: el control-plane no agrupa por proyecto y hacerlo en el
 * cliente evita una ruta nueva para lo mismo.
 *
 * El consumo SIN proyecto (el de la propia organización) no se descarta: se agrupa
 * aparte. Esconderlo haría que las partes no sumaran el total y quien compare con la
 * factura pensaría que falta algo.
 */
export async function loadUsageByProject(
  session: SytueSession,
  projects: readonly Project[],
  sinProyecto: string,
): Promise<Sourced<ProjectUsage[]>> {
  try {
    // Misma ventana que loadUsage: si las partes cubrieran un mes rodante y el
    // total el mes natural, el reparto por proyecto no sumaría el total.
    const records: SdkUsageRecord[] = await client(session).getUsageDetail({
      from: monthStartISO(),
    });
    type Suma = Map<string, { total: number; unit: string }>;
    const porProyecto = new Map<string, Suma>();
    for (const r of records) {
      const key = r.project_id ?? '';
      const metricas: Suma =
        porProyecto.get(key) ?? new Map<string, { total: number; unit: string }>();
      const previo = metricas.get(r.metric);
      metricas.set(r.metric, {
        total: (previo?.total ?? 0) + (Number(r.quantity) || 0),
        unit: r.unit || (previo?.unit ?? ''),
      });
      porProyecto.set(key, metricas);
    }
    const nombre = new Map(projects.flatMap((p) => (p.id ? [[p.id, p] as const] : [])));
    const out: ProjectUsage[] = [...porProyecto].map(([id, metricas]) => {
      const p = nombre.get(id);
      return {
        projectId: id,
        // Un proyecto borrado deja su consumo: se enseña el identificador antes que
        // omitir la línea, que descuadraría el total.
        name: p?.name ?? (id === '' ? sinProyecto : id),
        slug: p?.slug ?? '',
        metrics: [...metricas].map(([metric, v]) => ({ metric, total: v.total, unit: v.unit })),
      };
    });
    return { live: true, data: out };
  } catch (e) {
    return degrade(e, []);
  }
}

// ---- Facturación (Wave 10) ----
//
// Proxy PURO al control-plane (GET /billing/*, PUT /billing/spend-limit): el BFF
// nunca lleva lógica de precios (invariante). Snake_case de la API → camelCase de
// la vista. Sin backend vivo: cuenta en cero + pricing = tabla local (pricing.ts,
// fallback documentado) + facturas vacías (estado vacío honesto, nunca inventadas).

/** Cuenta de facturación: plan, límite de gasto y gasto acumulado del período (€). */
export interface BillingAccount {
  plan: string;
  /** Límite de gasto mensual en €, o null si no hay tope configurado. */
  spendLimitEur: number | null;
  /** Gasto acumulado del período en curso (€). */
  currentSpendEur: number;
}

/** Precio por unidad de una métrica facturable (tal como lo fija el control-plane). */
export interface PricingMetric {
  metric: string;
  unit: string;
  priceEur: number;
}

/** Plan de suscripción: cuota base mensual + cuota incluida por métrica.
 *  `included` es el MAPA que devuelve el control-plane (build_minutes,
 *  compute_gb_hours, storage_gb); el texto para pintar lo compone
 *  describeIncluded() en lib/pricing.ts, para que no haya dos redacciones. */
export interface BillingPlan {
  id: string;
  baseEur: number;
  included: Record<string, number>;
  /** Puestos de suite que EXIGE el plan (Free = 1): Cloud no se vende suelto. */
  minSuiteSeats: number;
}

/** Suscripción a la suite Sytue: precio de entrada a la plataforma. */
export interface SuiteSubscription {
  seatPriceEur: number;
  products: string[];
}

export interface BillingPricing {
  metrics: PricingMetric[];
  plans: BillingPlan[];
  suite: SuiteSubscription;
}

/** Resumen de una factura para el listado. */
export interface InvoiceSummary {
  id: string;
  periodFrom: string;
  periodTo: string;
  status: string;
  totalEur: number;
}

/** Línea de desglose de una factura: consumo de una métrica × precio unitario. */
export interface InvoiceLine {
  metric: string;
  quantity: number;
  unitPriceEur: number;
  subtotalEur: number;
}

export interface Invoice extends InvoiceSummary {
  lines: InvoiceLine[];
}

/** Fallback de pricing (sin backend): tabla local pricing.ts + planes de referencia. */
const FALLBACK_PRICING: BillingPricing = {
  metrics: Object.entries(METRICS).map(([metric, m]) => ({
    metric,
    unit: m.unit,
    priceEur: m.pricePerUnit,
  })),
  plans: [
    // Espejo de planSpecs en services/cloud/internal/domain/billing.go. El fallback
    // anunciaba Pro 49 €/Team 199 € cuando se factura 19 € y 49 €: un precio
    // inventado en la UI es un problema comercial, no un detalle de maqueta.
    {
      id: 'free',
      baseEur: 0,
      included: { build_minutes: 100, compute_gb_hours: 100, storage_gb: 1 },
      minSuiteSeats: 1, // Free exige suscripción a la suite: Cloud no se vende suelto
    },
    {
      id: 'pro',
      baseEur: 19,
      included: { build_minutes: 1000, compute_gb_hours: 1000, storage_gb: 25 },
      minSuiteSeats: 0,
    },
    {
      id: 'team',
      baseEur: 49,
      included: { build_minutes: 3000, compute_gb_hours: 4000, storage_gb: 100 },
      minSuiteSeats: 0,
    },
  ],
  suite: { seatPriceEur: SUITE_SEAT_PRICE_EUR, products: SUITE_PRODUCTS },
};

/** Cuenta de facturación del tenant. Sin backend vivo: cuenta en cero (no inventa gasto). */
export async function loadBillingAccount(session: SytueSession): Promise<Sourced<BillingAccount>> {
  try {
    const r = await http(session).json<{
      plan?: string;
      spend_limit_eur?: number | null;
      current_spend_eur?: number;
    }>({ path: '/billing/account' });
    return {
      live: true,
      data: {
        plan: r.plan ?? 'free',
        spendLimitEur: typeof r.spend_limit_eur === 'number' ? r.spend_limit_eur : null,
        currentSpendEur: r.current_spend_eur ?? 0,
      },
    };
  } catch (e) {
    return degrade(e, { plan: 'free', spendLimitEur: null, currentSpendEur: 0 });
  }
}

/** Precios + planes vigentes. Fuente autoritativa; sin backend degrada a pricing.ts. */
export async function loadBillingPricing(session: SytueSession): Promise<Sourced<BillingPricing>> {
  try {
    // Nombres EXACTOS del control-plane (handlers_billing.GetPricing): el precio
    // viaja como `price_per_unit_eur` y el plan se identifica por `plan`, no `id`.
    // Antes se leían `price_eur`/`id`, así que todos los precios llegaban undefined
    // —y la comprobación de forma no lo detectaba: la página los pintaba como si
    // fueran datos vivos—.
    const r = await http(session).json<{
      metrics?: Array<{ metric: string; unit: string; price_per_unit_eur: number }>;
      plans?: Array<{
        plan: string;
        base_eur: number;
        included?: Record<string, number>;
        min_suite_seats?: number;
      }>;
      suite?: { seat_price_eur: number; products: string[] };
    }>({ path: '/billing/pricing' });
    if (!Array.isArray(r.metrics) || r.metrics.length === 0) throw new Error('bad_shape');
    if (r.metrics.some((m) => typeof m.price_per_unit_eur !== 'number')) {
      throw new Error('bad_shape'); // sin precio no se pinta: mejor el fallback declarado
    }
    return {
      live: true,
      data: {
        metrics: r.metrics.map((m) => ({
          metric: m.metric,
          unit: m.unit,
          priceEur: m.price_per_unit_eur,
        })),
        plans: (r.plans ?? []).map((p) => ({
          id: p.plan,
          baseEur: p.base_eur,
          included: p.included ?? {},
          minSuiteSeats: p.min_suite_seats ?? 0,
        })),
        suite: {
          seatPriceEur: r.suite?.seat_price_eur ?? SUITE_SEAT_PRICE_EUR,
          products: r.suite?.products ?? SUITE_PRODUCTS,
        },
      },
    };
  } catch (e) {
    return degrade(e, FALLBACK_PRICING);
  }
}

/** Facturas del tenant (más recientes primero). Sin backend vivo: lista vacía (no seed). */
export async function loadInvoices(session: SytueSession): Promise<Sourced<InvoiceSummary[]>> {
  try {
    const r = await http(session).json<{
      invoices?: Array<{
        id: string;
        period_from: string;
        period_to: string;
        status: string;
        total_eur: number;
      }>;
    }>({ path: '/billing/invoices' });
    const list = (r.invoices ?? []).map(mapInvoiceSummary);
    return { live: true, data: list };
  } catch (e) {
    return degrade(e, []);
  }
}

/** Detalle de una factura con su desglose de líneas. undefined si no existe / sin backend. */
export async function loadInvoice(session: SytueSession, id: string): Promise<Invoice | undefined> {
  try {
    const r = await http(session).json<{
      id: string;
      period_from: string;
      period_to: string;
      status: string;
      total_eur: number;
      lines?: Array<{
        metric: string;
        quantity: number;
        unit_price_eur: number;
        subtotal_eur: number;
      }>;
    }>({ path: `/billing/invoices/${encodeURIComponent(id)}` });
    return {
      ...mapInvoiceSummary(r),
      lines: (r.lines ?? []).map((l) => ({
        metric: l.metric,
        quantity: l.quantity,
        unitPriceEur: l.unit_price_eur,
        subtotalEur: l.subtotal_eur,
      })),
    };
  } catch {
    return undefined;
  }
}

function mapInvoiceSummary(r: {
  id: string;
  period_from: string;
  period_to: string;
  status: string;
  total_eur: number;
}): InvoiceSummary {
  return {
    id: r.id,
    periodFrom: r.period_from,
    periodTo: r.period_to,
    status: r.status,
    totalEur: r.total_eur,
  };
}

/**
 * Fija el límite de gasto mensual (€) del tenant. Devuelve false si no hay backend
 * vivo (queda en demostración, misma degradación que el resto de mutaciones).
 */
export async function setSpendLimit(
  session: SytueSession,
  spendLimitEur: number,
): Promise<boolean> {
  try {
    await http(session).json<unknown>({
      method: 'PUT',
      path: '/billing/spend-limit',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ spend_limit_eur: spendLimitEur }),
    });
    return true;
  } catch (err) {
    if (httpErrorInfo(err)) throw err; // error real del control-plane: propágalo a la UI
    return false; // sin backend vivo (fetch rechazado/red): degrada a demostración
  }
}

/**
 * Log de build del proyecto: resuelve el proyecto por slug, toma su build mas
 * reciente (GET /projects/{id}/builds?limit=1) y pide el log en texto plano
 * (GET /builds/{id}/logs), convertido a lineas para el visor. `q` filtra las
 * lineas server-side (subcadena case-insensitive) sobre TODO el log, no solo el
 * tramo cargado. Degrada al seed (filtrado por `q` si aplica) si no hay backend
 * vivo o aun no hay log. NUNCA expone tokens.
 */
export async function loadLogs(
  session: SytueSession,
  slug: string,
  q?: string,
  opts?: {
    since?: string;
    until?: string;
    cursor?: string;
    limit?: number;
    source?: 'build' | 'runtime';
  },
): Promise<Sourced<LogLine[]> & { nextCursor?: string }> {
  try {
    const cloud = client(session);
    const sdkProjects = await cloud.listProjects();
    const match = sdkProjects.find((p) => slugFromSdk(p) === slug);
    if (!match) throw new Error('not_found_live');

    // El log de COMPILACIÓN sigue siendo consultable: es el que dice por qué no llegó a
    // arrancar. Son dos preguntas distintas —«por qué no compila» y «qué está pasando
    // ahora»— y se responden con dos registros distintos.
    if (opts?.source === 'build') {
      const builds = await http(session).json<{ builds?: Array<{ id: string }> }>({
        path: `/projects/${encodeURIComponent(match.id)}/builds`,
        query: { limit: 1 },
      });
      const buildId = builds.builds?.[0]?.id;
      if (!buildId) throw new Error('no_builds');
      const text = await http(session).text({
        path: `/builds/${encodeURIComponent(buildId)}/logs`,
        ...(q ? { query: { q } } : {}),
      });
      return { live: true, data: parseBuildLog(text) };
    }

    // Los logs de EJECUCIÓN, no los de compilación. Esta búsqueda vivía sobre
    // `/builds/{id}/logs`, así que quien preguntaba por qué falla su aplicación en
    // producción leía el log de cuando se construyó: dos cosas distintas que se
    // parecen lo bastante como para no notar el cambiazo. El recolector, la retención
    // y esta búsqueda con rango y cursor ya estaban en el control-plane.
    const query: Record<string, string | number> = {};
    if (q) query.q = q;
    if (opts?.since) query.since = opts.since;
    if (opts?.until) query.until = opts.until;
    if (opts?.cursor) query.cursor = opts.cursor;
    if (opts?.limit) query.limit = opts.limit;
    const page = await http(session).json<{
      lines?: Array<{ ts: string; text: string; deployment_id?: string }>;
      next_cursor?: string;
    }>({ path: `/projects/${encodeURIComponent(match.id)}/logs`, query });

    const data = (page.lines ?? []).map(toRuntimeLine);
    return page.next_cursor
      ? { live: true, data, nextCursor: page.next_cursor }
      : { live: true, data };
  } catch (e) {
    const ql = q?.toLowerCase();
    const demo = ql ? seed.buildLogs.filter((l) => contiene(l.message, ql)) : seed.buildLogs;
    return degrade(e, demo);
  }
}

/**
 * Una línea de ejecución del control-plane, en la forma que pinta el visor.
 *
 * El nivel lo deduce el texto: el control-plane guarda la línea TAL CUAL la escribió el
 * contenedor —sin inventarle una severidad que no traía— y el visor necesita algo con
 * lo que colorear. La misma heurística que ya se aplicaba al log de compilación.
 */
function toRuntimeLine(l: { ts: string; text: string }): LogLine {
  return {
    ts: l.ts,
    level: /\b(error|fatal|failed|panic)\b/i.test(l.text)
      ? 'error'
      : /\bwarn/i.test(l.text)
        ? 'warn'
        : 'info',
    source: 'runtime',
    message: l.text,
  };
}

/** Convierte el log de build (texto plano) en lineas para el visor; nivel por heuristica. */
function parseBuildLog(text: string): LogLine[] {
  return text
    .split('\n')
    .filter((line) => line.trim() !== '')
    .map((message): LogLine => ({
      ts: '',
      level: /\b(error|fatal|failed|panic)\b/i.test(message)
        ? 'error'
        : /\bwarn/i.test(message)
          ? 'warn'
          : 'info',
      source: 'build',
      message,
    }));
}

// ---- Logs de runtime en vivo (SSE) y terminal (WebSocket) ----
//
// Estas dos superficies del control-plane exigen `Authorization: Bearer` y NI
// EventSource NI WebSocket permiten poner cabeceras desde el navegador (y el
// navegador tampoco habla directo con el control-plane). Por eso la conexión la
// abre SIEMPRE el servidor: el cliente llama a una ruta BFF de mismo origen
// (autenticado por la cookie httpOnly de sesión) y aquí se añade el Bearer. El
// access token nunca sale del servidor. Ver src/pages/api/log-stream.ts y
// src/pages/api/exec.ts.

/** URL absoluta de un recurso de la API del control-plane (para fetch/WS crudos). */
function apiUrl(path: string): string {
  return `${cloudBaseUrl().replace(/\/$/, '')}/api/v1${path}`;
}

/** Código de error del envelope canónico Go de un cuerpo ya leído como texto. */
function codeFromBody(text: string, fallback: string): string {
  try {
    const e = (JSON.parse(text) as { error?: unknown }).error;
    if (typeof e === 'string' && e) return e;
    if (e && typeof e === 'object') {
      const c = (e as { code?: unknown }).code;
      if (typeof c === 'string' && c) return c;
    }
  } catch {
    // Cuerpo no-JSON: se queda el fallback legible.
  }
  return fallback;
}

/** Fallo con causa REAL (status del control-plane) o «no hay backend» (503). */
type StreamFailure = { ok: false; status: number; code: string };

/** Traduce un fallo previo al stream (resolver proyecto, red) sin inventar datos. */
function streamFailure(err: unknown): StreamFailure {
  const info = httpErrorInfo(err);
  if (info) return { ok: false, ...info };
  return { ok: false, status: 503, code: 'no_backend' };
}

export type LogStreamOpen = { ok: true; body: ReadableStream<Uint8Array> } | StreamFailure;

/**
 * Abre el stream SSE de logs de RUNTIME de un despliegue. Devuelve el cuerpo tal
 * cual para reenviarlo al navegador sin bufferizar (la ruta BFF hace de tubería).
 * `signal` es el del Request entrante: si el usuario cierra la pestaña, se aborta
 * también la conexión al control-plane. Rol exigido por el backend: member+.
 */
export async function openRuntimeLogStream(
  session: SytueSession,
  slug: string,
  deploymentId: string,
  signal: AbortSignal,
): Promise<LogStreamOpen> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return { ok: false, status: 404, code: 'project_not_found' };
    const res = await fetch(
      apiUrl(
        `/projects/${encodeURIComponent(projectId)}/deployments/${encodeURIComponent(deploymentId)}/logs`,
      ),
      {
        headers: {
          authorization: `Bearer ${session.tokens.accessToken}`,
          accept: 'text/event-stream',
        },
        signal,
      },
    );
    if (!res.ok || !res.body) {
      return {
        ok: false,
        status: res.status,
        code: codeFromBody(await res.text().catch(() => ''), res.statusText || 'backend_error'),
      };
    }
    return { ok: true, body: res.body };
  } catch (err) {
    return streamFailure(err);
  }
}

/**
 * Descarga las variables del proyecto como fichero `.env`.
 *
 * Reenvía la Response del control-plane (cuerpo, tipo y nombre de fichero) en vez
 * de leerla: lo que viaja son los secretos en claro, y cuanto menos se copien por
 * el camino, mejor. undefined si no hay backend vivo.
 */
export async function streamEnvExport(
  session: SytueSession,
  slug: string,
  target: 'production' | 'preview',
): Promise<Response | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return undefined;
    const res = await fetch(
      apiUrl(`/projects/${encodeURIComponent(projectId)}/env/export?target=${target}`),
      { headers: { authorization: `Bearer ${session.tokens.accessToken}` } },
    );
    if (!res.ok || !res.body) return undefined;
    return new Response(res.body, {
      status: 200,
      headers: {
        'content-type': 'text/plain; charset=utf-8',
        'content-disposition':
          res.headers.get('content-disposition') ?? `attachment; filename=".env.${target}"`,
        'cache-control': 'no-store',
      },
    });
  } catch {
    return undefined; // sin backend vivo
  }
}

/**
 * Descarga los logs de runtime del proyecto como fichero de texto.
 *
 * Devuelve la Response del control-plane REENVIADA —cuerpo, tipo y nombre de
 * fichero incluidos— en vez de leerla entera: un log filtrado puede pesar cientos
 * de megas y materializarlo en el BFF ataría esa memoria por descarga. undefined
 * si no hay backend vivo: en demostración no hay fichero que bajar, y devolver
 * uno vacío haría creer que la app no escribió nada.
 */
export async function streamLogExport(
  session: SytueSession,
  slug: string,
  q?: string,
): Promise<Response | undefined> {
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return undefined;
    const params = new URLSearchParams({ format: 'txt' });
    if (q) params.set('q', q);
    const res = await fetch(
      apiUrl(`/projects/${encodeURIComponent(projectId)}/logs?${params.toString()}`),
      { headers: { authorization: `Bearer ${session.tokens.accessToken}` } },
    );
    if (!res.ok || !res.body) return undefined;
    return new Response(res.body, {
      status: 200,
      headers: {
        'content-type': 'text/plain; charset=utf-8',
        // El nombre del fichero lo pone el control-plane; si faltara, uno legible
        // vale más que dejar al navegador inventarse el nombre de la ruta.
        'content-disposition':
          res.headers.get('content-disposition') ?? `attachment; filename="${slug}.log"`,
        'cache-control': 'no-store',
      },
    });
  } catch {
    return undefined; // sin backend vivo
  }
}

export type ExecResult =
  { ok: true; output: string; truncated: boolean; timedOut: boolean } | StreamFailure;

/** Tope de salida por comando: `yes` o un `cat` de un binario no pueden reventar el BFF. */
const EXEC_MAX_OUTPUT = 256 * 1024;
/** Un comando que no termina en 20 s devuelve lo acumulado y cierra (no cuelga la petición). */
const EXEC_TIMEOUT_MS = 20_000;

/**
 * Ejecuta UN comando en el pod del despliegue por el WebSocket de exec y devuelve
 * la salida completa. Rol exigido por el backend: admin/owner (y rechaza tokens
 * de API).
 *
 * Diseño deliberado — una sesión de `/bin/sh` POR COMANDO (petición sin estado):
 * el dashboard corre con varias réplicas detrás del balanceador, así que guardar
 * la sesión abierta en memoria del BFF obligaría a sesiones pegajosas y se caería
 * al reciclar un pod. Consecuencia visible (la UI la dice): `cd` y las variables
 * no persisten entre comandos; se encadena con `&&`.
 *
 * `exec 2>&1` como primera línea porque el control-plane descarta el stderr del
 * pod (io.Discard): sin esto un comando que falla no devolvería NADA. `exit`
 * cierra el shell y con él el WS, que es la señal de «salida completa».
 */
export async function execInDeployment(
  session: SytueSession,
  slug: string,
  deploymentId: string,
  command: string,
): Promise<ExecResult> {
  let url: string;
  try {
    const projectId = await resolveProjectId(session, slug);
    if (!projectId) return { ok: false, status: 404, code: 'project_not_found' };
    url = apiUrl(
      `/projects/${encodeURIComponent(projectId)}/deployments/${encodeURIComponent(deploymentId)}/exec`,
    );
  } catch (err) {
    return streamFailure(err);
  }

  const token = session.tokens.accessToken;
  return await new Promise<ExecResult>((resolve) => {
    let socket: WebSocket;
    try {
      // Bun (runtime de esta app: `bun ./dist/server/entry.mjs`) admite `headers`
      // en el 2º argumento del WebSocket; los tipos WHATWG solo declaran
      // subprotocolos, de ahí el cast. El subprotocolo NO sirve como transporte
      // del token: el control-plane no negocia ninguno y el navegador cortaría.
      socket = new WebSocket(url.replace(/^http/, 'ws'), {
        headers: { authorization: `Bearer ${token}` },
      } as unknown as string[]);
    } catch {
      resolve({ ok: false, status: 503, code: 'no_backend' });
      return;
    }
    socket.binaryType = 'arraybuffer';

    let output = '';
    let truncated = false;
    let timedOut = false;
    let opened = false;
    let settled = false;

    const finish = (r: ExecResult) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      try {
        socket.close();
      } catch {
        // Ya cerrado por el otro extremo.
      }
      resolve(r);
    };

    const timer = setTimeout(() => {
      // Sin handshake no hay «salida vacía» que devolver: es un fallo de conexión
      // y se sondea igual que en el cierre (si no, un 403 se vería como comando OK).
      if (!opened) {
        void probeExecFailure(url, token).then(finish);
        return;
      }
      timedOut = true;
      finish({ ok: true, output, truncated, timedOut: true });
    }, EXEC_TIMEOUT_MS);

    socket.onopen = () => {
      opened = true;
      socket.send(`exec 2>&1\n${command}\nexit\n`);
    };
    socket.onmessage = (ev: MessageEvent) => {
      const chunk =
        typeof ev.data === 'string'
          ? ev.data
          : ev.data instanceof ArrayBuffer
            ? new TextDecoder().decode(ev.data)
            : '';
      if (output.length + chunk.length > EXEC_MAX_OUTPUT) {
        output += chunk.slice(0, Math.max(0, EXEC_MAX_OUTPUT - output.length));
        truncated = true;
        finish({ ok: true, output, truncated, timedOut });
        return;
      }
      output += chunk;
    };
    socket.onclose = () => {
      if (opened) {
        finish({ ok: true, output, truncated, timedOut });
        return;
      }
      // Cierre SIN handshake: el WebSocket no expone el status HTTP (siempre
      // 1006), así que se sondea para dar la causa REAL en vez de un error mudo.
      // Solo aquí (no en onerror): tras un error de conexión el cierre llega
      // SIEMPRE, y sondear en ambos sitios duplicaría la petición.
      void probeExecFailure(url, token).then(finish);
    };
  });
}

/**
 * Causa real de un handshake de exec fallido: repite la petición como GET normal.
 * La cadena de middleware del control-plane (authenticate → requireRole) corre
 * ANTES del upgrade, así que devuelve el mismo 401/403/404 que habría abortado el
 * WS; un 400 significa «autorizado, solo faltaban las cabeceras de upgrade» (el
 * fallo entonces es del propio upgrade, no de permisos).
 */
async function probeExecFailure(url: string, token: string): Promise<StreamFailure> {
  try {
    const res = await fetch(url, { headers: { authorization: `Bearer ${token}` } });
    if (res.status === 400) return { ok: false, status: 502, code: 'ws_handshake_failed' };
    return {
      ok: false,
      status: res.status,
      code: codeFromBody(await res.text().catch(() => ''), res.statusText || 'backend_error'),
    };
  } catch {
    return { ok: false, status: 503, code: 'no_backend' };
  }
}

/**
 * Primer error REAL del control-plane entre varias cargas de una página. Sirve
 * para que el aviso de cabecera diga la verdad (403 / 500) en vez de llamar
 * «modo demostración» a un permiso denegado. Sin errores devuelve undefined.
 */
export function firstSourceError(
  ...sources: Array<{ error?: { status: number; code: string } }>
): { status: number; code: string } | undefined {
  return sources.find((s) => s.error !== undefined)?.error;
}

// ---- Tokens de API (credencial del CLI, Terraform y CI) ----

/** Token de API: metadatos. El secreto solo existe en la respuesta de creación. */
export interface ApiToken {
  id: string;
  name: string;
  /** `hook`: credencial de una URL de despliegue; no vale en la API. */
  scope: 'read' | 'write' | 'deploy' | 'hook';
  /** Confinado a un proyecto; ausente = alcance de organización. */
  projectId?: string;
  createdBy?: string;
  expiresAt?: string;
  lastUsedAt?: string;
  createdAt: string;
}

/** Extrae una cadena de un JSON no tipado; '' si el campo falta o no es texto.
 *  Sin String(): sobre un objeto daría el literal «[object Object]» en la UI. */
function jsonStr(o: Record<string, unknown>, k: string): string {
  return typeof o[k] === 'string' ? o[k] : '';
}

function mapToken(t: Record<string, unknown>): ApiToken {
  const opt = (k: string): string | undefined => jsonStr(t, k) || undefined;
  const projectId = opt('project_id');
  const createdBy = opt('created_by');
  const expiresAt = opt('expires_at');
  const lastUsedAt = opt('last_used_at');
  return {
    id: jsonStr(t, 'id'),
    name: jsonStr(t, 'name'),
    scope: t.scope === 'write' || t.scope === 'deploy' || t.scope === 'hook' ? t.scope : 'read',
    ...(projectId !== undefined ? { projectId } : {}),
    ...(createdBy !== undefined ? { createdBy } : {}),
    ...(expiresAt !== undefined ? { expiresAt } : {}),
    ...(lastUsedAt !== undefined ? { lastUsedAt } : {}),
    createdAt: jsonStr(t, 'created_at'),
  };
}

/** Tokens del tenant. Sin backend: lista vacía — NUNCA tokens de ejemplo. */
export async function loadApiTokens(session: SytueSession): Promise<Sourced<ApiToken[]>> {
  try {
    const r = await http(session).json<{ tokens?: Array<Record<string, unknown>> }>({
      path: '/tokens',
    });
    return { live: true, data: (r.tokens ?? []).map(mapToken) };
  } catch (e) {
    // Sin datos de ejemplo a propósito: un token falso invita a copiarlo y usarlo.
    return degrade(e, []);
  }
}

/**
 * Crea un token y devuelve el SECRETO EN CLARO, que el control-plane entrega una
 * sola vez y no se puede volver a leer. Un error real se propaga (no se simula un
 * token que no existe: el usuario lo copiaría y no funcionaría).
 */
export async function createApiToken(
  session: SytueSession,
  input: { name: string; scope: string; projectId?: string; expiresAt?: string },
): Promise<{ token: ApiToken; plaintext: string }> {
  const created = await http(session).json<Record<string, unknown>>({
    method: 'POST',
    path: '/tokens',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      name: input.name,
      scope: input.scope,
      ...(input.projectId ? { project_id: input.projectId } : {}),
      ...(input.expiresAt ? { expires_at: input.expiresAt } : {}),
    }),
  });
  return { token: mapToken(created), plaintext: jsonStr(created, 'token') };
}

/**
 * Emite la credencial de una URL de despliegue para un proyecto.
 *
 * Es un token con scope `hook` confinado a ese proyecto, que solo acepta la ruta
 * pública `/hooks/deploy/{token}`: quien vea la URL no puede usarlo contra la API
 * (auditoría 2026-10-06, CLOUD-17). Devuelve el secreto en claro (solo esta vez) o
 * undefined si no hay control-plane vivo: un hook inventado sería una URL que no
 * despliega nada.
 */
export async function createDeployHookToken(
  session: SytueSession,
  slug: string,
): Promise<string | undefined> {
  const projectId = await resolveProjectId(session, slug);
  if (!projectId) return undefined;
  // El nombre lo lee quien mire la lista de tokens: tiene que decir de dónde
  // salió, o dentro de un mes nadie sabrá si se puede revocar.
  const { plaintext } = await createApiToken(session, {
    name: `Deploy hook · ${slug}`,
    // `hook` solo vale en /hooks/deploy/{token}: quien vea la URL no gana la API.
    scope: 'hook',
    projectId,
  });
  return plaintext;
}

/** Revoca un token por id. */
export async function revokeApiToken(session: SytueSession, id: string): Promise<void> {
  await http(session).text({ method: 'DELETE', path: `/tokens/${encodeURIComponent(id)}` });
}

// ---- Miembros de la organización (y, por tanto, puestos facturables) ----

/** Miembro de la organización. `sub` es su identificador en Sytue ID. */
export interface Member {
  sub: string;
  role: 'owner' | 'admin' | 'member' | 'billing';
  createdAt: string;
}

export interface MemberInvitation {
  id: string;
  email: string;
  role: Member['role'];
  expiresAt: string;
  createdAt: string;
  updatedAt: string;
}

const MEMBER_ROLES = new Set(['owner', 'admin', 'member', 'billing']);

function mapMember(m: Record<string, unknown>): Member {
  const role = jsonStr(m, 'role');
  return {
    sub: jsonStr(m, 'sub'),
    role: (MEMBER_ROLES.has(role) ? role : 'member') as Member['role'],
    createdAt: jsonStr(m, 'created_at'),
  };
}

function mapMemberInvitation(inv: Record<string, unknown>): MemberInvitation {
  const role = jsonStr(inv, 'role');
  return {
    id: jsonStr(inv, 'id'),
    email: jsonStr(inv, 'email'),
    role: (MEMBER_ROLES.has(role) ? role : 'member') as Member['role'],
    expiresAt: jsonStr(inv, 'expires_at'),
    createdAt: jsonStr(inv, 'created_at'),
    updatedAt: jsonStr(inv, 'updated_at'),
  };
}

/** Miembros del tenant. Sin backend: lista vacía — no se inventan compañeros. */
export async function loadMembers(session: SytueSession): Promise<Sourced<Member[]>> {
  try {
    const r = await http(session).json<{ members?: Array<Record<string, unknown>> }>({
      path: '/members',
    });
    return { live: true, data: (r.members ?? []).map(mapMember) };
  } catch (e) {
    // Sin ejemplos: un miembro falso sugiere que alguien tiene acceso y no lo tiene.
    return degrade(e, []);
  }
}

/** Invitaciones vigentes. No son miembros ni cuentan como puestos facturables. */
export async function loadMemberInvitations(
  session: SytueSession,
): Promise<Sourced<MemberInvitation[]>> {
  try {
    const r = await http(session).json<{ invitations?: Array<Record<string, unknown>> }>({
      path: '/invitations',
    });
    return { live: true, data: (r.invitations ?? []).map(mapMemberInvitation) };
  } catch (e) {
    return degrade(e, []);
  }
}

export type InviteMemberResult =
  { member: Member; invitation?: never } | { member?: never; invitation: MemberInvitation };

/** Invita por correo o materializa a quien ya era miembro de Sytue ID. */
export async function inviteMember(
  session: SytueSession,
  email: string,
  role: string,
): Promise<InviteMemberResult> {
  const result = await http(session).json<{
    member?: Record<string, unknown>;
    invitation?: Record<string, unknown>;
  }>({
    method: 'POST',
    path: '/invitations',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ email, role }),
  });
  if (result.member) return { member: mapMember(result.member) };
  if (result.invitation) return { invitation: mapMemberInvitation(result.invitation) };
  throw new Error('cloud: respuesta de invitación incompleta');
}

export async function revokeMemberInvitation(session: SytueSession, id: string): Promise<void> {
  await http(session).text({ method: 'DELETE', path: `/invitations/${encodeURIComponent(id)}` });
}

/** Añade (o actualiza el rol de) un miembro por su `sub` de Sytue ID. */
export async function addMember(session: SytueSession, sub: string, role: string): Promise<Member> {
  const m = await http(session).json<Record<string, unknown>>({
    method: 'POST',
    path: '/members',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ sub, role }),
  });
  return mapMember(m);
}

/** Quita a un miembro. El control-plane rechaza dejar la organización sin owner. */
export async function removeMember(session: SytueSession, sub: string): Promise<void> {
  await http(session).text({ method: 'DELETE', path: `/members/${encodeURIComponent(sub)}` });
}

// ---- Auditoría (quién hizo qué en la organización) ----

/** Evento de auditoría del control-plane. */
export interface AuditEvent {
  id: string;
  actorSub: string;
  action: string;
  resource: string;
  detail: Record<string, unknown>;
  occurredAt: string;
}

/**
 * Criterios del visor. Los aplica el CONTROL-PLANE, no el navegador: quien audita
 * busca algo de hace semanas, y filtrar en el cliente solo miraría la última página.
 */
export interface AuditQuery {
  actor?: string | undefined;
  action?: string | undefined;
  /** AAAA-MM-DD (o RFC3339). `until` incluye el día entero. */
  since?: string | undefined;
  until?: string | undefined;
  /** Token opaco de la página anterior (keyset; el backend nunca usa OFFSET). */
  cursor?: string | undefined;
  limit?: number | undefined;
}

/** Página de auditoría + token de continuación ('' si no quedan más). */
export interface AuditPage {
  events: AuditEvent[];
  nextCursor: string;
}

/** Traduce los criterios a la query del control-plane (omite lo vacío). */
function auditParams(q: AuditQuery): Record<string, string | number | undefined> {
  return {
    actor: q.actor?.trim() || undefined,
    action: q.action?.trim() || undefined,
    since: q.since || undefined,
    until: q.until || undefined,
    cursor: q.cursor || undefined,
    limit: q.limit,
  };
}

/** Una página de la auditoría. Sin backend: vacía — no se inventa un historial. */
export async function loadAudit(
  session: SytueSession,
  q: AuditQuery = {},
): Promise<Sourced<AuditPage>> {
  try {
    const r = await http(session).json<{
      events?: Array<Record<string, unknown>>;
      next_cursor?: string;
    }>({ path: '/audit', query: auditParams(q) });
    return {
      live: true,
      data: {
        events: (r.events ?? []).map((e) => ({
          id: jsonStr(e, 'id'),
          actorSub: jsonStr(e, 'actor_sub'),
          action: jsonStr(e, 'action'),
          resource: jsonStr(e, 'resource'),
          detail:
            typeof e.detail === 'object' && e.detail !== null
              ? (e.detail as Record<string, unknown>)
              : {},
          occurredAt: jsonStr(e, 'occurred_at'),
        })),
        nextCursor: typeof r.next_cursor === 'string' ? r.next_cursor : '',
      },
    };
  } catch (e) {
    // Un historial inventado es peor que ninguno: sugiere que no pasó nada.
    return degrade(e, { events: [], nextCursor: '' });
  }
}

/**
 * Export CSV del conjunto filtrado. Devuelve la respuesta CRUDA del control-plane
 * para reenviarla en streaming desde el BFF: un export de compliance puede ser de
 * decenas de miles de filas y no hay motivo para materializarlo en el proceso Node.
 */
export function exportAuditCsv(session: SytueSession, q: AuditQuery = {}): Promise<Response> {
  return http(session).request({
    path: '/audit',
    query: { ...auditParams(q), cursor: undefined, format: 'csv' },
    headers: { accept: 'text/csv' },
  });
}

// ---------------------------------------------------------------------------
// Recomendación de tamaño según el uso REAL
// ---------------------------------------------------------------------------

/**
 * Qué hacer con el tamaño de un proyecto y cuánto cambia eso la factura.
 *
 * Sin control-plane vivo devuelve `live:false` y NINGUNA recomendación: aquí una
 * cifra inventada sería peor que no enseñar nada, porque el usuario la aplicaría.
 * Aplicarla es `updateProject` con `instance_size` — ese camino ya valida contra
 * el catálogo, audita y respeta la cuota del plan.
 */
export async function loadProjectSizing(
  session: SytueSession,
  slug: string,
): Promise<Sourced<SizingAdvice | null>> {
  try {
    // El parámetro se llamaba `projectId` y la página le pasaba `project.slug`,
    // como a sus cinco hermanas. Pero estas resuelven el slug a UUID antes de
    // llamar y esta no: el control-plane exige UUID, devolvía 400, y `degrade`
    // se lo tragaba dejando `null`. Resultado: la recomendación de tamaño
    // —enseñar cuánto se ahorra— no se ha pintado NUNCA, y sin un solo error a
    // la vista, porque un 4xx tampoco se registraba.
    const id = await resolveProjectId(session, slug);
    if (!id) throw new Error('not_found_live');
    return { live: true, data: await client(session).projectSizing(id) };
  } catch (e) {
    return degrade<SizingAdvice | null>(e, null);
  }
}

// ---------------------------------------------------------------------------
// «Tu app todavía no ve este servicio»
// ---------------------------------------------------------------------------

// Pura y de cliente: vive en `servicios.ts` para que la isla del Resumen no importe
// este módulo de servidor (lo sigue exportando para quien ya lo usa desde aquí).
export { servicesNotYetLive } from './servicios.js';
