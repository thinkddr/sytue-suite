// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/google/uuid"
)

// Conversion entre el modelo de dominio de Sytue Contacts y vCard 4.0
// (RFC 6350), sobre la libreria permisiva emersion/go-vcard (MIT). El
// almacenamiento conserva la vCard serializada canonica (card_objects.data); la
// proyeccion Contact/Group es el indice consultable. Clean-room: mapeo propio
// sobre el estandar; sin codigo AGPL.

const (
	vcardVersion = "4.0"
	bdayLayout   = "2006-01-02"
)

// EncodeContact serializa un Contact a vCard 4.0. Si baseCard es no nil, se
// preservan las propiedades desconocidas (X-*, IMPP, ADR, ...) de la tarjeta
// original que Sytue Contacts no proyecta, para no perder datos en round-trip.
//
// Y también las dos gestionadas que la proyección ESCONDE: una PHOTO embebida
// (`photoRef` no la da como URL) y un BDAY que no es fecha completa («--0215», sin
// año). Quien edita por la API REST no las ve, así que no puede reenviarlas; sin esto,
// cambiar el teléfono de un contacto le borraba la foto. Para quitar la foto está
// `DELETE /contacts/photo`; lo que se proyecta sí se borra al no mandarlo.
//
//nolint:gocyclo // serializador vCard 4.0: una rama por propiedad opcional del estándar.
func EncodeContact(c Contact, baseCard string) (string, error) {
	card := vcard.Card{}
	var fotoOculta, cumpleOculto *vcard.Field
	if strings.TrimSpace(baseCard) != "" {
		if parsed, err := parseSingle(baseCard); err == nil {
			fotoOculta, cumpleOculto = ocultosDe(parsed)
			card = stripManagedFields(parsed)
		}
	}

	card.SetValue(vcard.FieldVersion, vcardVersion)
	if c.UID != "" {
		card.SetValue(vcard.FieldUID, ensureURN(c.UID))
	}
	card.SetValue(vcard.FieldKind, string(KindIndividual))

	fn := c.DisplayName()
	if fn == "" {
		return "", fmt.Errorf("%w: FN obligatorio", ErrInvalidVCard)
	}
	card.SetValue(vcard.FieldFormattedName, fn)

	// N: apellidos;nombre;segundos;prefijos;sufijos (RFC 6350 6.2.2).
	card.SetName(&vcard.Name{
		FamilyName:      c.FamilyName,
		GivenName:       c.GivenName,
		AdditionalName:  c.AdditionalName,
		HonorificPrefix: c.HonorificPrefix,
		HonorificSuffix: c.HonorificSuffix,
	})

	if c.Nickname != "" {
		card.SetValue(vcard.FieldNickname, c.Nickname)
	}
	if c.Organization != "" {
		card.SetValue(vcard.FieldOrganization, c.Organization)
	}
	if c.Title != "" {
		card.SetValue(vcard.FieldTitle, c.Title)
	}
	if c.Note != "" {
		card.SetValue(vcard.FieldNote, c.Note)
	}
	switch {
	case c.PhotoURL != "":
		card.SetValue(vcard.FieldPhoto, c.PhotoURL)
	case fotoOculta != nil:
		card[vcard.FieldPhoto] = []*vcard.Field{fotoOculta}
	}
	switch {
	case c.Birthday != nil:
		card.SetValue(vcard.FieldBirthday, c.Birthday.Format(bdayLayout))
	case cumpleOculto != nil:
		card[vcard.FieldBirthday] = []*vcard.Field{cumpleOculto}
	}

	// EMAIL (RFC 6350 6.4.2): TYPE y PREF=1 para el principal.
	emails := make([]*vcard.Field, 0, len(c.Emails))
	for i, e := range c.Emails {
		f := &vcard.Field{Value: e.Email, Params: vcard.Params{}}
		if e.Type != "" {
			f.Params.Set(vcard.ParamType, string(e.Type))
		}
		if e.IsPrimary {
			f.Params.Set(vcard.ParamPreferred, "1")
		}
		f.Params.Set("PID", strconv.Itoa(i+1))
		emails = append(emails, f)
	}
	if len(emails) > 0 {
		card[vcard.FieldEmail] = emails
	}

	// TEL (RFC 6350 6.4.1).
	phones := make([]*vcard.Field, 0, len(c.Phones))
	for i, p := range c.Phones {
		f := &vcard.Field{Value: p.Phone, Params: vcard.Params{}}
		if p.Type != "" {
			f.Params.Set(vcard.ParamType, string(p.Type))
		}
		if p.IsPrimary {
			f.Params.Set(vcard.ParamPreferred, "1")
		}
		f.Params.Set("PID", strconv.Itoa(i+1))
		phones = append(phones, f)
	}
	if len(phones) > 0 {
		card[vcard.FieldTelephone] = phones
	}

	// ADR (RFC 6350 6.3.1): apartado;ext;calle;localidad;region;CP;pais.
	for i, a := range c.Addresses {
		if a.IsZero() {
			continue
		}
		adr := &vcard.Address{
			Field:           &vcard.Field{Params: vcard.Params{}},
			PostOfficeBox:   a.POBox,
			ExtendedAddress: a.Extended,
			StreetAddress:   a.Street,
			Locality:        a.Locality,
			Region:          a.Region,
			PostalCode:      a.PostalCode,
			Country:         a.Country,
		}
		if a.Type != "" {
			adr.Params.Set(vcard.ParamType, string(a.Type))
		}
		if a.IsPrimary {
			adr.Params.Set(vcard.ParamPreferred, "1")
		}
		adr.Params.Set("PID", strconv.Itoa(i+1))
		card.AddAddress(adr)
	}

	card.SetValue(vcard.FieldRevision, time.Now().UTC().Format("20060102T150405Z"))
	return marshal(card)
}

