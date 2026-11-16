/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «?» EN TODAS LAS APPS, con la Sytue Shell de verdad: abre la hoja con los atajos que
 * la app declara y el grupo de la suite; dentro de un campo es texto, y la app que se
 * lo queda (`preventDefault`) manda.
 *
 * Visto en ROJO: sin `<HojaDeAtajosDeLaSuite />` en SytueShell no hay hoja y fallan
 * los tres primeros; sin la comprobación de `defaultPrevented`, el cuarto.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render, type ComponentChildren } from 'preact';
import { act } from 'preact/test-utils';
import { useEffect } from 'preact/hooks';
import type { GrupoDeAtajos } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import { SytueShell } from './SytueShell.js';
import { abrirHojaDeAtajos, componerHoja, useAtajosDeApp } from './HojaDeAtajos.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const user: SytueUser = {
  id: 'u1',
  email: 'ana@sytue.test',
  emailVerified: true,
  name: 'Ana García',
  preferredUsername: 'ana',
  roles: [],
  groups: [],
};

const GRUPOS: GrupoDeAtajos[] = [
  {
    titulo: 'Moverse',
    atajos: [
      { teclas: ['j'], descripcion: 'Siguiente' },
      { teclas: ['?'], descripcion: 'Ver esta lista' },
    ],
  },
];

function AppConAtajos({ children }: { children?: ComponentChildren }): ComponentChildren {
  useAtajosDeApp(GRUPOS);
  return children ?? <input aria-label="Asunto" />;
}

function montar(app: ComponentChildren): { host: HTMLElement; quitar: () => void } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(
      <SytueShell currentApp="mail" user={user} hubBaseUrl="/hub" accessToken="x" locale="es">
        {app}
      </SytueShell>,
      host,
    );
  });
  return {
    host,
    quitar: () => {
      act(() => render(null, host));
      host.remove();
    },
  };
}

/** Pulsa «?» sobre `destino` y deja pasar la vuelta en la que la shell decide. */
async function pulsarAyuda(destino: EventTarget = document.body): Promise<void> {
  await act(async () => {
    destino.dispatchEvent(
      new KeyboardEvent('keydown', { key: '?', shiftKey: true, bubbles: true, cancelable: true }),
    );
    await new Promise((r) => setTimeout(r, 5));
  });
}

const hoja = (): HTMLElement | null => document.querySelector<HTMLElement>('[role="dialog"]');
const titulos = (): string[] =>
  [...(hoja()?.querySelectorAll('h3') ?? [])].map((h) => h.textContent ?? '');

test('«?» abre la hoja con los grupos de la app y el de la suite', async () => {
  const { quitar } = montar(<AppConAtajos />);
  try {
    expect(hoja()).toBeNull();
    await pulsarAyuda();
    expect(hoja()?.textContent).toContain('Atajos de teclado');
    expect(titulos()).toEqual(['Moverse', 'Toda la suite']);
    const texto = hoja()?.textContent ?? '';
    expect(texto).toContain('Siguiente');
    expect(texto).toContain('Buscar en Sytue One');
    expect(texto).toContain('Cerrar un diálogo o un menú');
    // Mayús+F10, el clic derecho del teclado (fuera de un Mac, con el nombre de la tecla).
    expect(texto).toContain('Abrir el menú contextual');
    expect(texto).toMatch(/Mayús\+F10|⇧F10/);
    // La fila de «?» de la app no se repite: es de la suite.
    expect(texto.split('Ver esta lista').length - 1).toBe(1);
  } finally {
    quitar();
  }
});

test('una app que no declara nada también enseña los de la suite', async () => {
  const { quitar } = montar(<p>Sin atajos</p>);
  try {
    await pulsarAyuda();
    expect(titulos()).toEqual(['Toda la suite']);
  } finally {
    quitar();
  }
});

test('dentro de un campo, «?» es texto y no abre nada', async () => {
  const { host, quitar } = montar(<AppConAtajos />);
  try {
    const campo = host.querySelector<HTMLInputElement>('input[aria-label="Asunto"]')!;
    campo.focus();
    await pulsarAyuda(campo);
    expect(hoja()).toBeNull();
  } finally {
    quitar();
  }
});

function AppQueSeLoQueda(): ComponentChildren {
  useEffect(() => {
    const f = (e: KeyboardEvent): void => {
      if (e.key === '?') e.preventDefault();
    };
    // En `window` y registrado DESPUÉS que la shell: el caso difícil del orden.
    window.addEventListener('keydown', f);
    return () => window.removeEventListener('keydown', f);
  }, []);
  return <p>App</p>;
}

test('la app va primero: si hace preventDefault, la shell no abre la suya', async () => {
  const { quitar } = montar(<AppQueSeLoQueda />);
  try {
    await pulsarAyuda();
    expect(hoja()).toBeNull();
  } finally {
    quitar();
  }
});

test('abrirHojaDeAtajos la abre desde un botón de la app', async () => {
  const { quitar } = montar(<AppConAtajos />);
  try {
    await act(async () => abrirHojaDeAtajos());
    expect(titulos()).toEqual(['Moverse', 'Toda la suite']);
  } finally {
    quitar();
  }
});

test('componerHoja quita los grupos que se quedan vacíos sin el «?»', () => {
  const suite = { titulo: 'Suite', atajos: [{ teclas: ['?'], descripcion: 'Ayuda' }] };
  const solo = [{ titulo: 'Ayuda', atajos: [{ teclas: ['?'], descripcion: 'Ver esta lista' }] }];
  expect(componerHoja(solo, suite)).toEqual([suite]);
});
