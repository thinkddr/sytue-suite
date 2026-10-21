// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// El PUT del servicio SUSTITUYE el contacto: editar el nombre no puede llevarse por
// delante el segundo teléfono, la dirección ni el cumpleaños.
func TestEditarUnContactoNoPierdeNada(t *testing.T) {
	cumple := time.Date(1815, 12, 10, 0, 0, 0, 0, time.UTC)
	original := Contact{
		ID: "c1", FormattedName: "Ada Lovelace", Organization: "Analytical Engines", Birthday: &cumple, ETag: "11111111-1111-1111-1111-111111111111",
		Emails:    []ContactEmail{{Email: "ada@example.org", IsPrimary: true}, {Email: "ada@casa.example", Type: "home"}},
		Phones:    []ContactPhone{{Phone: "+34 600 000 000", Type: "cell", IsPrimary: true}},
		Addresses: []ContactAddress{{Street: "12 St James's Square", Locality: "London", Country: "UK"}},
	}
	var recibido ContactInput
	var ifMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/contacts/c1" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		ifMatch = r.Header.Get("If-Match")
		cuerpo, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(cuerpo, &recibido)
		_ = json.NewEncoder(w).Encode(original)
	}))
	defer srv.Close()

	in := original.Input()
	in.FormattedName = "Ada King"
	if _, err := NewContacts(srv.URL, nil, nil).Update(context.Background(), "c1", in, original.ETag); err != nil {
		t.Fatal(err)
	}
	if recibido.FormattedName != "Ada King" || len(recibido.Emails) != 2 || len(recibido.Phones) != 1 ||
		len(recibido.Addresses) != 1 || recibido.Birthday != "1815-12-10" || recibido.Organization != "Analytical Engines" {
		t.Fatalf("al servicio le llega un contacto recortado: %+v", recibido)
	}
	if ifMatch != `"`+original.ETag+`"` {
		t.Fatalf("If-Match = %q: sin él, una edición pisa la de otro dispositivo", ifMatch)
	}
}

func TestContactosPaginaYBusca(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/api/v1/contacts/all?limit=2&offset=0":
			_, _ = io.WriteString(w, `{"contacts":[{"id":"a","given_name":"Ada","family_name":"Lovelace"},{"id":"b","organization":"ACME"}],"next_offset":2}`)
		case "/api/v1/contacts/all?limit=2&offset=2":
			_, _ = io.WriteString(w, `{"contacts":[{"id":"c","formatted_name":"Grace Hopper"}]}`)
		case "/api/v1/contacts?limit=5&q=gra":
			_, _ = io.WriteString(w, `{"contacts":[{"id":"c","formatted_name":"Grace Hopper"}]}`)
		default:
			t.Errorf("petición inesperada: %s?%s", r.URL.Path, r.URL.RawQuery)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewContacts(srv.URL, nil, nil)
	primera, siguiente, err := c.All(context.Background(), 2, 0)
	if err != nil || siguiente != 2 || len(primera) != 2 {
		t.Fatalf("primera página: %v, siguiente %d, %v", primera, siguiente, err)
	}
	// El nombre que se enseña es el del servicio: FN, o nombre y apellido, o la organización.
	if primera[0].DisplayName() != "Ada Lovelace" || primera[1].DisplayName() != "ACME" {
		t.Fatalf("nombres: %q, %q", primera[0].DisplayName(), primera[1].DisplayName())
	}
	if _, siguiente, _ = c.All(context.Background(), 2, 2); siguiente != 0 {
		t.Fatalf("la última página dice que hay otra: %d", siguiente)
	}
	if hallados, err := c.Search(context.Background(), "gra", 5); err != nil || len(hallados) != 1 {
		t.Fatalf("búsqueda: %v, %v", hallados, err)
	}
}

// TestLaVCardLlegaTalCual: «Exportar» guarda la vCard del servicio, byte a byte, y la
// ruta es la que monta services/contacts.
func TestLaVCardLlegaTalCual(t *testing.T) {
	const tarjeta = "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada Lovelace\r\nPHOTO:data:image/png;base64,AAA\r\nEND:VCARD\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/api/v1/contacts/c%201/vcard" {
			t.Errorf("petición a %s", got)
		}
		w.Header().Set("Content-Type", "text/vcard")
		_, _ = io.WriteString(w, tarjeta)
	}))
	defer srv.Close()
	got, err := NewContacts(srv.URL, nil, nil).VCard(context.Background(), "c 1")
	if err != nil || string(got) != tarjeta {
		t.Fatalf("vCard %q, %v", got, err)
	}
	if !strings.Contains(webref.Read(t, "services/contacts/internal/http/server.go"), `api.Get("/contacts/{id}/vcard", h.ExportContactVCard)`) {
		t.Error("services/contacts ya no monta /contacts/{id}/vcard")
	}
}

