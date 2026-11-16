/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Remitentes de confianza (D-MAIL-08): «Mostrar siempre las imágenes de este
 * remitente» junto a «Mostrar imágenes», y los correos de un remitente de confianza
 * se abren con sus imágenes —por el proxy— sin pulsar nada. Salvo en spam o con la
 * autenticación en contra: ahí el `From` es justo lo que no se cree.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { ContextoDelRemitente } from './confianzaImagenes.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';

const { ThemeProvider } = await import('@sytue/ui');
const { HtmlBody } = await import('./MessageReader.js');
const { ConfianzaImagenes } = await import('./confianzaImagenes.js');
// Fuera de un componente: es un traductor, no un hook (el nombre `useT` lo es solo en las islas).
const { useT: traductor } = await import('../i18n/index.js');
type Contexto = ContextoDelRemitente;

const t = traductor('es');
const HTML = '<p>Ofertas</p><img src="https://cdn.tienda.eu/banner.png" alt="">';
const LIMPIO: Contexto = { enSpam: false, autenticacion: 'verificado' };

afterEach(() => {
  document.body.innerHTML = '';
});

function montar(opciones: {
  confiados?: string[];
  confiar?: ((d: string) => void) | null;
  remitente?: string;
  contexto?: Contexto;
}): HTMLElement {
  const host = document.body.appendChild(document.createElement('div'));
  act(() =>
    render(
      <ThemeProvider>
        <ConfianzaImagenes.Provider
          value={{
            confiados: new Set(opciones.confiados ?? []),
            confiar: opciones.confiar === undefined ? () => {} : opciones.confiar,
          }}
        >
          <HtmlBody
            t={t}
            html={HTML}
            remitente={opciones.remitente ?? 'Boletin@Tienda.eu'}
            contexto={opciones.contexto ?? LIMPIO}
          />
        </ConfianzaImagenes.Provider>
      </ThemeProvider>,
      host,
    ),
  );
  return host;
}

const documento = (host: HTMLElement): string =>
  host.querySelector('iframe')?.getAttribute('srcdoc') ?? '';
const boton = (host: HTMLElement, texto: string): HTMLButtonElement | undefined =>
  [...host.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === texto,
  );

test('un remitente desconocido se abre bloqueado y ofrece confiar en él', () => {
  const pedidos: string[] = [];
  const host = montar({ confiar: (d) => pedidos.push(d) });
  expect(documento(host)).not.toContain('/api/remote-image');
  expect(boton(host, 'Mostrar imágenes')).toBeDefined();
  const siempre = boton(host, 'Mostrar siempre las de este remitente');
  expect(siempre?.getAttribute('aria-label')).toBe(
    'Mostrar siempre las imágenes de Boletin@Tienda.eu',
  );
  act(() => siempre?.click());
  expect(pedidos).toEqual(['Boletin@Tienda.eu']);
});

test('un remitente de confianza se abre con las imágenes por el proxy, sin preguntar', () => {
  // La lista va normalizada; el `From` del mensaje puede venir con mayúsculas.
  const host = montar({ confiados: ['boletin@tienda.eu'] });
  expect(documento(host)).toContain('/api/remote-image');
  expect(documento(host)).not.toContain('src="https://cdn.tienda.eu');
  expect(boton(host, 'Mostrar imágenes')).toBeUndefined();
});

test('en spam o con la autenticación en contra no cuenta la confianza ni se ofrece', () => {
  for (const contexto of [
    { enSpam: true, autenticacion: 'verificado' },
    { enSpam: false, autenticacion: 'aviso' },
  ] satisfies Contexto[]) {
    const host = montar({ confiados: ['boletin@tienda.eu'], contexto });
    expect(documento(host)).not.toContain('/api/remote-image');
    expect(boton(host, 'Mostrar imágenes')).toBeDefined();
    expect(boton(host, 'Mostrar siempre las de este remitente')).toBeUndefined();
    document.body.innerHTML = '';
  }
});

test('sin poder confiar (buzón de solo lectura) solo queda «Mostrar imágenes»', () => {
  const host = montar({ confiar: null });
  expect(boton(host, 'Mostrar imágenes')).toBeDefined();
  expect(boton(host, 'Mostrar siempre las de este remitente')).toBeUndefined();
});
