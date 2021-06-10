package util

import (
	"io"

	"github.com/gorilla/handlers"
	log "github.com/sirupsen/logrus"
)

func WriteAccessLog(w io.Writer, params handlers.LogFormatterParams) {
	log.WithFields(log.Fields{
		"status":  params.StatusCode,
		"resSize": params.Size,
	}).Tracef("%v %v", params.Request.Method, params.URL.RequestURI())
}
