// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Traer los datos del programa anterior (`ImportarDesde.tsx`)
//
// Holded, Odoo, Salesforce o Business Central. El servicio sabe importar de los cuatro
// (`/importar/{proveedor}`, con los parsers de `services/syerp/internal/conectores`) y
// este asistente es su pantalla, como en la web: elegir el programa, pegar el
// identificador de la conexión de la pasarela y probarla, elegir qué traer e importar.
// Solo para quien administra la organización: el servicio exige owner o admin en esas
// rutas.
//
// Las claves del tercero NUNCA pasan por aquí: viven selladas en el Integration
// Gateway, que es quien habla con Holded y compañía. El ERP solo guarda qué conexión de
// la pasarela usar; por eso el único dato que se pide es su identificador, y la prueba
// sale con el token de quien está delante.

// ObjetoImportacion es lo que se puede traer de un programa: la clave del mapeo en el
// servicio (`conectores.Mapeo.Clave()`) y su rótulo.
type ObjetoImportacion struct {
	Clave, Rotulo string
}

// ProveedorImportacion es un programa del que se importa.
type ProveedorImportacion struct {
	ID string
	// Nombre es la marca: no se traduce.
	Nombre string
	// Entra son las categorías del `DataFlow`: lo que viene hacia Sytue.
	Entra   []string
	Objetos []ObjetoImportacion
}

// ProveedoresImportacion son los de la web (`PROVEEDORES`), con las MISMAS claves que
// los mapeos del servicio: una que no existe allí hace que el servicio rechace la
// importación entera, así que el test las compara con los ficheros Go del servicio.
var ProveedoresImportacion = []ProveedorImportacion{
	{ID: "holded", Nombre: "Holded", Entra: []string{"contacts", "records", "documents"}, Objetos: []ObjetoImportacion{
		{"contacts", "migrate.obj.contacts"},
		{"products", "migrate.obj.products"},
		{"invoices", "migrate.obj.salesInvoices"},
		{"purchases", "migrate.obj.purchaseInvoices"},
	}},
	{ID: "odoo", Nombre: "Odoo", Entra: []string{"contacts", "records", "documents"}, Objetos: []ObjetoImportacion{
		{"res.partner:empresas", "migrate.obj.companies"},
		{"res.partner:personas", "migrate.obj.people"},
		{"product.product", "migrate.obj.products"},
		{"account.move:ventas", "migrate.obj.salesInvoices"},
		{"crm.lead", "migrate.obj.opportunities"},
	}},
	{ID: "salesforce", Nombre: "Salesforce", Entra: []string{"contacts", "records"}, Objetos: []ObjetoImportacion{
		{"Account", "migrate.obj.companies"},
		{"Contact", "migrate.obj.people"},
		{"Opportunity", "migrate.obj.opportunities"},
	}},
	{ID: "business-central", Nombre: "Business Central", Entra: []string{"contacts", "records", "documents"}, Objetos: []ObjetoImportacion{
		{"customers:empresas", "migrate.obj.companies"},
		{"customers:personas", "migrate.obj.people"},
		{"vendors", "migrate.obj.suppliers"},
		{"items:bienes", "migrate.obj.products"},
		{"items:servicios", "migrate.obj.services"},
		{"salesInvoices", "migrate.obj.salesInvoices"},
		{"purchaseInvoices", "migrate.obj.purchaseInvoices"},
	}},
}

// Los pasos del asistente.
const (
	PasoImportarPrograma = iota
	PasoImportarConexion
	PasoImportarQue
	PasoImportarListo
)

// reInstancia es lo mismo que valida la pasarela (`uuid.Parse`): mejor decirlo antes de
// probar.
var reInstancia = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// InstanciaValida dice si el identificador tiene la forma de uno de la pasarela.
func InstanciaValida(s string) bool { return reInstancia.MatchString(strings.TrimSpace(s)) }

// ImportarDesde es el estado del asistente. El campo del identificador dura lo que el
// asistente, para no perder el foco ni el cursor al repintar.
type ImportarDesde struct {
	tr *i18n.Translator

	Paso      int
	Proveedor *ProveedorImportacion
	// Guardadas son las instancias de las conexiones ya guardadas, por proveedor: volver a
	// importar no obliga a buscarla otra vez en la consola.
	Guardadas map[string]string
	Instancia string
	// Probada: la instancia de ahora contestó. Probando: la prueba va de camino.
	Probada, Probando bool
	ErrorConexion     string
	Elegidos          map[string]bool
	// Ocupado: se guarda la conexión o se importa. Fallo es el aviso de arriba.
	Ocupado   bool
	Fallo     string
	Resultado []api.ERPImportado
	// Ventana es el ancho de la ventana (el `sm:` de la rejilla de programas).
	Ventana float32

	AlProbar   func(proveedor, instancia string)
	AlGuardar  func(proveedor, instancia string)
	AlImportar func(proveedor string, objetos []string)
	AlCerrar   func()
	AlPolitica func()
	AlCambio   func()
	asistente  *ui.Asistente
	campo      *ui.Entrada
}

