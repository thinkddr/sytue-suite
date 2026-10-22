import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useRef } from 'preact/hooks';
import { limitesViewport } from './viewport.js';
import { useFitInViewport } from './useFitInViewport.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

test('un panel abierto se vuelve a encajar tras girar y respeta las zonas seguras y el viewport visual', () => {
  const vv = Object.assign(new EventTarget(), {
    width: 390,
    height: 800,
    offsetLeft: 0,
    offsetTop: 0,
  });
  const previo = Object.getOwnPropertyDescriptor(window, 'visualViewport');
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv });
  const raiz = document.documentElement;
  const anterior = raiz.style.cssText;
  const host = document.body.appendChild(document.createElement('div'));
  const panel = document.body.appendChild(document.createElement('div'));
  panel.getBoundingClientRect = () => new DOMRect(180, 100, 200, 100);
  const Sonda = () => {
    const ref = useRef(panel);
    useFitInViewport(ref, true);
    return null;
  };
  try {
    for (const lado of ['top', 'right', 'bottom', 'left'])
      raiz.style.setProperty(`--sytue-safe-${lado}`, '0px');
    render(<Sonda />, host);
    expect(panel.style.translate).toBe('');

    vv.width = 260;
    raiz.style.setProperty('--sytue-safe-left', '30px');
    raiz.style.setProperty('--sytue-safe-right', '10px');
    raiz.style.setProperty('--sytue-safe-bottom', '20px');
    vv.dispatchEvent(new Event('resize'));
    expect(panel.style.maxWidth).toBe('204px');
    expect(panel.style.translate).toBe('-138px');
    expect(limitesViewport(8)).toEqual({
      left: 38,
      top: 8,
      right: 242,
      bottom: 772,
      width: 204,
      height: 764,
    });

    vv.offsetLeft = 10;
    vv.offsetTop = 80;
    vv.dispatchEvent(new Event('scroll'));
    expect(panel.style.translate).toBe('-128px');
    expect(limitesViewport(8).top).toBe(88);

    render(null, host);
    vv.width = 200;
    window.dispatchEvent(new Event('resize'));
    vv.dispatchEvent(new Event('scroll'));
    expect(panel.style.maxWidth).toBe('');
    expect(panel.style.translate).toBe('');
  } finally {
    render(null, host);
    host.remove();
    panel.remove();
    raiz.style.cssText = anterior;
    if (previo) Object.defineProperty(window, 'visualViewport', previo);
    else Reflect.deleteProperty(window, 'visualViewport');
  }
});

test('un visor propio sigue accesible sobre el teclado y restaura sus estilos al cerrar', () => {
  const vv = Object.assign(new EventTarget(), {
    width: 390,
    height: 800,
    offsetLeft: 0,
    offsetTop: 0,
  });
  const previo = Object.getOwnPropertyDescriptor(window, 'visualViewport');
  Object.defineProperty(window, 'visualViewport', { configurable: true, value: vv });
  const raiz = document.documentElement;
  const anterior = raiz.style.cssText;
  const host = document.body.appendChild(document.createElement('div'));
  const panel = document.body.appendChild(document.createElement('div'));
  panel.style.maxHeight = '500px';
  panel.style.overflowY = 'visible';
  // El navegador reduce la caja al max-height antes de medirla. El doble conserva
  // su anclaje original para comprobar que los empujones no se acumulan.
  let arriba = 600;
  panel.getBoundingClientRect = () =>
    new DOMRect(8, arriba, 300, Math.min(300, Number.parseFloat(panel.style.maxHeight)));
  const Sonda = () => {
    const ref = useRef(panel);
    useFitInViewport(ref, true, { vertical: true });
    return null;
  };
  try {
    for (const [lado, px] of [
      ['left', 44],
      ['right', 20],
      ['top', 10],
      ['bottom', 24],
    ] as const)
      raiz.style.setProperty(`--sytue-safe-${lado}`, `${px}px`);
    render(<Sonda />, host);
    expect(panel.style.maxHeight).toBe('500px');
    expect(panel.style.translate).toBe('44px -132px');

    vv.height = 300;
    vv.offsetTop = 40;
    vv.dispatchEvent(new Event('resize'));
    expect(panel.style.maxHeight).toBe('250px');
    expect(panel.style.overflowY).toBe('auto');
    expect(panel.style.translate).toBe('44px -542px');

    vv.offsetLeft = 30;
    vv.dispatchEvent(new Event('scroll'));
    expect(panel.style.translate).toBe('74px -542px');

    vv.height = 800;
    vv.offsetTop = 0;
    vv.dispatchEvent(new Event('resize'));
    expect(panel.style.maxHeight).toBe('500px');
    expect(panel.style.translate).toBe('74px -132px');

    // El panel sigue abierto y conserva tamaño; solo cambió su anclaje al
    // renderizar otra selección. No hay resize que avise al observador.
    arriba = 100;
    render(<Sonda />, host);
    expect(panel.style.translate).toBe('74px');

    render(null, host);
    expect(panel.style.maxHeight).toBe('500px');
    expect(panel.style.overflowY).toBe('visible');
    expect(panel.style.translate).toBe('');
    vv.height = 200;
    vv.dispatchEvent(new Event('resize'));
    expect(panel.style.maxHeight).toBe('500px');
  } finally {
    render(null, host);
    host.remove();
    panel.remove();
    raiz.style.cssText = anterior;
    if (previo) Object.defineProperty(window, 'visualViewport', previo);
    else Reflect.deleteProperty(window, 'visualViewport');
  }
});
