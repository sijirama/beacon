package web

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type Renderer struct {
	templates map[string]*template.Template
	funcs     template.FuncMap
}

func NewRenderer(templatesDir string, funcs template.FuncMap) (*Renderer, error) {
	layout := filepath.Join(templatesDir, "layouts", "base.html")
	pages, err := filepath.Glob(filepath.Join(templatesDir, "pages", "*.html"))
	if err != nil {
		return nil, err
	}
	partials, err := filepath.Glob(filepath.Join(templatesDir, "partials", "*.html"))
	if err != nil {
		return nil, err
	}

	templates := make(map[string]*template.Template, len(pages))
	for _, p := range pages {
		name := strings.TrimSuffix(filepath.Base(p), ".html")
		files := []string{layout, p}
		files = append(files, partials...)
		t, err := template.New(filepath.Base(layout)).Funcs(funcs).ParseFiles(files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		templates[name] = t
	}
	return &Renderer{templates: templates, funcs: funcs}, nil
}

func (r *Renderer) Render(c *gin.Context, status int, name string, data gin.H) {
	t, ok := r.templates[name]
	if !ok {
		c.String(http.StatusInternalServerError, "template not found: "+name)
		return
	}
	if data == nil {
		data = gin.H{}
	}
	if _, ok := data["Flash"]; !ok {
		data["Flash"] = ""
	}
	if _, ok := data["User"]; !ok {
		if v, exists := c.Get("user_email"); exists {
			data["User"] = v
		}
	}
	if _, ok := data["Path"]; !ok {
		data["Path"] = c.Request.URL.Path
	}
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(c.Writer, "base", data); err != nil {
		fmt.Fprintf(c.Writer, "<!-- render error: %v -->", err)
	}
}
