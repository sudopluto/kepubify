package kepub

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type files struct {
	content map[string][]byte
	m       *sync.RWMutex
}

func newFiles() *files {
	return &files{
		content: map[string][]byte{},
		m:       &sync.RWMutex{},
	}
}

func (f *files) Exists(path string) bool {
	f.m.RLock()
	defer f.m.RUnlock()

	path = filepath.ToSlash(filepath.Clean(path))
	_, ok := f.content[path]
	return ok
}

func (f *files) Remove(path string) (exists bool) {
	f.m.Lock()
	defer f.m.Unlock()

	path = filepath.ToSlash(filepath.Clean(path))
	_, ok := f.content[path]
	delete(f.content, path)
	return ok
}

func (f *files) RemoveAll(path string) (exists bool) {
	f.m.Lock()
	defer f.m.Unlock()

	path = filepath.ToSlash(filepath.Clean(path))

	exists = false
	for fpath := range f.content {
		if strings.HasPrefix(fpath, path) {
			exists = true
			delete(f.content, fpath)
		}
	}

	delete(f.content, path)
	return exists
}

func (f *files) Write(path string, contents []byte) {
	f.m.Lock()
	defer f.m.Unlock()

	path = filepath.ToSlash(filepath.Clean(path))
	f.content[path] = contents
}

func (f *files) Read(path string) (contents []byte, exists bool) {
	f.m.RLock()
	defer f.m.RUnlock()

	path = filepath.ToSlash(filepath.Clean(path))
	contents, exists = f.content[path]
	return contents, exists
}

func (f *files) List() []string {
	f.m.RLock()
	defer f.m.RUnlock()

	fs := make([]string, 0, len(f.content))
	for path := range f.content {
		path = filepath.ToSlash(filepath.Clean(path))
		fs = append(fs, path)
	}
	return fs
}

func unpack(epub string) (*files, error) {
	if !exists(epub) {
		return nil, fmt.Errorf(`"%s" does not exist`, epub)
	}

	zipReader, err := zip.OpenReader(epub)
	if err != nil {
		return nil, err
	}
	defer zipReader.Close()

	files := newFiles()
	for _, file := range zipReader.File {
		fileReader, err := file.Open()
		if err != nil {
			return nil, err
		}

		b, err := ioutil.ReadAll(fileReader)
		if err != nil {
			fileReader.Close()
			return nil, err
		}
		fileReader.Close()

		files.Write(file.Name, b)
	}

	return files, nil
}

func pack(epub string, overwrite bool, epubFiles *files) error {
	if !epubFiles.Exists("META-INF/container.xml") {
		return fmt.Errorf("could not find META-INF/container.xml")
	}

	if overwrite && exists(epub) {
		err := os.RemoveAll(epub)
		if err != nil {
			return fmt.Errorf("error removing existing epub output: %v", err)
		}
	}

	if !overwrite && exists(epub) {
		return fmt.Errorf("epub output already exists")
	}

	writer, err := os.Create(epub)
	if err != nil {
		return err
	}
	defer writer.Close()

	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	mimetypeWriter, err := zipWriter.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	})

	if err != nil {
		return err
	}

	_, err = mimetypeWriter.Write([]byte("application/epub+zip"))
	if err != nil {
		return err
	}

	for _, name := range epubFiles.List() {
		if name == "mimetype" {
			continue
		}

		var f io.Writer

		f, err := zipWriter.Create(name)
		if err != nil {
			return err
		}

		contents, _ := epubFiles.Read(name)
		_, err = f.Write(contents)
		if err != nil {
			return err
		}
	}

	return nil
}
