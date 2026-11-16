/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ⌘K ES LA PALETA DE LA SUITE: con la Sytue Shell de verdad, ⌘K abre una sola paleta
 * con lo que la app registró como capa y, con dos letras, lo que encuentra el Hub.
 *
 * Visto en ROJO: sin `ProveedorDePaleta` en `SytueShell`, ⌘K enfocaba el buscador de
 * la barra y no había diálogo; sin el `defaultPrevented`, la paleta le pisaba el ⌘K a un
 * editor; sin el `quitar` del desmontaje, la capa de una vista cerrada seguía ofrecida.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render, type ComponentChildren, type JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import type { HubClient, SearchResponse } from '@sytue/api-clients';
import type { SytueUser } from '@sytue/auth';
import { I18nProvider } from '@sytue/i18n/preact';
import { SytueShell } from './SytueShell.js';
import { ESPERA_BUSQUEDA_MS, ProveedorDePaleta, useCapaDePaleta } from './PaletaDeLaSuite.js';
import { abrirPaleta, type ComandoDeApp } from './paleta.js';

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

const elegidos: string[] = [];
const COMANDOS: ComandoDeApp[] = [
  {
    id: 'fact',
    rotulo: 'Facturas',
    grupo: 'Ir a',
    pista: 'Ventas',
    ejecutar: () => void elegidos.push('fact'),
  },
  {
    id: 'cli',
    rotulo: 'Clientes',
    grupo: 'Ir a',
    pista: 'CRM',
    ejecutar: () => void elegidos.push('cli'),
  },
];

function Capa({ comandos = COMANDOS }: { comandos?: ComandoDeApp[] }): null {
  useCapaDePaleta('prueba', { comandos });
  return null;
}

function montar(hijos: ComponentChildren): { host: HTMLElement; quitar: () => void } {
  const host = document.body.appendChild(document.createElement('div'));
  act(() => {
    render(
      <SytueShell currentApp="erp" user={user} hubBaseUrl="/hub" accessToken="x" locale="es">
        {hijos}
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

function pulsarK(objetivo: EventTarget = document): KeyboardEvent {
  const e = new KeyboardEvent('keydown', {
    key: 'k',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    objetivo.dispatchEvent(e);
  });
  return e;
}

const dialogo = () => document.querySelector<HTMLElement>('[role="dialog"]');
const opciones = () =>
  [...document.querySelectorAll<HTMLElement>('[role="option"]')].map((o) => o.textContent ?? '');

function escribir(texto: string): void {
  const campo = document.querySelector<HTMLInputElement>('[role="combobox"][aria-autocomplete]')!;
  act(() => {
    campo.value = texto;
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

test('⌘K abre la paleta de la suite, con la capa de la app y el foco en su campo', () => {
  const m = montar(<Capa />);
  expect(dialogo()).toBeNull();
  const e = pulsarK();
  expect(e.defaultPrevented).toBe(true);
  expect(dialogo()?.getAttribute('aria-label')).toBe('Ir a o buscar');
  expect(opciones()).toEqual(['FacturasVentas', 'ClientesCRM']);
  // El foco va al campo de la paleta, no al buscador de la barra.
  expect((document.activeElement as HTMLElement | null)?.id).not.toBe('sytue-busqueda-global');
  expect(document.activeElement?.getAttribute('aria-autocomplete')).toBe('list');
  // Filtra plegando acentos, y elegir cierra y ejecuta.
  escribir('CLIEN');
  expect(opciones()).toEqual(['ClientesCRM']);
  act(() => document.querySelector<HTMLElement>('[role="option"]')!.click());
  expect(dialogo()).toBeNull();
  expect(elegidos).toEqual(['cli']);
  // Otro ⌘K la abre en blanco, y uno más la cierra.
  pulsarK();
  expect(opciones()).toHaveLength(2);
  pulsarK();
  expect(dialogo()).toBeNull();
  m.quitar();
});

test('sin capa, la paleta invita a buscar en la suite', () => {
  const m = montar(<main>Mail</main>);
  pulsarK();
  expect(opciones()).toEqual([]);
  expect(dialogo()?.textContent).toContain('Escribe para buscar en toda Sytue One');
  pulsarK();
  m.quitar();
});

test('respeta el ⌘K que ya se ha quedado una app y el de otro diálogo', () => {
  const m = montar(
    <>
      <Capa />
      <textarea id="editor" />
    </>,
  );
  const editor = document.getElementById('editor')!;
  const reclamar = (e: Event) => e.preventDefault();
  editor.addEventListener('keydown', reclamar);
  pulsarK(editor);
  expect(dialogo()).toBeNull();
  editor.removeEventListener('keydown', reclamar);

  const otro = document.body.appendChild(document.createElement('div'));
  otro.setAttribute('role', 'dialog');
  pulsarK();
  expect(dialogo()).toBe(otro);
  otro.remove();
  m.quitar();
});

test('abrirPaleta() la abre desde un botón de fuera, y la capa se va al desmontarse', () => {
  let ver: (si: boolean) => void = () => {};
  function Vista(): JSX.Element | null {
    const [si, setSi] = useState(true);
    useEffect(() => {
      ver = setSi;
    }, []);
    return si ? <Capa /> : null;
  }
  const m = montar(<Vista />);
  act(() => abrirPaleta());
  expect(opciones()).toHaveLength(2);
  act(() => ver(false));
  expect(opciones()).toEqual([]);
  pulsarK();
  m.quitar();
});

test('con dos letras pregunta al Hub y pone sus resultados debajo, por producto', async () => {
  const pedidas: string[] = [];
  const hub = {
    search: async (q: string): Promise<SearchResponse> => {
      pedidas.push(q);
      return {
        results: [
          { id: '1', source: 'drive', title: 'Facturas 2026.xlsx', url: '/drive/1' },
          { id: '2', source: 'mail', title: 'Re: facturas', url: '/mail/2', snippet: 'Hola' },
        ],
        fallidos: [],
      } as unknown as SearchResponse;
    },
  } as unknown as HubClient;
  const host = document.body.appendChild(document.createElement('div'));
  act(() => {
    render(
      <I18nProvider locale="es">
        <ProveedorDePaleta hub={hub}>
          <Capa />
        </ProveedorDePaleta>
      </I18nProvider>,
      host,
    );
  });
  act(() => abrirPaleta());
  escribir('f');
  await act(() => new Promise((r) => setTimeout(r, ESPERA_BUSQUEDA_MS + 20)));
  // Una letra no pregunta.
  expect(pedidas).toEqual([]);
  escribir('fact');
  await act(() => new Promise((r) => setTimeout(r, ESPERA_BUSQUEDA_MS + 20)));
  expect(pedidas).toEqual(['fact']);
  expect(opciones()).toEqual(['FacturasVentas', 'Facturas 2026.xlsx', 'Re: facturasHola']);
  const grupos = [...document.querySelectorAll('[role="listbox"] > li[role="presentation"]')].map(
    (g) => g.textContent,
  );
  expect(grupos).toEqual(['Ir a', 'Drive', 'Mail']);
  act(() => render(null, host));
  host.remove();
});
