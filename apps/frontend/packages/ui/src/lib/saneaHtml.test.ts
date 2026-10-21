// El saneador parsea con `DOMParser` y el runtime de bun no trae DOM: se registra
// el que ya usa el resto de tests de @sytue/ui, con la misma guarda (registrarlo
// dos veces en el mismo proceso lanza).
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (!globalThis.document) GlobalRegistrator.register();

import { describe, expect, it } from 'bun:test';
import { saneaHtmlDeDocumento } from './saneaHtml.js';

const s = saneaHtmlDeDocumento;

describe('saneaHtmlDeDocumento: lo que ejecuta, fuera', () => {
  it('quita los manejadores de evento y deja el elemento', () => {
    const limpio = s('<img src="x" onerror="alert(1)">');
    expect(limpio).not.toContain('onerror');
    expect(limpio).toContain('<img');
    expect(s('<svg onload="alert(1)"><path d="M0 0"></path></svg>')).not.toContain('onload');
    expect(s('<p OnClick="alert(1)">x</p>')).not.toContain('nclick');
  });

  it('elimina con su contenido los elementos que ejecutan o cargan código', () => {
    for (const [html, resto] of [
      ['<p>a</p><script>alert(1)</script>', '<script'],
      ['<svg><script>alert(1)</script></svg>', 'alert'],
      ['<iframe src="javascript:alert(1)"></iframe>', '<iframe'],
      ['<iframe srcdoc="<script>alert(1)</script>"></iframe>', 'srcdoc'],
      ['<style>@import url(//x/e.css)</style>', '@import'],
      ['<object data="x"></object><embed src="x">', 'x'],
      ['<base href="//x/"><meta http-equiv="refresh" content="0;url=//x">', 'x'],
      ['<link rel="stylesheet" href="//x/e.css">', 'e.css'],
      ['<math><mtext><img src onerror="alert(1)"></mtext></math>', 'img'],
      [
        '<svg><a href="#"><animate attributeName="href" to="javascript:alert(1)"></animate>x</a></svg>',
        'animate',
      ],
      ['<svg><set attributeName="href" to="javascript:alert(1)"></set></svg>', 'javascript'],
    ] as const) {
      expect(s(html)).not.toContain(resto);
    }
  });

  it('quita lo que es texto crudo en un contexto y marcado en otro', () => {
    for (const html of [
      '<noscript><p title="</noscript><img src onerror=alert(1)>"></p></noscript>',
      '<xmp><img src onerror=alert(1)></xmp>',
      '<svg><noembed><a title="</noembed><img src onerror=alert(1)>"></a></noembed></svg>',
      '<template><img src onerror=alert(1)></template>',
    ]) {
      const limpio = s(html);
      expect(limpio).not.toContain('onerror');
      expect(limpio).not.toContain('<img');
    }
  });

  it('aplana el contenido de un <title> a texto', () => {
    const limpio = s(
      '<svg><title><a title="</title><img src onerror=alert(1)>">t</a></title></svg>',
    );
    expect(limpio).not.toContain('<a');
    expect(limpio).not.toContain('<img');
    expect(limpio).toContain('<title>t</title>');
  });

  it('quita los comentarios', () => {
    expect(s('<p>a<!-- x --></p><!--[if IE]><script>alert(1)</script><![endif]-->')).toBe(
      '<p>a</p>',
    );
  });

  it('corta los esquemas que ejecutan, también disfrazados', () => {
    for (const url of [
      'javascript:alert(1)',
      ' JaVaScRiPt:alert(1)',
      'java\tscript:alert(1)',
      '\u0001javascript:alert(1)',
      'vbscript:msgbox(1)',
      'data:text/html,<script>alert(1)</script>',
    ]) {
      const a = document.createElement('a');
      a.setAttribute('href', url);
      a.textContent = 'x';
      expect(s(a.outerHTML)).toBe('<a>x</a>');
    }
    expect(s('<svg><a xlink:href="javascript:alert(1)">x</a></svg>')).not.toContain('javascript');
    expect(
      s(
        '<form action="javascript:alert(1)"><button formaction="javascript:alert(2)">x</button></form>',
      ),
    ).not.toContain('javascript');
    expect(s('<svg><image href="javascript:alert(1)"></image></svg>')).not.toContain('javascript');
  });

  it('neutraliza un style con javascript: o expression()', () => {
    expect(s('<div style="background:url(javascript:alert(1))">x</div>')).toBe('<div>x</div>');
    expect(s('<div style="width:expression(alert(1))">x</div>')).toBe('<div>x</div>');
  });
});

