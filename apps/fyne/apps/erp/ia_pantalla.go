// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"os"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La costura de Sytue AI con el servicio: el panel, «Rellenar con Sytue AI» y los
// tableros guardados. Como el resto de `Pantalla`, lo que habla con la red va en
// `p.fondo` y vuelve a la interfaz por `enHilo`.

// PlazoIA es lo que se espera a una PROPUESTA: el modelo de calidad tarda del orden de
// diez segundos y el servicio le da cien (`plazoPropuesta` de server.go).
const PlazoIA = 100 * time.Second

// VistaTablero es la de un tablero guardado.
const VistaTablero = "tablero"

func (p *Pantalla) ctxIA() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), PlazoIA)
}

// anchoCuerpoDialogo es lo que le queda al cuerpo de un diálogo de esa talla en una
// ventana de ese ancho: `ui.AnchoCuerpoTalla`, que es la cuenta del propio diálogo. Era
// una copia que restaba el `p-4` también en la hoja del teléfono, que va a todo el
// ancho y sin filetes: 308 en 390, y no 342 (el cuerpo quedaba con 58 a la derecha).
func anchoCuerpoDialogo(talla ui.TallaDialogo, ventana float32) float32 {
	return ui.AnchoCuerpoTalla(talla, fyne.NewSize(ventana, 0))
}

// vivo es el cuerpo o el pie de un diálogo que se REHACE: `ui.Dialogo` monta los suyos
// UNA vez, en su renderer, así que asignarle otro `Cuerpo` después no se ve (el pie ni
// se coloca). Lo que cambia es lo de dentro de este contenedor, que es siempre el mismo.
type vivo struct{ c *fyne.Container }

func nuevoVivo(o fyne.CanvasObject) vivo { return vivo{container.NewStack(o)} }

// poner cambia lo de dentro.
func (x vivo) poner(o fyne.CanvasObject) {
	x.c.Objects = []fyne.CanvasObject{o}
	x.c.Refresh()
}

// AbrirIA abre el panel de Sytue AI sobre lo que haya abierto: la petición de un
// informe se entiende sobre la lista del modelo, si se está mirando una.
func (p *Pantalla) AbrirIA() {
	if p.lienzo == nil {
		return
	}
	var modelo *Modelo
	if p.Vista == "" && p.lista.Modelo.Clave != "" {
		m := p.lista.Modelo
		modelo = &m
	}
	x := NuevoPanelIA(p.tr, modelo, p.Modelos())
	x.Ventana = p.Ventana.Width
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := nuevoVivo(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoLG, p.Ventana.Width)))
	d := ui.NuevoDialogo("Sytue AI", cuerpo.c).Con(ui.DialogoLG, nil)
	pintar := func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		x.Ventana = p.Ventana.Width
		cuerpo.poner(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoLG, p.Ventana.Width)))
		d.Recolocar()
	}
	p.panelIA = x
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	x.AlCambio = pintar
	p.abrirDialogoIA(d, pintar)
	x.AlPreparar = func(peticion string) { p.prepararPlan(x, pintar, peticion) }
	x.AlCrear = func() { p.crearPlan(x, pintar, d) }
	x.AlProponer = func(peticion, para string) { p.proponerInforme(x, pintar, peticion, para) }
	x.AlGuardar = func() { p.guardarTablero(x, pintar, d) }
	x.AlExcel = func() { p.excelDelPanel(x, pintar) }
	d.Mostrar(p.lienzo)
}

// abrirDialogoIA apunta el diálogo de Sytue AI abierto y cómo repintarlo: si la ventana
// cambia de tamaño con él abierto, su cuerpo se vuelve a repartir al ancho nuevo, que es
// lo que hace el navegador. Al cerrarse se olvida.
func (p *Pantalla) abrirDialogoIA(d *ui.Dialogo, pintar func()) {
	p.repintarIA = pintar
	d.AlCerrar = func() {
		p.repintarIA = nil
		d.Cerrar()
	}
}

// cerrarDialogoIA lo cierra desde el código (tras crear o guardar).
func (p *Pantalla) cerrarDialogoIA(d *ui.Dialogo) {
	p.repintarIA = nil
	d.Cerrar()
}

