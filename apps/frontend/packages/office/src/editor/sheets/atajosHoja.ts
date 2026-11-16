/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los atajos de la rejilla de Calcus que no son de un comando de la cinta —rellenar
 * hacia abajo y a la derecha, la fecha de hoy, la fila o la columna entera— y la hoja
 * de atajos (`ShortcutsModal`) que los enseña junto a los de siempre. Quien viene de
 * Excel o de Sheets los busca con los dedos antes que con el ratón, y una tecla que
 * nadie enseña no la usa nadie.
 *
 * Aquí vive lo que no es pantalla: qué rango se copia sobre cuál, qué número es «hoy»
 * y la lista de la hoja. La app nativa tiene lo mismo en `apps/fyne/calcus/atajos.go`.
 */
import { esMac, type GrupoDeAtajos } from '@sytue/ui';
import type { DocsKey, DocsT } from '../../i18n';
import { ATAJOS_DEL_MARCO, atajosDelRegistro, textoAtajo } from '../atajosAyuda';
import { parseEsDate } from './format';

export interface RectHoja {
  c1: number;
  c2: number;
  r1: number;
  r2: number;
}

/**
 * Ctrl+D (abajo) y Ctrl+R (derecha), con el criterio de Excel: con un rango, la primera
 * fila (o columna) se copia sobre el resto; con una sola fila (o columna) seleccionada,
 * se copia la de encima (o la de la izquierda). Null si no hay de dónde copiar: la fila 1
 * con Ctrl+D o la columna A con Ctrl+R.
 */
export function rellenoDeAtajo(
  rc: RectHoja,
  sentido: 'abajo' | 'derecha',
): { origen: RectHoja; destino: RectHoja } | null {
  if (sentido === 'abajo') {
    if (rc.r1 === rc.r2) {
      if (rc.r1 === 0) return null;
      return { origen: { ...rc, r1: rc.r1 - 1, r2: rc.r1 - 1 }, destino: rc };
    }
    return { origen: { ...rc, r2: rc.r1 }, destino: { ...rc, r1: rc.r1 + 1 } };
  }
  if (rc.c1 === rc.c2) {
    if (rc.c1 === 0) return null;
    return { origen: { ...rc, c1: rc.c1 - 1, c2: rc.c1 - 1 }, destino: rc };
  }
  return { origen: { ...rc, c2: rc.c1 }, destino: { ...rc, c1: rc.c1 + 1 } };
}

/**
 * Ctrl+; escribe la fecha de HOY como valor (no `=HOY()`, que cambia cada día): el número
 * de serie del día del calendario de quien teclea, el mismo que da teclear la fecha.
 */
export function serialDeHoy(ahora: Date): number {
  const iso = `${ahora.getFullYear()}-${ahora.getMonth() + 1}-${ahora.getDate()}`;
  return parseEsDate(iso) ?? 0;
}

/** Mayús+Espacio (filas enteras) y Ctrl+Espacio (columnas enteras) del rango, como el
 *  clic en las cabeceras: el ancla va a la columna A o a la fila 1. */
export function seleccionEntera(
  rc: RectHoja,
  eje: 'fila' | 'columna',
  nCols: number,
  nRows: number,
): { sel: { c: number; r: number }; ext: { c: number; r: number } } {
  return eje === 'fila'
    ? { sel: { c: 0, r: rc.r1 }, ext: { c: nCols - 1, r: rc.r2 } }
    : { sel: { c: rc.c1, r: 0 }, ext: { c: rc.c2, r: nRows - 1 } };
}

/** Lo que hace la rejilla con una pulsación que no es de la cinta. */
export type AtajoHoja =
  'rellenarAbajo' | 'rellenarDerecha' | 'hoy' | 'fila' | 'columna' | 'ayuda' | 'irA';

