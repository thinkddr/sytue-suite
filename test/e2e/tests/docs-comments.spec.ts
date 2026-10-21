// Spec e2e de los comentarios ANCLADOS de Sytue Docs (feature «comentarios a la
// selección»): crear documento, escribir, seleccionar una palabra y anclar un
// comentario desde la pestaña Colaboración envuelve la selección en un
// <mark data-comment-id> (resaltado sutil) y crea el hilo. Se comprueba que el
// hilo aparece en el panel, que el texto queda resaltado, que ambos SOBREVIVEN a
// una recarga (el <mark> vive en el HTML del folio, que se autosava; el ancla en
// el jsonb libre de doc_comments) y que «Resolver» retira el resaltado.
//
// El resaltado persiste sin Go nuevo: el ancla usa el campo `anchor` (jsonb) que
// el backend YA acepta y devuelve verbatim, y el <mark> viaja en el propio HTML
// del documento. No requiere rebuild de ningún servicio Go.
//
// Corre autenticado (storageState del setup).
import { expect, test, type Page } from '@playwright/test';
import { APPS } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

/** Crea un documento por la API BFF y devuelve su id. */
async function crearDocumento(
  request: Parameters<Parameters<typeof test>[2]>[0]['request'],
): Promise<string> {
  const title = `e2e-comments-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const res = await request.post(`${DOCS}api/create`, { data: { kind: 'document', title } });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  expect(id).toBeTruthy();
  return id;
}

/** Espera un autosave confirmado (POST /api/ops OK). Los registra el llamante
 *  ANTES de la acción que teclea/muta, para no ver respuestas ya pasadas. */
function esperarAutosave(page: Page) {
  return page.waitForResponse(
    (r) => r.url().includes('/api/ops') && r.request().method() === 'POST' && r.ok(),
    { timeout: 25_000 },
  );
}

/** Espera el editor listo tras navegar/recargar: cromo (título) y folio. La isla
 *  es `client:only`; en este monorepo compartido el HMR de @sytue/api-clients
 *  (otras apps editándose en paralelo) puede servir un bundle invalidado que no
 *  arranca, así que si el título no aparece se recarga para tomar el bundle
 *  fresco. Solo se llama en puntos sin edición pendiente (recargar es seguro). */
async function esperarEditor(page: Page): Promise<void> {
  const titulo = page.locator('#editor-title-input');
  for (let intento = 0; intento < 4; intento++) {
    const ok = await titulo.waitFor({ state: 'visible', timeout: 30_000 }).then(
      () => true,
      () => false,
    );
    if (ok) break;
    await page.reload({ waitUntil: 'domcontentloaded' });
  }
  await expect(titulo).toBeVisible({ timeout: 30_000 });
  await expect(page.locator('.sytue-page')).toBeVisible({ timeout: 30_000 });
}

/** Selecciona la primera aparición de `word` en el folio por API de selección
 *  (fiable, sin depender de coordenadas del doble clic). Enfoca el folio para
 *  que la selección quede viva cuando el ribbon la lea. */
async function seleccionarPalabra(page: Page, word: string): Promise<void> {
  await page.evaluate((w) => {
    const folio = document.querySelector<HTMLElement>('.sytue-page');
    if (!folio) throw new Error('sin folio');
    folio.focus();
    const walker = document.createTreeWalker(folio, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const i = (n.nodeValue ?? '').indexOf(w);
      if (i >= 0) {
        const r = document.createRange();
        r.setStart(n, i);
        r.setEnd(n, i + w.length);
        const sel = window.getSelection();
        sel?.removeAllRanges();
        sel?.addRange(r);
        return;
      }
    }
    throw new Error(`palabra no encontrada en el folio: ${w}`);
  }, word);
}

test.describe('Comentarios anclados de Sytue Docs', () => {
  test('anclar a la selección: <mark> + hilo aparecen, persisten al recargar y «Resolver» los retira', async ({
    page,
    request,
  }) => {
    // Holgura amplia: dev-server en frío + reintentos de carga de la isla ante
    // el churn de HMR del monorepo compartido (ver esperarEditor).
    test.setTimeout(300_000);
    const id = await crearDocumento(request);
    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });

    const folio = page.locator('.sytue-page');
    await esperarEditor(page);

    // Escribir un párrafo con una palabra distinguible; esperar a que su autosave
    // repose ANTES de anclar (así el siguiente ops confirmado es el del <mark>).
    await folio.click();
    const opsTecleo = esperarAutosave(page);
    await page.keyboard.type('El documento tiene una palabra anclada aquí.');
    await opsTecleo;

    // Colaboración → seleccionar «anclada» → «Nuevo comentario» (prompt del cuerpo).
    await page.getByRole('tab', { name: 'Colaboración' }).click();
    await seleccionarPalabra(page, 'anclada');
    // Barrera explícita: el marcado lee el último Range (savedRange, que actualiza
    // el evento `selectionchange`). Confirmar que la selección viva es «anclada»
    // ANTES de pulsar garantiza que ese evento ya corrió (sin depender del timing).
    await expect
      .poll(() => page.evaluate(() => (window.getSelection()?.toString() ?? '').trim()))
      .toBe('anclada');

    const BODY = 'Comentario anclado e2e';
    const opsMarca = esperarAutosave(page);
    page.once('dialog', (d) => void d.accept(BODY));
    await page.getByRole('button', { name: 'Nuevo comentario' }).click();

    // El panel de comentarios se abre solo y muestra el hilo con su cita anclada.
    const panel = page.locator('aside[aria-label="Comentarios"]');
    await expect(panel.getByText(BODY)).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByText('«anclada»')).toBeVisible(); // ancla (jsonb) del backend
    // El texto seleccionado queda resaltado como <mark data-comment-id> en el folio.
    await expect(folio.locator('mark[data-comment-id]')).toHaveText('anclada');
    await opsMarca; // el <mark> quedó guardado en el HTML del folio

    // Contrato de persistencia: recargar y seguir ahí (comentario + resaltado).
    await page.reload({ waitUntil: 'domcontentloaded' });
    await esperarEditor(page);
    await expect(folio).toContainText('palabra anclada aquí');
    await expect(folio.locator('mark[data-comment-id]')).toHaveText('anclada');
    await page.getByRole('button', { name: 'Comentarios', exact: true }).click();
    await expect(panel.getByText(BODY)).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByText('«anclada»')).toBeVisible(); // el ancla jsonb sobrevivió

    // «Resolver» retira el resaltado: el <mark> desaparece del folio y se autosava.
    const opsResuelto = esperarAutosave(page);
    await panel.locator('article').first().getByRole('button', { name: 'Resolver' }).click();
    await expect(folio.locator('mark[data-comment-id]')).toHaveCount(0, { timeout: 15_000 });
    await opsResuelto;
  });
});
