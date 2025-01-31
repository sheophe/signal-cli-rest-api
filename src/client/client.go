package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/h2non/filetype"

	uuid "github.com/gofrs/uuid"
	qrcode "github.com/skip2/go-qrcode"

	utils "github.com/sheophe/signal-cli-rest-api/utils"
)

const groupPrefix = "group."

const signalCliV2GroupError = "Cannot create a V2 group as self does not have a versioned profile"

const endpointNotSupportedInJsonRpcMode = "This endpoint is not supported in JSON-RPC mode."

const endpointOnlySupportedInJsonRpcMode = "This endpoint is only supported in JSON-RPC mode."

const defaultDeviceName = "Dockerized Signal API"

type NetProtocol int

const (
	Http NetProtocol = iota + 1
	Https
)

type GroupPermission int

const (
	DefaultGroupPermission GroupPermission = iota + 1
	EveryMember
	OnlyAdmins
)

type SignalCliMode int

const (
	Normal SignalCliMode = iota + 1
	Native
	JsonRpc
)

type GroupLinkState int

const (
	DefaultGroupLinkState GroupLinkState = iota + 1
	Enabled
	EnabledWithApproval
	Disabled
)

func (g GroupPermission) String() string {
	switch g {
	case DefaultGroupPermission:
		return ""
	case EveryMember:
		return "every-member"
	case OnlyAdmins:
		return "only-admins"
	}
	return ""
}

func (g GroupPermission) FromString(input string) GroupPermission {
	if input == "every-member" {
		return EveryMember
	}
	if input == "only-admins" {
		return OnlyAdmins
	}
	return DefaultGroupPermission
}

func (g GroupLinkState) String() string {
	switch g {
	case DefaultGroupLinkState:
		return ""
	case Enabled:
		return "enabled"
	case EnabledWithApproval:
		return "enabled-with-approval"
	case Disabled:
		return "disabled"
	}
	return ""
}

func (g GroupLinkState) FromString(input string) GroupLinkState {
	if input == "enabled" {
		return Enabled
	}
	if input == "enabled-with-approval" {
		return EnabledWithApproval
	}
	if input == "disabled" {
		return Disabled
	}

	return DefaultGroupLinkState
}

type MessageMention struct {
	Start  int64  `json:"start"`
	Length int64  `json:"length"`
	Author string `json:"author"`
}

type ContactProfile struct {
	LastUpdateTimestamp int64  `json:"lastUpdateTimestamp,omitempty"`
	GivenName           string `json:"givenName,omitempty"`
	FamilyName          string `json:"familyName,omitempty"`
	About               string `json:"about,omitempty"`
	AboutEmoji          string `json:"aboutEmoji,omitempty"`
	MobileCoinAddress   string `json:"mobileCoinAddress,omitempty"`
}

type ContactEntry struct {
	Number                string          `json:"number,omitempty"`
	UUID                  string          `json:"uuid,omitempty"`
	Username              string          `json:"username,omitempty"`
	Name                  string          `json:"name,omitempty"`
	Color                 string          `json:"color,omitempty"`
	IsBlocked             bool            `json:"isBlocked"`
	MessageExpirationTime int64           `json:"messageExpirationTime,omitempty"`
	Profile               *ContactProfile `json:"profile,omitempty"`
}

type GroupEntry struct {
	Name            string   `json:"name"`
	Id              string   `json:"id"`
	InternalId      string   `json:"internal_id"`
	Members         []string `json:"members"`
	Blocked         bool     `json:"blocked"`
	PendingInvites  []string `json:"pending_invites"`
	PendingRequests []string `json:"pending_requests"`
	InviteLink      string   `json:"invite_link"`
	Admins          []string `json:"admins"`
}

type IdentityEntry struct {
	Number       string `json:"number"`
	Status       string `json:"status"`
	Fingerprint  string `json:"fingerprint"`
	Added        string `json:"added"`
	SafetyNumber string `json:"safety_number"`
}

type SignalCliGroupMember struct {
	Number string `json:"number"`
	Uuid   string `json:"uuid"`
}

type SignalCliGroupAdmin struct {
	Number string `json:"number"`
	Uuid   string `json:"uuid"`
}

type SignalCliGroupEntry struct {
	Name              string                 `json:"name"`
	Id                string                 `json:"id"`
	IsMember          bool                   `json:"isMember"`
	IsBlocked         bool                   `json:"isBlocked"`
	Members           []SignalCliGroupMember `json:"members"`
	PendingMembers    []SignalCliGroupMember `json:"pendingMembers"`
	RequestingMembers []SignalCliGroupMember `json:"requestingMembers"`
	GroupInviteLink   string                 `json:"groupInviteLink"`
	Admins            []SignalCliGroupAdmin  `json:"admins"`
}

type SignalCliIdentityEntry struct {
	Number                string `json:"number"`
	Uuid                  string `json:"uuid"`
	Fingerprint           string `json:"fingerprint"`
	SafetyNumber          string `json:"safetyNumber"`
	ScannableSafetyNumber string `json:"scannableSafetyNumber"`
	TrustLevel            string `json:"trustLevel"`
	AddedTimestamp        int64  `json:"addedTimestamp"`
}

type SignalLinkUrl struct {
	DeviceLinkUri string `json:"deviceLinkUri"`
}

type SignalLinkNumber struct {
	Number string `json:"number"`
}

type SendAddress struct {
	UUID     string `json:"uuid,omitempty"`
	Number   string `json:"number,omitempty"`
	Username string `json:"username,omitempty"`
}

type SendResults struct {
	RecepientAddress SendAddress `json:"recipientAddress"`
	Type             string      `json:"type"`
}

type SendResponse struct {
	Timestamp int64         `json:"timestamp"`
	Results   []SendResults `json:"results"`
}

type About struct {
	SupportedApiVersions []string            `json:"versions"`
	BuildNr              int                 `json:"build"`
	Mode                 string              `json:"mode"`
	Version              string              `json:"version"`
	Capabilities         map[string][]string `json:"capabilities"`
}

type SearchResultEntry struct {
	Number     string `json:"number"`
	Registered bool   `json:"registered"`
}

type AssignedNumbers struct {
	Numbers []string `json:"numbers"`
}

func cleanupTmpFiles(paths []string) {
	for _, path := range paths {
		os.Remove(path)
	}
}

func cleanupAttachmentEntries(attachmentEntries []AttachmentEntry) {
	for _, attachmentEntry := range attachmentEntries {
		attachmentEntry.cleanUp()
	}
}

func convertInternalGroupIdToGroupId(internalId string) string {
	return groupPrefix + base64.StdEncoding.EncodeToString([]byte(internalId))
}

func getStringInBetween(str string, start string, end string) (result string) {
	i := strings.Index(str, start)
	if i == -1 {
		return
	}
	i += len(start)
	j := strings.Index(str[i:], end)
	if j == -1 {
		return
	}
	return str[i : i+j]
}

