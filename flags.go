package main

// hasFlag checks if a boolean flag is present in args.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// getFlagValue returns the value of a flag, or defaultVal if not found.
// Supports --flag value format.
func getFlagValue(args []string, flag, defaultVal string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return defaultVal
}

// positionalArgs extracts non-flag arguments from args.
// flagsWithValue lists flags that consume the next arg (e.g., "--gpu").
// Boolean flags (e.g., "--json") are skipped automatically.
func positionalArgs(args []string, flagsWithValue ...string) []string {
	valueFlags := make(map[string]bool)
	for _, f := range flagsWithValue {
		valueFlags[f] = true
	}

	var result []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if valueFlags[arg] {
			skipNext = true
			continue
		}
		if len(arg) > 1 && arg[0] == '-' {
			continue
		}
		result = append(result, arg)
	}
	return result
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
