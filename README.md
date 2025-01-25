# Dev Team

### Your AI development team

dev-team is an AI Agent that monitors your git repositories and completes issues assigned to it.

Issues are assigned by giving them the `dev-team` label.

After the work is completed, the agent will create a pull request. Additional refinement can be requested by commenting on the pull request.

When the pull request is closed or merged, no more work will be completed unless the issue is reopened.

It is intended that the agent will be able to work on multiple issues at once through the creation of multiple pull requests.


## To Do

* Track open issues 
* Generate pull request using issue as prompt 
* Track open pull requests
* Use issue, diff, and comment as prompt for PR refinement 
* Perform RAG for better results 
* Increase accuracy through agent tool use
* Implement manager mode to allow spawning multiple agents that track their own repository 