// NuevoImportarDesde crea el asistente en su primer paso.
func NuevoImportarDesde(tr *i18n.Translator) *ImportarDesde {
	x := &ImportarDesde{tr: tr, Guardadas: map[string]string{}, Elegidos: map[string]bool{}}
	x.campo = ui.NuevaEntrada("")
	x.campo.Rotulo = x.t("migrate.instanceLabel")
	x.campo.AlCambiar(func(s string) {
		if s == x.Instancia {
			return
		}
		x.Instancia, x.Probada, x.ErrorConexion = s, false, ""
		x.cambio()
	})
	return x
}

func (x *ImportarDesde) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return x.tr.T(clave, v)
}

func (x *ImportarDesde) cambio() {
	if x.AlCambio != nil {
		x.AlCambio()
	}
}

// nombre es el del programa elegido, o "".
func (x *ImportarDesde) nombre() string {
	if x.Proveedor == nil {
		return ""
	}
	return x.Proveedor.Nombre
}

// PonerGuardadas apunta las conexiones guardadas y rellena el identificador si ya se
// eligió el programa y el campo sigue vacío (la lista puede llegar después de elegir).
func (x *ImportarDesde) PonerGuardadas(cx []api.ERPConexion) {
	for _, c := range cx {
		if c.Instancia != "" {
			x.Guardadas[c.Proveedor] = c.Instancia
		}
	}
	if x.Proveedor != nil && x.Instancia == "" {
		x.ponerInstancia(x.Guardadas[x.Proveedor.ID])
	}
}

func (x *ImportarDesde) ponerInstancia(s string) {
	x.Instancia = s
	if x.campo.Texto() != s {
		x.campo.PonerTexto(s)
	}
}

// Elegir elige el programa: su instancia guardada, sin probar, y todos sus objetos.
func (x *ImportarDesde) Elegir(id string) {
	for i := range ProveedoresImportacion {
		if ProveedoresImportacion[i].ID != id {
			continue
		}
		p := &ProveedoresImportacion[i]
		x.Proveedor = p
		x.ponerInstancia(x.Guardadas[p.ID])
		x.Probada, x.ErrorConexion = false, ""
		x.Elegidos = map[string]bool{}
		for _, o := range p.Objetos {
			x.Elegidos[o.Clave] = true
		}
		x.cambio()
		return
	}
}

// Probar pide a la pasarela una fila con la instancia escrita.
func (x *ImportarDesde) Probar() {
	if x.Proveedor == nil || x.Probando || x.Ocupado {
		return
	}
	if !InstanciaValida(x.Instancia) {
		x.ErrorConexion = x.t("migrate.invalidId")
		x.cambio()
		return
	}
	x.Probando, x.ErrorConexion = true, ""
	x.cambio()
	if x.AlProbar != nil {
		x.AlProbar(x.Proveedor.ID, strings.TrimSpace(x.Instancia))
	}
}

// Probado recibe la respuesta de la prueba. Un 422 es la pasarela diciendo que no
// conecta; lo demás, el mensaje de siempre.
func (x *ImportarDesde) Probado(err error, mensaje string) {
	x.Probando = false
	x.Probada = err == nil
	if err != nil {
		x.ErrorConexion = mensaje
		if estadoDe(err) == 422 {
			x.ErrorConexion = x.t("migrate.testFailed")
		}
	}
}

// guardar guarda solo lo que ya ha conectado: nunca una instancia que no responde.
func (x *ImportarDesde) guardar() {
	if x.Proveedor == nil || !x.Probada || x.Ocupado || x.AlGuardar == nil {
		return
	}
	x.Ocupado, x.Fallo = true, ""
	x.cambio()
	x.AlGuardar(x.Proveedor.ID, strings.TrimSpace(x.Instancia))
}

// Guardado recibe la respuesta de guardar la conexión; con éxito, se pasa a elegir.
func (x *ImportarDesde) Guardado(err error, mensaje string) {
	x.Ocupado = false
	if err != nil {
		x.Fallo = mensaje
		return
	}
	x.Guardadas[x.Proveedor.ID] = strings.TrimSpace(x.Instancia)
	x.Paso = PasoImportarQue
}

