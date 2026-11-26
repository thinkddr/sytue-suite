// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La cartera (`Cartera.tsx`, `vencimiento.ts` y el mayor de `Libro.tsx`)
//
// Lo que se debe y a quién, y lo que falta en el almacén, sobre las lecturas del servicio
// (`internal/http/cartera.go`), que cuentan TODO y no una página:
//
//   - las facturas abiertas y vencidas llevan «Vencida · N d» en la lista (en lugar del
//     estado, que una vencida siempre está emitida) y en la ficha, con «Enviar
//     recordatorio de pago»;
//   - la portada enseña la antigüedad de lo pendiente de cobro y los productos bajo su
//     stock mínimo;
//   - la ficha de un contacto lleva su estado de cuenta;
//   - el sumas y saldos abre el mayor de una cuenta.
//
// La web pinta la antigüedad con barras y el estado de cuenta con su tabla; aquí van en
// líneas de texto con las mismas cifras: es lo que falta medir en una ventana real.

// ahora es el reloj con el que la lista y la ficha cuentan los días vencida. Variable
// para que los guardianes de medidas, tomadas en la web con las facturas del arnés del
// 2026-09-23, no cambien de ancho el día que esas facturas vencen.
var ahora = time.Now

// estadosAbiertos son los de una factura emitida y aún por cobrar o pagar
// (`ESTADOS_ABIERTOS` de la web, `store.EstadosAbiertos` del servicio).
var estadosAbiertos = []string{"confirmado", "emitida", "pendiente"}

// diasVencida es `diasVencida` de la web: cuántos días lleva vencido un documento abierto
// (0 si no lo está, sin vencimiento, o vence hoy o después). Días de calendario en el
// huso del dispositivo: el día del vencimiento aún no es «vencido».
func diasVencida(r Registro, hoy time.Time) int {
	if !slices.Contains(estadosAbiertos, r.Estado) {
		return 0
	}
	s, _ := r.Data["vencimiento"].(string)
	v, err := time.ParseInLocation(time.DateOnly, s, hoy.Location())
	if err != nil {
		return 0
	}
	d := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	return max(int(d.Sub(v).Hours()/24+0.5), 0)
}

// --- La portada ------------------------------------------------------------------

// rotulosTramo son los nombres de cada tramo de la antigüedad, en su orden.
var rotulosTramo = map[string]string{
	"al_dia": "aging.current", "1_30": "aging.days1to30", "31_60": "aging.days31to60",
	"61_90": "aging.days61to90", "mas_90": "aging.over90",
}

// cargarCartera pide la antigüedad de los cobros y los productos bajo mínimo, cada una
// solo con sus modelos encendidos. Lo que falle no se pinta: la portada sigue sirviendo.
func (p *Pantalla) cargarCartera() {
	b := p.inicio
	if p.svc == nil || b == nil {
		return
	}
	conCobros := conFacturasVenta(p.Catalogo.Modulos, p.Catalogo.Activos)
	conStock := modeloEncendido(p.Catalogo.Modulos, p.Catalogo.Activos, "producto") &&
		modeloEncendido(p.Catalogo.Modulos, p.Catalogo.Activos, "existencia")
	hoy := b.hoy().Format(time.DateOnly)
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var a *api.ERPAntiguedad
		var bajo []api.ERPProductoBajoMinimo
		if conCobros {
			if r, err := p.svc.Antiguedad(ctx, "cobros", hoy); err == nil {
				a = &r
			}
		}
		if conStock {
			bajo, _ = p.svc.BajoMinimo(ctx)
		}
		enHilo(func() {
			b.Antiguedad, b.BajoMinimo = a, bajo
			p.Repintar()
		})
	})
}

// modeloEncendido dice si el modelo `clave` es de un módulo encendido.
func modeloEncendido(modulos []Modulo, activos []string, clave string) bool {
	for _, m := range modulos {
		if slices.Contains(activos, m.Clave) && slices.ContainsFunc(m.Modelos, func(x Modelo) bool { return x.Clave == clave }) {
			return true
		}
	}
	return false
}

// lineasAntiguedad son las líneas de la tarjeta: lo vencido y cada tramo con su importe.
func (b *Bienvenida) lineasAntiguedad() []string {
	a := b.Antiguedad
	if a == nil {
		return nil
	}
	if a.Total == 0 {
		return []string{b.t("aging.none")}
	}
	out := []string{b.t("aging.overdue", i18n.Vars{"amount": i18n.FormatCurrency(b.loc, a.Vencido)})}
	for _, tr := range a.Tramos {
		out = append(out, b.t(rotulosTramo[tr.Tramo])+" — "+i18n.FormatCurrency(b.loc, tr.Centimos))
	}
	return out
}

