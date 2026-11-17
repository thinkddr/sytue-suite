// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// EL COMPOSITOR NO OFRECE LO QUE EL CIFRADO NO DEJA HACER. Programar, encuesta, tarea y
// pedir acuse los guarda o los cuenta el servicio, que con cifrado de extremo a extremo
// no ve el mensaje. Con cifrado siguen a la vista pero apagados, con la línea de por qué;
// las tarjetas y los acuses ya publicados, en solo lectura con `matrix.unsupported`. Sin
// cifrado, todo igual. Gemelo de la web:
// apps/frontend/apps/rooms/src/components/cifradoNoOfrece.test.tsx.
//
// VISTO EN ROJO (2026-11-08): con `DisponibleConCifrado` devolviendo siempre true, la
// rama con cifrado ofrece los cuatro mandos, vota, confirma y no pinta ninguna nota.

func TestDisponibleConCifrado(t *testing.T) {
	for _, a := range noDisponiblesCifrado {
		if !DisponibleConCifrado(a, false) || DisponibleConCifrado(a, true) {
			t.Errorf("%s: sin cifrado %v, con cifrado %v", a, DisponibleConCifrado(a, false), DisponibleConCifrado(a, true))
		}
	}
	if len(noDisponiblesCifrado) != 6 {
		t.Errorf("la lista tiene %d acciones y la web seis", len(noDisponiblesCifrado))
	}
}

// salaCifrada es la sala de prueba con el cifrado puesto o quitado. El ciclo de vida
// del dispositivo no se arranca: solo cuenta que exista.
func salaCifrada(t *testing.T, s *servicioFalso, cifrado bool) *Sala {
	t.Helper()
	sa, _ := salaDePrueba(t, s)
	if cifrado {
		sa.Cifrado = NuevoCifrado(sa, nil, mx.Config{}, nil)
	}
	sa.Arrancar(context.Background())
	return sa
}

// textos son los de los rótulos y párrafos que cuelgan de o.
func textos(o fyne.CanvasObject) []string {
	var out []string
	var bajar func(fyne.CanvasObject)
	bajar = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *ui.Parrafo:
			out = append(out, v.Texto)
		case *ui.Rotulo:
			out = append(out, v.Texto)
		case *fyne.Container:
			for _, h := range v.Objects {
				bajar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(v).Objects() {
				bajar(h)
			}
		}
	}
	bajar(o)
	return out
}

func dice(o fyne.CanvasObject, texto string) bool {
	for _, s := range textos(o) {
		if s == texto {
			return true
		}
	}
	return false
}

