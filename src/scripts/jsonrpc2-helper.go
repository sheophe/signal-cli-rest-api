package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/sheophe/signal-cli-rest-api/utils"
	log "github.com/sirupsen/logrus"
)

const supervisorctlConfigTemplate = `
[program:%s]
environment=JAVA_HOME=/opt/java/openjdk
process_name=%s
command=bash -c "nc -l -p %d <%s | signal-cli -vvv --output=json %s jsonRpc >%s"
autostart=true
autorestart=true
startretries=10
user=signal-api
directory=/usr/bin/
redirect_stderr=true
stdout_logfile=/var/log/%s/out.log
stderr_logfile=/var/log/%s/err.log
stdout_logfile_maxbytes=50MB
stdout_logfile_backups=10
numprocs=1
`

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
	signalCliConfigDir := "/home/.local/share/signal-cli/"
	signalCliConfigDirEnv := utils.GetEnv("SIGNAL_CLI_CONFIG_DIR", "")
	if signalCliConfigDirEnv != "" {
		signalCliConfigDir = signalCliConfigDirEnv
		if !strings.HasSuffix(signalCliConfigDirEnv, "/") {
			signalCliConfigDir += "/"
		}
	}

	signalCliConfigDataDir := signalCliConfigDir + "data"

	jsonRpc2ClientConfig := utils.NewJsonRpc2ClientConfig()

	tcpBasePort := utils.LinkTcpPort
	fifoBasePathName := "/tmp/sigsocket"
	var ctr int64 = 0

	fifoPathname := fifoBasePathName + strconv.FormatInt(ctr, 10)
	jsonRpc2ClientConfig.AddEntry(utils.LinkNumber, utils.JsonRpc2ClientConfigEntry{TcpPort: utils.LinkTcpPort, FifoPathname: fifoPathname})
	saveSupervisonConf(&ctr, utils.LinkTcpPort, fifoPathname, utils.LinkNumber, signalCliConfigDataDir)

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

				fifoPathname := fifoBasePathName + strconv.FormatInt(ctr, 10)
				tcpPort := tcpBasePort + ctr
				jsonRpc2ClientConfig.AddEntry(number, utils.JsonRpc2ClientConfigEntry{TcpPort: tcpPort, FifoPathname: fifoPathname})

				saveSupervisonConf(&ctr, tcpPort, fifoPathname, number, signalCliConfigDataDir)
			}
		}
	}

	// write jsonrpc.yml config file
	err = jsonRpc2ClientConfig.Persist(signalCliConfigDir + "jsonrpc2.yml")
	if err != nil {
		log.Fatal("Couldn't persist jsonrpc2.yaml: ", err.Error())
	}
}

func saveSupervisonConf(ctr *int64, tcpPort int64, fifoPathname, number, signalCliConfigDir string) {
	os.Remove(fifoPathname) //remove any existing named pipe

	_, err := exec.Command("mkfifo", fifoPathname).Output()
	if err != nil {
		log.Fatal("Couldn't create fifo with name ", fifoPathname, ": ", err.Error())
	}

	uid := utils.GetEnv("SIGNAL_CLI_UID", "1000")
	gid := utils.GetEnv("SIGNAL_CLI_GID", "1000")
	_, err = exec.Command("chown", uid+":"+gid, fifoPathname).Output()
	if err != nil {
		log.Fatal("Couldn't change permissions of fifo with name ", fifoPathname, ": ", err.Error())
	}

	supervisorctlProgramName := "signal-cli-json-rpc-" + strconv.FormatInt(*ctr, 10)
	supervisorctlLogFolder := "/var/log/" + supervisorctlProgramName
	_, err = exec.Command("mkdir", "-p", supervisorctlLogFolder).Output()
	if err != nil {
		log.Fatal("Couldn't create log folder ", supervisorctlLogFolder, ": ", err.Error())
	}

	log.Info("Found number ", number, " and added it to jsonrpc2.yml")

	accountParams := ""
	if number != utils.LinkNumber {
		accountParams = fmt.Sprintf("-u %s --config %s", number, signalCliConfigDir)
	}

	//write supervisorctl config
	supervisorctlConfigFilename := "/etc/supervisor/conf.d/" + "signal-cli-json-rpc-" + strconv.FormatInt(*ctr, 10) + ".conf"
	supervisorctlConfig := fmt.Sprintf(supervisorctlConfigTemplate, supervisorctlProgramName, supervisorctlProgramName,
		tcpPort, fifoPathname, accountParams, fifoPathname, supervisorctlProgramName, supervisorctlProgramName)
	err = os.WriteFile(supervisorctlConfigFilename, []byte(supervisorctlConfig), 0644)
	if err != nil {
		log.Fatal("Couldn't write ", supervisorctlConfigFilename, ": ", err.Error())
	}

	*ctr += 1
}
