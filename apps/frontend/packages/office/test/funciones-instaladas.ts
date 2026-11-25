/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Precarga de los tests (bunfig.toml): el editor de hojas importa `@sytue/formula/motor`,
 * sin las funciones, que en la app llegan aparte (`cargarFunciones`). Los tests esperan
 * un motor que las tenga todas, como siempre: el índice del paquete las instala al
 * cargarse. El motor SIN ellas lo prueba `calcus-funciones-perezosas.test.tsx`.
 */
import '@sytue/formula';
