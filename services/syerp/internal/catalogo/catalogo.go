// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package catalogo carga el catálogo de módulos de SyERP: qué modelos existen,
// qué campos tiene cada uno y cómo se llaman en los cinco idiomas.
//
// EL CATÁLOGO ES DATO. Cada `modulos/*.json` declara un modelo; el núcleo no
// tiene una sola línea de código específica de Ventas, de Inventario ni de
// Flota. Eso es lo que permite tener «todos los módulos» sin tener doscientos
// CRUD escritos a mano que solo se diferencian en los nombres de las columnas,
// y lo que hace que añadir un módulo sea un fichero de treinta líneas y no una
// migración, un despliegue y una ventana de mantenimiento.
//
// Los rótulos NO se repiten en cada modelo: `terminos.json` es el diccionario
// común (fecha, importe, cliente, estado…) con sus cinco idiomas, y un campo
// solo declara texto propio cuando dice algo que el diccionario no tiene. Así
// «Fecha» se traduce una vez para todo el ERP, y la misma cosa se llama igual en
// todos los módulos — que es la mitad de por qué esto se maneja mejor que la
// competencia, donde cada módulo bautizó lo mismo a su manera.
//
// Clean-room: catálogo propio, escrito desde el dominio funcional. Que un ERP
// tenga oportunidades, albaranes o partes de horas son ideas del oficio, no
// código con licencia (CONVENTIONS §12).
package catalogo

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
)

//go:embed modulos/*.json terminos.json
var ficheros embed.FS

// Idiomas son los cinco de producto (CONVENTIONS §13.1). Inglés es la base.
var Idiomas = []string{"en", "bg", "cs", "da", "de", "el", "es", "et", "fi", "fr", "ga", "hr", "hu", "is", "it", "lt", "lv", "mt", "nb", "nl", "pl", "pt", "ro", "sk", "sl", "sv"}

// Termino es un texto en los cinco idiomas.
type Termino map[string]string

// Tipos de campo admitidos. Cerrado a propósito: cada tipo que se añade es un
// control más que pintar en la interfaz y una regla más que validar, y un ERP
// con cuarenta tipos de campo es justo el que nadie sabe manejar.
const (
	TipoTexto      = "texto"
	TipoTextoLargo = "texto_largo"
	TipoEntero     = "entero"
	TipoDecimal    = "decimal"
	TipoDinero     = "dinero"
	TipoFecha      = "fecha"
	TipoFechaHora  = "fechahora"
	TipoBooleano   = "booleano"
	TipoSeleccion  = "seleccion"
	TipoRelacion   = "relacion"
	TipoCorreo     = "correo"
	TipoTelefono   = "telefono"
	TipoURL        = "url"
)

var tiposValidos = []string{
	TipoTexto, TipoTextoLargo, TipoEntero, TipoDecimal, TipoDinero,
	TipoFecha, TipoFechaHora, TipoBooleano, TipoSeleccion, TipoRelacion,
	TipoCorreo, TipoTelefono, TipoURL,
}

var (
	claveRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	// secuenciaRe: el prefijo de numeración de un documento (PV, FV, ALB).
	secuenciaRe = regexp.MustCompile(`^[A-Z]{2,6}$`)
)

// Campo es un campo declarado de un modelo.
type Campo struct {
	Clave string `json:"clave"`
	Tipo  string `json:"tipo"`
	// Requerido lo comprueba el dominio al escribir, no la base: un campo
	// obligatorio de hoy no puede invalidar las filas de ayer.
	Requerido bool `json:"requerido,omitempty"`
	// Modelo es el modelo apuntado (solo tipo relacion).
	Modelo string `json:"modelo,omitempty"`
	// Opciones son los valores admitidos (solo tipo seleccion).
	Opciones []string `json:"opciones,omitempty"`
	// Etiqueta sobreescribe el diccionario cuando el campo dice algo suyo.
	Etiqueta Termino `json:"etiqueta,omitempty"`
}

