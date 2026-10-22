/**
 * Rutas de ESTA app, con su prefijo y, en el navegador, con su organización.
 *
 * Sytue Admin se construye con `base: '/admin'`, así que vive en
 * `admin.sytue.eu/admin` y en `<organizacion>.sytue.eu/admin`. Astro prefija sus
 * assets y sus páginas, pero no un `href="/users"` escrito a mano: eso pide la
 * raíz del host, donde el ingress no enruta nada, y el navegador recibe el 404
 * de Traefik. Cada enlace de navegación de la consola estaba así.
 *
 * La organización como segmento de path (ADR 0056): la canónica es
 * `admin.sytue.eu/admin/<org>/users`, así que `r()` la añade leyendo la URL
 * ACTUAL (`orgActual`) — la navegación hidratada conserva el segmento y la barra
 * sigue enseñando la org en la que se está. En SSR no hay URL de navegador y los
 * enlaces salen sin org: el middleware de la app los manda a la canónica con un
 * 302 local. La CONSOLA DE PLATAFORMA nunca los compone con org: sus pantallas
 * viven en la raíz de su host, donde `orgActual` no encuentra prefijo que mirar.
 *
 * Los prefijos de plomería (`/api`, `/auth`…) NO llevan org nunca: no la usan y
 * el middleware los deja pasar siempre (SIN_ORG_SIEMPRE).
 *
 * `import.meta.env.BASE_URL` lo sustituye Vite al construir ESTA app, de modo
 * que en dev (sin base) sigue siendo `/users` y no hay dos comportamientos.
 */
import { orgActual, rutaConOrg, SIN_ORG_SIEMPRE } from '@sytue/config';

export function r(path: string, org?: string | null): string {
  const base = import.meta.env.BASE_URL;
  const primero = path.replace(/^\/+/, '').split('/')[0] ?? '';
  const laOrg =
    org !== undefined ? org : SIN_ORG_SIEMPRE.includes(primero) ? null : orgActual(base);
  return rutaConOrg(base, laOrg, path);
}
