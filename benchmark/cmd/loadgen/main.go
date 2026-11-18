package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type config struct {
	TargetURL      string
	Targets        []int
	StageDuration  time.Duration
	WarmupDuration time.Duration
	RequestTimeout time.Duration
	MaxP95         time.Duration
	MaxErrorRate   float64
	MinAchievement float64
	Workers        int
	Documents      int
	RefineSteps    int
	PeakFactor     float64
	ProfileName    string
	ContinueOnFail bool
	ResultPath     string
}

type requestTask struct {
	sequence  uint64
	scheduled time.Time
	results   chan<- requestResult
	done      *sync.WaitGroup
}

type requestResult struct {
	latency   time.Duration
	status    int
	bytesRead int64
	failed    bool
}

type latencyReport struct {
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

type stageReport struct {
	TargetRPS        int            `json:"target_rps"`
	StageSeconds     float64        `json:"stage_seconds"`
	WallSeconds      float64        `json:"wall_seconds"`
	Scheduled        int64          `json:"scheduled"`
	Completed        int64          `json:"completed"`
	Successful       int64          `json:"successful"`
	Errors           int64          `json:"errors"`
	Dropped          int64          `json:"dropped"`
	AchievedRPS      float64        `json:"achieved_rps"`
	AchievementRatio float64        `json:"achievement_ratio"`
	ErrorRate        float64        `json:"error_rate"`
	ResponseBytes    int64          `json:"response_bytes"`
	PeakInFlight     int64          `json:"peak_in_flight"`
	DailyRequests    int64          `json:"modeled_daily_requests"`
	Latency          latencyReport  `json:"latency"`
	StatusCodes      map[string]int `json:"status_codes"`
	Sustainable      bool           `json:"sustainable"`
	FailureReasons   []string       `json:"failure_reasons,omitempty"`
}

type benchmarkReport struct {
	StartedAt             string        `json:"started_at"`
	FinishedAt            string        `json:"finished_at"`
	TargetURL             string        `json:"target_url"`
	Documents             int           `json:"documents"`
	Workers               int           `json:"workers"`
	RequestTimeout        string        `json:"request_timeout"`
	MaxP95                string        `json:"max_p95"`
	MaxErrorRate          float64       `json:"max_error_rate"`
	MinAchievement        float64       `json:"min_achievement"`
	PeakFactor            float64       `json:"peak_factor"`
	ProfileName           string        `json:"profile_name"`
	MaximumSustainableRPS int           `json:"maximum_sustainable_rps"`
	HighestStagePassed    bool          `json:"highest_stage_passed"`
	FirstUnsustainableRPS int           `json:"first_unsustainable_rps,omitempty"`
	Stages                []stageReport `json:"stages"`
}

type runner struct {
	config   config
	baseURL  *url.URL
	client   *http.Client
	tasks    chan requestTask
	sequence atomic.Uint64
	inFlight atomic.Int64
	peak     atomic.Int64
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	baseURL, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatalf("invalid BENCH_TARGET_URL: %v", err)
	}
	if err := validateTarget(baseURL); err != nil {
		log.Fatal(err)
	}

	transport := &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          cfg.Workers,
		MaxIdleConnsPerHost:   cfg.Workers,
		MaxConnsPerHost:       cfg.Workers,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
	}
	loadRunner := &runner{
		config:  cfg,
		baseURL: baseURL,
		client:  &http.Client{Transport: transport, Timeout: cfg.RequestTimeout},
		tasks:   make(chan requestTask, cfg.Workers),
	}
	for index := 0; index < cfg.Workers; index++ {
		go loadRunner.worker()
	}
	defer close(loadRunner.tasks)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	started := time.Now().UTC()

	warmupRPS := cfg.Targets[0]
	if warmupRPS > 100 {
		warmupRPS = 100
	}
	fmt.Printf("TechPedia isolated load benchmark\n")
	fmt.Printf("target=%s documents=%d workers=%d\n", cfg.TargetURL, cfg.Documents, cfg.Workers)
	fmt.Printf("thresholds: p95<=%s errors<=%.2f%% achievement>=%.1f%% dropped=0\n\n", cfg.MaxP95, cfg.MaxErrorRate*100, cfg.MinAchievement*100)
	if cfg.WarmupDuration > 0 {
		fmt.Printf("warming up: %d RPS for %s\n", warmupRPS, cfg.WarmupDuration)
		if _, err := loadRunner.runStage(ctx, warmupRPS, cfg.WarmupDuration); err != nil {
			log.Fatalf("warmup failed: %v", err)
		}
	}

	report := benchmarkReport{
		StartedAt:      started.Format(time.RFC3339),
		TargetURL:      cfg.TargetURL,
		Documents:      cfg.Documents,
		Workers:        cfg.Workers,
		RequestTimeout: cfg.RequestTimeout.String(),
		MaxP95:         cfg.MaxP95.String(),
		MaxErrorRate:   cfg.MaxErrorRate,
		MinAchievement: cfg.MinAchievement,
		PeakFactor:     cfg.PeakFactor,
		ProfileName:    cfg.ProfileName,
	}
	lastPassed := 0
	firstFailed := 0
	for _, target := range cfg.Targets {
		if err := ctx.Err(); err != nil {
			log.Fatal(err)
		}
		stage, err := loadRunner.runStage(ctx, target, cfg.StageDuration)
		if err != nil {
			log.Fatal(err)
		}
		report.Stages = append(report.Stages, stage)
		printStage(stage)
		if stage.Sustainable {
			if target > report.MaximumSustainableRPS {
				report.MaximumSustainableRPS = target
			}
			if firstFailed == 0 {
				lastPassed = target
			}
		} else {
			if firstFailed == 0 {
				firstFailed = target
			}
			if !cfg.ContinueOnFail {
				break
			}
		}
	}
	for step := 0; firstFailed > 0 && step < cfg.RefineSteps && firstFailed-lastPassed > 1; step++ {
		target := lastPassed + (firstFailed-lastPassed)/2
		stage, err := loadRunner.runStage(ctx, target, cfg.StageDuration)
		if err != nil {
			log.Fatal(err)
		}
		report.Stages = append(report.Stages, stage)
		printStage(stage)
		if stage.Sustainable {
			lastPassed = target
			report.MaximumSustainableRPS = target
		} else {
			firstFailed = target
		}
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	report.HighestStagePassed = firstFailed == 0
	report.FirstUnsustainableRPS = firstFailed

	if err := writeReport(cfg.ResultPath, report); err != nil {
		log.Fatalf("write benchmark report: %v", err)
	}
	fmt.Printf("\nmaximum sustainable target: %d RPS", report.MaximumSustainableRPS)
	if report.HighestStagePassed {
		fmt.Printf(" or higher (highest configured stage passed)")
	} else {
		fmt.Printf("; first measured failure: %d RPS", report.FirstUnsustainableRPS)
	}
	fmt.Printf("\nreport: %s\n", cfg.ResultPath)
}

