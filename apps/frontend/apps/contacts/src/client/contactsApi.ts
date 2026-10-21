/**
 * Cliente de navegador de Sytue Contacts: habla con los endpoints BFF same-origin
 * (cookie). El servidor reenvia al servicio Go Sytue Contacts (CardDAV) con el
 * access token Bearer. Asi el access token no toca el navegador para los datos de
 * contactos. Reexporta los tipos del SDK @sytue/api-clients.
 */
import type { AddressBook as AddressBookCardDav, Contact } from '@sytue/api-clients';
import type { TypedValue, PostalAddress } from '../lib/vcard.js';
/*
 * `withBase` en la vuelta al login: la app se sirve bajo su prefijo en el dominio
 * de organización (`<org>.sytue.eu/mail`, `/calendar`…), y ahí `/auth/login` a pelo
 * es la raíz del host, que devuelve el 404 de Traefik. O sea: al caducar la sesión
 * se acababa en una página de error en vez de en el formulario. El shim de `fetch`
 * (packages/ui/src/basePath.ts) no llega hasta aquí porque esto no es un fetch.
 */
import { withBase } from '@sytue/ui';

/**
 * El id REST (para renombrar/borrar) no lo trae CardDAV; el BFF lo empareja
 * por el slug de la URL y lo añade si lo encuentra (ver pages/api/addressbooks.ts).
 * Ausente ⇒ la libreta se lista igual, solo sin renombrar/borrar.
 */
export interface AddressBook extends AddressBookCardDav {
  id?: string;
  /** La libreta de fábrica: el servicio no deja borrarla (sí renombrarla). */
  isDefault?: boolean;
}
export type { Contact };
export type { TypedValue, PostalAddress };

export interface ContactInput {
  uid?: string;
  fullName: string;
  firstName?: string;
  lastName?: string;
  emails: string[];
  phones: string[];
  /** Correos/teléfonos con etiqueta y principal (editor): ganan a los planos. */
  emailsDetailed?: TypedValue[];
  phonesDetailed?: TypedValue[];
  /** Direcciones postales (ADR); definido (aunque sea []) = el editor las gestiona. */
  addresses?: PostalAddress[];
  /** Cumpleaños YYYY-MM-DD; definido (aunque sea '') = el editor gestiona BDAY. */
  birthday?: string;
  organization?: string;
  title?: string;
  note?: string;
  /**
   * vCard cruda existente (editar/fusionar): el BFF conserva sus propiedades no
   * proyectadas (PHOTO/X-*…, y ADR/BDAY si el editor no las gestiona) al
   * regenerar la tarjeta. Sin base = campos solo de la proyección (crear/importar).
   */
  baseVcard?: string;
}

async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: 'include', headers: { accept: 'application/json' } });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  if (!res.ok) throw new Error(`Error ${res.status}`);
  return (await res.json()) as T;
}

async function sendJson<T>(method: string, url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`Error ${res.status}`);
  return (await res.json()) as T;
}

/**
 * Pone al día la libreta «Organización» con quien trabaja en la empresa.
 *
 * NO LANZA: es una mejora de la agenda, no un requisito para abrirla. Si el
 * directorio no responde, Contactos tiene que funcionar igual con lo que ya hay.
 */
export async function sincronizarDirectorio(): Promise<void> {
  try {
    await fetch('/api/directorio', { method: 'POST', credentials: 'include' });
  } catch {
    // Sin directorio se sigue: las libretas propias no dependen de él.
  }
}

export function listAddressBooks(): Promise<{ addressBooks: AddressBook[] }> {
  return getJson('/api/addressbooks');
}

export function createAddressBook(displayName: string): Promise<{ addressBook: AddressBook }> {
  return sendJson('POST', '/api/addressbooks', { displayName });
}

export function renameAddressBook(
  id: string,
  displayName: string,
): Promise<{ addressBook: AddressBook }> {
  return sendJson('PATCH', '/api/addressbooks', { id, displayName });
}

export function deleteAddressBook(id: string): Promise<{ ok: boolean }> {
  return sendJson('DELETE', `/api/addressbooks?id=${encodeURIComponent(id)}`, undefined);
}

export function listContacts(addressBookUrl: string): Promise<{ contacts: Contact[] }> {
  return getJson(`/api/contacts?addressBookUrl=${encodeURIComponent(addressBookUrl)}`);
}

export function saveContact(input: {
  addressBookUrl: string;
  contactInput: ContactInput;
  href?: string;
  etag?: string;
}): Promise<{ ok: boolean; href: string; uid: string }> {
  return sendJson('POST', '/api/contact', input);
}

export function deleteContact(href: string): Promise<{ ok: boolean }> {
  return sendJson('DELETE', '/api/contact', { href });
}

/** Grupo de contactos (etiqueta) del servicio Go: MEMBER por UID (RFC 6350). */
export interface ContactGroup {
  id: string;
  uid: string;
  name: string;
  /** UID de los contactos miembros. */
  members: string[];
  etag: string;
}

export function listGroups(addressBookUrl: string): Promise<{ groups: ContactGroup[] }> {
  return getJson(`/api/groups?addressBookUrl=${encodeURIComponent(addressBookUrl)}`);
}

