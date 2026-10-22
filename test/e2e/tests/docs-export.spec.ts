// Spec e2e de Sytue Docs — Exportar/Descargar (BLUEPRINT.md §4.2).
//
// Test 1 (mecánica, END-TO-END contra la pila viva): el menú «Descargar» ofrece
// HTML/Markdown/ODF/OOXML y cada uno dispara una descarga real vía el BFF, que
// hace de PROXY del blob con Content-Disposition: attachment (el navegador no
// llega a S3/servicio Go). Verifica status, Content-Type, nombre de fichero y
// que baja un fichero íntegro (los ZIP —docx/odt— con su firma PK, probando que
// el proxy no corrompe binarios al no decodificarlos como texto).
//
// Test 2 (contenido): el fichero incluye el texto escrito. Está marcado
// test.fixme porque DEPENDE DE UN REBUILD del servicio Go `docs`: el exportador
// leía el modelo RGA/celdas en vez del registro doc:html del editor y salía
// vacío; el arreglo está en services/docs/internal/domain/export.go (go build +
// go test verdes) pero el contenedor en marcha aún corre el binario viejo.
import { readFileSync, statSync } from 'node:fs';
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

/** Crea un documento por la API BFF y devuelve su id. */
async function crearDocumento(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${DOCS}api/create`, { data: { kind: 'document', title } });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  expect(id).toBeTruthy();
  return id;
}

// Los cuatro formatos del menú Descargar (PDF NO: el backend no lo serializa).
// item = etiqueta accesible del MenuItem; mime = Content-Type que el BFF reenvía;
// ext = extensión del fichero; zip = paquete binario (comprueba firma PK).
const CASOS = [
  { item: /^HTML$/, ext: 'html', mime: 'text/html', zip: false },
  { item: /^Markdown$/, ext: 'md', mime: 'text/markdown', zip: false },
  { item: /^ODF/, ext: 'odt', mime: 'application/vnd.oasis.opendocument.text', zip: true },
  { item: /^OOXML/, ext: 'docx', mime: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', zip: true },
] as const;

test.describe('Sytue Docs — exportar/descargar', () => {
  test('el menú Descargar ofrece y baja HTML/Markdown/ODF/OOXML vía el BFF (proxy con attachment)', async ({
    page,
    request,
  }) => {
    test.slow(); // dev-server en frío: la isla del editor puede tardar en compilar

    const title = `e2e-export-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const id = await crearDocumento(request, title);

    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('#editor-title-input')).toBeVisible({ timeout: 60_000 });
    await expect(page.locator('.sytue-page')).toBeVisible();

    for (const caso of CASOS) {
      const trigger = page.getByRole('button', { name: 'Descargar' });
      await expect(trigger).toBeEnabled();
      await trigger.click();

      const item = page.getByRole('menu', { name: 'Descargar' }).getByRole('menuitem', { name: caso.item });
      const [download, resp] = await Promise.all([
        page.waitForEvent('download'),
        page.waitForResponse(
          (r) => r.url().includes('/api/export') && r.request().method() === 'POST',
          { timeout: 30_000 },
        ),
        item.click(),
      ]);

      // El BFF hace de proxy del blob con Content-Disposition attachment + MIME real.
      expect(resp.status(), `${caso.ext}: status`).toBe(200);
      const headers = resp.headers();
      expect(headers['content-disposition'] ?? '', `${caso.ext}: disposition`).toContain('attachment');
      expect(headers['content-type'] ?? '', `${caso.ext}: content-type`).toContain(caso.mime);

      // El navegador baja el fichero con el nombre y la extensión correctos…
      expect(download.suggestedFilename()).toBe(`${title}.${caso.ext}`);
      const ruta = await download.path();
      expect(ruta, `${caso.ext}: sin ruta de descarga`).toBeTruthy();

      // … y es un fichero íntegro. Los ZIP (docx/odt) deben conservar su firma PK:
      // prueba de que el proxy transporta binarios sin decodificarlos como texto.
      const buf = readFileSync(ruta);
      expect(statSync(ruta).size, `${caso.ext}: fichero vacío`).toBeGreaterThan(0);
      if (caso.zip) {
        expect(buf.subarray(0, 2).toString('latin1'), `${caso.ext}: no es un ZIP válido`).toBe('PK');
      } else {
        expect(buf.toString('utf8').toLowerCase(), `${caso.ext}: no parece del formato`).toContain(
          caso.ext === 'html' ? '<!doctype html>' : '#',
        );
      }

      // El menú queda abierto tras descargar (Dropdown solo cierra con Esc / clic
      // fuera); ciérralo y espera a que se desmonte para aislar cada iteración (si
      // no cerrase, el siguiente clic en el disparador reabriría en vez de abrir).
      await page.keyboard.press('Escape');
      await expect(page.getByRole('menu', { name: 'Descargar' })).toBeHidden();
    }
  });

  // Requiere REBUILD del contenedor `docs` (ver cabecera): con el binario nuevo el
  // export toma el contenido del registro doc:html y el fichero deja de salir vacío.
  test('el fichero exportado incluye el contenido escrito (requiere rebuild docs)', async ({
    page,
    request,
  }) => {
    test.slow();
    const title = `e2e-export-cont-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const id = await crearDocumento(request, title);

    await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });
    await expect(page.locator('#editor-title-input')).toBeVisible({ timeout: 60_000 });
    const folio = page.locator('.sytue-page');
    await expect(folio).toBeVisible();

    // Escribir contenido y esperar a que el autosave lo persista en el servidor
    // (de donde el backend materializa el export), además del guardado previo que
    // exportAs fuerza. Así el fichero nunca sale vacío (tarea §4).
    const MARCA = 'Contenido exportable de prueba';
    const opsOk = page.waitForResponse(
      (r) => r.url().includes('/api/ops') && r.request().method() === 'POST' && r.ok(),
      { timeout: 25_000 },
    );
    await folio.click();
    await page.keyboard.type(`${MARCA}.`);
    await expect(folio).toContainText(MARCA);
    await opsOk;

    await page.getByRole('button', { name: 'Descargar' }).click();
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByRole('menu', { name: 'Descargar' }).getByRole('menuitem', { name: /^HTML$/ }).click(),
    ]);
    const ruta = await download.path();
    expect(readFileSync(ruta, 'utf8')).toContain(MARCA);
  });
});
