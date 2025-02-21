# Mule

### Your AI development team

mule is an AI Agent that monitors your git repositories and completes issues assigned to it.

Issues are assigned by giving them the `mule` label.

After the work is completed, the agent will create a pull request. Additional refinement can be requested by commenting on the pull request.

When the pull request is closed or merged, no more work will be completed unless the issue is reopened.

It is intended that the agent will be able to work on multiple issues at once through the creation of multiple pull requests.

## Demo

Below is a quick demo of the agent interaction workflow using the local provider. This same workflow can be done using a GitHub provider and performing these steps in the GitHub UI.

https://storage.googleapis.com/mule-storage/devteam-local-demo.webm.mov

## Docs

Documentation is available on [muleai.io](https://muleai.io/docs)

## Contributing

* Find an issue marked `good first issue`
* Open a Pull Request

## To Do

* Perform RAG for better results
* Create multi-agent workflows
* Add the ability to create a new repository
* Implement manager mode to allow spawning multiple agents that track their own repository 
