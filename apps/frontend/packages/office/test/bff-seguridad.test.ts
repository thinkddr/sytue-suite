/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pruebas de BYPASS del BFF de Docs (ROADMAP-OFFICE permisos-2): el recorrido navegador →
 * BFF → servicio, ruta por ruta de las vías de lectura y escritura. El BFF no decide quién
 * puede qué —lo hace el servicio, con su propia matriz (`matriz_acl_test.go` y
 * `permisos_integration_test.go`)—; su trabajo es no estorbar ni ampliar esa decisión:
 *
 *   - sin sesión, nada sale hacia el servicio;
 *   - hacia el servicio solo va el token DE LA SESIÓN: nunca una cabecera, una cookie o un
 *     identificador que ponga el navegador;
 *   - el «no» del servicio llega tal cual (403, 404, 409, 410…): nunca se convierte en 200,
 *     ni se reintenta por otra ruta, ni se cachea (la revocación en caliente se nota en la
 *     petición siguiente);
 *   - un identificador que no es un UUID no llega a formar una ruta (`..` pasa por
 *     encodeURIComponent y fetch lo normaliza);
 *   - lo que el servicio sirve (el estado, sin la contraseña de protección) sale verbatim.
 *
 * Ejecutar: cd apps/frontend/packages/office && bun test test/bff-seguridad.test.ts
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { afterEach, beforeEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

interface Sesion {
  tokens: { accessToken: string };
  user: { id: string };
}
let sesion: Sesion | null = null;

// El wiring real exige la configuración OIDC al cargarse; lo que se prueba es lo que hace
// cada ruta con la sesión, así que se sustituye por una sesión de prueba.
mock.module('../src/lib/auth', () => ({
  bff: { getSession: () => Promise.resolve(sesion), basePath: '/' },
  toBffContext: (ctx: APIContext) => ({
    request: ctx.request,
    cookies: ctx.cookies,
    url: new URL(ctx.request.url),
  }),
  docsBaseUrl: () => 'http://docs.test',
  driveBaseUrl: () => 'http://drive.test',
}));

const ops = await import('../src/pages/api/ops');
const state = await import('../src/pages/api/state');
const collab = await import('../src/pages/api/collab');
const exportar = await import('../src/pages/api/export');
const versions = await import('../src/pages/api/versions');
const comments = await import('../src/pages/api/comments');
const sharing = await import('../src/pages/api/sharing');
const politicaEnlaces = await import('../src/pages/api/link-policy');
const presence = await import('../src/pages/api/presence');
const desbloquear = await import('../src/pages/s/[token]/desbloquear');
const abrirEnlace = await import('../src/pages/s/[token]/abrir');
const rename = await import('../src/pages/api/rename');
const star = await import('../src/pages/api/star');
const create = await import('../src/pages/api/create');
const abrirDeDrive = await import('../src/pages/open-from-drive');
const ai = await (async () => {
  const anterior = process.env.SYTUE_AI_URL;
  process.env.SYTUE_AI_URL = 'http://ai.test';
  try {
    return await import('../src/pages/api/ai');
  } finally {
    if (anterior === undefined) delete process.env.SYTUE_AI_URL;
    else process.env.SYTUE_AI_URL = anterior;
  }
})();

describe('POST /api/ai: errores del proveedor', () => {
  const pedir = () =>
    (ai.POST as Handler)(
      ctxDe(
        'POST',
        '/api/ai',
        JSON.stringify({
          action: 'resumir',
          instruction: 'Resume',
          context: 'texto',
          kind: 'document',
        }),
      ),
    );

  test('conserva sólo el código de proveedor no configurado, sin detalles internos', async () => {
    respuesta = () =>
      new Response(
        JSON.stringify({
          error: {
            code: 'ai_not_configured',
            message: 'detalle privado',
            params: { key: 'privada' },
          },
          request_id: 'interno',
        }),
        { status: 503 },
      );
    const res = await pedir();
    expect(res.status).toBe(503);
    expect(await res.json()).toEqual({ error: 'ai_not_configured' });
    expect(res.headers.get('cache-control')).toBe('no-store');
    expect(llamadas).toHaveLength(1);
    expect(llamadas[0]!.url).toBe('http://ai.test/api/v1/assist');
    expect(llamadas[0]!.headers.get('authorization')).toBe(`Bearer ${TOKEN}`);
  });

  test.each([
    [500, '{"error":{"code":"ai_not_configured","message":"privado"}}'],
    [503, '{"error":{"code":"otro_error","message":"privado"}}'],
    [503, '<html>detalle privado</html>'],
    [503, '{malformado'],
    [503, '{"error":"ai_not_configured"}'],
  ])('no propaga otros errores o cuerpos: %i %s', async (status, body) => {
    respuesta = () => new Response(body, { status });
    const res = await pedir();
    expect(res.status).toBe(502);
    expect(await res.json()).toEqual({ error: 'ai_failed' });
  });

  test('la falta de sesión no alcanza el proveedor', async () => {
    sesion = null;
    const res = await pedir();
    expect(res.status).toBe(401);
    expect(llamadas).toHaveLength(0);
  });

  test('un cuerpo 503 interrumpido mantiene el error genérico', async () => {
    respuesta = () =>
      new Response(
        new ReadableStream({
          start(controller) {
            controller.error(new Error('detalle privado del transporte'));
          },
        }),
        { status: 503 },
      );
    const res = await pedir();
    expect(res.status).toBe(502);
    expect(await res.json()).toEqual({ error: 'ai_failed' });
  });

  test('conserva la respuesta de asistencia correcta', async () => {
    respuesta = () => new Response('{"text":"resultado"}');
    const res = await pedir();
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ text: 'resultado' });
  });
});

