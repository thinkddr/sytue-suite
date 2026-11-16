/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El menú contextual de la rejilla de Calcus: clic derecho, Mayús+F10 o la tecla Menú
 * sobre la celda activa, y mantener pulsado con el dedo. Insertar o borrar una fila,
 * ordenar o pegar solo valores exigía ir a la cinta, y ninguna hoja de cálculo se usa así.
 *
 * Aquí está QUÉ ofrece cada variante (celdas, cabecera de columna, cabecera de fila) y qué
 * está vetado, sin pintar nada; el menú lo pinta SheetCanvas. Todas las opciones son
 * comandos que ya existen (`dispatchSheetCommand`), con el mismo nombre y valor que la
 * cinta, y la lista es la de Fyne (`apps/fyne/calcus/menu_celda.go`, `opcionesContextuales`)
 * más «Ancho de columna…» y «Alto de fila…» en sus cabeceras. Lo vetado sale inerte y con
 * su motivo, no escondido: si el menú cambiara de forma según la hoja, quien busca una
 * opción no sabría por qué no está.
 */
import type { DocsKey } from '../../i18n';
import type { SheetCommand } from './SheetCanvas';

/** Las tres variantes del menú, como en Excel y en Sheets. */
export type ZonaMenu = 'celdas' | 'columnas' | 'filas';

/** Lo que hace una opción: un comando de la hoja, el Supr de la rejilla o pedir una medida. */
export type AccionCtx =
  { cmd: SheetCommand; value?: string } | { borrar: true } | { medida: 'colwidth' | 'rowheight' };

export interface OpcionCtx {
  clave: DocsKey;
  accion: AccionCtx;
  vivo: boolean;
  /** Por qué no vive (clave del catálogo); null si vive o si no haría nada (congelar en A1). */
  motivo: DocsKey | null;
  /** Atajo que la hace sin menú, para `aria-keyshortcuts`. */
  atajo?: string;
}

/** Un grupo del menú; entre grupos va un separador. */
export type GrupoCtx = readonly OpcionCtx[];

export interface EstadoCtx {
  soloLectura: boolean;
  /** ¿La protección de la hoja veta este comando? (`comandoVetadoPorProteccion`). */
  vetado: (cmd: SheetCommand) => boolean;
  /** Hay celdas bloqueadas de una hoja protegida en la selección. */
  rangoBloqueado: boolean;
  /** Esquina superior izquierda de la selección. */
  c1: number;
  r1: number;
  /** Ya hay filas o columnas inmovilizadas. */
  congelada: boolean;
}

/** El nombre accesible de cada variante. */
export const ROTULO_MENU: Readonly<Record<ZonaMenu, DocsKey>> = {
  celdas: 'sheet.ctx.cells',
  columnas: 'sheet.ctx.cols',
  filas: 'sheet.ctx.rows',
};

/**
 * La variante del menú que abre el TECLADO (Mayús+F10, tecla Menú): la de la selección.
 * Columnas enteras, la de su cabecera; filas enteras, la de la suya; la hoja entera o un
 * rango, la de celdas. Es lo que hace Fyne en `abrirMenuTeclado`.
 */
export function zonaDeSeleccion(
  rect: { c1: number; r1: number; c2: number; r2: number },
  nCols: number,
  nRows: number,
): ZonaMenu {
  const colsEnteras = rect.r1 === 0 && rect.r2 >= nRows - 1;
  const filasEnteras = rect.c1 === 0 && rect.c2 >= nCols - 1;
  if (colsEnteras && filasEnteras) return 'celdas';
  if (colsEnteras) return 'columnas';
  if (filasEnteras) return 'filas';
  return 'celdas';
}

/**
 * ¿Conserva el clic derecho la selección? Como en Excel: pulsar DENTRO del rango lo
 * conserva (para borrar o copiar un rango entero desde su menú); pulsar fuera selecciona
 * lo pulsado. En una cabecera, además, el rango tiene que ser ya columnas (o filas)
 * enteras: si no, la cabecera selecciona la suya.
 */
export function conservaSeleccion(
  zona: ZonaMenu,
  pos: { c: number; r: number },
  rect: { c1: number; r1: number; c2: number; r2: number },
  nCols: number,
  nRows: number,
): boolean {
  if (zona === 'columnas') {
    return pos.c >= rect.c1 && pos.c <= rect.c2 && rect.r1 === 0 && rect.r2 >= nRows - 1;
  }
  if (zona === 'filas') {
    return pos.r >= rect.r1 && pos.r <= rect.r2 && rect.c1 === 0 && rect.c2 >= nCols - 1;
  }
  return pos.c >= rect.c1 && pos.c <= rect.c2 && pos.r >= rect.r1 && pos.r <= rect.r2;
}

