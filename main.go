package main

import (
	"context"
	"fmt"
	stdlog "log"
	"os"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	Log     *log.Entry
	logrus  = log.New()
	version = "master"
)

func init() {
	stdlog.SetFlags(0)
	stdlog.SetOutput(logrus.Writer())
	logrus.SetReportCaller(true)
	logrus.Out = os.Stdout
	ctx := context.Background()
	Log = logrus.WithContext(ctx)

	logrus.Level, _ = log.ParseLevel("debug")
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Println("In a terminal")
		logrus.Formatter = &log.TextFormatter{ForceColors: false, FullTimestamp: true, TimestampFormat: "Jan 2 15:04:05", CallerPrettyfier: callerPrettyfier}
	} else {
		logrus.Formatter = &HtmlFormatter{FullTimestamp: true, TimestampFormat: "15:04:05", CallerPrettyfier: callerPrettyfier}
	}
}

func main() {
	// "php", []string{userHomeDir+"/.composer/vendor/felixfbecker/language-server/bin/php-language-server.php"}
	proc := newLspClient(config{true, "intelephense", []string{"--stdio"}})

	// start server and block
	startServer(proc, "8787")
}
