import { expect, test } from 'bun:test';
import { disponibleConCifrado, type AccionCifrada } from './cifrado.js';

const TODAS: AccionCifrada[] = [
  'programar',
  'encuesta',
  'tarea',
  'pedirAcuse',
  'votar',
  'confirmarAcuse',
];

test('sin cifrado se ofrece todo, como siempre', () => {
  for (const a of TODAS) expect(disponibleConCifrado(a, false)).toBe(true);
});

test('con cifrado no existe ninguna de las acciones que cuenta el servicio', () => {
  for (const a of TODAS) expect(disponibleConCifrado(a, true)).toBe(false);
});
