/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Administración de Sytue Rooms desde la consola de la organización.
 *
 * Habla con `services/rooms` (Go) y no con Sytue ID: los canales son suyos. La
 * llamada sale del SERVIDOR con el access token de la sesión, así que Rooms
 * aplica el mismo gate que cualquier otra —`RequireRole("owner","admin")`— y el
 * aislamiento por organización lo pone RLS con el tenant del token. Aquí no se
 * decide nada de permisos: se pregunta y se propaga el código que conteste.
 */
import { HttpClient, HttpError, bearer } from '@sytue/api-clients';
import type { SytueSession } from '@sytue/auth';
import { roomsBaseUrl } from './auth.js';

/** Un canal visto desde la administración. */
export interface AdminChannel {
  id: string;
  team_id: string;
  team_name: string;
  slug: string;
  name: string;
  topic?: string;
  purpose?: string;
  kind: 'public' | 'private' | 'direct';
  archived: boolean;
  members: number;
  created_at: string;
  last_message_at?: string;
}

/** Resumen de Rooms en la organización. */
export interface RoomsStats {
  channels: number;
  private: number;
  archived: number;
  teams: number;
  last_event_at?: string;
}

/**
 * Resultado de una llamada, con el CÓDIGO conservado. Un 403 y un 502 llevan a
 * pantallas distintas —«no puedes» frente a «no contesta»— y colapsarlos en un
 * `null` deja al usuario mirando una lista vacía sin saber cuál de las dos es.
 */
export interface RoomsResult<T> {
  ok: boolean;
  status: number;
  data: T;
}

function http(session: SytueSession): HttpClient {
  return new HttpClient({
    baseUrl: `${roomsBaseUrl().replace(/\/$/, '')}/api/v1/admin`,
    auth: bearer(session.tokens.accessToken),
  });
}

async function attempt<T>(fallback: T, fn: () => Promise<T>): Promise<RoomsResult<T>> {
  try {
    return { ok: true, status: 200, data: await fn() };
  } catch (e) {
    return { ok: false, status: e instanceof HttpError ? e.status : 502, data: fallback };
  }
}

/**
 * Canales de la organización. Los archivados solo si se piden: son la excepción
 * y llenan la lista de ruido justo cuando se busca algo vivo.
 */
export async function loadRoomsChannels(
  session: SytueSession,
  includeArchived = false,
): Promise<RoomsResult<AdminChannel[]>> {
  return attempt<AdminChannel[]>([], async () => {
    const r = await http(session).json<{ items?: AdminChannel[] }>({
      path: `/channels${includeArchived ? '?archived=1' : ''}`,
    });
    return r.items ?? [];
  });
}

/** Resumen para la cabecera de la pantalla. */
export async function loadRoomsStats(
  session: SytueSession,
): Promise<RoomsResult<RoomsStats | null>> {
  return attempt<RoomsStats | null>(null, () => http(session).json<RoomsStats>({ path: '/stats' }));
}

/**
 * Archiva o restaura un canal. Es lo único que se cambia desde aquí: el nombre,
 * el tema y el propósito los lleva el equipo dueño del canal.
 */
export async function setChannelArchived(
  session: SytueSession,
  id: string,
  archived: boolean,
): Promise<RoomsResult<AdminChannel | null>> {
  return attempt<AdminChannel | null>(null, () =>
    http(session).json<AdminChannel>({
      method: 'PATCH',
      path: `/channels/${encodeURIComponent(id)}`,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ archived }),
    }),
  );
}
