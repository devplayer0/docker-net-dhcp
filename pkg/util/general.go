package util

import (
	"context"
	"time"
)

func AwaitCondition(ctx context.Context, cond func() (bool, error), interval time.Duration) error {
	errChan := make(chan error)
	go func() {
		for {
			ok, err := cond()
			if err != nil {
				errChan <- err
				return
			}

			if ok {
				errChan <- nil
				return
			}

			time.Sleep(interval)
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
