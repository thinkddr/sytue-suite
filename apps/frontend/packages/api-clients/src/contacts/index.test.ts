// Qué pide el SDK de Contacts y qué entiende de lo que le contestan.
//
// Los cuatro métodos REST de este cliente pedían rutas o métodos que el
// servicio no tiene, y ninguno daba error en consola: una lista vacía y una
// acción que no ocurre no se parecen a un 404. Medido contra
// `services/contacts/internal/http`. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { ContactsClient } from './index.js';

interface Llamada {
  method: string;
  path: string;
  query: string;
  body: unknown;
}

function clienteQueApunta(respuestas: Record<string, unknown>): {
  cliente: ContactsClient;
  llamadas: Llamada[];
} {
  const llamadas: Llamada[] = [];
  const fetchImpl = (async (url: string | URL, init?: RequestInit) => {
    const u = new URL(String(url));
    const path = u.pathname.replace('/api/v1', '');
    llamadas.push({
      method: init?.method ?? 'GET',
      path,
      query: u.search,
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    });
    const cuerpo = respuestas[`${init?.method ?? 'GET'} ${path}`] ?? respuestas[path];
    if (cuerpo === undefined) {
      return new Response(JSON.stringify({ error: 'not found' }), { status: 404 });
    }
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  return {
    cliente: new ContactsClient({
      baseUrl: 'https://contacts.test',
      auth: { getAccessToken: () => 'tok' },
      fetchImpl,
    }),
    llamadas,
  };
}

const CRUDO = {
  id: 'k1',
  uid: 'u1',
  address_book_id: 'b1',
  formatted_name: 'Ana Ruiz',
  organization: 'Sytue',
  note: 'Del comité',
  emails: [{ email: 'ana@sytue.eu' }],
  phones: [{ phone: '+34600000000' }],
};

test('listAddressBooks usa /address-books y desenvuelve el sobre', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    '/address-books': {
      address_books: [{ id: 'b1', name: 'personal', display_name: 'Personal' }],
    },
  });
  const libretas = await cliente.listAddressBooks();
  expect(llamadas[0]!.path).toBe('/address-books');
  expect(libretas).toEqual([{ id: 'b1', name: 'personal', displayName: 'Personal' }]);
});

test('sin filtro, listContacts pide el listado y no la búsqueda', async () => {
  const { cliente, llamadas } = clienteQueApunta({ '/contacts/all': { contacts: [CRUDO] } });
  const out = await cliente.listContacts();
  // `/contacts` exige `q`: sin texto habría contestado 400, no la lista entera.
  expect(llamadas[0]!.path).toBe('/contacts/all');
  expect(out[0]!.fullName).toBe('Ana Ruiz');
  expect(out[0]!.emails).toEqual(['ana@sytue.eu']);
  expect(out[0]!.notes).toBe('Del comité');
});

test('con libreta y sin texto, pide los contactos de esa libreta', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    '/address-books/b1/contacts': { contacts: [] },
  });
  await cliente.listContacts({ addressBookId: 'b1' });
  expect(llamadas[0]!.path).toBe('/address-books/b1/contacts');
});

test('con texto, busca y manda address_book_id con el nombre que el servicio lee', async () => {
  const { cliente, llamadas } = clienteQueApunta({ '/contacts': { contacts: [CRUDO] } });
  await cliente.listContacts({ query: 'ana', addressBookId: 'b1' });
  expect(llamadas[0]!.path).toBe('/contacts');
  expect(llamadas[0]!.query).toContain('q=ana');
  expect(llamadas[0]!.query).toContain('address_book_id=b1');
});

test('createContact crea dentro de la libreta y traduce los campos', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    'POST /address-books/b1/contacts': CRUDO,
  });
  const c = await cliente.createContact({
    addressBookId: 'b1',
    fullName: 'Ana Ruiz',
    emails: ['ana@sytue.eu'],
  });
  expect(llamadas[0]!.path).toBe('/address-books/b1/contacts');
  const cuerpo = llamadas[0]!.body as Record<string, unknown>;
  expect(cuerpo['formatted_name']).toBe('Ana Ruiz');
  expect(cuerpo['emails']).toEqual([{ email: 'ana@sytue.eu' }]);
  expect(cuerpo['addressBookId']).toBeUndefined();
  expect(c.fullName).toBe('Ana Ruiz');
});

test('updateContact lee antes de escribir: cambiar el teléfono no borra el correo', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    'GET /contacts/k1': CRUDO,
    'PUT /contacts/k1': { ...CRUDO, phones: [{ phone: '+34611111111' }] },
  });
  await cliente.updateContact('k1', { phones: ['+34611111111'] });
  expect(llamadas.map((l) => l.method)).toEqual(['GET', 'PUT']);
  const enviado = llamadas[1]!.body as Record<string, unknown>;
  expect(enviado['phones']).toEqual([{ phone: '+34611111111' }]);
  expect(enviado['emails']).toEqual([{ email: 'ana@sytue.eu' }]);
  expect(enviado['organization']).toBe('Sytue');
});

// La tarjeta de fuera va ENTERA, como `text/vcard`: es el alta que conserva la
// foto binaria de vCard 3.0 y los X-* (`domain.ImportCard`, #938).
test('importCard manda la tarjeta cruda como text/vcard a la libreta', async () => {
  const tarjeta =
    'BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ana\r\nPHOTO;ENCODING=b;TYPE=JPEG:/9j/4AAQ\r\nEND:VCARD\r\n';
  let visto: { url: string; tipo: string | null; cuerpo: unknown } | undefined;
  const cliente = new ContactsClient({
    baseUrl: 'https://contacts.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl: (async (url: string | URL, init?: RequestInit) => {
      visto = {
        url: String(url),
        tipo: new Headers(init?.headers).get('content-type'),
        cuerpo: init?.body,
      };
      return new Response(JSON.stringify(CRUDO), {
        status: 201,
        headers: { 'content-type': 'application/json' },
      });
    }) as unknown as typeof fetch,
  });
  const c = await cliente.importCard('b1', tarjeta);
  expect(visto).toEqual({
    url: 'https://contacts.test/api/v1/address-books/b1/contacts',
    tipo: 'text/vcard; charset=utf-8',
    cuerpo: tarjeta,
  });
  expect(c.fullName).toBe('Ana Ruiz');
});
