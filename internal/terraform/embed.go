package terraform

import (
	"embed"
)

//go:embed *.tf
var terraformFiles embed.FS

// GetEmbeddedFiles returns the embedded Terraform files
func GetEmbeddedFiles() embed.FS {
	return terraformFiles
}