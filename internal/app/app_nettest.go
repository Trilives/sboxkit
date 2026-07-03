package app

import (
	"context"
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/nettest"
)

func runNettest(stdout io.Writer, proxy string) {
	results := nettest.Run(context.Background(), nil, proxy)
	fmt.Fprint(stdout, nettest.Format(results))
}
