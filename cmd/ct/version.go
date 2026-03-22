package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// Set via -ldflags at build time.
var version = "dev"
var commit = "unknown"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the ct version",
	Run: func(cmd *cobra.Command, args []string) {
		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			out, _ := json.Marshal(map[string]string{"version": version, "commit": commit})
			fmt.Println(string(out))
			return
		}
		fmt.Println("ct", version)
	},
}

func init() {
	versionCmd.Flags().Bool("json", false, "Output version info as JSON")
	rootCmd.AddCommand(versionCmd)
}
