// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !linux && !darwin

package main

import "errors"

// clonarFichero no sabe clonar en este sistema: quien llama copia.
func clonarFichero(string, string) error { return errors.ErrUnsupported }
