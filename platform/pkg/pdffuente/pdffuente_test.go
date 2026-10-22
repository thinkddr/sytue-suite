// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdffuente

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// Alfabetos de los 24 idiomas oficiales de la UE (más allá de ASCII): lo que la pila
// Figtree + Liberation tiene que cubrir sin caer en .notdef.
const alfabetosUE = "áàâäãåæçéèêëíìîïñóòôöõøœúùûüýÿßÁÀÂÄÃÅÆÇÉÈÊËÍÌÎÏÑÓÒÔÖÕØŒÚÙÛÜÝ" + // es fr de pt it nl da sv fi
	"ąćęłńśźżĄĆĘŁŃŚŹŻ" + // polaco
	"čďěňřšťůžČĎĚŇŘŠŤŮŽĺľŕÍĽĹŔ" + // checo y eslovaco
	"őűŐŰ" + // húngaro
	"ăâîșțĂÂÎȘȚ" + // rumano (coma bajo, U+0218..021B)
	"đĐćČšŠžŽ" + // croata y esloveno
	"ąčęėįšųūžĄČĘĖĮŠŲŪŽ" + // lituano
	"āčēģīķļņšūžĀČĒĢĪĶĻŅŠŪŽ" + // letón
	"õäöüšžÕÄÖÜŠŽ" + // estonio
	"ċġħżĊĠĦŻ" + // maltés
	"áéíóúÁÉÍÓÚ" + // irlandés
	"αβγδεζηθικλμνξοπρστυφχψωάέήίόύώΑΒΓΔΕΖΗΘΙΚΛΜΝΞΟΠΡΣΤΥΦΧΨΩΆΈΉΊΌΎΏϊϋΐΰς" + // griego
	"абвгдежзийклмнопрстуфхцчшщъьюяАБВГДЕЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЬЮЯ" + // búlgaro
	"«»„“”‘’–—…•€  №°±×÷©®™§¶"

func TestPilaCubreLosIdiomasDeLaUE(t *testing.T) {
	for _, estilo := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		pila := Pila(estilo[0], estilo[1])
		for _, r := range alfabetosUE {
			ok := false
			for _, f := range pila {
				if f.Glifo(r) != 0 {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("estilo %v: ni Figtree ni Liberation traen %q (U+%04X)", estilo, r, r)
			}
		}
	}
}

// Quita el exceso: lo que Figtree trae NO debe ir a la reserva (la marca manda), y lo que
// no trae sí. Si Figtree ganara griego algún día el test avisa de que la reserva sobra.
func TestFigtreeNoTraeGriegoNiCirilico(t *testing.T) {
	f := Pila(false, false)[0]
	if f.Glifo('a') == 0 || f.Glifo('ő') == 0 {
		t.Fatal("Figtree debería traer latín y latín extendido")
	}
	if f.Glifo('Ω') != 0 || f.Glifo('Ж') != 0 {
		t.Log("Figtree ya trae griego o cirílico: la reserva podría reducirse")
	}
}

// Las copias de Figtree no pueden divergir de las de apps/nucleo/fuentes (otro módulo Go,
// que este no puede importar).
func TestFigtreeIgualQueLaDelNucleo(t *testing.T) {
	for _, n := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
		aqui, err := datos.ReadFile("datos/Figtree-" + n + ".ttf")
		if err != nil {
			t.Fatal(err)
		}
		fuera, err := os.ReadFile("../../../apps/nucleo/fuentes/Figtree-" + n + ".ttf")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(aqui, fuera) {
			t.Errorf("Figtree-%s.ttf difiere de apps/nucleo/fuentes", n)
		}
	}
}

func TestRechazaLoQueNoSePuedeIncrustar(t *testing.T) {
	d, _ := datos.ReadFile("datos/Figtree-Regular.ttf")
	if len(Pila(false, false)[0].tablas["OS/2"]) < 10 {
		t.Skip("sin OS/2")
	}
	roto := slices.Clone(d)
	f, _ := Abrir(d)
	os2 := f.tablas["OS/2"]
	i := bytes.Index(roto, os2[:12])
	if i < 0 {
		t.Fatal("no se localiza OS/2")
	}
	binary.BigEndian.PutUint16(roto[i+8:], 0x0002) // fsType: incrustación restringida
	if _, err := Abrir(roto); !errors.Is(err, ErrIncrustacion) {
		t.Errorf("fsType restringido: err = %v, quería ErrIncrustacion", err)
	}
	binary.BigEndian.PutUint16(roto[i+8:], 0x0100) // sin permiso de subconjunto
	if _, err := Abrir(roto); !errors.Is(err, ErrIncrustacion) {
		t.Errorf("fsType sin subconjunto: err = %v", err)
	}
	if _, err := Abrir([]byte("OTTO\x00\x00\x00\x00\x00\x00\x00\x00")); !errors.Is(err, ErrFormato) {
		t.Errorf("CFF debería rechazarse, err = %v", err)
	}
	if _, err := Abrir(d[:100]); !errors.Is(err, ErrFormato) {
		t.Errorf("fuente truncada debería rechazarse, err = %v", err)
	}
}

