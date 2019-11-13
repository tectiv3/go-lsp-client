package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/textproto"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type config struct {
	stdio  bool
	url    string
	params []string
}

type lspClient struct {
	config       config
	reqID        int
	in           io.ReadCloser
	out          io.WriteCloser
	responseChan chan *response
	crashesCount int
	sync.Mutex
}

func newLspClient(config config) *lspClient {
	client := lspClient{config: config}
	client.reqID = 0
	client.responseChan = make(chan *response)

	client.connectToServer()

	return &client
}

func (p *lspClient) connectToServer() {
	if p.config.stdio {
		cmd := exec.Command(p.config.url, p.config.params...)

		stdin, err := cmd.StdinPipe()
		checkError(err)
		p.out = stdin

		stdout, err := cmd.StdoutPipe()
		checkError(err)
		p.in = stdout

		stderr, err := cmd.StderrPipe()
		checkError(err)
		go p.readPipe(stderr)

		if err := cmd.Start(); err != nil {
			checkError(err)
		}
		go func() {
			if err := cmd.Wait(); err != nil {
				p.crashesCount++
				if p.crashesCount == 10 {
					checkError(err)
				}
				Log.WithField("err", err).Info("Restarting server after a crash...")
				go p.connectToServer()
				p.responseChan <- &response{Method: "restart"}
			}
		}()
	} else {
		conn, err := net.Dial("tcp", p.config.url)
		checkError(err)
		p.in = conn
		p.out = conn
	}

	go p.listen()
}

func (p *lspClient) listen() {
	Log.Info("Listening for messages, ^c to exit")
	for {
		msg, err := p.receive()
		if err != nil {
			Log.Error(err)
			break
		}
		if msg != nil {
			go p.processMessage(msg)
		}
	}
	Log.Info("Listener finished")
}

func (p *lspClient) readPipe(conn io.ReadCloser) {
	reader := bufio.NewReader(conn)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			Log.Error(err)
			return
		}
		if reader.Buffered() > 0 {
			var msgData []byte
			msgData = append(msgData, b)
			for reader.Buffered() > 0 {
				// read byte by byte until the buffered data is not empty
				b, err := reader.ReadByte()
				if err == nil {
					msgData = append(msgData, b)
				} else {
					log.Println("-------> unreadable caracter...", b)
				}
			}
			// msgData now contain the buffered data...
			Log.Error(string(msgData))
		}
	}
}

func (p *lspClient) processMessage(r *response) {
	if r.Method == "window/logMessage" {
		Log.Info(r.Params["message"])
	} else if r.Method == "serenata/didProgressIndexing" {
		Log.Info(r.Params["info"])
	} else {
		Log.WithField("method", r.Method).WithField("params", r.Params).Trace(string(r.Result))
		p.responseChan <- r
	}
}

func (p *lspClient) request(id int, method string, params interface{}) {
	p.Lock()
	defer p.Unlock()
	r := request{id, method, params}
	Log.Info(method)
	p.send(r.format())
}

func (p *lspClient) notification(method string, params interface{}) {
	p.Lock()
	defer p.Unlock()
	n := notification{method, params}
	Log.Info(method)
	p.send(n.format())
}

func (p *lspClient) response(id int, method string, res interface{}) {
	p.Lock()
	defer p.Unlock()
	result, _ := json.Marshal(res)
	r := response{ID: id, Method: method, Result: result}
	Log.Info(method)
	p.send(r.format())
}

func (p *lspClient) send(msg string) {
	Log.Trace(msg)
	fmt.Fprint(p.out, msg)
}

func (p *lspClient) receive() (*response, error) {
	reader := bufio.NewReader(p.in)
	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			Log.Error(err)
			return nil, err
		}
		Log.Trace(str)

		tp := textproto.NewReader(bufio.NewReader(strings.NewReader(str + "\n")))
		mimeHeader, err := tp.ReadMIMEHeader()
		if err != nil {
			Log.WithField("str", str).Error(err)
			break
		}

		if l, ok := mimeHeader["Content-Length"]; ok {
			_, err := reader.ReadString('\n')
			if err != nil {
				Log.Error(err)
				break
			}

			jsonLen, _ := strconv.ParseInt(l[0], 10, 32)

			buf := make([]byte, jsonLen)
			_, err = io.ReadFull(reader, buf)
			if err != nil {
				Log.Error(err)
				break
			}
			Log.Trace(string(buf))

			response := response{}
			if err := json.Unmarshal(buf, &response); err != nil {
				Log.WithField("err", err).Warn(string(buf))
			}
			return &response, nil
		}
	}
	return nil, nil
}
