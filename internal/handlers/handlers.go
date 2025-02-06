package handlers

import (
	"html/template"
)

var templates *template.Template

func InitTemplates(t *template.Template) {
	templates = t
}
