package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
	"github.com/oleiade/lane"
)

var DB *sql.DB
var JobQueue *lane.Queue

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	port := flag.Int("port", 8080, "HTTP Server Port")
	flag.Parse()

	var err error
	DB, err = sql.Open("sqlite3", "file:simulation.db?cache=shared&mode=rwc")
	if err != nil {
		log.Fatalln("[Database] Error:", err)
	}

	JobQueue = lane.NewQueue()

	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js/"))))
	http.Handle("/fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("./fonts/"))))
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("./css/"))))

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/server/add", serverAddHandker)
	http.HandleFunc("/server/remove", serverRemoveHandler)
	http.HandleFunc("/server/toggle", serverToggleHandler)

	http.HandleFunc("/job", jobHandler)
	http.HandleFunc("/job/add", jobAddHandler)

	http.HandleFunc("/model", modelHandler)
	http.HandleFunc("/model/add", modelAddHandler)
	http.HandleFunc("/model/remove", modelRemoveHandler)

	http.HandleFunc("/template", templateHandler)
	http.HandleFunc("/template/add", templateAddHandler)
	http.HandleFunc("/template/remove", templateRemoveHandler)

	// Load Servers
	servers, err := LoadServers(DB)
	if err != nil {
		log.Fatalln(err)
	}
	Servers = servers

	// Load Models
	models, err := LoadModels(DB)
	if err != nil {
		log.Fatalln(err)
	}
	Models = models

	// Load Templates
	templates, err := LoadTemplates(DB)
	if err != nil {
		log.Fatalln(err)
	}
	Templates = templates

	// Load Job Instances
	rows, err := DB.Query("select j.name, j.model_id, j.template_id, i.id, i.completed FROM job_instance AS i JOIN job AS j WHERE j.id=i.job_id")
	if err != nil {
		log.Fatalln(err)
	}

	for rows.Next() {
		var name string
		var id, model_id, template_id int
		var completed bool
		if err := rows.Scan(&name, &model_id, &template_id, &id, &completed); err != nil {
			log.Fatalln(err)
		}

		instance := JobInstance{ID: id, Completed: completed, Name: name}

		for i := 0; i < len(Models); i++ {
			if Models[i].ID == model_id {
				instance.Model = Models[i]
				break
			}
		}

		for i := 0; i < len(Templates); i++ {
			if Templates[i].ID == template_id {
				instance.Template = Templates[i]
				break
			}
		}

		JobQueue.Enqueue(instance)
	}
	rows.Close()

	for _, server := range Servers {
		server.ConnectResources()
	}

	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
