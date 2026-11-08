/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, describe, expect, it } from 'bun:test';
import {
  esXML,
  importarLeidos,
  leerFicheros,
  merecePreguntar,
  ofrecerDeshacer,
  SimulacionFallida,
  simularLeidos,
  revertirImportaciones,
  rutaImportar,
  sumarPrevia,
  type ResultadoLote,
} from './importacion.js';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

const lote = (r: Partial<ResultadoLote>): ResultadoLote => ({
  filas: 0,
  creados: 0,
  actualizados: 0,
  ignoradas: [],
  fallos: [],
  ...r,
});

describe('rutaImportar', () => {
  it('la vista previa lleva simular=1 y el nombre del fichero', () => {
    expect(rutaImportar('cliente', 'clientes.csv', true)).toBe(
      'cliente/importar?fichero=clientes.csv&simular=1',
    );
  });
  it('la de verdad, solo el nombre: es con lo que se reconoce el lote', () => {
    expect(rutaImportar('cliente', 'a b.csv', false)).toBe('cliente/importar?fichero=a+b.csv');
  });
});

describe('sumarPrevia', () => {
  const fila = (n: number, e: string) => `Fila ${n}: ${e}`;
  it('suma lo que pasaría y cuenta las filas que no entrarían', () => {
    const p = sumarPrevia(
      [
        {
          nombre: 'a.csv',
          res: lote({ creados: 3, actualizados: 1, fallos: [{ fila: 4, error: 'x' }] }),
        },
      ],
      fila,
    );
    expect(p).toEqual({ creados: 3, actualizados: 1, fallos: ['Fila 4: x'] });
    expect(merecePreguntar(p)).toBe(true);
  });
  it('con varios ficheros, cada fallo lleva el suyo delante', () => {
    const p = sumarPrevia(
      [
        { nombre: 'a.csv', res: lote({ creados: 1 }) },
        { nombre: 'b.csv', res: lote({ fallos: [{ fila: 2, error: 'y' }] }) },
      ],
      fila,
    );
    expect(p.fallos).toEqual(['b.csv · Fila 2: y']);
    expect(p.creados).toBe(1);
  });
  it('si no entraría nada, no se pregunta', () => {
    expect(
      merecePreguntar(sumarPrevia([{ nombre: 'a.csv', res: lote({ fallos: null }) }], fila)),
    ).toBe(false);
  });
});

describe('esXML', () => {
  it('por extensión o por tipo', () => {
    expect(esXML('f.XML')).toBe(true);
    expect(esXML('f', 'application/xml')).toBe(true);
    expect(esXML('f.csv', 'text/csv')).toBe(false);
  });
});

describe('revertirImportaciones', () => {
  it('revierte cada lote por su ruta y suma lo que pasó', async () => {
    const pedidas: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      pedidas.push(`${init?.method ?? 'GET'} ${String(input)}`);
      return new Response(JSON.stringify({ borrados: 2, restaurados: 1, intactos: 1 }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      });
    }) as typeof fetch;
    const r = await revertirImportaciones(['l1', 'l2']);
    expect(pedidas).toEqual([
      'POST /api/importaciones/l1/revertir',
      'POST /api/importaciones/l2/revertir',
    ]);
    expect(r).toEqual({ borrados: 4, restaurados: 2, intactos: 2 });
  });

  it('si uno falla, deshace los demás y luego lo dice', async () => {
    const pedidas: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      pedidas.push(String(input));
      if (String(input).includes('l1'))
        return new Response(JSON.stringify({ error: { message: 'sin permiso' } }), { status: 403 });
      return new Response(JSON.stringify({ borrados: 1, restaurados: 0, intactos: 0 }), {
        status: 200,
      });
    }) as typeof fetch;
    let error: unknown = null;
    await revertirImportaciones(['l1', 'l2']).catch((e: unknown) => {
      error = e;
    });
    expect(pedidas.length).toBe(2);
    expect(String(error)).toContain('sin permiso');
  });
});

const fila = (n: number, e: string): string => `Fila ${n}: ${e}`;

