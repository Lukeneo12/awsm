// Package shell produces the shell-side glue: the `eval` snippet that exports
// AWS_PROFILE in the user's current shell, and the wrapper function the user
// installs in their rc file.
package shell

import (
	"fmt"
	"strings"

	"github.com/Lukeneo12/awsm/internal/profiles"
)

// SupportedShells are the shells shell-init can emit a wrapper for.
var SupportedShells = []string{"zsh", "bash", "fish"}

// ExportSnippet returns the lines a shell should `eval` to make the profile
// active in the current session. It sets AWS_PROFILE (and AWS_REGION when the
// profile declares a region) for POSIX shells; fish uses `set -gx`.
func ExportSnippet(p profiles.Profile, shellName string) string {
	if shellName == "fish" {
		var b strings.Builder
		fmt.Fprintf(&b, "set -gx AWS_PROFILE %s\n", p.Name)
		if p.Region != "" {
			fmt.Fprintf(&b, "set -gx AWS_REGION %s\n", p.Region)
		}
		return b.String()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "export AWS_PROFILE=%s\n", p.Name)
	if p.Region != "" {
		fmt.Fprintf(&b, "export AWS_REGION=%s\n", p.Region)
	}
	return b.String()
}

// Wrapper returns the function to add to the user's rc file. It intercepts the
// subcommands that must affect the current shell (switch) and `eval`s their
// output; everything else is passed straight through to the binary.
func Wrapper(shellName string) (string, error) {
	switch shellName {
	case "zsh", "bash":
		return `awsm() {
  case "$1" in
    switch)
      eval "$(command awsm switch "${@:2}")"
      ;;
    "")
      # TUI: capture the profile chosen with 's' and apply it here.
      local _awsm_sf
      _awsm_sf="$(mktemp -t awsm-switch.XXXXXX)"
      command awsm --switch-file "$_awsm_sf"
      if [ -s "$_awsm_sf" ]; then
        eval "$(command awsm switch "$(cat "$_awsm_sf")")"
      fi
      rm -f "$_awsm_sf"
      ;;
    *)
      command awsm "$@"
      ;;
  esac
}
`, nil
	case "fish":
		return `function awsm
  switch "$argv[1]"
    case switch
      eval (command awsm switch $argv[2..-1])
    case ''
      set -l _awsm_sf (mktemp -t awsm-switch.XXXXXX)
      command awsm --switch-file "$_awsm_sf"
      if test -s "$_awsm_sf"
        eval (command awsm switch (cat "$_awsm_sf"))
      end
      rm -f "$_awsm_sf"
    case '*'
      command awsm $argv
  end
end
`, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", shellName, strings.Join(SupportedShells, ", "))
	}
}
