// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// almacenFalso hace de llavero y de Touch ID a la vez. Con cerrojo: la verificación y la
// lectura van en su rutina.
type almacenFalso struct {
	mu          sync.Mutex
	datos       map[string][]byte
	noVerifica  error
	verificados int
}

func (a *almacenFalso) recordador() *Recordador {
	return &Recordador{
		Metodo: "Touch ID", Motivo: "abrir tu cuaderno cifrado",
		Verificar: func(context.Context, string) error {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.verificados++
			return a.noVerifica
		},
		Leer: func(c string) ([]byte, error) {
			a.mu.Lock()
			defer a.mu.Unlock()
			return a.datos[c], nil
		},
		Guardar: func(c string, d []byte) error {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.datos[c] = d
			return nil
		},
		Borrar: func(c string) error {
			a.mu.Lock()
			defer a.mu.Unlock()
			delete(a.datos, c)
			return nil
		},
	}
}

func (a *almacenFalso) guardada(c string) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.datos[c]
}

// TestElCuadernoCifradoSeAbreConTouchIDSiSuDuenoLoPidio: con la clave recordada y el
// dueño verificado, el cuaderno se abre sin contraseña; si cancela, no se abre (queda la
// contraseña, que vale siempre); si la contraseña cambió desde la web, la guardada ya no
// vale y se olvida sola. Recordar pide verificar, y desmarcar la casilla la olvida.
// VISTO EN ROJO abriendo sin verificar y sin comprobar la clave contra el testigo.
func TestElCuadernoCifradoSeAbreConTouchIDSiSuDuenoLoPidio(t *testing.T) {
	c, clave, err := api.NuevoCifradoNotas("contraseña larga de prueba")
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := json.Marshal(c)
	cifrado, _ := api.EmpaquetarNota(clave, api.NotaEnClaro{Title: "Diario", Content: "<p>hoy</p>"})
	nota := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: cifrado, Data: []byte(`{}`)}
	cuenta := cuentaCuaderno("c1")

	abrir := func(a *almacenFalso) *Pantalla {
		p, _ := pantallaConServicio(t, nil, nota)
		// Con ventana, que el cuaderno siga cerrado se VE: sale el diálogo de la contraseña.
		p.win = test.NewTempWindow(t, p.Objeto())
		p.win.Resize(fyne.NewSize(1280, 800))
		p.activo.Cifrado, p.activo.Crudo.Crypto, p.bloqueado = true, crudo, true
		p.Recordar = a.recordador()
		fyne.DoAndWait(p.desbloquear)
		return p
	}
	// abierta espera a que la rutina de la verificación acabe —con el cuaderno abierto o
	// con el diálogo de la contraseña en pantalla, que es lo ÚLTIMO que hace— y dice si se
	// abrió. Sin dormir: antes se daba 20 ms de margen tras verificar, y bajo carga la
	// comprobación de la clave vieja no llegaba a olvidarla a tiempo.
	abierta := func(p *Pantalla) bool {
		t.Helper()
		fin := time.Now().Add(10 * time.Second)
		for time.Now().Before(fin) {
			var ok, pregunta bool
			fyne.DoAndWait(func() {
				ok, pregunta = p.clave != nil, len(p.win.Canvas().Overlays().List()) > 0
			})
			if ok || pregunta {
				return ok
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("la verificación no ha terminado en diez segundos")
		return false
	}

	a := &almacenFalso{datos: map[string][]byte{cuenta: clave}}
	p := abrir(a)
	if !abierta(p) {
		t.Fatal("con la clave recordada y el dueño verificado, el cuaderno sigue cerrado")
	}
	var titulo string
	fyne.DoAndWait(func() { n, _ := p.nota("n1"); titulo = n.Titulo })
	if titulo != "Diario" {
		t.Errorf("abierto, la nota es %q", titulo)
	}

	cancela := &almacenFalso{datos: map[string][]byte{cuenta: clave}, noVerifica: errors.New("cancelado")}
	if p := abrir(cancela); abierta(p) {
		t.Error("sin verificar al dueño, el cuaderno se ha abierto")
	}
	if cancela.guardada(cuenta) == nil {
		t.Error("cancelar Touch ID ha olvidado la clave recordada")
	}

	vieja := &almacenFalso{datos: map[string][]byte{cuenta: make([]byte, 32)}}
	if p := abrir(vieja); abierta(p) {
		t.Error("una clave que ya no abre el cuaderno lo ha abierto")
	}
	if vieja.guardada(cuenta) != nil {
		t.Error("la clave que ya no vale se sigue recordando")
	}

	// Recordar pide verificar; sin verificar no se guarda; desmarcar la casilla olvida.
	nueva := &almacenFalso{datos: map[string][]byte{}}
	p, _ = pantallaConServicio(t, nil, nota)
	p.Recordar = nueva.recordador()
	p.recordarClave("c1", clave, true)
	if nueva.guardada(cuenta) == nil || nueva.verificados != 1 {
		t.Errorf("recordar: guardada %v tras %d verificaciones", nueva.guardada(cuenta) != nil, nueva.verificados)
	}
	p.recordarClave("c1", clave, false)
	if nueva.guardada(cuenta) != nil {
		t.Error("desmarcar la casilla no la olvida")
	}
	nueva.noVerifica = errors.New("no es el dueño")
	p.recordarClave("c1", clave, true)
	if nueva.guardada(cuenta) != nil {
		t.Error("sin verificar al dueño se ha recordado la clave")
	}
}
