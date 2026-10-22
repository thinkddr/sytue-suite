// e2e de la búsqueda de Sytue Mail (Email/query filter.text end-to-end):
// envía un correo con asunto único al propio admin, espera a que se entregue al
// Inbox local y lo encuentra escribiendo en la barra de búsqueda de la app.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp } from './helpers';

test.describe('Sytue Mail — búsqueda', () => {
  test('envía, encuentra por asunto y muestra «sin resultados»', async ({ page }) => {
    // Primer hit en frío: el dev-server compila /api/send y /api/search antes de
    // responder, y la entrega SMTP local tarda unos segundos: triplica el timeout.
    test.slow();
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'mail');

    // Token único de una sola palabra (un lexeme para el FTS 'simple' del servidor,
    // sin espacios ni guiones): así filter.text lo casa exacto contra el asunto.
    const token = `sytuebusqueda${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`;
    const asunto = `Prueba de búsqueda ${token}`;

    // 1) Enviar al propio admin (se entrega al Inbox local). fetch same-origin:
    //    viaja la cookie de sesión del BFF, sin tokens en el test.
    const envio = await page.evaluate(async (subject) => {
      const res = await fetch('/api/send', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          to: [{ email: 'admin@sytue.localhost' }],
          subject,
          textBody: 'Cuerpo del correo de prueba de la búsqueda e2e de Sytue Mail.',
        }),
      });
      return { status: res.status, text: await res.text() };
    }, asunto);
    expect(envio.status, `respuesta de /api/send: ${envio.text}`).toBe(200);

    // 2) Esperar la entrega consultando el BFF de búsqueda (Email/query filter.text).
    //    Esto ya ejercita SDK → BFF → servicio Go de correo end-to-end.
    await expect
      .poll(
        () =>
          page.evaluate(async (q) => {
            const res = await fetch(`/api/search?q=${encodeURIComponent(q)}`, {
              credentials: 'same-origin',
              headers: { accept: 'application/json' },
            });
            if (!res.ok) return -1;
            const body = (await res.json()) as { emails?: unknown[] };
            return body.emails?.length ?? 0;
          }, token),
        {
          message: 'el correo enviado debe aparecer en /api/search tras entregarse',
          timeout: 60_000,
          intervals: [1000, 2000, 3000],
        },
      )
      .toBeGreaterThan(0);

    // 3) UI: escribir el token en la barra de búsqueda muestra el resultado.
    const buscador = page.getByRole('searchbox', { name: 'Buscar en el correo' });
    const cabeceraResultados = page.getByRole('heading', { name: 'Resultados de búsqueda' });
    await expect(buscador).toBeVisible({ timeout: 15_000 });
    await buscador.fill(token);
    // La cabecera confirma que la vista pasó a modo búsqueda (no que el asunto
    // casara por casualidad en la carpeta) y luego aparece el asunto único (puede
    // haber copia en Enviados con el mismo asunto: basta el primero).
    await expect(cabeceraResultados).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(asunto).first()).toBeVisible({ timeout: 15_000 });

    // 4) Limpiar no solo vacía el input: sale del modo búsqueda y vuelve a la carpeta.
    await page.getByRole('button', { name: 'Limpiar búsqueda' }).click();
    await expect(buscador).toHaveValue('');
    await expect(cabeceraResultados).toBeHidden();

    // 5) Estado vacío: un término inexistente muestra «Sin resultados».
    await buscador.fill(`inexistente${crypto.randomUUID().replace(/-/g, '').slice(0, 12)}`);
    await expect(page.getByText('Sin resultados', { exact: false })).toBeVisible({ timeout: 15_000 });

    sinErrores();
  });
});
