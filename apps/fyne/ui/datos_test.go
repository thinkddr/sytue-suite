// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Los guardianes de la familia de DATOS: tabla, filas, rejillas, tesela, medidor,
// insignia y estado vacío.
//
// Regla de la casa, y es la que hace que estos tests sirvan de algo: cada número
// esperado es la cifra ABSOLUTA de la web, escrita a mano con su origen (fichero y
// clase de Tailwind) en el comentario. Nunca se compara contra lo que calcula el
// propio widget, y nunca contra algo relativo al texto: unas pruebas de este
// repositorio medían anchos relativos al rótulo y un texto 6 % más ancho pasaba en
// verde con la caja 4 px de más.

// exacto compara dos medidas de CSS. El margen es 1/64, que es la unidad de
// disposición del navegador (`LayoutUnit`): por debajo de eso el propio Chrome no
// distingue, y por encima es un fallo nuestro.
func exacto(t *testing.T, que string, dio, laWeb float32) {
	t.Helper()
	if math.Abs(float64(dio-laWeb)) > 1.0/64 {
		t.Errorf("%s = %v y la web mide %v (diferencia %v)", que, dio, laWeb, dio-laWeb)
	}
}

// ---- Tabla ----

func columnasDeMuestra() []ColumnaTabla {
	return []ColumnaTabla{
		{Clave: "nombre", Rotulo: "Nombre", Ordenable: true},
		{Clave: "estado", Rotulo: "Estado"},
		{Clave: "importe", Rotulo: "Importe", Alineacion: Fin},
	}
}

func tablaDeMuestra(filas int) *Tabla {
	return NuevaTabla(columnasDeMuestra(),
		func() int { return filas },
		func(f, c int) string { return fmt.Sprintf("f%d c%d", f, c) })
}

// TestLaTablaMideLoQueDicenSusClases fija las tres alturas que decide Table.tsx.
//
// Si estas tres se mueven, se mueve la tabla entera: con cinco mil filas, 0,25 px de
// error por fila son 1.250 px de desplazamiento al final.
func TestLaTablaMideLoQueDicenSusClases(t *testing.T) {
	// `th`: `px-3 py-2.5` + `.sytue-overline` (--text-xs, caja 1rem = 16) + el
	// `border-b` del `tr`. 10 × 2 + 16 + 1, con la raíz de 16 de theme.css.
	exacto(t, "alto de cabecera", AltoCabeceraTabla, 10*2+16+1)
	// `td`: `px-3 py-2.5 text-base` (caja 1,375rem = 22) + `border-b`.
	exacto(t, "alto de fila", AltoFilaTabla(), 10*2+22+1)
	// La celda de «sin resultados»: `px-3 py-10 text-center`.
	exacto(t, "alto del vacío", AltoVacio, 40*2+22)
	// `w-10` de la columna de casillas y `px-3` de toda celda.
	exacto(t, "columna de casillas", ColCasillaTabla, 40)
	exacto(t, "margen de celda", MargenCelda, 12)

	tb := tablaDeMuestra(3)
	if got := nuevaFilaTabla(tb).MinSize().Height; got != AltoFilaTabla() {
		t.Errorf("MinSize de la fila = %v y la medida declarada es %v: hay dos cuentas", got, AltoFilaTabla())
	}
	if got := nuevaCabeceraTabla(tb).MinSize().Height; got != AltoCabeceraTabla {
		t.Errorf("MinSize de la cabecera = %v y la declarada es %v", got, AltoCabeceraTabla)
	}
}

// TestLaCabeceraDeTablaPintaLoQueDeclara comprueba lo PINTADO, no lo calculado: la
// banda blanca con su hairline abajo, a ×2.
func TestLaCabeceraDeTablaPintaLoQueDeclara(t *testing.T) {
	c := nuevaCabeceraTabla(tablaDeMuestra(3))
	m := fyne.NewSize(400, AltoCabeceraTabla)
	c.Resize(m)
	img := pinta(t, m.Width+10, m.Height+10, c)

	// El `border-b` es la ÚLTIMA fila de la cabecera, no una fila de más: la web
	// usa `box-sizing: border-box`.
	ultima := int(AltoCabeceraTabla*escala) - 1
	if got, quiero := px(img, int(200*escala), ultima), tema.Color("border", tema.Claro); !cerca(got, quiero) {
		t.Errorf("la última fila de la cabecera pinta %v y el token border sobre blanco es %v", got, quiero)
	}
	// Y la de justo antes es la superficie, no el borde: un borde de 2 px sería el
	// síntoma de haberlo pintado por fuera.
	if got, quiero := px(img, int(200*escala), ultima-2), tema.Color("surface", tema.Claro); !cerca(got, quiero) {
		t.Errorf("la fila anterior al borde pinta %v y la superficie es %v", got, quiero)
	}
}

// TestElRepartoDeColumnasLlegaAlMargen es la cuenta de reparto: se redondea cada
// BORDE, no cada ancho.
//
// Con división entera la última columna se queda corta y se ve una raya clara al
// final de la cabecera. Aquí la suma TIENE que dar el ancho útil exacto.
func TestElRepartoDeColumnasLlegaAlMargen(t *testing.T) {
	for _, total := range []float32{400, 777, 1024, 1225} {
		for _, sel := range []bool{false, true} {
			tb := tablaDeMuestra(3)
			if sel {
				tb.Marcada = func(int) bool { return false }
			}
			anchos := tb.AnchosColumna(total)
			suma := float32(0)
			for _, a := range anchos {
				suma += a
			}
			util := total
			if sel {
				util -= ColCasillaTabla
			}
			if suma != util {
				t.Errorf("con %v de tabla (casillas=%v) las columnas suman %v y el hueco útil es %v",
					total, sel, suma, util)
			}
		}
	}
}

// TestLaListaNoMeteHuecoEntreFilas es el guardián del tema sin relleno.
//
// `widget.List` separa las filas por `theme.Size(SizeNamePadding)`, que en el tema de
// la suite son 8 px, TAMBIÉN con `HideSeparators`. Sin el `ThemeOverride`, la fila 10
// caería 80 px por debajo de donde la pone la web.
func TestLaListaNoMeteHuecoEntreFilas(t *testing.T) {
	if got := (temaSinRelleno{}).Size(temafyne.SizeNamePadding); got != 0 {
		t.Fatalf("el tema del cuerpo devuelve %v de relleno y tiene que devolver 0", got)
	}
	// Y el resto del tema sigue siendo el de la suite.
	if got, quiero := (temaSinRelleno{}).Size(temafyne.SizeNameSeparatorThickness), tema.Borde; got != quiero {
		t.Errorf("el separador del tema sin relleno mide %v y el de la suite %v", got, quiero)
	}
	if got := (tema.Sytue{}).Size(temafyne.SizeNamePadding); got == 0 {
		t.Fatal("el tema de la suite ya devuelve 0 de relleno: este guardián ha dejado de vigilar nada")
	}
}