// Modelo es una entidad del ERP (contactos, pedidos de venta, vehículos).
type Modelo struct {
	Clave  string `json:"clave"`
	Modulo string `json:"modulo"`
	// Titulo es el campo cuyo valor hace de nombre legible de la fila. Vacío si
	// el modelo se numera: entonces el nombre legible es su referencia.
	Titulo string `json:"titulo,omitempty"`
	// Secuencia es el prefijo de numeración (PV → PV-2026-00042). Vacío = sin
	// numerar: un contacto no lleva número de documento.
	Secuencia string `json:"secuencia,omitempty"`
	// Estados es el flujo del documento. El primero es el inicial.
	Estados []string `json:"estados,omitempty"`
	Campos  []Campo  `json:"campos"`
	// Lista son las columnas de la vista de lista, en orden. Admite "ref" y
	// "estado" además de las claves de campo.
	Lista []string `json:"lista"`
	// Etiqueta es el nombre del modelo en plural ("Facturas de cliente").
	Etiqueta Termino `json:"etiqueta"`
	// Contable marca los modelos que generan asiento al confirmarse. El núcleo
	// no sabe qué es una factura; sabe que ESTE modelo pide libro diario.
	Contable *Contable `json:"contable,omitempty"`
}

// RecibeFacturas dice si el modelo es una factura RECIBIDA de un contacto: la
// que puede entrar como factura electrónica (UBL, CII o Facturae). Un gasto
// también es contable y no cobra, pero su tercero es un empleado.
func (m *Modelo) RecibeFacturas() bool {
	if m.Contable == nil || m.Contable.Cobro {
		return false
	}
	for _, f := range m.Campos {
		if f.Clave == m.Contable.CampoTercero {
			return f.Modelo == "contacto"
		}
	}
	return false
}

// Contable dice cómo se convierte un documento en asiento por partida doble.
// Es declarativo por el mismo motivo que el resto: la diferencia entre una
// factura de venta y una de compra son dos cuentas y un signo, no un módulo.
type Contable struct {
	// Diario al que va el asiento (ventas, compras, banco…).
	Diario string `json:"diario"`
	// Campos de los que salen los importes, en céntimos.
	CampoBase string `json:"campo_base"`
	CampoIVA  string `json:"campo_iva,omitempty"`
	// CampoTercero es el campo de relación con el contacto (cliente/proveedor).
	CampoTercero string `json:"campo_tercero,omitempty"`
	// Las tres cuentas del plan. `Contrapartida` es la de cliente/proveedor.
	CuentaContrapartida string `json:"cuenta_contrapartida"`
	CuentaIngresoGasto  string `json:"cuenta_ingreso_gasto"`
	CuentaIVA           string `json:"cuenta_iva,omitempty"`
	// Cobro indica el sentido: true en ventas (el tercero DEBE), false en
	// compras. Es el único bit que distingue los dos asientos.
	Cobro bool `json:"cobro"`
}

// Modulo agrupa modelos bajo una entrada de la barra lateral.
type Modulo struct {
	Clave        string    `json:"clave"`
	Dependencias []string  `json:"dependencias"`
	Modelos      []*Modelo `json:"modelos"`
}

// Catalogo es el catálogo completo ya validado.
type Catalogo struct {
	modelos      map[string]*Modelo
	modulos      []Modulo
	terminos     map[string]Termino
	dependencias map[string][]string
	transitivas  map[string][]string

	// vistas es la traducción de TODO el catálogo, ya serializada a JSON, para
	// cada uno de los cinco idiomas. Se calcula una vez al arrancar.
	//
	// Es la primera petición de cada carga de página y la que bloquea el primer
	// pintado. Medido: construirla y codificarla costaba 245 µs y 196 KB de
	// reservas POR PETICIÓN, para un resultado que no cambia nunca —el catálogo
	// va horneado en la imagen—. Guardarlo cuesta 266 KB de memoria en total, una
	// vez, y deja el manejador en una copia de bytes.
	vistas map[string]json.RawMessage
	// sellos es el ETag de cada idioma: la huella de esos bytes. Con él, una
	// recarga contesta 304 y no vuelve a bajar 54 KB (5,5 KB comprimidos).
	sellos map[string]string
}

