// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// # Favoritos y etiquetas de lo que está en la nube
//
// La estrella y las etiquetas libres (DR-09) de un elemento de una unidad de la nube se
// guardan EN ESTE EQUIPO, en un fichero propio ([Remotas.GuardarMarcasEn]), no en la
// cuenta de la persona: por cuenta y por el identificador ESTABLE que da el proveedor
// (`nube.Elemento.Estable`: el ID en Google Drive y OneDrive, el `oc:fileid` de Nextcloud
// y ownCloud), así que renombrarlo o moverlo, aquí o en la web del proveedor, no los
// pierde. Un WebDAV que no da ninguno se apunta por su ruta, que se lleva al renombrar o
// mover desde aquí (desde fuera, se pierde).
//
// No se usa el «destacado» de Google Drive (`starred`) ni nada parecido de los demás: sería
// escribir en la cuenta de la persona (lo vería en la web y en sus otros dispositivos) por
// algo que ha marcado en esta app, y solo Google lo tiene; aquí la estrella es de Sytue
// Office y de este equipo, como las del disco.
//
// Salen con las del disco en «Favoritos», en las etiquetas que el diálogo ofrece y en la
// búsqueda por etiqueta. Lo que se sabe de cada uno (nombre, carpeta, tamaño) es lo de la
// última vez que se listó: se pone al día cada vez que se ve su carpeta. Sin fichero de
// marcas (el valor cero), una unidad no admite favoritos ni etiquetas, como antes.

// marca es lo que se guarda de un elemento de la nube.
type marca struct {
	Cuenta string `json:"cuenta"`
	// Clave es su identificador estable o, si el proveedor no da ninguno, su ID.
	Clave      string    `json:"clave"`
	ID         string    `json:"id"`
	Padre      string    `json:"padre,omitempty"`
	Nombre     string    `json:"nombre"`
	Carpeta    bool      `json:"carpeta,omitempty"`
	Tipo       string    `json:"tipo,omitempty"`
	Tamano     int64     `json:"tamano,omitempty"`
	Modificado time.Time `json:"modificado,omitzero"`
	Favorito   bool      `json:"favorito,omitempty"`
	Etiquetas  []string  `json:"etiquetas,omitempty"`
}

func (m marca) vacia() bool { return !m.Favorito && len(m.Etiquetas) == 0 }

// claveDe es el identificador por el que se apunta un elemento.
func claveDe(e nube.Elemento) string {
	if e.Estable != "" {
		return e.Estable
	}
	return e.ID
}

// GuardarMarcasEn hace que los elementos de las nubes admitan favoritos y etiquetas,
// guardados en ruta (un JSON en los ajustes de la edición). Lee los que ya hubiera.
func (r *Remotas) GuardarMarcasEn(ruta string) error {
	var guardado struct {
		Marcas []marca `json:"marcas"`
	}
	raw, err := os.ReadFile(ruta)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(raw, &guardado); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rutaMarcas, r.marcas = ruta, guardado.Marcas
	return nil
}

// guardarMarcas escribe las marcas; con r.mu tomado.
func (r *Remotas) guardarMarcas() error {
	if r.rutaMarcas == "" {
		return nil
	}
	r.marcas = slices.DeleteFunc(r.marcas, marca.vacia)
	raw, err := json.MarshalIndent(struct {
		Marcas []marca `json:"marcas"`
	}{r.marcas}, "", "  ")
	if err != nil {
		return err
	}
	return biblioteca.EscribirAtomico(r.rutaMarcas, append(raw, '\n'))
}

// indiceMarca es la marca de un elemento, o -1; con r.mu tomado.
func (r *Remotas) indiceMarca(cuenta string, e nube.Elemento) int {
	clave := claveDe(e)
	return slices.IndexFunc(r.marcas, func(m marca) bool { return m.Cuenta == cuenta && m.Clave == clave })
}

// cambiarMarca pone al día (o crea) la marca de un elemento ya listado y la guarda.
func (r *Remotas) cambiarMarca(id string, cambiar func(*marca)) error {
	k, err := r.conocidoDe(id)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rutaMarcas == "" {
		return errRemoto
	}
	i := r.indiceMarca(k.Cuenta, k.E)
	if i < 0 {
		r.marcas = append(r.marcas, marca{Cuenta: k.Cuenta, Clave: claveDe(k.E)})
		i = len(r.marcas) - 1
	}
	m := &r.marcas[i]
	m.ponerAlDia(k.E, k.Padre)
	cambiar(m)
	return r.guardarMarcas()
}

