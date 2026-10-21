/// <reference types="astro/client" />

/**
 * Variables de entorno de Sytue Cast. Secretos desde el gestor de secretos en
 * prod, .env en dev (nunca en git). docs/CONVENTIONS.md §8, §10; CAST-ENCARGO §13.3.
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
  /** Servicio Go de Cast por la red interna (respaldo http://cast:8099). */
  readonly SYTUE_CAST_API_URL?: string;
  /** Raíz pública del WebSocket del chat (wss://cast.<dominio>). */
  readonly SYTUE_CAST_WS_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por petición, y
 * `Astro.locals.org` lo deja `middlewareDeOrganizacion` al reescribir una URL con
 * la organización en el path (ADR 0056).
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
    /** La organización del path (ADR 0056); ausente en URLs sin ella. */
    org?: string;
  }
}
