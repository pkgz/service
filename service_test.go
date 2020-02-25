package service

import (
	"bytes"
	"github.com/pkgz/logg"
	"github.com/stretchr/testify/require"
	"log"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestParseEnv(t *testing.T) {
	var args interface{}
	require.Error(t, ParseEnv(args))

	os.Args = []string{""}
	require.NoError(t, ParseEnv(args))

	var t2 struct {
		Name string `long:"name" env:"NAME" default:"undefined" description:"service name"`
	}
	require.NoError(t, ParseEnv(&t2))
	require.Equal(t, "undefined", t2.Name)

	require.NoError(t, os.Setenv("NAME", "env-test"))
	require.NoError(t, ParseEnv(&t2))
	require.Equal(t, "env-test", t2.Name)

	os.Args = []string{"", "--name=args-test"}
	require.NoError(t, ParseEnv(&t2))
	require.Equal(t, "args-test", t2.Name)
}

func TestContextWithCancel(t *testing.T) {
	t.Run("sigkill", func(t *testing.T) {
		ctx, cancel := ContextWithCancel()
		require.NotNil(t, ctx)
		require.NotNil(t, cancel)
		done := make(chan bool, 1)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			<-ctx.Done()
			done <- true
			wg.Done()
		}()

		go func() {
			time.Sleep(1 * time.Millisecond)
			err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
			require.Nil(t, err)

			wg.Done()
		}()

		wg.Wait()

		require.Equal(t, 1, len(done))
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := ContextWithCancel()
		require.NotNil(t, ctx)
		require.NotNil(t, cancel)
		done := make(chan bool, 1)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			<-ctx.Done()
			done <- true
			wg.Done()
		}()

		go func() {
			cancel()
			wg.Done()
		}()

		wg.Wait()

		require.Equal(t, 1, len(done))
	})
}

func TestInit(t *testing.T) {
	t.Run("wrong args", func(t *testing.T) {
		var args interface{}
		_, _, err := Init(args, "")
		require.Error(t, err)
	})

	t.Run("empty args", func(t *testing.T) {
		os.Args = []string{""}
		var args struct{}
		_, _, err := Init(&args, "")
		log.Print(err)
		require.Error(t, err)
	})

	t.Run("no helper.ARGS", func(t *testing.T) {
		os.Args = []string{""}
		var args struct {
			Test bool `long:"test" env:"TEST" description:"test env variable"`
		}
		_, _, err := Init(&args, "")
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		os.Args = []string{""}
		var args struct {
			ARGS
			Test bool `long:"test" env:"TEST" description:"test env variable"`
		}
		tracer, closer, err := Init(&args, "")
		require.NoError(t, err)
		require.NotNil(t, tracer)
		require.NotNil(t, closer)

		buf := new(bytes.Buffer)
		logg.SetWriter(buf)
		log.Print("[DEBUG] LoL")
		require.Empty(t, buf)
	})

	t.Run("debug mode", func(t *testing.T) {
		os.Args = []string{"", "--debug"}
		var args struct {
			ARGS
			Test bool `long:"test" env:"TEST" description:"test env variable"`
		}
		tracer, closer, err := Init(&args, "")
		require.NoError(t, err)
		require.NotNil(t, tracer)
		require.NotNil(t, closer)

		buf := new(bytes.Buffer)
		logg.SetWriter(buf)
		log.Print("[DEBUG] LoL")
		require.NotEmpty(t, buf)
	})

	t.Run("open tracing error", func(t *testing.T) {
		os.Args = []string{"", "--open_tracing_host=test"}
		var args struct {
			ARGS
			Test bool `long:"test" env:"TEST" description:"test env variable"`
		}
		_, _, err := Init(&args, "")
		require.Error(t, err)
	})
}