// lineasBajoMinimo son los seis productos más apurados con su stock y su mínimo.
func (b *Bienvenida) lineasBajoMinimo() []string {
	out := make([]string, 0, min(len(b.BajoMinimo), 6))
	for _, pr := range b.BajoMinimo[:min(len(b.BajoMinimo), 6)] {
		stock := pr.Stock
		if pr.Unidad != "" {
			stock += " " + pr.Unidad
		}
		out = append(out, pr.Nombre+" — "+b.t("stock.levels", i18n.Vars{"stock": stock, "min": pr.Minimo}))
	}
	return out
}

// tarjetaLineas es una tarjeta de la portada con su título y líneas de texto, a lo ancho
// de la rejilla: la `Card p-4` de la web.
func tarjetaLineas(v tema.Variante, titulo string, lineas []string, ancho float32) (fyne.CanvasObject, float32) {
	dentro := max(ancho-2*tema.Borde-2*TarjetaPX, 0)
	x0 := tema.Borde + TarjetaPX
	l := &lienzo{}
	tt := ui.Texto(titulo, tema.TextoXS, ui.Medio, tema.Color("text-muted", v))
	ui.Truncar(tt, titulo, tema.TextoXS, ui.Medio, dentro)
	y := x0
	l.texto(tt, x0, y)
	y += tema.TextoXS.Linea + TituloParteMB
	for _, s := range lineas {
		t := ui.Texto(s, tema.TextoSM, ui.Normal, tema.Color("text", v))
		ui.Truncar(t, s, tema.TextoSM, ui.Normal, dentro)
		l.texto(t, x0, y)
		y += tema.TextoSM.Linea + HuecoUno
	}
	alto := y + TarjetaPX + tema.Borde
	fondo, _ := fondoConCaja(v, tema.Color("surface-raised", v), tema.Color("border", v), tema.SombraSM)
	l.puestos = append([]puesto{{o: fondo, w: ancho, h: alto}}, l.puestos...)
	return l.objeto(alto)
}

// tarjetasCartera son las de la antigüedad y el bajo mínimo, si hay qué enseñar.
func (b *Bienvenida) tarjetasCartera(v tema.Variante, ancho float32) []celdaRejilla {
	var out []celdaRejilla
	if ls := b.lineasAntiguedad(); ls != nil {
		o, alto := tarjetaLineas(v, b.t("aging.title"), ls, ancho)
		out = append(out, celdaRejilla{o, alto})
	}
	if ls := b.lineasBajoMinimo(); len(ls) > 0 {
		o, alto := tarjetaLineas(v, b.t("stock.title"), ls, ancho)
		out = append(out, celdaRejilla{o, alto})
	}
	return out
}

// --- La ficha --------------------------------------------------------------------

// vencidaFicha son los días que lleva vencida la factura de la ficha (0 si no lo está).
func (f *Ficha) vencidaFicha() int {
	if f.Registro == nil || f.EnPapelera {
		return 0
	}
	hoy := f.Hoy
	if hoy.IsZero() {
		hoy = ahora()
	}
	return diasVencida(*f.Registro, hoy)
}

// botonRecordatorio es «Enviar recordatorio de pago», solo en una factura de cliente
// vencida y para quien la puede escribir; nil en cualquier otro caso.
func (f *Ficha) botonRecordatorio() *ui.Boton {
	if f.Registro == nil || f.Modelo.Clave != api.ERPFacturaVenta || f.SoloLectura || f.AlRecordar == nil || f.vencidaFicha() == 0 {
		return nil
	}
	return ui.NuevoBoton(f.t("remind.action"), f.AlRecordar).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Campana, 14)
}

