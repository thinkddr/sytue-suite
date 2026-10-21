/**
 * Smoke test de VirtualList: con 1000 items en el DOM solo debe haber una
 * VENTANA de filas (viewport + overscan), nunca las 1000. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { VirtualList } from './VirtualList.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

// happy-dom no tiene motor de layout: offsetWidth/offsetHeight devuelven 0
// (el core de @tanstack/virtual-core mide con ellos). Fingimos un viewport de
// 600px y filas de 32px (las filas del virtualizador llevan data-index).
const VIEWPORT_H = 600;
const ROW_H = 32;
Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
  get(this: HTMLElement) {
    return this.hasAttribute('data-index') ? ROW_H : VIEWPORT_H;
  },
});
Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
  get: () => 800,
});

test('VirtualList con 1000 items pinta solo una ventana de filas', async () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const items = Array.from({ length: 1000 }, (_, i) => `fila ${i}`);

  render(
    <VirtualList
      items={items}
      estimateSize={ROW_H}
      renderItem={(texto) => <span>{texto}</span>}
      aria-label="lista de prueba"
    />,
    host,
  );
  // Deja correr los layout effects + el rerender que dispara onChange.
  await new Promise((r) => setTimeout(r, 20));

  const list = host.querySelector('[role="list"]');
  expect(list).not.toBeNull();

  const rows = host.querySelectorAll('[role="listitem"]');
  // Ventana esperada: ~19 visibles (600/32) + overscan 6 por lado ≈ 31.
  expect(rows.length).toBeGreaterThanOrEqual(10);
  expect(rows.length).toBeLessThan(100);

  // La ventana arranca en el primer item y el spacer reserva el alto total.
  expect(rows[0]?.textContent).toBe('fila 0');
  const spacer = list?.firstElementChild as HTMLElement;
  expect(spacer.style.height).toBe(`${1000 * ROW_H}px`);
  render(null, host);
  host.remove();
});

test('el padre desplaza una ventana virtual bajo una cabecera que cambia de alto', async () => {
  // No se finge el virtualizador: solo la geometría que happy-dom no calcula y
  // las notificaciones de ResizeObserver. El core real decide qué filas pinta.
  const nativeRect = HTMLElement.prototype.getBoundingClientRect;
  const nativeResizeObserver = window.ResizeObserver;
  const observers: ResizeObserverPrueba[] = [];
  class ResizeObserverPrueba implements ResizeObserver {
    readonly targets = new Set<Element>();
    readonly callback: ResizeObserverCallback;
    constructor(callback: ResizeObserverCallback) {
      this.callback = callback;
      observers.push(this);
    }
    observe(target: Element): void {
      this.targets.add(target);
    }
    unobserve(target: Element): void {
      this.targets.delete(target);
    }
    disconnect(): void {
      this.targets.clear();
    }
    notificar(target: Element): void {
      if (this.targets.has(target)) this.callback([{ target } as ResizeObserverEntry], this);
    }
  }
  window.ResizeObserver = ResizeObserverPrueba;

  const parent = document.createElement('div');
  const header = document.createElement('header');
  const host = document.createElement('div');
  parent.append(header, host);
  document.body.appendChild(parent);
  const parentRef = { current: parent };
  let headerHeight = 128;
  let count = 1000;
  const parentTop = 40;
  const borderTop = 3;
  Object.defineProperties(parent, {
    clientTop: { get: () => borderTop },
    clientHeight: { get: () => VIEWPORT_H },
    scrollHeight: { get: () => headerHeight + count * ROW_H },
  });
  parent.getBoundingClientRect = () => new DOMRect(0, parentTop, 800, VIEWPORT_H);
  parent.scrollTo = (options: ScrollToOptions | number = {}, y?: number): void => {
    parent.scrollTop = typeof options === 'number' ? (y ?? 0) : (options.top ?? 0);
    parent.dispatchEvent(new Event('scroll'));
  };
  HTMLElement.prototype.getBoundingClientRect = function () {
    if (host.contains(this) && this.parentElement?.getAttribute('role') === 'list') {
      return new DOMRect(
        0,
        parentTop + borderTop + headerHeight - parent.scrollTop,
        800,
        count * ROW_H,
      );
    }
    return nativeRect.call(this);
  };

  const pintar = () =>
    render(
      <VirtualList
        items={Array.from({ length: count }, (_, i) => `fila ${i}`)}
        estimateSize={ROW_H}
        scrollElementRef={parentRef}
        renderItem={(texto) => <span>{texto}</span>}
      />,
      host,
    );
  const first = (): HTMLElement | null => host.querySelector('[role="listitem"]');
  const esperarVentana = async (): Promise<void> => {
    const expected = Math.max(0, Math.floor((parent.scrollTop - headerHeight) / ROW_H) - 6);
    const hasta = Date.now() + 2000;
    while (Number(first()?.dataset.index) !== expected && Date.now() < hasta) {
      await new Promise((resolve) => setTimeout(resolve, 10));
    }
    expect(Number(first()?.dataset.index)).toBe(expected);
    expect(first()?.style.transform).toBe(`translateY(${expected * ROW_H}px)`);
    const content = host.querySelector('[role="list"]')?.firstElementChild;
    if (!(content instanceof HTMLElement)) throw new Error('Falta el contenido de la lista');
    expect(content.style.height).toBe(`${count * ROW_H}px`);
    expect(host.querySelectorAll('[role="listitem"]').length).toBeLessThan(100);
  };

  try {
    // La lista puede montarse tras una petición, con el padre YA desplazado.
    parent.scrollTop = headerHeight + 500 * ROW_H;
    pintar();
    await esperarVentana();
    expect(parent.scrollTop).toBe(128 + 500 * ROW_H);
    const list = host.querySelector('[role="list"]') as HTMLElement;
    expect(list.classList.contains('overflow-y-auto')).toBe(false);
    expect(list.classList.contains('overscroll-contain')).toBe(false);
    expect(list.classList.contains('h-full')).toBe(false);
    expect(list.scrollTop).toBe(0);

    // Una plantilla/cabecera crece sin que la lista ni el viewport cambien de alto.
    headerHeight = 320;
    for (const observer of observers) observer.notificar(header);
    await esperarVentana();

    // Cambiar orientación/ancho puede mover el origen sin un evento scroll.
    headerHeight = 384;
    window.dispatchEvent(new Event('resize'));
    await esperarVentana();

    // Cambiar el número de columnas cambia las filas, pero no crea otra ventana.
    count = 500;
    pintar();
    parent.scrollTop = headerHeight + 200 * ROW_H;
    parent.dispatchEvent(new Event('scroll'));
    await esperarVentana();
  } finally {
    render(null, host);
    parent.remove();
    HTMLElement.prototype.getBoundingClientRect = nativeRect;
    window.ResizeObserver = nativeResizeObserver;
  }
});
