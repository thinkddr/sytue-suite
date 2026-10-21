/**
 * @sytue/ui/assets — logos de marca Sytue.
 *
 * Los SVG viven en branding/logo/ (fuente de verdad). Aqui se importan como
 * URL (bundlers Astro/Vite resuelven `?url`) para que las apps los referencien
 * sin redibujarlos. No se rediseñan logos por app (regla del design system).
 */
import wordmark from './logos/sytue.svg?url';
import suite from './logos/sytue-one.svg?url';
import suiteWhite from './logos/sytue-one-white.svg?url';
import mark from './logos/sytue-mark.svg?url';
import favicon from './logos/favicon.svg?url';

export const logos = {
  /** Wordmark "Sytue". */
  wordmark,
  /** Marca de la suite "Sytue One" (sobre fondo claro). */
  suite,
  /** Marca de la suite en blanco (sobre fondos oscuros / sidebar). */
  suiteWhite,
  /** Isotipo / S-swoosh. */
  mark,
  /** Favicon. */
  favicon,
} as const;

export type LogoKey = keyof typeof logos;
