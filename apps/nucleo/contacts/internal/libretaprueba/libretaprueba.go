// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package libretaprueba escribe libretas de Contacts grandes y realistas para las
// medidas (R-MEDIR) del repositorio de Contacts y de su lector entre apps. Solo lo
// importan las pruebas.
//
// Dar de alta miles de contactos con el repositorio llevaría mucho (cada alta escribe
// y valida), así que [Escribir] crea el almacén y la libreta «Personal» con Contacts y
// escribe cada contacto directamente con el puerto del almacén, en el mismo formato
// que el motor de registros (contenido e índice de cada agregado), con agregados
// hechos por Contacts (`Snapshot.NewPerson`) sobre una instantánea que solo tiene la
// libreta.
package libretaprueba

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/card"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// contenidoRegistro es el contenido de un agregado en el motor de registros.
type contenidoRegistro struct {
	Kind string `json:"kind"`
	JSON []byte `json:"json"`
}

const tipoEntidad = "org.sytue.contacts.entity.v1"

// Escribir da de alta n contactos realistas en dir: crea el almacén y la libreta
// «Personal» con Contacts y escribe cada contacto con el puerto. Devuelve el
// identificador de la libreta.
func Escribir(dir, sujeto string, n int) (string, error) {
	ctx := context.Background()
	rec, err := almacen.Abrir(ctx, dir, "contacts", sujeto)
	if err != nil {
		return "", err
	}
	repo, err := contacts.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		return "", err
	}
	if err := almacen.Preparar(ctx, repo); err != nil {
		repo.Close()
		rec.Cerrar()
		return "", err
	}
	libro, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		repo.Close()
		rec.Cerrar()
		return "", err
	}
	foto, err := repo.Snapshot(ctx)
	repo.Close()
	rec.Cerrar()
	if err != nil {
		return "", err
	}
	scope := storage.Scope{Tenant: "local", Subject: sujeto, Product: "contacts"}
	p, err := almacen.AbrirPuerto(dir, scope)
	if err != nil {
		return "", err
	}
	defer func() { _ = p.Cerrar() }()
	enlace, err := p.StorageBindings(ctx)
	if err != nil {
		return "", err
	}
	r := rand.New(rand.NewPCG(2026, 11)) //nolint:gosec // G404: datos de prueba repetibles, no un secreto
	ahora := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	for i := range n {
		ag, err := foto.NewPerson(libro.ID, Realista(r, i), "", ahora)
		if err != nil {
			return "", err
		}
		crudo, err := json.Marshal(ag)
		if err != nil {
			return "", err
		}
		rev := uuid.NewString()
		h := sha256.Sum256([]byte(ag.Kind + "\x00" + ag.ID))
		objeto := "contacts_" + hex.EncodeToString(h[:])
		carga, _ := json.Marshal(contenidoRegistro{Kind: tipoEntidad, JSON: crudo})
		ref, err := p.PutStorageRecordAt(ctx, enlace.Target, rev+"_content", storage.Record{
			Version: 1, Scope: scope,
			ObjectID: objeto, RevisionID: rev, Role: "content", Payload: carga,
		})
		if err != nil {
			return "", err
		}
		suma := sha256.Sum256(crudo)
		meta, _ := json.Marshal(map[string]string{"kind": tipoEntidad, "entity": ag.Kind, "id": ag.ID, "sha256": hex.EncodeToString(suma[:])})
		if _, err := p.PutStorageObjectAt(ctx, enlace.Target, rev+"_index", objeto, rev+"_index", storage.ObjectCommit{
			Kind: storage.ObjectKind, State: "live", Metadata: meta,
			Content: &storage.ContentPointer{Kind: "record", RevisionID: rev, Reference: ref},
		}); err != nil {
			return "", err
		}
	}
	return libro.ID, nil
}

