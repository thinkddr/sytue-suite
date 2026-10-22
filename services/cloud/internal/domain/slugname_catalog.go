// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

// ---------------------------------------------------------------------------
// Catálogos de palabras del sugeridor de nombres, uno por idioma de producto
// ---------------------------------------------------------------------------
//
// Las palabras se eligieron YA SIN ACENTOS, no transliteradas: «bahia» por
// «bahía», «gruen» por «grün» o «foret» por «forêt» son nombres mutilados, y un
// slug es una etiqueta DNS (ASCII en minúsculas). Cuando una palabra bonita no
// cabía en ASCII se cogió otra, no se le quitaron las tildes. El test
// TestElCatalogoNoTieneAcentos vigila esta regla en los cinco idiomas.
//
// Campo semántico común a las cinco lenguas: geografía y paisaje. Neutro, sobrio
// y sin connotación de producto, que es lo que hace que sirva para cualquier
// proyecto y no envejezca.

// slugAdjPair construye un adjetivo de dos formas para las lenguas SIN neutro
// (español, francés, italiano): la casilla neutra repite el masculino porque en
// esas lenguas no existe y nunca se consultará.
func slugAdjPair(m, f string) slugAdjective { return slugAdjective{m, f, m} }

// slugAdjInvariable declara adjetivos que no concuerdan: todos los del inglés y
// los acabados en -e del español/italiano/francés («firme», «verde», «noble»).
// La invariabilidad no se declara aparte: se DEDUCE de que las tres formas sean
// iguales (ver slugCatalog.invariableAdjectives), así que no puede desincronizarse.
func slugAdjInvariable(words ...string) []slugAdjective {
	out := make([]slugAdjective, len(words))
	for i, w := range words {
		out[i] = slugAdjective{w, w, w}
	}
	return out
}

// slugAdjGerman declina el adjetivo antepuesto SIN artículo, que en alemán exige
// declinación fuerte en nominativo: -er masculino, -e femenino, -es neutro
// («ruhiger hafen», «ruhige insel», «ruhiges tal»). Se guarda la raíz y no las
// tres formas porque así la regla vive en el código —donde se puede leer y
// testear— y las irregularidades se resuelven en la raíz: «hoch» se guarda como
// «hoh» (hoher/hohe/hohes) y «dunkel» como «dunkl» (dunkler/dunkle/dunkles).
func slugAdjGerman(stems ...string) []slugAdjective {
	out := make([]slugAdjective, len(stems))
	for i, s := range stems {
		out[i] = slugAdjective{s + "er", s + "e", s + "es"}
	}
	return out
}

// slugNounsOf etiqueta una lista de sustantivos con un mismo género.
func slugNounsOf(g slugGender, words ...string) []slugNoun {
	out := make([]slugNoun, len(words))
	for i, w := range words {
		out[i] = slugNoun{w, g}
	}
	return out
}

