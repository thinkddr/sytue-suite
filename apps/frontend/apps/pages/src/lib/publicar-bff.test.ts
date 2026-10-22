/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { DatosRender, Sitio } from '@sytue/pages-render';
import { crearPublicar, sha256 } from './publicar-bff';

const datos: DatosRender = {
  site: {
    id: 'site',
    slug: 'demo',
    sites_host: 'sites.example.test',
    canonical_host: 'demo.example.test',
    hosts: ['demo.example.test'],
    analytics: false,
  },
  images: {},
  files: {},
  fonts: {},
  forms: {},
  origins: { cast: 'https://cast.example.test', pages: 'https://pages.example.test' },
  now: '2026-09-29T00:00:00Z',
  legal: {
    holder_name: 'Sytue',
    tax_id: 'X',
    country: 'ES',
    address: { street: 'Calle Uno', postal_code: '28001', city: 'Madrid', region: '' },
    email: 'legal@example.test',
    phone: '',
    registry: '',
    privacy_contact: '',
  },
  store: null,
};

const sitio: Sitio = {
  version: 1,
  languages: { default: 'es', others: ['en'] },
  header: null,
  footer: null,
  components: [],
  seo: { site_name: { es: 'Sytue', en: 'Sytue' } },
  theme: {
    colors: {
      background: '#ffffff',
      text: '#111111',
      primary: '#000000',
      on_primary: '#ffffff',
      muted: '#eeeeee',
      border: '#dddddd',
    },
    fonts: { heading: 'inter', body: 'inter' },
    text_styles: {} as Sitio['theme']['text_styles'],
    radius: { sm: 4, md: 8, lg: 12 },
    container: 960,
  },
  pages: [
    {
      id: 'page000001',
      slug: { es: '', en: '' },
      title: { es: 'Inicio', en: 'Home' },
      description: { es: 'Descripción', en: 'Description' },
      show_header: false,
      show_footer: false,
      sections: [
        {
          id: 'section001',
          type: 'section',
          kind: 'hero',
          full_bleed: false,
          layout: {
            direction: 'column',
            gap: 0,
            padding: [0, 0, 0, 0],
            align: 'start',
            justify: 'start',
            wrap: false,
          },
          children: [
            {
              id: 'heading001',
              type: 'text',
              tag: 'h1',
              content: { es: [{ text: 'Hola' }], en: [{ text: 'Hello' }] },
            },
          ],
        },
      ],
    },
  ],
};

