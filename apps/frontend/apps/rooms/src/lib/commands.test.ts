// Checks PUROS del parser de comandos de barra (Ola 14). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  parseCommand,
  matchCommands,
  commandAutocompleteQuery,
  shrugBody,
  emoteBody,
  parseEmote,
} from './commands.js';

test('parsea comando + argumento', () => {
  const p = parseCommand('/status 🌴 de vacaciones');
  expect(p?.kind).toBe('command');
  if (p?.kind === 'command') {
    expect(p.name).toBe('status');
    expect(p.arg).toBe('🌴 de vacaciones');
  }
});

test('comando sin argumento', () => {
  const p = parseCommand('/away');
  expect(p?.kind).toBe('command');
  if (p?.kind === 'command') {
    expect(p.name).toBe('away');
    expect(p.arg).toBe('');
  }
});

test('los alias resuelven al nombre canónico', () => {
  expect((parseCommand('/dm @ana') as { name: string }).name).toBe('msg');
  expect((parseCommand('/header nuevo tema') as { name: string }).name).toBe('topic');
});

test('comando desconocido no es null (para avisar y NO enviar)', () => {
  const p = parseCommand('/noexiste algo');
  expect(p?.kind).toBe('unknown');
  if (p?.kind === 'unknown') expect(p.name).toBe('noexiste');
});

test('texto normal → null (se envía tal cual)', () => {
  expect(parseCommand('hola /nada')).toBeNull();
  expect(parseCommand('mira /etc no es comando aquí')).toBeNull();
  // Escapes: «//» y «/ » no son intentos de comando.
  expect(parseCommand('//ruta/absoluta')).toBeNull();
  expect(parseCommand('/ con espacio')).toBeNull();
});

test('autocompletado: query mientras se teclea el nombre; null tras el espacio', () => {
  expect(commandAutocompleteQuery('/sh')).toBe('sh');
  expect(commandAutocompleteQuery('/')).toBe('');
  expect(commandAutocompleteQuery('/shrug ')).toBeNull(); // ya hay argumento
  expect(commandAutocompleteQuery('hola')).toBeNull();
});

test('matchCommands filtra por nombre o alias', () => {
  expect(matchCommands('').length).toBeGreaterThan(5); // catálogo completo
  expect(matchCommands('me').some((c) => c.name === 'me')).toBe(true);
  expect(matchCommands('dm').some((c) => c.name === 'msg')).toBe(true); // por alias
  expect(matchCommands('zz')).toEqual([]);
});

test('shrugBody añade el shrug (con y sin texto)', () => {
  expect(shrugBody('vale')).toBe('vale ¯\\_(ツ)_/¯');
  expect(shrugBody('')).toBe('¯\\_(ツ)_/¯');
});

test('emote: emoteBody y parseEmote son inversos', () => {
  const body = emoteBody('saluda');
  expect(body).toBe('/me saluda');
  expect(parseEmote(body)).toBe('saluda');
  expect(parseEmote('un mensaje normal')).toBeNull();
});
