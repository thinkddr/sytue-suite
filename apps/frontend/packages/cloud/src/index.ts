/**
 * `@sytue/cloud` — el SDK de las apps que corren en Sytue DevX.
 *
 * Todo el paquete existe para que la primera línea útil sea la primera línea:
 *
 * ```js
 * import { sytue } from '@sytue/cloud';
 *
 * const posts = await sytue.table('posts').newest().limit(10).all();
 * await sytue.table('posts').add({ titulo: 'Hola' });
 * ```
 *
 * Sin URL que copiar, sin clave que pegar, sin `.env`. La plataforma ya escribió
 * las credenciales en el entorno de la app al desplegarla, así que el SDK las
 * lee. Ese paso —el que hace abandonar a quien empieza, y de paso la primera
 * forma de filtrar una credencial— simplemente no existe aquí.
 *
 * NO es el SDK del panel de control. Crear proyectos y despliegues es
 * `@sytue/api-clients`, que habla con el control-plane y lo usan las
 * herramientas, no las apps.
 *
 * Cero dependencias: solo `fetch`, que está en Node 18+, Bun y Deno.
 */
import { configFromEnv, type CloudConfig } from './config.js';
import { Table, type Row, type TableOptions } from './table.js';

export { SytueError, type CloudConfig } from './config.js';
export { LANGUAGES, type Language } from './messages.js';
export { Table, Query, type Row, type Scalar, type TableOptions } from './table.js';

/** Cliente de los servicios del proyecto. */
export class SytueCloud {
  constructor(readonly config: CloudConfig) {}

  /**
   * Una tabla de la base del proyecto.
   *
   * ```js
   * await sytue.table('posts').where({ publicado: true }).newest().all();
   * await sytue.table('usuarios', { key: 'email' }).find('ana@ejemplo.com');
   * ```
   */
  table<T extends Row = Row>(name: string, options?: TableOptions): Table<T> {
    return new Table<T>(this.config, name, options ?? {});
  }

  /**
   * Si el proyecto tiene base de datos conectada.
   *
   * Existe para que una plantilla pueda enseñar «conecta una base de datos» en
   * vez de reventar en la primera consulta: en Sytue DevX las credenciales se
   * inyectan AL DESPLEGAR, así que crear la base con la app corriendo la deja
   * aprovisionada y a la vez invisible hasta el siguiente despliegue.
   */
  get hasDatabase(): boolean {
    return Boolean(this.config.restUrl);
  }

  /** Slug del proyecto, tal y como aparece en el panel. */
  get project(): string {
    return this.config.project ?? '';
  }

  /** `production` o `preview`. Útil para no mandar correos de verdad en una preview. */
  get target(): string {
    return this.config.target ?? 'production';
  }

  /** True en un despliegue de vista previa (una rama, un PR). */
  get isPreview(): boolean {
    return this.target === 'preview';
  }
}

/**
 * Crea un cliente.
 *
 * Sin argumentos —que es como se usa dentro de Sytue DevX— lee la
 * configuración del entorno. Los argumentos existen para dos casos concretos:
 * desarrollar en el portátil contra la base de una preview, y las pruebas.
 */
export function createClient(overrides: CloudConfig = {}): SytueCloud {
  return new SytueCloud({ ...configFromEnv(), ...overrides });
}

/**
 * El cliente listo para usar. Es la forma en que se usa el 99 % de las veces, y
 * por eso es lo primero que aparece en la documentación.
 *
 * Se construye al importar: leer cuatro variables de entorno no cuesta nada y
 * no abre ninguna conexión —no hay pool que gestionar—, así que no hay motivo
 * para obligar a nadie a llamar a una función antes de escribir su primera
 * consulta.
 */
export const sytue: SytueCloud = createClient();

/** Alias para quien prefiera el nombre en inglés. */
export default sytue;
