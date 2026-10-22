// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
)

// previaAntesala es lo que la antesala abre ANTES de entrar, como PreJoin.tsx: la
// cámara para verse, el micrófono para ver que capta (el medidor) y la lista de
// dispositivos para elegirlos. Todo se suelta al entrar —la llamada abre los suyos— o
// al cerrar la ventana. Cada cosa se abre cuando la persona la enciende, que es cuando
// se pide el permiso (ADR 0046).
type previaAntesala struct {
	sa *Sala
	a  *Antesala
	w  fyne.Window

	previa      camara.Fuente
	pararPrevia func()

	cerrarMicro func()
	pararNivel  func()
	mu          sync.Mutex
	pico        float32
}

// Muestras por segundo del medidor y de la previa: las de la web (`requestAnimationFrame`
// para el nivel, el vídeo a su ritmo), acotadas a 15 para no despertar la interfaz más.
const ritmoAntesala = time.Second / 15

func (p *previaAntesala) preparar() {
	sa, a := p.sa, p.a
	if sa.Dispositivos != nil {
		a.Micros, a.Altavoces, a.Camaras = sa.Dispositivos()
	}
	a.MicroID, a.AltavozID, a.CamaraID = sa.MicroElegido, sa.AltavozElegido, sa.CamaraElegida
	a.AlCambiarCamara = p.alternarCamara
	a.AlCambiarMicro = func() {
		a.Micro = !a.Micro
		p.medir()
		a.Refresh()
	}
	a.AlElegirMicro = func(id string) { sa.MicroElegido, a.MicroID = id, id; p.medir() }
	a.AlElegirAltavoz = func(id string) { sa.AltavozElegido, a.AltavozID = id, id; p.medir() }
	a.AlElegirCamara = func(id string) {
		sa.CamaraElegida, a.CamaraID = id, id
		if a.Camara { // la previa pasa a la cámara elegida
			p.soltarCamara()
			a.Camara = false
			p.alternarCamara()
		}
	}
	p.medir()
}

// medir abre (o reabre con el micro elegido) el micrófono solo para el medidor, si el
// micro está encendido; si no, lo suelta.
func (p *previaAntesala) medir() {
	p.soltarMicro()
	sa, a := p.sa, p.a
	if !a.Micro || sa.AudioDe == nil {
		a.PonerNivel(0)
		return
	}
	d := sa.AudioDe(sa.MicroElegido, sa.AltavozElegido)
	if d == nil {
		return
	}
	cerrar, err := d.Abrir(func(pcm []int16) {
		pico := picoDe(pcm)
		p.mu.Lock()
		p.pico = max(p.pico, pico)
		p.mu.Unlock()
	}, func(out []int16) { clear(out) })
	if err != nil {
		return // sin permiso o sin tarjeta: la barra se queda a cero, como la web
	}
	p.cerrarMicro = cerrar
	var pintar func()
	pintar = func() {
		p.mu.Lock()
		n := p.pico
		p.pico = 0
		p.mu.Unlock()
		a.PonerNivel(n)
		p.pararNivel = sa.Esperar(ritmoAntesala, pintar)
	}
	p.pararNivel = sa.Esperar(ritmoAntesala, pintar)
}

// picoDe es el nivel de un tramo: el pico sobre la mitad de la escala, como la web
// (`pico / 64` sobre muestras de 8 bits centradas en 128), acotado a 1.
func picoDe(pcm []int16) float32 {
	var pico int32
	for _, v := range pcm {
		pico = max(pico, max(int32(v), -int32(v)))
	}
	return min(float32(pico)/16384, 1)
}

func (p *previaAntesala) alternarCamara() {
	sa, a := p.sa, p.a
	if a.SinCamara {
		return
	}
	if a.Camara {
		a.Camara = false
		p.soltarCamara()
		a.Refresh()
		return
	}
	pedir(sa, func(context.Context) (camara.Fuente, error) { return sa.Camara() }, func(f camara.Fuente, err error) {
		a.Denegado = errors.Is(err, camara.ErrSinPermiso)
		if err != nil || sa.ventanaLlamada != p.w {
			if f != nil {
				f.Cerrar()
			}
			a.Camara = false
			a.Refresh()
			return
		}
		a.Camara, p.previa = true, f
		a.Refresh()
		p.pintarPrevia()
	})
}

func (p *previaAntesala) pintarPrevia() {
	if p.previa == nil {
		return
	}
	if img, ok := p.previa.Leer(); ok {
		p.a.PonerPrevia(img)
	}
	p.pararPrevia = p.sa.Esperar(ritmoAntesala, p.pintarPrevia)
}

func (p *previaAntesala) soltarCamara() {
	if p.pararPrevia != nil {
		p.pararPrevia()
		p.pararPrevia = nil
	}
	if p.previa != nil {
		p.previa.Cerrar()
		p.previa = nil
	}
	p.a.PonerPrevia(nil)
}

func (p *previaAntesala) soltarMicro() {
	if p.pararNivel != nil {
		p.pararNivel()
		p.pararNivel = nil
	}
	if p.cerrarMicro != nil {
		p.cerrarMicro()
		p.cerrarMicro = nil
	}
}

// soltar suelta la cámara y el micrófono de la antesala.
func (p *previaAntesala) soltar() {
	p.soltarCamara()
	p.soltarMicro()
	p.a.PonerNivel(0)
}
