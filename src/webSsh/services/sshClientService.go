package services

import (
	"encoding/base64"
	"encoding/json"
	models "soli/formations/src/webSsh/models"
)

const (
	Layout     = "2006-01-02 15:04:05"
	LayoutDate = "2006-01-02"
)

type SshClientService interface {
	DecodeMsgToSSHClient(msg string) (models.SSHClient, error)
}

type sshClientService struct {
}

func NewSshClientService() SshClientService {
	return &sshClientService{}
}

func (s sshClientService) DecodeMsgToSSHClient(msg string) (models.SSHClient, error) {
	client := models.NewSSHClient()
	decoded, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return client, err
	}
	err = json.Unmarshal(decoded, &client)
	if err != nil {
		return client, err
	}
	return client, nil
}
