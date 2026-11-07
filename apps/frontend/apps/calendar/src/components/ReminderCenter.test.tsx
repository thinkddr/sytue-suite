/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS RECORDATORIOS EN EL CAJÓN DEL TELÉFONO.
 *
 * En el mes del teléfono la cabecera no se pinta y la campana pasa al cajón como una rama
 * del menú (`disparador="fila"`). Tres cosas que vigilar:
 *
 * - El estado se monta UNA vez (`useRecordatorios`): pasar de la campana a la fila —que
 *   es lo que pasa al girar el teléfono o cambiar de vista— no vuelve a sondear.
 * - Lo que actúa en el sitio (desplegar, descartar, activar avisos) lleva
 *   `data-mantiene-nav` y no cierra el cajón; el enlace a un recordatorio NO lo lleva:
 *   lleva a su día y cierra el cajón como cualquier destino.
 * - Con AppShell de verdad: el enlace desde el cajón llama a `onDeepLink` y el cajón se
 *   cierra; y Escape es del cajón (lo cierra y devuelve el foco al ☰) aunque la rama esté
 *   desplegada, que conserva su estado como cualquier rama del lateral.
 *
 * Vistos en ROJO: con el sondeo dentro de `ReminderCenter` (un `useRecordatorios` por
 * presentación: dos peticiones), con `{...enSitio}` también en el enlace (el cajón se
 * quedaba abierto) y sin él en «descartar».
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import { AppShell, Sidebar, SidebarSection, ToastProvider } from '@sytue/ui';
// `useT` no es un hook: devuelve el traductor ya atado a un idioma.
import { useT as traductor } from '../i18n/index.js';
import { ReminderCenter, useRecordatorios, type EstadoRecordatorios } from './ReminderCenter.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = traductor('es');
const RECORDATORIO = {
  id: 'r1',
  event_id: 'e1',
  uid: 'u1',
  summary: 'Tenis',
  dtstart: '2026-11-06T13:00:00Z',
  trigger_at: '2026-11-06T12:45:00Z',
  action: 'DISPLAY',
};

async function esperar(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 10));
    });
  }
  expect(cond()).toBe(true);
}

