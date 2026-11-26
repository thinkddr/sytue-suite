// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Dejar y recoger: el identificador es opaco y de un solo uso, la entrega no se ve a
// medias, y sin carpeta compartida (Android) no se deja nada.
func TestParaEnviarDejarYRecoger(t *testing.T) {
	if _, err := (ParaEnviar{}).Dejar("x.pdf", []byte("x")); !errors.Is(err, ErrSinParaEnviar) {
		t.Fatalf("sin carpeta: %v", err)
	}
	s := ParaEnviar{Dir: filepath.Join(t.TempDir(), "para-enviar")}
	id, err := s.Dejar("../../Factura\x00 FV-1.pdf", []byte("%PDF"))
	if err != nil || !IDParaEnviar(id) {
		t.Fatalf("dejar: %q %v", id, err)
	}
	if fi, err := os.Stat(s.Dir); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700) {
		t.Fatalf("carpeta: %v %v", fi, err)
	}
	// Solo la entrega, sin temporales a la vista.
	if es, _ := os.ReadDir(s.Dir); len(es) != 1 || es[0].Name() != id {
		t.Fatalf("en la carpeta: %v", es)
	}
	a, err := s.Recoger(id)
	if err != nil || a.Nombre != "Factura FV-1.pdf" || a.Tipo != "application/pdf" || string(a.Datos) != "%PDF" {
		t.Fatalf("recoger: %+v %v", a, err)
	}
	if _, err := s.Recoger(id); !errors.Is(err, ErrEntregaNoValida) {
		t.Fatalf("dos veces: %v", err)
	}
	if es, _ := os.ReadDir(s.Dir); len(es) != 0 {
		t.Fatalf("quedó: %v", es)
	}
	// Quitar: lo que Mail no llegó a recoger (no se abrió).
	id, _ = s.Dejar("y.pdf", []byte("y"))
	s.Quitar(id)
	if _, err := s.Recoger(id); !errors.Is(err, ErrEntregaNoValida) {
		t.Fatalf("quitado: %v", err)
	}
	if _, err := s.Dejar("grande.pdf", make([]byte, TopeParaEnviar+1)); !errors.Is(err, ErrEntregaGrande) {
		t.Fatalf("grande: %v", err)
	}
	// El enlace lleva las entregas por su identificador y lo demás escapado.
	u, err := url.Parse(EnlaceRedactar("a@b.eu", "Factura & co", "Hola\nadiós", "AAAAAAAAAAAAAAAAAAAAAAAAAA", "BBBBBBBBBBBBBBBBBBBBBBBBBB"))
	if err != nil || u.Scheme != "sytueoffice-mail" || u.Path != "/redactar" {
		t.Fatalf("enlace: %v %v", u, err)
	}
	if q := u.Query(); q.Get("asunto") != "Factura & co" || q.Get("cuerpo") != "Hola\nadiós" || len(q[ParamAdjunto]) != 2 {
		t.Fatalf("enlace: %v", q)
	}
}

// Lo que no es un fichero normal de la entrega no se recoge, y se borra igual.
func TestParaEnviarRechazaLoQueNoEsUnaEntrega(t *testing.T) {
	s := ParaEnviar{Dir: filepath.Join(t.TempDir(), "para-enviar")}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	entrega := func(id string, ficheros map[string][]byte) {
		t.Helper()
		if err := os.Mkdir(filepath.Join(s.Dir, id), 0o700); err != nil {
			t.Fatal(err)
		}
		for n, d := range ficheros {
			if err := os.WriteFile(filepath.Join(s.Dir, id, n), d, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	casos := map[string]func(id string){
		"vacía":  func(id string) { entrega(id, nil) },
		"dos":    func(id string) { entrega(id, map[string][]byte{"a.pdf": {1}, "b.pdf": {2}}) },
		"oculto": func(id string) { entrega(id, map[string][]byte{".a.pdf": {1}}) },
		"no carpeta": func(id string) {
			if err := os.WriteFile(filepath.Join(s.Dir, id), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"subcarpeta": func(id string) {
			entrega(id, nil)
			if err := os.Mkdir(filepath.Join(s.Dir, id, "dentro"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	}
	if runtime.GOOS != "windows" {
		fuera := filepath.Join(t.TempDir(), "secreto.txt")
		if err := os.WriteFile(fuera, []byte("secreto"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Un enlace duro es un fichero normal, pero con otro nombre fuera de la carpeta.
		casos["enlace duro"] = func(id string) {
			entrega(id, nil)
			if err := os.Link(fuera, filepath.Join(s.Dir, id, "f.pdf")); err != nil {
				t.Skip("sin enlaces duros:", err)
			}
		}
	}
	i := 0
	for nombre, preparar := range casos {
		id := string(bytes.Repeat([]byte{byte('A' + i)}, largoID))
		i++
		preparar(id)
		if a, err := s.Recoger(id); !errors.Is(err, ErrEntregaNoValida) {
			t.Errorf("%s: se recogió %+v", nombre, a)
		}
		if _, err := os.Lstat(filepath.Join(s.Dir, id)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: no se borró", nombre)
		}
	}
	if es, _ := os.ReadDir(s.Dir); len(es) != 0 {
		t.Errorf("quedó: %v", es)
	}
}

// Barrer quita lo que lleva más de un día y deja lo reciente; en una carpeta que es un
// enlace simbólico a otra no toca nada (lo viejo de la persona se quedaría sin borrar).
func TestParaEnviarBarreLoViejo(t *testing.T) {
	s := ParaEnviar{Dir: filepath.Join(t.TempDir(), "para-enviar")}
	viejo, _ := s.Dejar("viejo.pdf", []byte("v"))
	nuevo, _ := s.Dejar("nuevo.pdf", []byte("n"))
	if err := os.MkdirAll(filepath.Join(s.Dir, ".OLVIDADA"+sufijoEscribiendo), 0o700); err != nil {
		t.Fatal(err)
	}
	ayer := time.Now().Add(-CaducidadParaEnviar - time.Minute)
	for _, n := range []string{viejo, ".OLVIDADA" + sufijoEscribiendo} {
		if err := os.Chtimes(filepath.Join(s.Dir, n), ayer, ayer); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.Barrer(); n != 2 {
		t.Fatalf("barridas %d", n)
	}
	if _, err := s.Recoger(viejo); err == nil {
		t.Error("lo viejo sigue")
	}
	if a, err := s.Recoger(nuevo); err != nil || a.Nombre != "nuevo.pdf" {
		t.Errorf("lo nuevo: %+v %v", a, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	personal := t.TempDir()
	foto := filepath.Join(personal, "foto-de-2019.jpg")
	if err := os.WriteFile(foto, []byte("jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(foto, ayer, ayer); err != nil {
		t.Fatal(err)
	}
	enlazada := filepath.Join(t.TempDir(), "para-enviar")
	if err := os.Symlink(personal, enlazada); err != nil {
		t.Fatal(err)
	}
	if n := (ParaEnviar{Dir: enlazada}).Barrer(); n != 0 {
		t.Errorf("barrió %d en una carpeta ajena", n)
	}
	if _, err := os.Stat(foto); err != nil {
		t.Fatal("se borró lo de la persona:", err)
	}
	if _, err := (ParaEnviar{Dir: enlazada}).Dejar("x.pdf", []byte("x")); err == nil {
		t.Error("dejó en una carpeta ajena")
	}
}