// DecodeContact parsea una vCard a un Contact (proyeccion normalizada). El UID,
// FN y N se extraen siempre; el resto si estan presentes.
func DecodeContact(data string) (Contact, error) {
	card, err := parseSingle(data)
	if err != nil {
		return Contact{}, err
	}
	if k := strings.ToLower(card.Value(vcard.FieldKind)); k == string(KindGroup) {
		return Contact{}, fmt.Errorf("%w: es un grupo, no un contacto", ErrInvalidVCard)
	}

	c := Contact{
		UID:           cleanURN(card.Value(vcard.FieldUID)),
		FormattedName: card.Value(vcard.FieldFormattedName),
		Nickname:      card.Value(vcard.FieldNickname),
		Organization:  firstComponent(card.Value(vcard.FieldOrganization)),
		Title:         card.Value(vcard.FieldTitle),
		Note:          card.Value(vcard.FieldNote),
		PhotoURL:      photoRef(card),
	}
	if n := card.Name(); n != nil {
		c.FamilyName = n.FamilyName
		c.GivenName = n.GivenName
		c.AdditionalName = n.AdditionalName
		c.HonorificPrefix = n.HonorificPrefix
		c.HonorificSuffix = n.HonorificSuffix
	}
	if bd := card.Value(vcard.FieldBirthday); bd != "" {
		if t, err := parseBirthday(bd); err == nil {
			c.Birthday = &t
		}
	}
	if c.UID == "" {
		c.UID = NewUID()
	}
	if c.FormattedName == "" {
		c.FormattedName = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if c.FormattedName == "" {
		return Contact{}, fmt.Errorf("%w: falta FN/N", ErrInvalidVCard)
	}

	c.Emails = decodeEmails(card)
	c.Phones = decodePhones(card)
	c.Addresses = decodeAddresses(card)
	return c, nil
}

// ImportCard prepara una tarjeta de FUERA —un `.vcf` de Google, Apple, Outlook u
// otro Sytue— para darla de alta ENTERA: devuelve su proyección y la vCard 4.0 que se
// guarda. Lo que la proyección no entiende (X-*, IMPP, URL, un cumpleaños sin año…)
// viaja en la tarjeta tal cual, igual que al editar (EncodeContact con base); la foto
// binaria de vCard 3.0 se pasa a data URI (fotoEnLinea) para que se vea.
//
// Es el `parseVcf` + `baseVcard` de la web, hecho en el servicio para que un cliente
// que no tiene BFF —la app nativa— no pierda la foto ni los X-* al importar.
//
// El UID es NUEVO, como en la web: importar dos veces el mismo fichero no puede chocar
// con lo ya importado ni pisarlo.
func ImportCard(data string) (Contact, string, error) {
	card, err := parseSingle(data)
	if err != nil {
		return Contact{}, "", err
	}
	fotoEnLinea(card)
	uid := NewUID()
	card.SetValue(vcard.FieldUID, ensureURN(uid))
	base, err := marshal(card)
	if err != nil {
		return Contact{}, "", fmt.Errorf("%w: %w", ErrInvalidVCard, err)
	}
	c, err := DecodeContact(base)
	if err != nil {
		return Contact{}, "", err
	}
	c.UID = uid
	out, err := EncodeContact(c, base)
	return c, out, err
}

// fotoEnLinea pasa la PHOTO binaria de vCard 3.0 (`ENCODING=b`, el base64 pelado con
// el tipo aparte) a la forma de vCard 4.0, un data URI: es la que el servicio sirve y
// la que la web reconoce. El tipo sale de los bytes y no del TYPE, que cada cliente
// escribe a su manera. Si no es una imagen que el servicio admita —otro formato,
// demasiado grande—, se queda como venía: se guarda igual, solo que no se ve.
func fotoEnLinea(card vcard.Card) {
	f := card.Get(vcard.FieldPhoto)
	if f == nil {
		return
	}
	if enc := strings.ToLower(f.Params.Get("ENCODING")); enc != "b" && enc != "base64" {
		return
	}
	limpio := strings.Join(strings.Fields(f.Value), "")
	raw, err := base64.StdEncoding.DecodeString(limpio)
	if err != nil {
		return
	}
	tipo, _, _ := strings.Cut(http.DetectContentType(raw), ";")
	uri, err := ValidatePhotoDataURI("data:" + tipo + ";base64," + limpio)
	if err != nil {
		return
	}
	card[vcard.FieldPhoto] = []*vcard.Field{{Value: uri, Group: f.Group, Params: vcard.Params{}}}
}

// EncodeGroup serializa un Group a una vCard 4.0 con KIND=group y propiedades
// MEMBER por cada UID miembro (RFC 6350 6.1.4 / 6.6.5).
func EncodeGroup(g Group, baseCard string) (string, error) {
	card := vcard.Card{}
	if strings.TrimSpace(baseCard) != "" {
		if parsed, err := parseSingle(baseCard); err == nil {
			card = stripManagedFields(parsed)
			delete(card, vcard.FieldMember)
		}
	}
	if strings.TrimSpace(g.Name) == "" {
		return "", fmt.Errorf("%w: nombre obligatorio", ErrInvalidGroup)
	}
	card.SetValue(vcard.FieldVersion, vcardVersion)
	card.SetValue(vcard.FieldKind, string(KindGroup))
	if g.UID != "" {
		card.SetValue(vcard.FieldUID, ensureURN(g.UID))
	}
	card.SetValue(vcard.FieldFormattedName, g.Name)

	members := make([]*vcard.Field, 0, len(g.Members))
	for _, m := range g.Members {
		if strings.TrimSpace(m) == "" {
			continue
		}
		members = append(members, &vcard.Field{Value: ensureURN(m)})
	}
	if len(members) > 0 {
		card[vcard.FieldMember] = members
	}
	card.SetValue(vcard.FieldRevision, time.Now().UTC().Format("20060102T150405Z"))
	return marshal(card)
}

// DecodeGroup parsea una vCard KIND=group a un Group.
func DecodeGroup(data string) (Group, error) {
	card, err := parseSingle(data)
	if err != nil {
		return Group{}, err
	}
	if strings.ToLower(card.Value(vcard.FieldKind)) != string(KindGroup) {
		return Group{}, fmt.Errorf("%w: KIND distinto de group", ErrInvalidGroup)
	}
	g := Group{
		UID:  cleanURN(card.Value(vcard.FieldUID)),
		Name: card.Value(vcard.FieldFormattedName),
	}
	if g.UID == "" {
		g.UID = NewUID()
	}
	if strings.TrimSpace(g.Name) == "" {
		return Group{}, fmt.Errorf("%w: falta FN", ErrInvalidGroup)
	}
	for _, f := range card[vcard.FieldMember] {
		if uid := cleanURN(f.Value); uid != "" {
			g.Members = append(g.Members, uid)
		}
	}
	return g, nil
}

// KindOf inspecciona una vCard y devuelve su KIND (individual por defecto).
func KindOf(data string) Kind {
	card, err := parseSingle(data)
	if err != nil {
		return KindIndividual
	}
	k := Kind(strings.ToLower(card.Value(vcard.FieldKind)))
	if !ValidKind(k) {
		return KindIndividual
	}
	return k
}

// ---------------------------------------------------------------------------
// Helpers de codec
// ---------------------------------------------------------------------------

// parseSingle decodifica la primera vCard de un flujo.
func parseSingle(data string) (vcard.Card, error) {
	dec := vcard.NewDecoder(strings.NewReader(data))
	card, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidVCard, err)
	}
	return card, nil
}

