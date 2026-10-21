// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue Chat — smoke test (k6). Verifica salud y, con token, los endpoints REST
// de listado (teams). El tiempo real (WS) se ejercita en ws.js. La API esta
// autenticada (JWT via JWKS) y aislada por tenant (RLS).
//
// Endpoints (services/chat/internal/http/server.go):
//   GET /healthz, /readyz
//   GET /api/v1/teams
//
// Uso:
//   k6 run load/chat/smoke.js
//   k6 run -e CHAT_URL=https://chat.staging.sytue.eu -e ACCESS_TOKEN=... load/chat/smoke.js

import http from 'k6/http';
import { group, sleep } from 'k6';
import { BASE, headers, ACCESS_TOKEN, smokeThresholds } from '../lib/config.js';
import { expectOK } from '../lib/checks.js';

export const options = {
  vus: 1,
  iterations: 10,
  thresholds: smokeThresholds(),
};

export default function () {
  const base = BASE.chat;

  group('salud', () => {
    expectOK(http.get(`${base}/healthz`, { headers: headers() }), 'healthz');
    expectOK(http.get(`${base}/readyz`, { headers: headers() }), 'readyz');
  });

  if (ACCESS_TOKEN) {
    group('api rest autenticada', () => {
      expectOK(http.get(`${base}/api/v1/teams`, { headers: headers() }), 'teams');
    });
  }

  sleep(0.5);
}
