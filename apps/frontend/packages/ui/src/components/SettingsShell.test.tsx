/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ÍNDICE DE SECCIONES DE `SettingsShell`.
 *
 * Una página larga de ajustes (Mi cuenta) se recorre por su índice y se enlaza por
 * sección: «/account#seguridad» tiene que abrir ahí y con el foco en el título. Se
 * vigila:
 *
 * - que SIN `indice` la página sale byte a byte como antes (la montan quince pantallas);
 * - que el índice es un `nav` con nombre, con enlaces `#id` y `aria-current` en la
 *   sección que se lee;
 * - que pulsar una entrada desplaza, deja el ancla en la URL y lleva el foco al
 *   encabezado de la sección;
 * - que al abrir con `#id` (o con un alias, o con `seccionInicial`) se salta allí;
 * - que el observador marca la sección que asoma arriba.
 *
 * Vistos en ROJO: sin el foco al encabezado (falla «lleva el foco»), sin leer el
 * fragmento al montar (falla «abre en #id»), sin el alias y con `seccionInicial`
 * mandando sobre el `#id` de la URL.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import { SettingsSection, SettingsShell, seccionDelFragmento } from './SettingsShell.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account' });
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
let desplazados: string[];
const scrollOriginal = HTMLElement.prototype.scrollIntoView;
const rafOriginal = globalThis.requestAnimationFrame;

beforeEach(() => {
  // Si otro fichero registró antes el DOM compartido, la página es `about:blank`, y ahí
  // `replaceState` no puede poner ni ruta ni fragmento.
  (window as unknown as { happyDOM: { setURL: (u: string) => void } }).happyDOM.setURL(
    'https://id.sytue.test/account',
  );
  desplazados = [];
  HTMLElement.prototype.scrollIntoView = function (this: HTMLElement) {
    desplazados.push(this.id);
  };
  // El rAF de verdad: otros ficheros de la suite lo sustituyen en el DOM compartido.
  globalThis.requestAnimationFrame = ((cb: FrameRequestCallback) =>
    setTimeout(() => cb(0), 0)) as unknown as typeof requestAnimationFrame;
  host = document.createElement('div');
  document.body.appendChild(host);
});

afterEach(() => {
  act(() => render(null, host));
  host.remove();
  HTMLElement.prototype.scrollIntoView = scrollOriginal;
  globalThis.requestAnimationFrame = rafOriginal;
});

function monta(arbol: ComponentChild): void {
  act(() => {
    render(<I18nProvider locale="es">{arbol}</I18nProvider>, host);
  });
}

/** Deja correr el `requestAnimationFrame` del salto inicial. */
async function trasPintar(): Promise<void> {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 40));
  });
}

const INDICE = [
  { id: 'perfil', rotulo: 'Perfil' },
  { id: 'seguridad', rotulo: 'Seguridad', alias: ['claves'] },
  { id: 'borrar', rotulo: 'Borrar mi cuenta' },
];

function pagina(props: { seccionInicial?: string } = {}): ComponentChild {
  return (
    <SettingsShell title="Mi cuenta" description="Tu perfil" indice={INDICE} {...props}>
      <SettingsSection id="perfil" title="Perfil">
        uno
      </SettingsSection>
      <SettingsSection id="seguridad" title="Seguridad">
        dos
      </SettingsSection>
      <SettingsSection id="borrar" title="Borrar mi cuenta" plain>
        tres
      </SettingsSection>
    </SettingsShell>
  );
}

const enlaces = (): HTMLAnchorElement[] => [...host.querySelectorAll<HTMLAnchorElement>('nav a')];
const actual = (): string | null =>
  host.querySelector('nav a[aria-current="location"]')?.getAttribute('href') ?? null;

