// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// checks.js — helpers de asercion compartidos por los scripts k6. Comprueban
// codigo de estado, latencia contra el SLO p95 y contenido JSON esperado.

import { check } from 'k6';
import { SLO } from './config.js';

// Verifica una respuesta: estado esperado y latencia dentro del SLO p95.
// `name` etiqueta el check en el reporte. Devuelve true si todo pasa.
export function expectOK(res, name, status) {
  const want = status || 200;
  return check(
    res,
    {
      [`${name}: status ${want}`]: (r) => r.status === want,
      [`${name}: p95 < ${SLO.P95_MS}ms`]: (r) => r.timings.duration < SLO.P95_MS,
    },
    { endpoint: name },
  );
}

// Verifica que el cuerpo es JSON y contiene la clave indicada.
export function expectJSONKey(res, name, key) {
  return check(
    res,
    {
      [`${name}: json valido`]: (r) => {
        try {
          const body = r.json();
          return body !== null && Object.prototype.hasOwnProperty.call(body, key);
        } catch {
          return false;
        }
      },
    },
    { endpoint: name },
  );
}