const ID = '3f2b6d1c-8a4e-4c1b-9d2e-5a6b7c8d9e0f';
const OTRO = '9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d';
const TOKEN = 'tok-de-la-sesion';

interface Llamada {
  url: string;
  method: string;
  headers: Headers;
  body: unknown;
}
let llamadas: Llamada[] = [];
let respuesta: (l: Llamada) => Response = () => new Response('{}', { status: 200 });
const fetchReal = globalThis.fetch;

beforeEach(() => {
  sesion = { tokens: { accessToken: TOKEN }, user: { id: 'ana' } };
  llamadas = [];
  respuesta = () => new Response('{}', { status: 200 });
  globalThis.fetch = ((input: string | URL | Request, init?: RequestInit) => {
    const l: Llamada = {
      url: String(input),
      method: init?.method ?? 'GET',
      headers: new Headers(init?.headers),
      body: init?.body ?? null,
    };
    llamadas.push(l);
    return Promise.resolve(respuesta(l));
  }) as unknown as typeof fetch;
});
afterEach(() => {
  globalThis.fetch = fetchReal;
});

type Handler = (ctx: APIContext) => Response | Promise<Response>;

/** Una petición del navegador: con sus cabeceras, que el BFF no debe reenviar. */
function ctxDe(metodo: string, ruta: string, cuerpo?: string): APIContext {
  const url = new URL(`https://docs.sytue.eu${ruta}`);
  const request = new Request(url, {
    method: metodo,
    headers: {
      authorization: 'Bearer token-del-navegador',
      cookie: 'sytue_session=galleta-del-navegador',
      'x-forwarded-user': 'admin',
      'x-sytue-sub': 'owner',
      'content-type': 'application/json',
    },
    ...(cuerpo !== undefined ? { body: cuerpo } : {}),
  });
  return { request, url, cookies: {}, locals: { locale: 'es' } } as unknown as APIContext;
}

interface Via {
  nombre: string;
  handler: Handler;
  metodo: string;
  ruta: string;
  cuerpo?: string;
  /** Lo que llega al servicio (método y ruta bajo /api/v1). */
  sube: [string, RegExp];
  /** Lo que escribe (no es una lectura). */
  escribe: boolean;
  /** El identificador va por la query o por el cuerpo, para probar ids hostiles. */
  conId: 'query' | 'cuerpo' | null;
}

const OPS_BODY = JSON.stringify({
  ops: [{ kind: 'set', dot: { a: 'c', c: 1 }, key: 'doc:sheet', value: '"x"', lamport: 1 }],
  proteccion: { password: 'clave-de-proteccion' },
});

