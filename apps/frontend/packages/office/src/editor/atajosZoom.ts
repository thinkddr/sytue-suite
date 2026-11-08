/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ctrl/⌘ +, − y 0 sobre el zoom del documento, como en CertiPDF (`PdfApp.tsx`) y en
 * cualquier suite de escritorio. Sin esto el atajo se lo quedaba el NAVEGADOR, que
 * amplía la página entera —cinta, paneles y barra incluidos— mientras el «100 %» de
 * la barra de estado seguía diciendo 100: dos zooms que no se conocían.
 *
 * El gemelo de Fyne es `Contexto.AtajoZoom` (apps/fyne/editor/servicio.go).
 */
import { clampZoom, ZOOM_STEP } from './canvas/metrics';

/** Lo que el atajo necesita de un KeyboardEvent (los tests no montan uno entero). */
export type TeclaZoom = Pick<KeyboardEvent, 'key' | 'code' | 'ctrlKey' | 'metaKey' | 'altKey'>;

/**
 * El zoom que pide la tecla, o null si no es un atajo de zoom.
 *
 * `+` es Mayús+`=` en muchos teclados, así que vale `=` también; `_` es Mayús+`-`.
 * El 0 se mira por `code` además de por `key`: en un AZERTY el 0 de la fila de
 * números sin Mayús es «à», y Ctrl+0 tiene que seguir siendo Ctrl+0.
 */
export function zoomDeAtajo(e: TeclaZoom, zoom: number): number | null {
  if (!(e.ctrlKey || e.metaKey) || e.altKey) return null;
  if (e.key === '+' || e.key === '=') return clampZoom(zoom + ZOOM_STEP);
  if (e.key === '-' || e.key === '_') return clampZoom(zoom - ZOOM_STEP);
  if (e.key === '0' || e.code === 'Digit0' || e.code === 'Numpad0') return 100;
  return null;
}
