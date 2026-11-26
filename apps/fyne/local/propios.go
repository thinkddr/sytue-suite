// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import "git.sytue.eu/sytue/one/apps/nucleo/i18n"

// Propios es una tabla de textos propios de una app o un paquete de la edición (PLAN
// §5.14, C-12): español e inglés en Base, los otros veinticuatro idiomas en
// Traducciones, con `En(idioma, clave)` y `Faltan()`. Vive en `i18n.Propios` para que la
// usen también los paquetes que no pueden importar la cáscara (los que comparte la
// edición conectada); este es el mismo tipo.
type Propios = i18n.Propios
