package models

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type URLFormat int

const (
	UNKNOWN URLFormat = iota
	GIT_HTML
	GIT_SSH
)

func createFooterAlone(footer string) string {
	return "<!--\n" + "footer: '" + footer + "'\n-->\n\n"
}

func createHeaderFooter(header string, footer string) string {
	return "<!--\n" + "header: '" + header + "'\n" + "footer: '" + footer + "'\n-->\n\n"
}

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
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
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

func SSHToHTML(ssh string) string {
	// Replace the SSH specific parts with the HTML equivalent
	html := strings.Replace(ssh, "git@", "https://", 1)
	html = strings.Replace(html, ".com:", ".com/", 1)
	html = strings.Replace(html, ".git", "", 1)

	return html
}

func HTMLToSSH(html string) string {
	// Replace the HTML specific parts with the SSH equivalent
	ssh := strings.Replace(html, "https://", "git@", 1)
	ssh = strings.Replace(ssh, ".com/", ".com:", 1)
	ssh += ".git"

	return ssh
}

func DetectURLFormat(url string) URLFormat {
	if strings.HasPrefix(url, "https://") {
		return GIT_HTML
	} else if strings.HasPrefix(url, "git@") {
		return GIT_SSH
	}
	return UNKNOWN
}
