package netease

import (
	"sort"
	"strings"
)

// parseCookie turns a Cookie header into a name-to-value map. Everything before
// the first "=" of a pair is the name; the rest is the value, so values that
// contain "=" survive a round trip.
func parseCookie(header string) map[string]string {
	jar := make(map[string]string)
	for _, pair := range strings.Split(header, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, found := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !found {
			value = ""
		}
		jar[name] = strings.TrimSpace(value)
	}
	return jar
}

// formatCookie writes a jar back out in a stable order, so the saved session
// does not churn between runs.
func formatCookie(jar map[string]string) string {
	names := make([]string, 0, len(jar))
	for name := range jar {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+jar[name])
	}
	return strings.Join(parts, "; ")
}

// cookieValue reads one field out of a Cookie header.
func cookieValue(header, name string) string {
	return parseCookie(header)[name]
}

// mergeCookie adds the pairs of extra that are missing from base.
func mergeCookie(base, extra string) string {
	jar := parseCookie(base)
	for name, value := range parseCookie(extra) {
		if _, ok := jar[name]; !ok {
			jar[name] = value
		}
	}
	return formatCookie(jar)
}