func parseWhitespaceDelimitedKeyValueStringList(in string, keys []string) []map[string]string {
	l := []map[string]string{}
	lines := strings.Split(in, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		m := make(map[string]string)

		temp := line
		for i, key := range keys {
			if i == 0 {
				continue
			}

			idx := strings.Index(temp, " "+key+": ")
			pair := temp[:idx]
			value := strings.TrimPrefix(pair, key+": ")
			temp = strings.TrimLeft(temp[idx:], " "+key+": ")

			m[keys[i-1]] = value
		}
		m[keys[len(keys)-1]] = temp

		l = append(l, m)
	}
	return l
}

func getContainerId() (string, error) {
	data, err := ioutil.ReadFile("/proc/1/cpuset")
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return "", errors.New("Couldn't get docker container id (empty)")
	}
	containerId := strings.Replace(lines[0], "/docker/", "", -1)
	return containerId, nil
}

func ConvertGroupIdToInternalGroupId(id string) (string, error) {

	groupIdWithoutPrefix := strings.TrimPrefix(id, groupPrefix)
	internalGroupId, err := base64.StdEncoding.DecodeString(groupIdWithoutPrefix)
	if err != nil {
		return "", errors.New("Invalid group id")
	}

	return string(internalGroupId), err
}

func getSignalCliModeString(signalCliMode SignalCliMode) string {
	if signalCliMode == Normal {
		return "normal"
	} else if signalCliMode == Native {
		return "native"
	} else if signalCliMode == JsonRpc {
		return "json-rpc"
	}
	return "unknown"
}

type SignalClient struct {
	signalCliConfig          string
	attachmentTmpDir         string
	avatarTmpDir             string
	signalCliMode            SignalCliMode
	jsonRpc2ClientConfig     *utils.JsonRpc2ClientConfig
	jsonRpc2ClientConfigPath string
	jsonRpc2Clients          map[string]*JsonRpc2Client
	signalCliApiConfigPath   string
	signalCliApiConfig       *utils.SignalCliApiConfig
	cliClient                *CliClient
	subStorage               *utils.SubStorage
}

func NewSignalClient(signalCliConfig string, attachmentTmpDir string, avatarTmpDir string, signalCliMode SignalCliMode,
	jsonRpc2ClientConfigPath string, signalCliApiConfigPath string, subStorage *utils.SubStorage) *SignalClient {
	return &SignalClient{
		signalCliConfig:          signalCliConfig,
		attachmentTmpDir:         attachmentTmpDir,
		avatarTmpDir:             avatarTmpDir,
		signalCliMode:            signalCliMode,
		jsonRpc2ClientConfigPath: jsonRpc2ClientConfigPath,
		jsonRpc2Clients:          make(map[string]*JsonRpc2Client),
		signalCliApiConfigPath:   signalCliApiConfigPath,
		subStorage:               subStorage,
	}
}

func (s *SignalClient) GetSignalCliMode() SignalCliMode {
	return s.signalCliMode
}

func (s *SignalClient) Init() error {
	s.signalCliApiConfig = utils.NewSignalCliApiConfig()
	err := s.signalCliApiConfig.Load(s.signalCliApiConfigPath)
	if err != nil {
		return err
	}

	if s.signalCliMode == JsonRpc {
		s.jsonRpc2ClientConfig = utils.NewJsonRpc2ClientConfig()
		err := s.jsonRpc2ClientConfig.Load(s.jsonRpc2ClientConfigPath)
		if err != nil {
			return err
		}

		s.jsonRpc2Clients = make(map[string]*JsonRpc2Client)
		s.jsonRpc2Clients[utils.LinkNumber] = NewJsonRpc2Client(s.signalCliApiConfig, utils.LinkNumber, utils.LinkTcpPort, "")
		s.jsonRpc2Clients[utils.LinkNumber].Start()

		tcpPortsNumberMapping := s.jsonRpc2ClientConfig.GetTcpPortsForNumbers()
		for number, tcpPort := range tcpPortsNumberMapping {
			if number == utils.LinkNumber {
				continue
			}
			if sub, ok := s.subStorage.GetSubByNumber(number); ok {
				s.jsonRpc2Clients[number] = NewJsonRpc2Client(s.signalCliApiConfig, number, tcpPort, sub)
			}
		}
	} else {
		s.cliClient = NewCliClient(s.signalCliMode, s.signalCliApiConfig)
	}

	return nil
}

func (s *MessageMention) toString() string {
	return fmt.Sprintf("%d:%d:%s", s.Start, s.Length, s.Author)
}

