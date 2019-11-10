package main

import (
	"encoding/json"
	"fmt"
)

const EOL = "\r\n"

type request struct {
	id     int
	method string
	params interface{}
}

type notification struct {
	method string
	params interface{}
}

func (r *request) getBody() KeyValue {
	id := 1
	if r.id > 0 {
		id = r.id
	}
	return KeyValue{
		"id":     id,
		"method": r.method,
		"params": r.params,
	}
}

func (r *request) format() string {
	body := r.getBody()
	body["jsonrpc"] = "2.0"

	json, _ := json.Marshal(body)

	headers := fmt.Sprintf("Content-Length: %d", len(json)) + EOL +
		"Content-Type: application/vscode-jsonrpc; charset=utf-8"

	return fmt.Sprintf("%s%s%s%s", headers, EOL, EOL, json)
}

func (r request) getMethod() string {
	return r.method
}

func (r *notification) getBody() KeyValue {
	return KeyValue{
		"method": r.method,
		"params": r.params,
	}
}

func (r *notification) format() string {
	body := r.getBody()
	body["jsonrpc"] = "2.0"

	json, _ := json.Marshal(body)

	headers := fmt.Sprintf("Content-Length: %d", len(json)) + EOL +
		"Content-Type: application/vscode-jsonrpc; charset=utf-8"

	return fmt.Sprintf("%s%s%s%s", headers, EOL, EOL, json)
}

func (r notification) getMethod() string {
	return r.method
}

type response struct {
	ID     int
	Method string
	Params KeyValue
	Result KeyValue
	Error  KeyValue
}
