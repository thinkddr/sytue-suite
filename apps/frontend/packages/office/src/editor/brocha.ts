/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Copiar formato» (la brocha de Inicio › Fuente), como en un procesador de textos:
 *
 *  1. Con el cursor o la selección en un texto, se pulsa la brocha: se lleva el formato de
 *     carácter de ese punto (negrita…, fuente, talla, color, resaltado) y queda armada
 *     (el botón pulsado y el puntero de copiar sobre el folio).
 *  2. Lo siguiente que se seleccione con el ratón (arrastrar, doble clic) recibe ese
 *     formato y la brocha se desarma. Un clic suelto no la gasta: es colocar el cursor
 *     para empezar a seleccionar.
 *  3. Esc la desarma sin pintar nada. Pulsar otra vez la brocha pinta la selección actual
 *     si es otra que la de origen —el camino del teclado: Mayús+flechas y otra vez el
 *     botón— y si no, la desarma.
 *
 * Solo el formato de CARÁCTER, como la brocha de Word con una selección dentro de un
 * párrafo: el estilo de párrafo ya tiene su galería al lado, y cambiar un título en
 * párrafo por pintar unas letras sería una sorpresa. La app nativa hace lo mismo
 * (`apps/fyne/editor/docus/escribir.go`, `brocha`).
 *
 * El estado es del módulo y no del contexto del editor: lo leen el botón (para pintarse
 * pulsado) y el folio (para el puntero), y solo hay un lienzo de documento a la vez.
 */
import { copiarFormato, getEditorRoot, pegarFormato, type FormatoDeCaracter } from './canvas/exec';

interface Armada {
  formato: FormatoDeCaracter;
  /** Lo seleccionado al armarla: pulsar la brocha sobre lo mismo no lo pinta. */
  origen: Range | null;
  raiz: HTMLElement;
  quitar: () => void;
}

let armada: Armada | null = null;
const oyentes = new Set<() => void>();

const avisar = (): void => {
  for (const o of oyentes) o();
};

/** ¿Está la brocha armada? */
export function brochaArmada(): boolean {
  return armada !== null && armada.raiz.isConnected;
}

/** Avisa cada vez que la brocha se arma o se desarma; devuelve cómo dejar de oír. */
export function oirBrocha(oyente: () => void): () => void {
  oyentes.add(oyente);
  return () => oyentes.delete(oyente);
}

/** Desarma la brocha sin pintar nada. */
export function desarmarBrocha(): void {
  if (!armada) return;
  armada.quitar();
  armada = null;
  avisar();
}

function seleccionEn(raiz: HTMLElement): Range | null {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const r = sel.getRangeAt(0);
  return raiz.contains(r.commonAncestorContainer) ? r : null;
}

const mismoRango = (a: Range | null, b: Range | null): boolean =>
  a !== null &&
  b !== null &&
  a.compareBoundaryPoints(Range.START_TO_START, b) === 0 &&
  a.compareBoundaryPoints(Range.END_TO_END, b) === 0;

/** Pinta la selección actual con lo copiado y desarma; false si no hay qué pintar. */
function pintar(): boolean {
  if (!armada) return false;
  const r = seleccionEn(armada.raiz);
  if (!r || r.collapsed) return false;
  const { formato } = armada;
  desarmarBrocha(); // antes de pintar: pegarFormato mueve la selección y no debe re-entrar
  return pegarFormato(formato);
}

/**
 * El botón de la brocha. Desarmada: copia el formato bajo el cursor y la arma. Armada:
 * pinta la selección si es otra que la de origen, o la desarma.
 */
export function alternarBrocha(): void {
  // Un folio que ya no está (se cerró el documento) no tiene nada que pintar.
  if (armada && !armada.raiz.isConnected) desarmarBrocha();
  if (armada) {
    const r = seleccionEn(armada.raiz);
    if (!r || r.collapsed || mismoRango(r, armada.origen) || !pintar()) desarmarBrocha();
    return;
  }
  const raiz = getEditorRoot();
  const formato = copiarFormato();
  if (!raiz || !formato) return;
  // Al soltar el ratón ya está decidida la selección (también la del doble clic); el
  // `setTimeout` deja que el navegador termine de ajustarla antes de leerla.
  const alSoltar = (): void => {
    setTimeout(() => {
      if (armada?.raiz === raiz) pintar();
    }, 0);
  };
  const alPulsarTecla = (e: KeyboardEvent): void => {
    if (e.key === 'Escape') desarmarBrocha();
  };
  raiz.addEventListener('mouseup', alSoltar);
  document.addEventListener('keydown', alPulsarTecla, true);
  raiz.dataset.brocha = '';
  const origen = seleccionEn(raiz)?.cloneRange() ?? null;
  armada = {
    formato,
    origen,
    raiz,
    quitar: () => {
      raiz.removeEventListener('mouseup', alSoltar);
      document.removeEventListener('keydown', alPulsarTecla, true);
      delete raiz.dataset.brocha;
    },
  };
  avisar();
}
