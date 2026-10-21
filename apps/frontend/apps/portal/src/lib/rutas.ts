/**
 * Rutas de ESTA app, con su prefijo y, en el navegador, con su organización.
 *
 * El Portal se construye con `base: '/one'`, así que vive en `one.sytue.eu/one`
 * y en `<organizacion>.sytue.eu/one`. Astro prefija sus assets y sus páginas,
 * pero no un `href="/actividad"` escrito a mano: eso pide la raíz del host,
 * donde el ingress no enruta nada, y el navegador recibe el 404 de Traefik.
 *
 * La organización como segmento de path (ADR 0056): la canónica es
 * `one.sytue.eu/one/<org>/actividad`, así que `r()` la añade leyendo la URL
 * ACTUAL (`orgActual`) — la navegación hidratada conserva el segmento y la barra
 * sigue enseñando la org en la que se está. En SSR no hay URL de navegador y los
 * enlaces salen sin org: el middleware de la app los manda a la canónica con un
 * 302 local. Las páginas que se sirven ya por el rewrite pueden pasar la org
 * explícita (`Astro.locals.org`) en el segundo parámetro y ahorran ese salto.
 *
 * Los prefijos de plomería (`/api`, `/auth`…) NO llevan org nunca: no la usan y
 * el middleware los deja pasar siempre (SIN_ORG_SIEMPRE).
 *
 * `import.meta.env.BASE_URL` lo sustituye Vite al construir ESTA app, de modo
 * que en dev (sin base) sigue siendo `/actividad` y no hay dos comportamientos.
 */
import { orgActual, rutaConOrg, SIN_ORG_SIEMPRE } from '@sytue/config';

export function r(path: string, org?: string | null): string {
  const base = import.meta.env.BASE_URL;
  const primero = path.replace(/^\/+/, '').split('/')[0] ?? '';
  const laOrg =
    org !== undefined ? org : SIN_ORG_SIEMPRE.includes(primero) ? null : orgActual(base);
  return rutaConOrg(base, laOrg, path);
}
