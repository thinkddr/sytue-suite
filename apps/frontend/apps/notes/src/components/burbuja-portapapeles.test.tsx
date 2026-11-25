/*
 * COPIAR, CORTAR Y PEGAR EN LA BURBUJA DE LA SELECCIÓN.
 *
 * Al seleccionar, la burbuja enseña el formato y, en su propio grupo, Copiar, Cortar y
 * Pegar. Copiar lleva el HTML y el texto de lo seleccionado; cortar además lo borra;
 * pegar pasa por el mismo saneado que Ctrl+V; y sin permiso para leer el portapapeles se
 * dice cómo pegar en vez de no hacer nada. El portapapeles es de juguete.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { olvidarMedidas } from './BarraConMas.js';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  seleccionar,
  tick,
  type EdicionDePrueba,
  type EditorMontado,
} from './edicion-de-prueba.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let edicion: EdicionDePrueba;
let editor: EditorMontado | null = null;
const cajaOriginal = Range.prototype.getBoundingClientRect;
const mediaOriginal = window.matchMedia;
const portapapelesOriginal = Object.getOwnPropertyDescriptor(navigator, 'clipboard');

/** Lo que se escribe en el portapapeles de juguete. */
let escrito: Record<string, string>[] = [];
let textoEscrito: string[] = [];

class ItemDePrueba {
  constructor(readonly datos: Record<string, Blob>) {}
}

function portapapeles(
  leer: () => Promise<{ types: string[]; getType: (t: string) => Promise<Blob> }[]>,
) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      write: async (items: ItemDePrueba[]) => {
        for (const it of items) {
          const fila: Record<string, string> = {};
          for (const [tipo, blob] of Object.entries(it.datos)) fila[tipo] = await blob.text();
          escrito.push(fila);
        }
      },
      writeText: async (t: string) => void textoEscrito.push(t),
      read: leer,
      readText: async () => {
        throw new DOMException('denegado', 'NotAllowedError');
      },
    },
  });
}

beforeEach(() => {
  olvidarMedidas();
  edicion = instalarEdicionDePrueba();
  escrito = [];
  textoEscrito = [];
  (globalThis as { ClipboardItem?: unknown }).ClipboardItem = ItemDePrueba;
  // happy-dom no maqueta: la selección mide algo para que la burbuja tenga dónde ir.
  Range.prototype.getBoundingClientRect = () =>
    ({ left: 100, top: 200, bottom: 220, right: 160, width: 60, height: 20 }) as DOMRect;
  portapapeles(async () => []);
});
afterEach(() => {
  editor?.desmontar();
  editor = null;
  edicion.quitar();
  Range.prototype.getBoundingClientRect = cajaOriginal;
  window.matchMedia = mediaOriginal;
  delete (globalThis as { ClipboardItem?: unknown }).ClipboardItem;
  if (portapapelesOriginal) Object.defineProperty(navigator, 'clipboard', portapapelesOriginal);
});

const burbuja = () =>
  document.querySelector<HTMLElement>('[role="toolbar"][aria-label="editor.selectionTools"]');

async function conSeleccion(html: string, texto: string): Promise<EditorMontado> {
  const e = await montarEditor(html);
  seleccionar(e.campo, texto);
  document.dispatchEvent(new Event('selectionchange'));
  await tick();
  return e;
}

test('al seleccionar, la burbuja enseña copiar, cortar y pegar en su propio grupo', async () => {
  editor = await conSeleccion('<p>hola mundo</p>', 'hola');
  const b = burbuja()!;
  expect(b).not.toBeNull();
  const hijos = [...b.children];
  const etiquetas = hijos.map(
    (h) => h.getAttribute('aria-label') ?? (h.getAttribute('aria-hidden') ? '|' : ''),
  );
  expect(etiquetas.slice(-4)).toEqual(['|', 'editor.copy', 'editor.cut', 'editor.paste']);
  expect(etiquetas).toContain('editor.bold');
});

