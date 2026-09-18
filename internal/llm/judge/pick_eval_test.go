package judge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mizubot-go/internal/anilist"
	"mizubot-go/internal/llm"
	"mizubot-go/internal/typesafe"
)

type pickCase struct {
	Name        string `json:"name"`
	UserMessage string `json:"user_message"`
	Query       string `json:"query"`
	Fixture     string `json:"fixture"`
	ExpectedID  int    `json:"expected_id"`
}

// TestEvalMediaPicker measures how well Jev picks the intended anime from
// real AniList candidate lists (testdata/*.json are saved AniList responses,
// so the model is the only moving part). It calls the real TypeSafe API, so
// it is skipped unless LLM_JEV_API_KEY is set.
//
// It logs per-case results and overall accuracy, and fails only when Jev is
// confidently wrong: a wrong pick that would clear anilist.Pick.Decisive and
// so reach the user without a clarifying question.
func TestEvalMediaPicker(t *testing.T) {
	apiKey := os.Getenv("LLM_JEV_API_KEY")
	if apiKey == "" {
		t.Skip("set LLM_JEV_API_KEY to run the Jev disambiguation eval")
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "pick_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []pickCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}

	client := typesafe.NewClient(typesafe.Config{
		APIKey:  apiKey,
		BaseURL: os.Getenv("LLM_JEV_BASE_URL"),
		Model:   os.Getenv("LLM_JEV_MODEL"),
		Timeout: 30 * time.Second,
	})
	picker := NewMediaPicker(client, nil)

	var correct, decisive, confidentlyWrong int
	for _, tc := range cases {
		fixture, err := os.ReadFile(filepath.Join("testdata", tc.Fixture+".json"))
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(fixture)
		}))
		candidates, err := anilist.NewClient(anilist.Config{BaseURL: srv.URL, CacheTTL: -1}).Search(context.Background(), tc.Query, 5)
		srv.Close()
		if err != nil {
			t.Fatalf("%s: load fixture: %v", tc.Name, err)
		}

		pick, err := picker.Pick(context.Background(), llm.ToolContext{Message: tc.UserMessage}, tc.Query, candidates)
		if err != nil {
			t.Errorf("%s: Pick: %v", tc.Name, err)
			continue
		}

		ok := pick.ID == tc.ExpectedID
		if ok {
			correct++
		}
		if pick.Decisive() {
			decisive++
			if !ok {
				confidentlyWrong++
				t.Errorf("%s: confidently wrong: picked %d (top=%.2f runner_up=%.2f), want %d", tc.Name, pick.ID, pick.Top, pick.RunnerUp, tc.ExpectedID)
			}
		}
		t.Logf("%-24s correct=%-5v decisive=%-5v picked=%d want=%d top=%.2f runner_up=%.2f", tc.Name, ok, pick.Decisive(), pick.ID, tc.ExpectedID, pick.Top, pick.RunnerUp)
	}
	t.Logf("accuracy %d/%d, decisive %d/%d, confidently wrong %d", correct, len(cases), decisive, len(cases), confidentlyWrong)
}

// TestPickCaseFixturesAreConsistent runs offline: every eval case must point
// at a fixture that actually contains its expected ID, so a bad fixture can't
// masquerade as a Jev mistake.
func TestPickCaseFixturesAreConsistent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pick_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []pickCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("only %d eval cases, want at least 10", len(cases))
	}
	for _, tc := range cases {
		fixture, err := os.ReadFile(filepath.Join("testdata", tc.Fixture+".json"))
		if err != nil {
			t.Errorf("%s: %v", tc.Name, err)
			continue
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(fixture) }))
		candidates, err := anilist.NewClient(anilist.Config{BaseURL: srv.URL, CacheTTL: -1}).Search(context.Background(), tc.Query, 5)
		srv.Close()
		if err != nil {
			t.Errorf("%s: %v", tc.Name, err)
			continue
		}
		if len(candidates) < 2 {
			t.Errorf("%s: fixture has %d candidates, want an ambiguous list", tc.Name, len(candidates))
		}
		found := false
		for _, m := range candidates {
			if m.ID == tc.ExpectedID {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected id %d is not in fixture %s", tc.Name, tc.ExpectedID, tc.Fixture)
		}
	}
}
