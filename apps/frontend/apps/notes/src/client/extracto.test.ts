/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { extracto } from './extracto.js';

test('separa bloques y decodifica entidades una sola vez, también sin DOM', () => {
  expect(extracto('<p>Una&nbsp;nota &amp; café</p><div>otra<br>línea</div>')).toBe(
    'Una nota & café otra línea',
  );
  expect(extracto('<p>&#60;b&#62; &amp;lt; &quot;hola&quot;</p>')).toBe('<b> &lt; "hola"');
});

test('ignora atributos, comentarios y contenido activo sin eliminar texto legítimo', () => {
  expect(
    extracto(
      '<p title="a > b">antes</p><!-- oculto --><script>alert(1)</script><style>body{}</style><template>oculto</template><p>después</p>',
    ),
  ).toBe('antes después');
  expect(extracto('<img src=x onerror=alert(1)><b>texto</b>')).toBe('texto');
});

test('el marcado malformado no se recompone al eliminar etiquetas con regex', () => {
  expect(extracto('<<script>script>alert(1)</script>')).toBe('<');
  expect(extracto('<p>antes</p><script')).toBe('antes');
});

test('el extracto se recorta al límite y conserva el texto plano', () => {
  expect(extracto('Una nota larga', 8)).toBe('Una not…');
  expect(extracto('breve', 5)).toBe('breve');
  expect(extracto('')).toBe('');
});
