// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//nolint:tagliatelle // Google People y Microsoft Graph usan estas claves camelCase fijas.
package agenda

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/emersion/go-vcard"
)

// Las etiquetas de Contacts en Google y en Outlook (K-05). Aquí una etiqueta es un grupo
// de su libreta (una vCard KIND:group con un MEMBER por contacto, por su UID), como en
// CardDAV. Google y Outlook no guardan grupos como vCard:
//
//   - Google tiene grupos de contactos (`contactGroups`) con sus miembros por su
//     `resourceName`: cada grupo de la persona (no los del sistema, «Mis contactos» o
//     «Destacados») es una etiqueta, y una etiqueta de aquí es un grupo allí;
//   - Outlook pone categorías en cada contacto (`categories`): cada categoría que lleva
//     algún contacto de la carpeta es una etiqueta («categoria:<nombre>»), y una
//     etiqueta de aquí pone su nombre en las categorías de sus contactos. Una etiqueta
//     sin nadie no existe en Outlook: la de aquí se queda (no se borra por no verla
//     allí).
//
// El sincronizador las trata como un objeto más, con una ETag hecha del nombre y de los
// miembros: cambia si cambia cualquiera de los dos. Los miembros pasan de los ids del
// proveedor a los UID de aquí y al revés con los [Enlaces] de la colección. Una etiqueta
// de aquí que se llama como una del proveedor que aún no se ha visto no baja como otra:
// al subir se juntan (los miembros de las dos) y baja ya juntada.

// prefijoCategoria es el del id de una etiqueta de Outlook.
const prefijoCategoria = "categoria:"

// prefijoGrupoGoogle es el de un grupo de contactos de Google.
const prefijoGrupoGoogle = "contactGroups/"

// etagGrupo es la ETag de una etiqueta: su nombre y sus miembros, sin orden. Una
// categoría de Outlook sin nadie lleva «vacia-»: no se ve allí y no por eso se borró.
func etagGrupo(nombre string, ids []string) string {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	h := sha256.Sum256([]byte(nombre + "\x00" + strings.Join(slices.Compact(ids), "\x00")))
	if len(ids) == 0 {
		return "vacia-" + hex.EncodeToString(h[:8])
	}
	return "g-" + hex.EncodeToString(h[:8])
}

// vcardGrupo es la etiqueta como la guarda Contacts: vCard 4.0 KIND:group.
func vcardGrupo(uid, nombre string, miembros []string) string {
	var l lineas
	l.poner("BEGIN", "", "VCARD")
	l.poner("VERSION", "", "4.0")
	l.poner("UID", "", "urn:uuid:"+textoICS(limpiarUID(uid)))
	l.poner("KIND", "", "group")
	l.poner("FN", "", textoICS(nombre))
	ms := slices.Clone(miembros)
	slices.Sort(ms)
	for _, m := range slices.Compact(ms) {
		if m != "" {
			l.poner("MEMBER", "", "urn:uuid:"+textoICS(m))
		}
	}
	l.poner("END", "", "VCARD")
	return l.b.String()
}

// leerGrupo da el nombre y los UID de los miembros de una etiqueta de aquí.
func leerGrupo(datos []byte) (string, []string, error) {
	card, err := vcard.NewDecoder(strings.NewReader(grupoV4(string(datos)))).Decode()
	if err != nil {
		return "", nil, err
	}
	nombre := strings.TrimSpace(card.Value(vcard.FieldFormattedName))
	if nombre == "" {
		return "", nil, errors.New("agenda: la etiqueta no tiene nombre")
	}
	var ms []string
	for _, f := range card[vcard.FieldMember] {
		if u := limpiarUID(f.Value); u != "" && !slices.Contains(ms, u) {
			ms = append(ms, u)
		}
	}
	return nombre, ms, nil
}

// nombresSinEnlazar son los nombres (en minúsculas) de las etiquetas de aquí que aún no
// están en el proveedor.
func nombresSinEnlazar(e Enlaces) map[string]bool {
	out := map[string]bool{}
	if e.SinEnlazar == nil {
		return out
	}
	for _, o := range e.SinEnlazar() {
		if !esGrupoVCard(o.Datos) {
			continue
		}
		if n, _, err := leerGrupo([]byte(o.Datos)); err == nil {
			out[strings.ToLower(n)] = true
		}
	}
	return out
}

