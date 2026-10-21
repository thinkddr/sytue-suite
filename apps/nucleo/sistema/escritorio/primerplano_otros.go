// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package escritorio

func mantenerAvisos(string, string) error { return nil }

func soltarAvisos() error { return nil }
