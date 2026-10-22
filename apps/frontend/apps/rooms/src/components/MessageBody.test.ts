// Checks del parser de Markdown ligero de MessageBody. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { parseMarkdown, emojify, extractLinks } from './MessageBody.js';

test('texto plano → un único nodo text', () => {
  expect(parseMarkdown('hola mundo')).toEqual([{ kind: 'text', text: 'hola mundo' }]);
});

test('**negrita** entre texto', () => {
  expect(parseMarkdown('un **golpe** duro')).toEqual([
    { kind: 'text', text: 'un ' },
    { kind: 'bold', text: 'golpe' },
    { kind: 'text', text: ' duro' },
  ]);
});

test('_cursiva_ sí; snake_case no', () => {
  expect(parseMarkdown('_suave_ y foo_bar_baz')).toEqual([
    { kind: 'italic', text: 'suave' },
    { kind: 'text', text: ' y foo_bar_baz' },
  ]);
});

test('el código inline gana a las marcas interiores', () => {
  expect(parseMarkdown('`a **b** _c_`')).toEqual([{ kind: 'code', text: 'a **b** _c_' }]);
});

test('bloque ``` CONSERVA la etiqueta de lenguaje, y los saltos quedan limpios', () => {
  // Este check afirmaba antes que la etiqueta se descartaba. No era una regla:
  // era una limitación. Quien escribe ```go lo escribe para que se sepa qué es,
  // y tirarlo convertía ese esfuerzo en nada — ahora rotula el bloque.
  expect(parseMarkdown('antes\n```go\nfunc main() {}\n```\ndespués')).toEqual([
    { kind: 'text', text: 'antes' },
    { kind: 'block', text: 'func main() {}', lang: 'go' },
    { kind: 'text', text: 'después' },
  ]);
});

test('un bloque sin etiqueta no se inventa una', () => {
  // `lang` ausente y no cadena vacía: el render distingue las dos cosas para poner
  // «código» de rótulo en vez de un hueco.
  expect(parseMarkdown('```\nhola\n```')).toEqual([{ kind: 'block', text: 'hola' }]);
});

test('autolink http(s) con puntuación colgante fuera de la URL', () => {
  expect(parseMarkdown('mira https://sytue.eu/x, ya')).toEqual([
    { kind: 'text', text: 'mira ' },
    { kind: 'link', href: 'https://sytue.eu/x' },
    { kind: 'text', text: ', ya' },
  ]);
});

test('protocolo pelado no es enlace', () => {
  expect(parseMarkdown('ver https://. ya').every((n) => n.kind === 'text')).toBe(true);
});

test('marcas sin cerrar quedan literales', () => {
  expect(parseMarkdown('**sin cerrar y `suelto')).toEqual([
    { kind: 'text', text: '**sin cerrar y `suelto' },
  ]);
  expect(parseMarkdown('```solo')).toEqual([{ kind: 'text', text: '```solo' }]);
});

test('el HTML nunca se interpreta: queda como texto (sin XSS)', () => {
  expect(parseMarkdown('<b>hola</b> & <script>alert(1)</script>')).toEqual([
    { kind: 'text', text: '<b>hola</b> & <script>alert(1)</script>' },
  ]);
});

test('mezcla completa en orden', () => {
  expect(parseMarkdown('`c` **b** _i_ https://x.eu')).toEqual([
    { kind: 'code', text: 'c' },
    { kind: 'text', text: ' ' },
    { kind: 'bold', text: 'b' },
    { kind: 'text', text: ' ' },
    { kind: 'italic', text: 'i' },
    { kind: 'text', text: ' ' },
    { kind: 'link', href: 'https://x.eu' },
  ]);
});

// --- GFM añadido: tachado, encabezados, listas y citas ---

test('~~tachado~~ inline', () => {
  expect(parseMarkdown('un ~~error~~ ya')).toEqual([
    { kind: 'text', text: 'un ' },
    { kind: 'strike', text: 'error' },
    { kind: 'text', text: ' ya' },
  ]);
  // Sin cerrar → literal (como el resto de marcas).
  expect(parseMarkdown('~~suelto')).toEqual([{ kind: 'text', text: '~~suelto' }]);
});

test('encabezados #/##/### con inline dentro; # sin espacio no es encabezado', () => {
  expect(parseMarkdown('# Grande')).toEqual([
    { kind: 'heading', level: 1, children: [{ kind: 'text', text: 'Grande' }] },
  ]);
  expect(parseMarkdown('### Con **negrita**')).toEqual([
    {
      kind: 'heading',
      level: 3,
      children: [
        { kind: 'text', text: 'Con ' },
        { kind: 'bold', text: 'negrita' },
      ],
    },
  ]);
  expect(parseMarkdown('#etiqueta')).toEqual([{ kind: 'text', text: '#etiqueta' }]);
});

test('lista con viñetas (- y *), con inline por ítem', () => {
  expect(parseMarkdown('- uno\n- **dos**\n* tres')).toEqual([
    {
      kind: 'ul',
      items: [
        [{ kind: 'text', text: 'uno' }],
        [{ kind: 'bold', text: 'dos' }],
        [{ kind: 'text', text: 'tres' }],
      ],
    },
  ]);
});

