// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package banca lee los extractos de los bancos y casa cada movimiento con su
// factura.
//
// POR QUÉ NORMA 43 PRIMERO. Es el Cuaderno 43 de la AEB: TODOS los bancos
// españoles (Santander, BBVA, CaixaBank, Sabadell, Bankinter, Unicaja, Kutxabank,
// Abanca, Ibercaja, Cajamar…) lo dan desde su banca de empresas, y leerlo no
// necesita contrato ni licencia. La conexión automática (PSD2) exige un
// proveedor con licencia de información de cuentas y llegará por la pasarela de
// integraciones; sus movimientos entrarán por el mismo casado.
package banca

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Cuenta es un bloque de cuenta del fichero: del registro 11 al 33.
type Cuenta struct {
	Entidad, Oficina, Numero string
	Desde, Hasta             time.Time
	// Saldos en céntimos; negativos si son deudores.
	SaldoInicial, SaldoFinal int64
	// Divisa es el código numérico ISO 4217 («978» es el euro).
	Divisa      string
	Titular     string
	Movimientos []Movimiento
}

// Movimiento es un apunte del extracto.
type Movimiento struct {
	Fecha, FechaValor time.Time
	// Centimos es positivo si entra dinero (abono) y negativo si sale (cargo).
	Centimos int64
	// ConceptoComun es la clave interbancaria: 03 recibos, 04 transferencias,
	// 12 tarjetas, 15 nóminas y seguros sociales, 17 comisiones e impuestos…
	ConceptoComun string
	Documento     string
	Referencia1   string
	Referencia2   string
	// Descripcion junta los conceptos complementarios (registros 23).
	Descripcion string
}

// ErrN43 es un fichero que no es un Norma 43 legible.
var ErrN43 = errors.New("el fichero no es un extracto Norma 43 válido")

// bancos son las entidades más comunes por su código del Banco de España: lo
// que permite enseñar «CaixaBank ··6789» en vez de «2100 0001 0123456789».
var bancos = map[string]string{
	"0049": "Banco Santander", "0182": "BBVA", "2100": "CaixaBank", "0081": "Banco Sabadell",
	"0128": "Bankinter", "2103": "Unicaja", "0073": "Openbank", "1465": "ING", "2095": "Kutxabank",
	"2080": "Abanca", "2085": "Ibercaja", "3058": "Cajamar", "3035": "Laboral Kutxa",
	"0061": "Banca March", "0237": "Cajasur", "1491": "Triodos Bank", "0019": "Deutsche Bank",
}

// Nombre es cómo se enseña la cuenta: el banco y los cuatro últimos dígitos.
func (c Cuenta) Nombre() string {
	banco, ok := bancos[c.Entidad]
	if !ok {
		banco = "Banco " + c.Entidad
	}
	return banco + " ··" + c.Numero[max(0, len(c.Numero)-4):]
}

// Clave identifica la cuenta sin depender del fichero: entidad, oficina y número.
func (c Cuenta) Clave() string {
	return "n43:" + c.Entidad + c.Oficina + c.Numero
}

// conceptos nombra la clave interbancaria para los movimientos que llegan sin
// concepto complementario: mejor «Recibo» que una tarea en blanco.
var conceptos = map[string]string{
	"01": "Talón o reintegro", "02": "Ingreso", "03": "Recibo", "04": "Transferencia", "05": "Préstamo",
	"06": "Remesa de efectos", "08": "Dividendos", "09": "Operación de valores", "11": "Cajero automático",
	"12": "Tarjeta", "13": "Operación con el extranjero", "14": "Devolución", "15": "Nómina o seguros sociales",
	"17": "Intereses, comisiones o impuestos", "98": "Anulación", "99": "Varios",
}

// Texto es la descripción del movimiento para una persona: sus conceptos y la
// referencia (suele ser el número de factura que puso quien paga) o, si el banco
// no manda nada, la clase de operación.
func (m Movimiento) Texto() string {
	var partes []string
	if m.Descripcion != "" {
		partes = append(partes, m.Descripcion)
	}
	if m.Referencia2 != "" && !strings.Contains(m.Descripcion, m.Referencia2) {
		partes = append(partes, m.Referencia2)
	}
	if len(partes) > 0 {
		return strings.Join(partes, " · ")
	}
	if c, ok := conceptos[m.ConceptoComun]; ok {
		return c
	}
	return "Movimiento"
}