const VIAS: Via[] = [
  {
    nombre: 'POST /api/ops',
    handler: ops.POST!,
    metodo: 'POST',
    ruta: `/api/ops?id=${ID}`,
    cuerpo: OPS_BODY,
    sube: ['POST', new RegExp(`^/documents/${ID}/ops$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'GET /api/ops',
    handler: ops.GET!,
    metodo: 'GET',
    ruta: `/api/ops?id=${ID}&since=0&limit=50`,
    sube: ['GET', new RegExp(`^/documents/${ID}/ops\\?since=0&limit=50$`)],
    escribe: false,
    conId: 'query',
  },
  {
    nombre: 'GET /api/state',
    handler: state.GET!,
    metodo: 'GET',
    ruta: `/api/state?id=${ID}`,
    sube: ['GET', new RegExp(`^/documents/${ID}/state$`)],
    escribe: false,
    conId: 'query',
  },
  {
    nombre: 'GET /api/versions',
    handler: versions.GET!,
    metodo: 'GET',
    ruta: `/api/versions?id=${ID}`,
    sube: ['GET', new RegExp(`^/documents/${ID}/versions\\?limit=50$`)],
    escribe: false,
    conId: 'query',
  },
  {
    nombre: 'POST /api/versions (crear)',
    handler: versions.POST!,
    metodo: 'POST',
    ruta: `/api/versions?id=${ID}`,
    cuerpo: '{}',
    sube: ['POST', new RegExp(`^/documents/${ID}/versions$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'POST /api/versions (restaurar)',
    handler: versions.POST!,
    metodo: 'POST',
    ruta: `/api/versions?id=${ID}&restore=${OTRO}`,
    cuerpo: '{"proteccion":{"password":"clave-de-proteccion"}}',
    sube: ['POST', new RegExp(`^/documents/${ID}/versions/${OTRO}/restore$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'GET /api/comments',
    handler: comments.GET!,
    metodo: 'GET',
    ruta: `/api/comments?id=${ID}`,
    sube: ['GET', new RegExp(`^/documents/${ID}/comments$`)],
    escribe: false,
    conId: 'query',
  },
  {
    nombre: 'POST /api/comments',
    handler: comments.POST!,
    metodo: 'POST',
    ruta: `/api/comments?id=${ID}`,
    cuerpo: '{"body":"hola"}',
    sube: ['POST', new RegExp(`^/documents/${ID}/comments$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'PUT /api/comments (resolver)',
    handler: comments.PUT!,
    metodo: 'PUT',
    ruta: `/api/comments?id=${ID}&commentId=${OTRO}`,
    cuerpo: '{"resolved":true}',
    sube: ['PUT', new RegExp(`^/documents/${ID}/comments/${OTRO}/resolve$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'DELETE /api/comments',
    handler: comments.DELETE!,
    metodo: 'DELETE',
    ruta: `/api/comments?id=${ID}&commentId=${OTRO}`,
    sube: ['DELETE', new RegExp(`^/documents/${ID}/comments/${OTRO}$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'POST /api/sharing (crear enlace)',
    handler: sharing.POST!,
    metodo: 'POST',
    ruta: `/api/sharing?id=${ID}`,
    cuerpo: '{"role":"viewer"}',
    sube: ['POST', new RegExp(`^/documents/${ID}/shares$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'DELETE /api/sharing (revocar enlace)',
    handler: sharing.DELETE!,
    metodo: 'DELETE',
    ruta: `/api/sharing?id=${ID}&linkId=${OTRO}`,
    sube: ['DELETE', new RegExp(`^/documents/${ID}/shares/${OTRO}$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'PATCH /api/sharing (contraseña de un enlace)',
    handler: sharing.PATCH!,
    metodo: 'PATCH',
    ruta: `/api/sharing?id=${ID}&linkId=${OTRO}`,
    cuerpo: '{"password":"una-clave-larga"}',
    sube: ['PUT', new RegExp(`^/documents/${ID}/shares/${OTRO}/password$`)],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'GET /api/link-policy (techo de caducidad)',
    handler: politicaEnlaces.GET!,
    metodo: 'GET',
    ruta: '/api/link-policy',
    sube: ['GET', /^\/link-policy$/],
    escribe: false,
    conId: null,
  },
  {
    nombre: 'PUT /api/link-policy (cambiar el techo)',
    handler: politicaEnlaces.PUT!,
    metodo: 'PUT',
    ruta: '/api/link-policy',
    cuerpo: '{"max_days":0}',
    sube: ['PUT', /^\/link-policy$/],
    escribe: true,
    conId: null,
  },
  {
    nombre: 'DELETE /api/sharing (revocar permiso)',
    handler: sharing.DELETE!,
    metodo: 'DELETE',
    ruta: `/api/sharing?id=${ID}&principalKind=user&principalId=ana`,
    sube: [
      'DELETE',
      new RegExp(`^/documents/${ID}/permissions\\?principal_kind=user&principal_id=ana$`),
    ],
    escribe: true,
    conId: 'query',
  },
  {
    nombre: 'POST /api/rename',
    handler: rename.POST!,
    metodo: 'POST',
    ruta: '/api/rename',
    cuerpo: JSON.stringify({ id: ID, title: 'Nuevo' }),
    sube: ['PATCH', new RegExp(`^/documents/${ID}$`)],
    escribe: true,
    conId: 'cuerpo',
  },
  {
    nombre: 'POST /api/star',
    handler: star.POST!,
    metodo: 'POST',
    ruta: '/api/star',
    cuerpo: JSON.stringify({ id: ID, starred: true }),
    sube: ['PUT', new RegExp(`^/documents/${ID}/star$`)],
    escribe: true,
    conId: 'cuerpo',
  },
];

const hostiles = ['..', '../x', 'x/y', '%2e%2e', 'not-a-uuid', `${ID}/..`, `${ID}?x=1`, ''];

describe('sin sesión no sale nada hacia el servicio', () => {
  for (const v of VIAS) {
    test(v.nombre, async () => {
      sesion = null;
      const res = await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo));
      expect(res.status).toBe(401);
      expect(llamadas).toHaveLength(0);
    });
  }
  test('GET /api/collab, POST /api/export y POST /api/create', async () => {
    sesion = null;
    expect((await collab.GET!(ctxDe('GET', `/api/collab?id=${ID}`))).status).toBe(401);
    expect(
      (await exportar.POST!(ctxDe('POST', `/api/export?id=${ID}`, '{"format":"pdf"}'))).status,
    ).toBe(401);
    expect((await create.POST!(ctxDe('POST', '/api/create', '{"kind":"document"}'))).status).toBe(
      401,
    );
    expect(llamadas).toHaveLength(0);
  });
});

describe('hacia el servicio solo va el token de la sesión', () => {
  for (const v of VIAS) {
    test(v.nombre, async () => {
      const res = await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo));
      expect(res.status).toBe(200);
      expect(llamadas).toHaveLength(1);
      const l = llamadas[0]!;
      expect(l.method).toBe(v.sube[0]);
      expect(l.url.startsWith('http://docs.test/api/v1')).toBe(true);
      expect(l.url.slice('http://docs.test/api/v1'.length)).toMatch(v.sube[1]);
      expect(l.headers.get('authorization')).toBe(`Bearer ${TOKEN}`);
      // Nada de lo que puso el navegador cruza: ni su cookie ni identidades inventadas.
      expect(l.headers.get('cookie')).toBeNull();
      expect(l.headers.get('x-forwarded-user')).toBeNull();
      expect(l.headers.get('x-sytue-sub')).toBeNull();
    });
  }
  test('POST /api/export y POST /api/create', async () => {
    respuesta = (l) =>
      l.url.endsWith('/exports')
        ? new Response(
            JSON.stringify({
              export: { id: OTRO, format: 'pdf', content_type: 'application/pdf', size_bytes: 3 },
            }),
            { status: 201, headers: { 'content-type': 'application/json' } },
          )
        : l.url.includes('/exports/')
          ? new Response('PDF', { status: 200, headers: { 'content-type': 'application/pdf' } })
          : new Response(JSON.stringify({ document: { id: ID, title: 'T', kind: 'document' } }), {
              status: 201,
              headers: { 'content-type': 'application/json' },
            });
    const e = await exportar.POST!(
      ctxDe('POST', `/api/export?id=${ID}&filename=x`, '{"format":"pdf"}'),
    );
    expect(e.status).toBe(200);
    expect(await e.text()).toBe('PDF');
    const c = await create.POST!(ctxDe('POST', '/api/create', '{"kind":"document"}'));
    expect(c.status).toBe(201);
    expect(llamadas.length).toBeGreaterThanOrEqual(3);
    for (const l of llamadas) {
      expect(l.headers.get('authorization')).toBe(`Bearer ${TOKEN}`);
      expect(l.headers.get('cookie')).toBeNull();
    }
  });
  test('GET /api/collab compone la URL con el token de la sesión y el id validado', async () => {
    const res = await collab.GET!(ctxDe('GET', `/api/collab?id=${ID}`));
    expect(res.status).toBe(200);
    const { url } = (await res.json()) as { url: string };
    expect(url).toContain(`/documents/${ID}/ws`);
    expect(url).toContain(`access_token=${TOKEN}`);
    expect(url).not.toContain('token-del-navegador');
    expect(llamadas).toHaveLength(0);
  });
});

describe('el no del servicio llega tal cual y no se cachea', () => {
  const noS = [
    // El techo de caducidad de un enlace lo impone el servicio: su 400 no se maquilla.
    [400, '{"error":{"code":"share_expiry_too_far"}}'],
    [401, '{"error":{"code":"unauthorized"}}'],
    [403, '{"error":{"code":"forbidden"}}'],
    [403, '{"error":{"code":"protected"}}'],
    [403, '{"error":{"code":"protection_password"}}'],
    [404, '{"error":{"code":"not_found"}}'],
    [409, '{"error":{"code":"stale_seq"}}'],
    [410, '{"error":{"code":"share_expired"}}'],
    [429, '{"error":{"code":"rate_limited"}}'],
  ] as const;
  for (const v of VIAS) {
    test(v.nombre, async () => {
      for (const [status, cuerpo] of noS) {
        respuesta = () => new Response(cuerpo, { status });
        const res = await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo));
        expect(res.status).toBe(status);
        expect(await res.text()).toBe(cuerpo);
        expect(res.headers.get('cache-control')).toBe('no-store');
      }
      // Un servicio caído no es un 200 ni un 403 inventado.
      globalThis.fetch = (() =>
        Promise.reject(new Error('ECONNREFUSED'))) as unknown as typeof fetch;
      expect((await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo))).status).toBe(502);
    });
  }
  test('export y colaboración: un 403 del servicio no entrega ni un byte', async () => {
    respuesta = () => new Response('{"error":{"code":"forbidden"}}', { status: 403 });
    const res = await exportar.POST!(ctxDe('POST', `/api/export?id=${ID}`, '{"format":"ooxml"}'));
    expect(res.status).not.toBe(200);
    expect(res.headers.get('content-disposition')).toBeNull();
    expect(await res.text()).not.toContain('forbidden"}}PK');
    expect(llamadas).toHaveLength(1); // no pasa al paso 2 (la descarga) sin el 1 (la creación)
  });
  test('la revocación en caliente se nota en la petición siguiente', async () => {
    const v = VIAS[0]!;
    let permitido = true;
    respuesta = () =>
      permitido
        ? new Response('{"seq":1}', { status: 200 })
        : new Response('{"error":{"code":"forbidden"}}', { status: 403 });
    expect((await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo))).status).toBe(200);
    permitido = false; // el propietario le quita el permiso
    expect((await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo))).status).toBe(403);
    expect(llamadas).toHaveLength(2); // el BFF no recuerda el permiso: pregunta cada vez
    // Y una sesión que caduca corta de raíz, sin tocar el servicio.
    sesion = null;
    expect((await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo))).status).toBe(401);
    expect(llamadas).toHaveLength(2);
  });
  test('quien solo lee o comenta: el 403 del servicio en cada vía de escritura', async () => {
    respuesta = () => new Response('{"error":{"code":"forbidden"}}', { status: 403 });
    for (const v of VIAS.filter((x) => x.escribe)) {
      const res = await v.handler(ctxDe(v.metodo, v.ruta, v.cuerpo));
      expect([v.nombre, res.status]).toEqual([v.nombre, 403]);
    }
  });
});

