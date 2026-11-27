/**
 * LA IDA Y VUELTA ENTRE LA WEB Y EL ESCRITORIO, BYTE A BYTE.
 *
 * `apps/nucleo/notes/model/testdata/ida-y-vuelta/*.html` es el HTML de los bloques que
 * escriben los dos editores: desplegable abierto y cerrado, código con números y ajuste,
 * tabla numerada y con celdas combinadas, columnas, subíndice y superíndice, imagen y
 * fichero del texto. Fyne comprueba que lo lee y lo vuelve a escribir igual
 * (`TestLaIdaYVueltaConLaWebEsByteAByte`), el servicio que su saneador lo deja pasar, y
 * aquí, que el editor de la web —lo que el navegador tiene en el DOM al pintarlo, con las
 * imágenes ya puestas, y entrega con `normalizarSalida`— lo devuelve igual. Lo único que
 * se perdona es la barra de las vacías (`<img/>` de Go y `<img>` del navegador).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { normalizarSalida } from './salida.js';
import { pintarAdjuntos } from '../components/adjuntos-texto.js';

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

const DIR = new URL('../../../../../nucleo/notes/model/testdata/ida-y-vuelta/', import.meta.url);
const sinBarra = (html: string) => html.replace(/<(br|hr|img|input)([^>]*?)\/>/g, '<$1$2>');

// Los adjuntos de `imagen-y-fichero.html`: la imagen se pinta con sus datos en el editor.
const ADJUNTOS = [
  {
    id: '1732540000000-ab12cd',
    name: 'plano.png',
    size: 4,
    type: 'image/png',
    dataUrl: 'data:image/png;base64,iVBORw0KGgo=',
  },
  {
    id: '1732540000000-ef34gh',
    name: 'informe.pdf',
    size: 4,
    type: 'application/pdf',
    dataUrl: 'data:application/pdf;base64,JVBERg==',
  },
];

test('cada fichero compartido vuelve igual del editor de la web', () => {
  const ficheros = readdirSync(DIR).filter((f) => f.endsWith('.html'));
  expect(ficheros.length).toBeGreaterThanOrEqual(10);
  for (const f of ficheros) {
    const fuente = readFileSync(new URL(f, DIR), 'utf8').trim();
    const campo = document.createElement('div');
    campo.innerHTML = fuente;
    pintarAdjuntos(campo, ADJUNTOS, false);
    expect([f, normalizarSalida(campo)]).toEqual([f, sinBarra(fuente)]);
  }
});

test('la imagen se pinta con los datos de su adjunto y se entrega nombrándolo', () => {
  const campo = document.createElement('div');
  campo.innerHTML = '<p><img src="adjunto:1732540000000-ab12cd" alt="plano.png"></p>';
  pintarAdjuntos(campo, ADJUNTOS, false);
  const img = campo.querySelector('img')!;
  expect(img.getAttribute('src')).not.toBe('adjunto:1732540000000-ab12cd');
  expect(img.getAttribute('data-adjunto')).toBe(ADJUNTOS[0]!.id);
  expect(normalizarSalida(campo)).toBe(
    '<p><img src="adjunto:1732540000000-ab12cd" alt="plano.png"></p>',
  );
});

test('los atributos de un bloque se entregan en el orden de siempre, pulsados como se pulsen', () => {
  const campo = document.createElement('div');
  campo.innerHTML =
    '<pre data-ajuste="" data-lineas="">a</pre><details open="" data-expandir=""><summary>T</summary></details>';
  expect(normalizarSalida(campo)).toBe(
    '<pre data-lineas="" data-ajuste="">a</pre><details data-expandir="" open=""><summary>T</summary></details>',
  );
});
