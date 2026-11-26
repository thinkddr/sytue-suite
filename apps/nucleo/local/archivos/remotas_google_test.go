// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"context"
	"crypto/md5" //nolint:gosec // la huella de Google
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// ficheroGoogle es un fichero o una carpeta de googleFalso.
type ficheroGoogle struct {
	id, nombre, tipo string
	padres           []string
	datos            []byte
	version          int
	creado           time.Time
	tirado           bool
}

// googleFalso es la API v3 de Google Drive en memoria, lo que usa `nube.GoogleDrive`:
// listar por carpeta, bajar, subir (crear con multipart y sustituir el contenido por id),
// cambiar metadatos (nombre, padres, papelera) y crear carpetas. Como Google, admite dos
// con el mismo nombre en una carpeta, y lista por identificador, no por nombre.
type googleFalso struct {
	mu       sync.Mutex
	ficheros map[string]*ficheroGoogle
	n        int
	hechos   []string
}

func (g *googleFalso) poner(id, nombre, tipo, padre, datos string, creado time.Time) {
	g.ficheros[id] = &ficheroGoogle{id: id, nombre: nombre, tipo: tipo, padres: []string{padre}, datos: []byte(datos), version: 1, creado: creado}
}

func (g *googleFalso) json(f *ficheroGoogle) map[string]any {
	m := map[string]any{
		"id": f.id, "name": f.nombre, "mimeType": f.tipo, "size": strconv.Itoa(len(f.datos)),
		"version": strconv.Itoa(f.version), "createdTime": f.creado, "modifiedTime": f.creado.Add(time.Duration(f.version) * time.Minute),
	}
	// Como Google: los ficheros con bytes llevan su MD5; las carpetas y los documentos
	// de Google, no.
	if !strings.HasPrefix(f.tipo, "application/vnd.google-apps.") {
		h := md5.Sum(f.datos) //nolint:gosec // la huella de Google
		m["md5Checksum"] = hex.EncodeToString(h[:])
	}
	return m
}