func (s *SignalClient) send(number string, message string,
	recipients []string, base64Attachments []string, isGroup bool, sticker string, mentions []MessageMention,
	quoteTimestamp *int64, quoteAuthor *string, quoteMessage *string, quoteMentions []MessageMention, textMode *string) (*SendResponse, error) {

	var resp SendResponse

	if len(recipients) == 0 {
		return nil, errors.New("Please specify at least one recipient")
	}

	signalCliTextFormatStrings := []string{}
	if textMode != nil && *textMode == "styled" {
		message, signalCliTextFormatStrings = utils.ParseMarkdownMessage(message)
	}

	var groupId string = ""
	if isGroup {
		if len(recipients) > 1 {
			return nil, errors.New("More than one recipient is currently not allowed")
		}

		grpId, err := base64.StdEncoding.DecodeString(recipients[0])
		if err != nil {
			return nil, errors.New("Invalid group id")
		}
		groupId = string(grpId)
	}

	attachmentEntries := []AttachmentEntry{}
	for _, base64Attachment := range base64Attachments {
		attachmentEntry := NewAttachmentEntry(base64Attachment, s.attachmentTmpDir)

		err := attachmentEntry.storeBase64AsTemporaryFile()
		if err != nil {
			cleanupAttachmentEntries(attachmentEntries)
			return nil, err
		}

		attachmentEntries = append(attachmentEntries, *attachmentEntry)
	}

	if s.signalCliMode == JsonRpc {
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return nil, err
		}

		type Request struct {
			Recipients     []string `json:"recipient,omitempty"`
			Message        string   `json:"message"`
			GroupId        string   `json:"group-id,omitempty"`
			Attachments    []string `json:"attachment,omitempty"`
			Sticker        string   `json:"sticker,omitempty"`
			Mentions       []string `json:"mentions,omitempty"`
			QuoteTimestamp *int64   `json:"quote-timestamp,omitempty"`
			QuoteAuthor    *string  `json:"quote-author,omitempty"`
			QuoteMessage   *string  `json:"quote-message,omitempty"`
			QuoteMentions  []string `json:"quote-mentions,omitempty"`
			TextStyles     []string `json:"text-style,omitempty"`
		}

		request := Request{Message: message}
		if isGroup {
			request.GroupId = groupId
		} else {
			request.Recipients = recipients
		}
		for _, attachmentEntry := range attachmentEntries {
			request.Attachments = append(request.Attachments, attachmentEntry.toDataForSignal())
		}

		request.Sticker = sticker
		if mentions != nil {
			request.Mentions = make([]string, len(mentions))
			for i, mention := range mentions {
				request.Mentions[i] = mention.toString()
			}
		} else {
			request.Mentions = nil
		}
		request.QuoteTimestamp = quoteTimestamp
		request.QuoteAuthor = quoteAuthor
		request.QuoteMessage = quoteMessage
		if quoteMentions != nil {
			request.QuoteMentions = make([]string, len(quoteMentions))
			for i, mention := range quoteMentions {
				request.QuoteMentions[i] = mention.toString()
			}
		} else {
			request.QuoteMentions = nil
		}

		if len(signalCliTextFormatStrings) > 0 {
			request.TextStyles = signalCliTextFormatStrings
		}

		rawData, err := jsonRpc2Client.getRaw("send", request, nil)
		if err != nil {
			cleanupAttachmentEntries(attachmentEntries)
			return nil, err
		}

		log.Info(rawData)

		err = json.Unmarshal([]byte(rawData), &resp)
		if err != nil {
			if strings.Contains(err.Error(), signalCliV2GroupError) {
				return nil, errors.New("Cannot send message to group - please first update your profile.")
			}
			return nil, err
		}
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "send", "--message-from-stdin"}
		if !isGroup {
			cmd = append(cmd, recipients...)
		} else {
			cmd = append(cmd, []string{"-g", groupId}...)
		}

		if len(signalCliTextFormatStrings) > 0 {
			cmd = append(cmd, "--text-style")
			cmd = append(cmd, signalCliTextFormatStrings...)
		}

		if len(attachmentEntries) > 0 {
			cmd = append(cmd, "-a")
			for _, attachmentEntry := range attachmentEntries {
				cmd = append(cmd, attachmentEntry.toDataForSignal())
			}
		}

		for _, mention := range mentions {
			cmd = append(cmd, "--mention")
			cmd = append(cmd, mention.toString())
		}

		if sticker != "" {
			cmd = append(cmd, "--sticker")
			cmd = append(cmd, sticker)
		}

		if quoteTimestamp != nil {
			cmd = append(cmd, "--quote-timestamp")
			cmd = append(cmd, strconv.FormatInt(*quoteTimestamp, 10))
		}

		if quoteAuthor != nil {
			cmd = append(cmd, "--quote-author")
			cmd = append(cmd, *quoteAuthor)
		}

		if quoteMessage != nil {
			cmd = append(cmd, "--quote-message")
			cmd = append(cmd, *quoteMessage)
		}

		for _, mention := range quoteMentions {
			cmd = append(cmd, "--quote-mention")
			cmd = append(cmd, mention.toString())
		}

		rawData, err := s.cliClient.Execute(true, cmd, message)
		if err != nil {
			cleanupAttachmentEntries(attachmentEntries)
			if strings.Contains(err.Error(), signalCliV2GroupError) {
				return nil, errors.New("Cannot send message to group - please first update your profile.")
			}
			return nil, err
		}
		resp.Timestamp, err = strconv.ParseInt(strings.TrimSuffix(rawData, "\n"), 10, 64)
		if err != nil {
			cleanupAttachmentEntries(attachmentEntries)
			return nil, errors.New(strings.Replace(rawData, "\n", "", -1)) //in case we can't parse the timestamp, it means signal-cli threw an error. So instead of returning the parsing error, return the actual error from signal-cli
		}
	}

	cleanupAttachmentEntries(attachmentEntries)

	return &resp, nil
}

func (s *SignalClient) About() About {
	about := About{
		SupportedApiVersions: []string{"v1", "v2"},
		BuildNr:              2,
		Mode:                 getSignalCliModeString(s.signalCliMode),
		Version:              utils.GetEnv("BUILD_VERSION", "unset"),
		Capabilities:         map[string][]string{"v2/send": {"quotes", "mentions"}},
	}
	return about
}

func (s *SignalClient) RegisterNumber(number string, useVoice bool, captcha string) error {
	if s.signalCliMode == JsonRpc {
		return errors.New(endpointNotSupportedInJsonRpcMode)
	}
	command := []string{"--config", s.signalCliConfig, "-a", number, "register"}

	if useVoice {
		command = append(command, "--voice")
	}

	if captcha != "" {
		command = append(command, []string{"--captcha", captcha}...)
	}

	_, err := s.cliClient.Execute(true, command, "")
	return err
}

func (s *SignalClient) UnregisterNumber(number string, deleteAccount bool, deleteLocalData bool) error {
	if s.signalCliMode == JsonRpc {
		return errors.New("This functionality is only available in normal/native mode!")
	}

	command := []string{"--config", s.signalCliConfig, "-a", number, "unregister"}
	if deleteAccount {
		command = append(command, "--delete-account")
	}

	_, err := s.cliClient.Execute(true, command, "")

	if deleteLocalData {
		command := []string{"--config", s.signalCliConfig, "-a", number, "deleteLocalAccountData"}
		_, err2 := s.cliClient.Execute(true, command, "")
		if (err2 != nil) && (err != nil) {
			err = fmt.Errorf("%w (%s)", err, err2.Error())
		} else if (err2 != nil) && (err == nil) {
			err = err2
		}
	}

	return err
}

func (s *SignalClient) VerifyRegisteredNumber(number string, token string, pin string) error {
	if s.signalCliMode == JsonRpc {
		return errors.New(endpointNotSupportedInJsonRpcMode)
	}

	cmd := []string{"--config", s.signalCliConfig, "-a", number, "verify", token}
	if pin != "" {
		cmd = append(cmd, "--pin")
		cmd = append(cmd, pin)
	}

	_, err := s.cliClient.Execute(true, cmd, "")
	return err
}

func (s *SignalClient) SendV1(number string, message string, recipients []string, base64Attachments []string, isGroup bool) (*SendResponse, error) {
	timestamp, err := s.send(number, message, recipients, base64Attachments, isGroup, "", nil, nil, nil, nil, nil, nil)
	return timestamp, err
}

func (s *SignalClient) getJsonRpc2Client(number string) (*JsonRpc2Client, error) {
	if number == utils.LinkNumber {
		return nil, errors.New("Number not registered with JSON-RPC")
	}
	if val, ok := s.jsonRpc2Clients[number]; ok {
		return val, nil
	}
	return nil, errors.New("Number not registered with JSON-RPC")
}

func (s *SignalClient) getJsonRpc2Clients() []*JsonRpc2Client {
	jsonRpc2Clients := []*JsonRpc2Client{}
	for _, client := range s.jsonRpc2Clients {
		jsonRpc2Clients = append(jsonRpc2Clients, client)
	}
	return jsonRpc2Clients
}

