/**
 * Tests de SelectorDeColor — su contrato: un radiogroup con nombres de color
 * (nunca el hex), una sola parada de Tab, flechas/Inicio/Fin que eligen y mueven
 * el foco, y el elegido marcado con visto. Y un guardián: ningún `aria-label`
 * con forma `#rrggbb` en el código de las apps. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { render } from 'preact';
import type { VNode } from 'preact';
import { SelectorDeColor } from './SelectorDeColor.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const PALETA = [
  { valor: '', nombre: 'Sin color' },
  { valor: '#b0413a', nombre: 'Rojo' },
  { valor: '#16794a', nombre: 'Verde' },
  { valor: '#1c58d9', nombre: 'Azul' },
];

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

function radios(host: HTMLElement): HTMLButtonElement[] {
  return Array.from(host.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
}

function tecla(el: HTMLElement, key: string, extra: KeyboardEventInit = {}): KeyboardEvent {
  const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...extra });
  el.dispatchEvent(ev);
  return ev;
}

test('es un radiogroup con nombre y cada tono se anuncia por su nombre', () => {
  const host = mount(
    <SelectorDeColor colores={PALETA} valor="#16794A" onChange={() => {}} aria-label="Color" />,
  );
  const grupo = host.querySelector('[role="radiogroup"]')!;
  expect(grupo.getAttribute('aria-label')).toBe('Color');
  const rs = radios(host);
  expect(rs.map((r) => r.getAttribute('aria-label'))).toEqual([
    'Sin color',
    'Rojo',
    'Verde',
    'Azul',
  ]);
  // La comparación ignora mayúsculas: el servidor puede devolver el hex en otra caja.
  expect(rs.map((r) => r.getAttribute('aria-checked'))).toEqual([
    'false',
    'false',
    'true',
    'false',
  ]);
  expect(rs[2]!.querySelector('svg')).not.toBeNull();
  expect(rs.map((r) => r.tabIndex)).toEqual([-1, -1, 0, -1]);
  render(null, host);
});

test('fuera de paleta, la primera muestra es la parada de Tab y no hay ninguna marcada', () => {
  const host = mount(<SelectorDeColor colores={PALETA} valor="#123456" onChange={() => {}} />);
  const rs = radios(host);
  expect(rs.map((r) => r.tabIndex)).toEqual([0, -1, -1, -1]);
  expect(rs.some((r) => r.getAttribute('aria-checked') === 'true')).toBe(false);
  render(null, host);
});

test('las flechas, Inicio y Fin eligen y llevan el foco, dando la vuelta', () => {
  let valor = '#1c58d9';
  const host = document.createElement('div');
  document.body.appendChild(host);
  const pinta = (): void =>
    render(
      <SelectorDeColor
        colores={PALETA}
        valor={valor}
        onChange={(v) => {
          valor = v;
          pinta();
        }}
      />,
      host,
    );
  pinta();
  const grupo = host.querySelector<HTMLElement>('[role="radiogroup"]')!;
  tecla(grupo, 'ArrowRight');
  expect(valor).toBe('');
  expect(document.activeElement).toBe(radios(host)[0]!);
  tecla(grupo, 'ArrowLeft');
  expect(valor).toBe('#1c58d9');
  tecla(grupo, 'ArrowUp');
  expect(valor).toBe('#16794a');
  tecla(grupo, 'Home');
  expect(valor).toBe('');
  tecla(grupo, 'End');
  expect(valor).toBe('#1c58d9');
  expect(document.activeElement).toBe(radios(host)[3]!);
  render(null, host);
});

test('con Alt, Ctrl o Cmd la flecha es del navegador: ni elige ni se cancela', () => {
  let valor = '#16794a';
  const host = mount(
    <SelectorDeColor colores={PALETA} valor={valor} onChange={(v) => (valor = v)} />,
  );
  const grupo = host.querySelector<HTMLElement>('[role="radiogroup"]')!;
  for (const mod of [{ altKey: true }, { ctrlKey: true }, { metaKey: true }]) {
    expect(tecla(grupo, 'ArrowLeft', mod).defaultPrevented).toBe(false);
  }
  expect(valor).toBe('#16794a');
  render(null, host);
});

test('el clic elige y `pintar` traduce tokens a color', () => {
  let elegido = '';
  const host = mount(
    <SelectorDeColor
      colores={[{ valor: 'sage', nombre: 'Salvia' }]}
      valor=""
      pintar={(v) => `var(--x-${v})`}
      onChange={(v) => (elegido = v)}
    />,
  );
  const r = radios(host)[0]!;
  expect(r.getAttribute('style') ?? '').toContain('var(--x-sage)');
  r.click();
  expect(elegido).toBe('sage');
  render(null, host);
});

// Guardián: el hex no es un nombre. Recorre el código de las apps y paquetes.
test('ningún aria-label de la suite tiene forma #rrggbb ni es una variable de color suelta', () => {
  const raiz = join(import.meta.dir, '..', '..', '..', '..');
  const malos: string[] = [];
  // En las apps, además, ni `aria-label={c}` ni `{color}`: son paletas de usuario
  // con nombre. La ofimática sí rotula colores sacados del propio documento
  // (filtros por color de celda), que no tienen nombre: ahí solo se veta el literal.
  const recorrer = (dir: string, estricto: boolean): void => {
    // Las entradas del directorio ya traen su tipo: sin un `stat` aparte que pueda
    // quedar desfasado respecto de la lectura posterior (carrera comprobar/usar).
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const n = e.name;
      if (n === 'node_modules' || n === 'dist' || n.startsWith('.')) continue;
      const p = join(dir, n);
      if (e.isDirectory()) recorrer(p, estricto);
      else if (e.isFile() && n.endsWith('.tsx') && !n.endsWith('.test.tsx')) {
        const s = readFileSync(p, 'utf8');
        const literal = /aria-label=\{?['"`]#[0-9a-f]{3,8}/i.test(s);
        const variable = /aria-label=\{(c|color)\}/.test(s);
        if (literal || (estricto && variable)) malos.push(p);
      }
    }
  };
  recorrer(join(raiz, 'apps'), true);
  recorrer(join(raiz, 'packages'), false);
  expect(malos).toEqual([]);
});
