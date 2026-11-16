/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Adjuntos, del lado de la interfaz. El límite es espejo de
 * `services/bases/internal/domain/adjuntos.go` (el servicio manda).
 */
import { HttpError } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { mensajeDeError } from './api.js';
import { ruta } from './rutas.js';

export const MAX_BYTES_ADJUNTO = 20 << 20;

/**
 * La dirección de un adjunto en el BFF, o la de su miniatura: el diálogo de adjuntos y
 * la portada de la galería piden lo mismo y no deben divergir.
 */
export function urlDeAdjunto(registro: string, adjunto: string, miniatura = false): string {
  return ruta(`/api/records/${registro}/attachments/${adjunto}${miniatura ? '/thumb' : ''}`);
}

/**
 * El mensaje de un fallo al subir un adjunto: el diálogo de adjuntos y lo soltado sobre
 * la rejilla dicen lo mismo (`errorDeAdjunto` en Fyne).
 */
export function errorDeAdjunto(e: unknown, t: T): string {
  if (e instanceof HttpError) {
    if (e.status === 413) return t('attach.tooLarge');
    if (e.status === 422) return t('attach.badType');
    if (e.status === 503) return t('attach.unavailable');
  }
  return mensajeDeError(e, t);
}
