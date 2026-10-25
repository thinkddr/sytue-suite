// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { expect, test } from 'bun:test';
import { extracto } from './extracto.js';

test('plain and empty previews preserve the existing contract', () => {
  expect(extracto('')).toBe('');
  expect(extracto('<p><br></p>')).toBe('');
  expect(extracto('plain text')).toBe('plain text');
});

test('block boundaries separate words while inline formatting keeps them together', () => {
  expect(extracto('<p>one<br>two</p><div>three</div><ul><li>four</li><li>five</li></ul>')).toBe(
    'one two three four five',
  );
  expect(extracto('a<strong>b</strong><em>c</em>')).toBe('abc');
});

test('quoted angle brackets in attributes never leak into preview text', () => {
  expect(extracto('<p title="a > b">safe</p>')).toBe('safe');
  expect(extracto("<p title='a > b'>safe</p>")).toBe('safe');
});

test('malformed and unfinished markup follows HTML parsing rules', () => {
  expect(extracto('<p>one<p>two')).toBe('one two');
  expect(extracto('<script')).toBe('');
  expect(extracto('<img src=x onerror=alert(1)')).toBe('');
});

test('comments and non-visible executable or template content are excluded', () => {
  expect(
    extracto(
      '<p>safe</p><!--hidden--><script>alert(1)</script><style>secret</style>' +
        '<template>hidden<img src=x onerror=alert(1)></template><noscript>hidden</noscript>',
    ),
  ).toBe('safe');
});

test('entities decode exactly once and remain plain text', () => {
  expect(extracto('&amp;lt;script&amp;gt; &lt;b&gt; &#x1f600; &#60; &quot; &apos;')).toBe(
    '&lt;script&gt; <b> 😀 < " \'',
  );
  expect(extracto('a&nbsp;\n\t b')).toBe('a b');
});

test('previews retain their length limit and ellipsis', () => {
  expect(extracto('abcdef', 5)).toBe('abcd…');
  expect(extracto('abcdef', 1)).toBe('…');
  expect(extracto('a'.repeat(100))).toBe('a'.repeat(71) + '…');
});
