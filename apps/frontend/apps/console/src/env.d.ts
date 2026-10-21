/// <reference types="astro/client" />

/**
 * Variables de entorno (inyectadas por entorno; NUNCA literales en git —
 * Scaleway Secret Manager / Sealed Secrets). Client OIDC: `sytue-console`
 * (docs/CONVENTIONS.md §8). El acceso a la API de administración de Sytue ID se
 * hace vía Sytue Hub / control-plane con el token del usuario (rol admin/owner).
 */
interface ImportMetaEnv {
  readonly SYTUE_OIDC_ISSUER: string;
  readonly SYTUE_OIDC_CLIENT_ID: string;
  readonly SYTUE_OIDC_CLIENT_SECRET: string;
  readonly SYTUE_OIDC_REDIRECT_URI: string;
  readonly SYTUE_OIDC_POST_LOGOUT_URI?: string;
  readonly SYTUE_OIDC_SCOPES?: string;
  readonly SYTUE_SESSION_SECRET: string;
  /** URL del Hub (shell global + puente SCIM/admin). */
  readonly SYTUE_HUB_URL?: string;
  /** URL base de la API de administración de Sytue ID vía control-plane. */
  readonly SYTUE_ADMIN_API_URL?: string;
  /** URL base del Sytue Integration Gateway (catálogo + instancias de integraciones). */
  readonly SYTUE_INTEGRATIONS_BASE_URL?: string;
  /** URL base del servicio Go de Sytue Mail (compliance: legal hold + eDiscovery). */
  readonly SYTUE_MAIL_API_URL?: string;
  /** URL base del servicio Go de Sytue Drive (compliance: legal hold + eDiscovery). */
  readonly SYTUE_DRIVE_API_URL?: string;
  readonly SYTUE_COOKIES_SECURE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por petición
 * (cookie > Accept-Language > inglés), y `Astro.locals.org` lo deja
 * `middlewareDeOrganizacion` al reescribir una URL con la organización en el path
 * (ADR 0056). Se declaran aquí para que el layout, las páginas y las props de las
 * islas lo lean con tipo y no como string suelto (docs/CONVENTIONS.md §13.1).
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
    /** La organización del path (ADR 0056); ausente en URLs sin ella. */
    org?: string;
  }
}
