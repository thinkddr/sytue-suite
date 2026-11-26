// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// # El contrato de Contacts (PLAN-EDICION-LOCAL §8.2)
//
// El MISMO juego de pruebas de comportamiento contra las dos implementaciones de la
// libreta: el repositorio de Contacts sobre el almacén de registros (la de antes, la que
// sigue usando la edición conectada sobre su motor) y la libreta SQLite de Sytue Office.
// Cada prueba corre en las dos (porCadaUna); las diferenciales (TestContratoDaLoMismo…)
// hacen lo mismo en las dos y comparan lo que sale.

// datos es lo que se prueba: la fachada de la pantalla, sus opcionales y lo de las
// cuentas.
type datos interface {
	api.ContactsData
	api.ContactsBatchData
	api.ContactsPhotoReader
	contacts.LibretasExternas
}

// implementacion abre (o vuelve a abrir) los contactos de una carpeta.
type implementacion struct {
	nombre string
	abrir  func(t *testing.T, dir string) (datos, func())
}

const sujeto = "instalacion-1"

var implementaciones = []implementacion{
	{"registros", func(t *testing.T, dir string) (datos, func()) {
		t.Helper()
		rec, err := almacen.Abrir(context.Background(), dir, "contacts", sujeto)
		if err != nil {
			t.Fatal(err)
		}
		repo, err := contacts.New(rec.Config)
		if err != nil {
			rec.Cerrar()
			t.Fatal(err)
		}
		if err := almacen.Preparar(t.Context(), repo); err != nil {
			t.Fatal(err)
		}
		return repo, func() { repo.Close(); rec.Cerrar() }
	}},
	{"sqlite", func(t *testing.T, dir string) (datos, func()) {
		t.Helper()
		x, err := libreta.Abrir(t.Context(), dir, sujeto)
		if err != nil {
			t.Fatal(err)
		}
		return x, func() { _ = x.Cerrar() }
	}},
}

// porCadaUna corre prueba en cada implementación, con una carpeta nueva.
func porCadaUna(t *testing.T, prueba func(t *testing.T, imp implementacion, d datos)) {
	t.Helper()
	for _, imp := range implementaciones {
		t.Run(imp.nombre, func(t *testing.T) {
			d, cerrar := imp.abrir(t, t.TempDir())
			defer cerrar()
			prueba(t, imp, d)
		})
	}
}

func estado(t *testing.T, err error, quiero int, que string) {
	t.Helper()
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Status != quiero {
		t.Fatalf("%s: quería %d y fue %v", que, quiero, err)
	}
}

func libretaNueva(t *testing.T, d datos, nombre string) string {
	t.Helper()
	b, err := d.CreateAddressBook(t.Context(), nombre)
	if err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func alta(t *testing.T, d datos, libro string, in api.ContactInput) api.Contact {
	t.Helper()
	c, err := d.Create(t.Context(), libro, in)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func nombres(cs []api.Contact) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.FormattedName
	}
	return out
}

