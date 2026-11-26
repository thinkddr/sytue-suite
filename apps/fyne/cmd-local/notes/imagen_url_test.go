// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	privateNotes "git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// servidorDeImagenes es un servidor de imágenes `httptest` (TLS) que apunta cada
// petición y lo que el portero tenía abierto mientras la atendía.
type servidorDeImagenes struct {
	*httptest.Server
	mu       sync.Mutex
	pedidas  []string
	abiertos [][]red.Destino
}

func (s *servidorDeImagenes) vistas() ([]string, [][]red.Destino) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.pedidas), slices.Clone(s.abiertos)
}

func nuevoServidorDeImagenes(t *testing.T, p *red.Portero) *servidorDeImagenes {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 40, 20))); err != nil {
		t.Fatal(err)
	}
	avif, err := os.ReadFile("../../local/imagenes/testdata/foto.avif")
	if err != nil {
		t.Fatal(err)
	}
	s := &servidorDeImagenes{}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.pedidas = append(s.pedidas, r.URL.Path)
		s.abiertos = append(s.abiertos, p.Destinos())
		s.mu.Unlock()
		switch r.URL.Path {
		case "/foto.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(b.Bytes())
		case "/foto.avif":
			w.Header().Set("Content-Type", "image/avif")
			_, _ = w.Write(avif)
		case "/pagina":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html>no es una imagen</html>"))
		case "/grande":
			w.Header().Set("Content-Length", strconv.Itoa(notes.MaxImagenRemota+1))
			_, _ = w.Write(make([]byte, 1024))
		case "/salta":
			http.Redirect(w, r, "http://example.com/foto.png", http.StatusFound)
		default:
			http.Error(w, "no está", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// La nota de imagen por URL sale SOLO por el permiso puntual del portero de verdad, con
// un servidor de imágenes `httptest`: a una dirección de este equipo no llega (el servidor
// ni se entera); con el servidor a la vista de la prueba (PuntualDePrueba, como si fuera
// público), la imagen llega y, mientras dura, Ajustes → Red enseña el permiso temporal de
// `imagen-remota` a ese host, que se cierra al acabar; http (también en una redirección)
// no sale; una página o lo que pasa del tope no es una imagen; y con «Sin conexión» no
// sale nada.
func TestLaImagenDeUnaNotaSaleSoloPorElPermisoPuntual(t *testing.T) {
	ctx := t.Context()
	p := red.Abrir("")
	srv := nuevoServidorDeImagenes(t, p)

	if _, err := bajarImagen(p)(ctx, srv.URL+"/foto.png"); !errors.Is(err, notes.ErrImagenNoPermitida) {
		t.Fatalf("a este equipo: %v; quiero ErrImagenNoPermitida", err)
	}
	if pedidas, _ := srv.vistas(); len(pedidas) != 0 {
		t.Fatalf("la petición llegó al servidor de este equipo: %v", pedidas)
	}

	raices := x509.NewCertPool()
	raices.AddCert(srv.Certificate())
	p.PuntualDePrueba(func(ctx context.Context, red, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, red, srv.Listener.Addr().String())
	}, raices)
	bajar := bajarImagen(p)

	for _, ruta := range []string{"/foto.png", "/foto.avif"} {
		datos, err := bajar(ctx, "https://example.com"+ruta)
		if err != nil || len(datos) == 0 {
			t.Fatalf("%s: %v", ruta, err)
		}
	}
	pedidas, abiertos := srv.vistas()
	if !slices.Equal(pedidas, []string{"/foto.png", "/foto.avif"}) {
		t.Fatalf("pedidas: %v", pedidas)
	}
	for _, ds := range abiertos {
		if len(ds) != 1 || ds[0].Proposito != red.ImagenRemota || ds[0].Host != "example.com" || !ds[0].Temporal {
			t.Errorf("mientras baja, Ajustes → Red enseña %+v", ds)
		}
	}
	if ds := p.Destinos(); len(ds) != 0 {
		t.Fatalf("acabada, el permiso sigue abierto: %+v", ds)
	}

	for direccion, quiero := range map[string]error{
		"http://example.com/foto.png":   notes.ErrImagenNoPermitida,
		"ftp://example.com/foto.png":    notes.ErrImagenNoPermitida,
		"https://example.com/salta":     notes.ErrImagenNoPermitida,
		"https://example.com/pagina":    notes.ErrNoEsImagen,
		"https://example.com/grande":    notes.ErrNoEsImagen,
		"https://example.com/caida.png": nil,
	} {
		_, err := bajar(ctx, direccion)
		switch {
		case err == nil:
			t.Errorf("%s: sin error", direccion)
		case quiero != nil && !errors.Is(err, quiero):
			t.Errorf("%s: %v; quiero %v", direccion, err, quiero)
		case quiero == nil && (errors.Is(err, notes.ErrImagenNoPermitida) || errors.Is(err, notes.ErrNoEsImagen)):
			t.Errorf("%s: un 500 no es «no permitida» ni «no es una imagen»: %v", direccion, err)
		}
	}
	if pedidas, _ := srv.vistas(); slices.Contains(pedidas, "/foto.png") && strings.Count(strings.Join(pedidas, " "), "/foto.png") != 1 {
		t.Errorf("la redirección a http llegó a salir: %v", pedidas)
	}

	antes, _ := srv.vistas()
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	if _, err := bajar(ctx, "https://example.com/foto.png"); !errors.Is(err, notes.ErrImagenSinConexion) {
		t.Fatalf("con «Sin conexión»: %v", err)
	}
	if despues, _ := srv.vistas(); len(despues) != len(antes) {
		t.Fatalf("con «Sin conexión» salió: %v", despues[len(antes):])
	}
}

// El repositorio de Sytue Office (`notes.NewLocal`) guarda la nota de imagen por URL tal
// cual la deja la pantalla: su `data.url` y la imagen dentro, con su dirección por origen.
func TestElAlmacenGuardaLaImagenDeLaDireccion(t *testing.T) {
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), t.TempDir(), "notes", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	repo, err := privateNotes.NewLocal(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	svc, err := api.NewNotes("", nil, nil).WithPrivateData(notasLocales{repo, rec})
	if err != nil {
		t.Fatal(err)
	}
	libro, err := svc.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	const dir = "https://example.com/planos/cocina.png"
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	a := notes.Adjunto{
		ID: "1700000000000-abcdef", Name: "cocina.png", Size: int64(b.Len()), Type: "image/png",
		DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes()), Origen: dir,
	}
	data, _ := json.Marshal(map[string]any{"url": dir, "attachments": []notes.Adjunto{a}})
	n, err := svc.CreateNote(ctx, libro.ID, api.NoteInput{Kind: "image", Title: "cocina", Data: data})
	if err != nil {
		t.Fatal(err)
	}
	ns, err := svc.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || ns[0].ID != n.ID {
		t.Fatalf("notas: %v %v", ns, err)
	}
	var d struct {
		URL         string          `json:"url"`
		Attachments []notes.Adjunto `json:"attachments"`
	}
	if err := json.Unmarshal(ns[0].Data, &d); err != nil || d.URL != dir || len(d.Attachments) != 1 || d.Attachments[0] != a {
		t.Fatalf("guardada: %s %v", ns[0].Data, err)
	}
}
