// Package creds parses pasted AWS credential blocks in the various formats AWS
// hands out (shell export, ini, PowerShell, Windows cmd) into structured values.
package creds

import (
	"fmt"
	"strings"
)

// Parsed holds the credential fields extracted from a pasted block.
type Parsed struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
}

// Parse auto-detects the format of a pasted credentials block and extracts the
// fields. It tolerates `export `, `set `, `$env:`/`$Env:` prefixes, surrounding
// quotes, `[section]` headers, comments and blank lines. It errors when neither
// an access key id nor a secret can be found.
func Parse(text string) (Parsed, error) {
	var p Parsed
	for _, raw := range joinWrappedValues(strings.Split(text, "\n")) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue // ini section header
		}
		line = stripPrefix(line)

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := unquote(strings.TrimSpace(line[eq+1:]))
		if val == "" {
			continue
		}

		switch key {
		case "aws_access_key_id":
			p.AccessKeyID = val
		case "aws_secret_access_key":
			p.SecretAccessKey = val
		case "aws_session_token", "aws_security_token":
			p.SessionToken = val
		case "aws_default_region", "aws_region", "region":
			p.Region = val
		}
	}

	if p.AccessKeyID == "" || p.SecretAccessKey == "" {
		return Parsed{}, fmt.Errorf("could not find an access key id and secret access key in the pasted text")
	}
	return p, nil
}

// joinWrappedValues re-joins values that were line-wrapped inside an open
// quote when the block was copied (e.g. a long session token soft-wrapped by
// the terminal or portal page). A KEY="VALUE… line whose quote never closes on
// the same line is concatenated with the following lines until the closing
// quote appears — dropping the newlines and edge whitespace, since the wrap
// inserted them and the original value had none. A quote that never closes
// leaves the lines untouched, so malformed input degrades to the previous
// per-line behavior instead of swallowing the rest of the block.
func joinWrappedValues(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		stripped := stripPrefix(strings.TrimSpace(lines[i]))
		// Skip exactly what Parse skips (blanks, comments, section headers):
		// a comment like `; token="...` must not open a join that swallows
		// the real credential lines below it.
		if stripped == "" || strings.HasPrefix(stripped, "#") || strings.HasPrefix(stripped, ";") ||
			(strings.HasPrefix(stripped, "[") && strings.HasSuffix(stripped, "]")) {
			out = append(out, lines[i])
			continue
		}
		eq := strings.IndexByte(stripped, '=')
		if eq < 0 {
			out = append(out, lines[i])
			continue
		}
		q := openQuote(strings.TrimSpace(stripped[eq+1:]))
		if q == 0 {
			out = append(out, lines[i])
			continue
		}
		// Accumulate in a Builder: += in a loop is quadratic, and a crafted
		// paste of many short fragments inside one quote (bounded only by the
		// 1 MiB read cap) would burn seconds of memcpy.
		var joined strings.Builder
		joined.WriteString(stripped)
		closed := false
		j := i + 1
		for ; j < len(lines); j++ {
			frag := strings.TrimSpace(lines[j])
			// Cut the closing fragment at the quote: anything after it (a
			// stray comment, shell noise) is not part of the value, and
			// keeping it would defeat unquote's surrounding-pair strip.
			if k := strings.IndexByte(frag, q); k >= 0 {
				joined.WriteString(frag[:k+1])
				closed = true
				break
			}
			joined.WriteString(frag)
		}
		if !closed {
			out = append(out, lines[i])
			continue
		}
		out = append(out, joined.String())
		i = j
	}
	return out
}

// openQuote returns the quote byte opening val when that quote is not closed
// on the same line, or 0 when the value needs no joining.
func openQuote(val string) byte {
	if val == "" || (val[0] != '"' && val[0] != '\'') {
		return 0
	}
	if strings.IndexByte(val[1:], val[0]) >= 0 {
		return 0 // opens and closes on the same line
	}
	return val[0]
}

// stripPrefix removes a leading shell/PowerShell/cmd assignment prefix so every
// format reduces to "KEY = VALUE".
func stripPrefix(line string) string {
	for _, pre := range []string{"export ", "set ", "setx ", "$env:", "$Env:", "$ENV:"} {
		if len(line) >= len(pre) && strings.EqualFold(line[:len(pre)], pre) {
			return strings.TrimSpace(line[len(pre):])
		}
	}
	return line
}

// unquote trims a single pair of surrounding single or double quotes and any
// trailing shell noise after the value (e.g. a stray comment).
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