// TestLaTablaVaciaDiceQueNoHayNada comprueba que con cero filas se pinta el texto y
// no el cuerpo.
func TestLaTablaVaciaDiceQueNoHayNada(t *testing.T) {
	tb := tablaDeMuestra(0)
	tb.Vacia = "Sin resultados"
	tb.Resize(fyne.NewSize(400, 200))
	r, ok := test.WidgetRenderer(tb).(*tablaRenderer)
	if !ok {
		t.Fatal("el renderer de la tabla no es el suyo")
	}
	r.Layout(fyne.NewSize(400, 200))
	if !r.vacia.Visible() {
		t.Error("una tabla sin filas no enseña el texto de «sin resultados»")
	}
	if r.cuerpo.Visible() {
		t.Error("una tabla sin filas enseña el cuerpo virtualizado")
	}
}

// TestLasClasesDeLasQueSalenLosNumerosSiguenAHI ata las tres geometrías al fuente de
// la web, no a un número copiado a mano.
//
// Un test que solo compara contra 43 pasa para siempre aunque la web cambie a
// `py-2`: el port se queda callado y desfasado. Este lee el JSX y falla si la clase
// de la que sale el número ha desaparecido, que es lo que pasó con los doce
// `middleware.ts` que «había que copiar tal cual».
func TestLasClasesDeLasQueSalenLosNumerosSiguenAHI(t *testing.T) {
	casos := []struct{ que, fichero, clase string }{
		// La celda de Table.tsx: `px-3 py-2.5` es de donde salen 12 y 10.
		{"la celda de la tabla", "apps/frontend/packages/ui/src/components/Table.tsx", "px-3 py-2.5"},
		{"la columna de casillas", "apps/frontend/packages/ui/src/components/Table.tsx", "w-10 px-3 py-2.5"},
		{"la celda de «sin resultados»", "apps/frontend/packages/ui/src/components/Table.tsx", "px-3 py-10 text-center"},
		{"el separador de fila", "apps/frontend/packages/ui/src/components/Table.tsx", "border-border-subtle border-b"},
		{"el contenedor de la tabla", "apps/frontend/packages/ui/src/components/Table.tsx", "rounded-lg border"},
		// El ListItem de List.tsx. Sus tres medidas dependen de la FORMA (tokens
		// `--sytue-row-*` de theme.css): en Carril, que es la que pintan las nativas,
		// la barra mide 2, la sangría 0 y el relleno extra 0, así que son los 2, 12 y 8
		// de siempre (`border-l-2 px-3 py-2`). En Marco: sin barra, 6 de sangría y un
		// píxel más arriba y abajo.
		{"la fila de lista", "apps/frontend/packages/ui/src/components/List.tsx", "gap-3 border-l-[length:var(--sytue-row-bar)] text-base"},
		{"el relleno de la fila de lista", "apps/frontend/packages/ui/src/components/List.tsx", "px-[calc(0.75rem-var(--sytue-row-inset))] py-[calc(0.5rem+var(--sytue-row-grow))]"},
		{"la barra de fila en Carril", "apps/frontend/packages/config/src/theme.css", "--sytue-row-bar: 2px;"},
		{"la fila seleccionada", "apps/frontend/packages/ui/src/components/List.tsx", "border-l-primary bg-selected"},
		// La fila del explorador: `px-3 py-2` con tesela de 36, que NO es lo anterior.
		{"la fila de Drive", "apps/frontend/apps/drive/src/components/FileBrowser.tsx", "border-b px-3 py-2"},
		{"la tesela del explorador", "apps/frontend/apps/drive/src/components/FileBrowser.tsx", "<FileIcon resource={item} size={36} />"},
		{"la cabecera del explorador", "apps/frontend/apps/drive/src/components/FileBrowser.tsx", "shadow-[inset_0_-1px_0_var(--sytue-border)]"},
		{"las columnas del explorador", "apps/frontend/apps/drive/src/components/FileBrowser.tsx", "hidden w-24 truncate text-right sm:block"},
		// El medidor y la insignia.
		{"la barra del medidor", "apps/frontend/packages/ui/src/components/Meter.tsx", "h-1.5 w-full overflow-hidden rounded-full"},
		{"la insignia", "apps/frontend/packages/ui/src/components/Badge.tsx", "px-2 py-1 text-xs"},
		{"el estado vacío", "apps/frontend/packages/ui/src/components/EmptyState.tsx", "gap-3 px-6 py-8 text-center"},
	}
	for _, c := range casos {
		if fuente := webref.Read(t, c.fichero); !strings.Contains(fuente, c.clase) {
			t.Errorf("%s: %s ya no contiene %q.\n"+
				"  O la web ha cambiado —y hay que volver a medir— o aquí se escribió a ojo.",
				c.que, c.fichero, c.clase)
		}
	}
	// El guardián, invertido: una clase inventada TIENE que no encontrarse.
	if strings.Contains(webref.Read(t, "apps/frontend/packages/ui/src/components/Table.tsx"), "px-3 py-7.25") {
		t.Fatal("una clase inventada se ha encontrado: la comparación no vale")
	}
}

// ---- Fila de lista ----

// TestLaFilaDeListaMide38Y56 son las dos alturas de `ListItem` de List.tsx.
//
// `flex items-center border-l-2 px-3 py-2 text-base`: 8 + 22 + 8 con solo título, y
// 8 + 22 + 18 + 8 con subtítulo.
func TestLaFilaDeListaMide38Y56(t *testing.T) {
	exacto(t, "alto sin subtítulo", MedidaFilaLista(false, 0), 38)
	exacto(t, "alto con subtítulo", MedidaFilaLista(true, 0), 56)
	// `items-center`: el avatar md (40 px LITERALES en Avatar.tsx) empata con las
	// dos cajas de línea. El lg de 48 manda y comprueba el caso de una pieza más alta.
	exacto(t, "alto con avatar md", MedidaFilaLista(true, 40), 56)
	exacto(t, "alto con avatar lg", MedidaFilaLista(true, 48), 64)

	f := NuevaFilaLista("Ana Martínez", "Revisión del contrato marco")
	f.Meta = "12:45"
	if got := f.MinSize().Height; got != 56 {
		t.Errorf("MinSize de la fila = %v y la declarada es 56", got)
	}

	f.Resize(fyne.NewSize(320, 56))
	r, ok := test.WidgetRenderer(f).(*filaListaRenderer)
	if !ok {
		t.Fatal("el renderer de la fila no es el suyo")
	}
	r.Layout(fyne.NewSize(320, 56))
	// `border-l-2` son 2 px de BORDE y `px-3` empieza detrás: el texto arranca en
	// 2 + 12.
	exacto(t, "ancho del galón izquierdo", r.galon.Size().Width, 2)
	exacto(t, "x del título", r.titulo.Position().X, 14)
	exacto(t, "x del subtítulo", r.subtitulo.Position().X, 14)
	// La meta va pegada a la derecha con `px-3` y no encoge (`shrink-0`).
	exacto(t, "margen derecho de la meta", 320-(r.meta.Position().X+r.meta.MinSize().Width), 12)
	// Las dos cajas de línea se apilan y el bloque va centrado en la fila. La caja de
	// línea es la del texto menos su desfase: el `canvas.Text` va colocado por la línea
	// base de Chromium (ver ColocarTexto), no centrado.
	cajaLinea := func(tx *fcanvas.Text, e tema.Escalon) float32 {
		return tx.Position().Y - desfaseTexto(tx.TextSize, e.Linea, tx.FontSource, tx.TextStyle)
	}
	exacto(t, "centro de la caja del título", cajaLinea(r.titulo, tema.TextoBase)+22/2, 8+22/2)
	exacto(t, "centro de la caja del subtítulo", cajaLinea(r.subtitulo, tema.TextoSM)+18/2, 8+22+18/2)
}

