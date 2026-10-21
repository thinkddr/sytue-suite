// e2e del triaje de Sytue Mail: estrella ($flagged), marcar no leído, archivar,
// papelera y «Mover a…».
//
// Ejercita el flujo completo UI → BFF (/api/message-actions, /api/seen) →
// servicio Go (POST /messages/{id}/flags | /move). La UI es optimista, así que
// cada acción se verifica también contra el servidor: la estrella sobrevive a un
// reload y cada movimiento se comprueba abriendo la carpeta destino (su lista se
// recarga del backend). Corre como admin dev (storageState, ver auth.setup.ts).
//
// Nota de localizadores: las acciones de fila existen en TODAS las filas (ocultas
// con opacity-0 hasta el hover), así que las acciones del lector se buscan dentro
// de su grupo «Organizar el correo» y las de fila dentro de su propia fila.
import { expect, test, type Page } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp } from './helpers';

/** Espera la próxima respuesta del BFF de triaje (estrella o movimiento). */
const esperarAccion = (page: Page) =>
  page.waitForResponse(
    (r) => r.url().includes('/api/message-actions') && r.request().method() === 'POST',
    { timeout: 30_000 },
  );

test('destacar, marcar no leído, archivar, papelera y «Mover a…»', async ({ page }) => {
  // Primer hit en frío: compilación del dev-server + entrega SMTP loopback.
  test.slow();
  const sinErrores = expectNoConsoleErrors(page);

  // Token de una sola palabra (un lexeme del FTS del servidor) para localizar el
  // correo sin ambigüedad en las listas.
  const token = `triaje${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`;
  const asunto = `Correo de triaje ${token}`;

  await gotoApp(page, 'mail');

  // 1) Sembrar: admin→admin se entrega al Inbox local por loopback.
  const envio = await page.evaluate(async (subject) => {
    const res = await fetch('/api/send', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        to: [{ email: 'admin@sytue.localhost' }],
        subject,
        textBody: 'Cuerpo del correo del e2e de triaje de Sytue Mail.',
      }),
    });
    return { status: res.status, text: await res.text() };
  }, asunto);
  expect(envio.status, `respuesta de /api/send: ${envio.text}`).toBe(200);

  // Fila de la conversación (role=option del listbox) y grupo de triaje del lector.
  const fila = page.getByRole('option').filter({ hasText: asunto });
  const triaje = page.getByRole('group', { name: 'Organizar el correo' });
  const lectorVacio = page.getByText('Selecciona una conversación');

  // 2) Esperar la entrega en Recibidos (la lista no hace polling: recargar).
  await expect(async () => {
    await gotoApp(page, 'mail');
    await expect(fila).toBeVisible({ timeout: 8_000 });
  }).toPass({ intervals: [2_000, 3_000, 3_000, 5_000], timeout: 90_000 });

  // 3) Estrella desde la fila (acción al hover) y persistencia tras reload:
  //    la UI es optimista; el reload demuestra que el backend guardó $flagged.
  await fila.hover();
  const respFlag = esperarAccion(page);
  await fila.getByRole('button', { name: 'Destacar' }).click();
  expect((await respFlag).status(), 'el BFF debe aceptar el toggle de estrella').toBe(200);
  await expect(fila.getByRole('img', { name: 'Destacado' })).toBeVisible();
  await gotoApp(page, 'mail');
  await expect(fila).toBeVisible({ timeout: 20_000 });
  await expect(
    fila.getByRole('img', { name: 'Destacado' }),
    'la estrella debe persistir tras recargar',
  ).toBeVisible();

  // 4) Marcar como no leído desde el lector: cierra el lector y la fila vuelve
  //    a mostrar el estado no leído (texto sr-only «No leído.» en el título).
  await fila.click();
  const marcarNoLeido = triaje.getByRole('button', { name: 'Marcar como no leído' });
  await expect(marcarNoLeido).toBeVisible({ timeout: 20_000 });
  const respUnseen = page.waitForResponse(
    (r) =>
      r.url().includes('/api/seen') &&
      r.request().method() === 'POST' &&
      (r.request().postData() ?? '').includes('"seen":false'),
    { timeout: 30_000 },
  );
  await marcarNoLeido.click();
  expect((await respUnseen).status(), 'el BFF debe aceptar marcar no leído').toBe(200);
  await expect(lectorVacio).toBeVisible();
  await expect(fila.getByText('No leído.')).toBeAttached();

  // 5) Archivar desde el lector: la fila sale de Recibidos y aparece en Archivo.
  await fila.click(); // reabrir (vuelve a marcarse leído)
  const archivar = triaje.getByRole('button', { name: 'Archivar' });
  await expect(archivar).toBeVisible({ timeout: 20_000 });
  const respArchivar = esperarAccion(page);
  await archivar.click();
  expect((await respArchivar).status(), 'el BFF debe aceptar archivar').toBe(200);
  await expect(fila).toBeHidden();
  await expect(lectorVacio).toBeVisible();
  await page.getByRole('button', { name: /^Archivo/ }).click();
  await expect(page.getByRole('heading', { name: 'Archivo', exact: true })).toBeVisible();
  await expect(fila, 'el correo debe estar en Archivo (movimiento real en el servidor)').toBeVisible({
    timeout: 20_000,
  });

  // 6) Papelera desde el lector (estando en Archivo): aparece en Papelera.
  await fila.click();
  const aPapelera = triaje.getByRole('button', { name: 'Mover a la papelera' });
  await expect(aPapelera).toBeVisible({ timeout: 20_000 });
  const respPapelera = esperarAccion(page);
  await aPapelera.click();
  expect((await respPapelera).status(), 'el BFF debe aceptar mover a papelera').toBe(200);
  await expect(fila).toBeHidden();
  await page.getByRole('button', { name: /^Papelera/ }).click();
  await expect(page.getByRole('heading', { name: 'Papelera', exact: true })).toBeVisible();
  await expect(fila, 'el correo debe estar en Papelera').toBeVisible({ timeout: 20_000 });

  // 7) «Mover a…» (menú de carpetas) desde la Papelera: vuelta a Recibidos. En la
  //    Papelera no hay acción «Mover a la papelera» (ya estamos en ella).
  await fila.click();
  const moverA = triaje.getByRole('button', { name: 'Mover a', exact: true });
  await expect(moverA).toBeVisible({ timeout: 20_000 });
  await expect(triaje.getByRole('button', { name: 'Mover a la papelera' })).toBeHidden();
  await moverA.click();
  const respMover = esperarAccion(page);
  await page.getByRole('menuitem', { name: 'Recibidos' }).click();
  expect((await respMover).status(), 'el BFF debe aceptar «Mover a…»').toBe(200);
  await page.getByRole('button', { name: /^Recibidos/ }).click();
  await expect(page.getByRole('heading', { name: 'Recibidos', exact: true })).toBeVisible();
  await expect(fila, 'el correo debe volver a Recibidos').toBeVisible({ timeout: 20_000 });
  // La estrella sobrevive a los movimientos de carpeta.
  await expect(fila.getByRole('img', { name: 'Destacado' })).toBeVisible();

  sinErrores();
});