func (r *runner) runStage(ctx context.Context, targetRPS int, duration time.Duration) (stageReport, error) {
	if targetRPS <= 0 || duration <= 0 {
		return stageReport{}, errors.New("stage target and duration must be positive")
	}

	r.peak.Store(0)
	results := make(chan requestResult, r.config.Workers)
	var requests sync.WaitGroup
	var dropped atomic.Int64
	start := time.Now()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	total := int64(math.Round(float64(targetRPS) * duration.Seconds()))
	scheduled := int64(0)
	scheduleUntil := func(expected int64) {
		if expected > total {
			expected = total
		}
		for scheduled < expected {
			sequence := r.sequence.Add(1)
			offset := time.Duration(float64(scheduled) / float64(targetRPS) * float64(time.Second))
			task := requestTask{sequence: sequence, scheduled: start.Add(offset), results: results, done: &requests}
			requests.Add(1)
			select {
			case r.tasks <- task:
			default:
				requests.Done()
				dropped.Add(1)
			}
			scheduled++
		}
	}

	loop := true
	for loop {
		select {
		case <-ctx.Done():
			return stageReport{}, ctx.Err()
		case now := <-ticker.C:
			elapsed := now.Sub(start)
			if elapsed >= duration {
				scheduleUntil(total)
				loop = false
				continue
			}
			scheduleUntil(int64(float64(targetRPS) * elapsed.Seconds()))
		}
	}

	finished := make(chan struct{})
	go func() {
		requests.Wait()
		close(finished)
	}()
	go func() {
		<-finished
		close(results)
	}()

	latencies := make([]time.Duration, 0, total-dropped.Load())
	statuses := make(map[string]int)
	var failed int64
	var bytesRead int64
	for result := range results {
		latencies = append(latencies, result.latency)
		statuses[strconv.Itoa(result.status)]++
		bytesRead += result.bytesRead
		if result.failed {
			failed++
		}
	}
	wallDuration := time.Since(start)
	completed := int64(len(latencies))
	successful := completed - failed
	achieved := float64(completed) / wallDuration.Seconds()
	achievementRatio := achieved / float64(targetRPS)
	errorRate := 0.0
	if completed > 0 {
		errorRate = float64(failed) / float64(completed)
	}
	latency := summarizeLatency(latencies)

	stage := stageReport{
		TargetRPS:        targetRPS,
		StageSeconds:     duration.Seconds(),
		WallSeconds:      wallDuration.Seconds(),
		Scheduled:        total,
		Completed:        completed,
		Successful:       successful,
		Errors:           failed,
		Dropped:          dropped.Load(),
		AchievedRPS:      achieved,
		AchievementRatio: achievementRatio,
		ErrorRate:        errorRate,
		ResponseBytes:    bytesRead,
		PeakInFlight:     r.peak.Load(),
		DailyRequests:    int64(math.Round(float64(targetRPS) * 86400 / r.config.PeakFactor)),
		Latency:          latency,
		StatusCodes:      statuses,
		Sustainable:      true,
	}
	if time.Duration(latency.P95MS*float64(time.Millisecond)) > r.config.MaxP95 {
		stage.FailureReasons = append(stage.FailureReasons, "p95 latency exceeded")
	}
	if errorRate > r.config.MaxErrorRate {
		stage.FailureReasons = append(stage.FailureReasons, "error rate exceeded")
	}
	if achievementRatio < r.config.MinAchievement {
		stage.FailureReasons = append(stage.FailureReasons, "target RPS was not sustained")
	}
	if stage.Dropped > 0 {
		stage.FailureReasons = append(stage.FailureReasons, "load queue dropped requests")
	}
	stage.Sustainable = len(stage.FailureReasons) == 0
	return stage, nil
}

