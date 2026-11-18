package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"techpedia/internal/document"
	"techpedia/internal/edition"
	"techpedia/internal/storage"
	"techpedia/internal/techmark"
)

func main() {
	dataDir := envString("BENCH_DATA_DIR", "/data")
	documentCount := envInt("BENCH_DOCUMENTS", 1000)
	if documentCount < 10 || documentCount > 100000 {
		log.Fatalf("BENCH_DOCUMENTS must be between 10 and 100000")
	}

	store, err := storage.OpenBadgerDocumentStore(dataDir + "/badger")
	if err != nil {
		log.Fatalf("open benchmark store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.ProposeEdition(ctx, edition.Edition{
		Tag:         "ko",
		Name:        "Korean",
		NativeName:  "한국어",
		Direction:   "ltr",
		Status:      edition.StatusPending,
		RequestedBy: "benchmark",
		RequestedAt: time.Now().UTC(),
	}); err != nil {
		log.Fatalf("propose benchmark edition: %v", err)
	}
	messages := make(map[string]string, len(edition.InterfaceMessageKeys()))
	for _, key := range edition.InterfaceMessageKeys() {
		messages[key] = "Benchmark " + key
	}
	if _, err := store.UpdateInterfaceMessages(ctx, "ko", messages); err != nil {
		log.Fatalf("configure benchmark interface: %v", err)
	}
	if _, err := store.UpdateNavigation(ctx, "ko", []edition.NavigationSection{{
		Title: "Benchmark",
		Items: []edition.NavigationItem{{
			Label:  "Benchmark Main",
			Target: "Benchmark Main",
		}},
	}}); err != nil {
		log.Fatalf("configure benchmark navigation: %v", err)
	}
	if _, err := store.ActivateEdition(ctx, "ko", "Benchmark Main", time.Now().UTC()); err != nil {
		log.Fatalf("activate benchmark edition: %v", err)
	}

	for index := 0; index < documentCount; index++ {
		title := fmt.Sprintf("Benchmark Article %04d", index)
		if index == 0 {
			title = "Benchmark Main"
		}
		doc := document.Document{
			Locale:     "ko",
			Slug:       techmark.Slugify(title),
			Title:      title,
			Source:     fixtureSource(title, index, documentCount),
			Author:     "benchmark",
			Summary:    "isolated benchmark fixture",
			EditPolicy: document.EditPolicyEditors,
		}
		if _, err := store.Save(ctx, doc); err != nil {
			log.Fatalf("save benchmark document %d: %v", index, err)
		}
	}

	log.Printf("created %d benchmark documents in %s", documentCount, dataDir)
}

func fixtureSource(title string, index int, documentCount int) string {
	var source strings.Builder
	fmt.Fprintf(&source, "# %s\n\n", title)
	source.WriteString("이 문서는 TechPedia 엔진의 읽기, 검색, 링크 해석과 TechMark 렌더링 성능을 측정하기 위한 격리 fixture이다.\n\n")
	for section := 1; section <= 12; section++ {
		next := (index + section) % documentCount
		fmt.Fprintf(&source, "## 성능 측정 문단 %d\n\n", section)
		fmt.Fprintf(&source, "동시 요청에서 문서 렌더링과 내부 링크 [[Benchmark Article %04d]] 처리를 확인한다. ", next)
		source.WriteString("저장소 조회, HTML 안전 처리, 검색 후보 캐시가 실제 문서 크기에서도 안정적으로 동작해야 한다.\n\n")
		if section%3 == 0 {
			source.WriteString("| 항목 | 값 | 설명 |\n| --- | ---: | --- |\n")
			fmt.Fprintf(&source, "| 문단 | %d | 반복 가능한 부하 데이터 |\n", section)
			source.WriteString("| 상태 | active | 공개 문서 응답 |\n\n")
		}
	}
	source.WriteString("```go\nfunc benchmarkRequest() error {\n\treturn nil\n}\n```\n")
	return source.String()
}

func envString(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value := envString(name, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("%s must be an integer: %v", name, err)
	}
	return parsed
}
