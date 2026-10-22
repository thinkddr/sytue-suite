/// <reference types="astro/client" />

/**
 * Variables de entorno de Sytue Mail. Secretos resueltos desde Scaleway Secret
 * Manager / Sealed Secrets en prod y .env en dev (NUNCA literales en codigo ni
 * en git). docs/CONVENTIONS.md §8, §10.
 */
interface ImportMetaEnv {
  // OIDC (client confidencial `sytue-mail` del realm `sytue`).
  readonly SYTUE_OIDC_ISSUER: string;
  readonly SYTUE_OIDC_CLIENT_ID: string;
  readonly SYTUE_OIDC_CLIENT_SECRET: string;
  readonly SYTUE_OIDC_REDIRECT_URI: string;
  readonly SYTUE_OIDC_POST_LOGOUT_URI?: string;
  readonly SYTUE_OIDC_SCOPES?: string;
  readonly SYTUE_SESSION_SECRET: string;
  // Backends.
  readonly SYTUE_JMAP_SESSION_URL?: string;
  readonly SYTUE_JMAP_IDENTITY_ID?: string;
  readonly SYTUE_HUB_BASE_URL?: string;
  // Sytue Contacts (autocompletado de direcciones en el compositor).
  readonly SYTUE_CONTACTS_BASE_URL?: string;
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
