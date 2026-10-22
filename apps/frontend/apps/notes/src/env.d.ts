/// <reference types="astro/client" />

/**
 * Variables de entorno de Sytue Notepads. Secretos desde Scaleway Secret Manager /
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
  readonly SYTUE_NOTES_BASE_URL?: string;
  readonly PUBLIC_SYTUE_NOTES_BASE_URL?: string;
  readonly SYTUE_HUB_BASE_URL?: string;
  /** Servicios que alimentan los selectores de incrustación (server-side). */
  readonly SYTUE_DRIVE_API_URL?: string;
  readonly PUBLIC_SYTUE_DRIVE_BASE_URL?: string;
  readonly SYTUE_DOCS_API_URL?: string;
  readonly PUBLIC_SYTUE_DOCS_BASE_URL?: string;
  readonly SYTUE_CALENDAR_API_URL?: string;
  readonly PUBLIC_SYTUE_CALENDAR_BASE_URL?: string;
  readonly SYTUE_ROOMS_API_URL?: string;
  readonly PUBLIC_SYTUE_ROOMS_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por petición.
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
    /** La organización del path (ADR 0056); ausente en URLs sin ella. */
    org?: string;
  }
}
