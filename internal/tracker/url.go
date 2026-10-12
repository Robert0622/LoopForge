package tracker

import (
	"net/url"
	"strconv"
	"strings"
)

// IssueRef identifies a forge issue parsed from its web URL.
type IssueRef struct {
	Host   string
	Repo   string // owner/name
	Number int
}

// ParseIssueURL parses https://<host>/<owner>/<repo>/issues/<n>. Any host is
// accepted so GitHub Enterprise links work too.
func ParseIssueURL(raw string) (IssueRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return IssueRef{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return IssueRef{}, false
	}
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(parts) < 4 || parts[2] != "issues" {
		return IssueRef{}, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil || n <= 0 {
		return IssueRef{}, false
	}
	return IssueRef{Host: u.Host, Repo: parts[0] + "/" + parts[1], Number: n}, true
}
