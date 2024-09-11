package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/unfunco/anthropic-sdk-go"
)

const (
	Claude35Sonnet20240620 anthropic.LanguageModel = "claude-3-5-sonnet-20240620"
)

type Agent struct {
	Name           string `json:"name"`
	Specialization string `json:"specialization"`
}

var agents = make(map[string]*Agent)
var client *anthropic.Client
var agentsFile string

var rootCmd = &cobra.Command{
	Use:   "anthropic-cli",
	Short: "A CLI for interacting with the Anthropic API",
	Long:  `A multi-agent CLI interface for the Anthropic API using Go and Cobra.`,
}

// ... (other commands remain the same)

var chatCmd = &cobra.Command{
	Use:   "chat [agent1,agent2,...] [message]",
	Short: "Start a chat session with multiple agents",
	Long:  `Start a chat session with multiple agents. The conversation will continue until you type 'exit'.`,
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		agentNames := strings.Split(args[0], ",")
		initialMessage := args[1]

		var validAgents []*Agent
		for _, name := range agentNames {
			agent, exists := agents[name]
			if !exists {
				fmt.Printf("Agent %s does not exist.\n", name)
				return
			}
			validAgents = append(validAgents, agent)
		}

		systemPrompt := "You are participating in a multi-agent conversation. Each agent has a specific specialization. Respond according to your specialization and interact with other agents when appropriate. Keep your responses concise and relevant."

		conversation := fmt.Sprintf("\n\nHuman: %s\n\n", initialMessage)
		fmt.Println("Chat session started. Type 'exit' to end the conversation.")
		fmt.Print(conversation)

		for {
			for _, agent := range validAgents {
				resp, _, err := client.Messages.Create(
					context.Background(),
					&anthropic.CreateMessageInput{
						Model:     Claude35Sonnet20240620,
						MaxTokens: 150,
						System:    systemPrompt,
						Messages: []anthropic.Message{
							{
								Role:    "Human",
								Content: conversation + fmt.Sprintf("Assistant (%s, specializing in %s):", agent.Name, agent.Specialization),
								// Removed Content structure and used resp.Content directly
							},
						},
					},
				)
				if err != nil {
					fmt.Printf("Error in chat for agent %s: %v\n", agent.Name, err)
					return
				}

				agentResponse := fmt.Sprintf("Assistant (%s, specializing in %s): %s\n\n", agent.Name, agent.Specialization, resp.Content)
				conversation += agentResponse
				fmt.Print(agentResponse)
			}

			fmt.Print("Human: ")
			var userInput string
			fmt.Scanln(&userInput)

			if userInput == "exit" {
				fmt.Println("Chat session ended.")
				return
			}

			conversation += fmt.Sprintf("Human: %s\n\n", userInput)
		}
	},
}

var createAgentCmd = &cobra.Command{
	Use:   "create-agent [name] [specialization]",
	Short: "Create a new agent with a specialization",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		specialization := args[1]
		if _, exists := agents[name]; exists {
			fmt.Printf("Agent %s already exists.\n", name)
			return
		}
		agents[name] = &Agent{Name: name, Specialization: specialization}
		fmt.Printf("Created agent %s with specialization: %s\n", name, specialization)
		saveAgents()
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
			fmt.Printf("Agent: %s, Specialization: %s\n", agent.Name, agent.Specialization)
		}
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [agent] [prompt]",
	Short: "Generate a completion using a specific agent",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		agentName := args[0]
		prompt := args[1]
		agent, exists := agents[agentName]
		if !exists {
			fmt.Printf("Agent %s does not exist.\n", agentName)
			return
		}

		systemPrompt := fmt.Sprintf("You are an AI assistant specializing in %s. Respond to the user's message accordingly.", agent.Specialization)

		resp, _, err := client.Messages.Create(
			context.Background(),
			&anthropic.CreateMessageInput{
				Model:     Claude35Sonnet20240620,
				MaxTokens: 300,
				System:    systemPrompt,
				Messages: []anthropic.Message{
					{
						Role:    "Human",
						Content: prompt,
					},
				},
			},
		)
		if err != nil {
			fmt.Printf("Error generating completion: %v\n", err)
			return
		}

		fmt.Printf("Agent %s (Specialization: %s) responds:\n%s\n", agent.Name, agent.Specialization, resp.Content)
	},
}

var classifyCmd = &cobra.Command{
	Use:   "classify [agent] [text]",
	Short: "Classify the given text using a specific agent",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		agentName := args[0]
		text := args[1]
		agent, exists := agents[agentName]
		if !exists {
			fmt.Printf("Agent %s does not exist.\n", agentName)
			return
		}

		systemPrompt := fmt.Sprintf("You are an AI assistant specializing in %s. Classify the given text according to your specialization.", agent.Specialization)

		resp, _, err := client.Messages.Create(
			context.Background(),
			&anthropic.CreateMessageInput{
				Model:     Claude35Sonnet20240620,
				MaxTokens: 100,
				System:    systemPrompt,
				Messages: []anthropic.Message{
					{
						Role:    "Human",
						Content: fmt.Sprintf("Classify the following text: %s", text),
					},
				},
			},
		)
		if err != nil {
			fmt.Printf("Error classifying text: %v\n", err)
			return
		}

		fmt.Printf("Agent %s (Specialization: %s) classification:\n%s\n", agent.Name, agent.Specialization, resp.Content)
	},
}

func saveAgents() {
	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling agents: %v\n", err)
		return
	}

	err = os.WriteFile(agentsFile, data, 0644)
	if err != nil {
		fmt.Printf("Error saving agents to file: %v\n", err)
	}
}

func loadAgents() {
	data, err := os.ReadFile(agentsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return // File doesn't exist yet, which is fine for first run
		}
		fmt.Printf("Error reading agents file: %v\n", err)
		return
	}

	err = json.Unmarshal(data, &agents)
	if err != nil {
		fmt.Printf("Error unmarshaling agents: %v\n", err)
	}
}

func init() {
	rootCmd.AddCommand(createAgentCmd)
	rootCmd.AddCommand(listAgentsCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(classifyCmd)

	viper.SetEnvPrefix("ANTHROPIC")
	viper.AutomaticEnv()

	rootCmd.PersistentFlags().String("api-key", "", "Anthropic API Key")
	viper.BindPFlag("api_key", rootCmd.PersistentFlags().Lookup("api-key"))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting user home directory: %v\n", err)
		os.Exit(1)
	}
	agentsFile = filepath.Join(homeDir, ".anthropic_agents.json")
}

func main() {
	apiKey := viper.GetString("api_key")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		fmt.Println("Anthropic API key not set. Please set the ANTHROPIC_API_KEY environment variable or use the --api-key flag.")
		os.Exit(1)
	}

	client = anthropic.NewClient(&http.Client{
		Transport: &anthropic.Transport{
			APIKey: apiKey,
		},
	})

	loadAgents()

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
