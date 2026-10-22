/**
 * DataFlow — su contrato: la dirección sale de lo que se enumera, y la frase
 * dice qué política cubre cada lado. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { DataFlow, dataFlowMode } from './DataFlow.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<I18nProvider locale="es">{children}</I18nProvider>, host);
  return host;
}

test('la dirección se deduce de las listas', () => {
  expect(dataFlowMode(['files'], [])).toBe('in');
  expect(dataFlowMode([], ['identity'])).toBe('out');
  expect(dataFlowMode(['messages'], ['messages'])).toBe('both');
  expect(dataFlowMode()).toBe('none');
});

test('solo entra: nuestra política y ni una palabra de la del tercero', () => {
  const texto = mount(<DataFlow partner="Holded" incoming={['contacts', 'records']} />).textContent;
  expect(texto).toContain('Solo entra en Sytue One');
  expect(texto).toContain('Contactos');
  expect(texto).toContain('política de privacidad de Sytue One');
  expect(texto).not.toContain('política de privacidad de Holded');
});

test('en los dos sentidos: las dos políticas y el enlace a la nuestra', () => {
  const host = mount(<DataFlow partner="Slack" incoming={['messages']} outgoing={['messages']} />);
  expect(host.textContent).toContain('En los dos sentidos');
  expect(host.textContent).toContain('Sale hacia Slack');
  expect(host.textContent).toContain('política de privacidad de Slack');
  expect(host.querySelector('a')?.getAttribute('href')).toMatch(/\/es\/privacy$/);
});

test('sin intercambio: no promete ninguna política', () => {
  const host = mount(<DataFlow partner="Trello" />);
  expect(host.textContent).toContain('Sin intercambio de datos');
  expect(host.querySelector('a')).toBeNull();
});

test('compacto: una línea con la dirección y el enlace, sin categorías', () => {
  const host = mount(<DataFlow compact incoming={['mail']} />);
  expect(host.textContent).toContain('Solo entra en Sytue One');
  expect(host.textContent).not.toContain('Correo');
  expect(host.querySelector('a')).not.toBeNull();
});

test('compacto hacia fuera: dice que sale sin nombrar a nadie', () => {
  expect(mount(<DataFlow compact outgoing={['messages']} />).textContent).toContain(
    'Sale de Sytue One',
  );
});
