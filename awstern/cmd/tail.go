// Copyright 2025 The awstools Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/zchee/awstools/awstern/internal/tail"
	"github.com/zchee/awstools/pkg/awsconfig"
)

var (
	// Flags
	flagSince        string
	flagTail         int
	flagTimestamps   bool
	flagColor        string
	flagOutput       string
	flagAllLogGroups bool
	flagExclude      string
	flagMaxLogGroups int
	flagRegion       string
	flagPollInterval string
)

// NewTailCmd creates the tail command.
func NewTailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tail [log-group-regex]",
		Short: "Tail CloudWatch logs from log groups matching a pattern",
		Long: `Tail CloudWatch logs from log groups matching a regular expression pattern.

Examples:
  # Tail all log groups
  awstern tail --all-log-groups

  # Tail log groups matching "ecs"
  awstern tail ecs

  # Tail log groups matching regex pattern
  awstern tail "^/aws/ecs/.*"

  # Tail with custom time window
  awstern tail --since 1h ecs

  # Tail in JSON format
  awstern tail --output json ecs

  # Exclude specific log groups
  awstern tail --exclude "test" ecs`,
		Args: cobra.MaximumNArgs(1),
		RunE: runTail,
	}

	// Add flags
	cmd.Flags().StringVarP(&flagSince, "since", "s", "5m", "Return logs newer than a relative duration (e.g., 5s, 2m, 3h)")
	cmd.Flags().IntVar(&flagTail, "tail", 0, "Number of lines to show from the end of logs (0 = all)")
	cmd.Flags().BoolVarP(&flagTimestamps, "timestamps", "t", true, "Print timestamps")
	cmd.Flags().StringVar(&flagColor, "color", "auto", "Color output (auto/always/never)")
	cmd.Flags().StringVarP(&flagOutput, "output", "o", "default", "Output format (default/json/raw)")
	cmd.Flags().BoolVarP(&flagAllLogGroups, "all-log-groups", "A", false, "Tail from all log groups")
	cmd.Flags().StringVar(&flagExclude, "exclude", "", "Regular expression to exclude log groups")
	cmd.Flags().IntVar(&flagMaxLogGroups, "max-log-groups", 50, "Maximum number of log groups to tail")
	cmd.Flags().StringVar(&flagRegion, "region", "", "AWS region (overrides default)")
	cmd.Flags().StringVar(&flagPollInterval, "poll-interval", "1s", "Interval to poll for new logs")

	return cmd
}

func runTail(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse configuration
	config, err := parseConfig(args)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Load AWS configuration
	awsCfg, err := awsconfig.LoadConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	// Create and start coordinator
	coordinator := tail.NewCoordinator(awsCfg, config)
	if err := coordinator.Start(ctx); err != nil {
		return fmt.Errorf("tail logs: %w", err)
	}

	return nil
}

func parseConfig(args []string) (*tail.Config, error) {
	config := tail.DefaultConfig()

	// Parse log group pattern
	if len(args) == 0 && !flagAllLogGroups {
		return nil, fmt.Errorf("either provide a log group pattern or use --all-log-groups")
	}

	if len(args) > 0 {
		pattern := args[0]
		regex, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid log group regex pattern %q: %w", pattern, err)
		}
		config.LogGroupPattern = regex
	}

	// Parse exclude pattern
	if flagExclude != "" {
		regex, err := regexp.Compile(flagExclude)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", flagExclude, err)
		}
		config.ExcludePattern = regex
	}

	// Parse since duration
	since, err := time.ParseDuration(flagSince)
	if err != nil {
		return nil, fmt.Errorf("invalid since duration %q: %w", flagSince, err)
	}
	config.Since = since

	// Parse poll interval
	pollInterval, err := time.ParseDuration(flagPollInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid poll interval %q: %w", flagPollInterval, err)
	}
	config.PollInterval = pollInterval

	// Set other configuration
	config.Tail = flagTail
	config.Timestamps = flagTimestamps
	config.ColorMode = flagColor
	config.OutputFormat = flagOutput
	config.AllLogGroups = flagAllLogGroups
	config.MaxLogGroups = flagMaxLogGroups

	return config, nil
}