test('sin índice, la página sale exactamente como antes', () => {
  monta(
    <SettingsShell
      title="Ajustes"
      description="Qué se configura"
      actions={<b>acción</b>}
      banner={<i>aviso</i>}
    >
      <SettingsSection title="Uno" description="d">
        contenido
      </SettingsSection>
      <SettingsSection title="Dos" plain>
        libre
      </SettingsSection>
    </SettingsShell>,
  );
  // Comparado como árbol y no como texto: el orden de los atributos al serializar
  // cambia según qué otro fichero haya montado antes el DOM compartido.
  const antes = document.createElement('template');
  antes.innerHTML =
    '<div class="mx-auto w-full max-w-5xl px-4 py-6 sm:px-6"><div class="flex flex-wrap items-start gap-4"><div class="min-w-0 flex-1"><h1 class="sytue-h2 text-text">Ajustes</h1><p class="text-text-muted mt-1.5 max-w-2xl text-base leading-relaxed">Qué se configura</p></div><div class="flex shrink-0 items-center gap-2"><b>acción</b></div></div><div class="mt-4"><i>aviso</i></div><div class="mt-6 flex flex-col gap-6"><div class="rounded-card min-w-0 border sytue-glass-surface border-border-subtle bg-surface text-text shadow-[var(--shadow-sm)]" role="region" aria-label="Uno"><div class="px-4 py-4 flex flex-col gap-4"><div class="flex flex-wrap items-start gap-3"><div class="min-w-0 flex-1"><h2 class="sytue-h4 text-text">Uno</h2><p class="text-text-muted mt-1 max-w-2xl text-sm leading-relaxed">d</p></div></div>contenido</div></div><section class="flex flex-col gap-3"><div class="flex flex-wrap items-start gap-3"><div class="min-w-0 flex-1"><h2 class="sytue-h4 text-text">Dos</h2></div></div>libre</section></div></div>';
  expect(host.firstElementChild!.isEqualNode(antes.content.firstElementChild)).toBe(true);
});

