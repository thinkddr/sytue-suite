// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Sytue One — portal/lanzador (one.sytue.eu).
 *
 * SSR (output server) porque el BFF de @sytue/auth emite la cookie de sesion
 * httpOnly de la suite (.sytue.eu) y la shell se hidrata desde /auth/session.
 * Islands Preact + Tailwind v4 (tokens de @sytue/ui). Puerto dev 4321 (SPEC §6).
 */
export default defineConfig({
  site: 'https://one.sytue.eu',
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es
  // la de la ORGANIZACIÓN, https://<slug>.sytue.eu/one. Su subdominio propio
  // sirve el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  //
  // Se hornea al construir (Astro no lo lee en caliente), y decide la URL de
  // los assets del HTML: por eso una imagen sirve UN prefijo y no la raíz.
  base: '/one',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en
  // localhost: checkOrigin rechaza entonces todo POST con 403 (ver
  // @sytue/config/astro-security).
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4321, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: {
      // Los paquetes del workspace se transpilan (TS/JSX) en SSR.
      noExternal: [
        '@sytue/ui',
        '@sytue/shell',
        '@sytue/auth',
        '@sytue/api-clients',
        '@sytue/config',
        '@sytue/i18n',
      ],
    },
  },
});
