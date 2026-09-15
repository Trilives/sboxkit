//go:build !linux

package bootstrapdns

import (
	"context"
	"errors"
)

var ErrNoPhysicalInterface = errors.New("physical interface discovery is only supported on Linux")

func DiscoverDefaultInterface(context.Context, []string) (string, error) {
	return "", ErrNoPhysicalInterface
}
