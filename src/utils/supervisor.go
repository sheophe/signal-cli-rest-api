package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

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

const (
	supervisorConfDir                = "/etc/supervisor/conf.d/"
	supervisorConfigFileNameTemplate = "signal-cli-json-rpc-%d.conf"
	fifoBasePathName                 = "/tmp/sigsocket"
)

func SignalCliConfigDir() string {
	signalCliConfigDir := "/home/.local/share/signal-cli/"
	signalCliConfigDirEnv := GetEnv("SIGNAL_CLI_CONFIG_DIR", "")
	if signalCliConfigDirEnv != "" {
		signalCliConfigDir = signalCliConfigDirEnv
		if !strings.HasSuffix(signalCliConfigDirEnv, "/") {
			signalCliConfigDir += "/"
		}
	}

	return signalCliConfigDir
}

func SaveSupervisorConf(ctr *int64, number, signalCliConfigDir string) (tcpPort int64, fifoPathName string, err error) {
	fifoPathName = fifoBasePathName + strconv.FormatInt(*ctr, 10)
	tcpPort = LinkTcpPort + *ctr

	os.Remove(fifoPathName) //remove any existing named pipe

	_, err = exec.Command("mkfifo", fifoPathName).Output()
	if err != nil {
		err = fmt.Errorf("Couldn't create fifo with name %s: %s ", fifoPathName, err.Error())
		return
	}

	uid := GetEnv("SIGNAL_CLI_UID", "1000")
	gid := GetEnv("SIGNAL_CLI_GID", "1000")
	_, err = exec.Command("chown", uid+":"+gid, fifoPathName).Output()
	if err != nil {
		err = fmt.Errorf("Couldn't change permissions of fifo with name %s: %s", fifoPathName, err.Error())
		return
	}

	supervisorctlProgramName := "signal-cli-json-rpc-" + strconv.FormatInt(*ctr, 10)
	supervisorctlLogFolder := "/var/log/" + supervisorctlProgramName
	_, err = exec.Command("mkdir", "-p", supervisorctlLogFolder).Output()
	if err != nil {
		err = fmt.Errorf("Couldn't create log folder %s: %s", supervisorctlLogFolder, err.Error())
		return
	}

	log.Info("Found number ", number, " and added it to jsonrpc2.yml")

	accountParams := ""
	if number != LinkNumber {
		accountParams = fmt.Sprintf("-u %s --config %s", number, signalCliConfigDir)
	}

	//write supervisorctl config
	supervisorctlConfigFilename := supervisorConfDir + fmt.Sprintf(supervisorConfigFileNameTemplate, *ctr)
	supervisorctlConfig := fmt.Sprintf(
		supervisorctlConfigTemplate,
		supervisorctlProgramName,
		supervisorctlProgramName,
		tcpPort,
		fifoPathName,
		accountParams,
		fifoPathName,
		supervisorctlProgramName,
		supervisorctlProgramName,
	)
	err = os.WriteFile(supervisorctlConfigFilename, []byte(supervisorctlConfig), 0644)
	if err != nil {
		err = fmt.Errorf("Couldn't write %s: %s", supervisorctlConfigFilename, err.Error())
		return
	}

	*ctr += 1
	return
}

func NextCtr(signalCliConfigDir string) (current int64) {
	dir, err := os.ReadDir(signalCliConfigDir)
	if err != nil {
		return 0
	}

	for _, item := range dir {
		if item.IsDir() {
			continue
		}

		var ctr int64
		n, _ := fmt.Sscanf(item.Name(), supervisorConfigFileNameTemplate, &ctr)

		if n == 1 && ctr > current {
			current = ctr
		}
	}

	current += 1
	return
}

func ChownDirs() (err error) {
	_, err = exec.Command("chown", "signal-api", "/var/log").Output()
	if err != nil {
		err = fmt.Errorf("Couldn't chown log folder: %s", err.Error())
		return
	}

	_, err = exec.Command("chown", "signal-api", supervisorConfDir).Output()
	if err != nil {
		err = fmt.Errorf("Couldn't chown supervison confing folder: %s", err.Error())
		return
	}

	return
}

func UpdateSupervisor() error {
	if err := exec.Command("supervisorctl", "update").Run(); err != nil {
		return fmt.Errorf("Couldn't update and restart supervisor: %s", err.Error())
	}
	return nil
}
