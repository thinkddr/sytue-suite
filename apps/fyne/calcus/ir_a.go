// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"regexp"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Ir a» y el cuadro de nombres (`sheets/irA.ts`)
//
// Lo que se escribe en el cuadro de la barra de fórmulas —«B12», «Hoja2!A1:C5», «A:C»
// o un nombre definido— es la hoja y el rango a los que saltar. Ctrl+G y F5 llevan el
// foco allí. Es el puerto de la regla pura de la web, y los dos dan lo mismo con los
// vectores compartidos (`TestVectoresDeIrA`).

// DestinoIrA es adónde se va: la hoja (índice) y el rango, de base cero y ordenado.
type DestinoIrA struct {
	Hoja           int
	C1, R1, C2, R2 int
}

// Por qué no se puede ir: la clave de su texto es `sheet.goTo.<código>`. errIrAVacio no
// avisa: Intro en el cuadro vacío solo devuelve el foco a la hoja.
const (
	errIrAVacio    = "vacio"
	errIrAInvalido = "invalido"
	errIrAHoja     = "hoja"
	errIrANombre   = "nombre"
	errIrAFuera    = "fuera"
	errIrANoRango  = "noRango"
)

var (
	// reConHoja es «Hoja!Nombre» o «'Mi hoja'!Nombre» (`CON_HOJA_RE`).
	reConHoja = regexp.MustCompile(`^(?:'((?:[^']|'')+)'|([^!']+))!(.+)$`)
	// reFormaNombre es lo que puede ser un nombre definido (`FORMA_NOMBRE_RE`).
	reFormaNombre = regexp.MustCompile(`^[\p{L}_\\][\p{L}\p{N}_.\\?]*$`)
)

// indiceHoja busca una hoja por su nombre sin distinguir mayúsculas, como Excel; -1 si
// no está.
func indiceHoja(hojas []string, nombre string) int {
	for i, h := range hojas {
		if h == nombre {
			return i
		}
	}
	for i, h := range hojas {
		if strings.EqualFold(h, nombre) {
			return i
		}
	}
	return -1
}

// referenciaIrA es `referencia`: si el texto es UNA referencia entera, su destino (o su
// error); ok false si no lo es.
func referenciaIrA(texto string, hojas []string, porDefecto int) (d DestinoIrA, codigo string, ok bool) {
	refs := formula.ExtractRefs("=" + texto)
	if len(refs) != 1 || refs[0].Inicio != 1 || refs[0].Fin != len(texto)+1 {
		return d, "", false
	}
	r := refs[0]
	hoja := porDefecto
	if r.Hoja != "" {
		hoja = indiceHoja(hojas, r.Hoja)
	}
	if hoja < 0 {
		return d, errIrAHoja, true
	}
	// Un eje libre llega hasta 2^20 − 1, que son las filas de la hoja pero no sus columnas.
	c2, r2 := r.Rect.C2, r.Rect.R2
	if c2 == formula.EjeEntero {
		c2 = MaxColumnas - 1
	}
	if r2 == formula.EjeEntero {
		r2 = MaxFilas - 1
	}
	if c2 > MaxColumnas-1 || r2 > MaxFilas-1 {
		return d, errIrAFuera, true
	}
	return DestinoIrA{Hoja: hoja, C1: r.Rect.C1, R1: r.Rect.R1, C2: c2, R2: r2}, "", true
}

// irA es `irA`: el destino de lo escrito en el cuadro de nombres, o el código de por qué
// no lo hay. Primero una referencia; si no, un nombre definido (el de la hoja activa
// tapa al del libro, y «Hoja2!Nombre» busca el de esa hoja), cuya definición tiene que
// ser un rango.
func irA(texto string, hojas []string, activa int, nombres []Nombre) (DestinoIrA, string) {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(texto), "="))
	if t == "" {
		return DestinoIrA{}, errIrAVacio
	}
	if d, codigo, ok := referenciaIrA(t, hojas, activa); ok {
		return d, codigo
	}
	ambito, nombre := "", t
	if activa >= 0 && activa < len(hojas) {
		ambito = hojas[activa]
	}
	m := reConHoja.FindStringSubmatch(t)
	if m != nil {
		hoja := m[2]
		if m[1] != "" {
			hoja = strings.ReplaceAll(m[1], "''", "'")
		}
		i := indiceHoja(hojas, hoja)
		if i < 0 {
			return DestinoIrA{}, errIrAHoja
		}
		// «Año!B2»: en una fórmula esa hoja iría entre comillas, pero en el cuadro se
		// escribe tal cual, así que lo de detrás del «!» se mira también como referencia.
		if d, codigo, ok := referenciaIrA(m[3], hojas, i); ok {
			return d, codigo
		}
		ambito, nombre = hojas[i], m[3]
	}
	if !reFormaNombre.MatchString(nombre) {
		return DestinoIrA{}, errIrAInvalido
	}
	def := buscarNombre(nombres, ambito, nombre, m == nil)
	if def == nil {
		return DestinoIrA{}, errIrANombre
	}
	// Una referencia sin hoja dentro de un nombre es de su hoja, o de la activa si es del libro.
	hojaDef := activa
	if def.Sheet != "" {
		hojaDef = max(indiceHoja(hojas, def.Sheet), 0)
	}
	ref := strings.TrimPrefix(strings.TrimSpace(def.Ref), "=")
	if d, codigo, ok := referenciaIrA(ref, hojas, hojaDef); ok {
		return d, codigo
	}
	return DestinoIrA{}, errIrANoRango
}

