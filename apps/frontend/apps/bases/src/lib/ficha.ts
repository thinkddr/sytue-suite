/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La ficha de un registro: todos sus campos en un formulario, cada uno con su control.
 * Lo que se lleva escrito vive en un BORRADOR (un valor por campo, como lo da su control)
 * y al guardar se convierte en lo que viaja, pero SOLO lo cambiado: abrir una ficha y
 * guardarla sin tocar nada no escribe, y no pisa lo que otra persona haya cambiado
 * mientras tanto en otro campo. Es espejo de `fichaRegistro.cambios()` de la ficha de
 * Fyne (`apps/fyne/apps/bases/registro.go`), y el servicio vuelve a validarlo todo.
 */
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import { enlacesDe, textoDeValor, valorDeTexto } from './valores.js';

/** Lo que dice el control de un campo: texto, casilla o lista de marcados. */
export type ValorFicha = string | boolean | string[];
export type BorradorFicha = Record<string, ValorFicha>;

export interface CambiosFicha {
  /** Los valores que difieren de los del registro (null es vaciar). */
  datos: Record<string, BasesValue>;
  /** Los enlaces que difieren, por campo. */
  enlaces: Record<string, string[]>;
  /** Los campos cuyo texto no encaja en su tipo: no se envía nada mientras haya alguno. */
  invalidos: string[];
}

/**
 * Si el campo se escribe desde la ficha. Lo calculado no; los adjuntos tampoco, porque
 * se suben y se borran en el momento (`DialogoAdjuntos`), no con «Guardar».
 */
export function editableEnFicha(c: BasesField): boolean {
  return c.kind !== 'formula' && c.kind !== 'attachment';
}

function cadenas(v: BasesValue | undefined): string[] {
  if (Array.isArray(v)) return v.filter((x): x is string => typeof x === 'string');
  return typeof v === 'string' && v ? [v] : [];
}

/** El borrador con el que se abre la ficha: lo que el registro tiene ahora. */
export function borradorDe(campos: readonly BasesField[], r: BasesRecord): BorradorFicha {
  const b: BorradorFicha = {};
  for (const c of campos) {
    if (!editableEnFicha(c)) continue;
    const v = r.data[c.id];
    switch (c.kind) {
      case 'checkbox':
        b[c.id] = v === true;
        break;
      case 'select':
        b[c.id] = typeof v === 'string' ? v : '';
        break;
      case 'multiselect':
        b[c.id] = cadenas(v);
        break;
      case 'link':
        b[c.id] = [...enlacesDe(r, c)];
        break;
      default:
        b[c.id] = textoDeValor(c, v);
    }
  }
  return b;
}

/**
 * Las opciones de un selector. Un valor que ya no es una opción (la quitaron del campo)
 * se sigue ofreciendo: abrir y guardar la ficha no puede borrarlo sin que nadie lo toque.
 */
export function opcionesDeSelect(c: BasesField, actual: string): string[] {
  const opciones = [...(c.choices ?? [])];
  if (actual && !opciones.includes(actual)) opciones.push(actual);
  return opciones;
}

/**
 * Marca o desmarca `id` en una lista de varios. Lo marcado sale en el orden de las
 * opciones, no en el de los toques; lo que no está entre ellas (un valor que ya no es
 * opción, un enlace más allá de las que se cargan) se CONSERVA detrás, en su orden.
 */
export function alternar(
  lista: readonly string[],
  id: string,
  marcar: boolean,
  orden: readonly string[],
): string[] {
  const marcados = new Set(lista);
  if (marcar) marcados.add(id);
  else marcados.delete(id);
  const conocidos = new Set(orden);
  const extra = [...lista, id].filter((x, i, a) => !conocidos.has(x) && a.indexOf(x) === i);
  return [...orden.filter((x) => marcados.has(x)), ...extra.filter((x) => marcados.has(x))];
}

function iguales(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((x, i) => x === b[i]);
}

/**
 * Lo que hay que escribir: lo que difiere del registro tal como se abrió la ficha (no
 * del de ahora: si otra persona ha cambiado un campo que aquí no se ha tocado, se queda
 * con su valor). Se compara como se lee: un 21 escrito «21» no es un cambio.
 */
export function cambiosDeFicha(
  campos: readonly BasesField[],
  r: BasesRecord,
  borrador: BorradorFicha,
): CambiosFicha {
  const out: CambiosFicha = { datos: {}, enlaces: {}, invalidos: [] };
  for (const c of campos) {
    if (!editableEnFicha(c) || !(c.id in borrador)) continue;
    const b = borrador[c.id]!;
    if (c.kind === 'link') {
      const ids = Array.isArray(b) ? b : [];
      const antes = enlacesDe(r, c);
      if (!iguales(ids, antes)) out.enlaces[c.id] = ids;
      continue;
    }
    let valor: BasesValue | undefined;
    switch (c.kind) {
      case 'checkbox':
        valor = b === true ? true : null;
        break;
      case 'select':
        // Del selector solo sale una opción ofrecida (o la que ya tenía): no se revalida.
        valor = typeof b === 'string' && b ? b : null;
        break;
      case 'multiselect':
        valor = Array.isArray(b) && b.length > 0 ? b : null;
        break;
      default:
        valor = valorDeTexto(c, typeof b === 'string' ? b : '');
    }
    if (valor === undefined) {
      out.invalidos.push(c.id);
      continue;
    }
    if (textoDeValor(c, r.data[c.id]) !== textoDeValor(c, valor)) out.datos[c.id] = valor;
  }
  return out;
}

/** Si hay algo que escribir. */
export function hayCambios(c: CambiosFicha): boolean {
  return Object.keys(c.datos).length > 0 || Object.keys(c.enlaces).length > 0;
}