// prepararPlan pide a Sytue AI qué registros crear. No crea nada.
func (p *Pantalla) prepararPlan(x *PanelIA, pintar func(), peticion string) {
	if p.svc == nil || x.Ocupado != "" {
		return
	}
	gen := x.Generacion
	x.Ocupado, x.Fallo, x.Plan = OcupadoProponer, "", nil
	pintar()
	p.fondo(func() {
		ctx, cancelar := p.ctxIA()
		defer cancelar()
		plan, err := p.svc.ProponerAcciones(ctx, peticion)
		enHilo(func() {
			if x.Generacion != gen {
				return
			}
			x.Ocupado = ""
			if err != nil {
				x.Fallo = MensajeIA(err, p.tr)
			} else {
				x.Plan = &plan
			}
			pintar()
		})
	})
}

// crearPlan crea los pasos del plan de una vez —o entran todos, o ninguno— y hace lo
// que la web en `onCreado`: olvida lo cacheado, cuenta las tareas y, si se creó alguna,
// cierra el panel y lleva a Tareas.
func (p *Pantalla) crearPlan(x *PanelIA, pintar func(), d *ui.Dialogo) {
	if p.svc == nil || x.Plan == nil || x.Ocupado != "" || x.Plan.Incompleto() {
		return
	}
	gen, pasos := x.Generacion, x.Plan.Pasos
	x.Ocupado, x.Fallo = OcupadoCrear, ""
	pintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		res, err := p.svc.CrearAcciones(ctx, pasos)
		enHilo(func() {
			if x.Generacion != gen {
				return
			}
			x.Ocupado = ""
			if err != nil {
				x.Fallo = MensajeDeError(err, p.tr)
				pintar()
				return
			}
			detalle := ""
			if len(res.Actividades) > 0 {
				detalle = p.t("ai.actTaskCreated", i18n.Vars{"task": res.Actividades[0].Resumen})
			}
			p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.t("ai.actCreated", i18n.Vars{"count": len(res.Creados)}), detalle))
			x.Plan = nil
			x.peticion.PonerTexto("")
			p.mu.Lock()
			p.cache = map[string]Pagina{}
			p.mu.Unlock()
			p.revision++
			p.contarTareas()
			if len(res.Actividades) > 0 {
				p.cerrarDialogoIA(d)
				p.IrA(VistaTareas)
				return
			}
			if p.lista.Modelo.Clave != "" {
				p.pedirLista()
			}
			pintar()
		})
	})
}

// proponerInforme pide a Sytue AI el informe y, en cuanto llega, lo enseña con una rueda
// en cada parte mientras se piden sus datos, como la web (`setInforme` y después
// `setDatos(await datosDe(…))`): se ve cómo avanza en vez de una sola espera. «Crear
// tablero» sigue cargando y las acciones del informe, apagadas, hasta que llegan.
func (p *Pantalla) proponerInforme(x *PanelIA, pintar func(), peticion, para string) {
	if p.svc == nil || x.Ocupado != "" {
		return
	}
	gen := x.Generacion
	x.Para, x.Ocupado, x.Fallo, x.Informe = para, OcupadoProponer, "", nil
	pintar()
	modelo := ""
	if x.Modelo != nil {
		modelo = x.Modelo.Clave
	}
	vigente := func() bool { return x.Generacion == gen }
	p.fondo(func() {
		ctx, cancelar := p.ctxIA()
		defer cancelar()
		inf, err := p.svc.ProponerInforme(ctx, peticion, para, modelo)
		if err != nil {
			enHilo(func() {
				if vigente() {
					x.Ocupado, x.Fallo = "", MensajeIA(err, p.tr)
					pintar()
				}
			})
			return
		}
		vista := &Informe{tr: p.tr, loc: p.tr.Locale(), Informe: inf}
		enHilo(func() {
			if vigente() {
				x.Informe = vista
				pintar()
			}
		})
		datos, err := p.svc.DatosInforme(ctx, inf)
		enHilo(func() {
			if !vigente() || x.Informe != vista {
				return
			}
			x.Ocupado = ""
			if err != nil {
				// Como en la web: el fallo se dice y las partes se quedan con su rueda.
				x.Fallo = MensajeIA(err, p.tr)
			} else {
				vista.Datos = datos
			}
			pintar()
		})
	})
}

