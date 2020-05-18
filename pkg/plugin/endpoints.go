package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// ParseJSONBody attempts to parse the request body as JSON
func ParseJSONBody(v interface{}, w http.ResponseWriter, r *http.Request) error {
	d := json.NewDecoder(r.Body)
	if err := d.Decode(v); err != nil {
		JSONErrResponse(w, fmt.Errorf("failed to parse request body: %w", err), http.StatusBadRequest)
		return err
	}

	return nil
}

// JSONResponse Sends a JSON payload in response to a HTTP request
func JSONResponse(w http.ResponseWriter, v interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.WithField("err", err).Error("Failed to serialize JSON payload")

		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Failed to serialize JSON payload")
	}
}

// ErrorResponse is a formatted error message that libnetwork can understand
type ErrorResponse struct {
	Err string
}

// JSONErrResponse Sends an `error` as a JSON object with a `message` property
func JSONErrResponse(w http.ResponseWriter, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	json.NewEncoder(w).Encode(ErrorResponse{err.Error()})
}

func errToStatus(err error) int {
	switch {
	case errors.Is(err, ErrIPAM), errors.Is(err, ErrBridge):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// CapabilitiesResponse returns whether or not this network is global or local
type CapabilitiesResponse struct {
	Scope             string
	ConnectivityScope string
}

func (p *Plugin) apiGetCapabilities(w http.ResponseWriter, r *http.Request) {
	JSONResponse(w, CapabilitiesResponse{
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

// CreateNetworkGenericOptions contains options for the DHCP network driver
type CreateNetworkGenericOptions struct {
	Bridge string
	IPv6   bool
}

// CreateNetworkOptions contains a map of options for the network driver
type CreateNetworkOptions struct {
	Generic CreateNetworkGenericOptions `json:"com.docker.network.generic"`
}

// CreateNetworkRequest is sent by the daemon when a network needs to be created
type CreateNetworkRequest struct {
	NetworkID string
	Options   CreateNetworkOptions
	IPv4Data  []*IPAMData
	IPv6Data  []*IPAMData
}

func (p *Plugin) apiCreateNetwork(w http.ResponseWriter, r *http.Request) {
	var req CreateNetworkRequest
	if err := ParseJSONBody(&req, w, r); err != nil {
		return
	}

	if err := p.CreateNetwork(req); err != nil {
		JSONErrResponse(w, err, errToStatus(err))
		return
	}

	JSONResponse(w, struct{}{}, http.StatusOK)
}

// DeleteNetworkRequest is sent by the daemon when a network needs to be removed
type DeleteNetworkRequest struct {
	NetworkID string
}

func (p *Plugin) apiDeleteNetwork(w http.ResponseWriter, r *http.Request) {
	var req DeleteNetworkRequest
	if err := ParseJSONBody(&req, w, r); err != nil {
		return
	}

	if err := p.DeleteNetwork(); err != nil {
		JSONErrResponse(w, err, errToStatus(err))
		return
	}

	JSONResponse(w, struct{}{}, http.StatusOK)
}
