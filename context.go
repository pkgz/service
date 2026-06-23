package service

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// ContextWithCancel - creates a context that is canceled on the first SIGINT/SIGTERM
// or when the returned CancelFunc is called. The interrupt is logged when it arrives.
func ContextWithCancel() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)

		select {
		case <-stop:
			log.Print("[INFO] interrupt signal")
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
