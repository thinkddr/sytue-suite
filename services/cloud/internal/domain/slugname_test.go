// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"slices"
	"strings"
	"testing"
)

// Lo primero que tiene que cumplir un nombre sugerido es SERVIR: si no pasa la
// validación de slug, la sugerencia le regala al usuario un 400 en vez de ahorrarle
// un 409. Se cubren los cinco idiomas, los dos formatos (con y sin sufijo) y muchas
// tiradas.
func TestTodaSugerenciaEsUnSlugValido(t *testing.T) {
	for _, lang := range SlugLanguages {
		for i := range 300 {
			for _, attempt := range []int{0, 1} {
				s := SuggestSlug(lang, attempt)
				if !ValidSlug(s) {
					t.Fatalf("%s tirada %d (intento %d): %q no es un slug válido", lang, i, attempt, s)
				}
				if len(s) > 40 {
					t.Fatalf("%s: %q mide %d: no cabe en una etiqueta DNS", lang, s, len(s))
				}
			}
		}
	}
}

// Un idioma que no hablamos no puede reventar ni devolver vacío: cae al base.
func TestUnIdiomaDesconocidoCaeAlBase(t *testing.T) {
	s := SuggestSlug("pt", 0)
	if !ValidSlug(s) {
		t.Fatalf("%q: un idioma desconocido debe caer al base, no romper", s)
	}
	if _, _, ok := descomponer(catalogFor(BaseLanguage), s); !ok {
		t.Errorf("%q no sale del catálogo base", s)
	}
}

// El detalle que separa un nombre escrito de uno generado: la gramática. Este test
// comprueba de una vez el ORDEN (el sustantivo tiene que aparecer donde le toca a
// esa lengua) y la CONCORDANCIA (el adjetivo, en la forma del género del
// sustantivo). «duna sereno» o «ruhig hafen» caen aquí.
func TestLaGramaticaDeCadaIdioma(t *testing.T) {
	for _, lang := range SlugLanguages {
		c := catalogFor(lang)
		for range 400 {
			s := SuggestSlug(lang, 0)
			noun, adj, ok := descomponer(c, s)
			if !ok {
				t.Fatalf("%s: %q no tiene la forma que pide la lengua "+
					"(adjetivo %s)", lang, s, posicion(c.adjectiveFirst))
			}
			if !formaValida(c, noun.gender, adj) {
				t.Errorf("%s: %q lleva el adjetivo en un género que no es el del sustantivo", lang, s)
			}
		}
	}
}

// El alemán es el caso que obliga a modelar tres géneros: el adjetivo antepuesto
// sin artículo se declina en nominativo fuerte. Ejemplos clavados a propósito: si
// alguien «simplifica» la declinación, esto lo dice con nombre y apellidos.
func TestElAlemanDeclinaElAdjetivoAntepuesto(t *testing.T) {
	got := slugAdjGerman("ruhig")[0]
	if want := (slugAdjective{"ruhiger", "ruhige", "ruhiges"}); got != want {
		t.Fatalf("declinación fuerte de «ruhig» = %v, se esperaba %v", got, want)
	}
	c := catalogFor("de")
	casos := []struct {
		noun slugNoun
		want string
	}{
		{slugNoun{"hafen", slugGenderMasculine}, "ruhiger-hafen"},
		{slugNoun{"insel", slugGenderFeminine}, "ruhige-insel"},
		{slugNoun{"tal", slugGenderNeuter}, "ruhiges-tal"},
	}
	for _, caso := range casos {
		if got := c.compose(caso.noun, got); got != caso.want {
			t.Errorf("compose(%q) = %q, se esperaba %q", caso.noun.word, got, caso.want)
		}
	}
}

