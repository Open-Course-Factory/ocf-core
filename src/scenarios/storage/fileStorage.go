package storage

// FileStorage defines the interface for storing and retrieving file content.
type FileStorage interface {
	Store(content string) (ref string, err error)
	Retrieve(ref string) (content string, err error)
	Delete(ref string) error
	Type() string
}

// DatabaseStorage is a no-op pass-through implementation of FileStorage.
// Content lives directly in the ProjectFile model's Content field,
// so no external storage management is needed.
type DatabaseStorage struct{}

func NewDatabaseStorage() *DatabaseStorage {
	return &DatabaseStorage{}
}

func (d *DatabaseStorage) Store(content string) (string, error) {
	// Content is stored directly in the model; no external ref needed.
	return "", nil
}

func (d *DatabaseStorage) Retrieve(ref string) (string, error) {
	// Content is read directly from the model; nothing to retrieve externally.
	return "", nil
}

func (d *DatabaseStorage) Delete(ref string) error {
	// Content is deleted with the model row; nothing to clean up externally.
	return nil
}

func (d *DatabaseStorage) Type() string {
	return "database"
}
