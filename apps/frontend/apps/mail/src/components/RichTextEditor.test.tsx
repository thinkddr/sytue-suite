/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las imágenes del cuerpo: «Desde este dispositivo» admite varias y entran en el orden
 * en que se eligen, y una imagen `data:` pegada entra como parte del mensaje (`cid:`) en
 * vez de perderse en el saneado. Ejecutar: `bun test`.
 *
 * `execCommand` no existe en jsdom: se anota lo que el editor le pide, que es lo que
 * hay que fijar aquí (el navegador ya sabe insertar HTML).
 */
import { setupJsdom } from '../../test/dom.js';
import { beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

setupJsdom();

const { RichTextEditor } = await import('./RichTextEditor.js');

let pedidos: [string, string | undefined][] = [];
// Como el navegador: `insertHTML` escribe donde está la selección y deja el cursor detrás,
// en un rango NUEVO (el que se hubiera guardado antes no se mueve con él).
beforeAll(() => {
  (document as unknown as { execCommand: unknown }).execCommand = (
    c: string,
    _: boolean,
    v?: string,
  ) => {
    pedidos.push([c, v]);
    const sel = window.getSelection();
    if (c !== 'insertHTML' || !sel || sel.rangeCount === 0 || !v) return true;
    const r = sel.getRangeAt(0).cloneRange();
    r.deleteContents();
    const trozo = r.createContextualFragment(v);
    const ultimo = trozo.lastChild;
    r.insertNode(trozo);
    const cursor = document.createRange();
    if (ultimo) cursor.setStartAfter(ultimo);
    cursor.collapse(true);
    sel.removeAllRanges();
    sel.addRange(cursor);
    return true;
  };
});

async function montar(incrustados: string[]): Promise<HTMLElement> {
  document.body.innerHTML = '';
  pedidos = [];
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <RichTextEditor
        locale="es"
        id="cuerpo"
        initialHtml=""
        ariaLabel="Cuerpo"
        onChange={() => {}}
        onInsertarImagen={async (f) => {
          incrustados.push(f.name);
          return `cid${incrustados.length}`;
        }}
      />,
      host,
    );
  });
  return host;
}

const esperar = () => act(async () => new Promise((r) => setTimeout(r, 0)));
const insertados = () => pedidos.filter(([c]) => c === 'insertHTML').map(([, v]) => v ?? '');

test('«Desde este dispositivo» admite varias y entran en su orden', async () => {
  const incrustados: string[] = [];
  const host = await montar(incrustados);
  const cuerpo = host.querySelector<HTMLElement>('#cuerpo')!;
  const r = document.createRange();
  r.setStart(cuerpo, 0);
  window.getSelection()!.removeAllRanges();
  window.getSelection()!.addRange(r);
  const boton = host.querySelector<HTMLButtonElement>('button[aria-label="Insertar imagen"]');
  await act(async () => boton!.click());
  const campo = host.querySelector<HTMLInputElement>('input[type="file"]');
  expect(campo?.multiple).toBe(true);
  const ficheros = [
    new File(['a'], 'a.png', { type: 'image/png' }),
    new File(['b'], 'b.png', { type: 'image/png' }),
  ];
  Object.defineProperty(campo, 'files', { value: ficheros });
  await act(async () => {
    campo!.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await esperar();
  expect(incrustados).toEqual(['a.png', 'b.png']);
  // En el cuerpo, en el MISMO orden: la segunda detrás de la primera.
  expect([...cuerpo.querySelectorAll('img')].map((i) => i.getAttribute('src'))).toEqual([
    'cid:cid1',
    'cid:cid2',
  ]);
  expect(insertados()).toEqual([
    '<img src="cid:cid1" alt="" style="max-width: 100%">',
    '<img src="cid:cid2" alt="" style="max-width: 100%">',
  ]);
});

test('una imagen data: pegada entra como parte del mensaje', async () => {
  const incrustados: string[] = [];
  const host = await montar(incrustados);
  const cuerpo = host.querySelector<HTMLElement>('#cuerpo');
  const pegado = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(pegado, 'clipboardData', {
    value: {
      getData: (tipo: string) =>
        tipo === 'text/html' ? '<p>Foto <img src="data:image/png;base64,iVBORw0K"></p>' : 'Foto',
    },
  });
  await act(async () => {
    cuerpo!.dispatchEvent(pegado);
  });
  await esperar();
  expect(incrustados).toEqual(['image001.png']);
  const [html] = insertados();
  expect(html).toContain('<img src="cid:cid1">');
  expect(html).not.toContain('data:');
});

test('el HTML de un borrador se sanea al cargarlo, no solo al pegar y al enviar', async () => {
  // Lo que vuelve del buzón no tiene por qué haber pasado por este compositor.
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <RichTextEditor
        locale="es"
        id="cuerpo"
        initialHtml={
          '<p onclick="x()">hola <b>mundo</b></p><img src="x" onerror="x()"><script>x()</script>'
        }
        ariaLabel="Cuerpo"
        onChange={() => {}}
      />,
      host,
    );
  });
  const cuerpo = host.querySelector('[contenteditable]')!;
  expect(cuerpo.innerHTML).toBe('<p>hola <b>mundo</b></p>');
});
