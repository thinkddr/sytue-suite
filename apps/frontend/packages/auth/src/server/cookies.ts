/**
 * Sellado/lectura de la cookie de sesion. La sesion se cifra (JWE A256GCM con
 * clave derivada del SYTUE_SESSION_SECRET) y se emite como cookie host-only,
 * httpOnly, Secure y SameSite=Lax. No puede abarcar .sytue.eu: ese dominio
 * contiene publicaciones no confiables bajo *.apps/preview.sytue.eu.
 */
import { CompactEncrypt, compactDecrypt } from 'jose';
import type { SytueSession } from '../types.js';

export const SESSION_COOKIE = 'sytue_session';
/** Cookie efimera para el estado OIDC (state/nonce/verifier) durante el login. */
export const OIDC_STATE_COOKIE = 'sytue_oidc_state';

async function deriveKey(secret: string): Promise<Uint8Array> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(secret));
  return new Uint8Array(digest);
}

/** Cifra un valor serializable en un JWE compacto. */
export async function seal(value: unknown, secret: string): Promise<string> {
  const key = await deriveKey(secret);
  const plaintext = new TextEncoder().encode(JSON.stringify(value));
  return new CompactEncrypt(plaintext)
    .setProtectedHeader({ alg: 'dir', enc: 'A256GCM' })
    .encrypt(key);
}

/** Descifra un JWE compacto; devuelve null si es invalido. */
export async function unseal<T>(token: string | undefined, secret: string): Promise<T | null> {
  if (!token) return null;
  try {
    const key = await deriveKey(secret);
    const { plaintext } = await compactDecrypt(token, key);
    return JSON.parse(new TextDecoder().decode(plaintext)) as T;
  } catch {
    return null;
  }
}

export interface CookieOptions {
  maxAge: number;
  secure?: boolean;
}

/** Atributos de la cookie de sesion. La ausencia deliberada de Domain la hace host-only. */
export function sessionCookieAttributes(opts: CookieOptions): Record<string, unknown> {
  return {
    path: '/',
    httpOnly: true,
    secure: opts.secure ?? true,
    sameSite: 'lax',
    maxAge: opts.maxAge,
  };
}

/**
 * En HTTPS el prefijo __Host- hace que el navegador rechace cualquier intento
 * de fijar Domain o un Path distinto de / (defensa contra cookie tossing desde
 * publicaciones de usuario). Dev HTTP conserva el nombre simple.
 */
export function hostCookieName(base: string, secure: boolean): string {
  return secure ? `__Host-${base}` : base;
}

export async function sealSession(session: SytueSession, secret: string): Promise<string> {
  return seal(session, secret);
}

export async function readSession(
  cookieValue: string | undefined,
  secret: string,
): Promise<SytueSession | null> {
  if (!cookieValue) return null;
  return unseal<SytueSession>(cookieValue, secret);
}

/** Nombre base de la cookie de dispositivo que emite Sytue ID (`session.go`). */
export const DEVICE_COOKIE = 'sytue_device';

/**
 * Cabecera `cookie` para reenviar la cookie de dispositivo a Sytue ID, que la usa
 * para marcar cuál es el dispositivo ACTUAL y para probar posesión al marcarlo de
 * confianza. El BFF llama servidor a servidor con bearer y no propaga cookies.
 *
 * Se prueban los dos nombres porque el que vale depende del despliegue: en HTTPS la
 * emite `__Host-sytue_device` y en dev HTTP `sytue_device`. Reenviar el nombre
 * equivocado es como no reenviar nada —`r.Cookie` busca el EXACTO—, y el síntoma es
 * mudo: la lista sale bien y solo «actual» no se ilumina.
 */
export function deviceCookieHeader(
  get: (name: string) => string | undefined,
): Record<string, string> {
  for (const secure of [true, false]) {
    const name = hostCookieName(DEVICE_COOKIE, secure);
    const value = get(name);
    if (value) return { cookie: `${name}=${value}` };
  }
  return {};
}
