// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"cmp"
	"context"
	"errors"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Comprobar en VIES lo que se propone
//
// El NIF-IVA de un cliente o de un proveedor —el que escribe la persona, o el que propone
// Sytue AI al rellenar una ficha o en un plan— se puede comprobar en VIES, el servicio de
// la Comisión Europea: «Comprobar en VIES» debajo del NIF de la ficha de un contacto y en
// cada contacto nuevo del plan. Lo contestado (válido o no, y el nombre y la dirección
// registrados, si el país los publica) va AL LADO de lo propuesto y no cambia ningún
// campo: nada es automático, y nada sale sin pulsar.
//
// Lo ofrece quien tenga con qué preguntar (Pantalla.ComprobarVIES): Sytue Office, por el
// portero de red. Su valor cero es el de siempre: la ficha y el plan no lo enseñan.

// ModeloContacto es el de clientes y proveedores, el único con NIF-IVA que se comprueba.
const ModeloContacto = "contacto"

// PlazoVIES es lo que se espera a VIES, que pregunta a su vez al servicio del país.
const PlazoVIES = 40 * time.Second

// ComprobacionVIES es lo que contesta VIES de un NIF-IVA.
type ComprobacionVIES struct {
	// Pais y Numero son lo preguntado: «ES» y «B12345678».
	Pais, Numero string
	// Valido: dado de alta para operaciones dentro de la UE.
	Valido bool
	// Nombre y Direccion son los registrados; vacíos si el país no los publica.
	Nombre, Direccion string
}

// FalloVIES es por qué no se pudo comprobar, para decirlo en el idioma de la pantalla.
// Quien pregunta (Pantalla.ComprobarVIES) devuelve uno de estos; cualquier otro error se
// dice como VIESNoResponde.
type FalloVIES string

// Los fallos de una comprobación.
const (
	// VIESNoEsNIFIVA: no tiene forma de NIF-IVA de la UE (sin prefijo de país ni país
	// del contacto que lo dé).
	VIESNoEsNIFIVA FalloVIES = "nif"
	// VIESNoResponde: VIES, o el servicio del país detrás, no contesta ahora.
	VIESNoResponde FalloVIES = "no_responde"
	// VIESSinRed: no se llegó (sin conexión, o «Sin conexión» encendido).
	VIESSinRed FalloVIES = "sin_red"
)

func (f FalloVIES) Error() string { return "erp: VIES: " + string(f) }

// EstadoVIES es una comprobación: en marcha, lo que contestó o por qué no se pudo.
type EstadoVIES struct {
	// Nif es el que se preguntó: si el campo cambia, lo contestado ya no es de él.
	Nif         string
	Consultando bool
	Resultado   *ComprobacionVIES
	Fallo       FalloVIES
}

// deEste dice si el estado es del NIF que hay ahora.
func (e *EstadoVIES) deEste(nif string) bool {
	return e != nil && strings.EqualFold(strings.Join(strings.Fields(e.Nif), ""), strings.Join(strings.Fields(nif), ""))
}

// propuestoVIES es lo que se compara con VIES: el NIF y el nombre y la dirección de la
// ficha o del plan, con los rótulos de sus campos en el catálogo.
type propuestoVIES struct {
	nif, nombre, direccion        string
	rotuloNombre, rotuloDireccion string
	columna                       string // «En la ficha» o «Propuesto»
}

// propuestoDe es lo propuesto en un contacto: sus datos y los rótulos de su modelo.
func propuestoDe(tr *i18n.Translator, m Modelo, datos func(string) string, columna string) propuestoVIES {
	return propuestoVIES{
		nif: datos("nif"), nombre: datos("nombre"), direccion: direccionDe(datos),
		rotuloNombre: rotuloCampo(m, "nombre"), rotuloDireccion: rotuloCampo(m, "direccion"),
		columna: textoVIES(columna, tr),
	}
}

// TextosVIES son los textos de «Comprobar en VIES», que solo dice Sytue Office (es, en;
// los otros veinticuatro idiomas, en `traduccionesVIES`, textos_vies_idiomas.go).
var TextosVIES = map[string][2]string{
	"comprobar":       {"Comprobar en VIES", "Check in VIES"},
	"pista":           {"Solo al pulsarlo: el NIF-IVA se consulta en VIES, el servicio de la Comisión Europea.", "Only when you press it: the VAT number is checked in VIES, the European Commission’s service."},
	"valido":          {"Válido en VIES", "Valid in VIES"},
	"noValido":        {"No válido en VIES", "Not valid in VIES"},
	"noValidoTexto":   {"VIES no lo reconoce como NIF-IVA para operaciones dentro de la UE.", "VIES does not recognise it as a VAT number for trade within the EU."},
	"sinDatos":        {"Su país no publica el nombre ni la dirección.", "Its country does not publish the name or the address."},
	"enVIES":          {"En VIES", "In VIES"},
	"enFicha":         {"En la ficha", "In the record"},
	"propuesto":       {"Propuesto", "Proposed"},
	"falloNif":        {"No parece un NIF-IVA europeo: ponle delante el prefijo del país (ES, FR, DE…) o rellena el país del contacto.", "This does not look like an EU VAT number: put the country prefix in front (ES, FR, DE…) or fill in the contact’s country."},
	"falloNoResponde": {"VIES o el servicio del país no responde ahora. Prueba más tarde.", "VIES or the country’s service is not answering right now. Try again later."},
	"falloSinRed":     {"No se ha podido llegar a VIES. Mira la conexión, y que «Sin conexión» no esté puesto en Ajustes.", "Could not reach VIES. Check the connection, and that «Offline» is not on in Settings."},
}

