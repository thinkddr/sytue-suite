/**
 * Fábrica del cliente Hub (Sytue Hub) del lado servidor. Usa el SDK
 * @sytue/api-clients contra el servicio Go propio Sytue Hub (hub.sytue.eu,
 * dev :8092): búsqueda federada, notificaciones, presencia e intents cross-app.
 * Autorizado con el access token de Sytue ID (Bearer) — que NUNCA sale al
 * navegador: el portal proxya vía BFF (src/pages/api/*). docs/CONVENTIONS.md §11.
 */
import { HubClient, bearer } from '@sytue/api-clients';
import { hubInternalUrl } from './auth';

/** Crea un HubClient autorizado con el access token de la sesión (solo servidor). */
export function hubClient(accessToken: string): HubClient {
  return new HubClient({ baseUrl: hubInternalUrl(), auth: bearer(accessToken) });
}
