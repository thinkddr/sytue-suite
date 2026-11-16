import type { APIRoute } from 'astro';
import { CalendarClient, JmapClient, bearer, type Event, type Mailbox } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/auth';
import { recortarAgenda, type EventoDeHoy } from '../../lib/hoy';

export const prerender = false;

const env = serverEnv(import.meta.env);
const CALENDAR_BASE_URL = env.SYTUE_CALENDAR_BASE_URL ?? 'https://calendar.sytue.eu';
const JMAP_SESSION_URL = env.SYTUE_JMAP_SESSION_URL ?? 'https://mail.sytue.eu/.well-known/jmap';

/**
 * Lo que la portada del Hub enseña de «hoy»: los eventos del día y el correo
 * sin leer. Cada fuente va por su lado y falla por su lado (`null`): un Mail
 * caído no deja la agenda a oscuras. De la agenda van los seis primeros, cuántos
 * hay en total (`total`, para el «+N más») y si falta algún calendario que no
 * contestó (`parcial`): sin eso, un calendario caído parece un día más tranquilo.
 * Los límites del día los pone el navegador (`from`/`to`), que es quien sabe en
 * qué zona horaria vive la persona.
 */
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const from = ctx.url.searchParams.get('from') ?? '';
  const to = ctx.url.searchParams.get('to') ?? '';
  if (!esInstante(from) || !esInstante(to)) return json({ error: 'invalid_range' }, 400);
  const auth = bearer(session.accessToken);

  const [agenda, correo] = await Promise.all([
    new CalendarClient({ baseUrl: CALENDAR_BASE_URL, auth })
      .listEventsDetallado({ from, to })
      .then((r) => recortarAgenda(r.events.map(resumir), r.failed.length, r.calendars))
      .catch(() => null),
    new JmapClient({ sessionUrl: JMAP_SESSION_URL, auth })
      .listMailboxes()
      .then((buzones) => ({ unread: sinLeer(buzones) }))
      .catch(() => null),
  ]);
  return json({
    eventos: agenda?.eventos ?? null,
    total: agenda?.total ?? 0,
    parcial: agenda?.parcial ?? false,
    correo,
  });
};

function resumir(e: Event): EventoDeHoy {
  return {
    id: e.id,
    summary: e.summary,
    start: e.start,
    ...(e.end ? { end: e.end } : {}),
    allDay: Boolean(e.allDay) || /^\d{8}$/.test(e.start) || /^\d{4}-\d{2}-\d{2}$/.test(e.start),
    ...(e.location ? { location: e.location } : {}),
  };
}

/** Los no leídos de la bandeja de entrada; si no hay buzón con ese rol, de todos. */
function sinLeer(buzones: Mailbox[]): number {
  const entrada = buzones.find((m) => m.role === 'inbox');
  if (entrada) return entrada.unreadEmails ?? 0;
  return buzones.reduce((n, m) => n + (m.unreadEmails ?? 0), 0);
}

function esInstante(s: string): boolean {
  return s.length >= 10 && !Number.isNaN(new Date(s).getTime());
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
