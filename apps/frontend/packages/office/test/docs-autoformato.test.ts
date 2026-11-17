/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Autoformato Markdown al escribir y listas de casillas del folio (D-DOCU-04): las reglas
 * puras, y su aplicación sobre un folio de verdad con un execCommand de mentira que hace
 * lo mínimo de cada comando (happy-dom no lo trae) y apunta qué se le pidió, para ver que
 * el autoformato va por pasos que el deshacer del navegador conoce.
 *   cd apps/frontend && bun test packages/office/test/docs-autoformato.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, describe, expect, test } from 'bun:test';
import type { Editar } from '../src/editor/canvas/autoformato';

if (typeof document === 'undefined') GlobalRegistrator.register();

const {
  autoformatear,
  autoformatoActivo,
  deshacerAutoformato,
  fijarAutoformato,
  hayAutoformatoPendiente,
  olvidarAutoformato,
  reglaDeBloque,
  reglaEnLinea,
} = await import('../src/editor/canvas/autoformato');
const {
  alternarCasillasDeLaSeleccion,
  casillaDelClic,
  dejarSeleccionEnCasillas,
  marcarComoCasillas,
  trasNuevoElemento,
} = await import('../src/editor/canvas/casillas');
const { exec, getFormatState, setEditorRoot } = await import('../src/editor/canvas/exec');

describe('reglas de bloque', () => {
  test('títulos, cita, viñetas, numerada y casillas', () => {
    expect(reglaDeBloque('#')).toEqual({ tipo: 'titulo', nivel: 1 });
    expect(reglaDeBloque('###')).toEqual({ tipo: 'titulo', nivel: 3 });
    expect(reglaDeBloque('>')).toEqual({ tipo: 'cita' });
    for (const m of ['-', '*', '+']) expect(reglaDeBloque(m)).toEqual({ tipo: 'vinetas' });
    expect(reglaDeBloque('1.')).toEqual({ tipo: 'numerada', inicio: 1 });
    expect(reglaDeBloque('4)')).toEqual({ tipo: 'numerada', inicio: 4 });
    expect(reglaDeBloque('[]')).toEqual({ tipo: 'casillas', hecha: false });
    expect(reglaDeBloque('[ ]')).toEqual({ tipo: 'casillas', hecha: false });
    expect(reglaDeBloque('[ ]')).toEqual({ tipo: 'casillas', hecha: false });
    expect(reglaDeBloque('[x]')).toEqual({ tipo: 'casillas', hecha: true });
  });

  test('lo demás no es marca', () => {
    for (const s of ['', '####', '#a', ' #', 'a -', '1', '1.5', '1234.', '[y]', '--']) {
      expect(reglaDeBloque(s)).toBeNull();
    }
  });

  test('en una viñeta solo vale pasarla a casillas', () => {
    expect(reglaDeBloque('#', true)).toBeNull();
    expect(reglaDeBloque('-', true)).toBeNull();
    expect(reglaDeBloque('[ ]', true)).toEqual({ tipo: 'casillas', hecha: false });
  });
});

describe('reglas en línea', () => {
  test('negrita, cursiva y tachado al cerrar la marca', () => {
    expect(reglaEnLinea('di **hola**')).toEqual({ formato: 'bold', desde: 3, texto: 'hola' });
    expect(reglaEnLinea('__dos palabras__')).toEqual({
      formato: 'bold',
      desde: 0,
      texto: 'dos palabras',
    });
    expect(reglaEnLinea('un *poco*')).toEqual({ formato: 'italic', desde: 3, texto: 'poco' });
    expect(reglaEnLinea('(_así_')).toEqual({ formato: 'italic', desde: 1, texto: 'así' });
    expect(reglaEnLinea('~~no~~')).toEqual({ formato: 'strikeThrough', desde: 0, texto: 'no' });
  });

  test('lo que no se cierra o va pegado a una palabra no cambia', () => {
    for (const s of [
      '**hola*', // la negrita a medio cerrar no es una cursiva
      'snake_case_var_',
      'mi_variable_',
      '2*3*',
      '* hola*',
      '*hola *',
      '**',
      'hola*',
    ]) {
      expect(reglaEnLinea(s)).toBeNull();
    }
  });
});

/* ---------- Sobre el folio ---------- */

