import { describe, expect, test } from 'bun:test';
import { CSP_INCRUSTADO, cabecerasDeRuta } from './cabeceras.js';

const MEDIOS = 'camera=(self)';

describe('cabecerasDeRuta', () => {
  test('solo /cast/embed/<id> se deja encuadrar', () => {
    expect(cabecerasDeRuta('/cast/embed/7b0c', MEDIOS)).toEqual({
      'content-security-policy': CSP_INCRUSTADO,
      'x-frame-options': '',
    });
    expect(CSP_INCRUSTADO).toContain('frame-ancestors *');
  });

  test('ninguna otra ruta hereda la excepción del incrustado', () => {
    for (const ruta of [
      '/cast/watch/7b0c',
      '/cast/embed',
      '/cast/embed/7b0c/otra',
      '/cast/corporate/embed/7b0c',
      '/cast/',
      '/cast/api/v1/videos',
      '/cast/manage/7b0c',
    ]) {
      expect(cabecerasDeRuta(ruta, MEDIOS)['content-security-policy']).toBeUndefined();
    }
  });

  test('cámara y micrófono solo en el estudio, con o sin organización', () => {
    expect(cabecerasDeRuta('/cast/studio', MEDIOS)).toEqual({ 'permissions-policy': MEDIOS });
    expect(cabecerasDeRuta('/cast/corporate/studio', MEDIOS)).toEqual({
      'permissions-policy': MEDIOS,
    });
    expect(cabecerasDeRuta('/cast/upload', MEDIOS)).toEqual({});
  });
});
