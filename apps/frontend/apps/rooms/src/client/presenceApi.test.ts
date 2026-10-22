// Checks de los helpers puros de presencia/typing. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { MAX_PRESENCE, queryPresence, toPresenceStatus, typingLabel } from './presenceApi.js';
import { roomsT } from '../i18n/index.js';

test('toPresenceStatus mapea busy -> dnd (Avatar de @sytue/ui) y el resto igual', () => {
  expect(toPresenceStatus('busy')).toBe('dnd');
  expect(toPresenceStatus('online')).toBe('online');
  expect(toPresenceStatus('away')).toBe('away');
  expect(toPresenceStatus('offline')).toBe('offline');
});

test('typingLabel compone la frase según el número de escribientes', () => {
  const t = roomsT('en');
  expect(typingLabel(t, [])).toBe('');
  expect(typingLabel(t, ['Ana'])).toBe('Ana is typing…');
  expect(typingLabel(t, ['Ana', 'Beto'])).toBe('Ana and Beto are typing…');
  expect(typingLabel(t, ['Ana', 'Beto', 'Caro'])).toBe('Several people are typing…');
  // Cada idioma trae la frase entera, no trozos pegados.
  expect(typingLabel(roomsT('es'), ['Ana'])).toBe('Ana está escribiendo…');
  expect(typingLabel(roomsT('de'), ['Ana', 'Beto'])).toBe('Ana und Beto schreiben…');
});

// Más de MAX_PRESENCE sujetos van en varias consultas (el Hub contesta 400 a partir de
// 200) y se juntan. VISTO EN ROJO (2026-09-26, saboteado a mano): con una sola consulta
// sale una petición de 250.
test('queryPresence parte la consulta en tandas del tope del Hub', async () => {
  const tandas: number[] = [];
  const antes = globalThis.fetch;
  globalThis.fetch = (async (url: string) => {
    const subs = decodeURIComponent(
      new URL(url, 'https://x').searchParams.get('subjects') ?? '',
    ).split(',');
    tandas.push(subs.length);
    if (subs.length > MAX_PRESENCE) return new Response('', { status: 400 });
    return Response.json({ presence: [{ subject: subs[0], state: 'online' }] });
  }) as typeof fetch;
  try {
    const subs = Array.from({ length: 250 }, (_, i) => `s${i}`);
    const out = await queryPresence(subs);
    expect(tandas).toEqual([200, 50]);
    expect(Object.keys(out).sort()).toEqual(['s0', 's200']);
  } finally {
    globalThis.fetch = antes;
  }
});
