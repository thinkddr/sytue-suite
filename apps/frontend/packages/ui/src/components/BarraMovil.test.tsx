/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA BARRA ÚNICA DEL TELÉFONO: la topbar de la suite acoge la barra de la app.
 *
 * Lo que se vigila:
 *   · sin `movil`, la topbar sale byte a byte como antes (el marcado de origin/main,
 *     copiado aquí): el sitio público y cualquier barra sin Shell no cambian;
 *   · con `movil`, en ≥768 nada cambia: todo lo añadido va bajo `md:hidden` y lo de la
 *     suite solo gana clases `max-md:`;
 *   · con la topbar montada, AppShell y AdminShell NO pintan su barra de 44: su ☰, su
 *     título y sus acciones caen dentro de la topbar (una sola barra);
 *   · la entrada a la suite despliega la cara de la suite, ✕ y Esc la pliegan y el foco
 *     vuelve a la entrada; el punto de avisos y su nombre.
 *
 * Vistos en ROJO: quitando `md:hidden` de los huecos (falla «en ≥768 nada cambia»),
 * pintando la barra de 44 también con contexto (falla «una sola barra»), dejando la
 * suite sin `max-md:hidden` en la cara plegada (falla «la cara de la app esconde la
 * suite») y sin el `setSuite(false)` de Esc (falla «Esc pliega»).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render, type ComponentChildren } from 'preact';
import { useState } from 'preact/hooks';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import { AdminShell } from './AdminShell.js';
import { AppShell } from './AppShell.js';
import { BarraUnicaContext, type DestinosBarra } from './BarraMovil.js';
import { SectionHeader } from './SectionHeader.js';
import { TopBar } from './TopBar.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

/** El marcado de la topbar en origin/main antes de la barra única. */
const ANTES_CARRIL =
  '<header role="banner" class="sytue-topbar sytue-glass-surface bg-topbar text-topbar-fg shadow-[inset_0_-1px_0_var(--color-border)] sticky top-0 flex items-center gap-3 px-4 lg:px-6 sytue-topbar-carril" style="height: var(--spacing-topbar); z-index: var(--z-topbar);"><div class="-ms-4 flex shrink-0 self-stretch md:w-[var(--spacing-sidebar)] lg:-ms-6"><div class="sytue-esquina-marca bg-sidebar text-sidebar-fg flex w-full items-center gap-2 ps-4 pe-3"><b>marca</b></div></div><div class="flex min-w-0 flex-1 justify-center"><div class="flex w-full max-w-[30rem] justify-center"><i>buscar</i></div></div><div class="flex shrink-0 items-center gap-1"><u>cuenta</u></div></header>';
const ANTES_PLANA =
  '<header role="banner" class="sytue-topbar sytue-glass-surface bg-topbar text-topbar-fg shadow-[inset_0_-1px_0_var(--color-border)] sticky top-0 flex items-center gap-3 px-4 lg:px-6" style="height: var(--spacing-topbar); z-index: var(--z-topbar);"><div class="flex shrink-0 items-center gap-2"><b>marca</b></div><div class="flex-1"></div><div class="flex shrink-0 items-center gap-1"><u>cuenta</u></div></header>';

function montar(arbol: ComponentChildren): { host: HTMLElement; quitar: () => void } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(<I18nProvider locale="es">{arbol}</I18nProvider>, host);
  });
  return {
    host,
    quitar: () => {
      render(null, host);
      host.remove();
    },
  };
}

/** El marcado con los atributos en orden: el de su escritura no significa nada. */
function ordenado(html: string): string {
  const caja = document.createElement('div');
  caja.innerHTML = html;
  for (const e of caja.querySelectorAll('*')) {
    const attrs = [...e.attributes].map((a) => [a.name, a.value] as const);
    for (const [n] of attrs) e.removeAttribute(n);
    for (const [n, v] of attrs.sort(([a], [b]) => a.localeCompare(b))) e.setAttribute(n, v);
  }
  return caja.innerHTML;
}