// ObjetosElegidos son las claves elegidas, en el orden del proveedor.
func (x *ImportarDesde) ObjetosElegidos() []string {
	if x.Proveedor == nil {
		return nil
	}
	var out []string
	for _, o := range x.Proveedor.Objetos {
		if x.Elegidos[o.Clave] {
			out = append(out, o.Clave)
		}
	}
	return out
}

func (x *ImportarDesde) importar() {
	objetos := x.ObjetosElegidos()
	if x.Proveedor == nil || len(objetos) == 0 || x.Ocupado || x.AlImportar == nil {
		return
	}
	x.Ocupado, x.Fallo = true, ""
	x.cambio()
	x.AlImportar(x.Proveedor.ID, objetos)
}

// Importado recibe lo que trajo la importación; con éxito, el resumen.
func (x *ImportarDesde) Importado(r []api.ERPImportado, err error, mensaje string) {
	x.Ocupado = false
	if err != nil {
		x.Fallo = mensaje
		return
	}
	x.Resultado = r
	x.Paso = PasoImportarListo
}

// rotuloDe es el rótulo de un objeto del proveedor elegido; la clave si no lo conoce.
func (x *ImportarDesde) rotuloDe(clave string) string {
	if x.Proveedor != nil {
		for _, o := range x.Proveedor.Objetos {
			if o.Clave == clave {
				return x.t(o.Rotulo)
			}
		}
	}
	return clave
}

// Objeto es el asistente con el aviso de arriba (`InlineHelp tone="danger"`), al ancho
// del cuerpo del diálogo.
func (x *ImportarDesde) Objeto(v tema.Variante, ancho float32) fyne.CanvasObject {
	pasos := []ui.PasoAsistente{
		{
			Titulo: x.t("migrate.stepSource"), Encabezado: x.t("migrate.sourceHeading"),
			Cuerpo:      x.cuerpoPrograma(ancho),
			PuedeSeguir: func() bool { return x.Proveedor != nil },
		},
		{
			Titulo: x.t("migrate.stepConnect"), Encabezado: x.t("migrate.connectHeading", i18n.Vars{"name": x.nombre()}),
			Descripcion: x.t("migrate.connectHint", i18n.Vars{"name": x.nombre()}),
			Cuerpo:      x.cuerpoConexion(v, ancho),
			PuedeSeguir: func() bool { return x.Probada },
			AlSiguiente: x.guardar,
		},
		{
			Titulo: x.t("migrate.stepWhat"), Encabezado: x.t("migrate.whatHeading"),
			Descripcion: x.t("migrate.whatHint"), RotuloSiguiente: x.t("migrate.run"),
			Cuerpo:      x.cuerpoQue(),
			PuedeSeguir: func() bool { return len(x.ObjetosElegidos()) > 0 },
			AlSiguiente: x.importar,
		},
		{
			Titulo: x.t("migrate.stepDone"), Encabezado: x.t("migrate.doneHeading"), Terminal: true,
			Cuerpo: x.cuerpoListo(v, ancho),
		},
	}
	if x.asistente == nil {
		x.asistente = ui.NuevoAsistente(x.tr, pasos...)
		x.asistente.AlCambiar = func(n int) {
			// Con el resumen a la vista ya no se vuelve atrás.
			if x.Resultado == nil {
				x.Paso, x.Fallo = n, ""
				x.cambio()
			}
		}
	}
	a := x.asistente
	a.Pasos, a.Ventana, a.Ocupado = pasos, x.Ventana, x.Ocupado
	a.AlCancelar = nil
	if x.Resultado == nil {
		a.AlCancelar = x.AlCerrar
	}
	if a.Actual != x.Paso {
		a.IrA(x.Paso)
	}
	a.Refresh()
	if x.Fallo == "" {
		return a
	}
	aviso := ui.NuevoParrafo(x.Fallo, tema.TextoSM, ui.Normal, "danger-text", v).A(ancho)
	return container.New(ui.Columna{Hueco: 1 * tema.Rem}, aviso, a)
}

// cuerpoPrograma son los cuatro programas, en dos columnas desde `sm:`, y el `DataFlow`
// del elegido.
func (x *ImportarDesde) cuerpoPrograma(ancho float32) fyne.CanvasObject {
	columnas := 1
	if x.Ventana == 0 || x.Ventana >= 640 {
		columnas = 2
	}
	var hueco float32 = 2 * 0.25 * tema.Rem
	w := (ancho - hueco*float32(columnas-1)) / float32(columnas)
	tarjetas := make([]fyne.CanvasObject, 0, len(ProveedoresImportacion))
	for _, p := range ProveedoresImportacion {
		id := p.ID
		c := ui.NuevaTarjetaEleccion(p.Nombre, "", func() { x.Elegir(id) })
		c.Densa, c.Ancho = true, w
		c.Elegida = x.Proveedor != nil && x.Proveedor.ID == id
		tarjetas = append(tarjetas, c)
	}
	filas := []fyne.CanvasObject{container.New(ui.FilaAuto{Hueco: hueco, SaltoFila: hueco, Partir: true, Derecha: len(tarjetas)}, tarjetas...)}
	if x.Proveedor != nil {
		filas = append(filas, ui.FlujoDatos(x.tr, x.Proveedor.Nombre, x.Proveedor.Entra, nil, false, x.AlPolitica, ancho))
	}
	return container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem}, filas...)
}

