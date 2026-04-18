package evaluate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DiffSource int

const (
	DiffFromBranches DiffSource = iota
	DiffFromPR
	DiffFromRaw
)

type DiffInput struct {
	Source     DiffSource
	BaseBranch string
	HeadBranch string
	PRNumber   int
	RawDiff    string
	WorkDir    string
}

func (d DiffInput) GetDiff() (string, error) {
	switch d.Source {
	case DiffFromBranches:
		return d.getBranchDiff()
	case DiffFromPR:
		return d.getPRDiff()
	case DiffFromRaw:
		return d.RawDiff, nil
	default:
		return "", fmt.Errorf("unknown diff source: %d", d.Source)
	}
}

func (d DiffInput) getBranchDiff() (string, error) {
	if d.BaseBranch == "" {
		d.BaseBranch = "main"
	}
	if d.HeadBranch == "" {
		return "", fmt.Errorf("head branch is required for branch diff")
	}
	mergeBase, err := exec.Command("git", "merge-base", d.HeadBranch, d.BaseBranch).Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base: %w", err)
	}
	cmd := exec.Command("git", "diff", strings.TrimSpace(string(mergeBase))+".."+d.HeadBranch)
	if d.WorkDir != "" {
		cmd.Dir = d.WorkDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}

func (d DiffInput) getPRDiff() (string, error) {
	if d.PRNumber == 0 {
		return "", fmt.Errorf("PR number is required for PR diff")
	}
	cmd := exec.Command("gh", "pr", "diff", fmt.Sprintf("%d", d.PRNumber))
	if d.WorkDir != "" {
		cmd.Dir = d.WorkDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh pr diff: %w", err)
	}
	return string(out), nil
}

// Caller is the interface for invoking an LLM.
type Caller interface {
	Call(prompt string) (string, error)
	ModelName() string
}

// LLMCaller invokes an LLM via CLI in non-interactive mode.
type LLMCaller struct {
	Command           string
	Args              []string
	PrintFlag         string
	PromptFlag        string
	ModelFlag         string
	Model             string
	AllowedToolsFlag  string
	WorkDir           string
}

func NewLLMCaller(command string, args []string, printFlag, promptFlag, modelFlag, model, allowedToolsFlag, workDir string) *LLMCaller {
	return &LLMCaller{
		Command:          command,
		Args:            args,
		PrintFlag:       printFlag,
		PromptFlag:      promptFlag,
		ModelFlag:       modelFlag,
		Model:           model,
		AllowedToolsFlag: allowedToolsFlag,
		WorkDir:         workDir,
	}
}

func (c *LLMCaller) ModelName() string { return c.Model }

