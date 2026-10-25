// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestDispositivosUsaSistemaYConsentimiento(t *testing.T) {
	d := NuevosDispositivos(castellano(t))
	if !d.DriveEnSistema {
		t.Fatal("Drive debe estar integrado por defecto")
	}
	cambios, aperturas, consentimientos := 0, 0, 0
	d.AlCambiarDrive = func(enabled bool) { cambios++; d.DriveEnSistema = enabled; d.Refresh() }
	d.AlAbrirDrive = func() { aperturas++ }
	d.AlConectarApps = func() { consentimientos++ }
	r := test.WidgetRenderer(d).(*dispositivosRenderer)
	w := test.NewWindow(d)
	defer w.Close()
	for _, ancho := range []float32{360, 390, 1280} {
		w.Resize(fyne.NewSize(ancho, 800))
		d.Resize(fyne.NewSize(ancho, r.alto(ancho)))
		for _, o := range r.Objects() {
			if _, ok := o.(*FilaCopia); ok {
				t.Fatal("la pantalla nativa no debe copiar claves ni direcciones de servidores")
			}
		}
		if r.apps.Inerte != true {
			t.Fatal("la conexión OAuth requiere permisos de la cuenta")
		}
		guardar(t, w.Canvas().Capture(), "dispositivos-nativo-"+fmtAncho(ancho))
	}
	r.drive.Tapped(nil)
	if aperturas != 1 {
		t.Fatal("abrir Drive debe usar el gestor de archivos del sistema")
	}
	r.activo.Tapped(nil)
	if cambios != 1 || d.DriveEnSistema || !r.drive.Inerte {
		t.Fatal("desactivar Drive debe persistir la elección e impedir abrir una conexión retirada")
	}
	r.apps.Tapped(nil)
	if consentimientos != 0 {
		t.Fatal("no se debe iniciar el consentimiento sin permisos")
	}
	d.PuedeConectarApps = true
	d.Refresh()
	r.apps.Tapped(nil)
	if consentimientos != 1 {
		t.Fatal("la conexión debe llegar al asistente OAuth autorizado")
	}
}

func fmtAncho(ancho float32) string {
	switch ancho {
	case 360:
		return "360"
	case 390:
		return "390"
	default:
		return "1280"
	}
}

func TestDispositivosConservaPreferenciaSiElSistemaFalla(t *testing.T) {
	v := NuevaVista(castellano(t), nil, time.Now())
	v.mostrar(ADispositivos)
	cambios := 0
	v.ConDriveEnSistema(func() bool { return true }, func(bool) error { cambios++; return errors.New("no disponible") }, nil)
	r := test.WidgetRenderer(v.disp).(*dispositivosRenderer)
	r.activo.Tapped(nil)
	esperar(t, "guardar la preferencia de Drive", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return !v.disp.DriveOcupado
	})
	if cambios != 1 || !v.disp.DriveEnSistema || !r.activo.Encendido {
		t.Fatalf("un fallo no puede presentar la integración como desactivada: cambios=%d, preferencia=%v, control=%v", cambios, v.disp.DriveEnSistema, r.activo.Encendido)
	}
	v.RefrescarDriveEnSistema(false)
	if r.activo.Encendido || !r.drive.Inerte {
		t.Fatal("la preferencia compartida de otra app debe reflejarse")
	}
}

func TestDispositivosGuardarNoBloqueaLaInterfazNiRepiteElCambio(t *testing.T) {
	v := NuevaVista(castellano(t), nil, time.Now())
	inicio, terminar := make(chan struct{}), make(chan struct{})
	devuelto := make(chan struct{})
	defer func() {
		close(terminar)
		select {
		case <-devuelto:
		case <-time.After(time.Second):
			t.Error("el callback no terminó al liberar el almacén")
		}
		esperar(t, "terminar el guardado de Drive", func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return !v.disp.DriveOcupado
		})
	}()
	v.ConDriveEnSistema(func() bool { return true }, func(bool) error {
		close(inicio)
		<-terminar
		return nil
	}, func() error { return nil })
	// Esta es la devolución del callback de UI, no la del almacén bloqueado.
	go func() { v.disp.AlCambiarDrive(false); close(devuelto) }()
	select {
	case <-devuelto:
	case <-time.After(time.Second):
		t.Fatal("el cambio de preferencia bloqueó el callback de la interfaz")
	}
	select {
	case <-inicio:
	case <-time.After(time.Second):
		t.Fatal("no comenzó el guardado")
	}
	r := test.WidgetRenderer(v.disp).(*dispositivosRenderer)
	if !r.activo.Disabled() || !r.drive.Inerte {
		t.Fatal("la preferencia y la apertura deben esperar al guardado en curso")
	}
	// Si iniciara una segunda escritura, el doble cerraría inicio dos veces.
	v.disp.AlCambiarDrive(true)
}
