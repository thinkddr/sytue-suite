/**
 * Puente REST del BFF hacia los endpoints `/api/v1` del servicio Go Sytue Mail
 * que el SDK JMAP no cubre (paginación KEYSET de buzón, envío con CCO). El SDK
 * (@sytue/api-clients, paquete compartido) es de solo lectura para esta app, así
 * que aquí se hace la llamada cruda autorizada con el access token de sesión
 * (Bearer, igual que el SDK) y se normaliza el `domain.Message` del Go al
 * `EmailSummary` del SDK — misma forma que consume el resto del cliente.
 */
import { JMAP_SESSION_URL } from './jmap.js';

/** Origen del servicio Go Sytue Mail (sin el path del well-known JMAP). */
export function mailOrigin(): string {
  return new URL(JMAP_SESSION_URL).origin;
}

/** GET autorizado (Bearer) a un endpoint REST del servicio Go Sytue Mail. */
export async function mailGet(accessToken: string, path: string): Promise<Response> {
  return fetch(`${mailOrigin()}${path}`, {
    headers: { authorization: `Bearer ${accessToken}`, accept: 'application/json' },
  });
}

/** POST JSON autorizado (Bearer) a un endpoint REST del servicio Go Sytue Mail. */
export async function mailPost(
  accessToken: string,
  path: string,
  body: unknown,
): Promise<Response> {
  return fetch(`${mailOrigin()}${path}`, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${accessToken}`,
      'content-type': 'application/json',
      accept: 'application/json',
    },
    body: JSON.stringify(body),
  });
}

/** PUT JSON autorizado (Bearer) a un endpoint REST del servicio Go Sytue Mail. */
export async function mailPut(accessToken: string, path: string, body: unknown): Promise<Response> {
  return fetch(`${mailOrigin()}${path}`, {
    method: 'PUT',
    headers: {
      authorization: `Bearer ${accessToken}`,
      'content-type': 'application/json',
      accept: 'application/json',
    },
    body: JSON.stringify(body),
  });
}

/**
 * PUT de TEXTO (no JSON) autorizado (Bearer) a un endpoint REST del servicio Go
 * Sytue Mail. Existe aparte de `mailPost` porque el material S/MIME (certificado
 * + clave) es un PEM tal cual, no un objeto — envolverlo en JSON obligaría a
 * escapar los saltos de línea de un fichero que la gente pega o sube sin tocar.
 */
export async function mailPutText(
  accessToken: string,
  path: string,
  body: string,
): Promise<Response> {
  return fetch(`${mailOrigin()}${path}`, {
    method: 'PUT',
    headers: {
      authorization: `Bearer ${accessToken}`,
      'content-type': 'application/x-pem-file',
      accept: 'application/json',
    },
    body,
  });
}

/** DELETE autorizado (Bearer) a un endpoint REST del servicio Go Sytue Mail. */
export async function mailDelete(accessToken: string, path: string): Promise<Response> {
  return fetch(`${mailOrigin()}${path}`, {
    method: 'DELETE',
    headers: { authorization: `Bearer ${accessToken}`, accept: 'application/json' },
  });
}

/** Tope del nombre visible: los sistemas de ficheros cortan sobre los 255. */
const MAX_NOMBRE = 80;

/**
 * Nombre de fichero a partir del asunto, para la descarga del mensaje original.
 *
 * El asunto lo escribe QUIEN ENVÍA y acaba dentro de una cabecera
 * `Content-Disposition`, así que sale de aquí sin comillas, sin barras y sin
 * caracteres de control: lo que rompería la respuesta o escribiría fuera de la
 * carpeta de descargas. Los acentos se conservan —viajan en `filename*` como
 * UTF-8— porque recortarlos deja nombres ilegibles en media Europa.
 */
export function nombreArchivo(asunto: string | null, id: string): string {
  const limpio = (asunto ?? '')
    .replace(/[\p{C}"\\/:*?<>|]/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, MAX_NOMBRE);
  return `${limpio || id}.eml`;
}

export { toEmailSummary } from '../client/mailPresentation.js';
