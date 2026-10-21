// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package verifactu

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Direcciones del servicio. «Sello» son las de quien se autentica con un
// certificado de sello de entidad en vez de uno de persona o de representante.
const (
	urlPruebas         = "https://prewww1.aeat.es/wlpl/TIKE-CONT/ws/SistemaFacturacion/VerifactuSOAP"
	urlPruebasSello    = "https://prewww10.aeat.es/wlpl/TIKE-CONT/ws/SistemaFacturacion/VerifactuSOAP"
	urlProduccion      = "https://www1.agenciatributaria.gob.es/wlpl/TIKE-CONT/ws/SistemaFacturacion/VerifactuSOAP"
	urlProduccionSello = "https://www10.agenciatributaria.gob.es/wlpl/TIKE-CONT/ws/SistemaFacturacion/VerifactuSOAP"
)

// MaxRegistros es el máximo de registros por envío (diseño de registro, 2.2).
const MaxRegistros = 1000

// EsperaInicial es el tiempo entre envíos hasta que la AEAT diga otro (Orden
// HAC/1177/2024, art. 16.2).
const EsperaInicial = 60 * time.Second

// Cliente remite registros a la AEAT con el certificado de quien presenta.
type Cliente struct {
	http *http.Client
	url  string
}

// NuevoCliente prepara el cliente con el certificado cualificado en PEM.
// `produccion` elige el entorno real; `sello`, las direcciones de certificado de
// sello.
func NuevoCliente(certPEM, keyPEM []byte, produccion, sello bool) (*Cliente, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("verifactu: certificado: %w", err)
	}
	url := urlPruebas
	switch {
	case produccion && sello:
		url = urlProduccionSello
	case produccion:
		url = urlProduccion
	case sello:
		url = urlPruebasSello
	}
	return &Cliente{url: url, http: &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
				// Los servidores de la AEAT pueden pedir el certificado de cliente
				// renegociando en TLS 1.2, y Go por defecto corta la conexión.
				Renegotiation: tls.RenegotiateOnceAsClient,
			},
		},
	}}, nil
}

// Linea es el resultado de un registro.
type Linea struct {
	Ref         string `xml:"RefExterna"`
	NumSerie    string `xml:"IDFactura>NumSerieFactura"`
	Estado      string `xml:"EstadoRegistro"`
	Codigo      int    `xml:"CodigoErrorRegistro"`
	Descripcion string `xml:"DescripcionErrorRegistro"`
	// Duplicado viene cuando la AEAT ya tenía ese registro: la respuesta de un
	// envío anterior se perdió por el camino.
	Duplicado *struct {
		Estado      string `xml:"EstadoRegistroDuplicado"`
		Codigo      int    `xml:"CodigoErrorRegistro"`
		Descripcion string `xml:"DescripcionErrorRegistro"`
	} `xml:"RegistroDuplicado"`
}

// Respuesta es lo que devuelve la AEAT a un envío que ha podido leer.
type Respuesta struct {
	CSV    string  `xml:"CSV"`
	Espera int     `xml:"TiempoEsperaEnvio"`
	Estado string  `xml:"EstadoEnvio"`
	Lineas []Linea `xml:"RespuestaLinea"`
}

// Error de envío. Cliente = el mensaje está mal y repetirlo no sirve hasta
// corregirlo; lo demás (fallo del servidor, red, respuesta ilegible) se repite.
type ErrorEnvio struct {
	Cliente bool
	Detalle string
}

func (e *ErrorEnvio) Error() string { return "verifactu: " + e.Detalle }

// EstadoFinal traduce el estado de una línea a lo que se guarda: el de la AEAT o,
// si lo rechazó por duplicado, el del registro que ya tenía.
func (l Linea) EstadoFinal() (estado string, codigo int, descripcion string) {
	if l.Duplicado != nil && l.Estado == "Incorrecto" {
		switch l.Duplicado.Estado {
		case "Correcta":
			return "Correcto", 0, ""
		case "AceptadaConErrores":
			return "AceptadoConErrores", l.Duplicado.Codigo, l.Duplicado.Descripcion
		}
	}
	return l.Estado, l.Codigo, l.Descripcion
}

