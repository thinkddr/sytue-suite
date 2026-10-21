// Checks de cuándo se cierra un popover anclado. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { cierraPorResize, cierraPorScroll, revelaPorToque } from './popover.js';

test('el dedo y el lápiz despliegan la barra de acciones; el ratón y el teclado no', () => {
  // ES EL BUG: `group-hover` vive dentro de `@media (hover: hover)`, falsa en táctil.
  // Sin este camino, en un móvil no se podía reaccionar, responder en hilo, editar,
  // borrar, fijar, guardar, copiar el enlace ni reenviar un mensaje: la barra era
  // permanentemente `opacity-0 pointer-events-none`.
  expect(revelaPorToque('touch')).toBe(true);
  expect(revelaPorToque('pen')).toBe(true);
  // Con ratón sigue mandando el hover: fijarla sería un estado de más.
  expect(revelaPorToque('mouse')).toBe(false);
  // Clic sin puntero (Enter sobre un chip de reacción): lo cubre `focus-within`.
  expect(revelaPorToque('')).toBe(false);
});

test('el teclado virtual NO cierra el popover', () => {
  // ES EL BUG: al abrir el selector se enfocaba el buscador, el teclado del móvil
  // subía, el viewport se encogía de ALTO y el `resize` cerraba el popover a los
  // milisegundos. Se abría y se cerraba solo sin que nadie tocara nada.
  expect(cierraPorResize(390, 390)).toBe(false);
  // Rotar el móvil o redimensionar la ventana sí: ahí el ancla se ha movido de
  // sitio y el popover se quedaría flotando donde ya no hay nada.
  expect(cierraPorResize(390, 844)).toBe(true);
});

test('desplazar la lista de dentro NO cierra; desplazar el fondo sí', () => {
  // El listener va en captura para enterarse del scroller virtualizado del
  // histórico, y por eso recibía también el de la rejilla de emoji: bajar por el
  // catálogo con el dedo cerraba el selector.
  const popover = { esDentro: true } as unknown as Node;
  const fondo = { esDentro: false } as unknown as Node;
  const dentro = (n: Node | null) => Boolean(n && (n as { esDentro?: boolean }).esDentro);
  expect(cierraPorScroll(popover, dentro)).toBe(false);
  expect(cierraPorScroll(fondo, dentro)).toBe(true);
  // Un scroll sin destino identificable se trata como de fuera: es lo prudente,
  // porque la posición fija ya no se puede dar por buena.
  expect(cierraPorScroll(null, dentro)).toBe(true);
});
