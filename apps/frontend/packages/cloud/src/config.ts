/**
 * De dónde saca el SDK sus datos: del entorno que Sytue DevX YA inyectó.
 *
 * Esta es la diferencia con cualquier otro SDK de este tipo. En los demás, el
 * primer paso siempre es «copia la URL y la clave del panel y pégalas en un
 * fichero .env» — que es exactamente donde se atasca quien empieza, y de paso la
 * primera forma de filtrar una credencial. Aquí no hay primer paso: el
 * controlador ya escribe DATABASE_URL, POSTGREST_URL, S3_* y OIDC_* en el
 * Deployment de la app (`controller/deploy.go`), así que el SDK los LEE.
 *
 * `createClient()` sin argumentos es la API principal a propósito. Los
 * argumentos existen solo para dos casos reales —desarrollo en el portátil y
 * pruebas—, y se documentan como tales.
 */

/** Lo que la plataforma deja escrito en el entorno de cada app. */
export interface CloudConfig {
  /**
   * Idioma de los mensajes de error. Sale de `SYTUE_LANG` o `LANG` —que ya trae
   * cualquier contenedor— y por defecto es inglés. No es un adorno: «los errores
   * dicen qué hacer» es la función del paquete, y en un idioma que no entiendes
   * deja de existir.
   */
  language?: string;
  /** URL de la API de datos (PostgREST) del Sytue Postgres del proyecto. */
  restUrl?: string;
  /** Token con el que esa API reconoce a la app. */
  restToken?: string;
  /** Slug del proyecto y entorno; útiles para logs y para distinguir preview. */
  project?: string;
  target?: string;
}

/** Igual que `process.env`, sin depender de los tipos de Node. */
type EnvLike = Record<string, string | undefined>;

/**
 * Lee el entorno del proceso funcione donde funcione: Node, Bun y Deno lo
 * exponen distinto, y una app de cliente puede correr en cualquiera de los tres.
 */
function readEnv(): EnvLike {
  const g = globalThis as {
    process?: { env?: EnvLike };
    Deno?: { env?: { toObject: () => EnvLike } };
  };
  if (g.process?.env) return g.process.env;
  if (g.Deno?.env) return g.Deno.env.toObject();
  return {};
}

/** Construye la configuración desde el entorno (o desde uno dado, en pruebas). */
export function configFromEnv(env: EnvLike = readEnv()): CloudConfig {
  const cfg: CloudConfig = {};
  // Solo se asignan las claves presentes: con exactOptionalPropertyTypes, meter
  // `undefined` explícito no es lo mismo que no meter la clave.
  if (env.POSTGREST_URL) cfg.restUrl = env.POSTGREST_URL;
  if (env.PGRST_JWT) cfg.restToken = env.PGRST_JWT;
  if (env.SYTUE_PROJECT) cfg.project = env.SYTUE_PROJECT;
  if (env.SYTUE_DEPLOYMENT_TARGET) cfg.target = env.SYTUE_DEPLOYMENT_TARGET;
  const lang = env.SYTUE_LANG ?? env.LANG ?? env.LC_ALL;
  if (lang) cfg.language = lang;
  return cfg;
}

/**
 * Error del SDK con un mensaje que dice QUÉ hacer.
 *
 * «fetch failed» o «401 Unauthorized» no ayudan a quien está empezando: no
 * distinguen «me he equivocado de nombre de tabla» de «no he conectado la base
 * de datos». Cada error de aquí nombra la causa probable y el siguiente paso.
 */
export class SytueError extends Error {
  constructor(
    message: string,
    /** Lo que devolvió el servidor, para quien sí quiera mirarlo. */
    readonly detail?: unknown,
  ) {
    super(message);
    this.name = 'SytueError';
  }
}
