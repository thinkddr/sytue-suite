// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// Docus, Calcus y Diapos: traer cada fichero de `docs/documents` sin repetirlo
// (repetir.go).

// resultado es cómo ha ido traer un documento.
type resultado int

const (
	docNuevo resultado = iota
	docRepetido
	docActualizado
	docAparte
	docBorrado
)

// documentos trae cada fichero de `docs/documents/` a la biblioteca de su app (Docus,
// Calcus o Diapos, por su extensión), con sus comentarios, sin repetir lo que ya se
// trajo: ver [repartidor.documento].
func (r *repartidor) documentos(es []entrada) ([]entrada, error) {
	bibs := map[string]*biblioteca.Biblioteca{}
	var resto []entrada
	p := r.parte("docs")
	for _, e := range es {
		producto, ok := productoDeDocumento(e.ruta)
		if !strings.HasPrefix(e.ruta, "docs/documents/") || !ok {
			resto = append(resto, e)
			continue
		}
		b := bibs[producto]
		if b == nil {
			var err error
			if b, err = biblioteca.Abrir(filepath.Join(r.d.Datos, "Documentos"), producto, r.d.Persona); err != nil {
				return append(resto, e), err
			}
			bibs[producto] = b
		}
		res, err := r.documento(b, producto, e)
		if err != nil {
			p.Fallidos++
			resto = append(resto, e)
			continue
		}
		switch res {
		case docNuevo:
			p.Importados++
		case docRepetido:
			p.Repetidos++
		case docActualizado:
			p.Actualizados++
		case docAparte:
			p.Conservados++
		case docBorrado:
			p.Borrados++
		}
	}
	return resto, nil
}

func productoDeDocumento(ruta string) (string, bool) {
	switch strings.ToLower(path.Ext(ruta)) {
	case ".docx", ".odt", ".md", ".html", ".txt":
		return "docs", true
	case ".xlsx", ".ods", ".csv":
		return "sheets", true
	case ".pptx", ".odp":
		return "slides", true
	}
	return "", false
}

// origenDeDocumento es el origen (`SourceRef`) de un documento traído: su ruta en la
// copia. Con él se reconoce aunque se pierda la memoria.
func origenDeDocumento(ruta string) string { return "sytue:" + ruta }

// documento trae el fichero e de la copia a b. Si ya se trajo (la memoria, o su origen):
// igual, o de una copia más vieja, se deja; cambiado en Sytue, entra como versión nueva
// del mismo documento si aquí no se ha tocado, y si se ha tocado, aparte, como un
// documento nuevo «<título> (de Sytue, <fecha>)», que es el que sigue a Sytue desde
// entonces; quitado aquí, no vuelve.
func (r *repartidor) documento(b *biblioteca.Biblioteca, producto string, e entrada) (resultado, error) {
	datos, err := e.leer()
	if err != nil {
		return docNuevo, err
	}
	nombre := path.Base(e.ruta)
	huella := huellaCorta(datos)
	apuntar := func(id string) error {
		a, err := r.huellaDeDocumento(b, id)
		if err == nil {
			r.mem.Documentos[e.ruta] = &docTraido{Producto: producto, ID: id, traido: traido{S: huella, A: a, C: r.copia}}
		}
		return err
	}
	m := r.mem.Documentos[e.ruta]
	if m == nil || m.Producto != producto {
		// Sin memoria: el que lleva su origen, si lo hay, ya estaba.
		if d, ok, err := b.DocumentBySource(r.ctx, origenDeDocumento(e.ruta)); err != nil || ok {
			if err == nil {
				err = apuntar(d.ID)
			}
			return docRepetido, err
		}
		id, err := r.importarDocumento(b, nombre, datos, "", origenDeDocumento(e.ruta))
		if err != nil {
			return docNuevo, err
		}
		return docNuevo, apuntar(id)
	}
	d, vivo := docVivo(r.ctx, b, m.ID)
	if !vivo {
		return docBorrado, nil // la persona lo tiró (o lo borró del todo): no vuelve
	}
	if huella == m.S || masVieja(r.copia, m.C) {
		return docRepetido, nil
	}
	ahora, err := r.huellaDeDocumento(b, d.ID)
	if err != nil {
		return docRepetido, err
	}
	if ahora == m.A {
		if err := r.ponerAlDia(b, d, nombre, datos); err != nil {
			return docActualizado, err
		}
		return docActualizado, apuntar(d.ID)
	}
	titulo := fmt.Sprintf("%s (de Sytue, %s)", tituloDe(nombre, ""), cmp.Or(r.copia, r.hoy))
	id, err := r.importarDocumento(b, nombre, datos, titulo, "")
	if err != nil {
		return docAparte, err
	}
	return docAparte, apuntar(id)
}

// docVivo es el documento id de b, si sigue fuera de la papelera.
func docVivo(ctx context.Context, b *biblioteca.Biblioteca, id string) (api.Document, bool) {
	d, _, err := b.GetDocument(ctx, id)
	return d, err == nil
}