// Cargar lee, valida y devuelve el catálogo embebido. Un manifiesto inválido es
// un error de ARRANQUE, no un aviso: un modelo mal escrito que llega vivo al
// servicio se convierte en un formulario roto en producción.
func Cargar() (*Catalogo, error) {
	terminos, err := cargarTerminos()
	if err != nil {
		return nil, err
	}

	entradas, err := fs.ReadDir(ficheros, "modulos")
	if err != nil {
		return nil, fmt.Errorf("catalogo: leer modulos/: %w", err)
	}
	c := &Catalogo{modelos: map[string]*Modelo{}, terminos: terminos}
	porModulo := map[string][]*Modelo{}

	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		crudo, err := ficheros.ReadFile("modulos/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("catalogo: %s: %w", e.Name(), err)
		}
		var ms []*Modelo
		if err := json.Unmarshal(crudo, &ms); err != nil {
			return nil, fmt.Errorf("catalogo: %s: %w", e.Name(), err)
		}
		for _, m := range ms {
			if _, dup := c.modelos[m.Clave]; dup {
				return nil, fmt.Errorf("catalogo: modelo duplicado %q (%s)", m.Clave, e.Name())
			}
			c.modelos[m.Clave] = m
			porModulo[m.Modulo] = append(porModulo[m.Modulo], m)
		}
	}

	// Segunda pasada: las relaciones apuntan a modelos que pueden vivir en otro
	// fichero, así que solo se pueden comprobar con todos cargados.
	for _, m := range c.modelos {
		if err := c.validar(m); err != nil {
			return nil, fmt.Errorf("catalogo: modelo %q: %w", m.Clave, err)
		}
	}

	c.resolverDependencias(porModulo)

	claves := make([]string, 0, len(porModulo))
	for k := range porModulo {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	for _, k := range claves {
		if _, ok := terminos["modulo."+k]; !ok {
			return nil, fmt.Errorf("catalogo: módulo %q sin rótulo en terminos.json", k)
		}
		ms := porModulo[k]
		sort.Slice(ms, func(i, j int) bool { return ms[i].Clave < ms[j].Clave })
		deps := c.dependencias[k]
		if deps == nil {
			deps = []string{}
		}
		c.modulos = append(c.modulos, Modulo{Clave: k, Dependencias: deps, Modelos: ms})
	}

	// Las cinco traducciones, de una vez y para siempre.
	c.vistas = make(map[string]json.RawMessage, len(Idiomas))
	c.sellos = make(map[string]string, len(Idiomas))
	for _, idioma := range Idiomas {
		crudo, err := json.Marshal(c.Vista(idioma))
		if err != nil {
			return nil, fmt.Errorf("catalogo: serializar la vista %q: %w", idioma, err)
		}
		c.vistas[idioma] = crudo
		suma := sha256.Sum256(crudo)
		c.sellos[idioma] = hex.EncodeToString(suma[:8])
	}
	return c, nil
}

func cargarTerminos() (map[string]Termino, error) {
	crudoTerminos, err := ficheros.ReadFile("terminos.json")
	if err != nil {
		return nil, fmt.Errorf("catalogo: leer terminos.json: %w", err)
	}
	terminos := map[string]Termino{}
	if err := json.Unmarshal(crudoTerminos, &terminos); err != nil {
		return nil, fmt.Errorf("catalogo: terminos.json: %w", err)
	}
	for clave, t := range terminos {
		if err := completo(t); err != nil {
			return nil, fmt.Errorf("catalogo: término %q: %w", clave, err)
		}
	}
	return terminos, nil
}

