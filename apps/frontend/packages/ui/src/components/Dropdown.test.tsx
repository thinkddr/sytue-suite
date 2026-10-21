/**
 * Dropdown — dónde va el panel. Al body, para que ningún contexto de apilamiento
 * lo tape; salvo dentro de un <dialog>, donde va al propio diálogo: `showModal()`
 * deja inerte todo lo de fuera y el menú no se podía tocar (el visor de Sytue
 * Fotos, 2026-09-15). Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { Dropdown, MenuItem, encajarPanel } from './Dropdown.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

/** Los efectos de Preact corren «tras el pintado»; en happy-dom, con un setTimeout. */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 150));

async function abrirEn(host: HTMLElement): Promise<HTMLElement | null> {
  render(
    <Dropdown
      label="Descargar"
      trigger={(props) => (
        <button type="button" {...props}>
          Descargar
        </button>
      )}
    >
      <MenuItem onClick={() => {}}>JPEG</MenuItem>
    </Dropdown>,
    host,
  );
  host.querySelector('button')?.click();
  await tick();
  return document.querySelector<HTMLElement>('[role="menu"]');
}

test('fuera de un diálogo, el panel va al body', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  const menu = await abrirEn(host);
  expect(menu?.parentElement).toBe(document.body);
  render(null, host);
  host.remove();
});

test('dentro de un <dialog>, el panel va al diálogo y no queda debajo', async () => {
  const dialogo = document.body.appendChild(document.createElement('dialog'));
  const host = dialogo.appendChild(document.createElement('div'));
  const menu = await abrirEn(host);
  expect(menu?.parentElement).toBe(dialogo);
  render(null, host);
  dialogo.remove();
});

/** Menú de idiomas como el de www: 26 enlaces, uno marcado como actual. */
async function abrirIdiomas(host: HTMLElement): Promise<HTMLElement> {
  const nombres = ['English', 'Dansk', 'Deutsch', 'Español', 'Eesti', 'Nederlands', 'Svenska'];
  render(
    <Dropdown
      label="Idioma"
      trigger={(props) => (
        <button type="button" {...props}>
          Idioma
        </button>
      )}
    >
      {nombres.map((n) => (
        <MenuItem key={n} href={`#${n}`} aria-current={n === 'Deutsch' ? 'true' : undefined}>
          {n}
        </MenuItem>
      ))}
    </Dropdown>,
    host,
  );
  host.querySelector('button')?.click();
  await tick();
  return document.querySelector<HTMLElement>('[role="menu"]')!;
}

const tecla = (key: string): void => {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }));
};
const enFoco = (): string => document.activeElement?.textContent ?? '';

test('teclado de menú: actual al abrir, Inicio/Fin, escribir para saltar, Tab cierra', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  const menu = await abrirIdiomas(host);

  expect(menu.className).toContain('overflow-y-auto');
  expect(enFoco()).toBe('Deutsch');
  // Foco itinerante: ningún item en el orden de Tab.
  for (const el of menu.querySelectorAll<HTMLElement>('[role="menuitem"]')) {
    expect(el.tabIndex).toBe(-1);
  }

  tecla('End');
  expect(enFoco()).toBe('Svenska');
  tecla('Home');
  expect(enFoco()).toBe('English');

  // «e» repetida recorre las que empiezan por e/é, sin distinguir tildes.
  tecla('e');
  expect(enFoco()).toBe('Español');
  tecla('e');
  expect(enFoco()).toBe('Eesti');
  await new Promise((r) => setTimeout(r, 600));
  tecla('n');
  tecla('e');
  expect(enFoco()).toBe('Nederlands');

  tecla('Tab');
  await tick();
  expect(document.querySelector('[role="menu"]')).toBeNull();
  expect(document.activeElement).toBe(host.querySelector('button'));

  render(null, host);
  host.remove();
});

test('Escape cierra y devuelve el foco al disparador', async () => {
  const host = document.body.appendChild(document.createElement('div'));
  await abrirIdiomas(host);
  tecla('Escape');
  await tick();
  expect(document.querySelector('[role="menu"]')).toBeNull();
  expect(document.activeElement).toBe(host.querySelector('button'));
  render(null, host);
  host.remove();
});

// El panel no tapa su disparador: se queda en el hueco de un lado y lo que no entra se
// desplaza por dentro. Las medidas son las del menú de cuenta, que al ganar los
// selectores de color y forma pasó a medir más que el hueco bajo la barra (2026-10-05).
const AVATAR = { top: 10, bottom: 46, left: 1220, right: 1256 };
const MENU = { ancho: 264, alto: 765 };

