/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los cambios de estructura de una hoja (insertar o borrar filas o columnas, o deshacerlo)
 * van cada uno en SU guardado: en una hoja protegida el servidor solo reconoce un
 * desplazamiento por guardado (apps/nucleo/office/model/proteccion_ajenas.go), y dos
 * inserciones que caían en el mismo autoguardado se rechazaban. Aquí se monta
 * `useDocument` de verdad, con el servicio simulado por `fetch`, y se mira qué sale.
 *
 * VISTO EN ROJO sin la cola de `useDocument`: las dos inserciones esperaban la pausa del
 * autoguardado y salían juntas en un único guardado con dos desplazamientos.
 *
 * Y al salir de la página (`flush`): VISTO EN ROJO sin la cadena, el beacon llevaba lo
 * último de una vez —dos desplazamientos en un guardado, que el servidor rechaza en una
 * hoja protegida— y el borrador local volvía como un único guardado.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, mock, test } from 'bun:test';
import { render } from 'preact';
import { useLayoutEffect } from 'preact/hooks';
import * as offlineReal from '@sytue/offline';
import { ToastProvider } from '@sytue/ui';
import { EditorI18nCtx } from '../context';
import { docsT } from '../../i18n';
import { conEstructura, estructuraDistinta } from '../sheets/anclas';
import type { SheetModel } from '../types';
import type { OfflineDocument } from './offline';
import type { UseDocumentResult } from './useDocument';

if (typeof document === 'undefined') GlobalRegistrator.register();

/**
 * El almacén local de recuperación (IndexedDB, que happy-dom no tiene) en memoria, solo
 * para quien abre como DUENO: para los demás sigue el de verdad, que aquí falla y se
 * ignora como siempre.
 */
const DUENO = 'u-salida';
const memoria = new Map<string, unknown>();
const reales = { ...offlineReal };
mock.module('@sytue/offline', () => ({
  ...reales,
  readOfflineValue: <T,>(o: string, ns: string, id: string): Promise<T | null> =>
    o === DUENO
      ? Promise.resolve(structuredClone((memoria.get(`${ns}/${id}`) as T | undefined) ?? null))
      : reales.readOfflineValue<T>(o, ns, id),
  mutateOfflineValue: <T,>(
    o: string,
    ns: string,
    id: string,
    update: (previo: T | null) => T | null,
  ): Promise<void> => {
    if (o !== DUENO) return reales.mutateOfflineValue<T>(o, ns, id, update);
    const k = `${ns}/${id}`;
    const siguiente = update(structuredClone((memoria.get(k) as T | undefined) ?? null));
    if (siguiente === null) memoria.delete(k);
    else memoria.set(k, structuredClone(siguiente));
    return Promise.resolve();
  },
}));
const { useDocument } = await import('./useDocument');

const borrador = (): OfflineDocument | undefined =>
  memoria.get('docs.documents/h1') as OfflineDocument | undefined;

const fetchOriginal = globalThis.fetch;
const beaconOriginal = Object.getOwnPropertyDescriptor(navigator, 'sendBeacon');
afterEach(() => {
  render(null, document.body);
  globalThis.fetch = fetchOriginal;
  if (beaconOriginal) Object.defineProperty(navigator, 'sendBeacon', beaconOriginal);
  else Reflect.deleteProperty(navigator, 'sendBeacon');
  memoria.clear();
});

/** Espera a que se cumpla `cond` (el hook trabaja con promesas y efectos). */
async function hasta(cond: () => boolean, que: string): Promise<void> {
  for (let i = 0; i < 200; i++) {
    if (cond()) return;
    await new Promise((r) => setTimeout(r, 5));
  }
  throw new Error(`no ha pasado: ${que}`);
}

const pausa = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms));

/** La hoja de partida: sin `id` en las hojas, así cada guardado lleva el registro entero. */
const M0: SheetModel = {
  active: 0,
  sheets: [{ name: 'Datos', cells: { A1: 'Zona', A2: 'Norte', A3: 'Sur', B1: '=A2' } }],
};

/** Inserta una fila en `desde` (base cero) de la hoja activa, como `applyStructural`. */
function insertarFila(m: SheetModel, desde: number): SheetModel {
  const s = m.sheets[0]!;
  const cells: Record<string, string> = {};
  for (const [ref, v] of Object.entries(s.cells)) {
    const fila = Number(ref.slice(1)) - 1;
    cells[`${ref[0]}${(fila < desde ? fila : fila + 1) + 1}`] = v;
  }
  return { ...m, sheets: [{ ...s, cells, ...conEstructura(s, 'fila', desde, 1) }] };
}

