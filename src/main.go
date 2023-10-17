package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"github.com/sheophe/signal-cli-rest-api/api"
	"github.com/sheophe/signal-cli-rest-api/client"
	docs "github.com/sheophe/signal-cli-rest-api/docs"
	"github.com/sheophe/signal-cli-rest-api/utils"
	log "github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title Signal Cli REST API
// @version 1.0
// @description This is the Signal Cli REST API documentation.

// @tag.name General
// @tag.description Some general endpoints.

// @tag.name Devices
// @tag.description Register and link Devices.

// @tag.name Groups
// @tag.description Create, List and Delete Signal Groups.

// @tag.name Messages
// @tag.description Send and Receive Signal Messages.

// @tag.name Attachments
// @tag.description List and Delete Attachments.

// @tag.name Profiles
// @tag.description Update Profile.

// @tag.name Identities
// @tag.description List and Trust Identities.

// @tag.name Reactions
// @tag.description React to messages.

// @tag.name Search
// @tag.description Search the Signal Service.

// @BasePath /
func main() {
	signalCliConfig := flag.String("signal-cli-config", "/home/.local/share/signal-cli/", "Config directory where signal-cli config is stored")
	attachmentTmpDir := flag.String("attachment-tmp-dir", "/tmp/", "Attachment tmp directory")
	avatarTmpDir := flag.String("avatar-tmp-dir", "/tmp/", "Avatar tmp directory")
	flag.Parse()

	router := gin.New()
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/v1/health"}, //do not log the health requests (to avoid spamming the log file)
	}))

	router.Use(gin.Recovery(), cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET"},
		AllowHeaders:     []string{"Origin"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	port := utils.GetEnv("HTTP_PORT", "8080")
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatal("Invalid HTTP_PORT ", port, " set. HTTP_PORT needs to be a number")
	}

	defaultSwaggerIp := utils.GetEnv("HOST_IP", "127.0.0.1")
	swaggerIp := utils.GetEnv("SWAGGER_IP", defaultSwaggerIp)

	httpsPort := ""
	netProtocol := client.Http
	protocol := utils.GetEnv("PROTOCOL", "http")

	switch protocol {
	case "http":
		netProtocol = client.Http
		docs.SwaggerInfo.Host = swaggerIp + ":" + port

	case "https":
		netProtocol = client.Https
		httpsPort = utils.GetEnv("HTTPS_PORT", "443")
		if _, err := strconv.Atoi(port); err != nil {
			log.Fatal("Invalid HTTPS_PORT ", port, " set. HTTPS_PORT needs to be a number")
		}
		docs.SwaggerInfo.Host = swaggerIp + ":" + httpsPort

	default:
		log.Fatal("Unsupported network protocol: ", protocol)
	}

	docs.SwaggerInfo.Schemes = []string{protocol}

	log.Info("Started Signal Messenger REST API")

	supportsSignalCliNative := "0"
	if _, err := os.Stat("/usr/bin/signal-cli-native"); err == nil {
		supportsSignalCliNative = "1"
	}

	err := os.Setenv("SUPPORTS_NATIVE", supportsSignalCliNative)
	if err != nil {
		log.Fatal("Couldn't set env variable: ", err.Error())
	}

	useNative := utils.GetEnv("USE_NATIVE", "")
	if useNative != "" {
		log.Warning("The env variable USE_NATIVE is deprecated. Please use the env variable MODE instead")
	}

	signalCliMode := client.Normal
	mode := utils.GetEnv("MODE", "normal")
	if mode == "normal" {
		signalCliMode = client.Normal
	} else if mode == "json-rpc" {
		signalCliMode = client.JsonRpc
	} else if mode == "native" {
		signalCliMode = client.Native
	}

	if useNative != "" {
		_, modeEnvVariableSet := os.LookupEnv("MODE")
		if modeEnvVariableSet {
			log.Fatal("You have both the USE_NATIVE and the MODE env variable set. Please remove the deprecated env variable USE_NATIVE!")
		}
	}

	if useNative == "1" || signalCliMode == client.Native {
		if supportsSignalCliNative == "0" {
			log.Error("signal-cli-native is not support on this system...falling back to signal-cli")
			signalCliMode = client.Normal
		}
	}

	if signalCliMode == client.JsonRpc {
		_, autoReceiveScheduleEnvVariableSet := os.LookupEnv("AUTO_RECEIVE_SCHEDULE")
		if autoReceiveScheduleEnvVariableSet {
			log.Fatal("Env variable AUTO_RECEIVE_SCHEDULE can't be used with mode json-rpc")
		}

		_, signalCliCommandTimeoutEnvVariableSet := os.LookupEnv("SIGNAL_CLI_CMD_TIMEOUT")
		if signalCliCommandTimeoutEnvVariableSet {
			log.Fatal("Env variable SIGNAL_CLI_CMD_TIMEOUT can't be used with mode json-rpc")
		}
	}

	jsonRpc2ClientConfigPathPath := *signalCliConfig + "/jsonrpc2.yml"
	signalCliApiConfigPath := *signalCliConfig + "/api-config.yml"
	signalClient := client.NewSignalClient(*signalCliConfig, *attachmentTmpDir, *avatarTmpDir, signalCliMode, jsonRpc2ClientConfigPathPath, signalCliApiConfigPath)
	err = signalClient.Init()
	if err != nil {
		log.Fatal("Couldn't init Signal Client: ", err.Error())
	}

	api := api.NewApi(signalClient, signalCliMode)
	v1 := router.Group("/v1")
	{
		about := v1.Group("/about")
		{
			about.GET("", api.About)
		}

		configuration := v1.Group("/configuration")
		{
			configuration.GET("", api.GetConfiguration)
			configuration.POST("", api.SetConfiguration)
			configuration.POST(":number/settings", api.SetTrustMode)
			configuration.GET(":number/settings", api.GetTrustMode)
		}

		health := v1.Group("/health")
		{
			health.GET("", api.Health)
		}

		register := v1.Group("/register")
		{
			register.POST(":number", api.RegisterNumber)
			register.POST(":number/verify/:token", api.VerifyRegisteredNumber)
		}

		unregister := v1.Group("unregister")
		{
			unregister.POST(":number", api.UnregisterNumber)
		}

		sendV1 := v1.Group("/send")
		{
			sendV1.POST("", api.Send)
		}

		receive := v1.Group("/receive")
		{
			receive.GET(":number", api.Receive)
		}

		groups := v1.Group("/groups")
		{
			groups.POST(":number", api.CreateGroup)
			groups.GET(":number", api.GetGroups)
			groups.GET(":number/:groupid", api.GetGroup)
			groups.DELETE(":number/:groupid", api.DeleteGroup)
			groups.POST(":number/:groupid/block", api.BlockGroup)
			groups.POST(":number/:groupid/join", api.JoinGroup)
			groups.POST(":number/:groupid/quit", api.QuitGroup)
			groups.PUT(":number/:groupid", api.UpdateGroup)
			groups.POST(":number/:groupid/members", api.AddMembersToGroup)
			groups.DELETE(":number/:groupid/members", api.RemoveMembersFromGroup)
			groups.POST(":number/:groupid/admins", api.AddAdminsToGroup)
			groups.DELETE(":number/:groupid/admins", api.RemoveAdminsFromGroup)
		}

		link := v1.Group("link")
		{
			link.GET("", api.GetDeviceLinkUri)
			link.GET("qrcode", api.GetLinkQrCode)
			link.GET("await", api.GetDeviceLinkAwait)
		}

		devices := v1.Group("devices")
		{
			devices.POST(":number", api.AddDevice)
		}

		attachments := v1.Group("attachments")
		{
			attachments.GET("", api.GetAttachments)
			attachments.DELETE(":attachment", api.RemoveAttachment)
			attachments.GET(":attachment", api.ServeAttachment)
		}

		profiles := v1.Group("profiles")
		{
			profiles.PUT(":number", api.UpdateProfile)
		}

		identities := v1.Group("identities")
		{
			identities.GET(":number", api.ListIdentities)
			identities.PUT(":number/trust/:numbertotrust", api.TrustIdentity)
		}

		typingIndicator := v1.Group("typing-indicator")
		{
			typingIndicator.PUT(":number", api.SendStartTyping)
			typingIndicator.DELETE(":number", api.SendStopTyping)
		}

		reactions := v1.Group("/reactions")
		{
			reactions.POST(":number", api.SendReaction)
			reactions.DELETE(":number", api.RemoveReaction)
		}

		search := v1.Group("/search")
		{
			search.GET("", api.SearchForNumbers)
			search.GET(":number", api.SearchForNumbers)
		}

		contacts := v1.Group("/contacts")
		{
			contacts.PUT(":number", api.UpdateContact)
			contacts.POST(":number/sync", api.SendContacts)
		}
	}

	v2 := router.Group("/v2")
	{
		sendV2 := v2.Group("/send")
		{
			sendV2.POST("", api.SendV2)
		}
	}

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	autoReceiveSchedule := utils.GetEnv("AUTO_RECEIVE_SCHEDULE", "")
	if autoReceiveSchedule != "" {
		p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := p.Parse(autoReceiveSchedule)
		if err != nil {
			log.Fatal("AUTO_RECEIVE_SCHEDULE: Invalid schedule: ", err.Error())
		}

		type SignalCliAccountConfig struct {
			Number string `json:"number"`
		}

		type SignalCliAccountConfigs struct {
			Accounts []SignalCliAccountConfig `json:"accounts"`
		}

		autoReceiveScheduleReceiveTimeout := utils.GetEnv("AUTO_RECEIVE_SCHEDULE_RECEIVE_TIMEOUT", "10")
		autoReceiveScheduleIgnoreAttachments := utils.GetEnv("AUTO_RECEIVE_SCHEDULE_IGNORE_ATTACHMENTS", "false")
		autoReceiveScheduleIgnoreStories := utils.GetEnv("AUTO_RECEIVE_SCHEDULE_IGNORE_STORIES", "false")

		c := cron.New()
		c.Schedule(schedule, cron.FuncJob(func() {
			accountsJsonPath := *signalCliConfig + "/data/accounts.json"
			if _, err := os.Stat(accountsJsonPath); err == nil {
				signalCliConfigJsonData, err := ioutil.ReadFile(accountsJsonPath)
				if err != nil {
					log.Fatal("AUTO_RECEIVE_SCHEDULE: Couldn't read accounts.json: ", err.Error())
				}
				var signalCliAccountConfigs SignalCliAccountConfigs
				err = json.Unmarshal(signalCliConfigJsonData, &signalCliAccountConfigs)
				if err != nil {
					log.Fatal("AUTO_RECEIVE_SCHEDULE: Couldn't parse accounts.json: ", err.Error())
				}

				for _, account := range signalCliAccountConfigs.Accounts {
					client := &http.Client{}

					log.Debug("AUTO_RECEIVE_SCHEDULE: Calling receive for number ", account.Number)
					req, err := http.NewRequest("GET", "http://127.0.0.1:"+port+"/v1/receive/"+account.Number, nil)
					if err != nil {
						log.Error("AUTO_RECEIVE_SCHEDULE: Couldn't call receive for number ", account.Number, ": ", err.Error())
					}

					q := req.URL.Query()
					q.Add("timeout", autoReceiveScheduleReceiveTimeout)
					q.Add("ignore_attachments", autoReceiveScheduleIgnoreAttachments)
					q.Add("ignore_stories", autoReceiveScheduleIgnoreStories)
					req.URL.RawQuery = q.Encode()

					resp, err := client.Do(req)
					if err != nil {
						log.Error("AUTO_RECEIVE_SCHEDULE: Couldn't call receive for number ", account.Number, ": ", err.Error())
					}

					if resp.StatusCode != 200 {
						jsonResp, err := ioutil.ReadAll(resp.Body)
						resp.Body.Close()
						if err != nil {
							log.Error("AUTO_RECEIVE_SCHEDULE: Couldn't read json response: ", err.Error())
							continue
						}

						type ReceiveResponse struct {
							Error string `json:"error"`
						}
						var receiveResponse ReceiveResponse
						err = json.Unmarshal(jsonResp, &receiveResponse)
						if err != nil {
							log.Error("AUTO_RECEIVE_SCHEDULE: Couldn't parse json response: ", err.Error())
							continue
						}

						log.Error("AUTO_RECEIVE_SCHEDULE: Couldn't call receive for number ", account.Number, ": ", receiveResponse)
					}
				}
			} else {
				log.Info("AUTO_RECEIVE_SCHEDULE: accounts.json doesn't exist")
			}
		}))
		c.Start()
	}

	if netProtocol == client.Http {
		router.Run()
	} else {
		cert := utils.GetEnv("CERT_FILE", "")
		key_file := utils.GetEnv("KEY_FILE", "")
		if cert == "" || key_file == "" {
			log.Fatal("CERT_FILE and KEY_FILE must be set")
		}
		router.RunTLS(":"+string(httpsPort), cert, key_file)
	}
}
