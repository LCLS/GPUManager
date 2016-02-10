package main

import (
	"database/sql"
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
	JobID     int
	Completed bool
}

var Jobs []Job

func FindJob(id int, jobs []Job) *Job {
	for i := 0; i < len(jobs); i++ {
		if jobs[i].ID == id {
			return &jobs[i]
		}
	}
	return nil
}

func LoadJobs(db *sql.DB) ([]Job, error) {
	var jobs []Job

	rows, err := DB.Query("SELECT id, name, model_id, template_id FROM job")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var job Job
		var model_id, template_id int
		if err := rows.Scan(&job.ID, &job.Name, &model_id, &template_id); err != nil {
			return nil, err
		}
		job.Model = *FindModel(model_id, Models)
		job.Template = *FindTemplate(template_id, Templates)

		jobs = append(jobs, job)
	}
	rows.Close()

	for i := 0; i < len(jobs); i++ {
		rows, err := DB.Query("SELECT id, completed, job_id FROM job_instance WHERE job_id = ?", jobs[i].ID)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var instance JobInstance
			if err := rows.Scan(&instance.ID, &instance.Completed, &instance.JobID); err != nil {
				return nil, err
			}
			jobs[i].Instances = append(jobs[i].Instances, instance)
		}
		rows.Close()
	}

	return jobs, nil
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

	data := JobData{Models: Models, Templates: Templates, Jobs: Jobs}
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

		iid, err := res.LastInsertId()
		if err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}
		job.Instances = append(job.Instances, JobInstance{ID: int(iid), Completed: false, JobID: int(id)})
	}

	Jobs = append(Jobs, job)
	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Job: job})
}
