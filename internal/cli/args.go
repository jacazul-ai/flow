package cli

import "strings"

// NormalizeFocusAlias rewrites "focus <plan>" shorthand into "focus plan <plan>"
// and appends "show" to a bare "focus", skipping leading global options.
func NormalizeFocusAlias(args []string) []string {
	result := append([]string(nil), args...)
	index := 0
	for index < len(result) {
		if strings.HasPrefix(result[index], "--project-id=") ||
			strings.HasPrefix(result[index], "--taskdata=") ||
			strings.HasPrefix(result[index], "--database-path=") ||
			strings.HasPrefix(result[index], "--session-id=") {
			index++
			continue
		}
		switch result[index] {
		case "-v", "--verbose", "-V", "--version":
			index++
		case "--project-id", "--taskdata", "--database-path", "--session-id":
			index += 2
		default:
			if result[index] != "focus" {
				return result
			}
			if index+1 == len(result) {
				return append(result, "show")
			}
			switch result[index+1] {
			case "show", "plan", "ini", "task", "pop", "clear", "back", "ind", "interest":
				return result
			default:
				if result[index+1] == "" || result[index+1][0] == '-' {
					return result
				}
				return append(result[:index+1], append([]string{"plan"}, result[index+1:]...)...)
			}
		}
	}
	return result
}
