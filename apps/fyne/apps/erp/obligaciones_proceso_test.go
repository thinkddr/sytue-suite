// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestProcesoObligacionExigeCadaComprobacion(t *testing.T) {
	x := NuevoProcesoObligacion(traductor(t), obligacionesArnes().Obligaciones[0])
	if x.PuedeContinuar() {
		t.Fatal("antes de leer el servidor no se avanza")
	}
	x.Cargado(api.ERPProcesoObligacion{})
	if !x.PuedeContinuar() {
		t.Fatal("sin pendientes se puede preparar")
	}
	x.Obligacion.Pendientes["facturas"] = 1
	if x.PuedeContinuar() {
		t.Fatal("con una factura pendiente no se prepara")
	}
	x.Obligacion.Pendientes["facturas"] = 0
	x.Cargado(api.ERPProcesoObligacion{Fase: 1})
	x.Revisado = true
	if x.PuedeContinuar() {
		t.Fatal("revisión calculable sin cifras no se confirma")
	}
	x.Calculo = &api.ERPCalculoObligacion{}
	if !x.PuedeContinuar() {
		t.Fatal("cifras y revisión permiten continuar")
	}
	x.Cargado(api.ERPProcesoObligacion{Fase: 2})
	if x.PuedeContinuar() {
		t.Fatal("abrir el portal no equivale a presentar")
	}
	x.Presentado = true
	if !x.PuedeContinuar() {
		t.Fatal("la confirmación permite guardar la fase del servidor")
	}
	x.Cargado(api.ERPProcesoObligacion{Fase: 3})
	x.referencia.PonerTexto("CSV-2026")
	if x.PuedeContinuar() {
		t.Fatal("sin adjunto no se archiva")
	}
	x.Adjuntos[RegistroDocumento(x.Obligacion, "justificante")] = []api.ERPAdjunto{{ID: "d1"}}
	if !x.PuedeContinuar() {
		t.Fatal("referencia y justificante permiten pedir el archivo")
	}
	x.referencia.PonerTexto("   ")
	if x.PuedeContinuar() {
		t.Fatal("una referencia en blanco no vale")
	}
	x.referencia.PonerTexto("CSV-2026")
	x.SoloLectura = true
	if x.PuedeContinuar() {
		t.Fatal("una sesión de lectura no modifica el proceso")
	}
	x.SoloLectura = false
	x.Ocupado = true
	if x.PuedeContinuar() {
		t.Fatal("una petición en curso no se duplica")
	}
	x.Ocupado = false
	x.Cargado(api.ERPProcesoObligacion{Fase: 4, Referencia: "CSV-otra-persona"})
	if x.Referencia() != "CSV-otra-persona" || x.PuedeContinuar() {
		t.Fatal("el final conserva la referencia del servidor y es terminal")
	}
}

func TestObligacionesSeparaArchivadasYReanuda(t *testing.T) {
	o := obligacionesDePrueba(t, 1280)
	o.Datos.Obligaciones[0].Proceso = &api.ERPProcesoObligacion{Fase: 4, Referencia: "CSV"}
	o.Datos.Obligaciones[3].Proceso = &api.ERPProcesoObligacion{Fase: 4, Referencia: "CSV-antiguo"}
	o.Datos.Obligaciones[1].Proceso = &api.ERPProcesoObligacion{Fase: 2}
	proximas, vencidas := o.Meses()
	if len(proximas) != 2 || len(vencidas) != 0 || len(o.Archivadas()) != 2 {
		t.Fatal("un justificante archivado no es próximo ni vencido")
	}
	var elegido string
	o.AlProceso = func(ob api.ERPObligacion) { elegido = IDObligacion(ob) }
	fila, _ := o.fila(tema.Claro, o.Datos.Obligaciones[1], 852)
	if !contiene(textosDe(fila), "Reanudar proceso") {
		t.Fatal("una presentación empezada ofrece reanudar")
	}
	recorrer(fila, func(obj fyne.CanvasObject) {
		if b, ok := obj.(*ui.Boton); ok && b.Texto == "Reanudar proceso" {
			b.Tapped(nil)
		}
	})
	if elegido != "111|2026-T3" {
		t.Fatalf("se abrió otro período: %s", elegido)
	}
	fila, _ = o.fila(tema.Claro, o.Datos.Obligaciones[0], 852)
	if !contiene(textosDe(fila), "Ver justificante") || !contiene(textosDe(fila), "Justificante archivado") {
		t.Fatal("el archivo sigue consultable")
	}
}

