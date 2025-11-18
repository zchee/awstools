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

package tail

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/zchee/awstools/awstern/internal/cloudwatch"
	"github.com/zchee/awstools/awstern/internal/output"
	"github.com/zchee/awstools/awstern/internal/types"
)

// Coordinator manages tailing multiple log groups concurrently.
type Coordinator struct {
	client    *cloudwatch.Client
	config    *Config
	formatter *output.Formatter
}

// NewCoordinator creates a new tail coordinator.
func NewCoordinator(cfg aws.Config, config *Config) *Coordinator {
	client := cloudwatch.NewClient(cfg)
	formatter := output.NewFormatter(config.OutputFormat, config.Timestamps, config.ColorMode)

	return &Coordinator{
		client:    client,
		config:    config,
		formatter: formatter,
	}
}

// Start begins tailing logs from all matching log groups.
func (c *Coordinator) Start(ctx context.Context) error {
	// Discover log groups
	logGroups, err := c.discoverLogGroups(ctx)
	if err != nil {
		return fmt.Errorf("discover log groups: %w", err)
	}

	if len(logGroups) == 0 {
		return fmt.Errorf("no log groups found matching pattern")
	}

	fmt.Printf("Tailing %d log group(s)...\n", len(logGroups))

	// Create a channel for log events
	events := make(chan types.LogEvent, 1000)

	// Start a goroutine to format and output events
	var outputWg sync.WaitGroup
	outputWg.Go(func() {
		for event := range events {
			if err := c.formatter.Format(event); err != nil {
				fmt.Fprintf(nil, "error formatting event: %v\n", err)
			}
		}
	})

	// Start tailers for each log group
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, logGroupName := range logGroups {
		wg.Go(func() {
			tailer := NewLogGroupTailer(c.client, logGroupName, c.config, events)
			if err := tailer.Start(ctx); err != nil && err != context.Canceled {
				fmt.Fprintf(nil, "error tailing %s: %v\n", logGroupName, err)
			}
		})
	}

	// Wait for all tailers to finish
	wg.Wait()

	// Close events channel and wait for output goroutine
	close(events)
	outputWg.Wait()

	return nil
}

// discoverLogGroups finds all log groups matching the configured pattern.
func (c *Coordinator) discoverLogGroups(ctx context.Context) ([]string, error) {
	// List all log groups (we'll filter by regex)
	allLogGroups, err := c.client.ListLogGroups(ctx, "")
	if err != nil {
		return nil, err
	}

	var matchingGroups []string
	for _, lg := range allLogGroups {
		logGroupName := aws.ToString(lg.LogGroupName)

		// Apply include pattern
		if !c.config.LogGroupPattern.MatchString(logGroupName) {
			continue
		}

		// Apply exclude pattern if set
		if c.config.ExcludePattern != nil && c.config.ExcludePattern.MatchString(logGroupName) {
			continue
		}

		matchingGroups = append(matchingGroups, logGroupName)

		// Respect max log groups limit
		if len(matchingGroups) >= c.config.MaxLogGroups {
			break
		}
	}

	return matchingGroups, nil
}
