// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package camara

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// portalFalso hace de xdg-desktop-portal: contesta cada método por Request.Response en
// la ruta que el cliente tiene que haber calculado, como el de verdad.
type portalFalso struct {
	con      *dbus.Conn
	cancelar string // método en el que «la persona» cancela el diálogo
	mu       sync.Mutex
	pasos    []string // los apunta el hilo de godbus que atiende cada llamada
}

func (p *portalFalso) paso(m string) {
	p.mu.Lock()
	p.pasos = append(p.pasos, m)
	p.mu.Unlock()
}

func (p *portalFalso) responder(metodo string, opts, res map[string]dbus.Variant, remitente dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	p.paso(metodo)
	tok, _ := opts["handle_token"].Value().(string)
	ruta := dbus.ObjectPath(string(portalRuta) + "/request/" +
		strings.ReplaceAll(strings.TrimPrefix(string(remitente), ":"), ".", "_") + "/" + tok)
	codigo := uint32(0)
	if metodo == p.cancelar {
		codigo = 1
	}
	if res == nil {
		res = map[string]dbus.Variant{}
	}
	go func() { _ = p.con.Emit(ruta, "org.freedesktop.portal.Request.Response", codigo, res) }()
	return ruta, nil
}

func (p *portalFalso) CreateSession(opts map[string]dbus.Variant, s dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	return p.responder("CreateSession", opts, map[string]dbus.Variant{
		"session_handle": dbus.MakeVariant("/org/freedesktop/portal/desktop/session/1_1/t"),
	}, s)
}

func (p *portalFalso) SelectSources(_ dbus.ObjectPath, opts map[string]dbus.Variant, s dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	if t, _ := opts["types"].Value().(uint32); t != 1 {
		return "", dbus.MakeFailedError(errors.New("types"))
	}
	return p.responder("SelectSources", opts, nil, s)
}

func (p *portalFalso) Start(_ dbus.ObjectPath, _ string, opts map[string]dbus.Variant, s dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	type flujo struct {
		Nodo  uint32
		Props map[string]dbus.Variant
	}
	return p.responder("Start", opts, map[string]dbus.Variant{
		"streams": dbus.MakeVariant([]flujo{{42, map[string]dbus.Variant{}}}),
	}, s)
}

func (p *portalFalso) OpenPipeWireRemote(_ dbus.ObjectPath, _ map[string]dbus.Variant) (dbus.UnixFD, *dbus.Error) {
	p.paso("OpenPipeWireRemote")
	f, err := os.Open(os.DevNull)
	if err != nil {
		return 0, dbus.MakeFailedError(err)
	}
	return dbus.UnixFD(f.Fd()), nil
}

// busPrivado arranca un dbus-daemon propio para la prueba: el de la máquina no se toca.
func busPrivado(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("sin dbus-daemon")
	}
	sock := filepath.Join(t.TempDir(), "bus")
	d := exec.Command("dbus-daemon", "--session", "--nofork", "--address=unix:path="+sock)
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Process.Kill(); _ = d.Wait() })
	for range 100 {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond) // a que el bus escuche
	}
	return "unix:path=" + sock
}

