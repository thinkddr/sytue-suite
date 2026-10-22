/**
 * El color y la forma comparten una sola regla en el proveedor (`useEje`): manda la
 * persona; si no ha elegido, su organización; si no, la casa. Y elegir «la organización»
 * es olvidar la elección, no guardar otra.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { ThemeProvider, useTheme } from './ThemeProvider.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

type Tema = ReturnType<typeof useTheme>;

function montar(): { tema: () => Tema; desmontar: () => void } {
  const visto: { tema?: Tema } = {};
  const Sonda = (): null => {
    const tema = useTheme();
    // En un efecto y no durante el render: es lo que haría cualquier consumidor.
    useEffect(() => {
      visto.tema = tema;
    });
    return null;
  };
  const host = document.createElement('div');
  act(() => {
    render(
      <ThemeProvider>
        <Sonda />
      </ThemeProvider>,
      host,
    );
  });
  return { tema: () => visto.tema!, desmontar: () => act(() => render(null, host)) };
}

// El valor de la organización llega por cookie, pero happy-dom en `about:blank` no las
// guarda. El lector mira también el almacenamiento local, y la regla que se prueba aquí
// —quién manda y cómo se vuelve— no depende de por dónde llegó.
const deLaOrg = (clave: string, valor: string): void => localStorage.setItem(clave, valor);

function limpiar(): void {
  for (const clave of ['sytue-color', 'sytue-color-org', 'sytue-forma', 'sytue-forma-org']) {
    localStorage.removeItem(clave);
  }
  for (const attr of ['data-color', 'data-forma']) document.documentElement.removeAttribute(attr);
}

test('sin elección se aplica el color de cada app y se conserva el petróleo explícito', () => {
  limpiar();
  const { tema, desmontar } = montar();
  try {
    expect([tema().color, tema().colorChoice, tema().orgColor]).toEqual(['app', null, 'app']);
    expect(document.documentElement.getAttribute('data-color')).toBe('app');
    expect(localStorage.getItem('sytue-color')).toBeNull();

    act(() => tema().setColor('petroleo'));
    expect([tema().color, tema().colorChoice]).toEqual(['petroleo', 'petroleo']);
    expect(document.documentElement.hasAttribute('data-color')).toBe(false);

    act(() => tema().setColor(null));
    expect([tema().color, tema().colorChoice]).toEqual(['app', null]);
    expect(document.documentElement.getAttribute('data-color')).toBe('app');
  } finally {
    desmontar();
    limpiar();
  }
});

test('quien no ha elegido sigue a su organización; quien elige, manda; y puede volver', () => {
  limpiar();
  deLaOrg('sytue-color-org', 'cobalto');
  const { tema, desmontar } = montar();
  try {
    expect([tema().color, tema().colorChoice, tema().orgColor]).toEqual([
      'cobalto',
      null,
      'cobalto',
    ]);
    expect(document.documentElement.getAttribute('data-color')).toBe('cobalto');

    act(() => tema().setColor('petroleo'));
    expect([tema().color, tema().colorChoice, tema().orgColor]).toEqual([
      'petroleo',
      'petroleo',
      'cobalto',
    ]);
    expect(document.documentElement.hasAttribute('data-color')).toBe(false);
    expect(localStorage.getItem('sytue-color')).toBe('petroleo');

    // «La organización» no es un valor que se guarde: es dejar de tener elección.
    act(() => tema().setColor(null));
    expect([tema().color, tema().colorChoice]).toEqual(['cobalto', null]);
    expect(document.documentElement.getAttribute('data-color')).toBe('cobalto');
    expect(localStorage.getItem('sytue-color')).toBeNull();
  } finally {
    desmontar();
    limpiar();
  }
});

test('la forma va por la misma regla que el color', () => {
  limpiar();
  deLaOrg('sytue-forma-org', 'carril');
  const { tema, desmontar } = montar();
  try {
    expect([tema().forma, tema().formaChoice, tema().orgForma]).toEqual(['carril', null, 'carril']);
    act(() => tema().setForma('marco'));
    expect([tema().forma, tema().formaChoice]).toEqual(['marco', 'marco']);
    expect(document.documentElement.hasAttribute('data-forma')).toBe(false);
    act(() => tema().setForma(null));
    expect([tema().forma, tema().formaChoice]).toEqual(['carril', null]);
    expect(document.documentElement.getAttribute('data-forma')).toBe('carril');
  } finally {
    desmontar();
    limpiar();
  }
});
