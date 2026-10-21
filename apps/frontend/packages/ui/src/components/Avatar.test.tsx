/**
 * Avatar — su contrato: con foto la pinta, sin foto pone las iniciales, y si la
 * foto NO carga vuelve a las iniciales en vez de dejar un hueco.
 *
 * Lo último no es teórico: un `<img>` roto conserva su caja, así que en la topbar
 * de la suite se veía un cuadro gris vacío donde va la persona. Ocurre en cuanto
 * la URL del proveedor de identidad caduca o la bloquea el navegador, y ni el
 * usuario puede entenderlo ni arreglarlo. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { Avatar } from './Avatar.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

// happy-dom no carga imágenes y da TODAS por terminadas y sin píxeles, que es
// exactamente cómo se ve una rota. Aquí una imagen está «cargando» salvo que el
// test diga que ya falló, que es lo que pasa en un navegador de verdad.
let yaFallo = false;
Object.defineProperty(HTMLImageElement.prototype, 'complete', {
  configurable: true,
  get: () => yaFallo,
});

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

/** Preact re-renderiza en el siguiente tick, no dentro del propio evento. */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 20));

test('sin foto pone las iniciales del nombre', () => {
  expect(mount(<Avatar name="Daniel Delgado" size="sm" />).textContent).toBe('DD');
});

test('una foto que no carga cae a las iniciales, no deja un cuadro vacío', async () => {
  const host = mount(<Avatar name="Daniel Delgado" src="https://x.invalido/f.jpg" size="sm" />);
  expect(host.querySelector('img')).not.toBeNull();

  host.querySelector('img')!.dispatchEvent(new Event('error'));
  await tick();

  expect(host.querySelector('img')).toBeNull();
  expect(host.textContent).toBe('DD');
});

// El caso de la barra de la suite: se pinta en el servidor, el `<img>` falla
// antes de hidratar y su `error` no lo oye nadie. Sin mirarlo al montar se quedaba
// el icono de imagen rota en vez de las iniciales. VISTO EN ROJO quitando el
// `useEffect` de Avatar.tsx: queda el `<img>` y no salen las iniciales.
test('una foto que ya falló antes de montar también cae a las iniciales', async () => {
  yaFallo = true;
  try {
    const host = mount(<Avatar name="Daniel Delgado" src="https://x.invalido/g.jpg" size="sm" />);
    await tick();
    expect(host.querySelector('img')).toBeNull();
    expect(host.textContent).toBe('DD');
  } finally {
    yaFallo = false;
  }
});

test('cambiar de foto vuelve a intentarlo: el fallo es de UNA url, no del avatar', async () => {
  const host = mount(<Avatar name="Ana Soto" src="https://x.invalido/1.jpg" size="sm" />);
  host.querySelector('img')!.dispatchEvent(new Event('error'));
  await tick();
  expect(host.textContent).toBe('AS');

  render(<Avatar name="Ana Soto" src="https://x.valido/2.jpg" size="sm" />, host);
  await tick();
  expect(host.querySelector('img')?.getAttribute('src')).toBe('https://x.valido/2.jpg');
});

// Una lista de personas dispara UNA petición por fila. Sin `lazy`, abrir una
// libreta con foto en cada contacto encola cientos de GET a la vez, casi todos
// por debajo del pliegue; y sin `async` la decodificación bloquea el hilo
// principal fila a fila. Es un atributo de una línea que un refactor tira sin
// enterarse, y su ausencia no rompe nada visible: solo la red.
test('la foto se carga en diferido y se decodifica fuera del hilo principal', () => {
  const img = mount(
    <Avatar name="Ana Soto" src="https://x.valido/a.jpg" size="sm" />,
  ).querySelector('img');
  expect(img?.getAttribute('loading')).toBe('lazy');
  expect(img?.getAttribute('decoding')).toBe('async');
});

// La presencia no va solo en el color (WCAG 1.4.1): cada estado lleva su forma —en
// línea el disco, ausente la luna, no molestar la raya, desconectado el aro— y un
// nombre traducido. VISTO EN ROJO (2026-09-26, saboteado a mano): sin FORMA, los
// cuatro puntos son el mismo disco con otro color.
test('cada presencia lleva su forma y su nombre, no solo un color', () => {
  const formas = (['online', 'away', 'dnd', 'offline'] as const).map((p) => {
    const punto = mount(<Avatar name="Ada" size="sm" presence={p} />).querySelector(
      '[data-presencia]',
    ) as HTMLElement;
    expect(punto.getAttribute('aria-label')).toBeTruthy();
    const forma = punto.querySelector('span') as HTMLElement | null;
    return forma ? forma.getAttribute('style') : 'lleno';
  });
  expect(new Set(formas).size).toBe(4);
  expect(formas[0]).toBe('lleno');
});