test('copiar lleva el HTML y el texto de lo seleccionado', async () => {
  editor = await conSeleccion('<p>hola <b>mundo</b> feliz</p>', 'hola ');
  // Selección que cruza el formato: «hola mundo».
  const sel = document.getSelection()!;
  const r = sel.getRangeAt(0);
  r.setEnd(editor.campo.querySelector('b')!.firstChild!, 5);
  boton(burbuja()!, 'editor.copy').click();
  await tick(10);
  expect(escrito).toEqual([{ 'text/html': 'hola <b>mundo</b>', 'text/plain': 'hola mundo' }]);
  expect(editor.campo.textContent).toBe('hola mundo feliz');
});

test('cortar copia y borra, con la orden del navegador (Ctrl+Z lo devuelve)', async () => {
  editor = await conSeleccion('<p>hola mundo</p>', 'mundo');
  boton(burbuja()!, 'editor.cut').click();
  await tick(10);
  expect(escrito[0]?.['text/plain']).toBe('mundo');
  expect(edicion.ordenes.some(([o]) => o === 'delete')).toBe(true);
  expect(editor.campo.textContent).toBe('hola ');
});

test('pegar pasa por el saneado: ni manejadores, ni scripts, ni imágenes', async () => {
  portapapeles(async () => [
    {
      types: ['text/html', 'text/plain'],
      getType: async (t: string) =>
        new Blob([
          t === 'text/html'
            ? '<p onclick="robar()">hola <script>alert(1)</script><img src=x onerror=y><b>fuerte</b></p>'
            : 'hola fuerte',
        ]),
    },
  ]);
  editor = await conSeleccion('<p>antes</p>', 'antes');
  boton(burbuja()!, 'editor.paste').click();
  await tick(10);
  const html = edicion.ordenes.find(([o]) => o === 'insertHTML')?.[1] ?? '';
  expect(html).toContain('<b>fuerte</b>');
  for (const malo of ['onclick', '<script', '<img', 'onerror']) expect(html).not.toContain(malo);
  expect(editor.campo.textContent).toContain('hola fuerte');
});

test('sin permiso para leer el portapapeles, dice que se pegue con el teclado', async () => {
  portapapeles(async () => {
    throw new DOMException('denegado', 'NotAllowedError');
  });
  editor = await conSeleccion('<p>antes</p>', 'antes');
  boton(burbuja()!, 'editor.paste').click();
  await tick(10);
  expect(editor.host.querySelector('[role="status"]')?.textContent).toBe('editor.pasteBlocked');
  // Y no ha tocado la nota.
  expect(editor.campo.textContent).toBe('antes');
});

test('en el teléfono va debajo de la selección y tachado, resaltado y código van en «Más formato»', async () => {
  window.matchMedia = ((q: string) => ({ matches: true, media: q })) as typeof window.matchMedia;
  editor = await conSeleccion('<p>hola mundo</p>', 'hola');
  const b = burbuja()!;
  const etiquetas = [...b.querySelectorAll('button')].map((x) => x.getAttribute('aria-label'));
  expect(etiquetas).toContain('editor.moreFormatting');
  expect(etiquetas).not.toContain('editor.strike');
  expect(etiquetas).not.toContain('editor.highlight');
  // Debajo: el menú del sistema sale encima de lo seleccionado.
  const caja = b.parentElement!;
  expect(caja.style.top).toBe('228px');
  expect(caja.style.transform).toBe('translateX(-50%)');
});

test('en el teléfono todas las herramientas de la burbuja miden lo mismo: el hueco entre glifos es igual', async () => {
  window.matchMedia = ((q: string) => ({ matches: true, media: q })) as typeof window.matchMedia;
  editor = await conSeleccion('<p>hola mundo</p>', 'hola');
  const botones = [...burbuja()!.querySelectorAll('button')];
  expect(botones.map((x) => x.getAttribute('aria-label'))).toEqual([
    'editor.bold',
    'editor.italic',
    'editor.moreFormatting',
    'editor.linkTool',
    'editor.copy',
    'editor.cut',
    'editor.paste',
  ]);
  // «Más formato» era más ancho (con flecha y menos relleno) y descuadraba la fila.
  const formas = new Set(
    botones.map((x) =>
      x.className
        .split(/\s+/)
        .filter((c) => !c.startsWith('aria-expanded:'))
        .sort()
        .join(' '),
    ),
  );
  expect(formas.size).toBe(1);
  for (const x of botones) expect(x.querySelectorAll('svg')).toHaveLength(1);
});