func (e Enlaces) uid(id string) string {
	if e.UIDDe == nil {
		return ""
	}
	return e.UIDDe(id)
}

func (e Enlaces) id(uid string) string {
	if e.IDDe == nil {
		return ""
	}
	return e.IDDe(uid)
}

func (e Enlaces) conocidos(prefijo string) []string {
	if e.Conocidos == nil {
		return nil
	}
	var out []string
	for _, k := range e.Conocidos() {
		if strings.HasPrefix(k, prefijo) {
			out = append(out, k)
		}
	}
	return out
}

// idsDe son los ids del proveedor de los miembros que ya están allí.
func idsDe(e Enlaces, uids []string) []string {
	var out []string
	for _, u := range uids {
		if id := e.id(u); id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// ----- Google -----

type grupoGoogle struct {
	ResourceName string   `json:"resourceName,omitempty"`
	ETag         string   `json:"etag,omitempty"`
	Name         string   `json:"name,omitempty"`
	GroupType    string   `json:"groupType,omitempty"`
	Miembros     []string `json:"memberResourceNames,omitempty"`
	Metadata     *struct {
		Deleted bool `json:"deleted"`
	} `json:"metadata,omitempty"`
}

// Enlazar recibe los enlaces de la libreta antes de cada vuelta.
func (g *GooglePeople) Enlazar(_ Coleccion, e Enlaces) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enl = e
}

func (g *GooglePeople) enlaces() Enlaces {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.enl
}

// grupos son los grupos de la persona (no los del sistema) con sus miembros.
func (g *GooglePeople) grupos(ctx context.Context) ([]grupoGoogle, error) {
	var nombres []string
	q := url.Values{"pageSize": {"1000"}, "groupFields": {"name,groupType,metadata"}}
	for range 100 {
		var r struct {
			Grupos []grupoGoogle `json:"contactGroups"`
			Next   string        `json:"nextPageToken"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/contactGroups?"+q.Encode(), nil, nil, &r); err != nil {
			return nil, err
		}
		for _, x := range r.Grupos {
			if x.GroupType == "USER_CONTACT_GROUP" && (x.Metadata == nil || !x.Metadata.Deleted) && strings.HasPrefix(x.ResourceName, prefijoGrupoGoogle) {
				nombres = append(nombres, x.ResourceName)
			}
		}
		if r.Next == "" {
			break
		}
		q.Set("pageToken", r.Next)
	}
	var out []grupoGoogle
	for trozo := range slices.Chunk(nombres, 200) {
		q := url.Values{"resourceNames": trozo, "maxMembers": {"10000"}, "groupFields": {"name,groupType"}}
		var r struct {
			Respuestas []struct {
				Grupo *grupoGoogle `json:"contactGroup"`
			} `json:"responses"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, g.Base+"/contactGroups:batchGet?"+q.Encode(), nil, nil, &r); err != nil {
			return nil, err
		}
		for _, x := range r.Respuestas {
			if x.Grupo != nil && x.Grupo.ResourceName != "" {
				out = append(out, *x.Grupo)
			}
		}
	}
	slices.SortFunc(out, func(a, b grupoGoogle) int { return strings.Compare(a.ResourceName, b.ResourceName) })
	return out, nil
}

// grupo lee un grupo con sus miembros.
func (g *GooglePeople) grupo(ctx context.Context, id string) (grupoGoogle, error) {
	var x grupoGoogle
	err := g.api.pedir(ctx, http.MethodGet, g.Base+"/"+id+"?maxMembers=10000&groupFields=name,groupType", nil, nil, &x)
	return x, err
}

// uidPersona es el UID de aquí de un contacto de Google: el de clientData, el que se
// enlazó o el que le pone vcardDePersona.
func uidPersona(e Enlaces, rn string, vistos map[string]string) string {
	if u := vistos[rn]; u != "" {
		return u
	}
	if u := e.uid(rn); u != "" {
		return u
	}
	return "google-" + strings.TrimPrefix(rn, "people/")
}

// cambiosGrupos añade a ca los grupos que cambiaron y los que se borraron.
func (g *GooglePeople) cambiosGrupos(ctx context.Context, ca *Cambios, etagDe func(string) (string, bool), vistos map[string]string) error {
	gs, err := g.grupos(ctx)
	if errors.Is(err, ErrNoEsta) {
		return nil // sin grupos de contactos, sin etiquetas
	}
	if err != nil {
		return err
	}
	e := g.enlaces()
	libres := nombresSinEnlazar(e)
	conocidos := map[string]bool{}
	for _, k := range e.conocidos(prefijoGrupoGoogle) {
		conocidos[k] = true
	}
	for _, x := range gs {
		delete(conocidos, x.ResourceName)
		ca.Vistos = append(ca.Vistos, x.ResourceName)
		etag := etagGrupo(x.Name, x.Miembros)
		if viejo, ok := etagDe(x.ResourceName); ok && viejo == etag {
			continue
		}
		uid := e.uid(x.ResourceName)
		if uid == "" {
			if libres[strings.ToLower(x.Name)] {
				continue // una etiqueta de aquí con su nombre: se juntan al subirla
			}
			uid = "google-" + strings.TrimPrefix(x.ResourceName, prefijoGrupoGoogle)
		}
		var miembros []string
		for _, m := range x.Miembros {
			miembros = append(miembros, uidPersona(e, m, vistos))
		}
		ca.Puestos = append(ca.Puestos, Objeto{ID: x.ResourceName, ETag: etag, Datos: []byte(vcardGrupo(uid, x.Name, miembros))})
	}
	ca.Borrados = append(ca.Borrados, slices.Sorted(maps.Keys(conocidos))...)
	return nil
}

// leerGrupoGoogle es el objeto de un grupo.
func (g *GooglePeople) leerGrupoGoogle(ctx context.Context, id string) (Objeto, error) {
	x, err := g.grupo(ctx, id)
	if err != nil {
		return Objeto{}, err
	}
	e := g.enlaces()
	uid := cmp.Or(e.uid(id), "google-"+strings.TrimPrefix(id, prefijoGrupoGoogle))
	var miembros []string
	for _, m := range x.Miembros {
		miembros = append(miembros, uidPersona(e, m, nil))
	}
	return Objeto{ID: id, ETag: etagGrupo(x.Name, x.Miembros), Datos: []byte(vcardGrupo(uid, x.Name, miembros))}, nil
}

// miembrosGoogle cambia los miembros de un grupo.
func (g *GooglePeople) miembrosGoogle(ctx context.Context, id string, anadir, quitar []string) error {
	if len(anadir) == 0 && len(quitar) == 0 {
		return nil
	}
	cuerpo := map[string][]string{}
	if len(anadir) > 0 {
		cuerpo["resourceNamesToAdd"] = anadir
	}
	if len(quitar) > 0 {
		cuerpo["resourceNamesToRemove"] = quitar
	}
	return g.api.pedir(ctx, http.MethodPost, g.Base+"/"+id+"/members:modify", cuerpo, nil, nil)
}

// crearGrupo sube una etiqueta de aquí. Si allí ya hay un grupo con su nombre, se junta
// con él (se añaden los miembros de aquí; no se quita a nadie) y su ETag queda vacía: la
// vuelta siguiente baja el grupo juntado.
func (g *GooglePeople) crearGrupo(ctx context.Context, datos []byte) (Objeto, error) {
	nombre, uids, err := leerGrupo(datos)
	if err != nil {
		return Objeto{}, err
	}
	ids := idsDe(g.enlaces(), uids)
	gs, err := g.grupos(ctx)
	if err != nil && !errors.Is(err, ErrNoEsta) {
		return Objeto{}, err
	}
	for _, x := range gs {
		if strings.EqualFold(x.Name, nombre) {
			var faltan []string
			for _, id := range ids {
				if !slices.Contains(x.Miembros, id) {
					faltan = append(faltan, id)
				}
			}
			if err := g.miembrosGoogle(ctx, x.ResourceName, faltan, nil); err != nil {
				return Objeto{}, err
			}
			return Objeto{ID: x.ResourceName}, nil
		}
	}
	var nuevo grupoGoogle
	if err := g.api.pedir(ctx, http.MethodPost, g.Base+"/contactGroups", map[string]any{"contactGroup": map[string]string{"name": nombre}}, nil, &nuevo); err != nil {
		return Objeto{}, err
	}
	if nuevo.ResourceName == "" {
		return Objeto{}, errors.New("agenda: Google no dio el grupo creado")
	}
	if err := g.miembrosGoogle(ctx, nuevo.ResourceName, ids, nil); err != nil {
		// El grupo ya está: la vuelta siguiente lo baja tal como quedó.
		return Objeto{ID: nuevo.ResourceName}, nil //nolint:nilerr // creado sin miembros; sin ETag, la vuelta siguiente lo baja como quedó.
	}
	return Objeto{ID: nuevo.ResourceName, ETag: etagGrupo(nombre, ids)}, nil
}

// cambiarGrupo lleva a Google el nombre y los miembros de una etiqueta, si el grupo
// sigue como se vio (si no, ErrConflicto).
func (g *GooglePeople) cambiarGrupo(ctx context.Context, obj Objeto, datos []byte) (Objeto, error) {
	nombre, uids, err := leerGrupo(datos)
	if err != nil {
		return Objeto{}, err
	}
	x, err := g.grupo(ctx, obj.ID)
	if err != nil {
		return Objeto{}, err
	}
	if obj.ETag != "" && obj.ETag != etagGrupo(x.Name, x.Miembros) {
		return Objeto{}, ErrConflicto
	}
	if x.Name != nombre {
		cuerpo := map[string]any{"contactGroup": map[string]string{"name": nombre, "etag": x.ETag}, "updateGroupFields": "name"}
		if err := g.api.pedir(ctx, http.MethodPut, g.Base+"/"+obj.ID, cuerpo, nil, nil); err != nil {
			return Objeto{}, err
		}
	}
	quiero := idsDe(g.enlaces(), uids)
	var anadir, quitar []string
	for _, id := range quiero {
		if !slices.Contains(x.Miembros, id) {
			anadir = append(anadir, id)
		}
	}
	for _, id := range x.Miembros {
		if !slices.Contains(quiero, id) {
			quitar = append(quitar, id)
		}
	}
	if err := g.miembrosGoogle(ctx, obj.ID, anadir, quitar); err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: obj.ID, ETag: etagGrupo(nombre, quiero)}, nil
}

