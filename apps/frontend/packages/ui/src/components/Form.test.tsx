/**
 * Tests de los controles de formulario. Cubren la talla del `Input` y, sobre todo, el
 * empate de utilidades que la hizo posible: dos clases de tamaño de letra en el mismo
 * elemento no se resuelven por el orden en que se escriben, sino por el orden en la hoja
 * generada. Por eso cada talla declara el suyo y la base ya no trae ninguno.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { Input, Textarea, Select, FieldGroup, Checkbox, Radio, Switch } from './Form.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function montar(ui: preact.JSX.Element): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return host;
}

test('sin talla, el Input es el control de formulario de siempre (h-9)', () => {
  const host = montar(<Input />);
  const cls = host.querySelector('input')!.className;
  expect(cls).toContain('h-9');
  expect(cls).toContain('text-base');
  render(null, host);
});

test('`sm` es el control de cromo denso: 24 px y letra pequeña', () => {
  const host = montar(<Input size="sm" />);
  const cls = host.querySelector('input')!.className;
  // `min-h-[24px]` y no solo `h-6`: `h-6` son 1,5 rem, 21 px con la raíz a 14.
  expect(cls).toContain('min-h-[24px]');
  expect(cls).toContain('text-xs');
  // Y NO arrastra el tamaño del control grande: dos utilidades de tamaño de letra en el
  // mismo elemento son un empate que Tailwind resuelve por la hoja, no por la cadena.
  expect(cls).not.toContain('text-base');
  render(null, host);
});

test('el icono se recoloca con la talla, o se sale del control', () => {
  const grande = montar(<Input leadingIcon={<svg />} />);
  expect(grande.querySelector('span')!.className).toContain('left-3');
  expect(grande.querySelector('input')!.className).toContain('pl-9');
  render(null, grande);

  const chico = montar(<Input size="sm" leadingIcon={<svg />} />);
  expect(chico.querySelector('span')!.className).toContain('left-1.5');
  expect(chico.querySelector('input')!.className).toContain('pl-7');
  render(null, chico);
});

test('Textarea y Select conservan su tamaño de letra tras sacarlo de la base', () => {
  const ta = montar(<Textarea />);
  expect(ta.querySelector('textarea')!.className).toContain('text-base');
  render(null, ta);

  const sel = montar(
    <Select>
      <option>a</option>
    </Select>,
  );
  expect(sel.querySelector('select')!.className).toContain('text-base');
  render(null, sel);
});

test('`invalid` se anuncia, no solo se pinta', () => {
  const host = montar(<Input invalid />);
  expect(host.querySelector('input')!.getAttribute('aria-invalid')).toBe('true');
  render(null, host);
});

test('el aviso de campo obligatorio habla el idioma de la pagina', () => {
  // Estaba en duro en castellano, asi que un usuario en aleman oia «(obligatorio)».
  // Es la unica pista de que el campo es obligatorio para quien no ve el asterisco.
  const de = montar(
    <I18nProvider locale="de">
      <FieldGroup label="Name" htmlFor="n" required>
        <Input id="n" />
      </FieldGroup>
    </I18nProvider>,
  );
  const oculto = de.querySelector('.sr-only')!.textContent!;
  expect(oculto).toContain('erforderlich');
  expect(oculto).not.toContain('obligatorio');
  render(null, de);

  // Y sin `required` no se anuncia nada, que si no todo campo pareceria obligatorio.
  const suelto = montar(
    <I18nProvider locale="de">
      <FieldGroup label="Name" htmlFor="n2">
        <Input id="n2" />
      </FieldGroup>
    </I18nProvider>,
  );
  expect(suelto.querySelector('.sr-only')).toBeNull();
  render(null, suelto);
});

test('`Select` `sm` también mide 24 px de verdad', () => {
  const host = montar(
    <Select size="sm">
      <option>a</option>
    </Select>,
  );
  expect(host.querySelector('select')!.className).toContain('min-h-[24px]');
  render(null, host);
});

test('el aro de la casilla y el radio lo lleva un envoltorio con su forma', () => {
  // Chromium deja a 0 el `border-radius` de un `<input>` casilla o radio nativo: con el
  // aro en el `<input>` salía un cuadrado de 18 alrededor del círculo. Lo lleva el
  // `<span>`, redondeado como lo pinta Chromium: 2px la casilla, un círculo el radio.
  for (const [Control, esquina] of [
    [Checkbox, 'rounded-[2px]'],
    [Radio, 'rounded-full'],
  ] as const) {
    const host = montar(<Control label="Etiqueta" />);
    const input = host.querySelector('input')!;
    const envoltorio = input.parentElement!;
    expect(envoltorio.tagName).toBe('SPAN');
    expect(envoltorio.className).toContain('sytue-focus-ring-choice');
    expect(envoltorio.className).toContain(esquina);
    expect(input.className).not.toContain('sytue-focus-ring');
    render(null, host);
  }
});

test('con el dedo, campos, selectores, casillas e interruptores miden 44 como un botón', () => {
  // Las clases responsivas se resuelven por CSS (medido con Playwright en 390×844 táctil:
  // de 36 y 22 a 44); este test fija el contrato. `sm` es cromo denso y no lo lleva.
  const SUELO = 'pointer-coarse:min-h-[max(44px,2.75rem)]';
  const host = montar(
    <>
      <Input id="a" />
      <Input id="b" leadingIcon={<span />} />
      <Select id="c">
        <option>Uno</option>
      </Select>
      <Checkbox label="Todo el día" />
      <Radio label="Solo este" name="r" />
      <Switch label="Avisarme" />
      <Input id="d" size="sm" />
    </>,
  );
  for (const sel of ['#a', '#b', '#c']) expect(host.querySelector(sel)!.className).toContain(SUELO);
  for (const l of host.querySelectorAll('label')) expect(l.className).toContain(SUELO);
  expect(host.querySelector('#d')!.className).not.toContain(SUELO);
  render(null, host);
});
