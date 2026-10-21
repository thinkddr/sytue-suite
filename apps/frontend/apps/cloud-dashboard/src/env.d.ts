/// <reference types="astro/client" />

/**
 * Variables de entorno de la app (inyectadas por entorno; NUNCA literales en
 * git — Scaleway Secret Manager / Sealed Secrets). Client OIDC: `sytue-cloud`
 * (docs/CONVENTIONS.md §8).
 */
interface ImportMetaEnv {
  readonly SYTUE_OIDC_ISSUER: string;
  readonly SYTUE_OIDC_CLIENT_ID: string;
  readonly SYTUE_OIDC_CLIENT_SECRET: string;
  readonly SYTUE_OIDC_REDIRECT_URI: string;
  readonly SYTUE_OIDC_POST_LOGOUT_URI?: string;
  readonly SYTUE_OIDC_SCOPES?: string;
  readonly SYTUE_SESSION_SECRET: string;
  /** URL del Hub (busqueda/notificaciones/presencia globales). */
  readonly SYTUE_HUB_URL?: string;
  /** URL base de la API del control-plane del PaaS (Sytue DevX API). */
  readonly SYTUE_CLOUD_API_URL?: string;
  /**
   * URL base de la API de lectura de Sytue Analytics (:8095). Vacía = la sección
   * de analítica no aparece; es el interruptor de la función, no un descuido.
   */
  readonly SYTUE_ANALYTICS_API_URL?: string;
  /** Secure=false solo en dev local sobre http. */
  readonly SYTUE_COOKIES_SECURE?: string;
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
