/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del núcleo PURO del control de cambios (src/editor/trackchanges.ts):
 * horneado de la marca `<ins>`/`<del>` (escape anti-XSS del autor y del texto,
 * atributos data-* + color por autor), normalización de marcas adyacentes del
 * mismo autor, persistencia del MODO por documento (sentinela round-trip) y
 * resolución de marcas (aceptar/rechazar). Sin DOM (funciones string→string),
 * igual que docs-blocks; el pegamento vivo de canvas/exec.ts es cáscara sobre esto.
 * Ejecutar: cd apps/frontend && bun test packages/office/test/trackchanges.test.ts
 */
import { expect, test } from 'bun:test';
import {
  acceptTrackedChanges,
  authorColor,
  deletionHtml,
  insertionHtml,
  listTrackedChanges,
  mergeAdjacentTrackMarks,
  readTrackFlag,
  rejectTrackedChanges,
  resolveTrackedChange,
  TRACK_SENTINEL,
  withTrackFlag,
  type TrackAuthor,
} from '../src/editor/trackchanges';

const ANA: TrackAuthor = { name: 'Ana', id: 'u1' };
const BEA: TrackAuthor = { name: 'Bea', id: 'u2' };

test('insertionHtml envuelve en <ins data-track> con autoría y ts', () => {
  const html = insertionHtml('hola', ANA, 42);
  expect(html.startsWith('<ins data-track ')).toBe(true);
  expect(html).toContain('data-author="Ana"');
  expect(html).toContain('data-author-id="u1"');
  expect(html).toContain('data-ts="42"');
  expect(html).toContain(`--sytue-track:${authorColor('u1')}`);
  expect(html.endsWith('>hola</ins>')).toBe(true);
});

test('deletionHtml usa <del> con los mismos atributos', () => {
  const html = deletionHtml('x', ANA, 7);
  expect(html.startsWith('<del data-track ')).toBe(true);
  expect(html.endsWith('>x</del>')).toBe(true);
});

test('texto vacío no produce marca (nada que rastrear)', () => {
  expect(insertionHtml('', ANA, 1)).toBe('');
  expect(deletionHtml('', ANA, 1)).toBe('');
});

test('escapa el texto y el nombre del autor (frontera de confianza, anti-XSS)', () => {
  const evil: TrackAuthor = { name: '<img src=x onerror="alert(1)">', id: 'a"b' };
  const html = insertionHtml('1 < 2 & "tres" <script>', evil, 9);
  // Ni el texto ni el autor introducen marcado ejecutable.
  expect(html).not.toContain('<script>');
  expect(html).not.toContain('<img');
  expect(html).toContain('1 &lt; 2 &amp; &quot;tres&quot; &lt;script&gt;');
  expect(html).toContain('data-author="&lt;img src=x onerror=&quot;alert(1)&quot;&gt;"');
  expect(html).toContain('data-author-id="a&quot;b"');
});

test('authorColor es estable y sale de la paleta de marca', () => {
  expect(authorColor('u1')).toBe(authorColor('u1'));
  // Token de marca (Graphite & Brass), no un matiz HSL arbitrario: es el MISMO
  // color que el caret y el avatar de ese autor, y cumple AA sobre el folio.
  expect(authorColor('cualquier-sub-largo')).toMatch(/^var\(--sytue-avatar-[1-6]\)$/);
});

test('mergeAdjacentTrackMarks funde una racha del mismo autor y tipo', () => {
  const html = insertionHtml('a', ANA, 1) + insertionHtml('b', ANA, 2) + insertionHtml('c', ANA, 3);
  const merged = mergeAdjacentTrackMarks(html);
  // Una sola marca, con el contenido concatenado y el ts de la PRIMERA.
  expect(merged).toBe(
    `<ins data-track data-author="Ana" data-author-id="u1" data-ts="1" style="--sytue-track:${authorColor('u1')}">abc</ins>`,
  );
});

