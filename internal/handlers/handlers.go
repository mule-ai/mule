package handlers

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"sync"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/log"
	"github.com/mule-ai/mule/pkg/repository"

	"encoding/json"
	"html/template"
	"net/http"

	"github.com/jbutlerdev/genai/tools"
	"github.com/mule-ai/mule/pkg/validation"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	Logger *log.Logger
	Tmpl   *template.Template // Parsed HTML templates
	State  *state.State
	mu     sync.Mutex // To protect concurrent access (e.g., to settings in State)
}

// PageData defines the data passed to the main layout template.
type PageData struct {
	Page         string
	Repositories map[string]*repository.Repository
	Settings     *config.Config
	CurrentTab   string // Used for settings page active tab
	Error        string // For displaying errors on the page
}

// InitTemplates initializes the templates used by the handlers.
// This is called once from main.go after templates are parsed.
var templates *template.Template

func InitTemplates(t *template.Template) {
	templates = t
}

func HandleTools(w http.ResponseWriter, r *http.Request) {
	// These should match the tools defined in your codebase
	tools := tools.Tools()
	err := json.NewEncoder(w).Encode(tools)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(l *log.Logger, s *state.State) (*Handlers, error) {
	h := &Handlers{
		Logger: l,
		State:  s,
		Tmpl:   templates, // Assign the globally initialized templates
	}
	return h, nil
}

// RenderTemplate renders the main layout with the specified content template.
func (h *Handlers) RenderTemplate(w http.ResponseWriter, layout, content string, data PageData) {
	// Ensure Page is set for the layout's navigation/title logic
	data.Page = content

	buf := &bytes.Buffer{}
	// Execute the layout template which should internally call {{template "content" .}}
	err := templates.ExecuteTemplate(buf, layout+".html", data) // Use the package-level templates
	if err != nil {
		h.Logger.Error("error executing layout template", "layout", layout, "content", content, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = buf.WriteTo(w)
	if err != nil {
		h.Logger.Error("error writing template to response", "template", layout, "error", err)
	}
}

// RenderTemplatePartial renders only a specific template block/definition.
func (h *Handlers) RenderTemplatePartial(w http.ResponseWriter, templateFile, blockName string, data interface{}) {
	buf := &bytes.Buffer{}
	err := templates.ExecuteTemplate(buf, blockName, data)
	if err != nil {
		h.Logger.Error("error executing partial template", "template", blockName, "error", err)
		http.Error(w, fmt.Sprintf("Error rendering partial: %s", blockName), http.StatusInternalServerError)
		return
	}
	_, err = buf.WriteTo(w)
	if err != nil {
		h.Logger.Error("error writing partial template", "template", blockName, "error", err)
	}
}

func HandleValidationFunctions(w http.ResponseWriter, r *http.Request) {
	// Get validation functions from the validation package
	validationFuncs := validation.Validations()

	err := json.NewEncoder(w).Encode(validationFuncs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
