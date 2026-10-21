// Sytue Calendar — multi-calendario: gestion de calendarios via BFF
// (POST /api/calendars crea / {id,...} renombra-recolorea / {id,delete:true}
// borra), superposicion de eventos de varios calendarios en la rejilla y
// conmutacion de visibilidad por checkbox en la sidebar.
import { expect, test, type Page } from '@playwright/test';
import { APPS, gotoApp } from './helpers';

const BASE = APPS.calendar.replace(/\/$/, '');

// El dev-server (Astro/Vite) compila cada ruta /api/* a demanda: presupuesto
// holgado por peticion para las pasadas frias (ver calendar.spec.ts).
const API_TIMEOUT = 60_000;
test.beforeEach(() => test.setTimeout(180_000));

interface Calendario {
  id: string;
  url: string;
  displayName: string;
  color: string;
  isDefault: boolean;
}

/** Establece sesion en la app y devuelve la lista de calendarios (200 garantizado). */
async function listarCalendarios(page: Page): Promise<Calendario[]> {
  const res = await page.request.get(`${BASE}/api/calendars`, { timeout: API_TIMEOUT });
  expect(res.status(), 'GET /api/calendars debe responder 200').toBe(200);
  const body = (await res.json()) as { calendars: Calendario[] };
  expect(Array.isArray(body.calendars)).toBe(true);
  return body.calendars;
}

/** Crea un calendario via BFF y devuelve su representacion. */
async function crearCalendario(page: Page, displayName: string, color: string): Promise<Calendario> {
  const res = await page.request.post(`${BASE}/api/calendars`, {
    timeout: API_TIMEOUT,
    data: { displayName, color },
  });
  expect(res.status(), 'POST /api/calendars (crear) debe responder 201').toBe(201);
  const { calendar } = (await res.json()) as { calendar: Calendario };
  expect(calendar.id).toBeTruthy();
  expect(calendar.url).toBeTruthy();
  return calendar;
}

/** Borra un calendario via BFF (limpieza best-effort en los finally). */
async function borrarCalendario(page: Page, id: string): Promise<void> {
  const res = await page.request.post(`${BASE}/api/calendars`, {
    timeout: API_TIMEOUT,
    data: { id, delete: true },
  });
  expect.soft(res.ok(), `el calendario ${id} debe poder borrarse`).toBe(true);
}

test('ciclo de vida por API: crear, renombrar/recolorear y borrar un calendario', async ({ page }) => {
  await gotoApp(page, 'calendar');
  const nombre = `Multical e2e ${Date.now()}-${test.info().workerIndex}`;
  const creado = await crearCalendario(page, nombre, '#146a47');
  try {
    expect(creado.displayName).toBe(nombre);
    expect(creado.color.toLowerCase()).toBe('#146a47');
    expect(creado.isDefault).toBe(false);

    // Aparece en la lista.
    let lista = await listarCalendarios(page);
    expect(lista.map((c) => c.id)).toContain(creado.id);

    // Renombrar + cambiar color (PATCH en Go via POST {id,...}).
    const renombrado = `${nombre} bis`;
    const patch = await page.request.post(`${BASE}/api/calendars`, {
      timeout: API_TIMEOUT,
      data: { id: creado.id, displayName: renombrado, color: '#4338ca' },
    });
    expect(patch.status(), 'POST /api/calendars (patch) debe responder 200').toBe(200);
    const { calendar: actualizado } = (await patch.json()) as { calendar: Calendario };
    expect(actualizado.displayName).toBe(renombrado);
    expect(actualizado.color.toLowerCase()).toBe('#4338ca');
    // La URL CalDAV (name) no cambia al renombrar: los eventos no se mueven.
    expect(actualizado.url).toBe(creado.url);
  } finally {
    await borrarCalendario(page, creado.id);
  }
  // Tras el borrado ya no se lista.
  const final = await listarCalendarios(page);
  expect(final.map((c) => c.id)).not.toContain(creado.id);
});

test('el calendario por defecto es inmutable: borrar responde 403', async ({ page }) => {
  await gotoApp(page, 'calendar');
  const porDefecto = (await listarCalendarios(page)).find((c) => c.isDefault);
  expect(porDefecto, 'debe existir el calendario por defecto').toBeTruthy();
  const res = await page.request.post(`${BASE}/api/calendars`, {
    timeout: API_TIMEOUT,
    data: { id: porDefecto!.id, delete: true },
  });
  expect(res.status(), 'el default no se puede borrar (403 del servicio Go)').toBe(403);
});