/** Lo que importa de un `keydown` (así se prueba sin DOM). */
export interface PulsacionHoja {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

/**
 * El atajo de una pulsación con la rejilla enfocada, o null. Ctrl+; y Ctrl+/ se leen por
 * el CARÁCTER y no por la tecla: en un teclado español «;» y «/» van con Mayúsculas, y lo
 * que la persona busca es el signo. Con Alt no hay atajo: AltGr es Ctrl+Alt en Windows y
 * escribe caracteres. Ctrl+G y F5 son «Ir a» de Excel: llevan al cuadro de nombres (y
 * quien los atiende les quita lo que hace el navegador, buscar y recargar).
 */
export function atajoDeHoja(e: PulsacionHoja): AtajoHoja | null {
  if (e.altKey) return null;
  const ctrl = e.ctrlKey || e.metaKey;
  if (e.key === 'F5') return ctrl || e.shiftKey ? null : 'irA';
  if (e.key === ' ') {
    if (ctrl && !e.shiftKey) return 'columna';
    if (e.shiftKey && !ctrl) return 'fila';
    return null;
  }
  if (!ctrl) return null;
  if (e.key === ';') return 'hoy';
  if (e.key === '/') return 'ayuda';
  if (e.shiftKey) return null;
  const k = e.key.toLowerCase();
  if (k === 'd') return 'rellenarAbajo';
  if (k === 'r') return 'rellenarDerecha';
  if (k === 'g') return 'irA';
  return null;
}

/** Una fila de la hoja: las combinaciones en notación de la cinta («Ctrl+Shift+V») y la
 *  clave de lo que hacen. `{x}` es el nombre de una tecla que va por el catálogo. */
interface FilaAtajo {
  teclas: string[];
  descripcion: DocsKey;
}

/** Las teclas con nombre, por el catálogo («Intro», «AvPág»). */
const NOMBRES: Record<string, DocsKey> = {
  '{space}': 'slides.present.key.space',
  '{enter}': 'slides.present.key.enter',
  '{pageDown}': 'slides.present.key.pageDown',
  '{pageUp}': 'slides.present.key.pageUp',
  '{home}': 'slides.present.key.home',
  '{end}': 'slides.present.key.end',
  '{esc}': 'slides.present.key.esc',
  '{tab}': 'sheet.shortcuts.key.tab',
  '{delete}': 'sheet.shortcuts.key.delete',
};

/**
 * La hoja, en el orden de lo que más se usa. Cada tecla que sale aquí la atiende la
 * rejilla de las dos apps; la nativa monta la misma lista en `GruposDeAtajosHoja`.
 */
export const GRUPOS_HOJA: { titulo: DocsKey; filas: FilaAtajo[] }[] = [
  {
    titulo: 'sheet.shortcuts.group.move',
    filas: [
      { teclas: ['←', '↑', '→', '↓'], descripcion: 'sheet.shortcuts.move' },
      { teclas: ['Ctrl+→'], descripcion: 'sheet.shortcuts.jump' },
      { teclas: ['{home}', 'Ctrl+{home}'], descripcion: 'sheet.shortcuts.home' },
      { teclas: ['{end}', 'Ctrl+{end}'], descripcion: 'sheet.shortcuts.end' },
      { teclas: ['{pageDown}', '{pageUp}'], descripcion: 'sheet.shortcuts.page' },
      { teclas: ['{tab}', 'Shift+{tab}'], descripcion: 'sheet.shortcuts.tab' },
      { teclas: ['{esc}'], descripcion: 'sheet.shortcuts.escape' },
      { teclas: ['Ctrl+G', 'F5'], descripcion: 'sheet.shortcuts.goTo' },
    ],
  },
  {
    titulo: 'sheet.shortcuts.group.select',
    filas: [
      { teclas: ['Shift+→'], descripcion: 'sheet.shortcuts.extend' },
      { teclas: ['Shift+{space}'], descripcion: 'sheet.shortcuts.row' },
      { teclas: ['Control+{space}'], descripcion: 'sheet.shortcuts.column' },
      { teclas: ['Ctrl+A'], descripcion: 'sheet.shortcuts.all' },
    ],
  },
  {
    titulo: 'sheet.shortcuts.group.edit',
    filas: [
      { teclas: ['{enter}', 'F2'], descripcion: 'sheet.shortcuts.edit' },
      { teclas: ['{delete}'], descripcion: 'sheet.ctx.clear' },
      { teclas: ['Ctrl+D'], descripcion: 'sheet.shortcuts.fillDown' },
      { teclas: ['Ctrl+R'], descripcion: 'sheet.shortcuts.fillRight' },
      { teclas: ['Ctrl+;'], descripcion: 'sheet.shortcuts.today' },
      { teclas: ['Ctrl+C'], descripcion: 'cmd.copy' },
      { teclas: ['Ctrl+X'], descripcion: 'cmd.cut' },
      { teclas: ['Ctrl+V'], descripcion: 'cmd.paste' },
      { teclas: ['Ctrl+Shift+V'], descripcion: 'cmd.pasteValues' },
      { teclas: ['Ctrl+Z'], descripcion: 'action.undo' },
      { teclas: ['Ctrl+Y'], descripcion: 'action.redo' },
      { teclas: ['Ctrl+B'], descripcion: 'format.bold' },
      { teclas: ['Ctrl+I'], descripcion: 'format.italic' },
      { teclas: ['Alt+{enter}'], descripcion: 'sheet.shortcuts.link' },
      // El menú de la selección (menuContextual.ts), el del clic derecho, para quien no usa
      // el ratón: en la rejilla y en la pestaña activa.
      { teclas: ['Shift+F10'], descripcion: 'sheet.shortcuts.contextMenu' },
      { teclas: ['Ctrl+/'], descripcion: 'sheet.shortcuts.help' },
    ],
  },
];

/** Una combinación como se dibuja: modificadores del sistema (⌘ en el Mac) y las teclas
 *  con nombre en el idioma de quien lee. «Control+» es la tecla Control también en el
 *  Mac (⌃): ⌘+Espacio es de Spotlight, y Excel para Mac marca la columna con ⌃Espacio. */
export function teclaDeHoja(combinacion: string, t: DocsT, mac: boolean = esMac()): string {
  const conNombres = combinacion.replace(/\{[a-zA-Z]+\}/g, (m) => {
    const clave = NOMBRES[m];
    return clave ? t(clave) : m;
  });
  if (conNombres.startsWith('Control+')) {
    const resto = conNombres.slice('Control+'.length);
    return mac ? `⌃${resto}` : textoAtajo(`Ctrl+${resto}`, t, false);
  }
  return textoAtajo(conNombres, t, mac);
}

/** Los grupos de la hoja de atajos de Calcus, en el idioma de `t`. */
export function gruposDeAtajosHoja(
  t: DocsT,
  mac: boolean = esMac(),
  omitir: ReadonlySet<string> = new Set(),
): GrupoDeAtajos[] {
  return GRUPOS_HOJA.map((g) => ({
    titulo: t(g.titulo),
    atajos: g.filas
      .filter((f) => !f.teclas.every((k) => omitir.has(k)))
      .map((f) => ({
        teclas: f.teclas.map((k) => teclaDeHoja(k, t, mac)),
        descripcion: t(f.descripcion),
      })),
  })).filter((g) => g.atajos.length > 0);
}

/**
 * Lo que la hoja de atajos del editor (`atajosAyuda.ts`, Ctrl+/ y Vista) añade en Calcus:
 * los de la rejilla, sin repetir los que ya salen del marco o del registro (copiar, pegar,
 * deshacer, negrita, la propia Ctrl+/). Una sola hoja para todo el editor: la rejilla y la
 * cinta abren la misma.
 */
export function atajosPropiosDeHoja(t: DocsT, mac: boolean = esMac()): GrupoDeAtajos[] {
  const yaEstan = new Set<string>(ATAJOS_DEL_MARCO.map((f) => f.keys));
  for (const g of atajosDelRegistro('spreadsheet')) for (const f of g.filas) yaEstan.add(f.keys);
  return gruposDeAtajosHoja(t, mac, yaEstan);
}
