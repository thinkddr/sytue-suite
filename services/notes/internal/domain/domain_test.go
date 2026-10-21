// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// El saneado es la única barrera entre el HTML que escribe una persona y el
// navegador de otra: si se rompe, es XSS almacenado. Un caso por familia de
// ataque, no una suite exhaustiva.
func TestSanitizeHTMLQuitaLoPeligroso(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		fuera  []string // subcadenas que NO deben sobrevivir
		dentro []string // subcadenas que SÍ deben sobrevivir
	}{
		{
			nombre: "script se descarta con su contenido",
			in:     `<p>hola</p><script>alert(1)</script>`,
			fuera:  []string{"script", "alert"},
			dentro: []string{"<p>hola</p>"},
		},
		{
			nombre: "manejadores en línea no sobreviven",
			in:     `<p onclick="alert(1)">texto</p>`,
			fuera:  []string{"onclick", "alert"},
			dentro: []string{"texto"},
		},
		{
			nombre: "javascript: en href se cae y el resto del enlace queda",
			in:     `<a href="javascript:alert(1)">pulsa</a>`,
			fuera:  []string{"javascript"},
			dentro: []string{"pulsa"},
		},
		{
			nombre: "https en href sobrevive con noopener",
			in:     `<a href="https://sytue.eu/x">ir</a>`,
			dentro: []string{`href="https://sytue.eu/x"`, "noopener", "noreferrer"},
		},
		{
			nombre: "style se reduce a color",
			in:     `<span style="color:#ff0000;position:fixed;background:url(x)">rojo</span>`,
			fuera:  []string{"position", "url("},
			dentro: []string{"color:#ff0000", "rojo"},
		},
		{
			nombre: "iframe fuera",
			in:     `<iframe src="https://evil.example"></iframe><p>a</p>`,
			fuera:  []string{"iframe", "evil.example"},
			dentro: []string{"<p>a</p>"},
		},
		{
			nombre: "cierre huérfano no desbalancea",
			in:     `</div><b>negrita</b>`,
			dentro: []string{"<b>negrita</b>"},
			fuera:  []string{"</div>"},
		},
		{
			nombre: "etiqueta sin cerrar se cierra al final",
			in:     `<b>sin cerrar`,
			dentro: []string{"<b>sin cerrar</b>"},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := SanitizeHTML(c.in)
			for _, mal := range c.fuera {
				if strings.Contains(strings.ToLower(got), strings.ToLower(mal)) {
					t.Errorf("sobrevivió %q en %q", mal, got)
				}
			}
			for _, bien := range c.dentro {
				if !strings.Contains(got, bien) {
					t.Errorf("falta %q en %q", bien, got)
				}
			}
		})
	}
}

// Un cuaderno cifrado nunca puede acabar en el índice: es la invariante que hace
// que «cifrar» signifique algo. Se comprueba en las dos direcciones porque el
// cliente puede pedir las dos cosas en la misma petición.
func TestCuadernoCifradoNoSeIndexa(t *testing.T) {
	si := true
	in := NotebookInput{
		Title:     "Secretos",
		Encrypted: true,
		Indexable: &si,
		Crypto: &CryptoParams{
			V: 1, KDF: "PBKDF2-SHA256", Iterations: MinKDFIterations,
			Salt: strings.Repeat("a", 24), Check: "encv1:xxxx",
		},
	}
	if err := in.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if *in.Indexable {
		t.Fatal("un cuaderno cifrado quedó marcado como indexable")
	}
}

func TestCryptoParamsRechazaCosteBajo(t *testing.T) {
	c := CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Iterations: 1000, Salt: strings.Repeat("a", 24), Check: "x"}
	if err := c.Validate(); err == nil {
		t.Fatal("se aceptó un coste de derivación por debajo del mínimo")
	}
}

// En un cuaderno cifrado el texto en claro se RECHAZA. Si se guardara «por si
// acaso», el título —lo más revelador de una nota— quedaría legible en la base.
func TestNotaEnCuadernoCifradoExigeCriptograma(t *testing.T) {
	in := NoteInput{Kind: KindText, Title: "Contraseñas", Content: "<p>1234</p>"}
	if err := in.Normalize(true); err == nil {
		t.Fatal("se aceptó texto en claro en un cuaderno cifrado")
	}

	ok := NoteInput{CipherVersion: EncryptedNoteWriteVersion, Kind: KindText, Title: "Contraseñas", ContentCipher: "encv1:AAAA"}
	if err := ok.Normalize(true); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if ok.Title != "" || ok.Content != "" {
		t.Fatalf("quedó texto en claro: título=%q contenido=%q", ok.Title, ok.Content)
	}
}

func TestNotaEnClaroRechazaCriptograma(t *testing.T) {
	in := NoteInput{Kind: KindText, ContentCipher: "encv1:AAAA"}
	if err := in.Normalize(false); err == nil {
		t.Fatal("se aceptó un criptograma en un cuaderno sin cifrar")
	}
}

// El contenido de las notas de texto se sanea al entrar, no al pintar.
func TestNotaDeTextoSeSanea(t *testing.T) {
	in := NoteInput{Kind: KindText, Content: `<p>ok</p><script>alert(1)</script>`}
	if err := in.Normalize(false); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if strings.Contains(in.Content, "script") {
		t.Fatalf("el HTML no se saneó: %q", in.Content)
	}
}

// …y el de TODOS los tipos, no solo el de texto: las exportaciones, la búsqueda y los
// avisos tratan el contenido como HTML sea cual sea el tipo de la nota.
func TestNotaDeCualquierTipoSeSanea(t *testing.T) {
	for _, k := range []Kind{
		KindTodo, KindImage, KindFile, KindLink, KindColor, KindLauncher,
		KindCrossref, KindDoc, KindDrive, KindEvent, KindRoom,
	} {
		in := NoteInput{Kind: k, Content: `<p onclick="x()">ok</p><script>alert(1)</script>`}
		if err := in.Normalize(false); err != nil {
			t.Fatalf("%s: Normalize: %v", k, err)
		}
		if in.Content != "<p>ok</p>" {
			t.Errorf("%s: el HTML no se saneó: %q", k, in.Content)
		}
	}
}

func TestVisibilidadPrivadaNoIndexaPorDefecto(t *testing.T) {
	in := NotebookInput{Title: "Diario", Visibility: VisPrivate}
	if err := in.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if *in.Indexable {
		t.Fatal("un cuaderno privado nació indexable")
	}
}

func TestEnlaceConEsquemaRaroSeRechaza(t *testing.T) {
	in := NoteInput{
		Kind:  KindLink,
		Links: []NoteLink{{TargetKind: TargetDriveFile, TargetID: "x", URL: "javascript:alert(1)"}},
		Data:  json.RawMessage(`{}`),
	}
	if err := in.Normalize(false); err == nil {
		t.Fatal("se aceptó una URL con esquema no web")
	}
}
