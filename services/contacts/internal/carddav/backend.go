// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package carddav expone las libretas de direcciones de Sytue Contacts vía
// CardDAV (RFC 6352) usando la librería permisiva emersion/go-webdav/carddav
// (MIT). Implementa carddav.Backend sobre el store (PostgreSQL) y los códecs de
// vCard 4.0 del dominio, respetando el aislamiento por tenant del JWT de Sytue
// ID (RLS). Clean-room: implementación original sobre el estándar; sin código
// AGPL.
package carddav

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

// basePath es el prefijo de montaje del servidor CardDAV.
const basePath = "/dav"

// Backend implementa carddav.Backend sobre el store de Sytue Contacts. El
// principal (usuario) y el tenant se derivan del contexto (claims del JWT que
// el middleware authn inyecta antes de delegar en el handler CardDAV).
type Backend struct {
	store *store.Store
	// cache es la cache efímera compartida (Dragonfly). Guarda la LISTA DE LIBRETAS
	// del usuario, que es lo primero que pide cada sincronización CardDAV. NIL es un
	// estado normal: sin ella se consulta PostgreSQL como antes.
	cache *cache.Client
}

// UsarCache cablea la cache efímera compartida (nil la deshabilita).
func (b *Backend) UsarCache(c *cache.Client) { b.cache = c }

// libTTL es de un minuto, por el mismo motivo que en Calendar: lo que se paga al
// equivocarse no es servir una lista vieja, es que alguien no vea la libreta que
// acaba de crear y crea que la aplicación no funciona.
const libTTL = time.Minute

// libKey lleva el TENANT y el usuario. Servir las libretas de una organización a
// otra no sería una cache imprecisa, sería un fallo de aislamiento.
func libKey(tenant, sub string) string { return "carddav:libs:" + tenant + ":" + sub }

// LIMITACIÓN CONOCIDA, y se deja escrita en vez de descubrirla en un informe de
// fallo: la libreta también se puede renombrar por la vía REST
// (`PATCH /api/v1/address-books/{id}`, ver internal/http), que NO pasa por este
// backend y por tanto no invalida nada. Quien renombre desde la web verá el
// nombre nuevo al instante ahí y el viejo en su cliente CardDAV hasta que venza el
// minuto de TTL.
//
// Se acepta porque el desfase máximo es de sesenta segundos y solo afecta al
// nombre visible, nunca a los contactos. Arreglarlo de raíz pide bajar la cache al
// store —por donde pasan LAS DOS vías— y eso es un cambio mayor que el problema
// que resuelve. Si algún día el TTL sube, esto deja de ser aceptable.

// NewBackend construye el Backend CardDAV.
func NewBackend(st *store.Store) *Backend { return &Backend{store: st} }

var _ carddav.Backend = (*Backend)(nil)

// CurrentUserPrincipal devuelve la ruta del principal del usuario autenticado.
func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	sub, err := subject(ctx)
	if err != nil {
		return "", err
	}
	return principalPath(sub), nil
}

// AddressBookHomeSetPath devuelve la ruta del "home set" de libretas del usuario.
func (b *Backend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	sub, err := subject(ctx)
	if err != nil {
		return "", err
	}
	return homeSetPath(sub), nil
}

// ListAddressBooks lista las libretas del usuario (asegurando la por defecto).
func (b *Backend) ListAddressBooks(ctx context.Context) ([]carddav.AddressBook, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	// La cache se consulta ANTES del auto-aprovisionamiento, y ahí está el ahorro
	// grande: `EnsureDefaultAddressBook` es un UPSERT, o sea una ESCRITURA en la
	// base por cada PROPFIND. La libreta del teléfono sincroniza cada pocos minutos
	// y empieza siempre listando, así que eso es escritura constante sin que nadie
	// esté usando nada.
	//
	// Saltárselo es correcto porque hay entrada en cache: si la hay, esta lista ya
	// se sirvió antes y la libreta por defecto ya existe.
	claims, _ := authn.FromContext(ctx)
	if v, ok := b.libsCacheadas(ctx, claims.Tenant, sub); ok {
		return v, nil
	}
	if _, err := b.store.EnsureDefaultAddressBook(ctx, sub); err != nil {
		return nil, err
	}
	books, err := b.store.ListAddressBooks(ctx, sub)
	if err != nil {
		return nil, err
	}
	out := make([]carddav.AddressBook, 0, len(books))
	for _, bk := range books {
		out = append(out, toCardDAVBook(sub, bk))
	}
	b.cachearLibs(ctx, claims.Tenant, sub, out)
	return out, nil
}

