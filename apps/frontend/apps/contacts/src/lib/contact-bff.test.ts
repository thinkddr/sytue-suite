/**
 * El BFF de guardar contacto lleva a la vCard el cargo y el apodo que manda el
 * formulario. El apodo se perdía aquí: el modal lo mandaba, `buildVcard` lo sabía
 * escribir, pero `/api/contact` armaba el ContactInput sin él y el PUT a CardDAV
 * salía sin NICKNAME (y con la línea de la base intacta, así que ni se borraba).
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () => Promise.resolve({ accessToken: 't', user: { id: 'ana' } }),
}));

const { POST } = await import('../pages/api/contact.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

const BASE = [
  'BEGIN:VCARD',
  'VERSION:4.0',
  'UID:ana',
  'FN:Ana García',
  'TITLE:Directora',
  'NICKNAME;LANGUAGE=es:Anita',
  'END:VCARD',
].join('\r\n');

/** Guarda `contactInput` por el BFF y devuelve la vCard que llegó a CardDAV. */
async function vcardGuardada(contactInput: Record<string, unknown>): Promise<string> {
  const cuerpos: string[] = [];
  globalThis.fetch = ((_url: string, init: RequestInit) => {
    cuerpos.push(String(init.body));
    return Promise.resolve(new Response('', { status: 201, headers: { etag: '"e2"' } }));
  }) as unknown as typeof fetch;
  const request = new Request('https://contacts.sytue.eu/contacts/api/contact', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      addressBookUrl: '/dav/ana/libreta/',
      href: '/dav/ana/libreta/ana.vcf',
      contactInput,
    }),
  });
  const res = (await POST({ request } as unknown as APIContext)) as Response;
  expect(res.status).toBe(200);
  expect(cuerpos).toHaveLength(1);
  return cuerpos[0] ?? '';
}

const lineas = (vcard: string, prop: string): string[] =>
  vcard.split(/\r\n/).filter((l) => l.split(/[;:]/)[0] === prop);

test('/api/contact escribe el cargo y el apodo que cambia el formulario', async () => {
  const vcard = await vcardGuardada({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    title: 'Directora general',
    nickname: 'Anita, La Jefa',
    baseVcard: BASE,
  });
  expect(lineas(vcard, 'TITLE')).toEqual(['TITLE:Directora general']);
  expect(lineas(vcard, 'NICKNAME')).toEqual(['NICKNAME:Anita,La Jefa']);
});

test('/api/contact borra el apodo vaciado y conserva el que no viaja', async () => {
  const vaciado = await vcardGuardada({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    nickname: '',
    baseVcard: BASE,
  });
  expect(lineas(vaciado, 'NICKNAME')).toEqual([]);
  const intacto = await vcardGuardada({
    uid: 'ana',
    fullName: 'Ana García',
    emails: [],
    phones: [],
    baseVcard: BASE,
  });
  expect(lineas(intacto, 'NICKNAME')).toEqual(['NICKNAME;LANGUAGE=es:Anita']);
});