func TestContratoAltaEdicionYBusqueda(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		ana := alta(t, d, libro, api.ContactInput{
			FormattedName: "Ana García", GivenName: "Ana", FamilyName: "García",
			Emails: []api.ContactEmail{{Email: "ana@ejemplo.org", IsPrimary: true}}, Phones: []api.ContactPhone{{Phone: "+34 600 111 222"}},
			Birthday: "1990-02-03", Note: "Conocida en la feria",
		})
		alta(t, d, libro, api.ContactInput{FormattedName: "Íñigo Ruiz", GivenName: "Íñigo", FamilyName: "Ruiz", Organization: "Bodegas del Duero"})
		alta(t, d, libro, api.ContactInput{FormattedName: "Zoe Álvarez", GivenName: "Zoe", FamilyName: "Álvarez"})

		todos, siguiente, err := d.All(ctx, 100, 0)
		if err != nil || siguiente != 0 || !slices.Equal(nombres(todos), []string{"Ana García", "Íñigo Ruiz", "Zoe Álvarez"}) {
			t.Fatalf("All = %v, %d, %v", nombres(todos), siguiente, err)
		}
		pagina, siguiente, err := d.All(ctx, 2, 0)
		if err != nil || siguiente != 2 || len(pagina) != 2 {
			t.Fatalf("primera página = %v, %d, %v", nombres(pagina), siguiente, err)
		}
		if pagina, siguiente, err = d.All(ctx, 2, 2); err != nil || siguiente != 0 || len(pagina) != 1 {
			t.Fatalf("segunda página = %v, %d, %v", nombres(pagina), siguiente, err)
		}
		for q, quiero := range map[string][]string{
			"garcia": {"Ana García"}, "INIGO": {"Íñigo Ruiz"}, "duero": {"Íñigo Ruiz"}, "ejemplo.org": {"Ana García"},
			"600 111": {"Ana García"}, "feria": {"Ana García"}, "zoe": {"Zoe Álvarez"}, "a": {"Ana García", "Íñigo Ruiz", "Zoe Álvarez"}, "nadie": {},
		} {
			hits, err := d.Search(ctx, q, 10)
			if err != nil || !slices.Equal(nombres(hits), quiero) {
				t.Fatalf("buscar %q = %v, %v", q, nombres(hits), err)
			}
		}
		_, err = d.Search(ctx, "   ", 10)
		estado(t, err, http.StatusBadRequest, "buscar vacío")

		in := ana.Input()
		in.Note = "Editada"
		in.Emails = append(in.Emails, api.ContactEmail{Email: "ana@trabajo.es", Type: "work"})
		editada, err := d.Update(ctx, ana.ID, in, ana.ETag)
		if err != nil || editada.Note != "Editada" || len(editada.Emails) != 2 || editada.ETag == ana.ETag || editada.Birthday == nil {
			t.Fatalf("editar = %+v, %v", editada, err)
		}
		_, err = d.Update(ctx, ana.ID, in, ana.ETag)
		estado(t, err, http.StatusPreconditionFailed, "editar con la etiqueta vieja")
		_, err = d.Update(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21", in, "")
		estado(t, err, http.StatusNotFound, "editar uno que no existe")
		leida, err := d.Contact(ctx, ana.ID)
		if err != nil || leida.Note != "Editada" || leida.ETag != editada.ETag {
			t.Fatalf("leer = %+v, %v", leida, err)
		}
		_, err = d.Contact(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21")
		estado(t, err, http.StatusNotFound, "leer uno que no existe")
	})
}

const tarjetaConExtras = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:ada-1815\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nEMAIL:ada@ejemplo.org\r\nBDAY:--1210\r\nX-GUARDAR:esto se queda\r\nEND:VCARD\r\n"

func TestContratoUnaVCardConservaLoQueNoSeEnsena(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		// Importar una vCard le da un UID nuevo (la misma dos veces son dos contactos).
		ada, err := d.ImportCard(ctx, libro, tarjetaConExtras)
		if err != nil || ada.UID == "ada-1815" || ada.BirthdayNoYear == "" {
			t.Fatalf("importar = %+v, %v", ada, err)
		}
		if otra, err := d.ImportCard(ctx, libro, tarjetaConExtras); err != nil || otra.UID == ada.UID {
			t.Fatalf("importar otra vez = %+v, %v", otra, err)
		}
		in := ada.Input()
		in.Title = "Matemática"
		if _, err := d.Update(ctx, ada.ID, in, ""); err != nil {
			t.Fatal(err)
		}
		vcard, err := d.VCard(ctx, ada.ID)
		if err != nil || !strings.Contains(string(vcard), "X-GUARDAR:esto se queda") || !strings.Contains(string(vcard), "Matemática") {
			t.Fatalf("vCard tras editar = %q, %v", vcard, err)
		}
		lote, err := d.BatchVCard(ctx, []string{ada.ID, "no-existe", ada.ID})
		if err != nil || strings.Count(string(lote), "BEGIN:VCARD") != 1 {
			t.Fatalf("lote = %q, %v", lote, err)
		}
	})
}

