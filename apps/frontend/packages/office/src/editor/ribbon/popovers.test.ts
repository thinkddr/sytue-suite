import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { colocarPopover } from './popovers';

if (typeof document === 'undefined') GlobalRegistrator.register();

test('la paleta de cinta respeta la muesca y cabe aunque el teclado deje menos de 120px', () => {
  const descriptor = Object.getOwnPropertyDescriptor(window, 'visualViewport');
  const vv = { width: 844, height: 390, offsetLeft: 0, offsetTop: 0 };
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv });
  const raiz = document.documentElement;
  const anteriores = ['top', 'right', 'bottom', 'left'].map((lado) =>
    raiz.style.getPropertyValue(`--sytue-safe-${lado}`),
  );
  const estilo = document.createElement('style');
  estilo.textContent = '.paleta-de-prueba { max-height: 256px; }';
  const ancla = document.createElement('div');
  const panel = document.createElement('div');
  panel.className = 'paleta-de-prueba';
  ancla.appendChild(panel);
  document.head.appendChild(estilo);
  document.body.appendChild(ancla);
  ancla.getBoundingClientRect = () => ({ left: 700, bottom: 200 }) as DOMRect;
  Object.defineProperty(panel, 'offsetWidth', { value: 300 });
  Object.defineProperty(panel, 'scrollHeight', { value: 400 });
  try {
    for (const [lado, valor] of [
      ['top', 0],
      ['right', 44],
      ['bottom', 21],
      ['left', 44],
    ] as const) {
      raiz.style.setProperty(`--sytue-safe-${lado}`, `${valor}px`);
    }
    colocarPopover(panel);
    expect(panel.style.left).toBe('492px');
    expect(panel.style.top).toBe('105px');
    expect(panel.style.maxWidth).toBe('740px');
    expect(panel.style.maxHeight).toBe('256px');
    vv.height = 100;
    vv.offsetTop = 200;
    colocarPopover(panel);
    expect(panel.style.top).toBe('208px');
    expect(panel.style.maxHeight).toBe('63px');
    expect(panel.style.overflowY).toBe('auto');
  } finally {
    ancla.remove();
    estilo.remove();
    for (const [i, lado] of ['top', 'right', 'bottom', 'left'].entries()) {
      raiz.style.setProperty(`--sytue-safe-${lado}`, anteriores[i]!);
    }
    if (descriptor) Object.defineProperty(window, 'visualViewport', descriptor);
    else Reflect.deleteProperty(window, 'visualViewport');
  }
});