func (r *runner) worker() {
	for task := range r.tasks {
		inFlight := r.inFlight.Add(1)
		for {
			peak := r.peak.Load()
			if inFlight <= peak || r.peak.CompareAndSwap(peak, inFlight) {
				break
			}
		}
		result := r.execute(task.sequence, task.scheduled)
		r.inFlight.Add(-1)
		task.results <- result
		task.done.Done()
	}
}

func (r *runner) execute(sequence uint64, scheduled time.Time) requestResult {
	method, path := r.requestFor(sequence)
	requestURL := r.baseURL.ResolveReference(&url.URL{Path: path})
	if strings.Contains(path, "?") {
		parts := strings.SplitN(path, "?", 2)
		requestURL = r.baseURL.ResolveReference(&url.URL{Path: parts[0], RawQuery: parts[1]})
	}
	req, err := http.NewRequest(method, requestURL.String(), nil)
	if err != nil {
		return requestResult{latency: time.Since(scheduled), failed: true}
	}
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.1")
	req.Header.Set("User-Agent", "TechPedia-Isolated-Benchmark/1")
	response, err := r.client.Do(req)
	if err != nil {
		return requestResult{latency: time.Since(scheduled), failed: true}
	}
	bytesRead, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	failed := readErr != nil || closeErr != nil || response.StatusCode < 200 || response.StatusCode >= 300
	return requestResult{
		latency:   time.Since(scheduled),
		status:    response.StatusCode,
		bytesRead: bytesRead,
		failed:    failed,
	}
}

func (r *runner) requestFor(sequence uint64) (string, string) {
	documentIndex := 1 + int(sequence%uint64(r.config.Documents-1))
	articleName := fmt.Sprintf("Benchmark_Article_%04d", documentIndex)
	switch sequence % 100 {
	case 0, 1, 2, 3, 4:
		return http.MethodPost, "/api/pages/ko/" + articleName + "/views"
	case 5, 6, 7, 8, 9:
		return http.MethodGet, "/api/languages"
	case 10, 11, 12, 13, 14, 15, 16:
		return http.MethodGet, "/api/recent-changes?locale=ko&limit=20"
	case 17, 18, 19, 20, 21, 22, 23, 24:
		return http.MethodGet, "/api/links/resolve?locale=ko&target=Benchmark%20Article%200001&target=Benchmark%20Article%200002&target=Benchmark%20Article%200003"
	case 25, 26, 27, 28, 29, 30, 31, 32, 33, 34:
		return http.MethodGet, "/api/search?locale=ko&q=Benchmark&limit=8"
	case 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59:
		return http.MethodGet, "/api/pages/ko/" + articleName
	default:
		return http.MethodGet, "/ko/wiki/" + articleName
	}
}

