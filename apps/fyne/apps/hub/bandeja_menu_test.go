// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLosTiposCriticosSonLosDelHub: la lista de los que no se pueden callar es la del
// servicio, entera y sin uno de más. Si el Hub añade uno y aquí no, la bandeja ofrecería
// «Silenciar avisos como este» en un aviso que el Hub va a entregar igual.
func TestLosTiposCriticosSonLosDelHub(t *testing.T) {
	fuente := webref.Read(t, "services/hub/internal/domain/domain.go")
	inicio := strings.Index(fuente, "var tiposCriticos = []string{")
	if inicio < 0 {
		t.Fatal("services/hub ya no declara tiposCriticos")
	}
	bloque := fuente[inicio : inicio+strings.Index(fuente[inicio:], "}")]
	var delHub []string
	for _, linea := range strings.Split(bloque, "\n") {
		if l := strings.TrimSpace(linea); strings.HasPrefix(l, `"`) {
			delHub = append(delHub, strings.Trim(strings.TrimSuffix(l, ","), `"`))
		}
	}
	if !slices.Equal(delHub, TiposCriticos) {
		t.Errorf("TiposCriticos = %v; el Hub dice %v", TiposCriticos, delHub)
	}
}

// TestLaBandejaVaPorDias: «Hoy», «Ayer» y la fecha, cada uno sobre sus avisos y en el
// orden en que llegan; el filete del `divide-y` solo entre avisos del mismo día.
func TestLaBandejaVaPorDias(t *testing.T) {
	a := NuevaAvisos(traductor(t))
	a.Ahora = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	hoy := api.HubNotification{ID: "h", Source: "drive", Title: "De hoy", CreatedAt: a.Ahora.Add(-time.Hour)}
	viejo := api.HubNotification{ID: "v", Source: "mail", Title: "Antiguo", CreatedAt: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)}
	a.Estado, a.Items = Listo, append(append([]api.HubNotification{hoy}, avisosDePrueba()...), viejo)
	r := test.WidgetRenderer(a).(*avisosRenderer)

	if len(r.dias) != 3 {
		t.Fatalf("días = %d, quiero 3 (hoy, ayer y el 2 de septiembre)", len(r.dias))
	}
	if desde := []int{r.dias[0].desde, r.dias[1].desde, r.dias[2].desde}; !slices.Equal(desde, []int{0, 1, 4}) {
		t.Errorf("cada día empieza en %v, quiero [0 1 4]", desde)
	}
	if r.ids[0] != "h" || r.ids[4] != "v" {
		t.Errorf("orden de las filas = %v", r.ids)
	}
	for i, d := range r.divFilas {
		primera := slices.ContainsFunc(r.dias, func(p diaPintado) bool { return p.desde == i })
		if primera != (d == nil) {
			t.Errorf("fila %d: filete=%v, primera del día=%v", i, d != nil, primera)
		}
	}
	// La cuenta de alto y la colocación son la misma: la última fila acaba donde dice alto.
	r.Layout(fyne.NewSize(1280, r.alto(1280)))
	ultima := r.filas[len(r.filas)-1]
	_, ancho := CajaContenido(1280)
	y := MargenY + r.cabecera.alto(ancho) + MargenCabeceraActividad
	y += r.barra(0, y, ancho, false) + HuecoBandeja
	if fin := ultima.Position().Y + ultima.Size().Height; fin != y+r.altoLista(ancho) {
		t.Errorf("la lista acaba en %v y alto dice %v", fin, y+r.altoLista(ancho))
	}
}

// TestElMenuDeUnAvisoDesmarcaYSilencia: el «…» sale donde hay algo que ofrecer (leído, o
// de un tipo que se puede callar) y no en los demás; «Marcar como no leído» lo pide al
// Hub y vuelve atrás si falla; «Silenciar avisos como este» guarda origen + tipo.
func TestElMenuDeUnAvisoDesmarcaYSilencia(t *testing.T) {
	items := append(avisosDePrueba(),
		api.HubNotification{ID: "4", Source: "drive", Type: "drive.shared", Title: "Compartido", CreatedAt: time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)},
		api.HubNotification{ID: "5", Source: "pages", Type: "pages.order.oversold", Title: "Sin existencias", CreatedAt: time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)},
	)
	d := &buzonDoble{items: items, flujo: make(chan api.HubNotification)}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	v.Ir(AAvisos)
	defer v.Ir(AInicio)
	esperar(t, "la carga", lee(func(a *Avisos) bool { return a.Estado == Listo && len(a.Items) == len(items) }))

	fyne.DoAndWait(func() {
		r := test.WidgetRenderer(v.avisos).(*avisosRenderer)
		con := map[string]bool{}
		for i, id := range r.ids {
			con[id] = r.menus[i] != nil
		}
		// 2: leído. 4: se puede callar. 1 y 3: sin leer y sin tipo. 5: crítico y sin leer.
		quiero := map[string]bool{"1": false, "2": true, "3": false, "4": true, "5": false}
		for id, si := range quiero {
			if con[id] != si {
				t.Errorf("aviso %s: menú=%v, quiero %v", id, con[id], si)
			}
		}
	})

	porID := func(a *Avisos, id string) api.HubNotification {
		for _, n := range a.Items {
			if n.ID == id {
				return n
			}
		}
		return api.HubNotification{}
	}
	fyne.DoAndWait(func() { v.avisos.AlDesmarcar(porID(v.avisos, "2")) })
	esperar(t, "desmarcar", func() bool {
		return d.mira(func() bool { return slices.Equal(d.desleidos, []string{"2"}) }) &&
			lee(func(a *Avisos) bool { return !porID(a, "2").Read })()
	})

	d.mira(func() bool { d.falloDesleer = errors.New("500"); return true })
	fyne.DoAndWait(func() {
		n := porID(v.avisos, "2")
		n.Read = true
		for i := range v.avisos.Items {
			if v.avisos.Items[i].ID == "2" {
				v.avisos.Items[i].Read = true
			}
		}
		v.avisos.AlDesmarcar(n)
	})
	esperar(t, "vuelve a leído si el Hub no lo guarda", func() bool {
		return d.mira(func() bool { return len(d.desleidos) == 2 }) &&
			lee(func(a *Avisos) bool { return porID(a, "2").Read })()
	})

	fyne.DoAndWait(func() { v.avisos.AlSilenciarComoEste(porID(v.avisos, "4")) })
	esperar(t, "silenciar como este", func() bool {
		return d.mira(func() bool {
			return slices.Contains(d.puestas, api.HubNotificationPref{Source: "drive", Kind: "drive.shared", Muted: true})
		})
	})
}

