import { describe, expect, it } from 'bun:test';
import {
  conservaSeleccion,
  opcionesContextuales,
  zonaDeSeleccion,
  type EstadoCtx,
  type OpcionCtx,
} from './menuContextual';

const libre: EstadoCtx = {
  soloLectura: false,
  vetado: () => false,
  rangoBloqueado: false,
  c1: 2,
  r1: 4,
  congelada: false,
};

const claves = (g: readonly (readonly OpcionCtx[])[]): string[][] =>
  g.map((grupo) => grupo.map((o) => o.clave));
const opcion = (g: readonly (readonly OpcionCtx[])[], clave: string): OpcionCtx =>
  g.flat().find((o) => o.clave === clave)!;

describe('opcionesContextuales: qué ofrece cada zona', () => {
  it('celdas: portapapeles, insertar y eliminar filas y columnas, borrar, ordenar y congelar', () => {
    expect(claves(opcionesContextuales('celdas', libre))).toEqual([
      ['cmd.cut', 'cmd.copy', 'cmd.paste', 'cmd.pasteValues'],
      [
        'table.rowAbove',
        'table.rowBelow',
        'table.colLeft',
        'table.colRight',
        'table.deleteRows',
        'table.deleteCols',
        'sheet.ctx.clear',
      ],
      ['data.sortAsc', 'data.sortDesc'],
      ['sheet.ctx.freeze'],
    ]);
  });

  it('cabecera de columna: solo columnas, ordenar y el ancho', () => {
    expect(claves(opcionesContextuales('columnas', libre))).toEqual([
      ['cmd.cut', 'cmd.copy', 'cmd.paste', 'cmd.pasteValues'],
      ['table.colLeft', 'table.colRight', 'table.deleteCols', 'sheet.ctx.clear'],
      ['data.sortAsc', 'data.sortDesc'],
      ['sheet.colWidth'],
      ['sheet.ctx.freeze'],
    ]);
  });

  it('cabecera de fila: solo filas y el alto, sin ordenar (como Excel)', () => {
    expect(claves(opcionesContextuales('filas', libre))).toEqual([
      ['cmd.cut', 'cmd.copy', 'cmd.paste', 'cmd.pasteValues'],
      ['table.rowAbove', 'table.rowBelow', 'table.deleteRows', 'sheet.ctx.clear'],
      ['sheet.rowHeight'],
      ['sheet.ctx.freeze'],
    ]);
  });

  it('cada opción despacha el comando de la cinta con su valor', () => {
    const g = opcionesContextuales('celdas', libre);
    expect(opcion(g, 'cmd.cut').accion).toEqual({ cmd: 'copy', value: 'cut' });
    expect(opcion(g, 'cmd.copy').accion).toEqual({ cmd: 'copy' });
    expect(opcion(g, 'cmd.pasteValues').accion).toEqual({ cmd: 'paste', value: 'values' });
    expect(opcion(g, 'table.rowBelow').accion).toEqual({ cmd: 'insrow', value: 'below' });
    expect(opcion(g, 'table.colLeft').accion).toEqual({ cmd: 'inscol', value: 'left' });
    expect(opcion(g, 'table.deleteCols').accion).toEqual({ cmd: 'delcol' });
    expect(opcion(g, 'data.sortDesc').accion).toEqual({ cmd: 'sort', value: 'desc' });
    expect(opcion(g, 'sheet.ctx.clear').accion).toEqual({ borrar: true });
    const col = opcionesContextuales('columnas', libre);
    expect(opcion(col, 'sheet.colWidth').accion).toEqual({ medida: 'colwidth' });
  });
});