func TestProcesoObligacionCompruebaPendientesAntesDelPOST(t *testing.T) {
	var posts atomic.Int32
	calendario := obligacionesArnes()
	calendario.Obligaciones[0].Pendientes["facturas"] = 1
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/obligaciones/proceso":
			if r.Method == http.MethodPost {
				posts.Add(1)
			}
			_, _ = io.WriteString(w, `{"fase":0,"referencia":""}`)
		case "/api/v1/obligaciones":
			_ = json.NewEncoder(w).Encode(calendario)
		case "/api/v1/adjuntos":
			_, _ = io.WriteString(w, `{"adjuntos":[]}`)
		case "/api/v1/obligaciones/proceso/pendientes":
			_, _ = io.WriteString(w, `{"registros":[]}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	w := test.NewWindow(p.raiz)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.Atajos(w.Canvas(), nil)
	p.AbrirProcesoObligacion(obligacionesArnes().Obligaciones[0])
	x := p.oblig.Proceso
	esperar(t, func() bool { return x.Progreso != nil })
	x.AlAvanzar()
	esperar(t, func() bool { return x.Fallo != "" })
	if posts.Load() != 0 || x.Paso != 0 || !x.Pendientes() {
		t.Fatalf("avanzó sin resolver: posts=%d fase=%d", posts.Load(), x.Paso)
	}
	if !contiene(textosDe(w.Canvas().Overlays().Top()), "Resuelve los registros pendientes antes de continuar.") {
		t.Fatal("el bloqueo real debe verse en el diálogo abierto")
	}
}

func TestProcesoObligacionFalloReintentoYAvanceCompartido(t *testing.T) {
	var posts atomic.Int32
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/obligaciones/proceso":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"fase":0,"referencia":""}`)
				return
			}
			if posts.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"error":"offline"}`)
				return
			}
			var pedido api.ERPProcesoObligacion
			_ = json.NewDecoder(r.Body).Decode(&pedido)
			if pedido.Fase != 1 {
				t.Errorf("se pidió fase %d", pedido.Fase)
			}
			_, _ = io.WriteString(w, `{"fase":4,"referencia":"CSV-otra-persona"}`)
		case "/api/v1/obligaciones":
			_ = json.NewEncoder(w).Encode(obligacionesArnes())
		case "/api/v1/adjuntos":
			_, _ = io.WriteString(w, `{"adjuntos":[{"id":"d1","nombre":"justificante.pdf"}]}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	w := test.NewWindow(p.raiz)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.Atajos(w.Canvas(), nil)
	p.AbrirProcesoObligacion(obligacionesArnes().Obligaciones[0])
	x := p.oblig.Proceso
	esperar(t, func() bool { return x.Progreso != nil })
	x.AlAvanzar()
	esperar(t, func() bool { return x.Fallo != "" })
	if x.Paso != 0 || x.Ocupado || !x.PuedeContinuar() {
		t.Fatal("un fallo conserva la fase y permite reintentar")
	}
	x.AlAvanzar()
	esperar(t, func() bool { return x.Paso == 4 })
	if posts.Load() != 2 || x.Referencia() != "CSV-otra-persona" {
		t.Fatal("se debe adoptar la fase y referencia devueltas, incluso si otra persona avanzó")
	}
	if !contiene(textosDe(w.Canvas().Overlays().Top()), "El justificante está guardado. La aceptación se consulta en el portal oficial.") {
		t.Fatal("archivar no promete aceptación fiscal")
	}
}