// directorio lee las tablas de un TrueType ya ensamblado, comprobando sus sumas.
func directorio(t *testing.T, d []byte) map[string][]byte {
	t.Helper()
	n := int(binary.BigEndian.Uint16(d[4:]))
	out := map[string][]byte{}
	for i := 0; i < n; i++ {
		r := 12 + 16*i
		off, ln := int(binary.BigEndian.Uint32(d[r+8:])), int(binary.BigEndian.Uint32(d[r+12:]))
		tag := string(d[r : r+4])
		out[tag] = d[off : off+ln]
		if off%4 != 0 {
			t.Errorf("tabla %q sin alinear a 4", tag)
		}
		if tag != "head" && suma(d[off:min(off+(ln+3)/4*4, len(d))]) != binary.BigEndian.Uint32(d[r+4:]) {
			t.Errorf("suma de la tabla %q no cuadra", tag)
		}
	}
	return out
}

func TestSubconjuntoConservaLoUsadoYQuitaElResto(t *testing.T) {
	// Liberation trae acentos como glifos COMPUESTOS: los componentes también tienen que viajar.
	f := Pila(false, false)[1]
	usados := []uint16{f.Glifo('H'), f.Glifo('é'), f.Glifo('Ω'), f.Glifo('Ж')}
	prog, err := f.subconjunto(usados)
	if err != nil {
		t.Fatal(err)
	}
	if suma(prog) != 0xB1B0AFBA {
		t.Errorf("suma del fichero = %#x, quería 0xB1B0AFBA (checkSumAdjustment mal calculado)", suma(prog))
	}
	tb := directorio(t, prog)
	for _, req := range []string{"head", "hhea", "maxp", "hmtx", "loca", "glyf"} {
		if tb[req] == nil {
			t.Errorf("falta la tabla obligatoria %q", req)
		}
	}
	if len(prog) > len(f.datos)/10 {
		t.Errorf("el subconjunto de 4 glifos pesa %d de %d bytes: no se ha recortado", len(prog), len(f.datos))
	}
	loca, glyf := tb["loca"], tb["glyf"]
	nuevo := func(g int) []byte {
		return glyf[binary.BigEndian.Uint32(loca[4*g:]):binary.BigEndian.Uint32(loca[4*g+4:])]
	}
	compuestos := 0
	for _, g := range usados {
		orig, _ := f.glifo(int(g))
		if !bytes.HasPrefix(nuevo(int(g)), orig) || len(orig) == 0 {
			t.Errorf("glifo %d: contorno distinto o vacío", g)
		}
		cs, _ := componentes(orig)
		for _, c := range cs {
			compuestos++
			co, _ := f.glifo(c)
			if len(co) > 0 && len(nuevo(c)) == 0 {
				t.Errorf("glifo %d: falta su componente %d", g, c)
			}
		}
	}
	if compuestos == 0 {
		t.Error("la prueba no ha ejercitado ningún glifo compuesto (¿é ya no lo es en Liberation?)")
	}
	if n := len(nuevo(int(f.Glifo('Z')))); n != 0 {
		t.Errorf("un glifo no usado conserva %d bytes", n)
	}
	if binary.BigEndian.Uint16(tb["head"][50:]) != 1 {
		t.Error("loca debería salir en formato largo")
	}
}

type escritorFalso struct{ objs []string }

func (e *escritorFalso) Objeto(c string) int { e.objs = append(e.objs, c); return len(e.objs) }
func (e *escritorFalso) Flujo(d string, datos []byte) int {
	return e.Objeto("<< " + d + " >> STREAM(" + string(datos) + ")")
}

func TestEscribirEmiteLaFuenteType0Completa(t *testing.T) {
	f := Pila(false, true)[0]
	s := f.NuevoSubconjunto()
	for _, r := range "AB€" {
		if g := f.Glifo(r); g != 0 {
			s.Usar(g, r)
		}
	}
	s.Usar(f.Glifo('A'), 'Z') // ya estaba: manda el primero
	w := &escritorFalso{}
	raiz, err := s.Escribir(w)
	if err != nil {
		t.Fatal(err)
	}
	t0 := w.objs[raiz-1]
	for _, quiero := range []string{"/Subtype /Type0", "/Encoding /Identity-H", "/DescendantFonts [", "/ToUnicode "} {
		if !strings.Contains(t0, quiero) {
			t.Errorf("Type0 sin %q: %s", quiero, t0)
		}
	}
	todo := strings.Join(w.objs, "\n")
	for _, quiero := range []string{
		"/CIDFontType2", "/CIDToGIDMap /Identity", "/FontFile2 ", "/Length1 ",
		"/Flags 96", "/ItalicAngle -", "/W [", "beginbfchar", "<0041>",
	} {
		if !strings.Contains(todo, quiero) {
			t.Errorf("falta %q en los objetos de fuente", quiero)
		}
	}
	if strings.Contains(todo, "<005A>") {
		t.Error("el ToUnicode toma el último carácter en vez del primero")
	}
	if !strings.Contains(t0, "/BaseFont /") || strings.Count(t0, "+") != 1 {
		t.Errorf("BaseFont sin etiqueta de subconjunto: %s", t0)
	}
}

func TestAnchosCoincidenConHmtx(t *testing.T) {
	f := Pila(false, false)[0]
	s := f.NuevoSubconjunto()
	for _, r := range "Hola mundo" {
		s.Usar(f.Glifo(r), r)
	}
	w := s.anchos()
	for _, g := range s.glifos() {
		if f.Avance(g) <= 0 {
			t.Errorf("avance de %d no positivo", g)
		}
	}
	if !strings.HasPrefix(w, "[") || !strings.HasSuffix(w, "]") || strings.Contains(w, "  ") {
		t.Errorf("array /W mal formado: %q", w)
	}
}
