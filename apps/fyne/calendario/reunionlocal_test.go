// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Sytue Office (A-13): con el campo libre del enlace y Pantalla.Reunion, bajo el campo va
// el botón que crea la reunión con lo que haya en el calendario elegido. En uno cuyo
// proveedor la crea al guardar (Google, Microsoft) queda pedida —con su frase y
// «Quitar»— y, al guardar, se pide con el UID del evento; en otro, se crea ya y su enlace
// se pega en el campo; donde no hay con qué, el campo libre sin más. Un enlace escrito a
// mano quita el botón y lo pedido; cambiar de calendario olvida lo pedido. La edición
// conectada no lo mira.
func TestReunionDesdeElFormulario(t *testing.T) {
	a, _ := appConVentana(t, &escrituras{})
	a.P.SinPersonas = true
	a.P.PistaEnlace = "Pega el enlace"
	var creadas, pedidas []string
	falla := false
	a.P.Reunion = &Reunion{
		Boton: func(cal string) string {
			return map[string]string{"google": "Añadir Google Meet", "outlook": "Reunión de Teams", "caldav": "Crear reunión de Microsoft Teams"}[cal]
		},
		Pendiente: func(cal string) string {
			return map[string]string{"google": "Google crea el enlace al guardar", "outlook": "Outlook crea el enlace al guardar"}[cal]
		},
		Crear: func(_ context.Context, cal, titulo string, inicio, fin time.Time) (string, error) {
			creadas = append(creadas, cal+"|"+titulo+"|"+inicio.Format("15:04")+"-"+fin.Format("15:04"))
			if falla {
				return "", errors.New("sin red")
			}
			return "https://teams.microsoft.com/l/meetup-join/abc", nil
		},
		Pedir: func(cal, uid string) { pedidas = append(pedidas, cal+"|"+uid) },
	}
	destinos := []api.Calendar{
		{ID: "local", DisplayName: "Personal", IsDefault: true},
		{ID: "google", DisplayName: "Ana (Google)"},
		{ID: "caldav", DisplayName: "Trabajo (Nextcloud)"},
		{ID: "outlook", DisplayName: "Calendario (Outlook)"},
	}
	inicio := time.Date(2031, 3, 4, 9, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(30 * time.Minute), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, destinos, a.accionesFormulario(e))
	f.titulo.PonerTexto("Revisión")

	// En el de este equipo no hay con qué: el campo libre, sin botón.
	if f.botonReunion() != "" || buscarBoton(f.cuerpo, "Añadir Google Meet") != nil {
		t.Fatal("en un calendario sin proveedor de reuniones no hay botón")
	}
	// En el de Google: «Añadir Google Meet» la deja pedida, con su frase y «Quitar».
	f.calendario.AlElegir(1)
	b := buscarBoton(f.cuerpo, "Añadir Google Meet")
	if b == nil {
		t.Fatal("en un calendario de Google, «Añadir Google Meet»")
	}
	b.Tapped(nil)
	if !f.reunionPedida || !strings.Contains(textosDe(f.cuerpo), "Google crea el enlace al guardar") || len(creadas) != 0 {
		t.Fatalf("pedida: %v %q %v", f.reunionPedida, textosDe(f.cuerpo), creadas)
	}
	quitar := buscarBoton(f.cuerpo, f.p.t("common.remove", nil))
	if quitar == nil {
		t.Fatal("lo pedido se quita")
	}
	quitar.Tapped(nil)
	if f.reunionPedida || buscarBoton(f.cuerpo, "Añadir Google Meet") == nil {
		t.Fatal("quitada, vuelve el botón")
	}
	buscarBoton(f.cuerpo, "Añadir Google Meet").Tapped(nil)

	// Otro calendario: lo pedido a Google se olvida; en el CalDAV con una cuenta de
	// Teams, la reunión se crea ya y el enlace va al campo.
	f.calendario.AlElegir(2)
	if f.reunionPedida {
		t.Fatal("lo pedido a Google no vale en otro calendario")
	}
	falla = true
	buscarBoton(f.cuerpo, "Crear reunión de Microsoft Teams").Tapped(nil)
	if !f.errorReunion || !strings.Contains(textosDe(f.cuerpo), f.p.t("event.conferenceError", nil)) || f.e.Videollamada != "" {
		t.Fatal("si no se puede crear, se dice")
	}
	falla = false
	buscarBoton(f.cuerpo, "Crear reunión de Microsoft Teams").Tapped(nil)
	if len(creadas) != 2 || creadas[1] != "caldav|Revisión|09:00-09:30" {
		t.Fatalf("creada con el título y las horas: %v", creadas)
	}
	if f.enlace.Texto() != "https://teams.microsoft.com/l/meetup-join/abc" || f.e.Videollamada != f.enlace.Texto() || f.errorReunion {
		t.Fatalf("el enlace va al campo: %q %q", f.enlace.Texto(), f.e.Videollamada)
	}
	if buscarBoton(f.cuerpo, "Crear reunión de Microsoft Teams") != nil {
		t.Fatal("con un enlace puesto no hay botón")
	}

	// En Outlook, con el campo vaciado: pedida; un enlace escrito a mano la olvida.
	f.calendario.AlElegir(3)
	f.enlace.PonerTexto("")
	buscarBoton(f.cuerpo, "Reunión de Teams").Tapped(nil)
	f.enlace.PonerTexto("https://zoom.example/j/1")
	if f.reunionPedida || buscarBoton(f.cuerpo, "Reunión de Teams") != nil {
		t.Fatal("un enlace a mano olvida lo pedido y quita el botón")
	}
	f.enlace.PonerTexto("")
	buscarBoton(f.cuerpo, "Reunión de Teams").Tapped(nil)

	// Guardar: se pide con el UID con que se guarda el evento.
	var guardada Edicion
	f.a.Guardar = func(e Edicion, listo func(bool)) { guardada = e; listo(false) }
	f.enviar()
	if len(pedidas) != 1 || guardada.UID == "" || pedidas[0] != "outlook|"+guardada.UID || guardada.CalendarioID != "outlook" {
		t.Fatalf("pedida al guardar: %v %+v", pedidas, guardada)
	}

	// En consulta no hay botón; sin Reunion, el campo libre de siempre; la conectada no
	// lo mira.
	e.SoloLectura = true
	e.CalendarioID = "google"
	if g := a.P.nuevoFormulario(e, destinos, a.accionesFormulario(e)); g.botonReunion() != "" {
		t.Fatal("en consulta no se crea nada")
	}
	c := appDePrueba(t, &servicioFalso{})
	c.P.Reunion = a.P.Reunion
	e.SoloLectura = false
	if g := c.P.nuevoFormulario(e, destinos, c.accionesFormulario(e)); g.botonReunion() != "" || g.a.Reunion != nil {
		t.Fatal("la edición conectada no mira Reunion")
	}
}