describe('saneaHtmlDeDocumento: lo que es del editor, intacto', () => {
  it('conserva el formato rico', () => {
    // Por contenido y no por igualdad: el análisis normaliza detalles inocuos.
    const doc =
      '<h1 id="h-1">Título</h1><p><strong>a</strong> <em>b</em></p>' +
      '<table data-sytue="1"><tbody><tr><td colspan="2" style="text-align:center">c</td></tr></tbody></table>' +
      '<figure data-chart="bar"><svg viewBox="0 0 10 10"><path d="M0 0L10 10" fill="#333"></path></svg></figure>' +
      '<ins data-track="" data-author-id="u1">nuevo</ins><a href="/docs/edit?id=1">enlace</a>' +
      '<img src="https://cdn.example/x.png" data-wrap="inline" data-align="center">' +
      '<div data-sytue-embed="" data-instance="i" data-report="r"></div>';
    const limpio = s(doc);
    for (const t of [
      'id="h-1"',
      '<strong>a</strong>',
      'data-sytue="1"',
      'colspan="2"',
      'text-align:center',
      'data-chart="bar"',
      '<path d="M0 0L10 10" fill="#333">',
      'data-track',
      'data-author-id="u1"',
      'href="/docs/edit?id=1"',
      'src="https://cdn.example/x.png"',
      'data-wrap="inline"',
      'data-sytue-embed',
    ]) {
      expect(limpio).toContain(t);
    }
    expect(s(limpio)).toBe(limpio); // sanear dos veces no cambia nada
  });

  it('conserva <foreignObject> con su texto: es el de las formas y de SmartArt', () => {
    const forma =
      '<figure data-shape=""><svg viewBox="0 0 300 160"><ellipse cx="150" cy="80" rx="149" ry="79"></ellipse>' +
      '<foreignObject x="0" y="0" width="300" height="160"><div style="display:flex">Etiqueta</div></foreignObject></svg></figure>';
    const limpio = s(forma);
    expect(limpio.toLowerCase()).toContain('<foreignobject');
    expect(limpio).toContain('Etiqueta');
    // …pero lo que cuelga de él se sanea igual.
    const hostil = s(
      '<svg><foreignObject><img src="x" onerror="alert(1)"><script>alert(2)</script></foreignObject></svg>',
    );
    expect(hostil).not.toContain('onerror');
    expect(hostil).not.toContain('alert');
  });

  it('conserva la marca de agua de imagen: url(data:) en style y href de <image>', () => {
    const png = 'data:image/png;base64,AAAA';
    expect(s(`<div data-sytue-watermark="" style="background-image:url(${png})"></div>`)).toContain(
      png,
    );
    expect(s(`<svg><image href="${png}" x="0" y="0"></image></svg>`)).toContain(png);
    // El mismo valor como ENLACE no pasa.
    expect(s(`<a href="${png}">x</a>`)).toBe('<a>x</a>');
  });

  it('conserva los recursos de imagen, vídeo, audio y subtítulos', () => {
    expect(s('<img src="data:image/png;base64,AAAA">')).toContain('data:image/png');
    expect(s('<img src="blob:https://x/abc">')).toContain('blob:');
    expect(s('<video src="data:video/mp4;base64,AAAA"></video>')).toContain('data:video/mp4');
    expect(s('<audio><track src="data:text/vtt;charset=utf-8,WEBVTT"></audio>')).toContain(
      'data:text/vtt',
    );
    expect(s('<img src="data:text/html,<b>x</b>">')).not.toContain('data:text/html');
  });

  it('desenvuelve una etiqueta desconocida sin tirar su contenido', () => {
    expect(s('<o:p>hola <b>mundo</b></o:p>')).toBe('hola <b>mundo</b>');
    expect(s('<mi-widget onclick="alert(1)"><p>dentro</p></mi-widget>')).toBe('<p>dentro</p>');
    expect(s('<textarea>texto</textarea>')).toBe('texto');
  });

  it('paso rápido: sin `<` devuelve el texto tal cual', () => {
    expect(s('123,45')).toBe('123,45');
    expect(s('=SUMA(A1:A2)')).toBe('=SUMA(A1:A2)');
    expect(s('')).toBe('');
  });
});
