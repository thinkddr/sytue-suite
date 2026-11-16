/**
 * CopyField — su contrato: copia de verdad, lo dice en el propio botón y al lector
 * de pantalla, no finge cuando el portapapeles se niega, y un secreto no se copia.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import { CopyField } from './CopyField.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];
let copiados: string[] = [];
let rechazar = false;

beforeEach(() => {
  copiados = [];
  rechazar = false;
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: (s: string) => {
        if (rechazar) return Promise.reject(new Error('denegado'));
        copiados.push(s);
        return Promise.resolve();
      },
    },
  });
});

afterEach(() => {
  for (const host of hosts.splice(0)) {
    render(null, host);
    host.remove();
  }
});

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  act(() => render(<I18nProvider locale="es">{children}</I18nProvider>, host));
  return host;
}

async function pulsar(b: HTMLButtonElement): Promise<void> {
  await act(async () => {
    b.click();
    await Promise.resolve();
  });
}

const estado = (h: HTMLElement) => h.querySelector('[role="status"]')!;

test('copia el valor, dice «Copiado» en el botón y lo anuncia con el rótulo', async () => {
  let avisado = '';
  const h = mount(
    <CopyField
      label="Servidor IMAP"
      value="imap.acme.eu"
      onCopied={(v) => (avisado = v)}
      feedbackMs={10}
    />,
  );
  const b = h.querySelector('button')!;
  expect(b.getAttribute('aria-label')).toBe('Copiar Servidor IMAP');
  expect(estado(h).getAttribute('aria-live')).toBe('polite');
  expect(estado(h).textContent).toBe('');

  await pulsar(b);
  expect(copiados).toEqual(['imap.acme.eu']);
  expect(avisado).toBe('imap.acme.eu');
  expect(estado(h).textContent).toBe('Copiado: Servidor IMAP');
  expect(b.getAttribute('aria-label')).toBe('Copiado: Servidor IMAP');
  // Las dos palabras están siempre; cambia cuál se ve, no el ancho del botón.
  const [copiar, copiado] = [...b.querySelectorAll('[aria-hidden] > span')];
  expect(copiar!.className).toContain('invisible');
  expect(copiado!.className).not.toContain('invisible');

  await act(async () => {
    await new Promise((r) => setTimeout(r, 30));
  });
  expect(estado(h).textContent).toBe('');
  expect(b.getAttribute('aria-label')).toBe('Copiar Servidor IMAP');
});

test('si el portapapeles se niega, lo dice con palabras y no llama a onCopied', async () => {
  rechazar = true;
  let llamado = false;
  const h = mount(<CopyField label="Valor" value="v=spf1" onCopied={() => (llamado = true)} />);
  await pulsar(h.querySelector('button')!);
  expect(llamado).toBe(false);
  expect(estado(h).textContent).toBe(
    'No se ha podido copiar. Selecciona el valor y cópialo a mano.',
  );
  expect(estado(h).className).not.toContain('sr-only');
});

test('sin valor, una raya y el botón inerte', () => {
  const h = mount(<CopyField label="Servidor" value="" />);
  expect(h.querySelector('code')!.textContent).toBe('—');
  expect(h.querySelector('button')!.disabled).toBe(true);
});

test('un secreto no tiene botón de copiar: se marca y lleva la acción de la app', () => {
  const h = mount(
    <CopyField label="Clave" value="sk_live_…f3" secret action={<a href="/vars">Gestionar</a>} />,
  );
  expect(h.querySelector('button')).toBeNull();
  expect(h.textContent).toContain('secreto');
  expect(h.querySelector('a')!.getAttribute('href')).toBe('/vars');
});

test('el ancho del rótulo y la monoespaciada se eligen', () => {
  const h = mount(
    <div>
      <CopyField label="Tipo" value="TXT" labelWidth="sm" />
      <CopyField label="Nota" value="hola" labelWidth="stacked" mono={false} />
    </div>,
  );
  const [r1, , r2] = [...h.querySelectorAll('span.sytue-meta, code')];
  expect(r1!.className).toContain('sm:w-24');
  expect(r2!.className).not.toMatch(/sm:w-/);
  const codes = h.querySelectorAll('code');
  expect(codes[0]!.className).toContain('font-mono');
  expect(codes[1]!.className).not.toContain('font-mono');
});
