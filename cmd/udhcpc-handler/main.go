package main

import (
	"encoding/json"
	"net"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/devplayer0/docker-net-dhcp/pkg/udhcpc"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %v <event type>", os.Args[0])
		return
	}

	event := udhcpc.Event{
		Type: os.Args[1],
	}

	switch event.Type {
	case "bound", "renew":
		if v6, ok := os.LookupEnv("ipv6"); ok {
			// Clean up the IP (udhcpc6 emits a _lot_ of zeros)
			_, netV6, err := net.ParseCIDR(v6 + "/128")
			if err != nil {
				log.WithError(err).Warn("Failed to parse IPv6 address")
			}

			event.Data.IP = netV6.String()
		} else {
			event.Data.IP = os.Getenv("ip") + "/" + os.Getenv("mask")
			event.Data.Gateway = os.Getenv("router")
			event.Data.Domain = os.Getenv("domain")
		}
	case "deconfig", "leasefail", "nak":
		log.Debugf("Ignoring `%v` event", event.Type)
		return
	default:
		log.Warnf("Ignoring unknown event type `%v`", event.Type)
		return
	}

	if err := json.NewEncoder(os.Stdout).Encode(event); err != nil {
		log.Fatalf("Failed to encode udhcpc event: %w", err)
		return
	}
}