func (s *SignalClient) SendV2(number string, message string, recps []string, base64Attachments []string, sticker string, mentions []MessageMention,
	quoteTimestamp *int64, quoteAuthor *string, quoteMessage *string, quoteMentions []MessageMention, textMode *string) (*[]SendResponse, error) {
	if len(recps) == 0 {
		return nil, errors.New("Please provide at least one recipient")
	}

	if number == "" {
		return nil, errors.New("Please provide a valid number")
	}

	groups := []string{}
	recipients := []string{}

	for _, recipient := range recps {
		if strings.HasPrefix(recipient, groupPrefix) {
			groups = append(groups, strings.TrimPrefix(recipient, groupPrefix))
		} else {
			recipients = append(recipients, recipient)
		}
	}

	if len(recipients) > 0 && len(groups) > 0 {
		return nil, errors.New("Signal Messenger Groups and phone numbers cannot be specified together in one request! Please split them up into multiple REST API calls.")
	}

	if len(groups) > 1 {
		return nil, errors.New("A signal message cannot be sent to more than one group at once! Please use multiple REST API calls for that.")
	}

	timestamps := []SendResponse{}
	for _, group := range groups {
		timestamp, err := s.send(number, message, []string{group}, base64Attachments, true, sticker, mentions, quoteTimestamp, quoteAuthor, quoteMessage, quoteMentions, textMode)
		if err != nil {
			return nil, err
		}
		timestamps = append(timestamps, *timestamp)
	}

	if len(recipients) > 0 {
		timestamp, err := s.send(number, message, recipients, base64Attachments, false, sticker, mentions, quoteTimestamp, quoteAuthor, quoteMessage, quoteMentions, textMode)
		if err != nil {
			return nil, err
		}
		timestamps = append(timestamps, *timestamp)
	}

	return &timestamps, nil
}

func (s *SignalClient) Receive(number string, timeout int64, ignoreAttachments bool, ignoreStories bool, maxMessages int64) (string, error) {
	if s.signalCliMode == JsonRpc {
		return "", errors.New("Not implemented")
	} else {
		command := []string{"--config", s.signalCliConfig, "--output", "json", "-a", number, "receive", "-t", strconv.FormatInt(timeout, 10)}

		if ignoreAttachments {
			command = append(command, "--ignore-attachments")
		}

		if ignoreStories {
			command = append(command, "--ignore-stories")
		}

		if maxMessages > 0 {
			command = append(command, "--max-messages")
			command = append(command, strconv.FormatInt(maxMessages, 10))
		}

		out, err := s.cliClient.Execute(true, command, "")
		if err != nil {
			return "", err
		}

		out = strings.Trim(out, "\n")
		lines := strings.Split(out, "\n")

		jsonStr := "["
		for i, line := range lines {
			jsonStr += line
			if i != (len(lines) - 1) {
				jsonStr += ","
			}
		}
		jsonStr += "]"

		return jsonStr, nil
	}
}

func (s *SignalClient) GetReceiveChannel(number string) (chan JsonRpc2ReceivedMessage, error) {
	jsonRpc2Client, err := s.getJsonRpc2Client(number)
	if err != nil {
		return nil, err
	}
	return jsonRpc2Client.GetReceiveChannel(), nil
}

func (s *SignalClient) CreateGroup(number string, name string, members []string, description string, editGroupPermission GroupPermission, addMembersPermission GroupPermission, groupLinkState GroupLinkState) (string, error) {
	var internalGroupId string
	if s.signalCliMode == JsonRpc {
		type Request struct {
			Name                  string   `json:"name"`
			Members               []string `json:"members"`
			Link                  string   `json:"link,omitempty"`
			Description           string   `json:"description,omitempty"`
			EditGroupPermissions  string   `json:"setPermissionEditDetails,omitempty"`
			AddMembersPermissions string   `json:"setPermissionAddMember,omitempty"`
		}
		request := Request{Name: name, Members: members}

		if groupLinkState != DefaultGroupLinkState {
			request.Link = groupLinkState.String()
		}

		if description != "" {
			request.Description = description
		}

		if editGroupPermission != DefaultGroupPermission {
			request.EditGroupPermissions = editGroupPermission.String()
		}

		if addMembersPermission != DefaultGroupPermission {
			request.AddMembersPermissions = addMembersPermission.String()
		}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return "", err
		}
		rawData, err := jsonRpc2Client.getRaw("updateGroup", request, nil)
		if err != nil {
			return "", err
		}

		type Response struct {
			GroupId   string `json:"groupId"`
			Timestamp int64  `json:"timestamp"`
		}
		var resp Response
		json.Unmarshal([]byte(rawData), &resp)
		if err != nil {
			return "", err
		}
		internalGroupId = resp.GroupId
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "updateGroup", "-n", name, "-m"}
		cmd = append(cmd, members...)

		if addMembersPermission != DefaultGroupPermission {
			cmd = append(cmd, []string{"--set-permission-add-member", addMembersPermission.String()}...)
		}

		if editGroupPermission != DefaultGroupPermission {
			cmd = append(cmd, []string{"--set-permission-edit-details", editGroupPermission.String()}...)
		}

		if groupLinkState != DefaultGroupLinkState {
			cmd = append(cmd, []string{"--link", groupLinkState.String()}...)
		}

		if description != "" {
			cmd = append(cmd, []string{"--description", description}...)
		}

		rawData, err := s.cliClient.Execute(true, cmd, "")
		if err != nil {
			if strings.Contains(err.Error(), signalCliV2GroupError) {
				return "", errors.New("Cannot create group - please first update your profile.")
			}
			return "", err
		}
		internalGroupId = getStringInBetween(rawData, `"`, `"`)
	}
	groupId := convertInternalGroupIdToGroupId(internalGroupId)

	return groupId, nil
}

func (s *SignalClient) updateGroupMembers(number string, groupId string, members []string, add bool) error {
	var err error

	if len(members) == 0 {
		return nil
	}

	group, err := s.GetGroup(number, groupId)
	if err != nil {
		return err
	}

	if group == nil {
		return &NotFoundError{Description: "No group with that group id (" + groupId + ") found"}
	}

	internalGroupId, err := ConvertGroupIdToInternalGroupId(groupId)
	if err != nil {
		return errors.New("Invalid group id")
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			Name          string   `json:"name,omitempty"`
			Members       []string `json:"member,omitempty"`
			RemoveMembers []string `json:"remove-member,omitempty"`
			GroupId       string   `json:"groupId"`
		}
		request := Request{GroupId: internalGroupId}
		if add {
			request.Members = append(request.Members, members...)
		} else {
			request.RemoveMembers = append(request.RemoveMembers, members...)
		}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("updateGroup", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "updateGroup", "-g", internalGroupId}

		if add {
			cmd = append(cmd, "-m")
		} else {
			cmd = append(cmd, "-r")
		}
		cmd = append(cmd, members...)

		_, err = s.cliClient.Execute(true, cmd, "")
	}
	return err
}

func (s *SignalClient) AddMembersToGroup(number string, groupId string, members []string) error {
	return s.updateGroupMembers(number, groupId, members, true)
}

func (s *SignalClient) RemoveMembersFromGroup(number string, groupId string, members []string) error {
	return s.updateGroupMembers(number, groupId, members, false)
}

