// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytuePrecarga } from '@sytue/config/astro-precarga';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueArranque } from '@sytue/config/astro-arranque';
import { sytueCsp, sytueSecurity } from '@sytue/config/astro-security';

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
  // `csp`: script-src SIN 'unsafe-inline' (sytueCsp, astro-security.ts): Astro firma
  // sus scripts y los de arranque llegan de /arranque.js (sytueArranque).
  security: { ...sytueSecurity, csp: sytueCsp() },
  adapter: node({ mode: 'standalone' }),
  integrations: [
    sytuePrecarga(),
    sytueHealthEndpoint(),
    sytueNoEncontrada(),
    sytueArranque(),
    preact({ compat: true }),
  ],
  server: { port: 4332, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth', '@sytue/api-clients'] },
  },
});
