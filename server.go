package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/tectiv3/go-lsp-client/events"
)

const cacheTime = 10 * time.Minute

type mateRequest struct {
	Method string
	Body   json.RawMessage
}

type mateServer struct {
	client      *lspClient
	requestID   int
	initialized bool
	sync.Mutex
}

func (s *mateServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if perr := Panicf(recover(), "%v", r.Method); perr != nil {
			Log.Error(perr)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}()

	Log.WithField("method", r.Method).WithField("length", r.ContentLength).Debug(r.URL.Path)

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	decoder := json.NewDecoder(r.Body)
	mr := mateRequest{}
	err := decoder.Decode(&mr)
	if err != nil {
		Log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resultChan := make(kvChan)
	var result *KeyValue
	var tick <-chan time.Time
	tick = time.After(20 * time.Second)

	go s.processRequest(mr, resultChan)

	// block until result or timeout
	select {
	case <-tick:
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	case result = <-resultChan:
	}

	if result == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	tr, _ := json.Marshal(result)
	Log.WithField("method", mr.Method).Debug(string(tr))
	json.NewEncoder(w).Encode(result)
}

func (s *mateServer) requestAndWait(method string, params interface{}, cb kvChan) {
	s.Lock()
	s.requestID++
	reqID := s.requestID
	s.Unlock()
	// subscribe to response
	timer := time.NewTimer(10 * time.Second)
	listener := func(event string, payload ...interface{}) {
		timer.Stop()
		cb <- &KeyValue{"result": payload[0]}
	}

	events.Once("request."+strconv.Itoa(reqID), listener)
	s.client.request(reqID, method, params)

	// block until got response or timeout
	select {
	case <-timer.C:
		timer.Stop()
		events.RemoveListener("request."+strconv.Itoa(reqID), listener)
		cb <- &KeyValue{"result": "error", "message": method + " request time out"}
		return
	default:
		timer.Stop()
	}
}

func (s *mateServer) processRequest(mr mateRequest, cb kvChan) {
	defer s.handlePanic(mr)
	Log.WithField("method", mr.Method).Trace(string(mr.Body))
	switch mr.Method {
	case "hover":
		params := TextDocumentPositionParams{}
		if err := json.Unmarshal(mr.Body, &params); err != nil {
			cb <- &KeyValue{"result": "error", "message": err.Error()}
			return
		}
		s.requestAndWait("textDocument/hover", params, cb)
	case "completion":
		params := CompletionParams{}
		if err := json.Unmarshal(mr.Body, &params); err != nil {
			cb <- &KeyValue{"result": "error", "message": err.Error()}
			return
		}
		s.requestAndWait("textDocument/completion", params, cb)
	case "definition":
		params := TextDocumentPositionParams{}
		if err := json.Unmarshal(mr.Body, &params); err != nil {
			cb <- &KeyValue{"result": "error", "message": err.Error()}
			return
		}
		s.requestAndWait("textDocument/definition", params, cb)
	case "initialize":
		s.Lock()
		defer s.Unlock()
		if s.initialized {
			cb <- &KeyValue{"result": "ok", "message": "already initialized"}
			return
		}
		params := KeyValue{}
		if err := json.Unmarshal(mr.Body, &params); err != nil {
			cb <- &KeyValue{"result": "error", "message": err.Error()}
			return
		}

		timer := time.NewTimer(10 * time.Second)
		s.initialize(params)

		// subscribe to initialized response and wait for it
		listener := func(event string, payload ...interface{}) {
			timer.Stop()
			s.initialized = true
			cb <- &KeyValue{"result": "ok"}
		}
		events.Once("initialized", listener)

		// block until got response for initialized or timeout
		select {
		case <-timer.C:
			timer.Stop()
			s.initialized = true
			events.RemoveListener("initialized", listener)
			s.client.notification("initialized", KeyValue{}) // notify server that we are ready
			s.client.notification("workspace/didChangeConfiguration", DidChangeConfigurationParams{
				KeyValue{"intelephense.files.maxSize": 3000000},
			})
			cb <- &KeyValue{"result": "ok"}
			return
		default:
			timer.Stop()
		}
	case "didOpen":
		textDocument := TextDocumentItem{}
		if err := json.Unmarshal(mr.Body, &textDocument); err != nil {
			cb <- &KeyValue{"result": "error", "message": err.Error()}
			return
		}

		fn := string(textDocument.URI)
		if len(fn) == 0 {
			cb <- &KeyValue{"result": "error", "message": "Invalid document uri"}
			return
		}
		s.client.notification("textDocument/didClose", DocumentSymbolParams{TextDocumentIdentifier{
			DocumentURI(fn),
		}})
		s.client.notification("textDocument/didOpen", DidOpenTextDocumentParams{textDocument})
		cb <- &KeyValue{"result": "ok"}
	default:
		cb <- &KeyValue{"result": "error", "message": "unknown method"}
	}
}

func (s *mateServer) startListeners() {
	defer s.handlePanic(mateRequest{})

	events.Once("request.1", func(event string, payload ...interface{}) {
		s.client.notification("initialized", KeyValue{})
		s.client.notification("workspace/didChangeConfiguration", DidChangeConfigurationParams{
			KeyValue{"intelephense.files.maxSize": 3000000},
		})
		events.Emit("initialized")
	})

	for {
		select {
		case r := <-s.client.responseChan:
			Log.WithField("id", r.ID).WithField("r", r).Trace(string(r.Result))
			events.Emit("request."+strconv.Itoa(r.ID), r.Result)
		}
	}
}

func (s mateServer) initialize(params KeyValue) error {
	dir := params.string("dir", "")
	if len(dir) == 0 {
		return errors.New("Empty dir")
	}
	storagePath := params.string("storage", "/tmp/intelephense/")

	s.client.request(1, "initialize", InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   DocumentURI("file://" + dir),
		RootPath:  dir,
		InitializationOptions: KeyValue{
			"storagePath":   storagePath,
			"files.maxSize": 3000000,
		},
		Capabilities: KeyValue{
			"textDocument": KeyValue{
				"synchronization": KeyValue{
					"didSave":           true,
					"willSaveWaitUntil": true,
				},
				"completion": KeyValue{
					"dynamicRegistration": true,
					"contextSupport":      true,
					"completionItem": KeyValue{
						"snippetSupport":          true,
						"commitCharactersSupport": true,
						"documentationFormat":     []string{"markdown", "plaintext"},
						"deprecatedSupport":       true,
						"preselectSupport":        true,
					},
				},
				"hover": KeyValue{
					"dynamicRegistration": true,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
			},

			"workspace": KeyValue{
				"applyEdit":              true,
				"didChangeConfiguration": KeyValue{"dynamicRegistration": true},
				"configuration":          true,
				"executeCommand":         KeyValue{},
				"symbol": KeyValue{
					"symbolKind": KeyValue{
						"valueSet": []int{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
			},
		},
	})
	return nil
}

func (s mateServer) handlePanic(mr mateRequest) {
	if err := recover(); err != nil {
		Log.WithField("method", mr.Method).WithField("bt", string(debug.Stack())).Error("Recovered from:", err)
	}
}

func startServer(client *lspClient, port string) {
	Log.Info("Running webserver on port " + port)
	server := mateServer{client: client, requestID: 1, initialized: false}
	go server.startListeners()

	Log.Fatal(http.ListenAndServe(":"+port, &server))
}
