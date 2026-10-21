/**
 * Sytue One — preset/helper de Tailwind v4 para apps Astro.
 *
 * Tailwind v4 es CSS-first: la configuracion de tema vive en `theme.css`
 * (importado via `@import "@sytue/config/theme.css"`). Este modulo expone la
 * ruta del CSS de tema y los globs de contenido recomendados, para que cada app
 * los registre de forma uniforme (sin duplicar configuracion de marca).
 */
import { fileURLToPath } from 'node:url';

/** Ruta absoluta al CSS de tema Sytue (tokens + @theme Tailwind v4). */
export const themeCssPath: string = fileURLToPath(new URL('./theme.css', import.meta.url));

/**
 * Globs de contenido recomendados para que Tailwind v4 detecte clases en
 * paquetes compartidos y en la propia app. Anadir a la deteccion de la app.
 */
export const sytueContentGlobs: readonly string[] = [
  './src/**/*.{astro,html,ts,tsx,js,jsx,md,mdx}',
  '../../packages/ui/src/**/*.{ts,tsx}',
  '../../packages/shell/src/**/*.{ts,tsx}',
];
