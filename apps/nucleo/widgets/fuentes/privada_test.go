// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// drivePrivado es el Drive de la bóveda (o de Matrix) de una cuenta: una raíz con lo que
// se acaba de subir.
type drivePrivado struct {
	api.DriveData
	hijos api.Children
}

func (drivePrivado) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "ana", Product: "drive"}
}
func (drivePrivado) Root(context.Context) (api.Folder, error) { return api.Folder{ID: "raiz"}, nil }
func (d drivePrivado) Children(_ context.Context, carpeta, _ string) (api.Children, error) {
	if carpeta == "raiz" {
		return d.hijos, nil
	}
	return api.Children{}, nil
}
func (drivePrivado) Upload(context.Context, string, string, string, io.Reader, int64) (api.File, error) {
	return api.File{}, errors.New("no se sube en esta prueba")
}

// TestDriveLeeElRepositorioAbiertoYSinElConservaLoPublicado: el caso del dueño, «subo un
// fichero a Drive y el widget no lo enseña». Los datos de Drive están en el repositorio
// privado y el servidor está congelado: la Fuente lee el repositorio que la app tiene
// abierto, con lo recién subido. Sin él —iOS en segundo plano, el refresco sin ventana de
// Android— la Fuente falla con ErrSinDatosPrivados, que no es un rechazo de la sesión: el
// Publicador conserva lo último publicado y no enseña lo congelado del servidor.
func TestDriveLeeElRepositorioAbiertoYSinElConservaLoPublicado(t *testing.T) {
	ctx, parar := context.WithTimeout(context.Background(), 5*time.Second)
	defer parar()
	// El servidor, congelado: si alguien lo leyera, saldría «Viejo.pdf».
	congelado := servidor(t, map[string]any{
		"/folders/root":           api.Folder{ID: "r"},
		"/folders/r/children":     api.Children{Files: []api.File{{ID: "v", Name: "Viejo.pdf"}}},
		"/quota":                  map[string]any{"used_bytes": 50, "limit_bytes": 100},
		"/pdf/signature-requests": api.PDFPeticionesFirma{},
	})
	url := func(string) string { return congelado }
	for _, producto := range []string{"drive", "pdf"} {
		fabrica, _ := Registrada(producto)
		if _, err := fabrica(Comunes{}, url, nil)(ctx); !errors.Is(err, ErrSinDatosPrivados) ||
			widgets.AutenticacionRechazada(err) {
			t.Fatalf("%s sin la app: %v; quiero ErrSinDatosPrivados, que conserva lo publicado", producto, err)
		}
	}

	var p Privada
	if _, err := p.Fuente()(ctx); !errors.Is(err, ErrSinDatosPrivados) {
		t.Fatalf("sin montar: %v", err)
	}
	servidorDrive := api.NewDrive(congelado, nil, nil)
	datos, err := servidorDrive.WithPrivateData(drivePrivado{hijos: api.Children{
		Files: []api.File{{ID: "n", Name: "Nuevo.pdf", SizeBytes: 3, UpdatedAt: time.Now()}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	soltar := p.Montar(Drive(Comunes{}, datos, servidorDrive))
	d, err := p.Fuente()(ctx)
	if err != nil || len(d.Items) != 1 || d.Items[0].Title != "Nuevo.pdf" || d.Items[0].Route != "/?preview=%2FNuevo.pdf" {
		t.Fatalf("con el repositorio abierto: %+v, %v", d, err)
	}
	if len(d.Counters) != 1 || d.Counters[0] != (widgets.Counter{Kind: "storage_percent", Value: 50}) {
		t.Fatalf("la cuota es de la cuenta, del servidor: %+v", d.Counters)
	}
	// Otra cuenta monta el suyo antes de que la vieja suelte el de antes: soltar tarde
	// no quita el nuevo.
	nueva := p.Montar(func(context.Context) (widgets.Detail, error) {
		return widgets.Detail{Items: []widgets.Item{{Title: "B"}}}, nil
	})
	soltar()
	if d, err := p.Fuente()(ctx); err != nil || len(d.Items) != 1 || d.Items[0].Title != "B" {
		t.Fatalf("soltar el viejo quitó el nuevo: %+v, %v", d, err)
	}
	nueva()
	if _, err := p.Fuente()(ctx); !errors.Is(err, ErrSinDatosPrivados) {
		t.Fatalf("tras cerrar el repositorio: %v", err)
	}

	// CertiPDF: la carpeta «PDF» del Drive privado y las peticiones de firma del servicio.
	privadoPDF, err := api.NewPDF(congelado, congelado, nil, nil).WithPrivateDrive(drivePrivado{hijos: api.Children{
		Folders: []api.Folder{{ID: "pdf", Name: api.PDFCarpeta}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CertiPDF(Comunes{}, privadoPDF, api.NewPDF(congelado, congelado, nil, nil))(ctx); err != nil {
		t.Fatalf("CertiPDF con su Drive privado: %v", err)
	}
}
