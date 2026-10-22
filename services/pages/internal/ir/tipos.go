// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

import (
	"bytes"
	"encoding/json"
)

// Tipos del IR con los nombres de tipos.ts y etiquetas JSON idénticas. Reglas (contrato §4.4):
// ningún obligatorio lleva omitempty ni omitzero (un 0 o un false obligatorio se emiten); los
// opcionales, omitzero; tres estados con *bool y Opcional[T] donde false, null y ausente
// significan cosas distintas. TestEspejoDeCampos compara estas etiquetas con campos.ts.

// Textos es un texto por idioma.
type Textos map[string]string

// TextosRicos es un texto con formato por idioma.
type TextosRicos map[string][]Tramo

// Tramo es un trozo de texto con formato.
type Tramo struct {
	Text      string  `json:"text"`
	Bold      bool    `json:"bold,omitzero"`
	Italic    bool    `json:"italic,omitzero"`
	Underline bool    `json:"underline,omitzero"`
	Link      *Enlace `json:"link,omitzero"`
}

// Sitio es el documento entero.
type Sitio struct {
	Version    int          `json:"version"`
	Languages  IdiomasSitio `json:"languages"`
	Theme      Tema         `json:"theme"`
	Header     *Nodo        `json:"header"`
	Footer     *Nodo        `json:"footer"`
	Pages      []Pagina     `json:"pages"`
	Components []Componente `json:"components"`
	SEO        SeoSitio     `json:"seo"`
}

// IdiomasSitio son los idiomas de un sitio: el principal y los demás.
type IdiomasSitio struct {
	Default string   `json:"default"`
	Others  []string `json:"others"`
}

// Tema son los tokens de diseño del sitio.
type Tema struct {
	Colors     map[string]string `json:"colors"`
	Fonts      Fuentes           `json:"fonts"`
	TextStyles EstilosDeTexto    `json:"text_styles"`
	Radius     Radios            `json:"radius"`
	Container  int               `json:"container"`
}

// Fuentes son las familias del catálogo para títulos y cuerpo.
type Fuentes struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// EstilosDeTexto son los diez estilos de texto del tema (claves de EstilosTexto).
type EstilosDeTexto struct {
	H1     EstiloTipografico `json:"h1"`
	H2     EstiloTipografico `json:"h2"`
	H3     EstiloTipografico `json:"h3"`
	H4     EstiloTipografico `json:"h4"`
	H5     EstiloTipografico `json:"h5"`
	H6     EstiloTipografico `json:"h6"`
	Body   EstiloTipografico `json:"body"`
	Small  EstiloTipografico `json:"small"`
	Label  EstiloTipografico `json:"label"`
	Button EstiloTipografico `json:"button"`
}

// EstiloTipografico es un estilo de texto del tema, con tamaño por punto de corte.
type EstiloTipografico struct {
	Font          string  `json:"font"`
	Size          [3]int  `json:"size"`
	LineHeight    float64 `json:"line_height"`
	Weight        int     `json:"weight"`
	LetterSpacing float64 `json:"letter_spacing"`
	Transform     string  `json:"transform,omitzero"`
}

// Radios son los radios del tema en px.
type Radios struct {
	Sm int `json:"sm"`
	Md int `json:"md"`
	Lg int `json:"lg"`
}

// Pagina es una página del sitio (Pages[0] es la portada).
type Pagina struct {
	ID          string     `json:"id"`
	Slug        Textos     `json:"slug"`
	Title       Textos     `json:"title"`
	Description Textos     `json:"description"`
	Sections    []Nodo     `json:"sections"`
	ShowHeader  bool       `json:"show_header"`
	ShowFooter  bool       `json:"show_footer"`
	Noindex     bool       `json:"noindex,omitzero"`
	OGImage     *ImagenRef `json:"og_image,omitzero"`
}

