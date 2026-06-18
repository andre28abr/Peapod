// Package backend selects a sandbox driver by name.
package backend

import (
	"fmt"

	"peapod/internal/driver/applecontainer"
	"peapod/internal/driver/mock"
	"peapod/internal/driver/oci"
	"peapod/internal/sandbox"
)

// New returns the driver for the given backend name:
//   - "" / "oci"  -> docker/podman (container in a shared VM)
//   - "apple"     -> apple `container` (one microVM per sandbox, macOS 26+)
//   - "mock"      -> in-memory (tests / daemon-less dev)
func New(name string) (sandbox.Driver, error) {
	switch name {
	case "", "oci":
		return oci.New()
	case "apple", "apple-container", "applecontainer":
		return applecontainer.New()
	case "mock":
		return mock.New(), nil
	default:
		return nil, fmt.Errorf("unknown backend %q (use: oci, apple, mock)", name)
	}
}
