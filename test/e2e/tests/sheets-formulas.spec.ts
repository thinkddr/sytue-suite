// Spec e2e del motor de fórmulas en el KIND spreadsheet de Sytue Docs.
// Comprueba el cableado @sytue/formula ↔ grid: números en A1:A3, una fórmula
// «=SUM(A1:A3)» que muestra el valor calculado, un error canónico (#DIV/0!),
// y que la fórmula (texto crudo) persiste y se recalcula tras recargar.
// Corre autenticado (storageState del setup).
import { expect, test, type Page } from '@playwright/test';
import { APPS } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

/** Crea una hoja de cálculo por la API BFF y devuelve su id. */
async function crearHoja(
  request: Parameters<Parameters<typeof test>[2]>[0]['request'],
): Promise<string> {
  const title = `e2e-sheets-fx-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const res = await request.post(`${DOCS}api/create`, { data: { kind: 'spreadsheet', title } });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  expect(id).toBeTruthy();
  return id;
}

/** Escribe el texto CRUDO (literal o fórmula) en una celda: doble clic → teclear → Enter. */
async function escribirCelda(page: Page, ref: string, texto: string): Promise<void> {
  const celda = page.locator(`[data-ref="${ref}"]`);
  await celda.dblclick();
  await page.keyboard.type(texto);
  await page.keyboard.press('Enter'); // confirma y baja una fila
}

test.describe('Sytue Sheets — motor de fórmulas', () => {
  test('=SUM calcula, un error se muestra canónico y la fórmula persiste tras recargar', async ({
    page,
    request,
  }) => {
    test.slow(); // dev-server en frío: la isla del editor puede tardar en compilar
    const id = await crearHoja(request);

    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });

    // El grid y la barra de fórmulas deben estar montados.
    await expect(page.getByRole('textbox', { name: 'Barra de fórmulas' })).toBeVisible({
      timeout: 60_000,
    });
    await expect(page.getByRole('grid')).toBeVisible();

    // Números en A1:A3 y una fórmula de suma en B1.
    await escribirCelda(page, 'A1', '10');
    await escribirCelda(page, 'A2', '20');
    await escribirCelda(page, 'A3', '30');
    await escribirCelda(page, 'B1', '=SUM(A1:A3)');

    // El grid muestra el VALOR calculado, no el texto de la fórmula.
    await expect(page.locator('[data-ref="B1"]')).toHaveText('60');

    // Un error se muestra con su código canónico.
    await escribirCelda(page, 'C1', '=1/0');
    await expect(page.locator('[data-ref="C1"]')).toHaveText('#DIV/0!');

    // La barra de fórmulas refleja el texto CRUDO de la celda seleccionada.
    await page.locator('[data-ref="B1"]').click();
    await expect(page.getByRole('textbox', { name: 'Barra de fórmulas' })).toHaveValue(
      '=SUM(A1:A3)',
    );

    // Persistencia: esperar al autosave (op LWW `set` en doc:sheet) y recargar.
    await page.waitForResponse(
      (r) => r.url().includes('/api/ops') && r.request().method() === 'POST' && r.ok(),
      { timeout: 20_000 },
    );
    await expect(page.getByText(/^Guardado(\s|$)/).first()).toBeVisible({ timeout: 15_000 });

    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(page.getByRole('grid')).toBeVisible({ timeout: 60_000 });

    // La fórmula se recalcula tras la recarga: el valor sigue ahí…
    await expect(page.locator('[data-ref="B1"]')).toHaveText('60');
    await expect(page.locator('[data-ref="C1"]')).toHaveText('#DIV/0!');
    // …y el texto crudo (la fórmula) sobrevive: se ve en la barra al seleccionar.
    await page.locator('[data-ref="B1"]').click();
    await expect(page.getByRole('textbox', { name: 'Barra de fórmulas' })).toHaveValue(
      '=SUM(A1:A3)',
    );
  });
});
