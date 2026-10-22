/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MIDDLEWARE DE ORGANIZACIÓN, probado con un contexto Astro de mentira.
 *
 * Lo que se guarda es el CONTRATO con el que las doce apps componen: org en la
 * URL se sirve por dentro (rewrite con la marca que hace idempotente el
 * middleware), org ausente con sesión se redirige a la canónica, y lo que no
 * se escopia pasa. Sin bucle y sin importar cómo reejecute el runtime.
 */
import { afterEach, describe, expect, it } from 'bun:test';
import { MARCA_ORG, returnToCanonico } from './organizacion.js';
import { middlewareDeOrganizacion, type SesionOrganizacion } from './astro-middleware.js';

/** Contexto mínimo: lo que el middleware toca es url, rewrite y locals. */
function contexto(pathname: string, _sesion: SesionOrganizacion) {
  const url = new URL(`https://rooms.sytue.eu${pathname}`);
  const locals: Record<string, unknown> = {};
  let reescrito: URL | null = null;
  const ctx = {
    url,
    locals,
    request: new Request(url),
    rewrite: (destino: URL) => {
      reescrito = destino;
      return Promise.resolve(new Response('reescrito'));
    },
  };
  return { ctx, locals, reescrito: () => reescrito };
}

const fabrica = (sesionActiva: string | null, sinOrg: string[] = []) =>
  middlewareDeOrganizacion({
    base: '/rooms',
    sesion: async () =>
      sesionActiva ? ({ user: { tenant: sesionActiva } } as SesionOrganizacion) : null,
    sinOrg,
  });

/** Ejecuta el middleware de verdad, con next() que devuelve una respuesta cualquiera. */
async function ejecutar(
  middleware: ReturnType<typeof fabrica>,
  pathname: string,
  sesion: SesionOrganizacion = null,
) {
  const c = contexto(pathname, sesion);
  const respuesta = await middleware(
    c.ctx as unknown as Parameters<typeof middleware>[0],
    async () => new Response('sirto'),
  );
  return { respuesta, ...c };
}

describe('middlewareDeOrganizacion', () => {
  it('una URL CON org se sirve por dentro y deja la org en locals', async () => {
    const { respuesta, reescrito } = await ejecutar(fabrica('acme'), '/rooms/acme/channels/x');
    expect(reescrito()?.pathname).toBe('/rooms/channels/x');
    expect(reescrito()?.searchParams.get(MARCA_ORG)).toBe('acme');
    expect((await respuesta).status).toBe(200);
  });

  it('la org queda en locals en la MISMA pasada que el rewrite (sin depender de la reejecución)', async () => {
    // Si el runtime NO reejecuta la cadena tras el rewrite, la rama de la marca
    // no llega a correr: que `locals.org` no dependa de eso es el contrato.
    const { locals, reescrito } = await ejecutar(fabrica('acme'), '/rooms/acme/channels/x');
    expect(reescrito()).not.toBeNull();
    expect(locals['org']).toBe('acme');
  });

  it('el rewrite NO borra la query de la petición', async () => {
    const { reescrito } = await ejecutar(fabrica('acme'), '/rooms/acme/?rooms=x&segundo=y');
    expect(reescrito()?.searchParams.get('rooms')).toBe('x');
    expect(reescrito()?.searchParams.get('segundo')).toBe('y');
  });

  it('la marca resuelve SIN mirar nada más: reejecutar no buclea', async () => {
    const { respuesta, reescrito } = await ejecutar(
      fabrica('acme'),
      `/rooms/channels/x?${MARCA_ORG}=acme`,
      'acme',
    );
    expect(reescrito()).toBeNull();
    expect((await respuesta).status).toBe(200);
  });

  it('una ruta de página declarada en sinOrg pasa SIN salto: el guard de página canariza', async () => {
    // «channels» es página de Rooms (slug válido): sin estar en sinOrg el
    // middleware la leería como org — por eso las apps declaran sus páginas.
    const { respuesta, reescrito } = await ejecutar(
      middlewareDeOrganizacion({
        base: '/rooms',
        sesion: async () => ({ user: { tenant: 'acme' } }),
        sinOrg: ['channels'],
      }),
      '/rooms/channels/x?id=7',
    );
    expect(reescrito()).toBeNull();
    expect((await respuesta).status).toBe(200);
  });

  it('un primer segmento sin forma de slug pasa sin sesión', async () => {
    const { respuesta, reescrito } = await ejecutar(fabrica(null), '/rooms/_sin-org/x');
    expect(reescrito()).toBeNull();
    expect((await respuesta).status).toBe(200);
  });

  it('la raíz del prefijo también se redirige con sesión', async () => {
    const { respuesta } = await ejecutar(fabrica('acme'), '/rooms/', 'acme');
    expect((await respuesta).headers.get('location')).toBe('https://rooms.sytue.eu/rooms/acme/');
  });

  it('lo que no se escopia pasa siempre, con o sin sesión', async () => {
    for (const ruta of ['/rooms/api/channels', '/rooms/auth/login', '/rooms/healthz']) {
      const { respuesta, reescrito } = await ejecutar(fabrica('acme'), ruta, 'acme');
      expect(reescrito()).toBeNull();
      expect((await respuesta).status).toBe(200);
    }
  });

  it('lo que la app declara sin org (enlaces públicos) pasa igual', async () => {
    const { respuesta, reescrito } = await ejecutar(fabrica('acme', ['p']), '/rooms/p', 'acme');
    expect(reescrito()).toBeNull();
    expect((await respuesta).status).toBe(200);
  });

  it('lo que no cuelga del prefijo no se toca (la app en la raíz del host)', async () => {
    const { respuesta, reescrito } = await ejecutar(fabrica('acme'), '/', 'acme');
    expect(reescrito()).toBeNull();
    expect((await respuesta).status).toBe(200);
  });

  it('la org del path difiere de la de la sesión: NO se redirige a ciegas', async () => {
    const { respuesta, reescrito } = await ejecutar(
      fabrica('otra'),
      '/rooms/acme/channels/x',
      'otra',
    );
    expect(reescrito()?.pathname).toBe('/rooms/channels/x');
    expect((await respuesta).status).toBe(200);
  });

  it('SYTUE_ORG_PATH=off apaga el comportamiento entero', async () => {
    process.env['SYTUE_ORG_PATH'] = 'off';
    try {
      const { respuesta, reescrito } = await ejecutar(fabrica('acme'), '/rooms/channels/x', 'acme');
      expect(reescrito()).toBeNull();
      expect((await respuesta).status).toBe(200);
    } finally {
      delete process.env['SYTUE_ORG_PATH'];
    }
  });
});

