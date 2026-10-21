import { expect, test } from 'bun:test';
import { crearHistorialMovil } from './useMobileBack.js';

/** El back del navegador es asíncrono: el doble lo confirma por separado. */
function navegador() {
  const eventos = new EventTarget();
  const entradas: { estado: unknown; url: string }[] = [
    { estado: { canal: 'general' }, url: 'https://sytue.eu/rooms/channels/general' },
  ];
  let indice = 0;
  const pendientes: number[] = [];
  const location = {
    get href() {
      return entradas[indice]!.url;
    },
  };
  const history = {
    get state(): unknown {
      return entradas[indice]!.estado;
    },
    pushState(estado: unknown, _titulo: string, url?: string | URL | null) {
      const destino = url ? new URL(url, location.href).href : location.href;
      entradas.splice(indice + 1, entradas.length, { estado, url: destino });
      indice++;
    },
    replaceState(estado: unknown) {
      entradas[indice] = { estado, url: location.href };
    },
    back() {
      pendientes.push(-1);
    },
    forward() {
      pendientes.push(1);
    },
  };
  const ventana = {
    history,
    location,
    addEventListener: eventos.addEventListener.bind(eventos),
  } as unknown as Window;
  const recorrer = (salto: number) => {
    if (!entradas[indice + salto]) throw new Error('se ha salido de la página');
    indice += salto;
    const evento = new Event('popstate');
    Object.defineProperty(evento, 'state', { value: history.state });
    eventos.dispatchEvent(evento);
  };
  return {
    ventana,
    history,
    volver: () => recorrer(-1),
    get indice() {
      return indice;
    },
    get pendientes() {
      return pendientes.length;
    },
    confirmarBack() {
      const salto = pendientes.shift();
      if (salto === undefined) throw new Error('no hay recorrido solicitado');
      recorrer(salto);
    },
    alRouter(f: () => void) {
      eventos.addEventListener('popstate', f);
    },
  };
}

test('atrás recoge menú, modal, cajón y detalle antes de abandonar la página', async () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  const cerradas: string[] = [];
  // El orden de efectos de Preact no cambia el apilamiento visual.
  for (const [nombre, nivel] of [
    ['modal', 2],
    ['detalle', 0],
    ['cajón', 1],
    ['menú', 2],
  ] as const) {
    const soltar = registrar(() => {
      cerradas.push(nombre);
      soltar();
    }, nivel);
  }
  let router = 0;
  n.alRouter(() => router++);
  for (const nombre of ['menú', 'modal', 'cajón', 'detalle']) {
    n.volver();
    await Promise.resolve();
    expect(cerradas.at(-1)).toBe(nombre);
  }
  expect(router).toBe(0);
  expect(n.indice).toBe(0);
  expect(n.pendientes).toBe(0);
  expect(n.history.state).toEqual({ canal: 'general' });
});

test('un modal abierto desde un menú se recoge antes que el menú que quedó debajo', async () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  const cerradas: string[] = [];
  const menu = registrar(() => {
    cerradas.push('menú');
    menu();
  }, 2);
  const modal = registrar(() => {
    cerradas.push('modal');
    modal();
  }, 2);
  n.volver();
  await Promise.resolve();
  expect(cerradas).toEqual(['modal']);
  n.volver();
  await Promise.resolve();
  expect(cerradas).toEqual(['modal', 'menú']);
  expect(n.indice).toBe(0);
});

test('cerrar manualmente capas interiores no deja pasos y la última retira su entrada', () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  const detalle = registrar(() => {}, 0);
  const modal = registrar(() => {}, 2);
  let router = 0;
  n.alRouter(() => router++);
  detalle();
  expect(n.pendientes).toBe(0);
  modal();
  expect(n.pendientes).toBe(1);
  n.confirmarBack();
  expect(n.indice).toBe(0);
  expect(router).toBe(0);
  expect(n.history.state).toEqual({ canal: 'general' });
});

test('abrir otra capa durante la retirada asíncrona conserva su salida', async () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  registrar(() => {}, 2)();
  let cerrada = false;
  const soltar = registrar(() => {
    cerrada = true;
    soltar();
  }, 2);
  n.confirmarBack();
  expect(n.indice).toBe(1);
  n.volver();
  await Promise.resolve();
  expect(cerrada).toBe(true);
  expect(n.indice).toBe(0);
});

test('un diálogo que pide confirmación conserva atrás hasta que se cierra', async () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  let peticiones = 0;
  const soltar = registrar(() => peticiones++, 2);
  n.volver();
  await Promise.resolve();
  expect(peticiones).toBe(1);
  expect(n.indice).toBe(1);
  soltar();
  n.confirmarBack();
  expect(n.indice).toBe(0);
});

test('navegar a otro canal con el cajón abierto no pisa ni retrocede el router de Rooms', () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  const soltar = registrar(() => {}, 1);
  n.history.pushState({ canal: 'equipo' }, '', '/rooms/channels/equipo');
  soltar();
  expect(n.pendientes).toBe(0);
  expect(n.history.state).toEqual({ canal: 'equipo' });
  let router = 0;
  n.alRouter(() => router++);
  n.volver();
  expect(router).toBe(1);
  expect(n.history.state).toEqual({ canal: 'general' });
  expect(n.ventana.location.href).toEndWith('/rooms/channels/general');
});

test('un push del router durante el back asíncrono del cierre conserva la ruta nueva', () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  registrar(() => {}, 3)();
  // La acción del menú navega mientras aún se está retirando su entrada temporal.
  n.history.pushState({ canal: 'equipo' }, '', '/rooms/channels/equipo');
  let router = 0;
  n.alRouter(() => router++);
  n.confirmarBack();
  expect(n.pendientes).toBe(1);
  n.confirmarBack();
  expect(n.ventana.location.href).toEndWith('/rooms/channels/equipo');
  expect(n.history.state).toEqual({ canal: 'equipo' });
  expect(router).toBe(0);
});

test('copiar history.state en otra ruta no convierte la navegación del router en nuestra entrada', () => {
  const n = navegador();
  const registrar = crearHistorialMovil(n.ventana);
  const soltar = registrar(() => {}, 1);
  n.history.pushState(
    { ...(n.history.state as object), canal: 'equipo' },
    '',
    '/rooms/channels/equipo',
  );
  soltar();
  expect(n.pendientes).toBe(0);
  expect(n.ventana.location.href).toEndWith('/rooms/channels/equipo');
});