// marshal serializa una vCard a texto (CRLF, RFC 6350). go-vcard exige version
// 4.0 explicita.
func marshal(card vcard.Card) (string, error) {
	vcard.ToV4(card)
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", fmt.Errorf("contacts: serializar vCard: %w", err)
	}
	return buf.String(), nil
}

// stripManagedFields elimina de una tarjeta de base las propiedades que Sytue
// Contacts regenera siempre, para no duplicarlas al volver a serializar.
// ocultosDe son los dos campos gestionados que la proyección REST no enseña: la PHOTO
// embebida (`photoRef` solo da URLs) y un BDAY que no es fecha completa. Ver
// EncodeContact.
func ocultosDe(card vcard.Card) (foto, cumple *vcard.Field) {
	if f := card.Get(vcard.FieldPhoto); f != nil && photoRef(card) == "" && strings.TrimSpace(f.Value) != "" {
		foto = f
	}
	if f := card.Get(vcard.FieldBirthday); f != nil && strings.TrimSpace(f.Value) != "" {
		if _, err := parseBirthday(f.Value); err != nil {
			cumple = f
		}
	}
	return foto, cumple
}

func stripManagedFields(card vcard.Card) vcard.Card {
	for _, f := range []string{
		vcard.FieldVersion, vcard.FieldUID, vcard.FieldKind, vcard.FieldFormattedName,
		vcard.FieldName, vcard.FieldNickname, vcard.FieldOrganization, vcard.FieldTitle,
		vcard.FieldNote, vcard.FieldPhoto, vcard.FieldBirthday, vcard.FieldEmail,
		vcard.FieldTelephone, vcard.FieldAddress, vcard.FieldRevision,
	} {
		delete(card, f)
	}
	return card
}

