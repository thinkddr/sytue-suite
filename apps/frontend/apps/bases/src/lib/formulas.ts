/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Campos de fórmula, del lado de la interfaz. La fórmula la valida y la CALCULA el
 * servicio (`services/bases/internal/domain/formula.go`); aquí solo se traduce entre lo
 * que escribe una persona —`{Precio} * {Cantidad}`, `SUM({Líneas.Importe})`— y lo que se
 * guarda, que referencia los campos por id para que renombrar un campo no rompa nada.
 * Pura, para probarla sin navegador.
 */
import type { BasesField, BasesTable } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';

/**
 * El campo tal como se filtra, ordena y agrega: una fórmula se comporta como su
 * resultado, y un campo con formato como el número o el texto que guarda (igual que
 * `Campo.Efectivo` en el servicio).
 */
export function efectivo(c: BasesField): BasesField {
  switch (c.kind) {
    case 'formula':
      return { ...c, kind: c.result ?? 'number' };
    case 'currency':
    case 'rating':
      return { ...c, kind: 'number' };
    case 'url':
    case 'email':
    case 'phone':
      return { ...c, kind: 'text' };
    default:
      return c;
  }
}

/** Sustituye cada `{…}` que esté FUERA de un texto entre comillas; null si alguna no se resuelve. */
function sustituir(expr: string, ref: (dentro: string) => string | null): string | null {
  let out = '';
  let enTexto = false;
  for (let i = 0; i < expr.length; i++) {
    const c = expr[i]!;
    if (c === '"') enTexto = !enTexto;
    if (c !== '{' || enTexto) {
      out += c;
      continue;
    }
    const fin = expr.indexOf('}', i);
    if (fin < 0) return null;
    const r = ref(expr.slice(i + 1, fin));
    if (r === null) return null;
    out += `{${r}}`;
    i = fin;
  }
  return out;
}

const porNombre = (campos: readonly BasesField[], nombre: string) =>
  campos.find((c) => c.name.trim().toLowerCase() === nombre.trim().toLowerCase());

/**
 * De nombres a ids: `{Campo}` es un campo de esta tabla y `{Enlace.Campo}`, un campo de
 * la tabla a la que apunta ese enlace. Devuelve null si algún nombre no existe.
 */
export function formulaAIds(
  expr: string,
  propios: readonly BasesField[],
  tablas: readonly BasesTable[],
): string | null {
  return sustituir(expr, (dentro) => {
    // Un nombre de campo puede llevar punto: primero el nombre entero.
    const propio = porNombre(propios, dentro);
    if (propio) return propio.id;
    for (let p = dentro.indexOf('.'); p >= 0; p = dentro.indexOf('.', p + 1)) {
      const enlace = porNombre(propios, dentro.slice(0, p));
      if (enlace?.kind !== 'link') continue;
      const destino = tablas.find((t) => t.id === enlace.link_table_id);
      const campo = destino && porNombre(destino.fields, dentro.slice(p + 1));
      if (campo) return `${enlace.id}.${campo.id}`;
    }
    return null;
  });
}

/** De ids a nombres, para enseñar una fórmula guardada. Un id desconocido se deja tal cual. */
export function formulaANombres(
  expr: string,
  propios: readonly BasesField[],
  tablas: readonly BasesTable[],
): string {
  return (
    sustituir(expr, (dentro) => {
      const [a, b] = dentro.split('.');
      const propio = propios.find((c) => c.id === a);
      if (!propio) return dentro;
      if (!b) return propio.name;
      const campo = tablas
        .find((t) => t.id === propio.link_table_id)
        ?.fields.find((c) => c.id === b);
      return campo ? `${propio.name}.${campo.name}` : dentro;
    }) ?? expr
  );
}

/** Las referencias que se pueden escribir en una fórmula de esta tabla, ya entre llaves. */
export function referenciasDeFormula(
  propios: readonly BasesField[],
  tablas: readonly BasesTable[],
): string[] {
  const out: string[] = [];
  for (const c of propios) {
    out.push(`{${c.name}}`);
    if (c.kind !== 'link') continue;
    for (const d of tablas.find((t) => t.id === c.link_table_id)?.fields ?? []) {
      const k = efectivo(d).kind;
      if (k === 'number' || k === 'date' || k === 'checkbox') out.push(`{${c.name}.${d.name}}`);
    }
  }
  return out;
}

/**
 * Lo que se envía al EDITAR la fórmula de un campo: la expresión con ids, o null si un
 * nombre no existe. `cambia` es falso si no hay nada que guardar (vacía o la misma que
 * había), para no mandar una petición que solo recalcularía la tabla entera.
 */
export function formulaEditada(
  texto: string,
  campo: BasesField,
  propios: readonly BasesField[],
  tablas: readonly BasesTable[],
): { ids: string | null; cambia: boolean } {
  if (texto.trim() === '') return { ids: null, cambia: false };
  const ids = formulaAIds(texto, propios, tablas);
  return { ids, cambia: ids !== null && ids.trim() !== (campo.formula ?? '').trim() };
}

/**
 * Las fórmulas que usan un campo —de su tabla, o de otra a través de un enlace—, por
 * nombre. El servicio no deja borrar un campo así (la fórmula quedaría apuntando a nada,
 * `BorrarCampo` en `store/estructura.go`); aquí se sabe antes, para decir CUÁLES son.
 */
export function formulasQueUsan(campo: BasesField, tablas: readonly BasesTable[]): string[] {
  const out: string[] = [];
  for (const t of tablas) {
    for (const c of t.fields) {
      if (c.kind !== 'formula' || c.id === campo.id || !c.formula) continue;
      let usa = false;
      sustituir(c.formula, (dentro) => {
        if (dentro.split('.').includes(campo.id)) usa = true;
        return dentro;
      });
      if (usa) out.push(t.id === campo.table_id ? c.name : `${t.name}.${c.name}`);
    }
  }
  return out;
}

/**
 * El aviso antes de borrar un campo, y si se puede: borrar se lleva sus valores de
 * todos los registros, y lo que usa una fórmula no se borra hasta cambiarla.
 */
export function avisoBorrarCampo(
  t: T,
  campo: BasesField,
  tablas: readonly BasesTable[],
): { mensaje: string; bloqueado: boolean } {
  const usan = formulasQueUsan(campo, tablas);
  return usan.length > 0
    ? {
        mensaje: t('field.deleteBlocked', { name: campo.name, formulas: usan.join(', ') }),
        bloqueado: true,
      }
    : { mensaje: t('field.deleteConfirm', { name: campo.name }), bloqueado: false };
}
