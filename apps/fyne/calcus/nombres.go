// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Nombres definidos del libro (`SheetModel.names`): las reglas del Administrador de
// nombres, las de `sheets/nombres.ts` de la web. El motor ya sabía resolverlos
// (`Engine.DefineName`); lo que faltaba era guardarlos: un libro con `=SUMA(Ventas)` se
// abría en solo lectura por la clave `names`, y editarlo aquí la habría perdido.
//
//   - validarlo como Excel (sin espacios, sin forma de referencia A1 ni R1C1, 255
//     caracteres, único por ámbito y distinto de cualquier tabla);
//   - cargarlo en el motor con su ámbito (libro, u hoja: el de la hoja tapa al del libro);
//   - renombrarlo reescribiendo las fórmulas que lo usan;
//   - y que su definición siga a sus celdas al insertar o borrar filas y columnas, al
//     renombrar la hoja o al borrarla.

// Nombre es un `DefinedName`: Sheet vacío es ámbito de libro.
type Nombre struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	Sheet   string `json:"sheet,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// maxNombre es el tope de Excel para la longitud de un nombre.
const maxNombre = 255

var (
	// reR1C1 es la forma de referencia R1C1 («RC», «R2C3», «R4», «C7»): Excel no la admite.
	reR1C1 = regexp.MustCompile(`(?i)^(R\d*C\d*|R\d+|C\d+)$`)
	// reHojaSimple es el nombre de hoja que no pide comillas en un prefijo.
	reHojaSimple = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.$]*$`)
)

// claveAmbito: el mismo nombre puede existir una vez por ámbito (libro, o cada hoja).
func claveAmbito(nombre, hoja string) string {
	return hoja + "\x00" + strings.ToUpper(strings.TrimSpace(nombre))
}

// prefijoHoja es `sheetPrefix`: «Hoja1!» o «'Mi hoja'!».
func prefijoHoja(hoja string) string {
	if reHojaSimple.MatchString(hoja) {
		return hoja + "!"
	}
	return "'" + strings.ReplaceAll(hoja, "'", "''") + "'!"
}

// sanearNombres es `sanitizeNames`: conserva todo nombre con forma mínima, aunque el motor
// no lo entienda (uno con tilde que viene de Excel se guarda y vuelve a salir igual).
func sanearNombres(crudo any) []Nombre {
	lista, ok := crudo.([]any)
	if !ok {
		return nil
	}
	var out []Nombre
	for _, x := range lista {
		o, ok := x.(map[string]any)
		if !ok {
			continue
		}
		nombre, _ := o["name"].(string)
		ref, hayRef := o["ref"].(string)
		if nombre == "" || !hayRef {
			continue
		}
		n := Nombre{Name: nombre, Ref: ref}
		n.Sheet, _ = o["sheet"].(string)
		n.Comment, _ = o["comment"].(string)
		out = append(out, n)
	}
	return out
}

// Nombres son los nombres definidos del libro, en su orden.
func (l *Libro) Nombres() []Nombre { return slices.Clone(l.f.nombres) }

// definirNombres carga los nombres en el motor (`defineNames`).
func (l *Libro) definirNombres() {
	hojas := l.Hojas()
	for _, n := range l.f.nombres {
		switch {
		case n.Sheet == "":
			l.motor.DefineName(n.Name, n.Ref, "")
		case slices.Contains(hojas, n.Sheet):
			l.motor.DefineSheetName(n.Name, n.Ref, n.Sheet)
		}
	}
}

// probador es un motor desechable para preguntar si un nombre o una definición valen: el
// motor es quien sabe qué es un identificador y qué parsea.
func probador() *formula.Engine { return formula.New(formula.Options{Sheets: []string{"x"}}) }

// ErrorDeNombre dice por qué `n` no se puede guardar (editando `prev`, si no es nil): el
// código de `names.error.<código>`, o "" si vale. Es `nameError`.
func (l *Libro) ErrorDeNombre(n Nombre, prev *Nombre) string {
	nombre := strings.TrimSpace(n.Name)
	switch {
	case nombre == "":
		return "empty"
	case utf8.RuneCountInString(nombre) > maxNombre:
		return "tooLong"
	case reR1C1.MatchString(nombre) || !probador().DefineName(nombre, "1", ""):
		return "invalid"
	case n.Sheet != "" && !slices.Contains(l.Hojas(), n.Sheet):
		return "noSheet"
	}
	clave, propia := claveAmbito(nombre, n.Sheet), ""
	if prev != nil {
		propia = claveAmbito(prev.Name, prev.Sheet)
	}
	for _, x := range l.f.nombres {
		if k := claveAmbito(x.Name, x.Sheet); k == clave && (prev == nil || k != propia) {
			return "taken"
		}
	}
	for _, h := range l.f.hojas {
		for _, t := range h.tablas {
			if strings.EqualFold(t.Name, nombre) {
				return "taken"
			}
		}
	}
	ref := strings.TrimPrefix(strings.TrimSpace(n.Ref), "=")
	if ref == "" || !probador().DefineName("X_", ref, "") {
		return "badRef"
	}
	return ""
}

