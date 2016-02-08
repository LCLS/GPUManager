package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"text/template"

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

var servers []Server

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
	type ServerInfo struct {
		Hostname, URL string
		InUse, Max    int
		PercentUsed   float64
	}

	var data []ServerInfo

	for _, server := range servers {
		info := ServerInfo{Hostname: server.Hostname, URL: server.URL, Max: len(server.Resources)}
		for _, resource := range server.Resources {
			if resource.InUse {
				info.InUse += 1
			}
		}
		info.PercentUsed = (float64(info.InUse) / float64(info.Max)) * 100.0

		data = append(data, info)
	}

	t, _ := template.ParseFiles("index.html")
	t.Execute(w, data)
}

func serverAddHandker(w http.ResponseWriter, r *http.Request) {
	type ServerResponse struct {
		Hostname  string `json:"hostname"`
		URL       string `json:"url"`
		Resources int    `json:"resources"`
	}

	type JSONResponse struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Server  ServerResponse `json:"server"`
	}

	w.Header().Set("Content-Type", "application/json")

	if r.FormValue("server_name") == "" || r.FormValue("user_name") == "" || r.FormValue("password") == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
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
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: fmt.Sprintf("Error connecting to %s. Please check the url and username/password.", r.FormValue("server_name"))})
		return
	}

	session, err := client.NewSession()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to create session"})
		return
	}
	defer session.Close()

	result, err := session.CombinedOutput("nvidia-smi -L")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to execute command"})
		return
	}

	server := Server{Hostname: strings.Split(r.FormValue("server_name"), ".")[0], URL: r.FormValue("server_name")}

	// Find GPUs
	scanner := bufio.NewScanner(bytes.NewReader(result))
	scanner.Split(bufio.ScanLines)

	re := regexp.MustCompile("GPU \\d+: (.+) \\(UUID: (.+)\\)")
	for scanner.Scan() {
		result := re.FindAllStringSubmatch(scanner.Text(), -1)
		if len(result) == 1 && len(result[0]) == 3 {
			session, err := client.NewSession()
			if err != nil {
				json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to create session"})
				return
			}

			server.Resources = append(server.Resources, Resource{Name: result[0][1], UUID: result[0][2], InUse: false, Connection: session})
		}
	}

	servers = append(servers, server)

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: string(result), Server: ServerResponse{server.Hostname, server.URL, len(server.Resources)}})
}
