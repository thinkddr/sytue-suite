// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sello de tiempo de un tercero (RFC 3161) para la firma de un PDF.
//
// QUÉ AÑADE, Y POR QUÉ NO ES UN ADORNO. Una firma sin sello prueba QUIÉN firmó,
// no CUÁNDO. La fecha que lleva dentro la pone el propio firmante, así que no vale
// como prueba: se puede escribir cualquiera. La consecuencia práctica llega
// después — cuando el certificado caduca o se revoca, un validador ya no puede
// decidir si estaba vigente al firmar, y la firma pasa de válida a
// «indeterminada». Con sello, la fecha la atestigua un tercero y la firma sigue
// verificándose años más tarde. Es la diferencia entre firmar un contrato y poder
// demostrarlo en 2031.
//
// CÓMO FUNCIONA. Se manda el RESUMEN de la firma —no el documento, ni la firma
// misma— a una autoridad de sellado, que devuelve un token: otro CMS firmado por
// ella que dice «este resumen existía a esta hora». El token se cuelga de la firma
// como atributo NO firmado, que es el único sitio donde cabe: firmarlo sería
// imposible porque no existe hasta que la firma está hecha.
//
// LO QUE ESTO NO RESUELVE SOLO: el nivel LTV completo pide además la información
// de revocación incrustada (OCSP o CRL) en un diccionario /DSS. Con sello y sin
// revocación la firma ya sobrevive al vencimiento del certificado, que es el 90 %
// del valor; el resto es otra vuelta y está anotado como tal.
//
// QUÉ AUTORIDAD. Un sello solo vale lo que vale quien lo emite, así que para eIDAS
// tiene que ser un prestador CUALIFICADO de la lista de confianza de la UE. Desde
// el ADR 0051 es Izenpe (sector público vasco, servicio cualificado y abierto),
// medido contra la propia lista: el Certum gratuito de antes NO es cualificado. La
// URL y el ancla de su cadena llegan por configuración; sin URL las firmas salen
// sin sello y la interfaz lo dice, en vez de dejarlo en la letra pequeña.
package domain

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cms"
)

var (
	// ErrSelloNoConfigurado indica que no hay autoridad de sellado. No es un error
	// del usuario: es que el despliegue no la tiene puesta.
	ErrSelloNoConfigurado = errors.New("pdf: no hay autoridad de sellado configurada")
	// ErrSelloRechazado es la autoridad diciendo que no.
	ErrSelloRechazado = errors.New("pdf: la autoridad de sellado rechazó la petición")
)

// OID del atributo que lleva el token y del tipo de contenido de un sello.
var (
	oidAtributoSelloFirma = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
	oidTokenSello         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4}
)

// maxRespuestaSello acota lo que se lee de la autoridad. Un token son unos pocos
// kilobytes; un megabyte ya es un servidor que se ha vuelto loco o un intento de
// llenar la memoria.
const maxRespuestaSello = 1 << 20

// peticionDeSello es TimeStampReq (RFC 3161 §2.4.1).
type peticionDeSello struct {
	Version          int
	HuellaMensaje    huellaDeMensaje
	Politica         asn1.ObjectIdentifier `asn1:"optional"`
	Nonce            *big.Int              `asn1:"optional"`
	PedirCertificado bool                  `asn1:"optional,default:false"`
}

type huellaDeMensaje struct {
	Algoritmo pkix.AlgorithmIdentifier
	Huella    []byte
}

// respuestaDeSello es TimeStampResp. El token viene como `RawValue` porque se
// reemite TAL CUAL: es un CMS firmado por la autoridad y volver a codificarlo
// cambiaría sus bytes, con lo que su propia firma dejaría de verificar.
type respuestaDeSello struct {
	Estado estadoPKI
	Token  asn1.RawValue `asn1:"optional"`
}

type estadoPKI struct {
	Estado  int
	Texto   []asn1.RawValue `asn1:"optional"`
	Detalle asn1.BitString  `asn1:"optional"`
}