export function createGroup(addressBookUrl: string, name: string): Promise<ContactGroup> {
  return sendJson('POST', '/api/groups', { addressBookUrl, name });
}

/** Reemplaza nombre y pertenencia completa del grupo (PUT en el servicio Go). */
export function updateGroup(input: {
  id: string;
  name: string;
  members: string[];
}): Promise<ContactGroup> {
  return sendJson('POST', '/api/group', input);
}

/** Elimina una etiqueta. Los contactos que la llevaban NO se tocan: solo pierden
 *  la etiqueta (el servicio Go borra el grupo, no sus miembros). */
export function deleteGroup(id: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/group', { id, delete: true });
}

// --- Favoritos (destacados) --------------------------------------------------

/** UID de los contactos destacados del usuario (para pintar la estrella/filtrar). */
export function listFavorites(): Promise<{ uids: string[] }> {
  return getJson('/api/favorites');
}

/** Destaca (favorite=true) o quita el destacado de un contacto por su UID. */
export function toggleFavorite(uid: string, favorite: boolean): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/favorites', { uid, favorite });
}

// --- Papelera ----------------------------------------------------------------

/** Entrada de la papelera (contacto borrado suave) del servicio Go. */
export interface TrashItem {
  id: string;
  uid: string;
  formatted_name: string;
  organization?: string;
  primary_email?: string;
  deleted_at: string;
}

export function listTrash(): Promise<{ items: TrashItem[] }> {
  return getJson('/api/trash');
}

export function restoreContact(id: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/trash', { id, action: 'restore' });
}

export function purgeContact(id: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/trash', { id, action: 'purge' });
}

// --- Acciones EN LOTE (multiselección) ---------------------------------------

/**
 * Envía a la papelera los contactos indicados (por UID) en una sola operación.
 * El servicio Go resuelve el borrado suave con `uid = ANY` y SOLO sobre contactos
 * del propio dueño. Devuelve cuántos se enviaron a la papelera.
 */
export function batchTrash(uids: string[]): Promise<{ trashed: number }> {
  return sendJson('POST', '/api/batch', { action: 'trash', uids });
}

/** Destaca o quita el destacado de los contactos indicados (por UID) en lote. */
export function batchFavorite(uids: string[], favorite: boolean): Promise<{ updated: number }> {
  return sendJson('POST', '/api/batch', { action: 'favorite', uids, favorite });
}

/** Añade (add=true) o quita (add=false) los contactos indicados a un grupo, en lote. */
export function batchGroupMembers(
  groupId: string,
  uids: string[],
  add: boolean,
): Promise<ContactGroup> {
  return sendJson('POST', '/api/batch', { action: 'group', groupId, uids, add });
}

// --- Foto de contacto (PHOTO embebida en la vCard) ---------------------------

/** UID desnudo (sin urn:uuid:), misma clave de foto que usa la lista. */
export function cleanUid(uid: string): string {
  return uid.toLowerCase().startsWith('urn:uuid:') ? uid.slice(9) : uid;
}

/**
 * URL del avatar de un contacto (bytes servidos por el BFF, cacheables). `version`
 * (el etag de la tarjeta) invalida la caché del navegador cuando la foto cambia.
 * Solo se usa cuando la vCard indica que hay foto (hasEmbeddedPhoto).
 */
export function photoUrl(uid: string, version?: string): string {
  const v = version ? `&v=${encodeURIComponent(version)}` : '';
  // CON withBase: esto acaba en un `src`, no en un `fetch`, así que el shim de
  // `basePathScript` —que prefija las rutas `/api/` de fetch— no lo alcanza. Sin
  // el prefijo, en `<organizacion>.sytue.eu` la petición sale a la RAÍZ del host,
  // donde el ingress del tenant no enruta `/api`: 404 de Traefik. Medido.
  return withBase(import.meta.env.BASE_URL, `/api/photo?uid=${encodeURIComponent(uid)}${v}`);
}

/** Define/reemplaza la foto de un contacto (data URI ya redimensionado). */
export function setPhoto(uid: string, dataUri: string): Promise<{ photo: boolean }> {
  return sendJson('POST', '/api/photo', { uid, data: dataUri });
}

/** Quita la foto de un contacto. */
export function removePhoto(uid: string): Promise<{ ok: boolean }> {
  return sendJson('POST', '/api/photo', { uid, remove: true });
}

/** Da de alta UNA tarjeta vCard de fuera, entera: la interpreta el servicio. */
export function importCard(addressBookId: string, vcard: string): Promise<{ id: string }> {
  return sendJson('POST', '/api/import-vcard', { addressBookId, vcard });
}

// --- Import CSV (Google Contacts / Outlook) ----------------------------------

/** Importa un CSV a una libreta; devuelve cuántos se importaron y cuántos se omitieron. */
export function importCsv(
  addressBookUrl: string,
  csv: string,
): Promise<{ imported: number; skipped: number }> {
  return sendJson('POST', '/api/import-csv', { addressBookUrl, csv });
}
