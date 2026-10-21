// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
)

// Firma de los webhooks SALIENTES.
//
// Un token que viaja en claro en cada entrega no prueba nada: quien vea UNA petición
// puede forjar todas las siguientes. La firma sí, porque el secreto nunca sale de los
// dos extremos y cubre el cuerpo entero: cambiar un byte invalida la firma.
//
// Se emiten las cabeceras EXACTAS de Slack y de Teams —no un formato propio— porque el
// objetivo es que un bot ya escrito para ellos valide sin tocar una línea: Bolt y
// slack_sdk comprueban X-Slack-Signature y rechazan con 401 lo que no la traiga.

// SlackSignatureHeader y SlackTimestampHeader son las cabeceras que comprueban Bolt y
// slack_sdk. El timestamp va aparte y DENTRO de lo firmado: sin él, una petición
// capturada se podría reenviar tal cual para siempre.
const (
	SlackSignatureHeader = "X-Slack-Signature"
	SlackTimestampHeader = "X-Slack-Request-Timestamp"
	TeamsAuthHeader      = "Authorization"
)

// SignSlack devuelve el valor de X-Slack-Signature para un cuerpo: HMAC-SHA256 de
// «v0:<timestamp>:<cuerpo>» en hexadecimal, con el prefijo de versión que espera el
// receptor.
func SignSlack(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v0:" + strconv.FormatInt(ts, 10) + ":"))
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

// SignTeams devuelve el valor de Authorization para el dialecto Teams: «HMAC <b64>»,
// donde b64 es el HMAC-SHA256 del cuerpo en base64.
//
// La clave es el secreto BASE64-DECODIFICADO: es lo que hace Teams con la «clave de
// seguridad» que muestra en su interfaz, y por eso el secreto se genera ya en base64.
// Si no fuera base64 válido se usan los bytes tal cual, que es lo único que puede
// hacerse sin fallar en silencio.
func SignTeams(secret string, body []byte) string {
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		key = []byte(secret)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	return "HMAC " + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
