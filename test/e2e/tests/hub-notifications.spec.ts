// E2E: notificaciones reales en la campana del Hub (Sytue Shell).
//
// Emite una notificación al PROPIO usuario vía el BFF de prueba del portal
// (POST /api/notify-self → proxy autenticado del POST /notifications del Hub) y
// verifica end-to-end: el contador de la campana, el empuje EN VIVO por SSE, el
// listado en el popover (título, cuerpo, tiempo relativo) y el "marcar como
// leída" (el contador baja). docs/CONVENTIONS §11.
import { randomUUID } from 'node:crypto';
import { expect, test } from '@playwright/test';
import { APPS } from './helpers';

/** Nº de no leídas leído del aria-label de la campana ("… (N sin leer)"). */
function noLeidas(ariaLabel: string | null): number {
  const m = (ariaLabel ?? '').match(/\((\d+) sin leer\)/);
  return m ? Number(m[1]) : 0;
}

test('la campana del Hub cuenta, lista, empuja en vivo y marca leídas', async ({ page }) => {
  // Token único por ejecución (incluye UUID): con --repeat-each dos corridas no
  // colisionan ni se ven la notificación persistida de la otra (filtro por texto).
  const token = randomUUID();
  const titulo = `Prueba e2e ${token}`;
  const cuerpo = `Cuerpo de la notificación ${token}`;

  // Abrir una app de la suite (el portal monta la shell con la campana). La shell
  // abre el stream SSE al montarse. Registramos la espera de su respuesta ANTES de
  // navegar: el Hub solo vuelca las cabeceras + `ready` tras suscribir la conexión
  // (services/hub …/stream.go: Subscribe → ready → Flush), así que cuando esta
  // promesa resuelve la suscripción ya está viva y la emisión posterior se reparte
  // EN VIVO por SSE — sin carrera y sin dormir.
  const streamSuscrito = page.waitForResponse(
    (r) => r.url().includes('/notifications/stream') && r.ok(),
    { timeout: 30_000 },
  );
  await page.goto(APPS.one, { waitUntil: 'domcontentloaded' });
  const campana = page.getByRole('button', { name: /Notificaciones/ });
  await expect(campana).toBeVisible();

  // Abrir el popover. La campana es una isla Preact (client:load): reintentar
  // el clic hasta que hidrate y abra (sin cerrar uno ya abierto).
  const popover = page.getByRole('menu', { name: 'Notificaciones' });
  await expect(async () => {
    if (!(await popover.isVisible())) await campana.click();
    await expect(popover).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: 20_000 });
  // Esperar a que acabe la carga inicial (sin skeleton) para un contador estable.
  await expect(popover.locator('[aria-busy="true"]')).toHaveCount(0, { timeout: 15_000 });
  const antes = noLeidas(await campana.getAttribute('aria-label'));

  // La suscripción SSE ya está viva (su respuesta llegó): lo que emitamos ahora
  // se repartirá en vivo, sin depender de la latencia de apertura del popover.
  await streamSuscrito;

  // Emitir una notificación al propio usuario (proxy BFF autenticado del Hub).
  const res = await page.request.post(`${APPS.one}api/notify-self`, {
    data: { title: titulo, body: cuerpo },
  });
  expect(res.status()).toBe(201);

  // EN VIVO por SSE: aparece en el popover abierto, sin recargar la página.
  const fila = popover.locator('li', { hasText: titulo });
  await expect(fila).toBeVisible({ timeout: 15_000 });
  await expect(fila).toContainText(cuerpo);
  // Tiempo relativo renderizado — NO "Invalid Date" (prueba el campo created_at).
  await expect(fila.locator('time')).toBeVisible();
  await expect(fila.locator('time')).not.toContainText('Invalid');

  // El contador de la campana sube en vivo (SSE), sin recargar.
  await expect
    .poll(async () => noLeidas(await campana.getAttribute('aria-label')), { timeout: 15_000 })
    .toBe(antes + 1);

  // Marcar como leída (sin url → no navega; el popover sigue abierto): el
  // contador baja de nuevo.
  await fila.click();
  await expect
    .poll(async () => noLeidas(await campana.getAttribute('aria-label')), { timeout: 10_000 })
    .toBe(antes);
});
