// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLeerAjustesComoLaWeb: el JSON que escribe la web se lee igual, lo roto vuelve al
// valor de siempre campo a campo y una jornada al revés se descarta entera.
func TestLeerAjustesComoLaWeb(t *testing.T) {
	a := LeerAjustes(`{"duracion":30,"recordatorio":10,"inicioLaboral":7,"finLaboral":15,"finesDeSemana":false,"numeroSemana":true}`)
	if a.Duracion != 30 || a.Recordatorio == nil || *a.Recordatorio != 10 || a.InicioLaboral != 7 ||
		a.FinLaboral != 15 || a.FinesDeSemana || !a.NumeroSemana {
		t.Fatalf("leídos mal: %+v", a)
	}
	if got := LeerAjustes(a.Texto()); got.Texto() != a.Texto() {
		t.Errorf("ida y vuelta: %s ≠ %s", got.Texto(), a.Texto())
	}
	b := LeerAjustes(`{"duracion":7,"recordatorio":3,"inicioLaboral":18,"finLaboral":9}`)
	if b.Texto() != AjustesPorDefecto().Texto() {
		t.Errorf("lo roto no vuelve a lo de siempre: %+v", b)
	}
	if LeerAjustes("{no").Texto() != AjustesPorDefecto().Texto() {
		t.Error("un JSON ilegible no da los de siempre")
	}
}

// TestPaginaDeAjustes: el menú abre la página, cada control guarda al momento (por
// persona), y volver al calendario la cierra. Lo guardado se nota en la semana (sin
// fines de semana) y en el recordatorio con el que nace un evento nuevo.
func TestPaginaDeAjustes(t *testing.T) {
	prefs := test.NewApp().Preferences()
	almacen := sesion.NuevoPorPersona(prefs, func() string { return "ana" })
	a := appDePrueba(t, &servicioFalso{})
	a.Almacen = almacen
	a.Arrancar()

	if a.P.AlAbrirAjustes == nil {
		t.Fatal("sin puerta a los ajustes en el menú")
	}
	fyne.DoAndWait(func() { a.P.AlAbrirAjustes() })
	c := a.P.controlesAjustes
	if !a.P.EnAjustes || c == nil {
		t.Fatal("la página de ajustes no se ha montado")
	}
	fila, _ := buscarObjeto(a.P.menu(), func(o fyne.CanvasObject) bool {
		f, ok := o.(*ui.FilaLateral)
		return ok && f.Activa
	}).(*ui.FilaLateral)
	if fila == nil || fila.Rotulo != "Ajustes" {
		t.Errorf("con la página abierta, la fila activa del menú es %v", fila)
	}
	fyne.DoAndWait(func() {
		c.recordatorio.AlElegir(3) // 0 es «Sin recordatorio»; el 3.º atajo, 10 min
	})
	fyne.DoAndWait(func() { a.P.controlesAjustes.finde.AlCambiar(false) })
	fyne.DoAndWait(func() { a.P.controlesAjustes.duracion.AlElegir(1) })

	guardado := LeerAjustes(almacen.Leer(ClaveAjustes))
	if guardado.Recordatorio == nil || *guardado.Recordatorio != 10 || guardado.FinesDeSemana || guardado.Duracion != 30 {
		t.Fatalf("guardado %+v", guardado)
	}

	fyne.DoAndWait(func() { a.P.controlesAjustes.volver.AlPulsar() })
	if a.P.EnAjustes {
		t.Error("«Volver al calendario» no cierra la página")
	}

	a.P.Vista = VistaSemana
	a.P.Cursor = time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	dias := a.P.diasHorarios()
	if len(dias) != 5 || dias[0].Weekday() != time.Monday || dias[4].Weekday() != time.Friday {
		t.Errorf("sin fines de semana, la semana es %v", dias)
	}

	f := a.P.nuevoFormulario(Edicion{Inicio: a.P.Cursor, Fin: a.P.Cursor.Add(time.Hour)}, nil, AccionesFormulario{})
	if r := f.e.Recordatorios; r == nil || len(*r) != 1 || (*r)[0] != 10 {
		t.Errorf("un evento nuevo nace con recordatorios %v; tocaba [10]", r)
	}

	// Otra App sobre el mismo almacén (volver a abrir) recupera lo guardado.
	b := appDePrueba(t, &servicioFalso{})
	b.Almacen = almacen
	b.Arrancar()
	if b.P.ajustes().Texto() != guardado.Texto() {
		t.Errorf("al volver, %+v", b.P.ajustes())
	}
}

