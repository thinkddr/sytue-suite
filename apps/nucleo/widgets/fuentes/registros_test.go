// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// sesionDe es una sesión que dice de quién es, como auth.Session.
type sesionDe struct{ yo auth.IdentitySnapshot }

func (sesionDe) AccessToken(context.Context) (string, error) { return "t", nil }
func (sesionDe) Invalidate(string)                           {}
func (s sesionDe) SnapshotIdentity() auth.IdentitySnapshot   { return s.yo }

// basesPrivadas es el repositorio de Bases de una cuenta en Matrix: lo que el widget lee.
type basesPrivadas struct {
	api.BasesData
	bases []api.BasesBase
}

func (basesPrivadas) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "ana", Product: "bases"}
}
func (b basesPrivadas) ListBases(context.Context) ([]api.BasesBase, error) { return b.bases, nil }

// TestLasAppsDeRegistrosNoEnseñanLoCongeladoDelServidor: el caso del roadmap «las apps de
// registros en Matrix leen para el widget la fuente del servidor, congelada». Con la
// cuenta en `legacy` la registrada lee el servidor, como la app; en cuanto la fuente deja
// de ser `legacy` (la cuenta pasó a Matrix), no lo lee: falla con ErrSinDatosPrivados, que
// conserva lo publicado desde el repositorio, y la app monta la suya sobre él (Privados).
// El modo se pregunta una vez por VigenciaModo y por cuenta.
func TestLasAppsDeRegistrosNoEnseñanLoCongeladoDelServidor(t *testing.T) {
	const id = "0b5c6f3e-1d2a-4c8e-9f00-112233445566"
	ana := auth.IdentitySnapshot{Subject: "ana", Organization: "org"}
	scope := storage.Scope{Tenant: "org", Subject: "ana", Product: "bases"}
	var modo atomic.Value
	modo.Store("legacy")
	var preguntas, lecturas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/api/v1") {
		case "/private-storage/migration":
			preguntas.Add(1)
			cabecera, _ := storage.EncodeScopeHeader(scope)
			if r.Header.Get(storage.ScopeHeader) != cabecera {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			estado := storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: modo.Load().(string)}}
			if estado.Mode != "legacy" {
				empezada := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
				estado.Epoch, estado.StartedAt = "1b5c6f3e-1d2a-4c8e-9f00-112233445566", &empezada
			}
			_ = json.NewEncoder(w).Encode(estado)
		case "/bases":
			lecturas.Add(1)
			_ = json.NewEncoder(w).Encode([]api.BasesBase{{ID: id, Name: "Congelada"}})
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	ctx, parar := context.WithTimeout(context.Background(), 5*time.Second)
	defer parar()
	reloj := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	c := Comunes{Ahora: func() time.Time { return reloj }}
	url := func(string) string { return srv.URL }
	fabrica, _ := Registrada("bases")
	fuente := fabrica(c, url, sesionDe{yo: ana})

	if d, err := fuente(ctx); err != nil || len(d.Items) != 1 || d.Items[0].Title != "Congelada" {
		t.Fatalf("en legacy no lee el servidor: %+v, %v", d, err)
	}
	if _, err := fuente(ctx); err != nil || preguntas.Load() != 1 || lecturas.Load() != 2 {
		t.Fatalf("pregunta el modo %d veces en %d lecturas (%v); una por VigenciaModo", preguntas.Load(), lecturas.Load(), err)
	}

	// Otra fuente (otra ronda sin ventana) con la cuenta ya en Matrix: no lee el servidor.
	modo.Store("frozen")
	fuente = fabrica(c, url, sesionDe{yo: ana})
	if _, err := fuente(ctx); !errors.Is(err, ErrSinDatosPrivados) || widgets.AutenticacionRechazada(err) || lecturas.Load() != 2 {
		t.Fatalf("en Matrix: %v (lecturas del servidor %d); quiero ErrSinDatosPrivados sin leerlo", err, lecturas.Load())
	}
	// Pasada la vigencia vuelve a preguntar: soltar la fuente devuelve la cuenta a la API.
	modo.Store("legacy")
	reloj = reloj.Add(VigenciaModo + time.Second)
	if _, err := fuente(ctx); err != nil || lecturas.Load() != 3 {
		t.Fatalf("tras soltar la fuente: %v (lecturas %d)", err, lecturas.Load())
	}
	// Sin saber de quién es la sesión no se pregunta ni se lee.
	antes := preguntas.Load()
	if _, err := fabrica(c, url, nil)(ctx); !errors.Is(err, ErrSinDatosPrivados) || preguntas.Load() != antes {
		t.Fatalf("sin identidad: %v", err)
	}
	var nadie *auth.Session
	if _, err := fabrica(c, url, nadie)(ctx); !errors.Is(err, ErrSinDatosPrivados) {
		t.Fatalf("con una auth.Session nil: %v", err)
	}

	// Con la app abierta en Matrix: monta la Fuente de su repositorio en una Privada, que
	// sin él cae en la registrada.
	privadas, err := api.NewBases(srv.URL, nil, nil).WithPrivateData(basesPrivadas{bases: []api.BasesBase{{ID: id, Name: "Nueva"}}})
	if err != nil {
		t.Fatal(err)
	}
	f, ok := Privados("bases", c, privadas, url, nil)
	if !ok {
		t.Fatal("Bases no tiene Fuente privada")
	}
	modo.Store("frozen")
	reloj = reloj.Add(VigenciaModo + time.Second)
	var p Privada
	p.Respaldar(fuente)
	soltar := p.Montar(f)
	d, err := p.Fuente()(ctx)
	if err != nil || len(d.Items) != 1 || d.Items[0].Title != "Nueva" || d.Items[0].Route != RutaBase(id) {
		t.Fatalf("con el repositorio abierto: %+v, %v", d, err)
	}
	// Lo del repositorio sale marcado: ya rancio, el widget dice que se actualiza con la
	// app abierta (widgets.Snapshot.PideAbrir). Lo del servidor, no.
	if !d.SoloConLaApp {
		t.Fatal("lo leído del repositorio privado no sale SoloConLaApp")
	}
	soltar()
	if _, err := p.Fuente()(ctx); !errors.Is(err, ErrSinDatosPrivados) {
		t.Fatalf("al cerrar el repositorio cae en la registrada: %v", err)
	}
	// Soltada la fuente (otra vez `legacy`), la registrada lee el servidor y sin marca.
	modo.Store("legacy")
	reloj = reloj.Add(VigenciaModo + time.Second)
	if d, err := p.Fuente()(ctx); err != nil || d.SoloConLaApp || len(d.Items) != 1 || d.Items[0].Title != "Congelada" {
		t.Fatalf("del servidor: %+v, %v", d, err)
	}
}

