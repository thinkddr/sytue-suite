/**
 * Tests de cards — el voto optimista de una encuesta / tarea.
 *
 * Lo que puede salir mal aquí no da error: contar de más al votar dos veces, o dejar
 * dos opciones marcadas en una encuesta de respuesta ÚNICA, se ve como un resultado
 * plausible pero falso hasta que llega el recuento del servidor y «salta» solo.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { toggleMyCardVote, applyCardVotes } from './cards.js';
import type { ChatMessage, MessageCard } from '../client/chatApi.js';

const encuesta = (votes: MessageCard['votes']): MessageCard => ({
  kind: 'poll',
  options: ['sí', 'no', 'quizá'],
  ...(votes ? { votes } : {}),
});

test('votar por primera vez crea el valor con mi voto', () => {
  const votes = toggleMyCardVote(encuesta(undefined), '1', true, 'yo');
  expect(votes).toEqual([{ value: '1', count: 1, mine: true, subs: ['yo'] }]);
});

test('votar sobre una opción que ya tiene votos ajenos suma sin pisarlos', () => {
  const votes = toggleMyCardVote(
    encuesta([{ value: '0', count: 2, mine: false, subs: ['ana', 'luis'] }]),
    '0',
    true,
    'yo',
  );
  expect(votes[0]).toEqual({ value: '0', count: 3, mine: true, subs: ['ana', 'luis', 'yo'] });
});

test('votar dos veces la misma opción no cuenta dos veces', () => {
  const una = toggleMyCardVote(encuesta(undefined), '0', true, 'yo');
  const dos = toggleMyCardVote(encuesta(una), '0', true, 'yo');
  expect(dos[0]?.count).toBe(1);
});

test('en respuesta ÚNICA, votar otra opción retira la anterior', () => {
  const antes = encuesta([
    { value: '0', count: 1, mine: true, subs: ['yo'] },
    { value: '1', count: 1, mine: false, subs: ['ana'] },
  ]);
  const votes = toggleMyCardVote(antes, '1', true, 'yo');
  expect(votes.find((v) => v.value === '0')).toBeUndefined(); // sin votos: desaparece
  expect(votes.find((v) => v.value === '1')).toEqual({
    value: '1',
    count: 2,
    mine: true,
    subs: ['ana', 'yo'],
  });
});

test('con varias respuestas, votar otra opción CONSERVA la anterior', () => {
  const antes: MessageCard = {
    ...encuesta([{ value: '0', count: 1, mine: true, subs: ['yo'] }]),
    multi: true,
  };
  const votes = toggleMyCardVote(antes, '2', true, 'yo');
  expect(votes.map((v) => v.value)).toEqual(['0', '2']);
  expect(votes.every((v) => v.mine)).toBe(true);
});

test('desmarcar retira solo mi voto y deja los ajenos', () => {
  const votes = toggleMyCardVote(
    encuesta([{ value: '0', count: 2, mine: true, subs: ['ana', 'yo'] }]),
    '0',
    false,
    'yo',
  );
  expect(votes[0]).toEqual({ value: '0', count: 1, mine: false, subs: ['ana'] });
});

test('desmarcar lo que no había votado no resta', () => {
  const votes = toggleMyCardVote(
    encuesta([{ value: '0', count: 1, mine: false, subs: ['ana'] }]),
    '0',
    false,
    'yo',
  );
  expect(votes[0]?.count).toBe(1);
});

test('la tarea se marca y se desmarca con el mismo valor', () => {
  const tarea: MessageCard = { kind: 'task' };
  const hecha = toggleMyCardVote(tarea, 'done', true, 'yo');
  expect(hecha).toEqual([{ value: 'done', count: 1, mine: true, subs: ['yo'] }]);
  expect(toggleMyCardVote({ ...tarea, votes: hecha }, 'done', false, 'yo')).toEqual([]);
});

test('el recuento del servidor SUSTITUYE al optimista, y solo en su mensaje', () => {
  const m: ChatMessage = {
    id: 'm1',
    channelId: 'c1',
    author: { id: 'yo', name: 'Yo' },
    body: '¿Vamos?',
    createdAt: '2026-08-14T10:00:00Z',
    card: encuesta([{ value: '0', count: 9, mine: true, subs: ['yo'] }]),
  };
  const nuevo = applyCardVotes(m, 'm1', [{ value: '1', count: 1, mine: false, subs: ['ana'] }]);
  expect(nuevo.card?.votes).toEqual([{ value: '1', count: 1, mine: false, subs: ['ana'] }]);
  expect(applyCardVotes(m, 'otro', [])).toBe(m); // otro mensaje: intacto (misma referencia)
  const sinTarjeta: ChatMessage = { ...m };
  delete sinTarjeta.card;
  expect(applyCardVotes(sinTarjeta, 'm1', []).card).toBeUndefined();
});
