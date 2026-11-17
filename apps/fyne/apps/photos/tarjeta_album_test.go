// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// recorrerAlbumes baja por los contenedores y devuelve, en orden, los títulos de las
// tarjetas de álbum, las insignias y los selectores que encuentra.
func recorrerAlbumes(o fyne.CanvasObject) (titulos []string, insignias []*ui.Insignia, selectores []*ui.Selector) {
	var bajar func(fyne.CanvasObject)
	bajar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *fyne.Container:
			if l, ok := x.Layout.(tarjetaAlbumLayout); ok {
				titulos = append(titulos, l.titulo)
			}
			for _, h := range x.Objects {
				bajar(h)
			}
		case *ui.Insignia:
			insignias = append(insignias, x)
		case *ui.Selector:
			selectores = append(selectores, x)
		}
	}
	bajar(o)
	return titulos, insignias, selectores
}

func albumesDePrueba() []api.Album {
	dia := func(d int) time.Time { return time.Date(2026, 6, d, 10, 0, 0, 0, time.UTC) }
	// Llegan como los da el servicio: el último modificado primero.
	return []api.Album{
		{ID: "a1", Title: "Zoo", Count: 2, UpdatedAt: dia(3)},
		{ID: "a2", Title: "Boda", Count: 40, UpdatedAt: dia(2), Shared: true},
		{ID: "a3", Title: "Ávila", Count: 7, UpdatedAt: dia(1)},
	}
}

// TestTarjetaAlbumDiceSiEstaCompartido: un álbum con enlace público lleva la insignia
// «Compartido» en la esquina de la portada, sin cambiar el alto de la tarjeta; uno sin
// enlace, no.
func TestTarjetaAlbumDiceSiEstaCompartido(t *testing.T) {
	test.NewTempApp(t)
	v := vistaDePrueba(t, nil)
	al := albumesDePrueba()[1]

	con := TarjetaAlbum(v, al, nil).(*fyne.Container)
	_, insignias, _ := recorrerAlbumes(con)
	if len(insignias) != 1 || insignias[0].Texto != "Compartido" {
		t.Fatalf("la tarjeta de un álbum compartido lleva %v insignias; quiero «Compartido»", len(insignias))
	}
	al.Shared = false
	sin := TarjetaAlbum(v, al, nil).(*fyne.Container)
	if _, insignias, _ := recorrerAlbumes(sin); len(insignias) != 0 {
		t.Errorf("un álbum sin enlace lleva %d insignias", len(insignias))
	}

	con.Resize(fyne.NewSize(200, con.MinSize().Height))
	sin.Resize(fyne.NewSize(200, sin.MinSize().Height))
	if con.MinSize() != sin.MinSize() {
		t.Errorf("la insignia cambia la tarjeta de %v a %v", sin.MinSize(), con.MinSize())
	}
	ins := con.Objects[3]
	if p := ins.Position(); p.X != margenInsigniaAlbum || p.Y != margenInsigniaAlbum {
		t.Errorf("la insignia está en %v; va en la esquina de la portada", p)
	}
	if fin := ins.Position().Y + ins.Size().Height; fin > con.Objects[0].Size().Height {
		t.Errorf("la insignia acaba en %g, fuera de la portada de %g", fin, con.Objects[0].Size().Height)
	}
}

// TestAlbumesSeOrdenanComoSeElige: «Ordenar por» reordena las tarjetas y avisa del
// orden elegido; con un solo álbum no aparece.
func TestAlbumesSeOrdenanComoSeElige(t *testing.T) {
	test.NewTempApp(t)
	v := vistaDePrueba(t, nil)
	lista := albumesDePrueba()

	casos := map[OrdenAlbum][]string{
		"":                 {"Zoo", "Boda", "Ávila"},
		OrdenAlbumReciente: {"Zoo", "Boda", "Ávila"},
		OrdenAlbumTitulo:   {"Ávila", "Boda", "Zoo"},
		OrdenAlbumCuenta:   {"Boda", "Ávila", "Zoo"},
	}
	for o, quiero := range casos {
		titulos, _, sels := recorrerAlbumes(PantallaAlbumes(v, Albumes{Lista: lista, Orden: o}))
		if !slices.Equal(titulos, quiero) {
			t.Errorf("orden %q: %v, quiero %v", o, titulos, quiero)
		}
		if len(sels) != 1 {
			t.Fatalf("orden %q: %d selectores", o, len(sels))
		}
		if sels[0].Rotulo != "Ordenar por" || sels[0].Valor() != v.Traduce("albums.sort."+string(ordenOReciente(o))) {
			t.Errorf("orden %q: el selector se llama %q y marca %q", o, sels[0].Rotulo, sels[0].Valor())
		}
	}

	var elegido OrdenAlbum
	_, _, sels := recorrerAlbumes(PantallaAlbumes(v, Albumes{Lista: lista, AlOrdenar: func(o OrdenAlbum) { elegido = o }}))
	if want := []string{"Modificados recientemente", "Nombre", "Más fotos"}; !slices.Equal(sels[0].Opciones, want) {
		t.Errorf("opciones %v, quiero %v", sels[0].Opciones, want)
	}
	sels[0].AlElegir(2)
	if elegido != OrdenAlbumCuenta {
		t.Errorf("elegir «Más fotos» avisa con %q", elegido)
	}

	if _, _, sels := recorrerAlbumes(PantallaAlbumes(v, Albumes{Lista: lista[:1]})); len(sels) != 0 {
		t.Error("con un solo álbum no hay nada que ordenar y sale el selector")
	}
}

func ordenOReciente(o OrdenAlbum) OrdenAlbum {
	if o == "" {
		return OrdenAlbumReciente
	}
	return o
}
