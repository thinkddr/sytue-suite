/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Contacts — SDK CardDAV (RFC 6352) contra el servicio Go propio
 * (contacts.sytue.eu, dev :8088). Implementacion clean-room sobre el estandar
 * CardDAV (NO Nextcloud). Descubre libretas (PROPFIND) y consulta/crea/borra
 * contactos (REPORT/PUT/DELETE con cuerpos vCard). Estado en PostgreSQL.
 * Autorizacion Bearer (JWT de Sytue ID).
 *
 * Base CardDAV del servicio Go: /dav/addressbooks/<user>/
 */
import { HttpClient, sameOriginUrl, type AuthProvider } from '../core/http.js';
import { firstText, parseXml } from '../core/xml.js';

export interface CardDavOptions {
  /** URL base, p.ej. https://contacts.sytue.eu */
  baseUrl: string;
  userId: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

export interface AddressBook {
  url: string;
  displayName: string;
  ctag?: string | undefined;
}

export interface Contact {
  href: string;
  etag?: string | undefined;
  uid: string;
  fullName: string;
  emails: string[];
  phones: string[];
  organization?: string | undefined;
  /** vCard cruda. */
  vcard: string;
}

const PROPFIND_BOOKS = `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:" xmlns:cs="http://calendarserver.org/ns/" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop><d:resourcetype/><d:displayname/><cs:getctag/></d:prop>
</d:propfind>`;

const ADDRESSBOOK_QUERY = `<?xml version="1.0"?>
<card:addressbook-query xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">
  <d:prop><d:getetag/><card:address-data/></d:prop>
</card:addressbook-query>`;

export class CardDavClient {
  private readonly http: HttpClient;
  private readonly root: string;

  constructor(options: CardDavOptions) {
    this.http = new HttpClient({
      baseUrl: options.baseUrl,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
    this.root = `/dav/addressbooks/${encodeURIComponent(options.userId)}`;
  }

  /** Lista las libretas de direcciones. */
  async listAddressBooks(): Promise<AddressBook[]> {
    const res = await this.http.request({
      method: 'PROPFIND',
      path: `${this.root}/`,
      headers: { depth: '1', 'content-type': 'application/xml' },
      body: PROPFIND_BOOKS,
    });
    const doc = parseXml(await res.text());
    return Array.from(doc.getElementsByTagNameNS('DAV:', 'response'))
      .map((node): AddressBook | null => {
        const url = firstText(node, 'href');
        const isBook =
          node.getElementsByTagNameNS('urn:ietf:params:xml:ns:carddav', 'addressbook').length > 0;
        if (!url || !isBook) return null;
        return {
          url: decodeURIComponent(url),
          displayName: firstText(node, 'displayname') ?? 'Contactos',
          ctag: firstText(node, 'getctag'),
        } satisfies AddressBook;
      })
      .filter((b): b is AddressBook => b !== null);
  }

  /** Lista los contactos de una libreta (addressbook-query REPORT). */
  async listContacts(addressBookUrl: string): Promise<Contact[]> {
    const res = await this.http.request({
      method: 'REPORT',
      url: this.absolute(addressBookUrl),
      headers: { depth: '1', 'content-type': 'application/xml' },
      body: ADDRESSBOOK_QUERY,
    });
    const doc = parseXml(await res.text());
    return Array.from(doc.getElementsByTagNameNS('DAV:', 'response'))
      .map((node): Contact | null => {
        const href = firstText(node, 'href');
        const vcard = firstText(node, 'address-data');
        if (!href || !vcard) return null;
        return {
          href: decodeURIComponent(href),
          etag: firstText(node, 'getetag'),
          ...parseVcard(vcard),
          vcard,
        } satisfies Contact;
      })
      .filter((c): c is Contact => c !== null);
  }

  /** Crea o actualiza un contacto (PUT de la vCard). */
  async putContact(contactUrl: string, vcard: string, etag?: string): Promise<void> {
    await this.http.request({
      method: 'PUT',
      url: this.absolute(contactUrl),
      headers: {
        'content-type': 'text/vcard; charset=utf-8',
        ...(etag ? { 'if-match': etag } : { 'if-none-match': '*' }),
      },
      body: vcard,
    });
  }

  /** Elimina un contacto. */
  async deleteContact(contactUrl: string): Promise<void> {
    await this.http.request({ method: 'DELETE', url: this.absolute(contactUrl) });
  }

  // El recurso lo elige quien llama (y, vía el BFF, la consulta del navegador):
  // se exige que quede en el servicio configurado para no mandar el Bearer fuera.
  private absolute(url: string): string {
    return sameOriginUrl(this.http.baseUrl, url);
  }
}

/** Parser ligero de vCard 3/4 para los campos basicos. */
export function parseVcard(
  vcard: string,
): Pick<Contact, 'uid' | 'fullName' | 'emails' | 'phones' | 'organization'> {
  const lines = vcard.split(/\r?\n/);
  const emails: string[] = [];
  const phones: string[] = [];
  let uid = '';
  let fullName = '';
  let organization: string | undefined;
  for (const line of lines) {
    const [rawKey, ...rest] = line.split(':');
    if (!rawKey) continue;
    const value = rest.join(':').trim();
    const key = rawKey.split(';')[0]?.toUpperCase();
    if (key === 'UID') uid = value;
    else if (key === 'FN') fullName = value;
    else if (key === 'EMAIL') emails.push(value);
    else if (key === 'TEL') phones.push(value);
    else if (key === 'ORG') organization = value.replace(/;/g, ' ').trim();
  }
  return { uid, fullName: fullName || '(Sin nombre)', emails, phones, organization };
}