// Huellas identifican cada movimiento de una cuenta para que importar dos veces
// el mismo periodo no lo duplique. Dos movimientos idénticos el mismo día (dos
// comisiones iguales) son dos movimientos: llevan su número de orden, y como un
// extracto trae días enteros, el mismo día vuelve con los mismos números.
func Huellas(c Cuenta) []string {
	out := make([]string, len(c.Movimientos))
	vistas := map[string]int{}
	for i, m := range c.Movimientos {
		base := strings.Join([]string{
			m.Fecha.Format("20060102"), m.FechaValor.Format("20060102"), strconv.FormatInt(m.Centimos, 10),
			m.ConceptoComun, m.Documento, m.Referencia1, m.Referencia2, m.Descripcion,
		}, "|")
		vistas[base]++
		suma := sha256.Sum256([]byte(base + "|" + strconv.Itoa(vistas[base])))
		out[i] = c.Clave() + ":" + hex.EncodeToString(suma[:12])
	}
	return out
}

// EsN43 dice si un contenido parece un Norma 43: empieza por una cabecera de
// cuenta (registro 11) con sus campos numéricos donde deben estar.
func EsN43(contenido []byte) bool {
	linea, _, _ := strings.Cut(string(contenido), "\n")
	r := runas(strings.TrimRight(linea, "\r"))
	return len(r) >= 51 && string(r[:2]) == "11" && digitos(string(r[2:51]))
}

