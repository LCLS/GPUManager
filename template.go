package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"text/template"
)

type Template struct {
	ID   int
	Name string
	File string
}

func (t *Template) Process(device int, job Job) ([]byte, error) {
	type TemplateData struct {
		Input, Output  string
		Seed, DeviceID int
	}

	data := TemplateData{Seed: int(rand.Int31())}
	data.DeviceID = device
	data.Output = fmt.Sprintf("sim.%d.dcd", data.Seed)
	for _, file := range job.Model.Files {
		parts := strings.Split(strings.ToLower(file), ".")
		if parts[len(parts)-1] == "tpr" {
			data.Input = strings.ToLower(fmt.Sprintf("gromacstprfile ../../../model/%s/%s", job.Model.Name, file))
			break
		}

		if parts[len(parts)-1] == "pdb" {
			data.Input = strings.ToLower(fmt.Sprintf("../../../model/%s/%s", job.Model.Name, file))
			break
		}
	}

	temp, err := template.New(strings.Split(job.Template.File, "/")[1]).ParseFiles(job.Template.File)
	if err != nil {
		return nil, err
	}

	var templateData bytes.Buffer
	if err := temp.Execute(&templateData, data); err != nil {
		return nil, err
	}

	return templateData.Bytes(), nil
}

func (t *Template) IsProtoMol() bool {
	parts := strings.Split(strings.ToLower(t.File), ".")
	return parts[len(parts)-1] == "conf"
}

var Templates []Template

func FindTemplate(id int, templates []Template) *Template {
	for i := 0; i < len(templates); i++ {
		if templates[i].ID == id {
			return &templates[i]
		}
	}
	return nil
}

func LoadTemplates(db *sql.DB) ([]Template, error) {
	rows, err := DB.Query("SELECT * FROM template")
	if err != nil {
		return nil, err
	}

	var templates []Template
	for rows.Next() {
		var id int
		var name, file string
		if err := rows.Scan(&id, &name, &file); err != nil {
			return nil, err
		}
		templates = append(templates, Template{ID: id, Name: name, File: file})
	}

	return templates, nil
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

	if err := t.Execute(w, Templates); err != nil {
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

			parts := strings.Split(strings.ToLower(fileHeader.Filename), ".")
			extension := parts[len(parts)-1]
			ioutil.WriteFile(fmt.Sprintf("data/%s.template.%s", name, extension), buf, os.ModePerm)
			template.File = fmt.Sprintf("data/%s.template.%s", name, extension)
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

	Templates = append(Templates, template)
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