// claveFalloVIES es el texto de cada fallo.
var claveFalloVIES = map[FalloVIES]string{
	VIESNoEsNIFIVA: "falloNif",
	VIESNoResponde: "falloNoResponde",
	VIESSinRed:     "falloSinRed",
}

func textoVIES(clave string, tr *i18n.Translator) string {
	return i18n.Propios{Base: TextosVIES, Traducciones: traduccionesVIES}.En(tr.Locale(), clave)
}

// falloVIESDe es el fallo de un error de quien pregunta.
func falloVIESDe(err error) FalloVIES {
	var f FalloVIES
	if errors.As(err, &f) {
		if _, ok := claveFalloVIES[f]; ok {
			return f
		}
	}
	return VIESNoResponde
}

// bloqueVIES es «Comprobar en VIES» con su pista y, si ya se preguntó por este NIF, lo
// que contestó al lado de lo propuesto, o por qué no se pudo.
func bloqueVIES(tr *i18n.Translator, v tema.Variante, e *EstadoVIES, pr propuestoVIES, alPulsar func(), ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	boton := ui.NuevoBoton(textoVIES("comprobar", tr), func() {
		if alPulsar != nil {
			alPulsar()
		}
	}).Con(ui.Secundario, ui.TallaSM)
	actual := e.deEste(pr.nif)
	boton.Cargando = actual && e.Consultando
	// No se apaga sin NIF: la ficha no se repinta al teclear, y pulsarlo sin él lo dice.
	boton.Inerte = alPulsar == nil || boton.Cargando
	o, h := botonera([]pieza{piezaDe(boton, boton.MinSize())}, ancho)
	p.add(o, h, 0)
	pista := ui.NuevoParrafo(textoVIES("pista", tr), tema.TextoXS, ui.Normal, "text-subtle", v)
	p.add(pista, ui.AltoParrafo(pista, ancho), HuecoUno)
	if actual && !e.Consultando {
		switch {
		case e.Fallo != "":
			o, h := ayudaPeligro(textoVIES(claveFalloVIES[e.Fallo], tr), ancho)
			p.add(o, h, HuecoDos)
		case e.Resultado != nil:
			o, h := resultadoVIES(tr, v, *e.Resultado, pr, ancho)
			p.add(o, h, HuecoDos)
		}
	}
	return p.objeto()
}

// resultadoVIES es lo que contestó VIES: válido o no, con el NIF preguntado, y el nombre
// y la dirección registrados al lado de los propuestos.
func resultadoVIES(tr *i18n.Translator, v tema.Variante, r ComprobacionVIES, pr propuestoVIES, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	nif := r.Pais + " " + r.Numero
	var aviso *ui.AyudaEnLinea
	switch {
	case !r.Valido:
		aviso = ui.NuevaAyudaEnLinea(nif + " — " + textoVIES("noValidoTexto", tr)).ConTono(ui.AyudaAviso).ConTitulo(textoVIES("noValido", tr))
	case r.Nombre == "" && r.Direccion == "":
		aviso = ui.NuevaAyudaEnLinea(nif + " — " + textoVIES("sinDatos", tr)).ConTono(ui.AyudaExito).ConTitulo(textoVIES("valido", tr))
	default:
		aviso = ui.NuevaAyudaEnLinea(nif).ConTono(ui.AyudaExito).ConTitulo(textoVIES("valido", tr))
	}
	p.add(aviso, ui.AltoAyudaEnLinea(aviso, ancho), 0)
	if r.Valido && (r.Nombre != "" || r.Direccion != "") {
		o, h := comparaVIES(tr, v, r, pr, ancho)
		p.add(o, h, HuecoDos)
	}
	return p.objeto()
}

