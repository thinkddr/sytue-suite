// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build !js && !edicionlocal

package offline

import "git.sytue.eu/sytue/one/apps/nucleo/auth"

// systemSecrets es el llavero del sistema de la sesión de la suite (`nucleo/auth`).
type systemSecrets struct{}

func (systemSecrets) Read(k string) ([]byte, error)  { return auth.LeerSecreto(k) }
func (systemSecrets) Write(k string, v []byte) error { return auth.GuardarSecreto(k, v) }
