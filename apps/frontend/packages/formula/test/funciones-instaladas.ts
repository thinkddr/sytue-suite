/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Precarga de los tests (bunfig.toml): instala las funciones de la librería, que el
 * índice del paquete instala al cargarse (perezosas.ts). Los tests construyen el motor
 * importando `./engine.js` directamente y esperan que un `new Engine()` las tenga todas,
 * como siempre; el motor SIN ellas lo prueba `perezosas.test.ts`.
 */
import '../src/index.js';
