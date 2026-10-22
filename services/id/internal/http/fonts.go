// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"io/fs"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
)

// Reutiliza las fuentes canónicas SIL OFL del núcleo; no distribuye una segunda copia.
func handleFont(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/fonts/")
	switch name {
	case "Figtree-Regular.ttf", "Figtree-Medium.ttf", "Figtree-SemiBold.ttf", "Figtree-Bold.ttf", "OFL.txt":
	default:
		http.NotFound(w, r)
		return
	}
	body, err := fs.ReadFile(fuentes.FS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	mime := "font/ttf"
	if name == "OFL.txt" {
		mime = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}
