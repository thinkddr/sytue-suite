/**
 * Enlaces de reunión de OTROS proveedores. Rooms es una sala WebRTC a la que
 * entra cualquiera con un navegador; la interoperabilidad con Zoom, Google Meet,
 * Teams y Webex es de dos sentidos y ninguna necesita su API:
 *
 *   - hacia fuera, un enlace de Rooms se comparte por WhatsApp, correo o Teams
 *     con la URL de «compartir» de cada uno (ver `enlaceParaCompartir`);
 *   - hacia dentro, pegar un enlace de Zoom/Meet/Teams/Webex en «Unirse con un
 *     enlace» lo reconoce y lo abre: Rooms es el sitio desde el que se entra a
 *     cualquier reunión, sea de quien sea.
 *
 * Puro y sin dependencias: se prueba con `bun test`.
 */

export type ProveedorExterno = 'zoom' | 'meet' | 'teams' | 'webex';

const NOMBRES: Record<ProveedorExterno, string> = {
  zoom: 'Zoom',
  meet: 'Google Meet',
  teams: 'Microsoft Teams',
  webex: 'Webex',
};

/** Nombre legible del proveedor (son marcas: no se traducen). */
export function nombreDeProveedor(p: ProveedorExterno): string {
  return NOMBRES[p];
}

/**
 * Reconoce un enlace de reunión externo por su host. Solo https y solo hosts
 * de los propios proveedores: un enlace que se abre en una pestaña nueva con
 * la sesión del navegador no puede llevar a cualquier sitio.
 */
export function proveedorDeReunion(raw: string): ProveedorExterno | null {
  let u: URL;
  try {
    u = new URL(raw.trim());
  } catch {
    return null;
  }
  if (u.protocol !== 'https:') return null;
  const h = u.hostname.toLowerCase();
  if (h === 'zoom.us' || h.endsWith('.zoom.us'))
    return /\/(j|my|s|w)\//.test(u.pathname) ? 'zoom' : null;
  if (h === 'meet.google.com')
    return /^\/[a-z]{3}-[a-z]{4}-[a-z]{3}$/.test(u.pathname) ? 'meet' : null;
  if (h === 'teams.microsoft.com' || h === 'teams.live.com') {
    return u.pathname.includes('/meetup-join/') || u.pathname.startsWith('/meet/') ? 'teams' : null;
  }
  if (h === 'webex.com' || h.endsWith('.webex.com')) {
    return /\/(meet|join|j\.php)/.test(u.pathname) ? 'webex' : null;
  }
  return null;
}

export type DestinoCompartir = 'whatsapp' | 'email' | 'teams';

/**
 * La URL de «compartir» de cada destino, con el texto ya compuesto. Ninguna
 * necesita clave de API ni sesión: son las mismas que usa cualquier botón de
 * compartir de una web. Slack y Google Chat no tienen una URL así; para ellos
 * está el webhook entrante del canal.
 */
export function enlaceParaCompartir(
  destino: DestinoCompartir,
  texto: string,
  url: string,
  asunto = '',
): string {
  switch (destino) {
    case 'whatsapp':
      return `https://wa.me/?text=${encodeURIComponent(`${texto} ${url}`)}`;
    case 'email':
      return `mailto:?subject=${encodeURIComponent(asunto)}&body=${encodeURIComponent(`${texto}\n${url}`)}`;
    case 'teams':
      return `https://teams.microsoft.com/share?href=${encodeURIComponent(url)}&msgText=${encodeURIComponent(texto)}`;
  }
}
