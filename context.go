package service

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// ContextWithCancel - creating context with cancel. Also start goroutine which waiting for SIGTERM signal and closing
// the context.
func ContextWithCancel() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Print("[INFO] interrupt signal")
		cancel()
	}()

	return ctx, cancel
}