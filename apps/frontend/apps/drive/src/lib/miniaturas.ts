/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { DriveResource } from '@sytue/api-clients';

/**
 * Content-types ráster con miniatura server-side: JPEG/PNG/GIF (stdlib de Go) y WebP
 * (x/image). AVIF y SVG no: el servicio no los decodifica y caen al icono.
 */
const THUMB_CTS = new Set(['image/jpeg', 'image/png', 'image/gif', 'image/webp']);
/** Respaldo por extensión cuando el fichero no trae content-type. */
const THUMB_EXTS = new Set(['jpg', 'jpeg', 'png', 'gif', 'webp']);

/**
 * ¿Tiene el fichero miniatura server-side? Un desajuste residual lo cubre el onError
 * del <img>. Necesita id (la miniatura se pide por id, no por ruta). Fyne tiene el
 * mismo criterio en `ConMiniatura` (apps/fyne/apps/drive/explorador.go).
 */
export function thumbable(
  item: Pick<DriveResource, 'isDirectory' | 'id' | 'contentType' | 'name'>,
): boolean {
  if (item.isDirectory || !item.id) return false;
  const ct = (item.contentType ?? '').split(';')[0]!.trim().toLowerCase();
  if (ct) return THUMB_CTS.has(ct);
  const i = item.name.lastIndexOf('.');
  return THUMB_EXTS.has(i > 0 ? item.name.slice(i + 1).toLowerCase() : '');
}
