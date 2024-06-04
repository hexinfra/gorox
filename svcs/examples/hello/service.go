// This is a hello service showing how to use Gorox RPC Framework to host a service.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register service initializer.
	RegisterServiceInit("hello", func(service *Service) error {
		return nil
	})
}
