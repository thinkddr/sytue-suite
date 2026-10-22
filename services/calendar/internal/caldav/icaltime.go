// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// parseICalRaw parsea un DATE o DATE-TIME crudo de iCalendar (RFC 5545 3.3.4/
// 3.3.5) en sus formas basicas: "YYYYMMDD", "YYYYMMDDTHHMMSS" (flotante/local) y
// "YYYYMMDDTHHMMSSZ" (UTC). Devuelve el instante (UTC para la forma con Z).
func parseICalRaw(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch {
	case len(s) == 8:
		return time.Parse("20060102", s)
	case len(s) == 15:
		return time.ParseInLocation("20060102T150405", s, time.UTC)
	case len(s) == 16 && strings.HasSuffix(s, "Z"):
		return time.Parse("20060102T150405Z", s)
	default:
		return time.Time{}, fmt.Errorf("formato DATE/DATE-TIME invalido: %q", s)
	}
}

// parseICalDuration parsea una DURATION de iCalendar (RFC 5545 3.3.6), p. ej.
// "PT1H30M", "P1D", "-PT15M". Soporta semanas (W), dias (D), horas (H), minutos
// (M) y segundos (S).
//
//nolint:gocyclo // parser carácter a carácter de DURATION (RFC 5545 §3.3.6): la ramificación es la gramática.
func parseICalDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("DURATION vacia")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("DURATION debe empezar por P: %q", s)
	}
	s = s[1:]

	var total time.Duration
	inTime := false
	num := strings.Builder{}

	flush := func(unit byte) error {
		if num.Len() == 0 {
			return fmt.Errorf("DURATION: falta numero antes de %c", unit)
		}
		n, err := strconv.Atoi(num.String())
		num.Reset()
		if err != nil {
			return err
		}
		switch unit {
		case 'W':
			total += time.Duration(n) * 7 * 24 * time.Hour
		case 'D':
			total += time.Duration(n) * 24 * time.Hour
		case 'H':
			total += time.Duration(n) * time.Hour
		case 'M':
			total += time.Duration(n) * time.Minute
		case 'S':
			total += time.Duration(n) * time.Second
		default:
			return fmt.Errorf("DURATION: unidad desconocida %c", unit)
		}
		return nil
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 'T':
			inTime = true
		case unicode.IsDigit(rune(c)):
			num.WriteByte(c)
		case c == 'W' || c == 'D':
			if err := flush(c); err != nil {
				return 0, err
			}
		case c == 'H' || c == 'S':
			if err := flush(c); err != nil {
				return 0, err
			}
		case c == 'M':
			// 'M' es minutos solo en la parte de tiempo; en fecha seria meses
			// (no soportado por DURATION de iCalendar).
			if !inTime {
				return 0, fmt.Errorf("DURATION: 'M' fuera de la parte de tiempo")
			}
			if err := flush(c); err != nil {
				return 0, err
			}
		default:
			return 0, fmt.Errorf("DURATION: caracter invalido %q", c)
		}
	}
	if neg {
		total = -total
	}
	return total, nil
}
