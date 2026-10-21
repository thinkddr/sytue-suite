/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Valores de celda: de lo que viaja (JSON) a lo que se escribe en la rejilla y de vuelta.
 * Es COMODIDAD de la interfaz: el servidor (`services/bases/internal/domain/valores.go`)
 * vuelve a validar todo y es el que manda.
 */
import type { BasesField, BasesRecord, BasesRole, BasesValue } from '@sytue/api-clients/bases';

/** Si el rol permite escribir registros (editor o propietario). Solo pinta la interfaz. */
export function puedeEscribir(rol: BasesRole | '' | undefined): boolean {
  return rol === 'owner' || rol === 'editor';
}

/** Si el rol permite comentar (comentarista o superior). Solo pinta la interfaz. */
export function puedeComentar(rol: BasesRole | '' | undefined): boolean {
  return rol === 'commenter' || puedeEscribir(rol);
}

/** Si el rol permite cambiar el esquema (tablas y campos): solo el propietario. */
export function puedeEsquema(rol: BasesRole | '' | undefined): boolean {
  return rol === 'owner';
}

/** El valor de una celda como texto para mostrar o para editar. */
export function textoDeValor(_campo: BasesField, valor: BasesValue | undefined): string {
  if (valor === undefined || valor === null) return '';
  if (Array.isArray(valor)) return valor.join(', ');
  if (typeof valor === 'boolean') return valor ? '✓' : '';
  return String(valor);
}

/**
 * Lo escrito en un campo de texto → el valor a enviar. `null` = vaciar. `undefined` = no
 * es válido para ese tipo (la interfaz avisa y no envía).
 */
export function valorDeTexto(campo: BasesField, texto: string): BasesValue | undefined {
  const t = texto.trim();
  if (t === '') return null;
  switch (campo.kind) {
    case 'text':
      return texto;
    case 'number': {
      // Admite la coma decimal, que es como se escribe en gran parte de la UE.
      const n = Number(t.replace(',', '.'));
      return Number.isFinite(n) && Math.abs(n) <= 1e15 ? n : undefined;
    }
    case 'date':
      return /^\d{4}-\d{2}-\d{2}$/.test(t) && !Number.isNaN(Date.parse(t)) ? t : undefined;
    case 'select':
      return campo.choices?.includes(t) ? t : undefined;
    default:
      return undefined;
  }
}

/** Cómo se llama un registro en una lista de enlaces: su primer campo con valor. */
export function etiquetaDeRegistro(campos: readonly BasesField[], r: BasesRecord): string {
  for (const c of campos) {
    if (c.kind === 'link' || c.kind === 'checkbox' || c.kind === 'attachment') continue;
    const t = textoDeValor(c, r.data[c.id]);
    if (t) return t;
  }
  return r.id.slice(0, 8);
}

/** Los ids de los enlaces de un registro en un campo, o []. */
export function enlacesDe(r: BasesRecord, campo: BasesField): string[] {
  return r.links[campo.id] ?? [];
}