// NormalizarRefNombre es `normalizeRef`: la definición tal y como se guarda, sin «=» y, si
// es un rango suelto («B2:B9» o «Hoja1!B2:B9»), cualificado con su hoja y en absoluto
// («Hoja1!$B$2:$B$9»). Es como lo escriben Excel y Calc.
//
// ponytail: una fórmula («B2*2») se guarda tal cual, como en la web.
func NormalizarRefNombre(ref, hoja string) string {
	texto := strings.TrimPrefix(strings.TrimSpace(ref), "=")
	refs := formula.ExtractRefs("=" + texto)
	if len(refs) != 1 || refs[0].Inicio != 1 || refs[0].Fin != len(texto)+1 {
		return texto
	}
	r := refs[0]
	a1 := r.Texto[strings.LastIndex(r.Texto, "!")+1:]
	if _, ok := formula.ParseRangeRef(a1); !ok { // A:A, 1:1
		return texto
	}
	abs := func(c, f int) string { return "$" + formula.NumToCol(c) + "$" + strconv.Itoa(f+1) }
	rango := abs(r.Rect.C1, r.Rect.R1)
	if r.Rect.C1 != r.Rect.C2 || r.Rect.R1 != r.Rect.R2 {
		rango += ":" + abs(r.Rect.C2, r.Rect.R2)
	}
	if r.Hoja != "" {
		hoja = r.Hoja
	}
	return prefijoHoja(hoja) + rango
}

// ve dice si una fórmula de `hoja` resuelve `n`: uno de hoja solo allí; uno del libro,
// donde ningún nombre de la hoja se llame igual (`sees`).
func ve(n Nombre, hoja string, nombres []Nombre) bool {
	if n.Sheet != "" {
		return n.Sheet == hoja
	}
	for _, x := range nombres {
		if x.Sheet == hoja && strings.EqualFold(x.Name, n.Name) {
			return false
		}
	}
	return true
}

// renombrarEnFormula cambia `viejo` por `nuevo` donde la fórmula lo usa como nombre: no en
// una función, ni en un prefijo de hoja, ni dentro de `Tabla[…]` (`renombrarEnFormula`).
func renombrarEnFormula(texto, viejo, nuevo string) string {
	if !strings.HasPrefix(texto, "=") {
		return texto
	}
	toks := formula.TokenizeFx(texto)
	out := texto
	for k := len(toks) - 1; k >= 0; k-- {
		t := toks[k]
		if t.T != formula.FxWord || !strings.EqualFold(t.V, viejo) {
			continue
		}
		if k+1 < len(toks) && (toks[k+1].T == formula.FxLParen || toks[k+1].T == formula.FxBang) {
			continue
		}
		if k > 0 && toks[k-1].T == formula.FxBang {
			continue
		}
		if t.Inicio > 0 && strings.ContainsRune("[@", rune(texto[t.Inicio-1])) {
			continue
		}
		if t.Fin < len(texto) && strings.ContainsRune("[]", rune(texto[t.Fin])) {
			continue
		}
		out = out[:t.Inicio] + nuevo + out[t.Fin:]
	}
	return out
}

// reescribirRefs reescribe las referencias de una definición que apuntan a `hoja` (las
// sin hoja cuentan como de `base`); `a` decide el texto nuevo de cada una (`rewriteRefs`).
func reescribirRefs(ref, hoja, base string, a func(string) string) string {
	texto := "=" + ref
	refs := formula.ExtractRefs(texto)
	out := texto
	for i := len(refs) - 1; i >= 0; i-- {
		r := refs[i]
		de := r.Hoja
		if de == "" {
			de = base
		}
		if de != hoja {
			continue
		}
		if nuevo := a(r.Texto); nuevo != r.Texto {
			out = out[:r.Inicio] + nuevo + out[r.Fin:]
		}
	}
	return out[1:]
}

// moverNombres: las definiciones siguen a sus celdas al insertar o borrar filas o columnas
// en `hoja` (`primera` es la hoja base de los nombres del libro). Lo borrado entero pasa a
// #¡REF!, como en las fórmulas.
func moverNombres(nombres []Nombre, hoja, primera string, d formula.Desplazamiento) []Nombre {
	out := slices.Clone(nombres)
	for i, n := range out {
		base := n.Sheet
		if base == "" {
			base = primera
		}
		out[i].Ref = reescribirRefs(n.Ref, hoja, base, d.MoverReferencia)
	}
	return out
}

// renombrarHojaEnNombres cambia el prefijo de la hoja en las definiciones y el ámbito de
// sus nombres.
func renombrarHojaEnNombres(nombres []Nombre, viejo, nuevo string) []Nombre {
	out := slices.Clone(nombres)
	for i, n := range out {
		// Base imposible: solo se tocan las refs que nombran la hoja explícitamente.
		out[i].Ref = reescribirRefs(n.Ref, viejo, "\x00", func(t string) string {
			return prefijoHoja(nuevo) + t[strings.LastIndex(t, "!")+1:]
		})
		if n.Sheet == viejo {
			out[i].Sheet = nuevo
		}
	}
	return out
}