// lineasEstadoCuenta son las del estado de cuenta de un contacto: cada lado con sus
// cifras y cada factura con su referencia, fecha, estado (o «Vencida · N d») e importe.
func (f *Ficha) lineasEstadoCuenta() []string {
	e := f.EstadoCuenta
	if e == nil {
		return nil
	}
	if len(e.Documentos) == 0 {
		return []string{f.t("account.empty")}
	}
	dinero := func(c int64) string { return i18n.FormatCurrency(f.loc, c) }
	lado := func(titulo string, r api.ERPResumenCuenta) string {
		return titulo + ": " + strings.Join([]string{
			f.t("account.invoiced") + " " + dinero(r.Facturado),
			f.t("account.paid") + " " + dinero(r.Pagado),
			f.t("account.pending") + " " + dinero(r.Pendiente),
			f.t("account.overdue") + " " + dinero(r.Vencido),
		}, " · ")
	}
	var out []string
	if e.Ventas.Facturado != 0 {
		out = append(out, lado(f.t("account.sales"), e.Ventas))
	}
	if e.Compras.Facturado != 0 {
		out = append(out, lado(f.t("account.purchases"), e.Compras))
	}
	for _, d := range e.Documentos {
		estado := d.Estado
		if d.DiasVencida > 0 {
			estado = f.tr.T("due.overdue", i18n.Vars{"n": d.DiasVencida})
		}
		ref := d.Ref
		if ref == "" {
			ref = SinValor
		}
		out = append(out, strings.Join([]string{ref, d.Fecha, estado, dinero(d.Total)}, " · "))
	}
	if e.Recortado {
		out = append(out, f.t("account.truncated"))
	}
	return out
}

// seccionEstadoCuenta es el estado de cuenta en la ficha de un contacto: su título y
// las líneas, partidas al ancho del formulario.
func (f *Ficha) seccionEstadoCuenta(v tema.Variante) (fyne.CanvasObject, float32) {
	ancho := f.anchoCampos()
	lineas := f.lineasEstadoCuenta()
	hijos := make([]fyne.CanvasObject, 0, 1+len(lineas))
	hijos = append(hijos, ui.NuevoRotulo(f.t("account.title"), tema.TextoSM, ui.SemiNegrita, "text"))
	for _, s := range lineas {
		hijos = append(hijos, ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text", v).A(ancho))
	}
	c := container.New(ui.Columna{Hueco: HuecoUno}, hijos...)
	return c, c.MinSize().Height
}

// cargarEstadoCuenta pide el estado de cuenta del contacto de la ficha; un fallo (sin
// permiso en facturación, servicio caído) lo deja sin pintar.
func (p *Pantalla) cargarEstadoCuenta(f *Ficha) {
	if p.svc == nil || f.Registro == nil || f.Modelo.Clave != "contacto" || f.EnPapelera {
		return
	}
	id, hoy := f.Registro.ID, time.Now().Format(time.DateOnly)
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		e, err := p.svc.EstadoDeCuenta(ctx, id, hoy)
		enHilo(func() {
			if !p.esta(f) || err != nil {
				return
			}
			f.EstadoCuenta = &e
			p.Repintar()
		})
	})
}

// --- El recordatorio de pago -------------------------------------------------------

