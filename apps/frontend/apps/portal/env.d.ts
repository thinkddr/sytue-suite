/// <reference path="../.astro/types.d.ts" />
/// <reference types="astro/client" />

/**
 * Variables de entorno del BFF (Sytue One / portal). El secreto del client y la
 * clave de sesion NUNCA llegan al navegador: solo se leen en endpoints de
 * servidor (import.meta.env / process.env). Ver @sytue/auth/config.
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
  readonly SYTUE_INTEGRATIONS_BASE_URL?: string;
  readonly SYTUE_SECURE_COOKIES?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por peticion. Se
 * declara aqui para que el layout y las paginas lo lean con tipo y no como string
 * suelto (docs/CONVENTIONS.md §13.1). `org` la deja el middleware de la
 * organizacion en el path (ADR 0056): la org que trae la URL, que NO tiene por
 * que ser la de la sesion.
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
    org?: string;
  }
}