func summarizeLatency(values []time.Duration) latencyReport {
	if len(values) == 0 {
		return latencyReport{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return latencyReport{
		P50MS: durationMS(percentile(values, 0.50)),
		P95MS: durationMS(percentile(values, 0.95)),
		P99MS: durationMS(percentile(values, 0.99)),
		MaxMS: durationMS(values[len(values)-1]),
	}
}

func percentile(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(values))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func durationMS(value time.Duration) float64 {
	return math.Round(float64(value)/float64(time.Millisecond)*100) / 100
}

func printStage(stage stageReport) {
	status := "PASS"
	if !stage.Sustainable {
		status = "FAIL"
	}
	fmt.Printf("%-4s target=%6d RPS achieved=%7.0f p95=%8.1fms p99=%8.1fms errors=%6.2f%% dropped=%d in-flight=%d daily=%s\n",
		status, stage.TargetRPS, stage.AchievedRPS, stage.Latency.P95MS, stage.Latency.P99MS, stage.ErrorRate*100, stage.Dropped, stage.PeakInFlight, formatCount(stage.DailyRequests))
	if len(stage.FailureReasons) > 0 {
		fmt.Printf("     %s\n", strings.Join(stage.FailureReasons, "; "))
	}
}

func writeReport(path string, report benchmarkReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func loadConfig() (config, error) {
	var cfg config
	cfg.TargetURL = strings.TrimRight(envString("BENCH_TARGET_URL", "http://127.0.0.1:8080"), "/")
	var err error
	if cfg.Targets, err = parseTargets(envString("BENCH_TARGETS", "100,250,500,1000,2000,4000,8000")); err != nil {
		return cfg, err
	}
	if cfg.StageDuration, err = envDuration("BENCH_STAGE_DURATION", 20*time.Second); err != nil {
		return cfg, err
	}
	if cfg.WarmupDuration, err = envDuration("BENCH_WARMUP_DURATION", 8*time.Second); err != nil {
		return cfg, err
	}
	if cfg.RequestTimeout, err = envDuration("BENCH_REQUEST_TIMEOUT", 5*time.Second); err != nil {
		return cfg, err
	}
	if cfg.MaxP95, err = envDuration("BENCH_MAX_P95", time.Second); err != nil {
		return cfg, err
	}
	if cfg.MaxErrorRate, err = envFloat("BENCH_MAX_ERROR_RATE", 0.01); err != nil {
		return cfg, err
	}
	if cfg.MinAchievement, err = envFloat("BENCH_MIN_ACHIEVEMENT", 0.95); err != nil {
		return cfg, err
	}
	if cfg.Workers, err = envInt("BENCH_WORKERS", 4096); err != nil {
		return cfg, err
	}
	if cfg.Documents, err = envInt("BENCH_DOCUMENTS", 1000); err != nil {
		return cfg, err
	}
	if cfg.RefineSteps, err = envInt("BENCH_REFINE_STEPS", 4); err != nil {
		return cfg, err
	}
	if cfg.PeakFactor, err = envFloat("BENCH_PEAK_FACTOR", 1); err != nil {
		return cfg, err
	}
	if cfg.ContinueOnFail, err = envBool("BENCH_CONTINUE_AFTER_FAILURE", false); err != nil {
		return cfg, err
	}
	cfg.ProfileName = envString("BENCH_PROFILE_NAME", "staircase")
	cfg.ResultPath = envString("BENCH_RESULT", "result.json")
	if cfg.Workers < 1 || cfg.Workers > 50000 {
		return cfg, errors.New("BENCH_WORKERS must be between 1 and 50000")
	}
	if cfg.Documents < 10 {
		return cfg, errors.New("BENCH_DOCUMENTS must be at least 10")
	}
	if cfg.RefineSteps < 0 || cfg.RefineSteps > 12 {
		return cfg, errors.New("BENCH_REFINE_STEPS must be between 0 and 12")
	}
	if cfg.PeakFactor < 1 || cfg.PeakFactor > 1000 {
		return cfg, errors.New("BENCH_PEAK_FACTOR must be between 1 and 1000")
	}
	if cfg.MaxErrorRate < 0 || cfg.MaxErrorRate >= 1 || cfg.MinAchievement <= 0 || cfg.MinAchievement > 1 {
		return cfg, errors.New("invalid benchmark threshold")
	}
	return cfg, nil
}

func parseTargets(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	targets := make([]int, 0, len(parts))
	previous := 0
	for _, part := range parts {
		target, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || target <= previous || target > 1000000 {
			return nil, fmt.Errorf("BENCH_TARGETS must be increasing positive integers: %q", value)
		}
		targets = append(targets, target)
		previous = target
	}
	if len(targets) == 0 {
		return nil, errors.New("BENCH_TARGETS must not be empty")
	}
	return targets, nil
}

func validateTarget(target *url.URL) error {
	if target.Scheme != "http" && target.Scheme != "https" {
		return errors.New("benchmark target must use http or https")
	}
	host := strings.ToLower(target.Hostname())
	if host == "engine" || host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return nil
	}
	if strings.EqualFold(os.Getenv("BENCH_ALLOW_REMOTE"), "true") {
		return nil
	}
	return fmt.Errorf("refusing non-local benchmark target %q; use the isolated Compose runner", host)
}

func envString(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := envString(name, fallback.String())
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func envInt(name string, fallback int) (int, error) {
	value := envString(name, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func envFloat(name string, fallback float64) (float64, error) {
	value := envString(name, strconv.FormatFloat(fallback, 'f', -1, 64))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	return parsed, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value := envString(name, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func formatCount(value int64) string {
	formatted := strconv.FormatInt(value, 10)
	for index := len(formatted) - 3; index > 0; index -= 3 {
		formatted = formatted[:index] + "," + formatted[index:]
	}
	return formatted
}