test('no funde autores distintos ni tipos distintos', () => {
  const distintos = insertionHtml('a', ANA, 1) + insertionHtml('b', BEA, 2);
  expect(mergeAdjacentTrackMarks(distintos)).toBe(distintos);
  const insDel = insertionHtml('a', ANA, 1) + deletionHtml('b', ANA, 2);
  expect(mergeAdjacentTrackMarks(insDel)).toBe(insDel);
});

test('no funde a través de texto suelto (no se traga caracteres del documento)', () => {
  const conTexto = insertionHtml('a', ANA, 1) + ' x ' + insertionHtml('b', ANA, 2);
  expect(mergeAdjacentTrackMarks(conTexto)).toBe(conTexto);
});

test('funde el par contiguo aunque quede uno aislado por un tercero ajeno', () => {
  // A(u1) B(u2) C(u2): la B/C contiguas del mismo autor deben fundirse aunque la
  // A las preceda (un scan por pares ingenuo se dejaría este caso).
  const html = insertionHtml('a', ANA, 1) + insertionHtml('b', BEA, 2) + insertionHtml('c', BEA, 3);
  const merged = mergeAdjacentTrackMarks(html);
  expect(merged).toContain('>a</ins>');
  expect(merged).toContain('>bc</ins>');
  expect((merged.match(/<ins/g) ?? []).length).toBe(2);
});

test('respeta marcas sin data-track y contenido con etiquetas (conservador)', () => {
  const plain = '<ins>a</ins><ins>b</ins>'; // sin data-track: no son marcas de rastreo
  expect(mergeAdjacentTrackMarks(plain)).toBe(plain);
  const nested = insertionHtml('a', ANA, 1) + '<ins data-track data-author-id="u1"><b>x</b></ins>';
  // El segundo tiene contenido con etiqueta: no encaja en el patrón de texto plano y pasa intacto.
  expect(mergeAdjacentTrackMarks(nested)).toBe(nested);
});

test('es idempotente y no altera HTML sin marcas', () => {
  const doc = '<p>Hola <b>mundo</b></p>';
  expect(mergeAdjacentTrackMarks(doc)).toBe(doc);
  const once = mergeAdjacentTrackMarks(insertionHtml('a', ANA, 1) + insertionHtml('b', ANA, 2));
  expect(mergeAdjacentTrackMarks(once)).toBe(once);
});

test('preserva el texto del documento alrededor de las marcas', () => {
  const html = '<p>hola ' + insertionHtml('a', ANA, 1) + insertionHtml('b', ANA, 2) + ' mundo</p>';
  const merged = mergeAdjacentTrackMarks(html);
  expect(merged.startsWith('<p>hola ')).toBe(true);
  expect(merged.endsWith(' mundo</p>')).toBe(true);
  expect(merged).toContain('>ab</ins>');
});

/* ---------- Persistencia del MODO (sentinela round-trip, hallazgo 1) ---------- */

test('withTrackFlag antepone la sentinela solo si el modo está activo', () => {
  expect(withTrackFlag('<p>x</p>', true)).toBe(`${TRACK_SENTINEL}<p>x</p>`);
  expect(withTrackFlag('<p>x</p>', false)).toBe('<p>x</p>'); // off: contenido intacto
  // La sentinela es un comentario HTML (no se renderiza ni cuenta como palabra).
  expect(TRACK_SENTINEL.startsWith('<!--')).toBe(true);
});

test('readTrackFlag hace round-trip del flag y retira la sentinela', () => {
  expect(readTrackFlag(`${TRACK_SENTINEL}<p>x</p>`)).toEqual({ on: true, html: '<p>x</p>' });
  expect(readTrackFlag('<p>x</p>')).toEqual({ on: false, html: '<p>x</p>' });
  // Ida y vuelta con cualquier flag: el HTML del cuerpo sobrevive idéntico.
  for (const on of [true, false]) {
    const body = '<p>hola <b>mundo</b></p>';
    expect(readTrackFlag(withTrackFlag(body, on))).toEqual({ on, html: body });
  }
});