func TestContratoLibretas(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		personal := libretaNueva(t, d, "Personal")
		trabajo, err := d.CreateAddressBookAt(ctx, "trabajo", "Trabajo", "#112233")
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.CreateAddressBookAt(ctx, "TRABAJO", "Otra", "")
		estado(t, err, http.StatusConflict, "la misma ruta")
		_, err = d.CreateAddressBookAt(ctx, "x", "Mala", "rojo")
		estado(t, err, http.StatusBadRequest, "color inválido")
		ls, err := d.AddressBooks(ctx)
		if err != nil || len(ls) != 2 || ls[0].ID != personal || !ls[0].IsDefault || ls[1].ID != trabajo.ID {
			t.Fatalf("libretas = %+v, %v", ls, err)
		}
		if r, err := d.RenameAddressBook(ctx, trabajo.ID, "Oficina"); err != nil || r.DisplayName != "Oficina" {
			t.Fatalf("renombrar = %+v, %v", r, err)
		}
		alta(t, d, trabajo.ID, api.ContactInput{FormattedName: "Bea Torres"})
		borrado := alta(t, d, trabajo.ID, api.ContactInput{FormattedName: "Carla Gil"})
		if err := d.Delete(ctx, borrado.ID); err != nil {
			t.Fatal(err)
		}
		alta(t, d, personal, api.ContactInput{FormattedName: "Dani Sanz"})
		estado(t, d.DeleteAddressBook(ctx, personal), http.StatusBadRequest, "borrar la predeterminada")
		if err := d.DeleteAddressBook(ctx, trabajo.ID); err != nil {
			t.Fatal(err)
		}
		estado(t, d.DeleteAddressBook(ctx, trabajo.ID), http.StatusNotFound, "borrarla otra vez")
		todos, _, err := d.All(ctx, 100, 0)
		if err != nil || !slices.Equal(nombres(todos), []string{"Dani Sanz"}) {
			t.Fatalf("tras borrar la libreta = %v, %v", nombres(todos), err)
		}
		if tr, err := d.Trash(ctx, 50); err != nil || len(tr) != 0 {
			t.Fatalf("la papelera enseña lo de una libreta borrada: %+v, %v", tr, err)
		}
		if ok, err := d.ExisteLibreta(ctx, trabajo.ID); err != nil || ok {
			t.Fatal("la libreta borrada sigue", ok, err)
		}
		_, err = d.Groups(ctx, trabajo.ID)
		estado(t, err, http.StatusNotFound, "etiquetas de una libreta borrada")
		_, err = d.InBookPage(ctx, trabajo.ID, "")
		estado(t, err, http.StatusNotFound, "página de una libreta borrada")
		_, err = d.Create(ctx, trabajo.ID, api.ContactInput{FormattedName: "Nadie"})
		estado(t, err, http.StatusNotFound, "alta en una libreta borrada")
	})
}

func TestContratoEtiquetas(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		ana := alta(t, d, libro, api.ContactInput{FormattedName: "Ana"})
		bea := alta(t, d, libro, api.ContactInput{FormattedName: "Bea"})
		amigos, err := d.CreateGroup(ctx, libro, "amigos")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.CreateGroup(ctx, libro, "Bodegas"); err != nil {
			t.Fatal(err)
		}
		_, err = d.CreateGroup(ctx, libro, "  ")
		estado(t, err, http.StatusBadRequest, "etiqueta sin nombre")
		g, err := d.RenameGroup(ctx, amigos.ID, "Amigos", []string{ana.UID, "urn:uuid:remoto", ana.UID})
		if err != nil || g.Name != "Amigos" || !slices.Equal(g.Members, sorted(ana.UID, "remoto")) {
			t.Fatalf("renombrar = %+v, %v", g, err)
		}
		if err := d.BatchGroupMembers(ctx, amigos.ID, []string{bea.UID}, true); err != nil {
			t.Fatal(err)
		}
		if err := d.BatchGroupMembers(ctx, amigos.ID, []string{"remoto"}, false); err != nil {
			t.Fatal(err)
		}
		gs, err := d.Groups(ctx, libro)
		if err != nil || len(gs) != 2 || gs[0].Name != "Amigos" || gs[1].Name != "Bodegas" || !slices.Equal(gs[0].Members, sorted(ana.UID, bea.UID)) {
			t.Fatalf("etiquetas = %+v, %v", gs, err)
		}
		// Eliminar para siempre a un miembro lo deja como UID sin resolver.
		if err := d.Delete(ctx, ana.ID); err != nil {
			t.Fatal(err)
		}
		if err := d.Purge(ctx, ana.ID); err != nil {
			t.Fatal(err)
		}
		gs, err = d.Groups(ctx, libro)
		if err != nil || !slices.Equal(gs[0].Members, sorted(ana.UID, bea.UID)) {
			t.Fatalf("etiquetas tras eliminar a un miembro = %+v, %v", gs, err)
		}
		if err := d.DeleteGroup(ctx, amigos.ID); err != nil {
			t.Fatal(err)
		}
		estado(t, d.DeleteGroup(ctx, amigos.ID), http.StatusNotFound, "borrarla otra vez")
		_, err = d.RenameGroup(ctx, amigos.ID, "X", nil)
		estado(t, err, http.StatusNotFound, "renombrar una borrada")
		if gs, err := d.Groups(ctx, libro); err != nil || len(gs) != 1 {
			t.Fatalf("tras borrar = %+v, %v", gs, err)
		}
		if c, err := d.Contact(ctx, bea.ID); err != nil || c.FormattedName != "Bea" {
			t.Fatal("borrar la etiqueta tocó a sus contactos", err)
		}
	})
}

