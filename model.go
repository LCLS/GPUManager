package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"text/template"
)

type Model struct {
	Name  string
	Files []string
}

func modelHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	funcMap := template.FuncMap{
		"join": func(in []string) string {
			return strings.Join(in, ", ")
		},
	}

	t, err := template.New("model.html").Funcs(funcMap).ParseFiles("model.html")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	rows, err := DB.Query("SELECT model.name, model_file.name FROM model JOIN model_file ON model.name=model_file.model")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	var models []Model
	for rows.Next() {
		var name, file string
		if err := rows.Scan(&name, &file); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		found := false
		for i := 0; i < len(models); i++ {
			if models[i].Name == name {
				found = true
				models[i].Files = append(models[i].Files, file)
				break
			}
		}

		if !found {
			models = append(models, Model{Name: name, Files: []string{file}})
		}
	}

	if err := t.Execute(w, models); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
}

func modelAddHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Model   *Model `json:"model"`
	}

	if err := r.ParseMultipartForm(1 * 1024 * 1024); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to parse form", Model: nil})
		return
	}

	name := r.FormValue("name")
	if name == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Name", Model: nil})
		return
	}

	if err := os.Mkdir("data/"+name, 0755); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to create folder", Model: nil})
		return
	}

	model := Model{Name: name}
	if _, err := DB.Exec("insert into model(name) values (?)", model.Name); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			file, _ := fileHeader.Open()
			buf, _ := ioutil.ReadAll(file)
			ioutil.WriteFile(fmt.Sprintf("data/%s/%s", name, fileHeader.Filename), buf, os.ModePerm)
			model.Files = append(model.Files, fileHeader.Filename)

			if _, err := DB.Exec("insert into model_file(name, model) values (?, ?)", fileHeader.Filename, name); err != nil {
				json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
				return
			}
		}
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Model: &model})
}