func slugNounList(groups ...[]slugNoun) []slugNoun {
	var out []slugNoun
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func slugAdjList(groups ...[]slugAdjective) []slugAdjective {
	var out []slugAdjective
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// --- Inglés (BASE) ---------------------------------------------------------
//
// Sin género y con el adjetivo DELANTE: «serene-harbor». No hay concordancia que
// respetar, así que las tres casillas de cada adjetivo son la misma palabra.

var slugCatalogEN = slugCatalog{
	adjectiveFirst: true,
	nouns: slugNounsOf(slugGenderMasculine,
		"harbor", "lighthouse", "ridge", "oak", "cape", "valley", "lake", "river",
		"trail", "pier", "forest", "mill", "pine", "wind", "field", "olive",
		"reef", "cliff", "meadow", "reed", "dune", "hillside", "cove", "marsh",
		"slope", "tide", "sand", "summit", "mist", "breeze", "lagoon", "hill",
		"holly", "hamlet", "path", "shore", "plateau", "spring", "branch", "creek",
	),
	adjectives: slugAdjInvariable(
		"serene", "clear", "sober", "tall", "quiet", "slow", "bright", "frank",
		"clean", "wise", "new", "deep", "wide", "long", "calm", "firm",
		"green", "strong", "light", "grand", "humble", "wild", "azure", "brief",
		"noble",
	),
}

// --- Español ---------------------------------------------------------------
//
// Adjetivo DETRÁS y concordado: «puerto-sereno», «duna-serena».

var slugCatalogES = slugCatalog{
	nouns: slugNounList(
		slugNounsOf(slugGenderMasculine,
			"puerto", "faro", "monte", "roble", "cabo", "valle", "lago", "cauce",
			"sendero", "muelle", "bosque", "molino", "pinar", "viento", "campo", "olivo",
			"arrecife", "risco", "prado", "junco"),
		slugNounsOf(slugGenderFeminine,
			"duna", "sierra", "cala", "ribera", "ladera", "marea", "arena", "cumbre",
			"niebla", "brisa", "laguna", "colina", "encina", "veleta", "aldea", "senda",
			"orilla", "meseta", "fuente", "rama"),
	),
	adjectives: slugAdjList(
		[]slugAdjective{
			slugAdjPair("sereno", "serena"), slugAdjPair("claro", "clara"),
			slugAdjPair("sobrio", "sobria"), slugAdjPair("alto", "alta"),
			slugAdjPair("quieto", "quieta"), slugAdjPair("lento", "lenta"),
			slugAdjPair("vivo", "viva"), slugAdjPair("franco", "franca"),
			slugAdjPair("limpio", "limpia"), slugAdjPair("sabio", "sabia"),
			slugAdjPair("nuevo", "nueva"), slugAdjPair("hondo", "honda"),
			slugAdjPair("ancho", "ancha"), slugAdjPair("largo", "larga"),
			slugAdjPair("tranquilo", "tranquila"),
		},
		slugAdjInvariable("firme", "verde", "fuerte", "leve", "grande",
			"humilde", "agreste", "celeste", "breve", "noble"),
	),
}

// --- Francés ---------------------------------------------------------------
//
// Como el español: adjetivo DETRÁS y concordado, «port-serein», «dune-sereine».
// Los femeninos irregulares van escritos, no derivados: «vif/vive»,
// «franc/franche», «neuf/neuve», «doux/douce» no salen de añadir una -e.

var slugCatalogFR = slugCatalog{
	nouns: slugNounList(
		slugNounsOf(slugGenderMasculine,
			"port", "phare", "mont", "cap", "val", "lac", "ruisseau", "sentier",
			"quai", "bois", "moulin", "pin", "vent", "champ", "olivier", "rocher",
			"jonc", "roseau", "coteau", "plateau", "rivage", "hameau", "verger", "gouffre"),
		slugNounsOf(slugGenderFeminine,
			"dune", "colline", "crique", "rive", "pente", "brume", "brise", "lagune",
			"source", "branche", "plaine", "prairie", "lande", "falaise", "baie", "combe",
			"garrigue", "berge", "roche", "vague", "cime", "aube", "digue", "futaie"),
	),
	// OJO con el francés: aquí el adjetivo va detrás, pero un grupo pequeño y muy
	// frecuente lo hace AL REVÉS y va delante del sustantivo —los de belleza, edad,
	// número, bondad y tamaño, que se enseñan con la regla mnemotécnica BANGS: grand,
	// petit, haut, beau, joli, jeune, vieux, bon, nouveau…—. «un grand pin» es lo
	// natural; «pin grand» suena a extranjero. Por eso NINGUNO de esos entra en esta
	// lista: como el orden del catálogo es único por lengua, admitirlos obligaría a
	// una excepción por adjetivo, y el francés tiene de sobra con los pospuestos.
	adjectives: slugAdjList(
		[]slugAdjective{
			slugAdjPair("serein", "sereine"), slugAdjPair("clair", "claire"),
			slugAdjPair("lent", "lente"), slugAdjPair("vif", "vive"),
			slugAdjPair("franc", "franche"), slugAdjPair("neuf", "neuve"),
			slugAdjPair("profond", "profonde"), slugAdjPair("long", "longue"),
			slugAdjPair("vert", "verte"), slugAdjPair("fort", "forte"),
			slugAdjPair("doux", "douce"), slugAdjPair("fin", "fine"),
			slugAdjPair("net", "nette"), slugAdjPair("pur", "pure"),
			slugAdjPair("droit", "droite"), slugAdjPair("rond", "ronde"),
			slugAdjPair("discret", "discrete"), slugAdjPair("epais", "epaisse"),
		},
		slugAdjInvariable("sobre", "calme", "propre", "sage", "large",
			"tranquille", "ferme", "humble", "vaste", "libre", "agreste", "noble"),
	),
}

// --- Alemán ----------------------------------------------------------------
//
// El caso que obliga a modelar géneros de verdad: TRES (der/die/das), el adjetivo
// va DELANTE y, sin artículo, se declina en nominativo fuerte. «ruhig hafen» no
// es alemán; «ruhiger hafen» sí.

var slugCatalogDE = slugCatalog{
	adjectiveFirst: true,
	nouns: slugNounList(
		slugNounsOf(slugGenderMasculine, // der
			"hafen", "berg", "wald", "fluss", "pfad", "steg", "hain", "turm",
			"bach", "felsen", "garten", "strand", "damm", "anker", "gipfel"),
		slugNounsOf(slugGenderFeminine, // die
			"insel", "bucht", "quelle", "wiese", "klippe", "aue", "weide", "brise",
			"furt", "heide", "mole", "schlucht", "lichtung", "ebene", "woge", "birke", "eiche"),
		slugNounsOf(slugGenderNeuter, // das
			"tal", "moor", "feld", "riff", "ufer", "kap", "meer", "dorf",
			"wehr", "delta", "watt", "eiland", "segel", "licht"),
	),
	adjectives: slugAdjGerman(
		"ruhig", "klar", "still", "hell", "sanft", "weit", "tief", "schlicht",
		"stark", "leicht", "frisch", "mild", "rein", "fest", "jung", "wild",
		"lang", "breit", "flach", "warm", "hoh", "grau", "blau", "golden",
		"offen",
	),
}

// --- Italiano --------------------------------------------------------------
//
// Adjetivo DETRÁS y concordado: «porto-sereno», «duna-serena».

var slugCatalogIT = slugCatalog{
	nouns: slugNounList(
		slugNounsOf(slugGenderMasculine,
			"porto", "faro", "monte", "colle", "capo", "lago", "fiume", "sentiero",
			"molo", "bosco", "mulino", "pino", "vento", "campo", "ulivo", "scoglio",
			"prato", "giunco", "borgo", "torrente", "ruscello", "poggio", "canneto", "pioppo"),
		slugNounsOf(slugGenderFeminine,
			"duna", "cala", "riva", "costa", "marea", "sabbia", "cima", "nebbia",
			"brezza", "laguna", "collina", "quercia", "landa", "sponda", "radura", "pineta",
			"valle", "fonte", "isola", "baia", "conca", "rupe", "vetta", "piana"),
	),
	adjectives: slugAdjList(
		[]slugAdjective{
			slugAdjPair("sereno", "serena"), slugAdjPair("chiaro", "chiara"),
			slugAdjPair("sobrio", "sobria"), slugAdjPair("alto", "alta"),
			slugAdjPair("quieto", "quieta"), slugAdjPair("lento", "lenta"),
			slugAdjPair("vivo", "viva"), slugAdjPair("franco", "franca"),
			slugAdjPair("limpido", "limpida"), slugAdjPair("saggio", "saggia"),
			slugAdjPair("nuovo", "nuova"), slugAdjPair("profondo", "profonda"),
			slugAdjPair("largo", "larga"), slugAdjPair("lungo", "lunga"),
			slugAdjPair("tranquillo", "tranquilla"), slugAdjPair("fermo", "ferma"),
		},
		slugAdjInvariable("verde", "forte", "lieve", "grande", "umile",
			"agreste", "celeste", "breve", "nobile"),
	),
}
