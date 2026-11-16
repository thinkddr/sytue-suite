// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Los guardianes de la configuración guiada (§2.8): MarcoAjustes, SeccionAjustes,
// Resumen y Asistente. Comparan con las medidas de la web en «Tus datos» y «Conecta tus
// dispositivos» del portal, las que vigilaba el Hub cuando estas piezas vivían en
// `apps/hub/ajustes.go`, con la raíz de 16 px que fija hoy el tema compartido
// (`TestLaRaizEsLaDeLaWeb`): `max-w-5xl` son 1024, `px-6` 24, `px-4` 16 y `gap-6` 24.
// Las medidas de 2026-09 se tomaron cuando 1 rem eran 14 px (213 y 854 a 1280); escritas aquí
// con números fijos, un cambio de raíz ya no pasaría desapercibido.

func igual(a, b float32) bool { return a-b < 0.01 && b-a < 0.01 }

// TestLaCajaDeAjustesEsLaDeProduccion: a 1280 la caja empieza en 152 y mide 976
// (`max-w-5xl` de 1024 menos el `sm:px-6`); a 390, en 16 y mide 358 (`px-4`).
func TestLaCajaDeAjustesEsLaDeProduccion(t *testing.T) {
	for _, c := range []struct{ ventana, x, ancho float32 }{
		{1280, 152, 976},
		{390, 16, 358},
		// En el corte mismo ya manda el `sm:`.
		{640, 24, 592},
	} {
		x, ancho := CajaAjustes(c.ventana)
		if !igual(x, c.x) || !igual(ancho, c.ancho) {
			t.Errorf("a %v la caja empieza en %v y mide %v; producción: %v y %v", c.ventana, x, ancho, c.x, c.ancho)
		}
	}
}

// TestElMarcoEnLaHojaMideElCorteEnLaVentana: en Drive o en Mail el marco vive en la
// hoja, junto al menú, y el `sm:px-6` de la web es de la VENTANA: con una hoja de 600 en
// una ventana de 1280 el margen es 24, no el 16 que tocaría si la ventana midiera 600.
//
// Visto en ROJO a mano: sin `Ventana`, el marco miraba su propio ancho y caía en 16.
func TestElMarcoEnLaHojaMideElCorteEnLaVentana(t *testing.T) {
	for _, c := range []struct{ ventana, x float32 }{{1280, 24}, {0, 16}} {
		sec := NuevaSeccionAjustes("Unidades", "", nil)
		m := NuevoMarcoAjustes("Unidades de equipo", "", sec)
		m.Ventana = c.ventana
		test.WidgetRenderer(m)
		m.Resize(MedidaMarcoAjustes(m, 600))
		if got, ancho := sec.Position().X, sec.Size().Width; !igual(got, c.x) || !igual(ancho, 600-2*c.x) {
			t.Errorf("con la ventana a %v la sección empieza en %v y mide %v; quiero %v y %v", c.ventana, got, ancho, c.x, 600-2*c.x)
		}
	}
}

