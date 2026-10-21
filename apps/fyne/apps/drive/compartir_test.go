// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioCompartir hace de Drive y de Sytue ID a la vez, y apunta las escrituras.
type servicioCompartir struct {
	mu         sync.Mutex
	escrituras []string
}

func (s *servicioCompartir) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		ruta := r.Method + " " + r.URL.Path
		switch ruta {
		case "GET /api/v1/files/f1/shares":
			_, _ = io.WriteString(w, `{"shares":[{"id":"s1","kind":"link","permission":"read","has_password":true},{"id":"s2","kind":"user","permission":"write","subject":"sub-ana"}]}`)
			return
		case "POST /api/v1/directory:resolve":
			if strings.Contains(string(cuerpo), "nadie") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, `{"user_id":"sub-ana","email":"ana@acme.test","name":"Ana"}`)
			return
		case "GET /api/v1/files/f1/versions":
			_, _ = io.WriteString(w, `{"versions":[{"version":3,"size_bytes":9,"is_current":true},{"version":2,"size_bytes":7,"author_sub":"ana"}]}`)
			return
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
			return
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
			return
		}
		s.mu.Lock()
		s.escrituras = append(s.escrituras, strings.TrimSpace(ruta+" "+string(cuerpo)))
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"share":{"id":"s9","kind":"link"},"url":"https://drive.example/public/x"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pantallaDePrueba monta la pantalla en una ventana de pruebas, con el trabajo «aparte»
// hecho en el sitio para que el guardián no compita con otra rutina.
func pantallaDePrueba(t *testing.T, srv *httptest.Server) *Pantalla {
	t.Helper()
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	p.Directorio = api.NewDirectorio(srv.URL, sesionDoble{}, nil)
	p.aparte = func(f func()) { f() }
	w := test.NewWindow(container.NewStack())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	p.win = w
	return p
}