describe('un identificador que no es un UUID no forma una ruta', () => {
  for (const v of VIAS.filter((x) => x.conId !== null)) {
    test(v.nombre, async () => {
      for (const malo of hostiles) {
        let ruta = v.ruta;
        let cuerpo = v.cuerpo;
        if (v.conId === 'query') ruta = ruta.replace(`id=${ID}`, `id=${encodeURIComponent(malo)}`);
        else cuerpo = (cuerpo ?? '').replace(ID, malo);
        const res = await v.handler(ctxDe(v.metodo, ruta, cuerpo));
        expect([v.nombre, malo, res.status]).toEqual([v.nombre, malo, 400]);
      }
      expect(llamadas).toHaveLength(0);
    });
  }
  test('commentId, restore y linkId', async () => {
    const piezas: [Handler, string, string, string | undefined][] = [
      [comments.PUT!, 'PUT', `/api/comments?id=${ID}&commentId=`, '{}'],
      [comments.DELETE!, 'DELETE', `/api/comments?id=${ID}&commentId=`, undefined],
      [versions.POST!, 'POST', `/api/versions?id=${ID}&restore=`, '{}'],
      [sharing.DELETE!, 'DELETE', `/api/sharing?id=${ID}&linkId=`, undefined],
    ];
    for (const [h, m, base, cuerpo] of piezas)
      for (const malo of ['..', '../..', '%2e%2e', 'x/y']) {
        const res = await h(ctxDe(m, base + encodeURIComponent(malo), cuerpo));
        expect([m, base, malo, res.status]).toEqual([m, base, malo, 400]);
      }
    expect(llamadas).toHaveLength(0);
  });
  test('colaboración y exportación', async () => {
    for (const malo of hostiles) {
      expect(
        (await collab.GET!(ctxDe('GET', `/api/collab?id=${encodeURIComponent(malo)}`))).status,
      ).toBe(400);
      expect(
        (
          await exportar.POST!(
            ctxDe('POST', `/api/export?id=${encodeURIComponent(malo)}`, '{"format":"pdf"}'),
          )
        ).status,
      ).toBe(400);
    }
    expect(llamadas).toHaveLength(0);
  });
});

