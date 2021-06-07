package plugin

import (
	"fmt"
	"net"
	"net/http"
	"time"

	docker "github.com/docker/docker/client"
	"github.com/gorilla/handlers"
	"github.com/mitchellh/mapstructure"

	"github.com/devplayer0/docker-net-dhcp/pkg/util"
)

// DriverName is the name of the Docker Network Driver
const DriverName string = "net-dhcp"

const defaultLeaseTimeout = 10 * time.Second

// DHCPNetworkOptions contains options for the DHCP network driver
type DHCPNetworkOptions struct {
	Bridge       string
	IPv6         bool
	LeaseTimeout time.Duration `mapstructure:"lease_timeout"`
}

func decodeOpts(input interface{}) (DHCPNetworkOptions, error) {
	var opts DHCPNetworkOptions
	optsDecoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      &opts,
		ErrorUnused: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
	})
	if err != nil {
		return opts, fmt.Errorf("failed to create options decoder: %w", err)
	}

	if err := optsDecoder.Decode(input); err != nil {
		return opts, err
	}

	return opts, nil
}

type gatewayHint struct {
	V4 string
	V6 string
}

// Plugin is the DHCP network plugin
type Plugin struct {
	docker *docker.Client
	server http.Server

	gatewayHints map[string]gatewayHint
}

// NewPlugin creates a new Plugin
func NewPlugin() (*Plugin, error) {
	client, err := docker.NewClient("unix:///run/docker.sock", "v1.13.1", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	p := Plugin{
		docker: client,

		gatewayHints: make(map[string]gatewayHint),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/NetworkDriver.GetCapabilities", p.apiGetCapabilities)

	mux.HandleFunc("/NetworkDriver.CreateNetwork", p.apiCreateNetwork)
	mux.HandleFunc("/NetworkDriver.DeleteNetwork", p.apiDeleteNetwork)

	mux.HandleFunc("/NetworkDriver.CreateEndpoint", p.apiCreateEndpoint)
	mux.HandleFunc("/NetworkDriver.EndpointOperInfo", p.apiEndpointOperInfo)
	mux.HandleFunc("/NetworkDriver.DeleteEndpoint", p.apiDeleteEndpoint)

	mux.HandleFunc("/NetworkDriver.Join", p.apiJoin)
	mux.HandleFunc("/NetworkDriver.Leave", p.apiLeave)

	p.server = http.Server{
		Handler: handlers.CustomLoggingHandler(nil, mux, util.WriteAccessLog),
	}

	return &p, nil
}

// Listen starts the plugin server
func (p *Plugin) Listen(bindSock string) error {
	l, err := net.Listen("unix", bindSock)
	if err != nil {
		return err
	}

	return p.server.Serve(l)
}

// Close stops the plugin server
func (p *Plugin) Close() error {
	if err := p.docker.Close(); err != nil {
		return fmt.Errorf("failed to close docker client: %w", err)
	}

	if err := p.server.Close(); err != nil {
		return fmt.Errorf("failed to close http server: %w", err)
	}

	return nil
}
