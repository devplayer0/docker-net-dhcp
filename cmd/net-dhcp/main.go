package main

import (
	"flag"
	"os"
	"os/signal"

	"github.com/devplayer0/docker-net-dhcp/pkg/plugin"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var (
	logLevel = flag.String("log", "info", "log level")
	logFile  = flag.String("logfile", "", "log file")
	bindSock = flag.String("sock", "/run/docker/plugins/net-dhcp.sock", "bind unix socket")
)

func main() {
	flag.Parse()

	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse log level")
	}
	log.SetLevel(level)

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.WithError(err).Fatal("Failed to open log file for writing")
			return
		}
		defer f.Close()

		log.StandardLogger().Out = f
	}

	p := plugin.NewPlugin()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, unix.SIGINT, unix.SIGTERM)

	go func() {
		log.Info("Starting server...")
		if err := p.Start(*bindSock); err != nil {
			log.WithError(err).Fatal("Failed to start server")
		}
	}()

	<-sigs
	log.Info("Shutting down...")
	p.Stop()
}
