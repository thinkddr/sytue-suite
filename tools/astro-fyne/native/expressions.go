// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Scope is the data supplied to a generated component. It contains values, never
// JavaScript to evaluate. Actions are the explicit native boundary for side effects.
type Scope map[string]any
type Actions map[string]func(...any) any

// Undefined represents JavaScript undefined separately from null (Go nil).
type undefinedValue struct{}

var Undefined any = undefinedValue{}

func isUndefined(value any) bool { _, ok := value.(undefinedValue); return ok }

// Require prevents a generated page from displaying actions which do nothing.
func Require(actions Actions, names []string) error {
	for _, name := range names {
		if actions[name] == nil {
			return fmt.Errorf("webui: missing native action %q", name)
		}
	}
	return nil
}

// Get implements the property/index access accepted by the source compiler.
// Unsupported access is a contract error, rather than a silently missing value.
func Get(value any, key any) any {
	if value == nil || isUndefined(value) {
		panic("webui: cannot read a property of null or undefined")
	}
	switch v := value.(type) {
	case Scope:
		if result, ok := v[String(key)]; ok {
			return result
		}
		return Undefined
	case map[string]any:
		if result, ok := v[String(key)]; ok {
			return result
		}
		return Undefined
	}
	r := reflect.ValueOf(value)
	if r.Kind() == reflect.Pointer {
		if r.IsNil() {
			panic("webui: cannot read a property of a nil pointer")
		}
		r = r.Elem()
	}
	switch r.Kind() {
	case reflect.Array, reflect.Slice, reflect.String:
		if String(key) == "length" {
			if r.Kind() == reflect.String {
				return len(utf16.Encode([]rune(r.String())))
			}
			return r.Len()
		}
		n, canonical := index(key)
		if r.Kind() == reflect.String {
			units := utf16.Encode([]rune(r.String()))
			if !canonical || n >= len(units) {
				return Undefined
			}
			unit := units[n]
			if unit >= 0xd800 && unit <= 0xdfff {
				panic("webui: isolated UTF-16 surrogate indexing is unsupported; use a native Unicode adapter")
			}
			return string(rune(unit))
		}
		if !canonical || n >= r.Len() {
			return Undefined
		}
		return r.Index(n).Interface()
	case reflect.Struct:
		f := r.FieldByName(String(key))
		if f.IsValid() && f.CanInterface() {
			return f.Interface()
		}
		return Undefined
	case reflect.Map:
		k := reflect.ValueOf(key)
		if !k.IsValid() || !k.Type().AssignableTo(r.Type().Key()) {
			panic(fmt.Sprintf("webui: property %v does not match %s", key, r.Type().Key()))
		}
		if f := r.MapIndex(k); f.IsValid() {
			return f.Interface()
		}
		return Undefined
	default:
		panic(fmt.Sprintf("webui: cannot access property %v of %T", key, value))
	}
}

// JavaScript array properties are canonical decimal keys: "01", "1.0" and true
// do not become numeric indices just because Number can convert some of them.
func index(key any) (int, bool) {
	s := String(key)
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil || strconv.FormatUint(n, 10) != s || n >= (1<<32)-1 || n > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(n), true
}