test('readTrackFlag solo consume una sentinela AL FRENTE (no una interior)', () => {
  const interior = `<p>a${TRACK_SENTINEL}b</p>`; // sentinela suelta en medio: no es el flag
  expect(readTrackFlag(interior)).toEqual({ on: false, html: interior });
});

/* ---------- Resolver marcas: aceptar / rechazar (hallazgo 2) ---------- */

test('aceptar desenvuelve la inserción y elimina el borrado', () => {
  const html = '<p>' + insertionHtml('nuevo', ANA, 1) + deletionHtml('viejo', ANA, 2) + '</p>';
  expect(acceptTrackedChanges(html)).toBe('<p>nuevo</p>'); // se aplica el cambio
});

test('rechazar elimina la inserción y restaura el borrado', () => {
  const html = '<p>' + insertionHtml('nuevo', ANA, 1) + deletionHtml('viejo', ANA, 2) + '</p>';
  expect(rejectTrackedChanges(html)).toBe('<p>viejo</p>'); // vuelve al estado previo
});

test('resolver conserva texto y marcas sin data-track (conservador, anti-XSS)', () => {
  const marca = insertionHtml('1 < 2 & "x"', { name: 'Ana', id: 'u1' }, 3); // texto escapado
  const html = '<p>antes ' + marca + ' <ins>manual</ins> después</p>'; // <ins> sin data-track: intacto
  const acc = acceptTrackedChanges(html);
  expect(acc).toBe('<p>antes 1 &lt; 2 &amp; &quot;x&quot; <ins>manual</ins> después</p>');
  // El <ins> del usuario (sin rastreo) no se toca ni al aceptar ni al rechazar.
  expect(rejectTrackedChanges(html)).toContain('<ins>manual</ins>');
});

test('resolver un documento sin marcas es identidad (la UI evita el aviso falso)', () => {
  const doc = '<p>Hola <b>mundo</b></p>';
  expect(acceptTrackedChanges(doc)).toBe(doc);
  expect(rejectTrackedChanges(doc)).toBe(doc);
});

/* ---------- Contrato end-to-end (spec de coalesceMark, hallazgo 3) ----------
 * teclear→<ins>, fusión de una racha y borrar→<del> SIN perder texto. Es la
 * especificación PURA que el pegamento vivo (coalesceMark, sin entorno DOM en la
 * suite) debe respetar: aceptar deja lo tecleado; rechazar deja lo original. */
test('teclear (fusión) + borrar: aceptar deja lo nuevo, rechazar lo original', () => {
  // «Teclea» a y b (mismo autor, contiguas) → una sola <ins>ab</ins> (fusión).
  const tecleado = mergeAdjacentTrackMarks(insertionHtml('a', ANA, 1) + insertionHtml('b', ANA, 2));
  expect(tecleado).toContain('>ab</ins>');
  expect((tecleado.match(/<ins/g) ?? []).length).toBe(1); // fusionadas, sin duplicar
  // «Borra» el texto original x → <del>x</del>. Documento intermedio con ambas marcas.
  const doc = '<p>' + tecleado + deletionHtml('x', ANA, 3) + '</p>';
  expect(acceptTrackedChanges(doc)).toBe('<p>ab</p>'); // queda lo tecleado, sin la x borrada
  expect(rejectTrackedChanges(doc)).toBe('<p>x</p>'); // vuelve la x, se va lo tecleado
});

/* ---------- Panel de revisión: listar y resolver UNA marca ---------- */

test('listTrackedChanges lista en orden de documento con id, tipo, autor y extracto', () => {
  const html =
    '<p>hola ' + insertionHtml('mundo', ANA, 1) + ' ' + deletionHtml('viejo', BEA, 2) + '</p>';
  const changes = listTrackedChanges(html);
  expect(changes).toEqual([
    { id: 0, type: 'insert', author: 'Ana', text: 'mundo' },
    { id: 1, type: 'delete', author: 'Bea', text: 'viejo' },
  ]);
  // La longitud ES el recuento; los ids son contiguos 0..n-1.
  expect(changes.length).toBe(2);
});