/** execCommand de mentira: lo mínimo de cada comando, y la lista de lo pedido. */
function editorFalso(pedidos: string[]): Editar {
  return (comando, valor) => {
    pedidos.push(valor === undefined ? comando : `${comando} ${valor}`);
    const sel = document.getSelection()!;
    const r = sel.getRangeAt(0);
    const bloque = (): Element =>
      (r.startContainer instanceof Element
        ? r.startContainer
        : r.startContainer.parentElement)!.closest('p,div,li,h1,h2,h3,blockquote')!;
    switch (comando) {
      case 'insertText': {
        const n = r.startContainer;
        if (n instanceof Text) {
          n.insertData(r.startOffset, valor!);
          sel.collapse(n, r.startOffset + valor!.length);
        } else {
          const t = document.createTextNode(valor!);
          r.insertNode(t);
          sel.collapse(t, t.length);
        }
        return true;
      }
      case 'delete':
        r.deleteContents();
        sel.collapse(r.startContainer, r.startOffset);
        return true;
      case 'formatBlock': {
        const viejo = bloque();
        const nuevo = document.createElement(valor!.replace(/[<>]/g, ''));
        nuevo.append(...Array.from(viejo.childNodes));
        viejo.replaceWith(nuevo);
        sel.collapse(nuevo, 0);
        return true;
      }
      case 'insertUnorderedList':
      case 'insertOrderedList': {
        const tag = comando === 'insertOrderedList' ? 'OL' : 'UL';
        const viejo = bloque();
        const li = document.createElement('li');
        li.append(...Array.from(viejo.childNodes));
        const previo = viejo.previousElementSibling;
        // Como el navegador: se junta con la lista de arriba si es del mismo tipo.
        if (previo?.tagName === tag) {
          previo.append(li);
          viejo.remove();
        } else {
          const lista = document.createElement(tag.toLowerCase());
          lista.append(li);
          viejo.replaceWith(lista);
        }
        sel.collapse(li, 0);
        return true;
      }
      case 'insertHTML': {
        r.deleteContents();
        const plantilla = document.createElement('template');
        plantilla.innerHTML = valor!;
        const nodos = Array.from(plantilla.content.childNodes);
        r.insertNode(plantilla.content);
        const ultimo = nodos[nodos.length - 1]!;
        sel.collapse(
          ultimo.parentNode!,
          Array.from(ultimo.parentNode!.childNodes).indexOf(ultimo as ChildNode) + 1,
        );
        return true;
      }
      default:
        return true; // undo, bold… solo se apuntan
    }
  };
}

let folio: HTMLElement;
function montar(html: string): HTMLElement {
  folio?.remove();
  folio = document.createElement('div');
  folio.className = 'sytue-page';
  folio.innerHTML = html;
  document.body.append(folio);
  return folio;
}
afterEach(() => olvidarAutoformato());

/** Pone el cursor al final del primer texto que contiene `texto`. */
function cursorTras(texto: string): void {
  const w = document.createTreeWalker(folio, NodeFilter.SHOW_TEXT);
  for (let n = w.nextNode(); n; n = w.nextNode()) {
    const t = n as Text;
    const i = t.data.indexOf(texto);
    if (i >= 0) {
      document.getSelection()!.collapse(t, i + texto.length);
      return;
    }
  }
  throw new Error(`sin «${texto}» en el folio`);
}

/** Teclea `tecla` por el autoformato; si no lo coge, la inserta como el navegador. */
function teclear(tecla: string, editar: Editar, enFormato = true): boolean {
  let cancelado = false;
  const hecho = autoformatear(
    folio,
    {
      inputType: 'insertText',
      data: tecla,
      isComposing: false,
      preventDefault: () => {
        cancelado = true;
      },
    },
    editar,
    () => enFormato,
  );
  expect(cancelado).toBe(hecho);
  return hecho;
}

const ctrlZ = (mods: Partial<KeyboardEvent> = {}) => {
  let consumida = false;
  const e = {
    key: 'z',
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
    preventDefault: () => {
      consumida = true;
    },
    ...mods,
  };
  return { hecho: deshacerAutoformato(e), consumida: () => consumida };
};

