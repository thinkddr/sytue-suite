// Con «color de cada app» cada producto de este despliegue pinta su color, y la hoja
// sabe cuál es por `data-app`. El layout lo pone en el servidor; al abrir otro documento
// sin recargar lo cambia `aplicarMarca`, que es quien ya cambiaba el icono de la pestaña.
import { expect, test } from 'bun:test';
import { aplicarMarca, productoDeKind } from './producto.js';

test('al cambiar de documento, la app que dice el <html> es la del producto nuevo', () => {
  const g = globalThis as { document?: unknown };
  const original = Object.getOwnPropertyDescriptor(globalThis, 'document');
  const raiz = { dataset: { app: 'docs' } as Record<string, string> };
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    writable: true,
    value: { documentElement: raiz, querySelector: () => null },
  });
  try {
    aplicarMarca(productoDeKind('presentation'));
    expect(raiz.dataset.app).toBe('slides');
    aplicarMarca(productoDeKind('spreadsheet'));
    expect(raiz.dataset.app).toBe('sheets');
    aplicarMarca('pdf');
    expect(raiz.dataset.app).toBe('pdf');
  } finally {
    if (original) Object.defineProperty(globalThis, 'document', original);
    else delete g.document;
  }
});
