// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package videoteca

import "context"

// miniaturaExplorador: el Explorador solo está en Windows.
func miniaturaExplorador(context.Context, string) ([]byte, error) { return nil, ErrSinMiniatura }
