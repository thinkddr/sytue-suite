// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// # Prueba de carga: los documentos de Docus, Calcus y Diapos (PLAN-EDICION-LOCAL §5.6)
//
// La ofimática no va por el motor de registros sino por la biblioteca (un JSON por
// documento y sus versiones al lado). 2.000 documentos de unas tres páginas, cada uno con
// 50 versiones guardadas, escritos con la biblioteca: apertura en frío en un proceso
// nuevo (leer la biblioteca y la primera página de la portada) y su memoria, guardar un
// documento abierto, abrir su contenido y su lista de versiones. Las cifras están en
// SYTUE-OFFICE.md:
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaDocumentos' -v -timeout 0 ./local/biblioteca/

const (
	documentosCarga = 2_000
	versionesCarga  = 50
	claveHTML       = "doc:html"
)

var personaCarga = biblioteca.Persona{ID: "instalacion-carga", Nombre: "Ana"}

// TestCargaDocumentos mide 2.000 documentos con 50 versiones.
func TestCargaDocumentos(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirDocumentosMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	dir := cargaprueba.Plantilla(t, "documentos-2000", func(dir string) error {
		return cargaprueba.EnMemoria(dir, func(dir string) error { return generarDocumentos(dir, documentosCarga, versionesCarga) })
	})
	veces := cargaprueba.Veces()
	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "leer la biblioteca", "primera pantalla")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	// Con la biblioteca abierta (una copia): abrir un documento distinto cada vez, ver
	// sus versiones y guardarlo cambiado.
	ctx := t.Context()
	b, err := biblioteca.Abrir(cargaprueba.Copia(t, dir), "docs", personaCarga)
	if err != nil {
		t.Fatal(err)
	}
	docs, _, err := b.ListDocuments(ctx, api.DocsFilter{Kind: api.DocText, Limit: 200})
	if err != nil || len(docs) < 4*veces {
		t.Fatal(len(docs), err)
	}
	bases := make([]api.DocContent, len(docs))
	abrirDoc := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		bases[i], err = b.DocumentContent(ctx, docs[i].ID, claveHTML)
		return err
	})
	versiones := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		vs, err := b.DocumentVersions(ctx, docs[i].ID)
		if err == nil && len(vs) != versionesCarga {
			err = fmt.Errorf("%d versiones", len(vs))
		}
		return err
	})
	guardar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		_, err := b.SaveDocumentChecked(ctx, docs[i].ID, claveHTML, bases[i].Value+"<p>Una frase más.</p>", bases[i], api.DocDot{}, true)
		return err
	})
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: "2.000 documentos, 50 versiones", Medida: "abrir en frío (biblioteca + portada)", Valor: abrir.String(), Umbral: "< 2 s", Cumple: abrir.Mediana() < 2000},
		cargaprueba.Fila{Carga: "2.000 documentos, 50 versiones", Medida: "memoria: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss)},
		cargaprueba.Fila{Carga: "2.000 documentos, 50 versiones", Medida: "abrir un documento", Valor: abrirDoc.String()},
		cargaprueba.Fila{Carga: "2.000 documentos, 50 versiones", Medida: "lista de versiones (50)", Valor: versiones.String()},
		cargaprueba.Fila{Carga: "2.000 documentos, 50 versiones", Medida: "guardar", Valor: guardar.String(), Umbral: "< 80 ms", Cumple: guardar.Mediana() < 80},
	)
}

// abrirDocumentosMidiendo es lo que hace Docus al arrancar, cronometrado: leer la
// biblioteca y pedir la primera página de la portada.
func abrirDocumentosMidiendo(dir string, r *cargaprueba.Reloj) (func(), error) {
	var b *biblioteca.Biblioteca
	err := r.Medir("leer la biblioteca", func() error {
		var err error
		b, err = biblioteca.Abrir(dir, "docs", personaCarga)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = r.Medir("primera pantalla", func() error {
		_, _, err := b.ListDocuments(context.Background(), api.DocsFilter{Kind: api.DocText, Limit: 50})
		return err
	})
	// La biblioteca sigue abierta hasta después de medir la memoria.
	return func() { runtime.KeepAlive(b) }, err
}

// generarDocumentos escribe n documentos de Docus con la biblioteca, cada uno guardado
// versiones veces con un cambio pequeño y una versión con nombre tras cada guardado.
func generarDocumentos(raiz string, n, versiones int) error {
	ctx := context.Background()
	b, err := biblioteca.Abrir(raiz, "docs", personaCarga)
	if err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(2026, 7)) //nolint:gosec // G404: datos de prueba repetibles
	for i := range n {
		d, _, err := b.CreateDocumentFromSource(ctx, api.DocText, fmt.Sprintf("Documento %d", i+1), "")
		if err != nil {
			return err
		}
		html := documentoRealista(r)
		base := api.DocContent{}
		for v := range versiones {
			html += fmt.Sprintf("<p>Revisión %d.</p>", v+1)
			if base, err = b.SaveDocumentChecked(ctx, d.ID, claveHTML, html, base, api.DocDot{}, true); err != nil {
				return fmt.Errorf("documento %d, guardado %d: %w", i, v, err)
			}
			if _, err := b.CreateDocumentVersion(ctx, d.ID, fmt.Sprintf("Versión %d", v+1)); err != nil {
				return err
			}
		}
	}
	return nil
}

var parrafos = []string{
	"El presente informe recoge los resultados del trimestre y las propuestas de mejora acordadas en la última reunión del equipo.",
	"Se observa un aumento sostenido de la demanda en los meses de verano, con picos que superan en un veinte por ciento la media anual.",
	"Para el próximo ejercicio se propone revisar los contratos de suministro y renegociar los plazos de entrega con los proveedores principales.",
	"La formación del personal sigue siendo una prioridad: se han impartido cuarenta horas de cursos y la valoración media ha sido de 4,5 sobre 5.",
	"Queda pendiente la decisión sobre la ampliación del almacén, que depende de la licencia municipal y de la financiación disponible.",
}

// documentoRealista es un documento de unas tres páginas (unos 8 KB de HTML): títulos,
// párrafos y alguna lista.
func documentoRealista(r *rand.Rand) string {
	var b strings.Builder
	for s := range 6 {
		fmt.Fprintf(&b, "<h2>Apartado %d</h2>", s+1)
		for range 8 {
			fmt.Fprintf(&b, "<p>%s</p>", parrafos[r.IntN(len(parrafos))])
		}
		if r.IntN(2) == 0 {
			b.WriteString("<ul><li>Revisar cifras</li><li>Enviar a dirección</li><li>Archivar</li></ul>")
		}
	}
	return b.String()
}