// libsCacheadas devuelve la lista guardada, si la hay.
func (b *Backend) libsCacheadas(ctx context.Context, tenant, sub string) ([]carddav.AddressBook, bool) {
	if !b.cache.Enabled() || tenant == "" {
		return nil, false
	}
	raw, found, err := b.cache.Get(ctx, libKey(tenant, sub))
	if err != nil || !found {
		return nil, false
	}
	var libs []carddav.AddressBook
	if json.Unmarshal([]byte(raw), &libs) != nil {
		return nil, false
	}
	return libs, true
}

// cachearLibs guarda la lista. Un fallo aquí no cambia la respuesta.
func (b *Backend) cachearLibs(ctx context.Context, tenant, sub string, libs []carddav.AddressBook) {
	if !b.cache.Enabled() || tenant == "" {
		return
	}
	blob, err := json.Marshal(libs)
	if err != nil {
		return
	}
	_ = b.cache.SetTTL(ctx, libKey(tenant, sub), string(blob), libTTL)
}

// invalidarLibs tira la lista cacheada del usuario: se llama en todo camino que
// cree, renombre o borre una libreta.
func (b *Backend) invalidarLibs(ctx context.Context, sub string) {
	if !b.cache.Enabled() {
		return
	}
	claims, _ := authn.FromContext(ctx)
	if claims.Tenant == "" {
		return
	}
	_ = b.cache.Del(ctx, libKey(claims.Tenant, sub))
}

// GetAddressBook devuelve una libreta por su ruta CardDAV.
func (b *Backend) GetAddressBook(ctx context.Context, urlPath string) (*carddav.AddressBook, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	_, bookName, _, err := splitPath(urlPath)
	if err != nil {
		return nil, err
	}
	bk, err := b.store.GetAddressBookByName(ctx, sub, bookName)
	if err != nil {
		return nil, mapErr(err)
	}
	ab := toCardDAVBook(sub, bk)
	return &ab, nil
}

// CreateAddressBook crea una libreta (MKCOL extendido de CardDAV).
func (b *Backend) CreateAddressBook(ctx context.Context, ab *carddav.AddressBook) error {
	sub, err := subject(ctx)
	if err != nil {
		return err
	}
	_, bookName, _, err := splitPath(ab.Path)
	if err != nil {
		return err
	}
	_, err = b.store.CreateAddressBook(ctx, store.NewAddressBook{
		OwnerSub:    sub,
		Name:        bookName,
		DisplayName: ab.Name,
		Description: ab.Description,
	})
	if err != nil {
		return mapErr(err)
	}
	// Sin esto, quien acaba de crear la libreta no la vería hasta que venciera el
	// TTL, y desde el cliente eso se lee como que la creación falló.
	b.invalidarLibs(ctx, sub)
	return nil
}

// DeleteAddressBook borra una libreta por su ruta.
func (b *Backend) DeleteAddressBook(ctx context.Context, urlPath string) error {
	sub, err := subject(ctx)
	if err != nil {
		return err
	}
	_, bookName, _, err := splitPath(urlPath)
	if err != nil {
		return err
	}
	bk, err := b.store.GetAddressBookByName(ctx, sub, bookName)
	if err != nil {
		return mapErr(err)
	}
	if err := b.store.DeleteAddressBook(ctx, sub, bk.ID); err != nil {
		return mapErr(err)
	}
	// Sin esto, la libreta borrada seguiría apareciendo en la lista durante todo el
	// TTL. En un cliente CardDAV eso es peor que una lista vieja: intenta
	// sincronizar contra una colección que ya no existe y encadena errores.
	b.invalidarLibs(ctx, sub)
	return nil
}

