package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/unfunco/anthropic-sdk-go"
)

type ModelType string

const (
	ModelTypeAnthropic ModelType = "anthropic"
	ModelTypeOllama    ModelType = "ollama"
	OutputDir          string    = "/home/jbutler/hello/"
	maxIterations      int       = 50 // Maximum number of SW-QA iteration cycles
	doNotTouch         string    = "/home/jbutler/hello/eval_test.go"
)

type Agent struct {
	Name           string    `json:"name"`
	Specialization string    `json:"specialization"`
	ModelType      ModelType `json:"model_type"`
	ModelName      string    `json:"model_name"`
	SystemPrompt   string    `json:"system_prompt"`
}

var (
	agents          = make(map[string]*Agent)
	anthropicClient *anthropic.Client
	agentsFile      string
	updatedFiles    = []string{}
)

var rootCmd = &cobra.Command{
	Use:   "ai-cli",
	Short: "A CLI for interacting with multiple AI models",
	Long:  `A multi-agent CLI interface for interacting with Anthropic and Ollama AI models.`,
}

var createAgentCmd = &cobra.Command{
	Use:   "create-agent [name] [specialization] [model_type] [model_name] [system_prompt]",
	Short: "Create a new agent with a specialization, model, and system prompt",
	Args:  cobra.ExactArgs(5),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		specialization := args[1]
		modelType := ModelType(args[2])
		modelName := args[3]
		systemPrompt := args[4]

		if _, exists := agents[name]; exists {
			fmt.Printf("Error: Agent %s already exists.\n", name)
			return
		}

		if modelType != ModelTypeAnthropic && modelType != ModelTypeOllama {
			fmt.Printf("Error: Invalid model type. Must be 'anthropic' or 'ollama'.\n")
			return
		}

		agents[name] = &Agent{Name: name, Specialization: specialization, ModelType: modelType, ModelName: modelName, SystemPrompt: systemPrompt}
		fmt.Printf("Created agent %s with specialization: %s, model type: %s, model name: %s, system prompt: %s\n", name, specialization, modelType, modelName, systemPrompt)
		if err := saveAgents(); err != nil {
			fmt.Printf("Error saving agents: %v\n", err)
		}
	},
}

var listAgentsCmd = &cobra.Command{
	Use:   "list-agents",
	Short: "List all created agents",
	Run: func(cmd *cobra.Command, args []string) {
		if len(agents) == 0 {
			fmt.Println("No agents created yet.")
			return
		}
		for _, agent := range agents {
			fmt.Printf("Agent: %s, Specialization: %s, Model Type: %s, Model Name: %s\n", agent.Name, agent.Specialization, agent.ModelType, agent.ModelName)
		}
	},
}