interface Guardado {
  modelo: SheetModel;
  responder: (r: Response) => void;
}

/** El servicio: la ficha, el estado y los guardados, que contesta la prueba a mano. */
function servicio(): { guardados: Guardado[]; estados: () => number } {
  const guardados: Guardado[] = [];
  let estados = 0;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input);
    if (url.startsWith('/api/documents?')) {
      return Promise.resolve(
        Response.json({
          document: { id: 'h1', kind: 'spreadsheet', title: 'Cuentas' },
          role: 'editor',
        }),
      );
    }
    if (url.startsWith('/api/state?')) {
      estados++;
      return Promise.resolve(
        Response.json({
          seq: 1,
          state: {
            lamport: 1,
            registers: {
              'doc:sheet': { value: JSON.stringify(M0), lamport: 1, dot: { a: 'srv', c: 1 } },
            },
            elements: null,
          },
        }),
      );
    }
    if (url.startsWith('/api/ops?') && init?.method === 'POST') {
      const cuerpo = JSON.parse(String(init.body)) as {
        ops: { kind: string; key: string; value?: unknown }[];
      };
      const set = cuerpo.ops.find((o) => o.kind === 'set' && o.key === 'doc:sheet');
      return new Promise((responder) => {
        guardados.push({ modelo: JSON.parse(String(set?.value)) as SheetModel, responder });
      });
    }
    if (url.startsWith('/api/collab?')) return Promise.resolve(new Response('', { status: 404 }));
    return Promise.resolve(Response.json({}));
  }) as typeof fetch;
  return { guardados, estados: () => estados };
}

async function abrir(usuario = 'u1'): Promise<{ doc: () => UseDocumentResult }> {
  // El resultado del hook sale por un efecto, no desde el render: oxlint
  // (react/globals, react/immutability) no deja tocar desde el render algo de fuera.
  const sonda: { actual: UseDocumentResult | null } = { actual: null };
  function Sonda(): null {
    const resultado = useDocument('h1', usuario);
    useLayoutEffect(() => {
      sonda.actual = resultado;
    });
    return null;
  }
  render(
    <EditorI18nCtx.Provider value={docsT('es')}>
      <ToastProvider>
        <Sonda />
      </ToastProvider>
    </EditorI18nCtx.Provider>,
    document.body,
  );
  const doc = (): UseDocumentResult => sonda.actual!;
  await hasta(() => sonda.actual?.status === 'ready', 'la hoja se abre');
  return { doc };
}

/** Filas insertadas hasta ese modelo: cada inserción da una identidad nueva (anclas.ts). */
const filas = (m: SheetModel): number =>
  m.sheets[0]!.rowIds?.filter((a) => !/^t\d+$/.test(a.id)).length ?? 0;