func conectar(t *testing.T, dir string) *dbus.Conn {
	t.Helper()
	c, err := dbus.Connect(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestElPortalDeWayland(t *testing.T) {
	espera := EsperaPortal
	EsperaPortal = 3 * time.Second // una respuesta que no llega no cuelga la prueba
	t.Cleanup(func() { EsperaPortal = espera })
	dir := busPrivado(t)
	for _, caso := range []struct{ cancelar, falla string }{{"", ""}, {"Start", "ErrSinPermiso"}} {
		srv := conectar(t, dir)
		p := &portalFalso{con: srv, cancelar: caso.cancelar}
		if err := srv.Export(p, portalRuta, portalCaptura); err != nil {
			t.Fatal(err)
		}
		if _, err := prop.Export(srv, portalRuta, prop.Map{portalCaptura: {
			"AvailableSourceTypes": {Value: uint32(3)},
			"version":              {Value: uint32(4)},
		}}); err != nil {
			t.Fatal(err)
		}
		if r, err := srv.RequestName(portalNombre, dbus.NameFlagReplaceExisting|dbus.NameFlagAllowReplacement); err != nil ||
			r != dbus.RequestNameReplyPrimaryOwner {
			t.Fatal(r, err)
		}
		cli := conectar(t, dir)
		s, err := pedirPortal(cli)
		if caso.falla != "" {
			if !errors.Is(err, ErrSinPermiso) {
				t.Fatalf("cancelar en %s: %v, se esperaba ErrSinPermiso", caso.cancelar, err)
			}
			_ = srv.Close()
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if s.nodo != 42 || s.fd < 0 || s.sesion == "" {
			t.Fatalf("sesión %+v", s)
		}
		want := "CreateSession SelectSources Start OpenPipeWireRemote"
		p.mu.Lock()
		got := strings.Join(p.pasos, " ")
		p.mu.Unlock()
		if got != want {
			t.Fatalf("pasos %q, se esperaba %q", got, want)
		}
		s.Cerrar()
		_ = srv.Close()
	}
}

// Sin portal en el bus, la app lo dice (ErrSinPantalla) en vez de quedarse esperando.
func TestSinPortalNoSeEspera(t *testing.T) {
	cli := conectar(t, busPrivado(t))
	if _, err := pedirPortal(cli); !errors.Is(err, ErrSinPantalla) {
		t.Fatalf("%v, se esperaba ErrSinPantalla", err)
	}
}

// El vídeo por PipeWire de verdad: un demonio propio, una fuente de GStreamer en BGRx y
// el enlace hecho a mano con pw-link (sin gestor de sesión nadie enlaza). Solo corre
// donde están las tres herramientas; en el CI el portal se prueba con el falso de arriba.
func TestPipeWireDeVerdad(t *testing.T) {
	for _, h := range []string{"pipewire", "gst-launch-1.0", "pw-link", "pw-cli"} {
		if _, err := exec.LookPath(h); err != nil {
			t.Skip("sin " + h)
		}
	}
	rt := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", rt)
	arrancar := func(nombre string, args ...string) {
		c := exec.Command(nombre, args...)
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	}
	arrancar("pipewire")
	time.Sleep(500 * time.Millisecond)
	arrancar("gst-launch-1.0", "videotestsrc", "pattern=red", "!",
		"video/x-raw,format=BGRx,width=64,height=48,framerate=10/1", "!",
		"pipewiresink", "mode=provide", "stream-properties=p,node.name=prueba-sytue")
	var nodo uint32
	for i := 0; i < 50 && nodo == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		out, _ := exec.Command("pw-cli", "ls", "Node").Output()
		var id uint32
		for _, l := range strings.Split(string(out), "\n") {
			if _, err := fmt.Sscanf(strings.TrimSpace(l), "id %d,", &id); err == nil {
				continue
			}
			if strings.Contains(l, `node.name = "prueba-sytue"`) {
				nodo = id
			}
		}
	}
	if nodo == 0 {
		t.Fatal("la fuente de GStreamer no apareció en PipeWire")
	}
	f, err := abrirPipeWire(-1, nodo)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Cerrar()
	for i := 0; i < 50; i++ {
		_ = exec.Command("pw-link", "prueba-sytue:out_0", "sytue-rooms-pantalla:in_0").Run() // hasta que existan los puertos
		if img, ok := f.Leer(); ok {
			if img.Rect.Dx() != 64 || img.Rect.Dy() != 48 {
				t.Fatalf("tamaño %v", img.Rect)
			}
			// Rojo puro en BGRx: si se leyera como RGBx saldría azul (Cr bajo).
			if cr := img.Cr[0]; cr < 200 {
				t.Fatalf("Cr=%d: el orden de los canales está al revés", cr)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	enlaces, _ := exec.Command("pw-link", "-lio").CombinedOutput()
	nodos, _ := exec.Command("pw-cli", "ls", "Node").CombinedOutput()
	t.Fatalf("no llegó ningún fotograma\n%s\n%s", enlaces, nodos)
}
