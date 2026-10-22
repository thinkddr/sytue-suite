// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Las funciones de Excel que Calcus no tenía y entraron en la versión 3 del catálogo
// (catalogo.ts). Son el espejo de las del motor TypeScript, que están junto a las de su
// familia (stats.ts, financial.ts, text.ts, information.ts); aquí van juntas en un
// fichero para no tocar el de cada familia.

// funcionesCompat es el registro de este fichero. Es un MAPA y no un init(): ver funcionesTexto.
var funcionesCompat = map[string]fn{
	// Las «B» cuentan bytes solo en los idiomas de doble byte: en los demás son las normales.
	"LEFTB":    funcionesTexto["LEFT"],
	"RIGHTB":   funcionesTexto["RIGHT"],
	"MIDB":     funcionesTexto["MID"],
	"LENB":     funcionesTexto["LEN"],
	"FINDB":    funcionesTexto["FIND"],
	"SEARCHB":  funcionesTexto["SEARCH"],
	"REPLACEB": funcionesTexto["REPLACE"],
	"ASC":      conUnTexto(func(t string) Value { return anchoMedio(t) }),
	"DBCS":     conUnTexto(func(t string) Value { return anchoCompleto(t) }),
	"JIS":      conUnTexto(func(t string) Value { return anchoCompleto(t) }),
	"PHONETIC": conUnTexto(func(t string) Value { return t }),
	"ENCODEURL": conUnTexto(func(t string) Value {
		var b strings.Builder
		for i := 0; i < len(t); i++ {
			c := t[i]
			if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
				b.WriteByte(c)
			} else {
				fmt.Fprintf(&b, "%%%02X", c)
			}
		}
		return b.String()
	}),
	"BAHTTEXT": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		n, err := numArg(args, 0)
		if err != "" {
			return err
		}
		return bahtText(n)
	},

	"STDEVPA": deNumeros(collectA, func(nums []float64) Value {
		if len(nums) < 1 {
			return ErrDiv0
		}
		return finito(math.Sqrt(sumSqDev(nums) / float64(len(nums))))
	}),
	"VARPA": deNumeros(collectA, func(nums []float64) Value {
		if len(nums) < 1 {
			return ErrDiv0
		}
		return finito(sumSqDev(nums) / float64(len(nums)))
	}),
	"BINOM.DIST.RANGE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args) > 4 {
			return ErrValue
		}
		a, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		s2 := a[2]
		if len(args) == 4 {
			v, err := numArg(args, 3)
			if err != "" {
				return err
			}
			s2 = v
		}
		n, p, s1 := math.Trunc(a[0]), a[1], math.Trunc(a[2])
		s2 = math.Trunc(s2)
		if n < 0 || p < 0 || p > 1 || s1 < 0 || s1 > n || s2 < s1 || s2 > n {
			return ErrNum
		}
		total := 0.0
		for j := s1; j <= s2; j++ {
			total += binomPmf(j, n, p)
		}
		return finito(total)
	},

	"FVSCHEDULE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		r, err := numArg(args, 0)
		if err != "" {
			return err
		}
		for _, fila := range args[1].Matrix() {
			for _, v := range fila {
				if e, ok := v.(Error); ok {
					return e
				}
				if v == "" {
					continue
				}
				t, err := CoerceNumber(v)
				if err != "" {
					return err
				}
				r *= 1 + t
			}
		}
		if math.IsInf(r, 0) || math.IsNaN(r) {
			return ErrNum
		}
		return r
	},
	"TBILLPRICE": conLetra(func(dias, d float64) Value {
		if d <= 0 {
			return ErrNum
		}
		return 100 * (1 - d*dias/360)
	}),
	"TBILLYIELD": conLetra(func(dias, pr float64) Value {
		if pr <= 0 {
			return ErrNum
		}
		return (100 - pr) / pr * (360 / dias)
	}),
	"TBILLEQ": conLetra(func(dias, d float64) Value {
		if d <= 0 {
			return ErrNum
		}
		if dias <= 182 {
			den := 360 - d*dias
			if den <= 0 {
				return ErrNum
			}
			return 365 * d / den
		}
		precio := 100 * (1 - d*dias/360)
		if precio <= 0 {
			return ErrNum
		}
		a, b, c := dias/730-0.25, dias/365, (precio-100)/precio
		disc := b*b - 4*a*c
		if disc < 0 {
			return ErrNum
		}
		y := (-b + math.Sqrt(disc)) / (2 * a)
		return finito(y)
	}),
	"PRICEMAT": conNums(5, []float64{0}, func(a []float64) Value {
		settle, maturity, issue := math.Floor(a[0]), math.Floor(a[1]), math.Floor(a[2])
		rate, yld, basis := a[3], a[4], math.Trunc(a[5])
		if maturity <= settle || issue > settle || rate < 0 || yld < 0 {
			return ErrNum
		}
		fim, err := yearFrac(issue, maturity, basis)
		if err != "" {
			return err
		}
		fsm, err := yearFrac(settle, maturity, basis)
		if err != "" {
			return err
		}
		fis, err := yearFrac(issue, settle, basis)
		if err != "" {
			return err
		}
		return (100+fim*rate*100)/(1+fsm*yld) - fis*rate*100
	}),
	"YIELDMAT": conNums(5, []float64{0}, func(a []float64) Value {
		settle, maturity, issue := math.Floor(a[0]), math.Floor(a[1]), math.Floor(a[2])
		rate, pr, basis := a[3], a[4], math.Trunc(a[5])
		if maturity <= settle || issue > settle || rate < 0 || pr <= 0 {
			return ErrNum
		}
		fim, err := yearFrac(issue, maturity, basis)
		if err != "" {
			return err
		}
		fsm, err := yearFrac(settle, maturity, basis)
		if err != "" {
			return err
		}
		fis, err := yearFrac(issue, settle, basis)
		if err != "" {
			return err
		}
		if fsm == 0 {
			return ErrNum
		}
		pagado := pr/100 + fis*rate
		return (1 + fim*rate - pagado) / pagado / fsm
	}),
	"AMORLINC": conAmort(func(cost, salvage float64, per int, rate, yf float64) Value {
		anual := cost * rate
		primero := yf * anual
		completos := math.Floor((cost - salvage - primero) / anual)
		p := float64(per)
		switch {
		case per == 0:
			return primero
		case p <= completos:
			return anual
		case p == completos+1:
			return cost - salvage - anual*completos - primero
		}
		return 0.0
	}),
	"AMORDEGRC": conAmort(func(cost, salvage float64, per int, rate0, yf float64) Value {
		vida := 1 / rate0
		coef := 2.5
		switch {
		case vida < 3:
			coef = 1
		case vida < 5:
			coef = 1.5
		case vida <= 6:
			coef = 2
		}
		rate := rate0 * coef
		cuota := math.Floor(yf*rate*cost + 0.5)
		cost -= cuota
		resto := cost - salvage
		for n := 0; n < per; n++ {
			cuota = math.Floor(rate*cost + 0.5)
			resto -= cuota
			if resto < 0 {
				if per-n == 1 {
					return math.Floor(cost*0.5 + 0.5)
				}
				return 0.0
			}
			cost -= cuota
		}
		return cuota
	}),

	"AREAS": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 || args[0].Ref() == nil {
			return ErrValue
		}
		return 1.0
	},
	"FORMULATEXT": func(c *FnCtx, args []*Arg) any {
		if len(args) != 1 || args[0].Ref() == nil {
			return ErrValue
		}
		if t, ok := c.FormulaText(*args[0].Ref()); ok {
			return t
		}
		return ErrNA
	},
	"SHEET": func(c *FnCtx, args []*Arg) any {
		if len(args) > 1 {
			return ErrValue
		}
		nombres := c.SheetNames()
		hoja := c.Here.Sheet
		if len(args) == 1 {
			if ref := args[0].Ref(); ref != nil {
				hoja = ref.Sheet
			} else {
				v := args[0].Value()
				if e, ok := v.(Error); ok {
					return e
				}
				t, err := CoerceText(v)
				if err != "" {
					return err
				}
				for i, n := range nombres {
					if strings.EqualFold(n, t) {
						return float64(i + 1)
					}
				}
				return ErrNA
			}
		}
		for i, n := range nombres {
			if n == hoja {
				return float64(i + 1)
			}
		}
		return ErrNA
	},
	"SHEETS": func(c *FnCtx, args []*Arg) any {
		switch {
		case len(args) > 1:
			return ErrValue
		case len(args) == 1:
			if args[0].Ref() == nil {
				return ErrValue
			}
			return 1.0
		}
		return float64(len(c.SheetNames()))
	},
	"CELL": func(c *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		t0, err := textArg(args, 0)
		if err != "" {
			return err
		}
		var ref RefRange
		switch {
		case len(args) == 2 && args[1].Ref() == nil:
			return ErrValue
		case len(args) == 2:
			ref = *args[1].Ref()
		default:
			ref = RefRange{Sheet: c.Here.Sheet, C1: c.Here.Col, R1: c.Here.Row, C2: c.Here.Col, R2: c.Here.Row}
		}
		switch strings.ToLower(t0) {
		case "address":
			return "$" + NumToCol(ref.C1) + "$" + strconv.Itoa(ref.R1+1)
		case "col":
			return float64(ref.C1 + 1)
		case "row":
			return float64(ref.R1 + 1)
		case "filename":
			return ""
		case "contents", "type":
			var v Value = ""
			if m := c.Resolve(RefRange{Sheet: ref.Sheet, C1: ref.C1, R1: ref.R1, C2: ref.C1, R2: ref.R1}); len(m) > 0 && len(m[0]) > 0 {
				v = m[0][0]
			}
			if strings.EqualFold(t0, "contents") {
				return v
			}
			if t, ok := v.(string); ok {
				if t == "" {
					return "b"
				}
				return "l"
			}
			return "v"
		}
		return ErrNA
	},
}

