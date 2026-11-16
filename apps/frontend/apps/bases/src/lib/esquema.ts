/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cambiar el esquema de una tabla sin rehacerla: las opciones de un campo de selección
 * y el orden de los campos. Puro, para probarlo sin navegador.
 */
import type { BasesField } from '@sytue/api-clients/bases';

/**
 * Las opciones escritas una por línea: sin espacios de más, sin líneas vacías y sin
 * repetidas (la primera manda). Lo mismo al crear un campo que al editarlo.
 */
export function listaDeOpciones(texto: string): string[] {
  const out: string[] = [];
  for (const linea of texto.split('\n')) {
    const o = linea.trim();
    if (o && !out.includes(o)) out.push(o);
  }
  return out;
}

/** Las opciones que había y ya no: los registros que las usan las conservan, retiradas. */
export function opcionesQueSeRetiran(antes: readonly string[], ahora: readonly string[]): string[] {
  return antes.filter((o) => !ahora.includes(o));
}

/** Un valor que el registro tiene pero que ya no es una opción del campo. */
export function esOpcionRetirada(campo: BasesField, valor: string): boolean {
  return (
    (campo.kind === 'select' || campo.kind === 'multiselect') &&
    !(campo.choices ?? []).includes(valor)
  );
}

/**
 * El orden de `ids` con `id` puesto donde estaba `destino` (los de en medio se corren
 * un puesto). Si alguno no está, el orden no cambia.
 */
export function moverCampo(ids: readonly string[], id: string, destino: string): string[] {
  const de = ids.indexOf(id);
  const a = ids.indexOf(destino);
  if (de < 0 || a < 0 || de === a) return [...ids];
  const out = ids.filter((x) => x !== id);
  out.splice(a, 0, id);
  return out;
}

/**
 * El campo vecino de `id` hacia `paso` (-1 izquierda, +1 derecha) dentro de los que se
 * ven, o null si ya está en el borde. Es el destino de Alt+flecha.
 */
export function vecino(ids: readonly string[], id: string, paso: -1 | 1): string | null {
  const i = ids.indexOf(id);
  if (i < 0) return null;
  return ids[i + paso] ?? null;
}
