// Spec e2e del editor nativo de Sytue Docs (BLUEPRINT.md §11): humo del
// contrato de persistencia — crear documento, renderizar las cuatro zonas,
// escribir, negrita, chip «Guardado», recargar y comprobar que el texto (y el
// fragmento en negrita) sobreviven. Corre autenticado (storageState del setup).
import { expect, test, type Page } from '@playwright/test';
import { APPS } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

/** Crea un documento por la API BFF y devuelve su id. */
async function crearDocumento(
  request: Parameters<Parameters<typeof test>[2]>[0]['request'],
  kind: 'document' | 'spreadsheet' | 'presentation',
): Promise<string> {
  const title = `e2e-editor-${kind}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const res = await request.post(`${DOCS}api/create`, { data: { kind, title } });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  expect(id).toBeTruthy();
  return id;
}

/** Espera el cromo común del editor (cabecera, ribbon y barra de estado). */
async function esperarCromo(page: Page): Promise<void> {
  await expect(page.locator('#editor-title-input')).toBeVisible({ timeout: 60_000 });
  await expect(page.getByRole('tablist', { name: 'Cinta de opciones' })).toBeVisible();
  await expect(page.getByRole('slider', { name: 'Zoom' })).toBeVisible();
}

test.describe('Editor de Sytue Docs', () => {
  test('documento: cuatro zonas, negrita en <b>/<strong>, renombrado y texto persisten tras recargar', async ({
    page,
    request,
  }) => {
    test.slow(); // dev-server en frío: la isla del editor puede tardar en compilar
    const id = await crearDocumento(request, 'document');

    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });

    // Las cuatro zonas: cabecera, ribbon, folio y barra de estado.
    await esperarCromo(page); // cabecera (título) + ribbon (tablist) + barra de estado (zoom)
    const folio = page.locator('.sytue-page');
    await expect(folio).toBeVisible();
    await expect(page.getByText(/^Palabras: \d+$/)).toBeVisible(); // contador en la barra de estado

    // Escribir un párrafo único y un fragmento en negrita desde el ribbon.
    await folio.click();
    await page.keyboard.type('Hola mundo persistente. ');
    await page.getByRole('button', { name: 'Negrita', exact: true }).click();
    await page.keyboard.type('énfasis');
    // La negrita debe existir como etiqueta semántica en el DOM, no solo como CSS.
    await expect(folio.locator('b, strong')).toContainText('énfasis');

    // Renombrar el título (optimista, debounce 800 ms → POST /api/rename).
    // Waiters registrados ANTES de disparar nada: el autosave (1500 ms tras la
    // última tecla) y el rename compiten y waitForResponse no ve respuestas pasadas.
    const opsOk = page.waitForResponse(
      (r) => r.url().includes('/api/ops') && r.request().method() === 'POST' && r.ok(),
      { timeout: 20_000 },
    );
    const renameOk = page.waitForResponse((r) => r.url().includes('/api/rename') && r.ok(), {
      timeout: 20_000,
    });
    const titulo = `e2e renombrado ${Date.now()}`;
    await page.locator('#editor-title-input').fill(titulo);
    await Promise.all([opsOk, renameOk]);
    await expect(page.getByText(/^Guardado(\s|$)/).first()).toBeVisible({ timeout: 15_000 });

    // LA verificación del contrato de persistencia: recargar y seguir ahí.
    await page.reload({ waitUntil: 'domcontentloaded' });
    await expect(page.locator('.sytue-page')).toContainText('Hola mundo persistente.', {
      timeout: 60_000,
    });
    // El fragmento en negrita sobrevive como <b>/<strong> en el DOM persistido.
    await expect(page.locator('.sytue-page').locator('b, strong')).toContainText('énfasis');
    // El título renombrado persiste.
    await expect(page.locator('#editor-title-input')).toHaveValue(titulo);
    // El contador de palabras cuenta el texto guardado (> 0).
    await expect(page.getByText(/^Palabras: [1-9]\d*$/)).toBeVisible();
  });

  test('hoja de cálculo: renderiza rejilla y barra de fórmulas', async ({ page, request }) => {
    test.slow();
    const id = await crearDocumento(request, 'spreadsheet');
    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });
    await esperarCromo(page);
    await expect(page.getByRole('textbox', { name: 'Barra de fórmulas' })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByRole('grid')).toBeVisible();
  });

  test('presentación: renderiza escenario y carril de miniaturas', async ({ page, request }) => {
    test.slow();
    const id = await crearDocumento(request, 'presentation');
    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });
    await esperarCromo(page);
    await expect(page.getByRole('group', { name: /^Diapositiva 1 de/ })).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByRole('button', { name: 'Nueva diapositiva' }).first()).toBeVisible();
  });
});
