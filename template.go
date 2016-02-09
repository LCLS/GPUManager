package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
)

type Template struct {
	Name string
	File string
}

func templateHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	t, err := template.New("template.html").ParseFiles("template.html")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	rows, err := DB.Query("SELECT * FROM template")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	var templates []Template
	for rows.Next() {
		var name, file string
		if err := rows.Scan(&name, &file); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		templates = append(templates, Template{Name: name, File: file})
	}

	if err := t.Execute(w, templates); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
}

func templateAddHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success  bool      `json:"success"`
		Message  string    `json:"message"`
		Template *Template `json:"template"`
	}

	if err := r.ParseMultipartForm(1 * 1024 * 1024); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to parse form", Template: nil})
		return
	}

	name := r.FormValue("name")
	if name == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Name", Template: nil})
		return
	}

	template := Template{Name: name, File: fmt.Sprintf("data/%s.template", name)}

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			file, _ := fileHeader.Open()
			buf, _ := ioutil.ReadAll(file)
			ioutil.WriteFile(fmt.Sprintf("data/%s.template", name), buf, os.ModePerm)
		}
	}

	if _, err := DB.Exec("insert into template(name, file) values (?,?)", template.Name, template.File); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Template: &template})
}
