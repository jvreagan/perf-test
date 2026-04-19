package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jvreagan/perf-test/internal/config"
	"github.com/jvreagan/perf-test/internal/engine"
)

var version = "0.3.0"

func main() {
	root := &cobra.Command{
		Use:   "perf-test",
		Short: "A config-driven HTTP API performance testing tool",
		Long: `perf-test is an open-source CLI tool for load testing HTTP APIs.
It supports weighted multi-endpoint tests, stage-based load ramp profiles,
data templating, and periodic stats output.`,
	}

	root.AddCommand(runCmd(), validateCmd(), versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run [config.yaml]",
		Short: "Run a load test",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "perf-test.yaml"
			if len(args) > 0 {
				path = args[0]
			}

			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			fmt.Printf("Starting load test: %s\n", cfg.Name)
			if cfg.Description != "" {
				fmt.Printf("  %s\n", cfg.Description)
			}
			fmt.Printf("  Duration: %s  Endpoints: %d\n\n", cfg.TotalDuration(), len(cfg.Endpoints))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle OS signals for graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				signal.Stop(sigCh)
				fmt.Println("\nShutting down gracefully...")
				cancel()
			}()

			eng := engine.New(cfg)
			if _, err := eng.Run(ctx, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Test completed with failures: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [config.yaml]",
		Short: "Validate a config file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "perf-test.yaml"
			if len(args) > 0 {
				path = args[0]
			}

			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("config is invalid: %w", err)
			}

			fmt.Printf("Config is valid!\n")
			fmt.Printf("  Name:      %s\n", cfg.Name)
			if cfg.Description != "" {
				fmt.Printf("  Desc:      %s\n", cfg.Description)
			}
			fmt.Printf("  Duration:  %s\n", cfg.TotalDuration())

			// Load config
			mode := cfg.Load.Mode
			if mode == "" {
				mode = "vu"
			}
			fmt.Printf("  Mode:      %s\n", mode)
			if cfg.Load.ThinkTime.Duration > 0 {
				fmt.Printf("  Think Time: %s\n", cfg.Load.ThinkTime.Duration)
			}
			if cfg.Load.MaxRPS > 0 {
				fmt.Printf("  Max RPS:   %.1f\n", cfg.Load.MaxRPS)
			}

			// Stages
			fmt.Printf("  Stages:    %d\n", len(cfg.Load.Stages))
			for i, s := range cfg.Load.Stages {
				ramp := s.Ramp
				if ramp == "" {
					ramp = "linear"
				}
				fmt.Printf("    %d) %s → target %d (%s)\n", i+1, s.Duration.Duration, s.Target, ramp)
			}

			// HTTP
			fmt.Printf("  Timeout:   %s\n", cfg.HTTP.Timeout.Duration)
			if cfg.HTTP.InsecureSkipVerify {
				fmt.Printf("  TLS:       insecure (skip verify)\n")
			}

			// Endpoints
			fmt.Printf("  Endpoints: %d\n", len(cfg.Endpoints))
			for _, ep := range cfg.Endpoints {
				fmt.Printf("    - [weight:%d] %s %s\n", ep.Weight, ep.Method, ep.URL)
				if ep.Expect.Status > 0 {
					fmt.Printf("      expect: status %d\n", ep.Expect.Status)
				}
			}

			// Thresholds
			tc := cfg.Thresholds
			if tc.P95.Duration > 0 || tc.P99.Duration > 0 || tc.MaxLatency.Duration > 0 || tc.ErrorRate > 0 || tc.MinRPS > 0 {
				fmt.Printf("  Thresholds:\n")
				if tc.P95.Duration > 0 {
					fmt.Printf("    p95 <= %s\n", tc.P95.Duration)
				}
				if tc.P99.Duration > 0 {
					fmt.Printf("    p99 <= %s\n", tc.P99.Duration)
				}
				if tc.MaxLatency.Duration > 0 {
					fmt.Printf("    max_latency <= %s\n", tc.MaxLatency.Duration)
				}
				if tc.ErrorRate > 0 {
					fmt.Printf("    error_rate <= %.1f%%\n", tc.ErrorRate)
				}
				if tc.MinRPS > 0 {
					fmt.Printf("    min_rps >= %.1f\n", tc.MinRPS)
				}
			}

			// Output
			if cfg.Output.File != "" {
				fmt.Printf("  Output:    %s (%s)\n", cfg.Output.File, cfg.Output.Format)
			}
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("perf-test version %s\n", version)
		},
	}
}
