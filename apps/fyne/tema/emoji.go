// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import (
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
)

var (
	emojiUnaVez sync.Once
	caraEmoji   fyne.Resource
)

// Emoji es la cara de color portable. Se construye al pedirla por primera vez:
// una app que solo usa Figtree no enlaza el recurso del selector de emoji.
func Emoji() fyne.Resource {
	emojiUnaVez.Do(func() {
		caraEmoji = fyne.NewStaticResource("NotoColorEmoji.ttf", fuentes.Emoji())
	})
	return caraEmoji
}