// OpcionesSello configura la llamada a la autoridad.
type OpcionesSello struct {
	// URL de la autoridad de sellado. Vacía = sin sello.
	URL string
	// Politica, si la autoridad exige una concreta.
	Politica string
	// Timeout de la llamada. Una autoridad lenta no puede colgar una firma.
	Timeout time.Duration
	// Usuario y Clave para las autoridades que piden autenticación básica.
	Usuario, Clave string
	// Cliente permite inyectar el transporte; nil usa uno propio con su plazo.
	Cliente *http.Client
	// Anclas son las raíces contra las que se AUTENTICA el token: su firma, su
	// cadena y el uso timeStamping. Sin ellas solo se comprueba que el token sella
	// lo pedido, y eso no basta sobre HTTP plano: un intermediario puede FABRICAR
	// un token con la huella y el nonce buenos firmado por cualquiera, y el PDF
	// saldría con un «sello» que el primer validador de verdad rechaza.
	Anclas *x509.CertPool
}

// PedirSello manda el resumen a la autoridad y devuelve el token tal cual llega.
func PedirSello(ctx context.Context, resumen []byte, opts OpcionesSello) ([]byte, error) {
	if opts.URL == "" {
		return nil, ErrSelloNoConfigurado
	}
	if len(resumen) != sha256.Size {
		return nil, fmt.Errorf("pdf: el resumen para sellar debe ser SHA-256")
	}

	pet := peticionDeSello{
		Version: 1,
		HuellaMensaje: huellaDeMensaje{
			Algoritmo: pkix.AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.NullRawValue},
			Huella:    resumen,
		},
		// El certificado de la autoridad se PIDE: sin él, el token no se puede
		// verificar sin ir a buscarlo aparte, y el objetivo del sello es justo que
		// la firma se sostenga sola dentro del fichero.
		PedirCertificado: true,
	}
	if opts.Politica != "" {
		if oid, err := oidDesdeTexto(opts.Politica); err == nil {
			pet.Politica = oid
		}
	}
	// El nonce ata la respuesta a ESTA petición: sin él, un intermediario podría
	// devolver un token viejo y válido de otra firma.
	nonce, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		return nil, err
	}
	pet.Nonce = nonce

	cuerpo, err := asn1.Marshal(pet)
	if err != nil {
		return nil, fmt.Errorf("pdf: no se pudo codificar la petición de sello: %w", err)
	}

	plazo := opts.Timeout
	if plazo <= 0 {
		plazo = 20 * time.Second
	}
	ctx, cancelar := context.WithTimeout(ctx, plazo)
	defer cancelar()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.URL, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/timestamp-query")
	if opts.Usuario != "" {
		req.SetBasicAuth(opts.Usuario, opts.Clave)
	}

	cliente := opts.Cliente
	if cliente == nil {
		cliente = &http.Client{Timeout: plazo}
	}
	res, err := cliente.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pdf: no se pudo contactar con la autoridad de sellado: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(res.Body, maxRespuestaSello))
	if err != nil {
		return nil, fmt.Errorf("pdf: respuesta de sellado ilegible: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrSelloRechazado, res.StatusCode)
	}
	token, err := tokenDeRespuesta(datos)
	if err != nil {
		return nil, err
	}
	// Sobre HTTP plano esto es OBLIGATORIO: un intermediario puede sustituir el
	// token por otro auténtico de otra huella (`comprobarToken`) o fabricar uno
	// falso (lo para la verificación con las anclas, justo debajo).
	if err := comprobarToken(token, resumen, pet.Nonce); err != nil {
		return nil, err
	}
	if opts.Anclas != nil {
		if _, _, err := cms.VerifyEncapsulated(token, oidTokenSello, x509.VerifyOptions{
			Roots: opts.Anclas, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		}); err != nil {
			return nil, fmt.Errorf("%w: el token no es de la autoridad configurada: %w", ErrSelloRechazado, err)
		}
	}
	return token, nil
}

