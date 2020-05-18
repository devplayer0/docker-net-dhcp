package plugin

import (
	"net"
	"net/http"
)

// Plugin is the DHCP network plugin
type Plugin struct {
	server http.Server
}

// NewPlugin creates a new Plugin
func NewPlugin() *Plugin {
	p := Plugin{}

	mux := http.NewServeMux()
	mux.HandleFunc("/NetworkDriver.GetCapabilities", apiGetCapabilities)

	p.server = http.Server{
		Handler: mux,
	}

	return &p
}

// Start starts the plugin server
func (p *Plugin) Start(bindSock string) error {
	l, err := net.Listen("unix", bindSock)
	if err != nil {
		return err
	}

	return p.server.Serve(l)
}

// Stop stops the plugin server
func (p *Plugin) Stop() error {
	return p.server.Close()
}
