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

const cacheTime = 30 * time.Second

type mateRequest struct {
	Method string
	Body   json.RawMessage
}

type mateServer struct {
	client      *lspClient
	openFiles   map[string]time.Time
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
		w.Header().Set("Content-Type", "application/json")
		Log.WithField("method", mr.Method).Warn("Time out")
		json.NewEncoder(w).Encode(KeyValue{"result": "error", "message": "time out"})
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
	timer := time.NewTimer(10 * time.Second)

	events.Once("request."+strconv.Itoa(reqID), func(event string, payload ...interface{}) {
		timer.Stop()
		cb <- &KeyValue{"result": payload[0]}
	})
	s.client.request(reqID, method, params)

	// block until got response or timeout
	<-timer.C
	Log.WithField("method", method).Warn(" request " + strconv.Itoa(reqID) + " timed out")
	cb <- &KeyValue{"result": "error", "message": method + " request " + strconv.Itoa(reqID) + " timed out"}
	timer.Stop()
	events.RemoveAllListeners("request." + strconv.Itoa(reqID))
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
		s.Lock()
		defer s.Unlock()
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

		if _, ok := s.openFiles[fn]; ok {
			s.client.notification("textDocument/didClose", DocumentSymbolParams{TextDocumentIdentifier{
				DocumentURI(fn),
			}})
			time.Sleep(50 * time.Millisecond)
		}
		s.client.notification("textDocument/didOpen", DidOpenTextDocumentParams{textDocument})
		s.openFiles[fn] = time.Now()
		cb <- &KeyValue{"result": "ok"}
	case "didClose":
		s.Lock()
		defer s.Unlock()
		textDocument := TextDocumentIdentifier{}
		if err := json.Unmarshal(mr.Body, &textDocument); err != nil {
			cb <- &KeyValue{"result": "error", "message": err.Error()}
			return
		}

		fn := string(textDocument.URI)
		if len(fn) == 0 {
			cb <- &KeyValue{"result": "error", "message": "Invalid document uri"}
			return
		}
		s.client.notification("textDocument/didClose", DocumentSymbolParams{textDocument})
		if _, ok := s.openFiles[fn]; ok {
			delete(s.openFiles, fn)
		}
		cb <- &KeyValue{"result": "ok"}
	default:
		cb <- &KeyValue{"result": "error", "message": "unknown method"}
	}
}

func (s *mateServer) startListeners() {
	defer s.handlePanic(mateRequest{})

	events.On("request.1", func(event string, payload ...interface{}) {
		s.client.notification("initialized", KeyValue{})
		s.client.notification("workspace/didChangeConfiguration", DidChangeConfigurationParams{
			KeyValue{"intelephense.files.maxSize": 3000000},
		})
		events.Emit("initialized")
	})
	timer := time.NewTicker(30 * time.Second)

	for {
		select {
		case r := <-s.client.responseChan:
			switch r.Method {
			case "restart":
				s.initialized = false
				s.openFiles = make(map[string]time.Time)
			case "client/registerCapability":
				s.client.notification("client/registerCapability", KeyValue{})
			case "workspace/configuration":
				// temporary
				cfg := KeyValue{
					"files": KeyValue{
						"maxSize":      300000,
						"associations": []string{"*.php", "*.phtml"},
						"exclude": []string{
							"**/.git/**",
							"**/.svn/**",
							"**/.hg/**",
							"**/CVS/**",
							"**/.DS_Store/**",
							"**/node_modules/**",
							"**/bower_components/**",
							"**/vendor/**/{Test,test,Tests,tests}/**",
							"**/.git",
							"**/.svn",
							"**/.hg",
							"**/CVS",
							"**/.DS_Store",
							"**/nova/tests/**",
							"**/faker/**",
							"**/*.log",
							"**/*.log*",
							"**/*.min.*",
							"**/dist",
							"**/coverage",
							"**/build/*",
							"**/nova/public/*",
							"**/public/*",
						},
					},
					"stubs": []string{
						"apache",
						"bcmath",
						"bz2",
						"calendar",
						"com_dotnet",
						"Core",
						"ctype",
						"curl",
						"date",
						"dba",
						"dom",
						"enchant",
						"exif",
						"fileinfo",
						"filter",
						"fpm",
						"ftp",
						"gd",
						"hash",
						"iconv",
						"imap",
						"interbase",
						"intl",
						"json",
						"ldap",
						"libxml",
						"mbstring",
						"mcrypt",
						"meta",
						"mssql",
						"mysqli",
						"oci8",
						"odbc",
						"openssl",
						"pcntl",
						"pcre",
						"PDO",
						"pdo_ibm",
						"pdo_mysql",
						"pdo_pgsql",
						"pdo_sqlite",
						"pgsql",
						"Phar",
						"posix",
						"pspell",
						"readline",
						"recode",
						"Reflection",
						"regex",
						"session",
						"shmop",
						"SimpleXML",
						"snmp",
						"soap",
						"sockets",
						"sodium",
						"SPL",
						"sqlite3",
						"standard",
						"superglobals",
						"sybase",
						"sysvmsg",
						"sysvsem",
						"sysvshm",
						"tidy",
						"tokenizer",
						"wddx",
						"xml",
						"xmlreader",
						"xmlrpc",
						"xmlwriter",
						"Zend OPcache",
						"zip",
						"zlib",
					},
					"completion": KeyValue{
						"insertUseDeclaration":                    true,
						"fullyQualifyGlobalConstantsAndFunctions": false,
						"triggerParameterHints":                   true,
						"maxItems":                                100,
					},
					"format": KeyValue{
						"enable": false,
					},
					"environment": KeyValue{
						"documentRoot": "",
						"includePaths": []string{},
					},
					"runtime":   "",
					"maxMemory": 0,
					"telemetry": KeyValue{"enabled": false},
					"trace": KeyValue{
						"server": "verbose",
					},
				}
				s.client.response(r.ID, "workspace/configuration", []KeyValue{
					cfg,
					cfg,
				})
			default:
				events.Emit("request."+strconv.Itoa(r.ID), r.Result)
			}
		case <-timer.C:
			go s.cleanOpenFiles()
		}
	}
}

