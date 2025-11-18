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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/zchee/awstools/awstern/internal/cloudwatch"
	"github.com/zchee/awstools/awstern/internal/types"
)

// LogGroupTailer tails logs from a single log group.
type LogGroupTailer struct {
	client        *cloudwatch.Client
	logGroupName  string
	config        *Config
	events        chan<- types.LogEvent
	lastTimestamp *time.Time
}

// NewLogGroupTailer creates a new log group tailer.
func NewLogGroupTailer(client *cloudwatch.Client, logGroupName string, config *Config, events chan<- types.LogEvent) *LogGroupTailer {
	return &LogGroupTailer{
		client:       client,
		logGroupName: logGroupName,
		config:       config,
		events:       events,
	}
}

// Start begins tailing the log group.
func (t *LogGroupTailer) Start(ctx context.Context) error {
	// Calculate start time based on Since configuration
	startTime := time.Now().Add(-t.config.Since)
	t.lastTimestamp = &startTime

	// Fetch initial logs if Tail is configured
	if t.config.Tail > 0 {
		if err := t.fetchInitialLogs(ctx); err != nil {
			return fmt.Errorf("fetch initial logs: %w", err)
		}
	}

	// Start polling for new logs
	ticker := time.NewTicker(t.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := t.pollLogs(ctx); err != nil {
				// Log error but continue tailing
				fmt.Fprintf(nil, "error polling logs for %s: %v\n", t.logGroupName, err)
			}
		}
	}
}

// fetchInitialLogs retrieves the initial set of logs.
func (t *LogGroupTailer) fetchInitialLogs(ctx context.Context) error {
	events, err := t.client.GetAllLogEvents(ctx, t.logGroupName, t.lastTimestamp)
	if err != nil {
		return err
	}

	// If Tail is set, only show the last N events
	startIndex := 0
	if t.config.Tail > 0 && len(events) > t.config.Tail {
		startIndex = len(events) - t.config.Tail
	}

	for _, event := range events[startIndex:] {
		logEvent := types.LogEvent{
			Timestamp:     time.UnixMilli(aws.ToInt64(event.Timestamp)),
			Message:       aws.ToString(event.Message),
			LogGroupName:  t.logGroupName,
			LogStreamName: aws.ToString(event.LogStreamName),
		}

		select {
		case t.events <- logEvent:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Update last timestamp
		eventTime := logEvent.Timestamp
		if t.lastTimestamp == nil || eventTime.After(*t.lastTimestamp) {
			t.lastTimestamp = &eventTime
		}
	}

	return nil
}

// pollLogs polls for new logs since the last timestamp.
func (t *LogGroupTailer) pollLogs(ctx context.Context) error {
	// Add a small buffer to the start time to avoid missing events
	startTime := t.lastTimestamp
	if startTime != nil {
		bufferedTime := startTime.Add(1 * time.Millisecond)
		startTime = &bufferedTime
	}

	events, err := t.client.GetAllLogEvents(ctx, t.logGroupName, startTime)
	if err != nil {
		return err
	}

	for _, event := range events {
		logEvent := types.LogEvent{
			Timestamp:     time.UnixMilli(aws.ToInt64(event.Timestamp)),
			Message:       aws.ToString(event.Message),
			LogGroupName:  t.logGroupName,
			LogStreamName: aws.ToString(event.LogStreamName),
		}

		select {
		case t.events <- logEvent:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Update last timestamp
		eventTime := logEvent.Timestamp
		if t.lastTimestamp == nil || eventTime.After(*t.lastTimestamp) {
			t.lastTimestamp = &eventTime
		}
	}

	return nil
}
