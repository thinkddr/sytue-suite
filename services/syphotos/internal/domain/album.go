package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Album es una colección ordenada de fotos de una persona. La portada es
// explícita si la eligió; si no, la primera foto del álbum, y eso lo resuelve
// el store al leer, no una columna que habría que mantener.
type Album struct {
	ID        uuid.UUID
	Tenant    string
	Duenyo    string
	Titulo    string
	PortadaID *uuid.UUID
	// Compartido dice si tiene enlace público. El token no se guarda (solo su
	// huella, migración 0012): existe únicamente al generarlo.
	Compartido    bool
	Fotos         int
	CreadoEn      time.Time
	ActualizadoEn time.Time
	// Editable: compartido CON QUIEN PREGUNTA para editar (0010). Solo lo rellenan las
	// lecturas de «Compartido conmigo».
	Editable bool
}

// MaxTituloDeAlbum es lo que cabe en una tarjeta; más largo es un párrafo.
const MaxTituloDeAlbum = 200

// TituloDeAlbum normaliza y valida un título: sin espacios en los bordes, no
// vacío y de longitud razonable.
func TituloDeAlbum(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: el álbum necesita un título", ErrInvalidInput)
	}
	if utf8.RuneCountInString(s) > MaxTituloDeAlbum {
		return "", fmt.Errorf("%w: título de más de %d caracteres", ErrInvalidInput, MaxTituloDeAlbum)
	}
	return s, nil
}

// PersonaDelAlbum es alguien con quien su dueña ha compartido un álbum: su `sub` y
// cuándo. El correo y el nombre no se guardan —cambian—; los pone quien pinta la lista.
type PersonaDelAlbum struct {
	Sujeto string
	Editar bool
	Creado time.Time
}
