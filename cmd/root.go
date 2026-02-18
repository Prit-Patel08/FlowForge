package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var verbose bool
var profileName string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "flowforge",
	Short: "A CLI supervisor for AI agents",
	Long: `FlowForge is a CLI tool designed to supervise and monitor subprocesses.
It captures stdout/stderr, monitors CPU usage, and handles graceful shutdowns.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./flowforge.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&profileName, "profile", "", "monitoring profile: light, standard, heavy (overrides config file)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("flowforge")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if cfgFile != "" {
			fmt.Printf("Failed to read config file %q: %v\n", cfgFile, err)
			os.Exit(1)
		}
	} else if verbose {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	// Resolve active profile
	resolveProfile()

	if err := validateConfig(); err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		os.Exit(1)
	}
}

// resolveProfile merges the active profile's settings into the top-level Viper keys.
func resolveProfile() {
	// CLI flag takes priority, then config file's "profile" key, then default "standard"
	active := profileName
	if active == "" {
		active = viper.GetString("profile")
	}
	if active == "" {
		active = "standard"
	}

	// Read profile-specific settings
	prefix := fmt.Sprintf("profiles.%s", active)
	if viper.IsSet(prefix) {
		// Only override if not already set by a CLI flag
		if !viper.IsSet("max-cpu") || viper.GetFloat64("max-cpu") == 0 {
			viper.SetDefault("max-cpu", viper.GetFloat64(prefix+".max-cpu"))
		} else {
			// Profile value is used as the base; CLI flags override later via Cobra
			profileCPU := viper.GetFloat64(prefix + ".max-cpu")
			if profileCPU > 0 {
				viper.Set("max-cpu", profileCPU)
			}
		}

		pollInterval := viper.GetInt(prefix + ".poll-interval")
		if pollInterval > 0 {
			viper.Set("poll-interval", pollInterval)
		}

		logWindow := viper.GetInt(prefix + ".log-window")
		if logWindow > 0 {
			viper.Set("log-window", logWindow)
		}

		if verbose {
			fmt.Printf("Active profile: %s (max-cpu=%.1f, poll-interval=%dms, log-window=%d)\n",
				active,
				viper.GetFloat64("max-cpu"),
				viper.GetInt("poll-interval"),
				viper.GetInt("log-window"),
			)
		}
	} else {
		// Fallback defaults if no profiles section exists
		viper.SetDefault("max-cpu", 60.0)
		viper.SetDefault("poll-interval", 500)
		viper.SetDefault("log-window", 10)
	}
}