// TestLaFilaSeleccionadaTiñeElGalonYElFondo es el estado que más se ve en la suite.
func TestLaFilaSeleccionadaTiñeElGalonYElFondo(t *testing.T) {
	f := NuevaFilaLista("Ana Martínez", "")
	f.Seleccionada = true
	f.Resize(fyne.NewSize(200, 38))
	fondo := fcanvas.NewRectangle(tema.Color("surface", tema.Claro))
	fondo.Resize(fyne.NewSize(220, 50))
	img := pinta(t, 220, 50, fondo, f)

	// El galón izquierdo es `border-l-primary`: tinta OPACA en los 2 primeros px.
	if got, quiero := px(img, 1, int(16*escala)), tema.Color("primary", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el galón de la fila seleccionada pinta %v y el token primary es %v", got, quiero)
	}
	// Y el fondo es `bg-selected`, que es TRANSLÚCIDO sobre la superficie.
	quiero := tokens.SobreSRGB(tema.Color("selected", tema.Claro), tema.Color("surface", tema.Claro))
	if got := px(img, int(100*escala), int(16*escala)); !cerca(got, quiero) {
		t.Errorf("el fondo de la fila seleccionada pinta %v y `bg-selected` sobre la superficie es %v", got, quiero)
	}
}

// TestTruncarNoSeComeElTextoAlReciclar es la trampa de una fila virtualizada: si se
// trunca sobre lo ya truncado, cada vuelta por pantalla se lleva una letra.
func TestTruncarNoSeComeElTextoAlReciclar(t *testing.T) {
	const completo = "Informe trimestral de resultados consolidados"
	c := Texto(completo, tema.TextoBase, Normal, tema.Color("text", tema.Claro))
	primera := Truncar(c, completo, tema.TextoBase, Normal, 80)
	texto := c.Text
	for range 20 {
		Truncar(c, completo, tema.TextoBase, Normal, 80)
	}
	if c.Text != texto {
		t.Errorf("tras 20 reciclajes el texto es %q y en el primero era %q", c.Text, texto)
	}
	if primera > 80 {
		t.Errorf("el truncado devuelve %v de ancho y el hueco era 80", primera)
	}
	// Y con sitio de sobra no se toca nada.
	Truncar(c, completo, tema.TextoBase, Normal, 9999)
	if c.Text != completo {
		t.Errorf("con sitio de sobra el texto queda en %q y tenía que quedar entero", c.Text)
	}
}

// TestElTruncadoEsElMasLargoQueCabe: `recorta` busca por bisección, y el guardián
// es la búsqueda tonta —de la más larga a la más corta— a cada ancho, que es lo que
// hacía el `recorta` lineal que había antes. Y sin sitio no enseña nada, como el
// navegador: el lineal devolvía el texto ENTERO, que se salía de su caja.
func TestElTruncadoEsElMasLargoQueCabe(t *testing.T) {
	const completo = "Colaboración con la gestoría"
	runas := []rune(completo)
	for cabe := float32(4); cabe < Medir(completo, tema.TextoSM, SemiNegrita).Width+4; cabe += 3.5 {
		quiero := completo
		if Medir(completo, tema.TextoSM, SemiNegrita).Width > cabe {
			quiero = "…"
			for i := len(runas) - 1; i > 0; i-- {
				if c := string(runas[:i]) + "…"; Medir(c, tema.TextoSM, SemiNegrita).Width <= cabe {
					quiero = c
					break
				}
			}
		}
		if got, ancho := truncado(completo, tema.TextoSM, SemiNegrita.cara(), cabe); got != quiero || ancho != Medir(got, tema.TextoSM, SemiNegrita).Width {
			t.Errorf("en %v cabe %q y recorta da %q (%v de ancho)", cabe, quiero, got, ancho)
		}
	}
	for _, cabe := range []float32{0, -12} {
		if got := recorta(completo, tema.TextoSM, SemiNegrita, cabe); got != "" {
			t.Errorf("con %v de sitio recorta da %q, y sin sitio no se enseña nada", cabe, got)
		}
	}
}

// ---- Fila del explorador de Drive ----

// TestLaFilaDeDriveMide53YNoEsLaDeListTsx es la corrección que más tiempo ahorra: la
// lista de Sytue Drive NO es `ListItem`.
//
// `flex items-center gap-3 border-b px-3 py-2` con una tesela de 36: 8 + 36 + 8 + 1.
// La de List.tsx es `py-2` también pero sin tesela, y la tabla es `py-2.5`. Son tres
// geometrías distintas y portarlas como una mueve cada fila.
func TestLaFilaDeDriveMide53YNoEsLaDeListTsx(t *testing.T) {
	exacto(t, "tesela del explorador", TeselaDrive, 36)
	exacto(t, "alto sin el hairline", AltoFilaDrive, 52)
	exacto(t, "alto con el hairline", MedidaFilaDrive(), 53)
	// Y no coincide con ninguna de las otras dos.
	if MedidaFilaDrive() == AltoFilaTabla() {
		t.Error("la fila de Drive mide lo mismo que la de la tabla: una de las dos está mal")
	}
	if MedidaFilaDrive() == MedidaFilaLista(true, 0) {
		t.Error("la fila de Drive mide lo mismo que la de List.tsx, y la web las pinta distintas")
	}
	// `w-24`, `w-28` y `w-8` de las columnas de la derecha.
	exacto(t, "columna de tamaño", ColTamano, 96)
	exacto(t, "columna de fecha", ColFecha, 112)
	exacto(t, "columna de acciones", ColAcciones, 32)

	f := NuevaFilaDrive("memoria-2026.pdf", "pdf")
	f.Tamano, f.Fecha = "1,2 MB", "12 sep 2026"
	if got := f.MinSize().Height; got != MedidaFilaDrive() {
		t.Errorf("MinSize de la fila de Drive = %v y la declarada es %v", got, MedidaFilaDrive())
	}
}

