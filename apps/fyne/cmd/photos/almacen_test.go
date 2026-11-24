// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// cuentaDePrueba es una cifrado.Cuenta sin red: la de una vista por la API.
type cuentaDePrueba struct {
	scope   storage.Scope
	session api.Session
}

func (c cuentaDePrueba) Context() context.Context           { return context.Background() }
func (c cuentaDePrueba) PrivateStorageScope() storage.Scope { return c.scope }
func (c cuentaDePrueba) HTTP() api.Session                  { return c.session }
func (c cuentaDePrueba) Check(ctx context.Context) error    { return ctx.Err() }

var _ cifrado.Cuenta = cuentaDePrueba{}

type sesionDePrueba struct{}

func (sesionDePrueba) AccessToken(context.Context) (string, error) { return "tok-de-ada", nil }
func (sesionDePrueba) Invalidate(string)                           {}

// servidorQueAnota contesta {} a todo y anota método, ruta y credencial.
type servidorQueAnota struct {
	mu     sync.Mutex
	vistas []string
}

func (s *servidorQueAnota) levantar(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		s.mu.Lock()
		s.vistas = append(s.vistas, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"x","name":"Mi unidad"}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (s *servidorQueAnota) vio(parte string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.vistas {
		if strings.Contains(v, parte) {
			return true
		}
	}
	return false
}

// Por la API (fuente `legacy`), la galería habla con el servidor como la web: los
// registros con el servicio de Photos y los originales con el Drive del servidor,
// con la credencial de la cuenta. Nada pasa por el Drive oculto ni por Matrix. Y una
// escritura de Photos avisa al widget por el cliente que le da el `cmd`
// (Suite.ClienteConAviso); una lectura no.
//
// VISTO EN ROJO sin pasar `escrituras` a api.NewPhotos: 0 avisos tras Indexar.
func TestPhotosPorLaAPIHablaConElServidor(t *testing.T) {
	photosSrv, driveSrv := &servidorQueAnota{}, &servidorQueAnota{}
	cuenta := cuentaDePrueba{scope: storage.Scope{Tenant: "acme", Subject: "ada", Product: "photos"}, session: sesionDePrueba{}}
	var avisos atomic.Int32
	escrituras := api.ConAvisoDeEscritura(nil, func() { avisos.Add(1) })

	client, drive, err := clientesPhotos(photosSrv.levantar(t), driveSrv.levantar(t), escrituras, almacenPhotos{cuenta: cuenta})
	if err != nil {
		t.Fatal(err)
	}
	if client.CanImportPrivatePhotos() {
		t.Fatal("por la API no hay importación privada: los originales van al Drive del servidor")
	}
	if _, err := client.Indexar(t.Context(), "fich-1"); err != nil {
		t.Fatalf("Indexar: %v", err)
	}
	if !photosSrv.vio("POST") || !photosSrv.vio("/assets") || !photosSrv.vio("Bearer tok-de-ada") {
		t.Fatalf("Indexar no fue al servicio de Photos con la cuenta: %v", photosSrv.vistas)
	}
	if n := avisos.Load(); n != 1 {
		t.Fatalf("indexar una foto por la API avisa %d veces al widget; quiero 1", n)
	}
	if _, err := drive.Root(t.Context()); err != nil {
		t.Fatalf("Root: %v", err)
	}
	if !driveSrv.vio("/folders/root") || !driveSrv.vio("Bearer tok-de-ada") {
		t.Fatalf("el Drive de la galería no fue el del servidor: %v", driveSrv.vistas)
	}
	if n := avisos.Load(); n != 1 {
		t.Fatalf("leer el Drive de la galería ha avisado al widget (%d avisos)", n)
	}
}

// Sin cuenta no hay clientes; y con repositorio, sin el Drive oculto de los
// originales tampoco: la galería de Matrix nunca sube al Drive del servidor.
func TestPhotosSinCuentaOSinDriveOcultoNoMonta(t *testing.T) {
	if _, _, err := clientesPhotos("http://photos.invalid", "http://drive.invalid", nil, almacenPhotos{}); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatalf("sin cuenta: %v", err)
	}
	cuenta := cuentaDePrueba{scope: storage.Scope{Tenant: "acme", Subject: "ada", Product: "photos"}}
	var resolve func(context.Context) (*privateDrive.Repository, error)
	if _, _, err := clientesPhotos("http://photos.invalid", "http://drive.invalid", nil, almacenPhotos{cuenta: cuenta, repository: &privatePhotos.Repository{}, resolveDrive: resolve}); !errors.Is(err, cifrado.ErrPrivateResources) {
		t.Fatalf("con repositorio y sin Drive oculto: %v", err)
	}
}
