//go:build ventanareal

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func init() {
	runtime.LockOSThread()
	enVentanaReal = ventanaReal
}

// ventanaReal abre Sytue Docus en una ventana real (GL de Mesa bajo Xvfb) contra un
// servidor HTTP de prueba, escribe emojis compuestos localmente, recibe por el canal en
// vivo emojis compuestos remotos («👩🏽‍🔬», «🇪🇸», «é») junto con una op inválida de varios
// caracteres (FYE-05), verifica la convergencia y deja las capturas en dir.
func ventanaReal(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		return err
	}
	s := &servicioDocs{
		rol:     "editor",
		html:    "<h1>Prueba de coedición</h1><p>Acta</p>",
		remotas: `[]`,
	}
	srv := httptest.NewServer(s)
	defer srv.Close()

	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})
	w := a.NewWindow("Sytue Docus")
	w.SetPadded(false)

	d := NuevaApp(tr.T, "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	d.EnVivo = false
	d.Ventana = w
	w.SetContent(d.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	w.Show()

	captura := func(nombre string) error {
		if out, err := exec.Command("import", "-window", "root", dir+"/"+nombre+".png").CombinedOutput(); err != nil {
			return fmt.Errorf("import %s: %w %s", nombre, err, out)
		}
		return nil
	}

	fallo := make(chan error, 1)
	go func() {
		defer fyne.Do(a.Quit)
		pausa := func() { time.Sleep(1500 * time.Millisecond) }

		fyne.Do(func() { d.AbrirID("d1", "Propuesta") })
		pausa()
		if err := captura("1-inicial"); err != nil {
			fallo <- err
			return
		}

		// Escritura local de emoji compuesto con modificador de tono de piel («👍🏽»)
		// y guardado inmediato para sembrar la secuencia RGA.
		var actor string
		var puntoTrasEmoji uint64
		fyne.DoAndWait(func() {
			ab := d.abierto
			if ab == nil {
				return
			}
			f := ab.folio
			f.mover(punto{1, f.largo(1)}, false)
			f.escribir(" 👍🏽")
			d.guardarYa(ab)
			actor = d.actor
			// «<h1>Prueba de coedición</h1><p>Acta 👍🏽» son 38 runas (antes de `</p>`).
			puntoTrasEmoji = 38
		})
		pausa()
		if actor == "" {
			fallo <- fmt.Errorf("el documento no llegó a abrirse en ventana real")
			return
		}

		// Lote remoto por el canal en vivo: secuencia ZWJ («👩🏽‍🔬»), bandera («🇪🇸»),
		// vocal con acento combinante («é») y una op maliciosa de varios caracteres.
		remoto := " 👩🏽‍🔬 🇪🇸 e\u0301"
		lote := make([]api.DocOpSecuenciada, 0, len(remoto)+1)
		prevA, prevC := actor, puntoTrasEmoji
		seq := int64(10)
		var c uint64 = 60
		for _, r := range remoto {
			seq++
			c++
			crudo, err := json.Marshal(map[string]any{
				"kind":    "insert",
				"dot":     map[string]any{"a": "web-1", "c": c},
				"after":   map[string]any{"a": prevA, "c": prevC},
				"value":   string(r),
				"lamport": 50 + c,
			})
			if err != nil {
				fallo <- err
				return
			}
			lote = append(lote, api.DocOpSecuenciada{Seq: seq, Actor: "web-1", Op: crudo})
			prevA, prevC = "web-1", c
		}
		seq++
		lote = append(lote, api.DocOpSecuenciada{
			Seq:   seq,
			Actor: "eva",
			Op:    json.RawMessage(`{"kind":"insert","dot":{"a":"eva","c":999},"after":{"a":"` + actor + `","c":36},"value":"TRAMPA","lamport":999}`),
		})

		var htmlFinal string
		fyne.DoAndWait(func() {
			ab := d.abierto
			d.enVivo(ab, api.DocMensaje{Type: "welcome", Seq: 4})
			d.enVivo(ab, api.DocMensaje{Type: "ops", Ops: lote})
			htmlFinal = ab.folio.Serializar()
		})
		pausa()
		esperado := "<h1>Prueba de coedición</h1><p>Acta 👍🏽 👩🏽‍🔬 🇪🇸 e\u0301</p>"
		if htmlFinal != esperado {
			fallo <- fmt.Errorf("fusión en ventana real: %q != %q", htmlFinal, esperado)
			return
		}
		fallo <- captura("2-coedicion-emoji-compuesto")
	}()

	a.Run()
	select {
	case err := <-fallo:
		return err
	default:
		return fmt.Errorf("la ventana se cerró sin acabar el recorrido")
	}
}