// borrarGrupo quita el grupo de Google, sin borrar a sus contactos.
func (g *GooglePeople) borrarGrupo(ctx context.Context, id, etag string) error {
	if etag != "" {
		x, err := g.grupo(ctx, id)
		if errors.Is(err, ErrNoEsta) {
			return nil
		}
		if err != nil {
			return err
		}
		if etag != etagGrupo(x.Name, x.Miembros) {
			return ErrConflicto
		}
	}
	err := g.api.pedir(ctx, http.MethodDelete, g.Base+"/"+id+"?deleteContacts=false", nil, nil, nil)
	if errors.Is(err, ErrNoEsta) {
		return nil
	}
	return err
}

// ----- Outlook -----

// Enlazar recibe los enlaces de la carpeta antes de cada vuelta.
func (g *GraphContactos) Enlazar(_ Coleccion, e Enlaces) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enl = e
}

func (g *GraphContactos) enlaces() Enlaces {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.enl
}

// categorias son las categorías de los contactos de la carpeta: por nombre, sus
// contactos; y por contacto, sus categorías.
func (g *GraphContactos) categorias(ctx context.Context, col Coleccion) (map[string][]string, map[string][]string, error) {
	porNombre, porContacto := map[string][]string{}, map[string][]string{}
	u := g.contactos(col) + "?" + url.Values{"$select": {"id,categories"}, "$top": {"500"}}.Encode()
	for range 1000 {
		var r struct {
			Value []struct {
				ID         string   `json:"id"`
				Categories []string `json:"categories"`
			} `json:"value"`
			Next string `json:"@odata.nextLink"`
		}
		if err := g.api.pedir(ctx, http.MethodGet, u, nil, nil, &r); err != nil {
			return nil, nil, err
		}
		for _, c := range r.Value {
			porContacto[c.ID] = c.Categories
			for _, n := range c.Categories {
				if n = strings.TrimSpace(n); n != "" && !slices.Contains(porNombre[n], c.ID) {
					porNombre[n] = append(porNombre[n], c.ID)
				}
			}
		}
		if r.Next == "" || !mismoHost(g.Base, r.Next) {
			break
		}
		u = r.Next
	}
	return porNombre, porContacto, nil
}

