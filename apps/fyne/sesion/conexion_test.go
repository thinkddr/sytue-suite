// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package sesion

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// TestLaConexionEsDeLaSesion: cualquier app pide su cliente «red primero» con su
// producto y sus rutas privadas, y el monitor que devuelve es el que alimenta ese
// transporte. Era código de `cmd/drive`; sin esto, cada app lo copiaba.
func TestLaConexionEsDeLaSesion(t *testing.T) {
	aparte(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")

	red, cliente := s.Conexion("mail", "/api/v1/adjuntos/")
	tr, es := cliente.Transport.(*offline.Transport)
	if !es {
		t.Fatalf("el cliente lleva un %T y tiene que llevar la caché de la suite", cliente.Transport)
	}
	if tr.Product != "mail" || !slices.Equal(tr.Private, []string{"/api/v1/adjuntos/"}) {
		t.Errorf("el transporte es de %q con privadas %v", tr.Product, tr.Private)
	}
	if !slices.Equal(tr.CacheProducts, api.Productos()) || tr.Scope == nil {
		t.Fatal("global budget/session revision not wired")
	}
	if tr.Monitor != red {
		t.Error("el monitor devuelto no es el que alimenta el transporte: la pantalla no se enteraría")
	}
}

// viaDePrueba apunta lo que llega a la red.
type viaDePrueba struct{ vistas []string }

func (v *viaDePrueba) RoundTrip(r *http.Request) (*http.Response, error) {
	v.vistas = append(v.vistas, r.Method+" "+r.URL.Path)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
}

// TestSoloLecturaNoDejaEscribir: con `SYTUE_SOLO_LECTURA` un clic del recorrido de
// ventana real puede caer en cualquier botón y no llega nada a producción. Lo que lee
// pasa, y el POST al `/token` del IdP también: sin él no hay sesión.
//
// VISTO EN ROJO dejando pasar todo POST: «Nueva nota» llegaba a la red.
func TestSoloLecturaNoDejaEscribir(t *testing.T) {
	red := &viaDePrueba{}
	c := &http.Client{Transport: soloLectura{base: red}}
	for _, p := range []struct {
		metodo, url string
		pasa        bool
	}{
		{http.MethodGet, "https://notes.sytue.eu/api/v1/notebooks", true},
		{http.MethodPost, "https://id.sytue.eu/oauth2/token", true},
		{http.MethodPost, "https://notes.sytue.eu/api/v1/notebooks/1/notes", false},
		{http.MethodPut, "https://docs.sytue.eu/api/v1/documents/1", false},
		{http.MethodPatch, "https://mail.sytue.eu/api/v1/messages/1", false},
		{http.MethodDelete, "https://drive.sytue.eu/api/v1/files/1", false},
	} {
		req, err := http.NewRequest(p.metodo, p.url, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		if pasa := err == nil; pasa != p.pasa {
			t.Errorf("%s %s: pasa=%v y quiero %v (%v)", p.metodo, p.url, pasa, p.pasa, err)
		}
		if err != nil && !errors.Is(err, ErrSoloLectura) {
			t.Errorf("%s %s: el error es %v; quiero ErrSoloLectura", p.metodo, p.url, err)
		}
	}
	if len(red.vistas) != 2 {
		t.Errorf("a la red han llegado %v; quiero solo la lectura y el canje del token", red.vistas)
	}
}

// escrituraDePrueba es una escritura de la tabla de pruebas del desbloqueo.
type escrituraDePrueba struct {
	metodo, url string
	pasa        bool
}

// escriturasDePrueba: las del desbloqueo del cifrado y del alta del dispositivo de Matrix
// (pasan con SYTUE_PERMITIR_DESBLOQUEO) y, a su lado, las que se parecen y NO son de esa
// lista: otra ruta de Matrix que también escribe, otro dato de cuenta, la presencia, la
// contraseña de aplicación de Drive y el contenido.
var escriturasDePrueba = []escrituraDePrueba{
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/login", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/keys/upload", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/keys/query", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/filter", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/keys/signatures/upload", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/keys/device_signing/upload", true},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/account_data/m.cross_signing.master", true},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/account_data/m.secret_storage.key.abcDEF012", true},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/account_data/m.megolm_backup.v1", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/room_keys/version", true},
	{http.MethodPut, "https://rooms.sytue.eu/api/v1/matrix/recovery-wrappers?product=mail&retire_older=1", true},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/logout", true},
	{http.MethodPost, "https://id.sytue.eu/oauth2/revoke", true},

	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/logout/all", false},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/keys/claim", false},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/sendToDevice/m.room_key_request/1", false},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/room_keys/keys?version=1", false},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/account_data/m.direct", false},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/rooms/%21r%3Asytue.eu/account_data/m.tag", false},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/rooms/%21r%3Asytue.eu/send/m.room.message/1", false},
	{http.MethodDelete, "https://matrix.sytue.eu/_matrix/client/v3/devices/SYTUE_X", false},
	{http.MethodPost, "https://matrix.sytue.eu/_matrix/client/v3/user/%40sytue_x%3Asytue.eu/filter/extra", false},
	{http.MethodGet + "X", "https://matrix.sytue.eu/_matrix/client/v3/login", false},
	{http.MethodPut, "https://matrix.sytue.eu/_matrix/client/v3/login", false},
	{http.MethodPost, "https://rooms.sytue.eu/api/v1/matrix/recovery-wrappers", false},
	{http.MethodDelete, "https://rooms.sytue.eu/api/v1/matrix/recovery-wrappers", false},
	{http.MethodPost, "https://rooms.sytue.eu/api/v1/matrix/native/start", false},
	{http.MethodPost, "https://rooms.sytue.eu/api/v1/matrix/storage/mail/migration", false},
	{http.MethodPut, "https://hub.sytue.eu/api/v1/presence", false},
	{http.MethodPost, "https://id.sytue.eu/api/v1/me/app-passwords", false},
	{http.MethodPost, "https://id.sytue.eu/oauth2/revoke/extra", false},
	{http.MethodPost, "https://mail.sytue.eu/api/v1/message-actions", false},
	{http.MethodPost, "https://notes.sytue.eu/api/v1/notebooks/1/notes", false},
	{http.MethodDelete, "https://drive.sytue.eu/api/v1/files/1", false},
}