// Oráculo independiente: ProcesoObligacion.tsx/Wizard real, Chromium y Figtree,
// raíz16 (2026-10-04), después de hacer visibles las dos etiquetas y envolver el pie.
func TestProcesoObligacionMantieneRevisionYPieDentroDelDialogo(t *testing.T) {
	for _, c := range []struct{ ventana, ancho, preparar, revisar, pie, etiqueta float32 }{
		{1280, 670, 323, 483, 53, 22}, {360, 278, 367, 545, 97, 44},
	} {
		x := NuevoProcesoObligacion(traductor(t), obligacionesArnes().Obligaciones[0])
		x.Ventana = c.ventana
		x.Cargado(api.ERPProcesoObligacion{})
		cerca(t, "preparación del proceso", x.Cuerpo(tema.Claro, c.ancho).MinSize().Height, c.preparar)
		x.Cargado(api.ERPProcesoObligacion{Fase: 1})
		x.Calculo = &api.ERPCalculoObligacion{Desde: "2026-07-01", Hasta: "2026-09-30", ResultadoCentimos: 17226, Asentado: true}
		cerca(t, "revisión del proceso", x.Cuerpo(tema.Claro, c.ancho).MinSize().Height, c.revisar)
		cerca(t, "la etiqueta de revisión", x.revisado.MinSize().Height, c.etiqueta)
		o, h := x.pie(tema.Claro, c.ancho)
		o.Resize(fyne.NewSize(c.ancho, h))
		cerca(t, "pie del proceso", h, c.pie)
		recorrer(o, func(obj fyne.CanvasObject) {
			if b, ok := obj.(*ui.Boton); ok && (b.Position().X < 0 || b.Position().X+b.Size().Width > c.ancho+.3 || b.Position().Y+b.Size().Height > h+.3) {
				t.Errorf("%q se sale del pie a %.0f: posición %v, tamaño %v", b.Texto, c.ventana, b.Position(), b.Size())
			}
		})
	}
	// A 320, los dos botones del grupo también deben envolver, como max-w-full.
	x := NuevoProcesoObligacion(traductor(t), obligacionesArnes().Obligaciones[0])
	x.Cargado(api.ERPProcesoObligacion{Fase: 1})
	o, h := x.pie(tema.Claro, 238)
	o.Resize(fyne.NewSize(238, h))
	cerca(t, "pie a 320", h, 141)
	recorrer(o, func(obj fyne.CanvasObject) {
		if b, ok := obj.(*ui.Boton); ok && b.Position().X+b.Size().Width > 238.3 {
			t.Errorf("%q rebasa el diálogo de 320", b.Texto)
		}
	})
}

