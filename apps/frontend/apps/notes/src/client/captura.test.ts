/**
 * La captura decide qué NACE de lo que se suelta: un enlace o un texto. Si se
 * equivoca no hay error, hay una nota rara — una URL escrita como párrafo, o un
 * `javascript:` convertido en enlace pulsable.
 */
import { expect, test } from 'bun:test';
import { escapar, interpretarCaptura, textoAHtml } from '../client/captura.js';

test('una URL sola es una nota de enlace con el host de título', () => {
  expect(interpretarCaptura('  https://sytue.eu/docs?x=1  ')).toEqual({
    kind: 'link',
    title: 'sytue.eu',
    data: { url: 'https://sytue.eu/docs?x=1' },
  });
});

test('text/uri-list manda sobre el texto y salta los comentarios', () => {
  const c = interpretarCaptura('Documentación de Sytue', '# marcador\r\nhttps://sytue.eu/docs\r\n');
  expect(c?.kind).toBe('link');
  expect(c && 'data' in c ? c.data.url : '').toBe('https://sytue.eu/docs');
});

test('solo http y https son enlaces; lo demás es texto inerte', () => {
  for (const malo of ['javascript:alert(1)', 'data:text/html,hola', 'ftp://h/x', 'https://']) {
    const c = interpretarCaptura(malo);
    expect(c?.kind).toBe('text');
  }
  const c = interpretarCaptura('javascript:alert(1)');
  expect(c && 'content' in c ? c.content : '').toBe('<p>javascript:alert(1)</p>');
});

test('un texto con una URL dentro sigue siendo texto', () => {
  const c = interpretarCaptura('mira https://sytue.eu y dime');
  expect(c?.kind).toBe('text');
});

test('el texto se vuelve párrafos escapados: <script> no sobrevive', () => {
  expect(textoAHtml('Hola <b>mundo</b>\nsegunda\n\nOtro & "párrafo"')).toBe(
    '<p>Hola &lt;b&gt;mundo&lt;/b&gt;<br>segunda</p><p>Otro &amp; &quot;párrafo&quot;</p>',
  );
  expect(escapar(`<script>alert('x')</script>`)).not.toContain('<');
});

test('vacío o solo espacios no crea nada', () => {
  expect(interpretarCaptura('   \n  ')).toBeNull();
  expect(interpretarCaptura('', '# solo comentario')).toBeNull();
});
