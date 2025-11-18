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

package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// Client wraps the CloudWatch Logs client with helper methods.
type Client struct {
	svc *cloudwatchlogs.Client
}

// NewClient creates a new CloudWatch Logs client.
func NewClient(cfg aws.Config) *Client {
	return &Client{
		svc: cloudwatchlogs.NewFromConfig(cfg),
	}
}

// ListLogGroups retrieves all log groups matching the optional prefix.
func (c *Client) ListLogGroups(ctx context.Context, prefix string) ([]types.LogGroup, error) {
	var logGroups []types.LogGroup
	var nextToken *string

	input := &cloudwatchlogs.DescribeLogGroupsInput{}
	if prefix != "" {
		input.LogGroupNamePrefix = aws.String(prefix)
	}

	for {
		input.NextToken = nextToken
		output, err := c.svc.DescribeLogGroups(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe log groups: %w", err)
		}

		logGroups = append(logGroups, output.LogGroups...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return logGroups, nil
}

// FilterLogEvents retrieves log events from a log group.
func (c *Client) FilterLogEvents(ctx context.Context, logGroupName string, startTime, endTime *time.Time, nextToken *string) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
		NextToken:    nextToken,
	}

	if startTime != nil {
		input.StartTime = aws.Int64(startTime.UnixMilli())
	}
	if endTime != nil {
		input.EndTime = aws.Int64(endTime.UnixMilli())
	}

	output, err := c.svc.FilterLogEvents(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("filter log events for %s: %w", logGroupName, err)
	}

	return output, nil
}

// GetAllLogEvents retrieves all log events from a log group within a time range.
func (c *Client) GetAllLogEvents(ctx context.Context, logGroupName string, startTime *time.Time) ([]types.FilteredLogEvent, error) {
	var allEvents []types.FilteredLogEvent
	var nextToken *string

	for {
		output, err := c.FilterLogEvents(ctx, logGroupName, startTime, nil, nextToken)
		if err != nil {
			return nil, err
		}

		allEvents = append(allEvents, output.Events...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return allEvents, nil
}
