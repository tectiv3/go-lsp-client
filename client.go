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
)

type config struct {
	stdio  bool
	url    string
	params []string
}

type lspClient struct {
	conn         io.ReadWriteCloser
	reqID        int
	in           io.ReadCloser
	out          io.WriteCloser
	responseChan chan *response
}

func newLspClient(config config) *lspClient {
	client := lspClient{}
	client.reqID = 0
	client.responseChan = make(chan *response)

	if config.stdio {
		cmd := exec.Command(config.url, config.params...)

		stdin, err := cmd.StdinPipe()
		checkError(err)
		client.out = stdin

		stdout, err := cmd.StdoutPipe()
		checkError(err)
		client.in = stdout

		stderr, err := cmd.StderrPipe()
		checkError(err)
		go client.readPipe(stderr)

		if err := cmd.Start(); err != nil {
			checkError(err)
		}
		go func() {
			checkError(cmd.Wait())
		}()
	} else {
		conn, err := net.Dial("tcp", config.url)
		checkError(err)
		client.in = conn
		client.out = conn
	}

	go client.listen()

	return &client
}

func (p *lspClient) listen() {
	Log.Info("Listening for messages, ^c to exit")
	for {
		if msg := p.receive(); msg != nil {
			go p.processMessage(msg)
		}
	}
}

func (p *lspClient) readPipe(conn io.ReadCloser) {
	reader := bufio.NewReader(conn)
	for {
		b, _ := reader.ReadByte()
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
		Log.WithField("method", r.Method).WithField("params", r.Params).Debug(r.Result)
		p.responseChan <- r
	}
}

func (p *lspClient) request(id int, method string, params interface{}) {
	r := request{id, method, params}
	p.send(r.format())
}

func (p *lspClient) notification(method string, params interface{}) {
	n := notification{method, params}
	p.send(n.format())
}

func (p *lspClient) send(msg string) {
	Log.Trace(msg)
	fmt.Fprintf(p.out, msg)
}

func (p *lspClient) receive() *response {
	reader := bufio.NewReader(p.in)
	for {
		str, err := reader.ReadString('\n')
		checkError(err)
		Log.Trace(str)

		tp := textproto.NewReader(bufio.NewReader(strings.NewReader(str + "\n")))
		mimeHeader, err := tp.ReadMIMEHeader()
		if err != nil {
			Log.WithField("str", str).Error(err)
			break
		}

		if l, ok := mimeHeader["Content-Length"]; ok {
			_, err := reader.ReadString('\n')
			checkError(err)

			jsonLen, _ := strconv.ParseInt(l[0], 10, 32)

			buf := make([]byte, jsonLen)
			_, err = io.ReadFull(reader, buf)
			checkError(err)
			Log.Trace(string(buf))

			response := response{}
			if err := json.Unmarshal(buf, &response); err != nil {
				Log.WithField("str", string(buf)).Warn(string(buf))
			}
			return &response
		}
	}
	return nil
}