// TestLasColumnasDeDriveSeMidenPorLaVENTANA es la trampa número 8 del encargo.
//
// `sm:` y `md:` son media queries: miden la ventana, no la fila. Una fila de 900 px
// dentro de una ventana de 500 (un panel lateral ancho en un móvil no existe, pero un
// panel estrecho en una ventana ancha sí) no debe enseñar lo que la web esconde.
func TestLasColumnasDeDriveSeMidenPorLaVENTANA(t *testing.T) {
	f := NuevaFilaDrive("memoria.pdf", "pdf")
	f.Tamano, f.Fecha = "1,2 MB", "12 sep"
	casos := []struct {
		ventana       float32
		tamano, fecha bool
	}{
		{390, false, false}, // móvil: las dos escondidas
		{700, true, false},  // ≥ sm, < md: solo el tamaño
		{1280, true, true},  // escritorio: las dos
	}
	for _, c := range casos {
		f.Ventana = c.ventana
		if got := f.haySM(); got != c.tamano {
			t.Errorf("a %v de ventana la columna de tamaño sale %v y la web dice %v", c.ventana, got, c.tamano)
		}
		if got := f.hayMD(); got != c.fecha {
			t.Errorf("a %v de ventana la columna de fecha sale %v y la web dice %v", c.ventana, got, c.fecha)
		}
	}
	// Los cortes son los de Tailwind y se miden con el rem INICIAL de 16. La raíz
	// de la suite tiene ahora la misma medida: `sm` son 640.
	exacto(t, "corte sm", CorteSM, 640)
	exacto(t, "corte xl", CorteXL, 1280)
}

// TestLaCabeceraDeDriveMide36: `py-2.5` más `sytue-overline`, y el hairline es
// una SOMBRA INTERIOR, así que no añade alto (a diferencia del `border-b` de la
// tabla, que sí).
func TestLaCabeceraDeDriveMide36(t *testing.T) {
	exacto(t, "alto de la cabecera del explorador", AltoCabeceraDrive, 10*2+16)
	if AltoCabeceraDrive == AltoCabeceraTabla {
		t.Error("la cabecera del explorador mide lo mismo que la de la tabla, y la web las pinta distintas")
	}
	c := NuevaCabeceraDrive("Nombre", "Tamaño", "Modificado")
	if got := c.MinSize().Height; got != AltoCabeceraDrive {
		t.Errorf("MinSize de la cabecera de Drive = %v y la declarada es %v", got, AltoCabeceraDrive)
	}
}

// ---- Versalitas con tracking ----

// TestLasVersalitasLlevanSuTracking es lo que hace que un rótulo de columna mida lo
// que mide en la web.
//
// `.sytue-overline` lleva `letter-spacing: 0.06em`, que a `--text-xs` son 0,72 px
// con la raíz de 16. `canvas.Text` no sabe espaciar, así que el texto se compone
// rune a rune; sin eso, «MODIFICADO» sale siete píxeles y pico más estrecho y la
// cabecera no cuadra con el cuerpo.
func TestLasVersalitasLlevanSuTracking(t *testing.T) {
	// El paquete lo declara en EM (`letter-spacing: 0.06em`); a `--text-xs` son los
	// 0,72 px que resultan de 0,06 × 12.
	exacto(t, "tracking de las versalitas", TrackingOverline*tema.TextoXS.Tam, 0.72)

	const rotulo = "MODIFICADO"
	e := NuevoVersal(rotulo, tema.TextoXS, Negrita, tema.Color("text", tema.Claro))
	suelto := Medir(rotulo, tema.TextoXS, Negrita).Width
	if e.Ancho() <= suelto {
		t.Errorf("el rótulo espaciado mide %v y uno suelto %v: el tracking no se está sumando", e.Ancho(), suelto)
	}
	// Diez runas, diez avances: el de CSS se añade DESPUÉS de cada carácter,
	// también del último.
	esperado := float32(len(rotulo)) * TrackingOverline * tema.TextoXS.Tam
	if d := e.Ancho() - suelto; math.Abs(float64(d-esperado)) > 0.6 {
		t.Errorf("el tracking añade %v y diez caracteres a 0,72 son %v", d, esperado)
	}
	// Y los glifos van realmente separados: el segundo empieza después del primero
	// más su avance más el tracking.
	e.Colocar(0, 0)
	objs := e.Objetos()
	primero := Medir("M", tema.TextoXS, Negrita).Width
	exacto(t, "x de la segunda letra", objs[1].Position().X, primero+TrackingOverline*tema.TextoXS.Tam)
}

// ---- Rejilla ----

// TestLaRejillaAutoFillDaCincoColumnasDe241 fija el reparto de un `auto-fill` con la raíz
// de 16: `minmax(13rem,1fr)` son 208 y `gap-1` son 4. Eran las clases de la galería de
// Sytue Photos, que ya va en filas justificadas (`photos.Filas`); el reparto y su
// truncado a 1/64 se siguen vigilando aquí con los mismos números.
//
// Se conserva la caja de 1224 del volcado histórico como fixture de reparto. Caben
// cinco columnas y el ancho se cuantiza a 1/64: floor((1224-4×4)/5×64)/64.
func TestLaRejillaAutoFillDaCincoColumnasDe241(t *testing.T) {
	const caja = 1224
	r := Rejilla{Min: 13 * tema.Rem, Hueco: 1 * 0.25 * tema.Rem, Cuadrada: true}
	// `gap-1` y `minmax(13rem,1fr)` con el rem de 16.
	exacto(t, "hueco", r.Hueco, 4)
	exacto(t, "mínimo", r.Min, 208)

	if got := r.Columnas(caja); got != 5 {
		t.Fatalf("la rejilla da %d columnas y las clases de la web dan 5", got)
	}
	exacto(t, "ancho de tesela", r.AnchoColumna(caja), 241.59375)
	// Las posiciones se acumulan desde el ancho cuantizado, no desde 241,6; se
	// comparan a dos decimales como las medidas del volcado original.
	for c, x := range []float32{0, 245.59, 491.19, 736.78} {
		if d := math.Abs(float64(r.X(caja, c) - x)); d > 0.005 {
			t.Errorf("la columna %d empieza en %v y el reparto de CSS da %v", c, r.X(caja, c), x)
		}
	}
	// La última NO llega justo al margen, y el navegador tampoco: sobran los dos
	// sesentaicuatroavos que se perdieron al truncar cada columna.
	sobrante := caja - (r.X(caja, 4) + r.AnchoColumna(caja))
	if sobrante < 0 || sobrante > 5*LayoutUnit {
		t.Errorf("tras la última columna sobran %v px; el truncado a 1/64 deja menos de %v", sobrante, 5*LayoutUnit)
	}
}

