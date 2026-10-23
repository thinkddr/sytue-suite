// Checks del saneador del compositor de Sytue Mail. Ejecutar: `bun test`.
//
// Este fichero es de SEGURIDAD antes que de formato. El cuerpo se compone en un
// contenteditable donde se pega lo que sea —un correo recibido, una hoja de
// Excel, una página web— y lo que sobreviva a `sanitizeHtml` se inserta en el
// DOM del propio compositor y luego se envía. Cada caso de aquí es un vector que
// se probó a mano contra la lista blanca.
import { expect, test } from 'bun:test';
import { setupJsdom } from '../../test/dom.js';

setupJsdom();
import {
  sanitizeHtml,
  htmlToText,
  isHtmlEmpty,
  textToHtml,
  ficheroDeDatos,
  incrustarImagenesPegadas,
} from './richtext.js';

// --- lo que TIENE que pasar (formato de correo de verdad) -------------------

test('una tabla sobrevive entera, que es media bandeja de entrada', () => {
  // Antes se desenvolvía: quien pegaba una tabla de Excel enviaba una lista de
  // palabras sueltas y no se enteraba hasta que le contestaban.
  const html = sanitizeHtml(
    '<table border="1"><thead><tr><th scope="col">Mes</th></tr></thead>' +
      '<tbody><tr><td colspan="2" align="right">Enero</td></tr></tbody></table>',
  );
  expect(html).toContain('<table');
  expect(html).toContain('<th');
  expect(html).toContain('colspan="2"');
  expect(html).toContain('Enero');
});

test('los estilos en línea de la lista blanca se conservan', () => {
  // En correo NO hay hoja de estilos: sin `style` en línea no hay color ni
  // alineación ni tipografía en Gmail ni en Outlook.
  const html = sanitizeHtml(
    '<p style="color: #b45309; text-align: center; font-family: Georgia, serif">hola</p>',
  );
  expect(html).toContain('color: #b45309');
  expect(html).toContain('text-align: center');
  expect(html).toContain('font-family: Georgia, serif');
});

test('una imagen https o cid pasa; el resto se descarta ENTERA', () => {
  expect(sanitizeHtml('<img src="https://x.example/a.png" alt="a">')).toContain('src="https://');
  expect(sanitizeHtml('<img src="cid:logo@sytue">')).toContain('cid:logo@sytue');
  // Sin `src` admisible no se deja el hueco: sería un icono roto en el mensaje.
  expect(sanitizeHtml('<img src="http://x.example/a.png">')).not.toContain('<img');
  expect(sanitizeHtml('<img src="data:image/svg+xml;base64,AAA">')).not.toContain('<img');
});

// --- lo que NO puede pasar --------------------------------------------------

test('el rastreador por CSS no entra: `url(` se rechaza en cualquier propiedad', () => {
  // `background:url(...)` confirma la apertura del correo y filtra la IP sin que
  // haya una sola imagen visible. Es el motivo de que `url(` muera SIEMPRE.
  const html = sanitizeHtml('<p style="background-color: url(https://espia.example/p.gif)">x</p>');
  expect(html).not.toContain('url(');
  expect(html).not.toContain('espia.example');
});

test('las propiedades que permiten tapar el mensaje no están en la lista', () => {
  // position/z-index sirven para superponer contenido sobre el resto del correo,
  // que es phishing barato dentro de un mensaje legítimo.
  const html = sanitizeHtml(
    '<div style="position: fixed; top: 0; z-index: 9999; color: red">tapa</div>',
  );
  expect(html).not.toContain('position');
  expect(html).not.toContain('z-index');
  expect(html).toContain('color: red'); // lo legítimo del mismo atributo sí queda
});

test('expression(), javascript: y los comentarios CSS no sobreviven', () => {
  expect(sanitizeHtml('<p style="width: expression(alert(1))">x</p>')).not.toContain('expression');
  expect(sanitizeHtml('<p style="color: javascript:alert(1)">x</p>')).not.toContain('javascript:');
  // El comentario es la vía clásica de partir una palabra prohibida en dos.
  expect(sanitizeHtml('<p style="color: re/**/d">x</p>')).not.toContain('/*');
});

test('script e iframe se van CON su contenido, sin llevarse el resto', () => {
  const html = sanitizeHtml('<p>antes</p><script>alert(1)</script><iframe></iframe><p>después</p>');
  expect(html).toContain('antes');
  expect(html).toContain('después'); // lo de después del script NO se pierde
  expect(html).not.toContain('alert');
  expect(html).not.toContain('<iframe');
});

test('un svg no sobrevive, ni con un script dentro', () => {
  const html = sanitizeHtml('<svg><script>alert(2)</script></svg>');
  expect(html).not.toContain('<svg');
  expect(html).not.toContain('alert');
});

