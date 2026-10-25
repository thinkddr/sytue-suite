/**
 * El enlace entre notas es lo que hace de Notepads un wiki: si el destino se
 * calcula mal, el enlace lleva a otra nota, a ningún sitio o —peor— parece que
 * funciona en el editor y se rompe en el fichero exportado.
 */
import { expect, test } from 'bun:test';
import type { Note } from '@sytue/api-clients';
import {
  conAnclas,
  destinoInterno,
  enlacesAAnclas,
  enlacesDeNota,
  sinEtiquetas,
  sincronizarEnlaces,
  slugDe,
  titulosDe,
  urlDeNota,
} from '../client/enlaces.js';

const BASE = 'https://notes.sytue.eu';
const A = '11111111-1111-4111-8111-111111111111';
const B = '22222222-2222-4222-8222-222222222222';

test('el slug quita acentos y símbolos y no se alarga sin fin', () => {
  expect(slugDe('Presupuesto de la mudanza (2026)')).toBe('presupuesto-de-la-mudanza-2026');
  expect(slugDe('  Ñoño & Cía.  ')).toBe('nono-cia');
  expect(slugDe('x'.repeat(200)).length).toBe(80);
});

test('los títulos salen en orden, sin etiquetas internas y con slug único', () => {
  const t = titulosDe(
    '<h2>Cocina</h2><p>x</p><h3>Cocina</h3><h1>Salón <b>grande</b></h1><h4></h4>',
  );
  expect(t.map((x) => [x.nivel, x.texto, x.slug])).toEqual([
    [2, 'Cocina', 'cocina'],
    [3, 'Cocina', 'cocina-2'],
    [1, 'Salón grande', 'salon-grande'],
  ]);
});

test('sinEtiquetas no desescapa dos veces entidades anidadas', () => {
  expect(sinEtiquetas('&amp;lt;script&amp;gt;')).toBe('&lt;script&gt;');
});

test('sinEtiquetas parsea atributos y marcado incompleto sin reconstruir etiquetas', () => {
  expect(sinEtiquetas('<b title="a > b">uno<span>dos</span></b>')).toBe('unodos');
  expect(sinEtiquetas('<<script>script>alert(1)</script>')).toBe('<');
  expect(sinEtiquetas('antes<script')).toBe('antes');
  expect(sinEtiquetas('a<!--oculto--><template>no</template><style>no</style>b')).toBe('ab');
  expect(sinEtiquetas('&#x1f600; &quot;x&quot;')).toBe('😀 "x"');
});

test('conAnclas pone el id sin tocar lo demás', () => {
  expect(conAnclas('<h2 style="color:#112233">Cocina</h2><p>a</p>')).toBe(
    '<h2 id="h-cocina" style="color:#112233">Cocina</h2><p>a</p>',
  );
});

test('la URL de una nota es absoluta y la app la reconoce como destino interno', () => {
  const u = urlDeNota(BASE, 'nb', A, 'cocina');
  expect(u).toBe(`${BASE}/?notebook=nb&note=${A}&h=cocina`);
  expect(destinoInterno(u, BASE)).toEqual({ notebook: 'nb', note: A, h: 'cocina' });
  // Otro origen NO es interno aunque lleve los mismos parámetros: sería seguir
  // un enlace ajeno con la sesión de aquí.
  expect(destinoInterno(`https://otro.example/?notebook=nb&note=${A}`, BASE)).toBeNull();
  expect(destinoInterno('javascript:alert(1)', BASE)).toBeNull();
});

test('los enlaces del texto se vuelven links[] de tipo note, sin repetir', () => {
  const html =
    `<p><a href="${BASE}/?notebook=nb&amp;note=${A}">Cocina</a> y ` +
    `<a href="${BASE}/?notebook=nb&amp;note=${A}&amp;h=x">otra vez</a> y ` +
    `<a href="https://sytue.eu">fuera</a></p>`;
  const l = enlacesDeNota(html, BASE);
  expect(l).toHaveLength(1);
  expect(l[0]).toMatchObject({ target_kind: 'note', target_id: A, label: 'Cocina' });
  // Los enlaces de otros tipos que la nota ya tenía se conservan.
  const note = {
    links: [{ target_kind: 'drive_file', target_id: 'f', label: 'Doc' }],
  } as unknown as Note;
  expect(sincronizarEnlaces(note, html, BASE).map((x) => x.target_kind)).toEqual([
    'drive_file',
    'note',
  ]);
});

test('en la página pública, los enlaces a notas presentes son anclajes y el resto no', () => {
  const html = `<a href="${BASE}/?notebook=nb&amp;note=${A}">A</a><a href="${BASE}/?notebook=nb&amp;note=${B}">B</a>`;
  expect(enlacesAAnclas(html, new Set([A]))).toBe(
    `<a href="#nota-${A}">A</a><a href="${BASE}/?notebook=nb&amp;note=${B}">B</a>`,
  );
});