func (s *SignalClient) updateGroupAdmins(number string, groupId string, admins []string, add bool) error {
	var err error

	if len(admins) == 0 {
		return nil
	}

	group, err := s.GetGroup(number, groupId)
	if err != nil {
		return err
	}

	if group == nil {
		return &NotFoundError{Description: "No group with that group id (" + groupId + ") found"}
	}

	internalGroupId, err := ConvertGroupIdToInternalGroupId(groupId)
	if err != nil {
		return errors.New("Invalid group id")
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			Name         string   `json:"name,omitempty"`
			Admins       []string `json:"admin,omitempty"`
			RemoveAdmins []string `json:"remove-admin,omitempty"`
			GroupId      string   `json:"groupId"`
		}
		request := Request{GroupId: internalGroupId}
		if add {
			request.Admins = append(request.Admins, admins...)
		} else {
			request.RemoveAdmins = append(request.RemoveAdmins, admins...)
		}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("updateGroup", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "updateGroup", "-g", internalGroupId}

		if add {
			cmd = append(cmd, "--admin")
		} else {
			cmd = append(cmd, "--remove-admin")
		}
		cmd = append(cmd, admins...)

		_, err = s.cliClient.Execute(true, cmd, "")
	}
	return err
}

func (s *SignalClient) AddAdminsToGroup(number string, groupId string, admins []string) error {
	return s.updateGroupAdmins(number, groupId, admins, true)
}

func (s *SignalClient) RemoveAdminsFromGroup(number string, groupId string, admins []string) error {
	return s.updateGroupAdmins(number, groupId, admins, false)
}

func (s *SignalClient) GetGroups(number string) ([]GroupEntry, error) {
	groupEntries := []GroupEntry{}

	var signalCliGroupEntries []SignalCliGroupEntry
	var err error
	var rawData string

	if s.signalCliMode == JsonRpc {
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return groupEntries, err
		}
		rawData, err = jsonRpc2Client.getRaw("listGroups", nil, nil)
		if err != nil {
			return groupEntries, err
		}
	} else {
		rawData, err = s.cliClient.Execute(true, []string{"--config", s.signalCliConfig, "--output", "json", "-a", number, "listGroups", "-d"}, "")
		if err != nil {
			return groupEntries, err
		}
	}

	err = json.Unmarshal([]byte(rawData), &signalCliGroupEntries)
	if err != nil {
		return groupEntries, err
	}

	for _, signalCliGroupEntry := range signalCliGroupEntries {
		var groupEntry GroupEntry
		groupEntry.InternalId = signalCliGroupEntry.Id
		groupEntry.Name = signalCliGroupEntry.Name
		groupEntry.Id = convertInternalGroupIdToGroupId(signalCliGroupEntry.Id)
		groupEntry.Blocked = signalCliGroupEntry.IsBlocked

		members := []string{}
		for _, val := range signalCliGroupEntry.Members {
			members = append(members, val.Number)
		}
		groupEntry.Members = members

		pendingMembers := []string{}
		for _, val := range signalCliGroupEntry.PendingMembers {
			pendingMembers = append(pendingMembers, val.Number)
		}
		groupEntry.PendingRequests = pendingMembers

		requestingMembers := []string{}
		for _, val := range signalCliGroupEntry.RequestingMembers {
			requestingMembers = append(requestingMembers, val.Number)
		}
		groupEntry.PendingInvites = requestingMembers

		admins := []string{}
		for _, val := range signalCliGroupEntry.Admins {
			admins = append(admins, val.Number)
		}
		groupEntry.Admins = admins

		groupEntry.InviteLink = signalCliGroupEntry.GroupInviteLink

		groupEntries = append(groupEntries, groupEntry)
	}

	return groupEntries, nil
}

func (s *SignalClient) GetGroup(number string, groupId string) (*GroupEntry, error) {
	groupEntry := GroupEntry{}
	groups, err := s.GetGroups(number)
	if err != nil {
		return nil, err
	}

	for _, group := range groups {
		if group.Id == groupId {
			groupEntry = group
			return &groupEntry, nil
		}
	}

	return nil, nil
}

func (s *SignalClient) DeleteGroup(number string, groupId string) error {
	if s.signalCliMode == JsonRpc {
		type Request struct {
			GroupId string `json:"groupId"`
		}
		request := Request{GroupId: groupId}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("quitGroup", request, nil)
		return err
	} else {
		ret, err := s.cliClient.Execute(true, []string{"--config", s.signalCliConfig, "-a", number, "quitGroup", "-g", string(groupId)}, "")
		if strings.Contains(ret, "User is not a group member") {
			return errors.New("Can't delete group: User is not a group member")
		}
		return err
	}
}

func (s *SignalClient) GetDeviceLink(deviceName string) (SignalLinkUrl, error) {
	if s.signalCliMode == JsonRpc {
		jsonRpc2Client, ok := s.jsonRpc2Clients[utils.LinkNumber]
		if !ok {
			return SignalLinkUrl{}, errors.New("No system number registered")
		}

		deviceLinkUri, err := jsonRpc2Client.getRaw("startLink", nil, nil)
		if err != nil {
			return SignalLinkUrl{}, err
		}

		signalLinkUri := SignalLinkUrl{}
		err = json.Unmarshal([]byte(deviceLinkUri), &signalLinkUri)
		if err != nil {
			return SignalLinkUrl{}, err
		}

		log.Info("Start link response: ", signalLinkUri)

		return signalLinkUri, nil
	}

	command := []string{"--config", s.signalCliConfig, "link", "-n", deviceName}
	tsdeviceLink, err := s.cliClient.Execute(false, command, "")
	if err != nil {
		return SignalLinkUrl{}, errors.New("Couldn't create QR code: " + err.Error())
	}

	signalLinkUri := SignalLinkUrl{}
	err = json.Unmarshal([]byte(tsdeviceLink), &signalLinkUri)
	if err != nil {
		return SignalLinkUrl{}, err
	}

	return signalLinkUri, nil
}

func (s *SignalClient) GetDeviceLinkAwait(deviceLinkUri, sub string, ctx context.Context) (SignalLinkNumber, error) {
	if s.signalCliMode != JsonRpc {
		return SignalLinkNumber{}, errors.New(endpointOnlySupportedInJsonRpcMode)
	}

	jsonRpc2Client, ok := s.jsonRpc2Clients[utils.LinkNumber]
	if !ok {
		return SignalLinkNumber{}, errors.New("No system number registered")
	}

	deviceName := utils.GetEnv("DEVICE_NAME", defaultDeviceName)

	ctr, err := utils.NextCtr()
	if err != nil {
		return SignalLinkNumber{}, err
	}

	configDir := filepath.Join(s.signalCliConfig, strconv.FormatInt(ctr, 10))

	type Request struct {
		DeviceLinkUri string `json:"deviceLinkUri"`
		DeviceName    string `json:"deviceName"`
		ConfigDir     string `json:"configDir"`
	}
	request := Request{
		DeviceLinkUri: deviceLinkUri,
		DeviceName:    deviceName,
		ConfigDir:     configDir,
	}

	log.Info("Finish link request: ", request)

	rawData, err := jsonRpc2Client.getRaw("finishLink", request, ctx)
	if err != nil {
		return SignalLinkNumber{}, err
	}

	response := SignalLinkNumber{}
	err = json.Unmarshal([]byte(rawData), &response)
	if err != nil {
		return SignalLinkNumber{}, err
	}

	number := response.Number

	err = s.subStorage.LinkSub(sub, number, ctr)
	if err != nil {
		os.RemoveAll(configDir)
		return SignalLinkNumber{}, err
	}

	tcpPort, _, err := utils.SaveSupervisorConf(&ctr, number, s.signalCliConfig)
	if err != nil {
		return SignalLinkNumber{}, err
	}

	err = utils.RereadSupervisorConf()
	if err != nil {
		return SignalLinkNumber{}, err
	}

	s.jsonRpc2Clients[number] = NewJsonRpc2Client(s.signalCliApiConfig, number, tcpPort, sub)

	return response, err
}

