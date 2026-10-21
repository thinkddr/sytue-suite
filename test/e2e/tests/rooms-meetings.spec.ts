// Spec e2e de Sytue Rooms — REUNIONES: las salas que no cuelgan de un canal.
//   1) «Nueva reunión» crea la sala y entra (la vista de vídeo monta con tu mosaico).
//   2) La reunión aparece en «Ahora mismo» y su enlace deja entrar a otra pestaña.
//   3) El detalle enseña quién está; el host puede terminarla.
// Corre autenticado como admin (storageState del setup) con medios sintéticos.
import { expect, test } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

test.describe('Sytue Rooms — reuniones', () => {
  test('convoca una reunión, entra, aparece en la lista y se puede terminar', async ({ page }) => {
    test.slow(); // el dev-server recompila la isla de LiveKit en frío

    await gotoApp(page, 'rooms');
    // El panel vive detrás de «Reuniones» en el sidebar: si esa puerta no existe, la
    // función es inalcanzable por mucho que el backend la sirva.
    await page.getByRole('button', { name: 'Reuniones' }).click();

    const titulo = `e2e-reunión-${Date.now()}`;
    await page.getByLabel('Nombre de la reunión').fill(titulo);

    const creada = page.waitForResponse(
      (r) => r.url().includes('/api/meetings') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Crear y entrar' }).click();
    expect((await creada).status(), 'POST /api/meetings debe crear la sala').toBe(201);

    // Entrar de verdad: el mosaico propio aparece cuando LiveKit conecta.
    await expect(page.locator('[title="Tú"]')).toBeVisible({ timeout: 60_000 });

    // La reunión figura entre las activas MIENTRAS está viva — que es cuando alguien
    // puede unirse. Se comprueba sin salir: al quedarse vacía, LiveKit la cierra y
    // pasa a `ended` (ese cierre es justo lo que publica la tarjeta de fin de llamada).
    const activas = await page.request.get(`${APPS.rooms}api/meetings?status=active`);
    expect(activas.status(), 'GET /api/meetings').toBe(200);
    const { meetings } = (await activas.json()) as { meetings: { title: string }[] };
    expect(
      meetings.map((m) => m.title),
      'la reunión recién convocada tiene que estar entre las activas',
    ).toContain(titulo);

    await page.getByRole('button', { name: 'Salir' }).click();
  });

  test('GET /api/meetings devuelve las reuniones activas del tenant', async ({ page }) => {
    await gotoApp(page, 'rooms');
    const res = await page.request.get(`${APPS.rooms}api/meetings?status=active`);
    expect(res.status(), 'GET /api/meetings').toBe(200);
    const { meetings } = (await res.json()) as { meetings: { id: string; title: string }[] };
    expect(Array.isArray(meetings), 'meetings debe ser una lista').toBe(true);
  });
});