// quitarHojaDeNombres: borrar una hoja se lleva sus nombres de hoja (como Excel). Los del
// libro que la citaban se quedan, y dan #¡REF! al usarse.
func quitarHojaDeNombres(nombres []Nombre, hoja string) []Nombre {
	return slices.DeleteFunc(slices.Clone(nombres), func(n Nombre) bool { return n.Sheet == hoja })
}

// PeticionNombre es lo que pide el Administrador (`NameRequest`, el valor JSON de
// `nameset` y `namedel`).
type PeticionNombre struct {
	Name    string `json:"name"`
	Ref     string `json:"ref,omitempty"`
	Sheet   string `json:"sheet,omitempty"`
	Comment string `json:"comment,omitempty"`
	// Prev, al editar: el nombre que se cambia (nombre y ámbito de antes).
	Prev *struct {
		Name  string `json:"name"`
		Sheet string `json:"sheet,omitempty"`
	} `json:"prev,omitempty"`
}

// MandarNombre es `applyNameCommand`: crea, edita o borra un nombre. Devuelve el código de
// error (`names.error.<código>`) o "". Renombrar reescribe las fórmulas que lo usaban —las
// celdas y las otras definiciones—; si eso tocara una hoja protegida, no se hace nada.
// Borrar NO toca las fórmulas: pasan a #NAME?, que es lo honesto.
func (l *Libro) MandarNombre(cmd, valor string) string {
	var req PeticionNombre
	if json.Unmarshal([]byte(valor), &req) != nil {
		return ""
	}
	nombres := l.f.nombres
	if cmd == "namedel" {
		clave := claveAmbito(req.Name, req.Sheet)
		quedan := slices.DeleteFunc(slices.Clone(nombres), func(n Nombre) bool { return claveAmbito(n.Name, n.Sheet) == clave })
		if len(quedan) == len(nombres) {
			return ""
		}
		l.anotar()
		l.f.nombres = quedan
		l.rehacerMotor()
		return ""
	}
	base := req.Sheet
	if base == "" {
		base = l.Hoja()
	}
	draft := Nombre{
		Name: strings.TrimSpace(req.Name), Ref: NormalizarRefNombre(req.Ref, base),
		Sheet: req.Sheet, Comment: strings.TrimSpace(req.Comment),
	}
	iPrev := -1
	if req.Prev != nil {
		clave := claveAmbito(req.Prev.Name, req.Prev.Sheet)
		iPrev = slices.IndexFunc(nombres, func(n Nombre) bool { return claveAmbito(n.Name, n.Sheet) == clave })
	}
	var prev *Nombre
	if iPrev >= 0 {
		prev = &nombres[iPrev]
	}
	if codigo := l.ErrorDeNombre(draft, prev); codigo != "" {
		return codigo
	}
	lista := slices.Clone(nombres)
	if prev != nil {
		lista[iPrev] = draft
	} else {
		lista = append(lista, draft)
	}
	f := l.copia()
	if prev != nil && !strings.EqualFold(prev.Name, draft.Name) {
		if !renombrarUsos(&f, lista, iPrev, *prev, draft.Name, nombres) {
			return "protected"
		}
	}
	l.anotar()
	f.nombres = lista
	l.restaurar(f)
	return ""
}

// renombrarUsos reescribe, en la foto y en la lista, las fórmulas que veían `prev` para
// que usen `nuevo`. False si habría que tocar una hoja protegida.
func renombrarUsos(f *foto, lista []Nombre, iPrev int, prev Nombre, nuevo string, antes []Nombre) bool {
	for i := range f.hojas {
		h := &f.hojas[i]
		if !ve(prev, h.nombre, antes) {
			continue
		}
		for k, html := range h.celdas {
			if !esFormula(html) {
				continue
			}
			viejo := textoPlano(html)
			cambiado := renombrarEnFormula(viejo, prev.Name, nuevo)
			if cambiado == viejo {
				continue
			}
			if h.proteccion != nil {
				return false
			}
			p := leerCelda(html)
			p.dentro = escaparHTML(cambiado)
			h.celdas[k] = p.html()
		}
	}
	primera := f.hojas[0].nombre
	for i, n := range lista {
		base := n.Sheet
		if base == "" {
			base = primera
		}
		if i == iPrev || !ve(prev, base, antes) {
			continue
		}
		lista[i].Ref = renombrarEnFormula("="+n.Ref, prev.Name, nuevo)[1:]
	}
	return true
}

// ClaveErrorNombre es la clave de catálogo de cada código de error de un nombre.
var ClaveErrorNombre = map[string]string{
	"empty":     "names.error.empty",
	"invalid":   "names.error.invalid",
	"tooLong":   "names.error.tooLong",
	"taken":     "names.error.taken",
	"badRef":    "names.error.badRef",
	"noSheet":   "names.error.noSheet",
	"protected": "names.error.protected",
}
