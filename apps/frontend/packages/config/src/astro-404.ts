import type { AstroIntegration } from 'astro';

/**
 * Página 404 de la suite para las superficies SSR, inyectada como `/healthz`.
 *
 * Sin ella Astro sirve la suya —fondo casi negro, su cohete y «404: Not found»
 * en inglés—, y eso es lo que se veía en las trece apps: medido contra
 * producción el 2026-09-21, 15 de 15 pantallas en la pasada de ruta inexistente
 * de `test/e2e/medidas-web.mjs`.
 *
 * Una integración y no un `404.astro` en cada app por lo mismo que el middleware
 * de idioma: trece copias que se parecen no son trece páginas iguales. La página
 * vive en `@sytue/ui` porque pinta sus componentes y su hoja; aquí solo se
 * declara la ruta, con un especificador de paquete que resuelve cada app (todas
 * dependen de `@sytue/ui`), así que este paquete no la importa.
 *
 * `www` no la lleva: es estática y ya tiene su propio `404.astro`.
 */
export function sytueNoEncontrada(): AstroIntegration {
  return {
    name: 'sytue-no-encontrada',
    hooks: {
      'astro:config:setup': ({ injectRoute }) => {
        injectRoute({
          pattern: '/404',
          entrypoint: '@sytue/ui/pagina-404.astro',
          // En el servidor, para que el idioma salga de la petición y no del build.
          prerender: false,
        });
      },
    },
  };
}
