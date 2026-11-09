package participant

import "regexp"

var ansiEscapes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// visible is what a user reads: the text without terminal styling codes.
func visible(s string) string { return ansiEscapes.ReplaceAllString(s, "") }
