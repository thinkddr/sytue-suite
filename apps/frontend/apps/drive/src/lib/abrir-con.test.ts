/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import { appQueAbre, subibleACast, urlDeApertura, urlDeSubirACast } from './abrir-con.js';

const todo = { conMiniatura: true, conEditor: true };
const nada = { conMiniatura: false, conEditor: false };

describe('appQueAbre', () => {
  it('un PDF va a CertiPDF, haya o no editor ofimático', () => {
    expect(appQueAbre('pdf', nada)).toBe('pdf');
    expect(appQueAbre('pdf', todo)).toBe('pdf');
  });

  it('una imagen va a Fotos solo si el servicio sabe leer ese formato', () => {
    expect(appQueAbre('image', todo)).toBe('photos');
    // Un SVG o un AVIF no los procesa el servicio de fotos: mandarlo allí daría
    // un error tras salir de Drive, que es peor que quedarse en el visor.
    expect(appQueAbre('image', nada)).toBeNull();
  });

  it('la ofimática nativa va a Docus, Calcus o Diapos según la extensión', () => {
    expect(appQueAbre('office', { ...nada, ext: 'odt' })).toBe('docs');
    expect(appQueAbre('office', { ...nada, ext: 'docx' })).toBe('docs');
    expect(appQueAbre('office', { ...nada, ext: 'ods' })).toBe('sheets');
    expect(appQueAbre('office', { ...nada, ext: 'xlsx' })).toBe('sheets');
    expect(appQueAbre('office', { ...nada, ext: 'odp' })).toBe('slides');
    expect(appQueAbre('office', { ...nada, ext: 'pptx' })).toBe('slides');
  });

  it('los de iWork van a su app, y un .key de texto es una clave y se queda', () => {
    expect(appQueAbre('office', { ...nada, ext: 'pages' })).toBe('docs');
    expect(appQueAbre('office', { ...nada, ext: 'numbers' })).toBe('sheets');
    expect(appQueAbre('office', { ...nada, ext: 'key' })).toBe('slides');
    expect(appQueAbre('text', { ...nada, ext: 'key' })).toBeNull();
  });

  it('la ofimática restante va al editor solo si el editor declara esa extensión', () => {
    expect(appQueAbre('office', todo)).toBe('editor');
    expect(appQueAbre('office', nada)).toBeNull();
  });

  it('el texto va al editor solo si el editor lo declara', () => {
    // Un .csv en la hoja de cálculo se lee y se ordena; en el visor es un <pre>
    // con comas. Un .go o un .json no los declara nadie y se quedan.
    expect(appQueAbre('text', todo)).toBe('editor');
    expect(appQueAbre('text', nada)).toBeNull();
  });

  it('lo que no tiene visor tampoco tiene app', () => {
    expect(appQueAbre(null, todo)).toBeNull();
  });
});

describe('urlDeApertura', () => {
  it('el PDF lleva el PREFIJO del producto, que es lo que faltaba', () => {
    const u = new URL(urlDeApertura('pdf', { path: '/a b/informe.pdf' }));
    expect(u.pathname).toBe('/certipdf/pdf');
    expect(u.searchParams.get('path')).toBe('/a b/informe.pdf');
    expect(u.searchParams.get('volver')).toBe('/a b');
  });

  it('la foto viaja por id y con su vuelta', () => {
    const u = new URL(urlDeApertura('photos', { path: '/Fotos/playa.jpg', id: 'abc-123' }));
    expect(u.pathname).toBe('/photos/abrir');
    expect(u.searchParams.get('file')).toBe('abc-123');
    expect(u.searchParams.get('volver')).toBe('/Fotos');
  });

  it('docs, sheets y slides abren por open-from-drive preservando volver', () => {
    const docUrl = new URL(urlDeApertura('docs', { path: '/Docs/acta.odt', id: 'doc-123' }));
    expect(docUrl.pathname).toBe('/docus/open-from-drive');
    expect(docUrl.searchParams.get('file')).toBe('doc-123');
    expect(docUrl.searchParams.get('volver')).toBe('/Docs');

    const sheetUrl = new URL(
      urlDeApertura('sheets', { path: '/Finanzas/cuentas.xlsx', id: 'sheet-123' }),
    );
    expect(sheetUrl.pathname).toBe('/calcus/open-from-drive');
    expect(sheetUrl.searchParams.get('file')).toBe('sheet-123');
    expect(sheetUrl.searchParams.get('volver')).toBe('/Finanzas');

    const slideUrl = new URL(
      urlDeApertura('slides', { path: '/Pres/diapos.odp', id: 'slide-123' }),
    );
    expect(slideUrl.pathname).toBe('/diapos/open-from-drive');
    expect(slideUrl.searchParams.get('file')).toBe('slide-123');
    expect(slideUrl.searchParams.get('volver')).toBe('/Pres');
  });

  it('con org la organización encabeza el path del destino (ADR 0056)', () => {
    const foto = new URL(urlDeApertura('photos', { path: '/Fotos/x.jpg', id: 'f-123' }, 'acme'));
    expect(foto.pathname).toBe('/photos/acme/abrir');
    expect(foto.searchParams.get('file')).toBe('f-123');

    const doc = new URL(urlDeApertura('docs', { path: '/Docs/a.odt', id: 'd-1' }, 'acme'));
    expect(doc.pathname).toBe('/docus/acme/open-from-drive');

    const pdf = new URL(urlDeApertura('pdf', { path: '/a b/informe.pdf' }, 'acme'));
    expect(pdf.pathname).toBe('/certipdf/acme/pdf');
    expect(pdf.searchParams.get('volver')).toBe('/a b');
  });
});

describe('Subir a Cast', () => {
  const video = { id: 'v-1', isDirectory: false, contentType: 'video/mp4' };

  it('se ofrece solo con un fichero de vídeo que tiene id', () => {
    expect(subibleACast(video)).toBe(true);
    expect(subibleACast({ ...video, contentType: 'VIDEO/QuickTime' })).toBe(true);
    // Lo que el servicio de Cast rechazaría no se ofrece.
    expect(subibleACast({ ...video, contentType: 'audio/mpeg' })).toBe(false);
    expect(subibleACast({ id: 'v-1', isDirectory: false })).toBe(false);
    expect(subibleACast({ isDirectory: false, contentType: 'video/mp4' })).toBe(false);
    expect(subibleACast({ ...video, isDirectory: true })).toBe(false);
  });

  it('lleva a la subida de Cast con el id, en el host y con el PREFIJO de Cast', () => {
    const u = new URL(urlDeSubirACast('7d1c6a3e-0b8e-4f39-9d3a-4a3e8a1f2b10'));
    expect(u.hostname.startsWith('cast.')).toBe(true);
    expect(u.pathname).toBe('/cast/upload');
    expect(u.searchParams.get('drive')).toBe('7d1c6a3e-0b8e-4f39-9d3a-4a3e8a1f2b10');
  });

  it('con org la organización encabeza el path (ADR 0056)', () => {
    const u = new URL(urlDeSubirACast('v-1', 'acme'));
    expect(u.pathname).toBe('/cast/acme/upload');
    expect(u.searchParams.get('drive')).toBe('v-1');
  });
});
