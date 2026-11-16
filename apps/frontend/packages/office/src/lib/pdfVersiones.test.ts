// Las versiones de un PDF por su ruta: el puente traduce ruta → id, pide a Drive lo
// justo y cuenta los fallos con un código que el visor sabe decir.
import { expect, test } from 'bun:test';
import { HttpError, type DriveResource, type FileVersionPage } from '@sytue/api-clients';
import { listarOAbrir, restaurar, type ClienteVersiones } from './pdfVersiones.js';

interface Espia {
  cliente: ClienteVersiones;
  llamadas: string[];
}

function espia(opts: { restaurar?: () => Promise<void> } = {}): Espia {
  const llamadas: string[] = [];
  const pagina: FileVersionPage = {
    versions: [
      { version: 4, sizeBytes: 40, isCurrent: true },
      { version: 3, sizeBytes: 30, isCurrent: false },
    ],
    nextCursor: 'mas',
  };
  const cliente: ClienteVersiones = {
    stat: async (ruta: string): Promise<DriveResource> => {
      llamadas.push(`stat ${ruta}`);
      if (ruta === '/PDF/carpeta.pdf')
        return { path: ruta, name: 'carpeta.pdf', isDirectory: true, id: 'd1' };
      if (ruta !== '/PDF/contrato.pdf') throw new HttpError(404, 'Not Found', ruta);
      return { path: ruta, name: 'contrato.pdf', isDirectory: false, id: 'f1' };
    },
    versions: async (id, o) => {
      llamadas.push(`versions ${id} ${o?.cursor ?? ''}`);
      return pagina;
    },
    openVersionDownload: async (id, n) => {
      llamadas.push(`content ${id} ${n}`);
      return new Response(new Uint8Array([37, 80, 68, 70]));
    },
    restoreVersion:
      opts.restaurar ??
      (async (id, n) => {
        llamadas.push(`restore ${id} ${n}`);
      }),
  } as ClienteVersiones;
  return { cliente, llamadas };
}

const URL_BASE = 'https://office.test/api/pdf-versiones';

test('lista el historial del PDF por su ruta, con el cursor de la página siguiente', async () => {
  const { cliente, llamadas } = espia();
  const r = await listarOAbrir(
    new URL(`${URL_BASE}?path=${encodeURIComponent('/PDF/contrato.pdf')}&cursor=c2`),
    cliente,
  );
  expect(r.status).toBe(200);
  expect(await r.json()).toEqual({
    versions: [
      { version: 4, sizeBytes: 40, isCurrent: true },
      { version: 3, sizeBytes: 30, isCurrent: false },
    ],
    nextCursor: 'mas',
  });
  expect(llamadas).toEqual(['stat /PDF/contrato.pdf', 'versions f1 c2']);
});

test('abrir una versión la sirve como PDF que el navegador no reinterpreta', async () => {
  const { cliente, llamadas } = espia();
  const r = await listarOAbrir(new URL(`${URL_BASE}?path=/PDF/contrato.pdf&version=3`), cliente);
  expect(r.status).toBe(200);
  expect(r.headers.get('content-type')).toBe('application/pdf');
  expect(r.headers.get('x-content-type-options')).toBe('nosniff');
  expect(r.headers.get('content-disposition')).toBe(
    `inline; filename*=UTF-8''${encodeURIComponent('contrato (v3).pdf')}`,
  );
  expect(new Uint8Array(await r.arrayBuffer())).toEqual(new Uint8Array([37, 80, 68, 70]));
  expect(llamadas.at(-1)).toBe('content f1 3');
});

test('sin ruta de PDF, con una versión que no es un número o con una carpeta, no se sirve nada', async () => {
  const { cliente, llamadas } = espia();
  expect((await listarOAbrir(new URL(URL_BASE), cliente)).status).toBe(400);
  expect((await listarOAbrir(new URL(`${URL_BASE}?path=/PDF/nota.txt`), cliente)).status).toBe(400);
  for (const v of ['0', '-1', '2/../../x', 'abc']) {
    const r = await listarOAbrir(
      new URL(`${URL_BASE}?path=/PDF/contrato.pdf&version=${encodeURIComponent(v)}`),
      cliente,
    );
    expect(r.status).toBe(400);
  }
  // Nada de lo anterior debe haber llegado a Drive.
  expect(llamadas).toEqual([]);
  const carpeta = await listarOAbrir(new URL(`${URL_BASE}?path=/PDF/carpeta.pdf`), cliente);
  expect(carpeta.status).toBe(404);
  const otro = await listarOAbrir(new URL(`${URL_BASE}?path=/PDF/otro.pdf`), cliente);
  expect(otro.status).toBe(404);
});

function post(cuerpo: unknown): Request {
  return new Request(URL_BASE, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: typeof cuerpo === 'string' ? cuerpo : JSON.stringify(cuerpo),
  });
}

test('restaurar resuelve la ruta y restaura esa versión', async () => {
  const { cliente, llamadas } = espia();
  const r = await restaurar(post({ path: '/PDF/contrato.pdf', version: 3 }), cliente);
  expect(r.status).toBe(200);
  expect(await r.json()).toEqual({ ok: true });
  expect(llamadas).toEqual(['stat /PDF/contrato.pdf', 'restore f1 3']);
});

test('restaurar sin permiso devuelve 403 y Drive caído 502: el visor dice cosas distintas', async () => {
  const denegado = espia({
    restaurar: async () => {
      throw new HttpError(403, 'Forbidden', '/x');
    },
  });
  const r403 = await restaurar(post({ path: '/PDF/contrato.pdf', version: 3 }), denegado.cliente);
  expect(r403.status).toBe(403);
  expect(await r403.json()).toEqual({ error: 'forbidden' });

  const caido = espia({
    restaurar: async () => {
      throw new TypeError('fetch failed');
    },
  });
  const r502 = await restaurar(post({ path: '/PDF/contrato.pdf', version: 3 }), caido.cliente);
  expect(r502.status).toBe(502);
});

test('un cuerpo roto o sin versión válida no llega a Drive', async () => {
  const { cliente, llamadas } = espia();
  expect((await restaurar(post('{no es json'), cliente)).status).toBe(400);
  expect((await restaurar(post({ path: '/PDF/contrato.pdf' }), cliente)).status).toBe(400);
  expect((await restaurar(post({ path: '/PDF/contrato.pdf', version: 1.5 }), cliente)).status).toBe(
    400,
  );
  expect((await restaurar(post({ version: 2 }), cliente)).status).toBe(400);
  expect(llamadas).toEqual([]);
});
