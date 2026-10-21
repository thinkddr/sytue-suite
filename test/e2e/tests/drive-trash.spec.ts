// Spec e2e de Sytue Drive — Papelera (trash), API del BFF
// (https://drive.sytue.localhost/api/*). Usa el storageState de admin del
// proyecto, así que el fixture `request` ya va autenticado. Nombres únicos por
// test para independencia y re-ejecución (también en paralelo).
//
// QUÉ CUBRE (verde con el backend ACTUAL):
//   · Borrar un fichero lo saca del listado activo (efecto observable de
//     «enviar a la papelera»). drive.spec.ts ya comprueba que el DELETE
//     responde 200; aquí se verifica el efecto real: el recurso desaparece.
//
// QUÉ CUBRE tras REBUILD del contenedor `drive` (los dos test.fixme de abajo):
//   · Listar la papelera (GET /api/trash), restaurar (POST /api/trash) y purgar
//     (DELETE /api/trash?id=&kind=). El backend Go ya implementa borrado SOFT no
//     destructivo + /trash + /{files,folders}/{id}/restore + .../purge, el SDK
//     (listTrash/restore/purge), el BFF (/api/trash) y la UI (vista Papelera).
//     Los tests quedan como test.fixme SOLO porque el contenedor `drive` en dev
//     corre el binario Go anterior (sin /trash): REQUIERE REBUILD del contenedor
//     `drive`. Hecho el rebuild, cambiar `test.fixme` → `test` y pasan tal cual.
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const API = APPS.drive; // https://drive.sytue.localhost/
const ORIGIN = API.replace(/\/$/, '');

// Sufijo único por test (Date.now() puede colisionar entre workers; el tramo
// aleatorio lo evita). Mismo criterio que drive.spec.ts.
const unico = () => `${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;

/** Nombres de la raíz. GET /api/files?path=/ → 200 con items. */
async function nombresRaiz(request: APIRequestContext): Promise<string[]> {
  const res = await request.get(`${API}api/files?path=/`);
  expect(res.status()).toBe(200);
  const { items } = (await res.json()) as { items: Array<{ name: string }> };
  return items.map((it) => it.name);
}

interface ItemPapelera {
  id: string;
  name: string;
  isDirectory: boolean;
  size?: number;
}

/** Contenido de la papelera. GET /api/trash → 200 con items. */
async function papelera(request: APIRequestContext): Promise<ItemPapelera[]> {
  const res = await request.get(`${API}api/trash`);
  expect(res.status()).toBe(200);
  const { items } = (await res.json()) as { items: ItemPapelera[] };
  return items;
}

/** Sube un .txt a la raíz (octet-stream + ?type=, patrón anti-CSRF de Astro). */
async function subir(request: APIRequestContext, ruta: string, contenido: string): Promise<void> {
  const res = await request.put(`${API}api/content?path=${ruta}&type=text%2Fplain`, {
    headers: { 'content-type': 'application/octet-stream' },
    data: contenido,
  });
  expect(res.status()).toBe(201);
}

/** Envía un recurso a la papelera (DELETE con Origin real + octet-stream). */
async function borrar(request: APIRequestContext, ruta: string): Promise<void> {
  const res = await request.delete(`${API}api/files?path=${ruta}`, {
    headers: { origin: ORIGIN, 'content-type': 'application/octet-stream' },
  });
  expect(res.status()).toBe(200);
}

test.describe('Drive · Papelera (BFF /api)', () => {
  // El dev server (Vite sobre bind-mount Docker) compila rutas bajo demanda y en
  // frío puede tardar decenas de segundos. Margen holgado para no falsear infra.
  test.describe.configure({ timeout: 120_000 });

  test('borrar un fichero lo saca del listado activo (enviado a la papelera)', async ({
    request,
  }) => {
    const nombre = `e2e-papelera-${unico()}.txt`;
    const ruta = encodeURIComponent(`/${nombre}`);

    await subir(request, ruta, `a la papelera — ${nombre}`);

    // Precondición: aparece en el listado activo antes de borrar.
    expect(await nombresRaiz(request)).toContain(nombre);

    await borrar(request, ruta);

    // Efecto observable: ya no está en el listado activo (soft-delete → los reads
    // filtran deleted_at IS NULL). expect.poll absorbe cualquier propagación sin
    // sleeps y sin falsos negativos por carrera.
    await expect
      .poll(async () => (await nombresRaiz(request)).includes(nombre), { timeout: 15_000 })
      .toBe(false);
  });

  // --- Papelera navegable: REQUIERE REBUILD del contenedor `drive` ------------
  // Backend/SDK/BFF/UI ya implementados (borrado NO destructivo + /trash +
  // restore + purge). Quedan fixme porque el contenedor `drive` corre el binario
  // anterior sin /trash. Hecho el rebuild, cambiar `test.fixme` → `test`.

  test(
    'aparece en la papelera al borrarlo y se restaura a Mi unidad',
    async ({ request }) => {
      const nombre = `e2e-restaurar-${unico()}.txt`;
      const ruta = encodeURIComponent(`/${nombre}`);

      await subir(request, ruta, `restaurable — ${nombre}`);
      await borrar(request, ruta);

      // Sale del listado activo y entra en la papelera (soft-delete no destructivo).
      await expect
        .poll(async () => (await papelera(request)).some((it) => it.name === nombre), {
          timeout: 15_000,
        })
        .toBe(true);

      const item = (await papelera(request)).find((it) => it.name === nombre)!;
      expect(item.isDirectory).toBe(false);
      expect(item.id).toBeTruthy();

      // Restaurar: POST /api/trash { id, kind } (JSON no dispara el checkOrigin CSRF).
      const rest = await request.post(`${API}api/trash`, { data: { id: item.id, kind: 'file' } });
      expect(rest.status()).toBe(200);

      // Vuelve a Mi unidad (a su carpeta original: la raíz) y sale de la papelera.
      await expect
        .poll(async () => (await nombresRaiz(request)).includes(nombre), { timeout: 15_000 })
        .toBe(true);
      expect((await papelera(request)).some((it) => it.name === nombre)).toBe(false);

      await borrar(request, ruta); // limpieza best-effort
    },
  );

  test('eliminar definitivamente (purga) lo quita de la papelera', async ({ request }) => {
    const nombre = `e2e-purgar-${unico()}.txt`;
    const ruta = encodeURIComponent(`/${nombre}`);

    await subir(request, ruta, `purgable — ${nombre}`);
    await borrar(request, ruta);

    await expect
      .poll(async () => (await papelera(request)).some((it) => it.name === nombre), {
        timeout: 15_000,
      })
      .toBe(true);

    const item = (await papelera(request)).find((it) => it.name === nombre)!;

    // Purga definitiva: DELETE /api/trash?id=&kind= (Origin real + octet-stream,
    // sin cuerpo — mismo patrón anti-CSRF que el borrado a la papelera).
    const purge = await request.delete(`${API}api/trash?id=${item.id}&kind=file`, {
      headers: { origin: ORIGIN, 'content-type': 'application/octet-stream' },
    });
    expect(purge.status()).toBe(200);

    // Ya no está ni en la papelera ni en Mi unidad (borrado irreversible).
    await expect
      .poll(async () => (await papelera(request)).some((it) => it.name === nombre), {
        timeout: 15_000,
      })
      .toBe(false);
    expect(await nombresRaiz(request)).not.toContain(nombre);
  });
});