// String implements scalar JavaScript string conversion. JSX omission belongs in
// ChildText, so false in a template remains "false" rather than disappearing.
func String(value any) string {
	if isUndefined(value) {
		return "undefined"
	}
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case fmt.Stringer:
		return v.String()
	}
	if numeric(value) {
		n := Number(value)
		if math.IsNaN(n) {
			return "NaN"
		}
		if math.IsInf(n, 1) {
			return "Infinity"
		}
		if math.IsInf(n, -1) {
			return "-Infinity"
		}
		if n == 0 {
			return "0"
		}
		if math.Abs(n) >= 1e21 || math.Abs(n) < 1e-6 {
			raw := strconv.FormatFloat(n, 'e', -1, 64)
			parts := strings.Split(raw, "e")
			exponent, _ := strconv.Atoi(parts[1])
			return parts[0] + "e" + fmt.Sprintf("%+d", exponent)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	r := reflect.ValueOf(value)
	if r.Kind() == reflect.Bool {
		return strconv.FormatBool(r.Bool())
	}
	if r.Kind() == reflect.Slice || r.Kind() == reflect.Array {
		items := make([]string, r.Len())
		for i := range items {
			v := r.Index(i).Interface()
			if v != nil && !isUndefined(v) {
				items[i] = String(v)
			}
		}
		return strings.Join(items, ",")
	}
	if r.Kind() == reflect.Map || r.Kind() == reflect.Struct {
		return "[object Object]"
	}
	panic(fmt.Sprintf("webui: unsupported string conversion of %T", value))
}

// ChildText is React/Preact's textual child projection: booleans, null and undefined
// paint nothing; arrays concatenate children; objects are invalid JSX children.
func ChildText(value any) string {
	if value == nil || isUndefined(value) {
		return ""
	}
	if _, ok := value.(bool); ok {
		return ""
	}
	r := reflect.ValueOf(value)
	if r.Kind() == reflect.Bool {
		return ""
	}
	if r.Kind() == reflect.Slice || r.Kind() == reflect.Array {
		var result strings.Builder
		for i := range r.Len() {
			result.WriteString(ChildText(r.Index(i).Interface()))
		}
		return result.String()
	}
	if r.Kind() == reflect.Map || r.Kind() == reflect.Struct {
		panic("webui: objects cannot be rendered as JSX text")
	}
	return String(value)
}

// Number implements numeric conversion for the deliberately small expression
// subset. Objects cannot participate in arithmetic without an explicit adapter.
func Number(value any) float64 {
	if isUndefined(value) {
		return math.NaN()
	}
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		v = strings.TrimFunc(v, numericWhitespace)
		if v == "" {
			return 0
		}
		if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") || strings.HasPrefix(v, "0b") || strings.HasPrefix(v, "0B") || strings.HasPrefix(v, "0o") || strings.HasPrefix(v, "0O") {
			base := 16
			switch v[1] {
			case 'b', 'B':
				base = 2
			case 'o', 'O':
				base = 8
			}
			if !radixDigits(v[2:], base) {
				return math.NaN()
			}
			// JavaScript Number has no uint64 parsing bound. Parse the entire
			// unsigned integer, then round once to IEEE-754 (including +Infinity).
			n, ok := new(big.Int).SetString(v[2:], base)
			if !ok {
				return math.NaN()
			}
			result, _ := n.Float64()
			return result
		}
		if v == "Infinity" || v == "+Infinity" {
			return math.Inf(1)
		}
		if v == "-Infinity" {
			return math.Inf(-1)
		}
		if !decimalNumberLiteral(v) {
			return math.NaN()
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return math.NaN()
		}
		return n
	}
	r := reflect.ValueOf(value)
	switch r.Kind() {
	case reflect.Bool:
		if r.Bool() {
			return 1
		}
		return 0
	case reflect.Float32, reflect.Float64:
		return r.Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(r.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(r.Uint())
	default:
		panic(fmt.Sprintf("webui: %T is not a numeric expression", value))
	}
}

// ECMAScript StringNumericLiteral uses WhiteSpace and LineTerminator, which differ
// from Go's Unicode TrimSpace (for example BOM is whitespace, NEL is not).
func numericWhitespace(r rune) bool {
	return r == '\t' || r == '\v' || r == '\f' || r == '\n' || r == '\r' ||
		r == '\ufeff' || r == '\u2028' || r == '\u2029' || unicode.Is(unicode.Zs, r)
}