// tokenDeRespuesta valida el estado y saca el token.
func tokenDeRespuesta(datos []byte) ([]byte, error) {
	var resp respuestaDeSello
	if _, err := asn1.Unmarshal(datos, &resp); err != nil {
		return nil, fmt.Errorf("pdf: respuesta de sellado ilegible: %w", err)
	}
	// 0 = concedido, 1 = concedido con cambios. Cualquier otra cosa es un no, y
	// hay que distinguirlo: un 2 («rechazo») con un token pegado detrás sería un
	// sello que no vale nada.
	if resp.Estado.Estado != 0 && resp.Estado.Estado != 1 {
		return nil, fmt.Errorf("%w: estado %d", ErrSelloRechazado, resp.Estado.Estado)
	}
	if len(resp.Token.FullBytes) == 0 {
		return nil, fmt.Errorf("%w: sin token", ErrSelloRechazado)
	}
	// Se devuelven los BYTES ORIGINALES: el token va firmado por la autoridad y
	// recodificarlo invalidaría su propia firma.
	return resp.Token.FullBytes, nil
}

// oidDesdeTexto parsea un OID en notación con puntos.
func oidDesdeTexto(s string) (asn1.ObjectIdentifier, error) {
	var out asn1.ObjectIdentifier
	n := 0
	for i := 0; i <= len(s); i++ {
		if i != len(s) && s[i] != '.' {
			continue
		}
		if i == n {
			return nil, fmt.Errorf("pdf: OID inválido %q", s)
		}
		v := 0
		for _, c := range s[n:i] {
			if c < '0' || c > '9' {
				return nil, fmt.Errorf("pdf: OID inválido %q", s)
			}
			v = v*10 + int(c-'0')
		}
		out = append(out, v)
		n = i + 1
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("pdf: OID inválido %q", s)
	}
	return out, nil
}

// atributoDeSello envuelve el token en el atributo no firmado que espera un
// validador de PDF.
func atributoDeSello(token []byte) atributo {
	return atributo{
		Tipo:    oidAtributoSelloFirma,
		Valores: []asn1.RawValue{{FullBytes: token}},
	}
}

// --- Verificación del token --------------------------------------------------

// El contenido de un token de sello: la estructura por la que hay que bajar para
// llegar al TSTInfo, que es lo único que dice QUÉ se selló.
type (
	// tokenFirmado es el SignedData del token. Los campos que no se miran se
	// declaran igual porque `encoding/asn1` es estricto con el orden y con lo que
	// sobra: omitirlos hace fallar el parseo de un token perfectamente válido.
	tokenFirmado struct {
		Version           int
		AlgoritmosResumen asn1.RawValue `asn1:"set"`
		Contenido         contenidoConDatos
		Certificados      asn1.RawValue `asn1:"optional,tag:0"`
		CRLs              asn1.RawValue `asn1:"optional,tag:1"`
		Firmantes         asn1.RawValue `asn1:"set"`
	}
	contenidoConDatos struct {
		Tipo  asn1.ObjectIdentifier
		Datos asn1.RawValue `asn1:"optional,tag:0,explicit"`
	}
	// precisionSello es Accuracy. Tiene tipo propio y NO es un `asn1.RawValue`
	// por un motivo que cuesta encontrar: un RawValue opcional encaja con
	// CUALQUIER elemento, así que se comía el nonce que viene detrás cuando el
	// token no traía precisión. El síntoma era un nonce nil y una comprobación que
	// pasaba sin comprobar nada.
	precisionSello struct {
		Segundos int `asn1:"optional"`
		Millis   int `asn1:"optional,tag:0"`
		Micros   int `asn1:"optional,tag:1"`
	}
	// infoDelSello es TSTInfo (RFC 3161 §2.4.2).
	infoDelSello struct {
		Version       int
		Politica      asn1.ObjectIdentifier
		HuellaMensaje huellaDeMensaje
		Serie         *big.Int
		Generado      time.Time
		Precision     precisionSello `asn1:"optional"`
		Ordenado      bool           `asn1:"optional"`
		Nonce         *big.Int       `asn1:"optional"`
		TSA           asn1.RawValue  `asn1:"optional,tag:0"`
		Extensiones   asn1.RawValue  `asn1:"optional,tag:1"`
	}
)

