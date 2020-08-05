package events

import (
	"github.com/ksensehq/tracker/appstatus"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var tokenExtractRegexp = regexp.MustCompile("-event-(.*)-\\d\\d\\d\\d-\\d\\d-\\d\\dT")

type Uploader interface {
	Start()
}

type PeriodicUploader struct {
	fileMask       string
	filesBatchSize int
	uploadEvery    time.Duration

	tokenizedEventStorages map[string][]Storage
}

type DummyUploader struct{}

func (*DummyUploader) Start() {
	log.Println("There is no configured event destinations")
}

func NewUploader(fileMask string, filesBatchSize, uploadEveryS int, tokenizedEventStorages map[string][]Storage) Uploader {
	if len(tokenizedEventStorages) == 0 {
		return &DummyUploader{}
	}

	return &PeriodicUploader{
		fileMask:               fileMask,
		filesBatchSize:         filesBatchSize,
		uploadEvery:            time.Duration(uploadEveryS) * time.Second,
		tokenizedEventStorages: tokenizedEventStorages,
	}
}

func (u *PeriodicUploader) Start() {
	go func() {
		for {
			if appstatus.Instance.Idle {
				break
			}
			files, err := filepath.Glob(u.fileMask)
			if err != nil {
				log.Println("Error finding files by mask", u.fileMask, err)
				return
			}

			sort.Strings(files)
			batchSize := len(files)
			if batchSize > u.filesBatchSize {
				batchSize = u.filesBatchSize
			}
			for _, filePath := range files[:batchSize] {
				fileName := filepath.Base(filePath)

				b, err := ioutil.ReadFile(filePath)
				if err != nil {
					log.Println("Error reading file", filePath, err)
					continue
				}
				if len(b) == 0 {
					os.Remove(filePath)
					continue
				}
				//get token from filename
				regexResult := tokenExtractRegexp.FindStringSubmatch(fileName)
				if len(regexResult) != 2 {
					log.Printf("Error processing file %s. Malformed name", filePath)
					continue
				}

				token := regexResult[1]
				eventStorages, ok := u.tokenizedEventStorages[token]
				if !ok {
					log.Printf("Destination storages weren't found for token %s", token)
					continue
				}

				//TODO all storages must be in one transaction 1 or no one
				for _, storage := range eventStorages {
					if err = storage.Store(fileName, b); err != nil {
						log.Println("Error store file", filePath, "in", storage.Name(), "destination:", err)
						break
					}
				}

				if err != nil {
					continue
				}

				os.Remove(filePath)
			}

			time.Sleep(u.uploadEvery)
		}
	}()
}