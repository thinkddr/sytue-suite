/**
 * El aro de la casilla de una tarea sigue su forma (ver el final de app.css).
 *
 * Dos cosas lo rompen sin que se vea en ningún test de componente: meter la regla en
 * una `@layer` —el aro global, sin capa, le gana y vuelve el cuadrado— y pintarlo con
 * otro color que no sea el token del sistema.
 */
import { expect, test } from 'bun:test';

const css = await Bun.file(new URL('./app.css', import.meta.url)).text();

test('el aro de la tarea va en la fila, con esquina y con el token, fuera de toda capa', () => {
  const sel = ".sytue-prose .task-row:has(> input[type='checkbox']:focus-visible)::before {";
  const i = css.indexOf(sel);
  expect(i).toBeGreaterThan(-1);
  const regla = css.slice(i, css.indexOf('}', i));
  expect(regla).toContain('box-shadow: 0 0 0 2px var(--sytue-focus-ring)');
  expect(regla).toContain('border-radius: 2px');
  // Fuera de capa: todas las `@layer` del fichero se cierran antes de la regla.
  const antes = css.slice(0, i);
  const abiertas = (antes.match(/\{/g) ?? []).length - (antes.match(/\}/g) ?? []).length;
  expect(abiertas).toBe(0);
  // Y el input no pinta el suyo, que sería el cuadrado.
  expect(css).toMatch(/\.task-row > input\[type='checkbox'\]:focus-visible \{\s*box-shadow: none;/);
});
