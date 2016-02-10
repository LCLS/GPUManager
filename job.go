package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"text/template"
)

type Job struct {
	ID        int
	Name      string
	Model     Model
	Template  Template
	Instances []JobInstance
}

func (j *Job) Complete() int {
	retVal := 0
	for _, instance := range j.Instances {
		if instance.Completed {
			retVal += 1
		}
	}
	return retVal
}

type JobInstance struct {
	ID        int
	Name      string
	Model     Model
	Template  Template
	Completed bool
}

func jobHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	funcMap := template.FuncMap{
		"count": func(in []JobInstance) int {
			return len(in)
		},
		"complete": func(in Job) int {
			return in.Complete()
		},
	}

	t, err := template.New("job.html").Funcs(funcMap).ParseFiles("job.html")
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

	// Load Jobs
	rows, err = DB.Query("SELECT id, name, model_id, template_id FROM job")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	for rows.Next() {
		var id, model_id, template_id int
		var name string
		if err := rows.Scan(&id, &name, &model_id, &template_id); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		job := Job{ID: id, Name: name}

		for i := 0; i < len(data.Models); i++ {
			if data.Models[i].ID == model_id {
				job.Model = data.Models[i]
				break
			}
		}

		for i := 0; i < len(Templates); i++ {
			if Templates[i].ID == template_id {
				job.Template = Templates[i]
				break
			}
		}

		data.Jobs = append(data.Jobs, job)
	}
	rows.Close()

	// Load Job Instances
	for i := 0; i < len(data.Jobs); i++ {
		rows, err = DB.Query("SELECT id, completed FROM job_instance WHERE job_id = ?", data.Jobs[i].ID)
		if err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		for rows.Next() {
			var id int
			var completed bool
			if err := rows.Scan(&id, &completed); err != nil {
				json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
				return
			}

			data.Jobs[i].Instances = append(data.Jobs[i].Instances, JobInstance{ID: id, Completed: completed, Name: data.Jobs[i].Name, Model: data.Jobs[i].Model, Template: data.Jobs[i].Template})
		}
		rows.Close()
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

	job := Job{Name: name, Model: *FindModel(model_id, Models), Template: *FindTemplate(template_id, Templates)}

	res, err := DB.Exec("insert into job(name, model_id, template_id, count) values (?,?,?,?)", job.Name, model_id, template_id, count)
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

	for i := 0; i < count; i++ {
		if _, err := DB.Exec("insert into job_instance(job_id) values (?)", id); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		id, err := res.LastInsertId()
		if err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		job.Instances = append(job.Instances, JobInstance{ID: int(id), Completed: false, Name: job.Name, Model: job.Model, Template: job.Template})
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Job: job})
}
