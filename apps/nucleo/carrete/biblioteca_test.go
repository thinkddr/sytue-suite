// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package carrete

import (
	"errors"
	"testing"
	"unsafe"
)

// Una Pictures.library-ms como la que escribe Windows 11, con la carpeta del perfil por
// su carpeta conocida, una del disco D: que añadió la persona, una conocida que ya no
// existe y un conector de búsqueda que no es una carpeta.
const bibliotecaDeWindows = `<?xml version="1.0" encoding="UTF-8"?>
<libraryDescription xmlns="http://schemas.microsoft.com/windows/2009/library">
  <name>@shell32.dll,-34575</name>
  <ownerSID>S-1-5-21-1</ownerSID>
  <version>6</version>
  <isLibraryPinned>true</isLibraryPinned>
  <iconReference>imageres.dll,-1003</iconReference>
  <templateInfo><folderType>{B3690E58-E961-423B-B687-386EBFD83239}</folderType></templateInfo>
  <propertyStore><property name="HasModifiedLocations" type="boolean"><![CDATA[true]]></property></propertyStore>
  <searchConnectorDescriptionList>
    <searchConnectorDescription publisher="Microsoft" product="Windows">
      <description>@shell32.dll,-34577</description>
      <isDefaultSaveLocation>true</isDefaultSaveLocation>
      <isSupported>true</isSupported>
      <simpleLocation>
        <url>knownfolder:{33E28130-4E1E-4676-835A-98395C3BC3BB}</url>
        <serialized>MBAAAEAFCAAA</serialized>
      </simpleLocation>
    </searchConnectorDescription>
    <searchConnectorDescription>
      <isSupported>true</isSupported>
      <simpleLocation><url>D:\Fotos de la cámara</url></simpleLocation>
    </searchConnectorDescription>
    <searchConnectorDescription>
      <simpleLocation><url>knownfolder:{AB5FB87B-7CE2-4F83-915D-550846C9537B}</url></simpleLocation>
    </searchConnectorDescription>
    <searchConnectorDescription>
      <simpleLocation><url>https://buscador.example/fotos</url></simpleLocation>
    </searchConnectorDescription>
  </searchConnectorDescriptionList>
</libraryDescription>`

func TestCarpetasDeBiblioteca(t *testing.T) {
	conocidas := map[string]string{"{33E28130-4E1E-4676-835A-98395C3BC3BB}": `C:\Users\ana\OneDrive\Imágenes`}
	got := carpetasDeBiblioteca([]byte(bibliotecaDeWindows), func(guid string) (string, error) {
		if c, ok := conocidas[guid]; ok {
			return c, nil
		}
		return "", errors.New("no existe")
	})
	want := []string{`C:\Users\ana\OneDrive\Imágenes`, `D:\Fotos de la cámara`}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("carpetas %q, quiero %q", got, want)
	}
	if l := carpetasDeBiblioteca([]byte("no es XML"), nil); l != nil {
		t.Errorf("un fichero roto da %q", l)
	}
}

// TestMedidaPorPista: la regla de uso medido de Windows, y que la estructura mide lo que
// NL_NETWORK_CONNECTIVITY_HINT (dos enums y tres BOOLEAN: 12 bytes con el relleno).
func TestMedidaPorPista(t *testing.T) {
	if n := unsafe.Sizeof(pistaConectividad{}); n != 12 {
		t.Fatalf("NL_NETWORK_CONNECTIVITY_HINT mide 12 bytes; la estructura, %d", n)
	}
	for _, c := range []struct {
		p    pistaConectividad
		want bool
	}{
		{pistaConectividad{Coste: 1}, false}, // sin restricciones
		{pistaConectividad{Coste: 0}, false}, // no se sabe
		{pistaConectividad{Coste: costeFijo}, true},
		{pistaConectividad{Coste: costeVariable}, true},
		{pistaConectividad{Coste: 1, PasadoDelLimite: 1}, true},
		{pistaConectividad{Coste: 1, Itinerancia: 1}, true},
		{pistaConectividad{Coste: 1, CercaDelLimite: 1}, false},
	} {
		if got := medidaPorPista(c.p); got != c.want {
			t.Errorf("medidaPorPista(%+v) = %v, quiero %v", c.p, got, c.want)
		}
	}
}
