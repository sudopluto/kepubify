package kepub

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/beevik/etree"
	"github.com/cheggaaa/pb"
)

func cleanFiles(epubFiles *files) error {
	toRemove := []string{
		"META-INF/calibre_bookmarks.txt",
		"META-INF/iTunesMetadata.plist",
		"META-INF/iTunesArtwork.plist",
		"META-INF/.DS_STORE",
		"META-INF/thumbs.db",
		".DS_STORE",
		"thumbs.db",
		"iTunesMetadata.plist",
		"iTunesArtwork.plist",
	}

	for _, file := range toRemove {
		epubFiles.RemoveAll(file)
	}

	return nil
}

// Kepubify converts a .epub into a .kepub.epub
func Kepubify(src, dest string, printlog bool) error {
	defer func() {
		if printlog {
			fmt.Printf("\n")
		}
	}()

	if printlog {
		fmt.Printf("Reading ePub")
	}
	epubFiles, err := unpack(src)
	if err != nil {
		return fmt.Errorf("could not read epub: %v", err)
	}

	contentfiles := []string{}
	for _, path := range epubFiles.List() {
		if strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".xhtml") || strings.HasSuffix(path, ".htm") {
			contentfiles = append(contentfiles, path)
		}
	}

	if printlog {
		fmt.Printf("\rProcessing %v content files              \n", len(contentfiles))
	}

	var bar *pb.ProgressBar

	if printlog {
		bar = pb.New(len(contentfiles))
		bar.SetRefreshRate(time.Millisecond * 60)
		bar.SetMaxWidth(60)
		bar.Format("[=> ]")
		bar.Start()
	}
	defer func() {
		if printlog && bar != nil {
			bar.Finish()
		}
	}()

	runtime.GOMAXPROCS(runtime.NumCPU() + 1)
	wg := sync.WaitGroup{}
	cerr := make(chan error, 1)
	for _, f := range contentfiles {
		wg.Add(1)
		go func(cf string) {
			defer wg.Done()
			buf, ok := epubFiles.Read(cf)
			if !ok {
				select {
				case cerr <- fmt.Errorf("Could not open content file \"%s\" for reading: does not exist", cf): // Put err in the channel unless it is full
				default:
				}
				return
			}
			str, err := process(string(buf))
			if err != nil {
				select {
				case cerr <- fmt.Errorf("Error processing content file \"%s\": %s", cf, err): // Put err in the channel unless it is full
				default:
				}
				return
			}
			epubFiles.Write(cf, []byte(str))
			time.Sleep(time.Millisecond * 5)
			if printlog {
				bar.Increment()
			}
		}(f)
	}
	wg.Wait()
	if len(cerr) > 0 {
		return <-cerr
	}

	if printlog {
		bar.Finish()
		fmt.Printf("\rCleaning content.opf              ")
	}

	buf, ok := epubFiles.Read("META-INF/container.xml")
	if !ok {
		return fmt.Errorf("error opening container.xml: does not exist")
	}

	container := etree.NewDocument()
	err = container.ReadFromBytes(buf)
	if err != nil {
		return fmt.Errorf("error parsing container.xml: %s", err)
	}

	rootfile := ""
	for _, e := range container.FindElements("//rootfiles/rootfile[@full-path]") {
		rootfile = e.SelectAttrValue("full-path", "")
	}
	if rootfile == "" {
		return fmt.Errorf("error parsing container.xml")
	}

	buf, ok = epubFiles.Read(filepath.ToSlash(rootfile))
	if !ok {
		return fmt.Errorf("error opening content.opf: does not exist")
	}

	opf := string(buf)

	err = processOPF(&opf)
	if err != nil {
		return fmt.Errorf("error cleaning content.opf: %s", err)
	}

	epubFiles.Write(filepath.ToSlash(rootfile), []byte(opf))

	if printlog {
		fmt.Printf("\rCleaning epub files             ")
	}
	cleanFiles(epubFiles)

	if printlog {
		fmt.Printf("\rPacking ePub                    ")
	}
	pack(dest, true, epubFiles)
	debug.FreeOSMemory()
	return nil
}
