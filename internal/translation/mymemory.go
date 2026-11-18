package translation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"techpedia/internal/edition"
)

const (
	DefaultMyMemoryEndpoint  = "https://api.mymemory.translated.net/get"
	maxMyMemoryQueryBytes    = 500
	maxMyMemoryResponseBytes = 1 << 20
	interfaceTranslationTime = 20 * time.Second
	interfaceTranslationJobs = 4
)

var ErrTranslationUnavailable = errors.New("interface translation unavailable")

type InterfaceTranslator interface {
	TranslateInterface(ctx context.Context, targetLocale string) (map[string]string, error)
}

type MyMemory struct {
	endpoint string
	email    string
	client   *http.Client
}

func NewMyMemory(endpoint string, email string, client *http.Client) *MyMemory {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = DefaultMyMemoryEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &MyMemory{endpoint: endpoint, email: strings.TrimSpace(email), client: client}
}

func (m *MyMemory) TranslateInterface(ctx context.Context, targetLocale string) (map[string]string, error) {
	profile, ok := edition.LanguageProfileFor(targetLocale)
	if !ok {
		return nil, fmt.Errorf("%w: invalid target locale", ErrTranslationUnavailable)
	}
	targetLocale = profile.Tag
	seeds := InterfaceMessages()
	if targetLocale == "en" {
		result := make(map[string]string, len(seeds))
		for _, seed := range seeds {
			result[seed.Key] = seed.Source
		}
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, interfaceTranslationTime)
	defer cancel()

	translated := make([]string, len(seeds))
	jobs := make(chan int)
	var workers sync.WaitGroup
	var failure error
	var failureOnce sync.Once
	workerCount := min(interfaceTranslationJobs, len(seeds))
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				value, err := m.translateMessage(ctx, targetLocale, seeds[index])
				if err != nil {
					failureOnce.Do(func() {
						failure = err
						cancel()
					})
					return
				}
				translated[index] = value
			}
		}()
	}

sendJobs:
	for index := range seeds {
		select {
		case jobs <- index:
		case <-ctx.Done():
			break sendJobs
		}
	}
	close(jobs)
	workers.Wait()
	if failure != nil {
		return nil, failure
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: translation timed out", ErrTranslationUnavailable)
	}

	result := make(map[string]string, len(seeds))
	allSources := make([]string, 0, len(seeds))
	allTranslations := make([]string, 0, len(seeds))
	for index, seed := range seeds {
		result[seed.Key] = translated[index]
		allSources = append(allSources, seed.Source)
		allTranslations = append(allTranslations, translated[index])
	}
	if err := validateTranslation(targetLocale, allSources, allTranslations); err != nil {
		return nil, err
	}
	return result, nil
}

func (m *MyMemory) translateMessage(ctx context.Context, targetLocale string, seed InterfaceMessage) (string, error) {
	endpoint, err := url.Parse(m.endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return "", fmt.Errorf("%w: invalid translation endpoint", ErrTranslationUnavailable)
	}
	if seed.Source == "" || len(seed.Source) > maxMyMemoryQueryBytes || strings.ContainsAny(seed.Source, "\r\n") {
		return "", fmt.Errorf("%w: invalid interface seed", ErrTranslationUnavailable)
	}
	query := endpoint.Query()
	query.Set("q", seed.Source)
	query.Set("langpair", "en|"+providerLocale(targetLocale))
	query.Set("mt", "1")
	if m.email != "" {
		query.Set("de", m.email)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("%w: create request", ErrTranslationUnavailable)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "TechPedia interface translation")
	response, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: request failed", ErrTranslationUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrTranslationUnavailable, response.StatusCode)
	}
	var payload struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
		ResponseStatus int  `json:"responseStatus"`
		QuotaFinished  bool `json:"quotaFinished"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxMyMemoryResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("%w: invalid response", ErrTranslationUnavailable)
	}
	if payload.QuotaFinished || payload.ResponseStatus != http.StatusOK {
		return "", fmt.Errorf("%w: provider rejected request", ErrTranslationUnavailable)
	}
	text := html.UnescapeString(payload.ResponseData.TranslatedText)
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimSpace(text)
	if strings.Contains(text, "\n") {
		return "", fmt.Errorf("%w: translated message structure changed", ErrTranslationUnavailable)
	}
	if text == "" || utf8.RuneCountInString(text) > 240 {
		return "", fmt.Errorf("%w: invalid translated message", ErrTranslationUnavailable)
	}
	return text, nil
}

func validateTranslation(targetLocale string, sources []string, translated []string) error {
	if len(sources) == 0 || len(sources) != len(translated) {
		return fmt.Errorf("%w: incomplete translation", ErrTranslationUnavailable)
	}
	different := 0
	unique := make(map[string]struct{}, len(translated))
	joined := strings.Builder{}
	for index, value := range translated {
		value = strings.TrimSpace(value)
		lower := strings.ToLower(value)
		if value == "" || strings.Contains(lower, "mymemory warning") || strings.Contains(lower, "query limit") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
			return fmt.Errorf("%w: suspicious translated message", ErrTranslationUnavailable)
		}
		if utf8.RuneCountInString(value) > utf8.RuneCountInString(sources[index])*8+80 {
			return fmt.Errorf("%w: translated message is unexpectedly long", ErrTranslationUnavailable)
		}
		if !strings.EqualFold(value, sources[index]) {
			different++
		}
		unique[lower] = struct{}{}
		joined.WriteString(value)
		joined.WriteByte('\n')
	}
	if different < len(translated)/3 || len(unique) < len(translated)/4 {
		return fmt.Errorf("%w: translation did not change enough messages", ErrTranslationUnavailable)
	}
	if edition.CanDetectWritingSystem(targetLocale) {
		if matched := edition.WritingSystemRuneCount(targetLocale, joined.String()); matched < 3 {
			return fmt.Errorf("%w: target writing system was not detected", ErrTranslationUnavailable)
		}
	}
	return nil
}

func providerLocale(tag string) string {
	return tag
}
