// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// portalFalso hace de xdg-desktop-portal en un bus de sesión propio: contesta a
// PreparePrint con lo que diga `cancela`, y de Print se queda con los bytes que le
// llegan por el descriptor, que es lo que imprimiría la impresora.
type portalFalso struct {
	conn    *dbus.Conn
	cancela bool
	ficha   uint32
	impreso chan []byte
	pedidos int
	fichas  chan uint32
}

func (p *portalFalso) responder(ruta dbus.ObjectPath, codigo uint32, datos map[string]dbus.Variant) {
	// Después de devolver la ruta, como hace el portal de verdad: si el cliente no se ha
	// suscrito antes, la señal se pierde y el test se queda esperando.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = p.conn.Emit(ruta, "org.freedesktop.portal.Request.Response", codigo, datos)
	}()
}

func (p *portalFalso) ruta(remitente dbus.Sender) dbus.ObjectPath {
	p.pedidos++
	quien := strings.ReplaceAll(strings.TrimPrefix(string(remitente), ":"), ".", "_")
	return dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + quien + "/t" + string(rune('0'+p.pedidos)))
}

func (p *portalFalso) PreparePrint(remitente dbus.Sender, _, _ string, _, _, _ map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	r := p.ruta(remitente)
	if p.cancela {
		p.responder(r, 1, map[string]dbus.Variant{})
	} else {
		p.responder(r, 0, map[string]dbus.Variant{"token": dbus.MakeVariant(p.ficha)})
	}
	return r, nil
}

func (p *portalFalso) Print(remitente dbus.Sender, _, _ string, fd dbus.UnixFD, opciones map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	ficha, _ := opciones["token"].Value().(uint32)
	p.fichas <- ficha
	f := os.NewFile(uintptr(fd), "impreso")
	datos, _ := io.ReadAll(f)
	_ = f.Close()
	p.impreso <- datos
	r := p.ruta(remitente)
	p.responder(r, 0, map[string]dbus.Variant{})
	return r, nil
}

// busPropio arranca un dbus-daemon de sesión solo para este test y apunta el proceso a él.
func busPropio(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("sin dbus-daemon no hay bus en el que montar el portal")
	}
	ctx, parar := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "dbus-daemon", "--session", "--nofork", "--nopidfile", "--print-address=1")
	salida, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parar(); _ = cmd.Wait() })
	dir, err := bufio.NewReader(salida).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	dir = strings.TrimSpace(dir)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", dir)
	return dir
}

func montarPortal(t *testing.T, dir string, p *portalFalso) {
	t.Helper()
	conn, err := dbus.Connect(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	p.conn = conn
	if err := conn.Export(p, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.Print"); err != nil {
		t.Fatal(err)
	}
	if r, err := conn.RequestName("org.freedesktop.portal.Desktop", dbus.NameFlagDoNotQueue); err != nil || r != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("no se pudo tomar el nombre del portal: %v %v", r, err)
	}
}

func pdfDePrueba(t *testing.T) (string, []byte) {
	t.Helper()
	datos := []byte("%PDF-1.7\nuna página de prueba\n%%EOF\n")
	ruta := filepath.Join(t.TempDir(), "acta.pdf")
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta, datos
}

// TestImprimirPasaElFicheroAlPortal es el guardián del diálogo de impresión en Linux: el
// PDF tiene que llegar ENTERO por el descriptor, y con la ficha que devolvió el diálogo,
// que es la que dice al portal con qué impresora y qué ajustes imprimirlo. Se ha visto
// en rojo mandando otra ficha (falla la ficha) y abriendo otro fichero (fallan los
// bytes).
func TestImprimirPasaElFicheroAlPortal(t *testing.T) {
	dir := busPropio(t)
	p := &portalFalso{ficha: 4242, impreso: make(chan []byte, 1), fichas: make(chan uint32, 1)}
	montarPortal(t, dir, p)
	ruta, datos := pdfDePrueba(t)

	if err := Imprimir(ruta); err != nil {
		t.Fatalf("Imprimir: %v", err)
	}
	if f := <-p.fichas; f != 4242 {
		t.Errorf("Print llegó con la ficha %d; el diálogo dio 4242", f)
	}
	if got := <-p.impreso; !bytes.Equal(got, datos) {
		t.Errorf("el portal recibió %q; se mandó %q", got, datos)
	}
}

// TestCancelarNoImprimeNiAbreNada: si la persona cierra el diálogo, no hay trabajo de
// impresión y tampoco se le abre el visor como si hubiera fallado algo.
func TestCancelarNoImprimeNiAbreNada(t *testing.T) {
	dir := busPropio(t)
	p := &portalFalso{cancela: true, impreso: make(chan []byte, 1), fichas: make(chan uint32, 1)}
	montarPortal(t, dir, p)
	ruta, _ := pdfDePrueba(t)

	if err := porPortal(ruta); err != nil {
		t.Fatalf("cancelar no es un error: %v", err)
	}
	select {
	case <-p.impreso:
		t.Fatal("se imprimió algo que la persona había cancelado")
	default:
	}
}

// TestSinPortalCaeAlVisor: un bus sin portal es el escritorio sin xdg-desktop-portal, y
// ahí el camino es el de siempre (abrir en el visor), no un error en pantalla.
func TestSinPortalCaeAlVisor(t *testing.T) {
	busPropio(t)
	ruta, _ := pdfDePrueba(t)
	if err := porPortal(ruta); !errors.Is(err, errSinPortal) {
		t.Fatalf("sin portal porPortal debe decir errSinPortal; dijo %v", err)
	}
}
