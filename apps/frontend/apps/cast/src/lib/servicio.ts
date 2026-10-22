/**
 * El servicio Go de Sytue Cast visto desde el SERVIDOR de esta app (páginas SSR
 * y proxies). Nunca se importa desde una isla: lleva el token de la sesión.
 *
 * Las páginas públicas (portada, canal, vídeo, incrustado) se pintan en el
 * servidor con `/public/*` —dentro del clúster, sin pasar por el ingress— para
 * que un buscador y quien comparte un enlace vean el contenido sin JavaScript.
 */
import { serverEnv } from '@sytue/auth/server';

const env = serverEnv(import.meta.env);

/** Raíz del servicio por la red interna. */
export const DESTINO = (env.SYTUE_CAST_API_URL ?? 'http://cast:8099').replace(/\/$/, '');

/** Raíz pública del WebSocket del chat (el navegador conecta directo). */
export const WS_BASE = (env.SYTUE_CAST_WS_URL ?? 'wss://cast.sytue.eu').replace(/\/$/, '');

/** Resultado de una consulta al servicio: el cuerpo, o el código HTTP si no hubo. */
export type Consulta<T> = { ok: true; datos: T } | { ok: false; status: number };

/**
 * GET JSON al servicio. `token` presente → con sesión; ausente → sin ella. Un
 * fallo de red se devuelve como 503 para que la página pinte «no disponible» en
 * vez de reventar.
 */
export async function consultar<T>(ruta: string, token?: string): Promise<Consulta<T>> {
  try {
    const res = await fetch(DESTINO + ruta, {
      headers: {
        accept: 'application/json',
        ...(token ? { authorization: `Bearer ${token}` } : {}),
      },
    });
    if (!res.ok) return { ok: false, status: res.status };
    return { ok: true, datos: (await res.json()) as T };
  } catch {
    return { ok: false, status: 503 };
  }
}
