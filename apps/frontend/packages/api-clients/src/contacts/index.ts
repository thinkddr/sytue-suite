/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Contacts — entrada del SDK al servicio Go propio (contacts.sytue.eu,
 * dev :8088). Expone la API REST JSON (comoda para la UI) y, via re-export, el
 * cliente CardDAV (RFC 6352) para sincronizacion estandar. Estado en PostgreSQL.
 * Autorizacion Bearer (JWT de Sytue ID).
 *
 * REST base:     /api/v1
 * CardDAV base:  /dav/addressbooks/users/<user>/  (ver CardDavClient)
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface ContactsOptions {
  /** URL base, p.ej. https://contacts.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

export interface AddressBookInfo {
  id: string;
  /** El identificador estable (slug) que también aparece en la URL CardDAV de
   *  la libreta: es lo que permite emparejar esta ficha REST con la que
   *  devuelve el listado CardDAV (mismos datos, dos protocolos). */
  name: string;
  displayName: string;
  readOnly?: boolean;
  /** La libreta que trae la cuenta de fábrica: el servicio no deja borrarla. */
  isDefault?: boolean;
}

export interface ContactInput {
  addressBookId: string;
  fullName: string;
  emails?: string[];
  phones?: string[];
  organization?: string;
  notes?: string;
}

export interface ContactRecord extends ContactInput {
  id: string;
  uid: string;
  etag?: string;
  createdAt: string;
  updatedAt: string;
}

/** Lo que devuelve el servicio, tal cual: `snake_case` y nombres de vCard. */
interface LibretaCruda {
  id: string;
  name?: string;
  display_name?: string;
  is_default?: boolean;
  description?: string;
  color?: string;
}

/**
 * `name` es el identificador ESTABLE de la libreta (va en la URL CardDAV) y
 * tiene que ser único para el dueño; `display_name` es lo único que rellena
 * quien crea la libreta. Se deriva uno del título y se le añade un sufijo corto
 * para que dos libretas con el mismo título no choquen (el servicio devuelve
 * 409 si el nombre ya existe, en vez de fallar en silencio).
 */
function slugify(displayName: string): string {
  const base =
    displayName
      .trim()
      .toLowerCase()
      .normalize('NFD')
      .replace(/[̀-ͯ]/g, '') // acentos
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 96) || 'libreta';
  return `${base}-${Math.random().toString(36).slice(2, 8)}`;
}

function libretaInfo(b: LibretaCruda): AddressBookInfo {
  return {
    id: b.id,
    name: b.name ?? '',
    displayName: b.display_name || b.name || '',
    ...(b.is_default ? { isDefault: true } : {}),
  };
}

interface ContactoCrudo {
  id: string;
  uid: string;
  address_book_id: string;
  formatted_name: string;
  organization?: string;
  note?: string;
  emails?: { email?: string }[];
  phones?: { phone?: string }[];
  etag?: string;
  created_at?: string;
  updated_at?: string;
}

/**
 * El contacto del servicio → el del SDK.
 *
 * Sin esta traducción, `contacto.fullName` sale `undefined` —el servicio lo
 * llama `formatted_name`— y los correos son objetos donde el SDK promete
 * cadenas. No falla nada: sale una lista de contactos sin nombre.
 */
function contactoARegistro(c: ContactoCrudo): ContactRecord {
  return {
    id: c.id,
    uid: c.uid,
    addressBookId: c.address_book_id,
    fullName: c.formatted_name,
    ...(c.emails?.length ? { emails: c.emails.map((e) => e.email ?? '').filter(Boolean) } : {}),
    ...(c.phones?.length ? { phones: c.phones.map((t) => t.phone ?? '').filter(Boolean) } : {}),
    ...(c.organization ? { organization: c.organization } : {}),
    ...(c.note ? { notes: c.note } : {}),
    ...(c.etag ? { etag: c.etag } : {}),
    createdAt: c.created_at ?? '',
    updatedAt: c.updated_at ?? '',
  };
}

/**
 * El contacto del SDK → el cuerpo que el servicio espera.
 *
 * `addressBookId` no va en el cuerpo: la libreta la fija la ruta. Y los
 * correos y teléfonos son objetos, no cadenas.
 */
function aCuerpo(input: ContactInput): Record<string, unknown> {
  return {
    formatted_name: input.fullName,
    ...(input.organization ? { organization: input.organization } : {}),
    ...(input.notes ? { note: input.notes } : {}),
    ...(input.emails?.length ? { emails: input.emails.map((email) => ({ email })) } : {}),
    ...(input.phones?.length ? { phones: input.phones.map((phone) => ({ phone })) } : {}),
  };
}

export class ContactsClient {
  private readonly http: HttpClient;

