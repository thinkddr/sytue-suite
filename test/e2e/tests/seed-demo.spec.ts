/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Seed de DATOS DE EJEMPLO de la suite (solo dev). Puebla Mail, Contacts,
 * Calendar, Docs, Drive, Rooms y Meet con material coherente de la organización de
 * dev («Acme, S.L.») para poder VERIFICAR las funcionalidades sobre datos
 * reales en vez de pantallas vacías.
 *
 * No inventa una puerta trasera: escribe por los MISMOS BFF que usa el
 * navegador, con la sesión OIDC real del admin de dev — si el seed pasa, esos
 * caminos funcionan de verdad.
 *
 *   make dev-demo          (o: SYTUE_SEED=1 bunx playwright test seed-demo)
 *
 * Es idempotente por marca: si el buzón ya tiene el correo centinela, no
 * duplica nada.
 */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { completarMFASiHaceFalta } from './helpers';

const APP = (sub: string) => `https://${sub}.sytue.localhost`;
const T = 60_000;

/** Marca del seed: distingue lo sembrado de lo que crea el usuario probando. */
const CENTINELA = 'Bienvenida a Sytue One';

const PERSONAS = [
  {
    nombre: 'Lucía Ferrer',
    email: 'lucia.ferrer@acme.example',
    cargo: 'Directora de Operaciones',
    tel: '+34 911 200 301',
  },
  {
    nombre: 'Marc Oliveira',
    email: 'marc.oliveira@acme.example',
    cargo: 'Responsable de Producto',
    tel: '+34 911 200 302',
  },
  {
    nombre: 'Nadia Belkacem',
    email: 'nadia.belkacem@acme.example',
    cargo: 'Arquitecta de plataforma',
    tel: '+34 911 200 303',
  },
  {
    nombre: 'Tomás Iglesias',
    email: 'tomas.iglesias@acme.example',
    cargo: 'Comercial',
    tel: '+34 911 200 304',
  },
  {
    nombre: 'Ingrid Sørensen',
    email: 'ingrid.sorensen@nordika.example',
    cargo: 'Compras — Nordika A/S',
    tel: '+45 32 10 44 90',
  },
  {
    nombre: 'Paulo Rêgo',
    email: 'paulo.rego@lusitana.example',
    cargo: 'CTO — Lusitana Lda.',
    tel: '+351 21 880 55 12',
  },
];

const CORREOS = [
  {
    subject: CENTINELA,
    body: 'Tu cuenta de Acme ya está lista. Desde aquí tienes correo, archivos, documentos, calendario, contactos y las salas de equipo, todo con la misma sesión.',
  },
  {
    subject: 'Renovación del contrato marco — Nordika A/S',
    body: 'Ingrid confirma que quieren renovar por 24 meses. Piden revisar el anexo de tratamiento de datos antes del 15. ¿Lo vemos el martes?',
  },
  {
    subject: 'Presupuesto Q3 — revisión de plataforma',
    body: 'Adjunto el desglose por equipos. La partida de infraestructura baja un 12 % tras consolidar entornos. Falta cerrar la de soporte.',
  },
  {
    subject: 'RE: Incidencia de acceso del viernes',
    body: 'Resuelto. Era una política de acceso condicional que exigía segundo factor en la red de invitados. Documentado en el runbook.',
  },
  {
    subject: 'Propuesta de migración — Lusitana Lda.',
    body: 'Paulo pregunta por los tiempos de migración de buzones y por el soporte de IMAP durante la transición. Preparo una estimación para el jueves.',
  },
  {
    subject: 'Acta del comité de producto',
    body: 'Decisiones: priorizar la app de escritorio, posponer el conector de Jira a la siguiente ola y cerrar el rediseño de la consola.',
  },
  {
    subject: 'Recordatorio: formación de seguridad',
    body: 'La sesión obligatoria de phishing y gestión de contraseñas es el miércoles a las 10:00. Dura 45 minutos.',
  },
];

const EVENTOS = [
  { summary: 'Comité de producto', dia: 1, hora: 9, dur: 60, lugar: 'Sala Norte' },
  { summary: 'Revisión de presupuesto Q3', dia: 1, hora: 12, dur: 45, lugar: 'Sala Sur' },
  { summary: 'Formación de seguridad', dia: 2, hora: 10, dur: 45, lugar: 'Auditorio' },
  { summary: 'Nordika — renovación de contrato', dia: 2, hora: 16, dur: 30, lugar: 'Videollamada' },
  { summary: 'Uno a uno con Marc', dia: 3, hora: 11, dur: 30, lugar: 'Café' },
  { summary: 'Demo de plataforma a Lusitana', dia: 3, hora: 15, dur: 60, lugar: 'Videollamada' },
  { summary: 'Retro de la ola', dia: 4, hora: 17, dur: 45, lugar: 'Sala Norte' },
];