for (const [ancho, alto] of [
  [1280, 800],
  [1280, 720],
  [390, 700],
] as const) {
  test(`el menú de cuenta a ${ancho}×${alto} empieza bajo su botón y no se sale`, () => {
    const d = { ...AVATAR, left: ancho - 60, right: ancho - 24 };
    const caja = encajarPanel(d, MENU, { ancho, alto }, 'bottom', 'end');
    expect(caja.top).toBe(d.bottom + 6);
    expect(caja.top + Math.min(MENU.alto, caja.altoMaximo)).toBeLessThanOrEqual(alto - 8);
    expect(caja.left).toBeGreaterThanOrEqual(8);
    expect(caja.left + MENU.ancho).toBeLessThanOrEqual(ancho - 8);
  });
}

test('lo que cabe bajo el disparador no se acota', () => {
  const caja = encajarPanel(
    AVATAR,
    { ancho: 220, alto: 300 },
    { ancho: 1280, alto: 800 },
    'bottom',
    'end',
  );
  expect(caja).toMatchObject({ top: 52, left: 1036 });
  expect(caja.altoMaximo).toBeGreaterThanOrEqual(300);
});

test('cerca del pie se abre hacia arriba, pegado al disparador y sin taparlo', () => {
  const d = { top: 740, bottom: 776, left: 600, right: 636 };
  const cabe = encajarPanel(
    d,
    { ancho: 220, alto: 300 },
    { ancho: 1280, alto: 800 },
    'bottom',
    'start',
  );
  expect(cabe.top).toBe(740 - 6 - 300);
  // Y si ni arriba cabe entero, se queda con ese hueco y se desplaza por dentro.
  const noCabe = encajarPanel(
    d,
    { ancho: 220, alto: 900 },
    { ancho: 1280, alto: 800 },
    'bottom',
    'start',
  );
  expect(noCabe.altoMaximo).toBe(740 - 6 - 8);
  expect(noCabe.top).toBe(8);
  expect(noCabe.top + noCabe.altoMaximo).toBeLessThanOrEqual(d.top - 6);
});

test('sin hueco en ningún lado, el panel se queda dentro de la ventana', () => {
  const d = { top: 60, bottom: 96, left: 10, right: 46 };
  const caja = encajarPanel(
    d,
    { ancho: 220, alto: 400 },
    { ancho: 320, alto: 180 },
    'bottom',
    'start',
  );
  expect(caja.altoMaximo).toBe(120);
  expect(caja.top).toBeGreaterThanOrEqual(8);
  expect(caja.top + caja.altoMaximo).toBeLessThanOrEqual(180 - 8);
});

test('el mínimo del menú cede cuando un teclado deja menos de120px de altura', () => {
  const caja = encajarPanel(
    { top: 40, bottom: 76, left: 10, right: 46 },
    { ancho: 220, alto: 400 },
    { ancho: 320, alto: 96 },
    'bottom',
    'start',
  );
  expect(caja.altoMaximo).toBe(80);
  expect(caja.top).toBeGreaterThanOrEqual(8);
  expect(caja.top + caja.altoMaximo).toBeLessThanOrEqual(88);
});

test('en el teléfono el menú es una hoja inferior con el velo de los diálogos', async () => {
  // happy-dom no sabe de anchos: se le dice que estamos por debajo de 768.
  const antes = window.matchMedia;
  window.matchMedia = ((q: string) => ({
    matches: q === '(width < 48rem)',
    media: q,
    addEventListener() {},
    removeEventListener() {},
  })) as never;
  const host = document.body.appendChild(document.createElement('div'));
  try {
    const menu = await abrirEn(host);
    // Las clases se resuelven por CSS (medido con Playwright en 390×844): el contrato.
    expect(menu?.className).toContain('sytue-hoja-movil');
    expect(menu?.className).toContain('inset-x-0!');
    expect(menu?.className).toContain('rounded-t-card!');
    expect(menu?.style.top).toBe('');
    const velo = menu?.previousElementSibling as HTMLElement | null;
    expect(velo?.className).toContain('bg-[var(--sytue-overlay)]');
    // Tocar el velo es tocar fuera: cierra.
    velo?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    await tick();
    expect(document.querySelector('[role="menu"]')).toBeNull();
  } finally {
    window.matchMedia = antes;
    render(null, host);
    host.remove();
  }
});
