package util

import (
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
)

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

type jsonError struct {
	Message string `json:"Err"`
}

// JSONErrResponse Sends an `error` as a JSON object with a `message` property
func JSONErrResponse(w http.ResponseWriter, err error, statusCode int) {
	log.WithError(err).Error("Error while processing request")

	w.Header().Set("Content-Type", "application/problem+json")
	if statusCode == 0 {
		statusCode = ErrToStatus(err)
	}
	w.WriteHeader(statusCode)

	enc := json.NewEncoder(w)
	enc.Encode(jsonError{err.Error()})
}

// ParseJSONBody attempts to parse the request body as JSON
func ParseJSONBody(v interface{}, w http.ResponseWriter, r *http.Request) error {
	//data, err := io.ReadAll(r.Body)
	//if err != nil {
	//	JSONErrResponse(w, fmt.Errorf("failed to read request body: %w", err), 0)
	//	return err
	//}

	//log.WithField("body", string(data)).Debug("request body")

	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		//if err := json.Unmarshal(data, v); err != nil {
		JSONErrResponse(w, fmt.Errorf("failed to parse request body: %w", err), http.StatusBadRequest)
		return err
	}

	return nil
}
