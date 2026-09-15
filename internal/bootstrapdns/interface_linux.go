//go:build linux

package bootstrapdns

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var ErrNoPhysicalInterface = errors.New("no usable physical default interface")

// DiscoverDefaultInterface reads the kernel route table without invoking a
// shell. The lowest-metric active default route not explicitly excluded wins.
func DiscoverDefaultInterface(ctx context.Context, excluded []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", fmt.Errorf("read IPv4 routes: %w", err)
	}
	defer file.Close()
	return discoverDefaultInterfaceFromReader(ctx, file, excluded)
}

func discoverDefaultInterfaceFromReader(ctx context.Context, reader io.Reader, excluded []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		name = strings.TrimSpace(name)
		if name != "" {
			excludedSet[name] = struct{}{}
		}
	}
	bestName := ""
	bestMetric := int64(^uint64(0) >> 1)
	scanner := bufio.NewScanner(reader)
	if scanner.Scan() { // header
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			fields := strings.Fields(scanner.Text())
			if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
				continue
			}
			name := fields[0]
			if _, skip := excludedSet[name]; skip || name == "lo" {
				continue
			}
			flags, flagsErr := strconv.ParseUint(fields[3], 16, 32)
			metric, metricErr := strconv.ParseInt(fields[6], 10, 64)
			if flagsErr != nil || metricErr != nil || flags&1 == 0 || metric >= bestMetric {
				continue
			}
			bestName, bestMetric = name, metric
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan IPv4 routes: %w", err)
	}
	if bestName == "" {
		return "", ErrNoPhysicalInterface
	}
	return bestName, nil
}