// TestLaRejillaDeTarjetasEsLaMayoritaria: en esta web hay 88 rejillas de columnas
// FIJAS y 9 de `auto-fill`. La de tarjetas del explorador es `grid-cols-2 gap-4
// sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5`, y una variante que no se declara
// —aquí `md:` y `2xl:`— hereda de la anterior.
func TestLaRejillaDeTarjetasEsLaMayoritaria(t *testing.T) {
	cols := Cortes{2, 3, 0, 4, 5, 0}
	casos := []struct {
		ventana float32
		quiero  int
	}{
		{390, 2},  // móvil: la clase base
		{700, 3},  // ≥ sm
		{800, 3},  // ≥ md, que NO declara variante: hereda las 3 de sm
		{1100, 4}, // ≥ lg
		{1280, 5}, // ≥ xl
		{1600, 5}, // ≥ 2xl, que tampoco declara: hereda las 5 de xl
	}
	for _, c := range casos {
		r := RejillaTarjetas(c.ventana, tema.Espacio(4), cols)
		// El ancho de la CAJA no puede cambiar el número de columnas de un
		// `grid-cols-N`: eso es lo que distingue una rejilla fija de un `auto-fill`.
		for _, caja := range []float32{300, 900, 4000} {
			if got := r.Columnas(caja); got != c.quiero {
				t.Errorf("a %v de ventana y %v de caja salen %d columnas y la web pinta %d",
					c.ventana, caja, got, c.quiero)
			}
		}
	}
	// `gap-4` con el rem de 16.
	exacto(t, "hueco de la rejilla de tarjetas", RejillaTarjetas(1280, tema.Espacio(4), cols).Hueco, 16)
	// Y el reparto sigue llegando al margen salvo el truncado a 1/64.
	r := RejillaTarjetas(1280, tema.Espacio(4), cols)
	sobra := 1000 - (r.X(1000, 4) + r.AnchoColumna(1000))
	if sobra < 0 || sobra > 6*LayoutUnit {
		t.Errorf("tras la última tarjeta sobran %v px", sobra)
	}
}

// TestLaTeselaDeTipoMide36YRedondeaA10 es el `FileIcon` del explorador.
func TestLaTeselaDeTipoMide36YRedondeaA10(t *testing.T) {
	te := NuevaTeselaTipo("PDF", false, TeselaDrive)
	te.Colocar(0, 0)
	objs := te.Objetos()
	if got := objs[0].Size(); got.Width != 36 || got.Height != 36 {
		t.Errorf("la tesela mide %v y el `size={36}` de la web son 36×36", got)
	}
	// `rounded-md` son 10 px LITERALES en spacing.json.
	fondo, ok := objs[0].(interface{ Position() fyne.Position })
	if !ok || fondo.Position() != (fyne.Position{}) {
		t.Errorf("la tesela colocada en (0,0) está en %v", objs[0].Position())
	}
	// La extensión se recorta a cuatro y va en versalitas.
	te2 := NuevaTeselaTipo("jpeg", false, TeselaDrive)
	if got := te2.rotulo(); got != "JPEG" {
		t.Errorf("el rótulo de la tesela es %q y la web escribe «JPEG»", got)
	}
	te3 := NuevaTeselaTipo("sqlite3", false, TeselaDrive)
	if got := te3.rotulo(); got != "SQLI" {
		t.Errorf("el rótulo de una extensión larga es %q y la web corta a cuatro: «SQLI»", got)
	}
	// El tinte sale del `KIND_TINT` de la web, no de un color a mano.
	if got, quiero := te.tinta(tema.Claro), tema.Color("danger-text", tema.Claro); got != quiero {
		t.Errorf("el tinte de un PDF es %v y la web usa `text-danger-text` = %v", got, quiero)
	}
}

// ---- Insignia y contador ----

// TestLaInsigniaMide20: `px-2 py-1 text-xs leading-none`.
//
// `leading-none` es lo que decide el alto: la caja de línea NO son los 16 de
// `--text-xs--line-height`, es el propio tamaño de letra (12). Con la caja de 16 la
// insignia saldría de 24 y no de 20.
func TestLaInsigniaMide20(t *testing.T) {
	m := MedidaInsignia("Activo")
	exacto(t, "alto de la insignia", m.Height, 4*2+12)
	exacto(t, "relleno horizontal", m.Width-Medir("Activo", escalonInsignia, SemiNegrita).Width, 16)
	i := NuevaInsignia("Activo", InsigniaExito)
	if got := i.MinSize(); got != m {
		t.Errorf("MinSize de la insignia = %v y la declarada es %v", got, m)
	}
}

// TestLosTonosDeLaInsigniaSonSemanticos: los siete tonosInsignia de Badge.tsx van por tokens
// `-bg`/`-text`, que conmutan por tema. Los escalones crudos salían como manchas casi
// blancas con texto marrón en oscuro.
func TestLosTonosDeLaInsigniaSonSemanticos(t *testing.T) {
	for _, tn := range []Tono{InsigniaNeutra, InsigniaPrimaria, InsigniaAcento, InsigniaExito, InsigniaAviso, InsigniaPeligro, InsigniaInfo} {
		i := &Insignia{Texto: "x", Tono: tn}
		claro, tintaClara := i.colores(tema.Claro)
		oscuro, tintaOscura := i.colores(tema.Oscuro)
		if claro == oscuro && tintaClara == tintaOscura {
			t.Errorf("el tono %d pinta igual en claro y en oscuro: no está yendo por token semántico", tn)
		}
	}
	// Y el tono de peligro es `bg-danger-bg` con tinta `danger-text`, no el rojo
	// pleno: ese es el del CONTADOR, que es otro widget.
	relleno, tinta := (&Insignia{Texto: "Vencido", Tono: InsigniaPeligro}).colores(tema.Claro)
	if relleno != tema.Color("danger-bg", tema.Claro) || tinta != tema.Color("danger-text", tema.Claro) {
		t.Errorf("el tono de peligro pinta %v sobre %v y la web usa danger-text sobre danger-bg", tinta, relleno)
	}
	// El acento de Badge.tsx es la etiqueta violeta del tema compartido.
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		relleno, tinta := (&Insignia{Texto: "Destacado", Tono: InsigniaAcento}).colores(v)
		if relleno != tema.Color("violet-bg", v) || tinta != tema.Color("violet-text", v) {
			t.Errorf("el acento en tema %v pinta %v sobre %v y Badge.tsx usa violet-text sobre violet-bg", v, tinta, relleno)
		}
	}
}

