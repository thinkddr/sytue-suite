// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// selectorFalso hace de org.freedesktop.portal.FileChooser: apunta las opciones que le
// llegan y contesta con las URI que se le digan (o cancela).
type selectorFalso struct {
	conn     *dbus.Conn
	cancela  bool
	uris     []string
	opciones chan map[string]dbus.Variant
	// muere: en vez de contestar, el portal suelta su nombre con el diálogo abierto.
	muere bool
}

func (s *selectorFalso) OpenFile(remitente dbus.Sender, _, _ string, opciones map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	s.opciones <- opciones
	p := &portalFalso{conn: s.conn}
	r := p.ruta(remitente)
	if s.muere {
		go func() {
			time.Sleep(20 * time.Millisecond)
			_, _ = s.conn.ReleaseName("org.freedesktop.portal.Desktop")
		}()
		return r, nil
	}
	if s.cancela {
		p.responder(r, 1, map[string]dbus.Variant{})
	} else {
		p.responder(r, 0, map[string]dbus.Variant{"uris": dbus.MakeVariant(s.uris)})
	}
	return r, nil
}

func montarSelector(t *testing.T, s *selectorFalso) {
	t.Helper()
	dir := busPropio(t)
	conn, err := dbus.Connect(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	s.conn = conn
	if err := conn.Export(s, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.FileChooser"); err != nil {
		t.Fatal(err)
	}
	if r, err := conn.RequestName("org.freedesktop.portal.Desktop", dbus.NameFlagDoNotQueue); err != nil || r != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("no se pudo tomar el nombre del portal: %v %v", r, err)
	}
	sinProgramas(t)
}

// sinProgramas deja el PATH sin zenity ni kdialog (después de arrancar el bus, que
// necesita encontrar dbus-daemon).
func sinProgramas(t *testing.T) { t.Setenv("PATH", t.TempDir()) }

// TestSinZenityNiKdialogEligePorElPortal: en un escritorio sin ninguno de los dos
// programas, ElegirConTipos ya no se rinde con ErrNoDisponible sino que pide el selector
// al portal, con selección múltiple y el filtro de lo que se pide, y devuelve rutas
// locales (no las URI, y nada que no sea un fichero local).
func TestSinZenityNiKdialogEligePorElPortal(t *testing.T) {
	s := &selectorFalso{
		uris:     []string{"file:///home/ana/Im%C3%A1genes/playa.jpg", "file:///home/ana/b.png", "https://example.org/x.png"},
		opciones: make(chan map[string]dbus.Variant, 1),
	}
	montarSelector(t, s)

	rutas, err := ElegirConTipos("Fotos", "jpg", "png")
	if err != nil {
		t.Fatalf("ElegirConTipos: %v", err)
	}
	if want := []string{"/home/ana/Imágenes/playa.jpg", "/home/ana/b.png"}; !slices.Equal(rutas, want) {
		t.Errorf("rutas = %q; se esperaba %q", rutas, want)
	}
	op := <-s.opciones
	if m, _ := op["multiple"].Value().(bool); !m {
		t.Error("el portal no recibió multiple=true")
	}
	var filtros []filtroPortal
	if err := dbus.Store([]any{op["filters"].Value()}, &filtros); err != nil || len(filtros) != 1 {
		t.Fatalf("filtros = %v (%v)", op["filters"], err)
	}
	patrones := make([]string, 0, len(filtros[0].Patrones))
	for _, p := range filtros[0].Patrones {
		patrones = append(patrones, p.Patron)
	}
	if want := []string{"*.jpg", "*.JPG", "*.png", "*.PNG"}; filtros[0].Nombre != "Fotos" || !slices.Equal(patrones, want) {
		t.Errorf("filtro = %q %q", filtros[0].Nombre, patrones)
	}
}

