package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/sheophe/signal-cli-rest-api/utils"
	log "github.com/sirupsen/logrus"
)

func isSignalCliLinkedNumberConfigFile(filename string) (bool, error) {
	fileExtension := filepath.Ext(filename)
	if fileExtension != "" {
		return false, nil
	}

	mimetype, err := mimetype.DetectFile(filename)
	if err != nil {
		return false, err
	}
	if mimetype.String() == "application/json" {
		return true, nil
	}
	return false, nil
}

func getUsernameFromLinkedNumberConfigFile(filename string) (string, error) {
	type LinkedNumberConfigFile struct {
		Username string `json:"username"`
	}
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	var linkedNumberConfigFile LinkedNumberConfigFile
	err = json.Unmarshal(bytes, &linkedNumberConfigFile)
	if err != nil {
		return "", err
	}
	return linkedNumberConfigFile.Username, nil
}

func main() {
	signalCliConfigDir := utils.SignalCliConfigDir()
	signalCliConfigDataDir := signalCliConfigDir + "data"

	err := os.MkdirAll(signalCliConfigDataDir, 0644)
	if err != nil {
		log.Fatal(err)
	}

	jsonRpc2ClientConfig := utils.NewJsonRpc2ClientConfig()

	var ctr int64 = 0

	tcpPort, fifoPathname, err := utils.SaveSupervisorConf(&ctr, utils.LinkNumber, signalCliConfigDataDir)
	if err != nil {
		log.Fatal(err)
	}

	jsonRpc2ClientConfig.AddEntry(utils.LinkNumber, utils.JsonRpc2ClientConfigEntry{TcpPort: tcpPort, FifoPathname: fifoPathname})

	items, err := os.ReadDir(signalCliConfigDataDir)
	if err == nil {
		for _, item := range items {
			if item.IsDir() {
				continue
			}
			filename := filepath.Base(item.Name())
			isSignalCliLinkedNumberConfigFile, err := isSignalCliLinkedNumberConfigFile(signalCliConfigDataDir + "/" + filename)
			if err != nil {
				log.Error("Couldn't determine whether file ", filename, " is a signal-cli config file: ", err.Error())
				continue
			}

			if strings.HasPrefix(filename, "+") || isSignalCliLinkedNumberConfigFile {
				var number string = ""
				if utils.IsPhoneNumber(filename) {
					number = filename
				} else if isSignalCliLinkedNumberConfigFile {
					number, err = getUsernameFromLinkedNumberConfigFile(signalCliConfigDataDir + "/" + filename)
					if err != nil {
						log.Debug("Skipping ", filename, " as it is not a valid signal-cli config file: ", err.Error())
						continue
					}
				} else {
					log.Error("Skipping ", filename, " as it is not a valid phone number!")
					continue
				}

				tcpPort, fifoPathname, err = utils.SaveSupervisorConf(&ctr, number, signalCliConfigDataDir)
				if err != nil {
					log.Fatal(err)
				}

				jsonRpc2ClientConfig.AddEntry(number, utils.JsonRpc2ClientConfigEntry{TcpPort: tcpPort, FifoPathname: fifoPathname})
			}
		}
	}

	// write jsonrpc.yml config file
	err = jsonRpc2ClientConfig.Persist(signalCliConfigDir + "jsonrpc2.yml")
	if err != nil {
		log.Fatal("Couldn't persist jsonrpc2.yaml: ", err.Error())
	}
}