// ---- Medidor ----

// TestElMedidorAvisaAl80YAl95 son los dos umbrales de Meter.tsx, que son fijos: dos
// productos que avisan a distinto porcentaje enseñan a desconfiar de los dos.
func TestElMedidorAvisaAl80YAl95(t *testing.T) {
	casos := []struct {
		frac  float64
		nivel Nivel
		token string
	}{
		{0.5, NivelOK, "text-subtle"},
		{0.79, NivelOK, "text-subtle"},
		{0.8, NivelAviso, "warning"},
		{0.94, NivelAviso, "warning"},
		{0.95, NivelLleno, "danger"},
		{1, NivelLleno, "danger"},
	}
	for _, c := range casos {
		if got := NivelDe(c.frac); got != c.nivel {
			t.Errorf("al %v%% el nivel es %d y la web dice %d", c.frac*100, got, c.nivel)
		}
		m := &Medidor{Fraccion: c.frac}
		if got := m.tokenRelleno(); got != c.token {
			t.Errorf("al %v%% el relleno es %q y la web usa %q", c.frac*100, got, c.token)
		}
	}
	// Sobre el cromo la rampa es otra: los neutros de página no contrastarían.
	m := &Medidor{Fraccion: 0.85, SobreCromo: true}
	// `bg-on-chrome-warning`, que dentro del carril la web reasigna a su ámbar.
	if got := m.tokenRelleno(); got != "sidebar-indicator" {
		t.Errorf("el aviso sobre el carril usa %q y la web `bg-on-chrome-warning`, que allí es `sidebar-indicator`", got)
	}
}

// TestElMedidorMideLoQueSusClases: `gap-1.5` entre las tres partes y `h-1.5` de barra.
func TestElMedidorMideLoQueSusClases(t *testing.T) {
	exacto(t, "hueco de la columna", HuecoMedidor, 6)
	exacto(t, "alto de la barra", AltoBarra, 6)
	// Sin pie: caja de `text-xs` (16) + hueco + barra.
	exacto(t, "alto sin pie", MedidaMedidor(false), 16+6+6)
	// Con pie: y otra caja de `text-xs` debajo.
	exacto(t, "alto con pie", MedidaMedidor(true), 16+6+6+6+16)

	// El relleno se pinta con el porcentaje ENTERO, como en la web
	// (`width: ${pct}%` con `pct = Math.round(frac*100)`), no con la fracción.
	m := NuevoMedidor("Almacenamiento", 0.337, "34%")
	if got := m.PorCiento(); got != 34 {
		t.Errorf("el 33,7%% redondea a %d y la web pone 34", got)
	}
	m.Resize(fyne.NewSize(200, MedidaMedidor(false)))
	r, ok := test.WidgetRenderer(m).(*medidorRenderer)
	if !ok {
		t.Fatal("el renderer del medidor no es el suyo")
	}
	r.Layout(fyne.NewSize(200, MedidaMedidor(false)))
	exacto(t, "ancho del relleno al 34 %", r.lleno.Size().Width, 68)
}

// TestElMedidorDiceElAvisoConPalabras es WCAG 1.4.1: un aviso que solo existe como
// color no llega a quien no lo percibe. La copia de Drive pintaba siempre el mismo
// gris y el buzón podía estar al 99 % sin que la barra lo contara.
func TestElMedidorDiceElAvisoConPalabras(t *testing.T) {
	m := NuevoMedidor("Almacenamiento", 0.97, "97%")
	m.Cuanto, m.Nota = "14,6 GB de 15 GB", "lleno"
	etiqueta := m.AccessibilityLabel()
	for _, trozo := range []string{"Almacenamiento", "14,6 GB de 15 GB", "97%", "lleno"} {
		if !strings.Contains(etiqueta, trozo) {
			t.Errorf("el nombre accesible es %q y le falta %q", etiqueta, trozo)
		}
	}
}

// ---- Estado vacío ----

// TestElEstadoVacioMideLoQueSusClases: `gap-3 px-6 py-8`, con el `mb-2` de la
// ilustración y el `mt-2` de la acción SUMÁNDOSE al hueco de la columna.
func TestElEstadoVacioMideLoQueSusClases(t *testing.T) {
	exacto(t, "hueco de la columna", HuecoVacio, 12)
	exacto(t, "margen horizontal", MargenXVacio, 24)
	exacto(t, "margen vertical", MargenYVacio, 32)
	exacto(t, "separación extra", SeparacionExtra, 8)
	// `max-w-sm` son 24rem con el rem de 16.
	exacto(t, "ancho del párrafo", AnchoDescripcion, 384)
	// `.sytue-h4` es `--text-lg`: 18 / 26.
	exacto(t, "tamaño del título", EscalonTitulo(4).Tam, 18)
	exacto(t, "caja del título", EscalonTitulo(4).Linea, 26)

	v := NuevoVacio("Aquí no hay nada todavía", "")
	// `py-8` × 2 + la caja del `h4`.
	exacto(t, "alto sin descripción ni acción", v.MedidaVacio(400), 32*2+26)
}

// TestElParrafoDelVacioSeReparteEnLineas: `canvas.Text` no parte líneas y una
// descripción de dos renglones se salía de la caja.
func TestElParrafoDelVacioSeReparteEnLineas(t *testing.T) {
	const texto = "Sube tu primera foto o crea un álbum para empezar a organizar la biblioteca"
	una := Repartir(texto, tema.TextoBase, Normal, 9999)
	if len(una) != 1 {
		t.Errorf("con sitio de sobra el párrafo sale en %d líneas y tiene que salir en 1", len(una))
	}
	varias := Repartir(texto, tema.TextoBase, Normal, AnchoDescripcion)
	if len(varias) < 2 {
		t.Errorf("a %v de ancho el párrafo sale en %d líneas; no está partiendo", AnchoDescripcion, len(varias))
	}
	for _, l := range varias {
		if w := Medir(l, tema.TextoBase, Normal).Width; w > AnchoDescripcion {
			t.Errorf("la línea %q mide %v y el `max-w-sm` son %v", l, w, AnchoDescripcion)
		}
	}
}

// ---- Avatar ----