describe('lo que viaja y lo que vuelve, verbatim', () => {
  test('POST /api/ops reenvía el cuerpo tal cual, con el sobre de la contraseña', async () => {
    await ops.POST!(ctxDe('POST', `/api/ops?id=${ID}`, OPS_BODY));
    expect(llamadas[0]!.body).toBe(OPS_BODY);
  });
  test('GET /api/state devuelve lo que el servicio sirve, sin añadir nada', async () => {
    // El servicio ya redactó la contraseña: lo que sale del BFF es eso, byte a byte.
    const servido =
      '{"state":{"registers":{"doc:sheet":{"value":"{\\"sheets\\":[{\\"protect\\":{\\"hasPassword\\":true}}]}"}}},"seq":7}';
    respuesta = () => new Response(servido, { status: 200 });
    const res = await state.GET!(ctxDe('GET', `/api/state?id=${ID}`));
    expect(await res.text()).toBe(servido);
    expect(res.headers.get('cache-control')).toBe('no-store');
  });
  test('GET /api/ops acota since y limit, y rechaza lo que no es un número', async () => {
    expect((await ops.GET!(ctxDe('GET', `/api/ops?id=${ID}&since=abc`))).status).toBe(400);
    await ops.GET!(ctxDe('GET', `/api/ops?id=${ID}&since=-5&limit=999999`));
    expect(llamadas[0]!.url).toContain('since=0&limit=500');
  });
  test('el título y el estado favorito se reescriben: el cuerpo del navegador no se reenvía entero', async () => {
    await rename.POST!(
      ctxDe(
        'POST',
        '/api/rename',
        JSON.stringify({ id: ID, title: 'T', owner_sub: 'otro', role: 'owner' }),
      ),
    );
    expect(JSON.parse(String(llamadas[0]!.body))).toEqual({ title: 'T' });
  });
});

