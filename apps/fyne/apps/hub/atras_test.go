// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
)

func TestElAtrasDelHubVuelveDePasosYPantallas(t *testing.T) {
	v := NuevaVista(traductor(t), nil, time.Now())
	w := test.NewWindow(v.Objeto())
	defer w.Close()
	v.Atajos(w.Canvas())
	atras := func() { w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack}) }
	v.mostrar(ADispositivos)
	v.disp.Ir(2)
	atras()
	if v.Destino() != ADispositivos || v.disp.Paso != 1 {
		t.Fatal("atrás debe retroceder un paso del asistente de dispositivos")
	}
	atras()
	atras()
	if v.Destino() != AInicio || v.atras() {
		t.Fatal("desde el primer paso debe volver a Inicio y después al sistema")
	}
	v.mostrar(ABienvenida)
	v.bienv.paso, v.bienv.nombre = 2, "Mi organización"
	atras()
	if v.bienv.paso != 1 || v.bienv.nombre != "Mi organización" || v.Destino() != ABienvenida {
		t.Fatal("volver de un paso del alta no debe perder el formulario")
	}
	v.bienv.ocupado = true
	atras()
	if v.bienv.paso != 1 || v.Destino() != ABienvenida {
		t.Fatal("atrás no debe salir mientras el alta se está guardando")
	}
}
