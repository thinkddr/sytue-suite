/// <reference types="astro/client" />

/**
 * Variables de entorno del sitio público (SSG). Todas `PUBLIC_*`: se inlinean en
 * el bundle que sirve el navegador, así que aquí NO va nada secreto
 * (docs/CONVENTIONS.md §8). Sin este tipado, `import.meta.env.X` es `any` y el
 * lint lo rechaza (@typescript-eslint/no-unsafe-assignment).
 */
interface ImportMetaEnv {
  /** Base de los assets de descarga; por defecto, la última release de GitHub. */
  readonly PUBLIC_SYTUE_DOWNLOADS_BASE?: string;
  /** Ficha de App Store; vacía mientras no exista (la tarjeta sale «próximamente»). */
  readonly PUBLIC_SYTUE_STORE_IOS?: string;
  /** Ficha de Google Play; ídem. */
  readonly PUBLIC_SYTUE_STORE_ANDROID?: string;
  // Sin APK directo: con Play App Signing no se podría actualizar desde Play
  // (docs/operations.md, «Android: Play App Signing»).
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
