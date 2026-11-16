/**
 * Los atajos de Drive salen de UNA tabla (`atajos.ts`): lo que hace cada tecla y lo
 * que enseña la hoja de «?». Aquí se vigila que cada tecla de la hoja tiene acción,
 * que ninguna salta escribiendo, con un diálogo o un menú abiertos, que sin red se
 * apagan las que escriben y que Ctrl+A no le roba al buscador su «seleccionar todo».
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { accionDeEvento, gruposDeAtajos, teclaDeEvento } from './atajos';
import { useT } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

afterEach(() => {
  document.body.innerHTML = '';
});

/** Un `keydown` despachado sobre `destino` (por defecto, el cuerpo de la página). */
function pulsa(
  key: string,
  mods: Partial<Pick<KeyboardEventInit, 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>> = {},
  destino: HTMLElement = document.body,
): KeyboardEvent {
  let capturado: KeyboardEvent | null = null;
  const coger = (e: Event): void => {
    capturado = e as KeyboardEvent;
  };
  destino.addEventListener('keydown', coger);
  destino.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...mods }));
  destino.removeEventListener('keydown', coger);
  return capturado!;
}

const enLinea = { offline: false };

describe('atajos de Drive', () => {
  test('cada tecla de la tabla hace lo que dice la hoja', () => {
    expect(accionDeEvento(pulsa('/'), enLinea)).toBe('search');
    expect(accionDeEvento(pulsa('u'), enLinea)).toBe('upload');
    expect(accionDeEvento(pulsa('n'), enLinea)).toBe('newFolder');
    expect(accionDeEvento(pulsa('Delete'), enLinea)).toBe('trash');
    expect(accionDeEvento(pulsa('Backspace'), enLinea)).toBe('trash');
    expect(accionDeEvento(pulsa('F2'), enLinea)).toBe('rename');
    expect(accionDeEvento(pulsa('a', { ctrlKey: true }), enLinea)).toBe('selectAll');
    expect(accionDeEvento(pulsa('a', { metaKey: true }), enLinea)).toBe('selectAll');
    expect(accionDeEvento(pulsa('Escape'), enLinea)).toBe('clearSelection');
    expect(accionDeEvento(pulsa('v'), enLinea)).toBe('toggleView');
    expect(accionDeEvento(pulsa('V', { shiftKey: true }), enLinea)).toBe('toggleView');
    expect(accionDeEvento(pulsa('?', { shiftKey: true }), enLinea)).toBe('help');
  });

  test('lo que no es un atajo, o lleva un modificador del sistema, no hace nada', () => {
    expect(accionDeEvento(pulsa('x'), enLinea)).toBeNull();
    expect(accionDeEvento(pulsa('Enter'), enLinea)).toBeNull();
    // Sin Ctrl, «a» no es nada; con ⌥, «u» es una diéresis; ⌘N es ventana nueva.
    expect(accionDeEvento(pulsa('a'), enLinea)).toBeNull();
    expect(accionDeEvento(pulsa('u', { altKey: true }), enLinea)).toBeNull();
    expect(accionDeEvento(pulsa('n', { metaKey: true }), enLinea)).toBeNull();
    expect(accionDeEvento(pulsa('a', { ctrlKey: true, shiftKey: true }), enLinea)).toBeNull();
    expect(teclaDeEvento(pulsa('A', { ctrlKey: true }))).toBe('Ctrl+a');
  });

  test('ninguna salta escribiendo: Ctrl+A en el buscador sigue marcando su texto', () => {
    const buscador = document.createElement('input');
    buscador.type = 'search';
    document.body.append(buscador);
    for (const [k, mods] of [
      ['a', { ctrlKey: true }],
      ['/', {}],
      ['Delete', {}],
      ['Backspace', {}],
      ['v', {}],
      ['?', { shiftKey: true }],
    ] as const) {
      expect(accionDeEvento(pulsa(k, mods, buscador), enLinea)).toBeNull();
    }
    const editable = document.createElement('div');
    editable.contentEditable = 'true';
    document.body.append(editable);
    expect(accionDeEvento(pulsa('F2', {}, editable), enLinea)).toBeNull();
    // Una casilla de selección con el foco NO es un campo: tras marcar con el ratón,
    // Supr tiene que seguir tirando lo marcado.
    const casilla = document.createElement('input');
    casilla.type = 'checkbox';
    document.body.append(casilla);
    expect(accionDeEvento(pulsa('Delete', {}, casilla), enLinea)).toBe('trash');
  });

  test('con un diálogo o un menú abiertos, el teclado es suyo', () => {
    for (const rol of ['dialog', 'menu']) {
      const capa = document.createElement('div');
      capa.setAttribute('role', rol);
      document.body.append(capa);
      expect(accionDeEvento(pulsa('Delete'), enLinea)).toBeNull();
      expect(accionDeEvento(pulsa('a', { ctrlKey: true }), enLinea)).toBeNull();
      expect(accionDeEvento(pulsa('?', { shiftKey: true }), enLinea)).toBeNull();
      capa.remove();
    }
  });

  test('sin red se apagan las que escriben; las de mirar siguen', () => {
    const sinRed = { offline: true };
    for (const k of ['u', 'n', 'Delete', 'Backspace', 'F2']) {
      expect(accionDeEvento(pulsa(k), sinRed)).toBeNull();
    }
    // Sin red tampoco hay casillas que marcar.
    expect(accionDeEvento(pulsa('a', { ctrlKey: true }), sinRed)).toBeNull();
    expect(accionDeEvento(pulsa('/'), sinRed)).toBe('search');
    expect(accionDeEvento(pulsa('v'), sinRed)).toBe('toggleView');
    expect(accionDeEvento(pulsa('?', { shiftKey: true }), sinRed)).toBe('help');
  });

  test('la hoja enseña cada tecla, traducida y con las del sistema', () => {
    const t = useT('es');
    const pc = gruposDeAtajos(t, false);
    expect(pc.map((g) => g.titulo)).toEqual(['Acciones', 'Selección', 'Vista']);
    const teclasPc = pc.flatMap((g) => g.atajos.flatMap((a) => a.teclas));
    expect(teclasPc).toEqual(['u', 'n', 'Supr', 'F2', 'Ctrl+A', 'Esc', '/', 'v', '?']);
    const ayuda = pc.flatMap((g) => g.atajos).find((a) => a.teclas.includes('?'));
    expect(ayuda?.descripcion).toBe('Ver esta lista');

    const mac = gruposDeAtajos(t, true).flatMap((g) => g.atajos.flatMap((a) => a.teclas));
    expect(mac).toContain('⌘A');
    expect(mac).toContain('⌫');
    expect(mac).not.toContain('Supr');

    // Ninguna acción se queda sin explicar ni sin texto en otro idioma.
    const de = gruposDeAtajos(useT('de'), false).flatMap((g) => g.atajos);
    expect(de.length).toBe(9);
    expect(de.find((a) => a.teclas.includes('Entf'))?.descripcion).toBe(
      'Auswahl in den Papierkorb verschieben',
    );
  });

  test('FileBrowser escucha la tabla y pinta la hoja (y no queda el oyente suelto de «u»/«n»)', () => {
    const fuente = readFileSync(new URL('../components/FileBrowser.tsx', import.meta.url), 'utf8');
    expect(fuente).toContain('accionDeEvento(e, { offline })');
    expect(fuente).toContain('groups={gruposDeAtajos(t)}');
    expect(fuente).not.toContain("e.key === 'u'");
  });
});
