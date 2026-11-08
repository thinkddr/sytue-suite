/*
 * «PAUSAR AVISOS» SE OYE CON SU CONTEXTO.
 *
 * Las cinco duraciones cuelgan de un título que es un <p> suelto: un lector de
 * pantalla que recorre el menú por sus ítems oía «Durante 1 hora» sin saber de
 * qué (docs/design.md §2.6). Cada opción lleva el título en su nombre accesible.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { roomsT } from '../i18n/index.js';
import { StatusMenu } from './StatusMenu.js';

// Solo aquí, y se retira al acabar: el resto de la suite prueba los BFF con el
// fetch/Request/Response de Bun.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

test('cada duración de «Pausar avisos» dice en su nombre que pausa los avisos', async () => {
  const t = roomsT('es');
  const host = document.createElement('div');
  document.body.appendChild(host);
  try {
    await act(async () => {
      render(
        <StatusMenu
          state="online"
          status=""
          onChange={() => {}}
          dmNotify={false}
          onToggleDmNotify={() => {}}
          onOpenNotifySettings={() => {}}
          pausedUntilLabel={null}
          onPause={() => {}}
          onMarkAllRead={() => {}}
          onOpenIntegrations={() => {}}
          canManageIntegrations={false}
          t={t}
        />,
        host,
      );
    });
    await act(async () => {
      host.querySelector<HTMLButtonElement>('button[aria-haspopup]')!.click();
    });
    const items = [...document.querySelectorAll('[role="menuitem"]')];
    const hora = items.find((b) => b.textContent?.trim() === t('pause.1h'));
    expect(hora).toBeDefined();
    expect(hora!.getAttribute('aria-label')).toBe('Pausar avisos: Durante 1 hora');
  } finally {
    render(null, host);
    host.remove();
  }
});
