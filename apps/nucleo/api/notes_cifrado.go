// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

// EL CIFRADO DE LOS CUADERNOS DE NOTEPADS, el mismo que el de la web
// (apps/frontend/apps/notes/src/client/crypto.ts), para que un cuaderno cifrado en el
// navegador se abra aquí y al revés:
//
//	derivación  PBKDF2-SHA256, 600 000 iteraciones, sal de 16 bytes
//	cifrado     AES-256-GCM
//	formato     "encv1:" + base64(iv(12 B) || criptograma || etiqueta)
//
// La contraseña y la clave NO salen del cliente: el servicio guarda el criptograma y los
// parámetros de derivación, que no son secretos. Aquí no hay «recuperar contraseña», y
// es verdad: sin ella las notas no se pueden leer, ni siquiera desde dentro.
//
// Todo es la biblioteca estándar (`crypto/pbkdf2` desde Go 1.24): nada de terceros en la
// ruta del cifrado, por lo mismo que la web usa WebCrypto.

// NotesCrypto son los parámetros de derivación que viajan con el cuaderno.
type NotesCrypto struct {
	V          int    `json:"v"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Iterations int    `json:"iterations"`
	// Check es el criptograma de verificación: valida la contraseña sin descifrar
	// ninguna nota.
	Check string `json:"check"`
}

// Los números del esquema v1, los de crypto.ts.
const (
	IteracionesNotas = 600_000
	prefijoCifrado   = "encv1:"
	tamIVNotas       = 12
	// testigoNotas es el texto del criptograma de verificación. Su valor no es el secreto;
	// lo es que solo la clave buena lo produzca.
	testigoNotas = "sytue-notes-v1"
)

// ErrCifrado es un criptograma que no abre con esa clave (o que no es de este formato).
var ErrCifrado = errors.New("api: el criptograma no abre con esta clave")

// CryptoDe lee los parámetros de un cuaderno. `false` si no los trae o no se entienden.
func CryptoDe(crudo json.RawMessage) (NotesCrypto, bool) {
	var c NotesCrypto
	if len(crudo) == 0 || json.Unmarshal(crudo, &c) != nil || c.Salt == "" || c.Iterations <= 0 {
		return NotesCrypto{}, false
	}
	return c, true
}

func derivarNotas(pass string, sal []byte, iteraciones int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, pass, sal, iteraciones, 32)
}

// CifrarTextoNotas cifra un texto con la clave. Devuelve "encv1:<base64>".
func CifrarTextoNotas(clave []byte, texto string) (string, error) {
	aead, err := aeadNotas(clave)
	if err != nil {
		return "", err
	}
	iv := make([]byte, tamIVNotas)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	junto := aead.Seal(iv, iv, []byte(texto), nil)
	return prefijoCifrado + base64.StdEncoding.EncodeToString(junto), nil
}

// DescifrarTextoNotas deshace `CifrarTextoNotas`. GCM autentica: con otra clave, o con un
// byte cambiado, falla.
func DescifrarTextoNotas(clave []byte, criptograma string) (string, error) {
	if !strings.HasPrefix(criptograma, prefijoCifrado) {
		return "", ErrCifrado
	}
	junto, err := base64.StdEncoding.DecodeString(criptograma[len(prefijoCifrado):])
	if err != nil || len(junto) < tamIVNotas {
		return "", ErrCifrado
	}
	aead, err := aeadNotas(clave)
	if err != nil {
		return "", err
	}
	claro, err := aead.Open(nil, junto[:tamIVNotas], junto[tamIVNotas:], nil)
	if err != nil {
		return "", ErrCifrado
	}
	return string(claro), nil
}

func aeadNotas(clave []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(clave)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// NuevoCifradoNotas crea los parámetros de un cuaderno que se cifra por primera vez y
// devuelve también la clave, para no derivarla dos veces (600 000 iteraciones se notan).
func NuevoCifradoNotas(pass string) (NotesCrypto, []byte, error) {
	sal := make([]byte, 16)
	if _, err := rand.Read(sal); err != nil {
		return NotesCrypto{}, nil, err
	}
	clave, err := derivarNotas(pass, sal, IteracionesNotas)
	if err != nil {
		return NotesCrypto{}, nil, err
	}
	check, err := CifrarTextoNotas(clave, testigoNotas)
	if err != nil {
		return NotesCrypto{}, nil, err
	}
	return NotesCrypto{
		V: 1, KDF: "PBKDF2-SHA256", Salt: base64.StdEncoding.EncodeToString(sal),
		Iterations: IteracionesNotas, Check: check,
	}, clave, nil
}

// AbrirCifradoNotas deriva la clave y la comprueba contra el testigo. `ErrCifrado` si la
// contraseña no es la buena: así el aviso sale sin haber intentado abrir ninguna nota.
func AbrirCifradoNotas(c NotesCrypto, pass string) ([]byte, error) {
	sal, err := base64.StdEncoding.DecodeString(c.Salt)
	if err != nil {
		return nil, ErrCifrado
	}
	clave, err := derivarNotas(pass, sal, c.Iterations)
	if err != nil {
		return nil, err
	}
	if err := ComprobarClaveNotas(c, clave); err != nil {
		return nil, err
	}
	return clave, nil
}

// ComprobarClaveNotas dice si `clave` es la del cuaderno, contra su testigo, sin derivar
// nada: es para una clave ya derivada que se guardó (Notepads la recuerda con Touch ID
// si su dueño lo pide). `ErrCifrado` si no lo es, como cuando se cambia la contraseña.
func ComprobarClaveNotas(c NotesCrypto, clave []byte) error {
	t, err := DescifrarTextoNotas(clave, c.Check)
	if err != nil || subtle.ConstantTimeCompare([]byte(t), []byte(testigoNotas)) != 1 {
		return ErrCifrado
	}
	return nil
}

// NotaEnClaro es lo que va dentro del criptograma de una nota: título y cuerpo JUNTOS,
// porque el título suele ser lo más revelador que tiene una nota.
type NotaEnClaro struct {
	Title   string          `json:"title"`
	Content string          `json:"content"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// EmpaquetarNota cifra el contenido de una nota en UN criptograma.
func EmpaquetarNota(clave []byte, n NotaEnClaro) (string, error) {
	// El JSON auxiliar se guarda como cadena dentro del sobre para que JavaScript
	// pueda abrir/editar la nota sin redondear números que no sabe representar.
	stored := struct {
		Title    string `json:"title"`
		Content  string `json:"content"`
		DataJSON string `json:"data_json,omitempty"`
	}{Title: n.Title, Content: n.Content}
	if len(n.Data) > 0 {
		if !json.Valid(n.Data) {
			return "", ErrDatosCifradosNotas
		}
		stored.DataJSON = string(n.Data)
	}
	var crudo bytes.Buffer
	e := json.NewEncoder(&crudo)
	e.SetEscapeHTML(false)
	if err := e.Encode(stored); err != nil {
		return "", err
	}

	cifrado, err := CifrarTextoNotas(clave, strings.TrimSuffix(crudo.String(), "\n"))
	if len(cifrado) > MaxCipherNotas {
		return "", ErrDatosCifradosNotas
	}
	return cifrado, err
}

// DesempaquetarNota deshace `EmpaquetarNota`.
func DesempaquetarNota(clave []byte, criptograma string) (NotaEnClaro, error) {
	t, err := DescifrarTextoNotas(clave, criptograma)
	if err != nil {
		return NotaEnClaro{}, err
	}
	var stored struct {
		Title    string                     `json:"title"`
		Content  string                     `json:"content"`
		Data     json.RawMessage            `json:"data"`
		DataJSON string                     `json:"data_json"`
		Patch    map[string]json.RawMessage `json:"data_patch"`
		Removed  []string                   `json:"data_removed"`
	}
	if json.Unmarshal([]byte(t), &stored) != nil {
		return NotaEnClaro{}, ErrCifrado
	}
	n := NotaEnClaro{Title: stored.Title, Content: stored.Content, Data: stored.Data}
	if stored.DataJSON != "" {
		n.Data = json.RawMessage(stored.DataJSON)
		if !json.Valid(n.Data) {
			return NotaEnClaro{}, ErrCifrado
		}
	}
	if len(stored.Patch) > 0 || len(stored.Removed) > 0 {
		var data map[string]json.RawMessage
		if len(n.Data) > 0 && json.Unmarshal(n.Data, &data) != nil {
			return NotaEnClaro{}, ErrCifrado
		}
		if data == nil {
			data = map[string]json.RawMessage{}
		}
		for k, v := range stored.Patch {
			data[k] = v
		}
		for _, k := range stored.Removed {
			delete(data, k)
		}
		n.Data, err = CodificarDatosNotas(data)
		if err != nil {
			return NotaEnClaro{}, ErrCifrado
		}
	}

	return n, nil
}

// MaxCipherNotas coincide con el límite de escritura del servicio, tras base64.
const MaxCipherNotas = 16 << 20

// ErrDatosCifradosNotas impide descartar datos históricos incompatibles o demasiado grandes.
var ErrDatosCifradosNotas = errors.New("api: los datos de la nota requieren revisión antes de cifrarlos")

func controlNota(k string) bool {
	return k == "client_id" || k == "repeated_into" || k == "carried_from" || k == "carry_count"
}

// DatosPrivadosNotas conserva todos los campos auxiliares salvo los cuatro controles
// opacos que el servicio necesita para idempotencia y recurrencia.
func DatosPrivadosNotas(data json.RawMessage) (json.RawMessage, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		if !json.Valid(data) {
			return nil, ErrDatosCifradosNotas
		}
		return bytes.Clone(data), nil // legado escalar/array: se conserva dentro del sobre
	}
	for k := range obj {
		if controlNota(k) {
			delete(obj, k)
		}
	}
	if len(obj) == 0 {
		return nil, nil
	}
	return CodificarDatosNotas(obj)
}

