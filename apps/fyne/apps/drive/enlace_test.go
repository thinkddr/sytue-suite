// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElEnlaceDeLaWebLlegaALaCarpetaYAlFichero: lo que emite `resourceHref` en la web
// (`?path=<carpeta>&preview=<fichero>`) acaba en las migas de verdad y en el fichero, con
// su id. La carpeta de en medio está en la SEGUNDA página, que es donde un buscador que
// solo mirase la primera diría «no está». VISTO EN ROJO quitando el paso de página y
// cambiando `el.Carpeta == esCarpeta` por `true`.
func TestElEnlaceDeLaWebLlegaALaCarpetaYAlFichero(t *testing.T) {
	u, err := url.Parse("sytue-drive:/?path=%2FEquipo%2F2026&preview=%2FEquipo%2F2026%2Facta.docx")
	if err != nil {
		t.Fatal(err)
	}
	carpeta, fichero, ok := DestinoDeEnlace(u)
	if !ok || carpeta != "/Equipo/2026" || fichero != "/Equipo/2026/acta.docx" {
		t.Fatalf("destino = %q %q %v", carpeta, fichero, ok)
	}

	raiz := func(context.Context) (api.Folder, error) { return api.Folder{ID: "r", IsRoot: true}, nil }
	arbol := map[string][]api.Children{
		"r": {
			{Files: []api.File{{ID: "f0", Name: "Equipo"}}, NextCursor: "2"}, // un FICHERO con el nombre
			{Folders: []api.Folder{{ID: "c1", Name: "Equipo"}}},
		},
		"c1": {{Folders: []api.Folder{{ID: "c2", Name: "2026"}}}},
		"c2": {{Files: []api.File{{ID: "f1", Name: "acta.docx"}}}},
	}
	hijos := func(_ context.Context, id, cursor string) (api.Children, error) {
		paginas := arbol[id]
		if cursor == "2" {
			return paginas[1], nil
		}
		return paginas[0], nil
	}
	migas, el, err := Resolver(context.Background(), raiz, hijos, carpeta, fichero)
	if err != nil {
		t.Fatal(err)
	}
	if len(migas) != 2 || migas[0].ID != "c1" || migas[1].ID != "c2" || el == nil || el.ID != "f1" || el.Ruta != fichero {
		t.Fatalf("migas = %+v, fichero = %+v", migas, el)
	}
	if _, _, err := Resolver(context.Background(), raiz, hijos, "/Equipo/2025", ""); !errors.Is(err, errNoEsta) {
		t.Errorf("una carpeta que no está = %v, quiero errNoEsta", err)
	}

	// Un fichero de la raíz llega sin `path`, y un enlace sin destino no es de esto.
	u, _ = url.Parse("sytue-drive:/drive?preview=%2Facta.docx")
	if c, f, ok := DestinoDeEnlace(u); !ok || c != "/" || f != "/acta.docx" {
		t.Errorf("de la raíz = %q %q %v", c, f, ok)
	}
	for _, crudo := range []string{"sytue-drive:/", "sytue-drive:/files/42?path=%2Fa", "sytue-docs:/?path=%2Fa"} {
		u, _ = url.Parse(crudo)
		if _, _, ok := DestinoDeEnlace(u); ok {
			t.Errorf("%s no es un enlace a una carpeta de Drive", crudo)
		}
	}
}
