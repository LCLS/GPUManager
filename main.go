package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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

	http.HandleFunc("/model", modelHandler)
	http.HandleFunc("/model/add", modelAddHandler)

	http.HandleFunc("/template", templateHandler)
	http.HandleFunc("/template/add", templateAddHandler)

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

type Model struct {
	Name  string
	Files []string
}

var models []Model

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
	if err := t.Execute(w, models); err != nil {
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

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			file, _ := fileHeader.Open()
			buf, _ := ioutil.ReadAll(file)
			ioutil.WriteFile(fmt.Sprintf("data/%s/%s", name, fileHeader.Filename), buf, os.ModePerm)
			model.Files = append(model.Files, fileHeader.Filename)
		}
	}

	models = append(models, model)
	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: "", Model: &model})
}
