package service

import "github.com/jessevdk/go-flags"

// ParseEnv - parsing environment arguments. Expect pointer to struct.
func ParseEnv(args interface{}) error {
	p := flags.NewParser(args, flags.Default)

	if _, err := p.Parse(); err != nil {
		return err
	}

	return nil
}