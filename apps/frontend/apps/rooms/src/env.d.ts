/// <reference types="astro/client" />

/**
 * Variables de entorno de Sytue Chat. Secretos desde Scaleway Secret Manager /
 * Sealed Secrets en prod, .env en dev (nunca en git). docs/CONVENTIONS.md §8, §10.
 */
interface ImportMetaEnv {
  // OIDC (client confidencial `sytue-chat` del realm `sytue`).
  readonly SYTUE_OIDC_ISSUER: string;
  readonly SYTUE_OIDC_CLIENT_ID: string;
  readonly SYTUE_OIDC_CLIENT_SECRET: string;
  readonly SYTUE_OIDC_REDIRECT_URI: string;
  readonly SYTUE_OIDC_POST_LOGOUT_URI?: string;
  readonly SYTUE_OIDC_SCOPES?: string;
  readonly SYTUE_SESSION_SECRET: string;
  // Backend: servicio Go propio Sytue Rooms (mensajería, salas y señalización). Los
  // nombres SYTUE_CHAT_*/SYTUE_MEET_* son los anteriores a la fusión y se siguen
  // leyendo como respaldo para no romper un despliegue a medio migrar.
  readonly SYTUE_ROOMS_API_URL?: string;
  readonly SYTUE_ROOMS_WS_URL?: string;
  readonly SYTUE_CHAT_API_URL?: string;
  readonly SYTUE_CHAT_WS_URL?: string;
  readonly SYTUE_HUB_BASE_URL?: string;
  // Sytue Drive: la API REST existente donde viven los adjuntos (subida/descarga
  // vía los proxies BFF /api/upload y /api/attachment; solo servidor, como
  // SYTUE_ROOMS_API_URL). drive.sytue.eu; dev :8082.
  readonly SYTUE_DRIVE_API_URL?: string;
  // Videollamada (embebida en el canal) + URL del SFU LiveKit. La API es la MISMA
  // que la de mensajería desde la fusión: SYTUE_MEET_API_URL solo es el respaldo.
  readonly SYTUE_MEET_API_URL?: string;
  readonly SYTUE_LIVEKIT_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/**
 * `Astro.locals.locale` lo resuelve `src/middleware.ts` una vez por petición. Se
 * declara aquí para que el layout y las páginas lo lean con tipo y no como string
 * suelto (docs/CONVENTIONS.md §13.1). `org` la deja el middleware de la
 * organización en el path (ADR 0056): es la org que trae la URL, que NO tiene
 * por qué ser la de la sesión — un externo con enlace de otra organización
 * aterriza donde está.
 */
declare namespace App {
  interface Locals {
    locale: import('@sytue/i18n').Locale;
    org?: string;
  }
}