func (s *mateServer) cleanOpenFiles() {
	s.Lock()
	defer s.Unlock()
	if len(s.openFiles) == 0 {
		return
	}
	Log.Trace("Cleaning open files...")
	for fn, openTime := range s.openFiles {
		if time.Since(openTime).Seconds() > cacheTime.Seconds() {
			delete(s.openFiles, fn)
			s.client.notification("textDocument/didClose", DocumentSymbolParams{TextDocumentIdentifier{DocumentURI(fn)}})
		}
	}
}

func (s mateServer) initialize(params KeyValue) error {
	dir := params.string("dir", "")
	if len(dir) == 0 {
		return errors.New("Empty dir")
	}
	storagePath := params.string("storage", "/tmp/intelephense/")
	name := params.string("name", "phpProject")

	s.client.request(1, "initialize", InitializeParams{
		ProcessID:             os.Getpid(),
		RootURI:               DocumentURI("file://" + dir),
		RootPath:              dir,
		InitializationOptions: KeyValue{"storagePath": storagePath, "clearCache": false},
		Capabilities: KeyValue{
			"textDocument": KeyValue{
				"synchronization": KeyValue{
					"dynamicRegistration": true,
					"didSave":             true,
					"willSaveWaitUntil":   false,
					"willSave":            true,
				},
				"publishDiagnostics": KeyValue{},
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
					"completionItemKind": KeyValue{
						"valueSet": []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25},
					},
				},
				"hover": KeyValue{
					"dynamicRegistration": true,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
				"signatureHelp": KeyValue{
					"dynamicRegistration": true,
					"signatureInformation": KeyValue{
						"documentationFormat":  []string{"markdown", "plaintext"},
						"parameterInformation": KeyValue{"labelOffsetSupport": true},
					},
				},
				"codeLens":         KeyValue{"dynamicRegistration": true},
				"formatting":       KeyValue{"dynamicRegistration": true},
				"rangeFormatting":  KeyValue{"dynamicRegistration": true},
				"onTypeFormatting": KeyValue{"dynamicRegistration": true},
				"rename": KeyValue{
					"dynamicRegistration": true,
					"prepareSupport":      true,
				},
				"documentLink": KeyValue{"dynamicRegistration": true},
				"typeDefinition": KeyValue{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
				"implementation": KeyValue{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
				"colorProvider": KeyValue{"dynamicRegistration": true},
				"foldingRange": KeyValue{
					"dynamicRegistration": true,
					"rangeLimit":          5000,
					"lineFoldingOnly":     true,
				},
				"declaration": KeyValue{
					"dynamicRegistration": true,
					"linkSupport":         true,
				},
			},

			"workspace": KeyValue{
				"applyEdit":              true,
				"didChangeConfiguration": KeyValue{"dynamicRegistration": true},
				"configuration":          true,
				"executeCommand":         KeyValue{"dynamicRegistration": true},
				"workspaceFolders":       true,
				"symbol": KeyValue{
					"dynamicRegistration": true,
					"symbolKind": KeyValue{
						"valueSet": []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25},
					},
				},
				"workspaceEdit": KeyValue{
					"documentChanges":    true,
					"resourceOperations": []string{"create", "rename", "delete"},
					"failureHandling":    "textOnlyTransactional",
				},
				"didChangeWatchedFiles": KeyValue{"dynamicRegistration": true},
			},
			"workspaceFolders": []KeyValue{
				KeyValue{
					"uri":  "file://" + dir,
					"name": name,
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
	server := mateServer{client: client, openFiles: make(map[string]time.Time), requestID: 1, initialized: false}
	go server.startListeners()

	Log.Fatal(http.ListenAndServe(":"+port, &server))
}