/** Las opciones de una variante para la selección y la protección de ahora, por grupos. */
export function opcionesContextuales(zona: ZonaMenu, e: EstadoCtx): GrupoCtx[] {
  // El solo lectura antes que la protección: es el permiso más fuerte y el que no se
  // arregla quitando una contraseña.
  const motivoDe = (vetado: boolean): Pick<OpcionCtx, 'vivo' | 'motivo'> =>
    e.soloLectura
      ? { vivo: false, motivo: 'public.readOnly' }
      : vetado
        ? { vivo: false, motivo: 'sheet.msg.protectedSheet' }
        : { vivo: true, motivo: null };
  const cmd = (clave: DocsKey, c: SheetCommand, value?: string, atajo?: string): OpcionCtx => ({
    clave,
    accion: value === undefined ? { cmd: c } : { cmd: c, value },
    ...motivoDe(e.vetado(c)),
    ...(atajo ? { atajo } : {}),
  });

  const portapapeles: OpcionCtx[] = [
    // Cortar acaba vaciando el origen: con celdas bloqueadas no se puede.
    {
      clave: 'cmd.cut',
      accion: { cmd: 'copy', value: 'cut' },
      ...motivoDe(e.rangoBloqueado),
      atajo: 'Control+X',
    },
    // Copiar se puede siempre, también en solo lectura y en una hoja protegida.
    { clave: 'cmd.copy', accion: { cmd: 'copy' }, vivo: true, motivo: null, atajo: 'Control+C' },
    cmd('cmd.paste', 'paste', undefined, 'Control+V'),
    cmd('cmd.pasteValues', 'paste', 'values', 'Control+Shift+V'),
  ];
  const filas = [
    cmd('table.rowAbove', 'insrow', 'above'),
    cmd('table.rowBelow', 'insrow', 'below'),
  ];
  const cols = [cmd('table.colLeft', 'inscol', 'left'), cmd('table.colRight', 'inscol', 'right')];
  const borrarFilas = cmd('table.deleteRows', 'delrow');
  const borrarCols = cmd('table.deleteCols', 'delcol');
  const borrar: OpcionCtx = {
    clave: 'sheet.ctx.clear',
    accion: { borrar: true },
    ...motivoDe(e.rangoBloqueado),
    atajo: 'Delete',
  };
  const ordenar = [cmd('data.sortAsc', 'sort', 'asc'), cmd('data.sortDesc', 'sort', 'desc')];
  const medida = (m: 'colwidth' | 'rowheight'): OpcionCtx => ({
    clave: m === 'colwidth' ? 'sheet.colWidth' : 'sheet.rowHeight',
    accion: { medida: m },
    ...motivoDe(e.vetado(m)),
  });

  const grupos: GrupoCtx[] = [portapapeles];
  if (zona === 'columnas') {
    grupos.push([...cols, borrarCols, borrar], ordenar, [medida('colwidth')]);
  } else if (zona === 'filas') {
    // Sin ordenar: ordenar por una fila no es lo que nadie espera de ese menú, y Excel
    // tampoco lo ofrece en la cabecera de fila.
    grupos.push([...filas, borrarFilas, borrar], [medida('rowheight')]);
  } else {
    grupos.push([...filas, ...cols, borrarFilas, borrarCols, borrar], ordenar);
  }
  grupos.push([opcionCongelar(zona, e)]);
  return grupos;
}

/**
 * «Congelar hasta aquí», o «Descongelar paneles» si ya hay algo inmovilizado. Inmoviliza
 * lo que queda por encima y a la izquierda de la esquina de la selección, como el botón
 * de la cinta (`toggleFreeze`) y Excel: en una cabecera de columna, las columnas de su
 * izquierda; en una de fila, las de encima; en A1, la primera fila, como el botón.
 */
function opcionCongelar(zona: ZonaMenu, e: EstadoCtx): OpcionCtx {
  let value = '0,0';
  if (!e.congelada) {
    const c = zona === 'filas' ? 0 : e.c1;
    const r = zona === 'columnas' ? 0 : e.r1;
    value = zona === 'celdas' && c === 0 && r === 0 ? '0,1' : `${c},${r}`;
  }
  const clave: DocsKey = e.congelada ? 'sheet.ctx.unfreeze' : 'sheet.ctx.freeze';
  if (e.soloLectura) {
    return { clave, accion: { cmd: 'freeze', value }, vivo: false, motivo: 'public.readOnly' };
  }
  // Congelar la columna A desde su propia cabecera no inmoviliza nada: inerte, sin motivo.
  return {
    clave,
    accion: { cmd: 'freeze', value },
    vivo: e.congelada || value !== '0,0',
    motivo: null,
  };
}
