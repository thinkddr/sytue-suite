// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package videoteca

import "context"

// miniaturaAndroid solo existe en Android (miniatura_android.go).
func miniaturaAndroid(context.Context, string) ([]byte, error) { return nil, ErrSinMiniatura }
