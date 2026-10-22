/**
 * Tests de SidebarSection — lo nuevo: las ACCIONES del encabezado se pintan fuera
 * de la lista (no son un item más) y una sección plegable arranca como dice
 * `defaultOpen` y cambia al pulsar el rótulo. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import {
  SIDEBAR_ACTION_CLASS,
  Sidebar,
  SidebarAction,
  SidebarItem,
  SidebarSection,
} from './Sidebar.js';
import { Button } from './Button.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

test('las acciones del encabezado quedan fuera de la lista de items', () => {
  const host = mount(
    <SidebarSection
      title="Carpetas"
      actions={<SidebarAction label="Gestionar carpetas" icon={<i />} onClick={() => {}} />}
    >
      <SidebarItem label="Recibidos" />
    </SidebarSection>,
  );
  const accion = host.querySelector('button[aria-label="Gestionar carpetas"]');
  expect(accion).not.toBeNull();
  // No cuelga del <ul>: para un lector de pantalla la lista sigue teniendo UN item.
  expect(accion!.closest('ul')).toBeNull();
  expect(host.querySelectorAll('ul > li').length).toBe(1);
});

test('una sección plegable arranca cerrada si se le pide y se abre al pulsar el rótulo', async () => {
  const host = mount(
    <SidebarSection title="Ajustes" collapsible defaultOpen={false}>
      <SidebarItem label="Firma" />
    </SidebarSection>,
  );
  const rotulo = host.querySelector<HTMLButtonElement>('button[aria-expanded]')!;
  expect(rotulo.getAttribute('aria-expanded')).toBe('false');
  expect(host.querySelector('ul')).toBeNull();
  rotulo.click();
  // Preact vuelve a pintar en una microtarea; se le deja el turno.
  await new Promise((r) => setTimeout(r, 0));
  expect(rotulo.getAttribute('aria-expanded')).toBe('true');
  expect(host.querySelectorAll('ul > li').length).toBe(1);
});

test('el rótulo plegable NO cierra el cajón de navegación en móvil', () => {
  // `AppShell` cierra el cajón al pulsar cualquier `button` de dentro, salvo los
  // que llevan `data-mantiene-nav`. Este solo pliega y despliega: sin la marca,
  // desplegar un grupo hacía desaparecer la barra entera y había que volver a
  // abrirla para llegar a lo que se acababa de desplegar.
  //
  // Los `SidebarItem` NO deben llevarla: elegir algo sí cierra, y esa es la
  // diferencia que el test fija.
  const host = mount(
    <SidebarSection title="Ajustes" collapsible>
      <SidebarItem label="Firma" />
    </SidebarSection>,
  );
  const rotulo = host.querySelector<HTMLButtonElement>('button[aria-expanded]')!;
  expect(rotulo.hasAttribute('data-mantiene-nav')).toBe(true);
  expect(host.querySelector('a,li button')?.hasAttribute('data-mantiene-nav') ?? false).toBe(false);
});

test('sin `collapsible`, el rótulo no es un botón y la lista siempre está', () => {
  const host = mount(
    <SidebarSection title="Etiquetas">
      <SidebarItem label="Urgente" />
    </SidebarSection>,
  );
  expect(host.querySelector('button[aria-expanded]')).toBeNull();
  expect(host.querySelectorAll('ul > li').length).toBe(1);
});

/*
 * La ACCIÓN DE UN ITEM —«actualizar» en la carpeta que se está viendo— va
 * HERMANA del enlace, nunca dentro.
 *
 * Un <button> dentro de otro <button> es marcado inválido y el navegador lo
 * desanida por su cuenta: el segundo acaba fuera del sitio donde se escribió, y
 * el fallo no se ve en el código ni en los tipos, solo en el DOM del navegador.
 */
test('la acción del item no queda anidada dentro del disparador', () => {
  const host = mount(
    <SidebarItem
      label="Recibidos"
      active
      onClick={() => {}}
      action={<SidebarAction label="Actualizar" icon={<span>↻</span>} onClick={() => {}} />}
    />,
  );
  const item = host.querySelector('li');
  const disparador = item?.querySelector('button[aria-current]');
  expect(disparador).not.toBeNull();
  expect(disparador?.querySelector('button')).toBeNull();
  const accion = item?.querySelector('button[aria-label="Actualizar"]');
  expect(accion).not.toBeNull();
  expect(accion?.closest('button')).toBe(accion as HTMLElement);
});

