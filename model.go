package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"text/template"
)

type Model struct {
	ID    int
	Name  string
	Files []string
}

var Models []Model

func FindModel(id int, models []Model) *Model {
	for i := 0; i < len(models); i++ {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

func LoadModels(db *sql.DB) ([]Model, error) {
	rows, err := db.Query("SELECT id, name FROM model")
	if err != nil {
		return nil, err
	}

	var models []Model
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		models = append(models, Model{ID: id, Name: name})
	}
	rows.Close()

	for i := 0; i < len(models); i++ {
		rows, err = db.Query("SELECT name FROM model_file WHERE model_id = ?", models[i].ID)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}

			models[i].Files = append(models[i].Files, name)
		}
		rows.Close()
	}
	return models, nil
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

	if err := t.Execute(w, Models); err != nil {
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
	res, err := DB.Exec("insert into model(name) values (?)", model.Name)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	model.ID = int(id)

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			file, _ := fileHeader.Open()
			buf, _ := ioutil.ReadAll(file)
			ioutil.WriteFile(fmt.Sprintf("data/%s/%s", name, fileHeader.Filename), buf, os.ModePerm)
			model.Files = append(model.Files, fileHeader.Filename)

			if _, err := DB.Exec("insert into model_file(name, model_id) values (?, ?)", fileHeader.Filename, id); err != nil {
				json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
				return
			}
		}
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Model: &model})
}

func modelRemoveHandler(w http.ResponseWriter, r *http.Request) {
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

	var name string
	if err := DB.QueryRow("SELECT name FROM model WHERE id = ?", id).Scan(&name); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if err := os.RemoveAll("data/" + name); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if _, err := DB.Exec("DELETE FROM model_file WHERE model_id = ?", id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if _, err := DB.Exec("DELETE FROM model WHERE id = ?", id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true})
}