// LeerN43 lee un fichero Norma 43 entero y comprueba sus totales: un fichero
// cortado o mal descargado se rechaza en vez de dejar movimientos sin entrar.
func LeerN43(rd io.Reader) ([]Cuenta, error) {
	var lr lectorN43
	sc := bufio.NewScanner(rd)
	for n := 1; sc.Scan(); n++ {
		l := runas(strings.TrimRight(sc.Text(), "\r"))
		if strings.TrimSpace(string(l)) == "" {
			continue
		}
		fin, err := lr.registro(l)
		if err != nil {
			return nil, fmt.Errorf("%w: línea %d: %w", ErrN43, n, err)
		}
		if fin {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if lr.actual != nil {
		return nil, fmt.Errorf("%w: la cuenta %s no tiene registro de cierre (fichero cortado)", ErrN43, lr.actual.Numero)
	}
	if len(lr.cuentas) == 0 {
		return nil, fmt.Errorf("%w: no trae ninguna cuenta", ErrN43)
	}
	return lr.cuentas, nil
}

// lectorN43 es el estado de la lectura: las cuentas cerradas, la abierta y su
// último movimiento, al que se pegan los conceptos complementarios.
type lectorN43 struct {
	cuentas   []Cuenta
	actual    *Cuenta
	ultimo    *Movimiento
	registros int
}

// registro lee una línea; devuelve true al llegar al registro de fin (88).
func (lr *lectorN43) registro(l []rune) (bool, error) {
	switch tipo := campo(l, 1, 2); tipo {
	case "11":
		if lr.actual != nil {
			return false, errors.New("una cuenta empieza sin haber cerrado la anterior")
		}
		c, err := cabecera(l)
		if err != nil {
			return false, err
		}
		lr.actual, lr.ultimo = &c, nil
	case "22":
		if lr.actual == nil {
			return false, errors.New("movimiento fuera de una cuenta")
		}
		m, err := movimiento(l)
		if err != nil {
			return false, err
		}
		lr.actual.Movimientos = append(lr.actual.Movimientos, m)
		lr.ultimo = &lr.actual.Movimientos[len(lr.actual.Movimientos)-1]
	case "23":
		if lr.ultimo == nil {
			return false, errors.New("concepto complementario sin movimiento")
		}
		texto := strings.Join(strings.Fields(campo(l, 5, 42)+" "+campo(l, 43, 80)), " ")
		lr.ultimo.Descripcion = strings.TrimSpace(lr.ultimo.Descripcion + " " + texto)
	case "24":
		// Equivalencia en la divisa de origen: sin valor contable.
	case "33":
		if lr.actual == nil {
			return false, errors.New("cierre de cuenta sin cabecera")
		}
		if err := cierre(l, lr.actual); err != nil {
			return false, err
		}
		lr.cuentas = append(lr.cuentas, *lr.actual)
		lr.actual, lr.ultimo = nil, nil
	case "88":
		if total, err := strconv.Atoi(campo(l, 21, 26)); err == nil && total != lr.registros {
			return false, fmt.Errorf("el fichero dice %d registros y trae %d", total, lr.registros)
		}
		return true, nil
	default:
		return false, errors.New("tipo de registro desconocido " + tipo)
	}
	lr.registros++
	return false, nil
}

func cabecera(l []rune) (Cuenta, error) {
	desde, err1 := fecha(campo(l, 21, 26))
	hasta, err2 := fecha(campo(l, 27, 32))
	saldo, err3 := importe(campo(l, 34, 47), campo(l, 33, 33))
	if err := errors.Join(err1, err2, err3); err != nil {
		return Cuenta{}, err
	}
	return Cuenta{
		Entidad: campo(l, 3, 6), Oficina: campo(l, 7, 10), Numero: campo(l, 11, 20),
		Desde: desde, Hasta: hasta, SaldoInicial: saldo, Divisa: campo(l, 48, 50),
		Titular: campo(l, 52, 77),
	}, nil
}

func movimiento(l []rune) (Movimiento, error) {
	fop, err1 := fecha(campo(l, 11, 16))
	fval, err2 := fecha(campo(l, 17, 22))
	// En un apunte, 1 es Debe (sale dinero) y 2 es Haber (entra): el mismo signo
	// que en un saldo, visto desde quien tiene la cuenta.
	cents, err3 := importe(campo(l, 29, 42), campo(l, 28, 28))
	if err := errors.Join(err1, err2, err3); err != nil {
		return Movimiento{}, err
	}
	return Movimiento{
		Fecha: fop, FechaValor: fval, Centimos: cents, ConceptoComun: campo(l, 23, 24),
		Documento:   strings.TrimLeft(campo(l, 43, 52), "0"),
		Referencia1: strings.TrimLeft(campo(l, 53, 64), "0"),
		Referencia2: campo(l, 65, 80),
	}, nil
}

// cierre lee el registro 33 y comprueba que cuadra con los movimientos leídos.
func cierre(l []rune, c *Cuenta) error {
	nDebe, err1 := strconv.Atoi(campo(l, 21, 25))
	totDebe, err2 := importe(campo(l, 26, 39), "2")
	nHaber, err3 := strconv.Atoi(campo(l, 40, 44))
	totHaber, err4 := importe(campo(l, 45, 58), "2")
	final, err5 := importe(campo(l, 60, 73), campo(l, 59, 59))
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		return err
	}
	var debe, haber int64
	var cargos, abonos int
	for _, m := range c.Movimientos {
		if m.Centimos < 0 {
			debe -= m.Centimos
			cargos++
		} else {
			haber += m.Centimos
			abonos++
		}
	}
	if cargos != nDebe || abonos != nHaber || debe != totDebe || haber != totHaber {
		return fmt.Errorf("los totales de la cuenta %s no cuadran con sus movimientos", c.Numero)
	}
	if c.SaldoInicial+haber-debe != final {
		return fmt.Errorf("el saldo final de la cuenta %s no cuadra con sus movimientos", c.Numero)
	}
	c.SaldoFinal = final
	return nil
}

// campo devuelve las posiciones [desde, hasta] (de 1, como el cuaderno) sin los
// blancos de los lados. Una línea más corta —hay bancos que recortan los blancos
// del final— se lee como si los tuviera.
func campo(l []rune, desde, hasta int) string {
	if desde > len(l) {
		return ""
	}
	return strings.TrimSpace(string(l[desde-1 : min(hasta, len(l))]))
}

// fecha lee AAMMDD. Un Norma 43 es de este siglo.
func fecha(s string) (time.Time, error) {
	t, err := time.Parse("060102", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("fecha ilegible %q", s)
	}
	return t, nil
}

// importe lee 14 dígitos con dos decimales implícitos: con la clave 1 (deudor,
// debe) es negativo y con la 2 (acreedor, haber), positivo.
func importe(s, clave string) (int64, error) {
	if len(s) != 14 || !digitos(s) {
		return 0, fmt.Errorf("importe ilegible %q", s)
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	switch clave {
	case "1":
		return -n, nil
	case "2":
		return n, nil
	}
	return 0, fmt.Errorf("clave de debe o haber desconocida %q", clave)
}

func digitos(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// runas pasa la línea a caracteres. Los bancos españoles escriben el Norma 43 en
// ISO-8859-1: leída como UTF-8, cada «Ñ» o tilde se rompía y además descuadraba
// las posiciones fijas, que cuentan caracteres.
func runas(s string) []rune {
	if utf8.ValidString(s) {
		return []rune(s)
	}
	r := make([]rune, len(s))
	for i := 0; i < len(s); i++ {
		r[i] = rune(s[i])
	}
	return r
}