// guardarTablero guarda el informe como tablero, lo pone el primero en el menú y lo
// abre; el panel se cierra.
func (p *Pantalla) guardarTablero(x *PanelIA, pintar func(), d *ui.Dialogo) {
	if p.svc == nil || x.Informe == nil || x.Ocupado != "" {
		return
	}
	gen, inf := x.Generacion, x.Informe.Informe
	x.Ocupado = OcupadoGuardar
	pintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		tb, err := p.svc.GuardarTablero(ctx, inf)
		enHilo(func() {
			if x.Generacion != gen {
				return
			}
			x.Ocupado = ""
			if err != nil {
				x.Fallo = p.t("error.generic")
				pintar()
				return
			}
			p.avisar(ui.AvisoExito, p.t("dashboard.saved"))
			p.Tableros = append([]api.ERPTablero{tb}, p.Tableros...)
			p.cerrarDialogoIA(d)
			p.AbrirTablero(tb)
		})
	})
}

// excelDelPanel baja el informe propuesto como Excel.
func (p *Pantalla) excelDelPanel(x *PanelIA, pintar func()) {
	if p.svc == nil || x.Informe == nil || x.Ocupado != "" {
		return
	}
	gen, inf := x.Generacion, x.Informe.Informe
	x.Ocupado = OcupadoExcel
	pintar()
	p.fondo(func() {
		err := p.bajarExcel(inf)
		enHilo(func() {
			if x.Generacion != gen {
				return
			}
			x.Ocupado = ""
			if err != nil {
				x.Fallo = p.t("error.generic")
			}
			pintar()
		})
	})
}

// bajarExcel guarda el Excel del informe en Descargas con su título por nombre y lo
// abre, que es lo que hace el navegador con la descarga.
func (p *Pantalla) bajarExcel(inf api.ERPInforme) error {
	ctx, cancelar := p.ctx()
	defer cancelar()
	cuerpo, err := p.svc.ExcelInforme(ctx, inf)
	if err != nil {
		return err
	}
	defer cuerpo.Close()
	ruta, err := guardarDescarga(carpetaDescargas(), NombreExcel(inf.Titulo), cuerpo)
	if err != nil {
		return err
	}
	enHilo(func() { abrirFichero(ruta) })
	return nil
}

// --- Los tableros guardados -------------------------------------------------------------

// cargarTableros los pide para el menú. Sin ellos la app sigue igual: el menú no los
// lista.
func (p *Pantalla) cargarTableros(ctx context.Context) {
	ts, err := p.svc.Tableros(ctx)
	if err != nil {
		return
	}
	//nolint:contextcheck // Repintar monta botones que se pulsan mucho después: reutilizar este ctx los daría por caducados
	enHilo(func() { p.Tableros = ts; p.Repintar() })
}

// destinosTableros son las entradas del menú, la del que se está viendo activa.
func (p *Pantalla) destinosTableros() []Destino {
	out := make([]Destino, 0, len(p.Tableros))
	for _, tb := range p.Tableros {
		t := tb
		out = append(out, Destino{
			Rotulo:   t.Titulo,
			Activa:   p.Vista == VistaTablero && p.tablero != nil && p.tablero.Tablero.ID == t.ID,
			AlPulsar: func() { p.AbrirTablero(t) },
		})
	}
	return out
}

// AbrirTablero enseña un tablero guardado con los datos de AHORA, no los de cuando se
// guardó.
func (p *Pantalla) AbrirTablero(tb api.ERPTablero) {
	p.pararEspera()
	p.Vista, p.ficha, p.cajon, p.abierta = VistaTablero, nil, false, false
	p.lista.Modelo = Modelo{}
	t := NuevoTablero(p.tr, tb)
	t.AlExcel = func() { p.excelTablero(t) }
	t.AlBorrar = func() { p.pedirBorrarTablero(t) }
	p.tablero = t
	p.Repintar()
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		datos, err := p.svc.DatosInforme(ctx, tb.Informe)
		enHilo(func() {
			if p.tablero != t {
				return
			}
			if err != nil {
				// Cada parte con su aviso, que es lo que hace la web cuando no llegan.
				datos = make([]api.ERPTablaInforme, len(tb.Informe.Partes))
				for i := range datos {
					datos[i].Error = "no_disponible"
				}
			}
			t.Datos = datos
			p.Repintar()
		})
	})
}

