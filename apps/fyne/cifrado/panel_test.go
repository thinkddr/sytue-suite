// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

func panelEn(t *testing.T, width float32) (*Panel, fyne.Window) {
	t.Helper()
	app := test.NewApp()
	app.Settings().SetTheme(tema.Sytue{})
	w := app.NewWindow("Private device")
	w.SetContent(container.NewWithoutLayout())
	w.Resize(fyne.NewSize(width, 640))
	t.Cleanup(w.Close)
	t.Cleanup(app.Quit)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Rooms())
	if err != nil {
		t.Fatal(err)
	}
	c := &Panel{Lienzo: w.Canvas, Traducir: func(key string) string { return tr.T(key, nil) }, Copiar: func(string) {}}
	return c, w
}

func capturarPanel(t *testing.T, w fyne.Window, name string) {
	t.Helper()
	dir := os.Getenv("SYTUE_PRIVATE_PANEL_CAPTURES")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

// textos recoge lo que el panel enseña (párrafos, botones y entradas).
func textos(c *Panel) []string {
	var out []string
	cuerpo, pie := c.Objetos()
	for _, o := range append(append([]fyne.CanvasObject{}, cuerpo...), pie...) {
		switch v := o.(type) {
		case *ui.Boton:
			out = append(out, v.Texto)
		case *ui.Entrada:
			out = append(out, v.Texto())
		case *ui.Parrafo:
			out = append(out, v.Texto)
		}
	}
	return out
}

func entradas(c *Panel) []*ui.Entrada {
	var out []*ui.Entrada
	cuerpo, _ := c.Objetos()
	for _, o := range cuerpo {
		if e, ok := o.(*ui.Entrada); ok {
			out = append(out, e)
		}
	}
	return out
}

func boton(t *testing.T, c *Panel, key string) *ui.Boton {
	t.Helper()
	_, pie := c.Objetos()
	for _, o := range pie {
		if b, ok := o.(*ui.Boton); ok && b.Texto == c.t(key) {
			return b
		}
	}
	t.Fatalf("falta el botón %q", key)
	return nil
}

func esperarLibre(t *testing.T, c *Panel) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		busy := true
		fyne.DoAndWait(func() { busy = c.busy })
		if !busy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("la operación no terminó")
}

