// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package carrete

import (
	"encoding/xml"
	"strings"
)

// # La biblioteca Imágenes de Windows
//
// La app Fotos de Windows (y el Explorador, en «Imágenes») no lee UNA carpeta: lee la
// biblioteca Imágenes, que es la lista de carpetas de
// %APPDATA%\Microsoft\Windows\Libraries\Pictures.library-ms. Es un XML que escribe el
// propio Windows: cada carpeta es un `<simpleLocation><url>`, que es una ruta («D:\Fotos»,
// una carpeta que la persona añadió) o una carpeta conocida
// («knownfolder:{33E28130-…}», la de imágenes del perfil, que OneDrive puede haber
// movido). Leerlo no pide COM, y su forma es la misma desde Windows 7.

// bibliotecaXML es lo que importa del .library-ms.
type bibliotecaXML struct {
	Lugares []struct {
		URL string `xml:"simpleLocation>url"`
	} `xml:"searchConnectorDescriptionList>searchConnectorDescription"`
}

// carpetasDeBiblioteca saca las carpetas de una biblioteca de Windows, resolviendo las
// conocidas con `conocida` (el GUID, con sus llaves). Lo que no se entiende o no se
// resuelve se salta: una carpeta de red caída no deja sin galería.
func carpetasDeBiblioteca(datos []byte, conocida func(guid string) (string, error)) []string {
	var b bibliotecaXML
	if xml.Unmarshal(datos, &b) != nil {
		return nil
	}
	var l []string
	for _, lugar := range b.Lugares {
		url := strings.TrimSpace(lugar.URL)
		switch {
		case url == "":
		case strings.HasPrefix(strings.ToLower(url), "knownfolder:"):
			if c, err := conocida(url[len("knownfolder:"):]); err == nil && c != "" {
				l = append(l, c)
			}
		case strings.Contains(url, "://"):
			// Una dirección (un conector de búsqueda, una biblioteca en la red): no es una
			// carpeta que se pueda recorrer.
		default:
			l = append(l, url)
		}
	}
	return l
}

// pistaConectividad es NL_NETWORK_CONNECTIVITY_HINT, la pista de conectividad de Windows
// (carpeta_windows.go). Aquí y no allí para probar la regla en cualquier sistema.
type pistaConectividad struct {
	Nivel, Coste                    int32
	CercaDelLimite, PasadoDelLimite byte
	Itinerancia                     byte
	_                               byte
}

// Los costes de NL_NETWORK_CONNECTIVITY_COST_HINT.
const (
	costeFijo     = 2 // una tarifa con límite
	costeVariable = 3 // se paga por lo que se usa
)

// medidaPorPista: de uso medido con coste fijo o variable, pasada de su límite o en
// itinerancia.
func medidaPorPista(p pistaConectividad) bool { //nolint:unused // solo en Windows y en las pruebas
	return p.Coste == costeFijo || p.Coste == costeVariable || p.PasadoDelLimite != 0 || p.Itinerancia != 0
}