// cuerpoConexion es el campo del identificador, «Probar conexión» y lo que contestó.
func (x *ImportarDesde) cuerpoConexion(v tema.Variante, ancho float32) fyne.CanvasObject {
	x.campo.Ancho = ancho
	x.campo.Inerte(x.Probando || x.Ocupado)
	campo := ui.GrupoCampo(x.t("migrate.instanceLabel"), false, x.campo, "", x.ErrorConexion)
	probar := ui.NuevoBoton(x.t("migrate.test"), x.Probar).Con(ui.Secundario, ui.TallaMD)
	probar.Inerte = strings.TrimSpace(x.Instancia) == "" || x.Probando || x.Ocupado
	probar.Cargando = x.Probando
	fila := []fyne.CanvasObject{probar}
	if x.Probada {
		fila = append(fila, ui.NuevoParrafo(x.t("migrate.testOk", i18n.Vars{"name": x.nombre()}), tema.TextoSM, ui.Normal, "success-text", v).
			A(max(ancho-probar.MinSize().Width-3*0.25*tema.Rem, 0)))
	}
	return container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem}, campo,
		container.New(ui.FilaAuto{Hueco: 3 * 0.25 * tema.Rem, SaltoFila: 2 * 0.25 * tema.Rem, Partir: true, Derecha: len(fila)}, fila...))
}

// cuerpoQue es una casilla por objeto del programa, todas marcadas al elegirlo.
func (x *ImportarDesde) cuerpoQue() fyne.CanvasObject {
	var casillas []fyne.CanvasObject
	if x.Proveedor != nil {
		for _, o := range x.Proveedor.Objetos {
			clave := o.Clave
			c := ui.NuevaCasilla(x.t(o.Rotulo), func(marcada bool) {
				x.Elegidos[clave] = marcada
				x.cambio()
			})
			c.Marcada = x.Elegidos[clave]
			if x.Ocupado {
				c.Disable()
			}
			casillas = append(casillas, c)
		}
	}
	return container.New(ui.Columna{Hueco: 2 * 0.25 * tema.Rem}, casillas...)
}

// cuerpoListo es la tabla de lo que trajo cada objeto —nuevos, actualizados y fallidos,
// en rojo si hubo alguno— y «Cerrar».
func (x *ImportarDesde) cuerpoListo(v tema.Variante, ancho float32) fyne.CanvasObject {
	celda := func(s, token string, peso ui.Peso, e tema.Escalon) fyne.CanvasObject {
		return ui.NuevoParrafo(s, e, peso, token, v).A(ancho / 4)
	}
	celdas := make([]fyne.CanvasObject, 0, 4*(1+len(x.Resultado)))
	celdas = append(celdas,
		celda(x.t("migrate.stepWhat"), "text-muted", ui.Medio, tema.TextoXS),
		celda(x.t("migrate.created"), "text-muted", ui.Medio, tema.TextoXS),
		celda(x.t("migrate.updated"), "text-muted", ui.Medio, tema.TextoXS),
		celda(x.t("migrate.failed"), "text-muted", ui.Medio, tema.TextoXS))
	for _, r := range x.Resultado {
		tinta := "text"
		if len(r.Fallos) > 0 {
			tinta = "danger-text"
		}
		celdas = append(celdas,
			celda(x.rotuloDe(r.Objeto), "text", ui.Normal, tema.TextoSM),
			celda(strconv.Itoa(r.Creados), "text", ui.Normal, tema.TextoSM),
			celda(strconv.Itoa(r.Actualizados), "text", ui.Normal, tema.TextoSM),
			celda(strconv.Itoa(len(r.Fallos)), tinta, ui.Normal, tema.TextoSM))
	}
	cerrar := ui.NuevoBoton(x.t("action.close"), func() {
		if x.AlCerrar != nil {
			x.AlCerrar()
		}
	}).Con(ui.Primario, ui.TallaMD)
	return container.New(ui.Columna{Hueco: 1 * tema.Rem}, container.NewGridWithColumns(4, celdas...),
		container.New(ui.FilaAuto{Derecha: 1}, cerrar))
}