const DOCUMENTOS: { kind: 'document' | 'spreadsheet' | 'presentation'; title: string }[] = [
  { kind: 'document', title: 'Contrato marco — Nordika A/S (borrador)' },
  { kind: 'document', title: 'Runbook de incidencias de acceso' },
  { kind: 'spreadsheet', title: 'Presupuesto Q3 por equipos' },
  { kind: 'spreadsheet', title: 'Seguimiento comercial 2026' },
  { kind: 'presentation', title: 'Demo de plataforma — Lusitana' },
  { kind: 'presentation', title: 'Resultados del trimestre' },
];

const CARPETAS = [
  '/Comercial',
  '/Comercial/Contratos',
  '/Comercial/Propuestas',
  '/Producto',
  '/Producto/Especificaciones',
  '/Finanzas',
  '/Equipo',
];

/**
 * Salas de vídeo de ejemplo, ligadas al canal de Rooms del que nacen (source `chat`).
 * Esa ligadura es lo que permite que, al terminar la reunión, Rooms publique la tarjeta
 * de «llamada finalizada» en el canal de origen: sin una sala con origen, ese camino no
 * se puede ejercitar en local.
 */
const SALAS = ['Comité de producto', 'Demo de plataforma — Lusitana'];

const MENSAJES = [
  'Buenos días 👋 Arrancamos la ola: la revisión de presupuesto es a las 12.',
  'He subido el borrador del contrato de Nordika a Drive, en Comercial/Contratos.',
  'La incidencia de acceso del viernes queda cerrada — era la política de segundo factor en la red de invitados.',
  '¿Alguien puede revisar la estimación de migración de Lusitana antes del jueves?',
  'Recordad la formación de seguridad del miércoles a las 10:00.',
];

/** Lunes de la semana en curso, a las 00:00 — ancla de los eventos sembrados. */
function lunes(): Date {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  d.setDate(d.getDate() - ((d.getDay() + 6) % 7));
  return d;
}

/** POST JSON contra un BFF, exigiendo 2xx: un seed a medias es peor que ninguno. */
async function post(req: APIRequestContext, url: string, data: unknown): Promise<unknown> {
  const res = await req.post(url, { data, timeout: T });
  expect(res.status(), `POST ${url} → ${res.status()} ${await res.text()}`).toBeLessThan(300);
  return res.json().catch(() => ({}));
}

/** Sesión del segundo usuario de dev (member, sin MFA): remite los correos de
 *  entrada, para que la bandeja tenga tráfico real de otra persona y no solo
 *  autoenvíos del admin. */
async function sesionMember(page: Page): Promise<void> {
  await page.goto(`${APP('mail')}/`, { waitUntil: 'domcontentloaded' });
  if (/\/login\//.test(page.url())) {
    await page.fill('input[name=email]', 'member@sytue.localhost');
    await page.fill('input[name=password]', process.env.SYTUE_E2E_PASS || 'sytue-dev-admin');
    await page.click('button[type=submit]');
    await completarMFASiHaceFalta(page);
  }
  await page.waitForURL((u) => u.hostname === 'mail.sytue.localhost', { timeout: 90_000 });
}