// excelTablero baja el tablero como Excel. El fallo va en un aviso: la pantalla del
// tablero no tiene dónde pintarlo.
func (p *Pantalla) excelTablero(t *Tablero) {
	if p.svc == nil || t.Descargando {
		return
	}
	t.Descargando = true
	p.Repintar()
	inf := t.Tablero.Informe
	p.fondo(func() {
		err := p.bajarExcel(inf)
		enHilo(func() {
			t.Descargando = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.t("error.generic"))
			}
			p.Repintar()
		})
	})
}

// pedirBorrarTablero PREGUNTA antes: los datos no se tocan, pero el tablero no vuelve.
func (p *Pantalla) pedirBorrarTablero(t *Tablero) {
	if p.lienzo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(p.t("dashboard.deleteConfirm", i18n.Vars{"nombre": t.Tablero.Titulo}),
		tema.TextoBase, ui.Normal, "text-muted", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.t("dashboard.delete"), p.tr.T("ui.cancel", nil), p.t("dashboard.delete"), true, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); p.borrarTablero(t) })
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.lienzo)
}

// borrarTablero lo borra y vuelve a la portada, como la web.
func (p *Pantalla) borrarTablero(t *Tablero) {
	if p.svc == nil {
		return
	}
	id := t.Tablero.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.BorrarTablero(ctx, id)
		enHilo(func() {
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.t("error.generic"))
				return
			}
			p.avisar(ui.AvisoExito, p.t("dashboard.deleted"))
			p.Tableros = slices.DeleteFunc(p.Tableros, func(x api.ERPTablero) bool { return x.ID == id })
			if p.tablero == t {
				p.tablero = nil
				p.Abrir(Modelo{})
				return
			}
			p.Repintar()
		})
	})
}

// --- Rellenar con Sytue AI ----------------------------------------------------------------

// rellenarIA abre el diálogo de la ficha: un texto pegado, o el PDF de la factura.
func (p *Pantalla) rellenarIA(f *Ficha) {
	if p.lienzo == nil {
		return
	}
	x := NuevoRellenarIA(p.tr, f.Modelo)
	var d *ui.Dialogo
	cerrar := func() { p.cerrarDialogoIA(d) }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := nuevoVivo(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)))
	pie := nuevoVivo(x.Pie(cerrar))
	d = ui.NuevoDialogo(p.t("ai.fill"), cuerpo.c).Con(ui.DialogoMD, pie.c)
	pintar := func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		cuerpo.poner(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)))
		pie.poner(x.Pie(cerrar))
		d.Recolocar()
	}
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	x.AlCambio = pintar
	p.abrirDialogoIA(d, pintar)
	x.AlRellenar = func(texto string) {
		p.leerConIA(f, x, d, pintar, func(ctx context.Context) (api.ERPRelleno, error) {
			return p.svc.RellenarFicha(ctx, f.Modelo.Clave, texto)
		}, false)
	}
	if p.ElegirFicheros != nil {
		x.AlElegirPDF = func() {
			p.ElegirFicheros(func(rutas []string) {
				ruta, ok := primeroCon(rutas, []string{".pdf"})
				if !ok {
					return
				}
				p.leerConIA(f, x, d, pintar, func(ctx context.Context) (api.ERPRelleno, error) {
					pdf, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
					if err != nil {
						return api.ERPRelleno{}, err
					}
					return p.svc.RellenarFichaPDF(ctx, f.Modelo.Clave, pdf)
				}, true)
			})
		}
	}
	d.Mostrar(p.lienzo)
}

// leerConIA hace la petición de rellenar y, si va bien, escribe en la ficha SOLO los
// campos que Sytue AI ha leído —lo demás se queda como estaba— y cierra el diálogo.
func (p *Pantalla) leerConIA(f *Ficha, x *RellenarIA, d *ui.Dialogo, pintar func(),
	pedir func(context.Context) (api.ERPRelleno, error), pdf bool,
) {
	if p.svc == nil || x.Ocupado {
		return
	}
	x.Ocupado, x.Fallo = true, ""
	pintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err := pedir(ctx)
		enHilo(func() {
			if err != nil {
				x.Ocupado = false
				x.Fallo = MensajeIA(err, p.tr)
				if pdf && estadoDe(err) == 422 {
					x.Fallo = p.t("ai.pdfUnreadable")
				}
				pintar()
				return
			}
			p.cerrarDialogoIA(d)
			if !p.esta(f) {
				return
			}
			f.Rellenar(r.Data, r.Etiquetas)
			p.avisar(ui.AvisoExito, p.t("ai.filled"))
			p.Repintar()
		})
	})
}
