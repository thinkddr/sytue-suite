// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package altcha protege lo anónimo contra bots con ALTCHA (prueba de trabajo).
//
// Estándar abierto, compatible con el RGPD y sin seguimiento de usuarios: no
// requiere cookies, no envía datos a servidores de terceros y se resuelve
// en el propio navegador mediante Web Crypto (SHA-256).
//
// Vivió en Sytue ID (`services/id/internal/captcha`) hasta que Sytue Web lo
// necesitó para el carrito y los formularios de las webs de sus clientes. Dos
// cosas cambian con dónde se use, y por eso son enchufables:
//
//   - las repeticiones: ID recuerda las firmas usadas en memoria (una réplica);
//     Pages las guarda en su base con ConRepeticiones, que vale con cualquier
//     número de réplicas;
//   - el ámbito: un reto de ámbito (CrearReto) lleva dentro de la sal, firmada
//     con él, para qué se emitió y con qué dificultad, y Comprobar exige los
//     dos. Así el reto barato de una web no sirve en otra que ahora pide más.
package altcha

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

var (
	ErrInvalidPayload   = errors.New("altcha: payload inválido")
	ErrUnsupportedAlgo  = errors.New("altcha: algoritmo no soportado")
	ErrExpired          = errors.New("altcha: reto expirado")
	ErrInvalidSignature = errors.New("altcha: firma inválida")
	ErrInvalidSolution  = errors.New("altcha: solución matemática incorrecta")
	ErrNumberOutOfRange = errors.New("altcha: número fuera de rango")
	ErrReplayDetected   = errors.New("altcha: reto ya utilizado")
	ErrWrongScope       = errors.New("altcha: reto emitido para otro ámbito")
	ErrTooEasy          = errors.New("altcha: reto más fácil de lo exigido")
)

// Challenge representa el reto entregado al cliente para resolver.
type Challenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int64  `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// Payload representa la solución enviada por el cliente.
type Payload struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int64  `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

// Repeticiones apunta que una firma se ha usado y dice si es la primera vez. Recibe
// cuándo caduca el reto: pasada esa hora ya no hace falta recordarla, porque
// Verify rechaza el reto por caducado antes de preguntar. Un error cierra la puerta
// (el reto no se acepta).
type Repeticiones func(ctx context.Context, firma string, caduca time.Time) (nueva bool, err error)

// EnTabla es el almacén de repeticiones en una tabla de PostgreSQL con las
// columnas `signature text PRIMARY KEY` y `expires_at timestamptz`, compartida
// entre réplicas: INSERT … ON CONFLICT DO NOTHING, y de paso un barrido de las
// caducadas hace más de una hora (Verify ya las rechaza antes de preguntar). Es lo
// que Bases, Sytue ID y Web escribieron cada uno por su lado; Cast lo importa.
func EnTabla(db pg.Querier, tabla string) Repeticiones {
	t := pgx.Identifier{tabla}.Sanitize()
	return func(ctx context.Context, firma string, caduca time.Time) (bool, error) {
		_, _ = db.Exec(ctx, `DELETE FROM `+t+` WHERE expires_at < now() - interval '1 hour'`)
		tag, err := db.Exec(ctx, `INSERT INTO `+t+` (signature, expires_at) VALUES ($1, $2)
			ON CONFLICT (signature) DO NOTHING`, firma, caduca)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	}
}

// Opcion ajusta un Verifier al crearlo.
type Opcion func(*Verifier)

// ConRepeticiones sustituye la memoria del proceso por otro almacén de firmas
// usadas. Con más de una réplica es obligatorio: en memoria, cada réplica
// aceptaría una vez la misma solución.
func ConRepeticiones(fn Repeticiones) Opcion {
	return func(v *Verifier) { v.repeticiones = fn }
}

// Verifier gestiona la generación y verificación de retos ALTCHA.
type Verifier struct {
	key          []byte
	repeticiones Repeticiones
	mu           sync.Mutex
	replays      map[string]time.Time // signature -> expiresAt
}

// New crea un nuevo Verifier con la clave HMAC dada. Sin opciones, recuerda las
// firmas usadas en la memoria del proceso (lo que sirve a una sola réplica).
func New(key []byte, opciones ...Opcion) *Verifier {
	v := &Verifier{
		key:     key,
		replays: make(map[string]time.Time),
	}
	v.repeticiones = v.enMemoria
	for _, o := range opciones {
		o(v)
	}
	return v
}