test.describe('seed de datos de ejemplo', () => {
  test.skip(!process.env.SYTUE_SEED, 'seed bajo demanda: SYTUE_SEED=1 (o make dev-demo)');

  test('puebla la suite con datos de Acme, S.L.', async ({ page, browser }) => {
    test.setTimeout(600_000);

    // --- Contactos --------------------------------------------------------
    const libretas = await page.request.get(`${APP('contacts')}/api/addressbooks`, { timeout: T });
    expect(libretas.status(), 'GET /api/addressbooks').toBe(200);
    const { addressBooks } = (await libretas.json()) as {
      addressBooks: { url: string; displayName?: string }[];
    };
    const libreta = addressBooks[0];
    expect(libreta, 'el usuario debe tener al menos una libreta').toBeTruthy();

    // Idempotencia: se mira un DATO propio del seed (el dominio de los contactos
    // sembrados), no una búsqueda por el término — el buscador devuelve la query en su
    // respuesta y cualquier centinela buscado así da falso positivo.
    //
    // La comprobación va DESPUÉS de resolver la libreta porque /api/contacts EXIGE
    // addressBookUrl: sin él responde 400, la guarda lo leía como «no sembrado» y cada
    // `make dev-demo` duplicaba la suite entera (5 copias de cada contacto tras cinco
    // pasadas). Se pedía justo lo que hacía falta para no repetir, y nunca llegaba.
    const previo = await page.request.get(
      `${APP('contacts')}/api/contacts?addressBookUrl=${encodeURIComponent(libreta.url)}`,
      { timeout: T },
    );
    expect(previo.status(), 'GET /api/contacts (guarda de idempotencia)').toBe(200);
    test.skip((await previo.text()).includes('@acme.example'), 'la suite ya tiene los datos de ejemplo');

    for (const p of PERSONAS) {
      const [firstName, ...resto] = p.nombre.split(' ');
      await post(page.request, `${APP('contacts')}/api/contact`, {
        addressBookUrl: libreta.url,
        contactInput: {
          fullName: p.nombre,
          firstName,
          lastName: resto.join(' '),
          emails: [p.email],
          phones: [p.tel],
          organization: 'Acme, S.L.',
          title: p.cargo,
        },
      });
    }

    // --- Calendario -------------------------------------------------------
    const cals = await page.request.get(`${APP('calendar')}/api/calendars`, { timeout: T });
    expect(cals.status(), 'GET /api/calendars').toBe(200);
    const { calendars } = (await cals.json()) as { calendars: { url: string }[] };
    const cal = calendars[0];
    expect(cal, 'el usuario debe tener al menos un calendario').toBeTruthy();

    const base = lunes();
    for (const e of EVENTOS) {
      const inicio = new Date(base);
      inicio.setDate(base.getDate() + e.dia);
      inicio.setHours(e.hora, 0, 0, 0);
      const fin = new Date(inicio.getTime() + e.dur * 60_000);
      await post(page.request, `${APP('calendar')}/api/event`, {
        calendarUrl: cal.url,
        eventInput: {
          summary: e.summary,
          start: inicio.toISOString(),
          end: fin.toISOString(),
          location: e.lugar,
        },
      });
    }

    // --- Drive ------------------------------------------------------------
    for (const path of CARPETAS) {
      const res = await page.request.post(`${APP('drive')}/api/folders`, {
        data: { path },
        timeout: T,
      });
      // 409 = ya existía: el seed se puede repetir sin ensuciar el resultado.
      expect([200, 201, 409], `POST /api/folders ${path}`).toContain(res.status());
    }

    // --- Docs -------------------------------------------------------------
    for (const d of DOCUMENTOS) {
      await post(page.request, `${APP('docs')}/api/create`, d);
    }

    // --- Rooms ------------------------------------------------------------
    const canales = await page.request.get(`${APP('rooms')}/api/channels`, { timeout: T });
    expect(canales.status(), 'GET /api/channels').toBe(200);
    const { channels } = (await canales.json()) as { channels: { id: string; name: string }[] };
    const general = channels.find((c) => /general/i.test(c.name)) ?? channels[0];
    if (general) {
      for (const body of MENSAJES) {
        await post(page.request, `${APP('rooms')}/api/send`, { channelId: general.id, body });
      }

      // --- Meet -----------------------------------------------------------
      // Se crean POR EL MISMO camino que el botón de llamada del canal, así que
      // quedan ligadas a él (kind `chat` + ref_id). Es lo que hace realista la
      // prueba del ciclo de una reunión: el SFU avisa al terminar y Rooms publica
      // la tarjeta de «llamada finalizada» en este canal.
      // Una sala POR CANAL: la creación es idempotente por origen (kind + ref_id), así
      // que dos llamadas al mismo canal devuelven la misma sala, no dos.
      const canalesConSala = [general, channels.find((c) => c.id !== general.id)].filter(
        (c): c is { id: string; name: string } => Boolean(c),
      );
      for (const [i, canal] of canalesConSala.entries()) {
        await post(page.request, `${APP('rooms')}/api/call`, {
          channelId: canal.id,
          title: SALAS[i] ?? SALAS[0],
        });
      }
    }

    // --- Mail -------------------------------------------------------------
    // Los correos entran DESDE el member, en una pestaña con su propia sesión.
    const ctx = await browser.newContext({ ignoreHTTPSErrors: true });
    try {
      const otra = await ctx.newPage();
      await sesionMember(otra);
      for (const c of CORREOS) {
        await post(otra.request, `${APP('mail')}/api/send`, {
          to: [{ email: 'admin@sytue.localhost', name: 'Admin Sytue' }],
          subject: c.subject,
          textBody: c.body,
        });
      }
    } finally {
      await ctx.close();
    }

    // El admin responde a uno: así hay también hilo y carpeta de enviados.
    await post(page.request, `${APP('mail')}/api/send`, {
      to: [{ email: 'member@sytue.localhost', name: 'Member Sytue' }],
      subject: 'RE: Presupuesto Q3 — revisión de plataforma',
      textBody: 'Recibido. Lo reviso hoy y lo comentamos en el comité del lunes.',
    });
  });
});