func (c *Catalogo) resolverDependencias(porModulo map[string][]*Modelo) {
	c.dependencias = make(map[string][]string, len(porModulo))
	for modClave, ms := range porModulo {
		depsSet := map[string]bool{}
		for _, m := range ms {
			for _, f := range m.Campos {
				if f.Tipo == TipoRelacion {
					if mDest, ok := c.modelos[f.Modelo]; ok && mDest.Modulo != modClave {
						depsSet[mDest.Modulo] = true
					}
				}
			}
			if m.Contable != nil && modClave != "contabilidad" {
				if _, ok := porModulo["contabilidad"]; ok {
					depsSet["contabilidad"] = true
				}
			}
		}
		deps := make([]string, 0, len(depsSet))
		for d := range depsSet {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		c.dependencias[modClave] = deps
	}

	c.transitivas = make(map[string][]string, len(porModulo))
	for modClave := range porModulo {
		c.transitivas[modClave] = c.calcularTransitivas(modClave)
	}
}

// VistaJSON devuelve el catálogo traducido y YA SERIALIZADO. Cae al inglés —el
// idioma base— si se pide uno que no existe.
func (c *Catalogo) VistaJSON(idioma string) json.RawMessage {
	if v, ok := c.vistas[idioma]; ok {
		return v
	}
	return c.vistas["en"]
}

// Sello es la huella del catálogo en ese idioma, para el ETag.
func (c *Catalogo) Sello(idioma string) string {
	if s, ok := c.sellos[idioma]; ok {
		return s
	}
	return c.sellos["en"]
}

// completo exige los cinco idiomas. Este es el gate: una función sin sus cinco
// idiomas no está acabada (CONVENTIONS §13.1), y aquí se puede exigir de verdad
// porque el texto es DATO — no hay que buscarlo por el código.
func completo(t Termino) error {
	for _, idioma := range Idiomas {
		if strings.TrimSpace(t[idioma]) == "" {
			return fmt.Errorf("falta el idioma %q", idioma)
		}
	}
	for idioma := range t {
		if !slices.Contains(Idiomas, idioma) {
			return fmt.Errorf("idioma desconocido %q", idioma)
		}
	}
	return nil
}

func (c *Catalogo) validar(m *Modelo) error {
	if !claveRe.MatchString(m.Clave) {
		return fmt.Errorf("clave inválida")
	}
	if !claveRe.MatchString(m.Modulo) {
		return fmt.Errorf("módulo inválido %q", m.Modulo)
	}
	if err := completo(m.Etiqueta); err != nil {
		return fmt.Errorf("etiqueta: %w", err)
	}
	if len(m.Campos) == 0 {
		return fmt.Errorf("sin campos")
	}
	vistos, err := c.validarCampos(m)
	if err != nil {
		return err
	}
	if err := validarVistas(m, vistos); err != nil {
		return err
	}
	return validarContable(m, vistos)
}

// validarCampos comprueba cada campo y devuelve el conjunto de claves declaradas,
// que es lo que necesitan las comprobaciones siguientes.
func (c *Catalogo) validarCampos(m *Modelo) (map[string]bool, error) {
	vistos := map[string]bool{}
	for _, f := range m.Campos {
		if !claveRe.MatchString(f.Clave) {
			return nil, fmt.Errorf("campo %q: clave inválida", f.Clave)
		}
		if vistos[f.Clave] {
			return nil, fmt.Errorf("campo %q: duplicado", f.Clave)
		}
		vistos[f.Clave] = true
		if !slices.Contains(tiposValidos, f.Tipo) {
			return nil, fmt.Errorf("campo %q: tipo desconocido %q", f.Clave, f.Tipo)
		}
		if f.Tipo == TipoRelacion {
			if _, ok := c.modelos[f.Modelo]; !ok {
				return nil, fmt.Errorf("campo %q: apunta al modelo inexistente %q", f.Clave, f.Modelo)
			}
		} else if f.Modelo != "" {
			return nil, fmt.Errorf("campo %q: solo las relaciones llevan modelo", f.Clave)
		}
		if f.Tipo == TipoSeleccion && len(f.Opciones) == 0 {
			return nil, fmt.Errorf("campo %q: selección sin opciones", f.Clave)
		}
		// El rótulo sale del diccionario salvo que el campo traiga el suyo. Sin
		// ninguno de los dos el formulario saldría con la clave interna puesta.
		if f.Etiqueta != nil {
			if err := completo(f.Etiqueta); err != nil {
				return nil, fmt.Errorf("campo %q: %w", f.Clave, err)
			}
		} else if _, ok := c.terminos["campo."+f.Clave]; !ok {
			return nil, fmt.Errorf("campo %q: sin rótulo (ni en terminos.json ni propio)", f.Clave)
		}
	}
	return vistos, nil
}

// validarVistas comprueba lo que decide cómo se PINTA el modelo: su nombre
// legible y las columnas de la lista.
func validarVistas(m *Modelo, vistos map[string]bool) error {
	// Nombre legible: o un campo de texto, o la referencia de un documento
	// numerado. Sin ninguno de los dos, las listas saldrían con UUID.
	switch {
	case m.Titulo != "" && !vistos[m.Titulo]:
		return fmt.Errorf("titulo %q no es un campo del modelo", m.Titulo)
	case m.Titulo == "" && m.Secuencia == "":
		return fmt.Errorf("sin titulo ni secuencia: la fila no tendría nombre legible")
	}
	if m.Secuencia != "" && !secuenciaRe.MatchString(m.Secuencia) {
		return fmt.Errorf("secuencia %q: dos a seis mayúsculas", m.Secuencia)
	}
	if len(m.Lista) == 0 {
		return fmt.Errorf("sin columnas de lista")
	}
	for _, col := range m.Lista {
		if col == "ref" || col == "estado" {
			continue
		}
		if !vistos[col] {
			return fmt.Errorf("columna %q no es un campo del modelo", col)
		}
	}
	if slices.Contains(m.Lista, "ref") && m.Secuencia == "" {
		return fmt.Errorf("lista con columna \"ref\" pero el modelo no se numera")
	}
	if slices.Contains(m.Lista, "estado") && len(m.Estados) == 0 {
		return fmt.Errorf("lista con columna \"estado\" pero el modelo no tiene estados")
	}
	return nil
}

// validarContable comprueba lo único que mueve dinero: que un modelo que va al
// libro traiga diario, cuentas y la fecha del documento.
func validarContable(m *Modelo, vistos map[string]bool) error {
	cont := m.Contable
	if cont == nil {
		return nil
	}
	if !vistos[cont.CampoBase] {
		return fmt.Errorf("contable: campo_base %q no existe", cont.CampoBase)
	}
	if cont.CampoIVA != "" && !vistos[cont.CampoIVA] {
		return fmt.Errorf("contable: campo_iva %q no existe", cont.CampoIVA)
	}
	if cont.CampoTercero != "" && !vistos[cont.CampoTercero] {
		return fmt.Errorf("contable: campo_tercero %q no existe", cont.CampoTercero)
	}
	if cont.Diario == "" || cont.CuentaContrapartida == "" || cont.CuentaIngresoGasto == "" {
		return fmt.Errorf("contable: faltan diario o cuentas")
	}
	if cont.CampoIVA != "" && cont.CuentaIVA == "" {
		return fmt.Errorf("contable: hay campo_iva pero no cuenta_iva")
	}
	if m.Secuencia == "" {
		return fmt.Errorf("contable: un documento que va al libro tiene que estar numerado")
	}
	// El asiento lleva la fecha DEL DOCUMENTO, no la de cuando se confirma:
	// facturar el 31 y asentar el 2 son dos ejercicios distintos.
	if !vistos["fecha"] {
		return fmt.Errorf("contable: falta el campo \"fecha\" (es la del asiento)")
	}
	return nil
}

// Modelo devuelve el modelo por su clave.
func (c *Catalogo) Modelo(clave string) (*Modelo, bool) {
	m, ok := c.modelos[clave]
	return m, ok
}

// Modulos devuelve los módulos ordenados por clave.
func (c *Catalogo) Modulos() []Modulo { return c.modulos }

// Dependencias devuelve las dependencias directas de un módulo.
func (c *Catalogo) Dependencias(modulo string) []string {
	return c.dependencias[modulo]
}

// DependenciasTransitivas devuelve todas las dependencias transitivas de un módulo en orden.
func (c *Catalogo) DependenciasTransitivas(modulo string) []string {
	return c.transitivas[modulo]
}

// DependientesTransitivas devuelve todos los módulos que dependen directa o indirectamente
// de `raiz`, incluyendo los que tienen dependencias bidireccionales o interdependientes con él.
// Son los módulos que deben desactivarse si `raiz` se desactiva.
func (c *Catalogo) DependientesTransitivas(raiz string) []string {
	// Grafo inverso: rev[A] contiene todos los módulos B que dependen de A.
	rev := make(map[string][]string)
	for mod, deps := range c.dependencias {
		for _, d := range deps {
			rev[d] = append(rev[d], mod)
		}
	}

	visitados := map[string]bool{raiz: true}
	var cola []string
	cola = append(cola, rev[raiz]...)

	// Módulos con dependencia bidireccional directa con raiz
	for _, d := range c.dependencias[raiz] {
		if slices.Contains(c.dependencias[d], raiz) && !visitados[d] {
			cola = append(cola, d)
		}
	}

	var orden []string
	for len(cola) > 0 {
		actual := cola[0]
		cola = cola[1:]
		if visitados[actual] {
			continue
		}
		visitados[actual] = true
		orden = append(orden, actual)

		for _, dep := range rev[actual] {
			if !visitados[dep] {
				cola = append(cola, dep)
			}
		}
		for _, d := range c.dependencias[actual] {
			if slices.Contains(c.dependencias[d], actual) && !visitados[d] {
				cola = append(cola, d)
			}
		}
	}
	sort.Strings(orden)
	return orden
}

func (c *Catalogo) calcularTransitivas(raiz string) []string {
	visitados := map[string]bool{raiz: true}
	var orden []string
	var cola []string
	cola = append(cola, c.dependencias[raiz]...)
	for len(cola) > 0 {
		actual := cola[0]
		cola = cola[1:]
		if visitados[actual] {
			continue
		}
		visitados[actual] = true
		orden = append(orden, actual)
		for _, dep := range c.dependencias[actual] {
			if !visitados[dep] {
				cola = append(cola, dep)
			}
		}
	}
	sort.Strings(orden)
	return orden
}

// Rotulo traduce una clave del diccionario al idioma pedido, cayendo a inglés
// (idioma base) si el idioma no está.
func (c *Catalogo) Rotulo(clave, idioma string) string {
	t, ok := c.terminos[clave]
	if !ok {
		return ""
	}
	return traducir(t, idioma)
}

func traducir(t Termino, idioma string) string {
	if s := t[idioma]; s != "" {
		return s
	}
	return t["en"]
}

// --- Vista traducida: lo que se envía al navegador --------------------------
//
// El servicio traduce y el navegador PINTA. Ni una cadena de estos módulos vive
// en el frontend, así que añadir un módulo no toca el frontend ni sus cinco
// catálogos de mensajes.

// CampoVista es un campo con sus textos ya resueltos.
type CampoVista struct {
	Clave     string   `json:"clave"`
	Tipo      string   `json:"tipo"`
	Etiqueta  string   `json:"etiqueta"`
	Requerido bool     `json:"requerido,omitempty"`
	Modelo    string   `json:"modelo,omitempty"`
	Opciones  []Opcion `json:"opciones,omitempty"`
}

// Opcion es un valor de una selección con su rótulo traducido.
type Opcion struct {
	Valor    string `json:"valor"`
	Etiqueta string `json:"etiqueta"`
}

// ModeloVista es un modelo listo para pintar.
type ModeloVista struct {
	Clave    string `json:"clave"`
	Modulo   string `json:"modulo"`
	Etiqueta string `json:"etiqueta"`
	Numerado bool   `json:"numerado"`
	Contable bool   `json:"contable"`
	// Cobro: el documento se EMITE (factura de venta). Es lo que decide si se
	// puede sacar como factura electrónica UBL.
	Cobro bool `json:"cobro,omitempty"`
	// Recibe: el documento llega de un proveedor y se puede importar desde su
	// factura electrónica.
	Recibe  bool         `json:"recibe,omitempty"`
	Campos  []CampoVista `json:"campos"`
	Lista   []string     `json:"lista"`
	Estados []Opcion     `json:"estados,omitempty"`
}

// ModuloVista es un módulo con sus modelos, listo para la barra lateral.
type ModuloVista struct {
	Clave        string        `json:"clave"`
	Etiqueta     string        `json:"etiqueta"`
	Dependencias []string      `json:"dependencias"`
	Modelos      []ModeloVista `json:"modelos"`
}

// Vista traduce el catálogo entero al idioma pedido.
func (c *Catalogo) Vista(idioma string) []ModuloVista {
	out := make([]ModuloVista, 0, len(c.modulos))
	for _, mod := range c.modulos {
		deps := mod.Dependencias
		if deps == nil {
			deps = []string{}
		}
		v := ModuloVista{
			Clave:        mod.Clave,
			Etiqueta:     c.Rotulo("modulo."+mod.Clave, idioma),
			Dependencias: deps,
		}
		for _, m := range mod.Modelos {
			v.Modelos = append(v.Modelos, c.VistaModelo(m, idioma))
		}
		out = append(out, v)
	}
	return out
}

// VistaModelo traduce un modelo al idioma pedido.
func (c *Catalogo) VistaModelo(m *Modelo, idioma string) ModeloVista {
	v := ModeloVista{
		Clave:    m.Clave,
		Modulo:   m.Modulo,
		Etiqueta: traducir(m.Etiqueta, idioma),
		Numerado: m.Secuencia != "",
		Contable: m.Contable != nil,
		Cobro:    m.Contable != nil && m.Contable.Cobro,
		Recibe:   m.RecibeFacturas(),
		Lista:    m.Lista,
	}
	for _, f := range m.Campos {
		rotulo := c.Rotulo("campo."+f.Clave, idioma)
		if f.Etiqueta != nil {
			rotulo = traducir(f.Etiqueta, idioma)
		}
		cv := CampoVista{
			Clave: f.Clave, Tipo: f.Tipo, Etiqueta: rotulo,
			Requerido: f.Requerido, Modelo: f.Modelo,
		}
		for _, op := range f.Opciones {
			cv.Opciones = append(cv.Opciones, Opcion{Valor: op, Etiqueta: c.rotuloValor(op, idioma)})
		}
		v.Campos = append(v.Campos, cv)
	}
	for _, e := range m.Estados {
		v.Estados = append(v.Estados, Opcion{Valor: e, Etiqueta: c.rotuloValor(e, idioma)})
	}
	return v
}

// rotuloValor traduce un valor de selección o estado. Cae al valor crudo si no
// está en el diccionario: un desplegable con el valor interno se lee mal, pero
// se lee — y `validar` ya obliga a que los rótulos de campo existan, que es
// donde un hueco rompería el formulario entero.
func (c *Catalogo) rotuloValor(valor, idioma string) string {
	if s := c.Rotulo("valor."+valor, idioma); s != "" {
		return s
	}
	return valor
}
