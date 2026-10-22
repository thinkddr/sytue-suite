//go:build ventanareal

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
)

func init() {
	runtime.LockOSThread()
	enVentanaReal = ventanaRealNAT08
}

// ventanaRealNAT08 abre la barra de la suite en una ventana DE VERDAD bajo Xvfb con una
// sesión activa y un borrador de Notepads sin subir, abre el menú de cuenta con xdotool,
// pulsa «Cerrar sesión», captura el diálogo de aviso (NAT-08), pulsa «Subir y cerrar
// sesión», captura el estado ocupado (con «Descartar y cerrar sesión» apagado) y verifica
// que tras subir se cierra la sesión y el disco queda limpio.
func ventanaRealNAT08(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	casa, err := os.MkdirTemp("", "vr-nat08-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(casa) }()
	_ = os.Setenv("HOME", casa)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	_ = os.Setenv("XDG_CACHE_HOME", filepath.Join(casa, ".cache"))
	keyring.MockInit()

	llegada := make(chan struct{})
	soltar := make(chan struct{})
	var subidas atomic.Int32
	prev := api.Transporte
	defer func() { api.Transporte = prev }()
	api.Transporte = transporteFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/notebooks/nb-1/notes") {
			if subidas.Add(1) == 1 {
				close(llegada)
			}
			<-soltar
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"n-1"}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
		}, nil
	})

	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		return err
	}
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})
	suite := sesion.Abrir(a, "sytue.eu")
	suite.Sesion.Adoptar(auth.Tokens{
		Access: "tok", Refresh: "r", Subject: "ada",
		Expiry: time.Now().Add(time.Hour),
	})
	alm, err := sesion.Almacen()
	if err != nil {
		return err
	}
	if err := alm.Write("ada/", notas.EspacioBorradores, "nb-1", "Borrador pendiente en ventana real"); err != nil {
		return err
	}

	b := NuevaBarraSuite(tr, suite, iconos.Notes)
	defer b.Parar()
	fondo := fcanvas.NewRectangle(tema.Color("bg", tema.Claro))
	w := a.NewWindow("Sytue Notepads — NAT-08")
	w.SetPadded(false)
	w.SetContent(container.NewBorder(b, nil, nil, nil, fondo))
	w.Resize(fyne.NewSize(1280, 800))
	w.Show()

	captura := func(nombre string) error {
		if out, err := exec.Command("import", "-window", "root", filepath.Join(dir, nombre+".png")).CombinedOutput(); err != nil {
			return fmt.Errorf("import %s: %w %s", nombre, err, out)
		}
		return nil
	}
	clicEn := func(o fyne.CanvasObject) error {
		var x, y, ancho, alto float32
		fyne.DoAndWait(func() {
			pos := a.Driver().AbsolutePositionForObject(o)
			x, y, ancho, alto = pos.X, pos.Y, o.Size().Width, o.Size().Height
		})
		if ancho == 0 || alto == 0 {
			return fmt.Errorf("objeto sin tamaño (%v×%v)", ancho, alto)
		}
		cx, cy := fmt.Sprint(int(x+ancho/2)), fmt.Sprint(int(y+alto/2))
		if out, err := exec.Command("xdotool", "mousemove", cx, cy, "click", "1").CombinedOutput(); err != nil {
			return fmt.Errorf("xdotool (%s,%s): %w %s", cx, cy, err, out)
		}
		return nil
	}

	fallo := make(chan error, 1)
	go func() {
		defer fyne.Do(a.Quit)
		pausa := func() { time.Sleep(1500 * time.Millisecond) }
		pausa()
		if err := clicEn(b.cuenta.boton); err != nil {
			fallo <- fmt.Errorf("abrir menú de cuenta: %w", err)
			return
		}
		pausa()
		if err := captura("1-menu-cuenta"); err != nil {
			fallo <- err
			return
		}
		var itemSalir *ElementoMenu
		fyne.DoAndWait(func() {
			if cp, ok := w.Canvas().Overlays().Top().(*capa); ok {
				if m, ok := cp.panel.(*Menu); ok {
					for _, o := range m.Elementos {
						if em, ok := o.(*ElementoMenu); ok && em.Texto == tr.T("account.signOut", nil) {
							itemSalir = em
						}
					}
				}
			}
		})
		if itemSalir == nil {
			fallo <- fmt.Errorf("no se encontró «Cerrar sesión» en el menú")
			return
		}
		if err := clicEn(itemSalir); err != nil {
			fallo <- fmt.Errorf("pulsar cerrar sesión: %w", err)
			return
		}
		pausa()
		if err := captura("2-dialogo-nat08"); err != nil {
			fallo <- err
			return
		}
		var descartar, subir *Boton
		fyne.DoAndWait(func() {
			if cp, ok := w.Canvas().Overlays().Top().(*capa); ok {
				if d, ok := cp.panel.(*Dialogo); ok {
					for _, o := range d.Pie.(*fyne.Container).Objects {
						if bt, ok := o.(*Boton); ok {
							switch bt.Texto {
							case tr.T("account.signOutPending.discard", nil):
								descartar = bt
							case tr.T("account.signOutPending.upload", nil):
								subir = bt
							}
						}
					}
				}
			}
		})
		if descartar == nil || subir == nil {
			fallo <- fmt.Errorf("el diálogo NAT-08 no tiene los botones de descartar y subir")
			return
		}
		if err := clicEn(subir); err != nil {
			fallo <- fmt.Errorf("pulsar subir: %w", err)
			return
		}
		select {
		case <-llegada:
		case <-time.After(5 * time.Second):
			fallo <- fmt.Errorf("no empezó la subida al pulsar «Subir y cerrar sesión»")
			return
		}
		time.Sleep(300 * time.Millisecond)
		var descartarInerte, subirCargando bool
		fyne.DoAndWait(func() {
			descartarInerte = descartar.Inerte
			subirCargando = subir.Cargando
		})
		if !descartarInerte || !subirCargando {
			fallo <- fmt.Errorf("mientras sube: descartar.Inerte=%v subir.Cargando=%v", descartarInerte, subirCargando)
			return
		}
		if err := captura("3-subiendo-ocupado"); err != nil {
			fallo <- err
			return
		}
		close(soltar)
		pausa()
		if suite.Estado().Dentro() {
			fallo <- fmt.Errorf("la sesión sigue abierta tras subir")
			return
		}
		if n := sesion.PendientesNotepads(); n != 0 {
			fallo <- fmt.Errorf("quedan %d pendientes en disco tras salir", n)
			return
		}
		fallo <- captura("4-sesion-cerrada")
	}()
	a.Run()
	select {
	case err := <-fallo:
		return err
	default:
		return fmt.Errorf("la ventana se cerró sin terminar el recorrido")
	}
}