// TestLasRutasDeEtiquetasLibretasYCSVSonLasDelServicio: cada llamada nueva pega en la
// ruta y con el cuerpo que monta services/contacts.
func TestLasRutasDeEtiquetasLibretasYCSVSonLasDelServicio(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(cuerpo)))
		switch r.URL.Path {
		case "/api/v1/address-books/l1/import-csv":
			_, _ = io.WriteString(w, `{"imported":3,"skipped":1}`)
		case "/api/v1/export-csv":
			w.Header().Set("Content-Disposition", `attachment; filename="contactos.csv"`)
			_, _ = io.WriteString(w, "Name,E-mail 1 - Value\r\n")
		default:
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_, _ = io.WriteString(w, "{}")
		}
	}))
	defer srv.Close()
	c, ctx := NewContacts(srv.URL, nil, nil), context.Background()
	if got, err := c.Contact(ctx, "c1"); err != nil || got.ID != "" {
		t.Fatalf("contacto %+v, %v", got, err)
	}
	if err := c.DeleteGroup(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if err := c.BatchGroupMembers(ctx, "g1", []string{"u1", "u2"}, true); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteAddressBook(ctx, "l2"); err != nil {
		t.Fatal(err)
	}
	if n, o, err := c.ImportCSV(ctx, "l1", []byte("Name\r\nAda\r\n")); err != nil || n != 3 || o != 1 {
		t.Fatalf("importados %d, omitidos %d, %v", n, o, err)
	}
	if nombre, csv, err := c.ExportCSV(ctx); err != nil || nombre != "contactos.csv" || !strings.HasPrefix(string(csv), "Name,") {
		t.Fatalf("csv %q (%q), %v", csv, nombre, err)
	}
	quiero := []string{
		"GET /api/v1/contacts/c1 ",
		"DELETE /api/v1/groups/g1 ",
		`POST /api/v1/groups/g1/members {"add":true,"uids":["u1","u2"]}`,
		"DELETE /api/v1/address-books/l2 ",
		"POST /api/v1/address-books/l1/import-csv Name\r\nAda",
		"GET /api/v1/export-csv ",
	}
	if strings.Join(vistas, "|") != strings.Join(quiero, "|") {
		t.Errorf("peticiones:\n%s\ny el servicio espera:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
	rutas := webref.Read(t, "services/contacts/internal/http/server.go")
	for _, ruta := range []string{
		`api.Delete("/groups/{id}", h.DeleteGroup)`, `api.Post("/groups/{id}/members", h.BatchGroupMembers)`,
		`api.Delete("/address-books/{id}", h.DeleteAddressBook)`, `api.Post("/address-books/{id}/import-csv", h.ImportCSV)`,
		`api.Get("/export-csv", h.ExportCSV)`, `api.Get("/contacts/{id}", h.GetContact)`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/contacts ya no monta %s", ruta)
		}
	}
}

// TestLaFotoVaPorUIDYSinFotoNoEsUnError: GET, PUT y DELETE de /contacts/photo con el
// UID limpio; un 404 es «no tiene foto», no un fallo.
func TestLaFotoVaPorUIDYSinFotoNoEsUnError(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("uid") == "sin":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet:
			_, _ = io.WriteString(w, "PNG")
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(w, `{"photo":true}`)
		}
	}))
	defer srv.Close()
	c, ctx := NewContacts(srv.URL, nil, nil), context.Background()
	if b, err := c.Photo(ctx, "urn:uuid:u1"); err != nil || string(b) != "PNG" {
		t.Fatalf("foto %q, %v", b, err)
	}
	if b, err := c.Photo(ctx, "sin"); err != nil || b != nil {
		t.Fatalf("sin foto: %q, %v", b, err)
	}
	if err := c.SetPhoto(ctx, "u1", "data:image/png;base64,AA=="); err != nil {
		t.Fatal(err)
	}
	if err := c.RemovePhoto(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/contacts/photo?uid=u1 ", "GET /api/v1/contacts/photo?uid=sin ",
		`PUT /api/v1/contacts/photo? {"data":"data:image/png;base64,AA==","uid":"u1"}`,
		"DELETE /api/v1/contacts/photo?uid=u1 ",
	}
	if strings.Join(vistas, "|") != strings.Join(quiero, "|") {
		t.Errorf("peticiones:\n%s", strings.Join(vistas, "\n"))
	}
}

func TestLosCamposDeContactsSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/contacts/internal/domain/domain.go")
	for _, campo := range []string{
		`json:"formatted_name"`, `json:"address_book_id"`, `json:"is_primary"`, `json:"postal_code,omitempty"`,
		`json:"photo_url,omitempty"`, `json:"birthday,omitempty"`, `json:"etag"`, `json:"is_default"`, `json:"display_name"`,
		`json:"has_photo"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/contacts ya no declara %s: revisar internal/api/contacts.go", campo)
		}
	}
	rutas := webref.Read(t, "services/contacts/internal/http/server.go")
	for _, ruta := range []string{`"/address-books"`, `"/address-books/{id}/contacts"`, `"/contacts/all"`, `"/contacts/{id}"`, `api.Get("/contacts", h.SearchContacts)`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/contacts ya no monta %s", ruta)
		}
	}
	manejadores := webref.Read(t, "services/contacts/internal/http/handlers.go")
	for _, literal := range []string{`"address_books"`, `"next_offset"`, `"contacts"`, "RFC3339 o YYYY-MM-DD"} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/contacts/internal/http/handlers.go ya no dice %s", literal)
		}
	}
}

// TestImportarLaTarjetaEnteraYLaOrganizacion: la tarjeta de fuera viaja TAL CUAL como
// `text/vcard` al alta; la libreta se pide entera (500, el tope del servicio); el
// listado dice quién tiene foto; y la agenda de Sytue ID trae todas las direcciones.
func TestImportarLaTarjetaEnteraYLaOrganizacion(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Content-Type")+" "+strings.TrimSpace(string(cuerpo)))
		switch r.URL.Path {
		case "/api/v1/address-books/l1/contacts":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"contacts":[{"id":"a","uid":"u-a","formatted_name":"Ana","has_photo":true},{"id":"b","formatted_name":"Bea"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"n","uid":"u-n","formatted_name":"Ada"}`)
		case "/api/v1/directory":
			_, _ = io.WriteString(w, `{"items":[{"user_id":"p1","email":"ana@acme.es","emails":["ana@acme.es","a@acme.es"],"name":"Ana","department":"Ventas","title":"Directora"}],"nextCursor":""}`)
		default:
			_, _ = io.WriteString(w, `{"id":"l9","name":"organizacion"}`)
		}
	}))
	defer srv.Close()
	c, ctx := NewContacts(srv.URL, nil, nil), context.Background()
	tarjeta := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada\r\nX-SOCIAL:x\r\nEND:VCARD"
	if got, err := c.ImportCard(ctx, "l1", tarjeta); err != nil || got.UID != "u-n" {
		t.Fatalf("importar: %+v, %v", got, err)
	}
	lista, err := c.InBook(ctx, "l1")
	if err != nil || len(lista) != 2 || !lista[0].HasPhoto || lista[1].HasPhoto {
		t.Fatalf("libreta %+v, %v", lista, err)
	}
	if _, err := c.CreateAddressBookAt(ctx, "organizacion", "Organización", "#8A6A2F"); err != nil {
		t.Fatal(err)
	}
	gente, err := NewDirectorio(srv.URL, nil, nil).Agenda(ctx, 500)
	if err != nil || len(gente) != 1 || len(gente[0].Emails) != 2 || gente[0].Departamento != "Ventas" || gente[0].Puesto != "Directora" {
		t.Fatalf("agenda %+v, %v", gente, err)
	}
	quiero := []string{
		"POST /api/v1/address-books/l1/contacts text/vcard; charset=utf-8 " + tarjeta,
		"GET /api/v1/address-books/l1/contacts?limit=500  ",
		`POST /api/v1/address-books application/json {"color":"#8A6A2F","display_name":"Organización","name":"organizacion"}`,
		"GET /api/v1/directory?limit=500  ",
	}
	if strings.Join(vistas, "|") != strings.Join(quiero, "|") {
		t.Errorf("peticiones:\n%s\ny se esperaban:\n%s", strings.Join(vistas, "\n"), strings.Join(quiero, "\n"))
	}
	// El servicio acepta la tarjeta por el mismo alta.
	if m := webref.Read(t, "services/contacts/internal/http/handlers.go"); !strings.Contains(m, `tipo == "text/vcard"`) {
		t.Error("services/contacts ya no acepta text/vcard en el alta")
	}
}
