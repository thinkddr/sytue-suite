/// <reference path="../.astro/types.d.ts" />
/// <reference types="astro/client" />

/**
 * Variables de entorno del BFF de Mi cuenta (client OIDC `sytue-account`).
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
  /** URL del servicio Go de Sytue ID (id.sytue.eu, dev :8080). */
  readonly PUBLIC_SYTUE_ID_BASE_URL?: string;
  readonly SYTUE_SECURE_COOKIES?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por peticion. Se
 * declara aqui para que el layout y las paginas lo lean con tipo y no como string
 * suelto (docs/CONVENTIONS.md §13.1).
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
  }
}