describe('con índice', () => {
  test('es un nav con nombre, con un enlace #id por sección y la primera marcada', () => {
    monta(pagina());
    const nav = host.querySelector('nav');
    expect(nav?.getAttribute('aria-label')).toBe('Secciones de esta página');
    expect(enlaces().map((a) => a.getAttribute('href'))).toEqual([
      '#perfil',
      '#seguridad',
      '#borrar',
    ]);
    expect(enlaces().map((a) => a.textContent)).toEqual([
      'Perfil',
      'Seguridad',
      'Borrar mi cuenta',
    ]);
    expect(actual()).toBe('#perfil');
    // La fila del teléfono desborda A PROPÓSITO: lo dice al barrido E2E.
    expect(nav?.querySelector('ul')?.hasAttribute('data-scroll-x')).toBe(true);
    // El título de la página sigue siendo el h1, y cada sección su `id`.
    expect(host.querySelector('h1')?.textContent).toBe('Mi cuenta');
    expect(host.querySelector('#seguridad h2')?.textContent).toBe('Seguridad');
  });

  test('pulsar una entrada desplaza, deja el ancla en la URL y lleva el foco al encabezado', () => {
    monta(pagina());
    const [, seguridad] = enlaces();
    act(() => {
      seguridad!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    });
    expect(desplazados).toEqual(['seguridad']);
    expect(window.location.hash).toBe('#seguridad');
    const titulo = host.querySelector<HTMLElement>('#seguridad h2')!;
    expect(document.activeElement).toBe(titulo);
    // Enfocable por programa, pero fuera del orden del tabulador.
    expect(titulo.getAttribute('tabindex')).toBe('-1');
    expect(actual()).toBe('#seguridad');
  });

  test('con Ctrl (abrir en otra pestaña) el navegador hace lo suyo', () => {
    monta(pagina());
    const ev = new MouseEvent('click', { bubbles: true, cancelable: true, ctrlKey: true });
    act(() => {
      enlaces()[2]!.dispatchEvent(ev);
    });
    expect(ev.defaultPrevented).toBe(false);
    expect(desplazados).toEqual([]);
  });

  test('abre en el #id de la URL, con el foco en su encabezado', async () => {
    window.history.replaceState(null, '', '/account#borrar');
    monta(pagina());
    await trasPintar();
    expect(desplazados).toEqual(['borrar']);
    expect(document.activeElement).toBe(host.querySelector('#borrar h2'));
    expect(actual()).toBe('#borrar');
  });

  test('un alias abre la sección a la que apunta', async () => {
    window.history.replaceState(null, '', '/account#claves');
    monta(pagina());
    await trasPintar();
    expect(desplazados).toEqual(['seguridad']);
    expect(actual()).toBe('#seguridad');
  });

  test('sin #id, abre en la sección inicial; con #id, manda la URL', async () => {
    monta(pagina({ seccionInicial: 'seguridad' }));
    await trasPintar();
    expect(desplazados).toEqual(['seguridad']);
    expect(document.activeElement).toBe(host.querySelector('#seguridad h2'));

    act(() => render(null, host));
    desplazados = [];
    window.history.replaceState(null, '', '/account?mfa=required#perfil');
    monta(pagina({ seccionInicial: 'seguridad' }));
    await trasPintar();
    expect(desplazados).toEqual(['perfil']);
  });

  test('sin #id ni sección inicial no se mueve nada', async () => {
    monta(pagina());
    await trasPintar();
    expect(desplazados).toEqual([]);
  });

  test('un enlace #id de dentro de la página (hashchange) también salta y enfoca', () => {
    monta(pagina());
    window.history.replaceState(null, '', '/account#seguridad');
    act(() => {
      window.dispatchEvent(new HashChangeEvent('hashchange'));
    });
    expect(desplazados).toEqual(['seguridad']);
    expect(document.activeElement).toBe(host.querySelector('#seguridad h2'));
  });

  test('un enlace #id del contenido salta y enfoca aunque la URL ya traiga ese #id', () => {
    // Pasa en Mi cuenta: se entra por el índice a «Seguridad» y luego se pulsa el
    // aviso de la revisión que lleva a `#seguridad`. Sin cambio de fragmento el
    // navegador no emite `hashchange`, así que el foco se quedaba en el aviso.
    window.history.replaceState(null, '', '/account#seguridad');
    monta(
      <SettingsShell title="Mi cuenta" indice={INDICE}>
        <a href="#seguridad">ir</a>
        <a href="#otra-cosa">fuera</a>
        <SettingsSection id="perfil" title="Perfil">
          uno
        </SettingsSection>
        <SettingsSection id="seguridad" title="Seguridad">
          dos
        </SettingsSection>
      </SettingsShell>,
    );
    desplazados = [];
    const [ir, fuera] = [...host.querySelectorAll<HTMLAnchorElement>('nav ~ div a')];
    const ajeno = new MouseEvent('click', { bubbles: true, cancelable: true });
    act(() => {
      fuera!.dispatchEvent(ajeno);
    });
    expect(ajeno.defaultPrevented).toBe(false);
    act(() => {
      ir!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    });
    expect(desplazados).toEqual(['seguridad']);
    expect(document.activeElement).toBe(host.querySelector('#seguridad h2'));
  });

  test('marca la primera sección que asoma por arriba al desplazar', () => {
    type Cb = (e: { target: Element; isIntersecting: boolean }[]) => void;
    const original = globalThis.IntersectionObserver;
    let avisar: Cb = () => {};
    globalThis.IntersectionObserver = class {
      constructor(cb: Cb) {
        avisar = cb;
      }
      observe(): void {}
      disconnect(): void {}
    } as unknown as typeof IntersectionObserver;
    try {
      monta(pagina());
      const el = (id: string): Element => host.querySelector(`#${id}`)!;
      act(() =>
        avisar([
          { target: el('perfil'), isIntersecting: false },
          { target: el('seguridad'), isIntersecting: true },
          { target: el('borrar'), isIntersecting: true },
        ]),
      );
      expect(actual()).toBe('#seguridad');
    } finally {
      globalThis.IntersectionObserver = original;
    }
  });
});

test('seccionDelFragmento: por id, por alias, y nada si no es del índice', () => {
  expect(seccionDelFragmento(INDICE, '#seguridad')).toBe('seguridad');
  expect(seccionDelFragmento(INDICE, '#claves')).toBe('seguridad');
  expect(seccionDelFragmento(INDICE, '#otra')).toBeNull();
  expect(seccionDelFragmento(INDICE, '')).toBeNull();
  expect(seccionDelFragmento(INDICE, '#%E0%A4%A')).toBeNull();
});
