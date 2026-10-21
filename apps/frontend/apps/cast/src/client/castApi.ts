/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cliente de la API de Sytue Cast desde el navegador.
 *
 * Todo va por el BFF de esta app (mismo origen, cookie httpOnly): el token de
 * acceso no baja al navegador. Lo anónimo va por `/api/v1/public/…`, que el proxy
 * deja pasar sin sesión; `/public` a secas no está publicado en el ingress.
 */
import { resolverReto, type RetoAltcha } from '@sytue/config';
import { withBase } from '@sytue/ui';
import { peticionDeDenuncia } from '../lib/denuncia.js';
import { FalloDelServicio } from '../lib/fallo.js';
import type {
  AccionDeDenuncia,
  Bloqueado,
  CalidadesDelVideo,
  Canal,
  ClaveDeEmision,
  Comentario,
  ConexionDirecto,
  Cuota,
  Denuncia,
  ErrorDelServicio,
  Estimacion,
  FicheroDeDrive,
  MensajeChat,
  Motivo,
  ObjetivoDeDenuncia,
  Pagina,
  Persona,
  Portada,
  Presentacion,
  Reproduccion,
  Video,
  Visibilidad,
} from '../lib/tipos.js';

const api = (ruta: string): string => withBase(import.meta.env.BASE_URL, `/api/v1${ruta}`);

async function fallo(res: Response): Promise<FalloDelServicio> {
  let sobre: ErrorDelServicio = {};
  try {
    sobre = (await res.json()) as ErrorDelServicio;
  } catch {
    // Sin cuerpo JSON: basta con el código HTTP.
  }
  return new FalloDelServicio(res.status, sobre.error?.code ?? '', sobre.error?.details ?? {});
}

