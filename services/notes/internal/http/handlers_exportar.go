// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Exportar e importar un cuaderno: llevarse las notas fuera del sistema y traerlas.
//
// Sytue Notepads era el único servicio de la suite con contenido de usuario que no
// dejaba sacarlo: Contactos exporta vCard y CSV, Documentos doce formatos,
// Archivos la descarga directa y Correo el `.eml` original. Aquí solo se podía a
// mano, nota a nota, desde la pantalla.
//
// No es comodidad: el artículo 20 del RGPD da derecho a recibir los propios datos
// «en un formato estructurado, de uso común y lectura mecánica», y para una suite
// que se vende como soberana ese derecho hay que poder ejercerlo sin pedir nada a
// nadie. Los seis formatos cumplen las tres condiciones; el nativo —HTML— cumple
// además la que el artículo no exige: que al abrirlo se vea el cuaderno.
package noteshttp

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// ExportNotebook responde el cuaderno y sus notas como fichero. Sin `?format=`
// sale el nativo, que es el HTML.
func (h *Handlers) ExportNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The notebook id is not a valid UUID."))
		return
	}
	// `.MD`, `.md` y `.markdown` son el MISMO Markdown: la diferencia está en el
	// teclado de quien escribe y en con qué programa lo tiene asociado, no en el
	// contenido. Y el vacío no es un error: es el formato nativo.
	formato, ok := domain.FormatoDe(r.URL.Query().Get("format"))
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Unsupported export format. Use md, markdown, txt, html, onex or pdf.",
		))
		return
	}
	nb, notas, err := h.store.ExportNotebook(r.Context(), id, sub(r))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}

	// `?note=<uuid>` exporta UNA nota del cuaderno en vez del cuaderno entero.
	// Se filtra aquí y no en el store porque la verja es la misma —quien puede
	// leer el cuaderno puede leer sus notas— y así ninguna consulta se duplica.
	if raw := r.URL.Query().Get("note"); raw != "" {
		noteID, err := uuid.Parse(raw)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The note id is not a valid UUID."))
			return
		}
		var sola []domain.Note
		for _, n := range notas {
			if n.ID == noteID {
				sola = append(sola, n)
				break
			}
		}
		if len(sola) == 0 {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		// El título del fichero es el de la NOTA: «Mudanza.md» para una nota que
		// se llama «Presupuesto» no lo reconoce nadie en la carpeta de descargas.
		nb.Title = domain.TituloDeNota(sola[0])
		notas = sola
	}

	// DESCARGAR Y VER SON DOS COSAS. Por defecto se descarga: es un fichero que
	// alguien se lleva, y sin `attachment` el navegador pinta el Markdown como
	// texto pelado en la pestaña y hay que guardarlo a mano. Pero con `?view=1` se
	// sirve `inline`, que es lo que deja abrir el PDF en el visor del navegador o
	// el HTML como página sin pasar por el disco.
	//
	// Y `inline` NO se concede a cualquier formato: servir un HTML de contenido de
	// usuario en el origen de la API sería darle un sitio desde el que hablarle a
	// la sesión de quien lo abre. Lo decide domain.PuedeVerseEnLinea, no esta
	// función, para que el criterio esté junto a los formatos y no aquí suelto.
	disposicion := "attachment"
	if r.URL.Query().Get("view") == "1" && formato.PuedeVerseEnLinea() {
		disposicion = "inline"
	}
	w.Header().Set("Content-Type", formato.TipoMIME())
	w.Header().Set("Content-Disposition", disposicion+`; filename="`+domain.NombreDeFicheroExportado(nb.Title, formato)+`"`)
	// Aunque solo se sirva `inline` lo que no es HTML, la cabecera va SIEMPRE: si
	// mañana alguien amplía la lista de formatos visibles, el olvido aquí sería
	// silencioso. Un PDF con JavaScript dentro también abre en el visor.
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// La exportación es de AHORA: cachearla serviría el cuaderno de antes de la
	// última nota, que en un volcado de datos es peor que no tenerlo.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(domain.Exportar(nb, notas, formato))
}

// ImportNotebook crea un cuaderno a partir de un fichero `.onex`.
//
// No sobrescribe nada: crea un cuaderno NUEVO. Restaurar encima de lo que hay es
// como se pierde lo que había, y quien importa suele estar buscando justo lo
// contrario.
func (h *Handlers) ImportNotebook(w http.ResponseWriter, r *http.Request) {
	nb, notas, err := leerVolcado(w, r)
	if err != nil {
		return // leerVolcado ya respondió.
	}

	creado, puestas, descartadas, err := h.store.ImportNotebook(r.Context(), sub(r), nb, notas)
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	// `descartadas` viaja en la respuesta: una importación que se come notas en
	// silencio es peor que una que falla, porque el hueco se descubre semanas
	// después y ya no hay fichero al que volver.
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"notebook":    creado,
		"importadas":  puestas,
		"descartadas": descartadas,
	})
}

