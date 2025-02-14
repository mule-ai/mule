package agent

import (
	"fmt"
	"slices"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/dev-team/pkg/validation"
	"github.com/jbutlerdev/genai"
	"github.com/jbutlerdev/genai/tools"
)

type Agent struct {
	provider       *genai.Provider
	model          string
	promptTemplate string
	tools          []*tools.Tool
	logger         logr.Logger
	validations    []validation.ValidationFunc
	path           string
}

type AgentOptions struct {
	Provider            *genai.Provider
	Model               string
	PromptTemplate      string
	Logger              logr.Logger
	Tools               []string
	ValidationFunctions []string
	Path                string
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
	return &Agent{
		provider:       opts.Provider,
		model:          opts.Model,
		promptTemplate: opts.PromptTemplate,
		logger:         opts.Logger,
		validations:    validations,
		// I don't like this, but it's a hack to get the path to the repository
		path: opts.Path,
	}
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

func (a *Agent) Run() error {
	chat := a.provider.Chat(a.model, a.tools)

	go func() {
		for response := range chat.Recv {
			a.logger.Info("Response", "response", response)
		}
	}()

	// TODO:
	// Render prompt template with data
	chat.Send <- a.promptTemplate

	defer func() {
		chat.Done <- true
	}()
	// block until generation is complete
	<-chat.GenerationComplete
	// validate output
	err := validation.Run(&validation.ValidationInput{
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