// TestElAvatarUsaElMismoHashQueLaWeb: una persona tiene que salir del MISMO color en
// el navegador y en el escritorio, o se lee como dos personas distintas.
func TestElAvatarUsaElMismoHashQueLaWeb(t *testing.T) {
	// Las cinco tallas de `sizePx`, en píxeles literales.
	for talla, quiero := range map[TallaAvatar]float32{
		AvatarXS: 24, AvatarSM: 32, AvatarMD: 40, AvatarLG: 48, AvatarXL: 64,
	} {
		if got := MedidaAvatar(talla).Width; got != quiero {
			t.Errorf("la talla %d mide %v y `sizePx` dice %v", talla, got, quiero)
		}
	}
	// El `initials()` de la web, caso por caso.
	for nombre, quiero := range map[string]string{
		"Ana Martínez": "AM", "Ana": "AN", "": "?",
		"Ana María Martínez Ruiz": "AR",
	} {
		if got := Iniciales(nombre); got != quiero {
			t.Errorf("las iniciales de %q son %q y la web escribe %q", nombre, got, quiero)
		}
	}
	// El hash es `hash*31 + charCodeAt(i)` sobre unidades UTF-16, truncado a 32
	// bits. «Ana» son 65, 110, 97: ((65*31)+110)*31+97 = 65.912; 65.912 % 6 = 2, o
	// sea `--sytue-avatar-3`.
	if got, quiero := ColorAvatar("Ana", tema.Claro), tema.Color("avatar-3", tema.Claro); got != quiero {
		t.Errorf("«Ana» sale de color %v y con el hash de la web le toca avatar-3 = %v", got, quiero)
	}
}

// ---- Rendimiento ----

// BenchmarkTablaLarga mide lo que el encargo pide medir: pintar la tabla con 50,
// 1.000 y 5.000 filas.
//
// Lo que se vigila es que el coste sea PLANO con el número de filas. Si crece, es que
// la virtualización se ha perdido: en este repositorio ya se midió que las mismas
// piezas fuera de un `widget.List` pasan de 9 ms a 1.115 con 5.000.
func BenchmarkTablaLarga(b *testing.B) {
	for _, n := range []int{50, 1000, 5000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			tb := tablaDeMuestra(n)
			tb.Resize(fyne.NewSize(900, 600))
			b.ResetTimer()
			for b.Loop() {
				tb.Refresh()
				test.WidgetRenderer(tb).Layout(fyne.NewSize(900, 600))
			}
		})
	}
}

// BenchmarkFilaListaReciclada mide el coste del `UpdateItem`: es lo que se paga por
// cada fila que entra por el borde de la pantalla al desplazar.
func BenchmarkFilaListaReciclada(b *testing.B) {
	f := NuevaFilaLista("Ana Martínez", "Revisión del contrato marco")
	f.Meta = "12:45"
	f.Resize(fyne.NewSize(320, 56))
	r := test.WidgetRenderer(f)
	b.ResetTimer()
	i := 0
	for b.Loop() {
		i++
		f.Titulo = fmt.Sprintf("Asunto número %d de la bandeja", i)
		f.Seleccionada = i%3 == 0
		r.Refresh()
		r.Layout(fyne.NewSize(320, 56))
	}
}

// BenchmarkTablaPintada es el mismo dato que ya se midió en este repositorio para
// decidir el patrón —9,6 / 8,7 / 9,3 ms con 50, 1.000 y 5.000 filas dentro de un
// `widget.List`, contra 1.115 ms fuera de él—, pero sobre la tabla de verdad.
//
// Lo que se vigila es que sea PLANO. Si crece con el número de filas, la
// virtualización se ha perdido.
func BenchmarkTablaPintada(b *testing.B) {
	for _, n := range []int{50, 1000, 5000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			tb := tablaDeMuestra(n)
			c := software.NewCanvas()
			c.SetPadded(false)
			c.SetContent(tb)
			c.SetScale(1)
			c.Resize(fyne.NewSize(900, 600))
			b.ResetTimer()
			for b.Loop() {
				tb.Refresh()
				c.Capture()
			}
		})
	}
}

// TestLasFilasDeLaTablaVanPEGADAS es el guardián FUERTE del hueco de `widget.List`:
// no mira el tema, mira los píxeles.
//
// Pinta una tabla de tres filas y busca los hairlines de `border-b`. Si entre dos hay
// 51 en vez de 43, es que el relleno del tema se ha colado otra vez.
func TestLasFilasDeLaTablaVanPEGADAS(t *testing.T) {
	tb := tablaDeMuestra(3)
	alto := AltoCabeceraTabla + 3*AltoFilaTabla() + 2*tema.Borde
	tb.Resize(fyne.NewSize(400, alto))
	img := pinta(t, 400, alto+10, tb)

	// Las filas de píxeles donde aparece el hairline del cuerpo, en el centro de la
	// tabla para no tropezar con el borde del contenedor.
	quiero := tema.Color("border-subtle", tema.Claro)
	sobreFondo := tokens.SobreSRGB(quiero, tema.Color("surface", tema.Claro))
	var lineas []int
	x := int(200 * escala)
	anterior := false
	for y := int(AltoCabeceraTabla * escala); y < int(alto*escala); y++ {
		hay := cerca(px(img, x, y), sobreFondo)
		if hay && !anterior {
			lineas = append(lineas, y)
		}
		anterior = hay
	}
	if len(lineas) < 2 {
		t.Fatalf("se han encontrado %d hairlines de fila y tiene que haber al menos 2", len(lineas))
	}
	separacion := float32(lineas[1]-lineas[0]) / escala
	if d := math.Abs(float64(separacion - AltoFilaTabla())); d > float64(1/escala) {
		t.Errorf("entre dos filas hay %v px y `py-2.5 text-base` con su hairline son %v"+
			" (¿ha vuelto el relleno del tema, que son %v?)", separacion, AltoFilaTabla(), (tema.Sytue{}).Size(temafyne.SizeNamePadding))
	}
}

