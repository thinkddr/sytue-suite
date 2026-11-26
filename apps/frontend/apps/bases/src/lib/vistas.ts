/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que la interfaz hace con las vistas guardadas: agrupar para el tablero, armar la
 * configuración de una vista nueva y volver a abrir una guardada para editarla. El servidor valida y ejecuta los filtros y el orden
 * (no se evalúa nada aquí); esto solo decide cómo se PINTA.
 */
import type {
  BasesAttachment,
  BasesField,
  BasesFilter,
  BasesRecord,
  BasesValue,
  BasesView,
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

/**
 * La portada de una tarjeta de galería: el primer adjunto de imagen del registro,
 * recorriendo los campos de adjuntos en el orden de la tabla y sus ficheros en el orden
 * en que se subieron. Solo vale una imagen con miniatura: el servicio la genera de lo
 * que el CONTENIDO dice que es imagen, y la tarjeta nunca baja el original entero. Sin
 * ninguna, `null` (la tarjeta pinta su hueco).
 */
export function portadaDe(campos: readonly BasesField[], r: BasesRecord): BasesAttachment | null {
  for (const c of campos) {
    if (c.kind !== 'attachment') continue;
    const imagen = (r.attachments?.[c.id] ?? []).find(
      (a) => a.has_thumb && a.mime.startsWith('image/'),
    );
    if (imagen) return imagen;
  }
  return null;
}

/**
 * Los datos con los que nace una tarjeta añadida al pie de una columna: el valor de esa
 * columna en el campo de agrupación, para que no haya que crearla en la rejilla y moverla.
 * La de «sin valor» nace vacía.
 */
export function datosDeColumna(campo: BasesField, valor: string): Record<string, BasesValue> {
  return valor === SIN_VALOR ? {} : { [campo.id]: valor };
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
  /** Formulario: los incluidos que hay que rellenar para enviarlo. */
  obligatorios?: string[];
  filtroCampo: string;
  filtroOp: string;
  filtroValor: string;
  ordenCampo: string;
  ordenDir: 'asc' | 'desc';
  /**
   * Al editar: los filtros y órdenes que la vista ya tenía después del primero. El diálogo
   * solo enseña uno de cada (la API admite más), y editar una vista no debe perder lo que
   * no se ve; se reenvían tal cual detrás del primero.
   */
  restoFiltros?: BasesFilter[];
  restoOrden?: { field: string; dir: 'asc' | 'desc' }[];
}

/** El borrador de una vista guardada, para abrirla en el diálogo y editarla. */
export function borradorDeVista(v: BasesView, campos: readonly BasesField[]): BorradorVista {
  const [filtro, ...restoFiltros] = v.config.filters ?? [];
  const [orden, ...restoOrden] = v.config.sort ?? [];
  const valor = filtro?.value;
  return {
    nombre: v.name,
    tipo: v.kind,
    agrupar: v.config.group_by ?? campos.find((c) => efectivo(c).kind === 'select')?.id ?? '',
    // Un formulario sin lista pide todos los que admite: se abre con todos marcados, que
    // es lo mismo que guardarlo así.
    campos:
      v.kind === 'form' && (v.config.fields ?? []).length === 0
        ? camposDeFormulario(campos).map((c) => c.id)
        : [...(v.config.fields ?? [])],
    obligatorios: [...(v.config.required ?? [])],
    filtroCampo: filtro?.field ?? '',
    filtroOp: filtro?.op ?? 'eq',
    filtroValor:
      typeof valor === 'string' || typeof valor === 'number' || typeof valor === 'boolean'
        ? String(valor)
        : '',
    ordenCampo: orden?.field ?? '',
    ordenDir: orden?.dir ?? 'asc',
    restoFiltros,
    restoOrden,
  };
}

/** Operadores por tipo de campo, espejo de `domain.operadores` del servicio. */
export type Operador =
  | 'eq'
  | 'neq'
  | 'contains'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'empty'
  | 'notempty'
  | 'today'
  | 'this_month'
  | 'past_days'
  | 'next_days';

/**
 * Los operadores que no llevan valor: vacío o no, y los relativos a un día que pone la
 * lectura («hoy», «este mes»).
 */
export const OPS_SIN_VALOR: readonly string[] = ['empty', 'notempty', 'today', 'this_month'];

/** Los relativos que llevan un número de días (hoy incluido), de 1 a `MAX_DIAS`. */
export const OPS_CON_DIAS: readonly string[] = ['past_days', 'next_days'];

/** El tope de días de un filtro relativo, `domain.MaxDiasRelativos` del servicio. */
export const MAX_DIAS = 3650;

export const OPERADORES: Record<
  Exclude<
    BasesField['kind'],
    'formula' | 'attachment' | 'url' | 'email' | 'phone' | 'currency' | 'rating'
  >,
  readonly Operador[]
> = {
  text: ['contains', 'eq', 'neq', 'empty', 'notempty'],
  number: ['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'empty', 'notempty'],
  date: [
    'eq',
    'neq',
    'gt',
    'gte',
    'lt',
    'lte',
    'empty',
    'notempty',
    'today',
    'past_days',
    'next_days',
    'this_month',
  ],
  checkbox: ['eq'],
  select: ['eq', 'neq', 'empty', 'notempty'],
  multiselect: ['contains', 'empty', 'notempty'],
  link: ['empty', 'notempty'],
};

/** Los operadores de un campo; una fórmula o un campo con formato, los de lo que guarda. */
export function operadoresDe(campo: BasesField): readonly Operador[] {
  const k = efectivo(campo).kind;
  return k in OPERADORES ? OPERADORES[k as keyof typeof OPERADORES] : [];
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
    // Obligatorio solo lo que el formulario enseña, en su orden: el servicio rechaza
    // la vista entera por un obligatorio que no sale.
    const obligatorios = b.campos.filter((id) => (b.obligatorios ?? []).includes(id));
    if (obligatorios.length > 0) cfg.required = obligatorios;
    return cfg;
  }
  const elegido = campos.find((c) => c.id === b.filtroCampo);
  const f = elegido && efectivo(elegido);
  if (f) {
    if (!(operadoresDe(f) as readonly string[]).includes(b.filtroOp)) return null;
    const sinValor = OPS_SIN_VALOR.includes(b.filtroOp);
    let valor: string | number | boolean | undefined;
    if (OPS_CON_DIAS.includes(b.filtroOp)) {
      const n = Number(b.filtroValor.trim());
      if (b.filtroValor.trim() === '' || !Number.isInteger(n) || n < 1 || n > MAX_DIAS) return null;
      valor = n;
    } else if (!sinValor) {
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
  // Lo que se conserva de la vista editada, salvo lo de campos que ya no existen: el
  // servidor rechazaría la vista entera por un filtro sobre un campo borrado.
  const existe = (id: string): boolean => campos.some((c) => c.id === id);
  const restoFiltros = (b.restoFiltros ?? []).filter((x) => existe(x.field));
  if (restoFiltros.length > 0) cfg.filters = [...(cfg.filters ?? []), ...restoFiltros];
  if (b.ordenCampo) cfg.sort = [{ field: b.ordenCampo, dir: b.ordenDir }];
  const restoOrden = (b.restoOrden ?? []).filter((x) => existe(x.field));
  if (restoOrden.length > 0) cfg.sort = [...(cfg.sort ?? []), ...restoOrden];
  return cfg;
}
