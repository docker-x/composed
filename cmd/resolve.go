package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultConfigName = "composed.yaml"

// findConfig walks up the directory tree from cwd looking for composed.yaml.
// Returns the path if found, or the default name (current dir) if not.
func findConfig() string {
	dir, err := os.Getwd()
	if err != nil {
		return defaultConfigName
	}
	for {
		candidate := filepath.Join(dir, defaultConfigName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return defaultConfigName
}

// resolveConfig returns the config path: if -f was explicitly passed, use it
// as-is. Otherwise walk up the directory tree to find composed.yaml.
func resolveConfig(cmd *cobra.Command, flagVal string) string {
	if cmd != nil && cmd.Flags().Changed("file") {
		return flagVal
	}
	return findConfig()
}