// TestCapturaDeLaPaginaDeAjustes deja la página pintada con SYTUE_CAPTURAS, y comprueba
// que se monta en la ventana sin el menú de la vista encima.
func TestCapturaDeLaPaginaDeAjustes(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	fyne.DoAndWait(func() { a.P.AlAbrirAjustes() })
	if a.P.controlesAjustes == nil {
		t.Fatal("sin página de ajustes")
	}
	guardar(t, w.Canvas().Capture(), "calendario-ajustes")
}

// TestElNumeroDeSemanaEnElMes: con el ajuste, cada semana del mes lleva su número ISO
// en la esquina del lunes, con nombre accesible; sin él, ninguno (paridad con la web).
func TestElNumeroDeSemanaEnElMes(t *testing.T) {
	pastillas := func(m *Mes) []*pastillaSemana {
		w := test.NewWindow(m)
		t.Cleanup(w.Close)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(1280, 744))
		var ps []*pastillaSemana
		for _, o := range test.WidgetRenderer(m).Objects() {
			if p, ok := o.(*pastillaSemana); ok {
				ps = append(ps, p)
			}
		}
		return ps
	}
	cursor := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	sin := NuevoMes(i18n.Locale("es"), cursor)
	sin.Zona = time.UTC
	if ps := pastillas(sin); len(ps) != 0 {
		t.Errorf("sin el ajuste el mes enseña %d números de semana", len(ps))
	}
	con := NuevoMes(i18n.Locale("es"), cursor)
	con.Zona = time.UTC
	con.NumeroSemana = func(n int) string { return fmt.Sprintf("Semana %d", n) }
	ps := pastillas(con)
	if len(ps) < 5 {
		t.Fatalf("con el ajuste salen %d números de semana", len(ps))
	}
	// Noviembre de 2026 empieza en la semana 44 (lunes 26 de octubre).
	if ps[0].n != 44 || ps[0].AccessibilityLabel() != "Semana 44" {
		t.Errorf("la primera semana es %d («%s»)", ps[0].n, ps[0].AccessibilityLabel())
	}
	if x := ps[0].Position().X; x <= 0 || x+ps[0].Size().Width > Cuadricula.AnchoColumna(1280) {
		t.Errorf("el número no está en la celda del lunes: x=%v", x)
	}
}

// TestLasHorasFueraDeLaJornadaSeAtenuan: la rejilla horaria pinta dos franjas, antes
// del inicio y desde el fin de la jornada de los ajustes; con 0–24, ninguna.
func TestLasHorasFueraDeLaJornadaSeAtenuan(t *testing.T) {
	p := semanaDeMuestra(t)
	p.Ajustes = AjustesPorDefecto()
	p.Ajustes.InicioLaboral, p.Ajustes.FinLaboral = 8, 17
	c := cuerpoDe(t, p)
	r := test.WidgetRenderer(c).(*cuerpoRenderer)
	if r.antes == nil || r.despues == nil {
		t.Fatal("sin franjas fuera de la jornada")
	}
	if h := r.antes.Size().Height; h != 8*HoraPx {
		t.Errorf("antes de la jornada mide %v; tocaba %v", h, 8*HoraPx)
	}
	if y := r.despues.Position().Y; y != 17*HoraPx {
		t.Errorf("después de la jornada empieza en %v; tocaba %v", y, 17*HoraPx)
	}

	q := semanaDeMuestra(t)
	q.Ajustes = AjustesPorDefecto()
	q.Ajustes.InicioLaboral, q.Ajustes.FinLaboral = 0, 24
	rq := test.WidgetRenderer(cuerpoDe(t, q)).(*cuerpoRenderer)
	if rq.antes != nil || rq.despues != nil {
		t.Error("una jornada de 0 a 24 no atenúa nada")
	}
}