describe('guardado de los cambios de estructura', () => {
  test('dos inserciones seguidas salen en dos guardados, la segunda tras la primera', async () => {
    const { guardados } = servicio();
    const { doc } = await abrir();
    // Algo pendiente, sin guardar todavía (el autoguardado espera su pausa).
    const editado: SheetModel = {
      ...M0,
      sheets: [{ ...M0.sheets[0]!, cells: { ...M0.sheets[0]!.cells, C1: 'nota' } }],
    };
    doc().onContentInput(JSON.stringify(editado));
    const m1 = insertarFila(editado, 1);
    doc().onContentInput(JSON.stringify(m1), true);
    // Sale enseguida, sin la pausa: primero lo pendiente, solo.
    await hasta(() => guardados.length === 1, 'lo pendiente sale enseguida');
    expect(guardados[0]!.modelo).toEqual(editado);
    // La segunda inserción llega con la primera aún sin guardar: espera su turno.
    const m2 = insertarFila(m1, 1);
    doc().onContentInput(JSON.stringify(m2), true);
    await pausa(30);
    expect(guardados).toHaveLength(1);
    guardados[0]!.responder(Response.json({ seq: 2 }));
    await hasta(() => guardados.length === 2, 'la primera inserción sale tras lo pendiente');
    expect(guardados[1]!.modelo).toEqual(m1);
    await pausa(30);
    expect(guardados).toHaveLength(2);
    guardados[1]!.responder(Response.json({ seq: 3 }));
    await hasta(() => guardados.length === 3, 'la segunda inserción sale tras la primera');
    expect(guardados[2]!.modelo).toEqual(m2);
    guardados[2]!.responder(Response.json({ seq: 4 }));
    await hasta(() => doc().save === 'saved', 'todo guardado');
    // Cada guardado lleva un solo desplazamiento sobre el anterior.
    expect(filas(guardados[1]!.modelo)).toBe(filas(guardados[0]!.modelo) + 1);
    expect(filas(guardados[2]!.modelo)).toBe(filas(guardados[1]!.modelo) + 1);
    // Y no sale nada más: lo último ya está guardado.
    await pausa(30);
    expect(guardados).toHaveLength(3);
  });

  test('lo que se teclea mientras la cola guarda sale detrás, sin agruparse con ella', async () => {
    const { guardados } = servicio();
    const { doc } = await abrir();
    const m1 = insertarFila(M0, 0);
    doc().onContentInput(JSON.stringify(m1), true);
    await hasta(() => guardados.length === 1, 'la inserción sale enseguida');
    expect(guardados[0]!.modelo).toEqual(m1); // sin nada pendiente, solo ella
    const escrito: SheetModel = {
      ...m1,
      sheets: [{ ...m1.sheets[0]!, cells: { ...m1.sheets[0]!.cells, C1: 'nota' } }],
    };
    doc().onContentInput(JSON.stringify(escrito));
    guardados[0]!.responder(Response.json({ seq: 2 }));
    await hasta(() => guardados.length === 2, 'lo tecleado sale tras la inserción');
    expect(guardados[1]!.modelo).toEqual(escrito);
    guardados[1]!.responder(Response.json({ seq: 3 }));
    await hasta(() => doc().save === 'saved', 'todo guardado');
  });

  test('si el servidor rechaza una por la protección, se dice como siempre y no sigue la cola', async () => {
    const { guardados, estados } = servicio();
    const { doc } = await abrir();
    const m1 = insertarFila(M0, 0);
    const m2 = insertarFila(m1, 0);
    doc().onContentInput(JSON.stringify(m1), true);
    doc().onContentInput(JSON.stringify(m2), true);
    await hasta(() => guardados.length === 1, 'la primera sale');
    const leidos = estados();
    guardados[0]!.responder(Response.json({ error: { code: 'protected' } }, { status: 403 }));
    await hasta(
      () => document.body.textContent?.includes('Hoja protegida') === true,
      'el aviso de hoja protegida',
    );
    // Se adopta lo del servidor (se relee el estado) y la segunda no se manda.
    expect(estados()).toBe(leidos + 1);
    await hasta(() => doc().save === 'saved', 'el editor vuelve a lo del servidor');
    await pausa(30);
    expect(guardados).toHaveLength(1);
  });
});

/** Los beacons de la salida: `cabe` dice si el navegador acepta cada uno. */
function espiarBeacon(cabe: boolean): Blob[] {
  const enviados: Blob[] = [];
  Object.defineProperty(navigator, 'sendBeacon', {
    configurable: true,
    value: (_url: string, datos: Blob): boolean => {
      if (cabe) enviados.push(datos);
      return cabe;
    },
  });
  return enviados;
}

interface Op {
  kind: string;
  key: string;
  value?: string;
  lamport?: number;
}

/** El servicio que contesta cada guardado enseguida, con el registro en `inicial`. */
function servicioQueGuarda(inicial: SheetModel): { guardados: SheetModel[] } {
  const guardados: SheetModel[] = [];
  let seq = 5;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input);
    if (url.startsWith('/api/documents?')) {
      return Promise.resolve(
        Response.json({
          document: { id: 'h1', kind: 'spreadsheet', title: 'Cuentas' },
          role: 'editor',
        }),
      );
    }
    if (url.startsWith('/api/state?')) {
      return Promise.resolve(
        Response.json({
          seq,
          state: {
            lamport: 5,
            registers: {
              'doc:sheet': { value: JSON.stringify(inicial), lamport: 5, dot: { a: 'srv', c: 5 } },
            },
            elements: null,
          },
        }),
      );
    }
    if (url.startsWith('/api/ops?') && init?.method === 'POST') {
      const { ops } = JSON.parse(String(init.body)) as { ops: Op[] };
      for (const o of ops) {
        if (o.kind === 'set' && o.key === 'doc:sheet') {
          guardados.push(JSON.parse(String(o.value)) as SheetModel);
        }
      }
      seq += ops.length;
      return Promise.resolve(Response.json({ seq }));
    }
    if (url.startsWith('/api/collab?')) return Promise.resolve(new Response('', { status: 404 }));
    return Promise.resolve(Response.json({}));
  }) as typeof fetch;
  return { guardados };
}