func sorted(v ...string) []string {
	v = slices.Clone(v)
	slices.Sort(v)
	return v
}

func TestContratoFavoritosYPapelera(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		otra := libretaNueva(t, d, "Otra")
		ana := alta(t, d, libro, api.ContactInput{FormattedName: "Ana", Emails: []api.ContactEmail{{Email: "a@x.es"}, {Email: "ana@x.es", IsPrimary: true}}})
		bea := alta(t, d, libro, api.ContactInput{FormattedName: "Bea"})
		cris := alta(t, d, libro, api.ContactInput{FormattedName: "Cris"})
		estado(t, d.SetFavorite(ctx, "no-existe", true), http.StatusNotFound, "destacar a nadie")
		estado(t, d.SetFavorite(ctx, " ", true), http.StatusNotFound, "destacar un UID vacío")
		if err := d.BatchFavorite(ctx, []string{"no-existe"}, true); err != nil {
			t.Fatal("en lote, uno que no está no es un error:", err)
		}
		if err := d.SetFavorite(ctx, cris.UID, true); err != nil {
			t.Fatal(err)
		}
		if err := d.BatchFavorite(ctx, []string{ana.UID, "urn:uuid:" + bea.UID}, true); err != nil {
			t.Fatal(err)
		}
		if err := d.SetFavorite(ctx, bea.UID, false); err != nil {
			t.Fatal(err)
		}
		fs, err := d.Favorites(ctx)
		if err != nil || !slices.Equal(fs, []string{ana.UID, cris.UID}) {
			t.Fatalf("favoritos = %v, %v", fs, err)
		}
		// Papelera: borrar, deshacer en lote, restaurar y eliminar para siempre.
		if err := d.Delete(ctx, ana.ID); err != nil {
			t.Fatal(err)
		}
		estado(t, d.Delete(ctx, ana.ID), http.StatusNotFound, "borrarlo otra vez")
		if err := d.BatchTrash(ctx, []string{bea.UID, cris.UID}); err != nil {
			t.Fatal(err)
		}
		tr, err := d.Trash(ctx, 50)
		if err != nil || len(tr) != 3 || tr[2].ID != ana.ID || tr[2].PrimaryEmail != "ana@x.es" {
			t.Fatalf("papelera = %+v, %v", tr, err)
		}
		if fs, _ := d.Favorites(ctx); len(fs) != 0 {
			t.Fatal("los favoritos enseñan los de la papelera", fs)
		}
		n, err := d.BatchRestore(ctx, []string{bea.UID, "no-esta"})
		if err != nil || n != 1 {
			t.Fatal("deshacer en lote", n, err)
		}
		if err := d.Restore(ctx, ana.ID); err != nil {
			t.Fatal(err)
		}
		estado(t, d.Restore(ctx, ana.ID), http.StatusNotFound, "restaurar uno vivo")
		estado(t, d.Purge(ctx, ana.ID), http.StatusNotFound, "eliminar uno vivo")
		if err := d.Purge(ctx, cris.ID); err != nil {
			t.Fatal(err)
		}
		todos, _, err := d.All(ctx, 100, 0)
		if err != nil || !slices.Equal(nombres(todos), []string{"Ana", "Bea"}) {
			t.Fatalf("tras la papelera = %v, %v", nombres(todos), err)
		}
		if fs, _ := d.Favorites(ctx); !slices.Equal(fs, []string{ana.UID}) {
			t.Fatal("favoritos tras volver de la papelera", fs)
		}
		if tr, _ := d.Trash(ctx, 50); len(tr) != 0 {
			t.Fatal("papelera tras eliminar", tr)
		}
		// El índice de favoritos es por UID: un contacto que llega después con el UID
		// de uno destacado (aquí, en otra libreta) llega destacado.
		if _, err := d.PonerTarjetaCruda(ctx, otra, "", "", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:"+cris.UID+"\r\nFN:Cris de nuevo\r\nEND:VCARD\r\n"); err != nil {
			t.Fatal(err)
		}
		if fs, _ := d.Favorites(ctx); !slices.Equal(fs, []string{ana.UID, cris.UID}) {
			t.Fatal("el índice de favoritos no se aplica a lo que llega después", fs)
		}
	})
}

