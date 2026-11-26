// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer

import (
	"bytes"
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/local/correo"
)

// Mail: traer cada correo a «Importados» sin repetirlo (repetir.go).

// MaxEML es lo más grande que se importa de un correo (lo de más se copia).
const MaxEML = 50 << 20

// claveDeCorreo es con lo que se reconoce un correo: su Message-ID o, sin él, la huella
// de su .eml.
func claveDeCorreo(crudo []byte) string {
	if id := correo.MessageIDDe(crudo); id != "" {
		return "id:" + id
	}
	return "eml:" + huellaCorta(crudo)
}

// enImportados son las claves de los correos que ya hay en «Importados».
func (r *repartidor) enImportados(b *correo.Buzon) (map[string]bool, error) {
	out := map[string]bool{}
	cursor := ""
	for {
		ms, siguiente, err := b.Messages(r.ctx, correo.CarpetaImportados, cursor, 500)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			if m.MessageID != "" {
				out["id:"+m.MessageID] = true
				continue
			}
			if crudo, err := leerCorreo(r.ctx, b, m.ID); err == nil {
				out["eml:"+huellaCorta(crudo)] = true
			}
		}
		if siguiente == "" || siguiente == cursor {
			return out, nil
		}
		cursor = siguiente
	}
}

// leerCorreo es el .eml de un correo de aquí.
func leerCorreo(ctx context.Context, b *correo.Buzon, id string) ([]byte, error) {
	rc, err := b.Raw(ctx, id)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(rc)
	return buf.Bytes(), err
}

// correo importa cada `.eml` de `mail/messages/` en la carpeta «Importados» de Mail, que
// lo lee con su lector de siempre, salvo los que ya están allí (por su Message-ID o, sin
// él, por la huella del .eml) o se llevaron otra vez y la persona quitó de allí. Las
// tablas de buzones y mensajes se copian.
func (r *repartidor) correo(es []entrada) ([]entrada, error) {
	var resto, emls []entrada
	for _, e := range es {
		if strings.HasPrefix(e.ruta, "mail/messages/") && strings.EqualFold(path.Ext(e.ruta), ".eml") && e.f.UncompressedSize64 <= MaxEML {
			emls = append(emls, e)
		} else {
			resto = append(resto, e)
		}
	}
	if len(emls) == 0 {
		return resto, nil
	}
	if r.d.Secretos == nil {
		return es, errors.New("traer: sin secretos no se abre el correo")
	}
	b, err := correo.Abrir(filepath.Join(r.d.Datos, "Correo"), r.d.Secretos, correo.Red{})
	if err != nil {
		return es, err
	}
	defer b.Cerrar()
	ya, err := r.enImportados(b)
	if err != nil {
		return es, err
	}
	p := r.parte("mail")
	for _, e := range emls {
		datos, err := e.leer()
		if err != nil {
			p.Fallidos++
			resto = append(resto, e)
			continue
		}
		clave := claveDeCorreo(datos)
		llevado := huellaCorta([]byte(clave))
		switch {
		case ya[clave]:
			p.Repetidos++
			r.mem.Correos[llevado] = true
			continue
		case r.mem.Correos[llevado]:
			p.Borrados++
			continue
		}
		if _, err := b.ImportarEML(r.ctx, datos); err != nil {
			p.Fallidos++
			resto = append(resto, e)
			continue
		}
		ya[clave] = true
		r.mem.Correos[llevado] = true
		p.Importados++
	}
	return resto, nil
}
