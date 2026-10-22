// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android && !ios

package sesion

// En escritorio el selector es el del sistema (`ficheros.Elegir`), sin toolkit.
func (s *Suite) ponerSelector() {}
