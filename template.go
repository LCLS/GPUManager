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
	ID   int
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
		var id int
		var name, file string
		if err := rows.Scan(&id, &name, &file); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		templates = append(templates, Template{ID: id, Name: name, File: file})
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

	res, err := DB.Exec("insert into template(name, file) values (?,?)", template.Name, template.File)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
	template.ID = int(id)

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Template: &template})
}

func templateRemoveHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	w.Header().Set("Content-Type", "application/json")

	id := r.FormValue("id")
	if id == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
		return
	}

	var file string
	if err := DB.QueryRow("SELECT file FROM template WHERE id = ?", id).Scan(&file); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if err := os.Remove(file); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if _, err := DB.Exec("DELETE FROM template WHERE id = ?", id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true})
}
