package launch

import (
	"github.com/openshift/ci-chat-bot/pkg/manager"
	"github.com/slack-go/slack"
)

func buildOptions(options []string) []*slack.OptionBlockObject {
	var slackOptions []*slack.OptionBlockObject
	for _, platform := range options {
		slackOptions = append(slackOptions, &slack.OptionBlockObject{
			Value: platform,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: platform,
			},
		})
	}
	return slackOptions
}

// FirstStep is the modal view for submitting a new enhancement card to Jira
func FirstStep() slack.ModalViewRequest {
	platformOptions := buildOptions(manager.SupportedPlatforms)
	architectureOptions := buildOptions(manager.SupportedArchitectures)
	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		PrivateMetadata: string(Identifier),
		Title:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
		Submit:          &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Next"},
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			&slack.HeaderBlock{
				Type: slack.MBTHeader,
				Text: &slack.TextBlockObject{
					Type:     "plain_text",
					Text:     "Select the Launch Platform and Architecture",
					Emoji:    false,
					Verbatim: false,
				},
			},
			&slack.InputBlock{
				Type:     slack.MBTInput,
				BlockID:  "platform",
				Optional: true,
				Label:    &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Platform (Default - aws)"},
				Element: &slack.SelectBlockElement{
					Type:        slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "AWS"},
					Options:     platformOptions,
				},
			},
			&slack.InputBlock{
				Type:     slack.MBTInput,
				BlockID:  "architecture",
				Optional: true,
				Label:    &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Architecture (Default - amd64)"},
				Element: &slack.SelectBlockElement{
					Type:        slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "amd64"},
					Options:     architectureOptions,
				},
			},
		}},
	}
}

func SecondStep(callback *slack.InteractionCallback) *slack.ModalViewRequest {
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

func ThirdStep(callback *slack.InteractionCallback) *slack.ModalViewRequest {
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