// El inglés no concuerda y va delante: «serene-harbor», nunca «harbor-serene» ni
// una forma femenina inexistente.
func TestElInglesAnteponeYNoConcuerda(t *testing.T) {
	c := catalogFor("en")
	for _, a := range c.adjectives {
		if a[0] != a[1] || a[1] != a[2] {
			t.Fatalf("adjetivo inglés con formas por género: %v", a)
		}
	}
	if got := c.compose(slugNoun{"harbor", slugGenderMasculine}, slugAdjective{"serene", "serene", "serene"}); got != "serene-harbor" {
		t.Errorf("compose = %q, se esperaba serene-harbor", got)
	}
}

// Sin tildes, eñes, diéresis ni ß POR CONSTRUCCIÓN. Si algún día se añade «bahía»
// o «grün» al catálogo, este test lo caza antes de que salga un «bahia» o un
// «gruen» mutilados —o un slug inválido—: la regla es elegir otra palabra.
func TestElCatalogoNoTieneAcentos(t *testing.T) {
	for _, lang := range SlugLanguages {
		c := catalogFor(lang)
		comprobar := func(palabra string) {
			t.Helper()
			if !ValidSlug(palabra) {
				t.Errorf("%s: %q no es ASCII apto para slug: elige otra palabra, no la transliteres",
					lang, palabra)
			}
		}
		for _, n := range c.nouns {
			comprobar(n.word)
		}
		for _, a := range c.adjectives {
			for _, forma := range a {
				comprobar(forma)
			}
		}
	}
}

func TestNoHayPalabrasRepetidas(t *testing.T) {
	for _, lang := range SlugLanguages {
		c := catalogFor(lang)
		visto := map[string]bool{}
		for _, n := range c.nouns {
			if visto[n.word] {
				t.Errorf("%s: sustantivo duplicado %q: sesga las sugerencias hacia él", lang, n.word)
			}
			visto[n.word] = true
		}
		// Sustantivos y adjetivos tienen que ser conjuntos DISJUNTOS: si una palabra
		// estuviera en los dos, «monte-claro» sería ambiguo y el test de gramática
		// dejaría de poder distinguir el orden.
		for _, a := range c.adjectives {
			if visto[a[0]] {
				t.Errorf("%s: %q está en las dos listas o repetido entre adjetivos", lang, a[0])
			}
			visto[a[0]] = true
		}
	}
}

// A partir del segundo intento hay sufijo: sin él, con el catálogo agotado no
// habría forma de proponer un nombre libre y el usuario volvería al 409.
func TestElReintentoAnadeSufijo(t *testing.T) {
	for _, lang := range SlugLanguages {
		for range 50 {
			s := SuggestSlug(lang, 1)
			if partes := strings.Split(s, "-"); len(partes) != 3 {
				t.Fatalf("%s: %q: el reintento debe llevar sufijo numérico", lang, s)
			}
		}
		if strings.Count(SuggestSlug(lang, 0), "-") != 1 {
			t.Errorf("%s: el primer intento debe ser el nombre limpio, sin número", lang)
		}
	}
}

// El catálogo tiene que dar variedad suficiente para que no se repita el mismo
// nombre a la primera en cuanto haya unos pocos proyectos.
func TestHayVariedadSuficiente(t *testing.T) {
	for _, lang := range SlugLanguages {
		c := catalogFor(lang)
		if combinaciones := len(c.nouns) * len(c.adjectives); combinaciones < 500 {
			t.Errorf("%s: solo %d combinaciones: pocas para un slug único en toda la plataforma",
				lang, combinaciones)
		}
	}
}

// ---------------------------------------------------------------------------
// Negociación de idioma
// ---------------------------------------------------------------------------

