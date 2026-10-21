/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Interpretar lo que contesta Sytue AI al solicitar una edición fotográfica inteligente.
 *
 * Se le pide al modelo de visión un objeto JSON con:
 *   - explicacion: breve justificación de los cambios realizados
 *   - ajustes: { exposicion, contraste, saturacion, temperatura }
 *   - recorte (opcional): { x, y, ancho, alto }
 *
 * Esta función es tolerante con preámbulos conversacionales del LLM o bloques ```json,
 * pero es ESTRICTA con los rangos físicos y fotográficos admitidos por la receta de Sytue Photos.
 */
import type { Paso } from '../editor/receta.js';

export interface AjustesIA {
  exposicion?: number;
  contraste?: number;
  saturacion?: number;
  temperatura?: number;
}

export interface RecorteIA {
  x: number;
  y: number;
  ancho: number;
  alto: number;
}

export interface EdicionIA {
  explicacion: string;
  ajustes: AjustesIA;
  recorte?: RecorteIA;
}

export const MAX_EXPLICACION = 500;

function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}

function redondear2(v: number): number {
  return Math.round(v * 100) / 100;
}

function redondear4(v: number): number {
  return Math.round(v * 10000) / 10000;
}

/**
 * Interpreta la respuesta del modelo de IA extrayendo los ajustes numéricos
 * y la explicación fotográfica asociada.
 */
export function interpretarEdicionIA(texto: string): EdicionIA | null {
  const crudo = extraerObjeto(texto);
  if (crudo === null) return null;

  let dato: unknown;
  try {
    dato = JSON.parse(crudo);
  } catch {
    return null;
  }

  if (typeof dato !== 'object' || dato === null || Array.isArray(dato)) return null;
  const obj = dato as Record<string, unknown>;

  const explicacion = typeof obj['explicacion'] === 'string' ? obj['explicacion'].trim() : '';
  if (explicacion === '') return null;

  const rawAjustes =
    typeof obj['ajustes'] === 'object' && obj['ajustes'] !== null && !Array.isArray(obj['ajustes'])
      ? (obj['ajustes'] as Record<string, unknown>)
      : {};

  const ajustes: AjustesIA = {};

  if (typeof rawAjustes['exposicion'] === 'number' && Number.isFinite(rawAjustes['exposicion'])) {
    ajustes.exposicion = redondear2(clamp(rawAjustes['exposicion'], -5, 5));
  }
  if (typeof rawAjustes['contraste'] === 'number' && Number.isFinite(rawAjustes['contraste'])) {
    ajustes.contraste = redondear2(clamp(rawAjustes['contraste'], -1, 1));
  }
  if (typeof rawAjustes['saturacion'] === 'number' && Number.isFinite(rawAjustes['saturacion'])) {
    ajustes.saturacion = redondear2(clamp(rawAjustes['saturacion'], -1, 1));
  }
  if (typeof rawAjustes['temperatura'] === 'number' && Number.isFinite(rawAjustes['temperatura'])) {
    ajustes.temperatura = redondear2(clamp(rawAjustes['temperatura'], -1, 1));
  }

  // Al menos un ajuste de color o recorte debe ser provisto
  let recorte: RecorteIA | undefined;
  const rawRecorte =
    typeof obj['recorte'] === 'object' && obj['recorte'] !== null && !Array.isArray(obj['recorte'])
      ? (obj['recorte'] as Record<string, unknown>)
      : null;

  if (
    rawRecorte &&
    typeof rawRecorte['x'] === 'number' &&
    typeof rawRecorte['y'] === 'number' &&
    typeof rawRecorte['ancho'] === 'number' &&
    typeof rawRecorte['alto'] === 'number' &&
    Number.isFinite(rawRecorte['x']) &&
    Number.isFinite(rawRecorte['y']) &&
    Number.isFinite(rawRecorte['ancho']) &&
    Number.isFinite(rawRecorte['alto'])
  ) {
    const x = clamp(rawRecorte['x'], 0, 0.99);
    const y = clamp(rawRecorte['y'], 0, 0.99);
    const ancho = clamp(rawRecorte['ancho'], 0.01, 1 - x);
    const alto = clamp(rawRecorte['alto'], 0.01, 1 - y);

    // Solo si el recorte es menor al 98% del área se considera un recorte real intencional
    if (ancho < 0.99 || alto < 0.99 || x > 0.01 || y > 0.01) {
      recorte = {
        x: redondear4(x),
        y: redondear4(y),
        ancho: redondear4(ancho),
        alto: redondear4(alto),
      };
    }
  }

  const tieneAjuste =
    ajustes.exposicion !== undefined ||
    ajustes.contraste !== undefined ||
    ajustes.saturacion !== undefined ||
    ajustes.temperatura !== undefined ||
    recorte !== undefined;

  if (!tieneAjuste) return null;

  return {
    explicacion: explicacion.slice(0, MAX_EXPLICACION),
    ajustes,
    ...(recorte ? { recorte } : {}),
  };
}

/**
 * Convierte un resultado de EdicionIA en una lista de Pasos de la receta de Sytue Photos.
 */
export function convertirEdicionEnPasos(edicion: EdicionIA): Paso[] {
  const pasos: Paso[] = [];

  if (edicion.recorte) {
    pasos.push({
      tipo: 'recorte',
      params: {
        x: edicion.recorte.x,
        y: edicion.recorte.y,
        ancho: edicion.recorte.ancho,
        alto: edicion.recorte.alto,
      },
    });
  }

  if (edicion.ajustes.exposicion !== undefined && edicion.ajustes.exposicion !== 0) {
    pasos.push({ tipo: 'exposicion', params: { valor: edicion.ajustes.exposicion } });
  }
  if (edicion.ajustes.contraste !== undefined && edicion.ajustes.contraste !== 0) {
    pasos.push({ tipo: 'contraste', params: { valor: edicion.ajustes.contraste } });
  }
  if (edicion.ajustes.saturacion !== undefined && edicion.ajustes.saturacion !== 0) {
    pasos.push({ tipo: 'saturacion', params: { valor: edicion.ajustes.saturacion } });
  }
  if (edicion.ajustes.temperatura !== undefined && edicion.ajustes.temperatura !== 0) {
    pasos.push({ tipo: 'temperatura', params: { valor: edicion.ajustes.temperatura } });
  }

  return pasos;
}

/**
 * Extrae el primer bloque JSON delimitado por llaves emparejadas,
 * omitiendo texto previo o posterior e ignorando llaves dentro de literales de cadena.
 */
function extraerObjeto(texto: string): string | null {
  const inicio = texto.indexOf('{');
  if (inicio < 0) return null;
  let nivel = 0;
  let enCadena = false;
  let escapado = false;
  for (let i = inicio; i < texto.length; i++) {
    const c = texto[i]!;
    if (enCadena) {
      if (escapado) escapado = false;
      else if (c === '\\') escapado = true;
      else if (c === '"') enCadena = false;
      continue;
    }
    if (c === '"') enCadena = true;
    else if (c === '{') nivel++;
    else if (c === '}') {
      nivel--;
      if (nivel === 0) return texto.slice(inicio, i + 1);
    }
  }
  return null;
}