// DatosPublicosNotas solo reenvía el identificador opaco del cliente. Los otros
// controles son autoridad del servidor y nunca se copian desde una escritura.
func DatosPublicosNotas(data json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil || len(obj["client_id"]) == 0 {
		return json.RawMessage(`{}`)
	}
	out, _ := CodificarDatosNotas(map[string]json.RawMessage{"client_id": obj["client_id"]})
	return out
}

func unirDatosNota(a, b json.RawMessage) (json.RawMessage, error) {
	if len(a) == 0 || string(a) == "null" {
		return bytes.Clone(b), nil
	}
	if len(b) == 0 || string(b) == "null" {
		return bytes.Clone(a), nil
	}
	var av, bv any
	if decodificarDatosNota(a, &av) != nil || decodificarDatosNota(b, &bv) != nil {
		return nil, ErrDatosCifradosNotas
	}
	if reflect.DeepEqual(av, bv) {
		return bytes.Clone(a), nil
	}
	am, aok := av.(map[string]any)
	bm, bok := bv.(map[string]any)
	if !aok || !bok {
		return nil, ErrDatosCifradosNotas
	}
	for k, v := range bm {
		if old, ok := am[k]; ok && !reflect.DeepEqual(old, v) {
			return nil, ErrDatosCifradosNotas
		}
		am[k] = v
	}
	return CodificarDatosNotas(am)
}