/*
 * Y cae en la MISMA columna que la acción del encabezado de sección.
 *
 * `right-1` aquí y `pr-1` allí, con los dos botones de 24px: es lo que hace que
 * la rueda de «gestionar carpetas» y el «actualizar» de la carpeta activa se
 * lean como una rejilla y no como iconos sueltos. Si alguien cambia uno de los
 * dos valores, la alineación se rompe en un píxel que nadie mide pero todos ven.
 */
test('la acción del item se alinea con la del encabezado', () => {
  const host = mount(
    <SidebarItem
      label="Recibidos"
      onClick={() => {}}
      action={<SidebarAction label="Actualizar" icon={<span>↻</span>} onClick={() => {}} />}
    />,
  );
  const envoltorio = host.querySelector('li > span');
  expect(envoltorio?.className).toContain('right-1');
  expect(envoltorio?.className).toContain('absolute');
  expect(host.querySelector('li')?.className).toContain('relative');
  // Y el disparador deja hueco: sin `pr-9`, un rótulo largo pasa por debajo.
  expect(host.querySelector('li > button')?.className).toContain('pr-9');
});

/* Sin acción, nada cambia: ni `relative`, ni hueco, ni envoltorio de más. */
test('un item sin acción se queda exactamente como estaba', () => {
  const host = mount(<SidebarItem label="Borradores" onClick={() => {}} />);
  expect(host.querySelector('li')?.className).not.toContain('relative');
  expect(host.querySelector('li > span')).toBeNull();
  expect(host.querySelector('li > button')?.className).not.toContain('pr-9');
});

test('el menú no fija el primario a un escalón: el botón de la cabecera sigue al tema', () => {
  // El menú reasignaba `--color-primary` al 600 en línea. En oscuro el primario es el
  // primario CLARO con rótulo oscuro, y ese 600 fijo dejaba «Redactar» en 4,1:1. Que el
  // botón se distinga del menú lo mide ahora `contraste-paleta.test.ts` sobre la hoja.
  const host = mount(
    <Sidebar header={<button class="bg-primary">Redactar</button>}>
      <SidebarItem label="Recibidos" />
    </Sidebar>,
  );
  const nav = host.querySelector('nav')!;
  for (const p of ['--color-primary', '--sytue-primary-hover', '--sytue-primary-active']) {
    expect(nav.style.getPropertyValue(p)).toBe('');
  }
  expect(nav.contains(host.querySelector('button.bg-primary'))).toBe(true);
});

test('el botón de la cabecera toma el aro blanco del cromo, y solo él', () => {
  // El azul de foco se quedaba en 3,4:1 sobre el carril claro («Crear evento», medido).
  // La variable se cambia en el HIJO DIRECTO con aro de botón, no en el contenedor: se
  // heredaría hasta el menú de presencia de Rooms, que se abre dentro y es claro.
  const regla = '[&>.sytue-focus-ring-inset]:[--sytue-focus-ring:var(--sytue-chrome-focus)]';
  const host = mount(
    <Sidebar header={<Button>Crear evento</Button>}>
      <SidebarItem label="Recibidos" />
    </Sidebar>,
  );
  const boton = [...host.querySelectorAll('button')].find((b) => b.textContent === 'Crear evento')!;
  expect(boton.classList.contains('sytue-focus-ring-inset')).toBe(true);
  expect(boton.parentElement!.classList.contains(regla)).toBe(true);
});

test('la acción del carril mide 24 px de verdad, con la raíz a 14', () => {
  // `h-6` son 1.5rem = 21 px en esta suite: es lo que medía en producción. El suelo
  // de WCAG 2.5.8 va en px para que ninguna raíz lo baje.
  expect(SIDEBAR_ACTION_CLASS).toContain('min-h-[24px]');
  expect(SIDEBAR_ACTION_CLASS).toContain('min-w-[24px]');
});

test('un pie que no pinta nada no deja su filete', () => {
  // `<ImportData>` sin ningún origen conectado devuelve null, y `footer` es un
  // elemento igual. VISTO EN ROJO sin `empty:hidden`.
  const Nada = (): null => null;
  const host = mount(
    <Sidebar footer={<Nada />}>
      <SidebarItem label="Hoy" />
    </Sidebar>,
  );
  const pie = host.querySelector('nav > div.border-t');
  expect(pie).not.toBeNull();
  expect(pie!.childNodes.length).toBe(0);
  expect(pie!.className.split(' ')).toContain('empty:hidden');
});