/** Lo que se ve en ≥768: fuera lo `md:hidden` y las clases `max-md:`. */
function enEscritorio(el: Element): string {
  const copia = el.cloneNode(true) as Element;
  // Solo estrecha el canalón por debajo de 768 (theme.css).
  copia.removeAttribute('data-barra-unica');
  for (const e of copia.querySelectorAll('*')) {
    if (e.classList.contains('md:hidden')) e.remove();
  }
  for (const e of [copia, ...copia.querySelectorAll('*')]) {
    const quedan = [...e.classList].filter((c) => !c.startsWith('max-md:'));
    if (e.hasAttribute('class')) e.setAttribute('class', quedan.join(' '));
  }
  return copia.outerHTML;
}

/** La Shell en miniatura: topbar con `movil` y los huecos publicados a los hijos. */
function Suite({ children, aviso }: { children: ComponentChildren; aviso?: string }) {
  const [destinos, setDestinos] = useState<DestinosBarra | null>(null);
  return (
    <>
      <TopBar
        rail
        brand={<button type="button">marca</button>}
        center={<i>buscar</i>}
        actions={<u>cuenta</u>}
        movil={{
          titulo: 'Mail',
          entrada: <span>AG</span>,
          ...(aviso ? { aviso: { texto: aviso } } : {}),
          alDestinos: setDestinos,
        }}
      />
      <BarraUnicaContext.Provider value={{ destinos }}>{children}</BarraUnicaContext.Provider>
    </>
  );
}

const menu = <nav>menú</nav>;

test('sin `movil`, la topbar sale byte a byte como antes', () => {
  const a = montar(
    <TopBar rail brand={<b>marca</b>} center={<i>buscar</i>} actions={<u>cuenta</u>} />,
  );
  expect(ordenado(a.host.innerHTML)).toBe(ordenado(ANTES_CARRIL));
  a.quitar();
  const b = montar(<TopBar brand={<b>marca</b>} actions={<u>cuenta</u>} />);
  expect(ordenado(b.host.innerHTML)).toBe(ordenado(ANTES_PLANA));
  b.quitar();
});

test('con `movil`, en ≥768 nada cambia', () => {
  const m = montar(
    <TopBar
      rail
      brand={<b>marca</b>}
      center={<i>buscar</i>}
      actions={<u>cuenta</u>}
      movil={{ titulo: 'Mail', entrada: <span>AG</span>, alDestinos: () => {} }}
    />,
  );
  expect(ordenado(enEscritorio(m.host.querySelector('header')!))).toBe(ordenado(ANTES_CARRIL));
  m.quitar();
});

test('con la topbar, AppShell no pinta su barra de 44: ☰, título y acciones van dentro', () => {
  const m = montar(
    <Suite>
      <AppShell
        topbar={null}
        sidebar={menu}
        mobileBarCenter={<h1>noviembre</h1>}
        mobileBarActions={<button type="button">Hoy</button>}
        mobileNavNotice="Abrir menú, 3 pendientes"
      >
        contenido
      </AppShell>
    </Suite>,
  );
  expect(m.host.querySelector('.sytue-mobilebar')).toBeNull();
  const header = m.host.querySelector('header')!;
  // Una sola barra: el ☰ (con su aviso), el mes y «Hoy» están en la topbar.
  const boton = header.querySelector<HTMLButtonElement>('button[aria-controls]')!;
  expect(boton.getAttribute('aria-label')).toBe('Abrir menú, 3 pendientes');
  expect(boton.querySelector('[data-aviso-nav]')).not.toBeNull();
  expect(header.querySelector('[data-centrado] h1')?.textContent).toBe('noviembre');
  expect([...header.querySelectorAll('button')].some((b) => b.textContent === 'Hoy')).toBe(true);
  // El nombre del producto solo se lee sin título de la app (lo esconde `peer-empty`).
  expect(header.querySelector('[data-centrado]')!.parentElement!.className).toContain('peer');
  // El ☰ sigue abriendo el cajón.
  act(() => boton.click());
  expect(boton.getAttribute('aria-expanded')).toBe('true');
  expect(m.host.querySelector('aside')!.className).toContain('sytue-drawer-in');
  m.quitar();
});

