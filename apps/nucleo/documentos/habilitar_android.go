// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package documentos

import "git.wow.st/gmp/jni"

func habilitar(habilitado bool) error {
	return llamar("habilitar", "(Landroid/content/Context;Z)V", func(jni.Env) []jni.Value {
		valor := jni.Value(0)
		if habilitado {
			valor = 1
		}
		return []jni.Value{valor}
	})
}
