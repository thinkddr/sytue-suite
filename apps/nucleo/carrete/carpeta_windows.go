// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package carrete

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// carpetasGaleria en Windows es la biblioteca Imágenes (biblioteca.go) y, por si la
// persona la ha vaciado o no se puede leer, la carpeta de imágenes del perfil, que es
// donde se guarda.
func carpetasGaleria() ([]string, error) {
	r, err := raiz()
	if err != nil {
		return nil, err
	}
	l := []string{r}
	if datos, err := os.ReadFile(rutaBiblioteca()); err == nil { //nolint:gosec // la biblioteca del propio usuario, en su perfil
		l = append(l, carpetasDeBiblioteca(datos, carpetaConocida)...)
	}
	return l, nil
}

// rutaBiblioteca es la de Pictures.library-ms, en la carpeta de bibliotecas del perfil.
func rutaBiblioteca() string {
	if d, err := windows.KnownFolderPath(windows.FOLDERID_UsersLibraries, windows.KF_FLAG_DEFAULT); err == nil && d != "" {
		return filepath.Join(d, "Pictures.library-ms")
	}
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Libraries", "Pictures.library-ms")
}

// carpetaConocida resuelve «{GUID}» con SHGetKnownFolderPath.
func carpetaConocida(guid string) (string, error) {
	id, err := windows.GUIDFromString(guid)
	if err != nil {
		return "", err
	}
	return windows.KnownFolderPath((*windows.KNOWNFOLDERID)(&id), windows.KF_FLAG_DEFAULT)
}

// La pista de conectividad de Windows (iphlpapi, Windows 10 2004 y posteriores): lo que
// el sistema sabe del coste de la red de ahora. Es la misma que usa Windows Update para
// no descargar con una conexión de uso medido.
var pistaRed = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetNetworkConnectivityHint")

// redMedida dice si Windows tiene la red de ahora por de uso medido: con coste fijo o
// variable, pasada de su límite o en itinerancia. En un Windows anterior a la 2004, o si
// no contesta, no: ahí no hay forma barata de saberlo, y un portátil casi siempre está
// en una red sin coste.
func redMedida() bool {
	if pistaRed.Find() != nil {
		return false
	}
	var p pistaConectividad
	if r, _, _ := pistaRed.Call(uintptr(unsafe.Pointer(&p))); r != 0 {
		return false
	}
	return medidaPorPista(p)
}
