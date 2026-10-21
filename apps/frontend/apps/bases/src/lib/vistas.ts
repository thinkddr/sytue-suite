/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que la interfaz hace con las vistas guardadas: agrupar para el tablero y armar la
 * configuración de una vista nueva. El servidor valida y ejecuta los filtros y el orden
 * (no se evalúa nada aquí); esto solo decide cómo se PINTA.
 */
import type {
  BasesField,
  BasesRecord,
  BasesViewConfig,
  BasesViewKind,
} from '@sytue/api-clients/bases';
import { efectivo } from './formulas.js';

/** La columna de las tarjetas sin valor en el campo de agrupación. */
export const SIN_VALOR = '';

export interface Columna {
  /** El valor de la opción, o SIN_VALOR. */
  valor: string;
  registros: BasesRecord[];
}

/**
 * Reparte los registros en columnas, una por opción del campo de selección y, al final,
 * la de «sin valor». Un valor que ya no es una opción (se borró la opción) cae en «sin
 * valor» y no desaparece: un registro nunca se pierde de la vista.
 */
export function columnasDeTablero(campo: BasesField, registros: readonly BasesRecord[]): Columna[] {
  const opciones = campo.choices ?? [];
  const cols: Columna[] = opciones.map((valor) => ({ valor, registros: [] }));
  const sin: Columna = { valor: SIN_VALOR, registros: [] };
  for (const r of registros) {
    const v = r.data[campo.id];
    const destino = typeof v === 'string' ? cols.find((c) => c.valor === v) : undefined;
    (destino ?? sin).registros.push(r);
  }
  return [...cols, sin];
}

/** Los campos que admite un formulario público: todos menos enlaces, fórmulas y adjuntos. */
export function camposDeFormulario(campos: readonly BasesField[]): BasesField[] {
  return campos.filter((c) => c.kind !== 'link' && c.kind !== 'formula' && c.kind !== 'attachment');
}

export interface BorradorVista {
  nombre: string;
  tipo: BasesViewKind;
  /** Kanban: id del campo de selección. */
  agrupar: string;
  /** Formulario: ids de los campos incluidos, en orden. */
  campos: string[];
  filtroCampo: string;
  filtroOp: string;
  filtroValor: string;
  ordenCampo: string;
  ordenDir: 'asc' | 'desc';
}

/** Operadores por tipo de campo, espejo de `domain.operadores` del servicio. */
export type Operador =
  'eq' | 'neq' | 'contains' | 'gt' | 'gte' | 'lt' | 'lte' | 'empty' | 'notempty';

export const OPERADORES: Record<
  Exclude<BasesField['kind'], 'formula' | 'attachment'>,
  readonly Operador[]
> = {
  text: ['contains', 'eq', 'neq', 'empty', 'notempty'],
  number: ['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'empty', 'notempty'],
  date: ['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'empty', 'notempty'],
  checkbox: ['eq'],
  select: ['eq', 'neq', 'empty', 'notempty'],
  multiselect: ['contains', 'empty', 'notempty'],
  link: ['empty', 'notempty'],
};

/** Los operadores de un campo; una fórmula se filtra como el tipo de su resultado. */
export function operadoresDe(campo: BasesField): readonly Operador[] {
  const k = efectivo(campo).kind;
  return k === 'formula' || k === 'attachment' ? [] : OPERADORES[k];
}

/** La configuración que se envía al servicio, o `null` si el borrador no es válido todavía. */
export function configDeBorrador(
  b: BorradorVista,
  campos: readonly BasesField[],
): BasesViewConfig | null {
  const cfg: BasesViewConfig = {};
  if (b.tipo === 'kanban') {
    if (!b.agrupar) return null;
    cfg.group_by = b.agrupar;
  }
  if (b.tipo === 'form') {
    if (b.campos.length === 0) return null;
    cfg.fields = b.campos;
    return cfg;
  }
  const elegido = campos.find((c) => c.id === b.filtroCampo);
  const f = elegido && efectivo(elegido);
  if (f) {
    if (!(operadoresDe(f) as readonly string[]).includes(b.filtroOp)) return null;
    const sinValor = b.filtroOp === 'empty' || b.filtroOp === 'notempty';
    let valor: string | number | boolean | undefined;
    if (!sinValor) {
      if (f.kind === 'number') {
        const n = Number(b.filtroValor.replace(',', '.'));
        if (b.filtroValor.trim() === '' || !Number.isFinite(n)) return null;
        valor = n;
      } else if (f.kind === 'checkbox') {
        valor = b.filtroValor === 'true';
      } else {
        if (b.filtroValor.trim() === '') return null;
        valor = b.filtroValor;
      }
    }
    cfg.filters = [
      { field: f.id, op: b.filtroOp, ...(valor === undefined ? {} : { value: valor }) },
    ];
  }
  if (b.ordenCampo) cfg.sort = [{ field: b.ordenCampo, dir: b.ordenDir }];
  return cfg;
}
