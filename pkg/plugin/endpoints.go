package plugin

import (
	"net/http"

	"github.com/devplayer0/docker-net-dhcp/pkg/util"
)

// Payloads are based on https://github.com/docker/go-plugins-helpers/blob/master/network/api.go

// CapabilitiesResponse returns whether or not this network is global or local
type CapabilitiesResponse struct {
	Scope             string
	ConnectivityScope string
}

func (p *Plugin) apiGetCapabilities(w http.ResponseWriter, r *http.Request) {
	util.JSONResponse(w, CapabilitiesResponse{
		Scope:             "local",
		ConnectivityScope: "global",
	}, http.StatusOK)
}

// IPAMData contains IPv4 or IPv6 addressing information
type IPAMData struct {
	AddressSpace string
	Pool         string
	Gateway      string
	AuxAddresses map[string]interface{}
}

// CreateNetworkRequest is sent by the daemon when a network needs to be created
type CreateNetworkRequest struct {
	NetworkID string
	Options   map[string]interface{}
	IPv4Data  []*IPAMData
	IPv6Data  []*IPAMData
}

func (p *Plugin) apiCreateNetwork(w http.ResponseWriter, r *http.Request) {
	var req CreateNetworkRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	if err := p.CreateNetwork(req); err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, struct{}{}, http.StatusOK)
}

// DeleteNetworkRequest is sent by the daemon when a network needs to be removed
type DeleteNetworkRequest struct {
	NetworkID string
}

func (p *Plugin) apiDeleteNetwork(w http.ResponseWriter, r *http.Request) {
	var req DeleteNetworkRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	if err := p.DeleteNetwork(req); err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, struct{}{}, http.StatusOK)
}

// EndpointInterface contains endpoint interface information
type EndpointInterface struct {
	Address     string
	AddressIPv6 string
	MacAddress  string
}

// CreateEndpointRequest is sent by the daemon when an endpoint should be created
type CreateEndpointRequest struct {
	NetworkID  string
	EndpointID string
	Interface  *EndpointInterface
	Options    map[string]interface{}
}

// CreateEndpointResponse is sent as a response to a CreateEndpointRequest
type CreateEndpointResponse struct {
	Interface *EndpointInterface
}

func (p *Plugin) apiCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req CreateEndpointRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	res, err := p.CreateEndpoint(r.Context(), req)
	if err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, res, http.StatusOK)
}

// InfoRequest is sent by the daemon when querying endpoint information
type InfoRequest struct {
	NetworkID  string
	EndpointID string
}

// InfoResponse is endpoint information sent in response to an InfoRequest
type InfoResponse struct {
	Value map[string]string
}

func (p *Plugin) apiEndpointOperInfo(w http.ResponseWriter, r *http.Request) {
	var req InfoRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	res, err := p.EndpointOperInfo(r.Context(), req)
	if err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, res, http.StatusOK)
}

// DeleteEndpointRequest is sent by the daemon when an endpoint needs to be removed
type DeleteEndpointRequest struct {
	NetworkID  string
	EndpointID string
}

func (p *Plugin) apiDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	var req DeleteEndpointRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	if err := p.DeleteEndpoint(req); err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, struct{}{}, http.StatusOK)
}

// JoinRequest is sent by the Daemon when an endpoint needs be joined to a network
type JoinRequest struct {
	NetworkID  string
	EndpointID string
	SandboxKey string
	Options    map[string]interface{}
}

// InterfaceName consists of the name of the interface in the global netns and
// the desired prefix to be appended to the interface inside the container netns
type InterfaceName struct {
	SrcName   string
	DstPrefix string
}

// StaticRoute contains static route information
type StaticRoute struct {
	Destination string
	RouteType   int
	NextHop     string
}

// JoinResponse is sent in response to a JoinRequest
type JoinResponse struct {
	InterfaceName         InterfaceName
	Gateway               string
	GatewayIPv6           string
	StaticRoutes          []*StaticRoute
	DisableGatewayService bool
}

func (p *Plugin) apiJoin(w http.ResponseWriter, r *http.Request) {
	var req JoinRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	res, err := p.Join(r.Context(), req)
	if err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, res, http.StatusOK)
}

// LeaveRequest is sent by the daemon when a endpoint is leaving a network
type LeaveRequest struct {
	NetworkID  string
	EndpointID string
}

func (p *Plugin) apiLeave(w http.ResponseWriter, r *http.Request) {
	var req LeaveRequest
	if err := util.ParseJSONBody(&req, w, r); err != nil {
		return
	}

	if err := p.Leave(r.Context(), req); err != nil {
		util.JSONErrResponse(w, err, 0)
		return
	}

	util.JSONResponse(w, struct{}{}, http.StatusOK)
}
