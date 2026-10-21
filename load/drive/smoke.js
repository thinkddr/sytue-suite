// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue Drive — smoke test (k6). Verifica las sondas de salud y, si se aporta un
// token (ACCESS_TOKEN de Sytue ID), los endpoints REST criticos de listado. La
// API de Drive esta autenticada (JWT via JWKS) y aislada por tenant (RLS), por
// lo que sin token solo se comprueba salud.
//
// Endpoints (services/drive/internal/http/server.go):
//   GET /healthz, /readyz
//   GET /api/v1/folders/root                      (raiz del tenant)
//   GET /api/v1/quota                             (cuota del tenant)
//   GET /api/v1/search?q=...                      (busqueda)
//
// Uso:
//   k6 run load/drive/smoke.js
//   k6 run -e DRIVE_URL=https://drive.staging.sytue.eu -e ACCESS_TOKEN=... load/drive/smoke.js

import http from 'k6/http';
import { group, sleep } from 'k6';
import { BASE, headers, ACCESS_TOKEN, smokeThresholds } from '../lib/config.js';
import { expectOK, expectJSONKey } from '../lib/checks.js';

export const options = {
  vus: 1,
  iterations: 10,
  thresholds: smokeThresholds(),
};

export default function () {
  const base = BASE.drive;

  group('salud', () => {
    expectOK(http.get(`${base}/healthz`, { headers: headers() }), 'healthz');
    expectOK(http.get(`${base}/readyz`, { headers: headers() }), 'readyz');
  });

  if (ACCESS_TOKEN) {
    group('api rest autenticada', () => {
      const root = http.get(`${base}/api/v1/folders/root`, { headers: headers() });
      expectOK(root, 'folders_root');
      expectJSONKey(root, 'folders_root', 'id');

      expectOK(http.get(`${base}/api/v1/quota`, { headers: headers() }), 'quota');
      expectOK(http.get(`${base}/api/v1/search?q=informe`, { headers: headers() }), 'search');
    });
  }

  sleep(0.5);
}
