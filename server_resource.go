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
	"time"

	"github.com/pkg/sftp"
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
			time.Sleep(1 * time.Second)
		} else {
			if client == nil {
				client, err = ssh.Dial("tcp", server.URL+":22", config)
				if err != nil {
					log.Fatalln(err)
				}
			}

			if !r.InUse {
				jobInstance := <-JobQueue
				if jobInstance == nil {
					time.Sleep(1 * time.Second)
					continue
				}
				r.InUse = true

				job := FindJob(jobInstance.JobID, Jobs)

				time.Sleep(time.Duration(rand.Int31n(10)) * time.Second)
				log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), jobInstance)

				if jobInstance.PID == -1 {
					// Send Model Data
					log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), "Uploading Model")
					sftp, err := sftp.NewClient(client)
					if err != nil {
						log.Fatal(err)
					}

					sftp.Mkdir(sftp.Join(server.WorkingDirectory, "model"))
					sftp.Mkdir(sftp.Join(server.WorkingDirectory, "model", strings.ToLower(job.Model.Name)))

					for _, file := range job.Model.Files {
						fIn, err := os.Open(fmt.Sprintf("data/%s/%s", job.Model.Name, file))
						if err != nil {
							log.Fatalln(err)
						}
						defer fIn.Close()

						fOut, err := sftp.Create(sftp.Join(server.WorkingDirectory, "model", strings.ToLower(job.Model.Name), file))
						if err != nil {
							log.Fatalln(err)
						}
						defer fIn.Close()

						io.Copy(fOut, fIn)
					}

					time.Sleep(1 * time.Second)
					// Update Template
					log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), "Uploading Template")
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

					time.Sleep(1 * time.Second)
					// Send Template Data
					sftp.Mkdir(sftp.Join(server.WorkingDirectory, "job"))
					sftp.Mkdir(sftp.Join(server.WorkingDirectory, "job", strings.ToLower(job.Name)))
					sftp.Mkdir(sftp.Join(server.WorkingDirectory, "job", strings.ToLower(job.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.ID-job.Instances[0].ID))))

					fOut, err := sftp.Create(sftp.Join(server.WorkingDirectory, "job", strings.ToLower(job.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.ID-job.Instances[0].ID)), "sim.conf"))
					if err != nil {
						log.Fatalln(err)
					}
					fOut.Write(templateData.Bytes())
					fOut.Close()

					sftp.Close()

					time.Sleep(1 * time.Second)
					// Start job and retrieve PID
					log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), "Starting Job")
					session, err := client.NewSession()
					if err != nil {
						log.Fatalln(err)
					}

					command := "/bin/bash\n"
					command += "source ~/.bash_profile\n"
					if server.WorkingDirectory != "" {
						command += "cd " + server.WorkingDirectory + "\n"
					}
					command += strings.ToLower(fmt.Sprintf("cd job/%s/%d\n", job.Name, jobInstance.NumberInSequence(job)))
					command += "bash -c 'ProtoMol sim.conf &> Log.txt & echo $! > pidfile; wait $!; echo $? > exit-status' &> /dev/null &\n"
					command += "sleep 1\n"
					command += "cat pidfile"

					sPID, err := session.CombinedOutput(command)
					if err != nil {
						log.Fatalln(string(sPID), err)
					}
					session.Close()

					log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), string(sPID))

					// Parse PID
					pid, err := strconv.Atoi(strings.TrimSpace(string(sPID)))
					if err != nil {
						log.Fatalln(err)
					}
					log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), "PID:", pid)

					jobInstance.PID = pid
					if _, err := DB.Exec("update job_instance set pid = ? where id = ?", jobInstance.PID, jobInstance.ID); err != nil {
						log.Fatalln(err)
					}
				}

				time.Sleep(1 * time.Second)
				// Wait for completion
				log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), "Waiting for completion")
				session, err := client.NewSession()
				if err != nil {
					log.Fatalln(err)
				}

				command := ""
				if server.WorkingDirectory != "" {
					command += "cd " + server.WorkingDirectory + "\n"
				}
				command += strings.ToLower(fmt.Sprintf("cd job/%s/%d\n", job.Name, jobInstance.NumberInSequence(job)))
				command += fmt.Sprintf("bash -c 'while [[ ( -d /proc/%d ) && ( -z `grep zombie /proc/%d/status` ) ]]; do sleep 1; done; sleep 1; cat exit-status'", jobInstance.PID, jobInstance.PID)

				output, err := session.CombinedOutput(command)
				if err != nil {
					log.Fatalln(string(output), err)
				}

				exitcode, err := strconv.Atoi(strings.TrimSpace(string(output)))
				if err != nil {
					log.Fatalln(err)
				}
				log.Println(fmt.Sprintf("%s[%d]", server.URL, r.DeviceID), "Exit Code:", exitcode)

				jobInstance.Completed = true
				if _, err := DB.Exec("update job_instance set completed = ? where id = ?", jobInstance.Completed, jobInstance.ID); err != nil {
					log.Fatalln(err)
				}
				r.InUse = false
			}
		}
	}
}
