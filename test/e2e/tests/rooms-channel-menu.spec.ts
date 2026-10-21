// Spec e2e de Sytue Rooms — MENÚ DEL CANAL en su cabecera. Lo que se prueba es la
// DESCUBRIBILIDAD: el nombre del canal tiene que abrir sus acciones (Slack, Teams y
// Mattermost lo hacen así), porque hasta ahora era un <h1> inerte y todo vivía en el
// «…» del sidebar, que solo aparece al pasar el cursor.
import { expect, test } from '@playwright/test';
import { gotoApp } from './helpers';

test.describe('Sytue Rooms — menú del canal', () => {
  test('pulsar el nombre del canal abre su menú y desde ahí se llega a Miembros', async ({
    page,
  }) => {
    await gotoApp(page, 'rooms');
    await expect(page.getByLabel('Escribe un mensaje en General')).toBeVisible();

    // El nombre es el disparador; sigue siendo el <h1> de la vista.
    const disparador = page.getByRole('button', { name: /Menú del canal/ });
    await expect(disparador, 'el nombre del canal debe ser accionable').toBeVisible();
    await disparador.click();

    // Las puertas que antes no existían en ninguna parte de la cabecera.
    await expect(page.getByRole('menuitem', { name: 'Miembros' })).toBeVisible();
    await expect(page.getByRole('menuitem', { name: 'Fijados' })).toBeVisible();

    // Y llevan a alguna parte: «Miembros» abre su panel.
    await page.getByRole('menuitem', { name: 'Miembros' }).click();
    await expect(page.getByRole('complementary', { name: /Miembros/i })).toBeVisible({
      timeout: 15_000,
    });
  });
});
