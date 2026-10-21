// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { describe, expect, it } from 'bun:test';
import { OFFICE_EXPORTS, OFFICE_EXPORT_FORMATS, officeExportAvailable } from './office.js';
import catalog from './office-catalog.json';

describe('Office command catalog', () => {
  it('versiona el mismo orden y conjunto de cinco formatos de web y Fyne', () => {
    expect(catalog.version).toBe(1);
    expect(OFFICE_EXPORT_FORMATS).toEqual(['pdf', 'ooxml', 'odf', 'html', 'markdown']);
    expect(new Set(OFFICE_EXPORT_FORMATS).size).toBe(OFFICE_EXPORTS.length);
  });
  it('nombra la descarga por producto y aplica disponibilidad sin conceder permisos', () => {
    for (const command of OFFICE_EXPORTS) {
      for (const kind of ['document', 'spreadsheet', 'presentation'] as const) {
        expect(command.labels[kind]).toStartWith('file.download');
      }
      expect(officeExportAvailable(command, false, false)).toBe(true);
      expect(officeExportAvailable(command, true, false)).toBe(false);
      expect(officeExportAvailable(command, false, true)).toBe(false);
    }
    expect(OFFICE_EXPORTS.find((c) => c.id === 'ooxml')?.labels).toEqual({
      document: 'file.downloadDocx',
      spreadsheet: 'file.downloadXlsx',
      presentation: 'file.downloadPptx',
    });
  });
});
