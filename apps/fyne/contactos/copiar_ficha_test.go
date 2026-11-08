// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// botonesIcono son los botones de glifo de un árbol, por su nombre accesible.
func botonesIcono(o fyne.CanvasObject) map[string]*ui.BotonIcono {
	fuera := map[string]*ui.BotonIcono{}
	var andar func(fyne.CanvasObject)
	andar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				andar(h)
			}
		case *container.Scroll:
			andar(x.Content)
		case *ui.Tarjeta:
			andar(x.Cuerpo)
		case *ui.Pista:
			andar(x.Hijo)
		case *ui.BotonIcono:
			fuera[x.Rotulo] = x
		}
	}
	andar(o)
	return fuera
}

// TestCadaValorDeLaFichaSeCopia: los mismos casos que copiar-ficha.test.tsx. Cada
// correo, cada teléfono y cada dirección llevan su botón, con el valor en el nombre
// accesible, que copia ESE valor —la dirección entera, en líneas— con su aviso.
func TestCadaValorDeLaFichaSeCopia(t *testing.T) {
	p := pantallaDeMuestra(t)
	c := fichaDeMuestra()
	p.Seleccionado = &c
	type copia struct{ valor, aviso string }
	var copiado []copia
	p.AlCopiar = func(valor, aviso string) { copiado = append(copiado, copia{valor, aviso}) }
	bs := botonesIcono(p.ficha())
	for _, rotulo := range []string{
		"Copiar ada@casa.test", "Copiar ada@trabajo.test", "Copiar +34 600 11 22 33",
		"Copiar 12 St James's Square",
	} {
		b, ok := bs[rotulo]
		if !ok {
			t.Fatalf("no hay botón «%s»", rotulo)
		}
		b.Tapped(&fyne.PointEvent{})
	}
	quiero := []copia{
		{"ada@casa.test", "detail.emailCopied"},
		{"ada@trabajo.test", "detail.emailCopied"},
		{"+34 600 11 22 33", "detail.phoneCopied"},
		{"12 St James's Square\nSW1Y 4LB London\nUK", "detail.addressCopied"},
	}
	if len(copiado) != len(quiero) {
		t.Fatalf("se copió %v", copiado)
	}
	for i := range quiero {
		if copiado[i] != quiero[i] {
			t.Errorf("el %dº copia %+v y debería %+v", i+1, copiado[i], quiero[i])
		}
	}
}

// TestCopiarLoDejaEnElPortapapelesYLoDice: la app lo deja en el portapapeles y avisa
// con lo que se ha copiado.
func TestCopiarLoDejaEnElPortapapelesYLoDice(t *testing.T) {
	a := appDePrueba(t, &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}})
	a.Pantalla.AlCopiar("+34 600 11 22 33", "detail.phoneCopied")
	if got := fyne.CurrentApp().Clipboard().Content(); got != "+34 600 11 22 33" {
		t.Errorf("el portapapeles tiene %q", got)
	}
	av := a.avisos.Avisos()
	if len(av) != 1 || av[0].Tono != ui.AvisoExito || av[0].Titulo != "Teléfono copiado" {
		t.Errorf("avisos %v", av)
	}
}
