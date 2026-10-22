/**
 * NEG-06: quien consulta una conexión de datos sin estar en su lista recibe un 403
 * `connection_not_allowed`. Ese aviso lo pone la interfaz, en el idioma de quien lo lee:
 * el `message` del servidor viene en español y antes ganaba a la traducción.
 */
import { describe, expect, test } from 'bun:test';
import { errorMessageFrom } from './dataquery';
import { en } from '../i18n/messages/en';

describe('errores de datos externos', () => {
  test('no estar en la lista se dice con la traducción, no con el texto del servidor', () => {
    const cuerpo = {
      error: { code: 'connection_not_allowed', message: 'No estás en la lista de esta conexión.' },
    };
    expect(errorMessageFrom(cuerpo)).toBe(en['extdata.msg.notAllowed']);
  });

  test('el resto de mensajes del servidor se siguen respetando', () => {
    const cuerpo = { error: { code: 'bad_request', message: 'Falta el conjunto de datos.' } };
    expect(errorMessageFrom(cuerpo)).toBe('Falta el conjunto de datos.');
  });
});
