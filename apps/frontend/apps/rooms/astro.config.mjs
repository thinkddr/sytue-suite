// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Sytue Chat — Astro 7 + Preact 10 (islands). SSR (output: 'server') porque el
 * BFF de autenticacion (OIDC+PKCE contra Sytue ID) y los proxies REST al servicio
 * Go Sytue Chat son endpoints de servidor. El WebSocket de tiempo real se abre
 * desde el navegador contra el servicio Go usando un ticket de corta duracion
 * emitido por el BFF (el access token nunca se persiste en el cliente). Tailwind
 * v4 via plugin de Vite; tokens de marca por `@sytue/ui/styles.css`. Puerto dev
 * 4330 (docs/CONVENTIONS.md §6).
 */
export default defineConfig({
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: la dirección canónica es
  // la de la ORGANIZACIÓN, https://<slug>.sytue.eu/rooms. Su subdominio propio
  // sirve el MISMO prefijo y redirige desde la raíz — las dos son válidas.
  //
  // Se hornea al construir (Astro no lo lee en caliente), y decide la URL de
  // los assets del HTML: por eso una imagen sirve UN prefijo y no la raíz.
  base: '/rooms',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en
  // localhost: checkOrigin rechaza entonces todo POST con 403 (ver
  // @sytue/config/astro-security).
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4330, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth', '@sytue/api-clients'] },
  },
});
