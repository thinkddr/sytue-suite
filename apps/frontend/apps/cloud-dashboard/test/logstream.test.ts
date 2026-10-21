/**
 * Self-check de la desenvoltura del stream de logs en vivo. El control-plane emite
 * UNA LÍNEA POR EVENTO SSE (`streams_test.go` fija ese lado del contrato), así que
 * aquí lo único que hay que probar es que la red partiendo un evento por cualquier
 * sitio no pierda ni invente líneas. Sin DOM ni framework. Vive fuera de src/ para
 * no entrar en el `astro check`/eslint de la app. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { appendCapped, levelOf, runtimeLine, takeLines } from '../src/lib/logstream.js';

/** Emula al servidor: un evento por línea de log. */
const frame = (line: string) => `data: ${line}\n\n`;

/** Pasa una secuencia de trozos de red por takeLines y devuelve las líneas emitidas. */
function drain(wire: string[]): { lines: string[]; carry: string } {
  let carry = '';
  const lines: string[] = [];
  for (const w of wire) {
    const cut = takeLines(carry, w);
    carry = cut.carry;
    lines.push(...cut.lines);
  }
  return { lines, carry };
}

test('varios eventos en una sola lectura salen como varias líneas', () => {
  const out = drain([frame('arrancando') + frame('escuchando en :8080') + frame('GET / 200 3ms')]);
  expect(out.lines).toEqual(['arrancando', 'escuchando en :8080', 'GET / 200 3ms']);
  expect(out.carry).toBe('');
});

// Lo único que TCP no garantiza: el corte puede caer en CUALQUIER byte, también en
// mitad del prefijo o del separador. Se prueban todos los cortes posibles.
test('el evento se recompone se parta por donde se parta', () => {
  const wire = frame('GET / 200 3ms') + frame('ERROR db timeout');
  for (let cut = 0; cut <= wire.length; cut++) {
    const out = drain([wire.slice(0, cut), wire.slice(cut)]);
    expect(out.lines).toEqual(['GET / 200 3ms', 'ERROR db timeout']);
  }
});

test('el evento incompleto espera al trozo siguiente en vez de salir cortado', () => {
  const out = drain(['data: GET /']);
  expect(out.lines).toEqual([]);
  expect(takeLines(out.carry, ' 200\n\n').lines).toEqual(['GET / 200']);
});

test('lo que no es un campo data se ignora', () => {
  // Comentario de keep-alive: no es una línea de log y no puede aparecer como tal.
  const out = drain([': ping\n\n' + frame('GET / 200')]);
  expect(out.lines).toEqual(['GET / 200']);
});

test('levelOf clasifica por heurística', () => {
  expect(levelOf('panic: nil map')).toBe('error');
  expect(levelOf('warning: deprecated flag')).toBe('warn');
  expect(levelOf('listening on :8080')).toBe('info');
});

test('runtimeLine marca origen runtime', () => {
  expect(runtimeLine('hola')).toEqual({
    ts: '',
    level: 'info',
    source: 'runtime',
    message: 'hola',
  });
});

test('appendCapped conserva solo las últimas N líneas', () => {
  const mk = (n: number) => Array.from({ length: n }, (_, i) => runtimeLine(String(i)));
  const out = appendCapped(mk(3), mk(3), 4);
  expect(out).toHaveLength(4);
  expect(out.map((l) => l.message)).toEqual(['2', '0', '1', '2']);
  // Sin líneas nuevas devuelve el mismo array (evita re-render inútil).
  const prev = mk(2);
  expect(appendCapped(prev, [], 10)).toBe(prev);
});