// fotoPNG es una foto de 1×1 en data URI.
const fotoPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestContratoFotos(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		otra := libretaNueva(t, d, "Otra")
		ana := alta(t, d, libro, api.ContactInput{FormattedName: "Ana"})
		_, err := d.Photo(ctx, ana.UID)
		estado(t, err, http.StatusNotFound, "sin foto")
		if err := d.SetPhoto(ctx, ana.UID, fotoPNG); err != nil {
			t.Fatal(err)
		}
		if err := d.SetPhoto(ctx, ana.UID, "data:text/plain;base64,aG9sYQ=="); err == nil {
			t.Fatal("una foto que no es imagen")
		}
		con, err := d.Contact(ctx, ana.ID)
		if err != nil || !con.HasPhoto || con.ETag == ana.ETag {
			t.Fatalf("con foto = %+v, %v", con, err)
		}
		if f, err := d.Photo(ctx, "urn:uuid:"+ana.UID); err != nil || len(f) == 0 {
			t.Fatal("foto por UID", err)
		}
		if f, err := d.PhotoByID(ctx, ana.ID, con.ETag); err != nil || len(f) == 0 {
			t.Fatal("foto por id", err)
		}
		_, err = d.PhotoByID(ctx, ana.ID, ana.ETag)
		estado(t, err, http.StatusPreconditionFailed, "foto con la etiqueta vieja")
		_, err = d.PhotoByID(ctx, ana.ID, " ")
		estado(t, err, http.StatusBadRequest, "foto sin etiqueta")
		// El mismo UID en dos libretas: por UID es ambiguo, por id no.
		copia, err := d.PonerTarjetaCruda(ctx, otra, "", "", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:"+ana.UID+"\r\nFN:Ana copia\r\nEND:VCARD\r\n")
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Photo(ctx, ana.UID)
		estado(t, err, http.StatusConflict, "foto de un UID repetido")
		estado(t, d.RemovePhoto(ctx, ana.UID), http.StatusConflict, "quitar la foto de un UID repetido")
		if err := d.Delete(ctx, copia.ID); err != nil {
			t.Fatal(err)
		}
		if err := d.RemovePhoto(ctx, ana.UID); err != nil {
			t.Fatal(err)
		}
		if c, _ := d.Contact(ctx, ana.ID); c.HasPhoto {
			t.Fatal("la foto sigue")
		}
	})
}

func TestContratoPaginasDeUnaLibreta(t *testing.T) {
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		otra := libretaNueva(t, d, "Otra")
		for i := range 230 {
			alta(t, d, libro, api.ContactInput{FormattedName: fmt.Sprintf("Persona %03d", i), FamilyName: fmt.Sprintf("Apellido %03d", 229-i)})
		}
		alta(t, d, otra, api.ContactInput{FormattedName: "De la otra"})
		fachada, err := api.NewContacts("", nil, nil).WithPrivateData(d)
		if err != nil {
			t.Fatal(err)
		}
		todos, err := fachada.InBook(ctx, libro)
		if err != nil || len(todos) != 230 || todos[0].FormattedName != "Persona 229" || todos[229].FormattedName != "Persona 000" {
			t.Fatalf("la libreta entera = %d, %v", len(todos), err)
		}
		p1, err := d.InBookPage(ctx, libro, "")
		if err != nil || len(p1.Contacts) != 100 || p1.NextCursor == "" {
			t.Fatalf("primera página = %d, %q, %v", len(p1.Contacts), p1.NextCursor, err)
		}
		p2, err := d.InBookPage(ctx, libro, p1.NextCursor)
		if err != nil || len(p2.Contacts) != 100 || p2.Contacts[0].FormattedName != "Persona 129" {
			t.Fatalf("segunda página = %d, %v", len(p2.Contacts), err)
		}
		_, err = d.InBookPage(ctx, libro, "roto")
		estado(t, err, http.StatusBadRequest, "un cursor roto")
		_, err = d.InBookPage(ctx, libro, p1.Contacts[99].ID+"."+strings.Repeat("0", 64))
		estado(t, err, http.StatusConflict, "un cursor de otra libreta o de antes")
		// Un cambio de la libreta entre dos páginas es un conflicto, no una lista a medias.
		alta(t, d, libro, api.ContactInput{FormattedName: "Nueva"})
		_, err = d.InBookPage(ctx, libro, p2.NextCursor)
		estado(t, err, http.StatusConflict, "la libreta cambió entre dos páginas")
	})
}

