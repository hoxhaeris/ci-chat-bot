package launch

import (
	"fmt"
	"github.com/openshift/ci-chat-bot/pkg/manager"
	slackClient "github.com/slack-go/slack"
)

func buildOptions(options []string) []*slackClient.OptionBlockObject {
	var slackOptions []*slackClient.OptionBlockObject
	for _, platform := range options {
		slackOptions = append(slackOptions, &slackClient.OptionBlockObject{
			Value: platform,
			Text: &slackClient.TextBlockObject{
				Type: slackClient.PlainTextType,
				Text: platform,
			},
		})
	}
	return slackOptions
}

// FirstStepView is the modal view for submitting a new enhancement card to Jira
func FirstStepView(client *slackClient.Client, jobmanager manager.JobManager) slackClient.ModalViewRequest {
	platformOptions := buildOptions(manager.SupportedPlatforms)
	architectureOptions := buildOptions(manager.SupportedArchitectures)
	return slackClient.ModalViewRequest{
		Type:            slackClient.VTModal,
		PrivateMetadata: string(Identifier),
		Title:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Cancel"},
		Submit:          &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Next"},
		Blocks: slackClient.Blocks{BlockSet: []slackClient.Block{
			&slackClient.HeaderBlock{
				Type: slackClient.MBTHeader,
				Text: &slackClient.TextBlockObject{
					Type:     "plain_text",
					Text:     "Select the Launch Platform and Architecture",
					Emoji:    false,
					Verbatim: false,
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "platform",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Platform (Default - aws)"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "AWS"},
					Options:     platformOptions,
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "architecture",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Architecture (Default - amd64)"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "amd64"},
					Options:     architectureOptions,
				},
			},
		}},
	}
}

func SecondStepView(callback *slackClient.InteractionCallback, client *slackClient.Client, jobmanager manager.JobManager) *slackClient.ModalViewRequest {
	platform := "aws"
	architecture := "amd64"
	for key, value := range callback.View.State.Values {
		switch key {
		case "platform":
			for _, v := range value {
				platform = v.SelectedOption.Value
			}
		case "architecture":
			for _, v := range value {
				platform = v.SelectedOption.Value
			}
		}
	}
	fmt.Println(platform)
	_, nightly, _, _ := jobmanager.ResolveImageOrVersion("nightly", "", architecture)
	_, ci, _, _ := jobmanager.ResolveImageOrVersion("ci", "", architecture)
	return &slackClient.ModalViewRequest{
		Type:            slackClient.VTModal,
		PrivateMetadata: string(Identifier),
		Title:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Cancel"},
		Submit:          &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Next"},
		Blocks: slackClient.Blocks{BlockSet: []slackClient.Block{
			&slackClient.HeaderBlock{
				Type: slackClient.MBTHeader,
				Text: &slackClient.TextBlockObject{
					Type:     "plain_text",
					Text:     "Select a Version or enter a PR",
					Emoji:    false,
					Verbatim: false,
				},
			},
			&slackClient.HeaderBlock{
				Type: slackClient.MBTHeader,
				Text: &slackClient.TextBlockObject{
					Type:     "plain_text",
					Text:     "Select a Version, Stream name, Major/Minor or a Custom Build",
					Emoji:    false,
					Verbatim: false,
				},
			},
			&slackClient.DividerBlock{
				Type:    slackClient.MBTDivider,
				BlockID: "1st_divider",
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "latest_build",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Latest build (nightly) or CI build (CI)"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select an entry"},
					Options: []*slackClient.OptionBlockObject{
						{Value: "latest_nightly", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: fmt.Sprintf("Latest OCP Build (nightly): %s", nightly)}},
						{Value: "latest_ci", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: fmt.Sprintf("Latest CI build: %s", ci)}},
					},
				},
			},
		}},
	}
}

func ThirdStepView(callback *slackClient.InteractionCallback, client *slackClient.Client, jobmanager manager.JobManager) *slackClient.ModalViewRequest {
	return &slackClient.ModalViewRequest{
		Type:            slackClient.VTModal,
		PrivateMetadata: string(Identifier3rdStep),
		Title:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Cancel"},
		Submit:          &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Submit"},
		Blocks: slackClient.Blocks{BlockSet: []slackClient.Block{
			&slackClient.InputBlock{
				Type:    slackClient.MBTInput,
				BlockID: "launchOptions",
				Label:   &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select the platform for your cluster:"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select a platform..."},
					Options: []*slackClient.OptionBlockObject{
						{Value: "aws", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "aws"}},
						{Value: "gcp", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "gcp"}},
						{Value: "azure", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "azure"}},
					},
				},
			},
			&slackClient.InputBlock{
				Type:    slackClient.MBTInput,
				BlockID: "launchOptions2",
				Label:   &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select the variant for your cluster:"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.MultiOptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select one or more options..."},
					Options: []*slackClient.OptionBlockObject{
						{Value: "ovn", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "ovn"}},
						{Value: "ovn-hybrid", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "ovn-hybrid"}},
						{Value: "proxy", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "proxy"}},
					},
				},
			},
		}},
	}
}
