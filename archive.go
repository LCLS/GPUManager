package main

import (
	"html/template"
	"net/http"
)

func archiveHandler(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles("archive.html")
	t.Execute(w, nil)
}