func decodeEmails(card vcard.Card) []Email {
	fields := card[vcard.FieldEmail]
	if len(fields) == 0 {
		return nil
	}
	out := make([]Email, 0, len(fields))
	for _, f := range fields {
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		out = append(out, Email{
			Email:     strings.TrimSpace(f.Value),
			Type:      emailType(tipoDe(f, "home", "work")),
			IsPrimary: isPreferred(f),
		})
	}
	ensurePrimary(len(out), func(i int) *bool { return &out[i].IsPrimary })
	return out
}

func decodePhones(card vcard.Card) []Phone {
	fields := card[vcard.FieldTelephone]
	if len(fields) == 0 {
		return nil
	}
	out := make([]Phone, 0, len(fields))
	for _, f := range fields {
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		out = append(out, Phone{
			Phone:     strings.TrimSpace(f.Value),
			Type:      phoneType(tipoDe(f, "cell", "mobile", "home", "work", "fax")),
			IsPrimary: isPreferred(f),
		})
	}
	ensurePrimary(len(out), func(i int) *bool { return &out[i].IsPrimary })
	return out
}

func decodeAddresses(card vcard.Card) []Address {
	adrs := card.Addresses()
	if len(adrs) == 0 {
		return nil
	}
	out := make([]Address, 0, len(adrs))
	for _, a := range adrs {
		addr := Address{
			POBox:      strings.TrimSpace(a.PostOfficeBox),
			Extended:   strings.TrimSpace(a.ExtendedAddress),
			Street:     strings.TrimSpace(a.StreetAddress),
			Locality:   strings.TrimSpace(a.Locality),
			Region:     strings.TrimSpace(a.Region),
			PostalCode: strings.TrimSpace(a.PostalCode),
			Country:    strings.TrimSpace(a.Country),
			Type:       AddressHome,
		}
		if addr.IsZero() {
			continue
		}
		if a.Field != nil {
			addr.Type = addressType(tipoDe(a.Field, "home", "work", "other"))
			addr.IsPrimary = isPreferred(a.Field)
		}
		out = append(out, addr)
	}
	ensurePrimary(len(out), func(i int) *bool { return &out[i].IsPrimary })
	return out
}

