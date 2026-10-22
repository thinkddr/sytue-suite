/**
 * Modelos del control-plane de Sytue DevX (PaaS). Tipados que consume tanto el
 * SSR (carga inicial) como las islas Preact. Reflejan los recursos de la API de
 * Sytue DevX (cloud.sytue.eu / Sytue DevX API). docs/CONVENTIONS.md §14.
 */

export type DeployStatus = 'building' | 'ready' | 'error' | 'queued' | 'canceled';
export type DeployTarget = 'production' | 'preview';
export type Framework = 'astro' | 'next' | 'preact' | 'static' | 'node' | 'sveltekit';
export type Region = 'fr-par' | 'nl-ams';
export type ServiceKind =
  | 'postgres'
  | 'storage'
  /** Solo en los datos de demostración: el control-plane no emite este tipo. */
  | 'cache'
  | 'functions'
  | 'cron'
  /** Trabajo puntual (run-to-completion), hermano de `cron`. */
  | 'job'
  /** Proceso permanente sin puerto: el consumidor de una cola. */
  | 'worker'
  | 'auth'
  /** App open-source del catálogo (ADR 0029): caché, buscador, cola, CMS. */
  | 'app';
export type EnvScope = 'production' | 'preview' | 'development';
export type LogLevel = 'info' | 'warn' | 'error' | 'debug';

export interface Project {
  /** UUID del proyecto (solo en datos live). */
  id?: string;
  slug: string;
  name: string;
  framework: Framework;
  /** Repositorio Git conectado (estilo Vercel). */
  repo: string;
  /**
   * Rama (o etiqueta) que se despliega a producción. El control-plane la
   * resuelve a `main` cuando no se fijó, así que aquí nunca llega vacía.
   */
  productionRef: string;
  productionUrl: string;
  region: Region;
  /** Tenant (organizacion) propietario — grupo de Sytue ID org/<slug>. */
  tenant: string;
  createdAt: string;
  services: ServiceKind[];
  /**
   * Contrato de ejecución de la app. Son los valores EFECTIVOS: el control-plane
   * ya resuelve el defecto de cada uno, así que el formulario nunca tiene que
   * adivinar qué significa un campo vacío.
   */
  runtime: ProjectRuntime;
  /**
   * Si las previews piden contraseña. Solo el sí/no: el hash no sale de la API, y
   * la contraseña en claro se enseña una vez al activarla y no se guarda.
   */
  previewProtected?: boolean;
  /**
   * Proyecto apagado sin borrar: cero réplicas y cero factura de cómputo. Todo lo
   * demás —dominio, TLS, variables, base de datos— sigue en su sitio.
   */
  paused?: boolean;
  /** Redirecciones y cabeceras del borde (lo que en Vercel es `vercel.json`). */
  httpRules?: {
    redirects?: Array<{ from: string; to: string; status?: number }>;
    headers?: Record<string, string>;
  };
  /**
   * Endpoint al que el proveedor git manda el push. Solo llega en la respuesta de
   * CREACIÓN (y en el detalle pedido por un admin/owner): en la lista no viene.
   */
  webhookUrl?: string;
  /**
   * Secreto con el que se firma ese push. El control-plane lo entrega UNA SOLA VEZ
   * al crear el proyecto y no se puede volver a leer, así que hay que enseñarlo en
   * ese mismo momento o «git push → despliegue» queda inalcanzable desde la web.
   */
  webhookSecret?: string;
}

/** Escala y contrato de ejecución de una app (lo que se configura y se factura). */
export interface ProjectRuntime {
  /** Tamaño del catálogo: basic | standard | pro | pro-xl. */
  instanceSize: string;
  /** Instancias deseadas; es el MÍNIMO del autoescalado, no un techo. */
  instanceCount: number;
  /** Puerto en el que escucha la app. */
  appPort: number;
  /** Ruta de las sondas de salud. */
  healthPath: string;
  /**
   * Comando previo al arranque de la versión nueva (migraciones). Vacío = no hay
   * paso de release, que es el caso por defecto.
   */
  releaseCommand: string;
}

export interface Deployment {
  id: string;
  project: string;
  status: DeployStatus;
  target: DeployTarget;
  branch: string;
  /** Numero de PR si es preview por pull request. */
  pr?: number;
  commitSha: string;
  commitMessage: string;
  author: string;
  url: string;
  createdAt: string;
  /**
   * Última vez que el control-plane tocó este despliegue. Es la fecha buena para
   * saber si tiene las credenciales de un servicio: promocionar o reaplicar la
   * producción vuelve a resolverlas, así que `createdAt` daría falsos avisos tras
   * un rollback.
   */
  updatedAt?: string;
  /** Duracion del build en segundos (si finalizo). */
  durationSec?: number;
  /** Si es el despliegue de produccion actual. */
  current?: boolean;
}

export interface Domain {
  /** UUID del dominio (solo en datos live). */
  id?: string;
  host: string;
  /** apex | www | preview | apps */
  kind: 'custom' | 'platform';
  verified: boolean;
  /** Estado del certificado TLS automatico. */
  tls: 'active' | 'pending' | 'error';
  /** Objetivo CNAME (ingress.sytue.eu) para dominios propios. */
  cname?: string;
  /**
   * Registro TXT de verificación de propiedad. Sin él el dominio NUNCA pasa de
   * «sin verificar» —y sin verificar no hay certificado—, así que la pantalla lo
   * enseña mientras siga pendiente, no solo en el instante de añadirlo.
   */
  txtRecord?: string;
  /** Valor del TXT: el token que emite el control-plane para este dominio. */
  txtValue?: string;
  primary?: boolean;
}