describe('autoformato de bloque', () => {
  test('«# » hace título, con lo tecleado en su propio paso', () => {
    montar('<p>#</p>');
    cursorTras('#');
    const pedidos: string[] = [];
    expect(teclear(' ', editorFalso(pedidos))).toBe(true);
    expect(pedidos).toEqual(['insertText  ', 'delete', 'formatBlock <h1>']);
    expect(folio.innerHTML).toBe('<h1></h1>');
    expect(hayAutoformatoPendiente()).toBe(true);
  });

  test('Ctrl+Z justo después deshace solo el autoformato', () => {
    montar('<p>##</p>');
    cursorTras('##');
    const pedidos: string[] = [];
    teclear(' ', editorFalso(pedidos));
    pedidos.length = 0;
    const z = ctrlZ();
    expect(z.hecho).toBe(true);
    expect(z.consumida()).toBe(true);
    // Los dos pasos del formato (borrar la marca y titular); el espacio queda.
    expect(pedidos).toEqual(['undo', 'undo']);
    expect(hayAutoformatoPendiente()).toBe(false);
    // El siguiente Ctrl+Z ya es el del navegador.
    expect(ctrlZ().hecho).toBe(false);
  });

  test('otra tecla da el autoformato por bueno; las teclas de modificador, no', () => {
    montar('<p>&gt;</p>');
    cursorTras('>');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe('<blockquote></blockquote>');
    expect(ctrlZ({ key: 'Control' }).hecho).toBe(false);
    expect(hayAutoformatoPendiente()).toBe(true);
    expect(ctrlZ({ key: 'a', ctrlKey: false }).hecho).toBe(false);
    expect(hayAutoformatoPendiente()).toBe(false);
  });

  test('«1. » con el resto del párrafo detrás hace la numerada, y «3) » empieza en 3', () => {
    montar('<p>1.Comprar pan</p>');
    cursorTras('1.');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe('<ol><li>Comprar pan</li></ol>');
    montar('<p>3)</p>');
    cursorTras('3)');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe('<ol start="3"><li></li></ol>');
  });

  test('«[ ] » hace una lista de casillas y «[x] » la empieza hecha', () => {
    montar('<p>[ ]</p>');
    cursorTras('[ ]');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe('<ul data-checklist=""><li data-checked="0"></li></ul>');
    montar('<p>[x]</p>');
    cursorTras('[x]');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe('<ul data-checklist=""><li data-checked="1"></li></ul>');
  });

  test('una casilla escrita bajo una viñeta no se mete en ella', () => {
    montar('<ul><li>uno</li></ul><p>[ ]</p>');
    cursorTras('[ ]');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe(
      '<ul><li>uno</li></ul><ul data-checklist=""><li data-checked="0"></li></ul>',
    );
  });

  test('ni una viñeta bajo una lista de casillas', () => {
    montar('<ul data-checklist=""><li data-checked="1">hecho</li></ul><p>-</p>');
    cursorTras('-');
    teclear(' ', editorFalso([]));
    expect(folio.innerHTML).toBe(
      '<ul data-checklist=""><li data-checked="1">hecho</li></ul><ul><li></li></ul>',
    );
  });

  test('«[ ] » en una viñeta pasa la lista a casillas, y Ctrl+Z la devuelve', () => {
    const html = '<ul><li>uno</li><li>[ ]</li></ul>';
    montar(html);
    cursorTras('[ ]');
    const pedidos: string[] = [];
    teclear(' ', editorFalso(pedidos));
    expect(folio.innerHTML).toBe(
      '<ul data-checklist=""><li data-checked="0">uno</li><li data-checked="0"></li></ul>',
    );
    pedidos.length = 0;
    ctrlZ();
    expect(pedidos).toEqual(['undo']); // el borrado de «[ ] »; los atributos, a mano
    expect(folio.querySelector('ul')!.hasAttribute('data-checklist')).toBe(false);
    expect(folio.querySelector('[data-checked]')).toBeNull();
  });

  test('a media línea, en un título, en una tabla o en una isla no hace nada', () => {
    for (const [html, tras] of [
      ['<p>hola #</p>', '#'],
      ['<h2>#</h2>', '#'],
      ['<table><tbody><tr><td><p>#</p></td></tr></tbody></table>', '#'],
      ['<div data-doc-header=""><p>-</p></div>', '-'],
      ['<ol><li>[ ]</li></ol>', '[ ]'],
    ] as const) {
      montar(html);
      cursorTras(tras);
      const pedidos: string[] = [];
      expect(teclear(' ', editorFalso(pedidos))).toBe(false);
      expect(pedidos).toEqual([]);
    }
  });

  test('si el navegador no deja escribir, la tecla sigue su camino', () => {
    montar('<p>#</p>');
    cursorTras('#');
    const pedidos: string[] = [];
    const sinEdicion: Editar = (comando, valor) => {
      pedidos.push(`${comando} ${valor}`);
      return false;
    };
    expect(teclear(' ', sinEdicion)).toBe(false);
    expect(pedidos).toEqual(['insertText  ']);
    expect(folio.innerHTML).toBe('<p>#</p>');
    expect(hayAutoformatoPendiente()).toBe(false);
  });

  test('con la preferencia apagada no se consulta', () => {
    expect(autoformatoActivo()).toBe(true);
    fijarAutoformato(false);
    expect(autoformatoActivo()).toBe(false);
    fijarAutoformato(true);
    expect(autoformatoActivo()).toBe(true);
  });
});

