/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, expect, it } from 'bun:test';
import type { useToast } from '@sytue/ui';
import { invalidarModelo, type ListaAbierta } from './invalidar.js';
import { avisarPapelera } from './papelera.js';
import { useT as traducir } from '../i18n/index.js';

const t = traducir('es');
const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

type Aviso = Parameters<ReturnType<typeof useToast>['notify']>[0];
type M = { clave: string };

async function hasta(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) await new Promise((r) => setTimeout(r, 0));
}

// VISTO EN ROJO: con el callback del render del borrado (el modelo y los filtros de
// ENTONCES, como hacía `onImportado`/`onGuardado`), «Deshacer» pulsado ya en Facturas
// recarga la lista con los Clientes.
it('«Deshacer» tras navegar a otro modelo no recarga su lista con la del borrado', async () => {
  globalThis.fetch = (async (_input: RequestInfo | URL) =>
    new Response(JSON.stringify({ afectados: 1 }), { status: 200 })) as typeof fetch;
  const avisos: Aviso[] = [];
  const toast = { notify: (a: Aviso) => void avisos.push(a) } as unknown as ReturnType<
    typeof useToast
  >;
  const cache = new Map<string, unknown>([
    ['cliente||||', 'vieja'],
    ['cliente|ana|||', 'vieja'],
    ['factura_venta||||', 'actual'],
  ]);
  // Lo que el padre tiene en su ref: se lee al pulsar, no al crear el aviso.
  const abierta: { current: ListaAbierta<M> } = {
    current: {
      modelo: { clave: 'cliente' },
      busqueda: '',
      estado: '',
      orden: null,
      papelera: false,
    },
  };
  const recargas: ListaAbierta<M>[] = [];
  const invalidar = (clave: string): void =>
    invalidarModelo(clave, cache, abierta.current, (l) => recargas.push(l));
  avisarPapelera(toast, t, 'cliente', ['a'], () => invalidar('cliente'));

  // Se abre Facturas, filtrado, antes de pulsar «Deshacer».
  abierta.current = {
    modelo: { clave: 'factura_venta' },
    busqueda: '',
    estado: 'pagado',
    orden: null,
    papelera: false,
  };
  avisos[0]?.action?.onClick();
  await hasta(() => avisos.length > 1);
  await hasta(() => !cache.has('cliente||||'));
  expect(recargas).toEqual([]);
  expect([...cache.keys()]).toEqual(['factura_venta||||']);

  // De vuelta en Clientes, con otra búsqueda: recarga ESA.
  abierta.current = { ...abierta.current, modelo: { clave: 'cliente' }, busqueda: 'eva' };
  invalidar('cliente');
  expect(recargas).toEqual([
    {
      modelo: { clave: 'cliente' },
      busqueda: 'eva',
      estado: 'pagado',
      orden: null,
      papelera: false,
    },
  ]);
});
