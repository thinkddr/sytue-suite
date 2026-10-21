// @ts-check
import { defineConfig } from 'astro/config';
import preact from '@astrojs/preact';
import node from '@astrojs/node';
import tailwindcss from '@tailwindcss/vite';
import { sytueHealthEndpoint } from '@sytue/config/astro-health';
import { sytueNoEncontrada } from '@sytue/config/astro-404';
import { sytueSecurity } from '@sytue/config/astro-security';

/**
 * Mi cuenta — Astro 7 + Preact 10 (islands), SSR para el BFF.
 *
 * POR QUÉ ES UNA APP Y NO UNA PÁGINA MÁS DE SYTUE ID. La cuenta se servía como
 * HTML desde Go, sin bundler, así que no podía montar la Sytue Shell: el usuario
 * llegaba a una pantalla sin el selector de aplicaciones y sin salida hacia el
 * resto de la suite. Replicar el selector en Go significaba duplicar el catálogo
 * —nombres, iconos, colores de catorce productos— en un segundo lenguaje, que es
 * exactamente el tipo de duplicado que diverge a la primera app nueva.
 *
 * Los flujos de Sytue ID que NO son la cuenta —login, OIDC, SAML— siguen en Go:
 * son pantallas sin navegación por diseño y no ganan nada con esto.
 */
export default defineConfig({
  output: 'server',
  // La dirección es GLOBAL, https://id.<dominio>/account: la cuenta es de la
  // persona, no de una organización. `account.<dominio>/account` sirve el mismo
  // prefijo y `<slug>.<dominio>/account` solo redirige a la global.
  base: '/account',
  // `allowedDomains`: sin él, Astro descarta el Host real y `Astro.url` acaba en
  // localhost, y checkOrigin rechaza entonces todo POST con 403.
  //
  // `csp`: esta app comparte origen con Sytue ID y es la única con `script-src`
  // estricto (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp).
  // Los estilos quedan con 'unsafe-inline': Tailwind y los `style` de Preact los
  // necesitan, y un estilo inyectado no ejecuta nada.
  security: {
    ...sytueSecurity,
    csp: { styleDirective: { resources: ["'self'", "'unsafe-inline'"] } },
  },
  adapter: node({ mode: 'standalone' }),
  integrations: [sytueHealthEndpoint(), sytueNoEncontrada(), preact({ compat: true })],
  server: { port: 4333, host: true },
  vite: {
    plugins: [tailwindcss()],
    ssr: { noExternal: ['@sytue/ui', '@sytue/shell', '@sytue/auth', '@sytue/api-clients'] },
  },
});
