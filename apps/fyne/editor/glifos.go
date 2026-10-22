// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

// Los glifos PROPIOS de la portada de documentos.
//
// No están en `apps/fyne/iconos` porque en la web tampoco están en `packages/ui`: viven
// en `apps/frontend/packages/office/src/components/DocList.tsx` (`KIND_PATH` y `GLYPHS`), que
// es de una app. El catálogo compartido es el espejo EXACTO de `Icon.tsx` y su guardián
// compara en los dos sentidos, así que meterlos allí lo pondría en rojo — es la regla
// que `iconos.TrazoDeRuta` explica con los tres agentes que llegaron a ella.
//
// `TestLosGlifosDeLaPortadaSonLosDeLaWeb` los compara ruta a ruta contra ese `.tsx`.
const (
	// GlifoDocumento es la hoja con la esquina doblada y tres renglones.
	GlifoDocumento = "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5M9 13h6M9 17h4"
	// GlifoHoja es la rejilla: la tabla se reconoce por sus divisiones.
	GlifoHoja = "M4 5h16v14H4zM4 10h16M4 15h16M10 5v14"
	// GlifoPresentacion es el lienzo apaisado sobre pie.
	GlifoPresentacion = "M4 4h16v11H4zM12 15v4M9 21h6"

	// GlifoLapiz es el `pencil` de «Renombrar».
	GlifoLapiz = "M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"
	// GlifoPapelera es el `trash` de «Eliminar». NO es el `trash` de `Icon.tsx`: dibujan
	// cosas distintas, y por eso está aquí y no allí.
	GlifoPapelera = "M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6h14M10 11v6M14 11v6"
	// GlifoSubir es el `upload` de «Abrir un archivo».
	GlifoSubir = "M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12"
	// GlifoDescargar es el `download` de la tesela.
	GlifoDescargar = "M12 3v11m0 0 4-4m-4 4-4-4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"
	// GlifoCompartir son los tres nodos unidos.
	GlifoCompartir = "M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm12 7a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM8.6 13.5l6.8 3.9M15.4 6.6 8.6 10.5"
	// GlifoEstrella es la estrella de «Destacar»: el mismo trazado que la de Drive.
	GlifoEstrella = "m12 3 2.9 5.9 6.5.9-4.7 4.6 1.1 6.4L12 17.8l-5.8 3 1.1-6.4-4.7-4.6 6.5-.9Z"
)

// FuenteGlifos es el `.tsx` del que salen, relativo a la raíz del repositorio.
const FuenteGlifos = "apps/frontend/packages/office/src/components/DocList.tsx"

// GlifosPortada son los de `GLYPHS` por su clave, y GlifosTipo los de `KIND_PATH`. Es lo
// que compara el guardián.
var (
	GlifosPortada = map[string]string{
		"pencil": GlifoLapiz, "trash": GlifoPapelera, "upload": GlifoSubir,
		"download": GlifoDescargar, "share": GlifoCompartir, "star": GlifoEstrella,
	}
	GlifosTipo = map[string]string{
		"document": GlifoDocumento, "spreadsheet": GlifoHoja, "presentation": GlifoPresentacion,
	}
)

// Glifo es el dibujo del tipo de documento.
func (t Tipo) Glifo() string {
	switch t {
	case Hoja:
		return GlifoHoja
	case Presentacion:
		return GlifoPresentacion
	case Documento:
	}
	return GlifoDocumento
}
