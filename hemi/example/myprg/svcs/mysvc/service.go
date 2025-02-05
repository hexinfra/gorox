package mysvc

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServiceInit("mysvc", func(service *Service) error {
		return nil
	})
}
