/**
 * Las cabeceras que Cast cambia respecto al estándar de la suite, por ruta.
 *
 * Aparte del middleware para poder probarlo: una excepción de `frame-ancestors`
 * que se escapa a otra ruta es un agujero de clickjacking, y una que no llega a
 * `/embed/` es un incrustado que no se ve en ningún sitio.
 *
 * La ruta puede llegar con la organización delante (`/cast/<org>/studio`) o sin
 * ella (`/cast/studio`): el reescrito interno de ADR 0056 no garantiza cuál ve
 * este paso, así que se aceptan las dos.
 */

/** CSP del reproductor incrustado: encuadrable desde cualquier sitio, nada más. */
export const CSP_INCRUSTADO = [
  "object-src 'none'",
  "base-uri 'self'",
  'frame-ancestors *',
  "form-action 'none'",
].join('; ');

const RE_EMBED = /^\/cast\/embed\/[^/]+\/?$/;
const RE_STUDIO = /^\/cast\/(?:[a-z0-9-]+\/)?studio\/?$/;

/**
 * Las cabeceras a sobrescribir para `pathname`. `politicaDeMedios` es la
 * Permissions-Policy con cámara, micrófono y pantalla (la de Rooms).
 */
export function cabecerasDeRuta(
  pathname: string,
  politicaDeMedios: string,
): Readonly<Record<string, string>> {
  if (RE_EMBED.test(pathname)) {
    // Un valor vacío OMITE la cabecera: `X-Frame-Options: DENY` ganaría a la CSP
    // en los navegadores que aún la miran primero.
    return { 'content-security-policy': CSP_INCRUSTADO, 'x-frame-options': '' };
  }
  if (RE_STUDIO.test(pathname)) return { 'permissions-policy': politicaDeMedios };
  return {};
}
