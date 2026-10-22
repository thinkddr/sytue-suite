/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Comparación VISUAL de dos PDF: lo poco que la pantalla decide por su cuenta. El
 * juicio —qué píxeles difieren, cómo se agrupan en regiones— es del motor
 * (`domain.CompararVisual`); aquí solo se ordenan las regiones en «paradas» para
 * navegar de una a otra, se traducen a claves de catálogo y se sitúan las cajas sobre
 * la imagen. El cliente nativo (`apps/fyne/apps/certipdf/comparar.go`) hace lo mismo.
 */
import type { DiferenciaVisual, RegionVisual } from './api';
import type { DocsKey } from '../i18n';

/**
 * Cuánto cambio de tono por canal se ignora. «Normal» es el defecto del motor (48):
 * absorbe el ruido de antialiasing de un texto que se corrió una fracción de píxel
 * sin perder una coma cambiada. «Alta» ve más, a cambio de algo de ruido; «Baja»
 * deja pasar los cambios de tono suave (un gris por otro).
 */
export const SENSIBILIDADES = [
  { id: 'alta', tolerancia: 24, clave: 'pdf.dialog.comparar.visual.sens.alta' },
  { id: 'normal', tolerancia: 48, clave: 'pdf.dialog.comparar.visual.sens.normal' },
  { id: 'baja', tolerancia: 96, clave: 'pdf.dialog.comparar.visual.sens.baja' },
] as const satisfies readonly { id: string; tolerancia: number; clave: DocsKey }[];

export type Sensibilidad = (typeof SENSIBILIDADES)[number]['id'];

export const SENSIBILIDAD_POR_DEFECTO: Sensibilidad = 'normal';

export function toleranciaDe(s: Sensibilidad): number {
  return SENSIBILIDADES.find((x) => x.id === s)?.tolerancia ?? 48;
}

/** Un sitio al que ir: una región de una página, o una página entera si no tiene cajas. */
export interface Parada {
  pagina: number;
  /** Índice de la región dentro de la página, o -1 si la parada es la página entera. */
  region: number;
}

/**
 * Las paradas del documento, en orden de lectura. Una página sin regiones pero con
 * algo que decir (solo existe en uno, o no se pudo comparar) cuenta como UNA parada:
 * saltarla sería esconder que falta una página.
 */
export function paradasDe(paginas: readonly DiferenciaVisual[]): Parada[] {
  const out: Parada[] = [];
  for (const p of paginas) {
    const n = p.regiones?.length ?? 0;
    if (n === 0) out.push({ pagina: p.pagina, region: -1 });
    for (let i = 0; i < n; i++) out.push({ pagina: p.pagina, region: i });
  }
  return out;
}

/** La parada siguiente o anterior, DANDO LA VUELTA en los extremos. */
export function moverParada(actual: number, total: number, paso: 1 | -1): number {
  if (total <= 0) return 0;
  return (((actual + paso) % total) + total) % total;
}

/** Dónde pintar una caja sobre la imagen del lienzo, en porcentajes del lienzo. */
export function cajaEnPorcentaje(
  r: RegionVisual,
  ancho: number,
  alto: number,
): { left: string; top: string; width: string; height: string } {
  const pct = (v: number, total: number) =>
    `${total > 0 ? Math.min(100, Math.max(0, (v / total) * 100)).toFixed(3) : '0'}%`;
  return {
    left: pct(r.x, ancho),
    top: pct(r.y, alto),
    width: pct(r.ancho, ancho),
    height: pct(r.alto, alto),
  };
}

/** La frase de una página que no se pudo comparar o que solo está en un documento. */
export function claveDeAviso(p: DiferenciaVisual): DocsKey | null {
  if (p.solo_en === 'a') return 'pdf.dialog.comparar.visual.soloA';
  if (p.solo_en === 'b') return 'pdf.dialog.comparar.visual.soloB';
  if (p.omitida) {
    return p.motivo === 'demasiado_grande'
      ? 'pdf.dialog.comparar.visual.omitida.grande'
      : 'pdf.dialog.comparar.visual.omitida.ilegible';
  }
  return null;
}

/** El PNG de la respuesta, listo para un `<img src>`. */
export function srcDeImagen(base64: string): string {
  return `data:image/png;base64,${base64}`;
}

/** `0,0123 %` o `12,3 %`: pocos decimales cuanto más grande, y nunca «0 %» si difiere. */
export function porcentajeLegible(v: number, locale: string): string {
  if (v <= 0) return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(0);
  const decimales = v >= 10 ? 1 : v >= 1 ? 2 : 3;
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: 0,
    maximumFractionDigits: decimales,
  }).format(v < 0.001 ? 0.001 : v);
}
