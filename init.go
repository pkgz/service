package service

import (
	"context"
	"errors"
	"github.com/pkgz/logg"
	"os"
	"reflect"
)

// ARGS - default argument for application
type ARGS struct {
	Port  int  `long:"port" env:"PORT" default:"8080" description:"service rest port"`
	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

var (
	ErrMissingArgs = errors.New("helper.ARGS not find in args")
)

// Init - allows easily initialize app. Will parse environment arguments, and will initialize application context with cancel.
func Init(args interface{}) (context.Context, context.CancelFunc, error) {
	if err := ParseEnv(args); err != nil {
		return nil, nil, err
	}

	ctx, cancel := ContextWithCancel()

	s := reflect.ValueOf(args)
	value := s.Elem()
	argsValue := value.FieldByName("ARGS")
	if !argsValue.IsValid() {
		return nil, nil, ErrMissingArgs
	}

	var debug = argsValue.FieldByName("Debug").Bool()

	logg.NewGlobal(os.Stdout)
	if debug {
		logg.DebugMode()
	}

	return ctx, cancel, nil
}
