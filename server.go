package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/crypto/ssh"
)

type Server struct {
	ID            int
	Hostname, URL string
	Resources     []Resource
}

type Resource struct {
	InUse      bool
	Name, UUID string
	Connection *ssh.Session
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	type ServerInfo struct {
		ID            int
		Hostname, URL string
		Enabled       bool
		InUse, Max    int
		PercentUsed   float64
	}

	rows, err := DB.Query("SELECT server.id, server.url, server.enabled, server_resource.inuse FROM server JOIN server_resource ON server.id=server_resource.server_id")
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	var data []ServerInfo
	for rows.Next() {
		var id int
		var name string
		var enabled, inuse bool
		if err := rows.Scan(&id, &name, &enabled, &inuse); err != nil {
			json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
			return
		}

		found := false
		for i := 0; i < len(data); i++ {
			if data[i].URL == name {
				found = true
				data[i].Max += 1
				if inuse {
					data[i].InUse += 1
				}
				break
			}
		}

		if !found {
			data = append(data, ServerInfo{ID: id, URL: name, Hostname: strings.Split(name, ".")[0], Enabled: enabled, InUse: 0, Max: 1, PercentUsed: 0})
		}
	}

	for i := 0; i < len(data); i++ {
		data[i].PercentUsed = (float64(data[i].InUse) / float64(data[i].Max)) * 100.0
	}

	t, _ := template.ParseFiles("index.html")
	t.Execute(w, data)
}

func serverAddHandker(w http.ResponseWriter, r *http.Request) {
	type ServerResponse struct {
		ID        int
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

	res, err := DB.Exec("insert into server(url, username, password) values (?,?,?)", server.URL, r.FormValue("user_name"), r.FormValue("password"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	server.ID = int(id)

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

			res := Resource{Name: result[0][1], UUID: result[0][2], InUse: false, Connection: session}
			server.Resources = append(server.Resources, res)

			if _, err := DB.Exec("insert into server_resource(uuid, name, inuse, server_id) values (?,?,?,?)", res.UUID, res.Name, res.InUse, id); err != nil {
				json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
				return
			}
		}
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: string(result), Server: ServerResponse{server.ID, server.Hostname, server.URL, len(server.Resources)}})
}

func serverToggleHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Enabled bool   `json:"enabled"`
	}

	w.Header().Set("Content-Type", "application/json")

	id := r.FormValue("id")
	if id == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
		return
	}

	var enabled bool
	if err := DB.QueryRow("SELECT enabled FROM server WHERE id = ?", id).Scan(&enabled); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if _, err := DB.Exec("update server set enabled = ? where id = ?", !enabled, id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true, Enabled: !enabled})
}

func serverRemoveHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	w.Header().Set("Content-Type", "application/json")

	id := r.FormValue("id")
	if id == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
		return
	}

	if _, err := DB.Exec("DELETE FROM server_resource WHERE server_id = ?", id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	if _, err := DB.Exec("DELETE FROM server WHERE id = ?", id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(JSONResponse{Success: true})
}