describe('al salir de la página con cambios de estructura sin guardar', () => {
  const m1 = insertarFila(M0, 0);
  const m2 = insertarFila(m1, 0);

  /** Abre como DUENO, inserta dos filas (la primera queda en vuelo) y sale de la página. */
  async function insertarDosYSalir(cabe: boolean): Promise<Blob[]> {
    const { guardados } = servicio();
    const { doc } = await abrir(DUENO);
    const enviados = espiarBeacon(cabe);
    doc().onContentInput(JSON.stringify(m1), true);
    doc().onContentInput(JSON.stringify(m2), true);
    await hasta(() => guardados.length === 1, 'la primera inserción vuela');
    window.dispatchEvent(new Event('pagehide'));
    await hasta(() => borrador()?.cola?.length === 2, 'el borrador guarda la cola');
    return enviados;
  }

  test('el beacon lleva un guardado por desplazamiento, en orden', async () => {
    const enviados = await insertarDosYSalir(true);
    expect(enviados).toHaveLength(1);
    const { ops } = JSON.parse(await enviados[0]!.text()) as { ops: Op[] };
    const sets = ops.filter((o) => o.kind === 'set' && o.key === 'doc:sheet');
    expect(sets.map((o) => JSON.parse(String(o.value)) as SheetModel)).toEqual([m1, m2]);
    // El último gana el registro, y cada uno lleva un solo desplazamiento sobre el anterior.
    expect(sets[1]!.lamport!).toBeGreaterThan(sets[0]!.lamport!);
    expect(filas(m1)).toBe(filas(M0) + 1);
    expect(filas(m2)).toBe(filas(m1) + 1);
  });

  test('si no cabe, el borrador guarda la cola y al volver sale de uno en uno', async () => {
    expect(await insertarDosYSalir(false)).toHaveLength(0);
    render(null, document.body);
    // Al volver, el servidor no tiene nada de lo de antes.
    const { guardados } = servicioQueGuarda(M0);
    const { doc } = await abrir(DUENO);
    await hasta(() => doc().save === 'saved', 'todo guardado');
    expect(guardados).toEqual([m1, m2]);
    expect(borrador()?.contentDirty).toBe(false);
    expect(borrador()?.cola).toBeUndefined();
  });

  test('al volver no se repite lo que el servidor ya tiene', async () => {
    await insertarDosYSalir(false);
    render(null, document.body);
    // La primera inserción sí llegó (la que volaba): solo sale la segunda.
    const una = servicioQueGuarda(m1);
    const doc = (await abrir(DUENO)).doc;
    await hasta(() => doc().save === 'saved', 'todo guardado');
    expect(una.guardados).toEqual([m2]);
    render(null, document.body);
    // Y con todo ya en el servidor, nada.
    await insertarDosYSalir(false);
    render(null, document.body);
    const ninguna = servicioQueGuarda(m2);
    const doc2 = (await abrir(DUENO)).doc;
    await pausa(30);
    expect(doc2().save).toBe('saved');
    expect(ninguna.guardados).toEqual([]);
    expect(borrador()?.contentDirty).toBe(false);
  });
});

describe('estructuraDistinta', () => {
  const m1 = insertarFila(M0, 1);
  test('insertar, deshacerlo y rehacerlo cambian la estructura', () => {
    expect(estructuraDistinta(M0, m1)).toBe(true);
    expect(estructuraDistinta(m1, M0)).toBe(true);
  });
  test('escribir, renombrar o añadir una hoja no', () => {
    const escrito: SheetModel = {
      ...m1,
      sheets: [{ ...m1.sheets[0]!, cells: { ...m1.sheets[0]!.cells, C1: 'x' } }],
    };
    expect(estructuraDistinta(m1, escrito)).toBe(false);
    expect(estructuraDistinta(m1, { ...m1, sheets: [{ ...m1.sheets[0]!, name: 'Otra' }] })).toBe(
      false,
    );
    expect(
      estructuraDistinta(m1, { ...m1, sheets: [...m1.sheets, { name: 'Hoja2', cells: {} }] }),
    ).toBe(false);
  });
});
