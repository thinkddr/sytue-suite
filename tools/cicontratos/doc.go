// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cicontratos guarda los contratos de los workflows de CI: lo que un
// fichero de `.github/workflows` tiene que seguir cumpliendo aunque nadie lo
// mire. Son pruebas de Go (CONVENTIONS §13.4): corren con el resto de
// `go test ./...`, sin otro intérprete ni otro paso en el flujo.
package cicontratos