func TestContratoCSV(t *testing.T) {
	csv := "Name,Given Name,Family Name,E-mail 1 - Value,Phone 1 - Value\r\n" +
		"Ana Ruiz,Ana,Ruiz,ana@x.es,600111222\r\n" +
		"Bea Gil,Bea,Gil,BEA@x.es ,\r\n" +
		"Bea Otra,Bea,Otra,bea@x.es,\r\n" +
		",,,,\r\n"
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		libro := libretaNueva(t, d, "Personal")
		alta(t, d, libro, api.ContactInput{FormattedName: "Ya estaba", Emails: []api.ContactEmail{{Email: "Ana@X.es"}}})
		hechos, saltados, err := d.ImportCSV(ctx, libro, []byte(csv))
		if err != nil || hechos != 1 || saltados != 3 {
			t.Fatalf("importar = %d, %d, %v", hechos, saltados, err)
		}
		if hechos, saltados, err = d.ImportCSV(ctx, libro, []byte(csv)); err != nil || hechos != 0 || saltados != 4 {
			t.Fatalf("importar otra vez = %d, %d, %v", hechos, saltados, err)
		}
		_, _, err = d.ImportCSV(ctx, "6f1d2c3b-5a49-4e87-9c10-0d6b7a8e9f21", []byte(csv))
		estado(t, err, http.StatusNotFound, "importar en una libreta que no existe")
		_, _, err = d.ImportCSV(ctx, libro, make([]byte, 5<<20+1))
		estado(t, err, http.StatusRequestEntityTooLarge, "un CSV demasiado grande")
		nombre, salida, err := d.ExportCSV(ctx)
		if err != nil || nombre != "contacts.csv" || !strings.Contains(string(salida), "Bea Gil") || !strings.Contains(string(salida), "Ya estaba") {
			t.Fatalf("exportar = %q %q, %v", nombre, salida, err)
		}
	})
}

