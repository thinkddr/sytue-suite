// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || darwin

package ajustes

// GrupoSuite es el grupo de acceso al Keychain de las apps firmadas por el equipo.
// Es estable entre productos y actualizaciones; las extensiones de archivos usan
// su grupo separado y no reciben acceso a estos refresh tokens.
const GrupoSuite = "VHC5QW7BZ3.eu.sytue.suite"

// GrupoContenedorSuite es el App Group de marcas, preferencias y snapshots. Debe
// estar registrado y autorizado en los perfiles antes de distribuir las apps.
// Los tokens permanecen exclusivamente en GrupoSuite del Keychain.
const GrupoContenedorSuite = "group.eu.sytue.suite"