test('lista numerada conserva el inicio; 1.5 no es ítem', () => {
  expect(parseMarkdown('2. dos\n3. tres')).toEqual([
    {
      kind: 'ol',
      start: 2,
      items: [[{ kind: 'text', text: 'dos' }], [{ kind: 'text', text: 'tres' }]],
    },
  ]);
  expect(parseMarkdown('1.5 no es lista')).toEqual([{ kind: 'text', text: '1.5 no es lista' }]);
});

test('cita > con varias líneas e inline; el parseo inline sigue funcionando', () => {
  expect(parseMarkdown('> hola _mundo_\n> segunda')).toEqual([
    {
      kind: 'quote',
      children: [
        { kind: 'text', text: 'hola ' },
        { kind: 'italic', text: 'mundo' },
        { kind: 'text', text: '\nsegunda' },
      ],
    },
  ]);
});

test('bloques no rompen el texto plano circundante (no-regresión)', () => {
  expect(parseMarkdown('antes\n- item\ndespués')).toEqual([
    { kind: 'text', text: 'antes' },
    { kind: 'ul', items: [[{ kind: 'text', text: 'item' }]] },
    { kind: 'text', text: 'después' },
  ]);
});

test('seguridad: HTML dentro de cita/lista queda como texto (sin XSS)', () => {
  expect(parseMarkdown('> <script>alert(1)</script> & <b>x</b>')).toEqual([
    { kind: 'quote', children: [{ kind: 'text', text: '<script>alert(1)</script> & <b>x</b>' }] },
  ]);
  expect(parseMarkdown('- <img src=x onerror=y> & z')).toEqual([
    { kind: 'ul', items: [[{ kind: 'text', text: '<img src=x onerror=y> & z' }]] },
  ]);
});

// --- Emoji por :shortcode: ---

test('emojify sustituye shortcodes conocidos por Unicode', () => {
  expect(emojify(':smile:')).toBe('😄');
  expect(emojify('bien hecho :+1: :tada:')).toBe('bien hecho 👍 🎉');
  expect(emojify('me encanta :heart: el :fire:')).toBe('me encanta ❤️ el 🔥');
});

test('emojify deja literal un shortcode desconocido y el texto sin dos puntos', () => {
  expect(emojify('nada :desconocido_xyz: aquí')).toBe('nada :desconocido_xyz: aquí');
  expect(emojify('hora 12:30:00 en punto')).toBe('hora 12:30:00 en punto');
  expect(emojify('sin shortcodes')).toBe('sin shortcodes');
});

// --- extractLinks (unfurl, Ola 16): reusa el parser; solo autolinks reales ---

test('extractLinks toma las URLs de autolinks, deduplicadas y en orden', () => {
  expect(extractLinks('mira https://a.eu y https://b.eu y otra vez https://a.eu')).toEqual([
    'https://a.eu',
    'https://b.eu',
  ]);
});

test('extractLinks respeta el tope (por defecto 2)', () => {
  expect(extractLinks('https://a.eu https://b.eu https://c.eu')).toEqual([
    'https://a.eu',
    'https://b.eu',
  ]);
  expect(extractLinks('https://a.eu https://b.eu https://c.eu', 1)).toEqual(['https://a.eu']);
});

test('extractLinks NO previsualiza URLs dentro de código o bloques', () => {
  expect(extractLinks('`https://code.eu`')).toEqual([]);
  expect(extractLinks('```\nhttps://block.eu\n```')).toEqual([]);
});

test('extractLinks encuentra URLs dentro de listas y citas', () => {
  expect(extractLinks('- item https://list.eu')).toEqual(['https://list.eu']);
  expect(extractLinks('> cita https://quote.eu')).toEqual(['https://quote.eu']);
});

test('extractLinks sin URLs → vacío', () => {
  expect(extractLinks('solo texto, sin enlaces')).toEqual([]);
});

// Enlaces con texto, `[texto](url)`. Sin esto, quien no sabe markdown no podía
// poner un enlace legible: se veía el paréntesis y la URL cruda en medio de la
// frase, y el vocabulario de Slack/Teams (pegar un enlace sobre la selección)
// simplemente no respondía.
test('[texto](url) → enlace con su texto, no la URL cruda', () => {
  expect(parseMarkdown('mira [el informe](https://sytue.eu/q3) hoy')).toEqual([
    { kind: 'text', text: 'mira ' },
    { kind: 'link', href: 'https://sytue.eu/q3', text: 'el informe' },
    { kind: 'text', text: ' hoy' },
  ]);
});

test('el enlace con texto gana al autolink: la URL no se parte en el paréntesis', () => {
  const nodos = parseMarkdown('[web](https://sytue.eu)');
  expect(nodos).toEqual([{ kind: 'link', href: 'https://sytue.eu', text: 'web' }]);
});

// Un esquema que no sea http/https NO puede convertirse en un <a href>: sería un
// XSS de un solo mensaje. El patrón lo exige, así que cae a texto plano.
test('[texto](javascript:…) NO produce un enlace', () => {
  const nodos = parseMarkdown('[pincha](javascript:alert(1))');
  expect(nodos.some((n) => n.kind === 'link')).toBe(false);
});

test('un autolink suelto sigue mostrando su URL', () => {
  expect(parseMarkdown('https://sytue.eu')).toEqual([{ kind: 'link', href: 'https://sytue.eu' }]);
});
