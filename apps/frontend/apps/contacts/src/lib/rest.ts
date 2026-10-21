/**
 * Cliente REST del BFF hacia la API JSON del servicio Go Sytue Contacts
 * (/api/v1). Complementa a lib/carddav.ts: los grupos (etiquetas) no tienen
 * verbo CardDAV cómodo en el SDK, así que el BFF los proxya por REST con el
 * mismo access token Bearer. Solo servidor; el token nunca toca el navegador.
 */
import { CONTACTS_BASE_URL } from './carddav.js';

/** Error HTTP del servicio Go, con el status original para reenviarlo tal cual. */
export class RestError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'RestError';
  }
}

/** Petición JSON autenticada (Bearer) contra el servicio Go. 204 → undefined. */
export async function restJson<T>(
  accessToken: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${CONTACTS_BASE_URL}${path}`, {
    method,
    headers: {
      authorization: `Bearer ${accessToken}`,
      accept: 'application/json',
      ...(body !== undefined ? { 'content-type': 'application/json' } : {}),
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
  if (res.status === 204) return undefined as T;
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new RestError(res.status, text || `HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

/**
 * Resuelve el UUID de una libreta a partir de su URL CardDAV
 * (/dav/addressbooks/<sub>/<name>/). La API REST de grupos direcciona por UUID
 * pero el frontend solo maneja URLs CardDAV: se lista /api/v1/address-books y
 * se casa el último segmento de la URL con el `name` (slug) de cada libreta.
 * Devuelve null si no hay libreta con ese slug.
 */
export async function resolveAddressBookId(
  accessToken: string,
  addressBookUrl: string,
): Promise<string | null> {
  const slug = addressBookUrl.replace(/\/+$/, '').split('/').pop() ?? '';
  if (!slug) return null;
  const { address_books: books } = await restJson<{
    address_books: Array<{ id: string; name: string }>;
  }>(accessToken, 'GET', '/api/v1/address-books');
  return books.find((b) => b.name === slug)?.id ?? null;
}