// TestElDesbloqueoSoloDejaSuListaCerrada: con SYTUE_PERMITIR_DESBLOQUEO=1 pasan las
// escrituras del alta del dispositivo de Matrix, de sus firmas cruzadas, del envoltorio
// y del cierre de sesión, y NINGUNA otra; sin la variable, ninguna de ellas.
//
// VISTO EN ROJO quitando la comprobación del método (`PUT /login` pasaba) y dejando el
// segmento de la cuenta sin acotar (`/filter/extra` pasaba).
func TestElDesbloqueoSoloDejaSuListaCerrada(t *testing.T) {
	for _, desbloqueo := range []bool{true, false} {
		red := &viaDePrueba{}
		c := &http.Client{Transport: soloLectura{base: red, desbloqueo: desbloqueo}}
		quiero := 0
		for _, p := range escriturasDePrueba {
			req, err := http.NewRequest(p.metodo, p.url, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := c.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			pasaQuiero := p.pasa && desbloqueo
			if pasaQuiero {
				quiero++
			}
			if pasa := err == nil; pasa != pasaQuiero {
				t.Errorf("desbloqueo=%v · %s %s: pasa=%v y quiero %v (%v)", desbloqueo, p.metodo, p.url, pasa, pasaQuiero, err)
			}
			if err != nil && !errors.Is(err, ErrSoloLectura) {
				t.Errorf("desbloqueo=%v · %s %s: el error es %v; quiero ErrSoloLectura", desbloqueo, p.metodo, p.url, err)
			}
		}
		if len(red.vistas) != quiero {
			t.Errorf("desbloqueo=%v: a la red han llegado %d (%v); quiero %d", desbloqueo, len(red.vistas), red.vistas, quiero)
		}
	}
}

// TestElRegistroNoLlevaLaCuentaDeMatrix: lo que se corta se apunta en el registro de la
// app (y el CI lo sube con las capturas), pero sin la cuenta, la sala ni ids largos.
func TestElRegistroNoLlevaLaCuentaDeMatrix(t *testing.T) {
	for ruta, quiero := range map[string]string{
		"/_matrix/client/v3/user/@sytue_x:sytue.eu/filter":              "/_matrix/client/v3/user/…/filter",
		"/_matrix/client/v3/rooms/!sala:sytue.eu/send/m.room.message/1": "/_matrix/client/v3/rooms/…/send/m.room.message/1",
		"/api/v1/files/0d2e8451-58ea-4966-9802-b30e33577665":            "/api/v1/files/…",
		"/api/v1/presence": "/api/v1/presence",
	} {
		if got := rutaSinIdentificadores(ruta); got != quiero {
			t.Errorf("rutaSinIdentificadores(%q) = %q; quiero %q", ruta, got, quiero)
		}
	}
}

// TestSoloLecturaCubreLosDosTransportes: el solo lectura se pone en el transporte por
// defecto Y en api.Transporte, que es un clon suyo y no lo hereda. Corre en un proceso
// aparte porque ponerSoloLectura cambia variables globales una vez por proceso.
//
// VISTO EN ROJO con la versión de antes (solo http.DefaultTransport): por
// api.Transporte llegaba al servidor el PUT de la presencia.
func TestSoloLecturaCubreLosDosTransportes(t *testing.T) {
	if os.Getenv("SYTUE_PRUEBA_DOS_TRANSPORTES") == "1" {
		comprobarDosTransportes(t)
		return
	}
	for _, desbloqueo := range []string{"", "1"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSoloLecturaCubreLosDosTransportes$", "-test.count=1")
		cmd.Env = append(os.Environ(), "SYTUE_PRUEBA_DOS_TRANSPORTES=1", "SYTUE_SOLO_LECTURA=1", "SYTUE_PERMITIR_DESBLOQUEO="+desbloqueo)
		if salida, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("SYTUE_PERMITIR_DESBLOQUEO=%q: %v\n%s", desbloqueo, err, salida)
		}
	}
}

