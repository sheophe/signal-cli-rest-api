package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	lockedfile "github.com/sheophe/signal-cli-rest-api/utils/internal/lockedfile"
	log "github.com/sirupsen/logrus"
)

const supervisorctlConfigTemplate = `
[program:%s]
environment=JAVA_HOME=/opt/java/openjdk
process_name=%s
command=bash -c "nc -l -p %d <%s | signal-cli -vvv --output=json %sjsonRpc %s>%s"
autostart=false
autorestart=true
startretries=10
user=root
directory=/usr/bin/
redirect_stderr=true
stdout_logfile=/var/log/%s/out.log
stderr_logfile=/var/log/%s/err.log
stdout_logfile_maxbytes=50MB
stdout_logfile_backups=10
numprocs=1
`

const (
	linkClientConfigDir              = "/root/.local/share/signal-cli/"
	supervisorConfDir                = "/etc/supervisor/conf.d/"
	supervisorConfigFileNameTemplate = "signal-cli-json-rpc-%d.conf"
	fifoBasePathName                 = "/tmp/sigsocket"
	ctrLockedFileName                = "/tmp/signal-cli-ctr.lock"
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

	supervisorctlProgramName := "signal-cli-json-rpc-" + strconv.FormatInt(*ctr, 10)
	supervisorctlLogFolder := "/var/log/" + supervisorctlProgramName
	_, err = exec.Command("mkdir", "-p", supervisorctlLogFolder).Output()
	if err != nil {
		err = fmt.Errorf("Couldn't create log folder %s: %s", supervisorctlLogFolder, err.Error())
		return
	}

	log.Info("Adding number ", number)

	configDir := filepath.Join(signalCliConfigDir, strconv.FormatInt(*ctr, 10))
	provisioningParams := ""
	jsonRpcParams := ""
	if number != LinkNumber {
		provisioningParams = fmt.Sprintf("-u %s --config %s ", number, configDir)
	} else {
		jsonRpcParams = "--receive-mode=manual "
	}

	//write supervisorctl config
	supervisorctlConfigFilename := supervisorConfDir + fmt.Sprintf(supervisorConfigFileNameTemplate, *ctr)
	supervisorctlConfig := fmt.Sprintf(
		supervisorctlConfigTemplate,
		supervisorctlProgramName,
		supervisorctlProgramName,
		tcpPort,
		fifoPathName,
		provisioningParams,
		jsonRpcParams,
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

func RereadSupervisorConf() error {
	if err := exec.Command("supervisorctl", "reread").Run(); err != nil {
		return fmt.Errorf("couldn't update supervisor config: %s", err.Error())
	}
	return nil
}

func StartServiceByPort(tcpPort int64) error {
	id := tcpPort - LinkTcpPort
	if id < 0 {
		return fmt.Errorf("invalid port %d for service", tcpPort)
	}
	supervisorctlProgramName := "signal-cli-json-rpc-" + strconv.FormatInt(id, 10)
	output, err := exec.Command("supervisorctl", "start", supervisorctlProgramName).Output()
	if err != nil {
		return fmt.Errorf("couldn't start service: %s (%s)", err.Error(), string(output))
	}
	return nil
}

func StopServiceByPort(tcpPort int64) error {
	id := tcpPort - LinkTcpPort
	if id < 0 {
		return fmt.Errorf("invalid port %d for service", tcpPort)
	}
	supervisorctlProgramName := "signal-cli-json-rpc-" + strconv.FormatInt(id, 10)
	if err := exec.Command("supervisorctl", "stop", supervisorctlProgramName).Run(); err != nil {
		return fmt.Errorf("couldn't stop service: %s", err.Error())
	}
	return nil
}

func InitCtr(current int64) (err error) {
	file, err := lockedfile.Create(ctrLockedFileName)
	if err != nil {
		return
	}

	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	_, err = file.Write([]byte(strconv.FormatInt(current, 10)))
	return
}

func NextCtr() (next int64, err error) {
	err = lockedfile.Transform(ctrLockedFileName, func(lockedData []byte) ([]byte, error) {
		current, _ := strconv.ParseInt(string(lockedData), 10, 64)
		next = current + 1
		return []byte(strconv.FormatInt(next, 10)), nil
	})
	return
}