// ensurePrimary garantiza que, si hay al menos un elemento, exactamente uno es
// principal: si ninguno lo es, marca el primero.
func ensurePrimary(n int, flag func(i int) *bool) {
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		if *flag(i) {
			return
		}
	}
	*flag(0) = true
}

// tipoDe es el primer TYPE del campo que sea uno de los conocidos, o el primero sin
// más. Una vCard 3.0 los apila —`EMAIL;TYPE=INTERNET,WORK,pref`, lo que exportan Apple
// y Google— y quedarse con el primero sin mirar convertía todos esos correos en «otro».
func tipoDe(f *vcard.Field, conocidos ...string) string {
	tipos := f.Params.Types()
	for _, t := range tipos {
		if slices.Contains(conocidos, t) {
			return t
		}
	}
	if len(tipos) > 0 {
		return tipos[0]
	}
	return ""
}

// isPreferred indica si un campo es el preferido (PREF=1 o TYPE=pref).
func isPreferred(f *vcard.Field) bool {
	if f.Params.Get(vcard.ParamPreferred) == "1" {
		return true
	}
	for _, t := range f.Params[vcard.ParamType] {
		if strings.EqualFold(t, "pref") {
			return true
		}
	}
	return false
}

func emailType(t string) EmailType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "work":
		return EmailWork
	case "home":
		return EmailHome
	case "":
		return EmailHome
	default:
		return EmailOther
	}
}

func phoneType(t string) PhoneType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "cell", "mobile":
		return PhoneCell
	case "work":
		return PhoneWork
	case "home":
		return PhoneHome
	case "fax":
		return PhoneFax
	case "":
		return PhoneCell
	default:
		return PhoneOther
	}
}

func addressType(t string) AddressType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "work":
		return AddressWork
	case "other":
		return AddressOther
	default:
		return AddressHome
	}
}

// photoRef devuelve la PHOTO solo si es una referencia URI (no binario embebido).
func photoRef(card vcard.Card) string {
	f := card.Get(vcard.FieldPhoto)
	if f == nil {
		return ""
	}
	v := strings.TrimSpace(f.Value)
	if strings.HasPrefix(strings.ToLower(v), "data:") {
		return "" // foto embebida; no la proyectamos como URL
	}
	if f.Params.Get("ENCODING") != "" {
		// Binaria de vCard 3.0: el valor es base64, no una dirección. Antes acababa
		// entero en `photo_url` y, al editar, se reescribía sin su ENCODING.
		return ""
	}
	return v
}

// firstComponent devuelve el primer componente de un valor estructurado vCard
// (separado por ";"), p. ej. el nombre de organizacion sin las unidades.
func firstComponent(v string) string {
	if i := strings.IndexByte(v, ';'); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return strings.TrimSpace(v)
}

// ensureURN normaliza un UID a una URI estable (urn:uuid:... si es un UUID).
func ensureURN(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return uid
	}
	if strings.Contains(uid, ":") {
		return uid // ya es una URI (urn:uuid:, mailto:, ...)
	}
	if _, err := uuid.Parse(uid); err == nil {
		return "urn:uuid:" + uid
	}
	return uid
}

// CleanUID normaliza un UID de contacto a su forma desnuda (sin el prefijo
// urn:uuid:), la MISMA que se guarda en contacts.uid. La proyeccion almacena el
// UID limpio pero la vCard serializada puede llevar "urn:uuid:<x>" (lo antepone
// ensureURN a los UUID). El frontend, que lee el UID de la vCard vía CardDAV,
// puede mandarlo con prefijo: se normaliza aqui para casar por UID de forma
// robusta (favoritos).
func CleanUID(uid string) string { return cleanURN(uid) }

// cleanURN extrae el identificador desnudo de una URI urn:uuid: si lo es.
func cleanURN(uid string) string {
	uid = strings.TrimSpace(uid)
	const p = "urn:uuid:"
	if strings.HasPrefix(strings.ToLower(uid), p) {
		return uid[len(p):]
	}
	return uid
}

// parseBirthday acepta los formatos de fecha mas comunes de BDAY (RFC 6350
// 4.3.1): completa (YYYY-MM-DD), basica (YYYYMMDD).
func parseBirthday(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	for _, layout := range []string{bdayLayout, "20060102", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: BDAY no parseable %q", ErrInvalidVCard, v)
}