func TestContratoLoDeLasCuentas(t *testing.T) {
	const grupo = "BEGIN:VCARD\r\nVERSION:4.0\r\nKIND:group\r\nUID:grupo-1\r\nFN:Vecinos\r\nMEMBER:urn:uuid:persona-1\r\nX-OTRO:se queda\r\nEND:VCARD\r\n"
	porCadaUna(t, func(t *testing.T, _ implementacion, d datos) {
		ctx := t.Context()
		lb, err := d.CrearLibretaExterna(ctx, "  ")
		if err != nil || lb.DisplayName != "Contactos" || !strings.HasPrefix(lb.Name, "externa-") {
			t.Fatalf("libreta externa = %+v, %v", lb, err)
		}
		libro := lb.ID
		tarjeta := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:urn:uuid:persona-1\r\nFN:Persona Uno\r\nEMAIL:uno@x.es\r\nEND:VCARD\r\n"
		puesta, err := d.PonerTarjetaCruda(ctx, libro, "", "", tarjeta)
		if err != nil || puesta.ID == "" || !strings.Contains(puesta.Datos, "Persona Uno") {
			t.Fatalf("poner = %+v, %v", puesta, err)
		}
		_, err = d.PonerTarjetaCruda(ctx, libro, "", "", tarjeta)
		estado(t, err, http.StatusConflict, "el mismo UID dos veces")
		cambiada := strings.Replace(tarjeta, "Persona Uno", "Persona Uno Bis", 1)
		_, err = d.PonerTarjetaCruda(ctx, libro, puesta.ID, "otra cosa", cambiada)
		if !errors.Is(err, contacts.ErrCambioLocal) {
			t.Fatal("con otra vCard esperada:", err)
		}
		if _, err := d.PonerTarjetaCruda(ctx, libro, puesta.ID, "", strings.Replace(tarjeta, "persona-1", "otra-persona", 1)); err == nil {
			t.Fatal("cambiar el UID")
		}
		if puesta, err = d.PonerTarjetaCruda(ctx, libro, puesta.ID, puesta.Datos, cambiada); err != nil || !strings.Contains(puesta.Datos, "Bis") {
			t.Fatalf("reemplazar = %+v, %v", puesta, err)
		}
		ts, err := d.TarjetasCrudas(ctx, libro)
		if err != nil || len(ts) != 1 || ts[0].Datos != puesta.Datos {
			t.Fatalf("tarjetas = %+v, %v", ts, err)
		}
		g, err := d.PonerGrupoCrudo(ctx, libro, "", "", grupo)
		if err != nil || g.UID != "grupo-1" || !strings.Contains(g.Datos, "X-OTRO:se queda") {
			t.Fatalf("poner grupo = %+v, %v", g, err)
		}
		_, err = d.PonerGrupoCrudo(ctx, libro, "", "", grupo)
		estado(t, err, http.StatusConflict, "el mismo grupo dos veces")
		gs, err := d.Groups(ctx, libro)
		if err != nil || len(gs) != 1 || !slices.Equal(gs[0].Members, []string{"persona-1"}) {
			t.Fatalf("etiquetas = %+v, %v", gs, err)
		}
		if g, err = d.PonerGrupoCrudo(ctx, libro, g.ID, g.Datos, strings.Replace(grupo, "Vecinos", "Vecindario", 1)); err != nil {
			t.Fatal(err)
		}
		if gc, err := d.GruposCrudos(ctx, libro); err != nil || len(gc) != 1 || !strings.Contains(gc[0].Datos, "Vecindario") {
			t.Fatalf("grupos = %+v, %v", gc, err)
		}
		if !errors.Is(d.BorrarGrupoCrudo(ctx, g.ID, "otra"), contacts.ErrCambioLocal) {
			t.Fatal("borrar un grupo cambiado")
		}
		if err := d.BorrarGrupoCrudo(ctx, g.ID, g.Datos); err != nil {
			t.Fatal(err)
		}
		if err := d.BorrarGrupoCrudo(ctx, g.ID, ""); err != nil {
			t.Fatal("borrar uno que ya no está no es un error:", err)
		}
		if !errors.Is(d.BorrarTarjetaCruda(ctx, puesta.ID, "otra"), contacts.ErrCambioLocal) {
			t.Fatal("borrar un contacto cambiado")
		}
		if err := d.BorrarTarjetaCruda(ctx, puesta.ID, puesta.Datos); err != nil {
			t.Fatal(err)
		}
		if err := d.BorrarTarjetaCruda(ctx, puesta.ID, ""); err != nil {
			t.Fatal("borrar uno que ya no está no es un error:", err)
		}
		if ts, err := d.TarjetasCrudas(ctx, libro); err != nil || len(ts) != 0 {
			t.Fatalf("tarjetas tras borrar = %+v, %v", ts, err)
		}
		if tr, err := d.Trash(ctx, 10); err != nil || len(tr) != 1 {
			t.Fatalf("borrar una tarjeta la manda a la papelera: %+v, %v", tr, err)
		}
	})
}

func TestContratoSobreviveAReabrir(t *testing.T) {
	for _, imp := range implementaciones {
		t.Run(imp.nombre, func(t *testing.T) {
			dir := t.TempDir()
			d, cerrar := imp.abrir(t, dir)
			ctx := t.Context()
			libro := libretaNueva(t, d, "Personal")
			ana := alta(t, d, libro, api.ContactInput{FormattedName: "Ana"})
			if err := d.SetFavorite(ctx, ana.UID, true); err != nil {
				t.Fatal(err)
			}
			cerrar()
			d, cerrar = imp.abrir(t, dir)
			defer cerrar()
			fs, err := d.Favorites(ctx)
			if err != nil || !slices.Equal(fs, []string{ana.UID}) {
				t.Fatal("tras reabrir", fs, err)
			}
		})
	}
}

// ----- Diferenciales: lo mismo en las dos -----

var (
	reUUID  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reFecha = regexp.MustCompile(`"\d{4}-\d{2}-\d{2}T[0-9:.]+(Z|[+-]\d{2}:\d{2})"`)
)

// normal es v en JSON sin lo que cambia de una implementación a otra: identificadores,
// etiquetas y horas.
func normal(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := reUUID.ReplaceAllString(string(raw), "<id>")
	return reFecha.ReplaceAllString(s, `"<fecha>"`)
}

