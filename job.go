package main

import (
	"encoding/json"
	"net/http"
	"text/template"
)

type Job struct {
	Name            string
	Model, Template string
	Instances       int
}

func jobHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	t, err := template.New("job.html").ParseFiles("job.html")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	type JobData struct {
		Models    []Model
		Templates []Template
	}

	var data JobData

	rows, err := DB.Query("SELECT name FROM model")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		data.Models = append(data.Models, Model{Name: name})
	}

	rows, err = DB.Query("SELECT name FROM template")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		data.Templates = append(data.Templates, Template{Name: name})
	}

	if err := t.Execute(w, data); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
}

func jobAddHandler(w http.ResponseWriter, r *http.Request) {

}
