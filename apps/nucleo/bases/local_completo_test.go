// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func codigo(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code()
	}
	return ""
}

func nombres(t *testing.T, p api.BasesRecordPage, campo string) []string {
	t.Helper()
	out := make([]string, 0, len(p.Records))
	for _, r := range p.Records {
		s, _ := r.Data[campo].(string)
		out = append(out, s)
	}
	return out
}

// Lo que en Sytue Office no tenía camino local (Anexo A, B-03 y B-04), por la fachada
// que usan las pantallas y con las reglas del servicio.
func TestBasesLocalHaceLoQueHaciaElServicio(t *testing.T) {
	ctx := t.Context()
	b, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	base, _ := b.CreateBase(ctx, "Tienda")
	pedidos, err := b.CreateTable(ctx, base.ID, "Pedidos")
	if err != nil {
		t.Fatal(err)
	}
	lineas, _ := b.CreateTable(ctx, base.ID, "Líneas")
	nombre, _ := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Cliente", Kind: "text"})
	importe, _ := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Importe", Kind: "number"})
	estado, err := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Estado", Kind: "select", Choices: []string{"nuevo", "hecho"}})
	if err != nil || importe.Position != 1 || estado.Position != 2 {
		t.Fatal("posiciones:", importe.Position, estado.Position, err)
	}
	if _, err := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "", Kind: "text"}); codigo(err) != "invalid" {
		t.Fatal("un campo sin nombre:", err)
	}

	// Validación al escribir.
	if _, err := b.CreateRecord(ctx, pedidos.ID, map[string]any{importe.ID: "mucho"}, nil); codigo(err) != "invalid" {
		t.Fatal("un número que no lo es:", err)
	}
	if _, err := b.CreateRecord(ctx, pedidos.ID, map[string]any{estado.ID: "otro"}, nil); codigo(err) != "invalid" {
		t.Fatal("una opción que no existe:", err)
	}
	ana, err := b.CreateRecord(ctx, pedidos.ID, map[string]any{nombre.ID: "Ana", importe.ID: 5, estado.ID: "nuevo"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bea, _ := b.CreateRecord(ctx, pedidos.ID, map[string]any{nombre.ID: "Bea", importe.ID: 20, estado.ID: "hecho"}, nil)
	_, _ = b.CreateRecord(ctx, pedidos.ID, map[string]any{nombre.ID: "Cleo", importe.ID: 15}, nil)

	// Fórmula: el doble del importe, recalculada al cambiar la expresión.
	doble, err := b.CreateField(ctx, pedidos.ID, api.BasesNewField{Name: "Doble", Kind: "formula", Formula: "{" + importe.ID + "}*2"})
	if err != nil {
		t.Fatal("fórmula:", err)
	}
	if r, _ := b.GetRecord(ctx, bea.ID); r.Data[doble.ID] != 40.0 {
		t.Fatal("la fórmula no se calcula:", r.Data[doble.ID])
	}
	if err := b.SetFieldFormula(ctx, doble.ID, "{"+importe.ID+"}*3"); err != nil {
		t.Fatal(err)
	}
	if r, _ := b.GetRecord(ctx, bea.ID); r.Data[doble.ID] != 60.0 {
		t.Fatal("la fórmula no se recalcula:", r.Data[doble.ID])
	}
	if err := b.SetFieldFormula(ctx, doble.ID, "{00000000-0000-4000-8000-000000000000}"); codigo(err) != "invalid" {
		t.Fatal("una referencia a un campo ajeno:", err)
	}
	if _, err := b.WriteCell(ctx, ana.ID, doble.ID, api.BasesCell{Value: 1}, "op-x"); codigo(err) != "invalid" {
		t.Fatal("se ha escrito una fórmula a mano:", err)
	}

	// Vistas, búsqueda y orden de un momento, con paginación.
	vista, err := b.CreateView(ctx, pedidos.ID, "Grandes", "grid", api.BasesViewConfig{
		Filters: []api.BasesFilter{{Field: importe.ID, Op: "gt", Value: 10}}, Sort: []api.BasesSort{{Field: importe.ID, Dir: "desc"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := b.ListRecords(ctx, pedidos.ID, vista.ID, "", 0)
	if got := nombres(t, p, nombre.ID); err != nil || !slices.Equal(got, []string{"Bea", "Cleo"}) {
		t.Fatal("vista:", got, err)
	}
	if _, err := b.UpdateView(ctx, vista.ID, "Pequeños", api.BasesViewConfig{Filters: []api.BasesFilter{{Field: importe.ID, Op: "lt", Value: 10}}}); err != nil {
		t.Fatal(err)
	}
	if p, _ := b.ListRecords(ctx, pedidos.ID, vista.ID, "", 0); !slices.Equal(nombres(t, p, nombre.ID), []string{"Ana"}) {
		t.Fatal("la vista no cambia:", nombres(t, p, nombre.ID))
	}
	if _, err := b.UpdateView(ctx, vista.ID, "Mal", api.BasesViewConfig{Filters: []api.BasesFilter{{Field: importe.ID, Op: "contains", Value: 1}}}); codigo(err) != "invalid" {
		t.Fatal("un operador que no es del tipo:", err)
	}
	var todos []string
	cursor := ""
	for {
		p, err := b.ListRecordsCon(ctx, pedidos.ID, "", cursor, 1, api.BasesLectura{Sort: importe.ID + ":desc"})
		if err != nil {
			t.Fatal(err)
		}
		todos = append(todos, nombres(t, p, nombre.ID)...)
		if cursor = p.NextCursor; cursor == "" {
			break
		}
	}
	if !slices.Equal(todos, []string{"Bea", "Cleo", "Ana"}) {
		t.Fatal("de uno en uno con orden:", todos)
	}
	if p, _ := b.ListRecordsCon(ctx, pedidos.ID, "", "", 0, api.BasesLectura{Q: "LEO"}); !slices.Equal(nombres(t, p, nombre.ID), []string{"Cleo"}) {
		t.Fatal("búsqueda:", nombres(t, p, nombre.ID))
	}

	// Opciones: una retirada no rompe lo escrito.
	if err := b.SetFieldChoices(ctx, estado.ID, []string{"hecho", "cancelado"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteCell(ctx, ana.ID, estado.ID, api.BasesCell{Value: "nuevo", Expected: "nuevo"}, "op-1"); err != nil {
		t.Fatal("reescribir una opción retirada:", err)
	}
	res, err := b.WriteCell(ctx, ana.ID, estado.ID, api.BasesCell{Value: "cancelado", Expected: "hecho"}, "op-2")
	if err != nil || !res.Conflict || res.Record.Data[estado.ID] != "cancelado" {
		t.Fatal("conflicto:", res, err)
	}

	// Reordenar: todos los campos o es un conflicto.
	if err := b.ReorderFields(ctx, pedidos.ID, []string{importe.ID, nombre.ID}); codigo(err) != "rev_conflict" {
		t.Fatal("reordenar sin todos:", err)
	}
	orden := []string{doble.ID, estado.ID, importe.ID, nombre.ID}
	if err := b.ReorderFields(ctx, pedidos.ID, orden); err != nil {
		t.Fatal(err)
	}
	if err := b.ReorderFields(ctx, pedidos.ID, []string{nombre.ID, importe.ID, estado.ID, doble.ID}); err != nil {
		t.Fatal(err)
	}
	if err := b.ReorderFields(ctx, pedidos.ID, orden); err != nil {
		t.Fatal(err)
	}
	ts, _ := b.ListTables(ctx, base.ID)
	got := make([]string, 0, len(ts[0].Fields))
	for _, f := range ts[0].Fields {
		got = append(got, f.ID)
	}
	if !slices.Equal(got, orden) {
		t.Fatal("orden de los campos (volver a uno anterior también vale):", got)
	}

	// Enlaces, agregados y la integridad al borrar.
	pedido, _ := b.CreateField(ctx, lineas.ID, api.BasesNewField{Name: "Pedido", Kind: "link", LinkTableID: pedidos.ID})
	linea, err := b.CreateRecord(ctx, lineas.ID, nil, map[string][]string{pedido.ID: {bea.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateRecord(ctx, lineas.ID, nil, map[string][]string{pedido.ID: {linea.ID}}); codigo(err) != "invalid" {
		t.Fatal("un enlace a otra tabla:", err)
	}
	suma, err := b.CreateField(ctx, lineas.ID, api.BasesNewField{Name: "Suma", Kind: "formula", Formula: "SUM({" + pedido.ID + "." + importe.ID + "})"})
	if err != nil {
		t.Fatal("agregado:", err)
	}
	if r, _ := b.GetRecord(ctx, linea.ID); r.Data[suma.ID] != 20.0 {
		t.Fatal("el agregado no sale:", r.Data[suma.ID])
	}
	if _, err := b.WriteCell(ctx, bea.ID, importe.ID, api.BasesCell{Value: 25, Expected: 20}, "op-3"); err != nil {
		t.Fatal(err)
	}
	if r, _ := b.GetRecord(ctx, linea.ID); r.Data[suma.ID] != 25.0 {
		t.Fatal("el agregado no se recalcula a través del enlace:", r.Data[suma.ID])
	}
	if err := b.DeleteRecord(ctx, bea.ID, false); codigo(err) != "referenced" {
		t.Fatal("borrar algo enlazado:", err)
	}
	if err := b.DeleteRecord(ctx, bea.ID, true); err != nil {
		t.Fatal(err)
	}
	if r, _ := b.GetRecord(ctx, linea.ID); len(r.Links[pedido.ID]) != 0 {
		t.Fatal("el enlace sigue:", r.Links)
	}

	// Comentarios: hilos de un nivel y lápida.
	raiz, err := b.CreateComment(ctx, ana.ID, "  ¿Pagado?  ", "")
	if err != nil || raiz.Body != "¿Pagado?" {
		t.Fatal(raiz, err)
	}
	resp, _ := b.CreateComment(ctx, ana.ID, "Sí", raiz.ID)
	if _, err := b.CreateComment(ctx, ana.ID, "x", resp.ID); codigo(err) != "invalid" {
		t.Fatal("respuesta a una respuesta:", err)
	}
	if err := b.DeleteComment(ctx, raiz.ID); err != nil {
		t.Fatal(err)
	}
	cs, _ := b.ListComments(ctx, ana.ID)
	if len(cs) != 2 || !cs[0].Deleted || cs[0].Body != "" || cs[1].Body != "Sí" {
		t.Fatal("lápida:", cs)
	}
	_ = b.DeleteComment(ctx, resp.ID)
	if cs, _ := b.ListComments(ctx, ana.ID); len(cs) != 0 {
		t.Fatal("la lápida sin respuestas sigue:", cs)
	}

	// Exportar CSV: enlaces con su texto y sus ids, y celdas protegidas.
	_, _ = b.WriteCell(ctx, ana.ID, nombre.ID, api.BasesCell{Value: "=1+1"}, "op-4")
	rc, err := b.Export(ctx, pedidos.ID, "csv")
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := io.ReadAll(rc)
	filas, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(crudo, []byte("\xef\xbb\xbf")))).ReadAll()
	if err != nil || len(filas) != 3 || filas[0][3] != "Cliente" || !slices.Contains(filas[1], "'=1+1") {
		t.Fatal("csv:", filas, err)
	}
	rc, _ = b.Export(ctx, pedidos.ID, "json")
	var js struct {
		Table   string            `json:"table"`
		Records []json.RawMessage `json:"records"`
	}
	if err := json.NewDecoder(rc).Decode(&js); err != nil || js.Table != "Pedidos" || len(js.Records) != 2 {
		t.Fatal("json:", js, err)
	}
}

// Importar un CSV crea la tabla con los tipos inferidos y las filas en su orden.
func TestBasesLocalImportaCSV(t *testing.T) {
	ctx := t.Context()
	b, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	base, _ := b.CreateBase(ctx, "Importada")
	tabla, err := b.ImportCSV(ctx, base.ID, "Gente", []byte("Nombre;Edad;Alta\nAna;30;2024-01-02\nBea;;2025-05-06\n"))
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, 0, len(tabla.Fields))
	for _, f := range tabla.Fields {
		kinds = append(kinds, f.Kind)
	}
	if !slices.Equal(kinds, []string{"text", "number", "date"}) {
		t.Fatal("tipos:", kinds)
	}
	p, _ := b.ListRecords(ctx, tabla.ID, "", "", 0)
	if got := nombres(t, p, tabla.Fields[0].ID); !slices.Equal(got, []string{"Ana", "Bea"}) || p.Records[0].Data[tabla.Fields[1].ID] != 30.0 {
		t.Fatal("filas:", got, p.Records[0].Data)
	}
	if _, err := b.ImportCSV(ctx, base.ID, "Mal", []byte("\xff\xfe")); codigo(err) != "invalid" {
		t.Fatal("un CSV que no es UTF-8:", err)
	}
}

// Los adjuntos guardan sus bytes por el puerto y sobreviven a reabrir; la miniatura de
// una imagen se hace al pedirla.
func TestBasesLocalAdjuntos(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	b, cerrar := abrirLocal(t, dir)
	base, _ := b.CreateBase(ctx, "Fichas")
	tabla, _ := b.CreateTable(ctx, base.ID, "Cosas")
	campo, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Fotos", Kind: "attachment"})
	texto, _ := b.CreateField(ctx, tabla.ID, api.BasesNewField{Name: "Nombre", Kind: "text"})
	rec, _ := b.CreateRecord(ctx, tabla.ID, map[string]any{texto.ID: "Lámpara"}, nil)
	img := image.NewRGBA(image.Rect(0, 0, 900, 600))
	for y := range 600 {
		for x := range 900 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var foto bytes.Buffer
	_ = png.Encode(&foto, img)
	if _, err := b.UploadAttachment(ctx, rec.ID, texto.ID, "a.png", foto.Bytes()); codigo(err) != "invalid" {
		t.Fatal("adjuntar a un campo que no es de adjuntos:", err)
	}
	if _, err := b.UploadAttachment(ctx, rec.ID, campo.ID, "virus.exe", []byte("MZ...")); codigo(err) != "invalid" {
		t.Fatal("un ejecutable:", err)
	}
	adj, err := b.UploadAttachment(ctx, rec.ID, campo.ID, "lampara.png", foto.Bytes())
	if err != nil || adj.Mime != "image/png" || !adj.HasThumb {
		t.Fatal("subir:", adj, err)
	}
	cerrar()

	b, cerrar = abrirLocal(t, dir)
	defer cerrar()
	r, err := b.GetRecord(ctx, rec.ID)
	if err != nil || len(r.Attachments[campo.ID]) != 1 || r.Attachments[campo.ID][0].Name != "lampara.png" {
		t.Fatal("la ficha tras reabrir:", r.Attachments, err)
	}
	rc, err := b.DownloadAttachment(ctx, rec.ID, adj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if datos, _ := io.ReadAll(rc); !bytes.Equal(datos, foto.Bytes()) {
		t.Fatal("los bytes no vuelven iguales")
	}
	rc, err = b.DownloadThumb(ctx, rec.ID, adj.ID)
	if err != nil {
		t.Fatal(err)
	}
	mini, _, err := image.Decode(rc)
	if err != nil || mini.Bounds().Dx() > 512 {
		t.Fatal("miniatura:", err)
	}
	if err := b.DeleteAttachment(ctx, rec.ID, adj.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := b.GetRecord(ctx, rec.ID); len(r.Attachments) != 0 {
		t.Fatal("el adjunto borrado sigue:", r.Attachments)
	}
	if _, err := b.DownloadAttachment(ctx, rec.ID, adj.ID); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatal("bajar uno borrado:", err)
	}
}
