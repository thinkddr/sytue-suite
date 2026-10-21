#!/usr/bin/env node
// Self-check de la logica pura del puente (www/native.js): parseo de no-leidos y
// traduccion de deep-links. Sin frameworks: assert + `node scripts/selfcheck.cjs`.
'use strict';
const assert = require('node:assert');
const { parseUnread, deepLinkPath } = require('../www/native.js');

const O = 'https://rooms.sytue.eu';

// parseUnread: titulo "(N)" / "(N+)" de Rooms/Mattermost.
assert.strictEqual(parseUnread('(3) Sytue Rooms'), 3);
assert.strictEqual(parseUnread('  (12+) #general'), 12);
assert.strictEqual(parseUnread('Sytue Rooms'), 0);
assert.strictEqual(parseUnread(''), 0);
assert.strictEqual(parseUnread(undefined), 0);

// Contrato común de la suite: sytue-rooms://open?url=<https del producto>.
assert.strictEqual(
  deepLinkPath(
    'sytue-rooms://open?url=https%3A%2F%2Frooms.sytue.eu%2Fchannels%2Ftown-square%3Fmsg%3D42',
    O,
  ),
  '/channels/town-square?msg=42',
);
assert.strictEqual(
  deepLinkPath('sytue-rooms://open?url=https%3A%2F%2Fevil.example%2Fchannels%2Fx', O),
  '',
);
assert.strictEqual(deepLinkPath('sytue-rooms://open', O), '');
// Contrato histórico rooms:// -> ruta (compatibilidad con enlaces publicados).
assert.strictEqual(deepLinkPath('rooms://channels/town-square', O), '/channels/town-square');
assert.strictEqual(deepLinkPath('rooms://channels/x?msg=42', O), '/channels/x?msg=42');
// Universal link del MISMO origen -> ruta.
assert.strictEqual(deepLinkPath(O + '/channels/dev', O), '/channels/dev');
// Otro origen (externo) -> '' (se ignora: abre en el navegador del sistema).
assert.strictEqual(deepLinkPath('https://evil.example/channels/x', O), '');
// Malformado -> ''.
assert.strictEqual(deepLinkPath('not a url', O), '');

console.log('selfcheck OK');
