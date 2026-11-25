// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// drivePrivadoDePrueba es el Drive de la bóveda de Ana con lo que acaba de subir.
type drivePrivadoDePrueba struct {
	api.DriveData
	nombre string
}

func (drivePrivadoDePrueba) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "ana", Product: "drive"}
}

func (drivePrivadoDePrueba) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "raiz"}, nil
}

func (d drivePrivadoDePrueba) Children(context.Context, string, string) (api.Children, error) {
	return api.Children{Files: []api.File{{ID: "f", Name: d.nombre, SizeBytes: 1}}}, nil
}

// TestElWidgetDeDriveLeeElDrivePrivadoAbierto: el Drive de la cuenta vive en la bóveda (o
// en Matrix) y su fuente del servidor está congelada. Sin la vista montada —iOS en segundo
// plano, sin la clave— no se publica nada (ni lo congelado); con la vista montada se
// publica lo que hay en el Drive privado, con la ruta de cada fila; y al cerrarla (otra
// cuenta, cerrar sesión) lo publicado se queda, con su antigüedad.
func TestElWidgetDeDriveLeeElDrivePrivadoAbierto(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := suiteConSesion(t, appDeWidget{App: app, id: "eu.sytue.drive", esquema: "sytue-drive"}, "ana")
	reloj := widgets.NuevoRelojDePrueba(time.Now())
	s.ProbarWidgetsCon(reloj.Opciones(widgets.OpcionesPublicador{}))
	pub := s.PublicadorWidget(s.FuenteWidget(nil))
	if pub == nil {
		t.Fatal("Drive sin publicador")
	}
	// El servidor, congelado, enseñaría lo de antes de pasar a la bóveda.
	congelado := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/quota" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		t.Errorf("el widget de Drive leyó el servidor congelado: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer congelado.Close()

	pub.Avisar()
	reloj.Avanzar(2 * time.Second)
	if _, err := widgets.Read("eu.sytue.drive"); err == nil {
		t.Fatal("sin el Drive privado abierto se publicó algo")
	}

	cuenta := api.NewDrive(congelado.URL, nil, nil)
	datos, err := cuenta.WithPrivateData(drivePrivadoDePrueba{nombre: "Recién subido.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	vida, cerrar := context.WithCancel(context.Background())
	s.WidgetDesdeDrivePrivado(vida, datos, cuenta)
	reloj.Avanzar(2 * time.Second)
	snap, err := widgets.Read("eu.sytue.drive")
	if err != nil || len(snap.Items) != 1 || snap.Items[0].Title != "Recién subido.pdf" || !snap.SoloConLaApp ||
		snap.Items[0].Route != "/?preview=%2FReci%C3%A9n%20subido.pdf" {
		t.Fatalf("con el Drive privado abierto: %+v, %v", snap, err)
	}

	cerrar()
	pub.Avisar()
	reloj.Avanzar(2 * time.Second)
	if snap, err := widgets.Read("eu.sytue.drive"); err != nil || len(snap.Items) != 1 {
		t.Fatalf("al cerrar el Drive privado se perdió lo publicado: %+v, %v", snap, err)
	}
}

// basesPrivadasDePrueba es el repositorio de Bases de Ana, ya en Matrix.
type basesPrivadasDePrueba struct {
	api.BasesData
	nombre string
}

func (basesPrivadasDePrueba) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "ana", Product: "bases"}
}

func (b basesPrivadasDePrueba) ListBases(context.Context) ([]api.BasesBase, error) {
	return []api.BasesBase{{ID: "0b5c6f3e-1d2a-4c8e-9f00-112233445566", Name: b.nombre}}, nil
}

// TestElWidgetDeUnaAppDeRegistrosLeeSuRepositorioEnMatrix: lo mismo que Drive para las
// apps de registros con la cuenta en Matrix (Calendar, Contacts, Notes, Photos, Bases,
// ERP, Cast y la ofimática). Sin el repositorio montado no se publica lo del servidor
// congelado; montado, lo que hay en él; y con la vista ya cerrada lo publicado se queda.
func TestElWidgetDeUnaAppDeRegistrosLeeSuRepositorioEnMatrix(t *testing.T) {
	t.Cleanup(widgets.SimularLector(true))
	aparte(t)
	app := test.NewApp()
	defer app.Quit()
	s := suiteConSesion(t, appDeWidget{App: app, id: "eu.sytue.bases", esquema: "sytue-bases"}, "ana")
	reloj := widgets.NuevoRelojDePrueba(time.Now())
	s.ProbarWidgetsCon(reloj.Opciones(widgets.OpcionesPublicador{}))
	pub := s.PublicadorWidget(s.FuenteWidget(nil))
	if pub == nil {
		t.Fatal("Bases sin publicador")
	}
	pub.Avisar()
	reloj.Avanzar(2 * time.Second)
	if _, err := widgets.Read("eu.sytue.bases"); err == nil {
		t.Fatal("sin el repositorio ni el modo de la fuente se publicó algo")
	}

	congelado := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("el widget de Bases leyó el servidor congelado: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer congelado.Close()
	datos, err := api.NewBases(congelado.URL, nil, nil).WithPrivateData(basesPrivadasDePrueba{nombre: "Inventario"})
	if err != nil {
		t.Fatal(err)
	}
	vida, cerrar := context.WithCancel(context.Background())
	// Una fachada que no es la de su producto no monta nada.
	s.WidgetDesdeRegistrosPrivados(vida, nil, api.NewDocs(congelado.URL, nil, nil))
	s.WidgetDesdeRegistrosPrivados(vida, nil, datos)
	reloj.Avanzar(2 * time.Second)
	snap, err := widgets.Read("eu.sytue.bases")
	if err != nil || len(snap.Items) != 1 || snap.Items[0].Title != "Inventario" || !snap.SoloConLaApp ||
		snap.Items[0].Route != "/base/0b5c6f3e-1d2a-4c8e-9f00-112233445566" {
		t.Fatalf("con el repositorio abierto: %+v, %v", snap, err)
	}

	cerrar()
	pub.Avisar()
	reloj.Avanzar(2 * time.Second)
	if snap, err := widgets.Read("eu.sytue.bases"); err != nil || len(snap.Items) != 1 {
		t.Fatalf("al cerrar el repositorio se perdió lo publicado: %+v, %v", snap, err)
	}
}
