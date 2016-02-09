package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	Hostname, URL string
	Resources     []Resource
}

type Resource struct {
	InUse      bool
	Name, UUID string
	Connection *ssh.Session
}

var DB *sql.DB
var servers []Server

func main() {
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

	http.HandleFunc("/job", jobHandler)
	http.HandleFunc("/job/add", jobAddHandler)

	http.HandleFunc("/model", modelHandler)
	http.HandleFunc("/model/add", modelAddHandler)

	http.HandleFunc("/template", templateHandler)
	http.HandleFunc("/template/add", templateAddHandler)

	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
