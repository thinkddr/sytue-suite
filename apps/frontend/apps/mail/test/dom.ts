import { afterAll, beforeAll } from 'bun:test';
import { JSDOM } from 'jsdom';

export function setupJsdom(): void {
  // DOMPurify requires a compatible DOM; Happy DOM is not supported for security tests.
  const dom = new JSDOM('');
  const environment = {
    window: dom.window,
    document: dom.window.document,
    DOMParser: dom.window.DOMParser,
    Event: dom.window.Event,
    HTMLInputElement: dom.window.HTMLInputElement,
    HTMLTableCellElement: dom.window.HTMLTableCellElement,
    HTMLTableElement: dom.window.HTMLTableElement,
  };
  const previous = new Map<string, PropertyDescriptor | undefined>();
  beforeAll(() => {
    for (const [name, value] of Object.entries(environment)) {
      previous.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
      Object.defineProperty(globalThis, name, { configurable: true, writable: true, value });
    }
  });
  afterAll(() => {
    for (const [name, descriptor] of previous) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else Reflect.deleteProperty(globalThis, name);
    }
    dom.window.close();
  });
}