// TestMarcarTodasConUnProductoElegido: con «Mail» elegido, «Marcar todas» dice de quién
// es, marca solo las de Mail y se lo pide así al Hub; el aviso de Drive sin leer sigue.
func TestMarcarTodasConUnProductoElegido(t *testing.T) {
	items := append(avisosDePrueba(),
		api.HubNotification{ID: "4", Source: "drive", Type: "drive.shared", Title: "Compartido", CreatedAt: time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)})
	d := &buzonDoble{items: items, flujo: make(chan api.HubNotification)}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).ConBuzon(d)
	lee := func(f func(a *Avisos) bool) func() bool {
		return func() bool {
			v.mu.Lock()
			defer v.mu.Unlock()
			return f(v.avisos)
		}
	}
	v.Ir(AAvisos)
	defer v.Ir(AInicio)
	esperar(t, "la carga", lee(func(a *Avisos) bool { return a.Estado == Listo && len(a.Items) == len(items) }))

	fyne.DoAndWait(func() {
		v.avisos.Producto = "mail"
		v.avisos.Refresh()
		r := test.WidgetRenderer(v.avisos).(*avisosRenderer)
		if r.marcar == nil || !strings.Contains(r.marcar.Texto, "Mail") {
			t.Fatalf("el botón no dice que es de Mail: %+v", r.marcar)
		}
		test.Tap(r.marcar)
	})
	esperar(t, "marcar las de Mail", func() bool {
		return d.mira(func() bool { return slices.Equal(d.todasDe, []string{"mail"}) && d.todas == 0 }) &&
			lee(func(a *Avisos) bool {
				for _, n := range a.Items {
					if (n.Source == "mail") != n.Read && n.ID != "2" {
						return false
					}
				}
				return true
			})()
	})
}

// TestDescargasDejaCopiarLaHuella: con suma en el catálogo, cada instalador lleva
// «Copiar la huella», con el producto en su nombre accesible, y pulsarlo la pide.
func TestDescargasDejaCopiarLaHuella(t *testing.T) {
	d := NuevasDescargas(castellano(t))
	var copiadas []string
	d.AlCopiarHuella = func(i api.Instalador) { copiadas = append(copiadas, i.SHA256) }
	d.Estado, d.Catalogo = Listo, catalogoDeEjemplo
	r := test.WidgetRenderer(d).(*descargasRenderer)
	var huellas int
	for _, c := range r.tarjetas {
		if c.huella != nil {
			huellas++
		}
	}
	for _, f := range r.filas {
		if f.huella != nil {
			huellas++
			if !strings.Contains(f.huella.Nombre, ProductoDeFichero(f.completo)) && !strings.Contains(f.huella.Nombre, f.completo) {
				t.Errorf("nombre accesible sin el producto: %q", f.huella.Nombre)
			}
		}
	}
	if huellas != len(catalogoDeEjemplo.Instaladores) {
		t.Fatalf("enlaces de huella = %d, quiero %d", huellas, len(catalogoDeEjemplo.Instaladores))
	}
	r.Layout(fyne.NewSize(1280, r.alto(1280)))
	if len(r.filas) > 0 {
		test.Tap(r.filas[0].huella)
	} else {
		test.Tap(r.tarjetas[0].huella)
	}
	if !slices.Equal(copiadas, []string{sumaDMG}) {
		t.Errorf("copiadas = %v", copiadas)
	}
	// Sin suma en el catálogo, no hay enlace.
	sinSuma := api.HubDescargas{Version: "v3.6.0", Instaladores: []api.Instalador{{Fichero: "sytue-hub-linux-amd64.tar.xz", Plataforma: "linux"}}}
	d.Catalogo = sinSuma
	d.Refresh()
	r = test.WidgetRenderer(d).(*descargasRenderer)
	for _, f := range r.filas {
		if f.huella != nil {
			t.Error("un instalador sin suma ofrece copiarla")
		}
	}
	for _, c := range r.tarjetas {
		if c.huella != nil {
			t.Error("un instalador sin suma ofrece copiarla")
		}
	}
}
