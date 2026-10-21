// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// El móvil de la cuenta (ADR 0041): se verifica con un código por SMS al darse de
// alta y después es uno de los cuatro identificadores con los que se entra.

// reE164 es un número en formato internacional: «+», un dígito que no es cero y
// entre 7 y 14 más. Es la forma, no la validez: si el número no existe, lo dice
// el proveedor al no poder entregar el SMS.
var reE164 = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// NormalizePhone deja un móvil en E.164 («+34600123456»). Admite lo que la gente
// teclea —espacios, puntos, guiones, paréntesis— y el prefijo internacional «00»
// en lugar del «+». Sin prefijo de país no vale: «600 123 456» es de España o de
// cualquier otro sitio, y adivinarlo es dar de alta el número de otra persona.
func NormalizePhone(raw string) (string, bool) {
	p := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '.', '-', '(', ')', ' ':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if rest, ok := strings.CutPrefix(p, "00"); ok {
		p = "+" + rest
	}
	return p, reE164.MatchString(p)
}

// NormalizeLogin deja lo tecleado en el campo de acceso en su forma canónica: un
// móvil en E.164 o, si no lo es, como correo o @usuario (NormalizeEmail, que se
// queda con el primer campo). El móvil va primero porque trae espacios dentro, y
// NormalizeEmail lo cortaría en «+34».
func NormalizeLogin(raw string) string {
	if p, ok := NormalizePhone(raw); ok {
		return p
	}
	return NormalizeEmail(raw)
}

// PhoneTail son los tres últimos dígitos del móvil: lo que se enseña en pantalla
// para decir a qué número se ha mandado el código sin escribirlo entero.
func PhoneTail(phone string) string {
	if len(phone) < 3 {
		return phone
	}
	return phone[len(phone)-3:]
}

// Límites del código de alta. Seis dígitos y cinco intentos dejan una
// probabilidad de acertar a ciegas de 1 entre 200.000 por código; diez minutos
// dan tiempo a que llegue un SMS lento sin dejar el código vivo toda la tarde.
const (
	SignupCodeTTL         = 10 * time.Minute
	SignupCodeMaxAttempts = 5
	// SignupCodeResendAfter es lo que hay que esperar para pedir otro: evita que
	// un doble clic mande dos SMS y que alguien use el botón para bombardear.
	SignupCodeResendAfter = time.Minute
)

// SignupCode es un alta a medio hacer: lo que la persona rellenó, a la espera de
// que confirme el código que se le mandó al móvil. La cuenta no existe todavía.
type SignupCode struct {
	ID           string
	Phone        string
	Username     string
	Name         string
	PasswordHash string
	CodeHash     string
	Attempts     int
	SentAt       time.Time
	ExpiresAt    time.Time
}

// NewSignupCode genera un código de seis dígitos con crypto/rand.
func NewSignupCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashSignupCode liga el código a SU alta: el mismo «123456» de otra alta da otro
// hash, así que un código no sirve fuera de la fila para la que se emitió.
func HashSignupCode(id, code string) string { return HashToken(id + ":" + code) }
