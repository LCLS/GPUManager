package main

import (
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

type Conn struct {
	net.Conn
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (c *Conn) Read(b []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.ReadTimeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *Conn) Write(b []byte) (int, error) {
	if err := c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
}

func SSHDialTimeout(network, addr string, config *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	conn, err := net.DialTimeout(network, addr, timeout)
	if err != nil {
		return nil, err
	}

	timeoutConn := &Conn{conn, timeout, timeout}
	c, chans, reqs, err := ssh.NewClientConn(timeoutConn, addr, config)
	if err != nil {
		return nil, err
	}
	client := ssh.NewClient(c, chans, reqs)

	// this sends keepalive packets every 2 seconds
	// there's no useful response from these, so we can just abort if there's an error
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			<-t.C
			if _, _, err := client.Conn.SendRequest("keepalive@golang.org", true, nil); err != nil {
				return
			}
		}
	}()
	return client, nil
}
