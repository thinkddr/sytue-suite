/**
 * Tests de AppGrid — su contrato: renderiza la cuadrícula de aplicaciones,
 * aplica aria-disabled y elimina href en aplicaciones desactivadas.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { AppGrid } from './AppGrid.js';
import type { SuiteApp } from '../lib/apps.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

const appsPrueba: SuiteApp[] = [
  {
    key: 'mail',
    name: 'Sytue Mail',
    href: 'https://mail.sytue.eu',
    iconUrl: '/mail.svg',
    iconUrlSm: '/mail-sm.svg',
    color: '#dc3450',
  },
  {
    key: 'erp',
    name: 'ERP',
    href: 'https://erp.sytue.eu',
    iconUrl: '/erp.svg',
    iconUrlSm: '/erp-sm.svg',
    color: '#b45309',
    disabled: true,
  },
];

test('renderiza apps normales con href y apps desactivadas con aria-disabled y sin href', () => {
  const host = mount(<AppGrid apps={appsPrueba} />);
  const items = Array.from(host.querySelectorAll<HTMLAnchorElement>('[role="menuitem"]'));
  expect(items).toHaveLength(2);

  const [mail, erp] = items;
  expect(mail.getAttribute('href')).toBe('https://mail.sytue.eu');
  expect(mail.getAttribute('aria-disabled')).toBeNull();

  expect(erp.getAttribute('href')).toBeNull();
  expect(erp.getAttribute('aria-disabled')).toBe('true');
  expect(erp.getAttribute('title')).toContain('ERP');

  // Clic en item desactivado
  let prevented = false;
  const event = new MouseEvent('click', { cancelable: true });
  erp.dispatchEvent(event);
  if (event.defaultPrevented) prevented = true;
  expect(prevented).toBe(true);

  render(null, host);
});