// infoDeToken baja por las tres capas del token hasta el TSTInfo.
//
// El camino es ContentInfo → SignedData → eContent, y el eContent es un OCTET
// STRING que ENVUELVE el TSTInfo: olvidar esa capa da un error de ASN.1
// desconcertante sobre un token perfectamente válido.
func infoDeToken(token []byte) (infoDelSello, error) {
	var info infoDelSello
	var envoltorio contenidoInfo
	if _, err := asn1.Unmarshal(token, &envoltorio); err != nil {
		return info, fmt.Errorf("%w: el token no es un ContentInfo: %w", ErrSelloRechazado, err)
	}
	var firmado tokenFirmado
	if _, err := asn1.Unmarshal(envoltorio.Contenido.Bytes, &firmado); err != nil {
		return info, fmt.Errorf("%w: el token no es un SignedData: %w", ErrSelloRechazado, err)
	}
	if !firmado.Contenido.Tipo.Equal(oidTokenSello) {
		return info, fmt.Errorf("%w: el contenido del token no es un TSTInfo", ErrSelloRechazado)
	}
	var sobre []byte
	if _, err := asn1.Unmarshal(firmado.Contenido.Datos.Bytes, &sobre); err != nil {
		return info, fmt.Errorf("%w: el TSTInfo no viene en un OCTET STRING: %w", ErrSelloRechazado, err)
	}
	if _, err := asn1.Unmarshal(sobre, &info); err != nil {
		return info, fmt.Errorf("%w: TSTInfo ilegible: %w", ErrSelloRechazado, err)
	}
	return info, nil
}

// comprobarToken verifica que el token sella EXACTAMENTE lo que se pidió.
//
// POR QUÉ ESTO NO ES OPCIONAL. Las autoridades de sellado hablan HTTP PLANO —lo
// hacen TODAS las que existen, y es defendible: de ida viaja un hash y de vuelta
// algo firmado, así que nadie puede forjar un token ni leer el documento—. Pero un
// intermediario sí puede SUSTITUIRLO: devolver un token auténtico y válido... de
// otra huella.
//
// Sin esta comprobación el PDF se guardaría con un sello que no cubre su firma, y
// no daría ningún error: abriría bien, enseñaría una fecha, y el día que alguien
// lo valide de verdad diría que el sello no corresponde. Es el fallo que más caro
// sale, el que se descubre justo cuando hace falta la prueba.
func comprobarToken(token, resumen []byte, nonce *big.Int) error {
	info, err := infoDeToken(token)
	if err != nil {
		return err
	}
	// La huella se pidió en SHA-256: la misma huella con otro algoritmo declarado
	// es un token que un validador calcula distinto y rechaza.
	if !info.HuellaMensaje.Algoritmo.Algorithm.Equal(oidSHA256) {
		return fmt.Errorf("%w: el token sella con otro algoritmo que SHA-256", ErrSelloRechazado)
	}
	if !bytes.Equal(info.HuellaMensaje.Huella, resumen) {
		return fmt.Errorf("%w: el token sella OTRA huella que la pedida", ErrSelloRechazado)
	}
	// Un nonce distinto es un token de otra petición: puede ser auténtico y viejo,
	// que es exactamente el ataque que el nonce existe para cerrar.
	if nonce != nil && info.Nonce != nil && info.Nonce.Cmp(nonce) != 0 {
		return fmt.Errorf("%w: el nonce del token no es el de esta petición", ErrSelloRechazado)
	}
	return nil
}

// FechaDelSello saca la hora que atestigua un token, para que la interfaz pueda
// decir «sellado el …» en vez de un genérico «tiene sello».
func FechaDelSello(token []byte) (time.Time, error) {
	info, err := infoDeToken(token)
	if err != nil {
		return time.Time{}, err
	}
	return info.Generado, nil
}