// tituloDe es el título de un documento traído: el nombre de su fichero sin la extensión
// o, si se queda en nada, el que diga el propio fichero.
func tituloDe(nombre, delFichero string) string {
	return cmp.Or(strings.TrimSpace(strings.TrimSuffix(nombre, path.Ext(nombre))), delFichero)
}

// huellaDeDocumento es la huella del contenido del documento id: si cambia, la persona
// lo ha tocado.
func (r *repartidor) huellaDeDocumento(b *biblioteca.Biblioteca, id string) (string, error) {
	c, err := b.DocumentContent(r.ctx, id, api.DocContentKey(b.Tipo()))
	if err != nil {
		return "", err
	}
	return huellaCorta([]byte(c.Value)), nil
}

// importarDocumento crea en b un documento con lo del fichero, con sus comentarios, y
// devuelve su id. titulo vacío es el del fichero; fuente, su origen.
func (r *repartidor) importarDocumento(b *biblioteca.Biblioteca, nombre string, datos []byte, titulo, fuente string) (string, error) {
	imp, err := biblioteca.Importar(nombre, datos, true)
	if err != nil {
		return "", err
	}
	if imp.Kind != b.Tipo() {
		return "", fmt.Errorf("traer: %s no es de %s", nombre, b.Tipo())
	}
	doc, _, err := b.CreateDocumentFromSource(r.ctx, imp.Kind, cmp.Or(titulo, tituloDe(nombre, imp.Title)), fuente)
	if err != nil {
		return "", err
	}
	contenido := imp.Content
	if len(imp.Comments) > 0 {
		if anclados, err := b.ImportComments(r.ctx, doc.ID, imp.Comments); err == nil {
			contenido = imp.ContenidoConMarks(anclados)
		}
	}
	if contenido == "" {
		return doc.ID, nil
	}
	_, err = b.SaveDocumentContent(r.ctx, doc.ID, api.DocContentKey(imp.Kind), contenido, api.DocDot{}, 1, false, doc.Seq)
	if err != nil {
		_ = b.DeleteDocument(r.ctx, doc.ID)
	}
	return doc.ID, err
}

// ponerAlDia pone en d lo del fichero como versión nueva: guarda antes lo de ahora en su
// historial, como hace «Restaurar» de Docus, escribe encima lo de Sytue (con los
// comentarios que aún no tenía) y lo guarda también como versión, con la fecha de la
// copia.
func (r *repartidor) ponerAlDia(b *biblioteca.Biblioteca, d api.Document, nombre string, datos []byte) error {
	imp, err := biblioteca.Importar(nombre, datos, true)
	if err != nil {
		return err
	}
	if imp.Kind != b.Tipo() {
		return fmt.Errorf("traer: %s no es de %s", nombre, b.Tipo())
	}
	if _, err := b.CreateDocumentVersion(r.ctx, d.ID, "Antes de traer de Sytue ("+r.hoy+")"); err != nil {
		return err
	}
	clave := api.DocContentKey(imp.Kind)
	antes, err := b.DocumentContent(r.ctx, d.ID, clave)
	if err != nil {
		return err
	}
	contenido := imp.Content
	if len(imp.Comments) > 0 {
		nuevos, conocidos, err := comentariosNuevos(r.ctx, b, d.ID, imp.Comments)
		if err != nil {
			return err
		}
		if anclados, err := b.ImportComments(r.ctx, d.ID, nuevos); err == nil {
			contenido = imp.ContenidoConMarks(append(conocidos, anclados...))
		}
	}
	reloj := antes.MaxLamport + 1
	_, err = b.SaveDocumentContent(r.ctx, d.ID, clave, contenido, api.DocDot{Counter: reloj}, reloj, imp.Kind == api.DocText, antes.Seq)
	if err != nil {
		return err
	}
	_, err = b.CreateDocumentVersion(r.ctx, d.ID, "De Sytue ("+cmp.Or(r.copia, r.hoy)+")")
	return err
}

// comentariosNuevos son, de los comentarios de un fichero, los que el documento id aún
// no tiene (un anclado, por su marca, que es su id; otro, por su texto), con sus
// respuestas; y conocidos, las marcas de los anclados que ya tiene, que se quedan en el
// contenido.
func comentariosNuevos(ctx context.Context, b *biblioteca.Biblioteca, id string, cs []api.ImportadoComentario) (nuevos []api.ImportadoComentario, conocidos []string, err error) {
	tiene, err := b.DocumentComments(ctx, id, true)
	if err != nil {
		return nil, nil, err
	}
	ids, textos := map[string]bool{}, map[string]bool{}
	for _, c := range tiene {
		ids[c.ID] = true
		if c.ParentID == "" {
			textos[strings.TrimSpace(c.Body)] = true
		}
	}
	for _, c := range cs {
		switch {
		case c.ParentID != "":
			nuevos = append(nuevos, c) // ImportComments solo coge las de un comentario nuevo
		case c.Anchored:
			if u, err := uuid.Parse(c.ID); err == nil && ids[u.String()] {
				conocidos = append(conocidos, c.ID)
			} else {
				nuevos = append(nuevos, c)
			}
		case !textos[strings.TrimSpace(c.Text)]:
			nuevos = append(nuevos, c)
		}
	}
	return nuevos, conocidos, nil
}