// guion es una tanda de escrituras y consultas que hacen lo mismo en las dos; devuelve
// lo que sale de cada consulta, normalizado.
func guion(t *testing.T, d datos) []string {
	t.Helper()
	ctx := t.Context()
	var out []string
	anotar := func(que string, v any, err error) {
		out = append(out, que+": "+normal(t, v)+fmt.Sprint(" / ", err))
	}
	libro := libretaNueva(t, d, "Personal")
	otra := libretaNueva(t, d, "Trabajo")
	gente := []api.ContactInput{
		{FormattedName: "Ana García López", GivenName: "Ana", FamilyName: "García López", Emails: []api.ContactEmail{{Email: "ana@x.es", IsPrimary: true}}, Birthday: "1990-02-03"},
		{FormattedName: "Bea Núñez", GivenName: "Bea", FamilyName: "Núñez", Organization: "Bodegas", Phones: []api.ContactPhone{{Phone: "+34 600 000 001"}}},
		{FormattedName: "Ñoño", Nickname: "ñ", Note: "<b>nota</b> & más", BirthdayNoYear: "--12-10"},
		{FormattedName: "Carmen", GivenName: "Carmen", Addresses: []api.ContactAddress{{Street: "Calle Mayor 1", Locality: "Madrid", Country: "España"}}},
		{FormattedName: "", Organization: "Solo Empresa SL"},
	}
	creados := make([]api.Contact, 0, len(gente))
	for i, in := range gente {
		destino := libro
		if i == 3 {
			destino = otra
		}
		c, err := d.Create(ctx, destino, in)
		anotar("alta", c, err)
		creados = append(creados, c)
	}
	_, err := d.Create(ctx, libro, api.ContactInput{FormattedName: "Mal", Birthday: "1990-13-45"})
	anotar("alta mala", nil, err)
	c, err := d.ImportCard(ctx, libro, tarjetaConExtras)
	anotar("vcard", c, err)
	in := creados[0].Input()
	in.Phones = []api.ContactPhone{{Phone: "+34 911 000 000", Type: "work"}}
	c, err = d.Update(ctx, creados[0].ID, in, creados[0].ETag)
	anotar("editar", c, err)
	g, err := d.CreateGroup(ctx, libro, "Familia")
	anotar("etiqueta", g, err)
	anotar("miembros", nil, d.BatchGroupMembers(ctx, g.ID, []string{creados[1].UID, creados[2].UID, "fuera"}, true))
	anotar("destacar", nil, d.BatchFavorite(ctx, []string{creados[1].UID, creados[3].UID}, true))
	anotar("tirar", nil, d.Delete(ctx, creados[2].ID))
	anotar("foto", nil, d.SetPhoto(ctx, creados[1].UID, fotoPNG))
	hechos, saltados, err := d.ImportCSV(ctx, libro, []byte("Name,E-mail 1 - Value\r\nDiego,diego@x.es\r\nOtra Ana,ANA@x.es\r\n"))
	anotar("csv", []int{hechos, saltados}, err)

	todos, siguiente, err := d.All(ctx, 100, 0)
	anotar("todos", []any{todos, siguiente}, err)
	for _, q := range []string{"garcia", "ñ", "bodegas", "600", "x.es", "zz"} {
		hits, err := d.Search(ctx, q, 50)
		anotar("buscar "+q, hits, err)
	}
	ls, err := d.AddressBooks(ctx)
	anotar("libretas", ls, err)
	gs, err := d.Groups(ctx, libro)
	anotar("etiquetas", gs, err)
	fs, err := d.Favorites(ctx)
	anotar("favoritos", len(fs), err)
	tr, err := d.Trash(ctx, 10)
	anotar("papelera", tr, err)
	pagina, err := d.InBookPage(ctx, libro, "")
	anotar("página", pagina.Contacts, err)
	_, csv, err := d.ExportCSV(ctx)
	anotar("exportar", string(csv), err)
	return out
}

func TestContratoDaLoMismoEnLasDos(t *testing.T) {
	salidas := make([][]string, 0, len(implementaciones))
	for _, imp := range implementaciones {
		d, cerrar := imp.abrir(t, t.TempDir())
		salidas = append(salidas, guion(t, d))
		cerrar()
	}
	if len(salidas[0]) != len(salidas[1]) {
		t.Fatalf("salen %d y %d resultados", len(salidas[0]), len(salidas[1]))
	}
	for i := range salidas[0] {
		if salidas[0][i] != salidas[1][i] {
			t.Errorf("distinto:\n  registros: %s\n  sqlite:    %s", salidas[0][i], salidas[1][i])
		}
	}
}
