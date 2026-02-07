# gocommit

Small CLI that uses Ollama to generate commit message suggestions from your staged diff.

## Requirements

- `git`
- `ollama` running locally (default endpoint `http://localhost:11434`)

---

## Install

```bash
go install github.com/NotWilson1993/gocommit@latest
```

---

## Usage

1. Stage your changes:
```bash
git add -A
```

2. Run `gocommit`:
```bash
gocommit
```

3. Pick a suggestion (1-3) or type `e` to edit.

---

## Flags

- `-endpoint` Ollama endpoint (default `http://localhost:11434`)
- `-model` Ollama model (default `llama3.1`)
- `-n` number of suggestions (1-3)
- `-timeout` HTTP timeout (default 30s)

---

## How It Works

- Reads `git diff --staged`
- Asks Ollama for 1-3 commit suggestions
- You pick or edit
- It runs `git commit -m "..."`

---

## Notes

- If there are no staged changes, the tool exits with an error.
- Suggestions are free-form (no Conventional Commits enforced).
