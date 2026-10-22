// e2e de Sytue Rooms — Hilos y menciones. Sobre el canal General auto-aprovisionado:
// abre un mensaje como hilo, responde y comprueba que la respuesta aparece y que el
// contador de respuestas sube; menciona a un miembro con el selector «@» del composer
// y comprueba que la mención se resalta en el mensaje enviado.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp, APPS } from './helpers';

const BASE = APPS.rooms.replace(/\/$/, '');

/** Escapa una cadena para usarla literal dentro de un RegExp. */
function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test.describe('Sytue Rooms — hilos y menciones', () => {
  test('abre un mensaje como hilo, responde y el contador sube', async ({ page }) => {
    await gotoApp(page, 'rooms');

    // El composer del canal activo (General) confirma que la isla montó.
    const composer = page.getByLabel('Escribe un mensaje en General');
    await expect(composer).toBeVisible();

    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const rootBody = `hilo raíz ${stamp}`;
    const replyBody = `respuesta hilo ${stamp}`;

    // 1) Publica el mensaje raíz por el composer.
    const envioRaiz = page.waitForResponse(
      (r) => r.url().includes('/api/send') && r.request().method() === 'POST',
    );
    await composer.fill(rootBody);
    await composer.press('Enter');
    expect((await envioRaiz).status(), 'POST /api/send debe responder 200').toBe(200);

    // 2) Abre el mensaje como hilo desde la barra de acciones (aparece al pasar el
    //    cursor; el botón solo existe cuando el mensaje ya está persistido).
    const fila = page.locator('.group', { hasText: rootBody });
    const abrirHilo = fila.getByRole('button', { name: 'Responder en un hilo' });
    await expect(abrirHilo).toBeAttached();
    await fila.hover();
    const cargaHilo = page.waitForResponse(
      (r) => r.url().includes('/api/thread') && r.request().method() === 'GET',
    );
    await abrirHilo.click();

    // El panel de hilo se abre y muestra la raíz.
    const panel = page.getByRole('complementary', { name: 'Hilo' });
    await expect(panel).toBeVisible();
    await expect(panel.getByText(rootBody)).toBeVisible();
    // Espera a que el hilo termine de cargar antes de responder (evita la carrera
    // carga-del-hilo vs. envío-de-respuesta).
    expect((await cargaHilo).status(), 'GET /api/thread debe responder 200').toBe(200);

    // 3) Responde dentro del hilo.
    const reply = panel.getByLabel('Escribe una respuesta en el hilo');
    const envioReply = page.waitForResponse(
      (r) => r.url().includes('/api/reply') && r.request().method() === 'POST',
    );
    await reply.fill(replyBody);
    await reply.press('Enter');
    expect((await envioReply).status(), 'POST /api/reply debe responder 200').toBe(200);

    // 4) La respuesta aparece en el hilo…
    await expect(panel.getByText(replyBody), 'la respuesta no aparece en el hilo').toBeVisible();
    // …y el contador de respuestas sube en el mensaje raíz del histórico.
    await expect(
      fila.getByRole('button', { name: /respuestas? *$/ }),
      'el contador de respuestas no subió en el histórico',
    ).toBeVisible();
    await expect(fila).toContainText('1 respuesta');
  });

  test('menciona a un miembro con el selector «@» y se resalta', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'rooms');

    // Nombre real del miembro a mencionar (el BFF lo rellena desde la sesión).
    const membersRes = await page.request.get(`${BASE}/api/members`);
    expect(membersRes.status(), 'GET /api/members debe responder 200').toBe(200);
    const { members } = (await membersRes.json()) as { members: { sub: string; name: string }[] };
    expect(members.length, 'no hay miembros para mencionar').toBeGreaterThan(0);
    const nombre = members[0]!.name;

    const composer = page.getByLabel('Escribe un mensaje en General');
    await expect(composer).toBeVisible();
    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

    // Escribe hasta «@»: se despliega el selector de miembros del canal.
    await composer.click();
    await composer.pressSequentially(`aviso ${stamp} @`);
    const listbox = page.getByRole('listbox', { name: 'Personas a las que mencionar' });
    await expect(listbox, 'el selector de menciones no apareció al escribir «@»').toBeVisible();

    // Elige el miembro: inserta «@Nombre » en el texto del composer.
    await listbox.getByText(nombre, { exact: true }).click();
    await expect(composer).toHaveValue(new RegExp(`@${escapeRe(nombre)} `));

    // Envía el mensaje.
    const envio = page.waitForResponse(
      (r) => r.url().includes('/api/send') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Enviar', exact: true }).click();
    const res = await envio;
    expect(res.status(), 'POST /api/send debe responder 200').toBe(200);
    // El backend persiste la mención (mentions[] con el sub del miembro).
    const { message } = (await res.json()) as { message: { mentions?: string[] } };
    expect(message.mentions, 'el mensaje no persistió la mención').toContain(members[0]!.sub);

    // El mensaje enviado resalta la mención (@Nombre en acento).
    const fila = page.locator('.group', { hasText: stamp });
    const mencion = fila.locator('[data-mention]');
    await expect(mencion, 'la mención no se resaltó en el mensaje').toBeVisible();
    await expect(mencion).toHaveText(`@${nombre}`);
    sinErrores();
  });
});
