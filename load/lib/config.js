// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// config.js — configuracion compartida de las pruebas de carga k6 de Sytue One.
// Centraliza URLs base de los servicios Go (id/drive/chat), los umbrales (SLO)
// canonicos y un token opcional para endpoints autenticados. Se parametriza por
// variables de entorno (-e VAR=valor) para no fijar secretos ni URLs en codigo.
//
// SLO (contrato §0.8): p95 API < 200 ms. Los umbrales de k6 hacen fallar la
// prueba (exit != 0) si se incumplen -> gate de rendimiento en CI.

// Bases por servicio. En dev: *.sytue.localhost (Traefik). En CI/staging se
// sobreescriben con -e ID_URL=... etc. Sin barra final.
export const BASE = {
  id: __ENV.ID_URL || 'https://id.sytue.localhost',
  drive: __ENV.DRIVE_URL || 'https://drive.sytue.localhost',
  chat: __ENV.CHAT_URL || 'https://chat.sytue.localhost',
};

// Token de acceso (JWT de Sytue ID) para endpoints autenticados. Placeholder:
// se inyecta en CI con -e ACCESS_TOKEN=... (nunca se versiona un token real).
export const ACCESS_TOKEN = __ENV.ACCESS_TOKEN || '';

// Cabeceras estandar. Bearer solo si hay token (endpoints publicos no lo usan).
export function headers(extra) {
  const h = { Accept: 'application/json', ...(extra || {}) };
  if (ACCESS_TOKEN) h.Authorization = `Bearer ${ACCESS_TOKEN}`;
  return h;
}

// SLO canonico de latencia (contrato §0.8): p95 < 200 ms, p99 < 500 ms.
// Se anade un pequeno margen para el salto de red del entorno de prueba.
export const SLO = {
  P95_MS: Number(__ENV.SLO_P95_MS || 200),
  P99_MS: Number(__ENV.SLO_P99_MS || 500),
  ERROR_RATE: Number(__ENV.SLO_ERROR_RATE || 0.01), // < 1 % de fallos
};

// Umbrales de k6 para una prueba de carga: rompen el build si se incumplen.
export function loadThresholds() {
  return {
    http_req_duration: [`p(95)<${SLO.P95_MS}`, `p(99)<${SLO.P99_MS}`],
    http_req_failed: [`rate<${SLO.ERROR_RATE}`],
    checks: ['rate>0.99'],
  };
}

// Umbrales de un smoke test: mismos SLO, tolerancia cero a fallos.
export function smokeThresholds() {
  return {
    http_req_duration: [`p(95)<${SLO.P95_MS}`],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  };
}

// Perfil de carga por defecto: rampa de subida, meseta y bajada.
// Ajustable por entorno (-e VUS=50 -e DURATION=5m).
export function loadStages() {
  const vus = Number(__ENV.VUS || 50);
  const ramp = __ENV.RAMP || '30s';
  const hold = __ENV.DURATION || '2m';
  return [
    { duration: ramp, target: vus },
    { duration: hold, target: vus },
    { duration: '30s', target: 0 },
  ];
}
