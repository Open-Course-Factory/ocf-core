package models

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	stdssh "golang.org/x/crypto/ssh"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	authServices "soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
)

type URLFormat int

const (
	UNKNOWN URLFormat = iota
	GIT_HTTP
	GIT_SSH
)

func contains(intArray []int, intToFind int) bool {
	for _, v := range intArray {
		if v == intToFind {
			return true
		}
	}
	return false
}

func isMn(r rune) bool {
	return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks
}

func removeAccents(input string) string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(isMn), norm.NFC)
	result, _, _ := transform.String(t, input)
	return result
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func CopyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	return os.Chmod(dst, srcinfo.Mode())
}

func CopyDir(src string, dst string) error {
	var err error
	var fds []os.DirEntry
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = os.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		} else {
			if err = CopyFile(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}

func SSHToHTTP(ssh string) string {
	// Replace the SSH specific parts with the http equivalent
	http := strings.Replace(ssh, "git@", "https://", 1)
	http = strings.Replace(http, ".com:", ".com/", 1)
	http = strings.Replace(http, ".git", "", 1)

	return http
}

func HTTPToSSH(http string) string {
	// Replace the http specific parts with the SSH equivalent
	ssh := strings.Replace(http, "https://", "git@", 1)
	ssh = strings.Replace(ssh, ".com/", ".com:", 1)
	ssh += ".git"

	return ssh
}

func DetectURLFormat(url string) URLFormat {
	if strings.HasPrefix(url, "https://") {
		return GIT_HTTP
	} else if strings.HasPrefix(url, "git@") {
		return GIT_SSH
	}
	return UNKNOWN
}

type StringArray []string

func (o *StringArray) Scan(src any) error {
	bytes, ok := src.([]byte)
	if !ok {
		return errors.New("src value cannot cast to []byte")
	}
	*o = strings.Split(string(bytes), ",")
	return nil
}
func (o StringArray) Value() (driver.Value, error) {
	if len(o) == 0 {
		return nil, nil
	}
	return strings.Join(o, ","), nil
}

func GitClone(ownerId string, repositoryURL string, repositoryBranch string) (billy.Filesystem, error) {
	gitCloneOption, err := prepareGitCloneOptions(ownerId, repositoryURL, repositoryBranch)
	if err != nil {
		return nil, err
	}

	fs := memfs.New()

	_, errClone := git.Clone(memory.NewStorage(), fs, gitCloneOption)

	if errClone != nil {
		log.Printf("cloning repository")
		return nil, errClone
	}
	return fs, nil
}

func prepareGitCloneOptions(userId string, courseURL string, branchName ...string) (*git.CloneOptions, error) {
	var key ssh.AuthMethod
	var gitCloneOption *git.CloneOptions

	if branchName[0] == "" {
		branchName[0] = "main"
	}

	sks := authServices.NewSshKeyService(sqldb.DB)
	sshKeys, errSsh := sks.GetKeysByUserId(userId)

	if errSsh != nil {
		return nil, errSsh
	}

	if len(*sshKeys) == 0 {
		log.Printf("No SSH key found, trying without auth")

		urlFormat := DetectURLFormat(courseURL)

		if urlFormat == GIT_SSH {
			courseURL = SSHToHTTP(courseURL)
		}

		gitCloneOption = &git.CloneOptions{
			URL:           courseURL,
			Progress:      os.Stdout,
			ReferenceName: plumbing.ReferenceName("refs/heads/" + branchName[0]),
			SingleBranch:  true,
		}

	} else {
		array := *sshKeys
		firstKey := array[0].PrivateKey

		var err error
		key, err = ssh.NewPublicKeys("git", []byte(firstKey), "")

		if err != nil {
			log.Default().Println(err.Error())
			return nil, err
		}

		key.(*ssh.PublicKeys).HostKeyCallback = stdssh.InsecureIgnoreHostKey()

		urlFormat := DetectURLFormat(courseURL)

		if urlFormat == GIT_HTTP {
			courseURL = HTTPToSSH(courseURL)
		}

		gitCloneOption = &git.CloneOptions{
			Auth:          key,
			URL:           courseURL,
			Progress:      os.Stdout,
			ReferenceName: plumbing.ReferenceName("refs/heads/" + branchName[0]),
			SingleBranch:  true,
		}
	}
	return gitCloneOption, nil
}

func GetRepoNameFromURL(url string) string {
	// Trim the suffix ".git" if present
	cleanURL := strings.TrimSuffix(url, ".git")

	// Find the last index of "/" which precedes the repository name
	lastSlashIndex := strings.LastIndex(cleanURL, "/")
	if lastSlashIndex == -1 {
		return "" // Return an empty string if "/" is not found
	}

	// Extract the repository name using the last slash index
	repoName := cleanURL[lastSlashIndex+1:]
	return repoName
}
