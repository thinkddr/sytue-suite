/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Atajos de teclado del editor de Sytue Web (PAGES-PLAN §14.1).
 *
 * Reglas de seguridad y accesibilidad:
 * - Atajos con Ctrl/Cmd/Alt solo se procesan si !enCampoDeTexto(e.target) && !haySuperposicionAbierta().
 * - Teclas sueltas solo se procesan en la región del lienzo o árbol.
 */

import { enCampoDeTexto, haySuperposicionAbierta } from '@sytue/ui';

export type AccionAtajo =
  | 'deshacer'
  | 'rehacer'
  | 'duplicar'
  | 'envolverMarco'
  | 'desenvolverMarco'
  | 'eliminar'
  | 'zoomAjustar'
  | 'zoom100'
  | 'zoomMas'
  | 'zoomMenos'
  | 'escape';

export interface ManejadorAtajos {
  onDeshacer?: () => void;
  onRehacer?: () => void;
  onDuplicar?: () => void;
  onEnvolverMarco?: () => void;
  onDesenvolverMarco?: () => void;
  onEliminar?: () => void;
  onZoomAjustar?: () => void;
  onZoom100?: () => void;
  onZoomMas?: () => void;
  onZoomMenos?: () => void;
  onEscape?: () => void;
}

/**
 * Resuelve la acción asociada a un KeyboardEvent si procede, o null si la tecla
 * debe dejarse al navegador o al control que tiene el foco.
 */
export function clasificarAtajo(e: KeyboardEvent): AccionAtajo | null {
  if (enCampoDeTexto(e.target) || haySuperposicionAbierta()) {
    return null;
  }

  const conModificador = e.ctrlKey || e.metaKey;

  if (conModificador) {
    const k = e.key.toLowerCase();
    if (k === 'z' && !e.shiftKey) return 'deshacer';
    if (k === 'y' || (k === 'z' && e.shiftKey)) return 'rehacer';
    if (k === 'd') return 'duplicar';
    if (k === 'g' && !e.shiftKey) return 'envolverMarco';
    if (k === 'g' && e.shiftKey) return 'desenvolverMarco';
    if (e.key === '0') return 'zoomAjustar';
    if (e.key === '1') return 'zoom100';
    if (e.key === '+' || e.key === '=') return 'zoomMas';
    if (e.key === '-' || e.key === '_') return 'zoomMenos';
    return null;
  }

  if (e.key === 'Delete' || e.key === 'Backspace') {
    return 'eliminar';
  }
  if (e.key === 'Escape') {
    return 'escape';
  }

  return null;
}

/**
 * Procesa un evento de teclado invocando la acción correspondiente.
 * Devuelve true si el evento fue consumido (preventDefault llamado).
 */
export function procesarAtajo(e: KeyboardEvent, manejador: ManejadorAtajos): boolean {
  const accion = clasificarAtajo(e);
  if (!accion) return false;

  switch (accion) {
    case 'deshacer':
      if (manejador.onDeshacer) {
        e.preventDefault();
        manejador.onDeshacer();
        return true;
      }
      break;
    case 'rehacer':
      if (manejador.onRehacer) {
        e.preventDefault();
        manejador.onRehacer();
        return true;
      }
      break;
    case 'duplicar':
      if (manejador.onDuplicar) {
        e.preventDefault();
        manejador.onDuplicar();
        return true;
      }
      break;
    case 'envolverMarco':
      if (manejador.onEnvolverMarco) {
        e.preventDefault();
        manejador.onEnvolverMarco();
        return true;
      }
      break;
    case 'desenvolverMarco':
      if (manejador.onDesenvolverMarco) {
        e.preventDefault();
        manejador.onDesenvolverMarco();
        return true;
      }
      break;
    case 'eliminar':
      if (manejador.onEliminar) {
        e.preventDefault();
        manejador.onEliminar();
        return true;
      }
      break;
    case 'zoomAjustar':
      if (manejador.onZoomAjustar) {
        e.preventDefault();
        manejador.onZoomAjustar();
        return true;
      }
      break;
    case 'zoom100':
      if (manejador.onZoom100) {
        e.preventDefault();
        manejador.onZoom100();
        return true;
      }
      break;
    case 'zoomMas':
      if (manejador.onZoomMas) {
        e.preventDefault();
        manejador.onZoomMas();
        return true;
      }
      break;
    case 'zoomMenos':
      if (manejador.onZoomMenos) {
        e.preventDefault();
        manejador.onZoomMenos();
        return true;
      }
      break;
    case 'escape':
      if (manejador.onEscape) {
        e.preventDefault();
        manejador.onEscape();
        return true;
      }
      break;
  }
  return false;
}
