/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Filtrar por tipo»: carpetas, documentos, PDF, imágenes o vídeo y audio, sobre lo que
 * la carpeta ya tiene cargado. Es lo que se hace con la vista cuando se busca «aquel
 * PDF» sin recordar el nombre. Va en el cliente porque el Drive vive en la bóveda: el
 * servicio no ve los ficheros, así que no puede filtrarlos.
 *
 * El tipo sale del `contentType` y, si no lo trae, de la extensión.
 */
import type { DriveResource } from '@sytue/api-clients';

export const FILTROS_TIPO = ['todo', 'carpetas', 'documentos', 'pdf', 'imagenes', 'media'] as const;
export type FiltroTipo = (typeof FILTROS_TIPO)[number];

const EXT_IMAGEN = /\.(png|jpe?g|gif|webp|svg|heic|heif|avif|bmp|tiff?)$/i;
const EXT_MEDIA = /\.(mp4|m4v|mov|mkv|webm|avi|mp3|wav|flac|ogg|oga|m4a|opus|aac)$/i;
const EXT_DOC =
  /\.(docx?|odt|rtf|txt|md|pages|xlsx?|ods|csv|numbers|pptx?|odp|key|sytue-doc|sytue-sheet|sytue-slides)$/i;
const MIME_DOC = /(officedocument|opendocument|msword|ms-excel|ms-powerpoint|^text\/|rtf|csv)/i;

/** El tipo de un elemento para el filtro; `otros` no cae en ninguno (un .zip, un .exe). */
export function tipoDe(
  item: Pick<DriveResource, 'name' | 'isDirectory' | 'contentType'>,
): Exclude<FiltroTipo, 'todo'> | 'otros' {
  if (item.isDirectory) return 'carpetas';
  const ct = (item.contentType ?? '').toLowerCase();
  const n = item.name;
  if (ct === 'application/pdf' || /\.pdf$/i.test(n)) return 'pdf';
  if (ct.startsWith('image/') || EXT_IMAGEN.test(n)) return 'imagenes';
  if (ct.startsWith('video/') || ct.startsWith('audio/') || EXT_MEDIA.test(n)) return 'media';
  if (MIME_DOC.test(ct) || EXT_DOC.test(n)) return 'documentos';
  return 'otros';
}

/** Lo que deja ver un filtro, en el orden en que llegó. */
export function filtrarPorTipo<
  T extends Pick<DriveResource, 'name' | 'isDirectory' | 'contentType'>,
>(items: readonly T[], filtro: FiltroTipo): T[] {
  return filtro === 'todo' ? [...items] : items.filter((i) => tipoDe(i) === filtro);
}

/** El fichero anterior o siguiente de `actual` entre los que se ven (sin carpetas), o null. */
export function vecinoDe<T extends Pick<DriveResource, 'path' | 'isDirectory'>>(
  items: readonly T[],
  actual: T,
  paso: -1 | 1,
): T | null {
  const ficheros = items.filter((i) => !i.isDirectory);
  const i = ficheros.findIndex((f) => f.path === actual.path);
  if (i < 0) return null;
  return ficheros[i + paso] ?? null;
}

/** Las carpetas de una ruta, de la raíz al final: «/a/b/c» da «/a», «/a/b» y «/a/b/c». */
export function migasDe(path: string): { nombre: string; ruta: string }[] {
  const partes = path.split('/').filter(Boolean);
  return partes.map((nombre, i) => ({ nombre, ruta: '/' + partes.slice(0, i + 1).join('/') }));
}