func init() { registrar(funcionesCompat) }

// conLetra envuelve las letras del Tesoro: (liquidación, vencimiento, valor) con los días
// que quedan, que han de ser entre 1 y 365.
func conLetra(f func(dias, valor float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		a, err := finNums(args, 3)
		if err != "" {
			return err
		}
		dias := math.Floor(a[1]) - math.Floor(a[0])
		if dias <= 0 || dias > 365 {
			return ErrNum
		}
		return f(dias, a[2])
	}
}

// conAmort envuelve AMORLINC y AMORDEGRC: valida los argumentos y calcula la fracción de
// año del primer periodo.
func conAmort(f func(cost, salvage float64, per int, rate, yf float64) Value) fn {
	return conNums(6, []float64{0}, func(a []float64) Value {
		cost, bought, first, salvage, per, rate, basis := a[0], a[1], a[2], a[3], a[4], a[5], a[6]
		if cost <= 0 || rate <= 0 || salvage < 0 || salvage > cost || per < 0 || math.Trunc(basis) == 2 {
			return ErrNum
		}
		if math.Floor(first) < math.Floor(bought) {
			return ErrNum
		}
		yf, err := yearFrac(math.Floor(bought), math.Floor(first), math.Trunc(basis))
		if err != "" {
			return err
		}
		return f(cost, salvage, int(math.Trunc(per)), rate, yf)
	})
}

