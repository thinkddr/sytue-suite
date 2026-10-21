/// <reference path="../.astro/types.d.ts" />
/// <reference types="astro/client" />

/**
 * Variables de entorno del BFF de Sytue Docs (client OIDC `sytue-docs`).
 * Secreto del client y clave de sesion: SOLO servidor (nunca al navegador).
 */
interface ImportMetaEnv {
  readonly SYTUE_OIDC_ISSUER: string;
  readonly SYTUE_OIDC_CLIENT_ID: string;
  readonly SYTUE_OIDC_CLIENT_SECRET: string;
  readonly SYTUE_OIDC_REDIRECT_URI: string;
  readonly SYTUE_OIDC_POST_LOGOUT_URI?: string;
  readonly SYTUE_OIDC_SCOPES?: string;
  readonly SYTUE_SESSION_SECRET: string;
  readonly SYTUE_HUB_BASE_URL?: string;
  readonly PUBLIC_SYTUE_HUB_BASE_URL?: string;
  /** URL del servicio Go de Sytue Docs (docs.sytue.eu, dev :8083). */
  readonly PUBLIC_SYTUE_DOCS_BASE_URL?: string;
  /** URL del Sytue Integration Gateway (integrations.sytue.eu, dev :8093). Solo servidor. */
  readonly SYTUE_INTEGRATIONS_BASE_URL?: string;
  readonly SYTUE_SECURE_COOKIES?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por petición, y
 * `Astro.locals.org` lo deja `middlewareDeOrganizacion` al reescribir una URL con
 * la organización en el path (ADR 0056). Se declaran aquí para que el layout y las
 * páginas los lean con tipo y no como strings sueltos (docs/CONVENTIONS.md §13.1).
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
    /** La organización del path (ADR 0056); ausente en URLs sin ella. */
    org?: string;
  }
}
