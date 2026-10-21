// Checks de los atajos de formato del composer. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { aplicarFormato, marcaDeAtajo } from './formato.js';

test('negrita sobre la selección: la envuelve y la deja seleccionada', () => {
  //            0123456789
  const r = aplicarFormato('un golpe duro', 3, 8, 'bold');
  expect(r.value).toBe('un **golpe** duro');
  expect(r.value.slice(r.selStart, r.selEnd)).toBe('golpe');
});

test('sin selección deja el cursor ENTRE las marcas, listo para escribir', () => {
  const r = aplicarFormato('hola ', 5, 5, 'bold');
  expect(r.value).toBe('hola ****');
  expect(r.selStart).toBe(7);
  expect(r.selEnd).toBe(7);
});

test('alternar: si ya estaba en negrita, la quita', () => {
  const r = aplicarFormato('un **golpe** duro', 3, 12, 'bold');
  expect(r.value).toBe('un golpe duro');
  expect(r.value.slice(r.selStart, r.selEnd)).toBe('golpe');
});

test('cursiva usa _ y no rompe snake_case al alternar', () => {
  const r = aplicarFormato('dame eso', 5, 8, 'italic');
  expect(r.value).toBe('dame _eso_');
});

test('enlace sobre la selección: el cursor queda dentro del paréntesis', () => {
  const r = aplicarFormato('mira el informe', 8, 15, 'link');
  expect(r.value).toBe('mira el [informe]()');
  // Pegar ahora deja `[informe](https://…)`.
  expect(r.selStart).toBe(r.value.indexOf('()') + 1);
  expect(r.selStart).toBe(r.selEnd);
});

test('enlace sin selección: el hueco del texto va primero', () => {
  const r = aplicarFormato('', 0, 0, 'link');
  expect(r.value).toBe('[]()');
  expect(r.selStart).toBe(1);
});

test('marcaDeAtajo reconoce Cmd/Ctrl + B/I/K y nada más', () => {
  const base = { metaKey: true, ctrlKey: false, altKey: false };
  expect(marcaDeAtajo({ ...base, key: 'b' })).toBe('bold');
  expect(marcaDeAtajo({ ...base, key: 'I' })).toBe('italic');
  expect(marcaDeAtajo({ ...base, key: 'k' })).toBe('link');
  expect(marcaDeAtajo({ ...base, key: 'u' })).toBeNull();
  // Sin modificador no es atajo: escribir «b» debe escribir «b».
  expect(marcaDeAtajo({ key: 'b', metaKey: false, ctrlKey: false, altKey: false })).toBeNull();
  // Alt+B escribe un carácter en varios teclados: no lo secuestramos.
  expect(marcaDeAtajo({ key: 'b', metaKey: false, ctrlKey: true, altKey: true })).toBeNull();
});

test('tachado y código envuelven con ~~ y `, y alternan como la negrita', () => {
  expect(aplicarFormato('a b c', 2, 3, 'strike').value).toBe('a ~~b~~ c');
  expect(aplicarFormato('a ~~b~~ c', 2, 7, 'strike').value).toBe('a b c');
  expect(aplicarFormato('x = 1', 0, 5, 'code').value).toBe('`x = 1`');
});

test('cita y lista prefijan cada línea que toca la selección; si todas lo llevan, lo quitan', () => {
  const r = aplicarFormato('uno\ndos\ntres', 0, 7, 'list');
  expect(r.value).toBe('- uno\n- dos\ntres');
  expect(r.value.slice(r.selStart, r.selEnd)).toBe('uno\n- dos');
  const q = aplicarFormato('> ya citado', 4, 4, 'quote');
  expect(q.value).toBe('ya citado');
  expect(q.selStart).toBe(2);
  expect(q.selEnd).toBe(2);
});

test('marcaDeAtajo: Cmd/Ctrl+Mayús+X es tachado; sin Mayús, la X no es nada', () => {
  expect(
    marcaDeAtajo({ key: 'x', metaKey: true, ctrlKey: false, altKey: false, shiftKey: true }),
  ).toBe('strike');
  expect(marcaDeAtajo({ key: 'x', metaKey: true, ctrlKey: false, altKey: false })).toBeNull();
});