// TestElMarcoDeAjustesColocaComoProduccion: el título (`sytue-h2`) cae 24 por debajo
// del borde (`py-6`), la entrada 6 por debajo del título (`mt-1.5`) y la primera sección
// 24 más abajo (`mt-6`). Dentro de la sección, el `h2` (`sytue-h4`) va a 17 del borde
// de la tarjeta (1 de borde y el `p-4`) y el cuerpo, 16 por debajo del título.
func TestElMarcoDeAjustesColocaComoProduccion(t *testing.T) {
	cuerpo := NuevoBoton("Descargar copia", nil)
	sec := NuevaSeccionAjustes("Exportar tus datos", "", cuerpo)
	m := NuevoMarcoAjustes("Tus datos", "Descarga o borra lo que guardamos de ti.", sec)
	medida := MedidaMarcoAjustes(m, 1280)
	test.WidgetRenderer(m) // monta el renderer
	m.Resize(medida)
	if got := m.MinSize().Height; !igual(got, medida.Height) {
		t.Errorf("MinSize dice %v de alto y MedidaMarcoAjustes %v: la medida se calcula en dos sitios", got, medida.Height)
	}

	cab := m.cabecera()
	h2, h4 := EscalonTitulo(2).Linea, EscalonTitulo(4).Linea
	if got := cab.titulo.Position(); !igual(got.X, 152) || !igual(got.Y, 24) {
		t.Errorf("el título está en %v y la web lo pone en (152, 24)", got)
	}
	if got := cab.titulo.Size().Height; !igual(got, h2) {
		t.Errorf("el título mide %v de alto y el `sytue-h2` mide %v", got, h2)
	}
	yEntrada := 24 + h2 + 6
	if got := cab.desc.Position(); !igual(got.Y, yEntrada) {
		t.Errorf("la entrada empieza en %v y debería en %v (6 bajo el título)", got.Y, yEntrada)
	}
	ySeccion := yEntrada + cab.desc.Size().Height + 24
	if got := sec.Position(); !igual(got.X, 152) || !igual(got.Y, ySeccion) {
		t.Errorf("la sección está en %v y debería en (152, %v)", got, ySeccion)
	}
	if got := sec.Size().Width; !igual(got, 976) {
		t.Errorf("la sección mide %v de ancho y la caja 976", got)
	}

	scab := sec.cabecera()
	if got := scab.titulo.Position(); !igual(got.X, 17) || !igual(got.Y, 17) {
		t.Errorf("el título de la sección está en %v y va a (17, 17) del borde", got)
	}
	if got := scab.titulo.Size().Height; !igual(got, h4) {
		t.Errorf("el título de la sección mide %v de alto y el `sytue-h4` mide %v", got, h4)
	}
	if got := cuerpo.Position(); !igual(got.Y, 17+h4+16) || !igual(got.X, 17) {
		t.Errorf("el cuerpo de la sección está en %v y va a (17, %v)", got, 17+h4+16)
	}
	alto := 17 + h4 + 16 + cuerpo.Size().Height + 17
	if got := sec.Size().Height; !igual(got, alto) {
		t.Errorf("la sección mide %v de alto y debería %v", got, alto)
	}
	if got := medida.Height; !igual(got, ySeccion+alto+24) {
		t.Errorf("el marco mide %v de alto y debería %v (con el `py-6` de abajo)", got, ySeccion+alto+24)
	}

	// Y a 390 se cierra al `px-4`.
	m.Resize(MedidaMarcoAjustes(m, 390))
	if got := sec.Position().X; !igual(got, 16) {
		t.Errorf("a 390 la sección empieza en %v y la web en 16", got)
	}
}

// TestLaSeccionPintaSuTarjetaYSusAcciones: con acciones, van a la derecha y el título se
// queda con el resto; plana, sin tarjeta y con `gap-3`.
func TestLaSeccionPintaSuTarjetaYSusAcciones(t *testing.T) {
	guardar := NuevoBoton("Guardar", nil)
	cuerpo := NuevoBoton("Algo", nil)
	s := NuevaSeccionAjustes("Notificaciones", "Qué te avisamos y cuándo.", cuerpo)
	s.Acciones = []fyne.CanvasObject{guardar}
	test.WidgetRenderer(s)
	s.Resize(MedidaSeccionAjustes(s, 600))
	mg := guardar.MinSize()
	if got := guardar.Position().X + mg.Width; !igual(got, 600-17) {
		t.Errorf("la acción acaba en %v y va pegada al relleno de la derecha (583)", got)
	}
	if got := s.cabecera().titulo.Size().Width; !igual(got, 600-34-mg.Width-12) {
		t.Errorf("el título tiene %v de ancho: no le deja sitio a la acción", got)
	}
	img := pinta(t, 620, s.Size().Height+20, s)
	// El borde de la tarjeta a media altura del lado izquierdo es del color del borde.
	if got, quiero := px(img, 0, int(s.Size().Height/2*escala)), tema.Color("border", tema.Claro); !cercaDe(got, quiero, 2) {
		t.Errorf("el borde de la tarjeta sale %v y el token `border` es %v", got, quiero)
	}

	s.Plana = true
	s.Refresh()
	s.Resize(MedidaSeccionAjustes(s, 600))
	if got := s.cabecera().titulo.Position(); !igual(got.X, 0) || !igual(got.Y, 0) {
		t.Errorf("plana, el título está en %v y va en la esquina: sin tarjeta no hay relleno", got)
	}
	desc := s.cabecera().desc
	if got, quiero := cuerpo.Position().Y, desc.Position().Y+desc.Size().Height+12; !igual(got, quiero) {
		t.Errorf("plana, el cuerpo empieza en %v y va con `gap-3` bajo la cabecera (%v)", got, quiero)
	}
}

