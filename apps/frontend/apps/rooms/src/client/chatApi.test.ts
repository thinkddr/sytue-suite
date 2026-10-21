// Checks del enlace canónico a un mensaje y del traductor de frames del
// WebSocket. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { appDomains } from '@sytue/config';
import { messageLink, toChatEvent } from './chatApi.js';

// Con el prefijo de la app: `rooms.sytue.eu/channels/…` es el 404 de Traefik.
test('messageLink usa el formato del deep-link del Hub, con el prefijo de Rooms', () => {
  expect(messageLink('c1', 'm1')).toBe(`${appDomains.rooms}/rooms/channels/c1?message=m1`);
});

test('messageLink codifica canal y mensaje (sin romper la URL)', () => {
  expect(messageLink('a/b', 'x y', 'https://x')).toBe('https://x/channels/a%2Fb?message=x%20y');
});

// --- Frames de llamada por WebSocket -------------------------------------------
//
// Antes de esto, el otro extremo se enteraba de que le estaban llamando SONDEANDO
// cada ~25 segundos. Ahora lo anuncia el servicio; lo que se comprueba aquí es la
// traducción del frame, que es de lo que depende que el aviso salga o no.

test('call_started y call_ended se traducen al evento de llamada', () => {
  const empezada = toChatEvent({ type: 'call_started', channel_id: 'c-1' }, 'yo');
  expect(empezada).toEqual({ type: 'call', channelId: 'c-1', started: true });

  const terminada = toChatEvent({ type: 'call_ended', channel_id: 'c-1' }, 'yo');
  expect(terminada).toEqual({ type: 'call', channelId: 'c-1', started: false });
});

test('un frame de llamada sin canal se descarta', () => {
  // Sin canal no hay banner al que aplicarlo; dejarlo pasar con la cadena vacía
  // encendería el aviso en el canal que estuviera abierto, que es peor que nada.
  expect(toChatEvent({ type: 'call_started' }, 'yo')).toBeNull();
  expect(toChatEvent({ type: 'call_ended', channel_id: '' }, 'yo')).toBeNull();
});

// --- Borrado de un canal por WebSocket -------------------------------------------
//
// Sin traducirlo, el canal borrado por un admin seguía en la barra de todos hasta
// recargar, y abrirlo daba 404.

test('channel_deleted se traduce al evento de canal borrado', () => {
  const borrado = toChatEvent({ type: 'channel_deleted', channel_id: 'c-1' }, 'yo');
  expect(borrado).toEqual({ type: 'channel.deleted', channelId: 'c-1' });
  expect(toChatEvent({ type: 'channel_deleted' }, 'yo')).toBeNull();
});