/** Un servicio que contesta a la simulación con `estado` y apunta lo que se pide. */
function servicio(estadoSimulacion: number): string[] {
  const pedidas: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    pedidas.push(url);
    if (url.includes('simular=1')) {
      return estadoSimulacion === 200
        ? new Response(JSON.stringify(lote({ creados: 1 })), { status: 200 })
        : new Response(JSON.stringify({ error: { code: 'x', message: 'motivo' } }), {
            status: estadoSimulacion,
          });
    }
    return new Response(JSON.stringify(lote({ id: 'l1', creados: 1 })), { status: 200 });
  }) as typeof fetch;
  return pedidas;
}

const fichero = (name: string, cuerpo: string | Error) => ({
  name,
  type: '',
  text: () => (cuerpo instanceof Error ? Promise.reject(cuerpo) : Promise.resolve(cuerpo)),
});

describe('leer, simular e importar', () => {
  // VISTO EN ROJO: con el `Promise.all(f.text())` de antes, el fichero ilegible
  // rechaza la lectura entera y no se importa ninguno, sin aviso.
  it('un fichero ilegible es un fallo más: los demás entran y él se cuenta', async () => {
    const pedidas = servicio(200);
    const leidos = await leerFicheros([
      fichero('a.csv', 'nombre\nA'),
      fichero('b.csv', new Error('sin permiso de lectura')),
    ]);
    const previa = await simularLeidos('cliente', leidos, fila);
    expect(previa.creados).toBe(1);
    const r = await importarLeidos('cliente', leidos, fila, (e) => (e as Error).message);
    expect(r.creados).toBe(1);
    expect(r.lotes).toEqual(['l1']);
    expect(r.fallos).toEqual(['b.csv: sin permiso de lectura']);
    expect(pedidas).toEqual([
      '/api/cliente/importar?fichero=a.csv&simular=1',
      '/api/cliente/importar?fichero=a.csv',
    ]);
  });

  // VISTO EN ROJO: con el `.catch(() => null)` de antes, un 503 se descarta y se
  // importa sin vista previa.
  it('si la simulación cae por red o 5xx, no se sigue', async () => {
    servicio(503);
    const leidos = await leerFicheros([fichero('a.csv', 'nombre\nA')]);
    await expect(simularLeidos('cliente', leidos, fila)).rejects.toBeInstanceOf(SimulacionFallida);
  });

  // VISTO EN ROJO: con «cualquier 4xx es del fichero» (lo de antes), un 429 de límite o
  // un 401 de sesión caducada dejaba la previa en 0 y se importaba sin preguntar.
  it('un 401, 408 o 429 no es del fichero: no se sigue', async () => {
    for (const estado of [401, 408, 429]) {
      servicio(estado);
      const leidos = await leerFicheros([fichero('a.csv', 'nombre\nA')]);
      await expect(simularLeidos('cliente', leidos, fila)).rejects.toBeInstanceOf(
        SimulacionFallida,
      );
    }
  });

  it('un 4xx es del fichero (un Norma 43 con extensión .csv): sin vista previa, pero se sigue', async () => {
    servicio(422);
    const leidos = await leerFicheros([fichero('extracto.csv', '11...')]);
    const previa = await simularLeidos('linea_extracto', leidos, fila);
    expect(merecePreguntar(previa)).toBe(false);
  });
});

// VISTO EN ROJO: mirando `administrar` en el módulo del modelo (lo que pide importar),
// quien administra Ventas y no Contabilidad ve «Deshacer» y siempre recibe «sin permiso».
describe('ofrecerDeshacer', () => {
  it('pide administrar en contabilidad, no en el módulo del modelo', () => {
    const soloVentas = (m: string, v: string) => m === 'facturacion' && v === 'administrar';
    const conContabilidad = (m: string, v: string) => m === 'contabilidad' && v === 'administrar';
    expect(ofrecerDeshacer(['l1'], soloVentas)).toBe(false);
    expect(ofrecerDeshacer(['l1'], conContabilidad)).toBe(true);
    expect(ofrecerDeshacer([], conContabilidad)).toBe(false);
  });
});