function alLogin(): never {
  window.location.assign(
    withBase(
      import.meta.env.BASE_URL,
      `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
    ),
  );
  throw new FalloDelServicio(401);
}

async function pedir<T>(ruta: string, init: RequestInit = {}): Promise<T> {
  const cabeceras = new Headers(init.headers);
  cabeceras.set('accept', 'application/json');
  if (init.body && !cabeceras.has('content-type'))
    cabeceras.set('content-type', 'application/json');
  const res = await fetch(api(ruta), { credentials: 'include', ...init, headers: cabeceras });
  if (res.status === 401 && !ruta.startsWith('/public/')) alLogin();
  if (!res.ok) throw await fallo(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

const json = (metodo: string, cuerpo: unknown): RequestInit => ({
  method: metodo,
  body: JSON.stringify(cuerpo),
});

const q = (p: Record<string, string | undefined>): string => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(p)) if (v) s.set(k, v);
  const t = s.toString();
  return t ? `?${t}` : '';
};

// ── Público ─────────────────────────────────────────────────────────────────

export const portada = (cursor?: string) => pedir<Portada>(`/public/explore${q({ cursor })}`);
export const canal = (org: string, cursor?: string) =>
  pedir<Canal>(`/public/c/${encodeURIComponent(org)}${q({ cursor })}`);

// ── Biblioteca y vídeo ──────────────────────────────────────────────────────

export const biblioteca = (
  scope: 'mine' | 'org',
  opciones: { status?: string; q?: string; cursor?: string } = {},
) => pedir<Pagina>(`/videos${q({ scope, ...opciones })}`);
export const compartidos = (cursor?: string) => pedir<Pagina>(`/videos/shared${q({ cursor })}`);

/** Con sesión, la ficha con ACL; sin ella, la pública. */
export const video = (id: string, conSesion: boolean) =>
  pedir<Video>(`${conSesion ? '' : '/public'}/videos/${encodeURIComponent(id)}`);
export const reproducir = (id: string, conSesion: boolean) =>
  pedir<Reproduccion>(`${conSesion ? '' : '/public'}/videos/${encodeURIComponent(id)}/play`);

export const crear = (datos: {
  title: string;
  description?: string;
  visibility?: Visibilidad;
  /** Calidades elegidas; sin ellas, la recomendada (todas las que dé el original). */
  qualities?: number[];
}) => pedir<Video>('/videos', json('POST', datos));
export const actualizar = (
  id: string,
  cambios: Partial<
    Pick<
      Video,
      | 'title'
      | 'description'
      | 'visibility'
      | 'comments_enabled'
      | 'chat_enabled'
      | 'download_enabled'
    >
  >,
) => pedir<Video>(`/videos/${encodeURIComponent(id)}`, json('PATCH', cambios));
export const borrar = (id: string) =>
  pedir<void>(`/videos/${encodeURIComponent(id)}`, { method: 'DELETE' });

export async function subirPoster(id: string, imagen: File): Promise<Video> {
  return pedir<Video>(`/videos/${encodeURIComponent(id)}/poster`, {
    method: 'PUT',
    body: imagen,
    headers: { 'content-type': imagen.type || 'image/jpeg' },
  });
}
export const posterDeFotograma = (id: string, atMs: number) =>
  pedir<Video>(
    `/videos/${encodeURIComponent(id)}/poster:frame`,
    json('POST', { at_ms: Math.round(atMs) }),
  );

// ── Subir desde Sytue Drive ─────────────────────────────────────────────────

/**
 * Comprueba, sin crear nada, que el fichero de Drive se puede subir (existe para
 * quien pregunta, es un vídeo, cabe en el tope y en la cuota) y dice cuál es.
 */
export const ficheroDeDrive = (fileId: string) =>
  pedir<FicheroDeDrive>(`/videos:from-drive${q({ file_id: fileId })}`);
/** Crea el vídeo y deja la copia en marcha en el servicio: vuelve «subiendo» (202). */
export const subirDeDrive = (datos: {
  file_id: string;
  title?: string;
  description?: string;
  visibility?: Visibilidad;
  qualities?: number[];
}) => pedir<Video>('/videos:from-drive', json('POST', datos));

// ── Calidades ───────────────────────────────────────────────────────────────

/** Cuánto ocupará cada calidad de un original; duración y dimensiones si se saben. */
export const estimar = (m: {
  bytes: number;
  duracionMs?: number | null;
  ancho?: number | null;
  alto?: number | null;
}) =>
  pedir<Estimacion>(
    `/videos:estimate${q({
      size_bytes: String(Math.max(0, Math.round(m.bytes))),
      duration_ms: m.duracionMs ? String(Math.round(m.duracionMs)) : undefined,
      width: m.ancho && m.alto ? String(m.ancho) : undefined,
      height: m.ancho && m.alto ? String(m.alto) : undefined,
    })}`,
  );
export const calidadesDe = (id: string) =>
  pedir<CalidadesDelVideo>(`/videos/${encodeURIComponent(id)}/qualities`);
/** Añadir genera y quitar borra, en segundo plano (202). */
export const elegirCalidades = (id: string, calidades: number[]) =>
  pedir<CalidadesDelVideo>(
    `/videos/${encodeURIComponent(id)}/qualities`,
    json('PUT', { qualities: calidades }),
  );

// ── Personas ────────────────────────────────────────────────────────────────

export const personas = (id: string) =>
  pedir<{ people: Persona[] }>(`/videos/${encodeURIComponent(id)}/people`).then((r) => r.people);
export const anadirPersona = (id: string, consulta: string) =>
  pedir<Persona>(`/videos/${encodeURIComponent(id)}/people`, json('POST', { query: consulta }));
export const quitarPersona = (id: string, userId: string) =>
  pedir<void>(`/videos/${encodeURIComponent(id)}/people/${encodeURIComponent(userId)}`, {
    method: 'DELETE',
  });

// ── Cuota ───────────────────────────────────────────────────────────────────

export const cuota = () => pedir<Cuota>('/quota');

// ── Social ──────────────────────────────────────────────────────────────────

export const comentarios = (id: string, conSesion: boolean, cursor?: string) =>
  pedir<{ comments: Comentario[]; next_cursor?: string }>(
    `${conSesion ? '' : '/public'}/videos/${encodeURIComponent(id)}/comments${q({ cursor })}`,
  );
export const comentar = (id: string, body: string, replyTo?: string) =>
  pedir<Comentario>(
    `/videos/${encodeURIComponent(id)}/comments`,
    json('POST', { body, ...(replyTo ? { reply_to: replyTo } : {}) }),
  );
export const borrarComentario = (id: string, cid: string) =>
  pedir<void>(`/videos/${encodeURIComponent(id)}/comments/${encodeURIComponent(cid)}`, {
    method: 'DELETE',
  });
export const historialChat = (id: string) =>
  pedir<{ messages: MensajeChat[] }>(`/videos/${encodeURIComponent(id)}/chat`).then(
    (r) => r.messages,
  );
export const borrarMensaje = (id: string, mid: string) =>
  pedir<void>(`/videos/${encodeURIComponent(id)}/chat/${encodeURIComponent(mid)}`, {
    method: 'DELETE',
  });

// ── Denuncia y bloqueo ──────────────────────────────────────────────────────

/**
 * Manda una denuncia. Con sesión va tal cual; sin ella, por la ruta pública y con la
 * prueba de trabajo ALTCHA resuelta aquí (unas décimas de segundo). El servicio
 * contesta lo mismo haya o no una denuncia igual ya abierta.
 */
export async function denunciar(
  objetivo: ObjetivoDeDenuncia,
  datos: { reason: Motivo; details: string },
  conSesion: boolean,
): Promise<void> {
  if (conSesion) {
    const p = peticionDeDenuncia(objetivo, datos, null);
    return pedir<void>(p.ruta, json('POST', p.cuerpo));
  }
  const carga = await resolverReto(await pedir<RetoAltcha>('/public/reports/altcha'));
  if (!carga) throw new FalloDelServicio(400, 'altcha_invalid');
  const p = peticionDeDenuncia(objetivo, datos, carga);
  return pedir<void>(p.ruta, json('POST', p.cuerpo));
}

/** Bloquea a alguien que aparece en ese vídeo (su dueño o quien ha escrito en él). */
export const bloquear = (videoId: string, sub: string) =>
  pedir<void>(`/videos/${encodeURIComponent(videoId)}/blocks`, json('POST', { sub }));
export const bloqueados = () => pedir<{ blocks: Bloqueado[] }>('/blocks').then((r) => r.blocks);
export const desbloquear = (sub: string) =>
  pedir<void>(`/blocks/${encodeURIComponent(sub)}`, { method: 'DELETE' });

// Lo de quien administra la organización: la cola y los bloqueos de toda ella.
export const resumenDeModeracion = () => pedir<{ open: number }>('/moderation/summary');
export const denuncias = (resueltas: boolean, cursor?: string) =>
  pedir<{ reports: Denuncia[]; next_cursor?: string }>(
    `/moderation/reports${q({ status: resueltas ? 'resolved' : 'open', cursor })}`,
  );
export const resolverDenuncia = (id: string, action: AccionDeDenuncia) =>
  pedir<void>(`/moderation/reports/${encodeURIComponent(id)}`, json('POST', { action }));
export const bloqueadosEnOrg = () =>
  pedir<{ blocks: Bloqueado[] }>('/moderation/blocks').then((r) => r.blocks);
export const desbloquearEnOrg = (sub: string) =>
  pedir<void>(`/moderation/blocks/${encodeURIComponent(sub)}`, { method: 'DELETE' });

// ── Directo ─────────────────────────────────────────────────────────────────

/** La presentación de un directo (o de su grabación); null si no lleva ninguna. */
export const presentacion = (id: string, conSesion: boolean) =>
  pedir<{ presentation: Presentacion | null }>(
    `${conSesion ? '' : '/public'}/videos/${encodeURIComponent(id)}/slides`,
  ).then((r) => r.presentation);

export const claveDeEmision = () => pedir<ClaveDeEmision>('/studio/stream-key');
export const regenerarClave = () =>
  pedir<ClaveDeEmision>('/studio/stream-key:rotate', { method: 'POST' });
export const programar = (datos: {
  title: string;
  description?: string;
  visibility: Visibilidad;
  source: 'rtmp' | 'webrtc';
}) => pedir<{ video: Video; publish?: ConexionDirecto }>('/lives', json('POST', datos));
export const publicacion = (id: string) =>
  pedir<ConexionDirecto>(`/lives/${encodeURIComponent(id)}/publish`);
export const terminar = (id: string) =>
  pedir<Video>(`/lives/${encodeURIComponent(id)}/end`, { method: 'POST' });

// ── Subida del original (con progreso) ─────────────────────────────────────

/**
 * PUT del original con XHR: `fetch` todavía no da progreso de SUBIDA en ningún
 * navegador. El cuerpo es el `File` tal cual, así que el navegador pone el
 * `Content-Length` que el servicio exige y lo manda en streaming; el proxy del BFF
 * lo reenvía sin leerlo a memoria.
 */
export function subirOriginal(
  id: string,
  fichero: File,
  alProgresar: (fraccion: number) => void,
  senal?: AbortSignal,
): Promise<Video> {
  return new Promise((resolver, rechazar) => {
    const xhr = new XMLHttpRequest();
    xhr.open('PUT', api(`/videos/${encodeURIComponent(id)}/original`));
    xhr.withCredentials = true;
    xhr.setRequestHeader('content-type', fichero.type || 'video/mp4');
    xhr.setRequestHeader('accept', 'application/json');
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) alProgresar(e.loaded / e.total);
    };
    xhr.onerror = () => rechazar(new TypeError('network'));
    xhr.onabort = () => rechazar(new DOMException('abortada', 'AbortError'));
    xhr.onload = () => {
      let cuerpo: unknown = null;
      try {
        cuerpo = JSON.parse(xhr.responseText);
      } catch {
        // cuerpo vacío o no JSON
      }
      if (xhr.status >= 200 && xhr.status < 300) return resolver(cuerpo as Video);
      const e = (cuerpo as ErrorDelServicio | null)?.error;
      rechazar(new FalloDelServicio(xhr.status, e?.code ?? '', e?.details ?? {}));
    };
    senal?.addEventListener('abort', () => xhr.abort());
    xhr.send(fichero);
  });
}
