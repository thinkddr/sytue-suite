/**
 * Fabrica del cliente CalDAV (Sytue Calendar) del lado servidor, contra el
 * servicio Go propio Sytue Calendar (CalDAV RFC 4791) via el SDK
 * @sytue/api-clients/caldav. Autorizado con el access token de Sytue ID
 * (Bearer, validado por el servicio Go via JWKS). El userId CalDAV es el `sub`
 * del token (así indexa el backend sus colecciones). Clean-room. Soberania 100% UE.
 */
import { DOMParser } from '@xmldom/xmldom';
import {
  CalDavClient,
  bearer,
  setXmlParser,
  upstreamStatus,
  type XmlParser,
} from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';

// El runtime server (Bun) no expone DOMParser; se inyecta el de @xmldom/xmldom
// una sola vez para que el SDK CalDAV parsee las respuestas multistatus.
setXmlParser(new DOMParser() as unknown as XmlParser);

const env = serverEnv(import.meta.env);

/** Base del servicio Go Sytue Calendar (dev :8087, prod calendar.sytue.eu). */
// Por el Service INTERNO: el BFF corre en un pod y el nombre público no
// resuelve desde dentro (502 sin llegar a ser HTTP). La pública, de respaldo.
export const CALENDAR_BASE_URL =
  env.SYTUE_CALENDAR_INTERNAL_URL ?? env.SYTUE_CALENDAR_BASE_URL ?? 'https://calendar.sytue.eu';

export function calDavClient(accessToken: string, userId: string): CalDavClient {
  return new CalDavClient({
    baseUrl: CALENDAR_BASE_URL,
    userId,
    auth: bearer(accessToken),
  });
}

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/**
 * Una URL de recurso CalDAV del cuerpo o de la consulta solo puede ser una RUTA
 * del servicio: empieza por UNA barra. Un destino absoluto (`http://…`) o
 * protocolo‑relativo (`//host`) mandaría el Bearer de la sesión fuera —robo de
 * token y SSRF—, así que aquí se rechaza con un 400 limpio. El SDK lo bloquea
 * además en la raíz (`sameOriginUrl`); esto es la verja del BFF.
 */
export function rutaDav(url: unknown): string | null {
  return typeof url === 'string' && url.startsWith('/') && !url.startsWith('//') ? url : null;
}

/** Llamada REST al servicio Go con el Bearer de la sesion. */
export function rest(
  token: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<Response> {
  return fetch(`${CALENDAR_BASE_URL}/api/v1${path}`, {
    method,
    headers: {
      authorization: `Bearer ${token}`,
      accept: 'application/json',
      ...(body !== undefined ? { 'content-type': 'application/json' } : {}),
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
}

/** Reenvia tal cual una respuesta de error del servicio Go (status + cuerpo JSON). */
export async function forwardError(res: Response): Promise<Response> {
  return new Response(await res.text(), {
    status: res.status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/**
 * Respuesta por una excepción al hablar con el servicio (el SDK CalDAV lanza
 * `HttpError`): su 4xx tal cual —un 403 es «ya no tienes acceso», no una caída— y
 * 502 para lo demás. Sin el texto de la excepción: lleva la URL interna del Service.
 */
export function fallo(err: unknown, error: string): Response {
  return json({ error }, upstreamStatus(err));
}
