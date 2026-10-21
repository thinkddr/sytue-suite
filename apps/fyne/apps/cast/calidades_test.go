// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// estimacion720 es lo que contesta el servicio para un minuto a 720p.
func estimacion720() api.CastEstimate {
	return api.CastEstimate{
		Options: []api.CastQualityOption{
			{Quality: 1080, Allowed: false},
			{Quality: 720, Allowed: true, Recommended: true, Bytes: 33_180_000},
			{Quality: 360, Allowed: true, Recommended: true, Bytes: 11_430_000},
		},
		Recommended: []int{720, 360}, OriginalBytes: 100_000_000, DurationMS: 60_000, Approximate: true,
	}
}

func TestCalidadesSinRed(t *testing.T) {
	if got := Alternar([]int{720}, 720); !slices.Equal(got, []int{720}) {
		t.Errorf("la última no se quita: %v", got)
	}
	if got := Alternar([]int{360}, 1080); !slices.Equal(got, []int{1080, 360}) {
		t.Errorf("añadir ordena de mayor a menor: %v", got)
	}
	e := estimacion720()
	e.Options[0].Bytes = 999 // una no permitida no suma aunque traiga bytes
	if n := BytesDe(e, []int{1080, 720}); n != 33_180_000 {
		t.Errorf("BytesDe: %d", n)
	}
	s := SumarEstimaciones([]api.CastEstimate{estimacion720(), estimacion720()})
	if s == nil || s.OriginalBytes != 200_000_000 || s.Options[1].Bytes != 66_360_000 || s.Options[0].Allowed || !slices.Equal(s.Recommended, []int{720, 360}) {
		t.Errorf("sumar: %+v", s)
	}
	if got := Quitadas([]int{1080, 720, 360}, []int{720}); !slices.Equal(got, []int{1080, 360}) {
		t.Errorf("quitadas: %v", got)
	}
	if got := Generadas(api.CastVideoQualities{HLSQualities: []int{1080, 720}, WebMQualities: []int{720, 360}}); !slices.Equal(got, []int{1080, 720, 360}) {
		t.Errorf("generadas: %v", got)
	}
}

// casillasDe son las casillas del selector, en orden.
func casillasDe(o fyne.CanvasObject) []*ui.Eleccion {
	var out []*ui.Eleccion
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Eleccion:
			out = append(out, x)
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	mirar(o)
	return out
}

// textosDe junta los párrafos y textos de un objeto.
func textosDe(o fyne.CanvasObject) string {
	var b strings.Builder
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Parrafo:
			b.WriteString(x.Texto + "\n")
		case *ui.TarjetaEleccion:
			b.WriteString(x.Titulo + "\n" + x.Descripcion + "\n" + x.Nota + "\n")
		case *ui.Eleccion:
			b.WriteString(x.Etiqueta + "\n")
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	mirar(o)
	return b.String()
}

func TestSelectorDeCalidades(t *testing.T) {
	conVariante(t, tema.Claro)
	v := vistaDePrueba(t, tema.Claro)
	e := estimacion720()
	var vistas [][]int
	libres := int64(50_000_000)
	o := SelectorCalidades(v, SelCalidades{Estimacion: &e, Elegidas: []int{720, 360}, Libres: &libres, AlCambiar: func(qs []int) { vistas = append(vistas, qs) }})
	w := test.NewWindow(o)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(480, 600))

	texto := textosDe(o)
	for _, quiero := range []string{"Recomendada", "720p · 360p", "El original no llega a esta calidad.", "Estimación aproximada", "No cabe en el espacio que queda en tu plan."} {
		if !strings.Contains(texto, quiero) {
			t.Errorf("falta «%s» en:\n%s", quiero, texto)
		}
	}
	cs := casillasDe(o)
	if len(cs) != 3 || !cs[0].Disabled() || cs[0].Marcada || !cs[1].Marcada || !cs[2].Marcada {
		t.Fatalf("casillas: %d; 1080 inerte=%v", len(cs), len(cs) > 0 && cs[0].Disabled())
	}
	test.Tap(cs[2])
	if len(vistas) != 1 || !slices.Equal(vistas[0], []int{720}) {
		t.Fatalf("desmarcar 360: %v", vistas)
	}
	// Con una sola, esa no se puede desmarcar.
	o = SelectorCalidades(v, SelCalidades{Estimacion: &e, Elegidas: []int{720}})
	if cs := casillasDe(o); !cs[1].Disabled() {
		t.Fatal("la última elegida se podía desmarcar")
	}
}