// ponerAlDia copia lo que se sabe ahora del elemento.
func (m *marca) ponerAlDia(e nube.Elemento, padre string) bool {
	nueva := marca{
		Cuenta: m.Cuenta, Clave: m.Clave, ID: e.ID, Padre: padre, Nombre: e.Nombre, Carpeta: e.Carpeta, Tipo: e.Tipo,
		Tamano: e.Tamano, Modificado: e.Modificado, Favorito: m.Favorito, Etiquetas: m.Etiquetas,
	}
	if m.ID == nueva.ID && m.Padre == nueva.Padre && m.Nombre == nueva.Nombre && m.Carpeta == nueva.Carpeta && m.Tipo == nueva.Tipo &&
		m.Tamano == nueva.Tamano && m.Modificado.Equal(nueva.Modificado) {
		return false
	}
	*m = nueva
	return true
}

// alListar pone al día las marcas de lo que se acaba de listar (un renombrado o un
// movimiento hecho fuera, en un proveedor con identificador estable); con r.mu tomado.
func (r *Remotas) alListar(cuenta, carpeta string, es []nube.Elemento) {
	if len(r.marcas) == 0 {
		return
	}
	cambio := false
	for _, e := range es {
		if i := r.indiceMarca(cuenta, e); i >= 0 {
			cambio = r.marcas[i].ponerAlDia(e, carpeta) || cambio
		}
	}
	if cambio {
		_ = r.guardarMarcas()
	}
}

// trasCambiar lleva las marcas de un elemento que se ha renombrado o movido desde aquí:
// en un proveedor sin identificador estable (WebDAV), su ruta y las de lo que tiene dentro.
func (r *Remotas) trasCambiar(cuenta string, viejo, nuevo nube.Elemento, padre string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.marcas) == 0 {
		return
	}
	cambio := false
	for i := range r.marcas {
		m := &r.marcas[i]
		if m.Cuenta != cuenta {
			continue
		}
		switch {
		case m.Clave == claveDe(viejo):
			m.Clave = claveDe(nuevo)
			m.ponerAlDia(nuevo, padre)
			cambio = true
		case viejo.Estable == "" && viejo.Carpeta && cuelgaDe(m.Clave, viejo.ID) && m.Clave != viejo.ID:
			// Lo de dentro de una carpeta de WebDAV cambia de ruta con ella.
			resto := strings.TrimPrefix(m.Clave, strings.TrimSuffix(viejo.ID, "/"))
			m.Clave, m.ID = strings.TrimSuffix(nuevo.ID, "/")+resto, strings.TrimSuffix(nuevo.ID, "/")+resto
			if cuelgaDe(m.Padre, viejo.ID) {
				m.Padre = strings.TrimSuffix(nuevo.ID, "/") + strings.TrimPrefix(m.Padre, strings.TrimSuffix(viejo.ID, "/"))
			}
			cambio = true
		}
	}
	if cambio {
		_ = r.guardarMarcas()
	}
}

// soltarMarcas quita las de un elemento que se ha tirado (y, en WebDAV, las de lo que
// tenía dentro, por su ruta).
func (r *Remotas) soltarMarcas(cuenta string, e nube.Elemento) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.marcas)
	clave := claveDe(e)
	r.marcas = slices.DeleteFunc(r.marcas, func(m marca) bool {
		return m.Cuenta == cuenta && (m.Clave == clave || (e.Estable == "" && e.Carpeta && cuelgaDe(m.Clave, e.ID)))
	})
	if len(r.marcas) != n {
		_ = r.guardarMarcas()
	}
}

