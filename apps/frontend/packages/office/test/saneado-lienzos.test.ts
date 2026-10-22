/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL SANEADOR DE LOS LIENZOS, MEDIDO CONTRA LO QUE EL EDITOR PRODUCE.
 *
 * Los tres lienzos de Docs pintan HTML que escribió otra persona, y lo pasan antes
 * por `saneaHtmlDeDocumento` (@sytue/ui). Un saneador puede fallar por los dos
 * lados, y este fichero vigila los dos:
 *
 *   - por defecto: que un lienzo vuelva a pintar el modelo crudo. Lo mira la primera
 *     prueba, sobre el código fuente de los tres sumideros;
 *   - por exceso: que quite algo que el editor necesita. El folio se guarda desde el
 *     DOM, así que lo que el saneador quite al abrir se pierde al guardar. La
 *     segunda parte pasa por él las plantillas reales del servicio y lo que hornea
 *     cada bloque, y exige que salgan IGUAL que si solo se hubieran analizado.
 *
 * Esa segunda parte es la que cazó que `<foreignObject>` —el vector de libro de los
 * SVG— es donde las formas y SmartArt guardan su texto.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();

import { expect, test } from 'bun:test';
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { agregarNodo, escenaVacia, type Escena } from '@sytue/escena';
import {
  ATRIBUTOS_DE_ENLACE,
  ATRIBUTOS_DE_RECURSO,
  ELEMENTOS_DE_RECURSO,
  ESQUEMAS_DE_ENLACE,
  ESQUEMAS_DE_RECURSO,
  ESTILO_VETADO,
  ETIQUETAS_ELIMINADAS,
  ETIQUETAS_PERMITIDAS,
  saneaHtmlDeDocumento,
} from '../../../packages/ui/src/lib/saneaHtml';
import { buildIndexHtml } from '../src/editor/alphaindex';
import { dibujoFigureHtml } from '../src/editor/dibujo';
import { chartFigureHtml, newChartGrid, shapeFigureHtml } from '../src/editor/docblocks';
import { buildFormFieldHtml } from '../src/editor/formfields';
import { mergeFieldHtml } from '../src/editor/mailmerge';
import { pageBorderHtml, watermarkHtml } from '../src/editor/pagedecor';
import { smartArtFigureHtml } from '../src/editor/smartart';
import { deletionHtml, insertionHtml, TRACK_SENTINEL } from '../src/editor/trackchanges';
import {
  documentoDeMuestra,
  hojaDeMuestra,
  presentacionDeMuestra,
} from '../src/editor/dev/superficies-fixtures';

const leer = (ruta: string): string =>
  readFileSync(fileURLToPath(new URL(ruta, import.meta.url)), 'utf8');

/* ---------- por defecto: los tres sumideros pasan por el saneador ---------- */

test('ningún lienzo pinta el modelo sin sanear', () => {
  const folio = leer('../src/editor/canvas/PageCanvas.tsx');
  expect(folio).toContain('page.innerHTML = saneaHtmlDeDocumento(html)');
  expect(folio).not.toMatch(/page\.innerHTML = html\b/);

  // En la hoja, las dos ramas de `cellHtml` que emiten el literal de la celda.
  const hoja = leer('../src/editor/sheets/SheetCanvas.tsx');
  expect(hoja).not.toContain('withIcon(raw, icon)');
  expect(hoja.match(/withIcon\(saneaHtmlDeDocumento\(raw\), icon\)/g)?.length).toBe(2);
  // Y al entrar a editar una celda con formato, sparkline o total, que enseña el literal.
  expect(hoja).not.toMatch(/innerHTML = raw\b/);

  const diapos = leer('../src/editor/slides/SlidesCanvas.tsx');
  expect(diapos).not.toMatch(/__html: b\.html\b/);
  expect(diapos.match(/__html: saneaHtmlDeDocumento\(b\.html\)/g)?.length).toBe(2);
});

test('lo que rellena la IA entra en la celda escapado, como una fórmula tecleada', () => {
  // `aifill` escribe en `cells`, que guarda HTML, la respuesta de un modelo.
  const hoja = leer('../src/editor/sheets/SheetCanvas.tsx');
  expect(hoja).toContain('r: rect.r1 + dr })] = escapeHtml(formula);');
  expect(hoja).not.toContain('r: rect.r1 + dr })] = formula;');
});

/* ---------- por exceso: lo que el editor produce sale igual ---------- */