// TestElResumenEsElDeReviewSummary: filas `px-3 py-2.5` (12 y 10), la clave en una
// columna de 11rem (176) desde `sm:`, un filete de 1 entre filas y el «Editar» del catálogo a la derecha,
// con el nombre de su fila para el lector; la fila que lo lleva, 4 más alta.
//
// Visto en ROJO a mano: con el alto de antes, sin el puntal, la fila con «Editar» mide lo mismo que
// la otra y la clave no baja.
func TestElResumenEsElDeReviewSummary(t *testing.T) {
	editado := ""
	r := NuevoResumen(traductorPrueba(t),
		FilaResumen{Clave: "Organización", Valor: "Acme"},
		FilaResumen{Clave: "Dominio", Valor: "acme.example", Mono: true, AlEditar: func() { editado = "Dominio" }},
	)
	test.WidgetRenderer(r)
	r.Resize(MedidaResumen(r, 600))

	// La fila con «Editar» crece con el puntal de `text-base` de su `<dd>` y baja su
	// texto 3 (medido en la web en las reglas de Mail: 33,25 → 36,75 cuando 1 rem eran 14 px).
	fila, conEditar := 20+tema.TextoSM.Linea, 20+tema.TextoBase.Linea
	if got := r.Size().Height; !igual(got, 2+fila+conEditar+1) {
		t.Errorf("el resumen mide %v de alto y sus dos filas (%v y %v) con sus bordes son %v", got, fila, conEditar, 2+fila+conEditar+1)
	}
	if got, quiero := r.editar[1].Position().Y, 1+fila+1+10+BajoEditarResumen; !igual(got, quiero) {
		t.Errorf("el «Editar» está en y = %v y va 3 por debajo del `py-2.5` de su fila (%v)", got, quiero)
	}
	if got, quiero := r.claves[1].Position().Y, r.editar[1].Position().Y; !igual(got, quiero) {
		t.Errorf("la clave de la fila con «Editar» está en y = %v y baja con él (%v)", got, quiero)
	}
	if got := r.claves[0].Position(); !igual(got.X, 13) || !igual(got.Y, 11) {
		t.Errorf("la primera clave está en %v y va a (13, 11): borde y `px-3 py-2.5`", got)
	}
	if got := r.valores[0].Position().X; !igual(got, 13+176+12) {
		t.Errorf("el valor empieza en %v y va tras la columna de 11rem (201)", got)
	}
	// Cada fila es su propia rejilla: la que no lleva «Editar» no reserva su ancho, solo
	// el `gap-3` hasta la tercera pista vacía (574 de interior − 176 − 12 − 12).
	if got := r.valores[0].Size().Width; !igual(got, 374) {
		t.Errorf("el valor de la fila sin «Editar» mide %v y en la web 374", got)
	}
	if got := r.filetes[0].Position().Y; !igual(got, 1+fila) {
		t.Errorf("el filete está en %v y va bajo la primera fila (%v)", got, 1+fila)
	}
	if r.valores[1].Peso != Mono || r.valores[1].Escalon != tema.TextoXS {
		t.Error("el valor técnico no sale en `font-mono text-xs`")
	}
	e := r.editar[1]
	if e.Texto != "Edit" {
		t.Errorf("el «Editar» dice %q y el catálogo común (en) dice «Edit»", e.Texto)
	}
	if got := e.AccessibilityLabel(); got != "Edit Dominio" {
		t.Errorf("el lector oye %q y debería oír «Edit Dominio»", got)
	}
	if got := e.Position().X + e.Size().Width; !igual(got, 600-13) {
		t.Errorf("el «Editar» acaba en %v y va pegado al relleno de la derecha (587)", got)
	}
	for _, o := range test.WidgetRenderer(r).Objects() {
		if o == r.editar[0] {
			t.Error("la fila sin AlEditar pinta un «Editar»")
		}
	}
	test.Tap(e)
	if editado != "Dominio" {
		t.Error("pulsar «Editar» no ha devuelto a su paso")
	}

	// Por debajo de `sm:` la clave mide su texto.
	r.Ventana = 390
	r.Refresh()
	r.Resize(MedidaResumen(r, 362))
	w := Medir("Organización", tema.TextoSM, Normal).Width
	if got := r.valores[0].Position().X; !igual(got, 13+w+12) {
		t.Errorf("a 390 el valor empieza en %v y la clave mide su texto (%v)", got, 13+w+12)
	}
	// Y en el `flex` la fila sin «Editar» no tiene tercer hijo ni hueco.
	if got, quiero := r.valores[0].Size().Width, 362-26-w-12; !igual(got, quiero) {
		t.Errorf("a 390 el valor de la fila sin «Editar» mide %v y en la web %v", got, quiero)
	}
}

