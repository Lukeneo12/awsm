// Command awsm is a local manager for AWS credentials. It unifies switching,
// logging in (SSO / SAML / assume-role), inspecting session status, and
// managing static keys across the profiles in ~/.aws.
package main

import (
	"fmt"
	"os"

	"github.com/Lukeneo12/awsm/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