var chatCmd = &cobra.Command{
	Use:   "chat [agent1,agent2,...] [message] [file1,file2,...]",
	Short: "Start a chat session with multiple agents",
	Long:  `Start a chat session with multiple agents. The conversation will continue until you type 'exit'.`,
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		agentNames := strings.Split(args[0], ",")
		initialMessage := args[1]
		contextFiles := strings.Split(args[2], ",")

		var validAgents []*Agent
		var swEngineerAgent *Agent
		var qaEngineerAgent *Agent
		for _, name := range agentNames {
			agent, exists := agents[name]
			if !exists {
				fmt.Printf("Error: Agent %s does not exist.\n", name)
				return
			}
			validAgents = append(validAgents, agent)
			if strings.Contains(strings.ToLower(agent.Name), "sw_engineer") {
				swEngineerAgent = agent
			}
			if strings.Contains(strings.ToLower(agent.Name), "qa_engineer") {
				qaEngineerAgent = agent
			}
		}

		if swEngineerAgent == nil || qaEngineerAgent == nil {
			fmt.Println("Error: Both software engineer and QA engineer agents are required.")
			return
		}

		fileContents, err := readFiles(contextFiles)
		if err != nil {
			fmt.Printf("Error reading context files: %v\n", err)
			return
		}

		fmt.Println("Development process started. Software engineer will make the first update.")
		qaResponse := ""
		conversation := ""
		for iteration := 1; iteration <= maxIterations; iteration++ {
			fmt.Printf("\n--- Iteration %d ---\n", iteration)

			// Create conversation context
			conversation = fmt.Sprintf("Context Files:\n%s", fileContents)
			if qaResponse != "" {
				conversation += fmt.Sprintf("\n\nQA Response from previous attempt:\n%s", qaResponse)
			}
			conversation += fmt.Sprintf("\n\nHuman:\n%s", initialMessage)
			// Software engineer makes an update
			swUpdateResponse, err := getAgentResponse(swEngineerAgent, conversation, swEngineerAgent.SystemPrompt)
			if err != nil {
				fmt.Printf("Error getting response from software engineer agent: %v\n", err)
				return
			}

			fullSwResponse := fmt.Sprintf("Assistant (%s, specializing in %s): %s\n\n", swEngineerAgent.Name, swEngineerAgent.Specialization, swUpdateResponse)
			// conversation += fullSwResponse
			fmt.Print(fullSwResponse)

			fmt.Printf("Invoking QA engineer.\n")
			qaInput := fmt.Sprintf("Context Files:\n%s", fileContents)
			// Save code to file and apply patch
			patchApplyOutput := ""
			err = saveCodeToFile(iteration, swUpdateResponse)
			if err != nil {
				fmt.Printf("Error saving code to file: %v\n", err)
				patchApplyOutput = err.Error()
				// Prepare input for QA engineer
				qaInput += fmt.Sprintf("Software Engineer's update:\n%s\n\nPatch Apply output:\n%s\n\nPlease analyze the patch apply failures and suggest improvements.", swUpdateResponse, patchApplyOutput)
			}

			// Skip testing if patch failed to apply
			testOutput := ""
			if err == nil {
				// Run go test
				testOutput, err = runGoTest()
				if err == nil {
					fmt.Printf("All tests passed. Development process completed successfully in %d iterations.", iteration)
					return
				}
				// Prepare input for QA engineer
				qaInput += fmt.Sprintf("Software Engineer's update:\n%s\n\nTest output:\n%s\n\nPlease analyze the test failures and suggest improvements.", swUpdateResponse, testOutput)
			}

			// fmt.Println("Tests failed. Removing bad files...")
			// removeUpdatedFiles()
			fmt.Println("Resetting workspace...")
			resetWorkspace()

			qaResponse, err := getAgentResponse(qaEngineerAgent, qaInput, qaEngineerAgent.SystemPrompt)
			if err != nil {
				fmt.Printf("Error getting response from QA engineer agent: %v\n", err)
				return
			}

			fullQaResponse := fmt.Sprintf("Assistant (%s, specializing in %s): %s\n\n", qaEngineerAgent.Name, qaEngineerAgent.Specialization, qaResponse)
			// conversation += fullQaResponse
			fmt.Printf("Test Output:\n%s", testOutput)
			fmt.Print(fullQaResponse)

			// Add QA feedback to the conversation for the next iteration
			qaResponse = fmt.Sprintf("Please address the following QA feedback and make necessary updates:\n%s\n\n", qaResponse)
		}

		fmt.Printf("Maximum number of iterations (%d) reached without passing all tests.\n", maxIterations)
	},
}

func saveCodeToFile(iter int, response string) error {
	re := regexp.MustCompile("```((?s).+?)```")
	matches := re.FindAllStringSubmatch(response, -1)

	if len(matches) == 0 {
		return fmt.Errorf("No code blocks found in the response")
	}

	for i, match := range matches {
		code := match[1]
		filename := "patch-" + strconv.Itoa(iter) + "-" + strconv.Itoa(i) + ".patch"
		if err := os.WriteFile(filename, []byte(code), 0644); err != nil {
			return fmt.Errorf("Error writing code to file %s: %w", filename, err)
		}
		cmd := exec.Command("git", "apply", "-v", "/home/jbutler/git/dev-team/"+filename)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		cmd.Dir = OutputDir
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("error: %s, %s", err.Error(), out.String())
		}

	}
	return nil
}

func resetWorkspace() error {
	cmd := exec.Command("git", "reset", "--hard", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Dir = OutputDir
	err := cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("git", "clean", "-fd")
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Dir = OutputDir
	return cmd.Run()
}

func readFiles(filePaths []string) (string, error) {
	var contents strings.Builder
	for _, filePath := range filePaths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("Error reading file %s: %w", filePath, err)
		}
		contents.WriteString(fmt.Sprintf("File: %s\n", filePath))
		contents.WriteString(string(data))
		contents.WriteString("\n\n")
	}
	return contents.String(), nil
}

func removeUpdatedFiles() {
	for _, file := range updatedFiles {
		os.Remove(file)
	}
}

func runGoTest() (string, error) {
	cmd := exec.Command("go", "test", "-v", "./...")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Dir = OutputDir
	err := cmd.Run()
	return out.String(), err
}

