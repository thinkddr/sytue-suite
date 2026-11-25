/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { ReanudacionLlamada, type PasosReanudacion } from './reanudacion.js';

/** Pasos que apuntan lo que pasa; `reanudar` espera a que el test la deje seguir. */
function pasos(
  trace: string[],
  opciones: { fallaReanudar?: boolean; fallaSuspender?: boolean } = {},
) {
  let soltar!: () => void;
  const reanudada = new Promise<void>((resolve) => {
    soltar = resolve;
  });
  const p: PasosReanudacion = {
    suspender: () => {
      if (opciones.fallaSuspender) throw new Error('sin cifrado');
      trace.push('suspender');
    },
    pararPistas: () => trace.push('parar-pistas'),
    avisar: () => trace.push('reconectando'),
    cortarTransporte: async () => {
      trace.push('cortar');
    },
    reanudar: async () => {
      trace.push('reanudar');
      await reanudada;
      if (opciones.fallaReanudar) throw new Error('403');
      trace.push('publicar');
    },
    fallar: () => trace.push('fallar'),
  };
  return { p, soltar };
}

test('la reconexión del SDK invalida y para las pistas AL MOMENTO, antes de ceder el hilo', () => {
  const trace: string[] = [];
  const { p, soltar } = pasos(trace);
  const r = new ReanudacionLlamada(p);
  void r.alReconectarSolo();
  // Síncrono: nada del SDK puede publicar con el permiso de antes, y su transporte
  // se empieza a cortar sin esperar.
  expect(trace).toEqual(['suspender', 'parar-pistas', 'reconectando', 'cortar']);
  expect(r.reanudando).toBe(true);
  soltar();
});

test('se reanuda a mano: corta el transporte, pide ámbito y acuse nuevos y solo después publica', async () => {
  const trace: string[] = [];
  const { p, soltar } = pasos(trace);
  const r = new ReanudacionLlamada(p);
  const job = r.alReconectarSolo();
  await Promise.resolve();
  await Promise.resolve();
  expect(trace).toEqual(['suspender', 'parar-pistas', 'reconectando', 'cortar', 'reanudar']);
  expect(trace).not.toContain('publicar');
  soltar();
  await job;
  expect(trace).toEqual([
    'suspender',
    'parar-pistas',
    'reconectando',
    'cortar',
    'reanudar',
    'publicar',
  ]);
  expect(r.reanudando).toBe(false);
  expect(r.fallida).toBe(false);
});

test('un segundo aviso del SDK mientras se reanuda no vuelve a empezar', async () => {
  const trace: string[] = [];
  const { p, soltar } = pasos(trace);
  const r = new ReanudacionLlamada(p);
  const primera = r.alReconectarSolo();
  const segunda = r.alReconectarSolo();
  expect(segunda).toBe(primera);
  soltar();
  await primera;
  expect(trace.filter((x) => x === 'suspender')).toHaveLength(1);
  expect(trace.filter((x) => x === 'reanudar')).toHaveLength(1);
});

test('si la reanudación falla, la llamada termina cerrada y no se reanuda más', async () => {
  const trace: string[] = [];
  const { p, soltar } = pasos(trace, { fallaReanudar: true });
  const r = new ReanudacionLlamada(p);
  const job = r.alReconectarSolo();
  soltar();
  await job;
  expect(trace).toContain('fallar');
  expect(trace).not.toContain('publicar');
  expect(r.fallida).toBe(true);
  trace.length = 0;
  await r.alReconectarSolo();
  expect(trace).toEqual([]);
});

test('sin cifrado ligado no se reanuda: falla sin cortar ni publicar', async () => {
  const trace: string[] = [];
  const { p } = pasos(trace, { fallaSuspender: true });
  const r = new ReanudacionLlamada(p);
  await r.alReconectarSolo();
  expect(trace).toEqual(['fallar']);
  expect(r.fallida).toBe(true);
});

test('una red que se cae una y otra vez acaba la llamada en vez de reanudar sin fin', async () => {
  const trace: string[] = [];
  let ahora = 0;
  const p: PasosReanudacion = {
    suspender: () => trace.push('suspender'),
    pararPistas: () => undefined,
    avisar: () => undefined,
    cortarTransporte: async () => undefined,
    reanudar: async () => {
      trace.push('reanudar');
    },
    fallar: () => trace.push('fallar'),
  };
  const r = new ReanudacionLlamada(p, { maxSeguidas: 2, ventanaMs: 60_000, ahora: () => ahora });
  await r.alReconectarSolo();
  ahora += 1_000;
  await r.alReconectarSolo();
  ahora += 1_000;
  await r.alReconectarSolo();
  expect(trace.filter((x) => x === 'reanudar')).toHaveLength(2);
  expect(trace.at(-1)).toBe('fallar');
  expect(r.fallida).toBe(true);
});

test('pasada la ventana, una caída aislada se vuelve a reanudar', async () => {
  let ahora = 0;
  let reanudadas = 0;
  const p: PasosReanudacion = {
    suspender: () => undefined,
    pararPistas: () => undefined,
    avisar: () => undefined,
    cortarTransporte: async () => undefined,
    reanudar: async () => {
      reanudadas++;
    },
    fallar: () => {
      throw new Error('no debía fallar');
    },
  };
  const r = new ReanudacionLlamada(p, { maxSeguidas: 1, ventanaMs: 60_000, ahora: () => ahora });
  await r.alReconectarSolo();
  ahora += 61_000;
  await r.alReconectarSolo();
  expect(reanudadas).toBe(2);
  expect(r.fallida).toBe(false);
});

test('parar (colgar o cerrar la vista) impide reanudar después', async () => {
  const trace: string[] = [];
  const { p } = pasos(trace);
  const r = new ReanudacionLlamada(p);
  r.parar();
  await r.alReconectarSolo();
  expect(trace).toEqual([]);
});
