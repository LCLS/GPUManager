package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB
var JobQueue chan *JobInstance

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	port := flag.Int("port", 8080, "HTTP Server Port")
	flag.Parse()

	var err error
	DB, err = sql.Open("sqlite3", "file:simulation.db?cache=shared&mode=rwc")
	if err != nil {
		log.Fatalln("[Database] Error:", err)
	}

	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js/"))))
	http.Handle("/fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("./fonts/"))))
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("./css/"))))

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/server/add", serverAddHandker)
	http.HandleFunc("/server/remove", serverRemoveHandler)
	http.HandleFunc("/server/toggle", serverToggleHandler)

	http.HandleFunc("/job", jobHandler)
	http.HandleFunc("/job/add", jobAddHandler)
	http.HandleFunc("/job/remove", jobRemoveHandler)

	http.HandleFunc("/model", modelHandler)
	http.HandleFunc("/model/add", modelAddHandler)
	http.HandleFunc("/model/remove", modelRemoveHandler)

	http.HandleFunc("/template", templateHandler)
	http.HandleFunc("/template/add", templateAddHandler)
	http.HandleFunc("/template/remove", templateRemoveHandler)

	http.HandleFunc("/archive", archiveHandler)
	http.HandleFunc("/archive/add", archiveAddHandler)

	// Load Servers
	servers, err := LoadServers(DB)
	if err != nil {
		log.Fatalln(err)
	}
	Servers = servers

	resources := 0
	for _, server := range servers {
		resources += len(server.Resources)
	}
	JobQueue = make(chan *JobInstance, resources)

	// Load Archives
	archives, err := LoadArchives(DB)
	if err != nil {
		log.Fatalln(err)
	}
	Archives = archives

	for i := 0; i < len(Archives); i++ {
		Archives[i].Connect()
	}

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
	jobs, err := LoadJobs(DB)
	if err != nil {
		log.Fatalln(err)
	}
	Jobs = jobs

	go func() {
		for _, job := range Jobs {
			for i := 0; i < len(job.Instances); i++ {
				if !job.Instances[i].Completed {
					if job.Instances[i].Resource == nil {
						JobQueue <- &job.Instances[i]
					} else {
						job.Instances[i].Resource.JobQueue <- &job.Instances[i]
					}
				}
			}
		}
	}()

	for i := 0; i < len(Servers); i++ {
		Servers[i].Connect()
		for j := 0; j < len(Servers[i].Resources); j++ {
			log.Println(Servers[i].URL, Servers[i].Resources[j])
			go Servers[i].Resources[j].Handle()
		}
	}

	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
