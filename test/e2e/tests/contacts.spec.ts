// e2e de la API BFF de Sytue Contacts (contacts.sytue.localhost/api/*).
// Autenticado vía storageState del admin (cookie sytue_session en .sytue.localhost),
// por lo que el fixture `request` ya viaja con sesión: no hace falta abrir página.
import { randomUUID } from 'node:crypto';
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const api = (path: string) => `${APPS.contacts}api/${path}`;

/** Sufijo único por test: Date.now() a solas puede colisionar entre workers. */
const uniq = () => `${Date.now()}-${randomUUID().slice(0, 8)}`;

// Formas mínimas de las respuestas del BFF (ver @sytue/api-clients/contacts).
interface AddressBook {
  url: string;
  displayName: string;
}
interface Contact {
  fullName: string;
  emails: string[];
}

/** Devuelve la libreta por defecto «Contactos» (la crea el servicio Go al vuelo). */
async function getLibretaContactos(request: APIRequestContext): Promise<AddressBook> {
  const res = await request.get(api('addressbooks'));
  expect(res.status(), 'GET /api/addressbooks debe responder 200').toBe(200);
  const { addressBooks } = (await res.json()) as { addressBooks: AddressBook[] };
  const libreta = addressBooks.find((b) => b.displayName === 'Contactos');
  expect(libreta, 'debe existir la libreta «Contactos»').toBeTruthy();
  return libreta!;
}

/** Limpieza: borra el contacto creado para no engordar la libreta dev. */
async function borrarContacto(request: APIRequestContext, href: string): Promise<void> {
  const res = await request.delete(api('contact'), { data: { href } });
  expect(res.status(), 'DELETE /api/contact debe responder 200').toBe(200);
}

test.describe('Contacts API', () => {
  test('GET /api/addressbooks responde 200 con la libreta «Contactos»', async ({ request }) => {
    const libreta = await getLibretaContactos(request);
    expect(libreta.url).toBeTruthy();
  });

  test('POST /api/contact crea un contacto', async ({ request }) => {
    const libreta = await getLibretaContactos(request);
    const id = uniq();
    const res = await request.post(api('contact'), {
      data: {
        addressBookUrl: libreta.url,
        contactInput: { fullName: `E2E Alta ${id}`, emails: [`e2e-alta-${id}@sytue.localhost`], phones: [] },
      },
    });
    expect(res.status(), 'POST /api/contact debe responder 200').toBe(200);
    const body = (await res.json()) as { ok: boolean; href: string; uid: string };
    expect(body.ok).toBe(true);
    expect(body.href).toContain('.vcf');
    expect(body.uid).toBeTruthy();
    await borrarContacto(request, body.href);
  });

  test('GET /api/contacts lista el contacto recién creado con su fullName', async ({ request }) => {
    const libreta = await getLibretaContactos(request);
    const id = uniq();
    const fullName = `E2E Lista ${id}`;
    const alta = await request.post(api('contact'), {
      data: {
        addressBookUrl: libreta.url,
        contactInput: { fullName, emails: [`e2e-lista-${id}@sytue.localhost`], phones: [] },
      },
    });
    expect(alta.status(), 'POST /api/contact debe responder 200').toBe(200);
    const { href } = (await alta.json()) as { href: string };

    // expect.poll cubre un posible read-after-write perezoso del CardDAV.
    await expect
      .poll(
        async () => {
          const res = await request.get(api(`contacts?addressBookUrl=${encodeURIComponent(libreta.url)}`));
          if (res.status() !== 200) return `HTTP ${res.status()}`;
          const { contacts } = (await res.json()) as { contacts: Contact[] };
          return contacts.map((c) => c.fullName);
        },
        { message: 'el contacto creado debe aparecer en GET /api/contacts', timeout: 10_000 },
      )
      .toContain(fullName);

    await borrarContacto(request, href);
  });
});