// TestCadaAppDeRegistrosTieneSuFuentePrivada: Privados sabe construir la Fuente de cada
// app de registros sobre su fachada, y no la de otro producto (la fachada de Docs es de
// Docus o de Calcus, no de Calendar); y DeRegistros son justo esas, ni Drive, ni Mail.
func TestCadaAppDeRegistrosTieneSuFuentePrivada(t *testing.T) {
	url := func(string) string { return "http://127.0.0.1:9" }
	fachadas := map[string]any{
		"bases":    api.NewBases(url(""), nil, nil),
		"calendar": api.NewCalendars(url(""), nil, nil),
		"cast":     api.NewCast(url(""), nil, nil),
		"contacts": api.NewContacts(url(""), nil, nil),
		"docs":     api.NewDocs(url(""), nil, nil),
		"erp":      api.NewERP(url(""), nil, nil, ""),
		"notes":    api.NewNotes(url(""), nil, nil),
		"photos":   api.NewPhotos(url(""), nil, nil),
		"sheets":   api.NewDocs(url(""), nil, nil),
	}
	for producto, datos := range fachadas {
		if !DeRegistros(producto) {
			t.Errorf("%s no es de registros", producto)
		}
		if _, ok := Privados(producto, Comunes{}, datos, url, nil); !ok {
			t.Errorf("%s sin Fuente privada", producto)
		}
	}
	if _, ok := Privados("calendar", Comunes{}, api.NewDocs(url(""), nil, nil), url, nil); ok {
		t.Error("la fachada de Docs da la Fuente de Calendar")
	}
	if _, ok := Privados("drive", Comunes{}, api.NewDrive(url(""), nil, nil), url, nil); ok {
		t.Error("Drive monta la suya con Suite.WidgetDesdeDrivePrivado")
	}
	// Diapos monta un *api.Slides: su lista es la de la fachada de Docs que comparte el
	// repositorio (api.Slides.Docs).
	if _, ok := Privados("slides", Comunes{}, api.NewSlides(url(""), nil, nil), url, nil); !ok {
		t.Error("Diapos sin Fuente privada")
	}
	for _, p := range []string{"drive", "pdf", "mail", "rooms", "one", "slides"} {
		if DeRegistros(p) != (p == "slides") {
			t.Errorf("DeRegistros(%s) = %v", p, DeRegistros(p))
		}
	}
	// El ERP en Matrix no tiene tareas (api.ERPData): publica vacío, como la lista de la
	// app, en vez de conservar lo congelado.
	privado, err := api.NewERP(url(""), nil, nil, "").WithPrivateData(erpPrivado{})
	if err != nil {
		t.Fatal(err)
	}
	if d, err := ERP(Comunes{}, privado)(context.Background()); err != nil || len(d.Items) != 0 || len(d.Counters) != 0 {
		t.Fatalf("ERP en Matrix: %+v, %v", d, err)
	}
}

type erpPrivado struct{ api.ERPData }

func (erpPrivado) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "ana", Product: "erp"}
}
