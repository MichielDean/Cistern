package evaluate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const evalSchema = `
CREATE TABLE IF NOT EXISTS evaluation_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    ticket TEXT DEFAULT '',
    branch TEXT DEFAULT '',
    "commit" TEXT DEFAULT '',
    pr_number INTEGER DEFAULT 0,
    model TEXT DEFAULT '',
    scores_json TEXT NOT NULL,
    total_score INTEGER NOT NULL,
    max_score INTEGER NOT NULL,
    notes TEXT DEFAULT '',
    evaluated_at TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_eval_results_source ON evaluation_results(source);
CREATE INDEX IF NOT EXISTS idx_eval_results_pr ON evaluation_results(pr_number);
CREATE INDEX IF NOT EXISTS idx_eval_results_evaluated ON evaluation_results(evaluated_at);
`

type Storage struct {
	db *sql.DB
}

func OpenStorage(dbPath string) (*Storage, error) {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dir := filepath.Join(home, ".cistern")
		os.MkdirAll(dir, 0o755)
		dbPath = filepath.Join(dir, "evaluations.db")
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open eval db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(evalSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init eval schema: %w", err)
	}
	return &Storage{db: db}, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) Store(r *Result) (int64, error) {
	scoresJSON, err := json.Marshal(r.Scores)
	if err != nil {
		return 0, fmt.Errorf("marshal scores: %w", err)
	}
	ts := r.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	var prNum int
	if r.PRNumber != 0 {
		prNum = r.PRNumber
	}
	res, err := s.db.Exec(`
		INSERT INTO evaluation_results
			(source, ticket, branch, "commit", pr_number, model, scores_json, total_score, max_score, notes, evaluated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Source, r.Ticket, r.Branch, r.Commit, prNum, r.Model,
		string(scoresJSON), r.TotalScore, r.MaxScore, r.Notes, ts)
	if err != nil {
		return 0, fmt.Errorf("insert eval result: %w", err)
	}
	return res.LastInsertId()
}

type StoredResult struct {
	ID          int64
	Source      string
	Ticket      string
	Branch      string
	Commit      string
	PRNumber    int
	Model       string
	Scores      []Score
	TotalScore  int
	MaxScore    int
	Notes       string
	EvaluatedAt string
}

func (s *Storage) ListRecent(limit int) ([]StoredResult, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, source, ticket, branch, "commit", pr_number, model,
		       scores_json, total_score, max_score, notes, evaluated_at
		FROM evaluation_results
		ORDER BY evaluated_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query eval results: %w", err)
	}
	defer rows.Close()
	var results []StoredResult
	for rows.Next() {
		var sr StoredResult
		var scoresJSON string
		if err := rows.Scan(
			&sr.ID, &sr.Source, &sr.Ticket, &sr.Branch, &sr.Commit, &sr.PRNumber,
			&sr.Model, &scoresJSON, &sr.TotalScore, &sr.MaxScore, &sr.Notes, &sr.EvaluatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan eval result: %w", err)
		}
		if err := json.Unmarshal([]byte(scoresJSON), &sr.Scores); err != nil {
			return nil, fmt.Errorf("unmarshal scores for result %d: %w", sr.ID, err)
		}
		results = append(results, sr)
	}
	return results, nil
}

type TrendPoint struct {
	EvaluatedAt string
	Source      string
	TotalScore  int
	MaxScore    int
	Percentage  float64
	Scores      map[Dimension]int
}

func (s *Storage) Trend(sources []string, since string) ([]TrendPoint, error) {
	query := `
		SELECT evaluated_at, source, scores_json, total_score, max_score
		FROM evaluation_results
		WHERE 1=1`
	args := []any{}
	if len(sources) > 0 {
		placeholders := ""
		for i, src := range sources {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, src)
		}
		query += fmt.Sprintf(" AND source IN (%s)", placeholders)
	}
	if since != "" {
		query += " AND evaluated_at >= ?"
		args = append(args, since)
	}
	query += " ORDER BY evaluated_at ASC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query trend: %w", err)
	}
	defer rows.Close()
	var points []TrendPoint
	for rows.Next() {
		var tp TrendPoint
		var scoresJSON string
		if err := rows.Scan(&tp.EvaluatedAt, &tp.Source, &scoresJSON, &tp.TotalScore, &tp.MaxScore); err != nil {
			return nil, fmt.Errorf("scan trend: %w", err)
		}
		var scores []Score
		if err := json.Unmarshal([]byte(scoresJSON), &scores); err != nil {
			return nil, fmt.Errorf("unmarshal scores: %w", err)
		}
		tp.Scores = make(map[Dimension]int)
		for _, sc := range scores {
			tp.Scores[sc.Dimension] = sc.Score
		}
		if tp.MaxScore > 0 {
			tp.Percentage = float64(tp.TotalScore) / float64(tp.MaxScore) * 100
		}
		points = append(points, tp)
	}
	return points, nil
}

func (s *Storage) AverageByDimension(sources []string, since string) (map[Dimension]float64, error) {
	points, err := s.Trend(sources, since)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, nil
	}
	sums := make(map[Dimension]float64)
	counts := make(map[Dimension]int)
	for _, tp := range points {
		for dim, score := range tp.Scores {
			sums[dim] += float64(score)
			counts[dim]++
		}
	}
	avgs := make(map[Dimension]float64)
	for dim, sum := range sums {
		if counts[dim] > 0 {
			avgs[dim] = sum / float64(counts[dim])
		}
	}
	return avgs, nil
}