// GetAddressObject devuelve un recurso vCard por su ruta, aplicando el filtro de
// propiedades pedido (req).
func (b *Backend) GetAddressObject(ctx context.Context, urlPath string, req *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	_, bookName, resource, err := splitPath(urlPath)
	if err != nil || resource == "" {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("recurso no encontrado"))
	}
	bk, err := b.store.GetAddressBookByName(ctx, sub, bookName)
	if err != nil {
		return nil, mapErr(err)
	}
	obj, err := b.store.GetCardObjectByResource(ctx, bk.ID, resource)
	if err != nil {
		return nil, mapErr(err)
	}
	ao, err := toAddressObject(urlPath, obj)
	if err != nil {
		return nil, err
	}
	return ao, nil
}

// ListAddressObjects lista todos los recursos vCard de una libreta (PROPFIND).
func (b *Backend) ListAddressObjects(ctx context.Context, urlPath string, req *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	_, bookName, _, err := splitPath(urlPath)
	if err != nil {
		return nil, err
	}
	bk, err := b.store.GetAddressBookByName(ctx, sub, bookName)
	if err != nil {
		return nil, mapErr(err)
	}
	objs, err := b.store.ListCardObjects(ctx, bk.ID)
	if err != nil {
		return nil, err
	}
	out := make([]carddav.AddressObject, 0, len(objs))
	for _, o := range objs {
		full, err := b.store.GetCardObjectByResource(ctx, bk.ID, o.ResourceName)
		if err != nil {
			return nil, err
		}
		ao, err := toAddressObject(objectPath(bookName, sub, o.ResourceName), full)
		if err != nil {
			return nil, err
		}
		out = append(out, *ao)
	}
	return out, nil
}

// QueryAddressObjects ejecuta un addressbook-query (REPORT, RFC 6352 8.6). El
// filtrado fino lo aplica la librería sobre las vCards devueltas; aquí servimos
// el conjunto vivo de la libreta.
func (b *Backend) QueryAddressObjects(ctx context.Context, urlPath string, query *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	all, err := b.ListAddressObjects(ctx, urlPath, nil)
	if err != nil {
		return nil, err
	}
	// Sin filtros = listado completo: la librería trata un anyOf VACÍO como "no
	// coincide nada", pero los clientes que omiten <filter> (habitual) esperan
	// todos los recursos (un filtro vacío no restringe).
	if query == nil || len(query.PropFilters) == 0 {
		return all, nil
	}
	filtered, err := carddav.Filter(query, all)
	if err != nil {
		// Si la librería no puede filtrar, degradamos al conjunto completo.
		log.From(ctx).Warn("carddav: filtro de query no aplicado", "error", err)
		return all, nil
	}
	return filtered, nil
}

// PutAddressObject crea o reemplaza un recurso vCard (PUT). Aplica las
// precondiciones If-Match/If-None-Match que la librería traduce a opts.
func (b *Backend) PutAddressObject(ctx context.Context, urlPath string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	sub, err := subject(ctx)
	if err != nil {
		return nil, err
	}
	_, bookName, resource, err := splitPath(urlPath)
	if err != nil || resource == "" {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, fmt.Errorf("ruta de recurso inválida"))
	}
	bk, err := b.store.GetAddressBookByName(ctx, sub, bookName)
	if err != nil {
		return nil, mapErr(err)
	}

	data, err := encodeCard(card)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}

	ifMatch, requireAbsent := putConditions(opts)
	saved, err := b.upsertCard(ctx, sub, bk.ID, resource, data, ifMatch, requireAbsent)
	if err != nil {
		return nil, mapErr(err)
	}
	return &carddav.AddressObject{
		Path:    objectPath(bookName, sub, saved.ResourceName),
		ETag:    saved.ETag.String(),
		ModTime: saved.UpdatedAt,
	}, nil
}