// CreateChallenge genera un nuevo reto criptográfico sin ámbito. La dificultad
// (maxNumber) la decide cada llamada.
func (v *Verifier) CreateChallenge(maxNumber int64, ttl time.Duration) (Challenge, error) {
	return v.crear("", maxNumber, ttl)
}

// CrearReto genera un reto atado a un ámbito (p. ej. «<sitio>:checkout»): el ámbito
// y la dificultad viajan en la sal, que el reto firma, así que el cliente no puede
// cambiarlos sin invalidarlo. Se comprueba con Comprobar.
func (v *Verifier) CrearReto(ambito string, maxNumber int64, ttl time.Duration) (Challenge, error) {
	if ambito == "" {
		return Challenge{}, errors.New("altcha: reto de ámbito sin ámbito")
	}
	return v.crear(ambito, maxNumber, ttl)
}

func (v *Verifier) crear(ambito string, maxNumber int64, ttl time.Duration) (Challenge, error) {
	if maxNumber <= 0 {
		maxNumber = 50000
	}
	if ttl == 0 {
		ttl = 10 * time.Minute
	}

	// 12 bytes aleatorios como base de la sal (24 caracteres hex)
	saltBytes := make([]byte, 12)
	if _, err := rand.Read(saltBytes); err != nil {
		return Challenge{}, err
	}
	saltHex := hex.EncodeToString(saltBytes)

	// Caducidad (y, con ámbito, el ámbito y la dificultad) en query string dentro
	// de la sal: el reto es sha256(sal + número), así que quedan firmados con él.
	expiresAt := time.Now().Add(ttl).Unix()
	salt := fmt.Sprintf("%s?expires=%d&", saltHex, expiresAt)
	if ambito != "" {
		salt += "ambito=" + url.QueryEscape(ambito) + "&max=" + strconv.FormatInt(maxNumber, 10) + "&"
	}

	// Número aleatorio secreto en [0, maxNumber]
	nBig, err := rand.Int(rand.Reader, big.NewInt(maxNumber+1))
	if err != nil {
		return Challenge{}, err
	}
	secretNumber := nBig.Int64()

	// Reto computado como sha256(salt + número)
	h := sha256.Sum256([]byte(salt + strconv.FormatInt(secretNumber, 10)))
	challenge := hex.EncodeToString(h[:])

	// Firma HMAC-SHA256 del reto usando la clave secreta
	mac := hmac.New(sha256.New, v.key)
	mac.Write([]byte(challenge))
	signature := hex.EncodeToString(mac.Sum(nil))

	return Challenge{
		Algorithm: "SHA-256",
		Challenge: challenge,
		MaxNumber: maxNumber,
		Salt:      salt,
		Signature: signature,
	}, nil
}

// Verify comprueba la validez de una solución ALTCHA sin ámbito, en base64. Un
// reto de ámbito no vale aquí: se emitió para otra cosa. El contexto es el de la
// petición: lo usa el almacén de repeticiones.
func (v *Verifier) Verify(ctx context.Context, rawPayload string) (bool, error) {
	if err := v.comprobar(ctx, "", 0, rawPayload); err != nil {
		return false, err
	}
	return true, nil
}

// Comprobar verifica una solución de un reto de CrearReto: además de la firma, la
// solución, la caducidad y la repetición, que el ámbito sea exactamente `ambito` y
// que el reto se emitiera con una dificultad de al menos `minimo`.
func (v *Verifier) Comprobar(ctx context.Context, ambito string, minimo int64, rawPayload string) error {
	if ambito == "" {
		return ErrWrongScope
	}
	return v.comprobar(ctx, ambito, minimo, rawPayload)
}

