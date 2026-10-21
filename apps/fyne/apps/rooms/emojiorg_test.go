// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los emoji de la organización (customEmoji.ts): el catálogo llega al arrancar; el
// selector los enseña y los filtra; cada imagen se baja UNA vez (la miniatura de Drive);
// en el cuerpo, `:nombre:` es su imagen en línea —y en grande si es el mensaje entero—,
// uno que no es del catálogo queda literal; la píldora de reacción lleva la imagen; y
// «Subir» valida como `submitUpload` y deja el nuevo en el catálogo, en su orden.
//
// VISTO EN ROJO (2026-09-25, en el frontal, saboteado a mano): sin `ficheroPropio` en
// `conPropios`, «:nope:» salía como hueco de imagen en vez de texto.
func TestLosEmojiDeLaOrganizacion(t *testing.T) {
	base := servicioConDos()
	s := &conEmoji{servicioFalso: base, catalogo: []api.RoomsCustomEmoji{
		{Name: "loro", FileID: "f1", ContentType: "image/png"},
		{Name: "baile", FileID: "f2", ContentType: "image/gif"},
	}}
	sa, _ := salaDePrueba(t, base)
	dr := &driveDelTenant{ficherosFalso: &ficherosFalso{}}
	sa.S, sa.Ficheros = s, dr
	dir := t.TempDir()
	png := filepath.Join(dir, "gato.png")
	if err := os.WriteFile(png, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	sa.ElegirFicheros = func() ([]string, error) { return []string{png}, nil }
	sa.Arrancar(context.Background())
	if len(catalogoOrg.lista) != 2 {
		t.Fatalf("al arrancar, el catálogo tiene %d", len(catalogoOrg.lista))
	}

	// El selector.
	var elegido string
	sa.AbrirEmojis(sa.P.Canal.Compositor.BotonEmoji(), func(e string) { elegido = e })
	sel := sa.emojis
	for _, c := range sel.propias {
		test.WidgetRenderer(c).Refresh()
		test.WidgetRenderer(c).Refresh()
	}
	for _, f := range []string{"f1", "f2"} {
		if n := strings.Count(strings.Join(dr.llamadas, "|"), "mini "+f); n != 1 || sa.imagenesEmoji[f] == nil {
			t.Errorf("la miniatura de %s se pide %d veces", f, n)
		}
	}
	sel.Caja().PonerTexto("lor")
	if len(sel.propias) != 1 {
		t.Fatalf("buscar «lor» deja %d", len(sel.propias))
	}
	test.Tap(sel.propias[0])
	if elegido != ":loro:" {
		t.Errorf("elegir uno de la organización manda %q", elegido)
	}

	// El cuerpo.
	m := maquetadorDe("hola :loro: y :nope: :smile:", "", nil, 800)
	var propios, textos []string
	for _, p := range m.piezas {
		if p.estilo == estEmojiPropio {
			propios = append(propios, p.texto)
			if p.ancho != anchoEmojiPropio(tema.TextoBase)-2 {
				t.Errorf("el emoji en línea mide %v", p.ancho)
			}
		} else {
			textos = append(textos, p.texto)
		}
	}
	if !slices.Equal(propios, []string{":loro:"}) || !slices.Contains(textos, ":nope:") || !slices.Contains(textos, "😄") {
		t.Errorf("en línea: %v · %v", propios, textos)
	}
	solo := maquetadorDe(" :baile: ", "", nil, 800)
	if solo.solo != "f2" || solo.tamSolo.Width != 40 || solo.tamSolo.Height != 20 {
		t.Errorf("solo: %q %v", solo.solo, solo.tamSolo)
	}
	if alto, _ := MaquetarCuerpo(":baile:", nil, 800); alto != 20 {
		t.Errorf("el mensaje de un solo emoji mide %v de alto", alto)
	}

	// La píldora.
	if MedidaReaccion(Reaccion{Emoji: ":loro:", Veces: 1}).Width >= MedidaReaccion(Reaccion{Emoji: ":nope:", Veces: 1}).Width {
		t.Error("la píldora de un emoji de la organización mide lo del texto")
	}

	// «Subir».
	for _, c := range []struct{ nombre, ruta, clave string }{
		{"Mal nombre", png, "emoji.badName"},
		{"loro", png, "emoji.exists"},
		{"nuevo", "", "emoji.pickImage"},
		{"nuevo", filepath.Join(dir, "a.txt"), "emoji.mustBeImage"},
		{"gato", png, ""},
	} {
		if clave, _ := validarPropio(c.nombre, c.ruta); clave != c.clave {
			t.Errorf("validar %q %q = %q; quiero %q", c.nombre, c.ruta, clave, c.clave)
		}
	}
	sel.Caja().PonerTexto("")
	sel.subida.abierta = true
	sel.montar()
	sel.subida.nombre.PonerTexto("Gato")
	sel.AlElegirImagen(func(r string) { sel.subida.ruta = r })
	sel.subirPropio()
	if !s.pidio("CreateCustomEmoji(gato,f-gato.png)") || sel.subida.abierta || sel.subida.fallo != "" {
		t.Fatalf("subir: %v · %q", s.llamadas, sel.subida.fallo)
	}
	nombres := make([]string, 0, len(catalogoOrg.lista))
	for _, e := range catalogoOrg.lista {
		nombres = append(nombres, e.Name)
	}
	if !slices.Equal(nombres, []string{"baile", "gato", "loro"}) {
		t.Errorf("tras subir, el catálogo es %v", nombres)
	}
}
