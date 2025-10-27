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

// Command awstern tails AWS ECS logs from CloudWatch Logs for tasks in a cluster.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	cwlogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/spf13/cobra"
)

const version = "0.0.1"

type options struct {
	cluster    string
	service    string
	family     string
	container  string
	include    string
	exclude    string
	region     string
	profile    string
	since      string
	follow     bool
	noColor    bool
	showTime   bool
	maxStreams int
}

type taskInfo struct {
	taskARN           string
	taskID            string
	taskDefinitionARN string
	serviceName       string
	launchType        ecstypes.LaunchType
}

type containerLogConfig struct {
	containerName string
	logGroup      string
	streamPrefix  string
	region        string // resolved: default or awslogs-region override
}

type streamTarget struct {
	logGroup      string
	logStream     string
	region        string
	service       string
	taskID        string
	containerName string
}

type printEvent struct {
	ts        time.Time
	service   string
	taskID    string
	container string
	message   string
	region    string
}

type colorizer struct {
	enabled bool
	palette []string
	byKey   map[string]string
}

func newColorizer(enabled bool) *colorizer {
	return &colorizer{
		enabled: enabled,
		palette: []string{"31", "32", "33", "34", "35", "36", "90"},
		byKey:   map[string]string{},
	}
}

func (c *colorizer) color(key, s string) string {
	if !c.enabled {
		return s
	}
	if code, ok := c.byKey[key]; ok {
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
	idx := len(c.byKey) % len(c.palette)
	code := c.palette[idx]
	c.byKey[key] = code
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func newRootCmd() *cobra.Command {
	opt := &options{
		since:      "10m",
		follow:     true,
		noColor:    false,
		showTime:   true,
		maxStreams: 80,
	}
	cmd := &cobra.Command{
		Use:   "awstern",
		Short: "Tails AWS ECS logs from CloudWatch Logs for tasks in a cluster.",
		Long: `awstern: Tails AWS ECS logs from CloudWatch Logs for tasks in a cluster.
Targets tasks by --service or --family, resolves CloudWatch Logs streams from awslogs settings, and tails them concurrently.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate required
			if opt.cluster == "" {
				return fmt.Errorf("--cluster is required")
			}
			return run(cmd.Context(), *opt)
		},
	}

	// Flags
	cmd.Flags().StringVar(&opt.cluster, "cluster", "", "ECS cluster name (required)")
	cmd.Flags().StringVar(&opt.service, "service", "", "ECS service name to select tasks")
	cmd.Flags().StringVar(&opt.family, "family", "", "ECS task definition family to select tasks")
	cmd.Flags().StringVar(&opt.container, "container", "", "Container name regex filter")
	cmd.Flags().StringVar(&opt.include, "include", "", "Include regex to filter log messages")
	cmd.Flags().StringVar(&opt.exclude, "exclude", "", "Exclude regex to filter log messages")
	cmd.Flags().StringVar(&opt.region, "region", "", "AWS region (fallback to env/profile)")
	cmd.Flags().StringVar(&opt.profile, "profile", "", "AWS profile name")
	cmd.Flags().StringVar(&opt.since, "since", opt.since, "Since (duration like 30m/2h or RFC3339 time)")
	cmd.Flags().BoolVar(&opt.follow, "follow", opt.follow, "Follow log output")
	cmd.Flags().BoolVar(&opt.noColor, "no-color", opt.noColor, "Disable colorized prefixes")
	cmd.Flags().BoolVar(&opt.showTime, "timestamps", opt.showTime, "Show timestamps")
	cmd.Flags().IntVar(&opt.maxStreams, "max-streams", opt.maxStreams, "Safety limit for concurrent streams per region")

	// Completion subcommand
	completion := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactValidArgs(1),
		ValidArgs: []string{
			"bash", "zsh", "fish", "powershell",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	cmd.AddCommand(completion)

	return cmd
}

func main() {
	root := newRootCmd()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	root.SetContext(ctx)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opt options) error {
	// Parse since
	start, err := parseSince(opt.since)
	if err != nil {
		return err
	}

	cfg, err := loadCfg(ctx, opt.region, opt.profile)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	ecsCli := ecs.NewFromConfig(cfg)

	// Compile regex filters
	var contRe, incRe, excRe *regexp.Regexp
	if opt.container != "" {
		if contRe, err = regexp.Compile(opt.container); err != nil {
			return fmt.Errorf("invalid --container regex: %w", err)
		}
	}
	if opt.include != "" {
		if incRe, err = regexp.Compile(opt.include); err != nil {
			return fmt.Errorf("invalid --include regex: %w", err)
		}
	}
	if opt.exclude != "" {
		if excRe, err = regexp.Compile(opt.exclude); err != nil {
			return fmt.Errorf("invalid --exclude regex: %w", err)
		}
	}

	tasks, err := listTasks(ctx, ecsCli, opt.cluster, opt.service, opt.family)
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	if len(tasks) == 0 {
		fmt.Println("[info] no running tasks matched.")
		return nil
	}

	tds, err := describeTaskDefs(ctx, ecsCli, tasks)
	if err != nil {
		return fmt.Errorf("describe task definitions: %w", err)
	}

	type pair struct {
		task taskInfo
		cfgs []containerLogConfig
	}
	var pairs []pair
	for _, t := range tasks {
		td := tds[t.taskDefinitionARN]
		cfgs, err := buildContainerLogConfigs(td, cfg.Region, contRe)
		if err != nil {
			continue // skip tasks without awslogs
		}
		pairs = append(pairs, pair{task: t, cfgs: cfgs})
	}
	if len(pairs) == 0 {
		return errors.New("no awslogs-enabled containers found among matched tasks")
	}

	var targets []streamTarget
	for _, p := range pairs {
		ts, _ := resolveStreamsForTask(ctx, cfg, p.task, p.cfgs, opt.maxStreams-len(targets))
		targets = append(targets, ts...)
		if len(targets) >= opt.maxStreams {
			fmt.Fprintf(os.Stderr, "[warn] reached --max-streams limit (%d), truncating\n", opt.maxStreams)
			break
		}
	}
	if len(targets) == 0 {
		return errors.New("no log streams resolved")
	}

	col := newColorizer(!opt.noColor)
	eventsCh := make(chan printEvent, 200)
	var wg sync.WaitGroup
	var mu sync.Mutex
	byRegion := map[string]*cwlogs.Client{}

	getLogsCli := func(region string) *cwlogs.Client {
		if c, ok := byRegion[region]; ok {
			return c
		}
		if region != cfg.Region {
			alt, _ := loadCfg(ctx, region, opt.profile)
			mu.Lock()
			byRegion[region] = cwlogs.NewFromConfig(alt)
			mu.Unlock()
		} else {
			mu.Lock()
			byRegion[region] = cwlogs.NewFromConfig(cfg)
			mu.Unlock()
		}
		return byRegion[region]
	}

	for _, st := range targets {
		wg.Add(1)
		go func(st streamTarget) {
			defer wg.Done()
			cli := getLogsCli(st.region)
			err := tailStream(ctx, cli, st, start, opt.follow, incRe, excRe, eventsCh)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[warn] tail %s/%s: %v\n", st.logGroup, st.logStream, err)
			}
		}(st)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range eventsCh {
			prefixKey := ev.service + "/" + ev.container
			prefix := fmt.Sprintf("%s/%s/%s", ev.service, ev.taskID, ev.container)
			prefix = col.color(prefixKey, prefix)

			if opt.showTime {
				fmt.Printf("%s %s | %s\n", ev.ts.Format(time.RFC3339), prefix, ev.message)
			} else {
				fmt.Printf("%s | %s\n", prefix, ev.message)
			}
		}
	}()

	wg.Wait()
	close(eventsCh)
	<-done
	return nil
}

func parseSince(s string) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid --since; use duration (e.g., 30m) or RFC3339 (e.g., 2025-10-01T12:00:00Z)")
}

func loadCfg(ctx context.Context, region, profile string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}

func listTasks(ctx context.Context, ecsCli *ecs.Client, cluster, service, family string) ([]taskInfo, error) {
	var arns []string
	var next *string
	for {
		in := &ecs.ListTasksInput{
			Cluster:       aws.String(cluster),
			DesiredStatus: ecstypes.DesiredStatusRunning,
			NextToken:     next,
		}
		if service != "" {
			in.ServiceName = aws.String(service)
		}
		if family != "" {
			in.Family = aws.String(family)
		}
		out, err := ecsCli.ListTasks(ctx, in)
		if err != nil {
			return nil, err
		}
		arns = append(arns, out.TaskArns...)
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		next = out.NextToken
	}

	if len(arns) == 0 {
		return nil, nil
	}

	// Describe in batches
	var tasks []ecstypes.Task
	const step = 100
	for i := 0; i < len(arns); i += step {
		j := min(i+step, len(arns))
		out, err := ecsCli.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   arns[i:j],
		})
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, out.Tasks...)
	}

	var res []taskInfo
	for _, t := range tasks {
		id := lastSegment(aws.ToString(t.TaskArn))
		svc := ""
		if after, ok := strings.CutPrefix(aws.ToString(t.Group), "service:"); ok {
			svc = after
		}
		res = append(res, taskInfo{
			taskARN:           aws.ToString(t.TaskArn),
			taskID:            id,
			taskDefinitionARN: aws.ToString(t.TaskDefinitionArn),
			serviceName:       svc,
			launchType:        t.LaunchType,
		})
	}
	// Stable ordering by service -> taskID for deterministic color mapping
	sort.Slice(res, func(i, j int) bool {
		if res[i].serviceName == res[j].serviceName {
			return res[i].taskID < res[j].taskID
		}
		return res[i].serviceName < res[j].serviceName
	})
	return res, nil
}

func lastSegment(arn string) string {
	if arn == "" {
		return ""
	}
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}

func uniqStrings(in []string) []string {
	m := map[string]struct{}{}
	var res []string
	for _, s := range in {
		if _, ok := m[s]; !ok {
			m[s] = struct{}{}
			res = append(res, s)
		}
	}
	return res
}

func describeTaskDefs(ctx context.Context, ecsCli *ecs.Client, tasks []taskInfo) (map[string]ecstypes.TaskDefinition, error) {
	uniq := uniqStrings(func() []string {
		m := map[string]struct{}{}
		for _, t := range tasks {
			m[t.taskDefinitionARN] = struct{}{}
		}
		var out []string
		for arn := range m {
			out = append(out, arn)
		}
		return out
	}())

	tds := make(map[string]ecstypes.TaskDefinition, len(uniq))
	for _, arn := range uniq {
		out, err := ecsCli.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: aws.String(arn),
		})
		if err != nil {
			return nil, err
		}
		tds[arn] = *out.TaskDefinition
	}

	return tds, nil
}

func buildContainerLogConfigs(td ecstypes.TaskDefinition, defaultRegion string, containerRe *regexp.Regexp) ([]containerLogConfig, error) {
	var res []containerLogConfig
	for _, cd := range td.ContainerDefinitions {
		name := aws.ToString(cd.Name)
		if containerRe != nil && !containerRe.MatchString(name) {
			continue
		}
		if cd.LogConfiguration == nil || cd.LogConfiguration.LogDriver != ecstypes.LogDriverAwslogs {
			continue // only awslogs in MVP
		}

		opts := cd.LogConfiguration.Options // map[string]string
		group := opts["awslogs-group"]
		prefix := opts["awslogs-stream-prefix"]
		reg := defaultRegion
		if r := opts["awslogs-region"]; r != "" {
			reg = r
		}
		if group == "" || prefix == "" {
			continue
		}

		res = append(res, containerLogConfig{
			containerName: name,
			logGroup:      group,
			streamPrefix:  prefix,
			region:        reg,
		})
	}
	if len(res) == 0 {
		return nil, errors.New("no awslogs-enabled containers matched")
	}

	return res, nil
}

func resolveStreamsForTask(ctx context.Context, cfg aws.Config, task taskInfo, containerCfgs []containerLogConfig, maxStreams int) ([]streamTarget, error) {
	// Expect 1 stream per (container, task).
	// Stream name pattern: <prefix>/<container>/<taskId>
	var res []streamTarget
	byRegion := map[string]*cwlogs.Client{}
	getCli := func(region string) *cwlogs.Client {
		if c, ok := byRegion[region]; ok {
			return c
		}
		if region != cfg.Region {
			alt, _ := loadCfg(ctx, region, "")
			byRegion[region] = cwlogs.NewFromConfig(alt)
		} else {
			byRegion[region] = cwlogs.NewFromConfig(cfg)
		}
		return byRegion[region]
	}

	for _, c := range containerCfgs {
		cli := getCli(c.region)
		prefix := fmt.Sprintf("%s/%s/%s", c.streamPrefix, c.containerName, task.taskID)
		out, err := cli.DescribeLogStreams(ctx, &cwlogs.DescribeLogStreamsInput{
			LogGroupName:        aws.String(c.logGroup),
			LogStreamNamePrefix: aws.String(prefix),
			Limit:               aws.Int32(1),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] describe log stream failed for %s (%s/%s): %v\n",
				task.taskID, c.logGroup, prefix, err)
			continue
		}
		if len(out.LogStreams) == 0 {
			// stream may not exist yet
			res = append(res, streamTarget{
				logGroup:      c.logGroup,
				logStream:     prefix, // try expected name
				region:        c.region,
				service:       task.serviceName,
				taskID:        task.taskID,
				containerName: c.containerName,
			})
			continue
		}

		res = append(res, streamTarget{
			logGroup:      c.logGroup,
			logStream:     aws.ToString(out.LogStreams[0].LogStreamName),
			region:        c.region,
			service:       task.serviceName,
			taskID:        task.taskID,
			containerName: c.containerName,
		})
		if len(res) >= maxStreams {
			fmt.Fprintf(os.Stderr, "[warn] reached --max-streams limit (%d), truncating\n", maxStreams)
			break
		}
	}

	return res, nil
}

func tailStream(ctx context.Context, cli *cwlogs.Client, st streamTarget, start time.Time, follow bool, incRe, excRe *regexp.Regexp, out chan<- printEvent) error {
	startMs := start.UnixMilli()
	idle := 1200 * time.Millisecond
	var next *string
	var emptyCount int

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		resp, err := cli.GetLogEvents(ctx, &cwlogs.GetLogEventsInput{
			LogGroupName:  aws.String(st.logGroup),
			LogStreamName: aws.String(st.logStream),
			NextToken:     next,
			StartTime:     aws.Int64(startMs),
		})
		if err != nil {
			// If stream not found yet, backoff and retry while following
			if follow && isNotFound(err) {
				time.Sleep(1500 * time.Millisecond)
				continue
			}
			return err
		}

		var delivered int
		for _, e := range resp.Events {
			msg := strings.TrimRight(aws.ToString(e.Message), "\n")
			if incRe != nil && !incRe.MatchString(msg) {
				continue
			}
			if excRe != nil && excRe.MatchString(msg) {
				continue
			}
			ts := time.UnixMilli(aws.ToInt64(e.Timestamp))
			out <- printEvent{
				ts:        ts,
				service:   st.service,
				taskID:    st.taskID,
				container: st.containerName,
				message:   msg,
				region:    st.region,
			}
			delivered++
		}

		if resp.NextForwardToken != nil && (next == nil || *resp.NextForwardToken != *next) {
			next = resp.NextForwardToken
		} else {
			if !follow {
				return nil
			}
			emptyCount++
			if emptyCount%10 == 0 && idle < 5*time.Second {
				idle += 200 * time.Millisecond
			}
			time.Sleep(idle)
		}
	}
}

func isNotFound(err error) bool {
	s := err.Error()
	return strings.Contains(s, "ResourceNotFoundException") || strings.Contains(s, "The specified log stream does not exist")
}