// comparaVIES es la tabla de lo registrado en VIES junto a lo propuesto: una fila para el
// nombre y otra para la dirección, con el rótulo del campo a la izquierda. Lo que VIES no
// da (o la ficha no tiene) es «—».
func comparaVIES(tr *i18n.Translator, v tema.Variante, r ComprobacionVIES, pr propuestoVIES, ancho float32) (fyne.CanvasObject, float32) {
	filas := [][3]string{
		{"", textoVIES("enVIES", tr), pr.columna},
		{pr.rotuloNombre, cmp.Or(r.Nombre, "—"), cmp.Or(strings.TrimSpace(pr.nombre), "—")},
		{pr.rotuloDireccion, cmp.Or(r.Direccion, "—"), cmp.Or(strings.TrimSpace(pr.direccion), "—")},
	}
	var rotulo float32
	for _, f := range filas {
		rotulo = max(rotulo, ui.Medir(f[0], tema.TextoXS, ui.Medio).Width)
	}
	col := max((ancho-rotulo-2*HuecoDlX)/2, 0)
	l := &lienzo{}
	var y float32
	for i, f := range filas {
		tinta, peso := "text", ui.Normal
		if i == 0 {
			tinta, peso = "text-subtle", ui.Medio
		}
		r := ui.NuevoParrafo(f[0], tema.TextoXS, ui.Medio, "text-subtle", v)
		a := ui.NuevoParrafo(f[1], tema.TextoXS, peso, tinta, v)
		b := ui.NuevoParrafo(f[2], tema.TextoXS, peso, tinta, v)
		alto := max(ui.AltoParrafo(r, rotulo), ui.AltoParrafo(a, col), ui.AltoParrafo(b, col), tema.TextoXS.Linea)
		l.poner(r, 0, y, rotulo, alto)
		l.poner(a, rotulo+HuecoDlX, y, col, alto)
		l.poner(b, rotulo+2*HuecoDlX+col, y, col, alto)
		y += alto
		if i < len(filas)-1 {
			y += HuecoUno
		}
	}
	return l.objeto(y)
}

// direccionDe junta la dirección de un contacto como la da VIES: calle, código postal y
// ciudad.
func direccionDe(datos func(string) string) string {
	var partes []string
	if c := strings.TrimSpace(datos("direccion")); c != "" {
		partes = append(partes, c)
	}
	if cp := strings.TrimSpace(strings.TrimSpace(datos("cp")) + " " + strings.TrimSpace(datos("ciudad"))); cp != "" {
		partes = append(partes, cp)
	}
	return strings.Join(partes, ", ")
}

// rotuloCampo es la etiqueta de un campo del modelo en el catálogo, o la clave.
func rotuloCampo(m Modelo, clave string) string {
	for _, c := range m.Campos {
		if c.Clave == clave {
			return c.Etiqueta
		}
	}
	return clave
}

// tieneCampo dice si el modelo tiene ese campo.
func tieneCampo(m Modelo, clave string) bool {
	for _, c := range m.Campos {
		if c.Clave == clave {
			return true
		}
	}
	return false
}

// paisVIES es el país con el que se completa un NIF sin prefijo: el del contacto o, si no
// lo tiene, el de la organización.
func (p *Pantalla) paisVIES(delContacto string) string {
	if s := strings.TrimSpace(delContacto); s != "" {
		return s
	}
	if p.Organizacion != nil {
		return p.Organizacion.Pais
	}
	return ""
}

// consultarVIES pregunta a VIES por nif y deja lo contestado en *e; `pintar` se llama al
// empezar y al acabar, en el hilo de la interfaz. `vigente` dice si lo que llega aún
// tiene dónde pintarse (la ficha sigue abierta, el plan es el mismo).
func (p *Pantalla) consultarVIES(e **EstadoVIES, nif, pais string, vigente func() bool, pintar func()) {
	if p.ComprobarVIES == nil || ((*e).deEste(nif) && (*e).Consultando) {
		return
	}
	if strings.TrimSpace(nif) == "" {
		*e = &EstadoVIES{Nif: nif, Fallo: VIESNoEsNIFIVA}
		pintar()
		return
	}
	*e = &EstadoVIES{Nif: nif, Consultando: true}
	pintar()
	comprobar := p.ComprobarVIES
	p.fondo(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), PlazoVIES)
		defer cancelar()
		r, err := comprobar(ctx, nif, pais)
		enHilo(func() {
			if !vigente() {
				return
			}
			hecho := &EstadoVIES{Nif: nif}
			if err != nil {
				hecho.Fallo = falloVIESDe(err)
			} else {
				hecho.Resultado = &r
			}
			*e = hecho
			pintar()
		})
	})
}

// comprobarVIESFicha es «Comprobar en VIES» en la ficha de un contacto.
func (p *Pantalla) comprobarVIESFicha(f *Ficha) {
	p.consultarVIES(&f.VIES, f.Valores["nif"], p.paisVIES(f.Valores["pais"]),
		func() bool { return p.esta(f) }, p.Repintar)
}

// comprobarVIESPaso es «Comprobar en VIES» en un contacto nuevo del plan de Sytue AI.
func (p *Pantalla) comprobarVIESPaso(x *PanelIA, pintar func(), ref string) {
	if x.Plan == nil {
		return
	}
	for _, paso := range x.Plan.Pasos {
		if paso.Ref != ref || paso.Existente != nil {
			continue
		}
		if x.VIES == nil {
			x.VIES = map[string]*EstadoVIES{}
		}
		plan, gen := x.Plan, x.Generacion
		e := x.VIES[ref]
		p.consultarVIES(&e, texto(paso.Data["nif"]), p.paisVIES(texto(paso.Data["pais"])),
			func() bool { return x.Plan == plan && x.Generacion == gen },
			func() {
				if x.Plan == plan && x.Generacion == gen {
					x.VIES[ref] = e
				}
				pintar()
			})
		return
	}
}
