package models

import (
	"crypto/md5"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	stdssh "golang.org/x/crypto/ssh"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	authServices "soli/formations/src/auth/services"
	sqldb "soli/formations/src/db"
	"soli/formations/src/utils"
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
				utils.Error("%v", err)
			}
		} else {
			if err = CopyFile(srcfp, dstfp); err != nil {
				utils.Error("%v", err)
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
		utils.Error("cloning repository")
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
		utils.Info("No SSH key found, trying without auth")

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
			utils.Error("%s", err.Error())
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

// RepoCacheInfo stores repository cache metadata
type RepoCacheInfo struct {
	URL        string    `json:"url"`
	Branch     string    `json:"branch"`
	CommitHash string    `json:"commit_hash"`
	CachedAt   time.Time `json:"cached_at"`
}

// GetRemoteCommitHash retrieves the latest commit hash from a remote repository without cloning
func GetRemoteCommitHash(ownerId string, repositoryURL string, repositoryBranch string) (string, error) {
	gitCloneOption, err := prepareGitCloneOptions(ownerId, repositoryURL, repositoryBranch)
	if err != nil {
		return "", err
	}

	// Create a remote reference to get the commit hash without cloning
	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{gitCloneOption.URL},
	})

	// List references from the remote
	refs, err := remote.List(&git.ListOptions{
		Auth: gitCloneOption.Auth,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list remote references: %w", err)
	}

	// Find the commit hash for the specified branch
	branchRef := "refs/heads/" + repositoryBranch
	for _, ref := range refs {
		if ref.Name().String() == branchRef {
			return ref.Hash().String(), nil
		}
	}

	return "", fmt.Errorf("branch %s not found in remote repository", repositoryBranch)
}

// GetCacheKey generates a unique cache key for a repository
func GetCacheKey(repositoryURL string, repositoryBranch string) string {
	hasher := md5.New()
	hasher.Write([]byte(repositoryURL + ":" + repositoryBranch))
	return hex.EncodeToString(hasher.Sum(nil))
}

// GetCacheDir returns the cache directory path
func GetCacheDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "~/.ocf-cache"
	}
	return filepath.Join(homeDir, ".ocf-cache", "repositories")
}

// GetCachedRepo checks if a repository is cached and up-to-date
func GetCachedRepo(ownerId string, repositoryURL string, repositoryBranch string) (billy.Filesystem, bool, error) {
	cacheKey := GetCacheKey(repositoryURL, repositoryBranch)
	cacheDir := GetCacheDir()
	repoCacheDir := filepath.Join(cacheDir, cacheKey)
	cacheInfoFile := filepath.Join(repoCacheDir, ".cache_info.json")

	// Check if cache directory exists
	if _, err := os.Stat(repoCacheDir); os.IsNotExist(err) {
		return nil, false, nil
	}

	// Check if cache info file exists
	if _, err := os.Stat(cacheInfoFile); os.IsNotExist(err) {
		return nil, false, nil
	}

	// Read cache info
	cacheInfoBytes, err := os.ReadFile(cacheInfoFile)
	if err != nil {
		return nil, false, nil
	}

	var cacheInfo RepoCacheInfo
	if err := json.Unmarshal(cacheInfoBytes, &cacheInfo); err != nil {
		return nil, false, nil
	}

	// Get remote commit hash
	remoteCommitHash, err := GetRemoteCommitHash(ownerId, repositoryURL, repositoryBranch)
	if err != nil {
		utils.Warn("Failed to get remote commit hash, using cached version: %v", err)
		// If we can't check remote, use cached version (might be offline)
		fs := osfs.New(repoCacheDir)
		return fs, true, nil
	}

	// Compare commit hashes
	if cacheInfo.CommitHash == remoteCommitHash {
		utils.Debug("Repository cache is up-to-date (commit: %s)", remoteCommitHash[:8])
		fs := osfs.New(repoCacheDir)
		return fs, true, nil
	}

	utils.Info("Repository cache is outdated (cached: %s, remote: %s)", cacheInfo.CommitHash[:8], remoteCommitHash[:8])
	return nil, false, nil
}