const ID = '11111111-1111-4111-8111-111111111111';
const VERSION = '22222222-2222-4222-8222-222222222222';
const TOKEN = 'a'.repeat(32);
const SECRETO = 'publisher-server-only';
function contexto(kind: unknown = 'live', headers: Record<string, string> = {}): APIContext {
  const url = new URL(`https://pages.example.test/pages/api/publicar/${ID}`);
  return {
    url,
    params: { id: ID },
    request: {
      method: 'POST',
      headers: new Headers({ 'content-type': 'application/json', ...headers }),
      json: async () => ({ kind }),
    },
  } as unknown as APIContext;
}
function escenario(opts: { irVersion?: number; error?: number; previewToken?: string } = {}) {
  const peticiones: Request[] = [];
  const handler = crearPublicar({
    destino: 'http://pages:8100/',
    secreto: () => SECRETO,
    sesion: async () => ({ accessToken: 'user-token' }),
    fetch: (async (url: string | URL | Request, init?: RequestInit) => {
      const req = new Request(url, init);
      peticiones.push(req);
      if (opts.error)
        return Response.json(
          { error: { code: 'legal_data_missing', message: 'Missing legal data' } },
          { status: opts.error },
        );
      if (req.url.endsWith('/snapshots'))
        return Response.json({
          version_id: VERSION,
          ir_version: opts.irVersion ?? 1,
          ir: sitio,
          data: datos,
        });
      if (req.url.endsWith('/previews'))
        return Response.json({
          token: opts.previewToken ?? TOKEN,
          base_path: `/_vista/${opts.previewToken ?? TOKEN}`,
          expires_at: '2026-10-10T00:00:00Z',
        });
      return Response.json({ number: 3, url: `https://demo.example.test/_vista/${TOKEN}/` });
    }) as unknown as typeof fetch,
  });
  return { handler, peticiones };
}
describe('publicación desde el BFF', () => {
  test('renderiza live y envía un manifiesto con los hashes y todas las rutas, incluido otro idioma', async () => {
    const { handler, peticiones } = escenario();
    const res = await handler(contexto());
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({
      kind: 'live',
      number: 3,
      url: 'https://demo.example.test/',
    });
    expect(peticiones).toHaveLength(2);
    const snap = peticiones[0]!;
    expect(snap.headers.get('authorization')).toBe('Bearer user-token');
    expect(snap.headers.has('x-sytue-publicador')).toBe(false);
    expect(await snap.json()).toEqual({ purpose: 'publish' });
    const pub = peticiones[1]!;
    expect(pub.url).toEndWith('/publications?kind=live');
    expect(pub.headers.get('x-sytue-publicador')).toBe(SECRETO);
    const form = await pub.formData();
    expect([...form.keys()][0]).toBe('manifest');
    const manifest = JSON.parse(String(form.get('manifest')));
    expect(manifest.version_id).toBe(VERSION);
    expect(manifest.kind).toBe('live');
    expect(manifest.files.some((f: { path: string }) => f.path === 'en/index.html')).toBe(true);
    const ficheros = form.getAll('file') as File[];
    expect(ficheros).toHaveLength(manifest.files.length);
    for (const f of ficheros) {
      const entrada = manifest.files.find((m: { path: string }) => m.path === f.name);
      const bytes = new Uint8Array(await f.arrayBuffer());
      expect(entrada.bytes).toBe(bytes.length);
      expect(entrada.sha256).toBe(sha256(bytes));
    }
  });
  test('la vista previa usa la base y el token del servicio para renderizar y subir', async () => {
    const { handler, peticiones } = escenario();
    const res = await handler(contexto('preview'));
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({
      kind: 'preview',
      url: `https://demo.example.test/_vista/${TOKEN}/`,
      expires_at: '2026-10-10T00:00:00Z',
    });
    expect(peticiones).toHaveLength(3);
    expect(await peticiones[0]!.json()).toEqual({ purpose: 'preview' });
    expect(peticiones[1]!.headers.get('x-sytue-publicador')).toBe(SECRETO);
    expect(peticiones[2]!.url).toEndWith(`/publications?kind=preview&token=${TOKEN}`);
    const form = await peticiones[2]!.formData();
    const manifest = JSON.parse(String(form.get('manifest')));
    expect(manifest.preview_token).toBe(TOKEN);
    const html = await (form.getAll('file') as File[])
      .find((f) => f.name === 'es/index.html')!
      .text();
    expect(html).toContain(`/_vista/${TOKEN}`);
  });
  test('no pide snapshots sin sesión o sin capacidad de publicar', async () => {
    for (const [sesion, secreto, status] of [
      [null, SECRETO, 401],
      [{ accessToken: 'user' }, undefined, 500],
    ] as const) {
      let llamadas = 0;
      const handler = crearPublicar({
        destino: 'http://pages',
        secreto: () => secreto,
        sesion: async () => sesion,
        fetch: (async () => {
          llamadas++;
          throw new Error('No debe llamar');
        }) as unknown as typeof fetch,
      });
      expect((await handler(contexto())).status).toBe(status);
      expect(llamadas).toBe(0);
    }
  });
  test('rechaza origen ajeno, tipo inválido y preview malformada sin subir una publicación', async () => {
    for (const ctx of [
      contexto('live', { origin: 'https://evil.example.test' }),
      contexto('invalid'),
    ]) {
      const { handler, peticiones } = escenario();
      expect((await handler(ctx)).status).toBeGreaterThanOrEqual(400);
      expect(peticiones).toHaveLength(0);
    }
    const { handler, peticiones } = escenario({ previewToken: '../invalid' });
    expect((await handler(contexto('preview'))).status).toBe(502);
    expect(peticiones).toHaveLength(2);
  });
  test('mantiene el error del servicio y rechaza un IR de otra versión antes de publicar', async () => {
    const error = escenario({ error: 422 });
    const res = await error.handler(contexto());
    expect(res.status).toBe(422);
    expect((await res.json()).error.code).toBe('legal_data_missing');
    expect(error.peticiones).toHaveLength(1);
    const futuro = escenario({ irVersion: 99 });
    expect((await futuro.handler(contexto())).status).toBe(409);
    expect(futuro.peticiones).toHaveLength(1);
  });
});
