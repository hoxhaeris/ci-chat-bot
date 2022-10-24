package launch

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/openshift/ci-chat-bot/pkg/slack/interactions"
	"github.com/openshift/ci-chat-bot/pkg/slack/modals"
)

// Identifier is the view identifier for this modal
const Identifier modals.Identifier = "launch"
const Identifier2ndStep modals.Identifier = "launch2ndStep"
const Identifier3rdStep modals.Identifier = "launch3ddStep"

// View is the modal view for submitting a new enhancement card to Jira
func View() slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		PrivateMetadata: string(Identifier),
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
		Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Next"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: "launchMode",
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch an OpenShift cluster in a guided way, using a known image, version, or PR"},
				Element: &slack.SelectBlockElement{
					Type:        slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster using a:"},
					Options: []*slack.OptionBlockObject{
						{Value: "launch_with_pr", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "PR"}},
						{Value: "launch_with_imageVersion", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Known Image"}},
						{Value: "launch_with_version", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Version"}},
					},
				},
			},
		}},
	}
}

func launchNext(updater modals.ViewUpdater) interactions.Handler {
	return interactions.HandlerFunc("launch2", func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           launch2ndStepView(callback),
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal View update submission response.")
			return nil, err
		}
		return response, nil
	})
}

// PendingJiraView is a placeholder modal View for the user
// to know we are working on publishing a Jira issue
func launch2ndStepView(callback *slack.InteractionCallback) *slack.ModalViewRequest {
	var selectedAction string
	for _, action := range callback.View.State.Values["launchMode"] {
		selectedAction = action.SelectedOption.Value
	}
	if selectedAction == "launch_with_pr" {
		return &slack.ModalViewRequest{
			Type:            slack.VTModal,
			PrivateMetadata: string(Identifier2ndStep),
			Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster"},
			Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
			Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Next"},
			Blocks: slack.Blocks{BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Launch a Cluster with a custom PR",
					},
				},
				&slack.DividerBlock{
					Type: slack.MBTDivider,
				},
				&slack.InputBlock{
					Type:     slack.MBTInput,
					BlockID:  "pr",
					Optional: false,
					Label:    &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Enter the PR URL:"},
					Element:  &slack.PlainTextInputBlockElement{Type: slack.METPlainTextInput},
				},
			}},
		}
	}
	if selectedAction == "launch_with_imageVersion" {
		return &slack.ModalViewRequest{
			Type:            slack.VTModal,
			PrivateMetadata: string(Identifier2ndStep),
			Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster"},
			Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
			Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Next"},
			Blocks: slack.Blocks{BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Launch a Cluster with a known Image",
					},
				},
				&slack.DividerBlock{
					Type: slack.MBTDivider,
				},
				&slack.InputBlock{
					Type:    slack.MBTInput,
					BlockID: "image",
					Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster Using a known Image"},
					Element: &slack.SelectBlockElement{
						Type:        slack.OptTypeStatic,
						Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select a known Image..."},
						Options: []*slack.OptionBlockObject{
							{Value: "4.12.0-0.nightly-2022-10-20-104328", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "4.12.0-0.nightly-2022-10-20-104328"}},
							{Value: "4.12.0-0.nightly-2022-10-22-104328", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "4.12.0-0.nightly-2022-10-22-104328"}},
							{Value: "4.12.0-0.nightly-2022-10-23-104328", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "4.12.0-0.nightly-2022-10-23-104328"}},
						},
					},
				},
			}},
		}
	}
	return &slack.ModalViewRequest{
		Type:            slack.VTModal,
		PrivateMetadata: string(Identifier2ndStep),
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
		Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Next"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.SectionBlock{
				Type: slack.MBTSection,
				Text: &slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: "Launch a Cluster with Version",
				},
			},
			&slack.DividerBlock{
				Type: slack.MBTDivider,
			},
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: "version",
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster using a Version"},
				Element: &slack.SelectBlockElement{
					Type:        slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select a Version..."},
					Options: []*slack.OptionBlockObject{
						{Value: "4.12", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "4.12"}},
						{Value: "4.11", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "4.11"}},
						{Value: "4.10", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "4.10"}},
					},
				},
			},
		}},
	}
}

func launchNext2(updater modals.ViewUpdater) interactions.Handler {
	return interactions.HandlerFunc("launch2", func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           launch3rdStepView(callback),
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal View update submission response.")
			return nil, err
		}
		return response, nil
	})
}

func launch3rdStepView(callback *slack.InteractionCallback) *slack.ModalViewRequest {
	return &slack.ModalViewRequest{
		Type:            slack.VTModal,
		PrivateMetadata: string(Identifier3rdStep),
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
		Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Submit"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: "launchOptions",
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select the platform for your cluster:"},
				Element: &slack.SelectBlockElement{
					Type:        slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select a platform..."},
					Options: []*slack.OptionBlockObject{
						{Value: "aws", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "aws"}},
						{Value: "gcp", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "gcp"}},
						{Value: "azure", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "azure"}},
					},
				},
			},
			&slack.InputBlock{
				Type:    slack.MBTInput,
				BlockID: "launchOptions2",
				Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select the variant for your cluster:"},
				Element: &slack.SelectBlockElement{
					Type:        slack.MultiOptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Select one or more options..."},
					Options: []*slack.OptionBlockObject{
						{Value: "ovn", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "ovn"}},
						{Value: "ovn-hybrid", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "ovn-hybrid"}},
						{Value: "proxy", Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "proxy"}},
					},
				},
			},
		}},
	}
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler(updater modals.ViewUpdater) interactions.Handler {
	return launchNext(updater)
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler2ndStep(updater modals.ViewUpdater) interactions.Handler {
	return launchNext2(updater)
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler3rdStep(updater modals.ViewUpdater) interactions.Handler {
	return launchNext(updater)
}

// Register creates a registration entry for the enhancement request form
func Register(client *slack.Client) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier, View()).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: processSubmissionHandler(client),
	})
}

// Register2ndStep creates a registration entry for the enhancement request form
func Register2ndStep(client *slack.Client) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier2ndStep, View()).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: processSubmissionHandler2ndStep(client),
	})
}

// Register3rdStep creates a registration entry for the enhancement request form
func Register3rdStep(client *slack.Client) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier3rdStep, View()).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: processSubmissionHandler3rdStep(client),
	})
}