// anchoMedio pasa a ancho medio los caracteres de ancho completo (U+FF01–FF5E, U+3000).
func anchoMedio(t string) string {
	var b strings.Builder
	for _, ch := range t {
		switch {
		case ch >= 0xff01 && ch <= 0xff5e:
			b.WriteRune(ch - 0xfee0)
		case ch == 0x3000:
			b.WriteByte(' ')
		default:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// anchoCompleto es el camino contrario: ASCII imprimible → ancho completo.
func anchoCompleto(t string) string {
	var b strings.Builder
	for _, ch := range t {
		switch {
		case ch >= 0x21 && ch <= 0x7e:
			b.WriteRune(ch + 0xfee0)
		case ch == 0x20:
			b.WriteRune(0x3000)
		default:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

var (
	taiCifra = []string{"ศูนย์", "หนึ่ง", "สอง", "สาม", "สี่", "ห้า", "หก", "เจ็ด", "แปด", "เก้า"}
	taiLugar = []string{"", "สิบ", "ร้อย", "พัน", "หมื่น", "แสน"}
)

// grupoTailandes es un grupo de hasta seis cifras en tailandés; hayAntes dice que hay
// millones delante.
func grupoTailandes(g int, hayAntes bool) string {
	var out strings.Builder
	ds := fmt.Sprintf("%06d", g)
	for i := 0; i < 6; i++ {
		d := int(ds[i] - '0')
		lugar := 5 - i
		switch {
		case d == 0:
		case lugar == 1 && d == 1:
			out.WriteString("สิบ")
		case lugar == 1 && d == 2:
			out.WriteString("ยี่สิบ")
		case lugar == 1:
			out.WriteString(taiCifra[d] + "สิบ")
		case lugar == 0 && d == 1 && (g >= 10 || hayAntes):
			out.WriteString("เอ็ด")
		default:
			out.WriteString(taiCifra[d] + taiLugar[lugar])
		}
	}
	return out.String()
}

// enteroTailandes es un entero no negativo en tailandés: los millones se encadenan con «ล้าน».
func enteroTailandes(n int64) string {
	if n == 0 {
		return taiCifra[0]
	}
	millones, resto := n/1_000_000, int(n%1_000_000)
	out := ""
	if millones > 0 {
		out = enteroTailandes(millones) + "ล้าน"
	}
	if resto > 0 {
		out += grupoTailandes(resto, millones > 0)
	}
	return out
}

// bahtText es BAHTTEXT: «ลบ» si es negativo, los baht + «บาท», y los satang + «สตางค์» o «ถ้วน».
func bahtText(n float64) Value {
	cent := math.Floor(math.Abs(n)*100 + 0.5)
	if math.IsInf(cent, 0) || math.IsNaN(cent) || cent >= 1e15 {
		return ErrValue
	}
	total := int64(cent)
	baht, satang := total/100, int(total%100)
	out := ""
	if n < 0 && total > 0 {
		out = "ลบ"
	}
	if baht > 0 || satang == 0 {
		out += enteroTailandes(baht) + "บาท"
	}
	if satang == 0 {
		return out + "ถ้วน"
	}
	return out + grupoTailandes(satang, false) + "สตางค์"
}