func TestNegotiateLanguage(t *testing.T) {
	casos := []struct {
		nombre   string
		explicit string
		header   string
		want     string
	}{
		{"sin nada, el base", "", "", "en"},
		{"parámetro explícito", "it", "de", "it"},
		{"explícito con región", "fr-CA", "", "fr"},
		{"explícito que no hablamos: pasa a la cabecera", "pt", "de", "de"},
		{"explícito basura: pasa a la cabecera", "!!", "es", "es"},
		{"cabecera simple", "", "es", "es"},
		{"cabecera con región", "", "de-AT", "de"},
		{"la q ausente vale 1, no 0", "", "de,it;q=0.9", "de"},
		{"se ordena por q, no por aparición", "", "de;q=0.4,it;q=0.9", "it"},
		{"pt-BR no está: se pasa a la siguiente", "", "pt-BR,pt;q=0.9,en;q=0.5", "en"},
		{"ninguna candidata sirve", "", "pt-BR,ja,zh-CN", "en"},
		{"q=0 es un rechazo", "", "es;q=0,fr;q=0.3", "fr"},
		{"comodín", "", "*", "en"},
		{"comodín tras un idioma que no hablamos", "", "ja,*;q=0.5", "en"},
		{"espacios y mayúsculas", "", "  FR ; q=0.4 , DE ; q=0.9 ", "de"},
		{"q ilegible no puede ganar", "", "es;q=zz,de", "de"},
		{"empate: gana el primero", "", "it;q=0.8,fr;q=0.8", "it"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if got := NegotiateLanguage(caso.explicit, caso.header); got != caso.want {
				t.Errorf("NegotiateLanguage(%q, %q) = %q, se esperaba %q",
					caso.explicit, caso.header, got, caso.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Alternativas para un nombre ocupado
// ---------------------------------------------------------------------------

// Las alternativas salen del nombre que ELIGIÓ el usuario: si fueran aleatorias,
// serían la sugerencia de siempre con otro nombre y no responderían a lo que pidió.
func TestLasAlternativasSalenDelNombreDelUsuario(t *testing.T) {
	alts := SlugAlternatives("mi-tienda", "es", 6)
	if len(alts) != 6 {
		t.Fatalf("se pidieron 6 alternativas y llegaron %d: %v", len(alts), alts)
	}
	derivadas := 0
	for _, a := range alts {
		if strings.HasPrefix(a, "mi-tienda-") {
			derivadas++
		}
	}
	if derivadas < 3 {
		t.Errorf("solo %d de %v derivan del nombre pedido", derivadas, alts)
	}
	if !slices.Contains(alts, "mi-tienda-2") {
		t.Errorf("falta el sufijo numérico, que es la variante que conserva el nombre: %v", alts)
	}
}

// Toda alternativa tiene que ser usable tal cual: slug válido, distinta de la
// pedida y sin repetirse.
func TestLasAlternativasSonSlugsValidosYDistintos(t *testing.T) {
	for _, lang := range SlugLanguages {
		for _, base := range []string{"tienda", "mi-tienda", "app", "un-nombre-bastante-largo-de-proyecto"} {
			alts := SlugAlternatives(base, lang, 8)
			if len(alts) == 0 {
				t.Fatalf("%s/%s: sin alternativas; siempre debe quedar la red del catálogo", lang, base)
			}
			visto := map[string]bool{}
			for _, a := range alts {
				if !ValidSlug(a) {
					t.Errorf("%s/%s: %q no es un slug válido", lang, base, a)
				}
				if a == base {
					t.Errorf("%s/%s: se propone el mismo nombre que ya está ocupado", lang, base)
				}
				if visto[a] {
					t.Errorf("%s/%s: alternativa repetida %q", lang, base, a)
				}
				visto[a] = true
			}
		}
	}
}

// El detalle de calidad: del sustantivo del usuario no sabemos el género, así que
// las variantes con adjetivo solo pueden usar adjetivos INVARIABLES. «mi-tienda-
// sereno» queda fatal, y es exactamente lo que este test impide.
func TestLasVariantesConAdjetivoSonInvariables(t *testing.T) {
	for _, lang := range []string{"es", "fr", "it"} {
		c := catalogFor(lang)
		invariables := c.invariableAdjectives()
		if len(invariables) == 0 {
			t.Fatalf("%s: sin adjetivos invariables no habría variantes decentes", lang)
		}
		conGenero := map[string]bool{}
		for _, a := range c.adjectives {
			if a[0] != a[1] {
				conGenero[a[0]] = true
				conGenero[a[1]] = true
			}
		}
		for range 30 {
			for _, alt := range SlugAlternatives("mi-tienda", lang, 8) {
				sufijo := strings.TrimPrefix(alt, "mi-tienda-")
				if sufijo == alt {
					continue // nombre del catálogo, no una variante
				}
				if conGenero[sufijo] {
					t.Fatalf("%s: %q usa un adjetivo con género sobre un sustantivo "+
						"cuyo género no conocemos", lang, alt)
				}
			}
		}
	}
}

// En alemán el adjetivo antepuesto SIEMPRE se declina, así que sin conocer el
// género del sustantivo del usuario no hay forma correcta: no se ofrece ninguna
// variante con adjetivo, solo sufijo numérico y nombres del catálogo. Es preferible
// una alternativa menos a una que esté mal escrita.
func TestEnAlemanNoSeInventanAdjetivosSobreElNombreDelUsuario(t *testing.T) {
	if adjs := catalogFor("de").invariableAdjectives(); len(adjs) != 0 {
		t.Fatalf("el alemán no tiene adjetivos invariables antepuestos: %v", adjs)
	}
	for range 30 {
		for _, alt := range SlugAlternatives("kundenportal", "de", 8) {
			if !strings.Contains(alt, "kundenportal") {
				continue // nombre del catálogo
			}
			if resto := strings.TrimPrefix(alt, "kundenportal-"); strings.Trim(resto, "0123456789") != "" {
				t.Errorf("%q: en alemán solo cabe el sufijo numérico sobre el nombre del usuario", alt)
			}
		}
	}
}

// «mi-tienda-2» ocupado no puede llevar a «mi-tienda-2-2»: el sufijo numérico
// previo se recorta antes de añadir el nuevo.
func TestElSufijoNumericoNoSeApila(t *testing.T) {
	alts := SlugAlternatives("mi-tienda-2", "es", 6)
	if !slices.Contains(alts, "mi-tienda-3") {
		t.Errorf("se esperaba continuar la serie con mi-tienda-3: %v", alts)
	}
	for _, a := range alts {
		if strings.Contains(a, "-2-") {
			t.Errorf("%q apila sufijos numéricos", a)
		}
	}
	if got := trimNumericSuffix("2"); got != "2" {
		t.Errorf("trimNumericSuffix(%q) = %q: no puede dejar el nombre vacío", "2", got)
	}
}

// ---------------------------------------------------------------------------
// Utilidades del test
// ---------------------------------------------------------------------------

// descomponer parte un nombre generado en sustantivo y adjetivo SEGÚN EL ORDEN de
// la lengua. Si el sustantivo no aparece donde toca, devuelve ok=false: es la forma
// de comprobar el orden sin escribir dos tests distintos.
func descomponer(c *slugCatalog, s string) (slugNoun, string, bool) {
	partes := strings.Split(s, "-")
	if len(partes) != 2 {
		return slugNoun{}, "", false
	}
	noun, adj := partes[0], partes[1]
	if c.adjectiveFirst {
		noun, adj = partes[1], partes[0]
	}
	for _, n := range c.nouns {
		if n.word == noun {
			return n, adj, true
		}
	}
	return slugNoun{}, "", false
}

// formaValida dice si `forma` es una forma de algún adjetivo del catálogo PARA ESE
// GÉNERO.
func formaValida(c *slugCatalog, g slugGender, forma string) bool {
	for _, a := range c.adjectives {
		if a[g] == forma {
			return true
		}
	}
	return false
}

func posicion(first bool) string {
	if first {
		return "delante"
	}
	return "detrás"
}
