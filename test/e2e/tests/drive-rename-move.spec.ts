// Spec e2e de Sytue Drive — Renombrar y Mover, API del BFF
// (https://drive.sytue.localhost/api/files). Usa el storageState de admin del
// proyecto, así que el fixture `request` ya va autenticado. Nombres únicos por
// test para independencia y re-ejecución (también en paralelo).
//
// QUÉ CUBRE:
//   · POST /api/files { path, name }   → renombra (fichero y carpeta): el nuevo
//     nombre aparece en el listado y el antiguo desaparece.
//   · POST /api/files { path, toPath } → mueve un fichero a otra carpeta: entra
//     en el listado del destino y sale del origen.
//   · Validación del BFF: sin name/toPath → 400; mover una carpeta dentro de sí
//     misma → 400 (guarda anti-ciclo).
// El BFF delega en DriveClient.move() (PATCH /folders|files/{id} del Go). POST
// y no PATCH: el CSRF de Astro solo admite POST con cuerpo tras el proxy.
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const API = APPS.drive; // https://drive.sytue.localhost/
const ORIGIN = API.replace(/\/$/, '');

// Sufijo único por test (Date.now() puede colisionar entre workers; el tramo
// aleatorio lo evita). Mismo criterio que drive-trash.spec.ts.
const unico = () => `${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;

/** Nombres de una carpeta. GET /api/files?path= → 200 con items. */
async function nombres(request: APIRequestContext, ruta = '/'): Promise<string[]> {
  const res = await request.get(`${API}api/files?path=${encodeURIComponent(ruta)}`);
  expect(res.status()).toBe(200);
  const { items } = (await res.json()) as { items: Array<{ name: string }> };
  return items.map((it) => it.name);
}

/** Sube un .txt (octet-stream + ?type=, patrón anti-CSRF de Astro). */
async function subir(request: APIRequestContext, ruta: string, contenido: string): Promise<void> {
  const res = await request.put(
    `${API}api/content?path=${encodeURIComponent(ruta)}&type=text%2Fplain`,
    { headers: { 'content-type': 'application/octet-stream' }, data: contenido },
  );
  expect(res.status()).toBe(201);
}

/** Crea una carpeta. POST /api/folders { path } → 201. */
async function crearCarpeta(request: APIRequestContext, ruta: string): Promise<void> {
  const res = await request.post(`${API}api/folders`, { data: { path: ruta } });
  expect(res.status()).toBe(201);
}

/** Limpieza best-effort: a la papelera (DELETE con Origin real + octet-stream). */
async function borrar(request: APIRequestContext, ruta: string): Promise<void> {
  await request.delete(`${API}api/files?path=${encodeURIComponent(ruta)}`, {
    headers: { origin: ORIGIN, 'content-type': 'application/octet-stream' },
  });
}

test.describe('Drive · Renombrar y Mover (BFF /api/files POST)', () => {
  // El dev server (Vite sobre bind-mount Docker) compila rutas bajo demanda y en
  // frío puede tardar decenas de segundos. Margen holgado para no falsear infra.
  test.describe.configure({ timeout: 120_000 });

  test('renombrar un fichero: el nuevo nombre sustituye al antiguo en el listado', async ({
    request,
  }) => {
    const sufijo = unico();
    const antes = `e2e-renombrar-${sufijo}.txt`;
    const despues = `e2e-renombrado-${sufijo}.txt`;

    await subir(request, `/${antes}`, `renombrable — ${antes}`);
    expect(await nombres(request)).toContain(antes);

    // JSON no dispara el checkOrigin CSRF de Astro (solo content-type «de formulario»).
    const res = await request.post(`${API}api/files`, {
      data: { path: `/${antes}`, name: despues },
    });
    expect(res.status()).toBe(200);

    // expect.poll absorbe cualquier propagación sin sleeps.
    await expect
      .poll(async () => await nombres(request), { timeout: 15_000 })
      .toContain(despues);
    expect(await nombres(request)).not.toContain(antes);

    await borrar(request, `/${despues}`); // limpieza best-effort
  });

  test('renombrar una carpeta: conserva su contenido bajo el nuevo nombre', async ({
    request,
  }) => {
    const sufijo = unico();
    const antes = `e2e-carpeta-${sufijo}`;
    const despues = `e2e-carpeta-nueva-${sufijo}`;
    const fichero = `dentro-${sufijo}.txt`;

    await crearCarpeta(request, `/${antes}`);
    await subir(request, `/${antes}/${fichero}`, 'contenido que debe sobrevivir al rename');

    const res = await request.post(`${API}api/files`, {
      data: { path: `/${antes}`, name: despues },
    });
    expect(res.status()).toBe(200);

    await expect
      .poll(async () => await nombres(request), { timeout: 15_000 })
      .toContain(despues);
    expect(await nombres(request)).not.toContain(antes);
    // El contenido sigue dentro (el rename no toca los hijos).
    expect(await nombres(request, `/${despues}`)).toContain(fichero);

    await borrar(request, `/${despues}`); // limpieza best-effort
  });

  test('mover un fichero a una carpeta: entra en el destino y sale del origen', async ({
    request,
  }) => {
    const sufijo = unico();
    const carpeta = `e2e-destino-${sufijo}`;
    const fichero = `e2e-mover-${sufijo}.txt`;

    await crearCarpeta(request, `/${carpeta}`);
    await subir(request, `/${fichero}`, `movible — ${fichero}`);
    expect(await nombres(request)).toContain(fichero);

    const res = await request.post(`${API}api/files`, {
      data: { path: `/${fichero}`, toPath: `/${carpeta}` },
    });
    expect(res.status()).toBe(200);

    // Efecto observable: está en el destino y ya no en la raíz.
    await expect
      .poll(async () => await nombres(request, `/${carpeta}`), { timeout: 15_000 })
      .toContain(fichero);
    expect(await nombres(request)).not.toContain(fichero);

    await borrar(request, `/${carpeta}`); // limpieza best-effort (carpeta + contenido)
  });

  test('validación del BFF: sin destino → 400; carpeta dentro de sí misma → 400', async ({
    request,
  }) => {
    const sufijo = unico();
    const carpeta = `e2e-validar-${sufijo}`;
    await crearCarpeta(request, `/${carpeta}`);

    // Ni name ni toPath: nada que hacer.
    const sinDestino = await request.post(`${API}api/files`, {
      data: { path: `/${carpeta}` },
    });
    expect(sinDestino.status()).toBe(400);

    // Sin path: falta el recurso a mover.
    const sinPath = await request.post(`${API}api/files`, { data: { name: 'x' } });
    expect(sinPath.status()).toBe(400);

    // Mover una carpeta dentro de sí misma crearía un ciclo: la guarda del BFF corta.
    const ciclo = await request.post(`${API}api/files`, {
      data: { path: `/${carpeta}`, toPath: `/${carpeta}` },
    });
    expect(ciclo.status()).toBe(400);

    // La carpeta sigue intacta en la raíz tras los rechazos.
    expect(await nombres(request)).toContain(carpeta);

    await borrar(request, `/${carpeta}`); // limpieza best-effort
  });
});
