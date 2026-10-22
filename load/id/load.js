// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue ID — prueba de carga (k6). Somete a los endpoints de descubrimiento y
// JWKS (las rutas mas consultadas del IdP: cada validacion de token de cada
// servicio de la suite las golpea) a carga sostenida, verificando el SLO
// p95 < 200 ms bajo concurrencia. Los umbrales rompen el build en CI.
//
// Perfil (parametrizable): rampa -> meseta de VUS -> bajada.
//   k6 run load/id/load.js
//   k6 run -e ID_URL=https://id.staging.sytue.eu -e VUS=100 -e DURATION=5m load/id/load.js

import http from 'k6/http';
import { group, sleep } from 'k6';
import { BASE, headers, ACCESS_TOKEN, loadThresholds, loadStages } from '../lib/config.js';
import { expectOK } from '../lib/checks.js';

export const options = {
  scenarios: {
    id_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: loadStages(),
      gracefulRampDown: '20s',
    },
  },
  thresholds: loadThresholds(),
};

export default function () {
  const base = BASE.id;

  group('discovery + jwks (hot path de validacion de tokens)', () => {
    // Estos dos endpoints los cachean los servicios (Cache-Control: max-age=300)
    // pero se golpean masivamente al arranque y renovacion de claves.
    expectOK(http.get(`${base}/.well-known/openid-configuration`, { headers: headers() }), 'discovery');
    expectOK(http.get(`${base}/.well-known/jwks.json`, { headers: headers() }), 'jwks');
  });

  expectOK(http.get(`${base}/healthz`, { headers: headers() }), 'healthz');

  if (ACCESS_TOKEN) {
    // /userinfo es el endpoint autenticado mas caliente (cada login lo consulta).
    expectOK(http.get(`${base}/oauth2/userinfo`, { headers: headers() }), 'userinfo');
  }

  sleep(1);
}
