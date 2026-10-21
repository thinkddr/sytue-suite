// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

// VersionIR es la versión del esquema que emite y acepta este paquete (Migrar sube las anteriores).
const VersionIR = 1

// Constantes del IR. Espejo EXACTO de las `export const … as const` de tipos.ts: lo vigila
// TestEspejoDeConstantes.
var (
	// Idiomas en que puede estar una web; más que los de la interfaz (añade ca, eu y gl).
	Idiomas = []string{
		"bg", "ca", "cs", "da", "de", "el", "en", "es", "et", "eu", "fi", "fr", "ga", "gl", "hr",
		"hu", "is", "it", "lt", "lv", "mt", "nb", "nl", "pl", "pt", "ro", "sk", "sl", "sv",
	}
	// IdiomasVisitante son los que tienen textos de sistema para el visitante: solo esos se
	// pueden poner en languages.
	IdiomasVisitante = []string{"en", "es"}
	// TiposNodo son los valores de Nodo.Type.
	TiposNodo = []string{
		"section", "frame", "text", "image", "button", "icon", "embed", "form", "table",
		"product_grid", "cart_button", "language_switcher", "menu", "instance",
	}
	// TiposSeccion son los valores de kind en una sección.
	TiposSeccion = []string{
		"header", "footer", "hero", "features", "gallery", "text", "cta", "contact", "map",
		"testimonials", "faq", "pricing", "team", "products", "logos", "custom",
	}
	// Iconos es el juego propio de iconos de las webs.
	Iconos = []string{
		"phone", "mail", "map-pin", "clock", "calendar", "check", "star", "heart", "cart", "bag",
		"truck", "gift", "tag", "user", "users", "building", "home", "globe", "shield", "leaf", "wrench", "hammer",
		"stethoscope", "bed", "utensils", "graduation", "briefcase", "camera", "image", "play", "quote", "chat",
		"arrow-right", "arrow-left", "chevron-down", "menu", "close", "plus", "minus", "external", "download", "info",
		"whatsapp", "instagram", "facebook", "linkedin", "tiktok", "x", "youtube",
	}
	// ProveedoresIncrustado son los únicos incrustados admitidos.
	ProveedoresIncrustado = []string{"osm", "cast", "youtube", "vimeo"}
	// ColoresObligatorios son los tokens que todo tema define.
	ColoresObligatorios = []string{"background", "text", "primary", "on_primary", "muted", "border"}
	// EstilosTexto son las claves de theme.text_styles y de text.preset.
	EstilosTexto = []string{"h1", "h2", "h3", "h4", "h5", "h6", "body", "small", "label", "button"}
	// SlugsReservados son las rutas del sistema en las webs (A13).
	SlugsReservados = []string{"p", "cart", "order", "legal", "report"}
	// AnclasReservadas no las puede usar una sección.
	AnclasReservadas = []string{"contenido"}
)

// Limites del IR. Espejo del objeto LIMITES de tipos.ts (se comparan sin distinguir mayúsculas).
var Limites = struct {
	IRBytes, Paginas, SeccionesPorPagina, NodosPorPagina, Profundidad                  int
	Componentes, NodosPorComponente, CaracteresPorTexto, ColoresTema, CamposFormulario int
	ElementosMenu, HijosPorElementoMenu, ProductosEnRejilla, FilasTabla, ColumnasTabla int
	Slug, URL                                                                          int
}{
	IRBytes: 2_097_152, Paginas: 50, SeccionesPorPagina: 25, NodosPorPagina: 2000, Profundidad: 12,
	Componentes: 100, NodosPorComponente: 300, CaracteresPorTexto: 10_000, ColoresTema: 32, CamposFormulario: 20,
	ElementosMenu: 20, HijosPorElementoMenu: 12, ProductosEnRejilla: 48, FilasTabla: 50, ColumnasTabla: 8,
	Slug: 80, URL: 2048,
}