export interface EnvVar {
  key: string;
  /** Valor enmascarado (los secretos nunca se devuelven en claro). */
  preview: string;
  secret: boolean;
  scopes: EnvScope[];
  /** Origen: manual | inyectado por un servicio gestionado. */
  source: 'manual' | ServiceKind;
  updatedAt: string;
}

export interface LogLine {
  ts: string;
  level: LogLevel;
  source: 'build' | 'runtime' | 'edge';
  message: string;
}

export interface UsageMetric {
  label: string;
  value: string;
  delta?: string;
  trend?: 'up' | 'down' | 'flat';
  /** Clave de métrica del control-plane (build_minutes…); presente en datos con coste. */
  metric?: string;
  /** Cantidad numérica consumida en el período (para estimar coste). */
  quantity?: number;
  /** Unidad de display de la cantidad (min, GB·h, GB). */
  unit?: string;
}

/** Organización del tenant (facturación y cuotas). */
export interface Organization {
  name: string;
  slug: string;
  plan: 'free' | 'pro' | 'team';
  region: string;
}

export interface ManagedService {
  /**
   * Id del control-plane. Sin él no hay gestión posible —dar de baja va por
   * `DELETE /services/{id}`—, y su ausencia era justo lo que dejaba el panel en
   * solo lectura: se podían crear servicios y no retirarlos.
   *
   * Opcional porque los datos sembrados de la demostración no tienen uno real, y
   * un id inventado ahí acabaría en una petición de borrado contra un id que no
   * existe.
   */
  id?: string;
  kind: ServiceKind;
  name: string;
  status: 'running' | 'provisioning' | 'paused';
  /**
   * URL pública de una función gestionada (`https://<proyecto>-<fn>.fn.<dominio>`,
   * la compone el control-plane al aprovisionar). Es lo único que convierte una
   * función en algo usable: sin ella el usuario no sabe a dónde llamar.
   */
  url?: string;
  /** Programación de un cron (expresión de 5 campos o macro `@daily`). */
  schedule?: string;
  /** Imagen OCI con la que corre la función/cron/job (referencia por digest). */
  image?: string;
  /** URL que invoca un cron de tipo HTTP (el caso sin imagen). */
  targetUrl?: string;
  /**
   * Entrada del catálogo, solo con kind='app'. Es lo que permite saber QUÉ hay
   * levantado: el nombre lo escribe el usuario, así que «buscador» no dice si
   * dentro hay Meilisearch.
   */
  appId?: string;
  /**
   * URL pública, si el catálogo declara la app publicable. Sin ella, el servicio
   * solo se alcanza desde dentro del proyecto y no hay nada que enlazar.
   */
  webUrl?: string;
  /**
   * Cuándo se creó. No es decorativo: comparado con la fecha del despliegue vivo
   * dice si la app EN MARCHA llegó a recibir las credenciales de este servicio
   * (se resuelven al desplegar), que es lo que decide si la conexión funciona.
   */
  createdAt?: string;
}

// ---- Monitorización: uptime + alertas (Wave 2) ----

export type UptimeStatus = 'up' | 'down' | 'unknown';

/** Sonda de disponibilidad de una URL pública (vista del panel de monitorización). */
export interface UptimeCheck {
  id: string;
  name: string;
  targetUrl: string;
  intervalSeconds: number;
  enabled: boolean;
  /** Resultado del último sondeo ('unknown' = aún sin sondear). */
  status: UptimeStatus;
  /** Latencia del último sondeo en ms. */
  latencyMs: number;
  /** ISO del último sondeo (ausente si aún no se ha sondeado). */
  lastCheckedAt?: string;
}

export type AlertEvent =
  | 'deploy.failed'
  | 'deploy.ready'
  | 'uptime.down'
  | 'uptime.up'
  | 'billing.spend_threshold'
  | 'sizing.advice'
  | 'metric.cpu_high'
  | 'metric.memory_high';

/** Política de alerta: enruta un evento a un webhook (Slack/Discord/genérico). */
export interface AlertPolicy {
  id: string;
  event: AlertEvent;
  channelUrl: string;
  enabled: boolean;
  /** true = alcance organización (cualquier proyecto); false = un proyecto concreto. */
  orgScope: boolean;
}

// ---- Sytue DB Studio (Postgres gestionado + PostgREST) ----

export interface DbColumn {
  name: string;
  type: string;
  nullable: boolean;
  isPrimaryKey: boolean;
  /** Referencia FK "tabla.columna" si aplica. */
  references?: string;
  default?: string;
}

export interface DbTable {
  schema: string;
  name: string;
  rowCount: number;
  columns: DbColumn[];
  /** Si PostgREST expone la tabla en la API REST automatica. */
  apiExposed: boolean;
}

export interface DbRows {
  columns: string[];
  rows: Array<Record<string, unknown>>;
  totalRows: number;
}

export interface SqlResult {
  /** Nombres de columna del resultado. */
  columns: string[];
  rows: Array<Record<string, unknown>>;
  rowCount: number;
  /** Tiempo de ejecucion en ms. */
  elapsedMs: number;
  /** Mensaje (p.ej. "UPDATE 3") para sentencias sin filas. */
  notice?: string;
  error?: string;
}

export interface DbApiInfo {
  /** Endpoint REST automatico (PostgREST). */
  restUrl: string;
  /** Cabeceras de ejemplo (clave anon enmascarada). */
  anonKeyPreview: string;
  serviceKeyPreview: string;
  /** Cadena de conexion Postgres con credenciales enmascaradas. */
  connectionString: string;
}
