/**
 * Fabrica del cliente JMAP (Sytue Mail) del lado servidor. Usa el SDK
 * @sytue/api-clients/jmap contra el servicio Go propio Sytue Mail (mail.sytue.eu,
 * dev :8086), autorizado con el access token de Sytue ID (Bearer). El endpoint de
 * sesion JMAP por defecto es mail.sytue.eu/.well-known/jmap (override por entorno
 * para dev local). Soberania 100% UE (Scaleway). docs/CONVENTIONS.md §1.
 */
import { JmapClient, bearer } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';

const env = serverEnv(import.meta.env);

/** URL del endpoint de sesion JMAP del servicio Go Sytue Mail. */
export const JMAP_SESSION_URL =
  env.SYTUE_JMAP_SESSION_URL ?? 'https://mail.sytue.eu/.well-known/jmap';

/** Identidad de envio (EmailSubmission). Configurable por entorno/tenant. */
export const JMAP_IDENTITY_ID = env.SYTUE_JMAP_IDENTITY_ID ?? '';

/** Crea un JmapClient autorizado con el access token de la sesion. */
export function jmapClient(accessToken: string): JmapClient {
  return new JmapClient({
    sessionUrl: JMAP_SESSION_URL,
    auth: bearer(accessToken),
  });
}
