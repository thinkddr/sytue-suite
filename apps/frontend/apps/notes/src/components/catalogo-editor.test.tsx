/*
 * EL CATÁLOGO DE LO QUE SE INSERTA: «/», el «+» de la barra, «:» para los emoji y los
 * símbolos. Se mira que el menú filtre, que se lleve con el teclado (↑ ↓ Intro Esc), que
 * inserte donde está el cursor quitando lo tecleado, y que lo insertado sea el HTML de
 * los bloques compartido con Fyne. El `execCommand` es el de juguete
 * (edicion-de-prueba.tsx): el DOM de las pruebas no lo tiene.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { CLAVE_RECIENTES } from '../client/simbolos.js';
import { olvidarMedidas } from './BarraConMas.js';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  pulsar,
  teclear,
  tick,
  type EdicionDePrueba,
  type EditorMontado,
} from './edicion-de-prueba.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let edicion: EdicionDePrueba;
let editor: EditorMontado | null = null;
beforeEach(() => {
  olvidarMedidas();
  edicion = instalarEdicionDePrueba();
  try {
    localStorage.removeItem(CLAVE_RECIENTES);
  } catch {
    // sin almacenamiento: los recientes empiezan vacíos igual
  }
});
afterEach(() => {
  editor?.desmontar();
  editor = null;
  edicion.quitar();
});

const opciones = () => [
  ...document.querySelectorAll<HTMLElement>('#notes-menu-sugerencias [role="option"]'),
];
const rotulos = () => opciones().map((o) => o.querySelector('.block')?.textContent);

test('«/» filtra por nombre, alias o descripción, y Escape lo cierra sin salir', async () => {
  editor = await montarEditor('<p><br></p>');
  await teclear(editor.campo, '/');
  // Sin consulta, todo el catálogo, con su descripción debajo del nombre.
  expect(rotulos()).toContain('editor.panelInfo');
  expect(opciones()[0]!.textContent).toContain('editor.paragraphDesc');
  await teclear(editor.campo, 'panel');
  expect(rotulos()).toEqual([
    'editor.panelInfo',
    'editor.panelNote',
    'editor.panelSuccess',
    'editor.panelWarning',
    'editor.panelError',
  ]);
  pulsar(editor.campo, 'Escape');
  await tick();
  expect(opciones()).toEqual([]);
});

test('«/» sin nada que case lo dice', async () => {
  editor = await montarEditor('<p><br></p>');
  await teclear(editor.campo, '/zzz');
  expect(document.querySelector('#notes-menu-sugerencias')?.parentElement?.textContent).toContain(
    'editor.slashNoResults',
  );
});

test('↓ e Intro insertan el elegido donde estaba el cursor, sin el «/consulta»', async () => {
  editor = await montarEditor('<p>Plan <br></p>');
  await teclear(editor.campo, '/panel');
  pulsar(editor.campo, 'ArrowDown'); // del panel de información al de nota
  await tick();
  expect(opciones()[1]!.getAttribute('aria-selected')).toBe('true');
  expect(editor.campo.getAttribute('aria-activedescendant')).toBe('notes-sug-1');
  expect(pulsar(editor.campo, 'Enter')).toBe(true);
  await tick();
  expect(editor.campo.innerHTML).not.toContain('/panel');
  const panel = editor.campo.querySelector('[data-panel]');
  expect(panel?.getAttribute('data-panel')).toBe('nota');
  // La marca de un instante se ha ido, y lo entregado es lo que guarda el saneador.
  expect(panel?.hasAttribute('data-nuevo')).toBe(false);
  expect(editor.cambios.at(-1)).toContain('<div data-panel="nota">');
});

test('los bloques nuevos escriben el HTML compartido', async () => {
  const casos: [string, string][] = [
    ['decis', '<ul data-tipo="decisiones">'],
    ['expand', '<details data-expandir="" open=""><summary>editor.expand</summary>'],
    ['status', '<span data-estado="gris">EDITOR.STATUS</span>'],
    ['columns', '<div data-columnas="2"><div data-columna="">'],
  ];
  for (const [consulta, html] of casos) {
    editor = await montarEditor('<p><br></p>');
    await teclear(editor.campo, `/${consulta}`);
    pulsar(editor.campo, 'Enter');
    await tick();
    expect(editor.campo.innerHTML).toContain(html);
    editor.desmontar();
    editor = null;
  }
});

test('«Hoy» pone la fecha de hoy como <time datetime>', async () => {
  editor = await montarEditor('<p><br></p>');
  await teclear(editor.campo, '/today');
  pulsar(editor.campo, 'Enter');
  await tick();
  const fecha = editor.campo.querySelector('time');
  expect(fecha?.getAttribute('datetime')).toMatch(/^\d{4}-\d{2}-\d{2}$/);
});

test('el «+» de la barra busca en el mismo catálogo y se lleva con el teclado', async () => {
  editor = await montarEditor('<p><br></p>');
  const mas = [...editor.host.querySelectorAll<HTMLElement>('button')].find((b) =>
    b.getAttribute('aria-label')?.startsWith('editor.shortcut{"label":"editor.insert"'),
  )!;
  mas.click();
  await tick();
  const buscar = document.querySelector<HTMLInputElement>(
    '[role="dialog"] input[role="combobox"]',
  )!;
  expect(document.activeElement).toBe(buscar);
  buscar.value = 'decision';
  buscar.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
  const lista = [...document.querySelectorAll('#notes-insertar-lista [role="option"]')];
  expect(lista.map((o) => o.textContent)).toEqual(['editor.decisioneditor.decisionDesc']);
  expect(pulsar(buscar, 'Enter')).toBe(true);
  await tick(10);
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(editor.campo.innerHTML).toContain('<ul data-tipo="decisiones">');
});

test('«:» y dos letras ofrecen emoji, e Intro cambia «:fueg» por el emoji', async () => {
  editor = await montarEditor('<p>Hoy <br></p>');
  await teclear(editor.campo, ':fueg');
  expect(document.querySelector('#notes-menu-sugerencias')?.getAttribute('aria-label')).toBe(
    'editor.emoji',
  );
  expect(opciones()[0]!.textContent).toContain('🔥');
  pulsar(editor.campo, 'Enter');
  await tick();
  expect(editor.campo.textContent).toContain('🔥');
  expect(editor.campo.textContent).not.toContain(':fueg');
});

test('los símbolos se insertan, se recuerdan y se recorren con las flechas', async () => {
  editor = await montarEditor('<p>a<br></p>');
  boton(editor.host, 'editor.symbols').click();
  await tick();
  const panel = document.querySelector<HTMLElement>('[role="dialog"]')!;
  expect(panel.getAttribute('aria-label')).toBe('editor.symbols');
  // Sin recientes, «Recientes» enseña los frecuentes; las categorías van detrás.
  const grupos = [...panel.querySelectorAll('[role="group"]')].map((g) =>
    g.getAttribute('aria-labelledby'),
  );
  expect(grupos).toEqual([
    'notes-simbolos-recientes',
    'notes-simbolos-flechas',
    'notes-simbolos-matematicas',
    'notes-simbolos-moneda',
    'notes-simbolos-tipografia',
    'notes-simbolos-marcas',
    'notes-simbolos-emoji',
  ]);
  // ↓ desde la búsqueda baja a la rejilla; → avanza uno.
  const buscar = panel.querySelector('input')!;
  pulsar(buscar, 'ArrowDown');
  await tick();
  expect((document.activeElement as HTMLElement).dataset['i']).toBe('0');
  pulsar(document.activeElement!, 'ArrowRight');
  await tick();
  expect((document.activeElement as HTMLElement).dataset['i']).toBe('1');
  const flecha = [
    ...panel.querySelectorAll<HTMLElement>('[aria-labelledby="notes-simbolos-flechas"] button'),
  ].find((b) => b.textContent === '⇒')!;
  flecha.click();
  await tick(10);
  expect(edicion.ordenes.some(([o, v]) => o === 'insertText' && v === '⇒')).toBe(true);
  expect(editor.campo.textContent).toContain('⇒');
  expect(JSON.parse(localStorage.getItem(CLAVE_RECIENTES) ?? '[]')).toEqual(['⇒']);
  // Al volver, el último usado encabeza «Recientes».
  boton(editor.host, 'editor.symbols').click();
  await tick();
  const recientes = document.querySelector('[aria-labelledby="notes-simbolos-recientes"]');
  expect(recientes?.textContent).toBe('⇒');
});

test('la búsqueda de símbolos encuentra por palabra, en castellano o en inglés', async () => {
  editor = await montarEditor('<p>a<br></p>');
  boton(editor.host, 'editor.symbols').click();
  await tick();
  const buscar = document.querySelector<HTMLInputElement>('[role="dialog"] input')!;
  for (const [q, s] of [
    ['euro', '€'],
    ['infinito', '∞'],
    ['arrow right', '→'],
  ] as const) {
    buscar.value = q;
    buscar.dispatchEvent(new Event('input', { bubbles: true }));
    await tick();
    expect(document.querySelector('[role="dialog"] [role="group"]')?.textContent).toContain(s);
  }
});

test('en el título de una sección, Espacio escribe e Intro baja al contenido', async () => {
  editor = await montarEditor(
    '<details data-expandir open><summary>Plan</summary><p>dentro</p></details>',
    {},
    (campo) => [campo.querySelector('summary')!.firstChild!, 4],
  );
  // Espacio: un espacio de verdad, y la sección sigue abierta.
  expect(pulsar(editor.campo, ' ')).toBe(true);
  expect(edicion.ordenes.some(([o, v]) => o === 'insertText' && v === ' ')).toBe(true);
  const seccion = editor.campo.querySelector('details')!;
  expect(seccion.open).toBe(true);
  // Intro: al primer párrafo del contenido.
  document.getSelection()!.collapse(editor.campo.querySelector('summary')!.firstChild!, 2);
  expect(pulsar(editor.campo, 'Enter')).toBe(true);
  const ancla = document.getSelection()!.anchorNode;
  expect((ancla as Element).closest?.('p') ?? ancla?.parentElement?.closest('p')).toBe(
    editor.campo.querySelector('details p'),
  );
});
