import { expect, test } from 'bun:test';
import type { SytueUser } from '@sytue/auth';
import { almacenamientoExternoDisponible } from './almacenamiento-externo.js';

const driveUrl = 'https://drive.internal/';
type StorageFetch = NonNullable<Parameters<typeof almacenamientoExternoDisponible>[3]>;
const available: StorageFetch = async () => Response.json({ available: true, connected: false });

test('la cuenta individual gratuita conserva el acceso aunque no pueda entrar en Admin', async () => {
  const individual: Pick<SytueUser, 'roles' | 'orgKind' | 'plan'> = {
    roles: ['owner'],
    orgKind: 'personal',
    plan: 'personal_free',
  };
  expect(await almacenamientoExternoDisponible(individual, 'token', driveUrl, available)).toBe(
    true,
  );
});

test('owner y admin de organización consultan Drive con el token de su sesión', async () => {
  for (const role of ['owner', 'admin'] as const) {
    const observed: { authorization: string | null } = { authorization: null };
    let requestedUrl = '';
    const fetcher: StorageFetch = async (input, init) => {
      requestedUrl = String(input);
      observed.authorization = new Headers(init?.headers).get('authorization');
      return Response.json({ available: true, connected: true });
    };
    expect(
      await almacenamientoExternoDisponible({ roles: [role] }, 'session-token', driveUrl, fetcher),
    ).toBe(true);
    expect(requestedUrl).toBe('https://drive.internal/api/v1/storage/external');
    expect(observed.authorization).toBe('Bearer session-token');
  }
});

test('un miembro no recibe el acceso aunque el despliegue permita conectar almacenamiento', async () => {
  let calls = 0;
  const fetcher: StorageFetch = async () => {
    calls += 1;
    return Response.json({ available: true });
  };
  expect(
    await almacenamientoExternoDisponible({ roles: ['member'] }, 'token', driveUrl, fetcher),
  ).toBe(false);
  expect(calls).toBe(0);
});

test('solo se ofrece una puerta que Drive confirma disponible y autorizada', async () => {
  for (const response of [
    Response.json({ available: false }),
    Response.json({}),
    Response.json({ available: 'true' }),
    Response.json({ available: true }, { status: 403 }),
    new Response(null, { status: 503 }),
    new Response('invalid JSON'),
  ]) {
    const fetcher: StorageFetch = async () => response;
    expect(
      await almacenamientoExternoDisponible({ roles: ['admin'] }, 'token', driveUrl, fetcher),
    ).toBe(false);
  }
  const offline: StorageFetch = async () => {
    throw new TypeError('offline');
  };
  expect(
    await almacenamientoExternoDisponible({ roles: ['owner'] }, 'token', driveUrl, offline),
  ).toBe(false);
});

test('OAuth conserva el acceso sin KEK de Drive sólo con un proveedor permitido y preparado', async () => {
  for (const provider of ['gdrive', 'onedrive', 'dropbox']) {
    const requests: string[] = [];
    const fetcher: StorageFetch = async (input, init) => {
      requests.push(String(input));
      expect(new Headers(init?.headers).get('authorization')).toBe('Bearer session-token');
      return String(input).endsWith('/instances')
        ? Response.json({ providers: { [provider]: true } })
        : Response.json({ available: false });
    };
    expect(
      await almacenamientoExternoDisponible(
        { roles: ['admin'] },
        'session-token',
        driveUrl,
        fetcher,
      ),
    ).toBe(true);
    expect(requests).toEqual([
      'https://drive.internal/api/v1/storage/external',
      'https://drive.internal/api/v1/storage/external/instances',
    ]);
  }
});

test('el fallback OAuth conserva bloqueos y rechaza capacidades desconocidas o inválidas', async () => {
  for (const body of [
    { providers: { gdrive: false, onedrive: false, dropbox: false } },
    { providers: { unsupported: true } },
    { providers: { gdrive: 'true' } },
    { providers: null },
    {},
  ]) {
    const fetcher: StorageFetch = async (input) =>
      String(input).endsWith('/instances')
        ? Response.json(body)
        : Response.json({ available: false });
    expect(
      await almacenamientoExternoDisponible({ roles: ['owner'] }, 'token', driveUrl, fetcher),
    ).toBe(false);
  }
});

test('Drive debe autorizar antes del fallback OAuth y la segunda lectura también debe responder bien', async () => {
  for (const status of [401, 403, 503]) {
    let calls = 0;
    const fetcher: StorageFetch = async () => {
      calls += 1;
      return Response.json({ available: false, providers: { gdrive: true } }, { status });
    };
    expect(
      await almacenamientoExternoDisponible({ roles: ['admin'] }, 'token', driveUrl, fetcher),
    ).toBe(false);
    expect(calls).toBe(1);
  }
  for (const response of [
    new Response(null, { status: 403 }),
    new Response(null, { status: 503 }),
    new Response('invalid JSON'),
  ]) {
    const fetcher: StorageFetch = async (input) =>
      String(input).endsWith('/instances') ? response : Response.json({ available: false });
    expect(
      await almacenamientoExternoDisponible({ roles: ['owner'] }, 'token', driveUrl, fetcher),
    ).toBe(false);
  }
});