test('superposicion: eventos de dos calendarios a la vez y checkbox que los conmuta', async ({ page }) => {
  await gotoApp(page, 'calendar');
  const sello = `${Date.now()}-${test.info().workerIndex}`;
  const extra = await crearCalendario(page, `Superpuesto ${sello}`, '#a31f38');
  try {
    // Un evento HOY en el calendario extra (visible en la vista de mes actual).
    const resumen = `Evento multical ${sello}`;
    const inicio = new Date();
    inicio.setHours(12, 0, 0, 0);
    const fin = new Date(inicio.getTime() + 30 * 60 * 1000);
    const creado = await page.request.post(`${BASE}/api/event`, {
      timeout: API_TIMEOUT,
      data: {
        calendarUrl: extra.url,
        eventInput: { summary: resumen, start: inicio.toISOString(), end: fin.toISOString() },
      },
    });
    expect(creado.status(), 'POST /api/event debe responder 200').toBe(200);

    // Recarga de la isla: primera carga => todos los calendarios visibles.
    await gotoApp(page, 'calendar');
    const casilla = page.getByRole('checkbox', { name: `Mostrar ${extra.displayName}` });
    await expect(casilla, 'el calendario nuevo aparece en la sidebar').toBeVisible({ timeout: 60_000 });
    await expect(casilla).toBeChecked();

    // Vista «Día» (hoy): lista todos los eventos sin el recorte «+N más» de la
    // vista de mes — inmune a la acumulación de eventos de otras pasadas e2e.
    await page.getByRole('button', { name: 'Día' }).click();

    // Su evento se superpone en la rejilla junto a los del resto (el nombre
    // accesible del chip en vista día incluye la hora: regex por contenido).
    const chip = page.getByRole('button', { name: new RegExp(resumen) });
    await expect(chip, 'el evento del calendario extra se pinta en la rejilla').toBeVisible({ timeout: 60_000 });

    // Desmarcar oculta SOLO sus eventos; volver a marcar los muestra de nuevo.
    await casilla.uncheck();
    await expect(chip).toBeHidden();
    await casilla.check();
    await expect(chip).toBeVisible();
  } finally {
    // Borrar el calendario arrastra sus eventos: sin residuos en el entorno dev.
    await borrarCalendario(page, extra.id);
  }
});

test('crear y renombrar un calendario desde la sidebar (modal + kebab)', async ({ page }) => {
  await gotoApp(page, 'calendar');
  const nombre = `Sidebar e2e ${Date.now()}-${test.info().workerIndex}`;
  let id: string | null = null;
  try {
    // «+» de la seccion Mis calendarios -> modal de nuevo calendario.
    await page.getByRole('button', { name: 'Crear calendario' }).click({ timeout: 60_000 });
    const modal = page.getByRole('dialog', { name: 'Nuevo calendario' });
    await expect(modal).toBeVisible();
    await modal.getByLabel(/^Nombre/).fill(nombre);
    // Swatch de la paleta de 14 tonos (radiogroup accesible).
    await modal.getByRole('radio', { name: 'Índigo' }).click();
    await modal.getByRole('button', { name: 'Guardar' }).click();
    await expect(modal).toBeHidden();

    // La fila nueva aparece con su checkbox marcado (visible por defecto).
    const casilla = page.getByRole('checkbox', { name: `Mostrar ${nombre}` });
    await expect(casilla).toBeVisible({ timeout: 60_000 });
    await expect(casilla).toBeChecked();
    id = (await listarCalendarios(page)).find((c) => c.displayName === nombre)?.id ?? null;
    expect(id, 'el calendario creado por UI debe existir via API').toBeTruthy();

    // Kebab -> Renombrar (mismo modal en modo edicion).
    const fila = page.locator('li', { has: casilla });
    await fila.hover();
    await fila.getByRole('button', { name: `Opciones de ${nombre}` }).click();
    await page.getByRole('menuitem', { name: 'Renombrar' }).click();
    const editar = page.getByRole('dialog', { name: 'Editar calendario' });
    await expect(editar).toBeVisible();
    const renombrado = `${nombre} bis`;
    // Teclado, no `fill()`: sobre un input CONTROLADO de Preact que ya tiene
    // texto, `fill()` escribe el valor sin que Preact lo registre y el siguiente
    // render lo revierte al original, así que se guardaba el nombre viejo. Con
    // teclado (y con pegar, comprobado) el renombrado funciona. En el modal de
    // creación no se nota porque el campo parte vacío.
    const campoNombre = editar.getByLabel(/^Nombre/);
    await campoNombre.click();
    await page.keyboard.press('ControlOrMeta+A');
    await page.keyboard.press('Backspace');
    await campoNombre.pressSequentially(renombrado);
    await editar.getByRole('button', { name: 'Guardar' }).click();
    await expect(editar).toBeHidden();
    await expect(page.getByRole('checkbox', { name: `Mostrar ${renombrado}` })).toBeVisible({ timeout: 60_000 });
  } finally {
    if (id) await borrarCalendario(page, id);
  }
});
