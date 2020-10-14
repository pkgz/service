package service

import (
	"context"
	"github.com/pkg/errors"
	"github.com/pkgz/logg"
	"os"
	"reflect"
)

// ARGS - default argument for application
type ARGS struct {
	Port  int  `long:"port" env:"PORT" default:"8080" description:"service rest port"`
	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

// Init - allows easily initialize app. Will parse environment arguments, and will initialize application context with cancel.
func Init(args interface{}) (context.Context, context.CancelFunc, error) {
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

	var debug = argsValue.FieldByName("Debug").Bool()

	logg.NewGlobal(os.Stdout)
	if debug {
		logg.DebugMode()
	}

	return ctx, cancel, nil
}
