import { expect, test } from 'bun:test';
import { externalMailClient } from './externalMail.js';

// MailApp crea este cliente en un `useMemo`, que también corre al renderizar en el
// servidor. Allí no hay `window`, y leerlo tumbó todas las páginas de Mail con sesión
// (v3.19.3). Bun corre todos los tests en un proceso y otro puede haber registrado
// happy-dom: se quita `window` a mano para estar de verdad en el servidor.
test('el cliente de correo externo se puede crear sin window (renderizado en servidor)', () => {
  const g = globalThis as { window?: unknown };
  const habia = Object.prototype.hasOwnProperty.call(g, 'window');
  const antes = g.window;
  Reflect.deleteProperty(g, 'window');
  try {
    expect(typeof (globalThis as { window?: unknown }).window).toBe('undefined');
    expect(() => externalMailClient()).not.toThrow();
  } finally {
    if (habia) g.window = antes;
  }
});
