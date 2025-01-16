package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServiceInit("hello", func(service *Service) error {
		return nil
	})
}
