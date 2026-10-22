// Sytue Calendar — API BFF (calendarios/eventos CalDAV) y rejilla mensual.
// La storageState solo trae la sesión del IdP: la primera navegación a la app
// completa el baile OIDC (redirect SSR) y deja la cookie BFF en el contexto,
// que page.request comparte para llamar a /api/* autenticado.
import { expect, test, type Page } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

const BASE = APPS.calendar.replace(/\/$/, '');

// El dev-server (Astro/Vite) compila cada ruta /api/* a demanda: la primera
// petición en frío tarda 20-30 s (medido). Presupuesto holgado por test y por
// petición para que las pasadas frías no revienten el timeout global de 45 s.
const API_TIMEOUT = 60_000;
test.beforeEach(() => test.setTimeout(120_000));

interface Calendario {
  url: string;
  displayName: string;
}

/** Formato iCalendar UTC (yyyymmddThhmmssZ) que exige GET /api/events. */
function toIcalUtc(d: Date): string {
  return d.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z');
}

/** Establece sesión en la app y devuelve la lista de calendarios (200 garantizado). */
async function listarCalendarios(page: Page): Promise<Calendario[]> {
  await gotoApp(page, 'calendar');
  const res = await page.request.get(`${BASE}/api/calendars`, { timeout: API_TIMEOUT });
  expect(res.status(), 'GET /api/calendars debe responder 200').toBe(200);
  const body = (await res.json()) as { calendars: Calendario[] };
  expect(Array.isArray(body.calendars)).toBe(true);
  return body.calendars;
}

test('GET /api/calendars responde 200 e incluye el calendario Personal', async ({ page }) => {
  const calendarios = await listarCalendarios(page);
  expect(calendarios.map((c) => c.displayName)).toContain('Personal');
});

test('POST /api/event crea un evento y GET /api/events lo devuelve en el rango', async ({ page }) => {
  const calendarios = await listarCalendarios(page);
  const personal = calendarios.find((c) => c.displayName === 'Personal') ?? calendarios[0];
  expect(personal, 'debe existir al menos un calendario').toBeTruthy();

  // Datos únicos para no chocar con otros tests ni con re-ejecuciones
  // (workerIndex: dos repeticiones pueden correr en paralelo con workers=2).
  const summary = `Evento e2e ${Date.now()}-${test.info().workerIndex}`;
  const start = new Date(Date.now() + 60 * 60 * 1000);
  const end = new Date(start.getTime() + 30 * 60 * 1000);

  const creado = await page.request.post(`${BASE}/api/event`, {
    timeout: API_TIMEOUT,
    data: {
      calendarUrl: personal!.url,
      eventInput: { summary, start: start.toISOString(), end: end.toISOString() },
    },
  });
  expect(creado.status(), 'POST /api/event debe responder 200').toBe(200);
  const cuerpo = (await creado.json()) as { ok: boolean; href: string; uid: string };
  expect(cuerpo.ok).toBe(true);
  expect(cuerpo.href).toBeTruthy();

  try {
    // El rango (iCal UTC) que envuelve al evento debe devolverlo.
    const lista = await page.request.get(`${BASE}/api/events`, {
      timeout: API_TIMEOUT,
      params: {
        calendarUrl: personal!.url,
        start: toIcalUtc(new Date(start.getTime() - 60 * 60 * 1000)),
        end: toIcalUtc(new Date(end.getTime() + 60 * 60 * 1000)),
      },
    });
    expect(lista.status(), 'GET /api/events debe responder 200').toBe(200);
    const { events } = (await lista.json()) as { events: Array<{ summary: string }> };
    expect(events.map((e) => e.summary)).toContain(summary);
  } finally {
    // Limpieza incluso si la aserción anterior falla: sin residuos en el
    // calendario dev. `soft` para no enmascarar el error original del try.
    const borrado = await page.request.delete(`${BASE}/api/event`, {
      timeout: API_TIMEOUT,
      data: { href: cuerpo.href },
    });
    expect.soft(borrado.ok(), 'DELETE /api/event debe limpiar el evento').toBe(true);
  }
});

test('la vista de mes pinta la rejilla con las celdas de los días', async ({ page }) => {
  await gotoApp(page, 'calendar');
  // Cada celda del mes es un botón «Crear evento el 3 de julio» (día + mes,
  // ver dayMonthLabel en apps/calendar); la rejilla siempre trae 6×7 = 42
  // (monthGrid es de longitud fija, independiente del mes en curso).
  const celdas = page.getByRole('button', { name: /^Crear evento el \d+ de \p{L}+$/u });
  await expect(celdas).toHaveCount(42, { timeout: 60_000 });
  await expect(celdas.first()).toBeVisible();
});