func (s *SignalClient) GetLinkQrCode(linkUri string, qrCodeVersion int) ([]byte, error) {
	q, err := qrcode.NewWithForcedVersion(string(linkUri), qrCodeVersion, qrcode.Highest)
	if err != nil {
		return []byte{}, errors.New("Couldn't create QR code: " + err.Error())
	}

	q.DisableBorder = false
	var png []byte
	png, err = q.PNG(256)
	if err != nil {
		return []byte{}, errors.New("Couldn't create QR code: " + err.Error())
	}
	return png, nil
}

func (s *SignalClient) GetAttachments() ([]string, error) {
	files := []string{}

	err := filepath.Walk(s.signalCliConfig+"/attachments/", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		files = append(files, filepath.Base(path))
		return nil
	})

	return files, err
}

func (s *SignalClient) RemoveAttachment(attachment string) error {
	path, err := securejoin.SecureJoin(s.signalCliConfig+"/attachments/", attachment)
	if err != nil {
		return &InvalidNameError{Description: "Please provide a valid attachment name"}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &NotFoundError{Description: "No attachment with that name found"}
	}
	err = os.Remove(path)
	if err != nil {
		return &InternalError{Description: "Couldn't delete attachment - please try again later"}
	}

	return nil
}

func (s *SignalClient) GetAttachment(attachment string) ([]byte, error) {
	path, err := securejoin.SecureJoin(s.signalCliConfig+"/attachments/", attachment)
	if err != nil {
		return []byte{}, &InvalidNameError{Description: "Please provide a valid attachment name"}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []byte{}, &NotFoundError{Description: "No attachment with that name found"}
	}

	attachmentBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return []byte{}, &InternalError{Description: "Couldn't read attachment - please try again later"}
	}

	return attachmentBytes, nil
}

func (s *SignalClient) UpdateProfile(number string, profileName string, base64Avatar string) error {
	var err error
	var avatarTmpPath string
	if base64Avatar != "" {
		u, err := uuid.NewV4()
		if err != nil {
			return err
		}

		avatarBytes, err := base64.StdEncoding.DecodeString(base64Avatar)
		if err != nil {
			return errors.New("Couldn't decode base64 encoded avatar: " + err.Error())
		}

		fType, err := filetype.Get(avatarBytes)
		if err != nil {
			return err
		}

		avatarTmpPath = s.avatarTmpDir + u.String() + "." + fType.Extension

		f, err := os.Create(avatarTmpPath)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := f.Write(avatarBytes); err != nil {
			cleanupTmpFiles([]string{avatarTmpPath})
			return err
		}
		if err := f.Sync(); err != nil {
			cleanupTmpFiles([]string{avatarTmpPath})
			return err
		}
		f.Close()
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			Name         string `json:"given-name"`
			Avatar       string `json:"avatar,omitempty"`
			RemoveAvatar bool   `json:"remove-avatar"`
		}
		request := Request{Name: profileName}
		if base64Avatar == "" {
			request.RemoveAvatar = true
		} else {
			request.Avatar = avatarTmpPath
			request.RemoveAvatar = false
		}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("updateProfile", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "updateProfile", "--given-name", profileName}
		if base64Avatar == "" {
			cmd = append(cmd, "--remove-avatar")
		} else {
			cmd = append(cmd, []string{"--avatar", avatarTmpPath}...)
		}

		_, err = s.cliClient.Execute(true, cmd, "")
	}

	cleanupTmpFiles([]string{avatarTmpPath})
	return err
}

func (s *SignalClient) ListIdentities(number string) (*[]IdentityEntry, error) {
	identityEntries := []IdentityEntry{}
	if s.signalCliMode == JsonRpc {
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return nil, err
		}
		rawData, err := jsonRpc2Client.getRaw("listIdentities", nil, nil)
		signalCliIdentityEntries := []SignalCliIdentityEntry{}
		err = json.Unmarshal([]byte(rawData), &signalCliIdentityEntries)
		if err != nil {
			return nil, err
		}
		for _, signalCliIdentityEntry := range signalCliIdentityEntries {
			identityEntry := IdentityEntry{
				Number:       signalCliIdentityEntry.Number,
				Status:       signalCliIdentityEntry.TrustLevel,
				Added:        strconv.FormatInt(signalCliIdentityEntry.AddedTimestamp, 10),
				Fingerprint:  signalCliIdentityEntry.Fingerprint,
				SafetyNumber: signalCliIdentityEntry.SafetyNumber,
			}
			identityEntries = append(identityEntries, identityEntry)
		}
	} else {
		rawData, err := s.cliClient.Execute(true, []string{"--config", s.signalCliConfig, "-a", number, "listIdentities"}, "")
		if err != nil {
			return nil, err
		}

		keyValuePairs := parseWhitespaceDelimitedKeyValueStringList(rawData, []string{"NumberAndTrustStatus", "Added", "Fingerprint", "Safety Number"})
		for _, keyValuePair := range keyValuePairs {
			numberAndTrustStatus := keyValuePair["NumberAndTrustStatus"]
			numberAndTrustStatusSplitted := strings.Split(numberAndTrustStatus, ":")

			identityEntry := IdentityEntry{Number: strings.Trim(numberAndTrustStatusSplitted[0], " "),
				Status:       strings.Trim(numberAndTrustStatusSplitted[1], " "),
				Added:        keyValuePair["Added"],
				Fingerprint:  strings.Trim(keyValuePair["Fingerprint"], " "),
				SafetyNumber: strings.Trim(keyValuePair["Safety Number"], " "),
			}
			identityEntries = append(identityEntries, identityEntry)
		}
	}

	return &identityEntries, nil
}

func (s *SignalClient) TrustIdentity(number string, numberToTrust string, verifiedSafetyNumber *string, trustAllKnownKeys *bool) error {
	var err error
	if s.signalCliMode == JsonRpc {
		type Request struct {
			VerifiedSafetyNumber string `json:"verified-safety-number,omitempty"`
			TrustAllKnownKeys    bool   `json:"trust-all-known-keys,omitempty"`
			Recipient            string `json:"recipient"`
		}
		request := Request{Recipient: numberToTrust}

		if verifiedSafetyNumber != nil {
			request.VerifiedSafetyNumber = *verifiedSafetyNumber
		}

		if trustAllKnownKeys != nil {
			request.TrustAllKnownKeys = *trustAllKnownKeys
		}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("trust", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "trust", numberToTrust}

		if verifiedSafetyNumber != nil {
			cmd = append(cmd, []string{"--verified-safety-number", *verifiedSafetyNumber}...)
		}

		if trustAllKnownKeys != nil && *trustAllKnownKeys {
			cmd = append(cmd, "--trust-all-known-keys")
		}

		_, err = s.cliClient.Execute(true, cmd, "")
	}
	return err
}