func getAgentResponse(agent *Agent, conversation, systemPrompt string) (string, error) {
	switch agent.ModelType {
	case ModelTypeAnthropic:
		return getAnthropicResponse(agent, conversation, systemPrompt)
	case ModelTypeOllama:
		return getOllamaResponse(agent, conversation, systemPrompt)
	default:
		return "", fmt.Errorf("Unsupported model type for agent %s", agent.Name)
	}
}

func getAnthropicResponse(agent *Agent, conversation, systemPrompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, _, err := anthropicClient.Messages.Create(
		ctx,
		&anthropic.CreateMessageInput{
			Messages: []anthropic.Message{
				{
					Role:    "Human",
					Content: conversation + fmt.Sprintf("Assistant (%s, specializing in %s):", agent.Name, agent.Specialization),
				},
			},
			Model:         anthropic.LanguageModel(agent.ModelName),
			StopSequences: []string{"\n\nHuman:", "\n\nAssistant"},
			System:        systemPrompt,
		},
	)
	if err != nil {
		return "", fmt.Errorf("Anthropic API error: %w", err)
	}
	return resp.String(), nil
}

func getOllamaResponse(agent *Agent, conversation, systemPrompt string) (string, error) {
	prompt := fmt.Sprintf("%s\n\nSystem: %s\n\nAssistant (%s, specializing in %s):",
		conversation, systemPrompt, agent.Name, agent.Specialization)

	requestBody, err := json.Marshal(map[string]interface{}{
		"model":   agent.ModelName,
		"prompt":  prompt,
		"stream":  true,
		"options": map[string]interface{}{"num_ctx": 4096},
	})
	if err != nil {
		return "", fmt.Errorf("Error marshaling request body: %w", err)
	}

	ollamaEndpoint := viper.GetString("ollama-endpoint")
	client := &http.Client{Timeout: 5 * time.Minute} // Increased timeout for longer conversations
	resp, err := client.Post(ollamaEndpoint+"/api/generate", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("Ollama API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama API returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var fullResponse strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for {
		var result map[string]interface{}
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("Error decoding Ollama API response: %w", err)
		}

		if response, ok := result["response"].(string); ok {
			fullResponse.WriteString(response)
		}

		if done, ok := result["done"].(bool); ok && done {
			break
		}
	}

	return fullResponse.String(), nil
}

func saveAgents() error {
	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return fmt.Errorf("Error marshaling agents: %w", err)
	}

	if err := os.WriteFile(agentsFile, data, 0644); err != nil {
		return fmt.Errorf("Error saving agents to file: %w", err)
	}

	return nil
}

func loadAgents() error {
	data, err := os.ReadFile(agentsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, which is fine for first run
		}
		return fmt.Errorf("Error reading agents file: %w", err)
	}

	if err := json.Unmarshal(data, &agents); err != nil {
		return fmt.Errorf("Error unmarshaling agents: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(createAgentCmd)
	rootCmd.AddCommand(listAgentsCmd)
	rootCmd.AddCommand(chatCmd)

	viper.SetEnvPrefix("AI_CLI")
	viper.AutomaticEnv()

	rootCmd.PersistentFlags().String("anthropic-api-key", "", "Anthropic API Key")
	rootCmd.PersistentFlags().String("ollama-endpoint", "http://localhost:11434", "Ollama API Endpoint")
	viper.BindPFlag("anthropic-api-key", rootCmd.PersistentFlags().Lookup("anthropic-api-key"))
	viper.BindPFlag("ollama-endpoint", rootCmd.PersistentFlags().Lookup("ollama-endpoint"))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting user home directory: %v\n", err)
		os.Exit(1)
	}
	agentsFile = filepath.Join(homeDir, ".ai_agents.json")
}

func main() {
	if err := loadAgents(); err != nil {
		fmt.Printf("Error loading agents: %v\n", err)
		os.Exit(1)
	}

	anthropicApiKey := viper.GetString("anthropic-api-key")
	if anthropicApiKey == "" {
		anthropicApiKey = os.Getenv("AI_CLI_ANTHROPIC_API_KEY")
	}
	if anthropicApiKey == "" {
		fmt.Println("Error: Anthropic API key not set. Please set the AI_CLI_ANTHROPIC_API_KEY environment variable or use the --anthropic-api-key flag.")
		os.Exit(1)
	}

	anthropicClient = anthropic.NewClient(&http.Client{
		Transport: &anthropic.Transport{
			APIKey: anthropicApiKey,
		},
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
