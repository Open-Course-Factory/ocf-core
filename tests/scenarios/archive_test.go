package scenarios_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/services"
	"soli/formations/src/scenarios/utils"
)

func TestExtractArchive_Zip(t *testing.T) {
	// Create a zip archive with test files
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	// Create zip file
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	// Add index.json
	fw, err := w.Create("index.json")
	require.NoError(t, err)
	_, err = fw.Write([]byte(`{"title":"Test"}`))
	require.NoError(t, err)

	// Add a subdirectory file
	fw, err = w.Create("step1/text.md")
	require.NoError(t, err)
	_, err = fw.Write([]byte("# Step 1"))
	require.NoError(t, err)

	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	// Extract
	err = utils.ExtractArchive(zipPath, destDir)
	require.NoError(t, err)

	// Verify files exist
	data, err := os.ReadFile(filepath.Join(destDir, "index.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"title":"Test"}`, string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "step1", "text.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Step 1", string(data))
}

func TestExtractArchive_TarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	destDir := filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	// Create tar.gz archive
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	// Add a directory
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "step1/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	}))

	// Add index.json
	content := []byte(`{"title":"TarTest"}`)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "index.json",
		Size:     int64(len(content)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}))
	_, err = tw.Write(content)
	require.NoError(t, err)

	// Add step file
	stepContent := []byte("# Step 1 from tar")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "step1/text.md",
		Size:     int64(len(stepContent)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}))
	_, err = tw.Write(stepContent)
	require.NoError(t, err)

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	// Extract
	err = utils.ExtractArchive(archivePath, destDir)
	require.NoError(t, err)

	// Verify
	data, err := os.ReadFile(filepath.Join(destDir, "index.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"title":"TarTest"}`, string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "step1", "text.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Step 1 from tar", string(data))
}

func TestExtractArchive_PathTraversal_Zip(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "evil.zip")
	destDir := filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	// Create a zip with a path traversal entry
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	fw, err := w.Create("../../../etc/passwd")
	require.NoError(t, err)
	_, err = fw.Write([]byte("evil content"))
	require.NoError(t, err)

	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	err = utils.ExtractArchive(zipPath, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestExtractArchive_PathTraversal_TarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "evil.tar.gz")
	destDir := filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	content := []byte("evil content")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "../../../etc/passwd",
		Size:     int64(len(content)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}))
	_, err = tw.Write(content)
	require.NoError(t, err)

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	err = utils.ExtractArchive(archivePath, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestExtractArchive_SizeLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large archive test in short mode")
	}

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "big.zip")
	destDir := filepath.Join(tmpDir, "extracted")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	// Create a zip with a file exceeding 50MB
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	fw, err := w.Create("bigfile.bin")
	require.NoError(t, err)

	// Write 51MB of data
	chunk := strings.Repeat("x", 1024*1024) // 1MB
	for i := 0; i < 51; i++ {
		_, err = fw.Write([]byte(chunk))
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	err = utils.ExtractArchive(zipPath, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size exceeds limit")
}

func TestExtractArchive_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.rar")
	require.NoError(t, os.WriteFile(filePath, []byte("not a rar"), 0644))

	err := utils.ExtractArchive(filePath, tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported archive format")
}

func TestFindIndexJSON_AtRoot(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "index.json"), []byte("{}"), 0644))

	dir, err := utils.FindIndexJSON(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, dir)
}

func TestFindIndexJSON_OneLevel(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "my-scenario")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "index.json"), []byte("{}"), 0644))

	dir, err := utils.FindIndexJSON(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, subDir, dir)
}

func TestFindIndexJSON_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := utils.FindIndexJSON(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index.json not found")
}

func TestScenarioImporter_ImportFromDirectory_SourceType(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	indexJSON := `{
		"title": "Upload Test Lab",
		"description": "Testing upload source type",
		"difficulty": "beginner",
		"time": "10m",
		"details": {
			"intro": {"text": ""},
			"steps": [
				{"title": "Step One", "text": "", "verify": "", "background": "", "foreground": "", "hint": ""}
			],
			"finish": {"text": ""}
		},
		"backend": {"imageid": "alpine"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-789", nil, "upload")

	require.NoError(t, err)
	assert.Equal(t, "upload", scenario.SourceType)
	assert.Equal(t, "upload-test-lab", scenario.Name)
}

func TestScenarioImporter_ImportFromDirectory_DefaultSourceType(t *testing.T) {
	db := setupTestDB(t)
	importer := services.NewScenarioImporterService(db)

	tmpDir := t.TempDir()

	indexJSON := `{
		"title": "Default Source Lab",
		"description": "",
		"difficulty": "",
		"time": "",
		"details": {
			"intro": {"text": ""},
			"steps": [
				{"title": "Step One", "text": "", "verify": "", "background": "", "foreground": "", "hint": ""}
			],
			"finish": {"text": ""}
		},
		"backend": {"imageid": "alpine"}
	}`
	writeTestFile(t, tmpDir, "index.json", indexJSON)

	scenario, err := importer.ImportFromDirectory(tmpDir, "user-000", nil, "")

	require.NoError(t, err)
	assert.Equal(t, "builtin", scenario.SourceType)
}