// Tamano es el tamaño de un nodo.
type Tamano struct {
	Width       Medida   `json:"width"`
	Height      Medida   `json:"height"`
	MinWidth    *int     `json:"min_width,omitzero"`
	MaxWidth    *int     `json:"max_width,omitzero"`
	MinHeight   *int     `json:"min_height,omitzero"`
	MaxHeight   *int     `json:"max_height,omitzero"`
	AspectRatio *float64 `json:"aspect_ratio,omitzero"`
}

// Layout es el auto-layout de un contenedor (semántica de CSS flex).
type Layout struct {
	Direction string `json:"direction"`
	Gap       int    `json:"gap"`
	RowGap    *int   `json:"row_gap,omitzero"`
	Padding   [4]int `json:"padding"`
	Align     string `json:"align"`
	Justify   string `json:"justify"`
	Wrap      bool   `json:"wrap"`
	Columns   *int   `json:"columns,omitzero"`
}

// Estilo es la apariencia de un nodo.
type Estilo struct {
	Background *Fondo       `json:"background,omitzero"`
	Color      string       `json:"color,omitzero"`
	Border     *Borde       `json:"border,omitzero"`
	Radius     *Radio       `json:"radius,omitzero"`
	Shadow     string       `json:"shadow,omitzero"`
	Opacity    *float64     `json:"opacity,omitzero"`
	Text       *EstiloTexto `json:"text,omitzero"`
	Overflow   string       `json:"overflow,omitzero"`
}

// Borde es el borde de un nodo.
type Borde struct {
	Width int    `json:"width"`
	Color string `json:"color"`
	Style string `json:"style"`
}

// FondoImagen es un fondo con imagen.
type FondoImagen struct {
	Image    ImagenRef  `json:"image"`
	Fit      string     `json:"fit"`
	Position [2]float64 `json:"position"`
	Overlay  string     `json:"overlay,omitzero"`
}

// EstiloTexto es el estilo de texto de un nodo.
type EstiloTexto struct {
	Preset        string   `json:"preset,omitzero"`
	Font          string   `json:"font,omitzero"`
	Size          *int     `json:"size,omitzero"`
	Weight        *int     `json:"weight,omitzero"`
	LineHeight    *float64 `json:"line_height,omitzero"`
	LetterSpacing *float64 `json:"letter_spacing,omitzero"`
	Align         string   `json:"align,omitzero"`
	Transform     string   `json:"transform,omitzero"`
	Italic        *bool    `json:"italic,omitzero"`
}

// Puntos son las sobrescrituras por punto de corte.
type Puntos struct {
	Tablet *Delta `json:"tablet,omitzero"`
	Mobile *Delta `json:"mobile,omitzero"`
}

// Delta es la sobrescritura de un punto de corte: ausente = hereda, valor = sustituye y null =
// vuelve al valor por defecto.
type Delta struct {
	Hidden  *bool        `json:"hidden,omitzero"`
	Size    *TamanoDelta `json:"size,omitzero"`
	Layout  *LayoutDelta `json:"layout,omitzero"`
	Style   *EstiloDelta `json:"style,omitzero"`
	Columns *int         `json:"columns,omitzero"`
}

// TamanoDelta es el tamaño de un delta.
type TamanoDelta struct {
	Width       *Medida           `json:"width,omitzero"`
	Height      *Medida           `json:"height,omitzero"`
	MinWidth    Opcional[int]     `json:"min_width,omitzero"`
	MaxWidth    Opcional[int]     `json:"max_width,omitzero"`
	MinHeight   Opcional[int]     `json:"min_height,omitzero"`
	MaxHeight   Opcional[int]     `json:"max_height,omitzero"`
	AspectRatio Opcional[float64] `json:"aspect_ratio,omitzero"`
}

