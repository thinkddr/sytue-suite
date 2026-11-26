// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(linux && !android) && !windows

package local

// hayAppPara no se sabe de antemano: en macOS `open` falla si nadie atiende el esquema,
// y ese error es el que hace caer a la app del sistema.
func hayAppPara(string) (hay, sabido bool) { return false, false }
