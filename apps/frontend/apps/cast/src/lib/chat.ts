/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El protocolo del chat del directo (`services/cast/internal/social/protocolo.go`):
 * tramas `message`, `deleted`, `viewers` y `error` del servidor; `send` del
 * cliente. Aparte del componente para poder probar el estado sin un WebSocket.
 */
import type { MensajeChat } from './tipos.js';

export type Trama =
  | ({ type: 'message' } & MensajeChat)
  | { type: 'deleted'; id: string }
  | { type: 'viewers'; count: number }
  | { type: 'error'; code?: string; message?: string };

/** Lee una trama; lo que no se entiende se descarta (null), no revienta el chat. */
export function leerTrama(datos: unknown): Trama | null {
  if (typeof datos !== 'string') return null;
  let t: unknown;
  try {
    t = JSON.parse(datos);
  } catch {
    return null;
  }
  if (!t || typeof t !== 'object') return null;
  const o = t as Record<string, unknown>;
  switch (o['type']) {
    case 'message':
      return typeof o['id'] === 'string' && typeof o['body'] === 'string' && o['author']
        ? (o as unknown as Trama)
        : null;
    case 'deleted':
      return typeof o['id'] === 'string' ? { type: 'deleted', id: o['id'] } : null;
    case 'viewers':
      return typeof o['count'] === 'number' ? { type: 'viewers', count: o['count'] } : null;
    case 'error':
      return {
        type: 'error',
        ...(typeof o['code'] === 'string' ? { code: o['code'] } : {}),
        ...(typeof o['message'] === 'string' ? { message: o['message'] } : {}),
      };
  }
  return null;
}

/** Tope de mensajes en memoria: un directo largo no crece sin fin en la pestaña. */
export const MAX_MENSAJES = 500;

/**
 * Aplica una trama a la lista. Idempotente con el historial: un mensaje que ya
 * está (llegó por REST y por el socket) no se duplica.
 */
export function aplicar(lista: readonly MensajeChat[], t: Trama): readonly MensajeChat[] {
  if (t.type === 'message') {
    if (lista.some((m) => m.id === t.id)) return lista;
    const { type: _type, ...m } = t;
    const nueva = [...lista, m];
    return nueva.length > MAX_MENSAJES ? nueva.slice(-MAX_MENSAJES) : nueva;
  }
  if (t.type === 'deleted') return lista.filter((m) => m.id !== t.id);
  return lista;
}

/** Une el historial con lo que ya llegó por el socket, sin repetir y en orden. */
export function unir(
  historial: readonly MensajeChat[],
  vivos: readonly MensajeChat[],
): readonly MensajeChat[] {
  const vistos = new Set<string>();
  const todos = [...historial, ...vivos].filter((m) => !vistos.has(m.id) && vistos.add(m.id));
  todos.sort((a, b) => a.created_at.localeCompare(b.created_at));
  return todos.slice(-MAX_MENSAJES);
}

/** ws(s)://… del chat de un vídeo con el token de acceso en la query. */
export function urlDelChat(wsBase: string, videoId: string, token: string): string {
  const base = wsBase.replace(/\/+$/, '');
  return `${base}/ws/videos/${encodeURIComponent(videoId)}/chat?access_token=${encodeURIComponent(token)}`;
}
