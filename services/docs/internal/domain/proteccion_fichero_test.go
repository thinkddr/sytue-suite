// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Protección de Calcus en .xlsx y .ods (proteccion_fichero.go): ida y vuelta, lo que lee de
// Excel y de LibreOffice, y que una contraseña (o su hash) nunca sale en un fichero.

func hojaProtegida() string {
	return `{"active":0,"protect":{"hasPassword":true},"sheets":[
	{"name":"Cuentas","cells":{"A1":"10","B1":"libre","C1":"=A1*2"},
	 "styles":{"B1":{"locked":false},"C1":{"hidden":true},"D1":{"locked":false,"hidden":true,"fill":"#FFF2CC"}},
	 "protect":{"hasPassword":true,"formatCells":true,"insertRows":true,"sort":true,"selectCells":false,
	            "ranges":[{"ref":"B1","users":["ana"]}]}},
	{"name":"Libre","cells":{"A1":"x"}}]}`
}

func protectComoMapa(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if raw == nil {
		return nil
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// lo que sobrevive a un fichero: los permisos, nunca la contraseña ni los rangos por persona.
func comprobarProteccionDeFichero(t *testing.T, donde string, m sheetModel) {
	t.Helper()
	if len(m.Sheets) != 2 {
		t.Fatalf("%s: %d hojas", donde, len(m.Sheets))
	}
	if protectComoMapa(t, m.Protect) == nil {
		t.Errorf("%s: el libro perdió la protección de estructura", donde)
	}
	if protectComoMapa(t, m.Sheets[1].Protect) != nil {
		t.Errorf("%s: una hoja sin proteger salió protegida: %s", donde, m.Sheets[1].Protect)
	}
	p := protectComoMapa(t, m.Sheets[0].Protect)
	if p == nil {
		t.Fatalf("%s: la hoja perdió la protección", donde)
	}
	for _, permitido := range []string{"formatCells", "insertRows", "sort"} {
		if p[permitido] != true {
			t.Errorf("%s: %s debía seguir permitido: %v", donde, permitido, p)
		}
	}
	for _, vetado := range []string{"deleteRows", "insertCols", "deleteCols", "autoFilter", "objects", "scenarios"} {
		if p[vetado] == true {
			t.Errorf("%s: %s debía seguir vetado (más bloqueado nunca es menos): %v", donde, vetado, p)
		}
	}
	if p["selectCells"] != false {
		t.Errorf("%s: selectCells=false se perdió: %v", donde, p)
	}
	if _, hay := p["ranges"]; hay {
		t.Errorf("%s: los rangos por persona no caben en un fichero: %v", donde, p)
	}
	s := m.Sheets[0].Styles
	if s["B1"].Locked == nil || *s["B1"].Locked {
		t.Errorf("%s: B1 debía salir desbloqueada: %+v", donde, s["B1"])
	}
	if !s["C1"].Hidden || s["C1"].Locked != nil {
		t.Errorf("%s: C1 debía salir con la fórmula oculta y bloqueada: %+v", donde, s["C1"])
	}
	if d := s["D1"]; d.Locked == nil || *d.Locked || !d.Hidden || d.Fill == "" {
		t.Errorf("%s: D1 (celda vacía con estilo) = %+v", donde, d)
	}
}

func hojaImportada(t *testing.T, nombre string, data []byte) (sheetModel, *ImportResult) {
	t.Helper()
	res, err := Import(nombre, data)
	if err != nil {
		t.Fatal(err)
	}
	var m sheetModel
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatal(err)
	}
	return m, res
}

func TestProteccionDeHojaEnXlsxIdaYVuelta(t *testing.T) {
	data, ext := exportarHoja(t, hojaProtegida(), FormatOOXML)
	m, res := hojaImportada(t, "x."+ext, data)
	comprobarProteccionDeFichero(t, "xlsx", m)
	if len(res.Hashes) != 0 {
		t.Fatalf("un fichero de Sytue no lleva contraseña: %v", res.Hashes)
	}
	files, _ := readZip(data)
	hoja := string(files["xl/worksheets/sheet1.xml"])
	// OOXML: 1 = bloqueado con la hoja protegida.
	for _, quiero := range []string{`<sheetProtection sheet="1"`, `insertRows="0"`, `deleteRows="1"`, `sort="0"`, `selectLockedCells="1"`} {
		if !strings.Contains(hoja, quiero) {
			t.Errorf("falta %s en %s", quiero, hoja)
		}
	}
	if i, j := strings.Index(hoja, "<sheetProtection"), strings.Index(hoja, "</sheetData>"); i < j {
		t.Error("sheetProtection va tras sheetData")
	}
	if strings.Contains(hoja, "hashValue") || strings.Contains(string(files["xl/workbook.xml"]), "hashValue") {
		t.Error("el fichero lleva un hash de contraseña")
	}
	if !strings.Contains(string(files["xl/workbook.xml"]), `<workbookProtection lockStructure="1"/>`) {
		t.Error("falta workbookProtection")
	}
}

func TestProteccionDeHojaEnOdsIdaYVuelta(t *testing.T) {
	data, ext := exportarHoja(t, hojaProtegida(), FormatODF)
	m, _ := hojaImportada(t, "x."+ext, data)
	comprobarProteccionDeFichero(t, "ods", m)
	files, _ := readZip(data)
	c := string(files["content.xml"])
	for _, quiero := range []string{`table:protected="true"`, `table:structure-protected="true"`, `style:cell-protect="none"`, `style:cell-protect="hidden-and-protected"`, `style:cell-protect="formula-hidden"`} {
		if !strings.Contains(c, quiero) {
			t.Errorf("falta %s", quiero)
		}
	}
	if strings.Contains(c, "protection-key") {
		t.Error("el fichero lleva una clave")
	}
}

// Una hoja sin protección no gana nada ni cambia: el fichero es el de siempre.
func TestSinProteccionElFicheroNoLlevaNada(t *testing.T) {
	for _, f := range []BlobFormat{FormatOOXML, FormatODF} {
		data, _ := exportarHoja(t, `{"active":0,"sheets":[{"name":"H","cells":{"A1":"1"}}]}`, f)
		files, _ := readZip(data)
		for nombre, c := range files {
			if strings.Contains(string(c), "rotection") && !strings.Contains(nombre, "styles") {
				t.Errorf("%s lleva protección: %s", nombre, nombre)
			}
		}
	}
}

// Excel: la contraseña de una hoja es SHA-512 con sal y vueltas, el esquema del servicio.
func TestProteccionImportaElHashDeExcel(t *testing.T) {
	h, err := HashClave("de-excel")
	if err != nil {
		t.Fatal(err)
	}
	w := h.Wire()
	hoja := func(attrs string) []byte {
		data, _ := exportarHoja(t, hojaProtegida(), FormatOOXML)
		files, _ := readZip(data)
		files["xl/worksheets/sheet1.xml"] = []byte(strings.Replace(string(files["xl/worksheets/sheet1.xml"]), `<sheetProtection sheet="1"`, `<sheetProtection sheet="1" `+attrs, 1))
		out := map[string]string{}
		for k, v := range files {
			out[k] = string(v)
		}
		b, err := zipPackage(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	attrs := func(alg string, vueltas int, sal, hash string) string {
		return `algorithmName="` + alg + `" hashValue="` + hash + `" saltValue="` + sal + `" spinCount="` + itoa(vueltas) + `"`
	}
	_, res := hojaImportada(t, "excel.xlsx", hoja(attrs(w.Algoritmo, w.Vueltas, w.Sal, w.Hash)))
	got, ok := res.Hashes[AmbitoHoja("Cuentas")]
	if !ok || len(res.Hashes) != 1 {
		t.Fatalf("hashes importados: %v", res.Hashes)
	}
	ch, err := got.aHash()
	if err != nil || !ch.Coincide("de-excel") || ch.Coincide("otra") {
		t.Fatalf("el hash importado no verifica la contraseña: %v", err)
	}
	// Ni el modelo ni lo que se devuelve lleva el hash.
	res2, _ := Import("excel.xlsx", hoja(attrs(w.Algoritmo, w.Vueltas, w.Sal, w.Hash)))
	if strings.Contains(res2.Content, w.Hash) || strings.Contains(res2.Content, claveImportada) {
		t.Fatal("el hash quedó dentro del modelo")
	}
	// Lo que el servicio no sabe verificar, o es hostil, se importa SIN contraseña.
	for nombre, a := range map[string]string{
		"SHA-1":         attrs("SHA-1", w.Vueltas, w.Sal, w.Hash),
		"vueltas 1e9":   attrs(w.Algoritmo, 1_000_000_000, w.Sal, w.Hash),
		"vueltas cero":  attrs(w.Algoritmo, 0, w.Sal, w.Hash),
		"hash corto":    attrs(w.Algoritmo, w.Vueltas, w.Sal, "AAAA"),
		"sal vacía":     attrs(w.Algoritmo, w.Vueltas, "", w.Hash),
		"no es base64":  attrs(w.Algoritmo, w.Vueltas, "%%%", w.Hash),
		"clave antigua": `password="CC1A"`,
	} {
		m, res := hojaImportada(t, "excel.xlsx", hoja(a))
		if len(res.Hashes) != 0 {
			t.Errorf("%s: se aceptó el hash", nombre)
		}
		if protectComoMapa(t, m.Sheets[0].Protect) == nil {
			t.Errorf("%s: la hoja debía seguir protegida, sin contraseña", nombre)
		}
	}
}

// El hash importado, por la puerta del servicio: pone la contraseña a quien lo trae, y esa
// contraseña protege como cualquier otra. Un hash de otra clase se rechaza.
func TestProteccionHashesDeFicheroEnElServicio(t *testing.T) {
	h, _ := HashClave("de-excel")
	w := h.Wire()
	modelo := func(a1 string) json.RawMessage {
		inner := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"` + a1 + `"},"protect":{}}]}`
		v, _ := json.Marshal(inner)
		return v
	}
	est := EstadoProteccion{Claves: map[string]ClaveHash{}}
	op := func(a1 string, c uint64) Op {
		return Op{Kind: OpSet, Key: claveHoja, Value: modelo(a1), Dot: Dot{Actor: "c", Counter: c}, Lamport: c}
	}
	res, err := AplicarProteccion(est, []Op{op("1", 1)}, true, Proteccion{Hashes: map[string]ClaveWire{AmbitoHoja("H"): w}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Estado.Claves[AmbitoHoja("H")]; !got.Coincide("de-excel") {
		t.Fatal("el hash del fichero no quedó como contraseña")
	}
	if !strings.Contains(string(res.Ops[0].Value), `hasPassword`) || strings.Contains(string(res.Ops[0].Value), w.Hash) {
		t.Fatalf("modelo registrado: %s", res.Ops[0].Value)
	}
	// Con ella, un editor sin contraseña ya no escribe en la hoja bloqueada.
	est = res.Estado
	if _, err := AplicarProteccion(est, []Op{op("2", 2)}, false, Proteccion{}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("un editor cambió una celda bloqueada: %v", err)
	}
	// Un editor no cambia la contraseña con un hash de su elección.
	if _, err := AplicarProteccion(est, []Op{op("1", 3)}, false, Proteccion{Hashes: map[string]ClaveWire{AmbitoHoja("H"): w}}); err != nil {
		t.Fatalf("el mismo hash no es un cambio: %v", err)
	}
	otro, _ := HashClave("otra")
	if _, err := AplicarProteccion(est, []Op{op("1", 4)}, false, Proteccion{Hashes: map[string]ClaveWire{AmbitoHoja("H"): otro.Wire()}}); !errors.Is(err, ErrProtegido) {
		t.Fatalf("un editor cambió la contraseña con un hash propio: %v", err)
	}
	// Hash inválido: error, no contraseña muda.
	malo := w
	malo.Vueltas = 1_000_000_000
	if _, err := AplicarProteccion(EstadoProteccion{Claves: map[string]ClaveHash{}}, []Op{op("1", 5)}, true, Proteccion{Hashes: map[string]ClaveWire{AmbitoHoja("H"): malo}}); !errors.Is(err, ErrHashInvalido) {
		t.Fatalf("hash hostil: %v", err)
	}
}

// La contraseña de un documento protegido no sale nunca en ningún fichero.
func TestProteccionLaContrasenaNoSaleEnElFichero(t *testing.T) {
	const secreto = "s3creto-fichero-4b7"
	modelo := `{"active":0,"sheets":[{"name":"H","cells":{"A1":"1"},"protect":{"password":"` + secreto + `","hasPassword":true}}]}`
	for _, f := range []BlobFormat{FormatOOXML, FormatODF, FormatHTML, FormatMarkdown} {
		data, _ := exportarHoja(t, modelo, f)
		if strings.Contains(string(data), secreto) {
			t.Errorf("%s lleva la contraseña en claro", f)
		}
		if files, err := readZip(data); err == nil {
			for n, c := range files {
				if strings.Contains(string(c), secreto) {
					t.Errorf("%s/%s lleva la contraseña en claro", f, n)
				}
			}
		}
	}
}

// LibreOffice abre lo que exportamos y conserva la protección al guardarlo de vuelta (donde
// hay `soffice`: el contenedor de los agentes).
func TestProteccionSeAbreYSeConservaEnLibreOffice(t *testing.T) {
	for _, c := range []struct {
		formato BlobFormat
		ext, lo string
	}{
		{FormatOOXML, "xlsx", "xlsx:Calc MS Excel 2007 XML"},
		{FormatODF, "ods", "ods"},
	} {
		data, _ := exportarHoja(t, hojaProtegida(), c.formato)
		if pdf := sofficeConvertir(t, data, c.ext, "pdf"); !strings.HasPrefix(string(pdf), "%PDF") {
			t.Fatalf("%s: LibreOffice no abrió el fichero", c.ext)
		}
		m, _ := hojaImportada(t, "lo."+c.ext, sofficeConvertir(t, data, c.ext, c.lo))
		if protectComoMapa(t, m.Sheets[0].Protect) == nil {
			t.Errorf("%s: LibreOffice perdió la protección de la hoja", c.ext)
		}
		if protectComoMapa(t, m.Sheets[1].Protect) != nil {
			t.Errorf("%s: LibreOffice protegió la hoja libre", c.ext)
		}
		// ponytail: el .ods que reescribe LibreOffice no vuelve a aplicar el estilo de celda con
		// cell-protect a sus celdas (lo define y no lo usa), así que ahí solo se comprueba la hoja.
		if st := m.Sheets[0].Styles["B1"]; c.ext == "xlsx" && (st.Locked == nil || *st.Locked) {
			t.Errorf("%s: LibreOffice perdió el desbloqueo de B1: %+v", c.ext, st)
		}
		t.Logf("%s tras LibreOffice: libro %s hoja %s", c.ext, m.Protect, m.Sheets[0].Protect)
	}
}
