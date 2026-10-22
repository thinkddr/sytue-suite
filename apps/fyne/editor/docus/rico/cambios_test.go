// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"regexp"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

var (
	ana  = Autor{Nombre: "Ana", ID: "u1"}
	luis = Autor{Nombre: "Luis", ID: "u2"}
)

// marcaAna y marcaLuis son los atributos de `trackAttrs` con el sello 5. «u1» cae en el
// quinto color de persona y «u2» en el sexto (117·31 + 49 = 3676, resto 4; +1 → 5).
const (
	marcaAna  = ` data-track="" data-author="Ana" data-author-id="u1" data-ts="5" style="--sytue-track:var(--sytue-avatar-5)"`
	marcaLuis = ` data-track="" data-author="Luis" data-author-id="u2" data-ts="5" style="--sytue-track:var(--sytue-avatar-6)"`
)

// TestLasMarcasDeLaWebSeEditan: lo que escribe la web con el modo encendido —la
// sentinela, `<ins>` y `<del>` con autor, sello y color— se entiende, vuelve igual byte a
// byte y se pinta como `editor.css`. Sin `data-track`, o con otro `style`, no se entiende.
func TestLasMarcasDeLaWebSeEditan(t *testing.T) {
	web := Sentinela + `<p>ho<ins data-track="" data-author="A &amp; &lt;B&gt;" data-author-id="u1" data-ts="1700000000000" ` +
		`style="--sytue-track:var(--sytue-avatar-3)">la</ins><del` + marcaLuis + `>x</del></p>`
	d := Parse(web)
	if !d.Editable() || !d.Guardable() || d.HTML() != web || !d.Rastreo() {
		t.Fatalf("la web no vuelve igual o no se edita (%v): %s", d.NoEntendido(), d.HTML())
	}
	tr := d.Bloques()[0].Tramos
	cielo, lavanda := tokens.Color("avatar-3", tokens.Light), tokens.Color("avatar-6", tokens.Light)
	if e := tr[1].Estilo; e.Color != cielo || !e.Subrayado || e.Fondo != conAlfa(cielo, 0.1) {
		t.Errorf("la inserción se pinta %+v: en el color de su autor, subrayada y con su fondo al 10 %%", e)
	}
	lavanda.A = 191
	if e := tr[2].Estilo; e.Color != lavanda || !e.Tachado || e.Subrayado {
		t.Errorf("el borrado se pinta %+v: tachado y al 75 %%", e)
	}
	if Parse(`<p>a</p>`).Rastreo() || Parse(`<p>a</p>`+Sentinela).Rastreo() {
		t.Error("la sentinela cuenta solo al frente")
	}
	for fuente, motivo := range map[string]string{
		`<p><ins>x</ins></p>`:                                   "<ins>",
		`<p><del data-x="1">x</del></p>`:                        "<del>",
		`<p><ins data-track="" style="color: red;">x</ins></p>`: "style: --sytue-track",
		`<p>a</p><!--x-->`:                                      "<!-- -->",
	} {
		if d := Parse(fuente); d.Editable() || !slices.Contains(d.NoEntendido(), motivo) {
			t.Errorf("%s: quería %q entre los motivos, hay %v", fuente, motivo, d.NoEntendido())
		}
	}
}

// TestTeclearConElModoEncendido es `trackInsert`: lo tecleado va en una inserción del
// autor, que crece mientras se sigue escribiendo, se FUNDE con la suya de al lado y no se
// mete en la de otro ni en un borrado. Se vio en ROJO quitando `fundirPistas`: escribir
// delante de la propia inserción dejaba dos `<ins>` seguidas.
func TestTeclearConElModoEncendido(t *testing.T) {
	d := Parse(`<p>hola</p>`)
	abrir(t, d, 0).InsertarRastreado(2, "X", ana, 5)
	despues(t, "teclear", d, `<p>ho<ins`+marcaAna+`>X</ins>la</p>`)
	abrir(t, d, 0).InsertarRastreado(3, "Y", ana, 9)
	despues(t, "seguir tecleando", d, `<p>ho<ins`+marcaAna+`>XY</ins>la</p>`)
	abrir(t, d, 0).InsertarRastreado(2, "W", ana, 5)
	despues(t, "delante de la suya se funde", d, `<p>ho<ins`+marcaAna+`>WXY</ins>la</p>`)
	abrir(t, d, 0).InsertarRastreado(3, "Z", luis, 5)
	despues(t, "otro autor, aparte", d, `<p>ho<ins`+marcaAna+`>W</ins><ins`+marcaLuis+`>Z</ins><ins`+marcaAna+`>XY</ins>la</p>`)

	d = Parse(`<p>a<del` + marcaLuis + `>b</del>c</p>`)
	abrir(t, d, 0).InsertarRastreado(2, "N", ana, 5)
	despues(t, "tras un borrado, fuera de él", d, `<p>a<del`+marcaLuis+`>b</del><ins`+marcaAna+`>N</ins>c</p>`)
	d = Parse(`<p><b>ne</b></p>`)
	abrir(t, d, 0).InsertarRastreado(1, "g", ana, 5)
	despues(t, "con el formato de al lado", d, `<p><b>n<ins`+marcaAna+`>g</ins>e</b></p>`)
}

