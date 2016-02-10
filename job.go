package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"text/template"
)

type Job struct {
	ID                   int
	Name                 string
	Model, Template      string
	Instances, Completed int
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
		Jobs      []Job
	}

	var data JobData

	rows, err := DB.Query("SELECT id, name FROM model")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		data.Models = append(data.Models, Model{ID: id, Name: name})
	}

	rows, err = DB.Query("SELECT id, name FROM template")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		data.Templates = append(data.Templates, Template{ID: id, Name: name})
	}

	// Load Jobs
	rows, err = DB.Query("SELECT id, name, model_id, template_id, count FROM job")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for rows.Next() {
		var id, model_id, template_id, count int
		var name string
		if err := rows.Scan(&id, &name, &model_id, &template_id, &count); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		job := Job{ID: id, Name: name, Instances: count}

		for i := 0; i < len(data.Models); i++ {
			if data.Models[i].ID == model_id {
				job.Model = data.Models[i].Name
				break
			}
		}

		for i := 0; i < len(data.Templates); i++ {
			if data.Templates[i].ID == template_id {
				job.Template = data.Templates[i].Name
				break
			}
		}

		if err := DB.QueryRow("SELECT COUNT(*) FROM job_instance WHERE job_id = ? AND completed = 1", id).Scan(&job.Completed); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		data.Jobs = append(data.Jobs, job)
	}

	if err := t.Execute(w, data); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
}

func jobAddHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Job     Job    `json:"job"`
	}

	w.Header().Set("Content-Type", "application/json")

	name := r.FormValue("name")

	model_id, err := strconv.Atoi(r.FormValue("model"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	template_id, err := strconv.Atoi(r.FormValue("template"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	count, err := strconv.Atoi(r.FormValue("count"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if name == "" || count <= 0 {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
		return
	}

	log.Println(model_id)
	var model string
	if err := DB.QueryRow("SELECT name FROM model WHERE id = ?", model_id).Scan(&model); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Model Name Find: " + err.Error()})
		return
	}

	var template string
	if err := DB.QueryRow("SELECT name FROM template WHERE id = ?", template_id).Scan(&template); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	job := Job{Name: name, Model: model, Template: template, Instances: count, Completed: 0}

	res, err := DB.Exec("insert into job(name, model_id, template_id, count) values (?,?,?,?)", job.Name, model_id, template_id, job.Instances)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
	job.ID = int(id)

	for i := 0; i < job.Instances; i++ {
		if _, err := DB.Exec("insert into job_instance(job_id) values (?)", id); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Job: job})
}
