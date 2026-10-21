/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue DevX — SDK al control plane Go del PaaS (cloud.sytue.eu; API dev :8090,
 * builder :8091). Proyectos, despliegues (git push -> build Kaniko -> Registry
 * Scaleway -> deploy -> URL TLS), previews por PR y servicios gestionados
 * (Postgres/PostgREST, Storage, Functions, Cron, Secrets). Implementacion propia.
 * Autorizacion Bearer (client `sytue-cloud` / `sytue-cloud-api`, JWT de Sytue ID).
 *
 * REST base:  /api/v1
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface CloudOptions {
  /** URL base de la API, p.ej. https://cloud.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/**
 * Idioma para el texto que GENERA la API (nombres de proyecto sugeridos). El
 * control-plane negocia: `lang` explícito y, si falta, la cabecera Accept-Language.
 * Un BFF puede reenviar la del navegador con `acceptLanguage`.
 */
export interface SlugLanguageOptions {
  /** Idioma de producto: en (base) | es | fr | de | it. */
  lang?: string;
  /** Valor crudo de Accept-Language a reenviar. */
  acceptLanguage?: string;
}

export interface SlugSuggestion {
  slug: string;
  /** Se confirmó libre contra el índice global de slugs. */
  available: boolean;
  /** Idioma que se usó de verdad tras negociar. */
  lang?: string;
}

export interface SlugCheck {
  /** El nombre comprobado, ya normalizado: es el que hay que enviar al crear. */
  slug: string;
  available: boolean;
  /** Nombres LIBRES derivados del pedido. Vacío si el pedido está disponible. */
  alternatives: string[];
  lang?: string;
}

function acceptLanguage(opts: SlugLanguageOptions): { headers?: Record<string, string> } {
  return opts.acceptLanguage ? { headers: { 'accept-language': opts.acceptLanguage } } : {};
}

/** Qué hacer con el tamaño: dejarlo, bajarlo (ahorra) o subirlo (evita una caída). */
export type SizingAction = 'keep' | 'down' | 'up';

/**
 * Por qué. No es un código de error: es lo que el usuario va a leer.
 * `peak_blocks_downgrade` merece explicación propia — en régimen le sobraría
 * tamaño, pero un PICO observado no cabría en el menor, así que ahorrar ahí
 * acabaría en una caída.
 */
export type SizingReason =
  | 'insufficient_data'
  | 'well_sized'
  | 'over_provisioned'
  | 'peak_blocks_downgrade'
  | 'near_limit'
  | 'at_largest'
  | 'at_smallest'
  | 'cohort_over_provisioned';

/**
 * Agregado ANÓNIMO de las apps comparables. Solo llega mientras el proyecto no
 * tiene histórico propio; en cuanto lo tiene, mandan sus datos.
 *
 * `tenants` nunca baja de 5 (empresas distintas, no proyectos) y solo hay
 * porcentajes: la anonimización se aplica al publicar, en el servidor.
 */
export interface CohortStat {
  p50_memory_percent: number;
  /** El que decide: nueve de cada diez apps comparables tienen que caber. */
  p90_memory_percent: number;
  /** Empresas distintas de la cohorte. Sin esto la cifra no es contrastable. */
  tenants: number;
  projects: number;
}

/** Recomendación de tamaño basada en el consumo real (GET /projects/{id}/sizing). */
export interface SizingAdvice {
  action: SizingAction;
  reason: SizingReason;
  /** Nombres del catálogo de tamaños. Con `keep`, `to` es igual que `from`. */
  from: string;
  to: string;
  /** Cambio en la factura mensual. **NEGATIVO = ahorro.** */
  monthly_delta_eur: number;
  /** Cuánto de lo que se paga se está usando, en porcentaje del límite actual. */
  p95_memory_percent: number;
  peak_memory_percent: number;
  p95_cpu_milli: number;
  /** Sobre qué se decidió: sin esto la recomendación no es auditable. */
  samples: number;
  window_hours: number;
  /** Ausente mientras no haya bastantes empresas comparables, que es lo normal al principio. */
  benchmark?: CohortStat;
}

export interface Project {
  id: string;
  slug: string;
  /** Nombre visible del proyecto. */
  name: string;
  /** Tenant (org/<slug>) propietario. */
  organization?: string;
  repo_url?: string;
  framework?: string;
  production_ref?: string;
  /** Subdirectorio del repo que contiene la app (monorepo). Vacío = raíz. */
  root_dir?: string;
  /**
   * Secreto del webhook git del proyecto. El control-plane lo entrega UNA SOLA VEZ
   * al crear (POST /projects) y luego solo a un humano admin/owner que pida el
   * detalle (GET /projects/{id}); nunca a un token de API. Sin él no se puede
   * configurar «git push → despliegue» en el proveedor.
   */
  webhook_secret?: string;
  /** Endpoint público al que el proveedor git debe enviar el push. */
  webhook_url?: string;
  /** Tamaño de instancia del catálogo (basic|standard|pro|pro-xl). */
  instance_size?: string;
  /** Instancias deseadas; es el mínimo del autoescalado. */
  instance_count?: number;
  /** Puerto en el que escucha la app (defecto 8080). */
  app_port?: number;
  /** Ruta de las sondas de salud (defecto «/»). */
  health_path?: string;
  /**
   * Comando que corre ANTES de arrancar la versión nueva, con su misma imagen y
   * su mismo entorno: el sitio de una migración de base de datos. Si falla, la
   * versión nueva no arranca.
   */
  release_command?: string;
  /** Si las previews del proyecto piden contraseña (el hash no sale de la API). */
  preview_protected?: boolean;
  /** Proyecto apagado sin borrar: cero réplicas y cero factura de cómputo. */
  paused?: boolean;
  /** Redirecciones y cabeceras aplicadas en el borde, antes de llegar a la app. */
  http_rules?: {
    redirects?: Array<{ from: string; to: string; status?: number }>;
    headers?: Record<string, string>;
  };
  created_at: string;
}

/** Tamaño de instancia del catálogo (lo que se reserva, se limita y se factura). */
export interface InstanceSize {
  name: string;
  label: string;
  cpu_request: string;
  memory_request: string;
  cpu_limit: string;
  memory_limit: string;
  /** Memoria en GiB: es la magnitud que se factura como compute_gb_hours. */
  memory_gib: number;
}

export interface InstanceSizeCatalog {
  instance_sizes: InstanceSize[];
  /** Tamaño de una app que no elige ninguno. */
  default: string;
  /** Tope de instancias por app (el techo real lo pone la cuota del plan). */
  max_count: number;
}

export type DeploymentStatus = 'queued' | 'building' | 'deploying' | 'ready' | 'error' | 'canceled';

export type DeploymentEnv = 'production' | 'preview';

export interface Deployment {
  id: string;
  project_id: string;
  /**
   * Producción o preview. El control-plane lo llama `target`, no `env`: `env` es
   * el ÁMBITO de una variable de entorno y son cosas distintas.
   */
  target: DeploymentEnv;
  status: DeploymentStatus;
  /** URL pública (prod <app>.apps.sytue.eu o preview <proyecto>-pr-<n>.preview.sytue.eu). */
  url?: string;
  commit_sha?: string;
  /**
   * Asunto del commit desplegado (o título del PR/MR en previews), saneado y
   * acotado por el control-plane. Vacío en lo que no nace de un push: redeploy
   * manual, rollback, plantilla e imagen BYO.
   */
  commit_message?: string;
  git_ref?: string;
  build_id?: string;
  /** Imagen BYO cuando el despliegue no viene de una build nuestra. */
  image_ref?: string;
  /** Número de PR para previews. */
  pr_number?: number;
  replicas?: number;
  created_by?: string;
  created_at: string;
  updated_at?: string;
  /** Cuándo pasó a ready. No existe un «finished»: un deployment ready sigue vivo. */
  ready_at?: string;
  /** Mensaje de error cuando status es `error`. */
  error?: string;
}

/** Tipos de servicio gestionado disponibles en Sytue DevX. */
export type ManagedServiceKind =
  | 'postgres'
  | 'storage'
  | 'functions'
  | 'cron'
  | 'job'
  /** Proceso permanente sin puerto (worker component). */
  | 'worker'
  | 'auth'
  /** App open-source del catálogo (caché, buscador, cola, CMS). Ver CatalogApp. */
  | 'app';

/**
 * Entrada del catálogo de apps open-source (GET /app-catalog).
 *
 * El cliente NUNCA manda una imagen: manda el `id` y la plataforma pone el resto
 * —imagen fijada por digest, replicada en el registro de la UE, licencia
 * comprobada y límites obligatorios—.
 */
export interface CatalogApp {
  id: string;
  name: string;
  description: string;
  // Las tres últimas llegan con los productos COMPLETOS del catálogo —los que
  // se abren en un navegador y usa una persona, no otra máquina—. Sin ellas todo
  // lo que no fuera caché, cola o buscador caía en «herramientas».
  category:
    'cache' | 'search' | 'queue' | 'cms' | 'tools' | 'devtools' | 'observability' | 'productivity';
  /**
   * SPDX. Permisivas y copyleft fuerte (AGPL/GPL/LGPL), nunca «source
   * available» —SSPL, BUSL, Elastic v2, Sustainable Use—, que prohíben
   * exactamente esto: ofrecerlas como servicio gestionado. ADR 0032.
   */
  license: string;
  homepage: string;
  /**
   * Repositorio del código fuente. Presente siempre que la licencia sea
   * copyleft: es cómo se cumple el AGPL §13 con quien usa el servicio, así que
   * la interfaz lo enseña junto a la licencia y no en letra pequeña.
   */
  source?: string;
  /** De dónde se replicó, con su etiqueta. Hace el espejo auditable. */
  upstream: string;
  /** Digest del origen. Una copia fiel lo conserva, así que coincide con el de `image`. */
  upstream_digest: string;
  /**
   * Digest de la copia en el registro de Sytue. Ausente = todavía no replicada.
   * El host no viaja: lo compone el control-plane con su REGISTRY_URL.
   */
  mirrored_digest?: string;
  version: string;
  ports: { name: string; port: number }[];
  /** Variables que este servicio inyecta en tu app al conectarse. */
  exports: { key: string; value: string }[];
  /**
   * Otras apps del catálogo con las que esta se conecta sola cuando conviven en
   * un proyecto. `docs` es la página OFICIAL que documenta la integración: sin
   * ella la entrada no entra en el catálogo, porque «Sytue las conecta solas»
   * no puede significar «Sytue se inventa la configuración de otro».
   */
  integrations?: { app: string; docs: string; env: { key: string; value: string }[] }[];
  disk_gib: number;
  data_path?: string;
  /** Directorios efímeros que la imagen necesita escribir con la raíz de solo lectura. */
  writable_paths?: string[];
  memory_mib: number;
  cpu_milli: number;
  run_as_user: number;
  health_path?: string;
  /**
   * Puerto por el que la app se publica en internet, con hostname propio y TLS.
   * Ausente o 0 = no se publica y solo se alcanza desde dentro del proyecto.
   * Lo decide el catálogo, nunca el cliente: publicar el panel de administración
   * de una cola con la contraseña recién generada es una brecha a un clic.
   */
  web_port?: number;
  /**
   * Servicios gestionados del proyecto que la app necesita para arrancar (hoy
   * solo `postgres`). El alta los EXIGE, así que el panel lo dice antes de
   * dejar elegirla: descubrirlo al pulsar «crear» es enterarse tarde.
   */
  needs?: string[];
}

/**
 * Lo que se dedujo de un repositorio (GET /inspect-repo).
 *
 * `detected:false` NO es un error: significa que no se pudo mirar —forja
 * desconocida, repositorio privado, servidor caído— y que el formulario tiene que
 * preguntar, como hacía antes. Bloquear el alta por esto habría cambiado una
 * molestia por un muro.
 */
export interface RepoInspection {
  detected: boolean;
  /** Por qué no se pudo mirar. Solo con `detected:false`. */
  reason?: string;
  detection?: {
    framework: string;
    build_strategy?: 'dockerfile' | 'buildpacks';
    install_command?: string;
    build_command?: string;
    output_dir?: string;
    /**
     * Si se puede aplicar sin preguntar. En falso se dedujo algo razonable pero
     * no comprobable, y enseñarlo es mejor que aplicarlo en silencio.
     */
    confident: boolean;
    /** El fichero que lo delató. Se enseña: es lo que hace comprobable la magia. */
    evidence?: string;
  };
}

/** Respuesta de GET /app-catalog. */
export interface AppCatalog {
  /** Las que se pueden crear ahora mismo. */
  apps: CatalogApp[];
  /**
   * Escritas y revisadas, pero con la imagen sin replicar todavía. Se enseñan
   * como «en camino»: ofrecerlas fallaría al tirar de la imagen, después de
   * haber prometido el servicio.
   */
  pending: string[];
  allowed_licenses: string[];
}

/**
 * Build de un commit del proyecto (imagen OCI en el registro privado del tenant).
 *
 * `image_ref` solo viene en las que terminaron bien: es la referencia por digest
 * que hace falta para levantar una función, un cron o un job con código que YA
 * está construido, sin pedirle al usuario que la escriba a mano.
 */
export interface Build {
  id: string;
  project_id: string;
  commit_sha: string;
  git_ref: string;
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled';
  framework?: string;
  image_ref?: string;
  created_at: string;
  finished_at?: string;
}

export interface ManagedService {
  id: string;
  project_id: string;
  kind: ManagedServiceKind;
  name: string;
  status: 'provisioning' | 'ready' | 'error';
  /** Configuración NO sensible (host, base de datos, bucket, rest_url). */
  config?: Record<string, string>;
  /**
   * A qué despliegues entrega sus credenciales: `production` (lo normal) o
   * `preview`. Una vista previa solo recibe las de los servicios de vistas
   * previas, nunca las de producción (CLOUD-01).
   */
  target: DeploymentEnv;
  created_at: string;
  updated_at?: string;
}

/**
 * Ámbito de una variable: en qué despliegues se inyecta. `preview` es lo que
 * permite que una rama sin revisar corra con credenciales distintas de las de
 * producción. Vacío/ausente = `all` (el defecto del control-plane).
 */
export type EnvScope = 'all' | 'production' | 'preview';

export interface EnvVar {
  key: string;
  /** Valor enmascarado si es secreto (la API no devuelve secretos en claro). */
  value?: string;
  /** Nombre REAL del campo en la API (domain.EnvVar json:"is_secret"). */
  is_secret?: boolean;
  /** Despliegues en los que se inyecta. Ausente = `all`. */
  scope?: EnvScope;
  /** @deprecated La API nunca lo ha enviado con este nombre. Usar is_secret. */
  secret?: boolean;
}

export interface CloudDomain {
  id: string;
  project_id: string;
  hostname: string;
  status: 'pending' | 'verified' | 'active' | 'failed';
  is_primary: boolean;
  verify_token?: string;
  created_at: string;
}

/** Instrucciones DNS que devuelve el control-plane al asociar un dominio propio. */
export interface DomainDnsInstructions {
  /** Destino del CNAME (lo fija la configuración del control-plane, no la UI). */
  cname_target?: string;
  /** Nombre del registro TXT de verificación (_sytue-challenge.<hostname>). */
  txt_record?: string;
  /** Valor del TXT: el token de verificación del dominio. */
  txt_value?: string;
}

export interface AddDomainResult {
  domain: CloudDomain;
  dns?: DomainDnsInstructions;
}

export interface CloudOrganization {
  id: string;
  slug: string;
  name: string;
  plan: 'free' | 'pro' | 'team';
  region: string;
  created_at: string;
  updated_at: string;
}

export interface UsageAggregate {
  metric: string;
  total: number;
  unit: string;
}

/**
 * Un registro de consumo SIN agregar. Lo que aporta frente a `UsageAggregate` es
 * `project_id`: el agregado dice cuánto se gastó, este dice QUIÉN lo gastó.
 *
 * Es opcional porque hay consumo que no es de ningún proyecto (el de la propia
 * organización); quien lo pinte tiene que contemplar ese caso en vez de esconderlo.
 */
export interface UsageRecord {
  id: string;
  project_id?: string;
  metric: string;
  quantity: number;
  unit: string;
  period_from: string;
  period_to: string;
  created_at: string;
}

/** Estado del último sondeo de un uptime check. "" = aún sin sondear (baseline). */
export type UptimeStatus = '' | 'up' | 'down';

/** Sonda HTTP periódica de disponibilidad de una URL pública (Wave 2). */
export interface UptimeCheck {
  id: string;
  project_id: string;
  name: string;
  target_url: string;
  interval_seconds: number;
  enabled: boolean;
  next_run_at: string;
  last_status: UptimeStatus;
  last_latency_ms: number;
  last_checked_at?: string;
  created_at: string;
}

/** Eventos del control plane que una alert policy puede enrutar a un webhook. */
export type AlertEvent =
  | 'deploy.failed'
  | 'deploy.ready'
  | 'uptime.down'
  | 'uptime.up'
  | 'billing.spend_threshold'
  | 'sizing.advice'
  // Consumo pegado al techo del tamaño contratado (migración 0040). Es la alerta
  // que llega ANTES del reinicio por falta de memoria, y por eso no basta con
  // tenerla en el tipo del dashboard: este es el contrato que consume todo el
  // que hable con el control-plane.
  | 'metric.cpu_high'
  | 'metric.memory_high';

/** Política de alerta: enruta un evento a un canal (webhook Slack/Discord/genérico). */
export interface AlertPolicy {
  id: string;
  /** Ausente/null = alcance organización (casa cualquier proyecto). */
  project_id?: string;
  event: AlertEvent;
  channel_url: string;
  enabled: boolean;
  created_at: string;
}

/**
 * Lo que quedó creado al aplicar una plantilla (control-plane:
 * service.TemplateApplied, aplanado en la respuesta junto al webhook).
 *
 * Los campos van en snake_case porque son LOS DE GO tal cual: `ManagedService` y
 * `Deployment` de este SDK están declarados en camelCase y no describen lo que
 * llega por el cable, así que reutilizarlos aquí sería copiar el error.
 *
 * NUNCA trae el valor de una variable secreta: los secretos que genera la
 * plataforma se guardan cifrados y no vuelven (`env[].value` vacío si `is_secret`).
 */
export interface TemplateApplied {
  project: Project;
  /** Servicios gestionados aprovisionados por la plantilla. */
  services: Array<{
    id: string;
    project_id: string;
    kind: string;
    name: string;
    status: string;
    created_at: string;
  }>;
  /** Variables persistidas. Las `from_service` no están: las inyecta el controlador. */
  env: EnvVar[];
  /** Build encolada del primer despliegue. */
  build: { id: string; project_id: string; git_ref: string; status: string };
  /** Primer despliegue, ya en marcha: el proyecto nace ejecutándose. */
  deployment: {
    id: string;
    project_id: string;
    target: string;
    status: string;
    git_ref: string;
    url: string;
  };
  template_id: string;
  /** Revisión del catálogo con la que se creó. */
  template_version: string;
  /** Endpoint público al que el proveedor git debe enviar el push. */
  webhook_url?: string;
  /**
   * Secreto de firma del webhook. El control-plane lo entrega UNA sola vez y solo
   * a un humano (nunca a un token de API): sin él no hay «git push → despliegue».
   */
  webhook_secret?: string;
}

export class CloudClient {
  private readonly http: HttpClient;

  constructor(options: CloudOptions) {
    this.http = new HttpClient({
      baseUrl: `${options.baseUrl.replace(/\/$/, '')}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /** Proyectos del usuario/tenant. */
  async listProjects(): Promise<Project[]> {
    // El control plane envuelve las listas en un sobre ({"projects":[...]}); lo
    // desenvolvemos aquí para exponer un array plano al consumidor.
    const res = await this.http.json<{ projects?: Project[] }>({ path: '/projects' });
    return res.projects ?? [];
  }

  /**
   * Crea un proyecto (opcionalmente conectado a un repo). La respuesta trae, UNA
   * SOLA VEZ, `webhook_secret` + `webhook_url`: sin ellos no hay forma de conectar
   * «git push → despliegue» desde ninguna superficie.
   */
  createProject(input: {
    slug: string;
    name?: string;
    repo_url?: string;
    framework?: string;
    production_ref?: string;
    root_dir?: string;
  }): Promise<Project> {
    return this.http.postJson<Project>('/projects', {
      slug: input.slug,
      name: input.name ?? input.slug,
      repo_url: input.repo_url,
      framework: input.framework,
      production_ref: input.production_ref,
      root_dir: input.root_dir,
    });
  }

  /**
   * Da de alta un proyecto ENTERO desde una plantilla del catálogo en UNA sola
   * petición y de forma ATÓMICA: proyecto, servicios gestionados, variables de
   * entorno y el PRIMER despliegue encolado (control-plane: ApplyTemplate). Si
   * algo falla no queda nada a medias.
   *
   * `answers` responde SOLO a las variables con `origin: "ask"` de la plantilla:
   * una clave que la plantilla no declara es un 400, no un descuido tolerado (si
   * no, sería un canal para escribir variables arbitrarias en el pod de la app).
   * Las variables `generate` las crea y CIFRA el servidor: no viajan en claro ni
   * de ida ni de vuelta.
   *
   * Los nombres del cuerpo son los que decodifica `createFromTemplateRequest`
   * (services/cloud/internal/http/handlers_from_template.go), verificados por el
   * test de este módulo: Go descarta en silencio cualquier clave que no reconoce,
   * así que un nombre «razonable» pero distinto se pierde sin error.
   */
  /** Catálogo de apps open-source que se pueden levantar junto a la app. */
  appCatalog(): Promise<AppCatalog> {
    return this.http.json<AppCatalog>({ path: '/app-catalog' });
  }

  createProjectFromTemplate(input: {
    template_id: string;
    slug: string;
    name?: string;
    repo_url: string;
    answers?: Record<string, string>;
    /** Pisa el tamaño sugerido por la plantilla. Vacío/ausente = el suyo. */
    instance_size?: string;
  }): Promise<TemplateApplied> {
    return this.http.postJson<TemplateApplied>('/projects:from-template', {
      template_id: input.template_id,
      slug: input.slug,
      name: input.name ?? input.slug,
      repo_url: input.repo_url,
      answers: input.answers ?? {},
      instance_size: input.instance_size ?? '',
    });
  }

  /**
   * Nombre libre sugerido para un proyecto nuevo. El control-plane lo comprueba
   * contra el índice global de slugs antes de devolverlo; `available` dice si esa
   * comprobación pudo confirmarlo.
   *
   * El nombre se genera con la gramática del idioma negociado (§13.1), así que la
   * respuesta trae el idioma que se usó de verdad, no el que se pidió.
   */
  suggestProjectSlug(opts: SlugLanguageOptions = {}): Promise<SlugSuggestion> {
    return this.http.json<SlugSuggestion>({
      path: '/suggest-slug',
      query: { lang: opts.lang },
      ...acceptLanguage(opts),
    });
  }

  /**
   * Disponibilidad del nombre que ha escrito el usuario, con alternativas LIBRES
   * derivadas de él cuando ya está cogido.
   *
   * Pensado para llamarse mientras se teclea: dale debounce en el cliente. No
   * reserva nada, así que un `available: true` puede dejar de serlo antes de crear;
   * la creación sigue siendo la única palabra definitiva.
   */
  checkProjectSlug(slug: string, opts: SlugLanguageOptions = {}): Promise<SlugCheck> {
    return this.http.json<SlugCheck>({
      path: '/check-slug',
      query: { slug, lang: opts.lang },
      ...acceptLanguage(opts),
    });
  }

  /**
   * Deduce cómo desplegar un repositorio mirándolo por la API de su forja.
   *
   * Pensado para el alta: evita preguntar framework, comandos y directorio de
   * salida, que están escritos en el repositorio que el usuario acaba de pegar.
   * NUNCA hay que dejar que un fallo suyo impida crear el proyecto — por eso
   * `detected:false` es una respuesta 200 y no un error.
   */
  inspectRepo(repoUrl: string, rootDir = ''): Promise<RepoInspection> {
    return this.http.json<RepoInspection>({
      path: '/inspect-repo',
      query: { repo_url: repoUrl, root_dir: rootDir || undefined },
    });
  }

  /**
   * Catálogo de tamaños de instancia. Es catálogo del PRODUCTO, no del tenant: se
   * pide a la API en vez de replicarlo en cada superficie para que nadie ofrezca
   * un tamaño que el control-plane ya no acepta.
   */
  listInstanceSizes(): Promise<InstanceSizeCatalog> {
    return this.http.json<InstanceSizeCatalog>({ path: '/instance-sizes' });
  }

  /**
   * Actualización PARCIAL del proyecto. Solo viajan las claves presentes en
   * `patch`: la API interpreta cada campo ausente como «no lo toques», así que
   * enviar el objeto entero machacaría lo que no se está editando.
   */
  updateProject(
    projectId: string,
    patch: {
      name?: string;
      repo_url?: string;
      production_ref?: string;
      framework?: string;
      instance_size?: string;
      instance_count?: number;
      app_port?: number;
      health_path?: string;
      release_command?: string;
      paused?: boolean;
      http_rules?: {
        redirects?: Array<{ from: string; to: string; status?: number }>;
        headers?: Record<string, string>;
      };
    },
  ): Promise<Project> {
    return this.http.json<Project>({
      method: 'PATCH',
      path: `/projects/${encodeURIComponent(projectId)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    });
  }

  /**
   * Da de baja un proyecto entero (DESTRUCTIVO).
   *
   * En el control-plane es un borrado lógico que además revoca los tokens
   * acotados a ese proyecto en la misma transacción, y el reconciliador retira
   * después sus objetos del clúster. Exige rol admin/owner.
   */
  deleteProject(projectId: string): Promise<void> {
    return this.http
      .text({ method: 'DELETE', path: `/projects/${encodeURIComponent(projectId)}` })
      .then(() => undefined);
  }

  /**
   * Emite un secreto de webhook NUEVO para el proyecto y devuelve el claro una
   * vez. El anterior deja de valer en el acto.
   */
  rotateWebhookSecret(projectId: string): Promise<{ webhook_secret: string; webhook_url: string }> {
    return this.http.postJson(
      `/projects/${encodeURIComponent(projectId)}/webhook-secret:rotate`,
      {},
    );
  }

  /**
   * Pide una copia inmediata del Postgres gestionado. Devuelve en cuanto el
   * control-plane acepta (202): el Job corre en el clúster.
   */
  backupServiceNow(serviceId: string): Promise<void> {
    return this.http
      .postJson(`/services/${encodeURIComponent(serviceId)}/backup`, {})
      .then(() => undefined);
  }

  /**
   * Cierra las previews del proyecto con contraseña y la devuelve en claro UNA
   * vez. Sobre una protección activa, la rota.
   */
  protectPreviews(projectId: string): Promise<{ username: string; password: string }> {
    return this.http.postJson(`/projects/${encodeURIComponent(projectId)}/preview-protection`, {});
  }

  /** Vuelve a dejar las previews abiertas. */
  unprotectPreviews(projectId: string): Promise<void> {
    return this.http
      .text({
        method: 'DELETE',
        path: `/projects/${encodeURIComponent(projectId)}/preview-protection`,
      })
      .then(() => undefined);
  }

  /** Organización del tenant (se crea perezosamente si no existe). */
  getOrg(): Promise<CloudOrganization> {
    return this.http.json<CloudOrganization>({ path: '/org' });
  }

  /** Uso agregado del tenant en el periodo (?from=&to= ISO). */
  async getUsage(options?: { from?: string; to?: string }): Promise<UsageAggregate[]> {
    const res = await this.http.json<{ usage?: UsageAggregate[] }>({
      path: '/usage',
      query: { from: options?.from, to: options?.to },
    });
    return res.usage ?? [];
  }

  /**
   * Consumo del periodo registro a registro (?from=&to=&limit=), con el proyecto que
   * lo generó. Es lo que permite responder «¿qué me está subiendo la factura?», que
   * el agregado por métrica no puede contestar.
   */
  async getUsageDetail(options?: {
    from?: string;
    to?: string;
    limit?: number;
  }): Promise<UsageRecord[]> {
    const res = await this.http.json<{ records?: UsageRecord[] }>({
      path: '/usage/detail',
      query: {
        from: options?.from,
        to: options?.to,
        limit: options?.limit == null ? undefined : String(options.limit),
      },
    });
    return res.records ?? [];
  }

  /** Dominios de un proyecto. */
  async listDomains(projectId: string): Promise<CloudDomain[]> {
    const res = await this.http.json<{ domains?: CloudDomain[] }>({
      path: `/projects/${encodeURIComponent(projectId)}/domains`,
    });
    return res.domains ?? [];
  }

  /**
   * Asocia un dominio personalizado y devuelve los DOS registros DNS que hay que
   * crear: el CNAME que manda el tráfico y el TXT que PRUEBA la propiedad de la
   * zona. Sin el TXT el dominio no se verifica nunca (y sin verificar no hay TLS),
   * así que quien llame tiene que enseñar `dns` entero, no solo el CNAME.
   */
  addDomain(projectId: string, hostname: string): Promise<AddDomainResult> {
    return this.http.postJson(`/projects/${encodeURIComponent(projectId)}/domains`, { hostname });
  }

  /** Elimina un dominio por ID. */
  deleteDomain(domainId: string): Promise<void> {
    return this.http
      .text({ method: 'DELETE', path: `/domains/${encodeURIComponent(domainId)}` })
      .then(() => undefined);
  }

  /** Marca un dominio como principal. */
  setPrimaryDomain(domainId: string): Promise<CloudDomain> {
    return this.http
      .json<{ domain: CloudDomain }>({
        method: 'PATCH',
        path: `/domains/${encodeURIComponent(domainId)}`,
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ is_primary: true }),
      })
      .then((r) => r.domain);
  }

  /** Cancela una build en curso o en cola. */
  cancelBuild(buildId: string): Promise<void> {
    return this.http
      .text({ method: 'DELETE', path: `/builds/${encodeURIComponent(buildId)}` })
      .then(() => undefined);
  }

  /** Lista objetos del bucket de storage del proyecto. */
  async listStorageObjects(
    projectId: string,
    prefix?: string,
  ): Promise<{
    storage: { bucket: string; endpoint: string; access_key: string; region: string };
    objects: Array<{ key: string; size: number; last_modified: string }>;
  }> {
    return this.http.json({
      path: `/projects/${encodeURIComponent(projectId)}/storage/objects`,
      query: { prefix },
    });
  }

  /** URL prefirmada para subir/descargar un objeto. */
  presignStorage(projectId: string, key: string, method: 'GET' | 'PUT' = 'PUT'): Promise<string> {
    return this.http
      .postJson<{ url: string }>(`/projects/${encodeURIComponent(projectId)}/storage/presign`, {
        key,
        method,
      })
      .then((r) => r.url);
  }

  /**
   * Borra un objeto del bucket del proyecto (destructivo, sin versionado).
   *
   * La clave va en query y no en la ruta: las de S3 llevan barras dentro, y en un
   * segmento habría que escaparlas dos veces para llegar iguales al servidor.
   */
  deleteStorageObject(projectId: string, key: string): Promise<void> {
    return this.http
      .text({
        method: 'DELETE',
        path: `/projects/${encodeURIComponent(projectId)}/storage/objects`,
        query: { key },
      })
      .then(() => undefined);
  }

  /** Elimina una variable de entorno / secreto. */
  deleteEnv(projectId: string, key: string): Promise<void> {
    return this.http
      .text({
        method: 'DELETE',
        path: `/projects/${encodeURIComponent(projectId)}/env/${encodeURIComponent(key)}`,
      })
      .then(() => undefined);
  }

  /** Detalle de un proyecto. */
  getProject(projectId: string): Promise<Project> {
    return this.http.json<Project>({ path: `/projects/${encodeURIComponent(projectId)}` });
  }

  /**
   * Qué hacer con el tamaño del proyecto según lo que consume DE VERDAD, y cuánto
   * cambia eso la factura. `action: 'keep'` con `reason: 'insufficient_data'` es la
   * respuesta normal de un proyecto recién creado, no un error.
   *
   * Para APLICARLA se usa `updateProject` con `instance_size`: no hay verbo propio,
   * porque ese camino ya valida contra el catálogo, audita y respeta la cuota.
   */
  projectSizing(projectId: string): Promise<SizingAdvice> {
    return this.http.json<SizingAdvice>({
      path: `/projects/${encodeURIComponent(projectId)}/sizing`,
    });
  }

  /** Despliegues de un proyecto (mas recientes primero). */
  async listDeployments(
    projectId: string,
    options?: { env?: DeploymentEnv; limit?: number },
  ): Promise<Deployment[]> {
    // Sobre {"deployments":[...],"next_cursor":...}; devolvemos solo el array.
    const res = await this.http.json<{ deployments?: Deployment[] }>({
      path: `/projects/${encodeURIComponent(projectId)}/deployments`,
      query: { env: options?.env, limit: options?.limit ?? 20 },
    });
    return res.deployments ?? [];
  }

  /** Dispara un despliegue manual (p.ej. promover un commit a produccion). */
  createDeployment(
    projectId: string,
    input: { commitSha?: string; env?: DeploymentEnv },
  ): Promise<Deployment> {
    return this.http.postJson<Deployment>(
      `/projects/${encodeURIComponent(projectId)}/deployments`,
      input,
    );
  }

  /** Revierte (rollback) a un despliegue previo. */
  rollback(projectId: string, deploymentId: string): Promise<Deployment> {
    return this.http.postJson<Deployment>(
      `/projects/${encodeURIComponent(projectId)}/deployments/${encodeURIComponent(deploymentId)}/rollback`,
      {},
    );
  }

  /**
   * Logs de build/runtime de un despliegue via SSE. El token va por query porque
   * EventSource no admite cabeceras; el control plane lo valida via JWKS.
   */
  async streamDeploymentLogs(projectId: string, deploymentId: string): Promise<EventSource> {
    const token = await this.http.accessToken();
    const url = this.http.buildUrl({
      path: `/projects/${encodeURIComponent(projectId)}/deployments/${encodeURIComponent(deploymentId)}/logs`,
      query: { access_token: token },
    });
    return new EventSource(url, { withCredentials: true });
  }

  /** Servicios gestionados de un proyecto. */
  async listServices(projectId: string): Promise<ManagedService[]> {
    // Sobre {"services":[...]}.
    const res = await this.http.json<{ services?: ManagedService[] }>({
      path: `/projects/${encodeURIComponent(projectId)}/services`,
    });
    return res.services ?? [];
  }

  /** Aprovisiona un servicio gestionado (Postgres, Storage, Functions, Cron). */
  /**
   * Aprovisiona un servicio gestionado.
   *
   * `config` es lo específico de cada tipo. Para `kind: 'app'` lleva el `app_id`
   * de la entrada del catálogo — y SOLO eso: la imagen no viaja nunca desde el
   * cliente (ADR 0029).
   */
  createService(
    projectId: string,
    input: {
      kind: ManagedServiceKind;
      name: string;
      config?: Record<string, string>;
      /** Sin él, producción. */
      target?: DeploymentEnv;
    },
  ): Promise<ManagedService> {
    return this.http.postJson<ManagedService>(
      `/projects/${encodeURIComponent(projectId)}/services`,
      input,
    );
  }

  /**
   * Cambia nombre y/o configuración de un servicio gestionado. El `config` se
   * MEZCLA con el existente: solo viajan las claves que se quieren cambiar.
   */
  updateService(
    serviceId: string,
    patch: { name?: string; config?: Record<string, string> },
  ): Promise<ManagedService> {
    return this.http.json<ManagedService>({
      method: 'PATCH',
      path: `/services/${encodeURIComponent(serviceId)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    });
  }

  /**
   * Da de baja un servicio gestionado (DESTRUCTIVO: el reconciliador retira sus
   * objetos del clúster, y con Postgres eso incluye el volumen).
   *
   * Va por id de SERVICIO, no por proyecto: es el identificador con el que el
   * control-plane resuelve la pertenencia al tenant bajo RLS.
   */
  deleteService(serviceId: string): Promise<void> {
    return this.http
      .text({ method: 'DELETE', path: `/services/${encodeURIComponent(serviceId)}` })
      .then(() => undefined);
  }

  /**
   * Builds del proyecto, de la más reciente a la más antigua.
   *
   * Lo que de verdad hace falta de aquí es `image_ref`: es la ÚNICA forma de que
   * el panel ofrezca crear una función o un job con una imagen que existe, en vez
   * de pedirle al usuario que escriba a mano una referencia OCI del registro
   * privado que no tiene por qué conocer.
   */
  async listBuilds(projectId: string, limit = 20): Promise<Build[]> {
    const res = await this.http.json<{ builds?: Build[] }>({
      path: `/projects/${encodeURIComponent(projectId)}/builds`,
      query: { limit: String(limit) },
    });
    return res.builds ?? [];
  }

  /** Variables de entorno / secretos del proyecto (valores secretos enmascarados). */
  async listEnv(projectId: string): Promise<EnvVar[]> {
    // Sobre {"env":[...]}.
    const res = await this.http.json<{ env?: EnvVar[] }>({
      path: `/projects/${encodeURIComponent(projectId)}/env`,
    });
    return res.env ?? [];
  }

  /**
   * Crea o actualiza una variable de entorno / secreto.
   *
   * El cuerpo viaja como `is_secret`/`scope`, que son los nombres que decodifica
   * el control-plane (setEnvRequest). Se enviaba `secret`, un campo que Go
   * descartaba en silencio: TODA variable creada desde la web se guardaba como no
   * secreta y con ámbito por defecto.
   */
  setEnv(
    projectId: string,
    input: { key: string; value: string; secret?: boolean; scope?: EnvScope },
  ): Promise<void> {
    return this.http.json<void>({
      method: 'PUT',
      path: `/projects/${encodeURIComponent(projectId)}/env/${encodeURIComponent(input.key)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        value: input.value,
        is_secret: input.secret ?? false,
        ...(input.scope ? { scope: input.scope } : {}),
      }),
    });
  }

  // ---- Monitorización de uptime (Wave 2) ----

  /** Sondas de uptime de un proyecto. */
  async listUptimeChecks(projectId: string): Promise<UptimeCheck[]> {
    // Sobre {"uptime_checks":[...]}.
    const res = await this.http.json<{ uptime_checks?: UptimeCheck[] }>({
      path: `/projects/${encodeURIComponent(projectId)}/uptime-checks`,
    });
    return res.uptime_checks ?? [];
  }

  /** Crea una sonda. target_url vacío => URL del deployment de producción activo. */
  createUptimeCheck(
    projectId: string,
    input: { name: string; target_url?: string; interval_seconds?: number; enabled?: boolean },
  ): Promise<UptimeCheck> {
    return this.http.postJson<UptimeCheck>(
      `/projects/${encodeURIComponent(projectId)}/uptime-checks`,
      input,
    );
  }

  /** Actualización parcial de una sonda por ID. */
  updateUptimeCheck(
    checkId: string,
    patch: { name?: string; target_url?: string; interval_seconds?: number; enabled?: boolean },
  ): Promise<UptimeCheck> {
    return this.http.json<UptimeCheck>({
      method: 'PATCH',
      path: `/uptime-checks/${encodeURIComponent(checkId)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(patch),
    });
  }

  /** Elimina una sonda por ID. */
  deleteUptimeCheck(checkId: string): Promise<void> {
    return this.http
      .text({ method: 'DELETE', path: `/uptime-checks/${encodeURIComponent(checkId)}` })
      .then(() => undefined);
  }

  // ---- Alertas por evento (Wave 2; requiere rol admin/owner) ----

  /** Políticas de alerta del tenant. */
  async listAlertPolicies(): Promise<AlertPolicy[]> {
    // Sobre {"alert_policies":[...]}.
    const res = await this.http.json<{ alert_policies?: AlertPolicy[] }>({ path: '/alerts' });
    return res.alert_policies ?? [];
  }

  /** Crea una política de alerta. project_id ausente => alcance organización. */
  createAlertPolicy(input: {
    project_id?: string;
    event: AlertEvent;
    channel_url: string;
    enabled?: boolean;
  }): Promise<AlertPolicy> {
    return this.http.postJson<AlertPolicy>('/alerts', input);
  }

  /** Elimina una política de alerta por ID. */
  deleteAlertPolicy(id: string): Promise<void> {
    return this.http
      .text({ method: 'DELETE', path: `/alerts/${encodeURIComponent(id)}` })
      .then(() => undefined);
  }
}