// TestCompartirConUnaPersonaYConUnEnlace: la lista dice CON QUIÉN está compartido (el
// nombre, que el directorio resuelve a partir del `sub`), un correo de fuera se queda
// escrito junto al campo, y un enlace sale con su caducidad al FINAL del día elegido.
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando el `case codigo(err) ==
// http.StatusNotFound` de `invitarPersona`, un correo de fuera no deja nada escrito.
func TestCompartirConUnaPersonaYConUnEnlace(t *testing.T) {
	s := &servicioCompartir{}
	p := pantallaDePrueba(t, s.servidor(t))
	c := p.pedirCompartir(Elemento{ID: "f1", Nombre: "acta.odt"})
	if c == nil || len(c.comparticiones) != 2 || c.personas["sub-ana"].Nombre != "Ana" {
		t.Fatalf("la lista no llega o no resuelve a la persona: %+v %+v", c.comparticiones, c.personas)
	}

	c.correo.PonerTexto("nadie@fuera.test")
	c.invitarPersona()
	if c.errorPersona != p.tr.T("share.person.unknown", nil) {
		t.Errorf("un correo de fuera deja %q junto al campo", c.errorPersona)
	}

	c.correo.PonerTexto("ana@acme.test")
	c.accesoPersona.Elegida = 1
	c.invitarPersona()

	c.caduca.PonerTexto("01/10/2026")
	c.clave.PonerTexto("secreta")
	c.crearEnlace()
	if c.creado != "https://drive.example/public/x" {
		t.Errorf("el enlace creado no se enseña: %q", c.creado)
	}
	fin := time.Date(2026, 10, 1, 23, 59, 59, 0, time.Local).UTC().Format(time.RFC3339)
	quiero := []string{
		`POST /api/v1/shares {"file_id":"f1","kind":"user","permission":"write","subject":"sub-ana"}`,
		`POST /api/v1/shares {"expires_at":"` + fin + `","file_id":"f1","kind":"link","password":"secreta","permission":"read"}`,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Equal(s.escrituras, quiero) {
		t.Errorf("al servicio ha llegado\n%s\ny se esperaba\n%s", strings.Join(s.escrituras, "\n"), strings.Join(quiero, "\n"))
	}
	p.win.Canvas().Capture()
}

// TestLaEtiquetaQueBloqueaSeDiceAlAbrir: la web dice que no se puede compartir fuera AL
// ABRIR el diálogo, y apaga «Crear enlace»; no se descubre al fallar.
func TestLaEtiquetaQueBloqueaSeDiceAlAbrir(t *testing.T) {
	p := pantallaDePrueba(t, (&servicioCompartir{}).servidor(t))
	c := p.pedirCompartir(Elemento{ID: "f1", Nombre: "nóminas.xlsx", Etiqueta: &api.DriveLabel{Name: "Confidencial", BlockExternalShare: true}})
	if !c.bloqueaEtiqueta() || !c.crear.Inerte {
		t.Error("una etiqueta que bloquea deja crear el enlace")
	}
	if !contieneTexto(c.cuerpo, p.tr.T("share.blockedFile", nil)) {
		t.Error("el bloqueo no se dice al abrir")
	}
}

// TestFraseCaducidadCuentaPorDias es `expiryPhrase`: se cuenta por DÍA local.
func TestFraseCaducidadCuentaPorDias(t *testing.T) {
	tr := traductor(t)
	ahora := time.Date(2026, 9, 23, 10, 0, 0, 0, time.Local)
	for _, c := range []struct {
		cuando time.Time
		clave  string
	}{
		{ahora.Add(-48 * time.Hour), "expiry.past"},
		{time.Date(2026, 9, 23, 23, 59, 0, 0, time.Local), "expiry.today"},
		{time.Date(2026, 9, 24, 0, 1, 0, 0, time.Local), "expiry.tomorrow"},
	} {
		if got := FraseCaducidad(tr, tr.Locale(), c.cuando, ahora); got != tr.T(c.clave, nil) {
			t.Errorf("%v: %q", c.cuando, got)
		}
	}
	if got := FraseCaducidad(tr, tr.Locale(), ahora.Add(6*24*time.Hour), ahora); !strings.Contains(got, "6") {
		t.Errorf("a seis días dice %q", got)
	}
}

// TestLasVersionesSeListanYSeRestauran: el historial llega con la actual marcada, y
// restaurar pide confirmación EN LA FILA antes de llamar al servicio.
func TestLasVersionesSeListanYSeRestauran(t *testing.T) {
	s := &servicioCompartir{}
	p := pantallaDePrueba(t, s.servidor(t))
	dv := p.pedirVersiones(Elemento{ID: "f1", Nombre: "acta.odt"})
	if dv == nil || len(dv.versiones) != 2 || dv.actual() != 3 {
		t.Fatalf("el historial no llega: %+v", dv)
	}
	dv.confirmar = 2
	dv.pintar()
	if !contieneTexto(dv.cuerpo, p.tr.T("versions.confirmCta", nil)) {
		t.Error("la confirmación no sale en la fila")
	}
	dv.restaurar(dv.versiones[1])
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.escrituras, "POST /api/v1/files/f1/versions/2/restore") {
		t.Errorf("restaurar no llega al servicio: %q", s.escrituras)
	}
}

// TestLosDialogosSePintanEnLosDosTemas: un token mal escrito entra en pánico AL PINTAR.
func TestLosDialogosSePintanEnLosDosTemas(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		p := pantallaDePrueba(t, (&servicioCompartir{}).servidor(t))
		c := p.pedirCompartir(Elemento{ID: "f1", Nombre: "acta.odt"})
		c.creado, c.creadoClave, c.bloqueo = "https://drive.example/public/x", true, "Confidencial"
		c.ponerGrupos([]string{"Ventas"})
		p.win.Canvas().Capture()
		dv := p.pedirVersiones(Elemento{ID: "f1", Nombre: "acta.odt"})
		dv.confirmar = 2
		dv.pintar()
		p.win.Canvas().Capture()
	}
}