// TestBorrarConElModoEncendido es `deleteLiveRange`: lo borrado se marca y no se quita,
// con su formato; el cursor salta lo ya borrado; lo propio sin aceptar se retira; y un
// salto de línea se quita, que no tiene texto que marcar.
func TestBorrarConElModoEncendido(t *testing.T) {
	d := Parse(`<p>hola</p>`)
	if p, ok := abrir(t, d, 0).BorrarRastreado(2, 3, ana, 5, true); !ok || p != 2 {
		t.Errorf("Retroceso deja el cursor delante de lo marcado: %d (%v)", p, ok)
	}
	despues(t, "Retroceso", d, `<p>ho<del`+marcaAna+`>l</del>a</p>`)
	if p, _ := abrir(t, d, 0).BorrarRastreado(1, 2, ana, 5, true); p != 1 {
		t.Errorf("el segundo Retroceso deja el cursor en 1: %d", p)
	}
	despues(t, "se funde con el de al lado", d, `<p>h<del`+marcaAna+`>ol</del>a</p>`)
	if p, ok := abrir(t, d, 0).BorrarRastreado(2, 3, ana, 5, true); ok || p != 1 {
		t.Errorf("sobre lo borrado solo salta al principio del borrado: %d (%v)", p, ok)
	}
	if p, ok := abrir(t, d, 0).BorrarRastreado(3, 4, ana, 5, false); !ok || p != 4 {
		t.Errorf("Suprimir deja el cursor detrás: %d (%v)", p, ok)
	}
	despues(t, "Suprimir", d, `<p>h<del`+marcaAna+`>ola</del></p>`)

	d = Parse(`<p>a<ins` + marcaAna + `>XY</ins>b</p>`)
	if p, _ := abrir(t, d, 0).BorrarRastreado(2, 3, ana, 5, true); p != 2 {
		t.Errorf("retirar lo propio deja el cursor donde estaba: %d", p)
	}
	despues(t, "lo propio se retira", d, `<p>a<ins`+marcaAna+`>X</ins>b</p>`)
	abrir(t, d, 0).BorrarRastreado(1, 2, luis, 5, true)
	despues(t, "lo de otro se marca dentro", d, `<p>a<ins`+marcaAna+`><del`+marcaLuis+`>X</del></ins>b</p>`)

	d = Parse(`<p>a<b>bc</b>d<br>e</p>`)
	abrir(t, d, 0).BorrarRastreado(1, 5, ana, 5, false)
	despues(t, "con su formato, y sin el salto", d, `<p>a<b><del`+marcaAna+`>bc</del></b><del`+marcaAna+`>d</del>e</p>`)
}

// TestAceptarYRechazarComoLaWeb: `listTrackedChanges`, `accept/rejectTrackedChanges` y
// `resolveTrackedChange` sobre el mismo documento. Solo cuentan las marcas de texto llano,
// como el `[^<]*` de `scanTrackMarks`; un `<ins>` sin `data-track` no es un cambio.
func TestAceptarYRechazarComoLaWeb(t *testing.T) {
	const doc = `<p>a<ins` + marcaAna + `>b</ins><del` + marcaLuis + `>c</del>` +
		`<ins data-track="" data-ts="x"><b>n</b></ins></p>`
	cs := Parse(doc).Cambios()
	if len(cs) != 2 || !cs[0].Insercion || cs[0].Autor != "Ana" || cs[0].Texto != "b" || cs[0].TS != 5 ||
		cs[1].Insercion || cs[1].Autor != "Luis" || cs[1].Texto != "c" {
		t.Fatalf("los cambios: %+v", cs)
	}
	for _, c := range []struct {
		aceptar bool
		quiero  string
	}{
		{true, `<p>ab<ins data-track="" data-ts="x"><b>n</b></ins></p>`},
		{false, `<p>ac<ins data-track="" data-ts="x"><b>n</b></ins></p>`},
	} {
		d := Parse(doc)
		if !d.Resolver(c.aceptar) || d.HTML() != c.quiero {
			t.Errorf("resolver todo (aceptar %v): %s", c.aceptar, d.HTML())
		}
		if d.Resolver(c.aceptar) {
			t.Error("resuelto todo, ya no queda nada que resolver")
		}
	}
	d := Parse(doc)
	if !d.ResolverCambio(1, true) || d.ResolverCambio(5, true) {
		t.Fatal("resolver uno dice si lo había")
	}
	despues(t, "aceptar un borrado", d, `<p>a<ins`+marcaAna+`>b</ins><ins data-track="" data-ts="x"><b>n</b></ins></p>`)
	if b := d.Bloques()[0]; len(d.Cambios()) != 1 {
		t.Errorf("queda un cambio: %+v", d.Cambios())
	} else if desde, hasta, ok := b.Donde(d.Cambios()[0]); !ok || desde != 1 || hasta != 2 {
		t.Errorf("el cambio está en [%d, %d) (%v) y es [1, 2)", desde, hasta, ok)
	}
	for _, c := range []struct {
		crudo  string
		quiero int64
	}{{"", 0}, {"abc", 0}, {"-3", 0}, {"1e3", 1000}, {"9e15", 0}, {" 42 ", 42}} {
		if got := selloDe(c.crudo); got != c.quiero {
			t.Errorf("selloDe(%q) = %d; `trackTime` da %d", c.crudo, got, c.quiero)
		}
	}
}

// El sello se escribe tal cual, en milisegundos: lo lee `trackTime` en la web.
func TestLaMarcaNuevaEsLaDeTrackAttrs(t *testing.T) {
	d := Parse(`<p>a</p>`)
	abrir(t, d, 0).InsertarRastreado(1, `"<&`, Autor{Nombre: `A "b"`, ID: `x"y`}, 1700000000123)
	if !regexp.MustCompile(`^<p>a<ins data-track="" data-author="A &quot;b&quot;" data-author-id="x&quot;y" ` +
		`data-ts="1700000000123" style="--sytue-track:var\(--sytue-avatar-[1-6]\)">"&lt;&amp;</ins></p>$`).MatchString(d.HTML()) {
		t.Errorf("la marca nueva: %s", d.HTML())
	}
	if !d.Guardable() {
		t.Errorf("la marca nueva no se puede guardar: %v", d.NoEntendido())
	}
}
