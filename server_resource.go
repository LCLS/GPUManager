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

	client *ssh.Client
}

func (r *Resource) Connect() error {
	if r.client == nil {
		server := FindServer(r.ServerID, Servers)

		config := &ssh.ClientConfig{
			User: server.Username,
			Auth: []ssh.AuthMethod{
				ssh.Password(server.Password),
			},
		}

		var err error
		if r.client, err = ssh.Dial("tcp", server.URL+":22", config); err != nil {
			return err
		}
	}

	return nil
}

func (r *Resource) Disconnect() error {
	if r.client == nil {
		return nil
	}
	err := r.client.Close()
	r.client = nil
	return err
}

func (r *Resource) Handle() {
	server := FindServer(r.ServerID, Servers)
	Log := log.New(os.Stdout, fmt.Sprintf("%s[%d] ", server.URL, r.DeviceID), log.Ltime|log.Lshortfile)

	for {
		time.Sleep(1 * time.Second)
		if !server.Enabled {
			r.Disconnect()
			continue
		}
		r.Connect()

		if r.InUse {
			continue
		}
		jobInstance := <-JobQueue
		if jobInstance == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		r.InUse = true

		job := FindJob(jobInstance.JobID, Jobs)

		time.Sleep(time.Duration(rand.Int31n(10)) * time.Second)
		Log.Println(jobInstance)

		if jobInstance.PID == -1 {
			// Send Model Data
			Log.Println("Uploading Model")
			sftp, err := sftp.NewClient(r.client)
			if err != nil {
				Log.Fatal(err)
			}

			sftp.Mkdir(sftp.Join(server.WorkingDirectory, "model"))
			sftp.Mkdir(sftp.Join(server.WorkingDirectory, "model", strings.ToLower(job.Model.Name)))

			for _, file := range job.Model.Files {
				fIn, err := os.Open(fmt.Sprintf("data/%s/%s", job.Model.Name, file))
				if err != nil {
					Log.Fatalln(err)
				}
				defer fIn.Close()

				fOut, err := sftp.Create(sftp.Join(server.WorkingDirectory, "model", strings.ToLower(job.Model.Name), file))
				if err != nil {
					Log.Fatalln(err)
				}
				defer fIn.Close()

				io.Copy(fOut, fIn)
			}

			time.Sleep(1 * time.Second)
			// Update Template
			Log.Println("Uploading Template")
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
				Log.Fatalln(err)
			}

			var templateData bytes.Buffer
			if err := temp.Execute(&templateData, data); err != nil {
				Log.Fatalln(err)
			}

			time.Sleep(1 * time.Second)
			// Send Template Data
			sftp.Mkdir(sftp.Join(server.WorkingDirectory, "job"))
			sftp.Mkdir(sftp.Join(server.WorkingDirectory, "job", strings.ToLower(job.Name)))
			sftp.Mkdir(sftp.Join(server.WorkingDirectory, "job", strings.ToLower(job.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.NumberInSequence(job)))))

			fOut, err := sftp.Create(sftp.Join(server.WorkingDirectory, "job", strings.ToLower(job.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.NumberInSequence(job))), "sim.conf"))
			if err != nil {
				Log.Fatalln(err)
			}
			fOut.Write(templateData.Bytes())
			fOut.Close()

			sftp.Close()

			time.Sleep(1 * time.Second)
			// Start job and retrieve PID
			Log.Println("Starting Job")
			session, err := r.client.NewSession()
			if err != nil {
				Log.Fatalln(err)
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
				Log.Fatalln(string(sPID), err)
			}
			session.Close()

			Log.Println(string(sPID))

			// Parse PID
			pid, err := strconv.Atoi(strings.TrimSpace(string(sPID)))
			if err != nil {
				Log.Fatalln(err)
			}
			Log.Println("PID:", pid)

			jobInstance.PID = pid
			if _, err := DB.Exec("update job_instance set pid = ? where id = ?", jobInstance.PID, jobInstance.ID); err != nil {
				Log.Fatalln(err)
			}
		}

		time.Sleep(1 * time.Second)
		// Wait for completion
		Log.Println("Waiting for completion")
		session, err := r.client.NewSession()
		if err != nil {
			Log.Fatalln(err)
		}

		command := ""
		if server.WorkingDirectory != "" {
			command += "cd " + server.WorkingDirectory + "\n"
		}
		command += strings.ToLower(fmt.Sprintf("cd job/%s/%d\n", job.Name, jobInstance.NumberInSequence(job)))
		command += fmt.Sprintf("bash -c 'while [[ ( -d /proc/%d ) && ( -z `grep zombie /proc/%d/status` ) ]]; do sleep 1; done; sleep 1; cat exit-status'", jobInstance.PID, jobInstance.PID)

		output, err := session.CombinedOutput(command)
		if err != nil {
			Log.Fatalln(string(output), err)
		}

		exitcode, err := strconv.Atoi(strings.TrimSpace(string(output)))
		if err != nil {
			Log.Fatalln(err)
		}
		Log.Println("Exit Code:", exitcode)

		jobInstance.Completed = true
		if _, err := DB.Exec("update job_instance set completed = ? where id = ?", jobInstance.Completed, jobInstance.ID); err != nil {
			Log.Fatalln(err)
		}
		r.InUse = false
	}
}
