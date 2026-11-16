// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// servicioDetalles hace de Drive y de Sytue ID para «Detalles». `fallos` son las
// primeras peticiones del metadato que contestan 500.
func servicioDetalles(t *testing.T, fallos int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var metadatos atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/files/f1":
			if metadatos.Add(1) <= fallos {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = io.WriteString(w, `{"id":"f1","folder_id":"c2","name":"acta.odt","size_bytes":2048,
				"content_type":"application/vnd.oasis.opendocument.text","version":3,
				"owner_sub":"sub-ana","modified_by":"sub-x","created_at":"2026-03-02T10:00:00Z",
				"updated_at":"2026-09-14T08:30:00Z","path":"/Proyectos/2026/acta.odt",
				"label":{"name":"Confidencial","block_external_share":true}}`)
		case "GET /api/v1/files/f1/shares":
			_, _ = io.WriteString(w, `{"shares":[
				{"id":"s1","kind":"link","permission":"read"},
				{"id":"s2","kind":"link","permission":"read","expires_at":"2020-01-01T00:00:00Z"},
				{"id":"s3","kind":"user","subject":"sub-ana","permission":"read"},
				{"id":"s4","kind":"user","subject":"sub-ana","permission":"write"},
				{"id":"s5","kind":"user","subject":"sub-bea","permission":"read"},
				{"id":"s6","kind":"group","subject":"ventas","permission":"read"}]}`)
		case "GET /api/v1/folders/c3":
			_, _ = io.WriteString(w, `{"id":"c3","parent_id":"c2","name":"IVA","owner_sub":"sub-ana",
				"created_at":"2026-01-05T09:00:00Z","updated_at":"2026-02-01T09:00:00Z"}`)
		case "GET /api/v1/folders/c3/shares":
			// Un lector sin permiso para ver las comparticiones.
			w.WriteHeader(http.StatusForbidden)
		case "GET /api/v1/folders/c2":
			_, _ = io.WriteString(w, `{"id":"c2","parent_id":"c1","name":"2026"}`)
		case "GET /api/v1/folders/c1":
			_, _ = io.WriteString(w, `{"id":"c1","parent_id":"raiz","name":"Contabilidad"}`)
		case "GET /api/v1/folders/raiz":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case "POST /api/v1/directory:resolve":
			// Ana está en el directorio; quien modificó, no.
			if strings.Contains(string(cuerpo), "sub-ana") {
				_, _ = io.WriteString(w, `{"user_id":"sub-ana","email":"ana@acme.test","name":"Ana Ruiz"}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &metadatos
}

func fuentesDe(srv *httptest.Server) FuentesDetalles {
	svc := api.NewDrive(srv.URL, sesionDoble{}, nil)
	dir := api.NewDirectorio(srv.URL, sesionDoble{}, nil)
	return FuentesDetalles{
		Fichero: svc.File, Carpeta: svc.Folder, Shares: svc.Shares, Persona: dir.PorID,
		Raiz: "raiz", Ahora: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
	}
}

// TestDetallesDeUnFicheroComoEnLaWeb: las MISMAS filas y en el MISMO orden que
// `DetailsDialog.tsx`, con las reglas del BFF: el enlace caducado no cuenta, una persona
// con dos comparticiones es una, quien no está en el directorio es «Alguien» (nunca su
// `sub`) y la etiqueta que bloquea lo dice.
//
// VISTO EN ROJO (2026-11-15): sin `OwnerSub`, `ModifiedBy` y `CreatedAt` en `api.File`
// el propietario y la fecha de creación salen «No disponible»; contando todos los
// enlaces, «Enlaces públicos» dice 2.
func TestDetallesDeUnFicheroComoEnLaWeb(t *testing.T) {
	srv, _ := servicioDetalles(t, 0)
	tr := traductor(t)
	el := Elemento{ID: "f1", Nombre: "acta.odt", Tamano: 2048}
	d, err := ReunirDetalles(context.Background(), fuentesDe(srv), el)
	if err != nil {
		t.Fatal(err)
	}
	k := func(c string, v ...i18n.Vars) string {
		var vars i18n.Vars
		if len(v) > 0 {
			vars = v[0]
		}
		return tr.T(c, vars)
	}
	l := tr.Locale()
	modificado := fechaHora(l, time.Date(2026, 9, 14, 8, 30, 0, 0, time.UTC).Local())
	quiero := [][2]string{
		{k("details.type"), k("details.fileOfType", i18n.Vars{"ext": "ODT"})},
		{k("details.size"), Tamano(l, 2048)},
		{k("details.location"), k("nav.myDrive") + " › Proyectos › 2026"},
		{k("details.owner"), "Ana Ruiz"},
		{k("details.created"), fechaHora(l, time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC).Local())},
		{k("details.modified"), k("details.modifiedBy", i18n.Vars{"date": modificado, "name": k("details.someone")})},
		{k("details.version"), "3"},
		{k("details.label"), k("details.labelBlocks", i18n.Vars{"name": "Confidencial"})},
		{k("details.links"), "1"},
		{k("details.people"), "2"},
		{k("details.groups"), "1"},
	}
	if got := FilasDetalles(tr, el, d); !slices.Equal(got, quiero) {
		t.Errorf("las filas son\n%q\ny en la web\n%q", got, quiero)
	}
}

// TestDetallesDeUnaCarpetaSinComparticionesLegibles: una carpeta no tiene tamaño,
// versión ni clasificación; su ubicación se sube de padre en padre, y si las
// comparticiones no se pueden leer el acceso es «No disponible», no un «0» que mentiría.
func TestDetallesDeUnaCarpetaSinComparticionesLegibles(t *testing.T) {
	srv, _ := servicioDetalles(t, 0)
	tr := traductor(t)
	el := Elemento{ID: "c3", Nombre: "IVA", Carpeta: true, Tamano: -1}
	d, err := ReunirDetalles(context.Background(), fuentesDe(srv), el)
	if err != nil {
		t.Fatal(err)
	}
	k := func(c string) string { return tr.T(c, nil) }
	l := tr.Locale()
	vacio := k("details.unavailable")
	quiero := [][2]string{
		{k("details.type"), k("details.folder")},
		{k("details.location"), k("nav.myDrive") + " › Contabilidad › 2026"},
		{k("details.owner"), "Ana Ruiz"},
		{k("details.created"), fechaHora(l, time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC).Local())},
		{k("details.modified"), fechaHora(l, time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC).Local())},
		{k("details.links"), vacio},
		{k("details.people"), vacio},
		{k("details.groups"), vacio},
	}
	if got := FilasDetalles(tr, el, d); !slices.Equal(got, quiero) {
		t.Errorf("las filas son\n%q\ny en la web\n%q", got, quiero)
	}
}

// TestElPanelDeDetallesEsperaFallaYReintenta: mientras llega se ven las ocho líneas del
// esqueleto; si el servicio falla, el aviso con «Reintentar», y al reintentar, la lista.
// Una carpeta no ofrece el historial; un fichero, sí.
func TestElPanelDeDetallesEsperaFallaYReintenta(t *testing.T) {
	srv, metadatos := servicioDetalles(t, 1)
	p := pantallaDePrueba(t, srv)
	p.raizID = "raiz"
	// La carga se queda en espera hasta que el test la suelte: así se ve el esqueleto.
	var pendiente func()
	p.aparte = func(f func()) { pendiente = f }
	dd := p.pedirDetalles(Elemento{ID: "f1", Nombre: "acta.odt"})
	if dd == nil || pendiente == nil {
		t.Fatal("«Detalles» no abre nada o no pide nada")
	}
	if n := cuentaEsqueletos(dd.cuerpo); n != 2*FilasEsqueletoDetalles {
		t.Errorf("la espera pinta %d bloques de esqueleto, y la web %d", n, 2*FilasEsqueletoDetalles)
	}
	p.aparte = func(f func()) { f() }
	pendiente()
	esperar()
	if !dd.fallo || !contieneTexto(dd.cuerpo, p.tr.T("details.loadError", nil)) {
		t.Fatal("un 500 no deja el aviso de error")
	}
	dd.reintentar()
	esperar()
	if dd.fallo || dd.detalles == nil || metadatos.Load() != 2 {
		t.Fatalf("«Reintentar» no vuelve a pedir: fallo=%v, %d peticiones", dd.fallo, metadatos.Load())
	}
	for _, busca := range []string{p.tr.T("details.owner", nil), "Ana Ruiz", p.tr.T("details.labelBlocks", i18n.Vars{"name": "Confidencial"})} {
		if !contieneTexto(dd.cuerpo, busca) {
			t.Errorf("el panel no enseña «%s»", busca)
		}
	}
	if !contieneTexto(dd.d.Pie, p.tr.T("details.versions", nil)) || !contieneTexto(dd.d.Pie, p.tr.T("details.share", nil)) {
		t.Error("el fichero no ofrece los atajos a versiones y compartir")
	}
	p.win.Canvas().Capture()

	carpeta := p.pedirDetalles(Elemento{ID: "c3", Nombre: "IVA", Carpeta: true, Tamano: -1})
	if contieneTexto(carpeta.d.Pie, p.tr.T("details.versions", nil)) {
		t.Error("una carpeta ofrece el historial de versiones")
	}
}

func cuentaEsqueletos(o fyne.CanvasObject) int {
	n := 0
	recorrer(o, func(x fyne.CanvasObject) {
		if _, ok := x.(*ui.Esqueleto); ok {
			n++
		}
	})
	return n
}

// TestLaUbicacionAMediasEsNoDisponible: si al subir de padre en padre falla un peldaño,
// la ubicación es «No disponible». `MigasDe` se queda entonces con la última carpeta, y
// usarla pintaba «Mi unidad › 2026» de algo que vive en «Mi unidad › Contabilidad › 2026».
//
// VISTO EN ROJO (2026-11-15): con `MigasDe` la ubicación era [2026].
func TestLaUbicacionAMediasEsNoDisponible(t *testing.T) {
	srv, _ := servicioDetalles(t, 0)
	f := fuentesDe(srv)
	carpeta := f.Carpeta
	f.Carpeta = func(ctx context.Context, id string) (api.Folder, error) {
		if id == "c1" {
			return api.Folder{}, io.ErrUnexpectedEOF
		}
		return carpeta(ctx, id)
	}
	d, err := ReunirDetalles(context.Background(), f, Elemento{ID: "c3", Nombre: "IVA", Carpeta: true, Tamano: -1})
	if err != nil {
		t.Fatal(err)
	}
	if d.Ubicacion != nil {
		t.Errorf("con un peldaño roto la ubicación es %q y no «No disponible»", d.Ubicacion)
	}
	// Directamente en «Mi unidad»: la ubicación es la raíz, no «No disponible».
	if got := carpetasHasta(context.Background(), f.Carpeta, "raiz", "raiz"); got == nil || len(got) != 0 {
		t.Errorf("en la raíz la ubicación es %q", got)
	}
}