/** Cuenta las peticiones a /api/reminders y contesta con un recordatorio. */
function servidor(): { peticiones: () => number } {
  let n = 0;
  globalThis.fetch = (async (url: string) => {
    if (String(url).includes('/api/reminders')) n++;
    return new Response(JSON.stringify({ reminders: [RECORDATORIO], next_cursor: '' }), {
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
  return { peticiones: () => n };
}

function montar(arbol: preact.ComponentChild): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() =>
    render(
      <I18nProvider locale="es">
        <ToastProvider>{arbol}</ToastProvider>
      </I18nProvider>,
      host,
    ),
  );
  return host;
}

test('cambiar de la campana a la fila no vuelve a sondear', async () => {
  const s = servidor();
  const control = { fila: (_: boolean): void => {} };
  function App() {
    const [enFila, setEnFila] = useState(false);
    useEffect(() => {
      control.fila = setEnFila;
    }, []);
    const estado = useRecordatorios({ t, locale: 'es', onDeepLink: () => {} });
    return enFila ? (
      <ul>
        <ReminderCenter t={t} locale="es" estado={estado} disparador="fila" />
      </ul>
    ) : (
      <ReminderCenter t={t} locale="es" estado={estado} />
    );
  }
  const host = montar(<App />);
  await esperar(() => s.peticiones() === 1);
  act(() => control.fila(true));
  act(() => control.fila(false));
  act(() => control.fila(true));
  // La fila trae el contador del estado que había, sin pedirlo otra vez.
  await esperar(() => host.querySelector('[aria-expanded]')!.textContent!.includes('1'));
  expect(s.peticiones()).toBe(1);
  render(null, host);
  host.remove();
});

const ESTADO: EstadoRecordatorios = {
  lista: [RECORDATORIO],
  cargando: false,
  error: false,
  ocupado: null,
  permiso: 'default',
  descartar: async () => {},
  irA: () => {},
  pedirPermiso: async () => {},
};

test('la fila: rama con contador; descartar y activar avisos no cierran el cajón, el enlace sí', () => {
  const host = montar(
    <ul>
      <ReminderCenter t={t} locale="es" estado={ESTADO} disparador="fila" />
    </ul>,
  );
  const rama = host.querySelector<HTMLButtonElement>('button[aria-expanded]')!;
  expect(rama.textContent).toContain('Recordatorios');
  expect(rama.hasAttribute('data-mantiene-nav')).toBe(true);
  expect(rama.getAttribute('aria-expanded')).toBe('false');
  act(() => rama.click());
  expect(rama.getAttribute('aria-expanded')).toBe('true');
  const botones = [...host.querySelectorAll<HTMLButtonElement>('ul[role="group"] button')];
  const enlace = botones.find((b) => b.textContent?.startsWith('Tenis'))!;
  const descartar = botones.find((b) => b.getAttribute('aria-label')?.includes('Tenis'))!;
  const avisos = botones.find((b) => b.textContent === t('reminders.enableNotifications'))!;
  expect(enlace.hasAttribute('data-mantiene-nav')).toBe(false);
  expect(descartar.hasAttribute('data-mantiene-nav')).toBe(true);
  expect(avisos.hasAttribute('data-mantiene-nav')).toBe(true);
  render(null, host);
  host.remove();
});

test('desde el cajón de AppShell, el enlace de un recordatorio lleva al día y cierra el cajón', () => {
  const dias: string[] = [];
  const estado = { ...ESTADO, irA: (r: typeof RECORDATORIO) => dias.push(r.dtstart) };
  const host = montar(
    <AppShell
      topbar={null}
      sidebar={
        <Sidebar>
          <SidebarSection title="Vista">
            <ReminderCenter t={t} locale="es" estado={estado} disparador="fila" />
          </SidebarSection>
        </Sidebar>
      }
    >
      contenido
    </AppShell>,
  );
  const cajon = host.querySelector('aside')!;
  const menu = host.querySelector<HTMLButtonElement>('.sytue-mobilebar button')!;
  act(() => menu.click());
  expect(cajon.className).not.toContain('max-md:hidden');
  const rama = cajon.querySelector<HTMLButtonElement>('button[aria-expanded]')!;
  act(() => rama.click());
  // Desplegar la rama no cierra el cajón.
  expect(cajon.className).not.toContain('max-md:hidden');
  const enlace = [...cajon.querySelectorAll<HTMLButtonElement>('ul[role="group"] button')].find(
    (b) => b.textContent?.startsWith('Tenis'),
  )!;
  act(() => enlace.click());
  expect(dias).toEqual(['2026-11-06T13:00:00Z']);
  expect(cajon.className).toContain('max-md:hidden');
  render(null, host);
  host.remove();
});

test('Escape en el cajón con la rama desplegada: cierra el cajón, el foco al ☰ y la rama sigue', () => {
  const host = montar(
    <AppShell
      topbar={null}
      sidebar={
        <Sidebar>
          <SidebarSection title="Vista">
            <ReminderCenter t={t} locale="es" estado={ESTADO} disparador="fila" />
          </SidebarSection>
        </Sidebar>
      }
    >
      contenido
    </AppShell>,
  );
  const cajon = host.querySelector('aside')!;
  const menu = host.querySelector<HTMLButtonElement>('.sytue-mobilebar button')!;
  act(() => menu.click());
  const rama = cajon.querySelector<HTMLButtonElement>('button[aria-expanded]')!;
  act(() => rama.click());
  expect(rama.getAttribute('aria-expanded')).toBe('true');
  // Desde un botón del panel, como en el teléfono: el Escape burbujea desde el foco.
  const dentro = cajon.querySelector<HTMLButtonElement>('ul[role="group"] button')!;
  dentro.focus();
  act(() => {
    dentro.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  });
  expect(cajon.className).toContain('max-md:hidden');
  expect(document.activeElement).toBe(menu);
  // La rama conserva su estado, como cualquier rama del lateral (y `recEnCajon` en Fyne).
  expect(rama.getAttribute('aria-expanded')).toBe('true');
  render(null, host);
  host.remove();
});