// Sobre arma el mensaje SOAP con la cabecera del obligado y los registros ya
// generados (los fragmentos que devuelven Alta.XML y Anulacion.XML).
func Sobre(obligado Parte, registros []string) []byte {
	var e escritor
	e.b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	e.b.WriteString(`<soapenv:Envelope xmlns:soapenv="` + nsSOAP + `" xmlns:sum="` + nsSuministroLR + `" xmlns:sum1="` + nsSuministroInformacion + `">`)
	e.b.WriteString(`<soapenv:Header/><soapenv:Body><sum:RegFactuSistemaFacturacion><sum:Cabecera>`)
	e.abrir("ObligadoEmision")
	e.campo("NombreRazon", truncar(obligado.Nombre, 120))
	e.campo("NIF", obligado.NIF)
	e.cerrar("ObligadoEmision")
	e.b.WriteString(`</sum:Cabecera>`)
	for _, r := range registros {
		e.b.WriteString(`<sum:RegistroFactura>` + r + `</sum:RegistroFactura>`)
	}
	e.b.WriteString(`</sum:RegFactuSistemaFacturacion></soapenv:Body></soapenv:Envelope>`)
	return []byte(e.b.String())
}

// Enviar remite los registros (como mucho MaxRegistros) de un obligado.
func (c *Cliente) Enviar(ctx context.Context, obligado Parte, registros []string) (Respuesta, error) {
	if len(registros) == 0 || len(registros) > MaxRegistros {
		return Respuesta{}, fmt.Errorf("verifactu: un envío lleva entre 1 y %d registros", MaxRegistros)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(Sobre(obligado, registros)))
	if err != nil {
		return Respuesta{}, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `""`)
	resp, err := c.http.Do(req)
	if err != nil {
		return Respuesta{}, &ErrorEnvio{Detalle: err.Error()}
	}
	defer resp.Body.Close()
	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Respuesta{}, &ErrorEnvio{Detalle: err.Error()}
	}
	return LeerRespuesta(cuerpo, resp.StatusCode)
}

// LeerRespuesta interpreta el cuerpo SOAP. Un Fault llega con 500 y trae el
// motivo; un cuerpo que no es la respuesta esperada se trata como fallo de red.
func LeerRespuesta(cuerpo []byte, status int) (Respuesta, error) {
	var sobre struct {
		Body struct {
			Fault *struct {
				Code   string `xml:"faultcode"`
				String string `xml:"faultstring"`
			} `xml:"Fault"`
			Respuesta *Respuesta `xml:"RespuestaRegFactuSistemaFacturacion"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(cuerpo, &sobre); err != nil {
		return Respuesta{}, &ErrorEnvio{Detalle: fmt.Sprintf("respuesta ilegible (HTTP %d): %v", status, err)}
	}
	if f := sobre.Body.Fault; f != nil {
		_, codigo, _ := strings.Cut(f.Code, ":")
		if codigo == "" {
			codigo = f.Code
		}
		return Respuesta{}, &ErrorEnvio{Cliente: codigo == "Client", Detalle: strings.TrimSpace(f.String)}
	}
	if sobre.Body.Respuesta == nil {
		return Respuesta{}, &ErrorEnvio{Detalle: fmt.Sprintf("respuesta sin resultado (HTTP %d)", status)}
	}
	return *sobre.Body.Respuesta, nil
}

// EsperaDe es cuánto esperar antes del siguiente envío según la respuesta.
func (r Respuesta) EsperaDe() time.Duration {
	if r.Espera <= 0 {
		return EsperaInicial
	}
	return time.Duration(r.Espera) * time.Second
}
