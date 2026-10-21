/// <reference types="astro/client" />

/**
 * Variables de entorno de SyERP. Secretos desde Scaleway Secret Manager /
 * Sealed Secrets en prod, .env en dev (nunca en git). docs/CONVENTIONS.md §8, §10.
 */
interface ImportMetaEnv {
  readonly SYTUE_OIDC_ISSUER: string;
  readonly SYTUE_OIDC_CLIENT_ID: string;
  readonly SYTUE_OIDC_CLIENT_SECRET: string;
  readonly SYTUE_OIDC_REDIRECT_URI: string;
  readonly SYTUE_OIDC_POST_LOGOUT_URI?: string;
  readonly SYTUE_OIDC_SCOPES?: string;
  readonly SYTUE_SESSION_SECRET: string;
  /** URL INTERNA del servicio Go (Service del clúster); la pública es respaldo. */
  readonly SYTUE_SYERP_INTERNAL_URL?: string;
  readonly SYTUE_SYERP_BASE_URL?: string;
  readonly PUBLIC_SYTUE_SYERP_BASE_URL?: string;
  readonly SYTUE_HUB_BASE_URL?: string;
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
