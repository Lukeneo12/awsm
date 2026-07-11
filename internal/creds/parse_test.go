package creds

import "testing"

func TestParse_formats(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"export-bash", `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretvalue001"
export AWS_SESSION_TOKEN="tokenvalue001"
export AWS_DEFAULT_REGION="us-east-1"`},
		{"export-no-quotes", `
export AWS_ACCESS_KEY_ID=ASIAEXAMPLE0001
export AWS_SECRET_ACCESS_KEY=secretvalue001
export AWS_SESSION_TOKEN=tokenvalue001
export AWS_REGION=us-east-1`},
		{"ini", `
[123456789012_Admin]
aws_access_key_id = ASIAEXAMPLE0001
aws_secret_access_key = secretvalue001
aws_session_token = tokenvalue001
region = us-east-1`},
		{"powershell", `
$env:AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
$Env:AWS_SECRET_ACCESS_KEY='secretvalue001'
$env:AWS_SESSION_TOKEN="tokenvalue001"
$env:AWS_DEFAULT_REGION="us-east-1"`},
		{"cmd", `
set AWS_ACCESS_KEY_ID=ASIAEXAMPLE0001
set AWS_SECRET_ACCESS_KEY=secretvalue001
set AWS_SESSION_TOKEN=tokenvalue001
set AWS_DEFAULT_REGION=us-east-1`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if p.AccessKeyID != "ASIAEXAMPLE0001" {
				t.Errorf("access key: got %q", p.AccessKeyID)
			}
			if p.SecretAccessKey != "secretvalue001" {
				t.Errorf("secret: got %q", p.SecretAccessKey)
			}
			if p.SessionToken != "tokenvalue001" {
				t.Errorf("token: got %q", p.SessionToken)
			}
			if p.Region != "us-east-1" {
				t.Errorf("region: got %q", p.Region)
			}
		})
	}
}

func TestParse_minimal_no_token(t *testing.T) {
	p, err := Parse("AWS_ACCESS_KEY_ID=AKIA1\nAWS_SECRET_ACCESS_KEY=sec")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.SessionToken != "" || p.Region != "" {
		t.Errorf("expected empty token/region, got %+v", p)
	}
}

func TestParse_errors_when_incomplete(t *testing.T) {
	if _, err := Parse("AWS_ACCESS_KEY_ID=AKIA1"); err == nil {
		t.Error("expected error when secret missing")
	}
	if _, err := Parse("garbage\nno creds here"); err == nil {
		t.Error("expected error for non-credential text")
	}
}

func TestParse_ignores_comments_and_blank(t *testing.T) {
	in := `
# AWS SSO credentials
; another comment

export AWS_ACCESS_KEY_ID="AKIA1"
export AWS_SECRET_ACCESS_KEY="sec"
`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.AccessKeyID != "AKIA1" {
		t.Errorf("got %q", p.AccessKeyID)
	}
}

func TestParse_rejoins_wrapped_quoted_token(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"export-two-fragments", `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretvalue001"
export AWS_SESSION_TOKEN="tokenpart1
tokenpart2"`},
		{"export-three-fragments", `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretvalue001"
export AWS_SESSION_TOKEN="tokenpa
rt1token
part2"`},
		{"powershell-single-quotes", `
$env:AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
$env:AWS_SECRET_ACCESS_KEY="secretvalue001"
$env:AWS_SESSION_TOKEN='tokenpart1
tokenpart2'`},
		{"ini-wrapped", `
[dev]
aws_access_key_id = ASIAEXAMPLE0001
aws_secret_access_key = secretvalue001
aws_session_token = "tokenpart1
tokenpart2"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			want := "tokenpart1tokenpart2"
			if p.SessionToken != want {
				t.Errorf("session token: got %q want %q", p.SessionToken, want)
			}
			if p.AccessKeyID != "ASIAEXAMPLE0001" || p.SecretAccessKey != "secretvalue001" {
				t.Errorf("other fields disturbed: %+v", p)
			}
		})
	}
}

func TestParse_wrapped_secret_also_rejoined(t *testing.T) {
	in := `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretpart1
secretpart2"
export AWS_SESSION_TOKEN="tokenvalue001"`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.SecretAccessKey != "secretpart1secretpart2" {
		t.Errorf("secret: got %q", p.SecretAccessKey)
	}
	if p.SessionToken != "tokenvalue001" {
		t.Errorf("token after a joined value: got %q", p.SessionToken)
	}
}

// A wrapped value whose closing-quote line carries trailing junk after the
// closing quote (e.g. a stray shell comment) is cut at the quote:
// joinWrappedValues drops anything past the closing quote on the final
// fragment, so unquote sees a clean surrounding pair and the stored value is
// exactly the rejoined token — never the literal quotes or the junk.
func TestParse_wrapped_value_trailing_junk_after_close_quote_is_cut(t *testing.T) {
	in := "export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE0001\"\n" +
		"export AWS_SECRET_ACCESS_KEY=\"secretvalue001\"\n" +
		"export AWS_SESSION_TOKEN=\"tokenpart1\n" +
		"tokenpart2\" # trailing comment junk\n"
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "tokenpart1tokenpart2"
	if p.SessionToken != want {
		t.Errorf("session token: got %q want %q (quotes/junk must not leak into the stored value)", p.SessionToken, want)
	}
}

// A wrapped region value (not just secret/token) is re-joined the same way.
func TestParse_wrapped_region_value(t *testing.T) {
	in := `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretvalue001"
export AWS_DEFAULT_REGION="us-e
ast-1"`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Region != "us-east-1" {
		t.Errorf("region: got %q want us-east-1", p.Region)
	}
}