/** El mismo HTML tras analizarlo y volver a escribirlo, SIN sanear. */
const analizado = (html: string): string =>
  new DOMParser().parseFromString(html, 'text/html').body.innerHTML;

function sinCambios(nombre: string, html: string): void {
  expect(html.length, nombre).toBeGreaterThan(0);
  expect(saneaHtmlDeDocumento(html), nombre).toBe(analizado(html));
}

const PNG =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==';

/** Lo que hornea cada bloque del editor, por nombre. */
function muestrasDelEditor(): Record<string, string> {
  const autora = { name: 'Ana', id: 'u1' };
  let escena: Escena = {
    ...escenaVacia({ ancho: 800, alto: 400 }),
    titulo: 'Flujo',
    descripcion: 'Dos pasos',
  };
  escena = agregarNodo(escena, 'capa1', {
    id: 'a',
    tipo: 'forma',
    forma: 'rect',
    x: 10,
    y: 10,
    w: 120,
    h: 60,
    texto: 'Alta',
  });

  return {
    forma: shapeFigureHtml({
      shape: 'ellipse',
      w: 300,
      h: 160,
      fill: '#2563EB',
      stroke: '#0A0A0A',
      text: 'Etiqueta',
    }),
    grafico: chartFigureHtml('bar', newChartGrid()),
    smartartProceso: smartArtFigureHtml('process', [{ text: 'Uno' }, { text: 'Dos' }]),
    smartartLista: smartArtFigureHtml('list', [{ text: 'Uno' }, { text: 'Dos' }]),
    dibujo: dibujoFigureHtml(escena, { etiqueta: 'Dibujo', respaldo: PNG }),
    marcaDeAguaTexto: watermarkHtml({
      kind: 'text',
      text: 'BORRADOR',
      color: '#94A3B8',
      opacity: 0.15,
      size: 120,
      diagonal: true,
    }),
    marcaDeAguaImagen: watermarkHtml({
      kind: 'image',
      text: '',
      image: PNG,
      color: '#94A3B8',
      opacity: 0.15,
      size: 120,
      diagonal: false,
    }),
    bordeDePagina: pageBorderHtml({ style: 'dashed', width: 2, color: '#94A3B8' }),
    campoTexto: buildFormFieldHtml({ type: 'text', name: 'Nombre', placeholder: 'Tu nombre' }),
    campoCasilla: buildFormFieldHtml({ type: 'checkbox', name: 'Acepto', checked: true }),
    campoCombinado: mergeFieldHtml('Nombre'),
    insercion: insertionHtml('hola', autora, 42),
    borrado: deletionHtml('x', autora, 7),
    indice: buildIndexHtml([
      { term: 'Animales', sub: '', ref: '1' },
      { term: 'Animales', sub: 'Perros', ref: '2' },
    ]),
  };
}

test('lo que hornea cada bloque del editor pasa intacto', () => {
  const muestras = muestrasDelEditor();
  for (const [nombre, html] of Object.entries(muestras)) sinCambios(nombre, html);

  // Y lo que hace falta de cada uno sigue ahí, por si el análisis lo hubiera perdido.
  expect(saneaHtmlDeDocumento(muestras['forma']!)).toContain('Etiqueta');
  expect(saneaHtmlDeDocumento(muestras['smartartProceso']!)).toContain('Uno');
  expect(saneaHtmlDeDocumento(muestras['marcaDeAguaImagen']!)).toContain('data:image/png');
});

test('las plantillas del servicio pasan intactas', () => {
  const raiz = '../../../../../services/docs/internal/plantillas/';
  let vistas = 0;
  for (const carpeta of ['modelos/', 'bloques/']) {
    const dir = fileURLToPath(new URL(raiz + carpeta, import.meta.url));
    for (const fichero of readdirSync(dir)) {
      const texto = readFileSync(dir + fichero, 'utf8');
      if (fichero.endsWith('.html')) {
        sinCambios(carpeta + fichero, texto);
        vistas++;
      } else if (fichero.endsWith('.json')) {
        // Hojas y presentaciones: el HTML va en cadenas dentro del modelo.
        const recorrer = (v: unknown, ruta: string): void => {
          if (typeof v === 'string') {
            if (v.includes('<')) {
              sinCambios(ruta, v);
              vistas++;
            }
          } else if (v && typeof v === 'object') {
            for (const [k, hijo] of Object.entries(v)) recorrer(hijo, `${ruta}.${k}`);
          }
        };
        recorrer(JSON.parse(texto), carpeta + fichero);
      }
    }
  }
  // Que la prueba no pase por no haber encontrado nada que mirar.
  expect(vistas).toBeGreaterThan(10);
});

