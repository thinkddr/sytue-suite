/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El menú de la pestaña de hoja (SheetCanvas): qué se puede hacer con una hoja desde
 * su pestaña y con qué teclas se llega. Va aparte del lienzo para poder probarlo sin
 * montar la rejilla, y porque Fyne (`apps/fyne/calcus/pestanas.go`) sigue las mismas
 * reglas.
 */
import type { SheetModel } from '../types';

/** Lo que admite `tabColor`: un #RRGGBB, como `sanitizeSheet` y el import de .xlsx. */
const HEX = /^#[0-9a-f]{6}$/i;

/**
 * El libro con la pestaña `i` de otro color; `null` (o un valor que no es #RRGGBB) la
 * deja sin color. Devuelve el MISMO modelo si no cambia nada, para no apuntar en el
 * historial un paso que no hace nada.
 */
export function conColorDePestana(model: SheetModel, i: number, color: string | null): SheetModel {
  const hoja = model.sheets[i];
  if (!hoja) return model;
  const nuevo = color && HEX.test(color) ? color.toUpperCase() : undefined;
  if ((hoja.tabColor?.toUpperCase() ?? undefined) === nuevo) return model;
  const { tabColor: _fuera, ...resto } = hoja;
  return {
    ...model,
    sheets: model.sheets.map((s, j) =>
      j === i ? (nuevo ? { ...resto, tabColor: nuevo } : resto) : s,
    ),
  };
}

/**
 * ¿Son dos nombres de hoja el mismo salvo mayúsculas («Ventas» y «VENTAS»)? Excel y
 * LibreOffice no dejan tener los dos en un libro, y el servidor rechaza el guardado que
 * los crea (`nombrehoja.GemelaNueva`, con `strings.EqualFold` de Go). Por eso se mira
 * también en mayúsculas y no solo en minúsculas como `mismaHoja`: «ſ» y «s», o «ϑ» y «θ»,
 * son la misma letra para EqualFold y no en minúsculas; así la web nunca deja pasar lo
 * que el servidor rechazaría.
 */
export function nombresGemelos(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase() || a.toUpperCase() === b.toUpperCase();
}

/**
 * La hoja de `nombres`, distinta de `excepto`, que se llama como `nombre` salvo
 * mayúsculas; -1 si ninguna. Es `HojaGemela` de Fyne.
 */
export function hojaGemela(nombres: readonly string[], nombre: string, excepto = -1): number {
  return nombres.findIndex((n, i) => i !== excepto && nombresGemelos(n, nombre));
}

/**
 * El primer nombre de la serie `nombrar(desde)`, `nombrar(desde + 1)`… que no es gemelo
 * de ninguna hoja: «Hoja 3», o «Hoja 4» si ya hay una «hoja 3»; «Datos (2)», o «Datos
 * (3)» si ya hay una «DATOS (2)». `nombrar` da un nombre distinto para cada número.
 */
export function nombreLibreDeHoja(
  nombres: readonly string[],
  nombrar: (n: number) => string,
  desde: number,
): string {
  // Cada hoja ocupa a lo sumo un nombre de la serie: con length + 1 intentos hay uno libre.
  let n = desde;
  for (; n < desde + nombres.length; n++) {
    const s = nombrar(n);
    if (hojaGemela(nombres, s) < 0) return s;
  }
  return nombrar(n);
}

/** Qué opciones del menú de la pestaña `i` están vivas. */
export interface OpcionesPestana {
  renombrar: boolean;
  duplicar: boolean;
  color: boolean;
  izquierda: boolean;
  derecha: boolean;
  eliminar: boolean;
}

/**
 * Con el libro protegido TODO lo del menú es estructura —también el color, como en
 * Excel, donde «Proteger libro» apaga «Color de pestaña»—, así que sale inerte; y en
 * solo lectura, igual (como `Lienzo.OpcionesPestana` de Fyne). Mover depende además del
 * sitio, y eliminar de que quede otra hoja.
 */
export function opcionesDePestana(
  model: SheetModel,
  i: number,
  soloLectura = false,
): OpcionesPestana {
  const libre = !soloLectura && !model.protect && i >= 0 && i < model.sheets.length;
  return {
    renombrar: libre,
    duplicar: libre,
    color: libre,
    izquierda: libre && i > 0,
    derecha: libre && i < model.sheets.length - 1,
    eliminar: libre && model.sheets.length > 1,
  };
}

/**
 * Las teclas de la pestaña con el foco: F2 renombra (la de Excel y la de cualquier
 * explorador de archivos) y Mayús+F10 o la tecla de menú abren su menú, que es el
 * patrón de WAI-ARIA para el menú contextual. Sin esto renombrar solo se podía con
 * doble clic, que no alcanza ni el teclado (WCAG 2.1.1) ni el dedo.
 */
export function accionDeTeclaEnPestana(e: {
  key: string;
  shiftKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  metaKey: boolean;
}): 'renombrar' | 'menu' | null {
  if (e.ctrlKey || e.altKey || e.metaKey) return null;
  if (e.key === 'F2' && !e.shiftKey) return 'renombrar';
  if (esTeclaDeMenu(e)) return 'menu';
  return null;
}

/**
 * ¿Es la tecla que abre un menú contextual? Mayús+F10 o la tecla Menú, sin otros
 * modificadores. La usan la pestaña de hoja y la rejilla (el menú de la celda activa).
 */
export function esTeclaDeMenu(e: {
  key: string;
  shiftKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  metaKey: boolean;
}): boolean {
  if (e.ctrlKey || e.altKey || e.metaKey) return false;
  return (e.key === 'F10' && e.shiftKey) || e.key === 'ContextMenu';
}
