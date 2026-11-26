// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js || ios

package sistema

// Registrar: en el navegador y en iOS aún no hay descodificador del sistema (AVFoundation
// pide su puente nativo; ver gstreamer.go). El de Android está en mediacodec_android.go.
func Registrar() bool { return false }
