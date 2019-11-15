package main

import (
	"context"
	"flag"
	stdlog "log"
	"os"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	// Log is a global logrus instance
	Log    *log.Entry
	logrus = log.New()
)

var (
	server   = flag.String("server", "intelephense", `server type (intelephense or phpls), default intelephense`)
	logLevel = flag.String("level", "debug", `log level, default - debug`)
)

func init() {
	flag.Parse()
	stdlog.SetFlags(0)
	stdlog.SetOutput(logrus.Writer())
	logrus.SetReportCaller(true)
	logrus.Out = os.Stdout
	ctx := context.Background()
	Log = logrus.WithContext(ctx)

	logrus.Level, _ = log.ParseLevel(*logLevel)
	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		logrus.Formatter = &log.TextFormatter{ForceColors: false, FullTimestamp: true, TimestampFormat: "Jan 2 15:04:05", CallerPrettyfier: callerPrettyfier}
	} else {
		logrus.Formatter = &HTMLFormatter{FullTimestamp: true, TimestampFormat: "15:04:05", CallerPrettyfier: callerPrettyfier}
	}
}

func main() {
	var client *lspClient
	switch *server {
	case "phpls":
		client = newLspClient(config{true, "php", []string{userHomeDir() + "/.composer/vendor/felixfbecker/language-server/bin/php-language-server.php"}})
	case "intelephense":
		fallthrough
	default:
		client = newLspClient(config{true, "intelephense", []string{"--stdio"}})
	}
	go runProfiler()
	// start server and block
	startServer(client, "8787")
}
