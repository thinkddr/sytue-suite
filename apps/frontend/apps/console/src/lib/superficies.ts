/*
 * QUÉ PANTALLA EXISTE EN QUÉ SUPERFICIE.
 *
 * Este binario sirve dos consolas distintas con dos clients OIDC distintos:
 * `admin.<dominio>` administra UNA organización y `platform.<dominio>` administra
 * TODAS. Compartir el binario está bien; compartir las pantallas no.
 *
 * Hasta ahora el host de plataforma servía además la consola de organización
 * entera: `platform.sytue.eu/users` abría el directorio de la organización a la
 * que resulta que pertenece quien opera la plataforma, con su menú al lado. No
 * era un fallo de permisos —el aislamiento por organización lo sigue aplicando el
 * servicio— pero sí una pista falsa: invita a administrar «una» desde la consola
 * de «todas», y quien mira no distingue en cuál está actuando.
 *
 * La regla vive aquí, y no dentro del middleware, para poder probarla sin
 * levantar Astro: importar `astro:middleware` desde un test no es posible.
 */

/**
 * `platform.<dominio>` es la administración global. Se reconoce por el host y no
 * por una variable de entorno porque el mismo pod sirve los dos: la superficie la
 * decide quién llama, no cómo se desplegó.
 */
export function esHostDePlataforma(host: string | null): boolean {
  return host !== null && host.split(':')[0]!.startsWith('platform.');
}

/** Lo único que se sirve desde el host de plataforma. */
const RUTAS_DE_PLATAFORMA = [
  '/platform', // la consola global
  '/conexiones', // credenciales de la plataforma ante terceros (sección propia)
  '/moderacion', // la cola de denuncias del operador (Cast y Rooms)
  '/account', // la cuenta de quien opera, que es suya en cualquier host
  '/auth/', // el baile OIDC del client sytue-platform
  '/api/platform', // cubre también /api/platform-connections y /api/platform-moderation
  '/api/me/', // datos de la propia sesión
];

/** Recursos del framework: no son pantallas de nadie y pasan siempre. */
function esRecurso(p: string): boolean {
  return (
    p.startsWith('/_astro/') ||
    p.startsWith('/_image') ||
    p === '/favicon.ico' ||
    p === '/robots.txt' ||
    p.startsWith('/fonts/') ||
    p.startsWith('/img/')
  );
}

/**
 * ¿Esta ruta se sirve desde el host de plataforma?
 *
 * Prefijo y no igualdad: `/api/platform` tiene que cubrir también
 * `/api/platform-connections`, y `/auth/` toda la ida y vuelta del OIDC. La raíz
 * pasa porque el middleware la reescribe a `/platform`.
 */
export function esRutaDePlataforma(ruta: string): boolean {
  return ruta === '/' || esRecurso(ruta) || RUTAS_DE_PLATAFORMA.some((p) => ruta.startsWith(p));
}
