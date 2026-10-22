import { describe, expect, it } from 'bun:test';
import { candidatos, consultaMencion, moverActiva, ponerMencion } from './Menciones.js';

// La mención que se inserta es la que el servicio reconoce para avisar: `@correo`.
describe('menciones', () => {
  it('sale al escribir una arroba suelta, no en medio de una dirección', () => {
    expect(consultaMencion('Hola @an')).toBe('an');
    expect(consultaMencion('@')).toBe('');
    expect(consultaMencion('escribe a ana@x.es')).toBeNull();
    expect(consultaMencion('Hola @ana@x.es ya')).toBeNull();
  });
  it('escribe la dirección elegida en lugar de lo escrito', () => {
    expect(ponerMencion('Hola @an', 'ana@x.es')).toBe('Hola @ana@x.es ');
  });
  it('busca por nombre y por dirección', () => {
    const g = [{ email: 'ana@x.es', name: 'Ana Pérez' }, { email: 'luis@x.es' }];
    expect(candidatos(g, 'pér').map((p) => p.email)).toEqual(['ana@x.es']);
    expect(candidatos(g, 'luis').map((p) => p.email)).toEqual(['luis@x.es']);
  });
});

describe('menciones con el teclado', () => {
  it('las flechas recorren la lista y dan la vuelta en los extremos', () => {
    expect(moverActiva(0, 3, 'ArrowDown')).toBe(1);
    expect(moverActiva(2, 3, 'ArrowDown')).toBe(0);
    expect(moverActiva(0, 3, 'ArrowUp')).toBe(2);
    expect(moverActiva(1, 3, 'a')).toBe(1);
    expect(moverActiva(0, 0, 'ArrowDown')).toBe(0);
  });
});
