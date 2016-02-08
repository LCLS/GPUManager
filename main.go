package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"text/template"

	"golang.org/x/crypto/ssh"
)

func main() {
	port := flag.Int("port", 8080, "HTTP Server Port")
	flag.Parse()

	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js/"))))
	http.Handle("/fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("./fonts/"))))
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("./css/"))))
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/server/add", serverAddHandker)

	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles("index.html")
	t.Execute(w, nil)
}

func serverAddHandker(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	w.Header().Set("Content-Type", "application/json")

	if r.FormValue("server_name") == "" || r.FormValue("user_name") == "" || r.FormValue("password") == "" {
		json.NewEncoder(w).Encode(JSONResponse{false, "Missing Data"})
		return
	}

	// Test if we can connect
	config := &ssh.ClientConfig{
		User: r.FormValue("user_name"),
		Auth: []ssh.AuthMethod{
			ssh.Password(r.FormValue("password")),
		},
	}

	client, err := ssh.Dial("tcp", r.FormValue("server_name")+":22", config)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{false, fmt.Sprintf("Error connecting to %s. Please check the url and username/password.", r.FormValue("server_name"))})
		return
	}

	session, err := client.NewSession()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{false, "Unable to create session"})
		return
	}
	defer session.Close()

	result, err := session.CombinedOutput("nvidia-smi -L")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{false, "Unable to execute command"})
		return
	}

	json.NewEncoder(w).Encode(JSONResponse{true, string(result)})
}