// uidCategoria es el UID de una categoría que aún no está aquí.
func uidCategoria(nombre string) string {
	h := sha256.Sum256([]byte(strings.ToLower(nombre)))
	return "graph-categoria-" + hex.EncodeToString(h[:8])
}

func uidContactoGraph(e Enlaces, id string, vistos map[string]string) string {
	if u := vistos[id]; u != "" {
		return u
	}
	if u := e.uid(id); u != "" {
		return u
	}
	return "graph-" + id
}

// cambiosCategorias añade a ca las categorías que cambiaron y las que nadie lleva ya.
func (g *GraphContactos) cambiosCategorias(ctx context.Context, col Coleccion, ca *Cambios, etagDe func(string) (string, bool), vistos map[string]string) error {
	cats, _, err := g.categorias(ctx, col)
	if errors.Is(err, ErrNoEsta) {
		return nil
	}
	if err != nil {
		return err
	}
	e := g.enlaces()
	libres := nombresSinEnlazar(e)
	for _, nombre := range slices.Sorted(func(yield func(string) bool) {
		for k := range cats {
			if !yield(k) {
				return
			}
		}
	}) {
		id := prefijoCategoria + nombre
		ids := cats[nombre]
		ca.Vistos = append(ca.Vistos, id)
		etag := etagGrupo(nombre, ids)
		if viejo, ok := etagDe(id); ok && viejo == etag {
			continue
		}
		uid := e.uid(id)
		if uid == "" {
			if libres[strings.ToLower(nombre)] {
				continue
			}
			uid = uidCategoria(nombre)
		}
		var miembros []string
		for _, c := range ids {
			miembros = append(miembros, uidContactoGraph(e, c, vistos))
		}
		ca.Puestos = append(ca.Puestos, Objeto{ID: id, ETag: etag, Datos: []byte(vcardGrupo(uid, nombre, miembros))})
	}
	for _, id := range e.conocidos(prefijoCategoria) {
		if _, sigue := cats[strings.TrimPrefix(id, prefijoCategoria)]; sigue {
			continue
		}
		if viejo, _ := etagDe(id); strings.HasPrefix(viejo, "vacia-") {
			// Una etiqueta sin nadie no se ve en Outlook: sigue aquí.
			ca.Vistos = append(ca.Vistos, id)
			continue
		}
		ca.Borrados = append(ca.Borrados, id)
	}
	return nil
}