describe('opcionesContextuales: lo vetado sale inerte y con su motivo', () => {
  it('la hoja protegida veta lo que veta el lienzo; copiar sigue vivo', () => {
    const vetados = new Set(['paste', 'insrow', 'delrow', 'sort', 'colwidth']);
    const g = opcionesContextuales('columnas', {
      ...libre,
      vetado: (c) => vetados.has(c),
    });
    expect(opcion(g, 'cmd.paste')).toMatchObject({
      vivo: false,
      motivo: 'sheet.msg.protectedSheet',
    });
    expect(opcion(g, 'cmd.pasteValues').vivo).toBe(false);
    expect(opcion(g, 'data.sortAsc').motivo).toBe('sheet.msg.protectedSheet');
    expect(opcion(g, 'sheet.colWidth').vivo).toBe(false);
    expect(opcion(g, 'cmd.copy')).toMatchObject({ vivo: true, motivo: null });
    // insertar columnas no está en el conjunto vetado de esta prueba: vive.
    expect(opcion(g, 'table.colLeft').vivo).toBe(true);
  });

  it('las celdas bloqueadas vetan cortar y borrar contenido', () => {
    const g = opcionesContextuales('celdas', { ...libre, rangoBloqueado: true });
    expect(opcion(g, 'cmd.cut')).toMatchObject({ vivo: false, motivo: 'sheet.msg.protectedSheet' });
    expect(opcion(g, 'sheet.ctx.clear').vivo).toBe(false);
    expect(opcion(g, 'cmd.copy').vivo).toBe(true);
  });

  it('el solo lectura gana a la protección y lo apaga todo menos copiar', () => {
    const g = opcionesContextuales('celdas', {
      ...libre,
      soloLectura: true,
      vetado: () => true,
    });
    for (const o of g.flat()) {
      if (o.clave === 'cmd.copy') expect(o.vivo).toBe(true);
      else expect(o).toMatchObject({ vivo: false, motivo: 'public.readOnly' });
    }
  });
});

describe('opcionesContextuales: congelar', () => {
  const congelar = (zona: 'celdas' | 'columnas' | 'filas', e: Partial<EstadoCtx> = {}) =>
    opcion(opcionesContextuales(zona, { ...libre, ...e }), 'sheet.ctx.freeze');

  it('inmoviliza lo de encima y a la izquierda de la selección, como el botón de la cinta', () => {
    expect(congelar('celdas').accion).toEqual({ cmd: 'freeze', value: '2,4' });
    expect(congelar('columnas').accion).toEqual({ cmd: 'freeze', value: '2,0' });
    expect(congelar('filas').accion).toEqual({ cmd: 'freeze', value: '0,4' });
    // En A1, la primera fila.
    expect(congelar('celdas', { c1: 0, r1: 0 }).accion).toEqual({ cmd: 'freeze', value: '0,1' });
  });

  it('en la columna A desde su cabecera no haría nada: inerte y sin motivo', () => {
    expect(congelar('columnas', { c1: 0 })).toMatchObject({ vivo: false, motivo: null });
  });

  it('con algo inmovilizado ofrece descongelar', () => {
    const o = opcion(
      opcionesContextuales('celdas', { ...libre, congelada: true }),
      'sheet.ctx.unfreeze',
    );
    expect(o).toMatchObject({ vivo: true, accion: { cmd: 'freeze', value: '0,0' } });
  });
});

describe('zonaDeSeleccion y conservaSeleccion', () => {
  const N = { cols: 26, rows: 100 };
  it('el teclado abre el menú de la cabecera si hay columnas o filas enteras', () => {
    expect(zonaDeSeleccion({ c1: 1, r1: 3, c2: 2, r2: 5 }, N.cols, N.rows)).toBe('celdas');
    expect(zonaDeSeleccion({ c1: 1, r1: 0, c2: 2, r2: 99 }, N.cols, N.rows)).toBe('columnas');
    expect(zonaDeSeleccion({ c1: 0, r1: 3, c2: 25, r2: 3 }, N.cols, N.rows)).toBe('filas');
    expect(zonaDeSeleccion({ c1: 0, r1: 0, c2: 25, r2: 99 }, N.cols, N.rows)).toBe('celdas');
  });

  it('el clic derecho dentro del rango lo conserva y fuera selecciona lo pulsado', () => {
    const rango = { c1: 1, r1: 1, c2: 3, r2: 3 };
    expect(conservaSeleccion('celdas', { c: 2, r: 2 }, rango, N.cols, N.rows)).toBe(true);
    expect(conservaSeleccion('celdas', { c: 5, r: 2 }, rango, N.cols, N.rows)).toBe(false);
    // Una cabecera dentro de un rango que no son columnas enteras selecciona la suya.
    expect(conservaSeleccion('columnas', { c: 2, r: 0 }, rango, N.cols, N.rows)).toBe(false);
    const cols = { c1: 1, r1: 0, c2: 3, r2: 99 };
    expect(conservaSeleccion('columnas', { c: 2, r: 0 }, cols, N.cols, N.rows)).toBe(true);
    expect(conservaSeleccion('columnas', { c: 4, r: 0 }, cols, N.cols, N.rows)).toBe(false);
    const filas = { c1: 0, r1: 4, c2: 25, r2: 6 };
    expect(conservaSeleccion('filas', { c: 0, r: 5 }, filas, N.cols, N.rows)).toBe(true);
  });
});
