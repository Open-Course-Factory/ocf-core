package models

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	IpAddress string `json:"ipaddress"`
	Port      int    `json:"port"`
	Session   *ssh.Session
	Client    *ssh.Client
	channel   ssh.Channel
}

func NewSSHClient() SSHClient {
	client := SSHClient{}
	client.Username = "root"
	client.Port = 22
	return client
}

func (sc *SSHClient) GenerateClient(ip, name string, sshkeys []string, port int) error {
	var (
		auth         []ssh.AuthMethod
		addr         string
		clientConfig *ssh.ClientConfig
		client       *ssh.Client
		config       ssh.Config
		err          error
	)

	auth = make([]ssh.AuthMethod, 0)

	for _, sshkey := range sshkeys {
		signer, err := ssh.ParsePrivateKey([]byte(sshkey))
		if err != nil {
			log.Fatalf("unable to parse private key: %v", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}

	// Create the Signer for this private key.
	//auth = append(auth, ssh.Password(password))
	config = ssh.Config{
		Ciphers: []string{"aes128-ctr", "aes192-ctr", "aes256-ctr", "aes128-gcm@openssh.com", "arcfour256", "arcfour128", "aes128-cbc", "3des-cbc", "aes192-cbc", "aes256-cbc"},
	}
	clientConfig = &ssh.ClientConfig{
		User:    name,
		Auth:    auth,
		Timeout: 5 * time.Second,
		Config:  config,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}
	addr = fmt.Sprintf("%s:%d", ip, port)
	if client, err = ssh.Dial("tcp", addr, clientConfig); err != nil {
		return err
	}
	sc.Client = client
	return nil
}

func (sc *SSHClient) RequestTerminal(terminal Terminal) *SSHClient {
	session, err := sc.Client.NewSession()
	if err != nil {
		return nil
	}
	sc.Session = session
	channel, inRequests, err := sc.Client.OpenChannel("session", nil)
	if err != nil {
		return nil
	}
	sc.channel = channel
	go func() {
		for req := range inRequests {
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}()
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	var modeList []byte
	for k, v := range modes {
		kv := struct {
			Key byte
			Val uint32
		}{k, v}
		modeList = append(modeList, ssh.Marshal(&kv)...)
	}
	modeList = append(modeList, 0)
	req := ptyRequestMsg{
		Term:     "xterm",
		Columns:  terminal.Columns,
		Rows:     terminal.Rows,
		Width:    uint32(terminal.Columns * 8),
		Height:   uint32(terminal.Columns * 8),
		ModeList: string(modeList),
	}
	ok, err := channel.SendRequest("pty-req", true, ssh.Marshal(&req))
	if !ok || err != nil {
		return nil
	}
	ok, err = channel.SendRequest("shell", true, nil)
	if !ok || err != nil {
		return nil
	}
	return sc
}

func (sc *SSHClient) Connect(ws *websocket.Conn) {
	go func() {
		for {
			_, p, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_, err = sc.channel.Write(p)
			if err != nil {
				return
			}
		}
	}()

	go func() {
		br := bufio.NewReader(sc.channel)
		buf := []byte{}
		t := time.NewTimer(time.Microsecond * 100)
		defer t.Stop()
		r := make(chan rune)

		go func() {
			defer sc.Client.Close()
			defer sc.Client.Close()

			for {
				x, size, err := br.ReadRune()
				if err != nil {
					log.Println(err)
					ws.WriteMessage(1, []byte("\033[31m已经关闭连接!\033[0m"))
					ws.Close()
					return
				}
				if size > 0 {
					r <- x
				}
			}
		}()

		for {
			select {
			case <-t.C:
				if len(buf) != 0 {
					err := ws.WriteMessage(websocket.TextMessage, buf)
					buf = []byte{}
					if err != nil {
						log.Println(err)
						return
					}
				}
				t.Reset(time.Microsecond * 100)
			case d := <-r:
				if d != utf8.RuneError {
					p := make([]byte, utf8.RuneLen(d))
					utf8.EncodeRune(p, d)
					buf = append(buf, p...)
				} else {
					buf = append(buf, []byte("@")...)
				}
			}
		}
	}()

	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
		}
	}()
}
