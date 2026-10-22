// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

// emit escribe la respuesta: JSON crudo con -o json, o una tabla alineada por
// defecto. header y rows solo se usan en modo tabla.
func emit(c *Config, raw []byte, header []string, rows [][]string) error {
	if c.Output == "json" {
		return renderJSON(os.Stdout, raw)
	}
	renderTable(os.Stdout, header, rows)
	return nil
}

// renderJSON reindenta el cuerpo crudo. Si no es JSON (p. ej. logs en texto),
// lo escribe tal cual.
func renderJSON(w io.Writer, raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		_, err := w.Write(raw)
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderTable(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if len(header) > 0 {
		fmt.Fprintln(tw, strings.Join(header, "\t"))
	}
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

// dash sustituye cadenas vacías por "-" para que las tablas no queden con
// huecos ambiguos.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// zeroAs formatea un entero cuyo CERO no es una cantidad sino un centinela: el
// techo del autoescalado en 0 significa «automático», no «cero pods». Imprimirlo
// como 0 haría que la ficha del proyecto mintiera.
func zeroAs(n int, sentinel string) string {
	if n <= 0 {
		return sentinel
	}
	return strconv.Itoa(n)
}
