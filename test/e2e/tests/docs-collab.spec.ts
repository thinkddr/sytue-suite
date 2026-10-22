// Spec e2e de colaboración del editor nativo de Sytue Docs: dos sesiones en
// contextos de navegador independientes (mismo usuario admin) sobre el MISMO
// documento. Cubre: presencia (avatares de la cabecera), comentarios (panel de
// la franja izquierda) e historial de versiones (pestaña Colaboración).
//
// Mecanismo REAL de presencia (packages/office/src/editor/state/collab.ts):
// heartbeat por conexión a PUT /api/presence cada ~10 s (TTL 30 s en el
// servicio Go, en memoria). Abrir el documento registra la pestaña, así que el
// LECTOR PASIVO también aparece; cada pestaña se excluye a sí misma y deduplica
// por usuario, de modo que dos sesiones del mismo admin se ven entre sí (≥1
// avatar). La sincronización de contenido sigue por sondeo de GET /api/ops.
import {
  expect,
  test,
  type APIRequestContext,
  type BrowserContext,
  type Page,
} from '@playwright/test';
import { STORAGE_STATE } from '../playwright.config';
import { APPS } from './helpers';

const DOCS = APPS.docs; // https://docus.sytue.localhost/

/** Crea un documento (kind document) por la API BFF y devuelve su id. */
async function crearDocumento(request: APIRequestContext): Promise<string> {
  const title = `e2e-collab-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const res = await request.post(`${DOCS}api/create`, { data: { kind: 'document', title } });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  expect(id).toBeTruthy();
  return id;
}

/** Abre /edit?id= y espera cabecera + folio (dev-server en frío: margen amplio). */
async function abrirDocumento(page: Page, id: string): Promise<void> {
  await page.goto(`${DOCS}edit?id=${id}`, { waitUntil: 'domcontentloaded' });
  await expect(page.locator('#editor-title-input')).toBeVisible({ timeout: 60_000 });
  await expect(page.locator('.sytue-page')).toBeVisible();
}

/** Escribe en el folio y espera el autosave (POST /api/ops OK, debounce 1500 ms). */
async function escribirYGuardar(page: Page, texto: string): Promise<void> {
  const guardado = page.waitForResponse(
    (r) => r.url().includes('/api/ops') && r.request().method() === 'POST' && r.ok(),
    { timeout: 20_000 },
  );
  await page.locator('.sytue-page').click();
  await page.keyboard.type(texto);
  await guardado;
}

/** Grupo de avatares de la cabecera: aria-label «N colaborador(es) conectado(s)». */
const avatares = (page: Page) =>
  page.getByRole('group', { name: /colaborador(es)? conectados?/ });

test.describe('Colaboración en Sytue Docs', () => {
  test('presencia: dos sesiones sobre el mismo documento muestran ≥1 avatar en ambas', async ({
    page,
    browser,
    request,
  }) => {
    test.slow();
    const id = await crearDocumento(request);

    // Segundo contexto logueado con el MISMO storageState del setup; las
    // opciones del config (ignoreHTTPSErrors, viewport) no se heredan en
    // browser.newContext() y se pasan a mano.
    let contextoB: BrowserContext | null = null;
    try {
      contextoB = await browser.newContext({
        storageState: STORAGE_STATE,
        ignoreHTTPSErrors: true,
        viewport: { width: 1440, height: 900 },
      });
      const pageB = await contextoB.newPage();

      await abrirDocumento(page, id);
      await abrirDocumento(pageB, id);

      // B escribe: su op es «ajena» para A (actor aleatorio por pestaña).
      await escribirYGuardar(pageB, 'Presencia desde B. ');

      // A adopta el contenido remoto (LWW, sondeo ≤4 s): sincronización real…
      await expect(page.locator('.sytue-page')).toContainText('Presencia desde B.', {
        timeout: 15_000,
      });
      // …y el heartbeat de presencia enciende ≥1 avatar en A (aparece con solo
      // abrir; escribir no es necesario, pero aquí ya prueba también el sync).
      await expect(avatares(page)).toBeVisible({ timeout: 15_000 });

      // A escribe DESPUÉS de adoptar (la recarga suave del lienzo —
      // contentEpoch— recolocaría el caret si llegara en pleno tecleo); B ya ve
      // el avatar de A por presencia y también recibe su op.
      await escribirYGuardar(page, 'Presencia desde A. ');
      await expect(avatares(pageB)).toBeVisible({ timeout: 15_000 });
    } finally {
      await contextoB?.close();
    }
  });

  test('presencia pasiva: un lector sin ediciones aparece como avatar', async ({
    page,
    browser,
    request,
  }) => {
    test.slow();
    const id = await crearDocumento(request);

    let contextoB: BrowserContext | null = null;
    try {
      contextoB = await browser.newContext({
        storageState: STORAGE_STATE,
        ignoreHTTPSErrors: true,
        viewport: { width: 1440, height: 900 },
      });
      const pageB = await contextoB.newPage();

      // Ambas sesiones SOLO abren el documento; ninguna escribe. La presencia
      // real por conexión (PUT /api/presence, heartbeat ~10 s) las registra
      // igual: el lector pasivo cuenta. Cada pestaña se excluye a sí misma, así
      // que ve a la otra (mismo usuario, ≥1 avatar).
      await abrirDocumento(page, id);
      await abrirDocumento(pageB, id);

      await expect(avatares(page)).toBeVisible({ timeout: 20_000 });
      await expect(avatares(pageB)).toBeVisible({ timeout: 20_000 });
    } finally {
      await contextoB?.close();
    }
  });

  test('comentarios: se añade desde el panel y queda listado', async ({ page, request }) => {
    test.slow();
    const id = await crearDocumento(request);
    await abrirDocumento(page, id);

    // Franja izquierda → botón «Comentarios» abre el panel.
    await page.getByRole('button', { name: 'Comentarios' }).click();
    const panel = page.getByRole('complementary', { name: 'Comentarios' });
    await expect(panel).toBeVisible();

    const texto = `Comentario e2e ${Date.now()}`;
    const creado = page.waitForResponse(
      (r) => r.url().includes('/api/comments') && r.request().method() === 'POST' && r.ok(),
      { timeout: 15_000 },
    );
    await panel.getByRole('textbox', { name: 'Nuevo comentario' }).fill(texto);
    await panel.getByRole('button', { name: 'Comentar' }).click();
    await creado;

    // El listado se refresca re-leyendo del servidor (addComment → POST →
    // re-GET): ver el comentario aquí prueba el round-trip completo.
    await expect(panel.getByRole('list', { name: 'Hilos de comentarios' })).toContainText(texto, {
      timeout: 10_000,
    });
  });

  test('versiones: crear una versión con etiqueta y verla en el historial', async ({
    page,
    request,
  }) => {
    test.slow();
    const id = await crearDocumento(request);
    await abrirDocumento(page, id);

    await page.getByRole('tab', { name: 'Colaboración' }).click();

    // «Guardar versión» pide la etiqueta con window.prompt: se acepta con ella.
    const etiqueta = `v-e2e-${Date.now()}`;
    page.once('dialog', (dialog) => void dialog.accept(etiqueta));
    const creada = page.waitForResponse(
      (r) => r.url().includes('/api/versions') && r.request().method() === 'POST' && r.ok(),
      { timeout: 15_000 },
    );
    await page.getByRole('button', { name: 'Guardar versión' }).click();
    await creada;

    // Abrir el historial (dropdown role=menu; al abrir re-carga la lista) y
    // ver la versión recién etiquetada.
    await page.getByRole('button', { name: 'Historial' }).click();
    await expect(page.getByRole('menuitem', { name: new RegExp(etiqueta) })).toBeVisible({
      timeout: 10_000,
    });
  });
});