var (
	nombresPila = []string{
		"María", "José", "Ana", "Antonio", "Lucía", "Manuel", "Carmen", "Francisco", "Laura", "David",
		"Isabel", "Javier", "Marta", "Daniel", "Elena", "Carlos", "Paula", "Miguel", "Sara", "Pablo", "Núria", "Jordi",
		"Aitor", "Iratxe", "Xoán", "Uxía", "Sophie", "Pierre", "Giulia", "Marco", "Anna", "Jan", "Emma", "Lukas", "Inês", "João",
	}
	apellidos = []string{
		"García", "Fernández", "González", "Rodríguez", "López", "Martínez", "Sánchez", "Pérez", "Gómez",
		"Martín", "Jiménez", "Ruiz", "Hernández", "Díaz", "Moreno", "Muñoz", "Álvarez", "Romero", "Alonso", "Gutiérrez",
		"Navarro", "Torres", "Domínguez", "Vázquez", "Ramos", "Gil", "Ramírez", "Serrano", "Blanco", "Molina", "Puig",
		"Etxeberria", "Castro", "Ortega", "Rubio", "Marín", "Sanz", "Iglesias", "Núñez", "Medina", "Garrido", "Dubois",
		"Rossi", "Müller", "Silva", "Kowalski",
	}
	empresas = []string{
		"Construcciones Levante", "Talleres Norte", "Asesoría Fiscal Ruiz", "Clínica Dental Sonrisa",
		"Distribuciones Ibéricas", "Hotel Mirador", "Bodegas del Duero", "Transportes Rápidos", "Agencia Brava",
		"Estudio Arquitectura Luz", "Farmacia Central", "Ayuntamiento", "Universidad", "Colegio San José",
	}
	cargos    = []string{"Gerente", "Administración", "Compras", "Ventas", "Dirección", "Contabilidad", "Recepción", "Técnico"}
	dominios  = []string{"gmail.com", "hotmail.com", "outlook.es", "yahoo.es", "icloud.com", "telefonica.net", "proton.me"}
	ciudades  = [][2]string{{"Madrid", "28013"}, {"Barcelona", "08002"}, {"Valencia", "46002"}, {"Sevilla", "41001"}, {"Bilbao", "48001"}, {"Zaragoza", "50001"}, {"A Coruña", "15001"}, {"Málaga", "29005"}}
	calles    = []string{"Calle Mayor", "Avenida de la Constitución", "Plaza de España", "Calle del Sol", "Paseo Marítimo", "Calle Real"}
	notasTipo = []string{
		"Conocido en la feria de 2024.", "Prefiere que se le llame por la tarde.", "Cliente desde 2019; factura trimestral.",
		"Amiga de la familia.", "Proveedor de material de oficina.", "Vecino del tercero.",
	}
)

// sinAcentos es la parte local de una dirección: minúsculas y sin tildes, como se escribe.
var sinAcentos = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u", "à", "a", "ê", "e", "ç", "c", " ", "")

// Realista es una persona como las de una libreta de verdad: nombre y dos apellidos,
// uno a tres correos (el personal y, a veces, el del trabajo), uno o dos teléfonos, y
// a veces empresa, cargo, dirección, apodo, nota y cumpleaños. i hace únicas sus
// direcciones.
func Realista(r *rand.Rand, i int) card.Contact {
	pila := nombresPila[r.IntN(len(nombresPila))]
	a1, a2 := apellidos[r.IntN(len(apellidos))], apellidos[r.IntN(len(apellidos))]
	c := card.Contact{UID: card.NewUID(), GivenName: pila, FamilyName: a1 + " " + a2, FormattedName: pila + " " + a1 + " " + a2}
	local := strings.ToLower(sinAcentos.Replace(pila + "." + a1))
	c.Emails = append(c.Emails, card.Email{Email: fmt.Sprintf("%s%d@%s", local, i%997, dominios[r.IntN(len(dominios))]), Type: "home", IsPrimary: true})
	c.Phones = append(c.Phones, card.Phone{Phone: fmt.Sprintf("+34 6%02d %03d %03d", r.IntN(100), r.IntN(1000), r.IntN(1000)), Type: "cell", IsPrimary: true})
	if r.IntN(10) < 4 {
		e := empresas[r.IntN(len(empresas))]
		c.Organization, c.Title = e, cargos[r.IntN(len(cargos))]
		dom := strings.ToLower(sinAcentos.Replace(strings.Fields(e)[0])) + ".es"
		c.Emails = append(c.Emails, card.Email{Email: strings.ToLower(sinAcentos.Replace(pila)) + "@" + dom, Type: "work"})
		c.Phones = append(c.Phones, card.Phone{Phone: fmt.Sprintf("+34 9%02d %03d %03d", r.IntN(100), r.IntN(1000), r.IntN(1000)), Type: "work"})
	}
	if r.IntN(10) == 0 {
		c.Emails = append(c.Emails, card.Email{Email: fmt.Sprintf("%s.%d@%s", local, i, dominios[r.IntN(len(dominios))]), Type: "other"})
	}
	if r.IntN(2) == 0 {
		ci := ciudades[r.IntN(len(ciudades))]
		c.Addresses = append(c.Addresses, card.Address{
			Street:   fmt.Sprintf("%s, %d", calles[r.IntN(len(calles))], 1+r.IntN(120)),
			Locality: ci[0], PostalCode: ci[1], Country: "España", Type: "home",
		})
	}
	if r.IntN(8) == 0 {
		c.Nickname = strings.ToLower(pila[:min(len(pila), 3)])
	}
	if r.IntN(6) == 0 {
		c.Note = notasTipo[r.IntN(len(notasTipo))]
	}
	if r.IntN(5) == 0 {
		c.BirthdayNoYear = fmt.Sprintf("--%02d-%02d", 1+r.IntN(12), 1+r.IntN(28))
	}
	return c
}
