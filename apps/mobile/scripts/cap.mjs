#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Ejecuta la CLI de Capacitor con su telemetría APAGADA, siempre.
//
// POR QUÉ EXISTE
// --------------
// `@capacitor/cli` manda métricas a `metrics-capacitor.outsystems.com`
// (OutSystems, Estados Unidos) y lo hace **opt-out**: la primera vez que
// alguien ejecuta `cap` desde una terminal se activa sola, escribe
// `telemetry: true` en la configuración del usuario y avisa después.
//
// Lo que envía no es anónimo en ningún sentido útil:
//
//   · `app_id` es el SHA-1 del PRIMER COMMIT del repositorio, o sea una huella
//     estable y única de este monorepo privado;
//   · el comando ejecutado, sus argumentos literales y el JSON de opciones;
//   · el MENSAJE DE ERROR cuando algo falla, que arrastra rutas locales y
//     detalle interno;
//   · versión de Node, sistema operativo y versiones de cada `@capacitor/*`.
//
// En CI no dispara, pero eso es un accidente y no una decisión: depende de que
// la librería detecte que no hay terminal. El `DO_NOT_TRACK=1` de la imagen del
// runner NO sirve aquí —este código no lo lee—, así que en cuanto alguien
// ejecute `bun run sync` desde su portátil, sale.
//
// CÓMO
// ----
// `cap telemetry off` escribe `telemetry: false` en la configuración del
// usuario, y `sendMetric` comprueba ese valor antes de enviar nada. Se invoca
// con la propia CLI en lugar de escribir el fichero a mano a propósito: la ruta
// depende del sistema operativo y replicarla aquí sería otra copia que se
// desincroniza. Es idempotente y barato.
import { spawnSync } from 'node:child_process';

const argumentos = process.argv.slice(2);

// `stdio: 'ignore'`: si esto falla —CLI vieja sin el subcomando, permisos— no
// se interrumpe el trabajo de nadie. Pero el fallo NO se traga en silencio:
// se avisa, porque significa que la siguiente orden puede llevarse la huella
// del repositorio.
const apagado = spawnSync('cap', ['telemetry', 'off'], {
  stdio: 'ignore',
  shell: false,
});
if (apagado.status !== 0) {
  process.stderr.write(
    '⚠ no se pudo apagar la telemetría de Capacitor: revísalo antes de seguir,\n' +
      '  porque manda la huella de este repositorio a un tercero de EE. UU.\n',
  );
}

const resultado = spawnSync('cap', argumentos, { stdio: 'inherit', shell: false });
process.exit(resultado.status ?? 1);
