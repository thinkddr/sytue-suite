import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { countWords } from './useDocument.js';

if (!globalThis.document) GlobalRegistrator.register();

test('countWords omite el contenido de scripts incluso con espacios en el cierre', () => {
  expect(
    countWords(
      '<p>uno</p><p>dos<strong>tres</strong></p><script>contenido interno</script ><p>cuatro</p>',
      'document',
    ),
  ).toBe(4);
});

test('countWords no parsea otros tipos de documento', () => {
  expect(countWords('<script>no cuenta</script>', 'spreadsheet')).toBe(0);
});
