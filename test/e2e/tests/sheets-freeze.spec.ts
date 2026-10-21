// «Congelar paneles» de Sytue Sheets. Estaba rota de cinco formas a la vez y
// ninguna se veía en el código: cuatro eran clases concatenadas sin espacio
// (`w-max` + `sticky` = `w-maxsticky`, una clase que no existe y que además se
// lleva por delante la de al lado) y la quinta era el resaltado de selección,
// que es latón al 9 % — translúcido — y dejaba leer los números de la fila
// desplazada POR DEBAJO de la fila congelada.
//
// `scripts/lint-design.sh` ya impide que vuelvan las cuatro primeras. Esta spec
// cubre lo que un grep no puede ver: que al desplazar, la fila congelada siga
// en su sitio y tape lo de debajo.
import { expect, test } from '@playwright/test';
import { APPS } from './helpers';

test('congelar paneles deja la fila fija y opaca al desplazar', async ({ page }) => {
  const r = await page.request.post(`${APPS.docs}api/create`, {
    data: { kind: 'spreadsheet', title: `e2e-freeze-${Date.now()}` },
  });
  expect(r.status(), await r.text()).toBe(201);
  const { id } = (await r.json()) as { id: string };

  await page.goto(`${APPS.docs}edit?id=${id}`);
  await expect(page.locator('[role="grid"]').first()).toBeVisible({ timeout: 30_000 });

  // Un rótulo en A1 para reconocer la fila congelada tras desplazar.
  await page.locator('[role="gridcell"]').first().dblclick();
  await page.keyboard.type('ANCLA');
  await page.keyboard.press('Enter');

  await page.getByRole('tab', { name: /^(datos|data|daten|données|dati)$/i }).click();
  const congelar = page.locator('button[aria-label*="ongelar"], button[title*="ongelar"]').first();
  if ((await congelar.getAttribute('aria-pressed')) !== 'true') await congelar.click();
  await expect(congelar).toHaveAttribute('aria-pressed', 'true');

  await page.mouse.move(700, 500);
  await page.mouse.wheel(0, 600);
  await page.waitForTimeout(500);

  // 1) Sigue visible pese al desplazamiento: es lo que rompía `w-maxsticky`.
  const ancla = page.getByText('ANCLA', { exact: true }).first();
  await expect(ancla).toBeVisible();
  const fila = ancla.locator('xpath=ancestor::*[@role="row"][1]');
  await expect(fila).toHaveCSS('position', 'sticky');

  // 2) Y tapa lo de debajo: sin fondo opaco se leían dos números a la vez.
  const opaca = await fila.evaluate((el) => {
    const bg = getComputedStyle(el).backgroundColor;
    // `rgb(...)` trae 3 componentes y es opaco; `rgba(...)` y `color(srgb ... / a)`
    // traen 4, y el cuarto es el alfa (el resaltado de selección es 0.09).
    const n = bg.match(/[\d.]+/g) ?? [];
    return bg !== 'transparent' && (n.length < 4 || Number(n[3]) === 1);
  });
  expect(opaca, 'la fila congelada flota sobre otras: necesita fondo opaco').toBe(true);
});
