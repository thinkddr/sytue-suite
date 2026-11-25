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
 * Sytue Calendar — Astro 7 + Preact 10 (islands), SSR para el BFF y el proxy
 * CalDAV. Tailwind v4 via plugin de Vite; tokens de marca por @sytue/ui.
 * Puerto dev 4327 (docs/CONVENTIONS.md §6).
 */
export default defineConfig({
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es
  // la de la ORGANIZACIÓN, https://<slug>.sytue.eu/calendar. Su subdominio propio
  // sirve el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  //
  // Se hornea al construir (Astro no lo lee en caliente), y decide la URL de
  // los assets del HTML: por eso una imagen sirve UN prefijo y no la raíz.
  base: '/calendar',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en
  // localhost: checkOrigin rechaza entonces todo POST con 403 (ver
  // @sytue/config/astro-security).
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
  server: { port: 4327, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth', '@sytue/api-clients'] },
  },
});
