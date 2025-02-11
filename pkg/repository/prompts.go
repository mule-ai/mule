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

func IssuePrompt(issue string) string {
	// return a prompt that will have an agent write the code to fix the issue
	return fmt.Sprintf("Write the code to fix the following issue:\n\n%s\n\n"+
		"The code should be written in the language of the repository.\n"+
		"It is recommended that you first list the files in the repository and read one of them to get an idea of the codebase.\n"+
		"After that, make sure you use the writeFile tool to write the code to a file.\n\n", issue)
}

func PRCommentPrompt(issue, diffs, comment, diffHunk string) string {
	// return a prompt that includes the original issue and the comment
	return fmt.Sprintf("You were given the following Issue to address:\n\n%s\n\n"+
		"From this, you generated the following pull request:\n\n%s\n\n"+
		"A User provided you the following comment on the pull request:\n\n%s\n\n"+
		"Which applies to the following diff hunk:\n\n%s\n\n"+
		"Update your code to properly and thoroughly address the comment.\n"+
		"It is recommended that you first list the files in the repository and read one of them to get an idea of the codebase.\n"+
		"After that, make sure you use the writeFile tool to write the code to a file.\n\n", issue, diffs, comment, diffHunk)
}