func pasosDePrueba(puede *bool, accion func()) []PasoAsistente {
	return []PasoAsistente{
		{
			Titulo: "Elegir", Encabezado: "¿Qué quieres conectar?", Descripcion: "Una aplicación.",
			Cuerpo: NuevoBoton("Slack", nil), PuedeSeguir: func() bool { return *puede },
		},
		{Titulo: "Configurar", Cuerpo: NuevoBoton("Token", nil), RotuloSiguiente: "Activar", AlSiguiente: accion},
		{Titulo: "Listo", Cuerpo: NuevoBoton("Hecho", nil), Terminal: true},
	}
}

// TestElAsistenteColocaComoWizard: el progreso de 28 (`h-6` y `py-0.5`), el `gap-6`
// (24), el `sytue-h3` con su frase a 6 (`mt-1.5`) y el pie con su filete y `pt-4` (16),
// con «Continuar» a la derecha y «Cancelar» a la izquierda.
func TestElAsistenteColocaComoWizard(t *testing.T) {
	puede := true
	a := NuevoAsistente(traductorPrueba(t), pasosDePrueba(&puede, nil)...)
	a.AlCancelar = func() {}
	test.WidgetRenderer(a)
	a.Resize(MedidaAsistente(a, 854))
	if got := a.progreso.Size(); !igual(got.Height, 28) || !igual(got.Width, 854) {
		t.Errorf("el progreso mide %v y la web lo pinta de 854 × 28", got)
	}
	yEnc := float32(28 + 24)
	if got := a.encabezado.Position().Y; !igual(got, yEnc) {
		t.Errorf("el encabezado empieza en %v y va a %v (`gap-6`)", got, yEnc)
	}
	if got := a.encabezado.Size().Height; !igual(got, EscalonTitulo(3).Linea) {
		t.Errorf("el encabezado mide %v y el `sytue-h3` %v", got, EscalonTitulo(3).Linea)
	}
	if got := a.descripcion.Position().Y; !igual(got, yEnc+EscalonTitulo(3).Linea+6) {
		t.Errorf("la frase empieza en %v y va 6 bajo el encabezado", got)
	}
	cuerpo := a.Pasos[0].Cuerpo
	yCuerpo := a.descripcion.Position().Y + a.descripcion.Size().Height + 24
	if got := cuerpo.Position().Y; !igual(got, yCuerpo) {
		t.Errorf("el cuerpo empieza en %v y va a %v", got, yCuerpo)
	}
	yFilete := yCuerpo + cuerpo.Size().Height + 24
	if got := a.filete.Position().Y; !igual(got, yFilete) {
		t.Errorf("el filete del pie está en %v y va a %v", got, yFilete)
	}
	if got := a.siguiente.Position(); !igual(got.X+a.siguiente.Size().Width, 854) || !igual(got.Y, yFilete+1+16) {
		t.Errorf("«Continuar» está en %v: va a la derecha, 17 bajo el filete", got)
	}
	if got := a.cancelar.Position().X; !igual(got, 0) {
		t.Errorf("«Cancelar» está en %v y va a la izquierda", got)
	}
	if a.siguiente.Texto != "Continue" || a.cancelar.Texto != "Cancel" {
		t.Errorf("los textos del armazón son %q y %q: no salen del catálogo común", a.siguiente.Texto, a.cancelar.Texto)
	}
	if got := a.Size().Height; !igual(got, yFilete+1+16+a.siguiente.MinSize().Height) {
		t.Errorf("el asistente mide %v de alto y acaba con la fila de botones", got)
	}
}

