// e2e de Responder / Responder a todos / Reenviar en Sytue Mail.
//
// Es una feature de UI sobre el flujo /api/send ya existente (no requiere backend
// nuevo ni rebuild de Go): el lector pre-rellena el compositor. El spec siembra un
// correo a admin (con una copia/Cc), espera su entrega en Recibidos y luego, en
// el lector, comprueba que cada acción rellena el compositor como toca:
//   • Responder        → Para=remitente, Asunto="Re: …", cuerpo citado (> …).
//   • Responder a todos → añade a los demás destinatarios en CC.
//   • Reenviar         → Asunto="Fwd: …", Para vacío, original citado.
// Y que Responder/Reenviar se ENVÍAN de verdad por /api/send y llegan al buzón.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp } from './helpers';

const ADMIN = 'admin@sytue.localhost';
// CC externo (dominio reservado RFC 2606 .invalid): en dev solo admin@ tiene buzón
// local, así que un CC local no provisto rebotaría y abortaría también la entrega a
// admin (mismo lote por dominio). Un CC externo se encola en un lote de dominio
// aparte (falla su entrega en segundo plano, inofensivo) mientras la copia de admin
// se entrega igual con la cabecera Cc guardada — que es lo que "Responder a todos" lee.
const CC_EXT = 'equipo-e2e@example.invalid';