/* ---------- la misma lista que el servidor ---------- */

test('las listas son las del saneador del servicio', () => {
  // El servicio sanea al guardar con las mismas reglas (saneado_html.go); si una lista se
  // cambia en un lado y no en el otro, lo que uno deja pasar el otro lo quita.
  const go = leer('../../../../../services/docs/internal/domain/saneado_html.go');
  const conjunto = (nombre: string): string[] => {
    const m = go.match(new RegExp(`${nombre}\\s*=\\s*conjunto\\(\`([^\`]*)\`\\)`));
    if (!m) throw new Error(`no encuentro ${nombre} en saneado_html.go`);
    return m[1]!
      .split(/\s+/)
      .filter(Boolean)
      .map((x) => x.toLowerCase())
      .sort();
  };
  const cadenas = (nombre: string): string[] => {
    const m = go.match(new RegExp(`${nombre}\\s*=\\s*\\[\\]string\\{([^}]*)\\}`));
    if (!m) throw new Error(`no encuentro ${nombre} en saneado_html.go`);
    return [...m[1]!.matchAll(/"([^"]*)"/g)].map((x) => x[1]!).sort();
  };
  const orden = (xs: Iterable<string>): string[] => [...xs].sort();
  expect(orden(ETIQUETAS_ELIMINADAS)).toEqual(conjunto('htmlQuitadas'));
  expect(orden(ETIQUETAS_PERMITIDAS)).toEqual(conjunto('htmlPermitidas'));
  expect(orden(ATRIBUTOS_DE_ENLACE)).toEqual(conjunto('atributosDeEnlace'));
  expect(orden(ATRIBUTOS_DE_RECURSO)).toEqual(conjunto('atributosDeRecurso'));
  expect(orden(ELEMENTOS_DE_RECURSO)).toEqual(conjunto('elementosDeRecurso'));
  expect(orden(ESQUEMAS_DE_ENLACE)).toEqual(cadenas('esquemasDeEnlace'));
  expect(orden(ESQUEMAS_DE_RECURSO)).toEqual(cadenas('esquemasDeRecurso'));
  expect(orden(ESTILO_VETADO)).toEqual(cadenas('estiloVetado'));
});

/* ---------- un documento de cada tipo, para el servicio ---------- */

/** Un documento de cada tipo tal como lo guarda el editor. El folio es lo que deja
 *  `page.innerHTML` (analizado) con todos los bloques y la marca del control de cambios
 *  delante; la hoja y la presentación, sus modelos. */
function documentosDelEditor(): Record<string, string> {
  const folio = documentoDeMuestra() + Object.values(muestrasDelEditor()).join('');
  return {
    'doc:html': TRACK_SENTINEL + analizado(folio),
    'doc:sheet': JSON.stringify(hojaDeMuestra()),
    'doc:slides': JSON.stringify(presentacionDeMuestra()),
  };
}

test('un documento de cada tipo del editor, el mismo que mira el servicio', () => {
  // El servicio comprueba con este fichero que su saneador no cambia un byte de lo que el
  // editor guarda (saneado_contrato_test.go). Si el editor cambia lo que escribe, esto lo
  // dice: se regenera con SYTUE_REGENERAR_SANEADO=1 y se vuelven a pasar las dos pruebas.
  const ruta = fileURLToPath(
    new URL(
      '../../../../../services/docs/internal/domain/testdata/saneado/editor.json',
      import.meta.url,
    ),
  );
  const docs = documentosDelEditor();
  const hoy = `${JSON.stringify(docs, null, 1)}\n`;
  if (process.env['SYTUE_REGENERAR_SANEADO'] === '1') {
    mkdirSync(dirname(ruta), { recursive: true });
    writeFileSync(ruta, hoy);
  }
  expect(readFileSync(ruta, 'utf8')).toBe(hoy);

  // Y el saneador del navegador también lo deja como lo deja el análisis.
  sinCambios('doc:html', docs['doc:html']!.slice(TRACK_SENTINEL.length));
  const hoja = hojaDeMuestra();
  for (const h of hoja.sheets)
    for (const [ref, v] of Object.entries(h.cells)) if (v.includes('<')) sinCambios(ref, v);
  for (const d of presentacionDeMuestra().slides)
    for (const b of d.boxes) if (b.html?.includes('<')) sinCambios(b.id, b.html);
});
