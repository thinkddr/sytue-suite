/**
 * PKCE (RFC 7636) — generacion de code_verifier/code_challenge y `state`/`nonce`.
 * Usa WebCrypto (disponible en Node 26 y en el edge). Sin secretos persistentes.
 */

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function randomBytes(length: number): Uint8Array {
  const arr = new Uint8Array(length);
  crypto.getRandomValues(arr);
  return arr;
}

/** Valor aleatorio url-safe (state, nonce). */
export function randomToken(bytes = 32): string {
  return base64UrlEncode(randomBytes(bytes));
}

export interface PkcePair {
  codeVerifier: string;
  codeChallenge: string;
  codeChallengeMethod: 'S256';
}

/** Genera un par PKCE con method S256. */
export async function createPkcePair(): Promise<PkcePair> {
  const codeVerifier = base64UrlEncode(randomBytes(64));
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(codeVerifier));
  const codeChallenge = base64UrlEncode(new Uint8Array(digest));
  return { codeVerifier, codeChallenge, codeChallengeMethod: 'S256' };
}
