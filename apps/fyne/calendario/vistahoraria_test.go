// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// La semana del 6 de julio de 2026: lejos de hoy, para que ni la línea roja ni el
// encuadre inicial dependan del reloj.
var lunesJulio = time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

func ocurrencia(id, titulo string, ini, fin time.Time, todoElDia bool) Ocurrencia {
	return Ocurrencia{
		Instance: api.Instance{
			Event: api.Event{ID: id, Summary: titulo, DTStart: ini, DTEnd: fin, AllDay: todoElDia},
			Start: ini, End: fin,
		},
		Color: AcentoPorDefecto, CalendarioID: "personal",
	}
}

func semanaDeMuestra(t *testing.T) *Pantalla {
	t.Helper()
	h := func(d, hh, mm int) time.Time { return time.Date(2026, 7, d, hh, mm, 0, 0, time.UTC) }
	p := pantallaDeMuestra(t)
	p.Cursor, p.Vista = lunesJulio, VistaSemana
	p.Eventos = []Ocurrencia{
		ocurrencia("a", "Reunión de producto", h(6, 9, 0), h(6, 10, 0), false),
		ocurrencia("b", "Revisión", h(6, 9, 30), h(6, 11, 0), false),
		ocurrencia("c", "Comité", h(8, 10, 0), h(8, 10, 15), false),
		ocurrencia("d", "Vacaciones", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), true),
	}
	return p
}

// TestLaSemanaSePintaConSusMedidas: la cabecera de días, la franja de «todo el día» con
// su barra, y los bloques colocados por hora y repartidos en columnas donde se solapan.
func TestLaSemanaSePintaConSusMedidas(t *testing.T) {
	p := semanaDeMuestra(t)
	d := p.disponerHorario(p.diasHorarios())
	if len(d.franja) != 1 || d.franja[0].Col != 2 || d.franja[0].Ancho != 3 {
		t.Fatalf("la barra de vacaciones (8–10 jul) no cubre miércoles a viernes: %+v", d.franja)
	}
	if n := len(d.bloques[0]); n != 2 || d.bloques[0][0].Cols != 2 {
		t.Fatalf("el lunes tiene %d bloques y no dos en dos columnas: %+v", n, d.bloques[0])
	}
	// La fila de días: versalita de 14 y número de 15,75 alineados por la base, dentro
	// de `py-2`, más el borde. Con Figtree da 18 de contenido: 35.
	if got := AltoFilaDias(); !cerca(got, 35, 0.3) {
		t.Errorf("la fila de días mide %v y las clases dan 35", got)
	}

	img := pintar(t, p)
	cab := altoCab + MedidaCabeceraHoraria(d)
	// El primer bloque del lunes empieza a las 9:00: con el encuadre de las 7:00 (las
	// 8:00 menos una hora) cae 2 h × 48 más abajo del borde de la cabecera.
	y := int(cab + 2*HoraPx + 36) // bajo el título y la franja horaria
	x0 := int(tema.AnchoLateral + AnchoEje)
	if got, quiero := px(img, x0+20, y), tema.Color("bg-subtle", tema.Claro); !mismoColor(got, quiero) {
		t.Errorf("dentro del bloque de las 9:00 (%d,%d) se pinta %v y el token `bg-subtle` es %v", x0+20, y, got, quiero)
	}
	// La franja de color de 3 px pegada al borde.
	if got := px(img, x0+2, y); !mismoColor(got, AcentoPorDefecto) {
		t.Errorf("la franja de color del bloque (%d,%d) es %v y no %v", x0+2, y, got, AcentoPorDefecto)
	}
	guardar(t, img, "calendario-semana")

	p.Vista = VistaDia
	p.Cursor = time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	p.scrollPuesto = false
	guardar(t, pintar(t, p), "calendario-dia")
}

// arrastrar simula un gesto de ratón sobre el cuerpo: pulsar en `desde` y soltar en
// `hasta`, en coordenadas del propio cuerpo.
func arrastrar(c *cuerpoArrastrable, desde, hasta fyne.Position) {
	e := &fyne.DragEvent{Dragged: fyne.NewDelta(hasta.X-desde.X, hasta.Y-desde.Y)}
	e.Position = hasta
	c.Dragged(e)
	c.DragEnd()
}

