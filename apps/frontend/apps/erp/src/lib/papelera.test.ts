/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { afterEach, expect, it } from 'bun:test';
import { DURACION_DESHACER, type useToast } from '@sytue/ui';
import { accionesDeLote, aplicarLote, avisarPapelera, marcadasVisibles } from './papelera.js';
import { useT as traducir } from '../i18n/index.js';

const t = traducir('es');
const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

type Aviso = Parameters<ReturnType<typeof useToast>['notify']>[0];
function toastFalso(): { toast: ReturnType<typeof useToast>; avisos: Aviso[] } {
  const avisos: Aviso[] = [];
  return {
    avisos,
    toast: { notify: (a: Aviso) => void avisos.push(a) } as unknown as ReturnType<typeof useToast>,
  };
}

async function hasta(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) await new Promise((r) => setTimeout(r, 0));
}

// VISTO EN ROJO: sin `action` en el aviso no hay «Deshacer»; con `accion: 'borrar'`
// en el cuerpo, el lote se tiraría otra vez en vez de volver.
it('el aviso lleva «Deshacer», que devuelve todo el lote y lo dice', async () => {
  const pedidas: { url: string; cuerpo: string }[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    pedidas.push({ url: `${init?.method} ${String(input)}`, cuerpo: String(init?.body) });
    return new Response(JSON.stringify({ afectados: 2 }), { status: 200 });
  }) as typeof fetch;
  const { toast, avisos } = toastFalso();
  let repintado = 0;
  avisarPapelera(toast, t, 'cliente', ['a', 'b'], () => repintado++);
  expect(avisos[0]?.title).toBe('Movido a la papelera');
  expect(avisos[0]?.duration).toBe(DURACION_DESHACER);
  expect(avisos[0]?.action?.label).toBe('Deshacer');
  avisos[0]?.action?.onClick();
  await hasta(() => repintado > 0);
  expect(pedidas).toEqual([
    { url: 'POST /api/cliente/lote', cuerpo: '{"accion":"restaurar","ids":["a","b"]}' },
  ]);
  expect(avisos[1]?.title).toBe('Devuelto desde la papelera');
  expect(repintado).toBe(1);
});

it('si no vuelve, lo dice con el motivo', async () => {
  globalThis.fetch = (async (_input: RequestInfo | URL) =>
    new Response(JSON.stringify({ error: { message: 'Sin permiso' } }), {
      status: 403,
    })) as typeof fetch;
  const { toast, avisos } = toastFalso();
  let repintado = 0;
  avisarPapelera(toast, t, 'cliente', ['a'], () => repintado++);
  avisos[0]?.action?.onClick();
  await hasta(() => repintado > 0);
  expect(avisos[1]).toMatchObject({
    tone: 'danger',
    title: 'No se pudo deshacer',
    description: 'Sin permiso',
  });
});

// En la papelera la barra de lote DEVUELVE: el aviso es «Restaurado» y no lleva
// «Deshacer», que antes sacaba de la papelera lo que ya estaba en ella.
// VISTO EN ROJO: con `avisarPapelera` para cualquier acción, el aviso decía «Movido a la
// papelera» con «Deshacer».
it('el lote que devuelve dice «Restaurado» y no ofrece deshacer', async () => {
  const pedidas: string[] = [];
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    pedidas.push(String(init?.body));
    return new Response(JSON.stringify({ afectados: 3 }), { status: 200 });
  }) as typeof fetch;
  const { toast, avisos } = toastFalso();
  await aplicarLote(toast, t, 'cliente', 'restaurar', ['a', 'b', 'c'], () => {});
  expect(pedidas).toEqual(['{"accion":"restaurar","ids":["a","b","c"]}']);
  expect(avisos).toHaveLength(1);
  expect(avisos[0]?.title).toBe('Restaurado');
  expect(avisos[0]?.action).toBeUndefined();

  await aplicarLote(toast, t, 'cliente', 'borrar', ['a'], () => {});
  expect(avisos[1]?.title).toBe('Movido a la papelera');
  expect(avisos[1]?.action?.label).toBe('Deshacer');
});

// Tras un lote que sale bien, la lista se vuelve a pedir: antes se «refrescaba» poniendo
// el mismo filtro de estado, Preact no repintaba nada y lo restaurado seguía en la
// papelera. Si el lote falla, no se recarga nada.
// VISTO EN ROJO: sin la llamada a `recargar` al final de `aplicarLote`, `recargas` es 0.
it('el lote que sale bien recarga la lista; el que falla, no', async () => {
  globalThis.fetch = (async (_input: RequestInfo | URL) =>
    new Response(JSON.stringify({ afectados: 2 }), { status: 200 })) as typeof fetch;
  const { toast } = toastFalso();
  let recargas = 0;
  for (const accion of ['restaurar', 'borrar', 'estado'] as const) {
    recargas = 0;
    await aplicarLote(toast, t, 'cliente', accion, ['a', 'b'], () => recargas++, 'pagada');
    expect(recargas).toBe(1);
  }

  globalThis.fetch = (async (_input: RequestInfo | URL) =>
    new Response(JSON.stringify({ error: { message: 'Sin permiso' } }), {
      status: 403,
    })) as typeof fetch;
  recargas = 0;
  const fallo = await aplicarLote(toast, t, 'cliente', 'restaurar', ['a'], () => recargas++).then(
    () => null,
    (err: unknown) => err,
  );
  expect(fallo).toBeInstanceOf(Error);
  expect(recargas).toBe(0);
});

// La barra cuenta y actúa sobre lo marcado QUE SE VE: lo marcado en otro filtro no entra.
// VISTO EN ROJO: devolviendo `[...seleccionados]`, salen 'x' y 'y', que no están en la lista.
it('el lote solo lleva las filas marcadas que están en la lista, en su orden', () => {
  const registros = [{ id: 'c' }, { id: 'a' }, { id: 'b' }];
  expect(marcadasVisibles(new Set(['x', 'b', 'c', 'y']), registros)).toEqual(['c', 'b']);
  expect(marcadasVisibles(new Set(['x', 'y']), registros)).toEqual([]);
});

// VISTO EN ROJO: invirtiendo la rama de `papelera`, dentro sale «borrar» y fuera «restaurar».
it('dentro de la papelera la barra solo devuelve; fuera cambia el estado y tira', () => {
  expect(accionesDeLote(true, true)).toEqual({ cambiarEstado: false, accion: 'restaurar' });
  expect(accionesDeLote(false, true)).toEqual({ cambiarEstado: true, accion: 'borrar' });
  expect(accionesDeLote(false, false)).toEqual({ cambiarEstado: false, accion: 'borrar' });
});