// A wrapped single-quoted value in ini format (not just PowerShell) rejoins
// the same way -- the quote-matching logic is format-agnostic.
func TestParse_ini_wrapped_single_quote(t *testing.T) {
	in := `
[dev]
aws_access_key_id = ASIAEXAMPLE0001
aws_secret_access_key = secretvalue001
aws_session_token = 'tokenpart1
tokenpart2'`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.SessionToken != "tokenpart1tokenpart2" {
		t.Errorf("session token: got %q", p.SessionToken)
	}
}

// Two independently wrapped values in the same block are both rejoined,
// regardless of order (secret wraps first, then token).
func TestParse_two_wrapped_values_in_one_block(t *testing.T) {
	in := `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretpart1
secretpart2"
export AWS_SESSION_TOKEN="tokenpart1
tokenpart2"`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.SecretAccessKey != "secretpart1secretpart2" {
		t.Errorf("secret: got %q", p.SecretAccessKey)
	}
	if p.SessionToken != "tokenpart1tokenpart2" {
		t.Errorf("token: got %q", p.SessionToken)
	}
}

// A wrapped value's continuation lines may carry CRLF line endings (e.g. a
// block pasted from a Windows-authored source): strings.TrimSpace strips the
// trailing \r along with other whitespace on every fragment, so the joined
// value is unaffected by the line-ending style.
func TestParse_crlf_wrapped_block(t *testing.T) {
	in := "export AWS_ACCESS_KEY_ID=\"ASIAEXAMPLE0001\"\r\n" +
		"export AWS_SECRET_ACCESS_KEY=\"secretvalue001\"\r\n" +
		"export AWS_SESSION_TOKEN=\"tokenpart1\r\n" +
		"tokenpart2\"\r\n"
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.SessionToken != "tokenpart1tokenpart2" {
		t.Errorf("session token: got %q (CRLF may have leaked into the value)", p.SessionToken)
	}
}

// A value that is just a single quote character with nothing after it never
// finds a closing quote (openQuote treats it as needing a join, but no
// following line closes it since there IS no following line), so
// joinWrappedValues leaves the line untouched and unquote's len>=2 guard
// declines to touch it -- the field is stored as the literal quote
// character. This is degenerate input (a token that is just `"`), not
// something a real paste produces, and is documented here as current
// behavior rather than a bug: the alternative (silently dropping the field)
// would be equally arbitrary.
func TestParse_lone_quote_value_stored_literally(t *testing.T) {
	in := `
export AWS_ACCESS_KEY_ID="ASIAEXAMPLE0001"
export AWS_SECRET_ACCESS_KEY="secretvalue001"
export AWS_SESSION_TOKEN="`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.SessionToken != `"` {
		t.Errorf("expected the lone quote stored literally, got %q", p.SessionToken)
	}
}

func TestParse_unclosed_quote_degrades_to_per_line(t *testing.T) {
	// The quote never closes: the malformed line must not swallow the rest of
	// the block, and the fields on the following lines still parse.
	in := `
export AWS_SESSION_TOKEN="neverclosed
export AWS_ACCESS_KEY_ID=ASIAEXAMPLE0001
export AWS_SECRET_ACCESS_KEY=secretvalue001`
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.AccessKeyID != "ASIAEXAMPLE0001" || p.SecretAccessKey != "secretvalue001" {
		t.Errorf("fields after the malformed line lost: %+v", p)
	}
}

// A comment line containing '=' and an unclosed quote must not open a join
// that swallows the real credential lines below it — joinWrappedValues skips
// exactly the lines Parse skips (external review of PR #8, finding 1).
func TestParse_comment_with_unclosed_quote_does_not_swallow_creds(t *testing.T) {
	in := "; note: token=\"do not share\n" +
		"aws_access_key_id = \"ASIAEXAMPLE0001\"\n" +
		"aws_secret_access_key = secretvalue001\n"
	p, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.AccessKeyID != "ASIAEXAMPLE0001" || p.SecretAccessKey != "secretvalue001" {
		t.Errorf("comment line swallowed credential lines: %+v", p)
	}
}
