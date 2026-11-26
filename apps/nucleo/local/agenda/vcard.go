// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"bytes"
	"encoding/base64"
	"strings"

	"github.com/emersion/go-vcard"
)

// Las vCard que van y vienen de CardDAV: los grupos (KIND:group de la 4.0, o
// X-ADDRESSBOOKSERVER-KIND:group, que es como los guardan iCloud y los servidores de
// Apple en 3.0) y el paso a vCard 3.0 de lo que se guarda aquí en 4.0, para el servidor
// que no la quiere (K-05).

const (
	campoKindApple   = "X-ADDRESSBOOKSERVER-KIND"
	campoMemberApple = "X-ADDRESSBOOKSERVER-MEMBER"
)

// esGrupoVCard dice si una vCard es un grupo, en cualquiera de las dos formas.
func esGrupoVCard(datos string) bool {
	if !strings.Contains(strings.ToUpper(datos), "GROUP") {
		return false
	}
	card, err := vcard.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil {
		return false
	}
	return strings.EqualFold(card.Value(vcard.FieldKind), "group") || strings.EqualFold(card.Value(campoKindApple), "group")
}

// grupoV4 pasa un grupo de la forma de Apple a la de la 4.0 (KIND y MEMBER), que es la
// que guarda Contacts; uno que ya está así se queda igual.
func grupoV4(datos string) string {
	card, err := vcard.NewDecoder(strings.NewReader(datos)).Decode()
	if err != nil || !strings.EqualFold(card.Value(campoKindApple), "group") {
		return datos
	}
	card.SetValue(vcard.FieldKind, "group")
	for _, f := range card[campoMemberApple] {
		card.Add(vcard.FieldMember, &vcard.Field{Value: f.Value})
	}
	delete(card, campoKindApple)
	delete(card, campoMemberApple)
	var b bytes.Buffer
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		return datos
	}
	return b.String()
}

// aVCard3 escribe en 3.0 (RFC 2426) una vCard 4.0 de aquí: los grupos a la forma de
// Apple, el teléfono sin «tel:», la preferencia como TYPE=pref, la foto en base64 y el
// cumpleaños sin año como lo escribe Apple (año 1604 y X-APPLE-OMIT-YEAR). N es
// obligatoria en 3.0. Lo demás va tal cual.
func aVCard3(datos []byte) ([]byte, error) {
	card, err := vcard.NewDecoder(bytes.NewReader(datos)).Decode()
	if err != nil {
		return nil, err
	}
	card.SetValue(vcard.FieldVersion, "3.0")
	if strings.EqualFold(card.Value(vcard.FieldKind), "group") {
		card.SetValue(campoKindApple, "group")
		for _, f := range card[vcard.FieldMember] {
			card.Add(campoMemberApple, &vcard.Field{Value: f.Value})
		}
		delete(card, vcard.FieldMember)
	}
	delete(card, vcard.FieldKind)
	if card.Get(vcard.FieldName) == nil {
		card.SetValue(vcard.FieldName, strings.ReplaceAll(card.Value(vcard.FieldFormattedName), ";", `\;`)+";;;;")
	}
	camposA3(card)
	for _, f := range card[vcard.FieldPhoto] {
		if tipo, b64, ok := fotoEnLinea(f.Value); ok {
			f.Value = b64
			f.Params = vcard.Params{"ENCODING": {"b"}, vcard.ParamType: {tipo}}
		}
	}
	if f := card.Get(vcard.FieldBirthday); f != nil && strings.HasPrefix(f.Value, "--") {
		md := strings.ReplaceAll(strings.TrimPrefix(f.Value, "--"), "-", "")
		if len(md) == 4 {
			f.Value = "1604-" + md[:2] + "-" + md[2:]
			if f.Params == nil {
				f.Params = vcard.Params{}
			}
			f.Params.Set("X-APPLE-OMIT-YEAR", "1604")
		}
	}
	var b bytes.Buffer
	if err := vcard.NewEncoder(&b).Encode(card); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// camposA3 pasa los campos de 4.0 a 3.0: la preferencia como TYPE=pref, sin PID, y el
// teléfono sin VALUE=uri ni «tel:».
func camposA3(card vcard.Card) {
	for clave, campos := range card {
		for _, f := range campos {
			if f.Params == nil {
				continue
			}
			if v := f.Params.Get(vcard.ParamPreferred); v != "" {
				delete(f.Params, vcard.ParamPreferred)
				f.Params.Add(vcard.ParamType, "pref")
			}
			if strings.EqualFold(f.Params.Get(vcard.ParamValue), "uri") && clave == vcard.FieldTelephone {
				delete(f.Params, vcard.ParamValue)
			}
			delete(f.Params, "PID")
		}
		if clave == vcard.FieldTelephone {
			for _, f := range campos {
				f.Value = strings.TrimPrefix(f.Value, "tel:")
			}
		}
	}
}

// fotoEnLinea lee una foto `data:image/jpeg;base64,…` y da su tipo («JPEG») y su base64.
func fotoEnLinea(v string) (tipo, b64 string, ok bool) {
	resto, ok := strings.CutPrefix(v, "data:")
	if !ok {
		return "", "", false
	}
	cab, datos, ok := strings.Cut(resto, ",")
	if !ok || !strings.HasSuffix(cab, ";base64") {
		return "", "", false
	}
	mime := strings.TrimSuffix(cab, ";base64")
	if _, err := base64.StdEncoding.DecodeString(datos); err != nil {
		return "", "", false
	}
	_, sub, _ := strings.Cut(mime, "/")
	return strings.ToUpper(sub), datos, true
}
