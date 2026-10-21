// e2e de Sytue Rooms — Editar y borrar mensajes propios. Sobre el canal General
// auto-aprovisionado: envía un mensaje, lo edita (nuevo texto + marca «(editado)»)
// y lo borra (con confirmación), comprobando que el cuerpo original desaparece.
// Corre autenticado como admin dev (storageState del proyecto, ver auth.setup.ts).
import { expect, test } from '@playwright/test';
import { expectNoConsoleErrors, gotoApp } from './helpers';

test.describe('Sytue Rooms — editar y borrar mensajes propios', () => {
  test('envía, edita (muestra «(editado)») y borra un mensaje propio', async ({ page }) => {
    const sinErrores = expectNoConsoleErrors(page);
    await gotoApp(page, 'rooms');

    // El composer del canal activo (General) confirma que la isla montó.
    const composer = page.getByLabel('Escribe un mensaje en General');
    await expect(composer).toBeVisible();

    // Marca estable presente en el original Y en la edición: permite re-localizar la
    // fila del mensaje aunque cambie el cuerpo (el sufijo aleatorio cubre paralelismo).
    const marca = `e2e-ed-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const original = `${marca} original`;
    const editado = `${marca} editado`;

    // 1) Publica el mensaje por el composer.
    const envio = page.waitForResponse(
      (r) => r.url().includes('/api/send') && r.request().method() === 'POST',
    );
    await composer.fill(original);
    await composer.press('Enter');
    expect((await envio).status(), 'POST /api/send debe responder 200').toBe(200);

    const fila = page.locator('.group', { hasText: marca });
    await expect(fila, 'el mensaje enviado no aparece en el histórico').toContainText(original);

    // 2) Edita el mensaje desde el MENÚ de acciones (aparece al pasar el cursor;
    //    editar/borrar solo se ofrecen en los mensajes PROPIOS).
    const acciones = fila.getByRole('button', { name: 'Más acciones' });
    await expect(acciones, 'falta el menú «Más acciones» en el mensaje propio').toBeAttached();
    await fila.hover();
    await acciones.click();
    await page.getByRole('menuitem', { name: 'Editar' }).click();

    // El editor inline reemplaza el cuerpo: solo hay uno abierto a la vez (page-level).
    const editor = page.getByRole('textbox', { name: 'Editar el mensaje' });
    await expect(editor).toBeVisible();
    await editor.fill(editado);

    const edicion = page.waitForResponse(
      (r) => new URL(r.url()).pathname.endsWith('/api/message') && r.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Guardar' }).click();
    expect((await edicion).status(), 'POST /api/message (editar) debe responder 200').toBe(200);

    // 3) La fila muestra el nuevo texto, la marca «(editado)», y ya no el original.
    await expect(fila, 'el cuerpo editado no se refleja').toContainText(editado);
    await expect(
      fila.getByText('(editado)'),
      'falta la marca «(editado)» tras editar',
    ).toBeVisible();
    await expect(fila, 'el cuerpo original sigue visible tras editar').not.toContainText('original');

    // 4) Borra el mensaje: acepta la confirmación nativa antes de disparar el DELETE.
    page.on('dialog', (d) => void d.accept());
    const borrado = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname.endsWith('/api/message') &&
        r.request().method() === 'POST' &&
        (r.request().postData() ?? '').includes('"delete"'),
    );
    await fila.hover();
    await fila.getByRole('button', { name: 'Más acciones' }).click();
    await page.getByRole('menuitem', { name: 'Borrar' }).click();
    expect((await borrado).status(), 'POST /api/message (borrar) debe responder 200').toBe(200);

    // 5) El mensaje desaparece del histórico (borrado suave en el backend): ni la
    //    fila ni el cuerpo editado siguen presentes.
    await expect(
      page.locator('.group', { hasText: marca }),
      'el mensaje borrado sigue en el histórico',
    ).toHaveCount(0);
    await expect(page.getByText(editado)).toHaveCount(0);

    sinErrores();
  });
});
