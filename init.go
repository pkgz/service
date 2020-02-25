package service

import (
	"context"
	"github.com/pkg/errors"
	"github.com/pkgz/OpenTracing"
	"github.com/pkgz/logg"
	"log"
	"os"
	"reflect"
)

// ARGS - default argument for application
type ARGS struct {
	OpenTracingHost string `long:"open_tracing_host" env:"OPEN_TRACING_HOST" default:"localhost" description:"opentracing host"`

	Port  int  `long:"port" env:"PORT" default:"8080" description:"service rest port"`
	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

// Init - allows easily initialize app. Will parse environment arguments, set up jager tracer, and will initialize
// application context with cancel.
func Init(args interface{}, name string) (context.Context, context.CancelFunc, error) {
	err := ParseEnv(args)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse env error")
	}

	ctx, cancel := ContextWithCancel()

	s := reflect.ValueOf(args)
	value := s.Elem()
	argsValue := value.FieldByName("ARGS")
	if !argsValue.IsValid() {
		return nil, nil, errors.New("helper.ARGS not find in args")
	}

	var host = argsValue.FieldByName("OpenTracingHost").String()
	var debug = argsValue.FieldByName("Debug").Bool()

	logg.NewGlobal(os.Stdout)
	if debug {
		logg.DebugMode()
	}

	_, closer, err := OpenTracing.NewTracer(name, host)
	if err != nil {
		return nil, nil, errors.Wrap(err, "setup opentracing failed")
	}

	go func() {
		<-ctx.Done()
		if err = closer.Close(); err != nil {
			log.Printf("[WARN] tracer closer: %v", err)
		}
	}()

	return ctx, cancel, nil
}