// TestSubirConCalidadesElegidas: tocar la elección antes de confirmar la manda al dar
// de alta; sin tocarla no se manda (el servicio da a cada vídeo la recomendada).
func TestSubirConCalidadesElegidas(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.Ir(RutaSubir)
	a.anadir([]string{fixtureWebM})
	sel := a.selCalidadesSubir()
	if sel.Estimacion == nil || sel.Libres == nil || *sel.Libres != 143<<30 {
		t.Fatalf("el selector no tiene la estimación ni lo que queda: %+v", sel)
	}
	sel.AlCambiar(Alternar(sel.Elegidas, 1080))
	sel = a.selCalidadesSubir()
	sel.AlCambiar(Alternar(sel.Elegidas, 360))
	a.empezarSubida()
	if !contiene(s.vistas(), `POST /api/v1/videos? {"title":"dos-segundos","visibility":"org","qualities":[720]}`) {
		t.Fatalf("el alta no lleva la elección: %q", s.vistas())
	}
	// Descartar no sube nada.
	a.anadir([]string{fixtureWebM})
	antes := len(s.vistas())
	a.descartarSubida()
	if len(a.sub.preparados) != 0 || len(s.vistas()) != antes {
		t.Fatal("descartar subió algo")
	}
}

// TestGestionarCalidades: la tarjeta carga las calidades; quitar una pregunta antes y,
// al aceptar, manda la elección; lo que queda por aplicarse se dice.
func TestGestionarCalidades(t *testing.T) {
	s := servicioCompleto()
	a, w := appDePrueba(t, s, true)
	a.AbrirGestionar("v01")
	tc := a.tarjetaCalidades()
	if tc == nil || tc.Sel.Estimacion == nil || !slices.Equal(tc.Sel.Elegidas, []int{1080, 720, 360}) || tc.Cambiada {
		t.Fatalf("la tarjeta no cargó: %+v", tc)
	}
	tc.Sel.AlCambiar([]int{720})
	if tc = a.tarjetaCalidades(); !tc.Cambiada || tc.Sel.AOcupar == nil || *tc.Sel.AOcupar != 0 {
		t.Fatalf("quitar no ocupa nada más: %+v", tc)
	}
	tc.AlGuardar()
	// Pregunta antes de borrar: aún no se ha mandado nada.
	if contiene(s.vistas(), "PUT /api/v1/videos/v01/qualities") {
		t.Fatal("quitar calidades no preguntó")
	}
	quitar := botonEnCapa(t, w, "Quitar")
	test.Tap(quitar)
	if !contiene(s.vistas(), `PUT /api/v1/videos/v01/qualities? {"qualities":[720]}`) {
		t.Fatalf("no se mandó la elección: %q", s.vistas())
	}
	if tc = a.tarjetaCalidades(); tc.Cambiada || !tc.Pendiente || !avisado(a, "Calidades guardadas. Los cambios se aplican en segundo plano.") {
		t.Fatalf("tras guardar: %+v", tc)
	}
	// Añadir no pregunta.
	a.ges.cal.elegidas = []int{1080, 720, 360}
	a.guardarCalidades()
	if !contiene(s.vistas(), `PUT /api/v1/videos/v01/qualities? {"qualities":[1080,720,360]}`) {
		t.Fatalf("añadir: %q", s.vistas())
	}
}

// TestDesdeDriveEstimaYMandaLaEleccion: Subir a Cast estima con el tamaño del fichero
// de Drive y manda la elección si se tocó.
func TestDesdeDriveEstimaYMandaLaEleccion(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.AbrirDesdeDrive("0b3e1d7a-1111-4222-8333-444455556666")
	if !contiene(s.vistas(), "GET /api/v1/videos:estimate?size_bytes=769654784") {
		t.Fatalf("no se estimó con el tamaño de Drive: %q", s.vistas())
	}
	sel := a.selCalidadesDrive()
	if sel.Estimacion == nil || !sel.Estimacion.Approximate || !slices.Equal(sel.Elegidas, []int{1080, 720, 360}) {
		t.Fatalf("selector de Drive: %+v", sel)
	}
	sel.AlCambiar([]int{720, 360})
	a.subirDesdeDrive()
	if !contiene(s.vistas(), `POST /api/v1/videos:from-drive? {"file_id":"0b3e1d7a-1111-4222-8333-444455556666","title":"Demo de producto","visibility":"org","qualities":[720,360]}`) {
		t.Fatalf("la copia no lleva la elección: %q", s.vistas())
	}
}

