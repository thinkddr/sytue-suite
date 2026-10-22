// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

const libroConNombres = `{"active":0,"sheets":[{"name":"Ventas","cells":{"B2":"10","B3":"20","C1":"=SUM(Total)","C2":"=Total*IVA"}},{"name":"Otra","cells":{"A1":"=IVA"}}],` +
	`"names":[{"name":"Total","ref":"Ventas!$B$2:$B$3"},{"name":"IVA","ref":"0.21","comment":"general"},{"name":"IVA","ref":"0.1","sheet":"Otra"}]}`

func libroDePrueba(t *testing.T, contenido string) *Lienzo {
	t.Helper()
	lz := lienzoDePrueba()
	if !lz.Cargar(contenido) {
		t.Fatal("el libro de la prueba tiene que entenderse entero")
	}
	return lz
}

// Un libro con nombres se ENTIENDE (antes se abría en solo lectura por la clave `names`),
// las fórmulas los resuelven con su ámbito y guardarlo los devuelve tal cual.
func TestLosNombresSeCarganSeResuelvenYSeGuardan(t *testing.T) {
	if _, ok := leer(libroConNombres); !ok {
		t.Fatal("un libro con `names` tiene que entenderse entero")
	}
	lz := libroDePrueba(t, libroConNombres)
	l := lz.Libro
	if got := l.Texto(2, 0); got != "30" {
		t.Errorf("=SUM(Total) da %q", got)
	}
	// Total es un rango: en C2 la intersección implícita toma B2 (10 × 0,21).
	if got := l.Texto(2, 1); got != "2.1" && got != "2,1" {
		t.Errorf("=Total*IVA con el IVA del libro da %q", got)
	}
	l.Activar(1)
	if got := l.Texto(0, 0); got != "0.1" && got != "0,1" {
		t.Errorf("en «Otra» el IVA de la hoja tapa al del libro: %q", got)
	}
	var m struct {
		Names []Nombre `json:"names"`
	}
	if err := json.Unmarshal([]byte(lz.Serializar()), &m); err != nil || len(m.Names) != 3 ||
		m.Names[1] != (Nombre{Name: "IVA", Ref: "0.21", Comment: "general"}) || m.Names[2].Sheet != "Otra" {
		t.Errorf("los nombres no vuelven como entraron: %+v %v", m.Names, err)
	}
	// Y un libro sin nombres no gana la clave.
	if s := libroDePrueba(t, `{"active":0,"sheets":[{"name":"H","cells":{}}]}`).Serializar(); strings.Contains(s, "names") {
		t.Errorf("un libro sin nombres no escribe `names`: %s", s)
	}
}

// Los mismos casos que `sheets-nombres.test.ts`: qué nombre vale y por qué no.
func TestErrorDeNombre(t *testing.T) {
	l := libroDePrueba(t, libroConNombres).Libro
	casos := []struct {
		n      Nombre
		quiere string
	}{
		{Nombre{Name: "  ", Ref: "1"}, "empty"},
		{Nombre{Name: strings.Repeat("a", 256), Ref: "1"}, "tooLong"},
		{Nombre{Name: "con espacio", Ref: "1"}, "invalid"},
		{Nombre{Name: "A1", Ref: "1"}, "invalid"},
		{Nombre{Name: "R2C3", Ref: "1"}, "invalid"},
		{Nombre{Name: "Nuevo", Ref: "1", Sheet: "NoExiste"}, "noSheet"},
		{Nombre{Name: "total", Ref: "1"}, "taken"},
		{Nombre{Name: "Total", Ref: "1", Sheet: "Otra"}, ""}, // otro ámbito
		{Nombre{Name: "Nuevo", Ref: "SUM("}, "badRef"},
		{Nombre{Name: "Nuevo", Ref: ""}, "badRef"},
		{Nombre{Name: "Nuevo", Ref: "=Ventas!$B$2*2"}, ""},
	}
	for _, c := range casos {
		if got := l.ErrorDeNombre(c.n, nil); got != c.quiere {
			t.Errorf("%+v: %q, quiere %q", c.n, got, c.quiere)
		}
	}
	// Editándose a sí mismo no choca consigo.
	prev := l.Nombres()[0]
	if got := l.ErrorDeNombre(Nombre{Name: "TOTAL", Ref: "1"}, &prev); got != "" {
		t.Errorf("editar el propio nombre no es «taken»: %q", got)
	}
}

