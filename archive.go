package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Archive struct {
	ID                    int
	URL, WorkingDirectory string
	Username, Password    string
	Enabled               bool
	SpaceUsed, SpaceTotal uint64

	Client *ssh.Client
}

func (a *Archive) StringUsedTotal() string {
	total := a.SpaceTotal
	if a.SpaceTotal > (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2d TB / %.2d TB", a.SpaceUsed/(1024*1024*1024), total/(1024*1024*1024))
	} else if total > (1024 * 1024) {
		return fmt.Sprintf("%.2d GB / %.2d GB", a.SpaceUsed/(1024*1024), total/(1024*1024))
	} else if total > 1024 {
		return fmt.Sprintf("%.2d MB / %.2d MB", a.SpaceUsed/1024, total/1024)
	}
	return fmt.Sprintf("%.2d kB / %.2d kB", a.SpaceUsed, total)
}

var Archives []Archive

func LoadArchives(db *sql.DB) ([]Archive, error) {
	var archives []Archive

	// Load Servers
	rows, err := db.Query("SELECT id, url, wdir, username, password, used, total, enabled FROM archive")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var id int
		var enabled bool
		var url, wdir, username, password string
		var used, total uint64
		if err := rows.Scan(&id, &url, &wdir, &username, &password, &used, &total, &enabled); err != nil {
			return nil, err
		}

		archives = append(archives, Archive{ID: id, URL: url, WorkingDirectory: wdir, Username: username, Password: password, SpaceUsed: used, SpaceTotal: total, Enabled: enabled})
	}
	rows.Close()

	return archives, nil
}

func archiveHandler(w http.ResponseWriter, r *http.Request) {
	type ArchiveInfo struct {
		ID                    int
		URL, WorkingDirectory string
		Enabled               bool
		SpaceUsed, SpaceTotal uint64
		SpacePercent          float64
		SpaceText             string
	}

	var data []ArchiveInfo
	for _, archive := range Archives {
		data = append(data, ArchiveInfo{
			ID:               archive.ID,
			URL:              archive.URL,
			WorkingDirectory: archive.WorkingDirectory,
			Enabled:          archive.Enabled,
			SpaceUsed:        archive.SpaceUsed,
			SpaceTotal:       archive.SpaceTotal,
			SpaceText:        archive.StringUsedTotal(),
			SpacePercent:     float64(archive.SpaceUsed) / float64(archive.SpaceTotal) * 100.0,
		})
	}

	t, _ := template.ParseFiles("archive.html")
	t.Execute(w, data)
}

func archiveAddHandler(w http.ResponseWriter, r *http.Request) {
	type ArchiveResponse struct {
		ID               int
		WorkingDirectory string  `json:"wdir"`
		URL              string  `json:"url"`
		SpaceUsed        uint64  `json:"spaceused"`
		SpaceTotal       uint64  `json:"spacetotal"`
		SpacePercent     float64 `json:"spacepercent"`
		SpaceText        string  `json:"spacetext"`
	}

	type JSONResponse struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Archive ArchiveResponse `json:"archive"`
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

	client, err := SSHDialTimeout("tcp", r.FormValue("server_name")+":22", config, 1*time.Minute)
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

	result, err := session.CombinedOutput("mkdir -p " + r.FormValue("root") + "&& df -Pk " + r.FormValue("root"))
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: "Unable to calculate space"})
		return
	}
	session.Close()
	client.Close()

	archive := Archive{URL: r.FormValue("server_name"), Username: r.FormValue("user_name"), Password: r.FormValue("password"), WorkingDirectory: r.FormValue("root"), Enabled: true}

	// Find Space
	scanner := bufio.NewScanner(bytes.NewReader(result))
	scanner.Split(bufio.ScanLines)

	// Ignore First Line
	scanner.Scan()

	// Get Filesystem Data
	scanner.Scan()
	fields := strings.Fields(scanner.Text())

	used, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
	archive.SpaceUsed = used

	free, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}
	archive.SpaceTotal = archive.SpaceUsed + free

	log.Printf("%+v\n", archive)

	res, err := DB.Exec("insert into archive(url, wdir, username, password, used, total) values (?,?,?,?, ?,?)", archive.URL, archive.WorkingDirectory, r.FormValue("user_name"), r.FormValue("password"), archive.SpaceUsed, archive.SpaceTotal)
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		json.NewEncoder(w).Encode(JSONResponse{Success: false, Message: err.Error()})
		return
	}

	archive.ID = int(id)

	Archives = append(Archives, archive)
	json.NewEncoder(w).Encode(JSONResponse{Success: true, Message: string(result), Archive: ArchiveResponse{
		ID:               archive.ID,
		WorkingDirectory: archive.WorkingDirectory,
		URL:              archive.URL,
		SpaceUsed:        archive.SpaceUsed,
		SpaceTotal:       archive.SpaceTotal,
		SpacePercent:     float64(archive.SpaceUsed) / float64(archive.SpaceTotal) * 100.0,
		SpaceText:        archive.StringUsedTotal(),
	}})
}
