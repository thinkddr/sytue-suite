// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestCapturasDeLosDialogos deja en SYTUE_CAPTURA_DIALOGOS las cuatro muestras de la
// familia —confirmación, formulario, uno grande con desplazamiento y el panel lateral—
// en escritorio (1440×900) y teléfono (390×844), claro y oscuro: las mismas vistas que
// el arnés de la web, para mirarlas lado a lado. Sin la variable no hace nada: no es un
// guardián, es la cámara.
func TestCapturasDeLosDialogos(t *testing.T) {
	if os.Getenv("SYTUE_CAPTURA_DIALOGOS") == "" {
		t.Skip("sin SYTUE_CAPTURA_DIALOGOS")
	}
	tema.ElegirForma("carril")
	t.Cleanup(func() { tema.ElegirForma(""); tema.Elegir("") })
	for _, tam := range []struct {
		nombre  string
		ventana fyne.Size
		tactil  bool
	}{
		{"escritorio", fyne.NewSize(1440, 900), false},
		{"movil", fyne.NewSize(390, 844), true},
	} {
		for _, tono := range []string{"claro", "oscuro"} {
			for _, m := range []string{"confirmar", "formulario", "largo", "lateral"} {
				tema.Elegir(tono)
				tactil := tam.tactil
				antes := tactilDePrueba
				tactilDePrueba = &tactil
				v := tema.De(0)
				w := test.NewTempWindow(t, fondoDeMuestra(v, tam.ventana.Width))
				w.SetPadded(false)
				w.Canvas().(interface{ SetScale(float32) }).SetScale(2)
				w.Resize(tam.ventana)
				d := muestraDeDialogo(m, v)
				d.Mostrar(w.Canvas())
				guardarDialogoMovil(t, w.Canvas(), fmt.Sprintf("%s-%s-%s", m, tam.nombre, tono))
				d.Cerrar()
				tactilDePrueba = antes
			}
		}
	}
}

// fondoDeMuestra es una pantalla de la suite reducida a lo que se ve detrás del velo: el
// carril y unas filas en la hoja.
func fondoDeMuestra(v tema.Variante, ancho float32) fyne.CanvasObject {
	filas := container.New(Columna{Hueco: 4})
	for _, s := range []string{"Plan del trimestre", "Notas de la reunión de equipo", "Enviar la propuesta a Marta", "Preparar la demo del jueves", "Documentación del piloto", "Recordar: las licencias vencen en diciembre"} {
		filas.Add(NuevaFilaLista(s, "Trabajo · hoy"))
	}
	hoja := container.NewStack(fcanvas.NewRectangle(tema.Color("bg", v)),
		container.New(Margen{Izq: 24, Arr: 24, Der: 24, Abj: 24}, filas))
	if ancho < Estrecho {
		barra := fcanvas.NewRectangle(tema.Color("sidebar-bg", v))
		barra.SetMinSize(fyne.NewSize(0, 56))
		return container.NewBorder(barra, nil, nil, nil, hoja)
	}
	carril := fcanvas.NewRectangle(tema.Color("sidebar-bg", v))
	carril.SetMinSize(fyne.NewSize(260, 0))
	return container.NewBorder(nil, nil, carril, nil, hoja)
}

func muestraDeDialogo(m string, v tema.Variante) *Dialogo {
	parrafo := func(s string) fyne.CanvasObject { return NuevoParrafo(s, tema.TextoSM, Normal, "text", v) }
	pie := func(rotulo string) fyne.CanvasObject {
		return container.New(Fila{Hueco: HuecoPieDialogo},
			NuevoBoton("Cancelar", nil).Con(Fantasma, TallaMD),
			NuevoBoton(rotulo, nil).Con(Primario, TallaMD))
	}
	campo := func(rotulo, valor string, obligatorio bool) fyne.CanvasObject {
		e := NuevaEntrada("")
		e.PonerTexto(valor)
		return GrupoCampo(rotulo, obligatorio, e, "", "")
	}
	dos := func(a, b fyne.CanvasObject) fyne.CanvasObject { return container.NewGridWithColumns(2, a, b) }
	switch m {
	case "confirmar":
		return Confirmacion("Mover a la papelera", "Cancelar", "Mover a la papelera", true,
			parrafo("«Plan del trimestre» y sus 6 notas irán a la papelera. Se pueden recuperar durante 30 días."), nil, nil)
	case "formulario":
		desc := NuevaAreaTexto("")
		cuerpo := container.New(Columna{Hueco: 14},
			campo("Título", "Revisión del presupuesto", true),
			dos(campo("Empieza", "16/10/2026 10:00", false), campo("Termina", "16/10/2026 11:00", false)),
			NuevaCasilla("Todo el día", nil),
			GrupoCampo("Calendario", false, NuevoSelector([]string{"Trabajo", "Personal"}, nil), "", ""),
			GrupoCampo("Descripción", false, desc, "Se envía a las personas invitadas.", ""))
		return NuevoDialogo("Nuevo evento", cuerpo).Con(DialogoMD, pie("Guardar"))
	case "largo":
		lista := container.New(Columna{})
		for _, p := range [][2]string{
			{"Ana Ruiz", "ana@acme.es"},
			{"Mario Gil", "mario@acme.es"},
			{"Marta López", "marta@acme.es"},
			{"Equipo de ventas", "ventas@acme.es"},
			{"Lucía Fernández", "lucia@acme.es"},
			{"Pablo Sánchez", "pablo@acme.es"},
			{"Irene Navarro", "irene@acme.es"},
			{"Javier Torres", "javier@acme.es"},
			{"Elena Castro", "elena@acme.es"},
			{"Soporte", "soporte@acme.es"},
			{"Raúl Moreno", "raul@acme.es"},
			{"Clara Ortega", "clara@acme.es"},
			{"Dirección", "direccion@acme.es"},
			{"Sergio Romero", "sergio@acme.es"},
			{"Nuria Gómez", "nuria@acme.es"},
			{"Hugo Díaz", "hugo@acme.es"},
		} {
			f := NuevaFilaLista(p[0], p[1])
			f.Delantero = NuevoAvatar(p[0])
			lista.Add(f)
		}
		buscar := container.NewBorder(nil, nil, nil, NuevoBoton("Invitar", nil).Con(Secundario, TallaMD), NuevaEntrada("Añadir personas o grupos"))
		return NuevoDialogo("Compartir «Proyectos 2026»", container.New(Columna{Hueco: 14}, buscar, lista)).Con(DialogoLG, pie("Guardar cambios"))
	default:
		desc := NuevaAreaTexto("")
		cuerpo := container.New(Columna{Hueco: 14},
			campo("Nombre", "Licencia anual Sytue One", true),
			dos(campo("Referencia", "LIC-ONE-12", false), campo("Precio", "120,00 €", false)),
			GrupoCampo("Impuesto", false, NuevoSelector([]string{"IVA general (21 %)"}, nil), "", ""),
			GrupoCampo("Categoría", false, NuevoSelector([]string{"Software"}, nil), "", ""),
			GrupoCampo("Descripción", false, desc, "", ""),
			NuevaCasilla("Se vende por suscripción", nil),
			NuevaCasilla("Visible en el catálogo público", nil))
		return NuevoDialogo("Nuevo producto", cuerpo).Con(DialogoMD, pie("Crear producto")).ALaDerecha()
	}
}