func (s *SignalClient) BlockGroup(number string, groupId string) error {
	var err error
	if s.signalCliMode == JsonRpc {
		type Request struct {
			GroupId string `json:"groupId"`
		}
		request := Request{GroupId: groupId}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("block", request, nil)
	} else {
		_, err = s.cliClient.Execute(true, []string{"--config", s.signalCliConfig, "-a", number, "block", "-g", groupId}, "")
	}
	return err
}

func (s *SignalClient) JoinGroup(number string, groupId string) error {
	var err error
	if s.signalCliMode == JsonRpc {
		type Request struct {
			GroupId string `json:"groupId"`
		}
		request := Request{GroupId: groupId}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("updateGroup", request, nil)
	} else {
		_, err = s.cliClient.Execute(true, []string{"--config", s.signalCliConfig, "-a", number, "updateGroup", "-g", groupId}, "")
	}
	return err
}

func (s *SignalClient) QuitGroup(number string, groupId string) error {
	var err error
	if s.signalCliMode == JsonRpc {
		type Request struct {
			GroupId string `json:"groupId"`
		}
		request := Request{GroupId: groupId}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("quitGroup", request, nil)
	} else {
		_, err = s.cliClient.Execute(true, []string{"--config", s.signalCliConfig, "-a", number, "quitGroup", "-g", groupId}, "")
	}
	return err
}

func (s *SignalClient) UpdateGroup(number string, groupId string, base64Avatar *string, groupDescription *string) error {
	var err error
	var avatarTmpPath string = ""
	if base64Avatar != nil {
		u, err := uuid.NewV4()
		if err != nil {
			return err
		}

		avatarBytes, err := base64.StdEncoding.DecodeString(*base64Avatar)
		if err != nil {
			return errors.New("Couldn't decode base64 encoded avatar: " + err.Error())
		}

		fType, err := filetype.Get(avatarBytes)
		if err != nil {
			return err
		}

		avatarTmpPath = s.avatarTmpDir + u.String() + "." + fType.Extension

		f, err := os.Create(avatarTmpPath)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := f.Write(avatarBytes); err != nil {
			cleanupTmpFiles([]string{avatarTmpPath})
			return err
		}
		if err := f.Sync(); err != nil {
			cleanupTmpFiles([]string{avatarTmpPath})
			return err
		}
		f.Close()
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			GroupId     string  `json:"groupId"`
			Avatar      string  `json:"avatar,omitempty"`
			Description *string `json:"description,omitempty"`
		}
		request := Request{GroupId: groupId}

		if base64Avatar != nil {
			request.Avatar = avatarTmpPath
		}

		request.Description = groupDescription

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("updateGroup", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "updateGroup", "-g", groupId}
		if base64Avatar != nil {
			cmd = append(cmd, []string{"-a", avatarTmpPath}...)
		}

		if groupDescription != nil {
			cmd = append(cmd, []string{"-d", *groupDescription}...)
		}
		_, err = s.cliClient.Execute(true, cmd, "")
	}

	if avatarTmpPath != "" {
		cleanupTmpFiles([]string{avatarTmpPath})
	}

	return err
}

func (s *SignalClient) SendReaction(number string, recipient string, emoji string, target_author string, timestamp int64, remove bool) error {
	// see https://github.com/AsamK/signal-cli/blob/master/man/signal-cli.1.adoc#sendreaction
	var err error
	recp := recipient
	isGroup := false
	if strings.HasPrefix(recipient, groupPrefix) {
		isGroup = true
		recp, err = ConvertGroupIdToInternalGroupId(recipient)
		if err != nil {
			return errors.New("Invalid group id")
		}
	}
	if remove && emoji == "" {
		emoji = "👍" // emoji must not be empty to remove a reaction
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			Recipient    string `json:"recipient,omitempty"`
			GroupId      string `json:"group-id,omitempty"`
			Emoji        string `json:"emoji"`
			TargetAuthor string `json:"target-author"`
			Timestamp    int64  `json:"target-timestamp"`
			Remove       bool   `json:"remove,omitempty"`
		}
		request := Request{}
		if !isGroup {
			request.Recipient = recp
		} else {
			request.GroupId = recp
		}
		request.Emoji = emoji
		request.TargetAuthor = target_author
		request.Timestamp = timestamp
		if remove {
			request.Remove = remove
		}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("sendReaction", request, nil)
		return err
	}

	cmd := []string{
		"--config", s.signalCliConfig,
		"-a", number,
		"sendReaction",
	}
	if !isGroup {
		cmd = append(cmd, recp)
	} else {
		cmd = append(cmd, []string{"-g", recp}...)
	}
	cmd = append(cmd, []string{"-e", emoji, "-a", target_author, "-t", strconv.FormatInt(timestamp, 10)}...)
	if remove {
		cmd = append(cmd, "-r")
	}
	_, err = s.cliClient.Execute(true, cmd, "")
	return err
}

func (s *SignalClient) SendStartTyping(number string, recipient string) error {
	var err error
	recp := recipient
	isGroup := false
	if strings.HasPrefix(recipient, groupPrefix) {
		isGroup = true
		recp, err = ConvertGroupIdToInternalGroupId(recipient)
		if err != nil {
			return errors.New("Invalid group id")
		}
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			Recipient string `json:"recipient,omitempty"`
			GroupId   string `json:"group-id,omitempty"`
		}
		request := Request{}
		if !isGroup {
			request.Recipient = recp
		} else {
			request.GroupId = recp
		}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("sendTyping", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "sendTyping"}
		if !isGroup {
			cmd = append(cmd, recp)
		} else {
			cmd = append(cmd, []string{"-g", recp}...)
		}
		_, err = s.cliClient.Execute(true, cmd, "")
	}

	return err
}

func (s *SignalClient) SendStopTyping(number string, recipient string) error {
	var err error
	recp := recipient
	isGroup := false
	if strings.HasPrefix(recipient, groupPrefix) {
		isGroup = true
		recp, err = ConvertGroupIdToInternalGroupId(recipient)
		if err != nil {
			return errors.New("Invalid group id")
		}
	}

	if s.signalCliMode == JsonRpc {
		type Request struct {
			Recipient string `json:"recipient,omitempty"`
			GroupId   string `json:"group-id,omitempty"`
			Stop      bool   `json:"stop"`
		}
		request := Request{Stop: true}
		if !isGroup {
			request.Recipient = recp
		} else {
			request.GroupId = recp
		}

		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("sendTyping", request, nil)
	} else {
		cmd := []string{"--config", s.signalCliConfig, "-a", number, "sendTyping", "--stop"}
		if !isGroup {
			cmd = append(cmd, recp)
		} else {
			cmd = append(cmd, []string{"-g", recp}...)
		}
		_, err = s.cliClient.Execute(true, cmd, "")
	}

	return err
}