// ADR 0084: ningún estado del panel enseña, pide ni deja copiar la clave de
// recuperación, y ninguno pide una contraseña: sin clave de desbloqueo solo ofrece
// volver a entrar en Sytue ID; una clave desconocida, solo verificar con otro
// dispositivo.
func TestElPanelNuncaEnsenaNiPideNingunaClave(t *testing.T) {
	c, w := panelEn(t, 320)
	c.ctx = t.Context()
	c.client = &mx.Client{} // Solo pinta: ninguna operación llega al SDK.
	defer func() { c.client = nil; c.Cerrar() }()
	c.AbrirPanel()
	for _, tc := range []struct {
		name   string
		paso   Paso
		status mx.VerificationStatus
		busy   bool
		texto  string
	}{
		{"recovery-unlocking-mobile", PasoDesbloquear, mx.VerificationStatus{HasIdentity: true}, true, "matrix.unlocking"},
		{"recovery-signin-mobile", PasoIniciarSesion, mx.VerificationStatus{HasIdentity: true}, false, "matrix.signInBody"},
		{"recovery-unknown-mobile", PasoVerificar, mx.VerificationStatus{HasIdentity: true}, false, "matrix.unknownKey"},
		{"recovery-retry-mobile", PasoCrear, mx.VerificationStatus{}, false, "matrix.failed"},
	} {
		c.diag = Diagnostico{Paso: tc.paso, Status: tc.status, clave: "ARTIFICIAL RECOVERY KEY", claveID: "k"}
		c.status, c.busy = tc.status, tc.busy
		c.pintar()
		capturarPanel(t, w, tc.name)
		found := false
		for _, text := range textos(c) {
			if text == "ARTIFICIAL RECOVERY KEY" {
				t.Fatalf("%s: el panel enseña la clave", tc.name)
			}
			found = found || text == c.t(tc.texto)
		}
		if !found {
			t.Fatalf("%s: falta %q en %q", tc.name, tc.texto, textos(c))
		}
		for _, key := range []string{"matrix.recover", "matrix.recoveryKey", "matrix.saved", "action.copy", "matrix.usePassword"} {
			for _, text := range textos(c) {
				if text == c.t(key) {
					t.Fatalf("%s: el panel ofrece %q", tc.name, key)
				}
			}
		}
		if got := len(entradas(c)); got != 0 {
			t.Fatalf("%s: %d entradas; aquí no se teclea nada", tc.name, got)
		}
		if c.dialog.NoDescartable {
			t.Fatalf("%s: el panel no se puede cerrar", tc.name)
		}
	}
	c.busy = false
	c.diag, c.status = Diagnostico{Paso: PasoIniciarSesion}, mx.VerificationStatus{HasIdentity: true}
	c.pintar()
	boton(t, c, "matrix.signIn")
	// La clave desconocida lo explica y solo ofrece verificar (y, en Rooms, restablecer).
	c.diag, c.status = Diagnostico{Paso: PasoVerificar}, mx.VerificationStatus{HasIdentity: true}
	c.pintar()
	if textos(c)[1] != c.t("matrix.unknownKey") {
		t.Fatalf("falta la explicación: %q", textos(c))
	}
	boton(t, c, "matrix.verify")
	boton(t, c, "matrix.reset")
	// Restablecer: casilla obligatoria y ninguna contraseña.
	c.Restablecer()
	if got := len(entradas(c)); got != 0 {
		t.Fatalf("restablecer pide %d entradas", got)
	}
	if !boton(t, c, "matrix.resetAction").Inerte {
		t.Fatal("restablecer sin marcar la casilla")
	}
	// En una cuenta de almacenamiento restablecer no existe (ErrResetUnsupported).
	c.Base.Bootstrap.StorageProduct = "drive"
	c.pintar()
	for _, text := range textos(c) {
		if text == c.t("matrix.reset") {
			t.Fatal("un producto de almacenamiento ofrece restablecer")
		}
	}
}

