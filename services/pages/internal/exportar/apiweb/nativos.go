// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package apiweb

import (
	"fmt"
	"strings"
)

// GenerarClienteNativo genera los clientes mínimos y deterministas de la API
// pública. Todos conservan la misma base y cabecera del pedido que el cliente
// TypeScript; la UI generada por IA solo los consume.
func GenerarClienteDart(baseURL string) []byte {
	return []byte(fmt.Sprintf(`// Generated from apiweb/rutas.go. Do not edit.
import 'dart:convert';
import 'package:http/http.dart' as http;
class PagesApi { PagesApi(this.baseUrl); final String baseUrl;
  Future<Map<String,dynamic>> getOrder(String id, String token) async { final r=await http.get(Uri.parse('$baseUrl/_pages/orders/$id'),headers:{'X-Sytue-Pedido':token}); if(r.statusCode!=200) throw StateError('Pages API ${r.statusCode}'); return jsonDecode(r.body) as Map<String,dynamic>; }
}
const pagesBaseUrl=%q;
`, normalizarBase(baseURL)))
}

func GenerarClienteKotlin(baseURL string) []byte {
	return []byte(fmt.Sprintf(`// Generated from apiweb/rutas.go. Do not edit.
package pagesapi
import java.net.HttpURLConnection
import java.net.URL
class PagesApi(private val baseUrl:String){ fun getOrder(id:String,token:String):String { val c=URL("$baseUrl/_pages/orders/$id").openConnection() as HttpURLConnection; c.setRequestProperty("X-Sytue-Pedido",token); check(c.responseCode==200); return c.inputStream.bufferedReader().readText() } }
const val PAGES_BASE_URL=%q
`, normalizarBase(baseURL)))
}

func GenerarClienteSwift(baseURL string) []byte {
	return []byte(fmt.Sprintf(`// Generated from apiweb/rutas.go. Do not edit.
import Foundation
struct PagesAPI { let baseURL: URL
  func order(_ id:String, token:String) async throws -> Data { var request=URLRequest(url: baseURL.appending(path:"_pages/orders/"+id)); request.setValue(token,forHTTPHeaderField:"X-Sytue-Pedido"); let(data,response)=try await URLSession.shared.data(for:request); guard (response as? HTTPURLResponse)?.statusCode==200 else { throw URLError(.badServerResponse) }; return data }
}
let pagesBaseURL=%q
`, normalizarBase(baseURL)))
}

func GenerarClienteGo(baseURL string) []byte {
	return []byte(fmt.Sprintf(`// Code generated from apiweb/rutas.go; DO NOT EDIT.
package pagesapi
import("fmt";"io";"net/http")
const BaseURL=%q
func GetOrder(client *http.Client,id,token string)([]byte,error){ req,err:=http.NewRequest(http.MethodGet,BaseURL+"/_pages/orders/"+id,nil);if err!=nil{return nil,err};req.Header.Set("X-Sytue-Pedido",token);res,err:=client.Do(req);if err!=nil{return nil,err};defer res.Body.Close();if res.StatusCode!=http.StatusOK{return nil,fmt.Errorf("pages api: %%s",res.Status)};return io.ReadAll(res.Body) }
`, normalizarBase(baseURL)))
}

func normalizarBase(baseURL string) string {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "https://ejemplo.sytue.eu"
	}
	return baseURL
}
