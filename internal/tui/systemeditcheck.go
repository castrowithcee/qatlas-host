package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// syntaxProblem is a failed check of an edited text: the format, and the line where it failed, 0 where it is not
// known. It never holds a part of the text, so it may reach the screen of a file only root may read.
type syntaxProblem struct {
	format string
	line   int
}

// checkSyntax checks text by the extension of name: YAML, JSON or the .env syntax. Other files pass.
func checkSyntax(name string, text []byte) *syntaxProblem {
	var format string
	var err error
	switch strings.ToLower(path.Ext(name)) {
	case ".yaml", ".yml":
		format, err = "YAML", checkYAML(text)
	case ".json":
		format = "JSON"
		var v any
		if err = json.Unmarshal(text, &v); err != nil {
			return &syntaxProblem{format: format, line: jsonLine(text, err)}
		}
	case ".env":
		if line := envProblemLine(text); line > 0 {
			return &syntaxProblem{format: ".env", line: line}
		}
	}
	if err == nil {
		return nil
	}
	return &syntaxProblem{format: format, line: errorLine(err)}
}

// checkYAML decodes every document of text.
func checkYAML(text []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(text))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

var lineNumber = regexp.MustCompile(`\bline (\d+)\b`)

// errorLine is the line an error of the YAML or .env parser names; only the number is taken, as the rest of such a
// message may quote the text.
func errorLine(err error) int {
	if m := lineNumber.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// jsonLine is the line of the offset a JSON syntax error names.
func jsonLine(text []byte, err error) int {
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return 0
	}
	return bytes.Count(text[:min(int(se.Offset), len(text))], []byte("\n")) + 1
}

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// envProblemLine is the first line that is no KEY=VALUE statement, comment or blank, or whose quoted value is not
// closed; 0 when every line is one. The interface may not read stacks itself, so this checks only the form of a
// line, not the values a stack reads from it.
func envProblemLine(text []byte) int {
	for i, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		value = strings.TrimSpace(value)
		if !ok || !envKey.MatchString(strings.TrimSpace(key)) || !closedQuote(value) {
			return i + 1
		}
	}
	return 0
}

// closedQuote reports a value that is unquoted or whose opening quote is closed on the same line; a double quote
// may be escaped with a backslash.
func closedQuote(value string) bool {
	if value == "" || (value[0] != '\'' && value[0] != '"') {
		return true
	}
	for i := 1; i < len(value); i++ {
		switch {
		case value[0] == '"' && value[i] == '\\':
			i++
		case value[i] == value[0]:
			return true
		}
	}
	return false
}
