/**
 * Rutas de ESTA app, con su prefijo.
 *
 * Sytue Docs se construye con `base: '/docs'`, así que vive en
 * `docs.sytue.eu/docs` y en `<organizacion>.sytue.eu/docs`. Astro prefija sus
 * assets y sus páginas, pero no un `location.href = '/edit'` escrito a mano: eso
 * pide la raíz del host, donde el ingress no enruta nada, y el navegador recibe
 * el 404 de Traefik. Así estaba ABRIR UN DOCUMENTO, que es la acción principal
 * de la aplicación.
 *
 * Con la organización en el path (ADR 0056), en el NAVEGADOR `r()` la conserva:
 * leyendo la org de la URL actual (`orgActual`, null en SSR) la navegación
 * hidratada mantiene el segmento y la barra sigue enseñando la org en la que se
 * está, sin el 302 que de otro modo añade el middleware. En el SERVIDOR no hay
 * URL de navegador: los destinos salen sin org y es ese middleware quien los
 * manda a la canónica con un salto local.
 *
 * SALVO QUE QUIEN LLAMA LA CONOCE. Un endpoint SSR (open-from-drive, el canje de
 * `/s/<token>`) tiene la org en la sesión o en `locals.org`, y la pasa explícita:
 * sin ella, una ruta que COLISIONA con el namespace de organizaciones —`edit`,
 * declarada en `sinOrg`— se sirve siempre sin org y la isla hidrata creyendo que
 * `edit` ES la organización («copiar enlace» produce `/docs/edit/edit?id=…`,
 * 404). El parámetro manda sobre `orgActual` y no cambia nada para quien no lo
 * pasa: los componentes de navegador siguen igual.
 *
 * LAS EXCEPCIONES VAN aparte: un enlace pensado para anónimos —como el público
 * `/s/<token>` de ShareModal— se compone SIN org, con `withBase` a secas, porque
 * quien lo abre no tiene por qué pertenecer a ninguna.
 *
 * `import.meta.env.BASE_URL` lo sustituye Vite al construir ESTA app, de modo
 * que en dev (sin base) sigue siendo `/edit` y no hay dos comportamientos.
 */
import { orgActual, rutaConOrg } from '@sytue/config';
import { withBase } from '@sytue/ui';

export function r(path: string, org?: string | null): string {
  return withBase(
    import.meta.env.BASE_URL,
    rutaConOrg('', org ?? orgActual(import.meta.env.BASE_URL), path),
  );
}