// AbrirContenidoNota restaura Data del sobre y del legado, sin elegir silenciosamente
// una de dos versiones distintas. migrar solo es cierto si queda contenido fuera.
func AbrirContenidoNota(clave []byte, n Note) (claro NotaEnClaro, migrar bool, err error) {
	legado, err := DatosPrivadosNotas(n.Data)
	if err != nil {
		return claro, false, err
	}
	if n.ContentCipher == "" {
		return NotaEnClaro{Title: n.Title, Content: n.Content, Data: n.Data}, true, nil
	}
	claro, err = DesempaquetarNota(clave, n.ContentCipher)
	if err != nil {
		return claro, false, err
	}
	privado, err := DatosPrivadosNotas(claro.Data)
	if err != nil {
		return claro, false, err
	}
	claro.Data, err = unirDatosNota(privado, legado)
	if err != nil {
		return claro, false, err
	}
	// Los controles actuales prevalecen sobre cualquier copia histórica en el sobre.
	var actual map[string]json.RawMessage
	if json.Unmarshal(n.Data, &actual) == nil {
		controles := map[string]json.RawMessage{}
		for k, v := range actual {
			if controlNota(k) {
				controles[k] = v
			}
		}
		if len(controles) > 0 {
			raw, _ := CodificarDatosNotas(controles)
			claro.Data, err = unirDatosNota(claro.Data, raw)
		}
	}
	return claro, len(legado) > 0 || len(n.Data) == 0 || string(n.Data) == "null" || n.Title != "" || n.Content != "", err
}

// CifrarContenidoNota prepara la escritura v2 completa antes de tocar la entrada.
func CifrarContenidoNota(clave []byte, in *NoteInput) error {
	privado, err := DatosPrivadosNotas(in.Data)
	if err != nil {
		return err
	}
	cifrado, err := EmpaquetarNota(clave, NotaEnClaro{Title: in.Title, Content: in.Content, Data: privado})
	if err != nil {
		return err
	}
	in.Title, in.Content, in.ContentCipher = "", "", cifrado
	in.Data = DatosPublicosNotas(in.Data)
	in.CipherVersion = 2
	return nil
}

// CodificarDatosNotas serializa el JSON auxiliar sin expandir HTML dentro de los
// RawMessage: sirve también a las ediciones parciales de los clientes nativos.
func CodificarDatosNotas(v any) (json.RawMessage, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}

func decodificarDatosNota(raw json.RawMessage, dst *any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(dst)
}
