/**
 * Tests de `useTablist`: roving tabindex, flechas, Inicio/Fin y el enlace pestaña-panel.
 * Se monta un componente de verdad (como el resto del paquete), no se mockea `preact/hooks`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { useTablist } from './useTablist.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const IDS = ['uno', 'dos', 'tres'] as const;

function Demo() {
  const [activa, setActiva] = useState<(typeof IDS)[number]>('uno');
  const tl = useTablist(IDS, activa, setActiva);
  return (
    <div>
      {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus */}
      <div role="tablist" aria-label="demo" onKeyDown={tl.onKeyDown}>
        {IDS.map((id) => (
          <button type="button" key={id} {...tl.tab(id)}>
            {id}
          </button>
        ))}
      </div>
      <div {...tl.panel}>{activa}</div>
    </div>
  );
}

function montar() {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(<Demo />, host));
  const tabs = () => Array.from(host.querySelectorAll<HTMLElement>('[role="tab"]'));
  const tecla = (el: HTMLElement, key: string) =>
    act(() => {
      el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
    });
  return { host, tabs, tecla };
}

describe('useTablist', () => {
  test('solo la pestaña activa es parada de tabulación', () => {
    const { tabs } = montar();
    expect(tabs().map((t) => t.tabIndex)).toEqual([0, -1, -1]);
  });

  test('las flechas mueven la pestaña activa y el foco, dando la vuelta', () => {
    const { tabs, tecla } = montar();
    tecla(tabs()[0]!, 'ArrowRight');
    expect(tabs()[1]!.getAttribute('aria-selected')).toBe('true');
    expect(document.activeElement).toBe(tabs()[1]!);
    tecla(tabs()[1]!, 'ArrowLeft');
    tecla(tabs()[0]!, 'ArrowLeft');
    expect(tabs()[2]!.getAttribute('aria-selected')).toBe('true');
    expect(tabs().map((t) => t.tabIndex)).toEqual([-1, -1, 0]);
  });

  test('Inicio y Fin van a los extremos; otra tecla no hace nada', () => {
    const { tabs, tecla } = montar();
    tecla(tabs()[0]!, 'End');
    expect(tabs()[2]!.getAttribute('aria-selected')).toBe('true');
    tecla(tabs()[2]!, 'Home');
    expect(tabs()[0]!.getAttribute('aria-selected')).toBe('true');
    tecla(tabs()[0]!, 'a');
    expect(tabs()[0]!.getAttribute('aria-selected')).toBe('true');
  });

  test('cada pestaña apunta al panel y el panel a la activa', () => {
    const { host, tabs, tecla } = montar();
    const panel = host.querySelector<HTMLElement>('[role="tabpanel"]')!;
    expect(tabs().every((t) => t.getAttribute('aria-controls') === panel.id)).toBe(true);
    tecla(tabs()[0]!, 'ArrowRight');
    expect(panel.getAttribute('aria-labelledby')).toBe(tabs()[1]!.id);
    expect(panel.textContent).toBe('dos');
  });
});