/** Envía por el BFF (cookie de sesión, sin tokens en el test). Devuelve el estado. */
async function enviar(
  page: import('@playwright/test').Page,
  payload: Record<string, unknown>,
): Promise<{ status: number; text: string }> {
  return page.evaluate(async (body) => {
    const res = await fetch('/api/send', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
    return { status: res.status, text: await res.text() };
  }, payload);
}

test('Responder, Responder a todos y Reenviar prerellenan y envían por /api/send', async ({ page }) => {
  // Primer hit en frío: el dev-server compila páginas + rutas API y hay saltos de
  // entrega loopback de por medio. Amplía el margen (como el resto de specs de mail).
  test.slow();
  const sinErrores = expectNoConsoleErrors(page);

  // Token de una sola palabra (un lexeme para el FTS 'simple' del servidor): así
  // filter.text lo casa exacto contra el asunto del original, la respuesta y el reenvío.
  const token = `rf${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`;
  const asunto = `Hilo e2e ${token}`;
  const cuerpo = `Mensaje original ${token} para citar. Un saludo.`;

  await gotoApp(page, 'mail');

  // 1) Sembrar el hilo: admin→admin con copia (Cc) a una dirección externa, para
  //    probar el CC de "Responder a todos". Se entrega al Inbox local por loopback.
  const semilla = await enviar(page, {
    to: [{ email: ADMIN }],
    cc: [{ email: CC_EXT }],
    subject: asunto,
    textBody: cuerpo,
  });
  expect(semilla.status, `respuesta de /api/send (semilla): ${semilla.text}`).toBe(200);

  // 2) Esperar la entrega en Recibidos (la lista no hace polling: recargar) y abrir.
  //    `fila` se acota a la región de la lista para no chocar con el <h1> del lector
  //    (que muestra el mismo asunto una vez abierto el correo).
  const listaConv = page.getByRole('region', { name: /^Conversaciones/ });
  const fila = listaConv.getByText(asunto, { exact: true });
  await expect(async () => {
    await gotoApp(page, 'mail');
    await expect(fila).toBeVisible({ timeout: 8_000 });
  }).toPass({ intervals: [2_000, 3_000, 3_000, 5_000], timeout: 90_000 });
  await fila.click();

  const dialog = page.getByRole('dialog');
  const to = page.locator('#compose-to');
  const cc = page.locator('#compose-cc');
  const subject = page.locator('#compose-subject');

  // Tras ENVIAR, la lista se refresca y el lector se deselecciona (de forma asíncrona):
  // para encadenar otra acción, recargamos a un estado limpio y reabrimos el original.
  // (Descartar, en cambio, no envía ni refresca: mantiene el lector abierto.)
  async function abrirOriginal(): Promise<void> {
    await gotoApp(page, 'mail');
    await expect(fila).toBeVisible({ timeout: 20_000 });
    await fila.click();
  }

  const esperarEnvio = () =>
    page.waitForResponse(
      (r) => r.url().includes('/api/send') && r.request().method() === 'POST',
      { timeout: 120_000 },
    );

  // 3) Responder a todos: Para=remitente (admin), Asunto "Re: …" y el otro
  //    destinatario (el Cc del original) en CC. Descartar mantiene abierto el lector.
  await page.getByRole('button', { name: 'Responder a todos', exact: true }).click();
  await expect(subject).toBeVisible({ timeout: 20_000 });
  await expect(to).toHaveValue(new RegExp(ADMIN.replace(/[.]/g, '\\$&')));
  await expect(subject).toHaveValue(`Re: ${asunto}`);
  await expect(cc).toHaveValue(new RegExp(CC_EXT.replace(/[.]/g, '\\$&')));
  await dialog.getByRole('button', { name: 'Descartar' }).click();
  await expect(dialog).toBeHidden();

  // 4) Responder: Para=remitente, Asunto "Re: …", cuerpo citado (> …) con la
  //    cabecera de atribución. Se ENVÍA por /api/send.
  await page.getByRole('button', { name: 'Responder', exact: true }).click();
  await expect(subject).toBeVisible({ timeout: 20_000 });
  await expect(to).toHaveValue(new RegExp(ADMIN.replace(/[.]/g, '\\$&')));
  await expect(subject).toHaveValue(`Re: ${asunto}`);
  const cuerpoRe = await page.locator('#compose-body').inputValue();
  expect(cuerpoRe, 'la respuesta debe citar el original con "> "').toContain('> ');
  expect(cuerpoRe).toContain(token);
  expect(cuerpoRe).toContain('escribió:');
  const enviadoRe = esperarEnvio();
  await dialog.getByRole('button', { name: 'Enviar' }).click();
  expect((await enviadoRe).status(), 'la respuesta debe enviarse por /api/send').toBe(200);
  await expect(dialog).toBeHidden({ timeout: 15_000 });

  // 5) Reenviar: Asunto "Fwd: …", Para vacío, original citado. Reabrimos el original
  //    (el envío anterior lo deselecccionó), rellenamos el destinatario y se ENVÍA.
  await abrirOriginal();
  await page.getByRole('button', { name: 'Reenviar', exact: true }).click();
  await expect(subject).toBeVisible({ timeout: 20_000 });
  await expect(subject).toHaveValue(`Fwd: ${asunto}`);
  await expect(to).toHaveValue('');
  const cuerpoFwd = await page.locator('#compose-body').inputValue();
  expect(cuerpoFwd, 'el reenvío debe citar el original').toContain(token);
  expect(cuerpoFwd).toContain('escribió:');
  await to.fill(ADMIN); // Reenviar exige rellenar el destinatario (Para venía vacío).
  const enviadoFwd = esperarEnvio();
  await dialog.getByRole('button', { name: 'Enviar' }).click();
  expect((await enviadoFwd).status(), 'el reenvío debe enviarse por /api/send').toBe(200);
  await expect(dialog).toBeHidden({ timeout: 15_000 });

  // 6) Ambos llegan al buzón: la respuesta ("Re: …") y el reenvío ("Fwd: …")
  //    aparecen en /api/search (SDK → BFF → servicio Go end-to-end).
  await expect
    .poll(
      () =>
        page.evaluate(async (q) => {
          const res = await fetch(`/api/search?q=${encodeURIComponent(q)}`, {
            credentials: 'same-origin',
            headers: { accept: 'application/json' },
          });
          if (!res.ok) return { re: false, fwd: false };
          const body = (await res.json()) as { emails?: Array<{ subject?: string }> };
          const asuntos = (body.emails ?? []).map((e) => e.subject ?? '');
          return {
            re: asuntos.some((s) => /^Re:/i.test(s)),
            fwd: asuntos.some((s) => /^Fwd:/i.test(s)),
          };
        }, token),
      {
        message: 'la respuesta (Re:) y el reenvío (Fwd:) deben entregarse',
        timeout: 60_000,
        intervals: [1000, 2000, 3000],
      },
    )
    .toEqual({ re: true, fwd: true });

  sinErrores();
});
