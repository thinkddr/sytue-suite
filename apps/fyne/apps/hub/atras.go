// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Atajos conecta «atrás» después de montar el Hub en su ventana.
func (v *Vista) Atajos(c fyne.Canvas) { ui.AlAtras(c, v.atras) }

func (v *Vista) atras() bool {
	d := v.Destino()
	switch {
	case d == ADispositivos && v.disp.Paso > 0:
		v.disp.Ir(v.disp.Paso - 1)
	case d == ABienvenida && v.bienv.volverPaso():
	case d != AInicio:
		v.Ir(AInicio)
	default:
		return false
	}
	return true
}

// volverPaso es también la acción del botón del asistente: mientras se guarda no
// se sale, y retroceder conserva los datos que ya se habían escrito en cada paso.
func (b *Bienvenida) volverPaso() bool {
	if b.sincronizando.Load() {
		return true
	}
	b.mu.Lock()
	if b.ocupado {
		b.mu.Unlock()
		return true
	}
	if b.paso <= 0 {
		b.mu.Unlock()
		return false
	}
	b.paso--
	b.mu.Unlock()
	b.Refresh()
	return true
}
