/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cable de Sytue Cast tal como lo pinta `services/cast/internal/http` (CAST-ENCARGO
 * §5.1), en snake_case. Lo usan el servidor (páginas SSR) y las islas: son solo tipos.
 */

export type Visibilidad = 'public' | 'unlisted' | 'org' | 'people';
export const VISIBILIDADES: readonly Visibilidad[] = ['public', 'unlisted', 'org', 'people'];

export type Estado =
  'uploading' | 'processing' | 'ready' | 'failed' | 'scheduled' | 'live' | 'ended';

export interface Video {
  id: string;
  tenant: string;
  owner: { sub: string; name: string };
  kind: 'upload' | 'live';
  status: Estado;
  visibility: Visibilidad;
  title: string;
  description: string;
  duration_ms: number | null;
  width: number | null;
  height: number | null;
  /** Relativo a la raíz del servicio (`/media/<id>/<token>/poster.jpg`). */
  poster_url: string | null;
  comments_enabled: boolean;
  chat_enabled: boolean;
  download_enabled: boolean;
  live: {
    source: 'rtmp' | 'webrtc' | '';
    started_at: string | null;
    ended_at: string | null;
    viewers_now: number;
    viewers_peak: number;
  } | null;
  views: number;
  /** Por qué falló la transcodificación; solo llega a quien puede editar. */
  failure: string;
  published_at: string | null;
  created_at: string;
  updated_at: string;
  can: { edit: boolean; delete: boolean; moderate: boolean };
  /** Calidades elegidas (alturas, de mayor a menor), ya recortadas a lo que da el original. */
  qualities: number[];
  /** Las que existen en cada escalera: HLS (web) y WebM (escritorio). */
  hls_qualities: number[];
  webm_qualities: number[];
}

/** «Presentar con Cast»: en qué diapositiva va un directo y los capítulos de su grabación. */
export interface Presentacion {
  index: number;
  total: number;
  title: string;
  started_at: string;
  updated_at: string;
  /** Un capítulo por cada cambio de diapositiva; `at_ms` desde que empezó el directo. */
  marks: { at_ms: number; index: number; title: string }[];
}

export interface Pagina {
  videos: Video[];
  next_cursor?: string;
}

export interface Portada {
  live: Video[];
  videos: Video[];
  next_cursor?: string;
}

export interface Canal {
  org: string;
  name: string;
  videos: Video[];
  next_cursor?: string;
}

/** Conexión a un room de LiveKit (ver o emitir). */
export interface ConexionDirecto {
  server_url: string;
  token: string;
  room: string;
  identity: string;
}

export interface Reproduccion {
  kind: 'upload' | 'live';
  hls_url: string | null;
  webm_url: string | null;
  /** Los WebM que hay; cada uno en `webm/<q>p.webm` junto a `webm_url`. */
  webm_qualities?: number[];
  poster_url: string | null;
  download_url?: string | null;
  duration_ms: number | null;
  live?: ConexionDirecto;
}

export interface Cuota {
  limit_bytes: number;
  used_bytes: number;
  available_bytes: number;
  cast_bytes: number;
}

/** Una calidad con lo que ocuparía (su variante HLS y su WebM, con audio). */
export interface OpcionDeCalidad {
  quality: number;
  /** El original llega a ella (o aún no se sabe). */
  allowed: boolean;
  recommended: boolean;
  bytes: number;
}

/** Cuánto ocupará cada calidad (`GET /videos:estimate`): la única estimación, la del servicio. */
export interface Estimacion {
  options: OpcionDeCalidad[];
  recommended: number[];
  original_bytes: number;
  duration_ms: number;
  /** La duración se dedujo del tamaño del original, no se midió. */
  approximate: boolean;
}

/** Las calidades de un vídeo ya creado (`GET|PUT /videos/{id}/qualities`). */
export interface CalidadesDelVideo extends Estimacion {
  qualities: number[];
  hls_qualities: number[];
  webm_qualities: number[];
  /** Lo generado aún no es lo elegido: hay un trabajo por hacer o en curso. */
  pending: boolean;
}

/** Un fichero de Sytue Drive que se va a subir a Cast (`GET /videos:from-drive`). */
export interface FicheroDeDrive {
  file_id: string;
  name: string;
  size_bytes: number;
  content_type: string;
  /** El título que llevará el vídeo si no se cambia: el nombre sin la extensión. */
  title: string;
}

export interface Persona {
  user_id: string;
  name: string;
  username?: string;
  added_at: string;
}

export interface Autor {
  sub: string;
  name: string;
  username?: string;
}

export interface Comentario {
  id: string;
  author: Autor;
  body: string;
  reply_to?: string;
  created_at: string;
  /** El hueco de un comentario borrado que conserva respuestas. */
  deleted?: boolean;
  can: { delete: boolean };
}

export interface MensajeChat {
  id: string;
  author: Autor;
  body: string;
  created_at: string;
}

export interface ClaveDeEmision {
  rtmp_url: string;
  stream_key: string;
  ingress_id: string;
}

/** El sobre de error canónico de la suite (`httpx.Fail`). */
export interface ErrorDelServicio {
  error?: { code?: string; message?: string; details?: Record<string, unknown> };
}

// ── Denuncia y bloqueo ──────────────────────────────────────────────────────

export const MOTIVOS = ['harassment', 'sexual', 'violence', 'spam', 'copyright', 'other'] as const;
export type Motivo = (typeof MOTIVOS)[number];
export type TipoDeDenuncia = 'video' | 'comment' | 'chat' | 'person' | 'channel';
export type AccionDeDenuncia = 'dismiss' | 'delete' | 'unlist' | 'unpublish' | 'block';

/** Qué se denuncia: algo de un vídeo, o el canal de una organización. */
export type ObjetivoDeDenuncia =
  | { kind: 'video'; videoId: string }
  | { kind: 'comment' | 'chat'; videoId: string; target: string }
  | { kind: 'person'; videoId: string; target: string; nombre: string }
  | { kind: 'channel'; org: string };

/** Una fila de la cola. No dice quién denunció: solo si tenía cuenta. */
export interface Denuncia {
  id: string;
  kind: TipoDeDenuncia;
  video_id?: string;
  target_id: string;
  target_sub?: string;
  target_name?: string;
  excerpt?: string;
  reason: Motivo;
  details?: string;
  anonymous: boolean;
  status: 'open' | 'dismissed' | 'actioned';
  resolution?: AccionDeDenuncia;
  created_at: string;
  resolved_at?: string;
  actions: AccionDeDenuncia[];
}

export interface Bloqueado {
  sub: string;
  name: string;
  created_at: string;
}