// dialogoRecordatorio es `EnviarFactura` en modo recordatorio: la dirección de
// facturación del cliente, el asunto y el texto del recordatorio con el importe y el
// vencimiento ya escritos, y «Enviar». Sin correo saliente o sin dirección, lo dice.
func (p *Pantalla) dialogoRecordatorio(f *Ficha) {
	if p.svc == nil || f.Registro == nil || p.lienzo == nil {
		return
	}
	r := *f.Registro
	ref := r.Ref
	if ref == "" {
		ref = f.Titulo()
	}
	total, _ := numero(r.Data["total"])
	venc, _ := r.Data["vencimiento"].(string)
	if d, err := time.Parse(time.DateOnly, venc); err == nil {
		venc = i18n.FormatDayMonth(p.tr.Locale(), d) + " " + d.Format("2006")
	}
	para, asunto, mensaje := ui.NuevaEntrada(""), ui.NuevaEntrada(""), ui.NuevaAreaTexto("")
	para.Rotulo, asunto.Rotulo, mensaje.Rotulo, mensaje.Filas = p.t("send.to"), p.t("send.subject"), p.t("send.message"), 6
	asunto.PonerTexto(p.tr.T("remind.defaultSubject", i18n.Vars{"ref": ref}))
	mensaje.PonerTexto(p.tr.T("remind.defaultMessage", i18n.Vars{
		"ref": ref, "amount": i18n.FormatCurrency(p.tr.Locale(), int64(total)), "date": venc,
	}))
	fallo := ""
	var d *ui.Dialogo
	var enviar *ui.Boton
	cuerpo := func() fyne.CanvasObject {
		hijos := []fyne.CanvasObject{
			ui.GrupoCampo(p.t("send.to"), true, para, "", ""),
			ui.GrupoCampo(p.t("send.subject"), true, asunto, "", ""),
			ui.GrupoCampo(p.t("send.message"), false, mensaje, "", ""),
		}
		if fallo != "" {
			hijos = append(hijos, ui.NuevoParrafo(fallo, tema.TextoSM, ui.Normal, "danger-text", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
		}
		return container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...)
	}
	cerrar := ui.NuevoBoton(p.t("action.close"), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	enviar = ui.NuevoBoton(p.t("send.submit"), func() {
		dir, as, msj := strings.TrimSpace(para.Texto()), strings.TrimSpace(asunto.Texto()), mensaje.Texto()
		if !strings.Contains(dir, "@") || as == "" {
			fallo = p.t("send.invalidTo")
			d.CambiarCuerpo(cuerpo())
			return
		}
		enviar.Inerte = true
		enviar.Refresh()
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			err := p.svc.EnviarFactura(ctx, r.ID, dir, as, msj, true)
			enHilo(func() {
				if err != nil {
					fallo = MensajeDeError(err, p.tr)
					enviar.Inerte = false
					d.CambiarCuerpo(cuerpo())
					return
				}
				d.Cerrar()
				p.avisar(ui.AvisoExito, p.tr.T("remind.sent", i18n.Vars{"to": dir}))
				if p.esta(f) {
					p.cargarSeguimiento(f)
				}
			})
		})
	}).Con(ui.Primario, ui.TallaMD).ConIcono(iconos.Correo, 14)
	d = ui.NuevoDialogo(p.tr.T("remind.title", i18n.Vars{"ref": ref}), cuerpo()).Con(ui.DialogoMD,
		container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar, enviar))
	d.RotuloCerrar = p.t("ui.close")
	d.Foco = para
	d.Mostrar(p.lienzo)
	// La dirección de facturación del cliente, como en la web: llega por detrás.
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		datos, err := p.svc.DatosEnvio(ctx, r.ID)
		enHilo(func() {
			if err != nil {
				return
			}
			if para.Texto() == "" {
				para.PonerTexto(datos.Para)
			}
			switch {
			case !datos.Disponible:
				fallo = p.t("send.unavailable")
			case !datos.Enviable:
				fallo = p.t("send.notIssued")
			}
			enviar.Inerte = !datos.Disponible || !datos.Enviable
			d.CambiarCuerpo(cuerpo())
		})
	})
}

// --- El mayor de una cuenta --------------------------------------------------------

// lineasMayor son las del mayor: el saldo de entrada, cada apunte con el saldo que deja
// y el de salida.
func lineasMayor(tr *i18n.Translator, m api.ERPMayor) []string {
	l := tr.Locale()
	out := []string{tr.T("ledger.openingBalance", i18n.Vars{"amount": i18n.FormatCurrency(l, m.SaldoInicial)})}
	for _, a := range m.Apuntes {
		concepto := a.Concepto
		if concepto == "" {
			concepto = a.Ref
		}
		out = append(out, strings.Join([]string{
			a.Fecha, concepto, importeAsiento(a.Debe, l), importeAsiento(a.Haber, l),
			i18n.FormatCurrency(l, a.Saldo),
		}, " · "))
	}
	out = append(out, tr.T("ledger.closingBalance", i18n.Vars{"amount": i18n.FormatCurrency(l, m.SaldoFinal)}))
	if m.Recortado {
		out = append(out, tr.T("ai.truncated", nil))
	}
	return out
}

// verMayor abre el mayor de una cuenta en un diálogo, con el período del sumas y saldos.
func (p *Pantalla) verMayor(cuenta string) {
	if p.svc == nil || p.lienzo == nil || cuenta == "" {
		return
	}
	desde, hasta := RangoDePeriodo(p.libro.Periodo, time.Now())
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		m, err := p.svc.Mayor(ctx, cuenta, desde, hasta)
		enHilo(func() {
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
				return
			}
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			hijos := make([]fyne.CanvasObject, 0, len(m.Apuntes)+3)
			for _, s := range lineasMayor(p.tr, m) {
				hijos = append(hijos, ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text", v))
			}
			var d *ui.Dialogo
			cerrar := ui.NuevoBoton(p.t("action.close"), func() { d.Cerrar() })
			d = ui.NuevoDialogo(p.tr.T("ledger.accountTitle", i18n.Vars{"account": cuenta}),
				container.New(ui.Columna{Hueco: HuecoUno}, hijos...)).Con(ui.DialogoLG,
				container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar))
			d.RotuloCerrar = p.t("ui.close")
			d.Mostrar(p.lienzo)
		})
	})
}
