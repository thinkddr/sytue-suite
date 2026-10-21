// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"io"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// # Resumir con IA
//
// `ResumenIA.tsx`: «¿de qué va este fichero?» sin abrirlo. ES DE SOLO LECTURA a
// propósito: el resumen se mira y se copia, no se guarda ni toca el fichero.
//
// La web lo hace en su BFF (`pages/api/ai.ts`) para que el fichero no baje al navegador.
// Aquí no hay BFF: la app lee el fichero de Drive y le manda a Sytue AI el mismo cuerpo
// —las mismas dos instrucciones, `kind: file`, `modo: calidad` y como mucho 200 KB—
// directamente con el token de la persona (`api.IA`). El texto sale a Scaleway
// Generative APIs (UE) SIN anonimizar, igual que desde la web: no es una salida nueva.
//
// ponytail: SOLO ficheros de texto, como la web: un PDF o un .docx necesitan un
// extractor que la suite todavía no tiene.

// TopeResumen es lo que se lee del fichero: un resumen no mejora con más.
const TopeResumen = 200 * 1024

// PlazoResumen es el de la web: el modo calidad tarda del orden de quince segundos y el
// servicio le da 120.
const PlazoResumen = 130 * time.Second

// instruccionesResumen son las de la web, literales: son ÓRDENES al modelo y no rótulos,
// y la web las manda en castellano sea cual sea el idioma de la sesión.
var instruccionesResumen = map[string]string{
	"resumir": "Resume este documento en un párrafo, en su mismo idioma. Di lo que dice, no lo que trata de. Conserva los datos concretos (fechas, cifras, nombres). Devuelve solo el resumen.",
	"puntos":  "Resume este documento en como mucho seis puntos, en su mismo idioma. Un punto por idea, empezando cada uno con «- ». Conserva los datos concretos. Devuelve solo la lista.",
}

// EsTexto es el `previewKind(item) === 'text'` de la web: los ficheros de los que hay
// texto que leer.
func EsTexto(el Elemento) bool {
	return !el.Carpeta && el.ID != "" && extensionesTexto[Extension(el.Nombre)]
}

// dialogoResumen es el estado del diálogo.
type dialogoResumen struct {
	p       *Pantalla
	el      Elemento
	d       *ui.Dialogo
	cuerpo  *fyne.Container
	accion  string
	bloques []ia.Bloque
	fallo   bool
	ocupado bool
	copiado bool
	// vuelta descarta la respuesta de una petición anterior al cambiar de acción, que
	// si no puede llegar la segunda y pisarla la primera.
	vuelta int
	ancho  float32
}

// pedirResumen abre el diálogo y pide el resumen en párrafo.
func (p *Pantalla) pedirResumen(el Elemento) *dialogoResumen {
	if p.win == nil || p.svc == nil || p.IA == nil || !EsTexto(el) {
		return nil
	}
	dr := &dialogoResumen{p: p, el: el, accion: "resumir", cuerpo: container.NewStack()}
	dr.ancho = AnchoCuerpoDialogo(ui.DialogoMD, p.explor.Ventana)
	dr.d = ui.NuevoDialogo(p.tr.T("ai.summaryTitle", nil), dr.cuerpo).Con(ui.DialogoMD, nil)
	dr.d.RotuloCerrar = p.tr.T("ui.close", nil)
	dr.pedir()
	dr.d.Mostrar(p.win.Canvas())
	return dr
}

// pedir lanza la petición de la acción actual.
func (dr *dialogoResumen) pedir() {
	dr.vuelta++
	vuelta, accion := dr.vuelta, dr.accion
	dr.ocupado, dr.fallo, dr.copiado, dr.bloques = true, false, false, nil
	dr.pintar()
	dr.p.enSegundoPlano(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), PlazoResumen)
		defer cancelar()
		texto, err := Resumir(ctx, dr.p.svc, dr.p.IA, dr.el, accion)
		enHilo(func() {
			if vuelta != dr.vuelta {
				return
			}
			dr.ocupado = false
			texto = strings.TrimSpace(texto)
			if err != nil || texto == "" {
				dr.fallo = true
			} else {
				dr.bloques = ia.BloquesDe(texto)
			}
			dr.pintar()
		})
	})
}

// Resumir lee como mucho TopeResumen bytes del fichero y le pide a la IA la `accion`
// («resumir» o «puntos»). Un fichero vacío es un fallo, como en la web (422).
func Resumir(ctx context.Context, svc *api.Drive, cliente *api.IA, el Elemento, accion string) (string, error) {
	r, err := svc.Download(ctx, el.ID)
	if err != nil {
		return "", err
	}
	datos, err := io.ReadAll(io.LimitReader(r, TopeResumen))
	_ = r.Close()
	if err != nil {
		return "", err
	}
	// Un byte suelto sale como el rombo de sustitución en vez de tumbar la petición
	// entera, que es el `fatal: false` del `TextDecoder` de la web.
	texto := strings.TrimSpace(strings.ToValidUTF8(string(datos), "�"))
	if texto == "" {
		return "", io.ErrUnexpectedEOF
	}
	if _, ok := instruccionesResumen[accion]; !ok {
		accion = "resumir"
	}
	return cliente.Asistir(ctx, api.PeticionIA{
		Accion: accion, Instruccion: instruccionesResumen[accion], Contexto: texto,
		Tipo: "file", Modo: "calidad",
	})
}

