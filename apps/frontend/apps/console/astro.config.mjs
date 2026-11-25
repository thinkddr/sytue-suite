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
 * Sytue Admin — admin de tenants/usuarios/grupos. SSR (output server): el BFF
 * OIDC, el guard de roles (admin/owner) y las llamadas a la API de administracion
 * de Sytue ID / Sytue Hub se ejecutan en el servidor. Preact 10 para las islas.
 * Tailwind v4. Soberania 100% UE. docs/CONVENTIONS.md §4, §6, §8.
 */
export default defineConfig({
  site: 'https://admin.sytue.eu',
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es
  // la de la ORGANIZACIÓN, https://<slug>.sytue.eu/admin. Su subdominio propio
  // sirve el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  //
  // Se hornea al construir (Astro no lo lee en caliente), y decide la URL de
  // los assets del HTML: por eso una imagen sirve UN prefijo y no la raíz.
  base: '/admin',
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
  server: { port: 4322, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/api-clients'] },
  },
});
