package email

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/sijirama/beacon/internal/models"
)

// TemplateRenderer loads notification email templates and renders an Event into
// a subject + HTML body. V1 ships only `default.html` which styles by level.
// Drop a `<level>.html` next to it (e.g. danger.html) to override per-level.
type TemplateRenderer struct {
	defaultT  *template.Template
	overrides map[string]*template.Template
}

func LoadTemplates(dir string) (*TemplateRenderer, error) {
	defaultPath := filepath.Join(dir, "default.html")
	defaultT, err := template.New("default.html").Funcs(funcs()).ParseFiles(defaultPath)
	if err != nil {
		return nil, fmt.Errorf("parse default email template: %w", err)
	}
	overrides := make(map[string]*template.Template)
	for _, level := range []string{"info", "warn", "danger"} {
		p := filepath.Join(dir, level+".html")
		if _, err := os.Stat(p); err == nil {
			t, err := template.New(level + ".html").Funcs(funcs()).ParseFiles(p)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", p, err)
			}
			overrides[level] = t
		}
	}
	return &TemplateRenderer{defaultT: defaultT, overrides: overrides}, nil
}

func (r *TemplateRenderer) Render(ev models.Event) (subject, html string, err error) {
	t := r.defaultT
	if override, ok := r.overrides[string(ev.Level)]; ok {
		t = override
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, t.Name(), ev); err != nil {
		return "", "", err
	}
	subject = fmt.Sprintf("[BEACON][%s] %s", strings.ToUpper(string(ev.Level)), ev.Title)
	return subject, buf.String(), nil
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
	}
}
