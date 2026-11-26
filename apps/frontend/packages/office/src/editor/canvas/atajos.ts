/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos del texto editable: el folio, la caja de Diapos y el editor de celda llaman
 * aquí con su `keydown` en vez de llevar cada uno su copia de «busca en el registro y
 * despacha». El registro (`comandos.json`) dice qué combinación es qué comando; aquí solo
 * vive lo que `exec` no puede hacer a partir de una tecla: el enlace necesita una URL, así
 * que Ctrl+K la pide antes de despachar (como el botón de Referencias), en vez de mandar
 * `insertLink` vacío, que no hace nada.
 */
import { preguntarEnEditor } from '@sytue/ui';
import { editorT } from '../../i18n';
import { comandoDelAtajo, type IdComando, type Pulsacion } from '../comandos';
import { siguienteModoMayusculas } from '../mayusculas';
import { textoDeLaSeleccion } from '../recuento';
import type { EditorCommand } from '../types';

/**
 * Lo que los atajos del registro pueden hacer al texto del editor de celda: las marcas.
 * Alinear, titular o enlazar no son del texto de una celda (la alineación es de la hoja).
 */
export const MARCAS_DE_CELDA: ReadonlySet<IdComando> = new Set<IdComando>([
  'bold',
  'italic',
  'underline',
  'strike',
]);

export interface DestinoDeAtajo {
  exec: (command: EditorCommand, value?: string) => void;
}

/**
 * Atiende un atajo de comando sobre el texto editable: si la pulsación es de un comando del
 * registro, la consume (`preventDefault`) y lo despacha. `admite` acota los comandos (el
 * editor de celda solo quiere las marcas: alinear o titular una celda son de la hoja).
 * Devuelve si era un atajo suyo.
 */
export function atenderAtajoDeTexto(
  e: Pulsacion & Pick<KeyboardEvent, 'preventDefault'>,
  destino: DestinoDeAtajo,
  admite?: ReadonlySet<IdComando>,
): boolean {
  // Mayús+F3 alterna las mayúsculas de lo elegido (minúsculas, MAYÚSCULAS, Cada Palabra),
  // como en Word. No es del registro: no lleva Ctrl. En la celda de Calcus no (`admite`).
  if (!admite && e.key === 'F3' && e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
    const texto = textoDeLaSeleccion();
    if (texto === null) return false;
    e.preventDefault();
    destino.exec('changeCase', siguienteModoMayusculas(texto));
    return true;
  }
  const atajo = comandoDelAtajo(e, 'document');
  if (!atajo || (admite && !admite.has(atajo.id))) return false;
  e.preventDefault();
  if (atajo.id === 'insertLink') {
    void pedirEnlace(destino);
    return true;
  }
  destino.exec(atajo.id as EditorCommand, atajo.value);
  return true;
}

/**
 * Pide la URL y enlaza lo elegido. El diálogo se lleva el foco y, con él, la selección
 * viva del `contenteditable`: se fotografía antes y se repone al volver, para que el enlace
 * caiga donde estaba el cursor y no al principio del texto.
 */
async function pedirEnlace(destino: DestinoDeAtajo): Promise<void> {
  const sel = document.getSelection();
  const rango = sel && sel.rangeCount > 0 ? sel.getRangeAt(0).cloneRange() : null;
  const editable =
    rango && (rango.commonAncestorContainer as Node | null)
      ? (nodoElemento(rango.commonAncestorContainer)?.closest<HTMLElement>(
          '[contenteditable="true"],[contenteditable=""]',
        ) ?? null)
      : null;
  const url = (await preguntarEnEditor({ mensaje: editorT()('dialog.link.url') }))?.trim();
  if (!url) {
    editable?.focus({ preventScroll: true });
    return;
  }
  if (editable && rango) {
    editable.focus({ preventScroll: true });
    const ahora = document.getSelection();
    ahora?.removeAllRanges();
    ahora?.addRange(rango);
  }
  destino.exec('insertLink', url);
}

const nodoElemento = (n: Node): Element | null =>
  n.nodeType === Node.ELEMENT_NODE ? (n as Element) : n.parentElement;
