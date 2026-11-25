/**
 * Un atajo que dispara donde no debe se come caracteres que alguien escribió;
 * uno que no dispara es solo un atajo que no existe. Por eso los negativos
 * importan más que los positivos.
 */
import './html-estatico.js';
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { pegadoConFormato } from '@sytue/ui/browser/portapapeles';
import { atajoMarkdown, markdownAHtml, pareceMarkdown, queSePega } from '../client/markdown.js';

let registrado = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

test('guion, asterisco o más y espacio abren una lista', () => {
  for (const m of ['-', '*', '+'])
    expect(atajoMarkdown(m, ' ')).toEqual({ cmd: 'insertUnorderedList', borrar: 1 });
});

test('«1.» o «1)» y espacio abren una lista numerada', () => {
  expect(atajoMarkdown('1.', ' ')).toEqual({ cmd: 'insertOrderedList', borrar: 2 });
  expect(atajoMarkdown('1)', ' ')).toEqual({ cmd: 'insertOrderedList', borrar: 2 });
  expect(atajoMarkdown('2.', ' ')).toBeNull();
});

test('de una a cinco almohadillas son títulos; seis, no (el editor llega a h5)', () => {
  expect(atajoMarkdown('#', ' ')).toEqual({ cmd: 'formatBlock', valor: '<h1>', borrar: 1 });
  expect(atajoMarkdown('#####', ' ')).toEqual({ cmd: 'formatBlock', valor: '<h5>', borrar: 5 });
  expect(atajoMarkdown('######', ' ')).toBeNull();
});

test('cita y bloque de código', () => {
  expect(atajoMarkdown('>', ' ')).toEqual({ cmd: 'formatBlock', valor: '<blockquote>', borrar: 1 });
  expect(atajoMarkdown('```', ' ')).toEqual({ cmd: 'formatBlock', valor: '<pre>', borrar: 3 });
});

test('cuadros de tareas y listas de completado con espacio', () => {
  expect(atajoMarkdown('[]', ' ')?.cmd).toBe('insertHTML');
  expect(atajoMarkdown('[ ]', ' ')?.cmd).toBe('insertHTML');
  expect(atajoMarkdown('- [ ]', ' ')?.cmd).toBe('insertHTML');
});

test('tres guiones o asteriscos con Intro son una regla; con espacio, nada', () => {
  expect(atajoMarkdown('---', 'Enter')).toEqual({ cmd: 'insertHorizontalRule', borrar: 3 });
  expect(atajoMarkdown('*****', 'Enter')).toEqual({ cmd: 'insertHorizontalRule', borrar: 5 });
  expect(atajoMarkdown('--', 'Enter')).toBeNull();
  expect(atajoMarkdown('---', ' ')).toBeNull();
});

test('un marcador que no abre la línea no dispara nada', () => {
  expect(atajoMarkdown('hola -', ' ')).toBeNull();
  expect(atajoMarkdown('a#', ' ')).toBeNull();
  expect(atajoMarkdown('', ' ')).toBeNull();
  expect(atajoMarkdown('- ', ' ')).toBeNull();
});

/*
 * Pegar Markdown. Los casos son los de Fyne (`markdown_test.go`): el mismo texto
 * tiene que dar la misma nota en las dos.
 */
interface CasoMd {
  nombre: string;
  md: string;
  parece: boolean;
  html: string;
}
const CASOS: CasoMd[] = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/notes/testdata/markdown.json', import.meta.url),
    'utf8',
  ),
);

test('hay casos comunes que leer', () => {
  expect(CASOS.length).toBeGreaterThan(10);
});

for (const caso of CASOS) {
  test(`Markdown pegado: ${caso.nombre}`, () => {
    expect(pareceMarkdown(caso.md)).toBe(caso.parece);
    expect(markdownAHtml(caso.md)).toBe(caso.html);
  });
}

test('el pegado del plan sobrevive al saneado con su tarea, su título y su negrita', () => {
  const limpio = pegadoConFormato(markdownAHtml('## Plan\n- [ ] Cajas\n- **Camión**'), {
    tareas: true,
  });
  expect(limpio).toBe(
    '<h2>Plan</h2><div class="task-row"><input type="checkbox"> <span>Cajas</span></div>' +
      '<ul><li><b>Camión</b></li></ul>',
  );
});

test('lo escapado no revive al sanear y el enlace hostil se queda en texto', () => {
  const limpio = pegadoConFormato(
    markdownAHtml('# <img src=x onerror=alert(1)>\n- [x](javascript:alert(1))'),
    { tareas: true },
  );
  expect(limpio).not.toContain('<img');
  expect(limpio).not.toContain('href');
});

