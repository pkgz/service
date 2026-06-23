package service

import (
	"context"
	"os"
	"reflect"

	"github.com/pkgz/logg"
)

// ARGS - default argument for application
type ARGS struct {
	Port    int      `long:"port" env:"PORT" default:"8080" description:"service rest port"`
	Origins []string `long:"origins" env:"ORIGINS" env-delim:"," description:"service rest origins separated by ,"`
	ENV     string   `long:"env" env:"ENV" default:"local" description:"service env"`

	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

// Init - allows easily initialize app. Will parse environment arguments, and will initialize application context with cancel.
func Init(args any) (context.Context, context.CancelFunc, error) {
	if err := ParseEnv(args); err != nil {
		return nil, nil, err
	}

	ctx, cancel := ContextWithCancel()

	logg.NewGlobal(os.Stdout)

	if value := reflect.ValueOf(args).Elem(); value.Kind() == reflect.Struct {
		if debug := value.FieldByName("Debug"); debug.IsValid() && debug.Kind() == reflect.Bool && debug.Bool() {
			logg.DebugMode()
		}
	}

	return ctx, cancel, nil
}
