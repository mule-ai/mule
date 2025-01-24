package repository

import "fmt"

func CommitPrompt(changes string) string {
	return fmt.Sprintf("Generate a concise commit message for the following changes\n"+
		"no placeholders, explanation, or other text should be provided\n"+
		"limit the message to 72 characters\n\n%s", changes)
}

func PRPrompt(changes string) string {
	return fmt.Sprintf("Generate a detailed pull request description for the following changes:\n\n%s\n\n"+
		"The description should include:\n"+
		"1. A summary of the changes\n"+
		"2. The motivation for the changes\n"+
		"3. Any potential impact or breaking changes\n"+
		"4. Testing instructions if applicable\n\n"+
		"Format the response in markdown.\n"+
		"Do not include any other text in the response.\n"+
		"Do not include any placeholders in the response. It is expected to be a complete description.\n"+
		"Provide the output as markdown, but do not wrap it in a code block.\n\n",
		changes)
}
