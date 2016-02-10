package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
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

				// Start Job
				session, err = client.NewSession()
				if err != nil {
					log.Fatalln(err)
				}

				modes := ssh.TerminalModes{
					ssh.ECHO:          0,     // disable echoing
					ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
					ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
				}

				if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
					log.Fatal(err)
				}

				wp, err := session.StdinPipe()
				if err != nil {
					log.Fatal(err)
				}

				rp, err := session.StdoutPipe()
				if err != nil {
					log.Fatal(err)
				}

				in, out := MuxShell(wp, rp)
				if err := session.Start("/bin/bash"); err != nil {
					log.Fatal(err)
				}
				<-out

				in <- "source ~/.bash_profile"
				<-out

				if server.WorkingDirectory != "" {
					in <- "cd " + server.WorkingDirectory
					<-out
				}

				in <- strings.ToLower(fmt.Sprintf("cd job/%s/%d", job.Name, jobInstance.ID-job.Instances[0].ID))
				<-out

				log.Println(r.UUID, "Starting ProtoMol")
				in <- "ProtoMol sim.conf &> log.txt &"
				log.Println(r.UUID, <-out)

				log.Println(r.UUID, "Waiting for completion")
				in <- "wait $!"
				log.Println(r.UUID, <-out)

				log.Println(r.UUID, "Return Code")
				in <- "echo $?"
				log.Println(r.UUID, <-out)

				session.Close()
			}
		}
	}
}

func MuxShell(w io.Writer, r io.Reader) (chan<- string, <-chan string) {
	in := make(chan string, 1)
	out := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(1) //for the shell itself
	go func() {
		for cmd := range in {
			wg.Add(1)
			w.Write([]byte(cmd + "\n"))
			wg.Wait()
		}
	}()
	go func() {
		var (
			buf [65 * 1024]byte
			t   int
		)
		for {
			n, err := r.Read(buf[t:])
			if err != nil {
				close(in)
				close(out)
				return
			}
			t += n
			if buf[t-2] == '$' { //assuming the $PS1 == 'sh-4.3$ '
				out <- string(buf[:t])
				t = 0
				wg.Done()
			}
		}
	}()
	return in, out
}
