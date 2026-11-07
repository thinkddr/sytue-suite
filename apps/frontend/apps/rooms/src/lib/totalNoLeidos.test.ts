// Checks del total de no leídos de la pestaña. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { Channel } from './chat.js';
import { sumaNoVisto, tituloConNoLeidos, totalNoLeidos } from './totalNoLeidos.js';

const canal = (id: string, extra: Partial<Channel> = {}): Channel => ({
  id,
  name: id,
  kind: 'public',
  ...extra,
});

test('suma los no leídos de todos los canales menos los silenciados', () => {
  const cs = [
    canal('a', { unread: 3 }),
    canal('dm', { kind: 'direct', unread: 2 }),
    canal('ruido', { unread: 40, muted: true, notifyLevel: 'none' }),
    canal('menciones', { unread: 1, notifyLevel: 'mentions' }),
    canal('leido'),
  ];
  expect(totalNoLeidos(cs)).toBe(6);
  expect(totalNoLeidos([])).toBe(0);
});

test('el título lleva el total delante, con tope, y nada si no hay', () => {
  expect(tituloConNoLeidos('Sytue Rooms', 0)).toBe('Sytue Rooms');
  expect(tituloConNoLeidos('Sytue Rooms', 7)).toBe('(7) Sytue Rooms');
  expect(tituloConNoLeidos('Sytue Rooms', 120)).toBe('(99+) Sytue Rooms');
});

test('con la pestaña oculta, un mensaje ajeno del canal abierto suma al total', () => {
  const abierto = canal('general');
  expect(sumaNoVisto(true, { mine: false }, abierto)).toBe(true);
  // A la vista se está leyendo; lo propio y las respuestas en hilo no cuentan.
  expect(sumaNoVisto(false, { mine: false }, abierto)).toBe(false);
  expect(sumaNoVisto(true, { mine: true }, abierto)).toBe(false);
  expect(sumaNoVisto(true, { mine: false, parentId: 'raiz' }, abierto)).toBe(false);
  // Silenciado o con avisos a «nada»: no llama la atención, como en el total. Cada
  // condición va sola, para que quitar una de las dos se vea.
  expect(sumaNoVisto(true, {}, canal('ruido', { muted: true }))).toBe(false);
  expect(sumaNoVisto(true, {}, canal('ruido', { notifyLevel: 'none' }))).toBe(false);
  // Canal que aún no está en la lista: no se sabe si está silenciado (como en Fyne).
  expect(sumaNoVisto(true, { mine: false }, undefined)).toBe(false);
});
