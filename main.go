package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   any           `json:"format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message chatMessage `json:"message"`
}

type suggestionsPayload struct {
	Messages []string `json:"messages"`
}

func main() {
	var (
		endpoint = flag.String("endpoint", "http://localhost:11434", "Ollama endpoint")
		model    = flag.String("model", "llama3.1", "Ollama model")
		count    = flag.Int("n", 3, "number of suggestions (1-3)")
		timeout  = flag.Duration("timeout", 30*time.Second, "HTTP timeout")
	)
	flag.Parse()

	if *count < 1 {
		*count = 1
	}
	if *count > 3 {
		*count = 3
	}

	if err := ensureGitRepo(); err != nil {
		fatal(err)
	}

	diff, err := stagedDiff()
	if err != nil {
		fatal(err)
	}
	if strings.TrimSpace(diff) == "" {
		fatal(errors.New("no staged changes (git diff --staged is empty)"))
	}

	msgs, err := requestSuggestions(*endpoint, *model, *count, diff, *timeout)
	if err != nil {
		fatal(err)
	}
	if len(msgs) == 0 {
		fatal(errors.New("no suggestions returned"))
	}

	chosen, err := chooseMessage(msgs)
	if err != nil {
		fatal(err)
	}

	if err := gitCommit(chosen); err != nil {
		fatal(err)
	}

	fmt.Println("Committed:", chosen)
}

func ensureGitRepo() error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("not a git repository")
	}
	return nil
}

func stagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git diff --staged failed: %w", err)
	}
	return out.String(), nil
}

func requestSuggestions(endpoint, model string, n int, diff string, timeout time.Duration) ([]string, error) {
	prompt := buildPrompt(n, diff)

	reqBody := ollamaChatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: "You write concise git commit messages."},
			{Role: "user", Content: prompt},
		},
		Stream: false,
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"messages": map[string]any{
					"type": "array",
					"items": map[string]any{"type": "string"},
					"minItems": 1,
					"maxItems": n,
				},
			},
			"required": []string{"messages"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{Timeout: timeout}
	url := strings.TrimRight(endpoint, "/") + "/api/chat"
	resp, err := client.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var out ollamaChatResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	msgs, err := parseSuggestions(out.Message.Content)
	if err != nil {
		return nil, err
	}
	if len(msgs) > n {
		msgs = msgs[:n]
	}
	return msgs, nil
}

func buildPrompt(n int, diff string) string {
	return fmt.Sprintf(
		"Generate %d concise git commit message suggestions based on the staged diff below. "+
			"Use imperative present tense. Return ONLY JSON with shape {\"messages\": [\"...\"]}.\n\n"+
			"Diff:\n%s",
		n,
		diff,
	)
}

func parseSuggestions(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("empty response from model")
	}

	var payload suggestionsPayload
	if err := json.Unmarshal([]byte(content), &payload); err == nil && len(payload.Messages) > 0 {
		return normalizeMessages(payload.Messages), nil
	}

	// Fallback: split lines
	lines := strings.Split(content, "\n")
	var msgs []string
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(line, "-0123456789. "))
		if line != "" {
			msgs = append(msgs, line)
		}
	}
	if len(msgs) == 0 {
		return nil, errors.New("could not parse suggestions")
	}
	return normalizeMessages(msgs), nil
}

func normalizeMessages(msgs []string) []string {
	out := make([]string, 0, len(msgs))
	seen := map[string]bool{}
	for _, m := range msgs {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

func chooseMessage(msgs []string) (string, error) {
	fmt.Println("Suggestions:")
	for i, m := range msgs {
		fmt.Printf("%d. %s\n", i+1, m)
	}
	fmt.Println("Choose 1-3 or type 'e' to edit:")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read input: %w", err)
		}
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "e") {
			return promptEdit(reader)
		}
		if line == "" {
			continue
		}
		idx, err := parseChoice(line, len(msgs))
		if err != nil {
			fmt.Println("Invalid choice. Try again.")
			continue
		}
		return msgs[idx], nil
	}
}

func parseChoice(s string, max int) (int, error) {
	if len(s) == 0 {
		return 0, errors.New("empty")
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	if n < 1 || n > max {
		return 0, errors.New("out of range")
	}
	return n - 1, nil
}

func promptEdit(reader *bufio.Reader) (string, error) {
	fmt.Println("Enter commit message:")
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read input: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line, nil
	}
}

func gitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