// DeleteAddressObject borra un recurso vCard (DELETE).
func (b *Backend) DeleteAddressObject(ctx context.Context, urlPath string) error {
	sub, err := subject(ctx)
	if err != nil {
		return err
	}
	_, bookName, resource, err := splitPath(urlPath)
	if err != nil || resource == "" {
		return webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("recurso no encontrado"))
	}
	bk, err := b.store.GetAddressBookByName(ctx, sub, bookName)
	if err != nil {
		return mapErr(err)
	}
	return mapErr(b.store.DeleteByResource(ctx, sub, bk.ID, resource, nil))
}

// upsertCard decide, según el KIND de la vCard, si crea/actualiza un contacto o
// un grupo, reutilizando la lógica del store (owner-scoped por sub). Devuelve el
// objeto de tarjeta resultante (con su ETag) para la respuesta CardDAV.
func (b *Backend) upsertCard(ctx context.Context, sub string, bookID uuid.UUID, resource, data string, ifMatch *uuid.UUID, requireAbsent bool) (domain.CardObject, error) {
	rr, err := b.store.ResolveResource(ctx, bookID, resource)
	if err != nil {
		return domain.CardObject{}, err
	}
	if requireAbsent && rr.Found {
		return domain.CardObject{}, domain.ErrConflict // If-None-Match: * sobre recurso existente
	}
	if ifMatch != nil && (!rr.Found || rr.ETag != *ifMatch) {
		return domain.CardObject{}, domain.ErrPreconditionFailed
	}

	kind := domain.KindOf(data)
	if kind == domain.KindGroup {
		g, err := domain.DecodeGroup(data)
		if err != nil {
			return domain.CardObject{}, err
		}
		in := store.UpsertGroup{AddressBookID: bookID, Group: g, CardData: data, ResourceName: resource}
		if rr.Found && rr.GroupID != nil {
			if _, err := b.store.UpdateGroup(ctx, sub, *rr.GroupID, in, ifMatch); err != nil {
				return domain.CardObject{}, err
			}
		} else if rr.Found {
			return domain.CardObject{}, domain.ErrConflict // mismo recurso, distinto tipo
		} else if _, err := b.store.CreateGroup(ctx, sub, in); err != nil {
			return domain.CardObject{}, err
		}
	} else {
		c, err := domain.DecodeContact(data)
		if err != nil {
			return domain.CardObject{}, err
		}
		in := store.UpsertContact{AddressBookID: bookID, Contact: c, CardData: data, ResourceName: resource}
		if rr.Found && rr.ContactID != nil {
			if _, err := b.store.UpdateContact(ctx, sub, *rr.ContactID, in, ifMatch); err != nil {
				return domain.CardObject{}, err
			}
		} else if rr.Found {
			return domain.CardObject{}, domain.ErrConflict
		} else if _, err := b.store.CreateContact(ctx, sub, in); err != nil {
			return domain.CardObject{}, err
		}
	}
	return b.store.GetCardObjectByResource(ctx, bookID, resource)
}

// encodeCard serializa una vCard de la librería a texto canónico 4.0.
func encodeCard(card vcard.Card) (string, error) {
	if card == nil {
		return "", fmt.Errorf("vCard vacía")
	}
	// Garantizar UID y FN mínimos antes de persistir.
	if card.Value(vcard.FieldUID) == "" {
		card.SetValue(vcard.FieldUID, "urn:uuid:"+domain.NewUID())
	}
	if card.Value(vcard.FieldFormattedName) == "" {
		if n := card.Name(); n != nil {
			card.SetValue(vcard.FieldFormattedName, strings.TrimSpace(n.GivenName+" "+n.FamilyName))
		}
	}
	vcard.ToV4(card)
	var sb strings.Builder
	if err := vcard.NewEncoder(&sb).Encode(card); err != nil {
		return "", fmt.Errorf("serializar vCard: %w", err)
	}
	return sb.String(), nil
}

