// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue ID — smoke test (k6). Verifica que los endpoints publicos criticos del
// IdP responden y cumplen el SLO p95 < 200 ms con carga minima (1 VU). Es la
// prueba de humo que se ejecuta en cada despliegue y en CI antes de la carga.
//
// Endpoints (services/id/internal/http/server.go):
//   GET /healthz                                 (liveness)
//   GET /readyz                                  (readiness: Postgres)
//   GET /.well-known/openid-configuration        (OIDC Discovery)
//   GET /.well-known/jwks.json                   (JWKS)
//   GET /api/v1/me                               (solo si hay ACCESS_TOKEN)
//
// Uso:
//   k6 run load/id/smoke.js
//   k6 run -e ID_URL=https://id.staging.sytue.eu load/id/smoke.js

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
  const base = BASE.id;

  group('salud', () => {
    expectOK(http.get(`${base}/healthz`, { headers: headers() }), 'healthz');
    // /readyz puede devolver 503 si una dependencia esta caida; aqui exigimos 200.
    expectOK(http.get(`${base}/readyz`, { headers: headers() }), 'readyz');
  });

  group('oidc discovery', () => {
    const disc = http.get(`${base}/.well-known/openid-configuration`, { headers: headers() });
    expectOK(disc, 'discovery');
    expectJSONKey(disc, 'discovery', 'issuer');

    const jwks = http.get(`${base}/.well-known/jwks.json`, { headers: headers() });
    expectOK(jwks, 'jwks');
    expectJSONKey(jwks, 'jwks', 'keys');
  });

  if (ACCESS_TOKEN) {
    group('api autenticada', () => {
      expectOK(http.get(`${base}/api/v1/me`, { headers: headers() }), 'me');
    });
  }

  sleep(0.5);
}
