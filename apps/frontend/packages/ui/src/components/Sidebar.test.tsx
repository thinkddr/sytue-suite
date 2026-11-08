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

test('el id acota el panel desplazable sin incluir cabecera ni pie', () => {
  const host = mount(
    <Sidebar
      id="sidebar-gesto"
      header={<button>Crear canal</button>}
      footer={<button>Ayuda</button>}
    >
      <SidebarItem label="Canal del proyecto" />
    </Sidebar>,
  );
  try {
    // Rooms obtiene así el destino del autoscroll, dentro de su propia navegación.
    const nav = document.getElementById('sidebar-gesto');
    const scroll = nav?.querySelector<HTMLElement>('[data-sidebar-scroll]');
    expect(nav?.tagName).toBe('NAV');
    expect(scroll?.classList.contains('overflow-y-auto')).toBe(true);
    expect(scroll?.querySelector('li')?.textContent).toBe('Canal del proyecto');
    expect(scroll?.querySelector('button')?.textContent).toBe('Canal del proyecto');
    expect(scroll?.textContent).not.toContain('Crear canal');
    expect(scroll?.textContent).not.toContain('Ayuda');
  } finally {
    render(null, host);
    host.remove();
  }
});

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

test('el pie queda alcanzable cuando no cabe: tope y desplazamiento propio', () => {
  // «Von Microsoft 365 holen» —el pie de Calendar y Contacts— quedó a −30px POR
  // ABAJO (inalcanzable) en un Galaxy S24 landscape (780×360), barrido móvil del
  // 2026-11-03: el pie vive FUERA de la zona que desplaza del carril y el carril
  // entero recorta, así que un pie más alto que la ventana no tenía manera de
  // verse entero. El tope al 60 % del carril hace que encoja y desplace por
  // dentro; sin las dos clases de abajo, vuelve a quedar cortado.
  const host = mount(
    <Sidebar footer={<button>Von Microsoft 365 holen</button>}>
      <SidebarItem label="Hoy" />
    </Sidebar>,
  );
  const pie = host.querySelector('nav > div.border-t')!;
  expect(pie.className.split(' ')).toContain('overflow-y-auto');
  expect(pie.className.split(' ')).toContain('max-h-[60%]');
});

// La rama de Apple Mail («Todos los buzones de entrada» con la Entrada de cada cuenta,
// una cuenta con sus carpetas). VISTO EN ROJO (2026-11-06): sin `aria-expanded` ni
// `data-mantiene-nav` en el disparador, y con los hijos pintados estando cerrada.
test('una rama dice si está abierta, no cierra el cajón y solo enseña sus hijos abierta', () => {
  const rama = (expanded: boolean) =>
    mount(
      <ul>
        <SidebarItem label="ana@gmail.com" expanded={expanded} onClick={() => {}}>
          <SidebarItem label="Recibidos" />
        </SidebarItem>
      </ul>,
    );
  const abierta = rama(true);
  const cerrada = rama(false);
  try {
    const boton = abierta.querySelector('button');
    expect(boton?.getAttribute('aria-expanded')).toBe('true');
    expect(boton?.hasAttribute('data-mantiene-nav')).toBe(true);
    // Los hijos van en su propio grupo, fuera del disparador, y sí cierran el cajón.
    const grupo = abierta.querySelector('ul[role="group"]');
    expect(grupo?.textContent).toBe('Recibidos');
    expect(boton?.contains(grupo ?? null)).toBe(false);
    expect(grupo?.querySelector('[data-mantiene-nav]')).toBeNull();
    expect(cerrada.querySelector('button')?.getAttribute('aria-expanded')).toBe('false');
    expect(cerrada.querySelector('ul[role="group"]')).toBeNull();
    expect(cerrada.textContent).not.toContain('Recibidos');
  } finally {
    for (const host of [abierta, cerrada]) {
      render(null, host);
      host.remove();
    }
  }
});