// PreviewNotebook lee un `.onex` y devuelve lo que ENTRARÍA, sin crear nada.
//
// Un volcado llega por correo, de una copia de hace meses o de otra instalación,
// y quien lo abre no sabe de qué cuaderno es ni cuántas notas trae. Importar para
// averiguarlo deja un cuaderno de sobra en la cuenta cada vez que alguien mira un
// fichero, y borrarlo después no es simétrico: se va a la papelera y cuenta para
// la retención.
//
// ES UN ENSAYO DE LA IMPORTACIÓN, no un volcado del fichero. Cada pieza pasa por
// el mismo `Normalize` que usará la importación de verdad, por dos motivos:
//
//   - Lo que se enseña es lo que va a entrar: mismo saneado, mismo recorte, las
//     mismas notas descartadas y contadas. Una vista previa que enseña más de lo
//     que luego se importa es una promesa rota.
//   - Es la frontera de confianza. El contenido viene de un fichero que puede
//     haber llegado de cualquiera: sin sanear, el navegador de quien lo mira
//     pintaría HTML que nadie de esta instalación ha revisado.
//
// Comparte límite y lectura con ImportNotebook a propósito: si esta ruta aceptara
// ficheros que la otra rechaza, sería una vista previa de algo que luego no se
// puede importar.
func (h *Handlers) PreviewNotebook(w http.ResponseWriter, r *http.Request) {
	nb, notas, err := leerVolcado(w, r)
	if err != nil {
		return // leerVolcado ya respondió.
	}
	entrada := nb.AImportar()
	if err := entrada.Normalize(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"The notebook in this backup is not valid.",
		))
		return
	}
	nb.Title, nb.Emoji, nb.Color = entrada.Title, entrada.Emoji, entrada.Color
	nb.Layout, nb.Visibility = entrada.Layout, entrada.Visibility

	limpias := make([]domain.Note, 0, len(notas))
	descartadas := 0
	for _, n := range notas {
		in := n.AImportar()
		if err := in.Normalize(entrada.Encrypted); err != nil {
			if entrada.Encrypted {
				httpx.Fail(w, r, traducir(domain.ErrEncryptedMigrationRequired))
				return
			}
			descartadas++
			continue
		}
		// Se devuelve la nota TAL COMO QUEDARÍA: lo que `Normalize` recortó o
		// saneó es lo que se pinta, no lo que traía el fichero.
		n.Kind, n.Title, n.Content, n.ContentCipher, n.Links =
			in.Kind, in.Title, in.Content, in.ContentCipher, in.Links
		n.Data = in.Data
		limpias = append(limpias, n)
	}
	// `descartadas` viaja también aquí, y ANTES de importar: es cuando quien mira
	// todavía puede decidir no hacerlo, o ir a arreglar el fichero.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"notebook":    nb,
		"notas":       limpias,
		"descartadas": descartadas,
	})
}

// leerVolcado es el trozo común de importar y previsualizar: leer el cuerpo con
// tope y traducir los dos motivos de rechazo. Responde el error él mismo y
// devuelve `err` para que quien llama solo tenga que volverse.
//
// EL LÍMITE DE TAMAÑO ES LO PRIMERO. El cuerpo llega de fuera y se lee entero en
// memoria para interpretarlo como JSON: sin tope, un fichero de un gigabyte es
// una forma trivial de tumbar el servicio, y aquí no hace falta más que un
// cuaderno grande. `MaxBytesReader` corta en el transporte, sin llegar a reservar
// lo que no se va a leer.
const maxNotebookBackupBody int64 = 32 << 20

func leerVolcado(w http.ResponseWriter, r *http.Request) (domain.Notebook, []domain.Note, error) {
	const maxVolcado = maxNotebookBackupBody

	datos, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxVolcado))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"The file could not be read, or it is larger than 32 MiB.",
		))
		return domain.Notebook{}, nil, err
	}

	nb, notas, err := domain.LeerNativo(datos)
	if err != nil {
		// Los dos motivos se dicen por separado: «esto no es un volcado nuestro»
		// —el caso de un .onex de OneNote, que es binario y comparte extensión— y
		// «es más nuevo que esta instalación» piden cosas distintas a quien lo
		// intenta. Un mensaje único los dejaría a los dos sin saber qué hacer.
		if errors.Is(err, domain.ErrVersionNoSoportada) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				"This backup was made by a newer version of Sytue Notepads.",
			))
			return domain.Notebook{}, nil, err
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"This file is not a Sytue Notepads backup (.onex).",
		))
		return domain.Notebook{}, nil, err
	}
	return nb, notas, nil
}
