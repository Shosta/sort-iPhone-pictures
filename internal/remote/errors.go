package remote

import (
	"errors"
	"regexp"
	"strings"
)

// Kind classifies a failure so the UI can show an explanation a non-expert
// understands. The values are stable codes, translated by the front end.
type Kind string

const (
	KindConfig      Kind = "config"
	KindNotFound    Kind = "rsyncMissing"
	KindAuth        Kind = "auth"
	KindModule      Kind = "module"
	KindDenied      Kind = "denied"
	KindRefused     Kind = "refused"
	KindHost        Kind = "host"
	KindUnreachable Kind = "unreachable"
	KindHostKey     Kind = "hostKey"
	KindInterrupted Kind = "interrupted"
	KindDiskFull    Kind = "diskFull"
	KindPermission  Kind = "permission"
	KindPath        Kind = "path"
	KindRemoteTool  Kind = "remoteRsync"
	KindProtocol    Kind = "protocol"
	KindUnknown     Kind = "unknown"
)

// Resumable reports whether running the upload again continues where it stopped.
func (k Kind) Resumable() bool {
	switch k {
	case KindRefused, KindHost, KindUnreachable, KindInterrupted, KindUnknown:
		return true
	}
	return false
}

// Error is a classified failure. Detail is the last meaningful line of the
// tool's own message, with secrets removed.
type Error struct {
	Kind   Kind
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return "remote: " + string(e.Kind)
	}
	return "remote: " + string(e.Kind) + ": " + e.Detail
}

// KindOf returns the kind of a classified error.
func KindOf(err error) (Kind, string, bool) {
	var remoteErr *Error
	if errors.As(err, &remoteErr) {
		return remoteErr.Kind, remoteErr.Detail, true
	}
	return "", "", false
}

type rule struct {
	pattern *regexp.Regexp
	kind    Kind
}

func rx(pattern string) *regexp.Regexp { return regexp.MustCompile("(?i)" + pattern) }

// Ordered: the first match wins.
var rules = []rule{
	{rx(`auth failed on module`), KindAuth},
	{rx(`@ERROR: Unknown module`), KindModule},
	{rx(`@ERROR: access denied|@ERROR: chroot failed|@ERROR: failed to open|not allowed`), KindDenied},
	{rx(`Permission denied \((publickey|password|keyboard)`), KindAuth},
	{rx(`Permission denied, please try again|Too many authentication failures`), KindAuth},
	{rx(`password file must not be other-accessible|failed to read from password`), KindAuth},
	{rx(`Host key verification failed|REMOTE HOST IDENTIFICATION HAS CHANGED`), KindHostKey},
	{rx(`Connection refused`), KindRefused},
	{rx(`Could not resolve hostname|Name or service not known|nodename nor servname|getaddrinfo|no such host|Temporary failure in name resolution|unknown host`), KindHost},
	{rx(`timed out|No route to host|Network is unreachable|timeout in data|timeout waiting|Host is down|unable to connect`), KindUnreachable},
	{rx(`No space left on device|Disk quota exceeded`), KindDiskFull},
	{rx(`rsync: (command )?not found|rsync: No such file or directory.*rsync-path|command not found`), KindRemoteTool},
	{rx(`protocol version mismatch|protocol incompatibility|unknown option|unrecognized option|syntax or usage error`), KindProtocol},
	{rx(`Connection reset|Broken pipe|unexpected end of file|connection unexpectedly closed|closed by remote host|Connection closed|error in rsync protocol data stream|error in socket I/O`), KindInterrupted},
	{rx(`mkdir .*: No such file or directory|change_dir .* failed|No such file or directory`), KindPath},
	{rx(`Permission denied|Operation not permitted|Read-only file system`), KindPermission},
}

// Classify turns a tool's exit code and stderr into a classified error.
func Classify(exitCode int, stderr string, secrets ...string) *Error {
	stderr = Scrub(stderr, secrets...)
	for _, r := range rules {
		if r.pattern.MatchString(stderr) {
			return &Error{Kind: r.kind, Detail: lastLine(stderr)}
		}
	}
	switch exitCode {
	case 5, 6:
		return &Error{Kind: KindModule, Detail: lastLine(stderr)}
	case 10, 12, 30, 35:
		return &Error{Kind: KindInterrupted, Detail: lastLine(stderr)}
	case 11:
		return &Error{Kind: KindDiskFull, Detail: lastLine(stderr)}
	case 127:
		return &Error{Kind: KindNotFound, Detail: lastLine(stderr)}
	}
	return &Error{Kind: KindUnknown, Detail: lastLine(stderr)}
}

// Scrub removes secrets from text.
func Scrub(text string, secrets ...string) string {
	for _, secret := range secrets {
		if len(secret) >= 3 {
			text = strings.ReplaceAll(text, secret, "***")
		}
	}
	return text
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		// rsync's own trailer carries no information.
		if line != "" && !strings.HasPrefix(line, "rsync error:") && !strings.HasPrefix(line, "rsync warning:") {
			return truncate(line, 300)
		}
	}
	if text = strings.TrimSpace(text); text != "" {
		return truncate(text, 300)
	}
	return ""
}

func truncate(text string, limit int) string {
	if len(text) > limit {
		return text[:limit] + "…"
	}
	return text
}
