package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"text/template"

	"golang.org/x/crypto/ssh"
)

type Server struct {
	ID                    int
	URL, WorkingDirectory string
	Username, Password    string
	Enabled               bool
	Resources             []Resource

	Client *ssh.Client
}

func (s *Server) Connect() error {
	config := &ssh.ClientConfig{
		User: s.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.Password),
		},
	}

	var err error
	if s.Client, err = ssh.Dial("tcp", s.URL+":22", config); err != nil {
		return err
	}
	return nil
}

func (s *Server) Disconnect() {
	s.Client.Close()
	s.Client = nil
}

var Servers []Server

func FindServer(id int, servers []Server) *Server {
	for i := 0; i < len(servers); i++ {
		if servers[i].ID == id {
			return &servers[i]
		}
	}
	return nil
}

func LoadServers(db *sql.DB) ([]Server, error) {
	var servers []Server

	// Load Servers
	rows, err := db.Query("SELECT id, url, wdir, username, password, enabled FROM server")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var id int
		var enabled bool
		var url, wdir, username, password string
		if err := rows.Scan(&id, &url, &wdir, &username, &password, &enabled); err != nil {
			return nil, err
		}

		servers = append(servers, Server{ID: id, URL: url, WorkingDirectory: wdir, Username: username, Password: password, Enabled: enabled})
	}
	rows.Close()

	// Load Resources
	for i := 0; i < len(servers); i++ {
		rows, err := db.Query("SELECT inuse, name, uuid, device FROM server_resource WHERE server_id = ?", servers[i].ID)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var inuse bool
			var device_id int
			var name, uuid string
			if err := rows.Scan(&inuse, &name, &uuid, &device_id); err != nil {
				return nil, err
			}

			servers[i].Resources = append(servers[i].Resources, Resource{Name: name, UUID: uuid, InUse: inuse, DeviceID: device_id, Parent: &servers[i]})
		}
		rows.Close()
	}

	return servers, nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	type ServerInfo struct {
		ID                    int
		URL, WorkingDirectory string
		Enabled               bool
		InUse, Max            int
		PercentUsed           float64
	}

	var data []ServerInfo
	for _, server := range Servers {
		info := ServerInfo{ID: server.ID, URL: server.URL, WorkingDirectory: server.WorkingDirectory, Enabled: server.Enabled, Max: len(server.Resources)}
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
		ID               int
		WorkingDirectory string `json:"wdir"`
		URL              string `json:"url"`
		Resources        int    `json:"resources"`
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
	session.Close()
	client.Close()

	server := Server{URL: r.FormValue("server_name"), Username: r.FormValue("user_name"), Password: r.FormValue("password"), WorkingDirectory: r.FormValue("root"), Enabled: true}

	res, err := DB.Exec("insert into server(url, wdir, username, password) values (?,?,?,?)", server.URL, server.WorkingDirectory, r.FormValue("user_name"), r.FormValue("password"))
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

	device := 0
	re := regexp.MustCompile("GPU \\d+: (.+) \\(UUID: (.+)\\)")
	for scanner.Scan() {
		result := re.FindAllStringSubmatch(scanner.Text(), -1)
		if len(result) == 1 && len(result[0]) == 3 {
			res := Resource{Name: result[0][1], UUID: result[0][2], InUse: false, Parent: &server, DeviceID: device}
			server.Resources = append(server.Resources, res)

			if _, err := DB.Exec("insert into server_resource(uuid, name, inuse, device, server_id) values (?,?,?,?,?)", res.UUID, res.Name, res.InUse, device, id); err != nil {
				json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
				return
			}

			device += 1
		}
	}

	Servers = append(Servers, server)
	for i := 0; i < len(server.Resources); i++ {
		go server.Resources[i].Handle()
	}
	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: string(result), Server: ServerResponse{server.ID, server.WorkingDirectory, server.URL, len(server.Resources)}})
}

func serverToggleHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Enabled bool   `json:"enabled"`
	}

	w.Header().Set("Content-Type", "application/json")

	if r.FormValue("id") == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
		return
	}

	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	server := FindServer(id, Servers)
	if _, err := DB.Exec("update server set enabled = ? where id = ?", !server.Enabled, id); err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	server.Enabled = !server.Enabled
	json.NewEncoder(w).Encode(JSONResponse{Success: true, Enabled: server.Enabled})
}

func serverRemoveHandler(w http.ResponseWriter, r *http.Request) {
	type JSONResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}

	w.Header().Set("Content-Type", "application/json")

	if r.FormValue("id") == "" {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Missing Data"})
		return
	}

	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
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

	// Remove Servers
	index := 0
	for i := 0; i < len(Servers); i++ {
		if Servers[i].ID == id {
			index = i
			break
		}
	}
	Servers = append(Servers[:index], Servers[index+1:]...)

	json.NewEncoder(w).Encode(JSONResponse{Success: true})
}
