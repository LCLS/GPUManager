package main

import (
	"io"
	"log"
	"sync"

	"golang.org/x/crypto/ssh"
)

type Resource struct {
	InUse            bool
	Name, UUID       string
	WorkingDirectory string
}

func (res *Resource) Handle(s *ssh.Session) {
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := s.RequestPty("xterm", 80, 40, modes); err != nil {
		log.Fatal(err)
	}

	w, err := s.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	r, err := s.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	e, err := s.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}

	in, out := MuxShell(w, r, e)
	if err := s.Start("/bin/bash"); err != nil {
		log.Fatal(err)
	}
	<-out

	in <- "source ~/.bash_profile"
	<-out

	if res.WorkingDirectory != "" {
		in <- "cd " + res.WorkingDirectory
		<-out
	}

	for {
		if !res.InUse {
			//jobInstance := JobQueue.Pop().(*JobInstance)

			// Copy Template and Make Running Directory
			/*go func() {
				w, _ := session.StdinPipe()
				fmt.Fprintln(w, "D0755", 0, "job")
				fmt.Fprintln(w, "D0755", 0, strings.ToLower(jobInstance.Name))

				fIn, err := os.Open(jobInstance.Template)
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
				w.Close()
			}()*/

			/*if err := s.Run("scp -tr ./"); err != nil {
				log.Fatalln(err)
			}*/

			/*r.Input <- "cd Simulation/1VII-Langevin/fermife.crc.nd.edu/0/"
			<-r.Output

			log.Println("Starting ProtoMol")
			r.Input <- "ProtoMol sim.conf &> log.txt &"
			log.Println(r.UUID, <-r.Output)

			r.Input <- "wait $!"
			log.Println(r.UUID, <-r.Output)

			r.Input <- "echo $?"
			log.Println(r.UUID, <-r.Output)*/
		}
	}
}

func MuxShell(w io.Writer, r io.Reader, e io.Reader) (chan<- string, <-chan string) {
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
	go func() {
		var (
			buf [65 * 1024]byte
			t   int
		)
		for {
			n, err := e.Read(buf[t:])
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
