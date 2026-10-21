// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package catalogo carga el catálogo de fuentes OFL y la biblioteca CC0 embebidos
// en el binario y los expone como tipos Go (PAGES-ENCARGO §13.2-13.3, A6).
package catalogo

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

//go:embed fuentes.json biblioteca.json fuentes biblioteca
var Embebido embed.FS

// Familia describe una familia tipográfica OFL del catálogo.
type Familia struct {
	ID      string                    `json:"id"`
	License string                    `json:"license"`
	Source  string                    `json:"source"`
	Version string                    `json:"version"`
	Scripts []string                  `json:"scripts"`
	Faces   []domain.FuenteCaraRender `json:"faces"`
}

// Foto describe una imagen CC0 de la biblioteca.
type Foto struct {
	ID         string            `json:"id"`
	Fichero    string            `json:"fichero"`
	SHA256     string            `json:"sha256"`
	Ancho      int               `json:"ancho"`
	Alto       int               `json:"alto"`
	Licencia   string            `json:"licencia"`
	Fuente     string            `json:"fuente"`
	Autor      string            `json:"autor,omitempty"`
	Etiquetas  []string          `json:"etiquetas,omitempty"`
	Alt        map[string]string `json:"alt,omitempty"`
	Verticales []string          `json:"verticales,omitempty"`
}

// Catalogo agrupa fuentes y biblioteca.
type Catalogo struct {
	Familias   []Familia
	Fotos      []Foto
	idFamilias map[string]struct{}
	idFotos    map[string]struct{}
	recursos   map[string]Recurso
}

type manifestoFuentes struct {
	Version  int       `json:"version"`
	Familias []Familia `json:"families"`
}

type manifestoBiblioteca struct {
	Version int    `json:"version"`
	Fotos   []Foto `json:"fotos"`
}

// CargarPorDefecto carga el catálogo embebido en el binario.
func CargarPorDefecto() (*Catalogo, error) {
	return Cargar(Embebido)
}

// Cargar lee los manifiestos embebidos desde fsys (normalmente embed.FS).
func Cargar(fsys fs.FS) (*Catalogo, error) {
	b, err := fs.ReadFile(fsys, "fuentes.json")
	if err != nil {
		return nil, fmt.Errorf("catalogo: fuentes.json: %w", err)
	}
	var mf manifestoFuentes
	if err := json.Unmarshal(b, &mf); err != nil {
		return nil, fmt.Errorf("catalogo: fuentes.json: %w", err)
	}
	b, err = fs.ReadFile(fsys, "biblioteca.json")
	if err != nil {
		return nil, fmt.Errorf("catalogo: biblioteca.json: %w", err)
	}
	var mb manifestoBiblioteca
	if err := json.Unmarshal(b, &mb); err != nil {
		return nil, fmt.Errorf("catalogo: biblioteca.json: %w", err)
	}
	c := &Catalogo{
		Familias:   mf.Familias,
		Fotos:      mb.Fotos,
		idFamilias: make(map[string]struct{}, len(mf.Familias)),
		idFotos:    make(map[string]struct{}, len(mb.Fotos)),
		recursos:   make(map[string]Recurso),
	}
	for _, f := range mf.Familias {
		c.idFamilias[f.ID] = struct{}{}
		if len(f.Faces) != len(f.Scripts) {
			return nil, fmt.Errorf("catalogo: caras incompletas: %s", f.ID)
		}
		for i, script := range f.Scripts {
			name, err := c.cargarRecurso(fsys, "fuentes/"+f.ID+"/"+script+".woff2", "font/woff2", "")
			if err != nil {
				return nil, err
			}
			if f.Faces[i].File != name {
				return nil, fmt.Errorf("catalogo: SHA de fuente no coincide: %s/%s", f.ID, script)
			}
		}
	}
	for i, f := range mb.Fotos {
		c.idFotos[f.ID] = struct{}{}
		name, err := c.cargarRecurso(fsys, "biblioteca/"+f.Fichero, "image/jpeg", f.SHA256)
		if err != nil {
			return nil, err
		}
		c.Fotos[i].Fichero = name
	}
	return c, nil
}

// TieneFamilia informa si el id de fuente existe.
func (c *Catalogo) TieneFamilia(id string) bool {
	_, ok := c.idFamilias[id]
	return ok
}

// TieneFoto informa si el id de foto existe.
func (c *Catalogo) TieneFoto(id string) bool {
	_, ok := c.idFotos[id]
	return ok
}

// IDs devuelve los ids del catálogo para el validador IR.
func (c *Catalogo) IDs() (fuentes, biblioteca []string) {
	fuentes = make([]string, 0, len(c.idFamilias))
	for id := range c.idFamilias {
		fuentes = append(fuentes, id)
	}
	biblioteca = make([]string, 0, len(c.idFotos))
	for id := range c.idFotos {
		biblioteca = append(biblioteca, id)
	}
	return fuentes, biblioteca
}

// Recurso identifica bytes canónicos verificados, compartidos por todos los sitios.
type Recurso struct {
	Nombre    string
	Clave     string
	Tipo      string
	SHA256    string
	Contenido []byte
}

func (c *Catalogo) cargarRecurso(fsys fs.FS, origen, tipo, esperado string) (string, error) {
	b, err := fs.ReadFile(fsys, origen)
	if err != nil {
		return "", fmt.Errorf("catalogo: recurso %s: %w", origen, err)
	}
	if len(b) == 0 || (tipo == "font/woff2" && (len(b) < 4 || string(b[:4]) != "wOF2")) {
		return "", fmt.Errorf("catalogo: recurso inválido: %s", origen)
	}
	digest := sha256.Sum256(b)
	hash := hex.EncodeToString(digest[:])
	if esperado != "" && hash != esperado {
		return "", fmt.Errorf("catalogo: SHA no coincide: %s", origen)
	}
	nombre := hash[:32] + path.Ext(origen)
	grupo := "biblioteca"
	if strings.HasSuffix(nombre, ".woff2") {
		grupo = "fuentes"
	}
	c.recursos[nombre] = Recurso{nombre, "shared/catalogo/" + grupo + "/" + nombre, tipo, hash, b}
	return nombre, nil
}

// Recursos devuelve el índice de recursos existentes en el binario.
func (c *Catalogo) Recursos() map[string]Recurso { return c.recursos }

// ClaveRecurso solo resuelve nombres presentes y verificados en el catálogo.
func (c *Catalogo) ClaveRecurso(nombre string) (string, bool) {
	if c == nil {
		return "", false
	}
	rec, ok := c.recursos[nombre]
	return rec.Clave, ok
}