func comprobarDosTransportes(t *testing.T) {
	var vistas []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		vistas = append(vistas, r.Method+" "+r.URL.Path)
		mu.Unlock()
	}))
	defer srv.Close()
	ponerSoloLectura()
	desbloqueo := os.Getenv("SYTUE_PERMITIR_DESBLOQUEO") == "1"
	for nombre, tr := range map[string]http.RoundTripper{"http.DefaultTransport": http.DefaultTransport, "api.Transporte": api.Transporte} {
		if s, ok := tr.(soloLectura); !ok || s.desbloqueo != desbloqueo {
			t.Errorf("%s es %T; quiero soloLectura con desbloqueo=%v", nombre, tr, desbloqueo)
		}
		c := &http.Client{Transport: tr}
		for _, p := range []struct{ metodo, ruta string }{
			{http.MethodPut, "/api/v1/presence"},
			{http.MethodPost, "/_matrix/client/v3/logout"},
		} {
			req, _ := http.NewRequest(p.metodo, srv.URL+p.ruta, http.NoBody)
			if resp, err := c.Do(req); err == nil {
				resp.Body.Close()
			}
		}
	}
	quiero := []string(nil)
	if desbloqueo {
		quiero = []string{"POST /_matrix/client/v3/logout", "POST /_matrix/client/v3/logout"}
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(vistas, quiero) {
		t.Errorf("al servidor han llegado %v; quiero %v", vistas, quiero)
	}
}

func TestOfflineScopeRejectsOtherAppSessionBeforeWatcherAdoptsIt(t *testing.T) {
	aparte(t)
	ajustes.MarcarSesion()
	generation := ajustes.Leer().Sesion
	s := &Suite{generacionSesion: generation}
	if s.scopeCache() != generation {
		t.Fatal("adopted session rejected")
	}
	ajustes.MarcarSesion()
	if s.scopeCache() != "" {
		t.Fatal("old session accepted before shared watcher")
	}
	s.generacionSesion = ajustes.Leer().Sesion
	if s.scopeCache() != s.generacionSesion {
		t.Fatal("new adopted session rejected")
	}
}
