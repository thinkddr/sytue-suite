/**
 * /api/contacts — «Añadir a Contactos» / «Ver en Contactos» desde el remitente (MAIL-12).
 * `?email=` dice si la dirección EXACTA ya tiene ficha (la búsqueda del servicio es por
 * subcadena), y el POST la crea en la libreta por defecto solo si no la había: pulsar
 * dos veces, o desde dos pestañas, no deja dos fichas. Ejecutar: `bun test`.
 */
import { afterEach, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({ accessToken: 't', user: { id: 'ada', name: 'Ada', email: 'ada@sytue.eu' } }),
}));

const { GET, POST } = await import('../pages/api/contacts.js');
const { contactoConCorreo, esSinRespuesta, ofrecerContacto, nombreDelContacto } =
  await import('./contactoRemitente.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

const ANA = '0b6f6c2e-6a39-4bd2-9a57-4f2d3c9f5a10';
const MARIANA = '6c1d4c4e-0d8a-4bb8-9d3f-1f0f3a7c2b11';
const LIBRO = '9a2b1c3d-4e5f-4a6b-8c7d-0e1f2a3b4c5d';

/** Contacts simulado: una libreta por defecto y lo que haya en `fichas`. */
function servicio(fichas: { id: string; emails: { email: string }[] }[]) {
  const llamadas: { metodo: string; ruta: string; cuerpo: unknown }[] = [];
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    const u = new URL(String(url));
    const metodo = init?.method ?? 'GET';
    const cuerpo = init?.body ? JSON.parse(String(init.body)) : null;
    llamadas.push({ metodo, ruta: u.pathname, cuerpo });
    if (u.pathname === '/api/v1/contacts') return Response.json({ contacts: fichas });
    if (u.pathname === '/api/v1/address-books') {
      return Response.json({
        address_books: [
          { id: 'otra', is_default: false },
          { id: LIBRO, is_default: true },
        ],
      });
    }
    if (u.pathname === `/api/v1/address-books/${LIBRO}/contacts` && metodo === 'POST') {
      const id = '11111111-2222-4333-8444-555555555555';
      fichas.push({ id, emails: cuerpo.emails });
      return Response.json({ id }, { status: 201 });
    }
    return new Response('{}', { status: 404 });
  }) as unknown as typeof fetch;
  return llamadas;
}

function contexto(request: Request): APIContext {
  return { request, url: new URL(request.url) } as unknown as APIContext;
}

const post = (cuerpo: unknown) =>
  POST(
    contexto(
      new Request('https://mail.sytue.eu/api/contacts', {
        method: 'POST',
        body: JSON.stringify(cuerpo),
      }),
    ),
  );

test('?email= da el id de la ficha con esa dirección exacta, no la de otra que la contenga', async () => {
  servicio([
    { id: MARIANA, emails: [{ email: 'mariana@example.eu' }] },
    { id: ANA, emails: [{ email: 'Ana@Example.eu' }] },
  ]);
  const res = await GET(
    contexto(new Request('https://mail.sytue.eu/api/contacts?email=ana@example.eu')),
  );
  expect(await res.json()).toEqual({ id: ANA });

  servicio([{ id: MARIANA, emails: [{ email: 'mariana@example.eu' }] }]);
  const nadie = await GET(
    contexto(new Request('https://mail.sytue.eu/api/contacts?email=ana@example.eu')),
  );
  expect(await nadie.json()).toEqual({ id: null });
});

test('el POST crea la ficha en la libreta por defecto y la segunda vez devuelve la misma', async () => {
  const fichas: { id: string; emails: { email: string }[] }[] = [];
  const llamadas = servicio(fichas);
  const res = await post({ name: 'Ana Ruiz', email: 'ana@example.eu' });
  expect(res.status).toBe(201);
  const creada = (await res.json()) as { id: string; created: boolean };
  expect(creada.created).toBe(true);
  expect(llamadas.find((l) => l.metodo === 'POST')).toEqual({
    metodo: 'POST',
    ruta: `/api/v1/address-books/${LIBRO}/contacts`,
    cuerpo: {
      formatted_name: 'Ana Ruiz',
      emails: [{ email: 'ana@example.eu', is_primary: true }],
    },
  });

  const otra = await post({ name: 'Ana Ruiz', email: 'ana@example.eu' });
  expect(otra.status).toBe(200);
  expect(await otra.json()).toEqual({ id: creada.id, created: false });
  expect(llamadas.filter((l) => l.metodo === 'POST')).toHaveLength(1);
});

test('sin nombre se guarda con la dirección; sin dirección válida no se crea nada', async () => {
  const llamadas = servicio([]);
  await post({ name: '  ', email: 'ana@example.eu' });
  expect(llamadas.find((l) => l.metodo === 'POST')?.cuerpo).toMatchObject({
    formatted_name: 'ana@example.eu',
  });

  const malas = servicio([]);
  expect((await post({ name: 'Ana', email: 'no es un correo' })).status).toBe(400);
  expect(malas).toEqual([]);
});

test('si Contacts no contesta, 502 y no un alta a medias', async () => {
  globalThis.fetch = (async () => new Response('', { status: 503 })) as unknown as typeof fetch;
  expect((await post({ name: 'Ana', email: 'ana@example.eu' })).status).toBe(502);
});

test('a quién se ofrece: ni Spam, ni no-reply, ni uno mismo', () => {
  const ctx = { enSpam: false, propias: ['yo@sytue.eu'] };
  expect(ofrecerContacto('ana@example.eu', ctx)).toBe(true);
  expect(ofrecerContacto('ana@example.eu', { ...ctx, enSpam: true })).toBe(false);
  expect(ofrecerContacto('YO@sytue.eu', ctx)).toBe(false);
  expect(ofrecerContacto('', ctx)).toBe(false);
  for (const c of [
    'no-reply@banco.eu',
    'noreply@x.eu',
    'No_Reply@x.eu',
    'do-not-reply@x.eu',
    'donotreply@x.eu',
    'noreply-avisos@x.eu',
    'mailer-daemon@x.eu',
    'bounces+123@x.eu',
  ]) {
    expect(esSinRespuesta(c)).toBe(true);
  }
  for (const c of ['ana@x.eu', 'noelia@x.eu', 'replyall@x.eu'])
    expect(esSinRespuesta(c)).toBe(false);
  expect(contactoConCorreo([{ id: 'a', emails: [{ email: 'ana@x.eu' }] }], ' ANA@x.eu ')).toBe('a');
  expect(nombreDelContacto(undefined, 'ana@x.eu')).toBe('ana@x.eu');
});
