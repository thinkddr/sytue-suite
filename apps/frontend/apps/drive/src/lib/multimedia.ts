/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ¿Es un fichero vídeo o audio que la vista previa de Drive sabe reproducir?
 */

/**
 * Formatos que el `<video>`/`<audio>` de los navegadores de la suite reproducen sin
 * códecs extra. Un .avi o un .mkv no están: prometer un reproductor que luego se
 * queda en negro es peor que descargarlo.
 */
const VIDEO_EXTS = new Set(['mp4', 'm4v', 'webm', 'mov', 'ogv']);
const AUDIO_EXTS = new Set(['mp3', 'm4a', 'aac', 'wav', 'ogg', 'oga', 'opus', 'flac', 'weba']);

/**
 * 'video' | 'audio' según el content-type que guardó el servicio al subir, con la
 * extensión de respaldo cuando no lo hay (o es el genérico octet-stream).
 */
export function tipoMultimedia(contentType: string, ext: string): 'video' | 'audio' | null {
  const ct = contentType.toLowerCase();
  if (ct.startsWith('video/')) return 'video';
  if (ct.startsWith('audio/')) return 'audio';
  // Con un tipo concreto que no es multimedia, la extensión no lo contradice.
  if (ct && ct !== 'application/octet-stream') return null;
  const e = ext.toLowerCase();
  if (VIDEO_EXTS.has(e)) return 'video';
  if (AUDIO_EXTS.has(e)) return 'audio';
  return null;
}