// TestCancelarEnElPortalNoEsUnError: cerrar el diálogo es nada elegido, sin error.
func TestCancelarEnElPortalNoEsUnError(t *testing.T) {
	s := &selectorFalso{cancela: true, opciones: make(chan map[string]dbus.Variant, 1)}
	montarSelector(t, s)
	rutas, err := Elegir()
	if err != nil || rutas != nil {
		t.Fatalf("Elegir = %q, %v; cancelar es nil, nil", rutas, err)
	}
	if _, hay := (<-s.opciones)["filters"]; hay {
		t.Error("Elegir sin tipos no debe filtrar")
	}
}

// TestSinPortalNiProgramasNoSeOfrece: sin zenity, kdialog ni portal sigue siendo
// ErrNoDisponible, que la app trata como «no se ofrece».
func TestSinPortalNiProgramasNoSeOfrece(t *testing.T) {
	busPropio(t)
	sinProgramas(t)
	if _, err := ElegirConTipos("Fotos", "jpg"); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("sin nada debe ser ErrNoDisponible; fue %v", err)
	}
}

// TestSiElPortalMuereLaEsperaTermina: el selector espera sin plazo, como zenity, así que
// un portal que se cae con el diálogo abierto (sin mandar Response) tiene que cortar la
// espera con un error y no dejar la rutina colgada para siempre.
func TestSiElPortalMuereLaEsperaTermina(t *testing.T) {
	s := &selectorFalso{muere: true, opciones: make(chan map[string]dbus.Variant, 1)}
	montarSelector(t, s)
	hecho := make(chan error, 1)
	go func() {
		_, err := ElegirConTipos("Fotos", "jpg")
		hecho <- err
	}()
	select {
	case err := <-hecho:
		if err == nil || errors.Is(err, ErrNoDisponible) {
			t.Fatalf("un portal caído es un error del diálogo; fue %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ElegirConTipos se quedó esperando a un portal que ya no está")
	}
}

// TestFiltroSinTituloLlevaNombre: el desplegable de filtros del diálogo no enseña una
// entrada en blanco cuando quien llama no da título.
func TestFiltroSinTituloLlevaNombre(t *testing.T) {
	f := filtrosPortal("", []string{"xfdf", "xml"})
	if len(f) != 1 || f[0].Nombre != "*.xfdf *.xml" {
		t.Fatalf("filtro = %+v", f)
	}
	if f := filtrosPortal("Datos", []string{"xml"}); f[0].Nombre != "Datos" {
		t.Fatalf("con título el nombre es el título; fue %q", f[0].Nombre)
	}
}

// TestElegirCarpetaPorElPortal: sin zenity ni kdialog, la carpeta se pide al portal con
// `directory` y sin selección múltiple; sin portal, ErrNoDisponible.
func TestElegirCarpetaPorElPortal(t *testing.T) {
	s := &selectorFalso{uris: []string{"file:///home/ana/Proyecto"}, opciones: make(chan map[string]dbus.Variant, 1)}
	montarSelector(t, s)
	ruta, err := ElegirCarpeta("Subir carpeta")
	if err != nil || ruta != "/home/ana/Proyecto" {
		t.Fatalf("ElegirCarpeta = %q, %v", ruta, err)
	}
	op := <-s.opciones
	if d, _ := op["directory"].Value().(bool); !d {
		t.Error("el portal no recibió directory=true")
	}
	if _, hay := op["multiple"]; hay {
		t.Error("una carpeta no se elige con selección múltiple")
	}
}

// TestSinNadaElegirCarpetaNoSeOfrece: sin programas ni portal, ErrNoDisponible.
func TestSinNadaElegirCarpetaNoSeOfrece(t *testing.T) {
	busPropio(t)
	sinProgramas(t)
	if _, err := ElegirCarpeta("Subir carpeta"); !errors.Is(err, ErrNoDisponible) {
		t.Fatalf("sin nada debe ser ErrNoDisponible; fue %v", err)
	}
}