func (c *LLMCaller) Call(prompt string) (string, error) {
	resolvedCmd, err := exec.LookPath(c.Command)
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			fallback := filepath.Join(home, ".local", "bin", c.Command)
			if _, statErr := os.Stat(fallback); statErr == nil {
				resolvedCmd = fallback
			} else {
				return "", fmt.Errorf("LLM command %q not found in PATH or ~/.local/bin: %w", c.Command, err)
			}
		} else {
			return "", fmt.Errorf("LLM command %q not found: %w", c.Command, err)
		}
	}

	parts := []string{resolvedCmd}
	parts = append(parts, c.Args...)

	if c.Model != "" && c.ModelFlag != "" {
		parts = append(parts, c.ModelFlag, c.Model)
	}

	if c.PrintFlag != "" {
		parts = append(parts, c.PrintFlag)
	}

	if c.AllowedToolsFlag != "" {
		parts = append(parts, c.AllowedToolsFlag, "Glob,Grep,Read")
	}

	if c.PromptFlag != "" {
		parts = append(parts, c.PromptFlag)
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("LLM call failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// APICaller invokes an LLM via an OpenAI-compatible HTTP API.
type APICaller struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

func NewAPICaller(baseURL, model string) *APICaller {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	if model == "" {
		model = "glm-5.1:cloud"
	}
	return &APICaller{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Model:      model,
		HTTPClient: &http.Client{Timeout: 30 * time.Minute},
	}
}

func (c *APICaller) ModelName() string { return c.Model }

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Tools    json.RawMessage `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content    string     `json:"content"`
			Reasoning  string     `json:"reasoning,omitempty"`
			ToolCalls  []toolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *APICaller) Call(prompt string) (string, error) {
	reqBody := chatRequest{
		Model:    c.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.HTTPClient.Post(
		c.BaseURL+"/chat/completions",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", fmt.Errorf("API call to %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w\n\nRaw: %s", err, string(body))
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("API returned no choices")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

func (c *APICaller) CallWithTools(prompt string, tools []Tool, workDir string) (string, error) {
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("marshal tools: %w", err)
	}

	messages := []chatMessage{
		{Role: "user", Content: prompt},
	}

	const maxRounds = 15
	const maxContextChars = 120000
	for round := 0; round < maxRounds; round++ {
		if round == 8 {
			messages = append(messages, chatMessage{
				Role:    "user",
				Content: "IMPORTANT: Stop using tools now. You have enough information. Produce your final output immediately without any more tool calls.",
			})
		}

		reqBody := chatRequest{
			Model:    c.Model,
			Messages: messages,
			Stream:   false,
		}
		if round < 8 {
			reqBody.Tools = toolsJSON
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			return "", fmt.Errorf("marshal request: %w", err)
		}

		resp, err := c.HTTPClient.Post(
			c.BaseURL+"/chat/completions",
			"application/json",
			bytes.NewReader(data),
		)
		if err != nil {
			// Retry on connection errors
			if round < 3 {
				continue
			}
			return "", fmt.Errorf("API call (round %d): %w", round+1, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("read response (round %d): %w", round+1, err)
		}
		if resp.StatusCode == 500 && round < 3 {
			// Ollama transient error — retry
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("API returned %d (round %d): %s", resp.StatusCode, round+1, string(body))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return "", fmt.Errorf("parse response (round %d): %w\n\nRaw: %s", round+1, err, string(body))
		}
		if chatResp.Error != nil {
			return "", fmt.Errorf("API error (round %d): %s", round+1, chatResp.Error.Message)
		}
		if len(chatResp.Choices) == 0 {
			return "", fmt.Errorf("no choices (round %d)", round+1)
		}

		choice := chatResp.Choices[0]
		msg := choice.Message

		if len(msg.ToolCalls) == 0 || choice.FinishReason == "stop" {
			return strings.TrimSpace(msg.Content), nil
		}

		// After the "stop exploring" message, ignore tool calls and return content
		if round >= 8 {
			content := strings.TrimSpace(msg.Content)
			if content != "" {
				return content, nil
			}
			// Model still tried to call tools — execute them but this is the last round
			for _, tc := range msg.ToolCalls {
				result := executeToolCall(tc, workDir)
				if result == "" {
					result = "(empty)"
				}
				truncLen := len(result)
				if truncLen > 500 {
					truncLen = 500
				}
				messages = append(messages, chatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result[:truncLen],
				})
			}
			// One more chance next round, but force return after that
			if round >= maxRounds-2 {
				return "Agent exceeded exploration limit. Partial output:\n" + content, nil
			}
			continue
		}

		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		for _, tc := range msg.ToolCalls {
			result := executeToolCall(tc, workDir)
			content := result
			if content == "" {
				content = "(empty result)"
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    content,
			})
		}

		messages = trimMessages(messages, maxContextChars)
	}

	return "", fmt.Errorf("exceeded max tool-calling rounds (%d)", maxRounds)
}

func trimMessages(messages []chatMessage, maxChars int) []chatMessage {
	if len(messages) <= 2 {
		return messages
	}
	totalChars := 0
	for _, m := range messages {
		s, _ := json.Marshal(m)
		totalChars += len(s)
	}
	if totalChars <= maxChars {
		return messages
	}

	trimmed := make([]chatMessage, 0, len(messages))
	trimmed = append(trimmed, messages[0])

	for i := 1; i < len(messages); i++ {
		m := messages[i]
		content, _ := json.Marshal(m)
		if len(content) > 2000 {
			var m2 chatMessage
			json.Unmarshal(content, &m2)
			if str, ok := m2.Content.(string); ok && len(str) > 1500 {
				m2.Content = str[:1500] + "\n... (truncated for context limits)"
			}
			trimmed = append(trimmed, m2)
		} else {
			trimmed = append(trimmed, m)
		}
	}

	totalChars = 0
	for _, m := range trimmed {
		s, _ := json.Marshal(m)
		totalChars += len(s)
	}
	if totalChars <= maxChars {
		return trimmed
	}

	result := []chatMessage{messages[0]}
	result = append(result, messages[len(messages)-4:]...)
	return result
}

func executeToolCall(tc toolCall, workDir string) string {
	var args struct {
		FilePath string `json:"file_path"`
		Pattern  string `json:"pattern"`
		Path     string `json:"path"`
	}
	json.Unmarshal([]byte(tc.Function.Arguments), &args)

	switch tc.Function.Name {
	case "Read":
		if args.FilePath == "" {
			return "error: file_path is required"
		}
		cmd := exec.Command("cat", args.FilePath)
		cmd.Dir = workDir
		out, err := cmd.Output()
		if err != nil {
			return fmt.Sprintf("error reading %s: %v", args.FilePath, err)
		}
		content := string(out)
		if len(content) > 5000 {
			content = content[:5000] + "\n... (truncated)"
		}
		return content

	case "Glob":
		if args.Pattern == "" {
			return "error: pattern is required"
		}
		searchPath := args.Path
		if searchPath == "" {
			searchPath = "."
		}
		cmd := exec.Command("find", searchPath, "-name", args.Pattern, "-type", "f")
		cmd.Dir = workDir
		out, err := cmd.Output()
		if err != nil {
			return fmt.Sprintf("error globbing: %v", err)
		}
		return strings.TrimSpace(string(out))

	case "Grep":
		if args.Pattern == "" {
			return "error: pattern is required"
		}
		grepArgs := []string{"-rn", "--include=*.go", args.Pattern}
		if args.Path != "" {
			grepArgs = append(grepArgs, args.Path)
		}
		cmd := exec.Command("grep", grepArgs...)
		cmd.Dir = workDir
		out, _ := cmd.Output()
		result := strings.TrimSpace(string(out))
		if len(result) > 5000 {
			result = result[:5000] + "\n... (truncated)"
		}
		return result

	default:
		return fmt.Sprintf("error: unknown tool %s", tc.Function.Name)
	}
}

// AutoCaller detects the best available LLM caller: tries OpenAI-compatible
// API first, then falls back to CLI-based caller.
func AutoCaller(model string) (Caller, error) {
	apiCaller := NewAPICaller("", model)
	resp, err := apiCaller.HTTPClient.Get(apiCaller.BaseURL + "/models")
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return apiCaller, nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	return nil, fmt.Errorf("no LLM available: OpenAI-compatible API not reachable at %s", apiCaller.BaseURL)
}

func Evaluate(diff string, model string, source string, ticket string, branch string, commit string) (*Result, error) {
	if diff == "" {
		return nil, fmt.Errorf("diff is empty -- nothing to evaluate")
	}

	result := &Result{
		Source:     source,
		Ticket:     ticket,
		Branch:     branch,
		Commit:     commit,
		Model:      model,
		Scores:     []Score{},
		TotalScore: 0,
		MaxScore:   len(AllDimensions()) * 5,
		Notes:      "Evaluation not yet implemented -- rubric and scoring structure is defined",
	}

	return result, nil
}

func EvaluateWithLLM(diff string, caller Caller, source string, ticket string, branch string, commit string) (*Result, error) {
	if diff == "" {
		return nil, fmt.Errorf("diff is empty -- nothing to evaluate")
	}
	if caller == nil {
		return nil, fmt.Errorf("LLM caller is required")
	}

	prompt := ScoringPrompt() + "\n\n## Diff to evaluate:\n\n```\n" + diff + "\n```"

	response, err := caller.Call(prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM evaluation call failed: %w", err)
	}

	result, err := ParseEvaluationResult(response)
	if err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w\n\nRaw response:\n%s", err, response)
	}

	result.Source = source
	result.Ticket = ticket
	result.Branch = branch
	result.Commit = commit
	result.Model = caller.ModelName()

	return result, nil
}

func FormatComparison(cisternResult, vibeResult *Result) string {
	var sb strings.Builder

	sb.WriteString("# Pipeline Effectiveness Comparison\n\n")

	dims := AllDimensions()
	sb.WriteString("| Dimension | Cistern | Vibe-coded | Delta |\n")
	sb.WriteString("|---|---|---|---|\n")

	cisternScores := scoresByDimension(cisternResult)
	vibeScores := scoresByDimension(vibeResult)

	for _, d := range dims {
		cs := cisternScores[d]
		vs := vibeScores[d]
		delta := cs - vs
		deltaStr := fmt.Sprintf("%+d", delta)
		if delta > 0 {
			deltaStr = fmt.Sprintf("**+%d**", delta)
		} else if delta < 0 {
			deltaStr = fmt.Sprintf("**%d**", delta)
		}
		sb.WriteString(fmt.Sprintf("| %s | %d/5 | %d/5 | %s |\n", d, cs, vs, deltaStr))
	}

	cs := cisternResult.TotalScore
	vs := vibeResult.TotalScore
	delta := cs - vs
	sb.WriteString(fmt.Sprintf("\n| **Total** | **%d/%d** | **%d/%d** | **%+d** |\n",
		cs, cisternResult.MaxScore, vs, vibeResult.MaxScore, delta))

	sb.WriteString(fmt.Sprintf("\nCistern: %.0f%% | Vibe-coded: %.0f%%\n",
		cisternResult.Percentage(), vibeResult.Percentage()))

	if cisternResult.Notes != "" || vibeResult.Notes != "" {
		sb.WriteString("\n## Cistern Notes\n\n")
		sb.WriteString(cisternResult.Notes + "\n")
		sb.WriteString("\n## Vibe-coded Notes\n\n")
		sb.WriteString(vibeResult.Notes + "\n")
	}

	return sb.String()
}

func scoresByDimension(r *Result) map[Dimension]int {
	m := make(map[Dimension]int)
	for _, s := range r.Scores {
		m[s.Dimension] = s.Score
	}
	return m
}

func MarshalForStorage(r *Result) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func ParseEvaluationResult(body string) (*Result, error) {
	var result Result
	start := strings.Index(body, "{")
	if start < 0 {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	jsonStr := extractJSON(body[start:])
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parsing evaluation result: %w", err)
	}

	validDims := make(map[Dimension]bool)
	for _, d := range AllDimensions() {
		validDims[d] = true
	}

	totalScore := 0
	for _, s := range result.Scores {
		if !validDims[s.Dimension] {
			return nil, fmt.Errorf("unknown dimension: %s", s.Dimension)
		}
		if s.Score < 0 || s.Score > 5 {
			return nil, fmt.Errorf("score for %s must be 0-5, got %d", s.Dimension, s.Score)
		}
		totalScore += s.Score
	}

	result.TotalScore = totalScore
	result.MaxScore = len(AllDimensions()) * 5

	return &result, nil
}

// extractJSON finds the outermost balanced JSON object starting at the
// beginning of s. It handles nested braces and string-escaped braces.
func extractJSON(s string) string {
	depth := 0
	inString := false
	escape := false
	for i, c := range s {
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inString {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}