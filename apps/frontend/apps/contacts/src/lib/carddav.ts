/**
 * Fabrica del cliente CardDAV (Sytue Contacts) del lado servidor, contra el
 * servicio Go propio Sytue Contacts (CardDAV RFC 6352) via el SDK
 * @sytue/api-clients/carddav. Autorizado con el access token de Sytue ID
 * (Bearer, validado por el servicio Go via JWKS). El userId CardDAV es el `sub`
 * del token (así indexa el backend sus colecciones). Clean-room. Soberania 100% UE.
 */
import { DOMParser } from '@xmldom/xmldom';
import {
  CardDavClient,
  ContactsClient,
  bearer,
  setXmlParser,
  type XmlParser,
} from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';

// El runtime server (Bun) no expone DOMParser; se inyecta el de @xmldom/xmldom
// una sola vez para que el SDK CardDAV parsee las respuestas multistatus.
setXmlParser(new DOMParser() as unknown as XmlParser);

const env = serverEnv(import.meta.env);

/** Base del servicio Go Sytue Contacts (dev :8088, prod contacts.sytue.eu). */
// Por el Service INTERNO, igual que Calendar: desde un pod el nombre público
// no resuelve y la llamada muere antes de ser HTTP.
export const CONTACTS_BASE_URL =
  env.SYTUE_CONTACTS_INTERNAL_URL ?? env.SYTUE_CONTACTS_BASE_URL ?? 'https://contacts.sytue.eu';

export function cardDavClient(accessToken: string, userId: string): CardDavClient {
  return new CardDavClient({
    baseUrl: CONTACTS_BASE_URL,
    userId,
    auth: bearer(accessToken),
  });
}

/**
 * Cliente REST (`/api/v1`), aparte del CardDAV de arriba: crear/renombrar/
 * borrar una libreta son rutas propias del servicio, no verbos WebDAV
 * (MKCOL/PROPPATCH) que este BFF supiera hablar hasta ahora.
 */
export function contactsRestClient(accessToken: string): ContactsClient {
  return new ContactsClient({ baseUrl: CONTACTS_BASE_URL, auth: bearer(accessToken) });
}

/**
 * Una URL de recurso CardDAV del cuerpo o de la consulta solo puede ser una RUTA
 * del servicio: empieza por UNA barra. Un destino absoluto (`http://…`) o
 * protocolo‑relativo (`//host`) mandaría el Bearer de la sesión fuera —robo de
 * token y SSRF—, así que aquí se rechaza. El SDK lo bloquea además en la raíz
 * (`sameOriginUrl`); esto es la verja del BFF.
 */
export function rutaDav(url: unknown): string | null {
  return typeof url === 'string' && url.startsWith('/') && !url.startsWith('//') ? url : null;
}
