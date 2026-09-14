package modals

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"maps"

	"github.com/openshift/ci-chat-bot/pkg/manager"
	"github.com/openshift/ci-chat-bot/pkg/slack/interactions"
)

const (
	IdentifierError Identifier = "error"
)

// ViewUpdater is a subset of the Slack client
type ViewUpdater interface {
	UpdateView(view slack.ModalViewRequest, externalID, hash, viewID string) (*slack.ViewResponse, error)
}

// UpdateViewForButtonPress updates the given View if the interaction
// being handled was the identified button being pushed
func UpdateViewForButtonPress(identifier, buttonID string, updater ViewUpdater, view slack.ModalViewRequest) interactions.PartialHandler {
	return interactions.PartialHandlerFunc(identifier, func(callback *slack.InteractionCallback, logger *logrus.Entry) (bool, []byte, error) {
		// if someone pushed the identified button, show them that form
		if len(callback.ActionCallback.BlockActions) > 0 {
			action := callback.ActionCallback.BlockActions[0]
			if action.Type == "button" && action.Value == buttonID {
				logger.Debugf("The %s button was pressed, updating the View for handler %s", buttonID, identifier)
				response, err := updater.UpdateView(view, "", callback.View.Hash, callback.View.ID)
				if err != nil {
					logger.WithError(err).Warn("Failed to update a modal View.")
				}
				logger.WithField("response", response).Trace("Got a modal response.")
				return true, nil, err
			}
		}
		return false, nil, nil
	})
}

func SubmitPrepare(title, modalName string, logger *logrus.Entry) ([]byte, error) {
	response, err := json.Marshal(&slack.ViewSubmissionResponse{
		ResponseAction: slack.RAUpdate,
		View:           PrepareNextStepView(title),
	})
	if err != nil {
		logger.WithError(err).Errorf("Failed to marshal %s update submission response.", modalName)
		return nil, err
	}
	return response, err
}

func PrepareNextStepView(title string) *slack.ModalViewRequest {
	return &slack.ModalViewRequest{
		Type:  slack.VTModal,
		Title: &slack.TextBlockObject{Type: slack.PlainTextType, Text: title},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: "Processing the next step, do not close this window...",
				},
			},
		}},
	}
}

// ErrorView is a modal View to show the user an error
func ErrorView(action string, err error) slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		PrivateMetadata: string(IdentifierError),
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Error Occurred"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "OK"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: fmt.Sprintf("We encountered an error trying to %s:\n>%v", action, err),
				},
			},
		}},
	}
}

func SubmissionView(title, msg string) slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type:  slack.VTModal,
		Title: &slack.TextBlockObject{Type: slack.PlainTextType, Text: title},
		Close: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Close"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: msg,
				},
			},
		}},
	}
}

func OverwriteView(client ViewUpdater, view slack.ModalViewRequest, callback *slack.InteractionCallback, logger *logrus.Entry) {
	// don't pass a hash, so we overwrite the View always
	response, err := client.UpdateView(view, "", "", callback.View.ID)
	if err != nil {
		logger.WithError(err).WithField("messages", response.ResponseMetadata.Messages).Warn("Failed to update a modal View.")
		_, err := client.UpdateView(ErrorView("updating the modal view", err), "", "", callback.View.ID)
		if err != nil {
			logger.WithError(err).Warn("failed to update a modal View.")
		}
	}
	logger.WithField("response", response).Trace("Got a modal response.")
}

func CallbackSelection(callback *slack.InteractionCallback) map[string]string {
	selectionValues := make(map[string]string)
	for key, value := range callback.View.State.Values {
		for _, v := range value {
			if v.SelectedOption.Value != "" {
				selectionValues[key] = v.SelectedOption.Text.Text
			}
			if v.SelectedUser != "" {
				selectionValues[key] = v.SelectedUser
			}
		}
	}
	return selectionValues
}

