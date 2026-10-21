// Checks de quién ocupa el escenario grande. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { protagonista, ordenarPorProtagonista, type Escena } from './escenario.js';

function escena(over: Partial<Escena> = {}): Escena {
  return {
    modo: 'mosaico',
    fijado: null,
    ultimoHablante: null,
    compartiendo: null,
    presentes: ['ana', 'luis', 'eva'],
    ...over,
  };
}

test('sin nada que destacar, mosaico', () => {
  expect(protagonista(escena())).toBeNull();
});

test('lo fijado a mano gana a la pantalla compartida', () => {
  // Si no ganara, fijar a alguien dejaría de servir en cuanto otro comparte, que
  // es justo cuando hace falta mirar a la persona y no a la diapositiva.
  expect(protagonista(escena({ fijado: 'ana', compartiendo: 'luis' }))).toBe('ana');
});

test('quien comparte va en grande aunque el modo sea mosaico', () => {
  expect(protagonista(escena({ compartiendo: 'luis' }))).toBe('luis');
});

test('en modo orador manda quien habló, y el silencio NO desmonta el escenario', () => {
  // `ultimoHablante` y no «quien habla ahora»: la señal del SFU se vacía en cada
  // pausa y el escenario se derrumbaría entre frase y frase.
  expect(protagonista(escena({ modo: 'orador', ultimoHablante: 'eva' }))).toBe('eva');
});

test('en mosaico, hablar no saca a nadie en grande', () => {
  expect(protagonista(escena({ modo: 'mosaico', ultimoHablante: 'eva' }))).toBeNull();
});

test('quien ya no está en la sala no ocupa el escenario', () => {
  // Colgar dejaba el hueco grande con el nombre de quien se fue.
  expect(protagonista(escena({ fijado: 'quien-se-fue' }))).toBeNull();
  expect(protagonista(escena({ compartiendo: 'quien-se-fue' }))).toBeNull();
  expect(protagonista(escena({ modo: 'orador', ultimoHablante: 'quien-se-fue' }))).toBeNull();
});

test('el protagonista pasa al frente sin alterar el orden del resto', () => {
  const tiles = [{ id: 'ana' }, { id: 'luis' }, { id: 'eva' }];
  expect(ordenarPorProtagonista(tiles, 'eva').map((t) => t.id)).toEqual(['eva', 'ana', 'luis']);
  // Ya primero, o sin protagonista: se devuelve tal cual (sin copia inútil).
  expect(ordenarPorProtagonista(tiles, 'ana')).toBe(tiles);
  expect(ordenarPorProtagonista(tiles, null)).toBe(tiles);
  expect(ordenarPorProtagonista(tiles, 'nadie')).toBe(tiles);
});
