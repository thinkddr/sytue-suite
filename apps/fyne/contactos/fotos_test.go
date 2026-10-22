// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func pngDe(t *testing.T, c color.NRGBA, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestLaFichaEnsenaLaFotoYLaCambia: al abrir la ficha se pide su foto por UID y el
// avatar la enseña; un color predefinido la sube como PNG y «Quitar foto» la borra.
func TestLaFichaEnsenaLaFotoYLaCambia(t *testing.T) {
	ana := persona("ana", "Ana")
	s := &servicioFalso{contactos: []api.Contact{ana}, fotos: map[string][]byte{"u-ana": pngDe(t, color.NRGBA{R: 200, A: 255}, 8, 8)}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.Pantalla.AlElegirContacto(a.Pantalla.Contactos[0])
	if a.Pantalla.Foto == nil {
		t.Fatal("la ficha no ha traído la foto")
	}
	sel, es := a.Pantalla.avatarDeFicha(ana).(*ui.SelectorAvatar)
	if !es || sel.Avatar.Foto == nil || !sel.TieneFoto {
		t.Fatalf("el avatar de la ficha es %#v", sel)
	}
	sel.AlElegirColor("avatar-3")
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "PUT /api/v1/contacts/photo" ||
		!strings.Contains(s.escrituras[0].cuerpo, `"data":"data:image/png;base64,`) ||
		!strings.Contains(s.escrituras[0].cuerpo, `"uid":"u-ana"`) {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	sel.AlQuitar()
	if len(s.escrituras) != 2 || s.escrituras[1].ruta != "DELETE /api/v1/contacts/photo" {
		t.Fatalf("escrituras %v", s.escrituras)
	}
}

// TestLaFotoDeUnFicheroEsElCuadradoDelCentro es `fileToAvatarDataUri`: una imagen
// apaisada queda en un JPEG de 256 × 256 con el centro, y lo que no es imagen dice
// «formato no admitido».
func TestLaFotoDeUnFicheroEsElCuadradoDelCentro(t *testing.T) {
	uri, err := FotoDeFichero(pngDe(t, color.NRGBA{G: 180, A: 255}, 600, 300))
	if err != nil || !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Fatalf("%q, %v", uri[:min(len(uri), 40)], err)
	}
	img := imagenDeURI(uri)
	if img == nil || img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("la foto mide %v", img.Bounds())
	}
	if _, err := FotoDeFichero([]byte("no soy una imagen")); !errors.Is(err, errFotoFormato) {
		t.Errorf("con texto: %v", err)
	}
}

// TestElFormularioSubeLaFotoAlCrear: al crear aún no hay UID, así que la foto elegida
// se sube DESPUÉS, con el del contacto recién creado.
func TestElFormularioSubeLaFotoAlCrear(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a, f := formularioDePrueba(t, s, false)
	f.nombre.PonerTexto("Nuevo")
	f.selector.AlElegirColor("avatar-1")
	if f.avatar.Foto == nil || !f.selector.TieneFoto {
		t.Fatal("el color elegido no se ve en la vista previa")
	}
	f.guardar()
	if len(s.escrituras) != 2 || s.escrituras[0].ruta != "POST /api/v1/address-books/l1/contacts" ||
		s.escrituras[1].ruta != "PUT /api/v1/contacts/photo" || !strings.Contains(s.escrituras[1].cuerpo, `"uid":"u-nuevo"`) {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	_ = a
}

// TestLaListaEnsenaLaFotoDeQuienLaTiene: la fila pide la foto al pintarse, SOLO la de
// quien la tiene (`has_photo`) y una vez por versión de la tarjeta, y su avatar la
// enseña; la de quien no la tiene no se pide aunque el servicio la guardara.
func TestLaListaEnsenaLaFotoDeQuienLaTiene(t *testing.T) {
	ana, bruno := persona("ana", "Ana"), persona("bruno", "Bruno")
	ana.HasPhoto, ana.ETag = true, "e1"
	rojo := pngDe(t, color.NRGBA{R: 200, A: 255}, 8, 8)
	s := &servicioFalso{contactos: []api.Contact{ana, bruno}, fotos: map[string][]byte{"u-ana": rojo, "u-bruno": rojo}}
	a := appDePrueba(t, s)
	a.Cargar()
	s.correrCola()
	if !slices.Equal(s.fotosVistas, []string{"u-ana"}) {
		t.Fatalf("fotos pedidas al pintar la lista: %v", s.fotosVistas)
	}
	f := NuevaFilaContacto()
	a.Pantalla.rellenarFila(f, a.Pantalla.Contactos[0])
	if r, es := test.WidgetRenderer(f).(*filaContactoRenderer); !es || f.Foto == nil || r.avatar.Foto == nil {
		t.Fatal("la fila de Ana no enseña su foto")
	}
	// Y se VE: el centro del avatar es el rojo de la foto, no el color de las iniciales.
	alto := MedidaFilaContacto(false)
	x, y := int(xBoton()+ui.MedidaAvatar(ui.AvatarSM).Width/2), int(alto/2)
	if img := pintarObjeto(t, f, fyne.NewSize(360, alto)); !hayColorCerca(img, x, y, color.NRGBA{R: 200, A: 255}) {
		t.Fatalf("en el centro del avatar de Ana hay %v", px(img, x, y))
	}
	a.Pantalla.rellenarFila(f, a.Pantalla.Contactos[1])
	if f.Foto != nil {
		t.Fatal("la fila reciclada para Bruno se queda con la foto de Ana")
	}
	// Al recargar, la misma tarjeta no se vuelve a pedir; una versión nueva, sí, una vez.
	a.recargar(*a.Pantalla.Activa)
	s.correrCola()
	if len(s.fotosVistas) != 1 {
		t.Fatalf("la misma tarjeta se ha vuelto a pedir: %v", s.fotosVistas)
	}
	s.mu.Lock()
	s.contactos[0].ETag = "e2"
	s.mu.Unlock()
	a.recargar(*a.Pantalla.Activa)
	s.correrCola()
	if len(s.fotosVistas) != 2 {
		t.Fatalf("con la tarjeta cambiada: %v", s.fotosVistas)
	}
}
