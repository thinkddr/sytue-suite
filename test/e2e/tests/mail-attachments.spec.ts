// e2e de adjuntos en Sytue Mail: redacta un correo a admin@sytue.localhost con un
// adjunto .txt, lo envía, espera la entrega en Recibidos (loopback), abre el
// correo y verifica que el adjunto aparece y se descarga con el contenido correcto.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { gotoApp, sidebarNav } from './helpers';

test('redactar con adjunto → enviar → leer y descargar el adjunto', async ({ page }) => {
  // Primer hit en frío: el dev-server compila páginas + rutas API (compose, send,
  // attachments) y hay un salto de entrega loopback de por medio. Amplía el margen.
  test.slow();

  const marca = `${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
  const asunto = `e2e adjunto ${marca}`;
  const nombreAdjunto = `saludo-${marca}.txt`;
  const contenidoAdjunto = `Contenido del adjunto e2e ${marca}. áéíóú ñ.\n`;

  await gotoApp(page, 'mail');

  // --- Redactar --- (el botón de la sidebar; hay otro en el estado vacío del lector)
  await sidebarNav(page).getByRole('button', { name: 'Redactar' }).click();
  const modal = page.getByRole('dialog');
  await expect(modal.locator('#compose-to')).toBeVisible({ timeout: 20_000 });
  await modal.locator('#compose-to').fill('admin@sytue.localhost');
  await modal.locator('#compose-subject').fill(asunto);
  await modal.locator('#compose-body').fill('Cuerpo del correo con adjunto (e2e).');

  // Adjuntar un .txt generado (buffer en memoria, sin fichero temporal).
  await modal.locator('input[type=file]').setInputFiles({
    name: nombreAdjunto,
    mimeType: 'text/plain',
    buffer: Buffer.from(contenidoAdjunto, 'utf8'),
  });
  // El chip del adjunto aparece con su nombre en la lista de adjuntos.
  await expect(modal.getByText(nombreAdjunto, { exact: true })).toBeVisible({ timeout: 10_000 });

  // --- Enviar ---
  const enviado = page.waitForResponse(
    (r) => r.url().includes('/api/send') && r.request().method() === 'POST',
    { timeout: 120_000 },
  );
  await modal.getByRole('button', { name: 'Enviar' }).click();
  const resSend = await enviado;
  expect(resSend.status(), 'el BFF /api/send debe responder 200').toBe(200);
  // El modal se cierra al enviar con éxito.
  await expect(page.getByRole('dialog')).toBeHidden({ timeout: 15_000 });

  // --- Esperar la entrega en Recibidos (loopback ~pocos segundos) y abrir ---
  const fila = page.getByText(asunto, { exact: true });
  await expect(async () => {
    await gotoApp(page, 'mail'); // recarga: la lista de Recibidos no hace polling
    await expect(fila).toBeVisible({ timeout: 8_000 });
  }).toPass({ intervals: [2_000, 3_000, 3_000, 5_000], timeout: 90_000 });
  await fila.click();

  // --- El adjunto aparece en el lector ---
  const enlaceAdjunto = page.getByRole('link', { name: new RegExp(nombreAdjunto.replace(/[.\-]/g, '\\$&')) });
  await expect(enlaceAdjunto).toBeVisible({ timeout: 20_000 });

  // --- Descargar y verificar el contenido ---
  const descarga = page.waitForEvent('download', { timeout: 30_000 });
  await enlaceAdjunto.click();
  const download = await descarga;
  expect(download.suggestedFilename()).toBe(nombreAdjunto);
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const c of stream) chunks.push(Buffer.from(c));
  expect(Buffer.concat(chunks).toString('utf8')).toBe(contenidoAdjunto);
});