test('los manejadores en línea no pasan ni en las etiquetas permitidas', () => {
  const html = sanitizeHtml('<td onclick="alert(1)" onmouseover="alert(2)">x</td>');
  expect(html).not.toContain('onclick');
  expect(html).not.toContain('onmouseover');
});

test('un enlace javascript: pierde el href pero conserva el texto', () => {
  const html = sanitizeHtml('<a href="javascript:alert(1)">pincha</a>');
  expect(html).not.toContain('javascript:');
  expect(html).toContain('pincha');
});

// --- utilidades del borrador ------------------------------------------------

test('el texto plano del borrador respeta los saltos y no arrastra marcado', () => {
  expect(htmlToText('<p>uno</p><p>dos</p>')).toBe('uno\ndos');
  expect(htmlToText('<table><tr><td>a</td></tr></table>')).toContain('a');
  expect(isHtmlEmpty('<p> </p><br>')).toBe(true);
  expect(isHtmlEmpty('<p>x</p>')).toBe(false);
  expect(textToHtml('a<b>\nc')).toBe('a&lt;b&gt;<br>c');
});

// --- las imágenes `data:` pegadas entran como partes del mensaje -------------

test('una imagen data: de mapa de bits es un fichero; un SVG o un base64 roto, no', async () => {
  const png = ficheroDeDatos('data:image/png;base64,iVBO\nRw0K', 1);
  expect(png?.name).toBe('image001.png');
  expect(png?.type).toBe('image/png');
  expect([...new Uint8Array(await png!.arrayBuffer())]).toEqual([
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a,
  ]);
  expect(ficheroDeDatos('data:image/jpeg;base64,/9j/', 12)?.name).toBe('image012.jpg');
  expect(ficheroDeDatos('data:image/svg+xml;base64,PHN2Zz4=', 1)).toBeNull();
  expect(ficheroDeDatos('data:image/png;base64,%%%', 1)).toBeNull();
  expect(ficheroDeDatos('data:image/png;base64,', 1)).toBeNull();
  expect(ficheroDeDatos('https://x/a.png', 1)).toBeNull();
});

test('pegar: las data: pasan a cid:, y lo demás se queda para el saneado', async () => {
  const pedidos: string[] = [];
  const h = await incrustarImagenesPegadas(
    '<p>Foto <img src="data:image/png;base64,iVBORw0K" width="4"> y <img src="https://x/a.png"></p>' +
      '<img src="data:image/svg+xml;base64,PHN2Zz4="><noscript><img src="data:image/png;base64,iVBORw0K"></noscript>',
    async (f) => {
      pedidos.push(f.name);
      return `c${pedidos.length}`;
    },
  );
  expect(pedidos).toEqual(['image001.png']);
  const limpio = sanitizeHtml(h);
  expect(limpio).toContain('<img src="cid:c1" width="4">');
  expect(limpio).toContain('<img src="https://x/a.png">');
  expect(limpio).not.toContain('data:');
  const igual = '<p>sin imágenes en línea</p>';
  expect(await incrustarImagenesPegadas(igual, async () => 'x')).toBe(igual);
});

test('clipboard image conversion never returns active markup, even without inline images', async () => {
  for (const inline of ['', '<img src="data:image/png;base64,iVBORw0K" onerror="alert(1)">']) {
    const html = await incrustarImagenesPegadas(
      '<p onclick="alert(1)">safe</p><script>alert(1)</script>' +
        '<img src="https://x/a.png" onerror="alert(1)">' +
        inline,
      async () => 'embedded',
    );
    expect(html).toContain('safe');
    expect(html).not.toContain('onclick');
    expect(html).not.toContain('onerror');
    expect(html).not.toContain('<script');
    if (inline) expect(html).toContain('cid:embedded');
  }
});

test('sanitizing hostile clipboard markup remains safe after reparsing', () => {
  const html = sanitizeHtml(
    '<math><mtext><table><mglyph><style><!--</style>' +
      '<img title="--><img src=x onerror=alert(1)>">' +
      '<svg><g onload="alert(1)"></g></svg><a href="javascript:alert(1)">link</a>',
  );
  const host = document.createElement('div');
  host.innerHTML = html;
  expect(host.querySelector('script, math, svg, style, [onerror], [onload]')).toBeNull();
  expect(host.querySelector('[href^="javascript:"]')).toBeNull();
});

test('failed inline attachment conversion drops the data image safely', async () => {
  const html = await incrustarImagenesPegadas(
    '<p>text</p><img src="data:image/png;base64,iVBORw0K" onerror="alert(1)">',
    async () => null,
  );
  expect(html).toBe('<p>text</p>');
});