func CallbackInput(callback *slack.InteractionCallback) map[string]string {
	inputValues := make(map[string]string)
	for key, value := range callback.View.State.Values {
		for _, v := range value {
			if v.Value != "" {
				inputValues[key] = v.Value
			}
		}
	}
	return inputValues
}

func CallbackMultipleSelect(callback *slack.InteractionCallback) map[string][]string {
	selectedValues := make(map[string][]string)
	for key, value := range callback.View.State.Values {
		var selections []string
		for _, v := range value {
			if len(v.SelectedOptions) > 0 {
				for _, selection := range v.SelectedOptions {
					selections = append(selections, selection.Value)
				}
				selectedValues[key] = selections
			}
		}
	}
	return selectedValues
}

func CallBackInputAll(callback *slack.InteractionCallback) map[string]string {
	merged := make(map[string]string, 0)
	selectionValues := CallbackSelection(callback)
	inputValues := CallbackInput(callback)
	maps.Copy(merged, selectionValues)
	maps.Copy(merged, inputValues)
	return merged
}

func ValidationError(errors map[string]string) ([]byte, error) {
	response, err := json.Marshal(&slack.ViewSubmissionResponse{
		ResponseAction: slack.RAErrors,
		Errors:         errors,
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func BuildOptions(options []string, excludeList sets.Set[string]) []*slack.OptionBlockObject {
	slackOptions := make([]*slack.OptionBlockObject, 0)
	for _, parameter := range options {
		if !excludeList.Has(parameter) {
			slackOptions = append(slackOptions, &slack.OptionBlockObject{
				Value: parameter,
				Text: &slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: parameter,
				},
			})
		}
	}
	return slackOptions
}

var launchTypes = []string{LaunchVersion, LaunchFromMajorMinor, LaunchFromStream, LaunchFromReleaseController, LaunchFromCustom}

func GetVersion(data CallbackData, jobmanager manager.JobManager) string {
	var version string
	var ok bool
	for _, launchType := range launchTypes {
		version, ok = data.Input[launchType]
		if ok {
			break
		}
	}
	return version
}

func MergeCallbackData(callback *slack.InteractionCallback) CallbackData {
	merged := CallbackData{}
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &merged); err != nil {
		klog.Errorf("Failed to unmarshal private metadata: %v", err)
	}
	if merged.Input == nil {
		merged.Input = make(map[string]string)
	}
	maps.Copy(merged.Input, CallBackInputAll(callback))
	if merged.MultipleSelection == nil {
		merged.MultipleSelection = make(map[string][]string)
	}
	maps.Copy(merged.MultipleSelection, CallbackMultipleSelect(callback))
	return merged
}

func CallbackDataToMetadata(data CallbackData, identifier string) string {
	dataWithIdentifier := CallbackDataAndIdentifier{
		data,
		identifier,
	}
	privateMetadata, err := json.Marshal(dataWithIdentifier)
	if err != nil {
		klog.Errorf("Failed to marshal callback data: %v", err)
	}
	return string(privateMetadata)
}

// CallbackData contains only the user-generated input portion of slack.InteractionCallback
type CallbackData struct {
	Input             map[string]string
	MultipleSelection map[string][]string
	PreviousStep      string // Tracks the previous step identifier for back navigation
}

type CallbackDataAndIdentifier struct {
	CallbackData
	Identifier string
}

// SetPreviousStep returns a new CallbackData with the PreviousStep set to the current identifier
func SetPreviousStep(data CallbackData, currentStep string) CallbackData {
	data.PreviousStep = currentStep
	return data
}

// BackButtonBlock creates an actions block with a back button on the left
func BackButtonBlock() *slack.ActionBlock {
	return &slack.ActionBlock{
		Type:    slack.MBTAction,
		BlockID: BackButtonBlockID,
		Elements: &slack.BlockElements{
			ElementSet: []slack.BlockElement{
				&slack.ButtonBlockElement{
					Type:     slack.METButton,
					ActionID: BackButtonActionID,
					Text: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "←",
					},
				},
			},
		},
	}
}