func TestProcesoObligacionRespetaVerbosAunqueElNivelSeaUsuario(t *testing.T) {
	for _, permitido := range []bool{false, true} {
		t.Run(strconv.FormatBool(permitido), func(t *testing.T) {
			var posts atomic.Int32
			ob := obligacionesArnes().Obligaciones[0]
			ob.Pendientes["facturas"] = 1
			p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/obligaciones/proceso":
					_, _ = io.WriteString(w, `{"fase":0,"referencia":""}`)
				case "/api/v1/obligaciones/proceso/pendientes":
					_, _ = io.WriteString(w, `{"registros":[{"id":"f1","modelo":"factura_venta","data":{"total":12100}}]}`)
				case "/api/v1/factura_venta/f1/asentar":
					posts.Add(1)
					_, _ = io.WriteString(w, `{}`)
				case "/api/v1/obligaciones":
					_ = json.NewEncoder(w).Encode(obligacionesArnes())
				case "/api/v1/adjuntos":
					_, _ = io.WriteString(w, `{"adjuntos":[]}`)
				case "/api/v1/banco":
					_, _ = io.WriteString(w, `{"pendientes":[]}`)
				default:
					_, _ = io.WriteString(w, `{}`)
				}
			})
			p.SabePermisos = true
			p.Permisos = api.ERPPermisos{
				Administrador: true,
				Modulos:       map[string]string{"obligaciones": api.ERPUsuario, "facturacion": api.ERPUsuario, "contabilidad": api.ERPUsuario},
				Verbos:        map[string][]string{"facturacion": {"crear"}, "contabilidad": {}},
			}
			if permitido {
				p.Permisos.Verbos["facturacion"] = []string{"asentar"}
				p.Permisos.Verbos["contabilidad"] = []string{"conciliar"}
			}
			w := test.NewWindow(p.raiz)
			defer w.Close()
			w.Resize(fyne.NewSize(1280, 800))
			p.Atajos(w.Canvas(), nil)
			p.AbrirProcesoObligacion(ob)
			x := p.oblig.Proceso
			esperar(t, func() bool { return x.Progreso != nil && x.Facturas != nil })
			o, _ := x.facturas(tema.Claro, 670)
			if got := contiene(textosDe(o), x.t("action.post")); got != permitido {
				t.Fatalf("Asentar visible = %v; verbo concedido = %v", got, permitido)
			}
			x.AlBanco()
			if x.Banco.SoloLectura == permitido {
				t.Fatalf("banco SoloLectura = %v; conciliar concedido = %v", x.Banco.SoloLectura, permitido)
			}
			x.AlAsentar(x.Facturas.Registros[0])
			if permitido {
				esperar(t, func() bool { return posts.Load() == 1 && x.Ocupada == "" })
			} else if posts.Load() != 0 {
				t.Fatal("un callback directo no puede enviar el verbo denegado")
			}
		})
	}
}

func TestProcesoObligacionReleeLaFaseTrasBorrarElUltimoJustificante(t *testing.T) {
	var borrado atomic.Bool
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/obligaciones/proceso":
			fase := 4
			if borrado.Load() {
				fase = 3
			}
			_ = json.NewEncoder(w).Encode(api.ERPProcesoObligacion{Fase: fase, Referencia: "CSV"})
		case "/api/v1/adjuntos/d1":
			if r.Method != http.MethodDelete {
				t.Errorf("método = %s", r.Method)
			}
			borrado.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/adjuntos":
			if borrado.Load() {
				_, _ = io.WriteString(w, `{"adjuntos":[]}`)
			} else {
				_, _ = io.WriteString(w, `{"adjuntos":[{"id":"d1","nombre":"justificante.pdf"}]}`)
			}
		case "/api/v1/obligaciones":
			cal := obligacionesArnes()
			fase := 4
			if borrado.Load() {
				fase = 3
			}
			cal.Obligaciones[0].Proceso = &api.ERPProcesoObligacion{Fase: fase, Referencia: "CSV"}
			_ = json.NewEncoder(w).Encode(cal)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	})
	w := test.NewWindow(p.raiz)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p.Atajos(w.Canvas(), nil)
	p.AbrirProcesoObligacion(obligacionesArnes().Obligaciones[0])
	x := p.oblig.Proceso
	esperar(t, func() bool { return x.Paso == 4 && x.Recibos() })
	reg := RegistroDocumento(x.Obligacion, "justificante")
	x.AlBorrarAdjunto(reg, x.Adjuntos[reg][0])
	esperar(t, func() bool {
		return x.Paso == 3 && x.Progreso.Fase == 3 && !x.Recibos() && p.oblig.Datos != nil && p.oblig.Datos.Obligaciones[0].Proceso.Fase == 3
	})
	if contiene(textosDe(w.Canvas().Overlays().Top()), "Justificante archivado") {
		t.Fatal("el diálogo no puede seguir anunciando un archivo completado sin justificante")
	}
	if x.PuedeContinuar() {
		t.Fatal("el justificante debe volver a subirse antes de terminar")
	}
}
