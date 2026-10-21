// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Sytue Cast — Astro 7 + Preact 10 (islands), SSR para el BFF.
 * Tailwind v4 vía plugin de Vite; tokens de marca por @sytue/ui.
 * Puerto dev 4335 (docs/CONVENTIONS.md §6; CAST-ENCARGO §2).
 */
export default defineConfig({
  output: 'server',
  // Toda la app vive bajo el prefijo de su producto: cast.<dominio>/cast y
  // <org>.<dominio>/cast sirven el MISMO build (ADR 0056).
  base: '/cast',
  // Sin esto, Astro descarta el Host real y `Astro.url` acaba en localhost:
  // checkOrigin rechaza entonces todo POST con 403.
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4335, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth'] },
  },
});
