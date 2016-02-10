package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/crypto/ssh"
)

type Resource struct {
	ServerID, DeviceID int
	InUse              bool
	Name, UUID         string
}

func (r *Resource) Handle() {
	log.Println(r)
	server := FindServer(r.ServerID, Servers)

	config := &ssh.ClientConfig{
		User: server.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(server.Password),
		},
	}

	var err error
	var client *ssh.Client = nil
	for {
		if !server.Enabled {
			client = nil
		} else {
			if client == nil {
				client, err = ssh.Dial("tcp", server.URL+":22", config)
				if err != nil {
					log.Fatalln(err)
				}
			}

			if !r.InUse {
				jobInstance := JobQueue.Pop().(JobInstance)
				job := FindJob(jobInstance.JobID, Jobs)

				// Send Model Data
				session, err := client.NewSession()
				if err != nil {
					log.Fatalln(err)
				}

				go func() {
					w, _ := session.StdinPipe()
					fmt.Fprintln(w, "D0755", 0, "model")
					fmt.Fprintln(w, "D0755", 0, strings.ToLower(job.Model.Name))
					for _, file := range job.Model.Files {
						fIn, err := os.Open(fmt.Sprintf("data/%s/%s", job.Model.Name, file))
						if err != nil {
							log.Fatalln(err)
						}

						fStat, err := fIn.Stat()
						if err != nil {
							log.Fatalln(err)
						}

						fmt.Fprintln(w, "C0644", fStat.Size(), file)
						io.Copy(w, fIn)
						fmt.Fprint(w, "\x00")
					}
					w.Close()
				}()

				if server.WorkingDirectory != "" {
					if err := session.Run("scp -tr " + server.WorkingDirectory); err != nil {
						log.Fatalln(err)
					}
				} else {
					if err := session.Run("scp -tr ./"); err != nil {
						log.Fatalln(err)
					}
				}
				session.Close()

				// Update Template
				type TemplateData struct {
					Input, Output  string
					Seed, DeviceID int
				}

				data := TemplateData{Seed: rand.Int()}
				data.DeviceID = r.DeviceID
				data.Output = fmt.Sprintf("sim.%d.dcd", data.Seed)
				for _, file := range job.Model.Files {
					parts := strings.Split(strings.ToLower(file), ".")
					if parts[len(parts)-1] == "tpr" {
						data.Input = strings.ToLower(fmt.Sprintf("gromacstprfile ../../../model/%s/%s", job.Model.Name, file))
						break
					}
				}

				temp, err := template.New(strings.Split(job.Template.File, "/")[1]).ParseFiles(job.Template.File)
				if err != nil {
					log.Fatalln(err)
				}

				var templateData bytes.Buffer
				if err := temp.Execute(&templateData, data); err != nil {
					log.Fatalln(err)
				}

				// Send Template Data
				session, err = client.NewSession()
				if err != nil {
					log.Fatalln(err)
				}

				go func() {
					w, _ := session.StdinPipe()
					fmt.Fprintln(w, "D0755", 0, "job")
					fmt.Fprintln(w, "D0755", 0, strings.ToLower(job.Name))
					fmt.Fprintln(w, "D0755", 0, strings.ToLower(fmt.Sprintf("%d", jobInstance.ID-job.Instances[0].ID)))
					fmt.Fprintln(w, "C0644", templateData.Len(), "sim.conf")
					w.Write(templateData.Bytes())
					fmt.Fprint(w, "\x00")
					w.Close()
				}()

				if server.WorkingDirectory != "" {
					if err := session.Run("scp -tr " + server.WorkingDirectory); err != nil {
						log.Fatalln(err)
					}
				} else {
					if err := session.Run("scp -tr ./"); err != nil {
						log.Fatalln(err)
					}
				}
				session.Close()

				r.InUse = true

				// Start job and retrieve PID
				session, err = client.NewSession()
				if err != nil {
					log.Fatalln(err)
				}

				command := "/bin/bash\n"
				command += "source ~/.bash_profile\n"
				if server.WorkingDirectory != "" {
					command += "cd " + server.WorkingDirectory + "\n"
				}
				command += strings.ToLower(fmt.Sprintf("cd job/%s/%d\n", job.Name, jobInstance.ID-job.Instances[0].ID))
				command += "bash -c 'ProtoMol sim.conf &> log.txt 2>1 & echo $! > pidfile; wait $!; echo $? > exit-status' &> /dev/null &\n"
				command += "cat pidfile"

				sPID, err := session.CombinedOutput(command)
				if err != nil {
					log.Fatalln(err)
				}

				// Parse PID
				pid, err := strconv.Atoi(strings.TrimSpace(string(sPID)))
				if err != nil {
					log.Fatalln(err)
				}
				log.Println(pid)

				log.Println("Loop")
				for {

				}

				session.Close()
			}
		}
	}
}
