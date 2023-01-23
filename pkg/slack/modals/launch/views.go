package launch

import (
	"fmt"
	"github.com/openshift/ci-chat-bot/pkg/manager"
	slackClient "github.com/slack-go/slack"
	"strings"
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
func FirstStepView() slackClient.ModalViewRequest {
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

func SecondStepView(callback *slackClient.InteractionCallback, jobmanager manager.JobManager) slackClient.ModalViewRequest {
	platform := "aws"
	architecture := "amd64"
	metadata := fmt.Sprintf("Architecture: %s; Platform: %s", architecture, platform)
	nightly := "error_undefined"
	ci := "error_undefined"
	if callback != nil {
		for key, value := range callback.View.State.Values {
			switch key {
			case "platform":
				for _, v := range value {
					if v.SelectedOption.Value != "" {
						platform = v.SelectedOption.Value
					}
				}
			case "architecture":
				for _, v := range value {
					if v.SelectedOption.Value != "" {
						architecture = v.SelectedOption.Value
					}
				}
			}
		}
		metadata = fmt.Sprintf("Architecture: %s; Platform: %s", architecture, platform)
		var err error
		_, nightly, _, err = jobmanager.ResolveImageOrVersion("nightly", "", architecture)
		if err != nil {
			nightly = fmt.Sprintf("unable to find a release matching `nightly` for %s", architecture)
		} else {
			nightly = fmt.Sprintf("Latest Nightly: %s", nightly)
		}
	}
	var err error
	_, ci, _, err = jobmanager.ResolveImageOrVersion("ci", "", architecture)
	if err != nil {
		ci = fmt.Sprintf("unable to find a release matching \"ci\" for %s", architecture)
	} else {
		ci = fmt.Sprintf("Latest CI build: %s", ci)
	}

	return slackClient.ModalViewRequest{
		Type:            slackClient.VTModal,
		PrivateMetadata: string(Identifier2ndStep),
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
			&slackClient.SectionBlock{
				Type: slackClient.MBTSection,
				Text: &slackClient.TextBlockObject{
					Type: slackClient.MarkdownType,
					Text: "You can select a version from one of the drop-downs, and/or one or multiple PRs. To specify more that one PR, use a coma separator ",
				},
			},
			&slackClient.DividerBlock{
				Type:    slackClient.MBTDivider,
				BlockID: "divider",
			},
			&slackClient.SectionBlock{
				Type: slackClient.MBTSection,
				Text: &slackClient.TextBlockObject{
					Type: slackClient.MarkdownType,
					Text: "Select *only one of the following:* a Version, Stream name, Major/Minor or a Custom Build:",
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "release_controller_version",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "A version directly from Release-Controller"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select an entry"},
					Options: []*slackClient.OptionBlockObject{
						{Value: "release_controller_version", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "placeholder"}},
					},
				},
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
						{Value: "latest_nightly", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: nightly}},
						{Value: "latest_ci", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: ci}},
					},
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "stream",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Stream Name"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select an entry"},
					Options: []*slackClient.OptionBlockObject{
						{Value: "streams", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "placeholder"}},
					},
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "major_minor",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Major/Minor"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.OptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select an entry"},
					Options: []*slackClient.OptionBlockObject{
						{Value: "major_minor", Text: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "placeholder"}},
					},
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "custom_build",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Custom build:"},
				Element:  &slackClient.PlainTextInputBlockElement{Type: slackClient.METPlainTextInput},
			},
			&slackClient.DividerBlock{
				Type:    slackClient.MBTDivider,
				BlockID: "2nd_divider",
			},
			&slackClient.SectionBlock{
				Type: slackClient.MBTSection,
				Text: &slackClient.TextBlockObject{
					Type: slackClient.MarkdownType,
					Text: "Enter one or more PRs, separated by coma",
				},
			},
			&slackClient.InputBlock{
				Type:     slackClient.MBTInput,
				BlockID:  "pr_s",
				Optional: true,
				Label:    &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Enter one or more PRs:"},
				Element:  &slackClient.PlainTextInputBlockElement{Type: slackClient.METPlainTextInput},
			},
			&slackClient.DividerBlock{
				Type:    slackClient.MBTDivider,
				BlockID: "3rd_divider",
			},
			&slackClient.ContextBlock{
				Type:    slackClient.MBTContext,
				BlockID: "metadata_first_step",
				ContextElements: slackClient.ContextElements{Elements: []slackClient.MixedElement{
					&slackClient.TextBlockObject{
						Type:     slackClient.PlainTextType,
						Text:     metadata,
						Emoji:    false,
						Verbatim: false,
					},
				}},
			},
		}},
	}
}

func ThirdStepView(callback *slackClient.InteractionCallback, jobmanager manager.JobManager) slackClient.ModalViewRequest {
	metaData := ""
	platform := "aws"
	options := buildOptions([]string{"error_undefined"})
	if callback != nil {
		for _, value := range callback.View.Blocks.BlockSet {
			if value.BlockType() == slackClient.MBTContext {
				metadata, ok := value.(*slackClient.ContextBlock)
				if ok {
					text, ok := metadata.ContextElements.Elements[0].(*slackClient.TextBlockObject)
					if ok {
						metaData = text.Text
					}
				}
			}
		}
		metadataSlice := strings.Split(metaData, ";")
		if len(metadataSlice) == 2 {
			platform = strings.Split(metadataSlice[1], ":")[1]
		}
		parameters := manager.SupportedParameters
		for i, parameter := range manager.SupportedParameters {
			for k, envs := range manager.MultistageParameters {
				if k == parameter {
					if !envs.Platforms.Has(platform) {
						parameters[i] = parameters[len(parameters)-1]
						parameters[len(parameters)-1] = ""
						parameters = parameters[:len(parameters)-1]
					}
				}

			}
		}
		options = buildOptions(parameters)
	}
	return slackClient.ModalViewRequest{
		Type:            slackClient.VTModal,
		PrivateMetadata: string(Identifier3rdStep),
		Title:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Launch a Cluster"},
		Close:           &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Cancel"},
		Submit:          &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Submit"},
		Blocks: slackClient.Blocks{BlockSet: []slackClient.Block{
			&slackClient.InputBlock{
				Type:    slackClient.MBTInput,
				BlockID: "launchOptions",
				Label:   &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select one or more parameters for your cluster:"},
				Element: &slackClient.SelectBlockElement{
					Type:        slackClient.MultiOptTypeStatic,
					Placeholder: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Select one or more options..."},
					Options:     options,
				},
			},
		}},
	}
}

func PrepareNextStepView() *slackClient.ModalViewRequest {
	return &slackClient.ModalViewRequest{
		Type:  slackClient.VTModal,
		Title: &slackClient.TextBlockObject{Type: slackClient.PlainTextType, Text: "Launch a Cluster"},
		Blocks: slackClient.Blocks{BlockSet: []slackClient.Block{
			&slackClient.SectionBlock{
				Type: slackClient.MBTSection,
				Text: &slackClient.TextBlockObject{
					Type: slackClient.MarkdownType,
					Text: "Processing the next step, do not close this window...",
				},
			},
		}},
	}
}
