package apidAnalytics

import (
	_ "fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

var retriesMap map[string]int

//TODO:  make sure that this instance gets initialized only once since we dont want multiple upload manager tickers running
func initUploadManager() {

	retriesMap = make(map[string]int)

	go func() {
		ticker := time.NewTicker(time.Second * config.GetDuration(analyticsUploadInterval))
		log.Debugf("Intialized upload manager to check for staging directory")
		defer ticker.Stop() // Ticker will keep running till go routine is running i.e. till application is running

		for range ticker.C {
			files, err := ioutil.ReadDir(localAnalyticsStagingDir)

			if err != nil {
				log.Errorf("Cannot read directory %s: ", localAnalyticsStagingDir)
			}

			uploadedDirCnt := 0
			for _, file := range files {
				if file.IsDir() {
					status := uploadDir(file)
					handleUploadDirStatus(file, status)
					if status {
						uploadedDirCnt++
					}
				}
			}
			if uploadedDirCnt > 0 {
				// After a successful upload, retry the folders in failed directory as they might have
				// failed due to intermitent S3/GCS issue
				retryFailedUploads()
			}
		}
	}()
}

func handleUploadDirStatus(dir os.FileInfo, status bool) {
	completePath := filepath.Join(localAnalyticsStagingDir, dir.Name())
	if status {
		os.RemoveAll(completePath)
		log.Debugf("deleted directory after successful upload : %s", dir.Name())
		// remove key if exists from retry map after a successful upload
		delete(retriesMap, dir.Name())
	} else {
		retriesMap[dir.Name()] = retriesMap[dir.Name()] + 1
		if retriesMap[dir.Name()] >= maxRetries {
			log.Errorf("Max Retires exceeded for folder: %s", completePath)
			failedDirPath := filepath.Join(localAnalyticsFailedDir, dir.Name())
			err := os.Rename(completePath, failedDirPath)
			if err != nil {
				log.Errorf("Cannot move directory :%s to failed folder", dir.Name())
			}
			// remove key from retry map once it reaches allowed max failed attempts
			delete(retriesMap, dir.Name())
		}
	}
}

func retryFailedUploads() {
	failedDirs, err := ioutil.ReadDir(localAnalyticsFailedDir)

	if err != nil {
		log.Errorf("Cannot read directory %s: ", localAnalyticsFailedDir)
	}

	cnt := 0
	for _, dir := range failedDirs {
		// We rety failed folder in batches to not overload the upload thread
		if cnt < retryFailedDirBatchSize {
			failedPath := filepath.Join(localAnalyticsFailedDir, dir.Name())
			newStagingPath := filepath.Join(localAnalyticsStagingDir, dir.Name())
			err := os.Rename(failedPath, newStagingPath)
			if err != nil {
				log.Errorf("Cannot move directory :%s to staging folder", dir.Name())
			}
		} else {
			break
		}
	}
}