// ponerCategorias deja en cada contacto las categorías que tiene que tener.
func (g *GraphContactos) ponerCategorias(ctx context.Context, col Coleccion, porContacto map[string][]string, quiere func(id string, cats []string) []string, ids []string) error {
	for _, id := range ids {
		antes := porContacto[id]
		despues := quiere(id, slices.Clone(antes))
		if slices.Equal(antes, despues) {
			continue
		}
		if despues == nil {
			despues = []string{}
		}
		if err := g.api.pedir(ctx, http.MethodPatch, g.contactos(col)+"/"+url.PathEscape(id), map[string]any{"categories": despues}, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func sinCategoria(cats []string, nombre string) []string {
	return slices.DeleteFunc(cats, func(c string) bool { return strings.TrimSpace(c) == nombre })
}

// crearCategoria pone el nombre de la etiqueta en sus contactos. Si allí ya la llevaba
// alguno, se juntan (sin quitar a nadie) y la ETag queda vacía: la vuelta siguiente la
// baja juntada.
func (g *GraphContactos) crearCategoria(ctx context.Context, col Coleccion, datos []byte) (Objeto, error) {
	nombre, uids, err := leerGrupo(datos)
	if err != nil {
		return Objeto{}, err
	}
	ids := idsDe(g.enlaces(), uids)
	cats, porContacto, err := g.categorias(ctx, col)
	if err != nil {
		return Objeto{}, err
	}
	for n := range cats {
		if strings.EqualFold(n, nombre) {
			nombre = n
		}
	}
	_, yaEstaba := cats[nombre]
	err = g.ponerCategorias(ctx, col, porContacto, func(_ string, cs []string) []string {
		if slices.Contains(cs, nombre) {
			return cs
		}
		return append(cs, nombre)
	}, ids)
	if err != nil {
		return Objeto{}, err
	}
	if yaEstaba {
		return Objeto{ID: prefijoCategoria + nombre}, nil
	}
	return Objeto{ID: prefijoCategoria + nombre, ETag: etagGrupo(nombre, ids)}, nil
}

// cambiarCategoria lleva a Outlook el nombre (cambiándolo en cada contacto) y los
// miembros de una etiqueta, si la categoría sigue como se vio.
func (g *GraphContactos) cambiarCategoria(ctx context.Context, col Coleccion, obj Objeto, datos []byte) (Objeto, error) {
	viejo := strings.TrimPrefix(obj.ID, prefijoCategoria)
	nombre, uids, err := leerGrupo(datos)
	if err != nil {
		return Objeto{}, err
	}
	cats, porContacto, err := g.categorias(ctx, col)
	if err != nil {
		return Objeto{}, err
	}
	if obj.ETag != "" && obj.ETag != etagGrupo(viejo, cats[viejo]) {
		return Objeto{}, ErrConflicto
	}
	quiero := idsDe(g.enlaces(), uids)
	tocar := slices.Clone(cats[viejo])
	for _, id := range quiero {
		if !slices.Contains(tocar, id) {
			tocar = append(tocar, id)
		}
	}
	slices.Sort(tocar)
	err = g.ponerCategorias(ctx, col, porContacto, func(id string, cs []string) []string {
		if viejo != nombre {
			cs = sinCategoria(cs, viejo)
		}
		quiere, tiene := slices.Contains(quiero, id), slices.Contains(cs, nombre)
		switch {
		case quiere && !tiene:
			cs = append(cs, nombre)
		case !quiere && tiene:
			cs = sinCategoria(cs, nombre)
		}
		return cs
	}, tocar)
	if err != nil {
		return Objeto{}, err
	}
	return Objeto{ID: prefijoCategoria + nombre, ETag: etagGrupo(nombre, quiero)}, nil
}

// borrarCategoria la quita de los contactos que la llevan, si sigue como se vio.
func (g *GraphContactos) borrarCategoria(ctx context.Context, col Coleccion, id, etag string) error {
	nombre := strings.TrimPrefix(id, prefijoCategoria)
	cats, porContacto, err := g.categorias(ctx, col)
	if err != nil {
		return err
	}
	if etag != "" && !strings.HasPrefix(etag, "vacia-") && etag != etagGrupo(nombre, cats[nombre]) {
		return ErrConflicto
	}
	return g.ponerCategorias(ctx, col, porContacto, func(_ string, cs []string) []string { return sinCategoria(cs, nombre) }, cats[nombre])
}

// leerCategoria es el objeto de una categoría; ErrNoEsta si nadie la lleva.
func (g *GraphContactos) leerCategoria(ctx context.Context, col Coleccion, id string) (Objeto, error) {
	nombre := strings.TrimPrefix(id, prefijoCategoria)
	cats, _, err := g.categorias(ctx, col)
	if err != nil {
		return Objeto{}, err
	}
	ids, ok := cats[nombre]
	if !ok {
		return Objeto{}, ErrNoEsta
	}
	e := g.enlaces()
	var miembros []string
	for _, c := range ids {
		miembros = append(miembros, uidContactoGraph(e, c, nil))
	}
	return Objeto{ID: id, ETag: etagGrupo(nombre, ids), Datos: []byte(vcardGrupo(cmp.Or(e.uid(id), uidCategoria(nombre)), nombre, miembros))}, nil
}
