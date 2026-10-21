// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package portabilidad

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type contacto struct {
	ID      string    `json:"id"`
	Nombre  string    `json:"name"`
	Correos []string  `json:"emails,omitempty"`
	Nota    string    `json:"note,omitempty"`
	Alta    time.Time `json:"created_at"`
	Interno string    `json:"-"`
}

var filas = []contacto{
	{ID: "1", Nombre: "Ana <Núñez>", Correos: []string{"ana@x.es", "a@y.es"}, Alta: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), Interno: "no"},
	{ID: "2", Nombre: "Luis, el de \"arriba\"", Nota: "línea 1\nlínea 2"},
	{ID: "3", Nombre: "=HYPERLINK(\"https://x.example\")", Correos: []string{"@SUM(1)", "b@y"}, Nota: "-cmd"},
}

func TestTablaEnLosTresFormatos(t *testing.T) {
	var j bytes.Buffer
	if err := EscribirTabla(&j, JSON, "contacts", "contact", Todas(filas)); err != nil {
		t.Fatal(err)
	}
	var vuelta []map[string]any
	if err := json.Unmarshal(j.Bytes(), &vuelta); err != nil || len(vuelta) != 3 {
		t.Fatalf("JSON ilegible (%v): %s", err, j.String())
	}
	if _, ok := vuelta[0]["Interno"]; ok || strings.Contains(j.String(), `"no"`) {
		t.Fatal("un campo json:\"-\" no debe salir")
	}

	var c bytes.Buffer
	if err := EscribirTabla(&c, CSV, "contacts", "contact", Todas(filas)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.String(), "\ufeff") {
		t.Fatal("el CSV debe llevar BOM para que Excel lea las tildes")
	}
	registros, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(c.String(), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// La columna de un campo omitempty está aunque la primera fila no lo traiga.
	if got := strings.Join(registros[0], ","); got != "id,name,emails,note,created_at" {
		t.Fatalf("cabecera: %s", got)
	}
	if registros[1][2] != "ana@x.es; a@y.es" || registros[2][3] != "línea 1\nlínea 2" {
		t.Fatalf("celdas: %q", registros)
	}
	if registros[3][1] != "'=HYPERLINK(\"https://x.example\")" || registros[3][2] != "'@SUM(1); b@y" || registros[3][3] != "'-cmd" {
		t.Fatalf("una fórmula sale como texto (NEG-18): %q", registros[3])
	}

	var x bytes.Buffer
	if err := EscribirTabla(&x, XML, "contacts", "contact", Todas(filas)); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Contactos []struct {
			Nombre  string   `xml:"name"`
			Correos []string `xml:"emails>item"`
		} `xml:"contact"`
	}
	if err := xml.Unmarshal(x.Bytes(), &doc); err != nil {
		t.Fatalf("XML ilegible (%v): %s", err, x.String())
	}
	if len(doc.Contactos) != 3 || doc.Contactos[0].Nombre != "Ana <Núñez>" || len(doc.Contactos[0].Correos) != 2 {
		t.Fatalf("XML: %+v", doc)
	}
}

func TestEntregaDeIdaYVuelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := Nuevo(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = p.FicheroDeTamano("Mi carpeta/café.txt", time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC), 4, strings.NewReader("hola"))
		_ = Tabla(p, "contacts", "contact", Todas(filas))
		switch r.URL.Query().Get("fin") {
		case "fallo":
			_ = p.Fallo(errors.New("el almacén no responde"))
		case "corte":
			// Se va sin cerrar: lo que ve Hub cuando el producto se cae a mitad.
		default:
			_ = p.Cerrar()
		}
	}))
	defer srv.Close()

	leer := func(query string) ([]string, error) {
		resp, err := http.Get(srv.URL + "?" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var rutas []string
		for parte, err := range Leer(resp) {
			if err != nil {
				return rutas, err
			}
			b, _ := io.ReadAll(parte)
			if parte.Ruta == "Mi carpeta/café.txt" && (string(b) != "hola" || parte.Modificado.Day() != 2 || parte.Tamano != 4) {
				t.Fatalf("fichero alterado: %q %v %d", b, parte.Modificado, parte.Tamano)
			}
			if parte.Ruta == "contacts.csv" && parte.Tamano != -1 {
				t.Fatalf("una tabla no sabe su tamaño: %d", parte.Tamano)
			}
			rutas = append(rutas, parte.Ruta)
		}
		return rutas, nil
	}

	rutas, err := leer("format=csv")
	if err != nil || strings.Join(rutas, "|") != "Mi carpeta/café.txt|contacts.csv" {
		t.Fatalf("entrega: %v %v", rutas, err)
	}
	_, err = leer("format=xml&fin=fallo")
	var fallo *ErrProducto
	if !errors.As(err, &fallo) || fallo.Motivo != "el almacén no responde" {
		t.Fatalf("un fallo declarado debe llegar como ErrProducto: %v", err)
	}
	if _, err = leer("fin=corte"); err == nil {
		t.Fatal("una entrega sin cierre no puede darse por buena")
	}
	resp, err := http.Get(srv.URL + "?format=pdf")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("formato desconocido: %d", resp.StatusCode)
	}
}
