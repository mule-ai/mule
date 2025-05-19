package settings

import (
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/integration"
	"github.com/mule-ai/mule/pkg/integration/matrix"
	"github.com/mule-ai/mule/pkg/integration/types"
)

var DefaultSettings = Settings{
	Environment: []EnvironmentVariable{},
	GitHubToken: "",
	AIProviders: []AIProviderSettings{
		{
			Provider: "ollama",
			APIKey:   "",
			Server:   "http://localhost:11434",
		},
	},
	Agents: []agent.AgentOptions{
		{
			ID:             10,
			ProviderName:   "ollama",
			Name:           "code",
			Model:          "qwen2.5-coder:32b",
			PromptTemplate: "Your software team has been assigned the following issue.\n\n{{ .IssueTitle }}:\n{{ .IssueBody }}\n\n\n{{ if .IsPRComment }}\n\nYou generated the following diffs when solving the issue above.\n\n{{ .Diff }}\n\nA user has provided you the following comment:\n\n{{ .PRComment }}\n\non the following lines:\n\n{{ .PRCommentDiffHunk }}\n\n{{ end }}\n\n\nYour software architect has provided the context above. Be sure to use that while implementing your solution.\n\n",
			SystemPrompt:   "Act as an expert software developer.\nYou are diligent and tireless!\nYou NEVER leave comments describing code without implementing it!\nYou always COMPLETELY IMPLEMENT the needed code!\nAlways use best practices when coding.\nRespect and use existing conventions, libraries, etc that are already present in the code base.\n\nTake requests for changes to the supplied code.\nIf the request is ambiguous, ask questions.\n\n\nFor each file that needs to be changed, write out the changes similar to a unified diff like `diff -U0` would produce.\n\n1. Add an imports of sympy.\n2. Remove the is_prime() function.\n3. Replace the existing call to is_prime() with a call to sympy.isprime().\n\nHere are the diffs for those changes:\n\n```diff\n--- mathweb/flask/app.py\n+++ mathweb/flask/app.py\n@@ ... @@\n-class MathWeb:\n+import sympy\n+\n+class MathWeb:\n@@ ... @@\n-def is_prime(x):\n-    if x \u003c 2:\n-        return False\n-    for i in range(2, int(math.sqrt(x)) + 1):\n-        if x % i == 0:\n-            return False\n-    return True\n@@ ... @@\n-@app.route('/prime/\u003cint:n\u003e')\n-def nth_prime(n):\n-    count = 0\n-    num = 1\n-    while count \u003c n:\n-        num += 1\n-        if is_prime(num):\n-            count += 1\n-    return str(num)\n+@app.route('/prime/\u003cint:n\u003e')\n+def nth_prime(n):\n-    count = 0\n-    num = 1\n-    while count \u003c n:\n-        num += 1\n-        if sympy.isprime(num):\n-            count += 1\n-    return str(num)\n+    count = 0\n+    num = 1\n+    while count \u003c n:\n+        num += 1\n+        if sympy.isprime(num):\n+            count += 1\n+    return str(num)\n```",
			Tools: []string{
				"revertFile",
				"tree",
				"readFile",
			},
			UDiffSettings: agent.UDiffSettings{
				Enabled: true,
			},
		},
		{
			ID:             11,
			ProviderName:   "ollama",
			Name:           "architect",
			Model:          "qwq:32b-q8_0",
			PromptTemplate: "You have been assigned the following issue.\n\n{{ .IssueTitle }}:\n{{ .IssueBody }}\n\n{{ if .IsPRComment }}\n\nYou generated the following diffs when solving the issue above.\n\n{{ .Diff }}\n\nA user has provided you the following comment:\n\n{{ .PRComment }}\n\non the following lines:\n\n{{ .PRCommentDiffHunk }}\n\n{{ end }}\n\nHelp your team address the content above. Break it down into workable steps so that your software engineering team can complete it. Perform any software architecture work that will aid in a better solution. Make sure that your approach includes tested software.\n\nYou can use the tools provided to learn more about the codebase.",
			SystemPrompt:   "Act as an expert architect engineer and provide direction to your editor engineer.\nStudy the change request and the current code.\nDescribe how to modify the code to complete the request.\nThe editor engineer will rely solely on your instructions, so make them unambiguous and complete.\nExplain all needed code changes clearly and completely, but concisely.\nJust show the changes needed.\n\nDO NOT show the entire updated function/file/etc!",
			Tools: []string{
				"tree",
				"readFile",
			},
		},
	},
	SystemAgent: SystemAgentSettings{
		ProviderName:    "ollama",
		Model:           "gemma3:27b",
		CommitTemplate:  "You were given the following issue to complete:\n\n{{ .IssueTitle }}\n{{ .IssueBody }}\n\nGenerate a concise commit message for the following changes\n\n{{ .Diff }}\n\nno placeholders, explanation, or other text should be provided. Limit the message to 72 characters",
		PRTitleTemplate: "You were given the following issue to complete:\n\n{{ .IssueTitle }}\n{{ .IssueBody }}\n\nGenerate a concise pull request title for the following changes\n\n{{ .Diff }}\n\nno placeholders, explanation, or other text should be provided. Limit the message to 72 characters",
		PRBodyTemplate:  "You were given the following issue to complete:\n\n{{ .IssueTitle }}\n{{ .IssueBody }}\n\nGenerate a detailed pull request description for the following changes:\n\n{{ .Diff }}\n\nThe description should include:\n1. A summary of the changes\n2. The motivation for the changes\n3. Any potential impact or breaking changes\n4. Testing instructions if applicable\n\nFormat the response in markdown, but do not put it in a code block.\nDo not include any other text in the response.\nDo not include any placeholders in the response. It is expected to be a complete description.",
		SystemPrompt:    "",
	},
	Workflows: []agent.WorkflowSettings{
		{
			ID:          "workflow_code_generation",
			Name:        "Code Generation",
			Description: "This is a simple code generation workflow",
			IsDefault:   true,
			Outputs:     []types.TriggerSettings{},
			Steps: []agent.WorkflowStep{
				{
					ID:          "step_architect",
					AgentID:     11,
					AgentName:   "architect",
					OutputField: "generatedText",
				},
				{
					ID:          "step_code_generation",
					AgentID:     10,
					AgentName:   "code",
					OutputField: "generatedText",
				},
			},
			Triggers: []types.TriggerSettings{},
			ValidationFunctions: []string{
				"goFmt",
				"goModTidy",
				"golangciLint",
				"goTest",
				"getDeps",
			},
		},
	},
	Integration: integration.Settings{
		Matrix: &matrix.Config{
			Enabled: false,
		},
	},
}
