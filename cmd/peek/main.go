// Command peek decodes the hidden hints, explanations and reference
// solutions in solutions/. They are stored as ROT13 so that a glance at the
// file tree, a grep or an editor preview does not spoil an exercise.
//
// Usage:
//
//	go run ./cmd/peek <exercise> hint <1|2|3>
//	go run ./cmd/peek <exercise> explanation
//	go run ./cmd/peek <exercise> walkthrough
//	go run ./cmd/peek <exercise> solution
//
// <exercise> is the number ("7" or "07") or the directory name ("07-retry").
// Run it from the repository root. From an exercise directory, use
// "go run ../../cmd/peek ..."; peek finds solutions/ from any directory
// inside the repository.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const solutionsDir = "solutions"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "peek:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: peek <exercise> hint <1|2|3> | explanation | walkthrough | solution")
	}
	dir, err := findExercise(args[0])
	if err != nil {
		return err
	}

	switch args[1] {
	case "hint":
		if len(args) != 3 {
			return errors.New("usage: peek <exercise> hint <1|2|3>")
		}
		n, err := strconv.Atoi(args[2])
		if err != nil || n < 1 || n > 3 {
			return fmt.Errorf("hint number must be 1, 2 or 3, got %q", args[2])
		}
		text, err := decodeFile(filepath.Join(dir, "hints.md.rot13"))
		if err != nil {
			return err
		}
		hint, err := section(text, fmt.Sprintf("## Hint %d", n))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, hint)
		return err
	case "explanation", "walkthrough":
		text, err := decodeFile(filepath.Join(dir, args[1]+".md.rot13"))
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, text)
		return err
	case "solution":
		files, err := filepath.Glob(filepath.Join(dir, "*.go.rot13"))
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("no solution files in %s", dir)
		}
		sort.Strings(files)
		for _, f := range files {
			text, err := decodeFile(f)
			if err != nil {
				return err
			}
			name := strings.TrimSuffix(filepath.Base(f), ".rot13")
			if _, err := fmt.Fprintf(out, "// ===== %s =====\n%s\n", name, text); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown part %q: use hint, explanation, walkthrough or solution", args[1])
	}
}

// findExercise maps "7", "07" or "07-retry" to <repo>/solutions/07-retry.
func findExercise(arg string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	prefix := arg
	if n, err := strconv.Atoi(arg); err == nil {
		prefix = fmt.Sprintf("%02d-", n)
	}
	matches, err := filepath.Glob(filepath.Join(root, solutionsDir, prefix+"*"))
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("no single exercise matches %q in %s", arg, filepath.Join(root, solutionsDir))
	}
	return matches[0], nil
}

// repoRoot walks up from the working directory to the first directory that
// has a go.mod and a solutions/ directory. Learners often run commands from
// inside an exercise directory, not from the root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		_, errMod := os.Stat(filepath.Join(dir, "go.mod"))
		info, errSol := os.Stat(filepath.Join(dir, solutionsDir))
		if errMod == nil && errSol == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod with a solutions/ directory next to it in this directory or above: run peek inside the go-practice repository")
		}
		dir = parent
	}
}

// section returns the text from heading up to the next heading of the same level.
func section(text, heading string) (string, error) {
	start := strings.Index(text, heading)
	if start < 0 {
		return "", fmt.Errorf("%q not found", heading)
	}
	rest := text[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return heading + strings.TrimRight(rest, "\n"), nil
}

func decodeFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return rot13(string(b)), nil
}

// rot13 rotates ASCII letters by 13 places. Applying it twice gives back the
// input, so the same function encodes and decodes.
func rot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}