describe('compartir: el navegador no elige a quién se concede', () => {
  test('un permiso a una persona sin su dirección no llega al servicio', async () => {
    // Sin «@» no se resuelve ninguna persona: el `sub` elegido por el cliente no se admite.
    const res = await sharing.PUT!(
      ctxDe(
        'PUT',
        `/api/sharing?id=${ID}`,
        JSON.stringify({ principal_kind: 'user', principal_id: 'owner', role: 'editor' }),
      ),
    );
    expect(res.status).toBe(404);
    expect(llamadas).toHaveLength(0);
  });
});

describe('presencia: quién está en un documento solo lo sabe quien puede verlo', () => {
  const latido = (conn: string, extra: Record<string, unknown> = {}): APIContext =>
    ctxDe('PUT', `/api/presence?id=${ID}`, JSON.stringify({ conn, ...extra }));

  test('sin sesión no se anuncia ni se pregunta', async () => {
    sesion = null;
    expect((await presence.PUT!(latido('c1'))).status).toBe(401);
    expect(llamadas).toHaveLength(0);
  });
  test('la presencia la autoriza el servicio, con el token de la sesión y solo `conn`', async () => {
    respuesta = () => new Response('{"members":[{"conn":"c1","sub":"ana"}]}', { status: 200 });
    const res = await presence.PUT!(latido('c1', { sub: 'owner', role: 'owner' }));
    expect(res.status).toBe(200);
    expect(llamadas).toHaveLength(1);
    const l = llamadas[0]!;
    expect(l.method).toBe('PUT');
    expect(l.url).toBe(`http://docs.test/api/v1/documents/${ID}/presence`);
    expect(l.headers.get('authorization')).toBe(`Bearer ${TOKEN}`);
    expect(JSON.parse(String(l.body))).toEqual({ conn: 'c1' });
  });
  test('quien no tiene acceso no ve a los presentes ni se anuncia: 404, exista o no', async () => {
    // Ana, con acceso, tiene el documento abierto.
    respuesta = () => new Response('{"members":[{"conn":"c1","sub":"ana"}]}', { status: 200 });
    expect((await presence.PUT!(latido('c1'))).status).toBe(200);
    // Otra sesión, sin acceso a ese documento (403) o de otra organización (404).
    sesion = { tokens: { accessToken: 'tok-de-otra-persona' }, user: { id: 'intrusa' } };
    for (const status of [403, 404]) {
      respuesta = () => new Response('{"error":{"code":"x"}}', { status });
      const res = await presence.PUT!(latido('c2'));
      expect(res.status).toBe(404);
      const cuerpo = await res.text();
      expect(cuerpo).not.toContain('members');
      expect(cuerpo).not.toContain('ana');
    }
    // Y siempre preguntó al servicio: el BFF no guarda presencia por su cuenta.
    expect(llamadas).toHaveLength(3);
    expect(llamadas[2]!.headers.get('authorization')).toBe('Bearer tok-de-otra-persona');
  });
  test('un id que no es un UUID o un conn inválido no llegan al servicio', async () => {
    for (const id of hostiles) {
      const res = await presence.PUT!(
        ctxDe('PUT', `/api/presence?id=${encodeURIComponent(id)}`, '{"conn":"c1"}'),
      );
      expect(res.status).toBe(400);
    }
    expect((await presence.PUT!(latido(''))).status).toBe(400);
    expect((await presence.PUT!(latido('x'.repeat(65)))).status).toBe(400);
    expect(llamadas).toHaveLength(0);
  });
});