// TestLaVerjaDelAsistente: «Continuar» apagado mientras el paso no puede seguir; al
// avanzar, aviso, foco al encabezado y «Atrás»; un paso con acción la ejecuta en vez de
// avanzar; y el terminal no lleva pie.
func TestLaVerjaDelAsistente(t *testing.T) {
	puede, activado := false, false
	a := NuevoAsistente(traductorPrueba(t), pasosDePrueba(&puede, func() { activado = true })...)
	var avisado []int
	a.AlCambiar = func(i int) { avisado = append(avisado, i) }
	w := test.NewTempWindow(t, a)
	w.Resize(fyne.NewSize(854, 600))

	if c := w.Canvas().Focused(); c != nil {
		t.Errorf("al montar, el foco es de %T: el asistente no debe robarlo", c)
	}
	if !a.siguiente.Inerte {
		t.Error("con la verja cerrada «Continuar» está encendido")
	}
	test.Tap(a.siguiente)
	if a.Actual != 0 {
		t.Fatal("con la verja cerrada el asistente ha avanzado")
	}
	for _, o := range test.WidgetRenderer(a).Objects() {
		if o == a.atras {
			t.Error("en el primer paso hay «Atrás»")
		}
	}

	puede = true
	a.Refresh()
	if a.siguiente.Inerte {
		t.Fatal("con la verja abierta «Continuar» sigue apagado")
	}
	test.Tap(a.siguiente)
	if a.Actual != 1 || len(avisado) != 1 || avisado[0] != 1 {
		t.Fatalf("tras «Continuar» el paso es %d y se avisó %v", a.Actual, avisado)
	}
	if got := w.Canvas().Focused(); got != fyne.Focusable(a.encabezado) {
		t.Errorf("tras cambiar de paso el foco es de %T y va al encabezado", got)
	}
	if a.encabezado.AccessibilityLabel() != "Configurar" {
		t.Errorf("el encabezado dice %q y sin Encabezado es el título", a.encabezado.AccessibilityLabel())
	}
	if !a.encabezado.Disabled() {
		t.Error("el encabezado es una parada del tabulador y en la web es `tabIndex={-1}`")
	}
	if a.siguiente.Texto != "Activar" {
		t.Errorf("«Continuar» dice %q y el paso pide «Activar»", a.siguiente.Texto)
	}
	if a.progreso.AlElegir == nil {
		t.Error("el progreso no deja volver a un paso hecho")
	}

	test.Tap(a.siguiente)
	if !activado || a.Actual != 1 {
		t.Errorf("el paso con acción: activado=%v, paso=%d; tenía que ejecutarla y no avanzar", activado, a.Actual)
	}

	a.Ocupado = true
	a.Refresh()
	if !a.siguiente.Cargando || !a.atras.Inerte || a.progreso.AlElegir != nil {
		t.Error("ocupado, las acciones y el progreso siguen vivos")
	}
	a.Ocupado = false
	a.Refresh()

	test.Tap(a.atras)
	if a.Actual != 0 {
		t.Fatalf("«Atrás» deja el paso en %d", a.Actual)
	}

	a.IrA(2)
	for _, o := range test.WidgetRenderer(a).Objects() {
		if o == a.filete || o == a.siguiente {
			t.Error("el paso terminal pinta la barra de acciones")
		}
	}
	if a.progreso.AlElegir != nil {
		t.Error("el paso terminal deja volver atrás desde el progreso")
	}
}
