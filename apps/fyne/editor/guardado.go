// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"sync"
	"time"
)

// AutoguardadoMS es AUTOSAVE_MS de `useDocument.ts`: cuánto se espera tras el último
// cambio para guardar.
const AutoguardadoMS = 1500

// Autoguardado agrupa los cambios y guarda UNA vez cuando dejan de llegar: el
// `saveTimerRef` de la web. Guarda en su propia rutina; `AlTerminar` recibe el resultado
// y es de quien llama devolverlo al hilo de la interfaz.
//
// Vive en el cromo y no en cada producto porque es el mismo para las tres apps que
// editan documentos de Sytue Docs. Nació en Diapos (`diapos.Autoguardado`), que puede
// quedarse con un alias a este y borrar su copia.
type Autoguardado struct {
	Espera     time.Duration
	Guardar    func() error
	AlEmpezar  func()
	AlTerminar func(error)

	mu    sync.Mutex
	reloj *time.Timer
	// enVuelo serializa los guardados: dos a la vez leerían la misma base y el segundo
	// se tomaría por un conflicto con otra persona.
	enVuelo sync.Mutex
}

// Cambio rearma el reloj.
func (a *Autoguardado) Cambio() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reloj != nil {
		a.reloj.Stop()
	}
	a.reloj = time.AfterFunc(a.Espera, a.disparar)
}

// Parar desarma el reloj sin guardar: el documento se ha cerrado.
func (a *Autoguardado) Parar() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reloj != nil {
		a.reloj.Stop()
	}
}

// Ahora guarda YA lo pendiente y desarma el reloj: cerrar el documento —o cambiarlo
// por otro— antes de que venza no pierde el último cambio, y el reloj no repite luego
// el mismo PUT. Guarda en la rutina de quien llama.
func (a *Autoguardado) Ahora() {
	a.Parar()
	a.disparar()
}

func (a *Autoguardado) disparar() {
	a.enVuelo.Lock()
	defer a.enVuelo.Unlock()
	if a.AlEmpezar != nil {
		a.AlEmpezar()
	}
	err := a.Guardar()
	if a.AlTerminar != nil {
		a.AlTerminar(err)
	}
}