// CacheRepo saves a repository to the cache
func CacheRepo(ownerId string, repositoryURL string, repositoryBranch string, fs billy.Filesystem) error {
	cacheKey := GetCacheKey(repositoryURL, repositoryBranch)
	cacheDir := GetCacheDir()
	repoCacheDir := filepath.Join(cacheDir, cacheKey)

	// Create cache directory
	if err := os.MkdirAll(repoCacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Copy files from memory filesystem to cache directory
	err := copyFilesFromBillyFS(fs, repoCacheDir)
	if err != nil {
		return fmt.Errorf("failed to copy files to cache: %w", err)
	}

	// Get the current commit hash
	commitHash, err := GetRemoteCommitHash(ownerId, repositoryURL, repositoryBranch)
	if err != nil {
		utils.Warn("Failed to get commit hash for caching: %v", err)
		commitHash = "unknown"
	}

	// Save cache info
	cacheInfo := RepoCacheInfo{
		URL:        repositoryURL,
		Branch:     repositoryBranch,
		CommitHash: commitHash,
		CachedAt:   time.Now(),
	}

	cacheInfoBytes, err := json.MarshalIndent(cacheInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache info: %w", err)
	}

	cacheInfoFile := filepath.Join(repoCacheDir, ".cache_info.json")
	if err := os.WriteFile(cacheInfoFile, cacheInfoBytes, 0644); err != nil {
		return fmt.Errorf("failed to write cache info: %w", err)
	}

	utils.Info("Repository cached successfully at %s", repoCacheDir)
	return nil
}

// copyFilesFromBillyFS copies files from a billy.Filesystem to a local directory
func copyFilesFromBillyFS(fs billy.Filesystem, destDir string) error {
	return copyBillyFSRecursive(fs, "/", destDir)
}

// copyBillyFSRecursive recursively copies files from billy.Filesystem
func copyBillyFSRecursive(fs billy.Filesystem, srcPath string, destPath string) error {
	files, err := fs.ReadDir(srcPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		srcFilePath := filepath.Join(srcPath, file.Name())
		destFilePath := filepath.Join(destPath, file.Name())

		if file.IsDir() {
			// Create directory and recurse
			if err := os.MkdirAll(destFilePath, file.Mode()); err != nil {
				return err
			}
			if err := copyBillyFSRecursive(fs, srcFilePath, destFilePath); err != nil {
				return err
			}
		} else {
			// Copy file
			srcFile, err := fs.Open(srcFilePath)
			if err != nil {
				return err
			}
			defer srcFile.Close()

			destFile, err := os.Create(destFilePath)
			if err != nil {
				return err
			}
			defer destFile.Close()

			if _, err := io.Copy(destFile, srcFile); err != nil {
				return err
			}

			// Set file permissions
			if err := os.Chmod(destFilePath, file.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

// GitCloneWithCache clones a repository with caching support
func GitCloneWithCache(ownerId string, repositoryURL string, repositoryBranch string) (billy.Filesystem, error) {
	// First, try to get from cache
	if cachedFS, isCached, err := GetCachedRepo(ownerId, repositoryURL, repositoryBranch); err == nil && isCached {
		return cachedFS, nil
	}

	// If not cached or outdated, clone normally
	utils.Info("Cloning repository: %s (branch: %s)", repositoryURL, repositoryBranch)
	fs, err := GitClone(ownerId, repositoryURL, repositoryBranch)
	if err != nil {
		return nil, err
	}

	// Cache the repository for future use
	if err := CacheRepo(ownerId, repositoryURL, repositoryBranch, fs); err != nil {
		utils.Warn("Failed to cache repository: %v", err)
		// Continue with the cloned repository even if caching fails
	}

	return fs, nil
}

// LoadLocalDirectory loads a course from a local filesystem path
func LoadLocalDirectory(localPath string) (billy.Filesystem, error) {
	// Validate path exists
	info, err := os.Stat(localPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("local path not found: %s", localPath)
	}
	if err != nil {
		return nil, fmt.Errorf("error accessing path: %w", err)
	}

	// Ensure it's a directory
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", localPath)
	}

	// Return OS filesystem wrapper
	utils.Info("Loading course from local path: %s", localPath)
	return osfs.New(localPath), nil
}

// LoadTheme loads a theme from either a git repository or a local filesystem path
func LoadTheme(ownerId string, sourceType string, source string, branch string) (billy.Filesystem, error) {
	var fs billy.Filesystem
	var err error

	switch sourceType {
	case "git":
		utils.Info("Loading theme from git repository: %s (branch: %s)", source, branch)
		fs, err = GitClone(ownerId, source, branch)
	case "local":
		utils.Info("Loading theme from local path: %s", source)
		fs, err = LoadLocalDirectory(source)
	default:
		return nil, fmt.Errorf("unknown source type: %s (must be 'git' or 'local')", sourceType)
	}

	return fs, err
}
