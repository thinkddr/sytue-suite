import { describe, expect, it } from 'bun:test';
import { celdaEnElementos, destinoTirador, puntoDeTirador } from './tiradores';

const B2D5 = { c1: 1, r1: 1, c2: 3, r2: 4 };

describe('destinoTirador', () => {
  it('el tirador de abajo agranda el rango y deja quieta la esquina de arriba', () => {
    expect(destinoTirador(B2D5, 'fin', { c: 5, r: 9 })).toEqual({
      sel: { c: 1, r: 1 },
      ext: { c: 5, r: 9 },
    });
  });

  it('el de arriba mueve su esquina y la de abajo pasa a ser el ancla', () => {
    expect(destinoTirador(B2D5, 'inicio', { c: 0, r: 0 })).toEqual({
      sel: { c: 3, r: 4 },
      ext: { c: 0, r: 0 },
    });
  });

  it('cruzar al otro lado da la vuelta al rango en vez de deshacerlo', () => {
    // El de abajo llevado por encima de B2: el rango es ahora A1:B2, con B2 de ancla.
    expect(destinoTirador(B2D5, 'fin', { c: 0, r: 0 })).toEqual({
      sel: { c: 1, r: 1 },
      ext: { c: 0, r: 0 },
    });
  });

  it('llegar a la esquina quieta deja solo esa celda, sin rango de una', () => {
    expect(destinoTirador(B2D5, 'fin', { c: 1, r: 1 })).toEqual({ sel: { c: 1, r: 1 }, ext: null });
  });

  it('el de arriba de una combinada vuelve a su ancla si no sale de ella', () => {
    // La combinada B2:C3 sola: el tirador de arriba deja de ancla C3, que está cubierta.
    const b2c3 = { c1: 1, r1: 1, c2: 2, r2: 2 };
    const fusion = (c: number, r: number) => (c >= 1 && c <= 2 && r >= 1 && r <= 2 ? b2c3 : null);
    expect(destinoTirador(b2c3, 'inicio', { c: 1, r: 1 }, fusion)).toEqual({
      sel: { c: 1, r: 1 },
      ext: null,
    });
    // Saliendo de la combinada, el rango sigue anclado en la esquina quieta.
    expect(destinoTirador(b2c3, 'inicio', { c: 0, r: 0 }, fusion)).toEqual({
      sel: { c: 2, r: 2 },
      ext: { c: 0, r: 0 },
    });
  });

  it('desde una celda sola, el tirador de abajo empieza el rango', () => {
    const una = { c1: 2, r1: 2, c2: 2, r2: 2 };
    expect(destinoTirador(una, 'fin', { c: 2, r: 6 })).toEqual({
      sel: { c: 2, r: 2 },
      ext: { c: 2, r: 6 },
    });
  });
});

describe('puntoDeTirador', () => {
  it('suma el agarre y se mete hacia dentro del rango', () => {
    // El dedo cogió el tirador de abajo 5 px a la derecha y 3 por debajo de la esquina.
    expect(puntoDeTirador('fin', { x: 105, y: 53 }, { x: -5, y: -3 })).toEqual({ x: 98, y: 48 });
    expect(puntoDeTirador('inicio', { x: 40, y: 20 }, { x: 0, y: 0 })).toEqual({ x: 42, y: 22 });
  });
});

describe('celdaEnElementos', () => {
  const nodo = (celda: { c: string; r: string } | null) => ({
    closest: () => (celda ? ({ dataset: celda } as unknown as HTMLElement) : null),
  });

  it('salta lo que no es celda (el propio tirador, un gráfico) y da la primera de debajo', () => {
    expect(celdaEnElementos([nodo(null), nodo(null), nodo({ c: '4', r: '11' })])).toEqual({
      c: 4,
      r: 11,
    });
  });

  it('sin celda bajo el punto no hay destino', () => {
    expect(celdaEnElementos([nodo(null)])).toBeNull();
    expect(celdaEnElementos([nodo({ c: '', r: 'x' })])).toBeNull();
    expect(celdaEnElementos([nodo({ c: '', r: '3' })])).toBeNull();
  });
});
