import { describe, expect, it } from 'bun:test';
import { conIdsDeHoja } from './idhoja';

describe('conIdsDeHoja', () => {
  it('deduce el id del nombre, sin repetir, y conserva el que ya hay', () => {
    const a = conIdsDeHoja([
      { name: 'A', cells: {} },
      { name: 'A', cells: {} },
      { name: 'B', id: 'fijo', cells: {} },
    ]);
    expect(a.map((h) => h.id)).toEqual(['h:A', 'h:A#1', 'fijo']);
    expect(conIdsDeHoja(a)).toEqual(a);
  });
});