// «Todos los buzones de entrada» se abre al pulsar la fila y se despliega con su
// triángulo, como en Apple Mail. VISTO EN ROJO (2026-11-06): con el chevron dentro de
// la fila, pulsarla solo plegaba y no había forma de abrir la Entrada mezclada; con
// `data-mantiene-nav` en la fila, abrirla dejaba el cajón del móvil tapando la lista.
test('con `onToggle` la fila es un destino y el triángulo aparte pliega', () => {
  const pulsado: string[] = [];
  const rama = (expanded: boolean) =>
    mount(
      <ul>
        <SidebarItem
          label="Todos los buzones de entrada"
          active
          expanded={expanded}
          onClick={() => pulsado.push('fila')}
          onToggle={() => pulsado.push('triangulo')}
          toggleLabel="Cuentas"
        >
          <SidebarItem label="ana@gmail.com" />
        </SidebarItem>
      </ul>,
    );
  const abierta = rama(true);
  const cerrada = rama(false);
  try {
    const fila = abierta.querySelector<HTMLButtonElement>('li > button');
    const triangulo = abierta.querySelector<HTMLButtonElement>('li > span > button');
    expect(fila?.getAttribute('aria-current')).toBe('page');
    expect(fila?.hasAttribute('aria-expanded')).toBe(false);
    expect(fila?.hasAttribute('data-mantiene-nav')).toBe(false);
    expect(fila?.querySelector('svg')).toBeNull();
    expect(triangulo?.getAttribute('aria-expanded')).toBe('true');
    expect(triangulo?.getAttribute('aria-label')).toBe('Cuentas');
    expect(triangulo?.hasAttribute('data-mantiene-nav')).toBe(true);
    expect(fila?.contains(triangulo ?? null)).toBe(false);
    expect(abierta.querySelector('ul[role="group"]')?.textContent).toBe('ana@gmail.com');
    fila?.click();
    triangulo?.click();
    expect(pulsado).toEqual(['fila', 'triangulo']);
    expect(cerrada.querySelector('[aria-label="Cuentas"]')?.getAttribute('aria-expanded')).toBe(
      'false',
    );
    expect(cerrada.querySelector('ul[role="group"]')).toBeNull();
  } finally {
    for (const host of [abierta, cerrada]) {
      render(null, host);
      host.remove();
    }
  }
});

// VISTO EN ROJO (2026-11-06): con `children` como condición, el `[]` de una cuenta sin
// carpetas pintaba un grupo vacío con su margen.
test('una rama abierta sin nada que colgar no pinta un grupo vacío', () => {
  const host = mount(
    <ul>
      <SidebarItem label="ana@gmail.com" expanded onClick={() => {}}>
        {[].map(() => null)}
        {null}
      </SidebarItem>
    </ul>,
  );
  try {
    expect(host.querySelector('button')?.getAttribute('aria-expanded')).toBe('true');
    expect(host.querySelector('ul[role="group"]')).toBeNull();
  } finally {
    render(null, host);
    host.remove();
  }
});

// La fila que repite otra ya marcada se ve activa sin ser otra «página actual».
// VISTO EN ROJO (2026-11-06) con `aria-current` atado solo a `active`.
test('`ariaCurrent={false}` marca el item sin anunciarlo como página actual', () => {
  const host = mount(
    <ul>
      <SidebarItem label="Recibidos" active />
      <SidebarItem label="ana@sytue.eu" active ariaCurrent={false} />
    </ul>,
  );
  try {
    const [carpeta, fuente] = [...host.querySelectorAll('button')];
    expect(carpeta?.getAttribute('aria-current')).toBe('page');
    expect(fuente?.hasAttribute('aria-current')).toBe(false);
    expect(fuente?.className).toContain('sytue-nav-current');
  } finally {
    render(null, host);
    host.remove();
  }
});

test('un item que no es rama no anuncia nada desplegable', () => {
  const host = mount(<SidebarItem label="Recibidos" onClick={() => {}} />);
  try {
    const boton = host.querySelector('button');
    expect(boton?.hasAttribute('aria-expanded')).toBe(false);
    expect(boton?.hasAttribute('data-mantiene-nav')).toBe(false);
  } finally {
    render(null, host);
    host.remove();
  }
});
