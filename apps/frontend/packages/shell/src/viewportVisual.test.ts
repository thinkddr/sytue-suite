import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { publicarViewportVisual } from './viewportVisual.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function preparar() {
  const vv = Object.assign(new EventTarget(), {
    width: 390,
    height: 800,
    offsetTop: 0,
    scale: 1,
  });
  const llamadas: unknown[][] = [];
  const originales = new Map<string, PropertyDescriptor | undefined>();
  const poner = (nombre: string, valor: unknown): void => {
    originales.set(nombre, Object.getOwnPropertyDescriptor(window, nombre));
    Object.defineProperty(window, nombre, { configurable: true, value: valor });
  };
  poner('visualViewport', vv);
  poner('innerHeight', 800);
  poner('scrollY', 250);
  poner('scrollTo', (...args: unknown[]) => llamadas.push(args));
  const dejarDePublicar = publicarViewportVisual();
  return {
    vv,
    llamadas,
    restaurar: () => {
      dejarDePublicar();
      for (const [nombre, descriptor] of originales) {
        if (descriptor) Object.defineProperty(window, nombre, descriptor);
        else Reflect.deleteProperty(window, nombre);
      }
    },
  };
}

test('girar la pantalla conserva el sitio de lectura aunque la altura crezca más de80px', () => {
  const p = preparar();
  try {
    p.vv.width = 844;
    p.vv.height = 390;
    p.vv.dispatchEvent(new Event('resize'));
    p.vv.width = 390;
    p.vv.height = 800;
    p.vv.dispatchEvent(new Event('resize'));
    expect(p.llamadas).toEqual([]);
    expect(document.documentElement.style.getPropertyValue('--sytue-vvh')).toBe('800px');
  } finally {
    p.restaurar();
  }
});

test('un teclado real que desplaza la página sigue devolviéndola al origen al cerrarse', () => {
  const p = preparar();
  const campo = document.body.appendChild(document.createElement('textarea'));
  try {
    campo.focus();
    p.vv.height = 420;
    p.vv.offsetTop = 90;
    p.vv.dispatchEvent(new Event('resize'));
    campo.blur();
    p.vv.height = 800;
    p.vv.offsetTop = 0;
    p.vv.dispatchEvent(new Event('resize'));
    expect(p.llamadas).toEqual([[0, 0]]);
  } finally {
    campo.remove();
    p.restaurar();
  }
});

test('el pellizco no se interpreta como cierre de teclado y al desmontar se deja de medir', () => {
  const p = preparar();
  try {
    p.vv.scale = 2;
    p.vv.width = 195;
    p.vv.height = 400;
    p.vv.dispatchEvent(new Event('resize'));
    p.vv.scale = 1;
    p.vv.width = 390;
    p.vv.height = 800;
    p.vv.dispatchEvent(new Event('resize'));
    expect(p.llamadas).toEqual([]);
  } finally {
    p.restaurar();
  }
  p.vv.dispatchEvent(new Event('resize'));
  expect(document.documentElement.style.getPropertyValue('--sytue-vvh')).toBe('');
});
