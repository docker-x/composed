package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "composed",
	Short: "Compose anything into a Docker Compose file",
	Long: `composed turns Helm charts, Kubernetes manifests, plain images,
and existing compose files into a single docker-compose.yaml.

Run any stack on plain Docker — no Kubernetes cluster required.`,
}

// SetVersion configures the version info shown by --version.
func SetVersion(version, commit, date string) {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
