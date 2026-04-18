package evaluate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStorage_StoreAndList(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := OpenStorage(dbPath)
	if err != nil {
		t.Fatalf("OpenStorage: %v", err)
	}
	defer storage.Close()

	r := &Result{
		Source:     "cistern",
		Ticket:    "PROJ-123",
		Branch:    "feat/test",
		Commit:    "abc123",
		PRNumber:  42,
		Model:     "test-model",
		Scores: []Score{
			{Dimension: ContractCorrectness, Score: 4, Evidence: "all good", Suggested: "n/a"},
			{Dimension: IntegrationCoverage, Score: 3, Evidence: "missing some", Suggested: "add tests"},
		},
		TotalScore: 7,
		MaxScore:   40,
		Notes:     "test note",
		Timestamp: "2026-04-17T12:00:00Z",
	}

	id, err := storage.Store(r)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}

	results, err := storage.ListRecent(10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	sr := results[0]
	if sr.Source != "cistern" {
		t.Errorf("expected source 'cistern', got %q", sr.Source)
	}
	if sr.PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", sr.PRNumber)
	}
	if sr.TotalScore != 7 {
		t.Errorf("expected total score 7, got %d", sr.TotalScore)
	}
	if len(sr.Scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(sr.Scores))
	}
	if sr.Scores[0].Dimension != ContractCorrectness {
		t.Errorf("expected first dimension %s, got %s", ContractCorrectness, sr.Scores[0].Dimension)
	}
}

func TestStorage_Trend(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := OpenStorage(dbPath)
	if err != nil {
		t.Fatalf("OpenStorage: %v", err)
	}
	defer storage.Close()

	scores1 := []Score{
		{Dimension: ContractCorrectness, Score: 3, Evidence: "meh", Suggested: "fix"},
		{Dimension: IntegrationCoverage, Score: 2, Evidence: "bad", Suggested: "add"},
	}
	scores2 := []Score{
		{Dimension: ContractCorrectness, Score: 4, Evidence: "better", Suggested: "n/a"},
		{Dimension: IntegrationCoverage, Score: 4, Evidence: "good", Suggested: "n/a"},
	}
	scores3 := []Score{
		{Dimension: ContractCorrectness, Score: 3, Evidence: "ok", Suggested: "n/a"},
		{Dimension: IntegrationCoverage, Score: 3, Evidence: "ok", Suggested: "n/a"},
	}

	scoresJSON1, _ := json.Marshal(scores1)
	scoresJSON2, _ := json.Marshal(scores2)
	scoresJSON3, _ := json.Marshal(scores3)

	storage.db.Exec(`INSERT INTO evaluation_results (source, scores_json, total_score, max_score, notes, evaluated_at)
		VALUES ('cistern', ?, 5, 40, '', '2026-01-01T00:00:00Z')`, string(scoresJSON1))
	storage.db.Exec(`INSERT INTO evaluation_results (source, scores_json, total_score, max_score, notes, evaluated_at)
		VALUES ('cistern', ?, 8, 40, '', '2026-02-01T00:00:00Z')`, string(scoresJSON2))
	storage.db.Exec(`INSERT INTO evaluation_results (source, scores_json, total_score, max_score, notes, evaluated_at)
		VALUES ('vibe-coded', ?, 6, 40, '', '2026-02-15T00:00:00Z')`, string(scoresJSON3))

	points, err := storage.Trend(nil, "")
	if err != nil {
		t.Fatalf("Trend: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 trend points, got %d", len(points))
	}

	cisternPoints, err := storage.Trend([]string{"cistern"}, "")
	if err != nil {
		t.Fatalf("Trend(cistern): %v", err)
	}
	if len(cisternPoints) != 2 {
		t.Fatalf("expected 2 cistern trend points, got %d", len(cisternPoints))
	}

	sincePoints, err := storage.Trend(nil, "2026-02-01")
	if err != nil {
		t.Fatalf("Trend(since): %v", err)
	}
	if len(sincePoints) != 2 {
		t.Fatalf("expected 2 trend points since Feb, got %d", len(sincePoints))
	}
}

func TestStorage_AverageByDimension(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	storage, err := OpenStorage(dbPath)
	if err != nil {
		t.Fatalf("OpenStorage: %v", err)
	}
	defer storage.Close()

	scores1 := []Score{
		{Dimension: ContractCorrectness, Score: 2, Evidence: "", Suggested: ""},
		{Dimension: IntegrationCoverage, Score: 4, Evidence: "", Suggested: ""},
	}
	scores2 := []Score{
		{Dimension: ContractCorrectness, Score: 4, Evidence: "", Suggested: ""},
		{Dimension: IntegrationCoverage, Score: 2, Evidence: "", Suggested: ""},
	}

	scoresJSON1, _ := json.Marshal(scores1)
	scoresJSON2, _ := json.Marshal(scores2)

	storage.db.Exec(`INSERT INTO evaluation_results (source, scores_json, total_score, max_score, notes, evaluated_at)
		VALUES ('cistern', ?, 6, 40, '', '2026-01-01T00:00:00Z')`, string(scoresJSON1))
	storage.db.Exec(`INSERT INTO evaluation_results (source, scores_json, total_score, max_score, notes, evaluated_at)
		VALUES ('cistern', ?, 6, 40, '', '2026-02-01T00:00:00Z')`, string(scoresJSON2))

	avgs, err := storage.AverageByDimension(nil, "")
	if err != nil {
		t.Fatalf("AverageByDimension: %v", err)
	}
	if avgs == nil {
		t.Fatal("expected non-nil averages")
	}

	ccAvg, ok := avgs[ContractCorrectness]
	if !ok {
		t.Fatal("expected contract_correctness average")
	}
	if ccAvg != 3.0 {
		t.Errorf("expected avg 3.0 for contract_correctness, got %.1f", ccAvg)
	}

	icAvg, ok := avgs[IntegrationCoverage]
	if !ok {
		t.Fatal("expected integration_coverage average")
	}
	if icAvg != 3.0 {
		t.Errorf("expected avg 3.0 for integration_coverage, got %.1f", icAvg)
	}
}

func TestStorage_DefaultPath(t *testing.T) {
	storage, err := OpenStorage("")
	if err != nil {
		t.Fatalf("OpenStorage with empty path: %v", err)
	}
	defer storage.Close()

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".cistern", "evaluations.db")

	var dbPath string
	storage.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&dbPath)

	if dbPath != expected {
		t.Errorf("expected db at %s, got %s", expected, dbPath)
	}
}