func radixDigits(digits string, base int) bool {
	if digits == "" {
		return false
	}
	for _, digit := range digits {
		value := -1
		switch {
		case digit >= '0' && digit <= '9':
			value = int(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value = int(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value = int(digit-'A') + 10
		}
		if value < 0 || value >= base {
			return false
		}
	}
	return true
}

func decimalNumberLiteral(value string) bool {
	position := 0
	if value[0] == '+' || value[0] == '-' {
		position++
	}
	consumeDigits := func() int {
		start := position
		for position < len(value) && value[position] >= '0' && value[position] <= '9' {
			position++
		}
		return position - start
	}
	digits := consumeDigits()
	if position < len(value) && value[position] == '.' {
		position++
		digits += consumeDigits()
	}
	if digits == 0 {
		return false
	}
	if position < len(value) && (value[position] == 'e' || value[position] == 'E') {
		position++
		if position < len(value) && (value[position] == '+' || value[position] == '-') {
			position++
		}
		if consumeDigits() == 0 {
			return false
		}
	}
	return position == len(value)
}

// Truth is JavaScript truthiness for the supported scalar and collection values.
func Truth(value any) bool {
	if value == nil || isUndefined(value) {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	}
	r := reflect.ValueOf(value)
	switch r.Kind() {
	case reflect.Bool:
		return r.Bool()
	case reflect.Float32, reflect.Float64:
		return r.Float() != 0 && !math.IsNaN(r.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return r.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return r.Uint() != 0
	case reflect.Pointer, reflect.Interface:
		return !r.IsNil()
	default:
		return true // Empty arrays and objects are truthy in JavaScript.
	}
}

// Binary evaluates an operator already checked by the source compiler. Logical
// short circuiting is emitted as Go control flow, never evaluated here eagerly.
func Binary(op string, a, b any) any {
	switch op {
	case "+":
		if _, ok := a.(string); ok {
			return String(a) + String(b)
		}
		if _, ok := b.(string); ok {
			return String(a) + String(b)
		}
		return Number(a) + Number(b)
	case "-":
		return Number(a) - Number(b)
	case "*":
		return Number(a) * Number(b)
	case "/":
		return Number(a) / Number(b)
	case "%":
		return math.Mod(Number(a), Number(b))
	case "===", "!==":
		equal := scalarEqual(a, b)
		if op == "!==" {
			return !equal
		}
		return equal
	case "<", "<=", ">", ">=":
		if sa, ok := a.(string); ok {
			if sb, isString := b.(string); isString {
				order := compareUTF16(sa, sb)
				switch op {
				case "<":
					return order < 0
				case "<=":
					return order <= 0
				case ">":
					return order > 0
				default:
					return order >= 0
				}
			}
		}
		x, y := Number(a), Number(b)
		switch op {
		case "<":
			return x < y
		case "<=":
			return x <= y
		case ">":
			return x > y
		default:
			return x >= y
		}
	default:
		panic(fmt.Sprintf("webui: unsupported binary operator %q", op))
	}
}

// Relational string operators compare UTF-16 code units, not UTF-8 bytes or
// Unicode code points. Supplementary characters can sort before BMP characters.
func compareUTF16(a, b string) int {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := range min(len(x), len(y)) {
		if x[i] < y[i] {
			return -1
		}
		if x[i] > y[i] {
			return 1
		}
	}
	if len(x) < len(y) {
		return -1
	}
	if len(x) > len(y) {
		return 1
	}
	return 0
}

func scalarEqual(a, b any) bool {
	if isUndefined(a) || isUndefined(b) {
		return isUndefined(a) && isUndefined(b)
	}
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if numeric(a) && numeric(b) {
		return Number(a) == Number(b)
	}
	switch v := a.(type) {
	case string:
		w, ok := b.(string)
		return ok && v == w
	case bool:
		w, ok := b.(bool)
		return ok && v == w
	default:
		panic("webui: object equality requires a native adapter")
	}
}

func numeric(v any) bool {
	if v == nil {
		return false
	}
	k := reflect.TypeOf(v).Kind()
	return k >= reflect.Int && k <= reflect.Float64
}

// Unary evaluates the three unary operators supported in generated expressions.
func Unary(op string, value any) any {
	switch op {
	case "!":
		return !Truth(value)
	case "+":
		return Number(value)
	case "-":
		return -Number(value)
	default:
		panic(fmt.Sprintf("webui: unsupported unary operator %q", op))
	}
}

// Values supplies an array to a generated JSX map without losing element types.
func Values(value any) []any {
	if value == nil || isUndefined(value) {
		panic("webui: cannot map null or undefined")
	}
	r := reflect.ValueOf(value)
	if r.Kind() != reflect.Array && r.Kind() != reflect.Slice {
		panic(fmt.Sprintf("webui: map expects an array, received %T", value))
	}
	out := make([]any, r.Len())
	for i := range out {
		out[i] = r.Index(i).Interface()
	}
	return out
}
