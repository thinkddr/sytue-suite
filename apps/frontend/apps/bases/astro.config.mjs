// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Sytue Bases — Astro 7 + Preact 10 (islands), SSR para el BFF.
 * Tailwind v4 vía plugin de Vite; tokens de marca por @sytue/ui.
 * Puerto dev 4337 (docs/CONVENTIONS.md §6; ADR 0068).
 */
export default defineConfig({
  output: 'server',
  base: '/bases',
  security: sytueSecurity,
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4337, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth'] },
  },
});
