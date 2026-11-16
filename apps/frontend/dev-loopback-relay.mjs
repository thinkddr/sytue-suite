// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// SOLO DESARROLLO (Docker Compose). Bun resuelve cualquier `*.localhost` a
// 127.0.0.1 (RFC 6761) sin preguntar al DNS de Docker, así que el BFF de una app
// web que pide `https://id.sytue.localhost` desde su contenedor se conectaba a sí
// mismo y fallaba con ECONNREFUSED: ningún login funcionaba en local. Este relé
// escucha en 127.0.0.1:443 DENTRO del contenedor y pasa los bytes tal cual a
// Traefik, que enruta por SNI con el nombre real. No termina TLS ni lee nada.
//
// Lo arranca el entrypoint de los frontends en deploy/compose/docker-compose.yml.
// Destino: SYTUE_DEV_LOOPBACK_TARGET (por defecto traefik:443).
import net from 'node:net';

const objetivo = process.env.SYTUE_DEV_LOOPBACK_TARGET ?? 'traefik:443';
const separador = objetivo.lastIndexOf(':');
const host = objetivo.slice(0, separador);
const puerto = Number(objetivo.slice(separador + 1));

net
  .createServer((cliente) => {
    const destino = net.connect(puerto, host);
    cliente.pipe(destino);
    destino.pipe(cliente);
    cliente.on('error', () => destino.destroy());
    destino.on('error', () => cliente.destroy());
  })
  .on('error', (error) => {
    console.error(`[sytue] relé de loopback desactivado: ${error.message}`);
  })
  .listen(443, '127.0.0.1');
