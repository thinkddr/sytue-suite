// Spec e2e de «Abrir/Importar» en Sytue Docs: verifica el cableado de la Ola 2
// (backend POST /api/v1/import + BFF /api/import + botón «Abrir un archivo» de
// DocList que importa → crea el doc del kind → siembra el contenido → abre el
// editor). Corre autenticado (storageState del setup).
import { expect, test } from '@playwright/test';
import { APPS } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

const CSV = 'Nombre;Edad\nAna;30\nLuis;25\n'; // ';' → autodetección de separador es-ES

test.describe('Sytue Docs — abrir/importar ficheros', () => {
  test('BFF /api/import convierte un CSV a doc:sheet', async ({ request }) => {
    // Cuerpo crudo (octet-stream) + nombre en ?name=: evita el checkOrigin de Astro.
    const res = await request.post(`${DOCS}api/import?name=gente.csv`, {
      headers: { 'content-type': 'application/octet-stream' },
      data: Buffer.from(CSV),
    });
    expect(res.status(), await res.text()).toBe(200);
    const body = (await res.json()) as { kind: string; title?: string; content?: string };
    expect(body.kind).toBe('spreadsheet');
    expect(body.content ?? '').toContain('Ana');
    expect(body.content ?? '').toContain('Luis');
  });

  test('«Abrir un archivo»: subir un CSV crea y abre una hoja con el contenido', async ({ page }) => {
    test.slow(); // dev-server en frío: la isla del editor puede tardar en compilar
    await page.goto(DOCS, { waitUntil: 'domcontentloaded' });
    // Espera a que la isla DocList HIDRATE y cargue la lista: mientras `docs===null`
    // muestra «Cargando documentos…»; ese texto desaparece SOLO tras correr el
    // useEffect load() (que es post-hidratación) → garantiza que el onChange del
    // input ya está enganchado y evita la carrera de hidratación del dev-server.
    await expect(page.getByText('Cargando documentos…')).toHaveCount(0, { timeout: 120_000 });
    // El input oculto dispara importar→crear→sembrar→navegar en su onChange.
    await page.setInputFiles('input[type="file"]', {
      name: 'gente.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from(CSV),
    });
    await page.waitForURL(/\/edit\?id=/, { timeout: 45_000 });
    await expect(page.locator('#editor-title-input')).toBeVisible({ timeout: 60_000 });
    // La hoja abierta muestra el contenido importado del CSV.
    await expect(page.getByText('Ana', { exact: false }).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText('Luis', { exact: false }).first()).toBeVisible();
  });
});
