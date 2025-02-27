package agent

import (
	"bytes"
	"fmt"
	"html/template"
	"reflect"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/jbutlerdev/genai/tools"
	"github.com/mule-ai/mule/pkg/rag"
	"github.com/mule-ai/mule/pkg/validation"
)

const (
	RAG_N_RESULTS = 3
)

type Agent struct {
	provider       *genai.Provider
	model          string
	promptTemplate string
	tools          []*tools.Tool
	logger         logr.Logger
	validations    []validation.ValidationFunc
	name           string
	path           string
	rag            *rag.Store
}

type AgentOptions struct {
	Provider            *genai.Provider `json:"-"`
	ProviderName        string          `json:"providerName"`
	Name                string          `json:"name"`
	Model               string          `json:"model"`
	PromptTemplate      string          `json:"promptTemplate"`
	Logger              logr.Logger     `json:"-"`
	Tools               []string        `json:"tools"`
	ValidationFunctions []string        `json:"validationFunctions"`
	Path                string          `json:"-"`
	RAG                 *rag.Store      `json:"-"`
}

type PromptInput struct {
	IssueTitle        string `json:"issueTitle"`
	IssueBody         string `json:"issueBody"`
	Commits           string `json:"commits"`
	Diff              string `json:"diff"`
	IsPRComment       bool   `json:"isPRComment"`
	PRComment         string `json:"prComment"`
	PRCommentDiffHunk string `json:"prCommentDiffHunk"`
}

func NewAgent(opts AgentOptions) *Agent {
	validations := make([]validation.ValidationFunc, len(opts.ValidationFunctions))
	for i, fn := range opts.ValidationFunctions {
		v, ok := validation.Get(fn)
		if ok {
			validations[i] = v
		} else {
			opts.Logger.Error(fmt.Errorf("validation function %s not found", fn), "Validation function not found")
		}
	}
	agent := &Agent{
		provider:       opts.Provider,
		model:          opts.Model,
		promptTemplate: opts.PromptTemplate,
		logger:         opts.Logger,
		validations:    validations,
		name:           opts.Name,
		// I don't like this, but it's a hack to get the path to the repository
		path: opts.Path,
		rag:  opts.RAG,
	}
	err := agent.SetTools(opts.Tools)
	if err != nil {
		opts.Logger.Error(err, "Error setting tools")
	}
	return agent
}

func (a *Agent) SetModel(model string) error {
	models := a.provider.Models()
	if slices.Contains(models, model) {
		a.model = model
		return nil
	}
	return fmt.Errorf("model %s not found", model)
}

func (a *Agent) SetTools(toolList []string) error {
	for _, toolName := range toolList {
		tool, err := tools.GetTool(toolName)
		if err != nil {
			return fmt.Errorf("tool %s not found", toolName)
		}
		a.tools = append(a.tools, tool)
	}
	return nil
}

func (a *Agent) SetPromptTemplate(promptTemplate string) {
	a.promptTemplate = promptTemplate
}

func (a *Agent) Run(input PromptInput) error {
	if a.provider == nil {
		return fmt.Errorf("provider not set")
	}
	chat := a.provider.Chat(a.model, a.tools)

	go func() {
		for response := range chat.Recv {
			a.logger.Info("Response", "response", response)
		}
	}()

	prompt, err := a.renderPromptTemplate(input)
	if err != nil {
		return err
	}
	prompt, err = a.AddRAGContext(prompt)
	if err != nil {
		return err
	}
	chat.Send <- prompt

	defer func() {
		chat.Done <- true
	}()
	// block until generation is complete
	<-chat.GenerationComplete
	// validate output
	err = validation.Run(&validation.ValidationInput{
		Attempts:    10,
		Validations: a.validations,
		Send:        chat.Send,
		Done:        chat.GenerationComplete,
		Logger:      a.logger,
		Path:        a.path,
	})
	if err != nil {
		a.logger.Error(err, "Error validating output")
		return err
	}
	return nil
}

func (a *Agent) RunInPath(path string, input PromptInput) error {
	a.path = path
	for _, tool := range a.tools {
		tool.Options["basePath"] = path
	}
	return a.Run(input)
}

func (a *Agent) Generate(path string, input PromptInput) (string, error) {
	prompt, err := a.renderPromptTemplate(input)
	if err != nil {
		return "", err
	}
	if path != "" {
		a.path = path
		prompt, err = a.AddRAGContext(prompt)
		if err != nil {
			return "", err
		}
	}
	return a.provider.Generate(a.model, prompt)
}

func (a *Agent) renderPromptTemplate(input PromptInput) (string, error) {
	// use golang template to render prompt template
	tmpl, err := template.New("prompt").Parse(a.promptTemplate)
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	err = tmpl.Execute(&rendered, input)
	if err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (a *Agent) AddRAGContext(prompt string) (string, error) {
	ragContext, err := a.rag.Query(a.path, prompt, RAG_N_RESULTS)
	if err != nil {
		return "", err
	}
	prompt = "<context>\n" + ragContext + "\n</context>\n\n" + prompt
	return prompt, nil
}

func GetPromptTemplateValues() string {
	templates := []string{}
	s := &PromptInput{}
	val := reflect.ValueOf(s).Elem()
	for i := 0; i < val.NumField(); i++ {
		templates = append(templates, fmt.Sprintf("{{ .%s }}", val.Type().Field(i).Name))
	}
	return strings.Join(templates, ", ")
}
