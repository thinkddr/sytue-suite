// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"strings"
)

// DatosLegales son los datos de titularidad de la web (PAGES-ENCARGO §7.2).
type DatosLegales struct {
	HolderName     string             `json:"holder_name"`
	TaxID          string             `json:"tax_id"`
	Country        string             `json:"country"`
	Address        DireccionLegal     `json:"address"`
	Email          string             `json:"email"`
	Phone          string             `json:"phone"`
	Registry       string             `json:"registry"`
	PrivacyContact string             `json:"privacy_contact"`
	Profession     *ProfesionLegal    `json:"profession,omitempty"`
	Authorization  *AutorizacionLegal `json:"authorization,omitempty"`
}

type DireccionLegal struct {
	Street     string `json:"street"`
	PostalCode string `json:"postal_code"`
	City       string `json:"city"`
	Region     string `json:"region"`
}

type ProfesionLegal struct {
	Title   string `json:"title"`
	College string `json:"college"`
	Number  string `json:"number"`
	Country string `json:"country"`
}

type AutorizacionLegal struct {
	Authority string `json:"authority"`
	Number    string `json:"number"`
}

// EsLegalCompleto valida si los datos legales mínimos para publicar en vivo
// están presentes (PAGES-ENCARGO §7.2, LSSI art. 10):
// holder_name, tax_id, country, address.street, address.postal_code, address.city y email no vacíos;
// registry obligatorio solo si el país es ES y el NIF empieza por A, B, C, D o F.
func EsLegalCompleto(d DatosLegales) bool {
	if strings.TrimSpace(d.HolderName) == "" ||
		strings.TrimSpace(d.TaxID) == "" ||
		strings.TrimSpace(d.Country) == "" ||
		strings.TrimSpace(d.Address.Street) == "" ||
		strings.TrimSpace(d.Address.PostalCode) == "" ||
		strings.TrimSpace(d.Address.City) == "" ||
		strings.TrimSpace(d.Email) == "" {
		return false
	}

	pais := strings.ToUpper(strings.TrimSpace(d.Country))
	nif := strings.ToUpper(strings.TrimSpace(d.TaxID))
	if pais == "ES" && nif != "" {
		primer := nif[0]
		if primer == 'A' || primer == 'B' || primer == 'C' || primer == 'D' || primer == 'F' {
			if strings.TrimSpace(d.Registry) == "" {
				return false
			}
		}
	}

	return true
}

// TextosDemo son textos de relleno identificables que deben reemplazarse antes de publicar (§13.1).
var TextosDemo = [][]byte{
	[]byte("Aquí va la opinión de un cliente real"),
	[]byte("Nombre Apellido"),
	// Datos del negocio que las plantillas del catálogo traen de ejemplo: el nombre, el teléfono (en
	// el texto, en tel: y en wa.me), el correo, la dirección y el punto del mapa (Madrid).
	[]byte("Nombre del negocio"),
	[]byte("600 000 000"),
	[]byte("34600000000"),
	[]byte("info@example.com"),
	[]byte("Calle Mayor, 1"),
	[]byte("40.4168,-3.7038,15"),
	[]byte("00-0000"), // número de colegiado de ejemplo (sanitarios, abogados, arquitectos...)
	// Datos de ejemplo y avisos a la persona: la nota que lo dice sobrevive hasta que lo cambia.
	[]byte("Tu ciudad"),
	[]byte("ámbial"), // cámbialos / Cámbialas por los tuyos
	[]byte("ajústal"),
	[]byte("cambia cada fila"),
	[]byte("de ejemplo"),
	[]byte("De ejemplo"),
	[]byte("Cambia esta"),
	[]byte("ndica aquí"),
	[]byte("scribe aquí"),
	[]byte("Pon aquí"),
	[]byte("sustituye est"), // «sustituye estos destinos por los tuyos»
}

// TieneContenidoDemo comprueba si el IR contiene textos de plantilla de relleno sin personalizar (§13.1).
func TieneContenidoDemo(irRaw []byte, plantilla string) bool {
	for _, demo := range TextosDemo {
		if bytes.Contains(irRaw, demo) {
			return true
		}
	}
	return false
}
