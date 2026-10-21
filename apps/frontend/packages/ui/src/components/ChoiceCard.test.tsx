/**
 * Tests de ChoiceCard — su contrato: elegirla avisa una sola vez, `disabled` la deja
 * inerte, y el grupo es un radio nativo (mismo `name` ⇒ una sola opción marcada, se
 * navega con las flechas). Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { ChoiceCard } from './ChoiceCard.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

function radios(host: HTMLElement): HTMLInputElement[] {
  return Array.from(host.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
}

test('elegir una opción avisa al llamante', () => {
  let picked = 0;
  const host = mount(
    <ChoiceCard
      name="ambito"
      title="Solo el correo"
      description="Se preserva el buzón."
      selected={false}
      onSelect={() => picked++}
    />,
  );

  radios(host)[0]?.click();
  expect(picked).toBe(1);
  render(null, host);
});

test('disabled deja la tarjeta inerte', () => {
  let picked = 0;
  const host = mount(
    <ChoiceCard
      name="ambito"
      title="Archivos"
      selected={false}
      disabled
      onSelect={() => picked++}
    />,
  );

  const radio = radios(host)[0];
  expect(radio?.disabled).toBe(true);
  radio?.click();
  expect(picked).toBe(0);
  render(null, host);
});

test('el mismo name agrupa los radios: solo una queda marcada', () => {
  const host = mount(
    <>
      <ChoiceCard name="ambito" title="Correo" value="mail" selected onSelect={() => {}} />
      <ChoiceCard
        name="ambito"
        title="Archivos"
        value="drive"
        selected={false}
        onSelect={() => {}}
      />
    </>,
  );

  const [mail, drive] = radios(host);
  expect(mail?.name).toBe('ambito');
  expect(drive?.name).toBe('ambito');
  expect(mail?.value).toBe('mail');
  expect(mail?.checked).toBe(true);
  expect(drive?.checked).toBe(false);
  render(null, host);
});

test('sin value, el radio usa el título (grupo legible en el DOM)', () => {
  const host = mount(
    <ChoiceCard name="g" title="Toda la organización" selected onSelect={() => {}} />,
  );
  expect(radios(host)[0]?.value).toBe('Toda la organización');
  render(null, host);
});
