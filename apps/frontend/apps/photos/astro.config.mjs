// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Sytue Photos — Astro 7 + Preact 10 (islands), SSR para el BFF.
 * Tailwind v4 vía plugin de Vite; tokens de marca por @sytue/ui.
 * Puerto dev 4332 (docs/CONVENTIONS.md §6).
 */
export default defineConfig({
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es la
  // de la ORGANIZACIÓN, https://<slug>.sytue.eu/photos. Su subdominio propio sirve
  // el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  base: '/photos',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en localhost:
  // checkOrigin rechaza entonces todo POST con 403.
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4332, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth', '@sytue/api-clients'] },
  },
});
