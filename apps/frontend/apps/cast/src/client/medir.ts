/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Duración y dimensiones de un vídeo local ANTES de subirlo, para que la
 * estimación de espacio no tenga que adivinarlas: el navegador lee solo los
 * metadatos (`preload="metadata"`), sin reproducir ni bajar el fichero entero.
 * Si no puede (un códec que no entiende, un MKV), se sigue sin ellas y la
 * estimación sale aproximada: nunca bloquea la subida.
 */

export interface Medidas {
  duracionMs: number | null;
  ancho: number | null;
  alto: number | null;
}

const SIN: Medidas = { duracionMs: null, ancho: null, alto: null };

/** Lee los metadatos de un fichero de vídeo; a los `plazoMs` se rinde. */
export function medir(fichero: File, plazoMs = 4000): Promise<Medidas> {
  if (typeof document === 'undefined' || typeof URL.createObjectURL !== 'function')
    return Promise.resolve(SIN);
  return new Promise((resolver) => {
    const v = document.createElement('video');
    const url = URL.createObjectURL(fichero);
    let hecho = false;
    const fin = (m: Medidas): void => {
      if (hecho) return;
      hecho = true;
      clearTimeout(reloj);
      v.removeAttribute('src');
      v.load();
      URL.revokeObjectURL(url);
      resolver(m);
    };
    const reloj = setTimeout(() => fin(SIN), plazoMs);
    v.preload = 'metadata';
    v.muted = true;
    v.onloadedmetadata = () =>
      fin({
        duracionMs: Number.isFinite(v.duration) && v.duration > 0 ? v.duration * 1000 : null,
        ancho: v.videoWidth || null,
        alto: v.videoHeight || null,
      });
    v.onerror = () => fin(SIN);
    v.src = url;
  });
}