  constructor(options: ContactsOptions) {
    this.http = new HttpClient({
      baseUrl: `${options.baseUrl.replace(/\/$/, '')}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /**
   * Libretas de direcciones del usuario.
   *
   * Pedía `/addressbooks` —sin guion— y esperaba un array pelado. La ruta del
   * servicio es `/address-books` y contesta `{address_books: […]}`: dos fallos
   * en una línea, y el resultado era un 404 en cada llamada.
   */
  async listAddressBooks(): Promise<AddressBookInfo[]> {
    const r = await this.http.json<{ address_books?: LibretaCruda[] }>({
      path: '/address-books',
    });
    return (r.address_books ?? []).map(libretaInfo);
  }

  /**
   * Crea una libreta propia. `name` es el identificador estable (va en la URL
   * CardDAV, así que se deriva del título si no se aporta); `displayName` es lo
   * que se enseña y lo único que hace falta rellenar.
   */
  async createAddressBook(displayName: string): Promise<AddressBookInfo> {
    const b = await this.http.json<LibretaCruda>({
      method: 'POST',
      path: '/address-books',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ name: slugify(displayName), display_name: displayName }),
    });
    return libretaInfo(b);
  }

  /** Renombra una libreta propia. */
  async renameAddressBook(id: string, displayName: string): Promise<AddressBookInfo> {
    const b = await this.http.json<LibretaCruda>({
      method: 'PATCH',
      path: `/address-books/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ display_name: displayName }),
    });
    return libretaInfo(b);
  }

  /** Borra una libreta propia. El servicio rechaza borrar la libreta por defecto. */
  async deleteAddressBook(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/address-books/${encodeURIComponent(id)}` });
  }

  /**
   * Contactos, con filtro de libreta o de texto.
   *
   * El servicio tiene TRES puertas y no una: `/contacts` es una BÚSQUEDA y
   * exige `q` (sin él responde 400, no la lista entera), `/contacts/all` es el
   * listado del usuario y `/address-books/{id}/contacts` el de una libreta.
   * Este método pedía siempre la primera con el parámetro equivocado
   * (`addressBook` en vez de `address_book_id`), así que sin texto de búsqueda
   * devolvía un 400 y con él, nada de la libreta pedida.
   *
   * `offset` y no `cursor`: la paginación de este servicio es por
   * desplazamiento, y el cursor que prometía esta firma no lo lee nadie.
   */
  async listContacts(options?: {
    addressBookId?: string;
    query?: string;
    limit?: number;
    offset?: number;
  }): Promise<ContactRecord[]> {
    const limit = options?.limit ?? 100;
    const texto = options?.query?.trim();
    const path = texto
      ? '/contacts'
      : options?.addressBookId
        ? `/address-books/${encodeURIComponent(options.addressBookId)}/contacts`
        : '/contacts/all';
    const r = await this.http.json<{ contacts?: ContactoCrudo[] }>({
      path,
      query: {
        limit,
        ...(texto ? { q: texto } : {}),
        ...(texto && options?.addressBookId ? { address_book_id: options.addressBookId } : {}),
        ...(options?.offset ? { offset: options.offset } : {}),
      },
    });
    return (r.contacts ?? []).map(contactoARegistro);
  }

  /**
   * Crea un contacto en la libreta que dice `input.addressBookId`.
   *
   * Pedía `POST /contacts`, que no existe: el servicio crea dentro de una
   * libreta, porque un contacto sin ella no tiene dónde sincronizarse por
   * CardDAV.
   */
  async createContact(input: ContactInput): Promise<ContactRecord> {
    const crudo = await this.http.postJson<ContactoCrudo>(
      `/address-books/${encodeURIComponent(input.addressBookId)}/contacts`,
      aCuerpo(input),
    );
    return contactoARegistro(crudo);
  }

  /**
   * Da de alta una tarjeta de FUERA entera (`Content-Type: text/vcard`): el
   * servicio conserva lo que la ficha no enseña —los X-* de otros clientes— y
   * pasa la foto binaria de vCard 3.0 (`ENCODING=b`) a data URI para que se vea
   * (`domain.ImportCard`). Le da un UID nuevo: importar dos veces el mismo fichero
   * no pisa lo ya importado. Es el `ImportCard` de la app nativa.
   */
  async importCard(addressBookId: string, vcard: string): Promise<ContactRecord> {
    const crudo = await this.http.json<ContactoCrudo>({
      method: 'POST',
      path: `/address-books/${encodeURIComponent(addressBookId)}/contacts`,
      headers: { 'content-type': 'text/vcard; charset=utf-8' },
      body: vcard,
    });
    return contactoARegistro(crudo);
  }

  /**
   * Cambia lo que se le diga de un contacto y deja el resto como estaba.
   *
   * Mandaba PATCH a una ruta que solo acepta PUT (405), y un parche suelto a un
   * endpoint que REEMPLAZA la vCard entera: corregir un teléfono habría borrado
   * los correos y la organización. Por eso se lee antes de escribir.
   */
  async updateContact(id: string, patch: Partial<ContactInput>): Promise<ContactRecord> {
    const ruta = `/contacts/${encodeURIComponent(id)}`;
    const actual = contactoARegistro(await this.http.json<ContactoCrudo>({ path: ruta }));
    const crudo = await this.http.json<ContactoCrudo>({
      method: 'PUT',
      path: ruta,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(aCuerpo({ ...actual, ...patch })),
    });
    return contactoARegistro(crudo);
  }

  /** Elimina un contacto. */
  async deleteContact(id: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/contacts/${encodeURIComponent(id)}` });
  }
}

export { CardDavClient, parseVcard } from './carddav.js';
export type { CardDavOptions, AddressBook, Contact } from './carddav.js';