// TestSelectorDeCalidadesSePinta pinta «Antes de subir» y la tarjeta de Gestionar en
// claro y en oscuro sobre el fondo de su token. Con SYTUE_CAPTURA=/ruta deja las
// capturas para MIRARLAS.
func TestSelectorDeCalidadesSePinta(t *testing.T) {
	for _, variante := range []tema.Variante{tema.Claro, tema.Oscuro} {
		nombre := map[tema.Variante]string{tema.Claro: "claro", tema.Oscuro: "oscuro"}[variante]
		t.Run(nombre, func(t *testing.T) {
			conVariante(t, variante)
			v := vistaDePrueba(t, variante)
			s := servicioCompleto()
			q := api.CastQuota{LimitBytes: 200 << 30, UsedBytes: 57 << 30, AvailableBytes: 143 << 30, CastBytes: 12 << 30}
			libres := q.AvailableBytes
			e := api.CastEstimate{
				Options: []api.CastQualityOption{
					{Quality: 1080, Allowed: true, Recommended: true, Bytes: 810 << 20},
					{Quality: 720, Allowed: true, Recommended: true, Bytes: 446 << 20},
					{Quality: 360, Allowed: true, Recommended: true, Bytes: 150 << 20},
				},
				Recommended: []int{1080, 720, 360}, OriginalBytes: 812 << 20, DurationMS: 787_000, Approximate: true,
			}
			subir := PantallaSubir(v, Subir{
				Cuota: &q, Visibilidad: api.CastOrg,
				Preparados: []Preparado{{Clave: 1, Nombre: "resultados-t3.mp4", Tamano: 812 << 20, Estimacion: &e}},
				Calidades:  SelCalidades{Estimacion: &e, Elegidas: []int{1080, 720}, Libres: &libres, Avisar1080: true},
			})
			img := pintar(t, v, enMarco(v, SecSubir, subir))
			guardar(t, "calidades-subir-"+nombre, img)
			if f := colorEn(img, int(ui.AnchoMenu(false))+4, int(ventanaAlto)-4); !parecido(f, tema.Color("bg", variante), 2) {
				t.Errorf("el fondo pinta %v", f)
			}

			vid := s.mios[0]
			desc := ui.NuevaAreaTexto("")
			desc.Con(ui.CampoSM)
			titulo := ui.NuevaEntrada("").Con(ui.CampoSM)
			titulo.PonerTexto(vid.Title)
			eg := e
			eg.Approximate = false
			anade := int64(0)
			ges := PantallaGestionar(v, Gestionar{
				Video: vid, Titulo: titulo, Descripcion: desc, Persona: ui.NuevaEntrada(""),
				Calidades: &TarjetaCalidades{
					Sel:      SelCalidades{Estimacion: &eg, Elegidas: []int{720, 360}, Libres: &libres, AOcupar: &anade},
					Cambiada: true, Pendiente: true,
				},
			})
			img = pintar(t, v, enMarco(v, 255, ges))
			guardar(t, "calidades-gestionar-"+nombre, img)
		})
	}
}

// botonEnCapa encuentra un botón en la capa de encima (un diálogo).
func botonEnCapa(t *testing.T, w fyne.Window, texto string) *ui.Boton {
	t.Helper()
	var hallado *ui.Boton
	var mirar func(o fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if hallado != nil || o == nil {
			return
		}
		if b, ok := o.(*ui.Boton); ok && b.Texto == texto {
			hallado = b
			return
		}
		switch x := o.(type) {
		case *fyne.Container:
			for _, h := range x.Objects {
				mirar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				mirar(h)
			}
		}
	}
	for _, capa := range w.Canvas().Overlays().List() {
		mirar(capa)
	}
	if hallado == nil {
		t.Fatalf("no hay ningún botón «%s» en un diálogo", texto)
	}
	return hallado
}