describe('contraseña de un enlace: el BFF la transporta, no la decide ni la guarda', () => {
  const TK = 'a'.repeat(64);
  interface Galleta {
    nombre: string;
    valor: string;
    opciones: Record<string, unknown>;
  }
  /** Una petición a `/s/{token}/…` de alguien sin sesión, con sus cookies. */
  function ctxPublico(
    ruta: string,
    campos: Record<string, string> | null,
    cookies: Record<string, string> = {},
  ): { ctx: APIContext; puestas: Galleta[] } {
    const puestas: Galleta[] = [];
    const url = new URL(`https://docs.sytue.eu${ruta}`);
    const body = new URLSearchParams(campos ?? {});
    const request = new Request(url, {
      method: 'POST',
      headers: {
        'content-type': 'application/x-www-form-urlencoded',
        'x-forwarded-for': '198.51.100.7',
      },
      body,
    });
    const ctx = {
      request,
      url,
      params: { token: TK },
      locals: { locale: 'es' },
      cookies: {
        get: (n: string) => (n in cookies ? { value: cookies[n] } : undefined),
        set: (nombre: string, valor: string, opciones: Record<string, unknown>) =>
          puestas.push({ nombre, valor, opciones }),
      },
      redirect: (a: string, status: number) =>
        new Response(null, { status, headers: { location: a } }),
    } as unknown as APIContext;
    return { ctx, puestas };
  }

  test('acertar guarda la prueba en una cookie httpOnly de esa dirección; el token no va en la ruta', async () => {
    respuesta = () => new Response('{"proof":"PRUEBA"}', { status: 200 });
    const { ctx, puestas } = ctxPublico(`/s/${TK}/desbloquear`, { password: 'la-clave-buena' });
    const res = await desbloquear.POST!(ctx);
    expect(res.status).toBe(303);
    expect(res.headers.get('location')).toMatch(new RegExp(`/s/${TK}$`));
    expect(llamadas).toHaveLength(1);
    const l = llamadas[0]!;
    expect(l.url).toBe('http://docs.test/api/v1/share/unlock');
    expect(JSON.parse(String(l.body))).toEqual({ token: TK, password: 'la-clave-buena' });
    expect(l.headers.get('x-forwarded-for')).toBe('198.51.100.7');
    expect(l.headers.get('authorization')).toBeNull();
    expect(puestas).toHaveLength(1);
    const g = puestas[0]!;
    expect(g.valor).toBe('PRUEBA');
    expect(g.opciones.httpOnly).toBe(true);
    expect(String(g.opciones.path)).toMatch(new RegExp(`/s/${TK}$`));
    // Ni el token ni la contraseña en el nombre de la cookie.
    expect(g.nombre).not.toContain(TK);
    expect(g.nombre).not.toContain('la-clave-buena');
  });
  test('fallar no deja cookie ni distingue por qué: el mismo regreso para 403, 404, 410 y 500', async () => {
    for (const status of [403, 404, 410, 500]) {
      respuesta = () => new Response('{"error":{"code":"share_password_wrong"}}', { status });
      const { ctx, puestas } = ctxPublico(`/s/${TK}/desbloquear`, { password: 'no-es-la-clave' });
      const res = await desbloquear.POST!(ctx);
      expect(res.status).toBe(303);
      expect(res.headers.get('location')).toMatch(/\?clave=mal$/);
      expect(res.headers.get('location')).not.toContain('no-es-la-clave');
      expect(puestas).toHaveLength(0);
    }
    // Un 200 sin prueba tampoco entra.
    respuesta = () => new Response('{}', { status: 200 });
    const { ctx, puestas } = ctxPublico(`/s/${TK}/desbloquear`, { password: 'no-es-la-clave' });
    expect((await desbloquear.POST!(ctx)).headers.get('location')).toMatch(/\?clave=mal$/);
    expect(puestas).toHaveLength(0);
  });
  test('el bloqueo por intentos del servicio llega como «espera», sin reintentar', async () => {
    respuesta = () => new Response('{"error":{"code":"too_many_requests"}}', { status: 429 });
    const { ctx, puestas } = ctxPublico(`/s/${TK}/desbloquear`, { password: 'la-clave-buena' });
    const res = await desbloquear.POST!(ctx);
    expect(res.headers.get('location')).toMatch(/\?clave=espera$/);
    expect(puestas).toHaveLength(0);
    expect(llamadas).toHaveLength(1);
  });
  test('sin contraseña, o desmesurada, no se llama al servicio', async () => {
    for (const campos of [null, { password: '' }, { password: 'x'.repeat(2000) }]) {
      const { ctx } = ctxPublico(`/s/${TK}/desbloquear`, campos);
      expect((await desbloquear.POST!(ctx)).headers.get('location')).toMatch(/\?clave=mal$/);
    }
    expect(llamadas).toHaveLength(0);
  });
  test('canjear presenta la prueba de la cookie, y sin ella el 401 del servicio no da el documento', async () => {
    sesion = {
      tokens: { accessToken: TOKEN },
      user: { id: 'ana', tenant: 'acme' },
    } as unknown as Sesion;
    // Sin haber dado la contraseña: el servicio dice 401 y se vuelve a la vista.
    respuesta = () => new Response('{"error":{"code":"share_password_required"}}', { status: 401 });
    let res = await abrirEnlace.POST!(ctxPublico(`/s/${TK}/abrir`, null).ctx);
    expect(res.headers.get('location')).toMatch(new RegExp(`/s/${TK}$`));
    expect(JSON.parse(String(llamadas[0]!.body))).toEqual({ token: TK, proof: '' });
    // Con la cookie que dejó el desbloqueo.
    respuesta = () => new Response('{"proof":"PRUEBA"}', { status: 200 });
    const { ctx, puestas } = ctxPublico(`/s/${TK}/desbloquear`, { password: 'la-clave-buena' });
    await desbloquear.POST!(ctx);
    respuesta = () => new Response(`{"document_id":"${ID}","role":"editor"}`, { status: 200 });
    res = await abrirEnlace.POST!(
      ctxPublico(`/s/${TK}/abrir`, null, { [puestas[0]!.nombre]: 'PRUEBA' }).ctx,
    );
    expect(JSON.parse(String(llamadas[2]!.body))).toEqual({ token: TK, proof: 'PRUEBA' });
    expect(res.headers.get('location')).toContain(`/edit?id=${ID}`);
  });
  test('cambiar la contraseña: solo viaja `password`, y lo que no es texto no sale', async () => {
    respuesta = () => new Response(null, { status: 204 });
    const res = await sharing.PATCH!(
      ctxDe(
        'PATCH',
        `/api/sharing?id=${ID}&linkId=${OTRO}`,
        JSON.stringify({ password: 'una-clave-larga', role: 'owner', token: 'x' }),
      ),
    );
    expect(res.status).toBe(204);
    expect(JSON.parse(String(llamadas[0]!.body))).toEqual({ password: 'una-clave-larga' });
    for (const cuerpo of ['{}', '{"password":123}', 'no-json']) {
      const r = await sharing.PATCH!(
        ctxDe('PATCH', `/api/sharing?id=${ID}&linkId=${OTRO}`, cuerpo),
      );
      expect(r.status).toBe(400);
    }
    const sinEnlace = await sharing.PATCH!(
      ctxDe('PATCH', `/api/sharing?id=${ID}&linkId=..`, '{"password":"una-clave-larga"}'),
    );
    expect(sinEnlace.status).toBe(400);
    expect(llamadas).toHaveLength(1);
  });
});

