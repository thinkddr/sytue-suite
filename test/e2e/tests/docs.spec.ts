// Spec e2e de Sytue Docs: listado por API, creación de documento y accesos
// de creación en la página. Corre autenticado como admin (storageState del setup).
import { expect, test } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

test.describe('Sytue Docs', () => {
  test('GET /api/documents responde 200 con el listado', async ({ request }) => {
    const res = await request.get(`${DOCS}api/documents`);
    // El cuerpo como mensaje: si falla (401/502 del BFF) el motivo queda en el informe.
    expect(res.status(), await res.text()).toBe(200);
    const body = (await res.json()) as { documents: unknown };
    expect(Array.isArray(body.documents)).toBe(true);
  });

  test('POST /api/create crea un documento (201) y aparece en el listado', async ({ page, request }) => {
    test.slow(); // el dev-server puede recompilar en frío y comerse el timeout base
    // Sufijo aleatorio: Date.now() puede colisionar entre workers con --repeat-each.
    const title = `e2e-docs-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const res = await request.post(`${DOCS}api/create`, {
      data: { kind: 'document', title },
    });
    expect(res.status(), await res.text()).toBe(201);
    const creado = (await res.json()) as { id: string; title: string; kind: string };
    expect(creado.id).toBeTruthy();
    expect(creado.title).toBe(title);
    expect(creado.kind).toBe('document');

    // Aparece en el listado de la página (sección «Recientes», ordenada por
    // fecha de actualización: el recién creado sale el primero).
    await gotoApp(page, 'docs');
    const recientes = page.getByRole('region', { name: 'Recientes' });
    await expect(recientes.getByText(title, { exact: true })).toBeVisible({ timeout: 15_000 });
  });

  test('la página muestra los botones de crear (Documento/Hoja/Presentación)', async ({ page }) => {
    test.slow(); // el dev-server puede recompilar en frío y comerse el timeout base
    await gotoApp(page, 'docs');
    const nuevo = page.getByRole('region', { name: 'Crear nuevo' });
    await expect(nuevo.getByRole('button', { name: /^Documento/ })).toBeVisible({ timeout: 15_000 });
    await expect(nuevo.getByRole('button', { name: /^Hoja de cálculo/ })).toBeVisible();
    await expect(nuevo.getByRole('button', { name: /^Presentación/ })).toBeVisible();
  });
});