afterEach(() => {
  delete process.env['SYTUE_ORG_PATH'];
});

describe('returnToCanonico', () => {
  it('una petición reescrita (con marca) da el returnTo canónico org-ful, sin marca', () => {
    expect(
      returnToCanonico(new URL('https://rooms.sytue.eu/rooms/channels/x?__org=acme'), '/rooms'),
    ).toBe('/rooms/acme/channels/x');
    expect(returnToCanonico(new URL('https://rooms.sytue.eu/rooms/?__org=acme'), '/rooms')).toBe(
      '/rooms/acme/',
    );
    // La query de verdad se conserva; solo se quita la marca.
    expect(
      returnToCanonico(new URL('https://docs.sytue.eu/docs/edit?id=7&__org=acme'), '/docs'),
    ).toBe('/docs/acme/edit?id=7');
  });

  it('sin marca, el returnTo es la petición tal cual', () => {
    expect(returnToCanonico(new URL('https://rooms.sytue.eu/rooms/'), '/rooms')).toBe('/rooms/');
    expect(returnToCanonico(new URL('https://rooms.sytue.eu/rooms/channels/x'), '/rooms')).toBe(
      '/rooms/channels/x',
    );
  });

  it('una marca falsa o fuera de la app no toca nada', () => {
    expect(returnToCanonico(new URL('https://rooms.sytue.eu/rooms/x?__org=NO'), '/rooms')).toBe(
      '/rooms/x?__org=NO',
    );
    expect(returnToCanonico(new URL('https://rooms.sytue.eu/healthz?__org=acme'), '/rooms')).toBe(
      '/healthz?__org=acme',
    );
  });

  it('doble marca: la PRIMERA gana y la compuesta no lleva NINGUNA', () => {
    expect(
      returnToCanonico(
        new URL('https://rooms.sytue.eu/rooms/channels/c1?__org=a&__org=b'),
        '/rooms',
      ),
    ).toBe('/rooms/a/channels/c1');
  });

  it('marca vacía: no acredita nada y no compone', () => {
    expect(
      returnToCanonico(new URL('https://rooms.sytue.eu/rooms/channels/x?__org='), '/rooms'),
    ).toBe('/rooms/channels/x?__org=');
  });

  it('el prefijo a secas (sin barra final) también se compone', () => {
    expect(returnToCanonico(new URL('https://rooms.sytue.eu/rooms?__org=acme'), '/rooms')).toBe(
      '/rooms/acme/',
    );
  });

  it('un prefijo que solo SOLAPA con el base no es la app: tal cual', () => {
    expect(
      returnToCanonico(new URL('https://rooms.sytue.eu/roomsies/x?__org=acme'), '/rooms'),
    ).toBe('/roomsies/x?__org=acme');
  });

  it('base vacía (consola de plataforma): NUNCA compone org, tal cual entró', () => {
    // La plataforma no escopia organizaciones: componer `/org/...` ahí sería un
    // 404 tras el login. Contrato nuevo de la base vacía.
    expect(returnToCanonico(new URL('https://sytue.eu/consola?__org=acme'), '')).toBe(
      '/consola?__org=acme',
    );
  });
});

// Astro NO acepta un array como `onRequest` (`handle is not a function`): se
// compone con `sequence` de `astro:middleware`. Con el array, la v3.7.7 dejó
// doce frontends dando 500 en /healthz y sin arrancar en producción.
describe('composición del middleware en las apps', () => {
  it('ninguna app exporta onRequest como array', async () => {
    const { readdirSync, readFileSync, existsSync } = await import('node:fs');
    const { join } = await import('node:path');
    const apps = join(import.meta.dir, '../../../apps');
    const conArray = readdirSync(apps)
      .map((app) => join(apps, app, 'src/middleware.ts'))
      .filter(
        (f) =>
          existsSync(f) && /export const onRequest\s*(:[^=]*)?=\s*\[/.test(readFileSync(f, 'utf8')),
      );
    expect(conArray).toEqual([]);
  });
});