// piezasEn son los objetos de tipo P que cuelgan de o, en orden.
func piezasEn[P fyne.CanvasObject](o fyne.CanvasObject) []P {
	var out []P
	var bajar func(fyne.CanvasObject)
	bajar = func(o fyne.CanvasObject) {
		if p, ok := o.(P); ok {
			out = append(out, p)
			return
		}
		switch v := o.(type) {
		case *fyne.Container:
			for _, h := range v.Objects {
				bajar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(v).Objects() {
				bajar(h)
			}
		}
	}
	bajar(o)
	return out
}

func TestCompositorConYSinCifrado(t *testing.T) {
	for _, cifrado := range []bool{true, false} {
		t.Run(map[bool]string{true: "con cifrado", false: "sin cifrado"}[cifrado], func(t *testing.T) {
			sa := salaCifrada(t, servicioConDos(), cifrado)
			tr := sa.P.T
			nota := tr.T("matrix.notYetEncrypted", nil)
			c := sa.P.Canal.Compositor

			// «+»: encuesta y tarea, a la vista y apagadas, con la nota debajo.
			sa.MenuMas(c)
			vistas, notas := 0, 0
			for _, e := range sa.menuMas.Elementos {
				if it, ok := e.(*ui.ElementoMenu); ok && (it.Texto == tr.T("card.newPoll", nil) || it.Texto == tr.T("card.newTask", nil)) {
					vistas++
					if it.Inerte != cifrado {
						t.Errorf("«%s» inerte=%v", it.Texto, it.Inerte)
					}
				}
				if dice(e, nota) {
					notas++
				}
			}
			if vistas != 2 || (notas == 1) != cifrado {
				t.Errorf("el «+» trae %d de encuesta y tarea y %d notas", vistas, notas)
			}
			sa.menuMas.Cerrar()

			// «!»: la prioridad se elige igual; pedir acuse, apagado.
			test.Tap(c.botones[len(c.botones)-2].(*ui.BotonIcono))
			elecciones := eleccionesEn(c.emergente)
			if len(elecciones) != 4 {
				t.Fatalf("el «!» trae %d controles", len(elecciones))
			}
			for _, r := range elecciones[:3] {
				if r.Disabled() {
					t.Error("con cifrado se apaga también la prioridad")
				}
			}
			test.Tap(elecciones[3])
			if elecciones[3].Disabled() != cifrado || c.Acuse == cifrado || dice(c.emergente, nota) != cifrado {
				t.Errorf("pedir acuse: apagado=%v, marcado=%v, nota=%v", elecciones[3].Disabled(), c.Acuse, dice(c.emergente, nota))
			}
			c.emergente.Cerrar()
			c.PonerAcuse(false)

			// El reloj: los atajos a la vista y apagados; pulsarlos no programa.
			test.Tap(c.botones[len(c.botones)-1].(*ui.BotonIcono))
			filas := piezasEn[*FilaEmergente](c.emergente)
			if len(filas) != 3 {
				t.Fatalf("«Enviar más tarde» trae %d filas", len(filas))
			}
			for _, f := range filas {
				if f.Disabled() != cifrado {
					t.Errorf("«%s» inerte=%v", f.Rotulo, f.Disabled())
				}
			}
			test.Tap(filas[0])
			if c.ProgramarA.IsZero() != cifrado || dice(c.emergente, nota) != cifrado {
				t.Errorf("el atajo deja la hora %v y nota=%v", c.ProgramarA, dice(c.emergente, nota))
			}
			if e := c.emergente; e != nil && e.Abierto() {
				e.Cerrar()
			}
		})
	}
}

func TestTarjetaYAcuseConYSinCifrado(t *testing.T) {
	for _, cifrado := range []bool{true, false} {
		t.Run(map[bool]string{true: "con cifrado", false: "sin cifrado"}[cifrado], func(t *testing.T) {
			s := servicioConDos()
			s.mensajes["c1"][0].Card = &api.RoomsCard{Kind: "poll", Options: []string{"Viernes", "Lunes"}}
			s.mensajes["c1"][0].RequiresAck, s.mensajes["c1"][0].AckCount = true, 2
			sa := salaCifrada(t, s, cifrado)
			tr := sa.P.T
			f := filaDe(t, sa.P.Canal.Historial, "m1")
			test.WidgetRenderer(f)
			if f.tarjeta == nil {
				t.Fatal("la fila de la encuesta no lleva su tarjeta")
			}
			opciones := piezasEn[*OpcionEncuesta](f.tarjeta)
			if len(opciones) != 2 {
				t.Fatalf("la encuesta trae %d opciones", len(opciones))
			}
			for _, o := range opciones {
				if o.Inerte != cifrado {
					t.Errorf("la opción «%s» inerte=%v", o.Rotulo, o.Inerte)
				}
			}
			if dice(f.tarjeta, tr.T("matrix.unsupported", nil)) != cifrado {
				t.Error("la nota de la tarjeta no sigue al cifrado")
			}
			sa.Votar(sa.P.Canal.Historial.Mensajes[0], "1", true)
			if s.pidio("VoteCard(m1,1,sí)") == cifrado {
				t.Errorf("votar con cifrado=%v pidió %v", cifrado, s.llamadas)
			}

			// El acuse: «Confirmar» o la nota, y el recuento siempre.
			f = filaDe(t, sa.P.Canal.Historial, "m1")
			test.WidgetRenderer(f)
			_, conBoton := f.acuse[0].(*ui.Boton)
			if conBoton == cifrado || len(f.acuse) != 2 {
				t.Errorf("la fila del acuse trae %d piezas y botón=%v", len(f.acuse), conBoton)
			}
			if r, ok := f.acuse[0].(*ui.Rotulo); cifrado && (!ok || r.Texto != tr.T("matrix.unsupported", nil)) {
				t.Errorf("con cifrado el acuse dice %#v", f.acuse[0])
			}
			sa.Acusar(sa.P.Canal.Historial.Mensajes[0])
			if s.pidio("Ack(m1)") == cifrado {
				t.Errorf("confirmar con cifrado=%v pidió %v", cifrado, s.llamadas)
			}
		})
	}
}
