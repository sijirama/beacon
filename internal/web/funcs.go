package web

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

func DefaultFuncs() template.FuncMap {
	return template.FuncMap{
		"dict":      dict,
		"timeAgo":   timeAgo,
		"fmtDate":   fmtDate,
		"upper":     strings.ToUpper,
		"truncate":  truncate,
		"json":      jsonPretty,
		"maskToken": maskToken,
	}
}

func maskToken(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:8] + "…" + s[len(s)-4:]
}

func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("dict requires an even number of arguments")
	}
	m := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %d not a string", i)
		}
		m[key] = values[i+1]
	}
	return m, nil
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2, 2006")
	}
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("Jan 2, 2006 15:04")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func jsonPretty(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