// LayoutDelta es un Layout parcial.
type LayoutDelta struct {
	Direction string  `json:"direction,omitzero"`
	Gap       *int    `json:"gap,omitzero"`
	RowGap    *int    `json:"row_gap,omitzero"`
	Padding   *[4]int `json:"padding,omitzero"`
	Align     string  `json:"align,omitzero"`
	Justify   string  `json:"justify,omitzero"`
	Wrap      *bool   `json:"wrap,omitzero"`
	Columns   *int    `json:"columns,omitzero"`
}

// EstiloDelta es el estilo de un delta.
type EstiloDelta struct {
	Background Opcional[Fondo]  `json:"background,omitzero"`
	Color      string           `json:"color,omitzero"`
	Border     Opcional[Borde]  `json:"border,omitzero"`
	Radius     *Radio           `json:"radius,omitzero"`
	Shadow     Opcional[string] `json:"shadow,omitzero"`
	Opacity    *float64         `json:"opacity,omitzero"`
	Text       *EstiloTexto     `json:"text,omitzero"`
	Overflow   string           `json:"overflow,omitzero"`
}

// Nodo es la unión aplanada de todos los tipos de nodo: Type decide qué campos valen, y
// MarshalJSON emite solo los de su tipo (los obligatorios siempre, aunque valgan cero).
type Nodo struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Name        string  `json:"name,omitzero"`
	Locked      bool    `json:"locked,omitzero"`
	Hidden      bool    `json:"hidden,omitzero"`
	Size        *Tamano `json:"size,omitzero"`
	Style       *Estilo `json:"style,omitzero"`
	Breakpoints *Puntos `json:"breakpoints,omitzero"`
	// section y frame.
	Kind      string  `json:"kind,omitzero"`
	Anchor    string  `json:"anchor,omitzero"`
	FullBleed bool    `json:"full_bleed,omitzero"`
	Layout    *Layout `json:"layout,omitzero"`
	Children  []Nodo  `json:"children,omitzero"`
	Tag       string  `json:"tag,omitzero"`
	Link      *Enlace `json:"link,omitzero"`
	// text.
	Content TextosRicos `json:"content,omitzero"`
	// image.
	Src        *ImagenRef `json:"src,omitzero"`
	Alt        Textos     `json:"alt,omitzero"`
	Decorative bool       `json:"decorative,omitzero"`
	Fit        string     `json:"fit,omitzero"`
	Priority   bool       `json:"priority,omitzero"`
	// button, icon, cart_button y menu.
	Label        Textos  `json:"label,omitzero"`
	Variant      string  `json:"variant,omitzero"`
	Action       *Enlace `json:"action,omitzero"`
	Icon         string  `json:"icon,omitzero"`
	IconPosition string  `json:"icon_position,omitzero"`
	// embed.
	Provider string `json:"provider,omitzero"`
	Ref      string `json:"ref,omitzero"`
	Title    Textos `json:"title,omitzero"`
	// form.
	Form    string            `json:"form,omitzero"`
	Fields  []CampoFormulario `json:"fields,omitzero"`
	Submit  Textos            `json:"submit,omitzero"`
	Success Textos            `json:"success,omitzero"`
	Consent Textos            `json:"consent,omitzero"`
	// table.
	Caption Textos     `json:"caption,omitzero"`
	Header  []Textos   `json:"header,omitzero"`
	Rows    [][]Textos `json:"rows,omitzero"`
	// product_grid.
	Source        *FuenteProductos `json:"source,omitzero"`
	Limit         int              `json:"limit,omitzero"`
	Columns       int              `json:"columns,omitzero"`
	ShowPrice     bool             `json:"show_price,omitzero"`
	ShowAddToCart bool             `json:"show_add_to_cart,omitzero"`
	// language_switcher y menu.
	Display       string         `json:"display,omitzero"`
	Items         []ElementoMenu `json:"items,omitzero"`
	Orientation   string         `json:"orientation,omitzero"`
	CollapseBelow string         `json:"collapse_below,omitzero"`
	// instance.
	Component string              `json:"component,omitzero"`
	Overrides map[string]Override `json:"overrides,omitzero"`
}

