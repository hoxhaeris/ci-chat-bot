package ask_ai

import (
	"encoding/json"
	"fmt"

	botslack "github.com/openshift/ci-chat-bot/pkg/slack"
	"github.com/openshift/ci-chat-bot/pkg/slack/interactions"
	"github.com/openshift/ci-chat-bot/pkg/slack/modals"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
)

const identifier = "ask_ai"
const title = "Ask AI Assistant"
const questionBlockID = "question_input"

// Register creates the ask_ai modal flow.
func Register(client *slack.Client, aiClient *botslack.AIClient) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(modals.Identifier(identifier), View()).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: process(client, aiClient),
	})
}

// View creates the modal view with a text input for the question.
func View() slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type:   slack.VTModal,
		Title:  &slack.TextBlockObject{Type: slack.PlainTextType, Text: title},
		Submit: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Ask"},
		Close:  &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Cancel"},
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.MarkdownType,
						Text: "Ask the AI assistant about cluster-bot commands, options, workflows, and more.",
					},
				},
				&slack.InputBlock{
					Type:    slack.MBTInput,
					BlockID: questionBlockID,
					Label:   &slack.TextBlockObject{Type: slack.PlainTextType, Text: "Your Question"},
					Element: &slack.PlainTextInputBlockElement{
						Type:        slack.METPlainTextInput,
						ActionID:    questionBlockID,
						Multiline:   true,
						Placeholder: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "e.g., How do I launch a cluster on GCP with FIPS?"},
					},
				},
			},
		},
		PrivateMetadata: modals.CallbackDataToMetadata(modals.CallbackData{}, identifier),
	}
}

// process handles the modal submission.
func process(client *slack.Client, aiClient *botslack.AIClient) interactions.Handler {
	return interactions.HandlerFunc(identifier, func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		// Extract the question from the form
		question := ""
		if values, ok := callback.View.State.Values[questionBlockID]; ok {
			for _, v := range values {
				question = v.Value
			}
		}

		if question == "" {
			return modals.ValidationError(map[string]string{
				questionBlockID: "Please enter a question",
			})
		}

		// Process the question asynchronously
		go func() {
			userID := callback.User.ID

			if aiClient == nil || !aiClient.IsConfigured() {
				_, _, err := client.PostMessage(userID,
					slack.MsgOptionText("The AI assistant is not currently available. Please try `help` for command documentation.", false),
				)
				if err != nil {
					logger.WithError(err).Error("Failed to post AI unavailable message")
				}
				return
			}

			// Post the question in the user's DM to start a thread.
			// Use the returned channel ID (not userID) for all subsequent
			// messages, since Slack resolves the DM channel on first post.
			questionMsg := fmt.Sprintf("*Question:* %s", question)
			channel, parentTS, err := client.PostMessage(userID,
				slack.MsgOptionText(questionMsg, false),
			)
			if err != nil {
				logger.WithError(err).Error("Failed to post AI question from modal")
				return
			}

			aiClient.TrackAIThread(parentTS)

			req := botslack.AskRequest{
				Question:  question,
				UserID:    userID,
				ThreadID:  parentTS,
				ChannelID: channel,
			}

			botslack.PostAISyncResponse(client, aiClient, channel, parentTS, req)
		}()

		// Return a confirmation view immediately
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View: &slack.ModalViewRequest{
				Type:  slack.VTModal,
				Title: &slack.TextBlockObject{Type: slack.PlainTextType, Text: title},
				Close: &slack.TextBlockObject{Type: slack.PlainTextType, Text: "OK"},
				Blocks: slack.Blocks{BlockSet: []slack.Block{
					&slack.SectionBlock{
						Type: slack.MBTSection,
						Text: &slack.TextBlockObject{
							Type: slack.MarkdownType,
							Text: "Your question has been sent! Check your messages for the AI assistant's response.",
						},
					},
				}},
			},
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal ask_ai submission response")
			return nil, err
		}
		return response, nil
	})
}
