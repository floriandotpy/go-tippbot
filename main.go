package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/joho/godotenv"

	"github.com/flo/tippbot/gotipp"
)

func main() {
	allFlag := flag.Bool("all", false, "Re-predict and submit tipps for all matches that accept tipps, even if already tipped")
	flag.Parse()

	// Load .env file if present (no error if missing)
	_ = godotenv.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Config from environment
	llmKey := requireEnv("LLM_API_KEY")
	llmURL := requireEnv("LLM_BASE_URL")
	model := envOr("LLM_MODEL", "gpt-5-nano")

	apiToken := requireEnv("GOTIPP_API_TOKEN")
	baseURL := envOr("GOTIPP_BASE_URL", "http://localhost:8080")

	// Initialize Genkit with OpenAI-compatible provider
	g := genkit.Init(ctx,
		genkit.WithPlugins(&compat_oai.OpenAICompatible{
			Provider: "llm",
			APIKey:   llmKey,
			BaseURL:  llmURL,
		}),
		genkit.WithDefaultModel("llm/"+model),
	)

	// Initialize GoTipp API client
	client := gotipp.NewClient(baseURL, apiToken)

	// Load team stats
	teamStats, err := loadTeamStats("data/teams.csv")
	if err != nil {
		log.Fatalf("failed to load team stats: %v", err)
	}
	log.Printf("Loaded stats for %d teams", len(teamStats))

	// 1. Fetch matches
	matches, err := client.GetMatches(ctx)
	if err != nil {
		log.Fatalf("failed to fetch matches: %v", err)
	}

	// 2. Fetch existing tipps
	tipps, err := client.GetTipps(ctx)
	if err != nil {
		log.Fatalf("failed to fetch tipps: %v", err)
	}

	// 3. Find matches that need a tipp
	tippedMatchIDs := make(map[int]bool)
	for _, t := range tipps {
		tippedMatchIDs[t.MatchID] = true
	}

	var pending []gotipp.Match
	for _, m := range matches {
		if m.AcceptsTipps && (*allFlag || !tippedMatchIDs[m.ID]) {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		log.Println("No matches need tipps. All done!")
		return
	}

	log.Printf("Found %d match(es) needing tipps", len(pending))

	// 4. Predict in batches
	const batchSize = 10
	var allTipps []gotipp.TippRequest

	for i := 0; i < len(pending); i += batchSize {
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[i:end]

		log.Printf("Predicting batch %d–%d of %d...", i+1, end, len(pending))

		predictions, err := predictBatch(ctx, g, batch, teamStats)
		if err != nil {
			log.Printf("failed to predict batch: %v", err)
			continue
		}

		for j, p := range predictions {
			if j >= len(batch) {
				break
			}
			match := batch[j]
			log.Printf("  %s %d - %d %s", match.TeamA, p.TippA, p.TippB, match.TeamB)
			allTipps = append(allTipps, gotipp.TippRequest{
				MatchID: match.ID,
				TippA:   p.TippA,
				TippB:   p.TippB,
			})
		}
	}

	if len(allTipps) == 0 {
		log.Println("No predictions generated.")
		return
	}

	// 5. Submit all tipps in a single API call
	log.Printf("Submitting %d tipp(s)...", len(allTipps))
	resp, err := client.PostTipps(ctx, allTipps)
	if err != nil {
		log.Fatalf("failed to submit tipps: %v", err)
	}

	log.Printf("✓ Submitted %d tipp(s) successfully", resp.Count)
	log.Println("Done!")
}

// BatchPrediction is the structured output from the LLM for a batch of matches.
type BatchPrediction struct {
	Predictions []Prediction `json:"predictions" jsonschema:"description=Array of predictions in the same order as the matches provided"`
}

// Prediction is a single match score prediction.
type Prediction struct {
	TippA int `json:"tipp_a" jsonschema:"description=Predicted goals for team A,minimum=0,maximum=10"`
	TippB int `json:"tipp_b" jsonschema:"description=Predicted goals for team B,minimum=0,maximum=10"`
}

func predictBatch(ctx context.Context, g *genkit.Genkit, matches []gotipp.Match, stats map[string]TeamStats) ([]Prediction, error) {
	// Build match list for the prompt
	var matchList string
	for i, m := range matches {
		aStats := formatStats(m.TeamA, stats)
		bStats := formatStats(m.TeamB, stats)
		matchList += fmt.Sprintf("%d. %s %svs %s %s(type: %s, phase: %d)\n",
			i+1, m.TeamA, aStats, m.TeamB, bStats, m.MatchType, m.EventPhase)
	}

	prompt := fmt.Sprintf(`You are a football prediction expert. Predict the most likely final score for each of these matches.

Matches:
%s
Use the FIFA rankings and points to gauge relative team strength.
Consider historical performance and typical tournament scoring patterns.
Most football matches end with 0-4 goals per team.
Be decisive — pick the single most likely outcome for each match.
Return predictions in the same order as the matches listed above.`, matchList)

	result, _, err := genkit.GenerateData[BatchPrediction](ctx, g,
		ai.WithPrompt(prompt),
	)
	if err != nil {
		return nil, err
	}

	return result.Predictions, nil
}

// TeamStats holds FIFA ranking data for a team.
type TeamStats struct {
	FIFARank string
	Points   string
}

func loadTeamStats(path string) (map[string]TeamStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]TeamStats)
	for i, row := range records {
		if i == 0 { // skip header
			continue
		}
		if len(row) < 3 {
			continue
		}
		stats[row[0]] = TeamStats{FIFARank: row[1], Points: row[2]}
	}
	return stats, nil
}

func formatStats(team string, stats map[string]TeamStats) string {
	if s, ok := stats[team]; ok {
		return fmt.Sprintf("[FIFA #%s, %s pts] ", s.FIFARank, s.Points)
	}
	return ""
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
