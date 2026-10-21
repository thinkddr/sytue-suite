/**
 * Tests de hookVariants — deriva las tres direcciones de un webhook entrante.
 *
 * Lo que puede salir mal es sustituir en el sitio equivocado: si la organización sirve
 * Rooms en un host que contiene «hooks», tocar la PRIMERA aparición produciría una URL
 * que no existe y el administrador la pegaría en su herramienta sin saberlo.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { hookVariants, channelEmail } from './hookVariants.js';

test('deriva las cuatro direcciones del mismo token', () => {
  const v = hookVariants('https://rooms.sytue.eu/hooks/abc123');
  expect(v.map((x) => x.url)).toEqual([
    'https://rooms.sytue.eu/hooks/abc123',
    'https://rooms.sytue.eu/hooks/slack/abc123',
    'https://rooms.sytue.eu/hooks/teams/abc123',
    'https://rooms.sytue.eu/hooks/googlechat/abc123',
  ]);
});

test('un host que contiene «hooks» no despista: se sustituye la ÚLTIMA aparición', () => {
  const v = hookVariants('https://hooks.example.eu/hooks/tok');
  expect(v[1]?.url).toBe('https://hooks.example.eu/hooks/slack/tok');
  expect(v[2]?.url).toBe('https://hooks.example.eu/hooks/teams/tok');
});

test('una URL con otra forma devuelve SOLO la original, no dos inventadas', () => {
  // Mejor una dirección que funciona que tres de las que dos no existen.
  expect(hookVariants('https://rooms.sytue.eu/otra/ruta')).toHaveLength(1);
  expect(hookVariants('https://rooms.sytue.eu/hooks/')).toHaveLength(1);
});

test('la dirección de correo es el MISMO token, no otro secreto', () => {
  expect(channelEmail('https://rooms.sytue.eu/hooks/abc123', 'rooms.sytue.eu')).toBe(
    'abc123@rooms.sytue.eu',
  );
  // Se toleran los adornos que suele llevar una variable de entorno mal copiada.
  expect(channelEmail('https://rooms.sytue.eu/hooks/abc123', '@rooms.sytue.eu.')).toBe(
    'abc123@rooms.sytue.eu',
  );
});

test('la dirección lleva delante el nombre del canal, para poder leerla y dictarla', () => {
  // Un token de 43 caracteres a secas no se puede dictar por teléfono ni reconocer en
  // una lista de destinatarios. Lo que autoriza sigue siendo el token, detrás del «+».
  expect(channelEmail('https://rooms.sytue.eu/hooks/abc123', 'rooms.sytue.eu', 'General')).toBe(
    'general+abc123@rooms.sytue.eu',
  );
  // El nombre acaba en una dirección de correo: acentos, espacios y símbolos fuera.
  expect(
    channelEmail('https://rooms.sytue.eu/hooks/abc123', 'rooms.sytue.eu', 'Avisos de Producción'),
  ).toBe('avisos-de-produccion+abc123@rooms.sytue.eu');
  // Sin nombre utilizable no se inventa un prefijo vacío que dejaría un «+» suelto.
  expect(channelEmail('https://rooms.sytue.eu/hooks/abc123', 'rooms.sytue.eu', '###')).toBe(
    'abc123@rooms.sytue.eu',
  );
});

test('sin dominio configurado no se inventa una dirección', () => {
  // El correo a canal se apaga por despliegue: enseñar una dirección que rebota sería
  // peor que no enseñar ninguna.
  expect(channelEmail('https://rooms.sytue.eu/hooks/abc123', '')).toBe('');
  expect(channelEmail('https://rooms.sytue.eu/otra/ruta', 'rooms.sytue.eu')).toBe('');
  expect(hookVariants('https://rooms.sytue.eu/hooks/abc123')).toHaveLength(4);
  expect(hookVariants('https://rooms.sytue.eu/hooks/abc123', 'rooms.sytue.eu')).toHaveLength(5);
});
