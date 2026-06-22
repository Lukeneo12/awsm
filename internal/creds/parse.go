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
	for _, raw := range strings.Split(text, "\n") {
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
