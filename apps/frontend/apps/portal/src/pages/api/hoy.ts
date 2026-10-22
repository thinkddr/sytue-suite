import type { APIRoute } from 'astro';
import { CalendarClient, JmapClient, bearer, type Event, type Mailbox } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/auth';

export const prerender = false;

const env = serverEnv(import.meta.env);
const CALENDAR_BASE_URL = env.SYTUE_CALENDAR_BASE_URL ?? 'https://calendar.sytue.eu';
const JMAP_SESSION_URL = env.SYTUE_JMAP_SESSION_URL ?? 'https://mail.sytue.eu/.well-known/jmap';
const MAX_EVENTOS = 6;

export interface EventoDeHoy {
  id: string;
  summary: string;
  start: string;
  end?: string;
  allDay: boolean;
  location?: string;
}

/**
 * Lo que la portada del Hub enseña de «hoy»: los eventos del día y el correo
 * sin leer. Cada fuente va por su lado y falla por su lado (`null`): un Mail
 * caído no deja la agenda a oscuras. Los límites del día los pone el navegador
 * (`from`/`to`), que es quien sabe en qué zona horaria vive la persona.
 */
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const from = ctx.url.searchParams.get('from') ?? '';
  const to = ctx.url.searchParams.get('to') ?? '';
  if (!esInstante(from) || !esInstante(to)) return json({ error: 'invalid_range' }, 400);
  const auth = bearer(session.accessToken);

  const [eventos, correo] = await Promise.all([
    new CalendarClient({ baseUrl: CALENDAR_BASE_URL, auth })
      .listEvents({ from, to })
      .then((lista) => lista.slice(0, MAX_EVENTOS).map(resumir))
      .catch(() => null),
    new JmapClient({ sessionUrl: JMAP_SESSION_URL, auth })
      .listMailboxes()
      .then((buzones) => ({ unread: sinLeer(buzones) }))
      .catch(() => null),
  ]);
  return json({ eventos, correo });
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