func (v *Verifier) comprobar(ctx context.Context, ambito string, minimo int64, rawPayload string) error {
	if strings.TrimSpace(rawPayload) == "" {
		return ErrInvalidPayload
	}

	rawBytes, err := base64.StdEncoding.DecodeString(rawPayload)
	if err != nil {
		return ErrInvalidPayload
	}

	var p Payload
	if err := json.Unmarshal(rawBytes, &p); err != nil {
		return ErrInvalidPayload
	}

	if p.Algorithm != "SHA-256" {
		return ErrUnsupportedAlgo
	}

	if p.Number < 0 {
		return ErrNumberOutOfRange
	}

	// Parámetros de la sal: caducidad y, en un reto de ámbito, ámbito y dificultad.
	params, err := parametrosDeSal(p.Salt)
	if err != nil {
		return ErrExpired
	}
	expires, err := strconv.ParseInt(params.Get("expires"), 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return ErrExpired
	}
	if params.Get("ambito") != ambito {
		return ErrWrongScope
	}
	if ambito != "" {
		maximo, err := strconv.ParseInt(params.Get("max"), 10, 64)
		if err != nil || maximo < minimo {
			return ErrTooEasy
		}
	}

	// Comprobar firma HMAC del challenge
	mac := hmac.New(sha256.New, v.key)
	mac.Write([]byte(p.Challenge))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !constantTimeCompare(p.Signature, expectedSig) {
		return ErrInvalidSignature
	}

	// Comprobar solución matemática: sha256(salt + number) == challenge
	h := sha256.Sum256([]byte(p.Salt + strconv.FormatInt(p.Number, 10)))
	computedChallenge := hex.EncodeToString(h[:])

	if !constantTimeCompare(p.Challenge, computedChallenge) {
		return ErrInvalidSolution
	}

	// Prevención de reuso de reto (replay protection), lo último: solo se apunta
	// una firma que ya ha pasado todo lo demás.
	nueva, err := v.repeticiones(ctx, p.Signature, time.Unix(expires, 0))
	if err != nil {
		return fmt.Errorf("altcha: comprobar la repetición: %w", err)
	}
	if !nueva {
		return ErrReplayDetected
	}
	return nil
}

// parametrosDeSal lee la query string que la sal lleva tras el «?».
func parametrosDeSal(salt string) (url.Values, error) {
	idx := strings.IndexByte(salt, '?')
	if idx < 0 {
		return nil, errors.New("salt sin parámetros de expiración")
	}
	return url.ParseQuery(salt[idx+1:])
}

func constantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// enMemoria es el almacén de repeticiones por defecto. ponytail: una réplica; con
// varias, ConRepeticiones sobre la base.
func (v *Verifier) enMemoria(_ context.Context, signature string, expiresAt time.Time) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	// Limpieza de firmas caducadas si la caché supera 1000 elementos
	if len(v.replays) > 1000 {
		for sig, exp := range v.replays {
			if now.After(exp) {
				delete(v.replays, sig)
			}
		}
	}

	if _, exists := v.replays[signature]; exists {
		return false, nil
	}

	v.replays[signature] = expiresAt
	return true, nil
}

// SolveChallenge es un helper que resuelve un reto (útil para tests).
func SolveChallenge(ch Challenge) (string, error) {
	for i := int64(0); i <= ch.MaxNumber; i++ {
		h := sha256.Sum256([]byte(ch.Salt + strconv.FormatInt(i, 10)))
		if hex.EncodeToString(h[:]) == ch.Challenge {
			p := Payload{
				Algorithm: ch.Algorithm,
				Challenge: ch.Challenge,
				Number:    i,
				Salt:      ch.Salt,
				Signature: ch.Signature,
			}
			b, err := json.Marshal(p)
			if err != nil {
				return "", err
			}
			return base64.StdEncoding.EncodeToString(b), nil
		}
	}
	return "", errors.New("no solution found")
}

// Window de validez del bypass de pruebas: el mensaje firmado lleva timestamp
// Unix y se rechaza fuera de esta ventana (relojes del runner y del servidor).
const TestBypassWindow = 5 * time.Minute

// VerifyTestBypass valida un mensaje de bypass de pruebas: HMAC-SHA256 de
// "<timestamp>|<email>" con la clave dedicada de pruebas. Es la salida que
// usan los barridos de la forja (mobile/escritorio) cuando el captcha está
// encendido: la clave NUNCA llega a un navegador normal — solo está en el
// secreto del despliegue y en los secretos del CI —, así que quien no la
// tenga sigue obligado a resolver el PoW como todo el mundo. Mensaje viejo
// (fuera de ventana) o firma que no case: false, sin distinguir cuál.
func VerifyTestBypass(key []byte, mensaje, email string, now time.Time) bool {
	if len(key) == 0 || mensaje == "" {
		return false
	}
	ts, firma, ok := strings.Cut(mensaje, "|")
	if !ok {
		return false
	}
	stamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || stamp < 0 {
		return false
	}
	t := time.Unix(stamp, 0)
	if t.Before(now.Add(-TestBypassWindow)) || t.After(now.Add(TestBypassWindow)) {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ts + "|" + email))
	return subtle.ConstantTimeCompare([]byte(firma), []byte(hex.EncodeToString(mac.Sum(nil)))) == 1
}

// SignTestBypass compone el mensaje que VerifyTestBypass acepta. Vive aquí
// para que el que firma y el que verifica no puedan divergir.
func SignTestBypass(key []byte, email string, now time.Time) string {
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ts + "|" + email))
	return ts + "|" + hex.EncodeToString(mac.Sum(nil))
}
