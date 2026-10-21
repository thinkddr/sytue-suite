// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Sytue Drive — cliente propio sobre el servicio Go de Sytue Drive (drive.sytue.eu).
 * SSR para el BFF de sesion. Islands Preact + Tailwind v4. Puerto dev 4326 (CONVENTIONS §6).
 */
export default defineConfig({
  site: 'https://drive.sytue.eu',
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es
  // la de la ORGANIZACIÓN, https://<slug>.sytue.eu/drive. Su subdominio propio
  // sirve el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  //
  // Se hornea al construir (Astro no lo lee en caliente), y decide la URL de
  // los assets del HTML: por eso una imagen sirve UN prefijo y no la raíz.
  base: '/drive',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en
  // localhost: checkOrigin rechaza entonces todo POST con 403 (ver
  // @sytue/config/astro-security).
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4326, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: {
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
