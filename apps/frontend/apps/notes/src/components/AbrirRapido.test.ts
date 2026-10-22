/**
 * «Ir a» tiene que encontrar sin acentos, poner los cuadernos primero y no
 * repetir una nota que ya trae el cuaderno abierto. Los mismos datos que
 * TestIrAEncuentraCuadernosYNotas de apps/fyne/apps/notes.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import type { Note, NoteHit, Notebook } from '@sytue/api-clients';
import { opcionesIrA } from './AbrirRapido.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const cuadernos = [
  { id: 'c1', title: 'Reuniones' },
  { id: 'c2', title: 'Casa' },
] as Notebook[];
const notas = [
  { id: 'n1', notebook_id: 'c1', title: 'Reunión de lunes' },
  { id: 'n2', notebook_id: 'c1', title: 'Compra' },
  { id: 'n3', notebook_id: 'c1', title: '', content_cipher: 'x' },
] as Note[];
const remotas = [
  { id: 'n1', notebook_id: 'c1', notebook: 'Reuniones', title: 'Reunión de lunes' },
  { id: 'n7', notebook_id: 'c2', notebook: 'Casa', title: 'reunion vecinos' },
] as NoteHit[];

test('cuadernos, notas del abierto y búsqueda, sin acentos ni repetidas', () => {
  const o = opcionesIrA('reunion', cuadernos, notas, remotas, 'Sin título');
  expect(o.map((x) => `${x.tipo}:${x.id}`)).toEqual(['cuaderno:c1', 'nota:n1', 'nota:n7']);
  expect(o[1]?.detalle).toBe('Reuniones');
});

test('sin consulta salen todos los cuadernos y las notas legibles del abierto', () => {
  const o = opcionesIrA('', cuadernos, notas, [], 'Sin título');
  expect(o.map((x) => x.id)).toEqual(['c1', 'c2', 'n1', 'n2']);
});
