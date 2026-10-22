// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue Drive — prueba de carga (k6). Somete a los endpoints de listado y
// busqueda (lectura, el patron dominante en Drive) a carga sostenida,
// verificando el SLO p95 < 200 ms. Estos endpoints usan paginacion keyset e
// indices en Postgres (sin N+1); esta prueba detecta regresiones de latencia.
//
// Requiere ACCESS_TOKEN (JWT de Sytue ID) para las rutas autenticadas. Sin
// token, solo ejercita /healthz (util para validar el andamiaje HTTP).
//
//   k6 run -e ACCESS_TOKEN=... load/drive/load.js
//   k6 run -e DRIVE_URL=https://drive.staging.sytue.eu -e ACCESS_TOKEN=... -e VUS=80 load/drive/load.js

import http from 'k6/http';
import { group, sleep } from 'k6';
import { BASE, headers, ACCESS_TOKEN, loadThresholds, loadStages } from '../lib/config.js';
import { expectOK } from '../lib/checks.js';

export const options = {
  scenarios: {
    drive_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: loadStages(),
      gracefulRampDown: '20s',
    },
  },
  thresholds: loadThresholds(),
};

// ID de carpeta a listar (por defecto, la raiz). Parametrizable con -e FOLDER_ID.
const FOLDER_ID = __ENV.FOLDER_ID || '';

export default function () {
  const base = BASE.drive;

  if (!ACCESS_TOKEN) {
    expectOK(http.get(`${base}/healthz`, { headers: headers() }), 'healthz');
    sleep(1);
    return;
  }

  group('listado + busqueda (hot path de lectura)', () => {
    const root = http.get(`${base}/api/v1/folders/root`, { headers: headers() });
    expectOK(root, 'folders_root');

    // Listado de hijos con keyset (limit por defecto del servicio).
    let folderID = FOLDER_ID;
    if (!folderID) {
      try {
        folderID = root.json('id');
      } catch {
        folderID = '';
      }
    }
    if (folderID) {
      expectOK(
        http.get(`${base}/api/v1/folders/${folderID}/children`, { headers: headers() }),
        'folder_children',
      );
    }

    expectOK(http.get(`${base}/api/v1/search?q=informe`, { headers: headers() }), 'search');
    expectOK(http.get(`${base}/api/v1/quota`, { headers: headers() }), 'quota');
  });

  sleep(1);
}