// AltoEsperaResumen es el `py-8` de la espera con la rueda de 20.
const AltoEsperaResumen float32 = 20 + 2*8*0.25*tema.Rem

// pintar rehace el cuerpo con el estado de ahora.
func (dr *dialogoResumen) pintar() {
	v := dr.p.variante()
	t := func(k string) string { return dr.p.tr.T(k, nil) }
	nombre := ui.Texto(dr.el.Nombre, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	ui.Truncar(nombre, dr.el.Nombre, tema.TextoSM, ui.Normal, dr.ancho)

	// Párrafo o puntos: la misma pregunta con dos formas de respuesta, y cambiarla no
	// obliga a cerrar y volver a abrir.
	forma := func(clave, accion string) *ui.Boton {
		b := ui.NuevoBoton(t(clave), func() {
			if dr.accion != accion {
				dr.accion = accion
				dr.pedir()
			}
		})
		variante := ui.Secundario
		if dr.accion == accion {
			variante = ui.Primario
		}
		b = b.Con(variante, ui.TallaSM)
		b.Inerte = dr.ocupado
		return b
	}
	formas := container.New(ui.Fila{Hueco: tema.Espacio(2)}, forma("ai.asParagraph", "resumir"), forma("ai.asBullets", "puntos"))

	var dentro fyne.CanvasObject
	switch {
	case dr.ocupado:
		texto := lineaTexto(ui.Texto(t("ai.working"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), tema.TextoSM)
		dentro = container.New(&altoFijo{alto: AltoEsperaResumen},
			container.NewCenter(container.New(ui.Fila{Hueco: tema.Espacio(2)}, ui.NuevaRueda(20, "text", v), texto)))
	case dr.fallo:
		dentro = ui.NuevoParrafo(t("ai.failed"), tema.TextoSM, ui.Normal, "danger-text", v)
	default:
		dentro = dr.respuesta()
	}

	// La botonera va en el PIE del diálogo, como en la web: con su filete, a todo el
	// ancho en el teléfono y a la vista aunque el resumen sea largo. «Cerrar» primero y
	// «Copiar», la acción, a la derecha.
	botones := []fyne.CanvasObject{ui.NuevoBoton(t("preview.close"), func() { dr.d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)}
	if !dr.ocupado && !dr.fallo {
		clave := "ai.copy"
		if dr.copiado {
			clave = "ai.copied"
		}
		copiar := ui.NuevoBoton(t(clave), func() {
			// El texto PLANO, sin los asteriscos: lo que se pega en un correo tiene que
			// leerse, no llevar marcadores dentro.
			fyne.CurrentApp().Clipboard().SetContent(ia.TextoPlano(dr.bloques))
			dr.copiado = true
			dr.pintar()
		}).Con(ui.Secundario, ui.TallaMD).ConIcono(iconos.Tic, 16)
		botones = append(botones, copiar)
	}
	dr.d.CambiarPie(container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, botones...))

	dr.cuerpo.Objects = []fyne.CanvasObject{container.New(&pilaSeparada{
		separas: []float32{0, tema.Espacio(3), tema.Espacio(3)}, ancho: &dr.ancho,
	}, lineaTexto(nombre, tema.TextoSM), formas, dentro)}
	dr.cuerpo.Refresh()
	dr.d.Recolocar()
}

// respuesta es `flex flex-col gap-2 text-sm`: un párrafo por bloque —los títulos en
// seminegrita, los puntos con su viñeta y `gap-2`—.
func (dr *dialogoResumen) respuesta() fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(dr.bloques))
	for _, b := range dr.bloques {
		switch b.Tipo {
		case ia.Punto:
			vineta := ui.NuevoRotulo("•", tema.TextoSM, ui.Normal, "text")
			hueco := tema.Espacio(2)
			resto := dr.ancho - vineta.MinSize().Width - hueco
			filas = append(filas, container.New(ui.Fila{Hueco: hueco}, vineta, ui.ParrafoRico(b.Trozos, tema.Regular, tema.TextoSM, resto)))
		case ia.H1, ia.H2, ia.H3:
			filas = append(filas, ui.ParrafoRico(b.Trozos, tema.SemiNegra, tema.TextoSM, dr.ancho))
		default:
			filas = append(filas, ui.ParrafoRico(b.Trozos, tema.Regular, tema.TextoSM, dr.ancho))
		}
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)
}
