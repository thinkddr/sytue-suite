// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
)

// ImportCSV crea una tabla nueva a partir de un CSV con cabecera, leído y tipado con las
// reglas del servicio (dominio.LeerCSV: UTF-8, separador, límites e inferencia de tipos).
func (r *Repository) ImportCSV(ctx context.Context, base, nombre string, datos []byte) (api.BasesTable, error) {
	t, err := dominio.LeerCSV(bytes.NewReader(datos))
	if err != nil {
		return api.BasesTable{}, errorDe(err)
	}
	for _, c := range t.Campos {
		if _, err := dominio.ValidarEntradaCampo(c); err != nil {
			return api.BasesTable{}, errorDe(err)
		}
	}
	// La tabla, sus campos y sus filas se anotan como UNA importación.
	externo := ctx
	ctx = callar(ctx)
	tabla, err := r.CreateTable(ctx, base, nombre)
	if err != nil {
		return api.BasesTable{}, err
	}
	campos := make([]api.BasesField, 0, len(t.Campos))
	for pos, c := range t.Campos {
		entrada := map[string]any{"table": tabla.ID, "position": pos, "name": c.Name, "kind": c.Kind}
		f, err := crear(ctx, r, "field-create", "field", entrada, func(context.Context) (string, api.BasesField, error) {
			f := api.BasesField{ID: uuid.NewString(), TableID: tabla.ID, Name: c.Name, Kind: string(c.Kind), Position: pos}
			return f.ID, f, nil
		})
		if err != nil {
			return api.BasesTable{}, err
		}
		campos = append(campos, f)
	}
	inicio := time.Now().UTC()
	for i, fila := range t.Filas {
		m := map[string]json.RawMessage{}
		for j, v := range fila {
			if v != nil {
				m[campos[j].ID] = v
			}
		}
		if err := dominio.TamanoDatos(m); err != nil {
			return api.BasesTable{}, errorDe(err)
		}
		// Una fila más tarde que la anterior: la tabla sale en el orden del fichero.
		cuando := inicio.Add(time.Duration(i) * time.Microsecond)
		entrada := map[string]any{"table": tabla.ID, "row": i, "data": m}
		if _, err := crear(ctx, r, "record-import", "record", entrada, func(context.Context) (string, api.BasesRecord, error) {
			rec := api.BasesRecord{
				ID: uuid.NewString(), TableID: tabla.ID, Data: valores(m), Links: map[string][]string{}, Rev: 1,
				CreatedBy: r.scope.Subject, UpdatedBy: r.scope.Subject, CreatedAt: cuando, UpdatedAt: cuando,
			}
			return rec.ID, rec, nil
		}); err != nil {
			return api.BasesTable{}, err
		}
	}
	tabla.Fields = campos
	r.resumir(externo, "table.import", tabla.ID, tabla.Name)
	return tabla, nil
}

// Export saca la tabla entera en `csv` o `json` con el formato del servicio
// (dominio.CabeceraExportada, FilaExportada, RegistroExportado): los enlaces con el
// texto de los registros enlazados y sus ids, y las celdas protegidas contra fórmulas.
func (r *Repository) Export(ctx context.Context, tabla, formato string) (io.ReadCloser, error) {
	if formato != "csv" && formato != "json" {
		return nil, errorDe(dominio.ErrInvalido)
	}
	t, err := leerUno[api.BasesTable](ctx, r, "table", tabla)
	if err != nil {
		return nil, err
	}
	regs, fs, err := r.leerTabla(ctx, tabla, "", api.BasesLectura{})
	if err != nil {
		return nil, err
	}
	m, err := r.mundo(ctx)
	if err != nil {
		return nil, err
	}
	campos := camposDe(fs)
	etiquetas := map[string]string{}
	for _, c := range campos {
		if c.Kind != dominio.TipoRelacion {
			continue
		}
		for _, rec := range regs {
			for _, id := range rec.Links[c.ID] {
				otro := m.registros[id]
				datos, _ := crudos(otro.Data)
				etiquetas[id] = dominio.EtiquetaDe(m.campos[otro.TableID], datos, id)
			}
		}
	}
	var b bytes.Buffer
	if formato == "json" {
		fuera := make([]dominio.RegistroExportado, len(regs))
		for i, rec := range regs {
			d := registroDe(rec)
			fuera[i] = dominio.RegistroExportado{Registro: d, LinkValues: dominio.ValoresDeEnlaces(campos, d, etiquetas)}
		}
		if err := json.NewEncoder(&b).Encode(map[string]any{"table": t.Name, "fields": campos, "records": fuera}); err != nil {
			return nil, err
		}
		return io.NopCloser(&b), nil
	}
	b.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&b)
	_ = w.Write(dominio.CabeceraExportada(campos))
	for _, rec := range regs {
		_ = w.Write(dominio.FilaExportada(campos, registroDe(rec), etiquetas))
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return io.NopCloser(&b), nil
}

var _ api.BasesImportData = (*Repository)(nil)
