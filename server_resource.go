package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

type Resource struct {
	DeviceID   int
	InUse      bool
	Name, UUID string
	Parent     *Server
	JobQueue   chan *JobInstance
}

func (r *Resource) Handle() {
	Log := log.New(os.Stdout, fmt.Sprintf("%s[%d] ", r.Parent.URL, r.DeviceID), log.Ltime|log.Lshortfile)

	for {
		time.Sleep(1 * time.Second)

		if !r.Parent.Enabled || r.InUse {
			continue
		}

		// Get Job
		var jobInstance *JobInstance = nil
		select {
		case r := <-r.JobQueue:
			jobInstance = r
		default:

		}

		if jobInstance == nil {
			select {
			case a := <-JobQueue:
				jobInstance = a
			default:
			}
		}

		if jobInstance == nil {
			continue
		}
		r.InUse = true

		Log.Println(jobInstance)

		if jobInstance.PID == -1 {
			// Send Model Data
			Log.Println("Uploading Model")
			sftp, err := sftp.NewClient(r.Parent.Client)
			if err != nil {
				Log.Fatal(err)
			}

			sftp.Mkdir(sftp.Join(r.Parent.WorkingDirectory, "model"))
			sftp.Mkdir(sftp.Join(r.Parent.WorkingDirectory, "model", strings.ToLower(jobInstance.Parent.Model.Name)))

			for _, file := range jobInstance.Parent.Model.Files {
				fIn, err := os.Open(fmt.Sprintf("data/%s/%s", jobInstance.Parent.Model.Name, file))
				if err != nil {
					Log.Fatalln(err)
				}

				fOut, err := sftp.Create(sftp.Join(r.Parent.WorkingDirectory, "model", strings.ToLower(jobInstance.Parent.Model.Name), file))
				if err != nil {
					Log.Fatalln(err)
				}

				io.Copy(fOut, fIn)

				fIn.Close()
				fIn.Close()
			}

			// Setup Job Type Parameters
			executable := ""
			configuration := ""

			if jobInstance.Parent.Template.IsProtoMol() {
				executable = "ProtoMol"
				configuration = "sim.conf"
			} else {
				executable = "python"
				configuration = "sim.py"
			}

			// Send Template Data
			template, err := jobInstance.Parent.Template.Process(r.DeviceID, *jobInstance.Parent)
			if err != nil {
				Log.Fatalln(err)
			}

			sftp.Mkdir(sftp.Join(r.Parent.WorkingDirectory, "job"))
			sftp.Mkdir(sftp.Join(r.Parent.WorkingDirectory, "job", strings.ToLower(jobInstance.Parent.Name)))
			sftp.Mkdir(sftp.Join(r.Parent.WorkingDirectory, "job", strings.ToLower(jobInstance.Parent.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.NumberInSequence()))))

			fOut, err := sftp.Create(sftp.Join(r.Parent.WorkingDirectory, "job", strings.ToLower(jobInstance.Parent.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.NumberInSequence())), configuration))
			if err != nil {
				Log.Fatalln(err)
			}
			fOut.Write(template)
			fOut.Close()

			sftp.Close()

			// Start job and retrieve PID
			Log.Println("Starting Job")
			session, err := r.Parent.Client.NewSession()
			if err != nil {
				Log.Fatalln(err)
			}

			command := "/bin/bash\n"
			command += "source ~/.bash_profile\n"
			if r.Parent.WorkingDirectory != "" {
				command += "cd " + r.Parent.WorkingDirectory + "\n"
			}
			command += strings.ToLower(fmt.Sprintf("cd job/%s/%d\n", jobInstance.Parent.Name, jobInstance.NumberInSequence()))
			command += "bash -c '(" + executable + " " + configuration + " &> log.txt 2>&1 & echo $! > pidfile); sleep 1; wait $(cat pidfile); echo $? > exit-status' &> /dev/null &\n"
			command += "sleep 1\n"
			command += "cat pidfile"

			sPID, err := session.CombinedOutput(command)
			if err != nil {
				Log.Fatalln(string(sPID), err)
			}
			session.Close()

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

			if _, err := DB.Exec("update job_instance set resource_id = ? where id = ?", r.UUID, jobInstance.ID); err != nil {
				Log.Fatalln(err)
			}
		}

		// Wait for completion
		Log.Println("Waiting for completion")
		for {
			session, err := r.Parent.Client.NewSession()
			if err != nil {
				Log.Println(err)
				r.Parent.Connect()
			}

			command := ""
			if r.Parent.WorkingDirectory != "" {
				command += "cd " + r.Parent.WorkingDirectory + "\n"
			}
			command += strings.ToLower(fmt.Sprintf("cd job/%s/%d\n", jobInstance.Parent.Name, jobInstance.NumberInSequence()))
			command += fmt.Sprintf("if [[ ( ! -d /proc/%d ) || ( ! -z `grep zombie /proc/%d/status` ) ]]; then cat exit-status; fi", jobInstance.PID, jobInstance.PID)

			output, err := session.CombinedOutput(command)
			if err != nil {
				Log.Fatalln(string(output), err)
			}
			session.Close()

			if string(output) != "" {
				exitcode, err := strconv.Atoi(strings.TrimSpace(string(output)))
				if err != nil {
					Log.Fatalln(err)
				}
				Log.Println("Exit Code:", exitcode)

				break
			}

			time.Sleep(30 * time.Second)
		}

		// Copy Results to Archives
		for _, archive := range Archives {
			if !archive.Enabled {
				continue
			}

			jobFtp, err := sftp.NewClient(r.Parent.Client)
			if err != nil {
				Log.Fatal(err)
			}

			archiveFtp, err := sftp.NewClient(archive.Client)
			if err != nil {
				Log.Fatal(err)
			}

			workingPath := jobFtp.Join(strings.ToLower(jobInstance.Parent.Name), strings.ToLower(fmt.Sprintf("%d", jobInstance.NumberInSequence())))

			// Create Directory
			archiveFtp.Mkdir(archiveFtp.Join(archive.WorkingDirectory, strings.ToLower(jobInstance.Parent.Name)))
			archiveFtp.Mkdir(archiveFtp.Join(archive.WorkingDirectory, workingPath))

			// Find Files
			files, err := jobFtp.ReadDir(jobFtp.Join(r.Parent.WorkingDirectory, "job", workingPath))
			if err != nil {
				log.Fatal(err)
			}

			// Copy Files
			for _, file := range files {
				fIn, err := jobFtp.Open(jobFtp.Join(r.Parent.WorkingDirectory, "job", workingPath, file.Name()))
				if err != nil {
					Log.Fatalln(err)
				}
				defer fIn.Close()

				fOut, err := archiveFtp.Create(archiveFtp.Join(archive.WorkingDirectory, workingPath, file.Name()))
				if err != nil {
					Log.Fatalln(err)
				}
				defer fOut.Close()

				io.Copy(fOut, fIn)
			}
		}

		jobInstance.Completed = true
		if _, err := DB.Exec("update job_instance set completed = ? where id = ?", jobInstance.Completed, jobInstance.ID); err != nil {
			Log.Fatalln(err)
		}
		r.InUse = false
	}
}