describe('autoformato en línea', () => {
  test('**negrita** al cerrar, y lo siguiente ya sin negrita', () => {
    montar('<p>di **hola*</p>');
    cursorTras('hola*');
    const pedidos: string[] = [];
    expect(teclear('*', editorFalso(pedidos))).toBe(true);
    expect(pedidos).toEqual(['insertText *', 'insertHTML <b>hola</b>', 'bold']);
    expect(folio.innerHTML).toBe('<p>di <b>hola</b></p>');
    pedidos.length = 0;
    ctrlZ();
    expect(pedidos).toEqual(['undo']);
  });

  test('si el cursor ya quedó fuera de la negrita (Firefox), no la vuelve a encender', () => {
    montar('<p>di **hola*</p>');
    cursorTras('hola*');
    const pedidos: string[] = [];
    expect(teclear('*', editorFalso(pedidos), false)).toBe(true);
    expect(pedidos).toEqual(['insertText *', 'insertHTML <b>hola</b>']);
  });

  test('~~tachado~~ y _cursiva_ escapan lo que no es marcado', () => {
    montar('<p>~~a&lt;b~</p>');
    cursorTras('a<b~');
    teclear('~', editorFalso([]));
    expect(folio.innerHTML).toBe('<p><s>a&lt;b</s></p>');
    montar('<p>_sí</p>');
    cursorTras('_sí');
    teclear('_', editorFalso([]));
    expect(folio.innerHTML).toBe('<p><i>sí</i></p>');
  });

  test('una marca sin cerrar se escribe tal cual', () => {
    montar('<p>2*3</p>');
    cursorTras('2*3');
    expect(teclear('*', editorFalso([]))).toBe(false);
  });
});

