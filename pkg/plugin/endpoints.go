package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// ParseJSONBody attempts to parse the request body as JSON
func ParseJSONBody(v interface{}, w http.ResponseWriter, r *http.Request) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
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

	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		log.WithField("err", err).Error("Failed to serialize JSON payload")

		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Failed to serialize JSON payload")
	}
}

type jsonError struct {
	Message string `json:"message"`
}

// JSONErrResponse Sends an `error` as a JSON object with a `message` property
func JSONErrResponse(w http.ResponseWriter, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	enc := json.NewEncoder(w)
	enc.Encode(jsonError{err.Error()})
}

// CapabilitiesResponse returns whether or not this network is global or local
type CapabilitiesResponse struct {
	Scope             string
	ConnectivityScope string
}

func apiGetCapabilities(w http.ResponseWriter, r *http.Request) {
	JSONResponse(w, CapabilitiesResponse{
		Scope:             "local",
		ConnectivityScope: "global",
	}, http.StatusOK)
}
