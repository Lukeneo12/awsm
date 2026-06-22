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