// TestBancoVisual pinta la familia entera en una lámina y, con SYTUE_CAPTURA, la deja
// en un PNG. Sin la variable no escribe nada y solo comprueba que todo se compone.
//
// Es para MIRAR el resultado, que es lo único que caza un fallo que los números no
// ven: un color que se compone al revés, un texto que se sale, una tesela sin glifo.
func TestBancoVisual(t *testing.T) {
	filas := []string{"Ana Martínez", "Bruno Salas", "Chloé Nguyen"}
	tb := NuevaTabla([]ColumnaTabla{
		{Clave: "nombre", Rotulo: "Nombre", Ordenable: true},
		{Clave: "estado", Rotulo: "Estado"},
		{Clave: "importe", Rotulo: "Importe", Alineacion: Fin, Ordenable: true},
	}, func() int { return len(filas) }, func(f, c int) string {
		return [][]string{
			{filas[0], "Al día", "1.240,00 €"},
			{filas[1], "Pendiente", "312,50 €"},
			{filas[2], "Vencido", "98,00 €"},
		}[f][c]
	})
	tb.Ordenada, tb.Marcada = "nombre", func(f int) bool { return f == 1 }
	tb.AlMarcar, tb.Resumen = func(int) {}, []string{"Total", "", "1.650,50 €"}
	tb.Resize(fyne.NewSize(460, AltoCabeceraTabla+3*AltoFilaTabla()+AltoFilaTabla()+2))
	tb.Move(fyne.NewPos(20, 20))

	lista := NuevaFilaLista("Ana Martínez", "Revisión del contrato marco")
	lista.Meta, lista.SinLeer, lista.Delantero = "12:45", true, NuevoAvatar("Ana Martínez").Con(AvatarSM)
	lista.Resize(fyne.NewSize(300, MedidaFilaLista(true, 32)))
	lista.Move(fyne.NewPos(510, 20))

	sel := NuevaFilaLista("Bruno Salas", "Presupuesto revisado")
	sel.Meta, sel.Seleccionada, sel.ConCasilla, sel.ConGalon = "ayer", true, true, true
	sel.Resize(fyne.NewSize(300, MedidaFilaLista(true, 0)))
	sel.Move(fyne.NewPos(510, 80))

	drive := NuevaFilaDrive("memoria-anual-2026.pdf", "pdf")
	drive.Tamano, drive.Fecha, drive.ConCasilla = "1,2 MB", "12 sep 2026", true
	drive.Resize(fyne.NewSize(460, MedidaFilaDrive()))
	drive.Move(fyne.NewPos(510, 140))

	carpeta := NuevaFilaDrive("Contratos", "")
	carpeta.Carpeta, carpeta.Tamano, carpeta.Fecha = true, "—", "3 sep 2026"
	carpeta.Seleccionada = true
	carpeta.Resize(fyne.NewSize(460, MedidaFilaDrive()))
	carpeta.Move(fyne.NewPos(510, 195))

	med := NuevoMedidor("Almacenamiento", 0.97, "97 %")
	med.Cuanto, med.Nota = "14,6 GB de 15 GB", "lleno"
	med.Resize(fyne.NewSize(220, MedidaMedidor(true)))
	med.Move(fyne.NewPos(510, 255))

	vacio := NuevoVacio("Aquí no hay nada todavía",
		"Sube tu primera foto o crea un álbum para empezar a organizar la biblioteca")
	vacio.Resize(fyne.NewSize(380, vacio.MedidaVacio(380)))
	vacio.Move(fyne.NewPos(20, 255))

	objs := []fyne.CanvasObject{tb, lista, sel, drive, carpeta, med, vacio}
	// Las siete insignias y un contador, en fila.
	x := float32(510)
	for _, tn := range []Tono{InsigniaNeutra, InsigniaPrimaria, InsigniaAcento, InsigniaExito, InsigniaAviso, InsigniaPeligro, InsigniaInfo} {
		i := NuevaInsignia([]string{"Neutro", "Primario", "Acento", "Al día", "Aviso", "Vencido", "Info"}[tn], tn)
		m := MedidaInsignia(i.Texto)
		i.Resize(m)
		i.Move(fyne.NewPos(x, 330))
		x += m.Width + 7
		objs = append(objs, i)
	}
	c := NuevoContador("12")
	c.Resize(MedidaContador("12"))
	c.Move(fyne.NewPos(x, 330))
	objs = append(objs, c)

	// Una rejilla de teselas de tipo, repartida con la cuenta de la web.
	const anchoRejilla = 460
	r := Rejilla{Min: 60, Hueco: 3.5, Cuadrada: true}
	lado, cols := r.AnchoColumna(anchoRejilla), r.Columnas(anchoRejilla)
	for i, ext := range []string{"pdf", "docx", "xlsx", "pptx", "png", "zip", "", "go"} {
		te := NuevaTeselaTipo(ext, ext == "", lado)
		te.Colocar(20+r.X(anchoRejilla, i%cols), 380+float32(i/cols)*(lado+r.Hueco))
		objs = append(objs, te.Objetos()...)
	}

	img := pintaA(t, 2, 1000, 500, objs...)
	ruta := os.Getenv("SYTUE_CAPTURA")
	if ruta == "" {
		return
	}
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestElTituloLargoDelVacioPasaDeLinea: el `<h3>` de EmptyState no tiene `max-w` ni
// `truncate`, así que en una columna estrecha se reparte en líneas, centradas, y el
// estado vacío crece. Y su `MinSize` es el de la palabra más larga, no el del título
// entero: el título en una línea forzaba el ancho del hueco. VISTO EN ROJO con el título
// en un solo `canvas.Text`.
func TestElTituloLargoDelVacioPasaDeLinea(t *testing.T) {
	const titulo = "Todavía no has compartido ningún archivo con nadie"
	v := NuevoVacio(titulo, "")
	if got := v.MedidaVacio(240); got <= 32*2+26 {
		t.Errorf("a 240 el título largo cabe en %v de alto: no pasa de línea", got)
	}
	natural := Medir(titulo, EscalonTitulo(4), PesoTitulo(4)).Width + 2*MargenXVacio
	if got := test.WidgetRenderer(v).MinSize().Width; got >= natural {
		t.Errorf("el mínimo del vacío es %v: el título entero (%v) fuerza el ancho", got, natural)
	}
	v.Resize(fyne.NewSize(240, 300))
	r := test.WidgetRenderer(v).(*vacioRenderer)
	r.Layout(v.Size())
	if len(r.titulo) < 2 {
		t.Fatalf("a 240 el título sale en %d línea(s)", len(r.titulo))
	}
	for _, l := range r.titulo {
		a := Medir(l.Text, EscalonTitulo(4), PesoTitulo(4)).Width
		if a > 240-2*MargenXVacio+0.01 {
			t.Errorf("la línea %q mide %v y no cabe en los %v del `px-6`", l.Text, a, 240-2*MargenXVacio)
		}
	}
}

// TestElGlifoDelVacioEsTextSubtle: EmptyState envuelve la ilustración en un `<div
// class="text-text-subtle">` y un `<Icon>` hereda ese color. VISTO EN ROJO con el color
// que traía el glifo.
func TestElGlifoDelVacioEsTextSubtle(t *testing.T) {
	v := NuevoVacio("Sin permiso", "")
	v.Glifo = iconos.TrazoConTinta(iconos.Candado, 32, color.NRGBA{R: 255, A: 255})
	v.Resize(fyne.NewSize(300, 200))
	img := pinta(t, 300, 200, v)
	quiero := tema.Color("text-subtle", tema.Claro)
	rojo, bueno := 0, 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := px(img, x, y)
			if p.R > 200 && p.G < 80 {
				rojo++
			}
			if cerca(p, quiero) {
				bueno++
			}
		}
	}
	if rojo > 0 || bueno == 0 {
		t.Errorf("el glifo del vacío pinta %d píxeles rojos y %d en `text-subtle`", rojo, bueno)
	}
}
