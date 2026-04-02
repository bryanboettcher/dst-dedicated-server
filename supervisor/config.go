package main

import (
	"bufio"
	"os"
	"strings"
)

// ShardConfig holds values parsed from the shard's server.ini.
type ShardConfig struct {
	IsMaster bool
}

// ParseShardConfig reads the server.ini at the given path and extracts
// shard-relevant fields. Missing file or fields are not errors — defaults apply.
func ParseShardConfig(path string) ShardConfig {
	var cfg ShardConfig

	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if key == "is_master" {
			cfg.IsMaster = val == "true"
		}
	}

	return cfg
}