test('listTrackedChanges des-escapa autor y texto (para pintarlos como texto llano)', () => {
  const evil: TrackAuthor = { name: 'A & <B>', id: 'x' };
  const [change] = listTrackedChanges(insertionHtml('1 < 2 & "z"', evil, 1));
  expect(change).toEqual({ id: 0, type: 'insert', author: 'A & <B>', text: '1 < 2 & "z"' });
});

test('listTrackedChanges ignora <ins>/<del> sin data-track y marcas con etiquetas', () => {
  const manual = '<p><ins>manual</ins><del>x</del></p>'; // sin data-track: no son cambios
  expect(listTrackedChanges(manual)).toEqual([]);
  // Marca RASTREADA pero con un hijo elemento (Intro/formato inline/comentar sobre
  // el tramo): el scanner de texto plano `[^<]*` tampoco la cuenta. Es el conjunto
  // que el panel debe filtrar en el DOM (firstElementChild===null) para no desincronizar.
  const anidada = '<ins data-track data-author-id="u1"><b>x</b></ins>';
  expect(listTrackedChanges(anidada)).toEqual([]);
  // El ordinal SOLO cuenta marcas rastreadas de texto plano: aquí una, con id 0.
  const mixto = manual + anidada + insertionHtml('real', ANA, 1);
  expect(listTrackedChanges(mixto)).toEqual([
    { id: 0, type: 'insert', author: 'Ana', text: 'real' },
  ]);
});

test('resolveTrackedChange resuelve SOLO la marca del id dado', () => {
  const doc =
    '<p>' +
    insertionHtml('uno', ANA, 1) +
    insertionHtml('dos', ANA, 2) +
    deletionHtml('tres', ANA, 3) +
    '</p>';
  // Aceptar la inserción 0: queda su texto; el resto de marcas, intactas.
  const acc0 = resolveTrackedChange(doc, 0, true);
  expect(acc0).toBe(
    '<p>uno' + insertionHtml('dos', ANA, 2) + deletionHtml('tres', ANA, 3) + '</p>',
  );
  // Rechazar la inserción 1: desaparece; las marcas 0 y 2, intactas.
  const rej1 = resolveTrackedChange(doc, 1, false);
  expect(rej1).toBe('<p>' + insertionHtml('uno', ANA, 1) + deletionHtml('tres', ANA, 3) + '</p>');
  // Aceptar el borrado 2: se aplica (desaparece su texto).
  const acc2 = resolveTrackedChange(doc, 2, true);
  expect(acc2).toBe('<p>' + insertionHtml('uno', ANA, 1) + insertionHtml('dos', ANA, 2) + '</p>');
  // Rechazar el borrado 2: se restaura su texto.
  const rej2 = resolveTrackedChange(doc, 2, false);
  expect(rej2).toBe(
    '<p>' + insertionHtml('uno', ANA, 1) + insertionHtml('dos', ANA, 2) + 'tres</p>',
  );
});

test('resolveTrackedChange con id fuera de rango es identidad', () => {
  const doc = '<p>' + insertionHtml('uno', ANA, 1) + '</p>';
  expect(resolveTrackedChange(doc, 5, true)).toBe(doc);
  expect(resolveTrackedChange(doc, -1, false)).toBe(doc);
});

test('resolver por id es coherente con aceptar/rechazar todos (misma numeración)', () => {
  const doc = '<p>' + insertionHtml('a', ANA, 1) + deletionHtml('b', BEA, 2) + '</p>';
  // Resolver una a una en orden inverso (para no reindexar) == resolver todas.
  let acc = doc;
  for (let id = listTrackedChanges(doc).length - 1; id >= 0; id -= 1) {
    acc = resolveTrackedChange(acc, id, true);
  }
  expect(acc).toBe(acceptTrackedChanges(doc));
  expect(acc).toBe('<p>a</p>');
});