test('AdminShell también sube su ☰ y su título a la topbar', () => {
  const m = montar(
    <Suite>
      <AdminShell sidebar={menu} title="Mi unidad">
        contenido
      </AdminShell>
    </Suite>,
  );
  expect(m.host.querySelector('.sytue-mobilebar')).toBeNull();
  const header = m.host.querySelector('header')!;
  expect(header.querySelector('button[aria-controls]')).not.toBeNull();
  expect(header.textContent).toContain('Mi unidad');
  m.quitar();
});

test('sin topbar que la acoja (nativo), la barra de 44 de siempre', () => {
  const m = montar(
    <BarraUnicaContext.Provider value={null}>
      <AppShell topbar={null} sidebar={menu} title="Mail">
        contenido
      </AppShell>
    </BarraUnicaContext.Provider>,
  );
  expect(m.host.querySelector('.sytue-mobilebar button[aria-controls]')).not.toBeNull();
  m.quitar();
});

test('la entrada despliega la suite; ✕ y Esc la pliegan y el foco vuelve a la entrada', () => {
  const m = montar(
    <Suite aviso="Notificaciones (3 sin leer)">
      <AppShell topbar={null} sidebar={menu} title="Recibidos">
        contenido
      </AppShell>
    </Suite>,
  );
  const header = m.host.querySelector('header')!;
  const marca = (): HTMLElement => header.querySelector<HTMLElement>('.sytue-esquina-marca')!;
  // Plegada: la cara de la app esconde la suite en el teléfono.
  expect(marca().parentElement!.className).toContain('max-md:hidden');
  const entrada = header.querySelector<HTMLButtonElement>('button[aria-label^="Sytue"]')!;
  expect(entrada.getAttribute('aria-label')).toBe(
    'Sytue: aplicaciones, búsqueda, avisos y cuenta · Notificaciones (3 sin leer)',
  );
  // El punto de la campana, en la entrada.
  expect(entrada.querySelector('.bg-danger')).not.toBeNull();
  act(() => entrada.click());
  expect(marca().parentElement!.className).not.toContain('max-md:hidden');
  // El foco va a lo primero de la suite (el selector de aplicaciones).
  expect(document.activeElement?.textContent).toBe('marca');
  const cerrar = header.querySelector<HTMLButtonElement>(
    'button[aria-label="Volver a la aplicación"]',
  )!;
  act(() => cerrar.click());
  expect(marca().parentElement!.className).toContain('max-md:hidden');
  expect(document.activeElement?.getAttribute('aria-label')).toBe(
    entrada.getAttribute('aria-label'),
  );
  // Esc, desde dentro de la barra.
  act(() => header.querySelector<HTMLButtonElement>('button[aria-label^="Sytue"]')!.click());
  act(() => {
    header
      .querySelector('button')!
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  });
  expect(marca().parentElement!.className).toContain('max-md:hidden');
  m.quitar();
});

test('SectionHeader `repiteLaBarra`: con la barra única, el título solo para el lector y las acciones a la vista', () => {
  const con = montar(
    <BarraUnicaContext.Provider value={{ destinos: null }}>
      <SectionHeader
        level={1}
        title="Contactos"
        repiteLaBarra
        actions={<button type="button">Importar</button>}
      />
    </BarraUnicaContext.Provider>,
  );
  const h1 = con.host.querySelector('h1')!;
  expect(h1.textContent).toBe('Contactos');
  expect(h1.parentElement!.className).toContain('max-md:sr-only');
  expect(con.host.querySelector('button')!.closest('[class*="max-md:hidden"]')).toBeNull();
  con.quitar();
  // Sin acciones, la cabecera entera se va en el teléfono.
  const sola = montar(
    <BarraUnicaContext.Provider value={{ destinos: null }}>
      <SectionHeader level={1} title="Contactos" repiteLaBarra />
    </BarraUnicaContext.Provider>,
  );
  expect(sola.host.firstElementChild!.className).toContain('max-md:hidden');
  sola.quitar();
  // Sin barra única (la app nativa), como siempre.
  const sin = montar(<SectionHeader level={1} title="Contactos" repiteLaBarra />);
  expect(sin.host.querySelector('h1')!.parentElement!.className).not.toContain('sr-only');
  sin.quitar();
});
