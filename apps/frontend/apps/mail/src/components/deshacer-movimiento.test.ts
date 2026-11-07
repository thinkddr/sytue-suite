/**
 * «Deshacer» tras archivar, tirar a la papelera, marcar spam o mover.
 *
 * Lo que puede salir mal no da error: devolver TODO a la carpeta de la vista. En una
 * búsqueda o en una etiqueta la vista cruza carpetas, y un hilo tiene a la vez su
 * respuesta en Enviados y lo recibido en Recibidos; deshacer no puede dejar tu
 * respuesta en Recibidos. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import type { EmailSummary } from '@sytue/api-clients';
import { deshacerMovimiento, groupByThread, origenesDeMovimiento } from './utils.js';

function correo(id: string, threadId: string, mailboxId?: string): EmailSummary {
  return {
    id,
    threadId,
    subject: 'Asunto',
    from: [{ email: 'ana@ejemplo.eu' }],
    to: [],
    cc: [],
    preview: '',
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: {},
    hasAttachment: false,
    ...(mailboxId ? { mailboxId } : {}),
  };
}

test('cada mensaje vuelve a SU carpeta, no a la de la vista', () => {
  const convs = groupByThread([correo('m1', 'h1', 'in'), correo('m2', 'h1', 'se')]);
  const origenes = origenesDeMovimiento(convs, null, 'ar');
  expect(Object.fromEntries(origenes)).toEqual({ in: ['m1'], se: ['m2'] });
});

test('sin carpeta en el sobre se cae a la que se estaba mirando', () => {
  const convs = groupByThread([correo('m1', 'h1'), correo('m2', 'h2')]);
  expect(Object.fromEntries(origenesDeMovimiento(convs, 'in', 'tr'))).toEqual({
    in: ['m1', 'm2'],
  });
});

test('lo que ya estaba en el destino o no tiene origen conocido no se toca', () => {
  const convs = groupByThread([correo('m1', 'h1', 'ar'), correo('m2', 'h1')]);
  expect(origenesDeMovimiento(convs, null, 'ar').size).toBe(0);
});

/*
 * Y deshacer de verdad, con un `batchMove` simulado: una llamada por carpeta de
 * origen, y después se piden la carpeta, la búsqueda y los contadores —también si
 * falla—, o en una búsqueda lo devuelto no volvía hasta repetirla.
 */
function refrescos(): {
  hechos: string[];
  carpeta: () => void;
  busqueda: () => void;
  contadores: () => void;
} {
  const hechos: string[] = [];
  return {
    hechos,
    carpeta: () => hechos.push('carpeta'),
    busqueda: () => hechos.push('busqueda'),
    contadores: () => hechos.push('contadores'),
  };
}

test('deshacer devuelve cada lote a su carpeta y vuelve a pedir las dos listas', async () => {
  const movidos: [string[], string][] = [];
  const r = refrescos();
  const origenes = new Map([
    ['in', ['m1', 'm3']],
    ['se', ['m2']],
  ]);
  const ok = await deshacerMovimiento(
    origenes,
    async (ids, carpeta) => movidos.push([ids, carpeta]),
    r,
  );
  expect(ok).toBe(true);
  expect(movidos).toEqual([
    [['m1', 'm3'], 'in'],
    [['m2'], 'se'],
  ]);
  expect(r.hechos.sort()).toEqual(['busqueda', 'carpeta', 'contadores']);
});

test('si una carpeta rechaza, se avisa del fallo y las listas se piden igual', async () => {
  const r = refrescos();
  const ok = await deshacerMovimiento(
    new Map([['in', ['m1']]]),
    async () => {
      throw new Error('409');
    },
    r,
  );
  expect(ok).toBe(false);
  expect(r.hechos.sort()).toEqual(['busqueda', 'carpeta', 'contadores']);
});
