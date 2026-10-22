// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows && !darwin

package ficheros

// marcarDeInternet no hace nada fuera de Windows y macOS: ni Linux ni los móviles
// tienen una marca de procedencia que respeten sus abridores.
func marcarDeInternet(string) error { return nil }
