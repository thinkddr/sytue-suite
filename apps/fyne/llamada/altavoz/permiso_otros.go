// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package altavoz

// PedirMicro no hace nada fuera de Android: en macOS e iOS el sistema pregunta solo al
// abrir el micrófono (con NSMicrophoneUsageDescription en el Info.plist, que pone
// empaquetar.sh), y Windows y Linux no preguntan.
func PedirMicro() error { return nil }