// marcasDe son la estrella y las etiquetas de un id de la fachada (del disco o de una
// unidad).
func (r *Remotas) marcasDe(id string) (favorito bool, etiquetas []string) {
	if !EsRemoto(id) {
		a := r.Archivos // el disco, con su propio cerrojo (no el de Remotas)
		ruta, err := a.ruta(id)
		if err != nil {
			return false, nil
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		return slices.Contains(a.ind.Favoritos, ruta), slices.Clone(a.ind.Etiquetas[ruta])
	}
	k, err := r.conocidoDe(id)
	if err != nil {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := r.indiceMarca(k.Cuenta, k.E); i >= 0 {
		return r.marcas[i].Favorito, slices.Clone(r.marcas[i].Etiquetas)
	}
	return false, nil
}

// llevarMarcas pone a la copia de algo que se mueve entre unidades la estrella y las
// etiquetas del original.
func (r *Remotas) llevarMarcas(ctx context.Context, de, a string) {
	fav, ls := r.marcasDe(de)
	if !fav && len(ls) == 0 {
		return
	}
	k := api.DriveFile
	if c, err := r.Folder(ctx, a); err == nil && c.ID != "" {
		k = api.DriveFolder
	}
	if fav {
		_ = r.SetFavorite(ctx, k, a, true)
	}
	if len(ls) > 0 {
		_ = r.PonerEtiquetas(ctx, a, ls)
	}
}

// ----- Lo que ve la pantalla -----

// SetFavorite pone o quita la estrella; en una unidad, en este equipo (ver arriba).
func (r *Remotas) SetFavorite(ctx context.Context, k api.DriveKind, id string, si bool) error {
	if !EsRemoto(id) {
		return r.Archivos.SetFavorite(ctx, k, id, si)
	}
	return r.cambiarMarca(id, func(m *marca) { m.Favorito = si })
}

// Etiquetas son las de un elemento; en una unidad, las guardadas en este equipo.
func (r *Remotas) Etiquetas(ctx context.Context, id string) ([]string, error) {
	if !EsRemoto(id) {
		return r.Archivos.Etiquetas(ctx, id)
	}
	k, err := r.conocidoDe(id)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rutaMarcas == "" {
		return nil, errRemoto
	}
	if i := r.indiceMarca(k.Cuenta, k.E); i >= 0 {
		return slices.Clone(r.marcas[i].Etiquetas), nil
	}
	return nil, nil
}

// PonerEtiquetas sustituye las de un elemento; vacías, se las quita.
func (r *Remotas) PonerEtiquetas(ctx context.Context, id string, ls []string) error {
	if !EsRemoto(id) {
		return r.Archivos.PonerEtiquetas(ctx, id, ls)
	}
	limpias := limpiarEtiquetas(ls)
	return r.cambiarMarca(id, func(m *marca) { m.Etiquetas = limpias })
}

// TodasLasEtiquetas son las del disco y las de las nubes, sin repetir y en orden.
func (r *Remotas) TodasLasEtiquetas(ctx context.Context) []string {
	todas := r.Archivos.TodasLasEtiquetas(ctx)
	r.mu.Lock()
	for _, m := range r.marcas {
		todas = append(todas, m.Etiquetas...)
	}
	r.mu.Unlock()
	return limpiarEtiquetas(todas)
}

// marcadas son las marcas de las cuentas que siguen conectadas que cumplen quiere, ya
// apuntadas como listadas (para abrirlas, entrar en ellas o llevarlas a otra unidad).
func (r *Remotas) marcadas(quiere func(marca) bool) []conocido {
	vivas := map[string]bool{}
	for _, c := range r.cuentas() {
		vivas[c.ID] = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []conocido
	for _, m := range r.marcas {
		if !vivas[m.Cuenta] || !quiere(m) {
			continue
		}
		id := idRemoto(m.Cuenta, m.ID)
		k, ok := r.conocidos[id]
		if !ok {
			e := nube.Elemento{ID: m.ID, Nombre: m.Nombre, Carpeta: m.Carpeta, Tipo: m.Tipo, Tamano: m.Tamano, Modificado: m.Modificado}
			if m.Clave != m.ID {
				e.Estable = m.Clave
			}
			k = conocido{Cuenta: m.Cuenta, E: e, Padre: m.Padre}
			r.conocidos[id] = k
		}
		out = append(out, k)
	}
	return out
}

// Favorites son los del disco y, detrás, los de las nubes.
func (r *Remotas) Favorites(ctx context.Context) (api.Compartidos, error) {
	out, err := r.Archivos.Favorites(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range r.marcadas(func(m marca) bool { return m.Favorito }) {
		if k.E.Carpeta {
			c := r.carpetaRemota(k)
			out = append(out, api.Compartido{File: api.File{
				ID: c.ID, FolderID: *c.ParentID, Name: c.Name, UpdatedAt: c.UpdatedAt,
				Path: r.rutaVista(k),
			}, IsDir: true})
			continue
		}
		f := r.ficheroRemoto(k)
		f.Path = r.rutaVista(k)
		out = append(out, api.Compartido{File: f})
	}
	return out, nil
}

// rutaVista es dónde está lo de una nube en «Favoritos» y en la búsqueda:
// «/<unidad>/<nombre>», para que la vista diga en qué unidad está. No es una ruta del
// disco: abrirlo lo baja a la caché (File).
func (r *Remotas) rutaVista(k conocido) string {
	unidad := k.Cuenta
	if c, ok := r.cuenta(k.Cuenta); ok && c.Rotulo != "" {
		unidad = c.Rotulo
	}
	return "/" + strings.ReplaceAll(unidad, "/", "∕") + "/" + k.nombre()
}

// Search busca en el disco y, en las nubes, lo que lleva una etiqueta que coincide (por
// nombre no se busca en la nube: habría que recorrerla entera).
func (r *Remotas) Search(ctx context.Context, q string) (api.Children, error) {
	out, err := r.Archivos.Search(ctx, q)
	if err != nil {
		return out, err
	}
	q = buscar.Plegar(strings.TrimSpace(q))
	if q == "" {
		return out, nil
	}
	con := func(m marca) bool {
		return slices.ContainsFunc(m.Etiquetas, func(l string) bool { return strings.Contains(buscar.Plegar(l), q) })
	}
	for _, k := range r.marcadas(con) {
		if k.E.Carpeta {
			c := r.carpetaRemota(k)
			c.Path = r.rutaVista(k)
			out.Folders = append(out.Folders, c)
		} else {
			f := r.ficheroRemoto(k)
			f.Path = r.rutaVista(k)
			out.Files = append(out.Files, f)
		}
	}
	return out, nil
}
