import { expect, test } from 'bun:test';
import { cuerpoExport, exportExt, exportFilename } from './exportName';

test('cada formato trae su extensión, y el PDF no es «bin»', () => {
  expect(exportExt('document', 'pdf')).toBe('pdf');
  expect(exportExt('spreadsheet', 'pdf')).toBe('pdf');
  expect(exportExt('document', 'ooxml')).toBe('docx');
  expect(exportExt('spreadsheet', 'ooxml')).toBe('xlsx');
  expect(exportExt('presentation', 'ooxml')).toBe('pptx');
  expect(exportExt('presentation', 'odf')).toBe('odp');
  expect(exportExt('document', 'markdown')).toBe('md');
});

test('el nombre se sanea, se acorta y cae a «documento» si está vacío', () => {
  expect(exportFilename('Informe: 2026/Q3', 'document', 'pdf')).toBe('Informe_ 2026_Q3.pdf');
  expect(exportFilename('   ', 'spreadsheet', 'ooxml')).toBe('documento.xlsx');
  expect(exportFilename('x'.repeat(200), 'document', 'html')).toBe(`${'x'.repeat(120)}.html`);
});

test('«PDF accesible» apagado solo viaja con el PDF; encendido no añade nada', () => {
  expect(cuerpoExport('pdf', false, true)).toEqual({ format: 'pdf' });
  expect(cuerpoExport('pdf', false, false)).toEqual({ format: 'pdf', accessible: false });
  expect(cuerpoExport('ooxml', true, false)).toEqual({ format: 'ooxml', template: true });
});