const portapapeles = (tipos: Record<string, string>) => ({
  getData: (t: string) => tipos[t] ?? '',
});
const PLAN = '## Plan\n- [ ] Cajas\n- **Camión**';

test('pegar Markdown como texto plano llega con formato', () => {
  const pegado = queSePega(portapapeles({ 'text/plain': PLAN }));
  expect(pegado).toEqual({
    html:
      '<h2>Plan</h2><div class="task-row"><input type="checkbox"> <span>Cajas</span></div>' +
      '<ul><li><b>Camión</b></li></ul>',
  });
});

test('pegar prosa normal no cambia: entra como texto', () => {
  const prosa = 'Hola, quedamos el martes.\nLleva el *ticket*.';
  expect(queSePega(portapapeles({ 'text/plain': prosa }))).toEqual({ texto: prosa });
});

test('Ctrl+Mayús+V pega el Markdown sin formato', () => {
  expect(queSePega(portapapeles({ 'text/plain': PLAN }), true)).toEqual({ texto: PLAN });
});

test('el `.md` coloreado de un editor de código también se convierte', () => {
  const html = `<div><span class="mtk1">## Plan</span><br><span>- [ ] Cajas</span></div>`;
  const pegado = queSePega(
    portapapeles({ 'text/plain': '## Plan\n- [ ] Cajas', 'text/html': html }),
  );
  expect('html' in pegado && pegado.html.startsWith('<h2>Plan</h2>')).toBe(true);
});

test('el HTML ya interpretado (una web, un asistente) manda sobre su texto', () => {
  const html = '<h2>Plan</h2><ul><li>uno</li><li>dos</li></ul>';
  const pegado = queSePega(portapapeles({ 'text/plain': '- uno\n- dos', 'text/html': html }));
  expect(pegado).toEqual({ html: '<h2>Plan</h2><ul><li>uno</li><li>dos</li></ul>' });
});

test('en un bloque de código el Markdown pegado se queda en letras', () => {
  expect(queSePega(portapapeles({ 'text/plain': PLAN }), false, false)).toEqual({ texto: PLAN });
  const html = `<div><span class="mtk1">## Plan</span><br><span>- [ ] Cajas</span></div>`;
  expect(
    queSePega(
      portapapeles({ 'text/plain': '## Plan\n- [ ] Cajas', 'text/html': html }),
      false,
      false,
    ),
  ).toEqual({ texto: '## Plan\n- [ ] Cajas' });
});

test('una URL sola se pega como enlace; con texto seleccionado, enlaza ese texto', () => {
  expect(queSePega(portapapeles({ 'text/plain': 'https://sytue.eu/precios' }))).toEqual({
    html: '<a href="https://sytue.eu/precios">https://sytue.eu/precios</a>',
  });
  expect(
    queSePega(portapapeles({ 'text/plain': ' https://sytue.eu/ ' }), false, true, 'la web'),
  ).toEqual({ html: '<a href="https://sytue.eu/">la web</a>' });
  // El `<a>` que copian algunos navegadores con la misma URL dentro, igual.
  const html = '<a href="https://sytue.eu/x">https://sytue.eu/x</a>';
  expect(
    queSePega(portapapeles({ 'text/plain': 'https://sytue.eu/x', 'text/html': html })),
  ).toEqual({ html: '<a href="https://sytue.eu/x">https://sytue.eu/x</a>' });
});

test('lo que no es una URL sola no se convierte en enlace', () => {
  for (const texto of ['mira https://sytue.eu', 'javascript:alert(1)', 'sytue.eu', 'ftp://x.y/'])
    expect(queSePega(portapapeles({ 'text/plain': texto }))).toEqual({ texto });
  // Con Mayúsculas, o dentro de un bloque de código, la URL es texto.
  expect(queSePega(portapapeles({ 'text/plain': 'https://sytue.eu/' }), true)).toEqual({
    texto: 'https://sytue.eu/',
  });
  expect(queSePega(portapapeles({ 'text/plain': 'https://sytue.eu/' }), false, false)).toEqual({
    texto: 'https://sytue.eu/',
  });
});

test('lo copiado de otra nota conserva sus bloques, y nada que no esté en sus listas', () => {
  const html =
    '<div data-panel="info" onclick="x()"><p>a</p></div><span data-estado="rojo">NO</span>' +
    '<div class="task-row"><input type="checkbox" checked> <span>t</span></div>';
  expect(queSePega(portapapeles({ 'text/plain': 'a NO t', 'text/html': html }))).toEqual({
    html:
      '<div data-panel="info"><p>a</p></div><span data-estado="rojo">NO</span>' +
      '<div class="task-row"><input type="checkbox" checked=""> <span>t</span></div>',
  });
});
