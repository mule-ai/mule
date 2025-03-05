package agent

import (
	//"encoding/json"
	"fmt"
	//"strings"
)

type TaskBreakdown struct {
	Tasks []string `json:"tasks"`
}

func RunWorkflow(agents map[int]*Agent, promptInput PromptInput, path string) error {
	reasoningAgent, err := reasoningAgent(agents)
	if err != nil {
		return err
	}
	codeAgent, err := codeAgent(agents)
	if err != nil {
		return err
	}
	reasoning, err := reasoningAgent.Generate(path, promptInput)
	if err != nil {
		return err
	}
	/*
		tasks, err := extractTasks(reasoning)
		if err != nil {
			return err
		}
	*/
	codeAgent.SetPromptContext(reasoning)
	err = codeAgent.RunInPath(path, promptInput)
	if err != nil {
		return err
	}
	/*
		originalPromptTemplate := codeAgent.promptTemplate
		originalReasoningTemplate := reasoningAgent.promptTemplate
		for _, task := range tasks {
			codeAgent.promptTemplate = fmt.Sprintf("%s\n\nYou have been given the following task: \n%s", originalPromptTemplate, task)
			err = codeAgent.RunInPath(path, promptInput)
			if err != nil {
				// attempt reasoning rescue
				reasoningAgent.promptTemplate = fmt.Sprintf("Original Message:\n%s\n\nYour provided an agent with this task:\n%s\n\nAnd it got stuck. Try to provide more direction", originalReasoningTemplate, task)
				response, err := reasoningAgent.Generate(path, promptInput)
				if err != nil {
					return err
				}
				codeAgent.promptTemplate = fmt.Sprintf("%s\n\nYou have been given the following task: \n%s", originalPromptTemplate, response)
				err = codeAgent.RunInPath(path, promptInput)
				if err != nil {
					return err
				}
			}
		}
	*/
	return nil
}

func reasoningAgent(agents map[int]*Agent) (*Agent, error) {
	for _, agent := range agents {
		if agent.name == "reasoning" {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("reasoning agent not found")
}

func codeAgent(agents map[int]*Agent) (*Agent, error) {
	for _, agent := range agents {
		if agent.name == "code" {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("code agent not found")
}

/*
func extractTasks(reasoning string) ([]string, error) {
	// remove all text before </think> including </think>
	mark := strings.Index(reasoning, "</think>")
	if mark == -1 {
		return nil, fmt.Errorf("reasoning agent returned no output")
	}
	reasoning = reasoning[mark+7:]
	// remove the text from the code block if its in one
	codeBlock := strings.Index(reasoning, "```json")
	if codeBlock != -1 {
		reasoning = reasoning[codeBlock+7:]
	}
	// remove the closing code block
	end := strings.Index(reasoning, "```")
	if end != -1 {
		reasoning = reasoning[:end]
	}
	// decode json from reasoning
	var reasoningOutput TaskBreakdown
	err := json.Unmarshal([]byte(reasoning), &reasoningOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal reasoning: %w", err)
	}
	return reasoningOutput.Tasks, nil
}
*/