/* FE-02: abrir un fichero de Drive CREA un documento, y es un GET al que se llega con un
 * enlace. La cookie de sesión es SameSite=Lax y viaja en esa navegación aunque el enlace
 * esté en otra web: desde otro sitio no se hace nada. */
describe('abrir desde Drive', () => {
  const navegacion = (sitio: string): APIContext => {
    const url = new URL(`https://docs.sytue.eu/open-from-drive?file=${ID}`);
    const request = new Request(url, { headers: { 'sec-fetch-site': sitio } });
    const redirect = (destino: string) =>
      new Response(null, { status: 302, headers: { location: destino } });
    return {
      request,
      url,
      cookies: {},
      locals: { locale: 'es' },
      redirect,
    } as unknown as APIContext;
  };

  test('desde otra web no llega al servicio', async () => {
    const res = await (abrirDeDrive.GET as Handler)(navegacion('cross-site'));
    expect(res.status).toBe(302);
    expect(res.headers.get('location')).toContain('error=open_failed');
    expect(llamadas).toHaveLength(0);
  });

  test('desde Drive (el mismo sitio) o desde una app nativa, sí', async () => {
    for (const sitio of ['same-site', 'none']) {
      llamadas = [];
      await (abrirDeDrive.GET as Handler)(navegacion(sitio));
      expect(llamadas.length, sitio).toBeGreaterThan(0);
    }
  });

  /* Decisión del dueño (2026-10-16): un libro que no cabe entero se abre recortado, y esa
   * copia NO se ancla al fichero (sin `source_ref` no hay «Guardar en Drive», y guardar no
   * puede sobrescribir el original con la copia recortada). El que cabe se ancla como siempre. */
  test('un libro recortado se abre como documento aparte, sin anclarse al fichero', async () => {
    const fichero = {
      id: ID,
      folder_id: OTRO,
      name: 'ventas.xlsx',
      path: '/ventas.xlsx',
      size_bytes: 3,
      content_type: 'application/octet-stream',
      version: 1,
      created_at: '2026-10-16T00:00:00Z',
      updated_at: '2026-10-16T00:00:00Z',
    };
    const abrirCon = async (truncated: boolean) => {
      llamadas = [];
      respuesta = (l) => {
        const json = (cuerpo: unknown, status = 200) =>
          new Response(JSON.stringify(cuerpo), {
            status,
            headers: { 'content-type': 'application/json' },
          });
        if (l.url.includes('/folders/root')) return json({ id: OTRO, name: '', is_root: true });
        if (l.url.includes('/children')) return json({ folders: [], files: [fichero] });
        if (l.url.includes(`/files/${ID}/content`)) return new Response('xls', { status: 200 });
        if (l.url.includes(`/files/${ID}`)) return json(fichero);
        if (l.url.includes('/import')) {
          return json({
            kind: 'spreadsheet',
            title: 'ventas',
            content: '{"sheets":[]}',
            ...(truncated ? { truncated } : {}),
          });
        }
        if (l.url.includes('/documents?')) return json({ documents: [] });
        if (l.method === 'POST' && l.url.endsWith('/documents')) {
          return json(
            { document: { id: ID, kind: 'spreadsheet', title: 'ventas', version: 0 } },
            201,
          );
        }
        return json({});
      };
      await (abrirDeDrive.GET as Handler)(navegacion('same-site'));
      const crear = llamadas.find((l) => l.method === 'POST' && l.url.endsWith('/documents'));
      expect(crear, 'crea el documento').toBeDefined();
      return JSON.parse(String(crear!.body)) as Record<string, unknown>;
    };
    const recortado = await abrirCon(true);
    expect(recortado['source_ref']).toBeUndefined();
    expect(recortado['title']).toBe('ventas');
    const entero = await abrirCon(false);
    expect(entero['source_ref']).toBe(`drive:${ID}`);
  });
});