// ---------------------------------------------------------------------------
// Mapeo de rutas y errores
// ---------------------------------------------------------------------------

// principalPath es la ruta del principal de un usuario.
func principalPath(sub string) string { return path.Join(basePath, "principals", sub) + "/" }

// homeSetPath es la ruta del home set de libretas de un usuario.
func homeSetPath(sub string) string { return path.Join(basePath, "addressbooks", sub) + "/" }

// bookPath es la ruta de una libreta concreta.
func bookPath(bookName, sub string) string {
	return path.Join(basePath, "addressbooks", sub, bookName) + "/"
}

// objectPath es la ruta de un recurso vCard dentro de una libreta.
func objectPath(bookName, sub, resource string) string {
	return path.Join(basePath, "addressbooks", sub, bookName, resource)
}

// splitPath descompone una ruta CardDAV en (sub, bookName, resource). resource
// queda vacío si la ruta apunta a la colección (libreta).
//
//nolint:unparam // sub forma parte del contrato del parseo aunque hoy los llamantes no lo consuman.
func splitPath(urlPath string) (sub, bookName, resource string, err error) {
	p := strings.TrimPrefix(urlPath, basePath)
	p = strings.TrimPrefix(p, "/addressbooks/")
	p = strings.Trim(p, "/")
	if p == "" {
		return "", "", "", webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("ruta vacía"))
	}
	parts := strings.Split(p, "/")
	switch len(parts) {
	case 1:
		return parts[0], "", "", nil
	case 2:
		return parts[0], parts[1], "", nil
	default:
		return parts[0], parts[1], parts[len(parts)-1], nil
	}
}

// toCardDAVBook adapta una libreta del dominio al tipo de la librería.
func toCardDAVBook(sub string, bk domain.AddressBook) carddav.AddressBook {
	return carddav.AddressBook{
		Path:                 bookPath(bk.Name, sub),
		Name:                 bk.DisplayName,
		Description:          bk.Description,
		MaxResourceSize:      maxVCardBytes,
		SupportedAddressData: supportedAddressData(),
	}
}

// toAddressObject adapta un objeto de tarjeta (con su vCard) al tipo de la
// librería, decodificando la vCard almacenada.
func toAddressObject(urlPath string, obj domain.CardObject) (*carddav.AddressObject, error) {
	card, err := vcard.NewDecoder(strings.NewReader(obj.Data)).Decode()
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusInternalServerError, fmt.Errorf("vCard almacenada ilegible: %w", err))
	}
	return &carddav.AddressObject{
		Path:          urlPath,
		ModTime:       obj.UpdatedAt,
		ContentLength: int64(obj.SizeBytes),
		ETag:          obj.ETag.String(),
		Card:          card,
	}, nil
}

// mapErr traduce errores de dominio a *webdav.HTTPError con el código correcto.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case isErr(err, domain.ErrNotFound):
		return webdav.NewHTTPError(http.StatusNotFound, err)
	case isErr(err, domain.ErrConflict):
		return webdav.NewHTTPError(http.StatusPreconditionFailed, err)
	case isErr(err, domain.ErrPreconditionFailed):
		return webdav.NewHTTPError(http.StatusPreconditionFailed, err)
	case isErr(err, domain.ErrDefaultImmutable):
		return webdav.NewHTTPError(http.StatusForbidden, err)
	case isErr(err, domain.ErrInvalidName), isErr(err, domain.ErrInvalidVCard),
		isErr(err, domain.ErrInvalidContact), isErr(err, domain.ErrInvalidGroup):
		return webdav.NewHTTPError(http.StatusBadRequest, err)
	default:
		return webdav.NewHTTPError(http.StatusInternalServerError, err)
	}
}

// subject devuelve el sujeto autenticado o un 401 CardDAV.
func subject(ctx context.Context) (string, error) {
	if c, ok := authn.FromContext(ctx); ok && c.Subject != "" {
		return c.Subject, nil
	}
	return "", webdav.NewHTTPError(http.StatusUnauthorized, fmt.Errorf("no autenticado"))
}