func cuerpoDe(t *testing.T, p *Pantalla) *cuerpoArrastrable {
	t.Helper()
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ventana, altoUtil))
	w.Canvas().Capture()
	if p.cuerpoHorario == nil || p.cuerpoHorario.arrastrable == nil {
		t.Fatal("no se montó el cuerpo horario de escritorio")
	}
	return p.cuerpoHorario.arrastrable
}

// TestArrastrarEnUnaColumnaCreaElRangoPintado: pulsar a las 10:00 del martes y soltar a
// las 11:30 crea de 10:00 a 11:30; un clic sin arrastre, una hora.
func TestArrastrarEnUnaColumnaCreaElRangoPintado(t *testing.T) {
	p := semanaDeMuestra(t)
	var ini, fin time.Time
	p.AlCrearRango = func(a, b time.Time) { ini, fin = a, b }
	c := cuerpoDe(t, p)
	_, _, ancho := c.geometria()
	x := AnchoEje + ancho*1.5 // en medio del martes
	arrastrar(c, fyne.NewPos(x, 600*PxPorMinuto), fyne.NewPos(x, 690*PxPorMinuto))
	if quiero := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC); !ini.Equal(quiero) || !fin.Equal(quiero.Add(90*time.Minute)) {
		t.Errorf("el arrastre crea de %v a %v y no de 10:00 a 11:30 del martes", ini, fin)
	}
	c.Tapped(&fyne.PointEvent{Position: fyne.NewPos(x, 604*PxPorMinuto)})
	if quiero := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC); !ini.Equal(quiero) || !fin.Equal(quiero.Add(time.Hour)) {
		t.Errorf("el clic crea de %v a %v y no una hora desde las 10:00", ini, fin)
	}
}

// TestArrastrarUnBloqueLoMueveYSeQuedaDondeSeSolto: mover «Comité» (miércoles 10:00)
// al jueves a las 12:00 guarda ese rango y deja el fantasma mientras se guarda.
func TestArrastrarUnBloqueLoMueveYSeQuedaDondeSeSolto(t *testing.T) {
	p := semanaDeMuestra(t)
	var movido Ocurrencia
	var ini, fin time.Time
	p.AlMoverEvento = func(o Ocurrencia, a, b time.Time) { movido, ini, fin = o, a, b }
	c := cuerpoDe(t, p)
	_, _, ancho := c.geometria()
	desde := fyne.NewPos(AnchoEje+ancho*2+10, 600*PxPorMinuto+5)
	hasta := fyne.NewPos(AnchoEje+ancho*3+10, 720*PxPorMinuto+5)
	arrastrar(c, desde, hasta)
	if movido.Event.ID != "c" {
		t.Fatalf("se movió %q y no el comité", movido.Event.ID)
	}
	if quiero := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC); !ini.Equal(quiero) || !fin.Equal(quiero.Add(15*time.Minute)) {
		t.Errorf("el comité queda de %v a %v y no de 12:00 a 12:15 del jueves", ini, fin)
	}
	if p.pendiente == nil || p.pendiente.Desde != 720 {
		t.Errorf("no queda el fantasma mientras se guarda: %+v", p.pendiente)
	}
}

// TestLaSemanaDejaElCanalonDeLaBarra: en escritorio la web pierde 15 px de columnas
// por la barra clásica de Blink, que cubre cabecera y cuerpo. El cuerpo mide el
// contenido menos ese canalón, y la cabecera igual, para que sus columnas casen.
func TestLaSemanaDejaElCanalonDeLaBarra(t *testing.T) {
	p := semanaDeMuestra(t)
	c := cuerpoDe(t, p)
	// El número es el de la web, NO la constante: comparar contra CanalonBarra pasaría
	// también con el canalón a cero (visto así, en rojo, antes de escribirlo así).
	if got, quiero := c.Size().Width, anchoCont-15; got < quiero-0.5 || got > quiero+0.5 {
		t.Fatalf("el cuerpo mide %v y tenía que medir %v (1020 − 15)", got, quiero)
	}
}
