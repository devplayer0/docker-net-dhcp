package util

import (
	"context"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

const (
	OptionsKeyGeneric = "com.docker.network.generic"
)

func AwaitContainerInspect(ctx context.Context, docker *client.Client, id string, interval time.Duration) (types.ContainerJSON, error) {
	var err error
	ctrChan := make(chan types.ContainerJSON)
	go func() {
		for {
			var ctr types.ContainerJSON
			ctr, err = docker.ContainerInspect(ctx, id)
			if err == nil {
				ctrChan <- ctr
				return
			}

			time.Sleep(interval)
		}
	}()

	var dummy types.ContainerJSON
	select {
	case link := <-ctrChan:
		return link, nil
	case <-ctx.Done():
		if err != nil {
			log.WithError(err).WithField("id", id).Error("Failed to await container by ID")
		}
		return dummy, ctx.Err()
	}
}
