// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package documentos

import (
	"context"
	"git.wow.st/gmp/jni"
)

func abrir(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return llamar("abrir", "(Landroid/content/Context;)V", func(jni.Env) []jni.Value { return nil })
}