func (g *googleFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer tk" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	g.hechos = append(g.hechos, r.Method+" "+r.URL.Path)
	responder := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	id := path.Base(r.URL.Path)
	f := g.ficheros[id]
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/drive/v3/files":
		padre := strings.SplitN(r.URL.Query().Get("q"), "'", 3)[1]
		var ids []string
		for id, f := range g.ficheros {
			if !f.tirado && slices.Contains(f.padres, padre) {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids) // por identificador, no por nombre
		var files []map[string]any
		for _, id := range ids {
			files = append(files, g.json(g.ficheros[id]))
		}
		responder(map[string]any{"files": files})
	case r.Method == http.MethodGet && f != nil && r.URL.Query().Get("alt") == "media":
		_, _ = w.Write(f.datos)
	case r.Method == http.MethodGet && f != nil:
		responder(map[string]any{"parents": f.padres})
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && f != nil:
		f.datos, _ = io.ReadAll(r.Body)
		f.version++
		responder(g.json(f))
	case r.Method == http.MethodPatch && f != nil:
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		if n, ok := m["name"].(string); ok {
			f.nombre = n
		}
		if t, ok := m["trashed"].(bool); ok {
			f.tirado = t
		}
		if a := r.URL.Query().Get("addParents"); a != "" {
			quitar := strings.Split(r.URL.Query().Get("removeParents"), ",")
			f.padres = append(slices.DeleteFunc(f.padres, func(p string) bool { return slices.Contains(quitar, p) }), a)
		}
		f.version++
		responder(g.json(f))
	case r.Method == http.MethodPost && (r.URL.Path == "/drive/v3/files" || r.URL.Path == "/upload/drive/v3/files"):
		var meta struct {
			Name, MimeType string
			Parents        []string
		}
		var datos []byte
		if _, ps, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && ps["boundary"] != "" {
			mr := multipart.NewReader(r.Body, ps["boundary"])
			if p, err := mr.NextPart(); err == nil {
				_ = json.NewDecoder(p).Decode(&meta)
			}
			if p, err := mr.NextPart(); err == nil {
				datos, _ = io.ReadAll(p)
			}
		} else {
			_ = json.NewDecoder(r.Body).Decode(&meta)
		}
		g.n++
		nuevo := &ficheroGoogle{
			id: "nuevo-" + strconv.Itoa(g.n), nombre: meta.Name, tipo: meta.MimeType, padres: meta.Parents, datos: datos, version: 1,
			creado: time.Date(2026, 11, 20, 0, 0, g.n, 0, time.UTC),
		}
		g.ficheros[nuevo.id] = nuevo
		responder(g.json(nuevo))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (g *googleFalso) datosDe(id string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if f := g.ficheros[id]; f != nil {
		return string(f.datos)
	}
	return ""
}

// unidadGoogle es Drive con una cuenta de Google Drive servida por googleFalso: en
// «Informes» hay dos «Acta.docx» (la vieja, creada antes, lista DESPUÉS: Google lista por
// identificador), un «Acta (2).docx» de verdad y dos carpetas «Fotos».
func unidadGoogle(t *testing.T) (*Remotas, *googleFalso) {
	t.Helper()
	g := &googleFalso{ficheros: map[string]*ficheroGoogle{}}
	const carpeta, docx = "application/vnd.google-apps.folder", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	sep := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	g.poner("inf", "Informes", carpeta, "root", "", sep)
	g.poner("a-viejo", "Acta.docx", docx, "inf", "ACTA VIEJA", sep)
	g.poner("a-nuevo", "Acta.docx", docx, "inf", "ACTA NUEVA", sep.AddDate(0, 1, 0))
	g.poner("a-2", "Acta (2).docx", docx, "inf", "OTRA ACTA", sep.AddDate(0, 0, 3))
	g.poner("f-1", "Fotos", carpeta, "inf", "", sep)
	g.poner("f-2", "Fotos", carpeta, "inf", "", sep.AddDate(0, 0, 1))
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	gd := &nube.GoogleDrive{
		Token: func(_ context.Context) (string, error) { return "tk", nil }, HTTP: srv.Client(),
		API: srv.URL + "/drive/v3", Subidas: srv.URL + "/upload/drive/v3",
	}
	a, base := abrirPrueba(t)
	cuentas := func() []nube.Cuenta {
		return []nube.Cuenta{{ID: "g1", Tipo: nube.Google, Rotulo: "Google Drive de Ana"}}
	}
	return ConRemotas(a, cuentas, func(string) (nube.Proveedor, error) { return gd, nil }, filepath.Join(base, "cache")), g
}

// DR-13: dos documentos con el mismo nombre en una carpeta de Google Drive se ven los dos,
// el más antiguo con su nombre y el otro como «Acta (3).docx» («Acta (2).docx» ya es de
// otro), sin renombrar nada en la nube; igual dos carpetas. Abrir, guardar, sustituir,
// copiar fuera y en la nube, renombrar, mover y tirar actúan sobre el que es, por su
// identificador: guardar uno no pisa al otro.
func TestDosDocumentosConElMismoNombreEnGoogleDrive(t *testing.T) {
	r, g := unidadGoogle(t)
	ctx := t.Context()
	d, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(r)
	if err != nil {
		t.Fatal(err)
	}
	raiz, err := d.Children(ctx, r.Unidades()[0].ID, "")
	if err != nil || len(raiz.Folders) != 1 {
		t.Fatal(raiz, err)
	}
	informes := raiz.Folders[0].ID
	hijos, err := d.Children(ctx, informes, "")
	if err != nil {
		t.Fatal(err)
	}
	porNombre := map[string]string{}
	for _, f := range hijos.Files {
		_, id, _ := partirRemoto(f.ID)
		porNombre[f.Name] = id
	}
	for _, f := range hijos.Folders {
		_, id, _ := partirRemoto(f.ID)
		porNombre[f.Name+"/"] = id
	}
	quiero := map[string]string{"Acta.docx": "a-viejo", "Acta (3).docx": "a-nuevo", "Acta (2).docx": "a-2", "Fotos/": "f-1", "Fotos (2)/": "f-2"}
	if len(porNombre) != len(quiero) {
		t.Fatalf("se ven todos, cada uno con un nombre: %v", porNombre)
	}
	for n, id := range quiero {
		if porNombre[n] != id {
			t.Fatalf("%q es %q, quiero %q: %v", n, porNombre[n], id, porNombre)
		}
	}
	if g.ficheros["a-nuevo"].nombre != "Acta.docx" || g.ficheros["f-2"].nombre != "Fotos" {
		t.Fatal("en la nube no se renombra nada")
	}
	id := func(n string) string { return idRemoto("g1", porNombre[n]) }

	// Abrir baja el que es, con el nombre con que se ve.
	nuevo, err := d.File(ctx, id("Acta (3).docx"))
	if err != nil || filepath.Base(nuevo.Path) != "Acta (3).docx" || nuevo.Name != "Acta (3).docx" {
		t.Fatal("abrir", nuevo, err)
	}
	if b, _ := os.ReadFile(nuevo.Path); string(b) != "ACTA NUEVA" {
		t.Fatal("el contenido es el del nuevo", string(b))
	}
	viejo, err := d.File(ctx, id("Acta.docx"))
	if err != nil || filepath.Base(viejo.Path) != "Acta.docx" {
		t.Fatal("abrir el viejo", viejo, err)
	}
	if rc, err := d.Download(ctx, id("Acta (3).docx")); err != nil {
		t.Fatal(err)
	} else if b, _ := io.ReadAll(rc); string(b) != "ACTA NUEVA" {
		t.Fatal("descargar", string(b))
	} else {
		rc.Close()
	}

	// Guardar el nuevo sube encima de él, por su identificador; el viejo no cambia.
	if err := os.WriteFile(nuevo.Path, []byte("ACTA NUEVA v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := r.Sincronizar(ctx); len(s) != 1 || s[0].Err != nil || s[0].Conflicto {
		t.Fatal("subir", s)
	}
	if g.datosDe("a-nuevo") != "ACTA NUEVA v2" || g.datosDe("a-viejo") != "ACTA VIEJA" || len(g.ficheros) != 6 {
		t.Fatal("guardar el nuevo pisó al viejo o creó otro", g.datosDe("a-nuevo"), g.datosDe("a-viejo"), len(g.ficheros))
	}
	// Y sustituir el viejo (el que Google lista después), el viejo.
	if _, err := r.ReplaceFile(ctx, viejo, "", strings.NewReader("ACTA VIEJA v2"), 13); err != nil {
		t.Fatal(err)
	}
	if g.datosDe("a-viejo") != "ACTA VIEJA v2" || g.datosDe("a-nuevo") != "ACTA NUEVA v2" {
		t.Fatal("sustituir", g.datosDe("a-viejo"), g.datosDe("a-nuevo"))
	}
	if s := r.Sincronizar(ctx); len(s) != 0 {
		t.Fatal("no queda nada por subir", s)
	}

	// Copiar fuera deja «Acta (3).docx» con lo del nuevo; copiar en la nube, «Acta (4).docx».
	fuera := t.TempDir()
	if dst, err := r.CopiarFuera(ctx, id("Acta (3).docx"), fuera); err != nil || filepath.Base(dst) != "Acta (3).docx" {
		t.Fatal("copiar fuera", dst, err)
	} else if b, _ := os.ReadFile(dst); string(b) != "ACTA NUEVA v2" {
		t.Fatal("copiar fuera", string(b))
	}
	copia, err := d.CopyFile(ctx, id("Acta (3).docx"))
	if err != nil || copia.Name != "Acta (4).docx" {
		t.Fatal("copiar en la nube", copia, err)
	}

	// Renombrar, mover y tirar: el que es.
	if err := d.Rename(ctx, api.DriveFile, id("Acta (3).docx"), "Acta de octubre.docx"); err != nil {
		t.Fatal(err)
	}
	if err := d.Move(ctx, api.DriveFile, id("Acta.docx"), id("Fotos (2)/")); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, api.DriveFile, id("Acta (2).docx")); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	if g.ficheros["a-nuevo"].nombre != "Acta de octubre.docx" || g.ficheros["a-viejo"].nombre != "Acta.docx" ||
		!slices.Equal(g.ficheros["a-viejo"].padres, []string{"f-2"}) || !g.ficheros["a-2"].tirado || g.ficheros["a-nuevo"].tirado || g.ficheros["a-viejo"].tirado {
		g.mu.Unlock()
		t.Fatal("renombrar, mover o tirar actuaron sobre otro")
	}
	g.mu.Unlock()
	hijos, _ = d.Children(ctx, informes, "")
	nombres := make([]string, 0, len(hijos.Files))
	for _, f := range hijos.Files {
		nombres = append(nombres, f.Name)
	}
	slices.Sort(nombres)
	if strings.Join(nombres, "|") != "Acta (4).docx|Acta de octubre.docx" {
		t.Fatal("lo que queda en Informes", nombres)
	}
	fotos2, _ := d.Children(ctx, id("Fotos (2)/"), "")
	if len(fotos2.Files) != 1 || fotos2.Files[0].Name != "Acta.docx" {
		t.Fatal("movido a la segunda «Fotos»", fotos2)
	}
}

// Si el gemelo desaparece, el otro vuelve a verse con su nombre y se baja con él; lo que
// se cambió aquí y aún no subió no se pierde por eso. Y si el documento ya no está en la
// nube y su nombre es ahora de otro, lo de aquí sube aparte, sin pisarlo.
func TestElNombreDeLaVistaCambiaSinPerderNada(t *testing.T) {
	r, g := unidadGoogle(t)
	ctx := t.Context()
	raiz, _ := r.Children(ctx, r.Unidades()[0].ID, "")
	informes := raiz.Folders[0].ID
	if _, err := r.Children(ctx, informes, ""); err != nil {
		t.Fatal(err)
	}
	nuevo, err := r.File(ctx, idRemoto("g1", "a-nuevo"))
	if err != nil || nuevo.Name != "Acta (3).docx" {
		t.Fatal(nuevo, err)
	}
	if err := os.WriteFile(nuevo.Path, []byte("ACTA NUEVA, cambiada aquí"), 0o600); err != nil {
		t.Fatal(err)
	}
	// El viejo se va de la carpeta (otra persona lo tira) antes de que se suba lo de aquí.
	g.mu.Lock()
	g.ficheros["a-viejo"].tirado = true
	g.mu.Unlock()
	hijos, _ := r.Children(ctx, informes, "")
	visto := ""
	for _, f := range hijos.Files {
		if f.ID == idRemoto("g1", "a-nuevo") {
			visto = f.Name
		}
	}
	if visto != "Acta.docx" {
		t.Fatal("sin gemelo, con su nombre", visto)
	}
	// Abrirlo ahora da lo que se cambió aquí, no lo de la nube.
	otra, err := r.File(ctx, idRemoto("g1", "a-nuevo"))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(otra.Path); string(b) != "ACTA NUEVA, cambiada aquí" {
		t.Fatal("lo cambiado aquí se pierde al cambiar el nombre de la vista", string(b))
	}
	if s := r.Sincronizar(ctx); len(s) != 1 || s[0].Err != nil || g.datosDe("a-nuevo") != "ACTA NUEVA, cambiada aquí" {
		t.Fatal("subir", s, g.datosDe("a-nuevo"))
	}
	// Ya subido, abrirlo otra vez lo baja con el nombre de ahora.
	if f, err := r.File(ctx, idRemoto("g1", "a-nuevo")); err != nil || filepath.Base(f.Path) != "Acta.docx" {
		t.Fatal("con el nombre de ahora", f, err)
	}

	// «Acta (2).docx» se cambia aquí y en la nube lo tiran y otro sube uno con su nombre.
	dos, err := r.File(ctx, idRemoto("g1", "a-2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dos.Path, []byte("LO DE AQUÍ"), 0o600); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	g.ficheros["a-2"].tirado = true
	g.poner("a-2-otro", "Acta (2).docx", "", "inf", "DE OTRA PERSONA", time.Now())
	g.mu.Unlock()
	s := r.Sincronizar(ctx)
	if len(s) != 1 || s[0].Err != nil || !s[0].Conflicto || !strings.HasPrefix(s[0].Como, "Acta (2) (conflicto ") {
		t.Fatal("sube aparte", s)
	}
	if g.datosDe("a-2-otro") != "DE OTRA PERSONA" {
		t.Fatal("se pisó el de la otra persona")
	}
}
