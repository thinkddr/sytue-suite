// Checks de la lógica de menciones (@usuario). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  mentionsUser,
  mentionNotificationBody,
  shouldNotifyDirect,
  shouldNotifyNewMessage,
  matchesKeyword,
  inDndWindow,
  mentionsWholeChannel,
} from './mentions.js';

test('menciona al usuario cuando su sub está en mentions', () => {
  expect(mentionsUser({ mentions: ['u1', 'u2'] }, 'u2')).toBe(true);
});

test('no cuenta el mensaje propio aunque me mencione', () => {
  expect(mentionsUser({ mine: true, mentions: ['u1'] }, 'u1')).toBe(false);
});

test('sin menciones o sin coincidencia → false', () => {
  expect(mentionsUser({ mentions: ['u9'] }, 'u1')).toBe(false);
  expect(mentionsUser({}, 'u1')).toBe(false);
  expect(mentionsUser({ mentions: ['u1'] }, '')).toBe(false);
});

test('@channel/@here/@all (mentionsAll) sube el badge aunque no lleve tu sub', () => {
  // Sin tu sub en mentions, pero mentionsAll → te alcanza como miembro del canal.
  expect(mentionsUser({ mentionsAll: true }, 'u1')).toBe(true);
  // Incluso sin userId conocido (mentionsAll no lo exige).
  expect(mentionsUser({ mentionsAll: true }, '')).toBe(true);
  // Un @channel PROPIO no sube tu badge.
  expect(mentionsUser({ mine: true, mentionsAll: true }, 'u1')).toBe(false);
});

test('el cuerpo de la notificación recorta con elipsis por grafemas', () => {
  expect(mentionNotificationBody('  hola  ')).toBe('hola');
  expect(mentionNotificationBody('abcdef', 3)).toBe('abc…');
  // Recorte por runas (no bytes): 3 emojis siguen siendo 3.
  expect(mentionNotificationBody('😀😀😀', 3)).toBe('😀😀😀');
  // Y por GRAFEMAS, no code points: la familia es una secuencia ZWJ de cinco
  // code points pero un solo carácter visible; contarlos por separado partía
  // el emoji por la mitad y dejaba medio monigote en la notificación.
  expect(mentionNotificationBody('👨‍👩‍👧', 3)).toBe('👨‍👩‍👧');
  expect(mentionNotificationBody('👨‍👩‍👧👨‍👩‍👧', 1)).toBe('👨‍👩‍👧…');
});

const dm = { kind: 'direct' as const };
const unfocused = { windowFocused: false, enabled: true };

test('notifica un DM entrante con la ventana sin foco', () => {
  expect(shouldNotifyDirect({}, dm, unfocused)).toBe(true);
});

test('no notifica si la ventana tiene el foco (ya lo estás viendo)', () => {
  expect(shouldNotifyDirect({}, dm, { windowFocused: true, enabled: true })).toBe(false);
});

test('no notifica el ajuste desactivado, el propio, la respuesta en hilo, ni un canal silenciado', () => {
  expect(shouldNotifyDirect({}, dm, { windowFocused: false, enabled: false })).toBe(false);
  expect(shouldNotifyDirect({ mine: true }, dm, unfocused)).toBe(false);
  expect(shouldNotifyDirect({ parentId: 'root' }, dm, unfocused)).toBe(false);
  expect(shouldNotifyDirect({}, { kind: 'direct', muted: true }, unfocused)).toBe(false);
});

test('no notifica canales que no son DM (canal público/privado)', () => {
  expect(shouldNotifyDirect({}, { kind: 'public' }, unfocused)).toBe(false);
  expect(shouldNotifyDirect({}, undefined, unfocused)).toBe(false);
});

// --- Ola 13: nivel de notificación, palabras clave y No molestar (DND) ---

test('un DM plano solo avisa con notify_level all (o sin fijar); mentions/none no', () => {
  expect(shouldNotifyDirect({}, { kind: 'direct', notifyLevel: 'all' }, unfocused)).toBe(true);
  expect(shouldNotifyDirect({}, { kind: 'direct', notifyLevel: 'mentions' }, unfocused)).toBe(
    false,
  );
  // notify_level 'none' viaja como muted (⟺ none), ya cubierto, pero se comprueba igual:
  expect(
    shouldNotifyDirect({}, { kind: 'direct', notifyLevel: 'none', muted: true }, unfocused),
  ).toBe(false);
});

test('matchesKeyword: palabra completa, sin distinguir mayúsculas, no subcadena', () => {
  expect(matchesKeyword('Hay un Deploy pendiente', ['deploy'])).toBe(true);
  expect(matchesKeyword('cuidado con el redeploy', ['deploy'])).toBe(false); // no subcadena
  expect(matchesKeyword('incidencia grave.', ['incidencia'])).toBe(true); // límite por puntuación
  expect(matchesKeyword('nada relevante', ['deploy', 'incidencia'])).toBe(false);
  expect(matchesKeyword('lo que sea', [])).toBe(false);
  expect(matchesKeyword('lo que sea', undefined)).toBe(false);
});