// Sin la clave de desbloqueo, el único botón vuelve a Sytue ID (con los parámetros
// de los envoltorios de la ADR 0082) y, al volver, el panel revisa otra vez solo.
func TestIniciarSesionVuelveASytueIDYRevisaSolo(t *testing.T) {
	c, _ := panelEn(t, 320)
	c.ctx = t.Context()
	c.client = &mx.Client{}
	defer func() { c.client = nil; c.Cerrar() }()
	pedidas := make(chan string, 1)
	c.IniciarSesion = func(_ context.Context, legado string) error {
		pedidas <- legado
		return errors.New("artificial: el navegador no volvió")
	}
	c.diag = Diagnostico{Paso: PasoIniciarSesion, Legado: "p.EBESExQVFhcYGRobHB0eHw"}
	c.status = mx.VerificationStatus{HasIdentity: true}
	c.AbrirPanel()
	boton(t, c, "matrix.signIn").AlPulsar()
	select {
	case got := <-pedidas:
		if got != "p.EBESExQVFhcYGRobHB0eHw" {
			t.Fatalf("legado = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no se volvió a Sytue ID")
	}
	esperarLibre(t, c)
	// Si no volvió, sigue ofreciendo lo mismo y no se abre nada.
	boton(t, c, "matrix.signIn")
	if c.Verificado() {
		t.Fatal("se abrió sin clave")
	}
}

// Si la custodia falla, el panel sigue bloqueado y en el mismo paso (se puede
// reintentar) y los datos no se abren; sin clave de desbloqueo, pide volver a entrar.
func TestUnaCustodiaFallidaDejaElPanelBloqueadoYReintentable(t *testing.T) {
	c, _ := panelEn(t, 320)
	c.ctx = t.Context()
	c.client = &mx.Client{}
	defer func() { c.client = nil; c.Cerrar() }()
	ready := 0
	c.AlLista = func(context.Context) { ready++ }
	c.diag, c.status = Diagnostico{Paso: PasoCrear}, mx.VerificationStatus{}
	c.AbrirPanel()
	var used string
	c.custodiar(func(_ context.Context, _ *mx.Client, _ Custodia) (Diagnostico, bool, error) {
		used = "llamado"
		return Diagnostico{}, false, errors.New("artificial 503")
	})
	esperarLibre(t, c)
	if used == "" || c.diag.Paso != PasoCrear || c.status.Verified || ready != 0 || c.Verificado() {
		t.Fatalf("tras el fallo: paso=%v verificado=%v ready=%d", c.diag.Paso, c.status.Verified, ready)
	}
	boton(t, c, "action.retry") // se puede reintentar
	c.custodiar(func(context.Context, *mx.Client, Custodia) (Diagnostico, bool, error) {
		return Diagnostico{}, false, ErrSinDesbloqueo
	})
	esperarLibre(t, c)
	if c.diag.Paso != PasoIniciarSesion || ready != 0 {
		t.Fatalf("sin clave de desbloqueo: paso=%v", c.diag.Paso)
	}
	boton(t, c, "matrix.signIn")
}

func TestClosingThePanelForgetsTheDiagnosis(t *testing.T) {
	c, _ := panelEn(t, 320)
	c.ctx, c.cancel = context.WithCancel(t.Context())
	ctx := c.ctx
	c.client = &mx.Client{}
	c.diag, c.status = Diagnostico{Paso: PasoProteger, clave: "ARTIFICIAL KEY", claveID: "k"}, mx.VerificationStatus{HasIdentity: true}
	c.AbrirPanel()
	c.verifyUser = "@old-peer:matrix.test"
	c.authWait = make(chan bool, 1)
	c.client = nil
	c.Cerrar()
	if ctx.Err() == nil || c.diag.clave != "" || c.verifyUser != "" || c.authWait != nil || c.dialog != nil || c.diag.Paso != PasoListo {
		t.Fatal("closed panel retained device state or verification peer")
	}
}

func TestPrivatePanelOffersOnlyOwnerVerification(t *testing.T) {
	c, w := panelEn(t, 320)
	c.ctx = t.Context()
	c.status = mx.VerificationStatus{Verified: true}
	c.AbrirPanel()
	capturarPanel(t, w, "owner-devices-mobile")
	defer c.Cerrar()
	for _, obj := range c.footer.Objects {
		if b, ok := obj.(*ui.Boton); ok && b.Texto == c.t("matrix.verifyPerson") {
			t.Fatal("private storage offered peer verification")
		}
		position := fyne.CurrentApp().Driver().AbsolutePositionForObject(obj)
		if position.Y < 0 || position.Y+obj.Size().Height > w.Canvas().Size().Height {
			t.Fatalf("private device action is clipped: y=%v height=%v canvas=%v", position.Y, obj.Size().Height, w.Canvas().Size())
		}
	}
}

func TestPrivateApplicationCleanupHoldsTheDeviceCloseBarrier(t *testing.T) {
	c, _ := panelEn(t, 320)
	ctx, cancel := context.WithCancel(t.Context())
	c.ctx, c.cancel = ctx, cancel
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	detached := false
	c.AlCerrar = func() func() {
		detached = true
		if ctx.Err() == nil {
			t.Error("application detached before its device context was cancelled")
		}
		return func() { <-release }
	}
	c.Cerrar()
	if !detached {
		t.Fatal("application was not detached on UI close")
	}
	done := make(chan error, 1)
	go func() { done <- c.esperarCierre(t.Context()) }()
	select {
	case <-done:
		t.Fatal("next SDK open could overtake private application cleanup")
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
