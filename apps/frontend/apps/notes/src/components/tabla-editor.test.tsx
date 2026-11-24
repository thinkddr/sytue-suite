/*
 * LA TABLA EN EL EDITOR: su barra flotante sale con el cursor dentro (y se va al salir),
 * agrupada y con ayuda en cada botón; Tab va de celda en celda y en la última añade una
 * fila; el botón derecho abre el menú de la celda. El HTML es el compartido con Fyne.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { olvidarMedidas } from './BarraConMas.js';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  pulsar,
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
});
afterEach(() => {
  editor?.desmontar();
  editor = null;
  edicion.quitar();
});

const barraDeTabla = () =>
  document.querySelector<HTMLElement>('[role="toolbar"][aria-label="editor.tableTools"]');

/** La celda donde está el cursor. */
const celdaDelCursor = () => {
  const n = document.getSelection()?.anchorNode ?? null;
  return (n?.nodeType === Node.TEXT_NODE ? n.parentElement : (n as Element | null))?.closest(
    'td, th',
  );
};

async function conTabla(): Promise<EditorMontado> {
  const e = await montarEditor('<p><br></p>');
  boton(e.host, 'editor.table').click();
  await tick();
  return e;
}

test('insertar una tabla la deja con su fila de cabecera y el cursor en la primera celda', async () => {
  editor = await conTabla();
  const tabla = editor.campo.querySelector('table')!;
  expect(tabla.querySelectorAll('thead th').length).toBe(3);
  expect(tabla.querySelector('thead th')!.hasAttribute('scope')).toBe(false);
  expect(tabla.querySelectorAll('tbody tr').length).toBe(2);
  // Sin estilos en línea: los bordes los pinta la prosa, en claro y en oscuro.
  expect(tabla.outerHTML).not.toContain('style=');
  expect(celdaDelCursor()).toBe(tabla.querySelector('th'));
});

test('la barra de la tabla sale dentro de ella, agrupada y con ayuda en cada botón', async () => {
  editor = await conTabla();
  const barra = barraDeTabla()!;
  expect(barra).not.toBeNull();
  const etiquetas = [...barra.querySelectorAll('button')].map((b) => b.getAttribute('aria-label'));
  for (const e of [
    'editor.headerRow',
    'editor.headerColumn',
    'editor.numberedRows',
    'editor.cellBackground',
    'editor.rowAbove',
    'editor.rowBelow',
    'editor.colBefore',
    'editor.colAfter',
    'editor.rowDelete',
    'editor.colDelete',
    'editor.tableDelete',
  ])
    expect(etiquetas).toContain(e);
  // Grupos separados por un filete, y la cabecera de fila dice que está puesta.
  expect(barra.querySelectorAll('[aria-hidden="true"].bg-border').length).toBeGreaterThan(1);
  expect(boton(barra, 'editor.headerRow').getAttribute('aria-pressed')).toBe('true');
  // Fuera de la tabla, la barra se va.
  const p = editor.campo.querySelector('p')!;
  document.getSelection()!.collapse(p, 0);
  editor.campo.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
  await tick();
  expect(barraDeTabla()).toBeNull();
});

test('cabecera de columna, filas numeradas y fondo de celda escriben su HTML', async () => {
  editor = await conTabla();
  // A una celda del cuerpo.
  const td = editor.campo.querySelector('tbody td')!;
  document.getSelection()!.collapse(td, 0);
  editor.campo.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
  await tick();
  boton(barraDeTabla()!, 'editor.headerColumn').click();
  await tick();
  expect(editor.campo.querySelectorAll('tbody th[scope="row"]').length).toBe(2);
  boton(barraDeTabla()!, 'editor.numberedRows').click();
  await tick();
  expect(editor.campo.querySelector('table')!.hasAttribute('data-numerada')).toBe(true);
  // Fondo: en la celda del cursor, con el nombre del token.
  const celda = editor.campo.querySelector('tbody td')!;
  document.getSelection()!.collapse(celda, 0);
  editor.campo.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
  await tick();
  boton(barraDeTabla()!, 'editor.cellBackground').click();
  await tick();
  boton(document.querySelector('[role="dialog"]')!, 'editor.statusBlue').click();
  await tick();
  expect(editor.campo.querySelector('tbody td')!.getAttribute('data-fondo')).toBe('azul');
  expect(editor.cambios.at(-1)).toContain('data-fondo="azul"');
});

test('Tab va a la celda siguiente, Mayús+Tab a la anterior y en la última añade una fila', async () => {
  editor = await conTabla();
  const celdas = () => [...editor!.campo.querySelectorAll('th, td')];
  expect(pulsar(editor.campo, 'Tab')).toBe(true);
  expect(celdaDelCursor()).toBe(celdas()[1]);
  pulsar(editor.campo, 'Tab', { shift: true });
  expect(celdaDelCursor()).toBe(celdas()[0]);
  // A la última y un Tab más: fila nueva, con el cursor en su primera celda.
  const ultima = celdas().at(-1)!;
  document.getSelection()!.collapse(ultima, 0);
  pulsar(editor.campo, 'Tab');
  await tick();
  expect(editor.campo.querySelectorAll('tbody tr').length).toBe(3);
  expect(celdaDelCursor()).toBe(editor.campo.querySelector('tbody tr:last-child td'));
  // Lo que añade el módulo común no lleva los bordes en línea del correo.
  expect(editor.campo.querySelector('tbody tr:last-child td')!.hasAttribute('style')).toBe(false);
});

test('el botón derecho en una celda abre su menú, con las acciones de la tabla', async () => {
  editor = await conTabla();
  const td = editor.campo.querySelector('tbody td')!;
  const e = new MouseEvent('contextmenu', {
    bubbles: true,
    cancelable: true,
    clientX: 50,
    clientY: 50,
  });
  td.dispatchEvent(e);
  await tick();
  expect(e.defaultPrevented).toBe(true);
  const menu = document.querySelector<HTMLElement>(
    '[role="menu"][aria-label="editor.tableTools"]',
  )!;
  expect(menu).not.toBeNull();
  boton(menu, 'editor.rowBelow').click();
  await tick();
  expect(editor.campo.querySelectorAll('tbody tr').length).toBe(3);
});
