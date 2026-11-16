// El «Deshacer» de la papelera de Fotos. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { deshacerTirada, reponer, tirada, tirarSinRepetir, type GaleriaViva } from './deshacer.js';

const f = (id: string) => ({ id });
const ids = (l: { id: string }[]) => l.map((x) => x.id);

test('la tanda guarda solo lo que el servicio aceptó y el orden de entonces', () => {
  const t = tirada([f('a'), f('b'), f('c')], new Set(['b', 'x']), 7);
  expect(ids([...t.fotos])).toEqual(['b']);
  expect(t.orden).toEqual(['a', 'b', 'c']);
  expect(t.generacion).toBe(7);
});

test('lo devuelto vuelve a su sitio, no al final', () => {
  const orden = ['a', 'b', 'c', 'd', 'e'];
  expect(ids(reponer([f('a'), f('c'), f('e')], [f('d'), f('b')], orden))).toEqual(orden);
  // La primera de la galería vuelve arriba del todo.
  expect(ids(reponer([f('b'), f('c')], [f('a')], ['a', 'b', 'c']))).toEqual(['a', 'b', 'c']);
});

test('reponer no repite una foto que ya está ni mueve lo cargado después', () => {
  const r = reponer([f('a'), f('b'), f('z')], [f('b')], ['a', 'b']);
  expect(ids(r)).toEqual(['a', 'b', 'z']);
  // Una página nueva llegada en esos 10 s se queda detrás.
  expect(ids(reponer([f('a'), f('n1')], [f('b')], ['a', 'b']))).toEqual(['a', 'b', 'n1']);
});

test('deshacer restaura cada una y repone las que volvieron', async () => {
  let lista = [f('a'), f('c')];
  const pedidas: string[] = [];
  const t = { fotos: [f('b'), f('d')], orden: ['a', 'b', 'c', 'd'], generacion: 1 };
  const galeria = { current: { montada: true, generacion: 1 } satisfies GaleriaViva };
  const r = await deshacerTirada(
    t,
    async (id) => {
      pedidas.push(id);
      if (id === 'd') throw new Error('403');
    },
    galeria,
    (cambio) => {
      lista = cambio(lista);
    },
  );
  expect(pedidas.sort()).toEqual(['b', 'd']);
  expect(r).toEqual({ devueltas: 1, fallidas: 1 });
  expect(ids(lista)).toEqual(['a', 'b', 'c']);
});

test('si la galería ya es otra o no está, restaura pero no pinta', async () => {
  const t = { fotos: [f('b')], orden: ['a', 'b'], generacion: 1 };
  for (const vista of [
    { montada: true, generacion: 2 },
    { montada: false, generacion: 1 },
  ]) {
    let tocada = false;
    const r = await deshacerTirada(
      t,
      async () => {},
      { current: vista },
      () => {
        tocada = true;
      },
    );
    expect(r.devueltas).toBe(1);
    expect(tocada).toBe(false);
  }
});

// Lo reversible no pregunta (design §5.4): ni el visor ni la barra de selección
// abren un diálogo antes de mandar a la papelera, y el aviso lleva su «Deshacer».
test('la galería manda a la papelera sin preguntar y ofrece deshacer 10 s', () => {
  const fuente = readFileSync(new URL('../components/PhotosApp.tsx', import.meta.url), 'utf8');
  expect(fuente).not.toContain('confirmarPapelera');
  expect(fuente).not.toContain("t('viewer.deleteConfirm')");
  expect(fuente).not.toContain('gallery.trashSelectedConfirm');
  expect(fuente).toContain('duration: DURACION_DESHACER');
  expect(fuente).toContain('deshacerTirada(');
  expect(fuente).toMatch(/case 'papelera':\s*if \(onDelete && !e\.repeat\) onDelete\(foto\)/);
});

test('un segundo toque con la primera llamada en vuelo no vuelve a mandar la foto', async () => {
  const enVuelo = new Set<string>();
  const pedidas: string[] = [];
  let soltar = () => {};
  const tirar = (id: string) => {
    pedidas.push(id);
    return id === 'a' ? new Promise<void>((r) => (soltar = r)) : Promise.reject(new Error('500'));
  };
  const primera = tirarSinRepetir(['a'], tirar, enVuelo);
  const segunda = await tirarSinRepetir(['a', 'b'], tirar, enVuelo);
  expect(segunda.nuevas).toEqual(['b']);
  expect(segunda.hechas.size).toBe(0);
  expect(segunda.fallo?.reason).toBeInstanceOf(Error);
  soltar();
  const r = await primera;
  expect([...r.hechas]).toEqual(['a']);
  expect(r.fallo).toBeUndefined();
  expect(pedidas).toEqual(['a', 'b']);
  // Acabada, la foto se puede volver a mandar (p. ej. tras deshacer).
  expect((await tirarSinRepetir(['a'], async () => {}, enVuelo)).nuevas).toEqual(['a']);
});