func TestNormalizarRefNombre(t *testing.T) {
	for entrada, quiere := range map[string]string{
		"B2:B9":      "Ventas!$B$2:$B$9",
		"=b2":        "Ventas!$B$2",
		"Otra!A1:A3": "Otra!$A$1:$A$3",
		"B2*2":       "B2*2",
		"0.21":       "0.21",
		"$C$4":       "Ventas!$C$4",
	} {
		if got := NormalizarRefNombre(entrada, "Ventas"); got != quiere {
			t.Errorf("%q: %q, quiere %q", entrada, got, quiere)
		}
	}
	if got := NormalizarRefNombre("A1", "Mi hoja"); got != "'Mi hoja'!$A$1" {
		t.Errorf("una hoja con espacio va entre comillas: %q", got)
	}
}

// Crear, renombrar (reescribe las fórmulas que lo usan, no las funciones ni los prefijos)
// y borrar; deshacer devuelve el nombre.
func TestMandarNombreCreaRenombraYBorra(t *testing.T) {
	lz := libroDePrueba(t, libroConNombres)
	l := lz.Libro
	if c := l.MandarNombre("nameset", `{"name":"Meses","ref":"B2:B3"}`); c != "" {
		t.Fatalf("crear: %q", c)
	}
	if n := l.Nombres(); len(n) != 4 || n[3].Ref != "Ventas!$B$2:$B$3" {
		t.Fatalf("el nuevo se guarda normalizado: %+v", n)
	}
	if c := l.MandarNombre("nameset", `{"name":"Total","ref":"1"}`); c != "taken" {
		t.Errorf("uno repetido: %q", c)
	}
	if c := l.MandarNombre("nameset", `{"name":"Suma","ref":"Ventas!$B$2:$B$3","prev":{"name":"Total"}}`); c != "" {
		t.Fatalf("renombrar: %q", c)
	}
	if got := l.Crudo(2, 0); got != "=SUM(Suma)" {
		t.Errorf("la fórmula que lo usaba se reescribe: %q", got)
	}
	if got := l.Crudo(2, 1); got != "=Suma*IVA" {
		t.Errorf("y la otra: %q", got)
	}
	if got := l.Texto(2, 0); got != "30" {
		t.Errorf("y sigue calculando: %q", got)
	}
	if c := l.MandarNombre("namedel", `{"name":"Suma"}`); c != "" || len(l.Nombres()) != 3 {
		t.Fatalf("borrar: %q %+v", c, l.Nombres())
	}
	if got := l.Texto(2, 0); !strings.HasPrefix(got, "#") {
		t.Errorf("borrar no toca las fórmulas: pasan a error, y da %q", got)
	}
	if !l.Deshacer() || len(l.Nombres()) != 4 || l.Texto(2, 0) != "30" {
		t.Errorf("deshacer devuelve el nombre: %+v %q", l.Nombres(), l.Texto(2, 0))
	}
}

func TestRenombrarEnFormulaRespetaFuncionesYPrefijos(t *testing.T) {
	for entrada, quiere := range map[string]string{
		"=Total+total(1)+Total!A1": "=Suma+total(1)+Total!A1",
		`="Total"&Total`:           `="Total"&Suma`,
		"=Tabla[Total]+Total":      "=Tabla[Total]+Suma",
		"Total":                    "Total",
	} {
		if got := renombrarEnFormula(entrada, "Total", "Suma"); got != quiere {
			t.Errorf("%q: %q, quiere %q", entrada, got, quiere)
		}
	}
}

// Las definiciones siguen a sus celdas al insertar una fila, al renombrar la hoja y al
// borrarla.
func TestLosNombresSiguenASusCeldas(t *testing.T) {
	l := libroDePrueba(t, libroConNombres).Libro
	l.Estructura("insrow", "above", 0, 0, 0, 0)
	if got := l.Nombres()[0].Ref; got != "Ventas!$B$3:$B$4" {
		t.Errorf("tras insertar una fila arriba: %q", got)
	}
	if got := l.Texto(2, 1); got != "30" {
		t.Errorf("la suma sigue dando lo mismo: %q", got)
	}
	l.Renombrar(0, "Año 2026")
	if got := l.Nombres()[0].Ref; got != "'Año 2026'!$B$3:$B$4" {
		t.Errorf("tras renombrar la hoja: %q", got)
	}
	l.Renombrar(1, "Resto")
	if got := l.Nombres()[2].Sheet; got != "Resto" {
		t.Errorf("el ámbito sigue a su hoja: %q", got)
	}
	l.Activar(1)
	l.Borrar()
	if n := l.Nombres(); len(n) != 2 {
		t.Errorf("borrar la hoja se lleva su nombre de hoja: %+v", n)
	}
	d := formula.Desplazamiento{Filas: true, Desde: 2, Cantidad: -2, Tope: Filas}
	if got := moverNombres([]Nombre{{Name: "X", Ref: "H!$B$3:$B$4"}}, "H", "H", d)[0].Ref; !strings.Contains(got, formula.RefRota) {
		t.Errorf("lo borrado entero pasa a #¡REF!: %q", got)
	}
}