// MarshalJSON emite solo los campos del tipo del nodo y, de ellos, los obligatorios siempre.
func (n Nodo) MarshalJSON() ([]byte, error) {
	type plano Nodo
	crudo, err := codificarSinEscape(plano(n))
	if err != nil {
		return nil, err
	}
	var campos map[string]json.RawMessage
	if err := json.Unmarshal(crudo, &campos); err != nil {
		return nil, err
	}
	tabla := CamposNodo[n.Type]
	out := make(map[string]json.RawMessage, len(campos))
	for k, v := range campos {
		if _, ok := tabla[k]; ok {
			out[k] = v
		}
	}
	for k, d := range tabla {
		if _, ok := out[k]; !ok && obligatorio(d) {
			out[k] = ceroDe(d)
		}
	}
	return codificarSinEscape(out)
}

// ceroDe es el valor JSON cero de un descriptor: lo que omitzero quitó de un obligatorio.
func ceroDe(d string) json.RawMessage {
	switch d = limpio(d); {
	case d == "b" || d == "t":
		return json.RawMessage("false")
	case d == "i" || d == "n":
		return json.RawMessage("0")
	case d == "s" || len(d) > 2 && d[:2] == "e:":
		return json.RawMessage(`""`)
	case len(d) > 2 && d[:2] == "a:":
		return json.RawMessage("[]")
	default:
		return json.RawMessage("{}")
	}
}

// ImagenRef es una imagen: un recurso subido, una foto de la biblioteca o un relleno.
type ImagenRef struct {
	Asset       *string `json:"asset,omitzero"`
	Library     *string `json:"library,omitzero"`
	Placeholder *string `json:"placeholder,omitzero"`
}

// Enlace es la unión de destinos de un enlace (exactamente uno; page admite además anchor).
type Enlace struct {
	Page    *string `json:"page,omitzero"`
	Anchor  *string `json:"anchor,omitzero"`
	URL     *string `json:"url,omitzero"`
	NewTab  *bool   `json:"new_tab,omitzero"`
	Product *string `json:"product,omitzero"`
	File    *string `json:"file,omitzero"`
	Legal   *string `json:"legal,omitzero"`
	Cart    bool    `json:"cart,omitzero"`
}

// FuenteProductos es la unión de orígenes de una rejilla de productos.
type FuenteProductos struct {
	All      bool     `json:"all,omitzero"`
	Category *string  `json:"category,omitzero"`
	Products []string `json:"products,omitzero"`
}

// CampoFormulario es un campo de un formulario.
type CampoFormulario struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Label        Textos   `json:"label"`
	Required     bool     `json:"required"`
	Options      []Textos `json:"options,omitzero"`
	MaxLength    *int     `json:"max_length,omitzero"`
	Min          *int     `json:"min,omitzero"`
	Max          *int     `json:"max,omitzero"`
	Autocomplete string   `json:"autocomplete,omitzero"`
}

// ElementoMenu es una entrada de un menú.
type ElementoMenu struct {
	ID       string         `json:"id"`
	Label    Textos         `json:"label"`
	Link     Enlace         `json:"link"`
	Children []ElementoMenu `json:"children,omitzero"`
}

// Override es lo que una instancia cambia de un nodo de su maestro.
type Override struct {
	Text   TextosRicos      `json:"text,omitzero"`
	Label  Textos           `json:"label,omitzero"`
	Image  *ImagenRef       `json:"image,omitzero"`
	Alt    Textos           `json:"alt,omitzero"`
	Link   Opcional[Enlace] `json:"link,omitzero"`
	Icon   string           `json:"icon,omitzero"`
	Hidden *bool            `json:"hidden,omitzero"`
	Style  *Estilo          `json:"style,omitzero"`
}

// Componente es un maestro reutilizable (su raíz es un marco).
type Componente struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root Nodo   `json:"root"`
}

