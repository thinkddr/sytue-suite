// Sytue Calendar — invitaciones/asistentes (iTIP básico, RFC 5546 / RFC 6638).
// Demuestra la feature END-TO-END:
//   1) El modal crea un evento con un invitado y el backend persiste el ATTENDEE.
//   2) Una invitación pendiente en la bandeja del admin se acepta desde la UI y
//      cambia de estado (queda fuera de las pendientes).
//
// Límite conocido (dev de UN solo usuario), auditado contra el backend real:
//   · El guardado CalDAV del modal persiste asistentes pero NO auto-agenda iTIP
//     (solo la API REST con send=true entrega a la bandeja).
//   · El backend OMITE el self-invite (asistente == organizador).
// Por eso la invitación de la bandeja se SIEMBRA vía la API REST del servicio Go
// con organizador ≠ admin (único camino que puebla la bandeja del admin en dev).
import { expect, test, type Page } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

const BASE = APPS.calendar.replace(/\/$/, '');
// API REST del servicio Go (mismo host que usa el BFF para CalDAV en dev).
const REST = 'https://calendar-dav.sytue.localhost/api/v1';
const ADMIN = 'admin@sytue.localhost';

// El dev-server compila cada ruta /api/* en frío (20-30 s medido): presupuesto
// holgado por test para las pasadas frías del modal + bandeja.
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

/** Access token vigente (para hablar con la API REST del servicio Go). */
async function accessToken(page: Page): Promise<string> {
  const res = await page.request.get(`${BASE}/auth/token`, { timeout: API_TIMEOUT });
  expect(res.status(), 'GET /auth/token debe responder 200').toBe(200);
  return ((await res.json()) as { accessToken: string }).accessToken;
}

test('el modal invita a un asistente y el backend persiste el ATTENDEE', async ({ page }) => {
  const invitee = `invitado-${Date.now()}-${test.info().workerIndex}@sytue.localhost`;
  const summary = `Reunión e2e ${Date.now()}-${test.info().workerIndex}`;
  const personal = await calendarioPersonal(page);

  await gotoApp(page, 'calendar');

  // --- Crea el evento con un invitado desde el modal ---
  await page.getByRole('button', { name: 'Crear evento', exact: true }).click({ timeout: 60_000 });
  const modal = page.getByRole('dialog', { name: 'Nuevo evento' });
  await expect(modal).toBeVisible();

  await modal.locator('#ev-summary').fill(summary);
  await modal.locator('#ev-invitee').fill(invitee);
  await modal.locator('#ev-invitee').press('Enter');
  // El email confirmado se muestra como chip en la lista de invitados.
  await expect(modal.getByText(invitee, { exact: true })).toBeVisible();
  await modal.getByRole('button', { name: 'Guardar' }).click();
  await expect(modal).toBeHidden({ timeout: API_TIMEOUT });

  // --- El backend CalDAV guardó el VEVENT con la línea ATTENDEE ---
  const now = Date.now();
  const lista = await page.request.get(`${BASE}/api/events`, {
    timeout: API_TIMEOUT,
    params: {
      calendarUrl: personal.url,
      start: toIcalUtc(new Date(now - 24 * 60 * 60 * 1000)),
      end: toIcalUtc(new Date(now + 24 * 60 * 60 * 1000)),
    },
  });
  expect(lista.status(), 'GET /api/events debe responder 200').toBe(200);
  const { events } = (await lista.json()) as { events: Array<{ summary: string; href: string; ical: string }> };
  const propio = events.find((e) => e.summary === summary);
  expect(propio, 'el evento creado debe estar en el calendario').toBeTruthy();

  try {
    expect(propio!.ical, 'el VEVENT conserva el ATTENDEE invitado').toContain(invitee);
  } finally {
    // Limpieza: borra el evento sembrado aunque falle la aserción del ATTENDEE,
    // para no dejar residuos en el calendario dev entre re-ejecuciones.
    await page.request
      .delete(`${BASE}/api/event`, { timeout: API_TIMEOUT, data: { href: propio!.href } })
      .catch(() => {});
  }
});

test('bandeja de invitaciones: una pendiente se acepta desde la UI y cambia de estado', async ({ page }) => {
  await gotoApp(page, 'calendar');
  const token = await accessToken(page);
  const authHeaders = { authorization: `Bearer ${token}` };

  // Calendario destino (API REST del servicio Go).
  const calsRes = await page.request.get(`${REST}/calendars`, { headers: authHeaders, timeout: API_TIMEOUT });
  expect(calsRes.status(), 'GET REST /calendars debe responder 200').toBe(200);
  const calId = ((await calsRes.json()) as { calendars: Array<{ id: string }> }).calendars[0]!.id;

  // Siembra: evento con organizador ≠ admin e invitado = admin, send=true. El
  // backend entrega la invitación REQUEST a la bandeja del admin (un self-invite
  // se omitiría). Único camino que puebla la bandeja en dev de un solo usuario.
  const summary = `Invitación e2e ${Date.now()}-${test.info().workerIndex}`;
  const seed = await page.request.post(`${REST}/calendars/${calId}/events`, {
    headers: authHeaders,
    timeout: API_TIMEOUT,
    data: {
      summary,
      start: new Date(Date.now() + 3600e3).toISOString(),
      end: new Date(Date.now() + 5400e3).toISOString(),
      send: true,
      organizer: 'organizador-e2e@sytue.localhost',
      attendees: [{ email: ADMIN, role: 'REQ-PARTICIPANT', rsvp: true }],
    },
  });
  expect(seed.status(), 'POST REST evento con send=true debe responder 201').toBe(201);
  const seedEventId = ((await seed.json()) as { id: string }).id;

  try {
    // La bandeja del BFF lista la invitación pendiente (200 + enriquecida).
    const inboxRes = await page.request.get(`${BASE}/api/invitations?pending=true`, { timeout: API_TIMEOUT });
    expect(inboxRes.status(), 'GET /api/invitations debe responder 200').toBe(200);
    const { invitations } = (await inboxRes.json()) as {
      invitations: Array<{ id: string; summary?: string; status: string }>;
    };
    const invit = invitations.find((i) => i.summary === summary);
    expect(invit, 'la invitación sembrada debe aparecer pendiente en la bandeja').toBeTruthy();
    expect(invit!.status).toBe('pending');

    // --- UI: abre la bandeja y acepta la invitación ---
    await page.getByRole('button', { name: /Invitaciones/ }).click({ timeout: 60_000 });
    const modal = page.getByRole('dialog', { name: 'Invitaciones' });
    await expect(modal).toBeVisible();
    const item = modal.locator('li', { hasText: summary });
    await expect(item, 'la invitación debe aparecer en la bandeja de la UI').toBeVisible({ timeout: API_TIMEOUT });
    await item.getByRole('button', { name: 'Aceptar' }).click();
    await expect(item, 'aceptada, sale de las pendientes').toBeHidden({ timeout: API_TIMEOUT });

    // --- Backend: la invitación queda aceptada ---
    const after = await page.request.get(`${BASE}/api/invitations?pending=false`, { timeout: API_TIMEOUT });
    expect(after.status()).toBe(200);
    const all = ((await after.json()) as { invitations: Array<{ id: string; status: string }> }).invitations;
    expect(all.find((i) => i.id === invit!.id)?.status, 'la invitación debe quedar aceptada').toBe('accepted');
  } finally {
    // Limpieza (best-effort): borra el evento sembrado.
    await page.request
      .delete(`${REST}/events/${seedEventId}`, { headers: authHeaders, timeout: API_TIMEOUT })
      .catch(() => {});
  }
});
