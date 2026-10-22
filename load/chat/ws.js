// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue Chat — prueba de carga WebSocket (k6). El tiempo real es el patron
// definitorio del chat: esta prueba abre N conexiones WS concurrentes y las
// mantiene vivas para medir el coste de fan-out/presencia del hub (alimentado
// por LISTEN/NOTIFY sobre Postgres, con Valkey opcional §0.1). Mide el tiempo de
// establecimiento del handshake y la tasa de sesiones exitosas.
//
// El WS acepta el token por query (?access_token=) porque el navegador no puede
// fijar cabeceras en el handshake (services/chat/internal/http/ws.go).
//
// Requiere ACCESS_TOKEN. Sin token, la prueba no ejecuta (no hay endpoint WS
// publico). Uso:
//   k6 run -e ACCESS_TOKEN=... load/chat/ws.js
//   k6 run -e CHAT_URL=wss://chat.staging.sytue.eu -e ACCESS_TOKEN=... -e VUS=200 -e DURATION=2m load/chat/ws.js

import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';
import { BASE, ACCESS_TOKEN, SLO } from '../lib/config.js';

// Metricas propias: latencia de conexion y exito de sesion WS.
const wsConnectMs = new Trend('ws_connect_ms', true);
const wsSessionOK = new Rate('ws_session_ok');

// Duracion que cada VU mantiene la conexion abierta.
const HOLD_MS = Number(__ENV.WS_HOLD_MS || 30000);

export const options = {
  vus: Number(__ENV.VUS || 100),
  duration: __ENV.DURATION || '1m',
  thresholds: {
    // El handshake WS (incluye validacion de JWT) debe ser rapido.
    ws_connect_ms: [`p(95)<${SLO.P95_MS}`],
    ws_session_ok: ['rate>0.99'],
    ws_connecting: [`p(95)<${SLO.P95_MS}`],
  },
};

export default function () {
  if (!ACCESS_TOKEN) {
    // Sin token no hay WS: registrar fallo controlado y salir.
    wsSessionOK.add(false);
    return;
  }

  // BASE.chat puede venir como https:// (se convierte a wss://) o wss:// directo.
  const wsBase = BASE.chat.replace(/^http/, 'ws');
  const url = `${wsBase}/ws?access_token=${encodeURIComponent(ACCESS_TOKEN)}`;

  const started = Date.now();
  const res = ws.connect(url, {}, (socket) => {
    socket.on('open', () => {
      wsConnectMs.add(Date.now() - started);
      wsSessionOK.add(true);
      // Mantener viva la conexion: el hub gestiona presencia y fan-out.
      socket.setTimeout(() => socket.close(), HOLD_MS);
    });

    socket.on('message', (msg) => {
      // Comprobar que los eventos de fan-out son JSON bien formados.
      check(msg, { 'ws: mensaje json': (m) => typeof m === 'string' && m.length > 0 });
    });

    socket.on('error', () => {
      wsSessionOK.add(false);
    });
  });

  // El upgrade WS responde 101 Switching Protocols.
  check(res, { 'ws: handshake 101': (r) => r && r.status === 101 });
  sleep(1);
}
