// Sytue Calendar — eventos recurrentes (RRULE iCalendar).
// Demuestra la feature END-TO-END: el modal crea un evento diario con COUNT=4,
// el backend CalDAV guarda UN VEVENT maestro con su RRULE (NO expande), y la
// rejilla del mes pinta las 4 ocurrencias tras expandirlas en cliente.
import { expect, test, type Page } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

const BASE = APPS.calendar.replace(/\/$/, '');

// El dev-server (Astro/Vite) compila cada ruta /api/* en frío (20-30 s medido);
// presupuesto holgado por test para las pasadas frías del modal + reload.
const API_TIMEOUT = 60_000;
test.beforeEach(() => test.setTimeout(180_000));

interface Calendario {
  url: string;
  displayName: string;
}

/** Formato iCalendar UTC (yyyymmddThhmmssZ) que exige GET /api/events. */
function toIcalUtc(d: Date): string {
  return d.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z');
}

async function calendarioPersonal(page: Page): Promise<Calendario> {
  const res = await page.request.get(`${BASE}/api/calendars`, { timeout: API_TIMEOUT });
  expect(res.status(), 'GET /api/calendars debe responder 200').toBe(200);
  const { calendars } = (await res.json()) as { calendars: Calendario[] };
  const personal = calendars.find((c) => c.displayName === 'Personal') ?? calendars[0];
  expect(personal, 'debe existir al menos un calendario').toBeTruthy();
  return personal!;
}

test('evento diario COUNT=4: 1 maestro con RRULE en el backend, 4 ocurrencias en la rejilla', async ({
  page,
}) => {
  const summary = `Recurrente e2e ${Date.now()}-${test.info().workerIndex}`;
  const personal = await calendarioPersonal(page);

  await gotoApp(page, 'calendar');

  // --- Crea el evento recurrente desde el modal (vista mes, cursor = hoy) ---
  await page.getByRole('button', { name: 'Crear evento', exact: true }).click({ timeout: 60_000 });
  const modal = page.getByRole('dialog', { name: 'Nuevo evento' });
  await expect(modal).toBeVisible();

  await modal.locator('#ev-summary').fill(summary);
  await modal.locator('#ev-repeat').selectOption('DAILY');
  await modal.locator('#ev-repeat-end').selectOption('count');
  await modal.locator('#ev-repeat-count').fill('4');
  await modal.getByRole('button', { name: 'Guardar' }).click();
  await expect(modal).toBeHidden({ timeout: API_TIMEOUT });

  // --- La rejilla del mes pinta 4 ocurrencias (expansión en cliente) ---
  // Diario COUNT=4 desde hoy: 4 días consecutivos que SIEMPRE caben en la rejilla
  // de 6 semanas del mes vigente, sea cual sea la fecha (el último día del mes está
  // como mucho en el offset 36 de 42, +3 días = 39 < 42). Un semanal, en cambio,
  // podría dejar su 3ª/4ª instancia fuera de la ventana si el mes va avanzado.
  const chips = page.getByTitle(summary, { exact: true });
  await expect(chips, 'la rejilla debe mostrar 4 ocurrencias del evento diario').toHaveCount(4, {
    timeout: API_TIMEOUT,
  });

  // --- Auditoría del backend: NO expande; devuelve 1 VEVENT maestro con RRULE ---
  const now = Date.now();
  const lista = await page.request.get(`${BASE}/api/events`, {
    timeout: API_TIMEOUT,
    params: {
      calendarUrl: personal.url,
      start: toIcalUtc(new Date(now - 2 * 24 * 60 * 60 * 1000)),
      end: toIcalUtc(new Date(now + 40 * 24 * 60 * 60 * 1000)),
    },
  });
  expect(lista.status(), 'GET /api/events debe responder 200').toBe(200);
  const { events } = (await lista.json()) as { events: Array<{ summary: string; href: string; ical: string }> };
  const propios = events.filter((e) => e.summary === summary);
  expect(propios, 'el backend CalDAV devuelve UN maestro (no expande la serie)').toHaveLength(1);
  expect(propios[0]!.ical, 'el maestro conserva la RRULE diaria COUNT=4').toContain('RRULE:FREQ=DAILY;COUNT=4');

  // --- Limpieza: borra la serie (por el href del maestro) ---
  const borrado = await page.request.delete(`${BASE}/api/event`, {
    timeout: API_TIMEOUT,
    data: { href: propios[0]!.href },
  });
  expect.soft(borrado.ok(), 'DELETE /api/event debe limpiar la serie').toBe(true);
});
