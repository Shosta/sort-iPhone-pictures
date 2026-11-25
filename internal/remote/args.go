package remote

import (
	"strings"
)

// Safety net: every rsync command line goes through checkArgs. Photosort only
// ever adds files: it never deletes anything, locally or remotely, and never
// removes a source file. Any option outside this list is refused, so a future
// change cannot introduce --delete* or --remove-source-files by accident.
var allowedFlags = map[string]bool{
	"-rLt":                         true, // upload: recurse, follow links, keep times
	"-rLn":                         true, // checksum verification (dry run)
	"-r":                           true,
	"-d":                           true,
	"-t":                           true,
	"-8":                           true,
	"-e":                           true,
	"--list-only":                  true,
	"--ignore-existing":            true,
	"--partial-dir=.rsync-partial": true,
	"--out-format=%n":              true,
	"--chmod=D755,F644":            true,
	"--mkpath":                     true,
	"--dry-run":                    true,
	"--info=progress2":             true,
	"--progress":                   true,
	"--checksum":                   true,
	"--no-motd":                    true,
	"--timeout=60":                 true,
	"--contimeout=15":              true,
}

var allowedPrefixes = []string{"--exclude=", "--password-file="}

// checkArgs refuses any option that is not explicitly allowed.
func checkArgs(args []string) error {
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") || allowedFlags[arg] {
			continue
		}
		// The value of -e (the ssh command) is not an option.
		if i > 0 && args[i-1] == "-e" {
			continue
		}
		ok := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(arg, prefix) {
				ok = true
			}
		}
		if !ok {
			return &Error{Kind: KindConfig, Detail: "refused rsync option " + arg}
		}
	}
	return nil
}