test('inDndWindow: ventana del mismo día (09:00→17:00)', () => {
  const start = 9 * 60; // 540
  const end = 17 * 60; // 1020
  expect(inDndWindow(10 * 60, start, end)).toBe(true); // 10:00 dentro
  expect(inDndWindow(8 * 60, start, end)).toBe(false); // 08:00 antes
  expect(inDndWindow(17 * 60, start, end)).toBe(false); // 17:00 = fin (exclusivo)
});

test('inDndWindow: ventana que cruza medianoche (22:00→07:00)', () => {
  const start = 22 * 60; // 1320
  const end = 7 * 60; // 420
  expect(inDndWindow(23 * 60, start, end)).toBe(true); // 23:00 dentro
  expect(inDndWindow(1 * 60, start, end)).toBe(true); // 01:00 dentro (madrugada)
  expect(inDndWindow(12 * 60, start, end)).toBe(false); // 12:00 fuera
  expect(inDndWindow(7 * 60, start, end)).toBe(false); // 07:00 = fin (exclusivo)
});

test('inDndWindow: start==end desactiva el DND', () => {
  expect(inDndWindow(600, 480, 480)).toBe(false);
  expect(inDndWindow(480, 480, 480)).toBe(false);
});

/* ---------- «Todas las notificaciones» en un canal (no solo en DM) ---------- */

const canal = { kind: 'public' as const };
const despistado = { windowFocused: false, enabled: true };

test('un canal con «Todas las notificaciones» avisa: antes era un no-op', () => {
  // La opción estaba en la interfaz y se guardaba en la base de datos, pero la política
  // exigía `kind === 'direct'` y ahí se acababa: el usuario la elegía y no pasaba nada.
  expect(shouldNotifyNewMessage({}, { ...canal, notifyLevel: 'all' }, despistado)).toBe(true);
});

test('un canal SIN configurar no avisa de cada mensaje', () => {
  // El defecto contrario dejaría a cualquiera con cincuenta canales recibiendo un aviso
  // por cada mensaje de la organización. Sin fijar, un canal solo avisa por @mención,
  // que va por otra ruta.
  expect(shouldNotifyNewMessage({}, canal, despistado)).toBe(false);
  expect(shouldNotifyNewMessage({}, { ...canal, notifyLevel: 'mentions' }, despistado)).toBe(false);
  expect(shouldNotifyNewMessage({}, { ...canal, notifyLevel: 'none' }, despistado)).toBe(false);
});

test('un DM sí avisa por defecto: va dirigido a ti', () => {
  // Ahí la asimetría es deliberada — un DM sin configurar avisa; un canal, no.
  expect(shouldNotifyNewMessage({}, { kind: 'direct' }, despistado)).toBe(true);
  expect(shouldNotifyNewMessage({}, { kind: 'direct', notifyLevel: 'mentions' }, despistado)).toBe(
    false,
  );
});

test('silenciado gana sobre «all», y la respuesta en hilo nunca avisa por aquí', () => {
  expect(
    shouldNotifyNewMessage({}, { ...canal, notifyLevel: 'all', muted: true }, despistado),
  ).toBe(false);
  // Una respuesta en hilo tiene su propia ruta de aviso; avisar dos veces sería peor.
  expect(
    shouldNotifyNewMessage({ parentId: 'm1' }, { ...canal, notifyLevel: 'all' }, despistado),
  ).toBe(false);
});

// El detector de @channel/@here/@all: es el freno antes de despertar a un canal
// entero. Tiene que decir lo MISMO que `channelMentionRE` del backend —si se
// separan, o se confirma un mensaje que no notifica a nadie, o se notifica a
// todos sin preguntar—.
test('mentionsWholeChannel reconoce las tres formas, al principio y en medio', () => {
  expect(mentionsWholeChannel('@channel reunión a las 5')).toBe(true);
  expect(mentionsWholeChannel('ojo @here que se cae')).toBe(true);
  expect(mentionsWholeChannel('aviso para @all')).toBe(true);
});

test('mentionsWholeChannel no se dispara con nombres que empiezan igual', () => {
  // Casos que SÍ pasaban con un `includes('@channel')` ingenuo.
  expect(mentionsWholeChannel('@channelling')).toBe(false);
  expect(mentionsWholeChannel('@herest')).toBe(false);
  expect(mentionsWholeChannel('@allan trae el informe')).toBe(false);
  expect(mentionsWholeChannel('correo@here.com')).toBe(false);
  expect(mentionsWholeChannel('sin menciones')).toBe(false);
});
