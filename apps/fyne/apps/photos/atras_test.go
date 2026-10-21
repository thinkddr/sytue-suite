// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
)

func TestElAtrasCierraVisorYRecorteAntesDeSalir(t *testing.T) {
	a, w := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(1), porPagina: 100})
	a.Arrancar()
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	a.abrirVisor(desdeGaleria, 0)
	atras()
	if a.vis != nil {
		t.Fatal("atrás no cierra el visor")
	}
	a.AbrirEditor(a.g.fotos[0], "")
	e := a.ed
	e.recortando = true
	atras()
	if a.ed != e || e.recortando {
		t.Fatal("atrás debe cancelar el recorte conservando el editor")
	}
	atras()
	if a.ed != nil || a.Ruta() != RutaFotos {
		t.Fatal("el segundo atrás no vuelve del editor a la galería")
	}
	a.Ir(RutaFavoritas)
	atras()
	if a.Ruta() != RutaFotos {
		t.Fatal("atrás desde una sección no vuelve a Fotos")
	}
}