func (s *SignalClient) SearchForNumbers(number string, numbers []string) ([]SearchResultEntry, error) {
	searchResultEntries := []SearchResultEntry{}

	var err error
	var rawData string
	if s.signalCliMode == JsonRpc {
		type Request struct {
			Numbers []string `json:"recipient"`
		}
		request := Request{Numbers: numbers}

		jsonRpc2Clients := s.getJsonRpc2Clients()
		if len(jsonRpc2Clients) == 0 {
			return searchResultEntries, errors.New("No JsonRpc2Client registered!")
		}
		for _, jsonRpc2Client := range jsonRpc2Clients {
			rawData, err = jsonRpc2Client.getRaw("getUserStatus", request, nil)
			if err == nil { //getUserStatus doesn't need an account to work, so try all the registered acounts and stop until we succeed
				break
			}
		}

		if err != nil {
			return searchResultEntries, err
		}
	} else {
		cmd := []string{"--config", s.signalCliConfig, "--output", "json"}
		if number != "" {
			cmd = append(cmd, []string{"-a", number}...)
		}
		cmd = append(cmd, "getUserStatus")
		cmd = append(cmd, numbers...)
		rawData, err = s.cliClient.Execute(true, cmd, "")
	}

	if err != nil {
		return searchResultEntries, err
	}

	type SignalCliResponse struct {
		Number       string `json:"number"`
		IsRegistered bool   `json:"isRegistered"`
	}

	var resp []SignalCliResponse
	err = json.Unmarshal([]byte(rawData), &resp)
	if err != nil {
		return searchResultEntries, err
	}

	for _, val := range resp {
		searchResultEntry := SearchResultEntry{Number: val.Number, Registered: val.IsRegistered}
		searchResultEntries = append(searchResultEntries, searchResultEntry)
	}

	return searchResultEntries, err
}

func (s *SignalClient) SendContacts(number string) error {
	var err error
	if s.signalCliMode == JsonRpc {
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("sendContacts", nil, nil)
		return err
	}

	cmd := []string{"--config", s.signalCliConfig, "-a", number, "sendContacts"}
	_, err = s.cliClient.Execute(true, cmd, "")
	return err
}

func (s *SignalClient) GetContacts(number string) ([]ContactEntry, error) {
	if s.signalCliMode != JsonRpc {
		return []ContactEntry{}, errors.New(endpointOnlySupportedInJsonRpcMode)
	}

	type Request struct {
		AllRecepients bool `json:"allRecepients"`
	}
	request := Request{AllRecepients: true}

	jsonRpc2Client, err := s.getJsonRpc2Client(number)
	if err != nil {
		return []ContactEntry{}, err
	}
	rawData, err := jsonRpc2Client.getRaw("listContacts", request, nil)
	if err != nil {
		return []ContactEntry{}, err
	}

	contactEntries := []ContactEntry{}
	err = json.Unmarshal([]byte(rawData), &contactEntries)
	if err != nil {
		return []ContactEntry{}, err
	}

	return contactEntries, nil
}

func (s *SignalClient) UpdateContact(number string, recipient string, name *string, expirationInSeconds *int) error {
	var err error
	if s.signalCliMode == JsonRpc {
		type Request struct {
			Recipient  string `json:"recipient"`
			Name       string `json:"name,omitempty"`
			Expiration int    `json:"expiration,omitempty"`
		}
		request := Request{Recipient: recipient}
		if name != nil {
			request.Name = *name
		}
		if expirationInSeconds != nil {
			request.Expiration = *expirationInSeconds
		}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("updateContact", request, nil)
		return err
	}

	cmd := []string{"--config", s.signalCliConfig, "-a", number, "updateContact", recipient}
	if name != nil {
		cmd = append(cmd, []string{"-n", *name}...)
	}
	if expirationInSeconds != nil {
		cmd = append(cmd, []string{"-e", strconv.Itoa(*expirationInSeconds)}...)
	}
	_, err = s.cliClient.Execute(true, cmd, "")
	return err
}

func (s *SignalClient) AddDevice(number string, uri string) error {
	var err error
	if s.signalCliMode == JsonRpc {
		type Request struct {
			Uri string `json:"uri"`
		}
		request := Request{Uri: uri}
		jsonRpc2Client, err := s.getJsonRpc2Client(number)
		if err != nil {
			return err
		}
		_, err = jsonRpc2Client.getRaw("addDevice", request, nil)
		return err
	}

	cmd := []string{"--config", s.signalCliConfig, "-a", number, "addDevice", "--uri", uri}
	_, err = s.cliClient.Execute(true, cmd, "")
	return err
}

func (s *SignalClient) SetTrustMode(number string, trustMode utils.SignalCliTrustMode) error {
	s.signalCliApiConfig.SetTrustModeForNumber(number, trustMode)
	return s.signalCliApiConfig.Persist()
}

func (s *SignalClient) GetTrustMode(number string) utils.SignalCliTrustMode {
	trustMode, err := s.signalCliApiConfig.GetTrustModeForNumber(number)
	if err != nil { //no trust mode explicitly set, use signal-cli default
		return utils.OnFirstUseTrust
	}
	return trustMode
}

func (s *SignalClient) IsNumberLoggedIn(number string) (bool, error) {
	client, ok := s.jsonRpc2Clients[number]
	if !ok {
		return false, fmt.Errorf("unknown number %s", number)
	}

	if client.loggedIn {
		return true, fmt.Errorf("number %s is already logged in", number)
	}

	return false, nil
}

func (s *SignalClient) CheckAccess(sub, number string) error {
	client, ok := s.jsonRpc2Clients[number]
	if !ok {
		return fmt.Errorf("unknown number %s", number)
	}

	if !client.loggedIn {
		return fmt.Errorf("number %s is not logged in", number)
	}

	if client.sub != sub {
		return fmt.Errorf("number %s does not belong to this user", number)
	}

	return nil
}

func (s *SignalClient) Login(sub, number string) error {
	_, err := s.IsNumberLoggedIn(number)
	if err != nil {
		return err
	}

	err = s.subStorage.CheckIfSubIsValid(sub, number)
	if err != nil {
		return err
	}

	err = s.jsonRpc2Clients[number].Start()
	if err != nil {
		return err
	}

	return nil
}

func (s *SignalClient) Logout(sub, number string) error {
	loggedIn, _ := s.IsNumberLoggedIn(number)
	if !loggedIn {
		return fmt.Errorf("number %s is not logged in", number)
	}

	err := s.subStorage.CheckIfSubIsValid(sub, number)
	if err != nil {
		return err
	}

	err = s.jsonRpc2Clients[number].Stop()
	if err != nil {
		return err
	}

	return nil
}

func (s *SignalClient) GetNumbers(sub string) (*AssignedNumbers, error) {
	numbers, ok := s.subStorage.GetNumbersBySub(sub)
	if !ok {
		return nil, fmt.Errorf("'%s' does not have any numbers linked to them", sub)
	}
	return &AssignedNumbers{Numbers: numbers}, nil
}
