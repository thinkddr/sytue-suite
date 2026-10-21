import type { AstroIntegration } from 'astro';

/**
 * Ruta de salud uniforme para todas las imágenes web de la suite. Astro la
 * inyecta sin pasar por una página de producto: no abre sesión, no consulta
 * OIDC ni expone estado interno. Así las sondas de Kubernetes miden el proceso
 * web y no el recorrido completo de login de una persona.
 */
export function sytueHealthEndpoint(): AstroIntegration {
  return {
    name: 'sytue-health-endpoint',
    hooks: {
      'astro:config:setup': ({ config, injectRoute }) => {
        injectRoute({
          pattern: '/healthz',
          entrypoint: new URL('./health-endpoint.ts', import.meta.url),
          // `www` no tiene runtime SSR y necesita un fichero estático. En las
          // demás apps la ruta vive en el servidor, para que el build no simule
          // headers/cookies al atravesar su middleware de idioma.
          prerender: config.output === 'static',
        });
      },
    },
  };
}