// SeoSitio son los metadatos del sitio.
type SeoSitio struct {
	SiteName Textos     `json:"site_name"`
	OGImage  *ImagenRef `json:"og_image,omitzero"`
	Favicon  *ImagenRef `json:"favicon,omitzero"`
	Noindex  bool       `json:"noindex,omitzero"`
	Business *Negocio   `json:"business,omitzero"`
}

// Negocio son los datos del JSON-LD de la portada.
type Negocio struct {
	Type         string     `json:"type"`
	Phone        string     `json:"phone,omitzero"`
	Email        string     `json:"email,omitzero"`
	Address      *Direccion `json:"address,omitzero"`
	Geo          *Geo       `json:"geo,omitzero"`
	OpeningHours []Horario  `json:"opening_hours,omitzero"`
}

// Direccion es una dirección postal (Country en ISO 3166-1 alfa-2).
type Direccion struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	Region     string `json:"region,omitzero"`
	Country    string `json:"country"`
}

// Geo es una posición.
type Geo struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Horario es una franja de apertura ('HH:MM').
type Horario struct {
	Days   []string `json:"days"`
	Opens  string   `json:"opens"`
	Closes string   `json:"closes"`
}

// Medida es 'fill', 'hug' o px fijos.
type Medida struct {
	Clave string // "fill" o "hug"; vacía = Px
	Px    int
}

// MarshalJSON emite la clave o el número.
func (m Medida) MarshalJSON() ([]byte, error) {
	if m.Clave != "" {
		return json.Marshal(m.Clave)
	}
	return json.Marshal(m.Px)
}

// UnmarshalJSON lee una cadena o un número.
func (m *Medida) UnmarshalJSON(b []byte) error {
	*m = Medida{}
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &m.Clave)
	}
	return json.Unmarshal(b, &m.Px)
}

// Radio es un radio en px o un token del tema ('$sm', '$md', '$lg').
type Radio struct {
	Token string // vacío = Px
	Px    int
}

// MarshalJSON emite el token o el número.
func (r Radio) MarshalJSON() ([]byte, error) {
	if r.Token != "" {
		return json.Marshal(r.Token)
	}
	return json.Marshal(r.Px)
}

// UnmarshalJSON lee una cadena o un número.
func (r *Radio) UnmarshalJSON(b []byte) error {
	*r = Radio{}
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &r.Token)
	}
	return json.Unmarshal(b, &r.Px)
}

// Fondo es un color ('#rrggbb(aa)' o '$token') o una imagen.
type Fondo struct {
	Color  string
	Imagen *FondoImagen
}

// MarshalJSON emite la imagen o el color.
func (f Fondo) MarshalJSON() ([]byte, error) {
	if f.Imagen != nil {
		return codificarSinEscape(f.Imagen)
	}
	return codificarSinEscape(f.Color)
}

// UnmarshalJSON lee una cadena o un objeto.
func (f *Fondo) UnmarshalJSON(b []byte) error {
	*f = Fondo{}
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &f.Color)
	}
	f.Imagen = &FondoImagen{}
	return json.Unmarshal(b, f.Imagen)
}

// Opcional es un campo de tres estados: ausente (Presente = false), null (Nulo) o un valor.
type Opcional[T any] struct {
	Presente bool
	Nulo     bool
	Valor    T
}

// IsZero hace que omitzero lo quite cuando está ausente.
func (o Opcional[T]) IsZero() bool { return !o.Presente }

// MarshalJSON emite null o el valor.
func (o Opcional[T]) MarshalJSON() ([]byte, error) {
	if o.Nulo {
		return []byte("null"), nil
	}
	return codificarSinEscape(o.Valor)
}

// UnmarshalJSON solo se llama si la clave está: la marca presente, y nula si es null.
func (o *Opcional[T]) UnmarshalJSON(b []byte) error {
	*o = Opcional[T]{Presente: true}
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Nulo = true
		return nil
	}
	return json.Unmarshal(b, &o.Valor)
}