// buscarNombre es el nombre definido que se llama `nombre` (sin distinguir
// mayúsculas): el de la hoja `ambito` tapa al del libro, que solo vale con `delLibro`
// (con «Hoja!Nombre» solo cuenta el de esa hoja). Nil si no hay.
func buscarNombre(nombres []Nombre, ambito, nombre string, delLibro bool) *Nombre {
	for i := range nombres {
		if nombres[i].Sheet == ambito && nombres[i].Sheet != "" && strings.EqualFold(nombres[i].Name, nombre) {
			return &nombres[i]
		}
	}
	if !delLibro {
		return nil
	}
	for i := range nombres {
		if nombres[i].Sheet == "" && strings.EqualFold(nombres[i].Name, nombre) {
			return &nombres[i]
		}
	}
	return nil
}

// nombresParaIrA es `nombresParaIrA`: los nombres que ve la hoja activa y que llevan a un
// rango, por su texto en mayúsculas.
func nombresParaIrA(hojas []string, activa int, nombres []Nombre) []string {
	hoja := ""
	if activa >= 0 && activa < len(hojas) {
		hoja = hojas[activa]
	}
	vistos := map[string]string{}
	var orden []string
	for _, n := range nombres {
		k := strings.ToUpper(n.Name)
		if n.Sheet != "" && n.Sheet != hoja {
			continue
		}
		if _, ya := vistos[k]; ya && n.Sheet == "" {
			continue
		}
		if _, ya := vistos[k]; !ya {
			orden = append(orden, k)
		}
		vistos[k] = n.Name
	}
	out := []string{}
	for _, k := range orden {
		if _, codigo := irA(vistos[k], hojas, activa, nombres); codigo == "" {
			out = append(out, vistos[k])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToUpper(out[i]) < strings.ToUpper(out[j]) })
	return out
}

// IrA es Intro en el cuadro de nombres: salta a lo escrito —cambiando de hoja si hace
// falta— y devuelve el foco a la rejilla, o avisa sin moverse. Devuelve si se ha ido (o
// no había nada que hacer).
func (c *Lienzo) IrA(texto string) bool {
	d, codigo := irA(texto, c.Libro.Hojas(), c.Libro.Activa(), c.Libro.Nombres())
	if codigo == errIrAVacio {
		c.Rejilla.recuperarFoco()
		return true
	}
	if codigo != "" {
		if c.AlAviso != nil {
			c.AlAviso(ui.AvisoAdvertencia, c.t("sheet.goTo.error", nil),
				c.t("sheet.goTo."+codigo, i18n.Vars{"texto": strings.TrimSpace(texto)}))
		}
		return false
	}
	c.CambiarHoja(d.Hoja)
	c.Rejilla.SeleccionarDestino(d.C1, d.R1, d.C2, d.R2)
	c.Rejilla.recuperarFoco()
	return true
}

// SeleccionarDestino marca el rango de «Ir a» y trae a la vista su esquina de ARRIBA,
// que es por donde se lee (la web hace lo mismo con `revelarRef = 'ancla'`). Una celda
// sola cubierta por una combinada es su ancla.
func (r *Rejilla) SeleccionarDestino(c1, f1, c2, f2 int) {
	if c1 == c2 && f1 == f2 {
		r.Seleccionar(c1, f1, false)
		return
	}
	r.sel, r.ext = Celda{C: c1, F: f1}, &Celda{C: c2, F: f2}
	r.crecer(r.desX, r.desY)
	r.aLaVista(c1, f1)
	if r.AlMoverse != nil {
		r.AlMoverse()
	}
	r.Refresh()
}

// enfocarCuadro es Ctrl+G o F5 con el foco en la rejilla: lo lleva al cuadro de nombres.
func (r *Rejilla) enfocarCuadro() {
	if r.AlIrA != nil {
		r.AlIrA()
	}
}