describe('listas de casillas', () => {
  test('Ctrl+Alt+Intro marca la del cursor, o todas las de la selección', () => {
    montar(
      '<ul data-checklist=""><li data-checked="0">a</li><li data-checked="1">b</li><li data-checked="0">c</li></ul>',
    );
    cursorTras('a');
    expect(alternarCasillasDeLaSeleccion(folio)).toBe(true);
    const estados = () =>
      Array.from(folio.querySelectorAll('li')).map((li) => li.getAttribute('data-checked'));
    expect(estados()).toEqual(['1', '1', '0']);
    // De «a» a «c»: no estaban todas hechas, así que se marcan todas; otra vez, se desmarcan.
    const r = document.createRange();
    r.setStart(folio.querySelector('li')!.firstChild!, 0);
    r.setEnd(folio.querySelectorAll('li')[2]!.firstChild!, 1);
    document.getSelection()!.removeAllRanges();
    document.getSelection()!.addRange(r);
    alternarCasillasDeLaSeleccion(folio);
    expect(estados()).toEqual(['1', '1', '1']);
    alternarCasillasDeLaSeleccion(folio);
    expect(estados()).toEqual(['0', '0', '0']);
    // Fuera de una lista de casillas no hace nada (y la tecla sigue su camino).
    montar('<p>a</p>');
    cursorTras('a');
    expect(alternarCasillasDeLaSeleccion(folio)).toBe(false);
  });

  test('el elemento que crea Intro nace sin hacer', () => {
    // Intro al final de uno hecho: el navegador clona el <li> con su atributo.
    montar(
      '<ul data-checklist=""><li data-checked="1">hecho</li><li data-checked="1">nuevo</li></ul>',
    );
    cursorTras('nuevo');
    trasNuevoElemento(folio);
    expect(folio.querySelectorAll('li')[1]!.getAttribute('data-checked')).toBe('0');
    // Intro al principio: el vacío queda arriba y el texto sigue hecho.
    montar('<ul data-checklist=""><li data-checked="1"></li><li data-checked="1">hecho</li></ul>');
    cursorTras('hecho');
    trasNuevoElemento(folio);
    const lis = folio.querySelectorAll('li');
    expect([lis[0]!.getAttribute('data-checked'), lis[1]!.getAttribute('data-checked')]).toEqual([
      '0',
      '1',
    ]);
  });

  test('el clic marca solo en la casilla, a la izquierda del texto', () => {
    montar('<ul data-checklist=""><li data-checked="0">a</li></ul>');
    const li = folio.querySelector('li')!;
    li.getBoundingClientRect = () => ({ left: 100 }) as DOMRect;
    expect(casillaDelClic(folio, li, 90)).toBe(li);
    expect(casillaDelClic(folio, li, 120)).toBeNull();
    expect(casillaDelClic(folio, li.firstChild, 90)).toBeNull();
  });

  test('marcar como casillas se deshace entero', () => {
    montar('<ul><li>a</li><li data-checked="1">b</li></ul>');
    const ul = folio.querySelector('ul')!;
    const antes = folio.innerHTML;
    const revertir = marcarComoCasillas(ul, ul.firstElementChild, true);
    expect(folio.innerHTML).toBe(
      '<ul data-checklist=""><li data-checked="1">a</li><li data-checked="1">b</li></ul>',
    );
    revertir();
    expect(folio.innerHTML).toBe(antes);
  });

  test('con varios párrafos seleccionados, todos pasan a casillas y la viñeta vecina no', () => {
    // Lo que deja execCommand('insertUnorderedList') con «a» y «b» seleccionados bajo una
    // lista de viñetas: los junta con ella.
    montar('<ul><li>x</li><li>a</li><li>b</li></ul><p>y</p>');
    const [a, b] = [folio.querySelectorAll('li')[1]!, folio.querySelectorAll('li')[2]!];
    const r = document.createRange();
    r.setStart(a.firstChild!, 0);
    r.setEnd(b.firstChild!, 1);
    document.getSelection()!.removeAllRanges();
    document.getSelection()!.addRange(r);
    dejarSeleccionEnCasillas(folio, document.getSelection()!.getRangeAt(0));
    expect(folio.innerHTML).toBe(
      '<ul><li>x</li></ul><ul data-checklist=""><li data-checked="0">a</li>' +
        '<li data-checked="0">b</li></ul><p>y</p>',
    );
    // La selección sigue en lo que se seleccionó.
    const sel = document.getSelection()!;
    expect([sel.anchorNode, sel.focusNode]).toEqual([a.firstChild, b.firstChild]);

    // Sin vecina, la lista entera; con el cursor en un solo elemento, solo ese.
    montar('<ul><li>a</li><li>b</li></ul>');
    const todo = document.createRange();
    todo.selectNodeContents(folio.querySelector('ul')!);
    document.getSelection()!.removeAllRanges();
    document.getSelection()!.addRange(todo);
    cursorTras('a');
    const s2 = document.getSelection()!;
    s2.setBaseAndExtent(s2.anchorNode!, 0, folio.querySelectorAll('li')[1]!.firstChild!, 1);
    dejarSeleccionEnCasillas(folio, s2.getRangeAt(0));
    expect(folio.innerHTML).toBe(
      '<ul data-checklist=""><li data-checked="0">a</li><li data-checked="0">b</li></ul>',
    );
    montar('<ul><li>x</li><li>a</li></ul>');
    cursorTras('a');
    dejarSeleccionEnCasillas(folio, document.getSelection()!.getRangeAt(0));
    expect(folio.innerHTML).toBe(
      '<ul><li>x</li></ul><ul data-checklist=""><li data-checked="0">a</li></ul>',
    );
  });

  test("exec('listCheck') pasa una viñeta a casillas y la quita; la cinta lo ve", () => {
    montar('<ul><li>a</li><li>b</li></ul>');
    setEditorRoot(folio);
    let input = 0;
    folio.addEventListener('input', () => input++);
    cursorTras('b');
    expect(getFormatState().checklist).toBe(false);
    exec('listCheck');
    expect(folio.innerHTML).toBe(
      '<ul data-checklist=""><li data-checked="0">a</li><li data-checked="0">b</li></ul>',
    );
    expect(input).toBe(1);
    cursorTras('b');
    const fmt = getFormatState();
    expect([fmt.checklist, fmt.bullet]).toEqual([true, false]);
    exec('listCheck');
    expect(folio.querySelector('[data-checklist],[data-checked]')).toBeNull();
    setEditorRoot(null);
  });
});
