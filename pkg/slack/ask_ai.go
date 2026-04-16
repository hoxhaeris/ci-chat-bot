package slack

import (
	"fmt"
	"strings"

	"github.com/openshift/ci-chat-bot/pkg/slack/parser"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/klog"
)

// postFinalAnswer updates the thinking message with the final answer text,
// splitting into multiple thread messages if the answer exceeds Slack's limit.
func postFinalAnswer(client parser.SlackClient, channel, threadTS, messageTS, answer string) {
	if len(answer) > 3500 {
		parts := splitMessage(answer, 3500)
		// Update the thinking message with the first part
		_, _, _, updateErr := client.UpdateMessage(channel, messageTS,
			slack.MsgOptionText(parts[0], false),
		)
		if updateErr != nil {
			klog.Errorf("Failed to update final message: %v", updateErr)
		}
		// Post remaining parts as new messages in the thread
		for _, part := range parts[1:] {
			_, _, postErr := client.PostMessage(channel,
				slack.MsgOptionText(part, false),
				slack.MsgOptionTS(threadTS),
			)
			if postErr != nil {
				klog.Errorf("Failed to post AI response part: %v", postErr)
			}
		}
	} else {
		_, _, _, updateErr := client.UpdateMessage(channel, messageTS,
			slack.MsgOptionText(answer, false),
		)
		if updateErr != nil {
			klog.Errorf("Failed to update final message: %v", updateErr)
		}
	}
}

// PostAISyncResponse posts a "thinking" message, sends the question via synchronous
// Ask(), and updates the message with the response. Returns the final answer text.
func PostAISyncResponse(client parser.SlackClient, aiClient *AIClient, channel, threadTS string, req AskRequest) string {
	// Post a visible status message so the user knows we're working
	_, thinkingTS, err := client.PostMessage(channel,
		slack.MsgOptionText("_Processing your question..._", false),
		slack.MsgOptionTS(threadTS),
	)
	if err != nil {
		klog.Errorf("Failed to post thinking message: %v", err)
		// Can't update in-place, just post the answer directly below
		thinkingTS = ""
	}

	resp, syncErr := aiClient.Ask(req)
	if syncErr != nil {
		klog.Errorf("AI service error: %v", syncErr)
		errMsg := "I encountered an error processing your question. Please try again or use `help` for documentation."
		if thinkingTS != "" {
			_, _, _, _ = client.UpdateMessage(channel, thinkingTS,
				slack.MsgOptionText(errMsg, false),
			)
		} else {
			_, _, _ = client.PostMessage(channel,
				slack.MsgOptionText(errMsg, false),
				slack.MsgOptionTS(threadTS),
			)
		}
		return ""
	}

	answer := resp.Answer
	if thinkingTS != "" {
		postFinalAnswer(client, channel, threadTS, thinkingTS, answer)
	} else {
		// Fallback: post as new message(s) if we couldn't create the thinking message
		if len(answer) > 3500 {
			for _, part := range splitMessage(answer, 3500) {
				_, _, _ = client.PostMessage(channel,
					slack.MsgOptionText(part, false),
					slack.MsgOptionTS(threadTS),
				)
			}
		} else {
			_, _, _ = client.PostMessage(channel,
				slack.MsgOptionText(answer, false),
				slack.MsgOptionTS(threadTS),
			)
		}
	}
	return answer
}

// HandleAskAI handles the "ask" command by forwarding the question to the AI service
// and posting the response as a thread reply. Dispatches the AI call asynchronously.
func HandleAskAI(client parser.SlackClient, aiClient *AIClient, event *slackevents.MessageEvent, properties *parser.Properties) string {
	if aiClient == nil || !aiClient.IsConfigured() {
		return "The AI assistant is not currently available. Please try `help` for command documentation."
	}

	question := properties.StringParam("question", "")
	if question == "" {
		return "Please provide a question. Example: `ask how do I launch a cluster on GCP?`"
	}

	// Post the question as the start of a thread, then reply with the AI answer
	questionMsg := fmt.Sprintf("*Question:* %s", question)
	_, parentTS, err := client.PostMessage(event.Channel, slack.MsgOptionText(questionMsg, false))
	if err != nil {
		klog.Errorf("Failed to post AI question: %v", err)
		return "Failed to start AI conversation. Please try again."
	}

	// Track this thread as an AI conversation
	aiClient.TrackAIThread(parentTS)

	// Dispatch AI call asynchronously — the response will be posted in the thread
	channel := event.Channel
	userID := event.User
	go func() {
		if !aiClient.acquireSem() {
			return
		}
		defer aiClient.releaseSem()
		req := AskRequest{
			Question:  question,
			UserID:    userID,
			ThreadID:  parentTS,
			ChannelID: channel,
		}
		PostAISyncResponse(client, aiClient, channel, parentTS, req)
	}()

	// Return empty string since the response will be posted asynchronously in the thread
	return ""
}

// HandleAIThreadFollowUp handles a follow-up message in an existing AI thread,
// or routes an unmatched top-level message to the AI assistant as a new conversation.
func HandleAIThreadFollowUp(client parser.SlackClient, aiClient *AIClient, event *slackevents.MessageEvent) {
	if aiClient == nil || !aiClient.IsConfigured() {
		return
	}

	question := strings.TrimSpace(event.Text)
	if question == "" {
		return
	}

	threadTS := event.ThreadTimeStamp

	// Top-level message (not a thread reply) — start a new AI conversation thread
	if threadTS == "" {
		questionMsg := fmt.Sprintf("*Question:* %s", question)
		_, parentTS, err := client.PostMessage(event.Channel, slack.MsgOptionText(questionMsg, false))
		if err != nil {
			klog.Errorf("Failed to post AI question: %v", err)
			return
		}
		aiClient.TrackAIThread(parentTS)
		threadTS = parentTS
	}

	req := AskRequest{
		Question:  question,
		UserID:    event.User,
		ThreadID:  threadTS,
		ChannelID: event.Channel,
	}

	PostAISyncResponse(client, aiClient, event.Channel, threadTS, req)
}

// HandleAIErrorHelp handles the "Ask AI for help" button click from error messages.
// It posts the error context as a question to the AI service and starts a threaded conversation.
func HandleAIErrorHelp(client parser.SlackClient, aiClient *AIClient, userID, channel, errorContext string) {
	if aiClient == nil || !aiClient.IsConfigured() {
		_, _, err := client.PostMessage(channel,
			slack.MsgOptionText("The AI assistant is not currently available. Please try `help` for command documentation.", false),
		)
		if err != nil {
			klog.Errorf("Failed to post AI unavailable message: %v", err)
		}
		return
	}

	// Post the initial message to start a thread
	questionMsg := "*AI Help Request:* A command encountered an error. Let me help you understand what went wrong and how to fix it."
	_, parentTS, err := client.PostMessage(channel, slack.MsgOptionText(questionMsg, false))
	if err != nil {
		klog.Errorf("Failed to post AI error help: %v", err)
		return
	}

	// Track this thread as an AI conversation
	aiClient.TrackAIThread(parentTS)

	req := AskRequest{
		Question:  "A user's command failed. Please help them understand the error and suggest how to fix it.",
		UserID:    userID,
		ThreadID:  parentTS,
		Context:   errorContext,
		ChannelID: channel,
	}

	PostAISyncResponse(client, aiClient, channel, parentTS, req)
}

// handleErrorSuggestion posts an AI-generated suggestion as a threaded reply
// to a command error message. The error message has already been posted at parentTS.
// Called by AIClient.HandleErrorSuggestion which handles concurrency limiting.
func handleErrorSuggestion(c *AIClient, client parser.SlackClient, channel, parentTS, userCommand, errorMessage string) {
	if c == nil || !c.IsConfigured() {
		return
	}

	// Track this thread so follow-ups work
	c.TrackAIThread(parentTS)

	req := AskRequest{
		Question:  "A user's command had an error. Analyze the error and suggest the correct command. Be concise — just give the fix.",
		ThreadID:  parentTS,
		Context:   fmt.Sprintf("Command: %s\nError: %s", userCommand, errorMessage),
		ChannelID: channel,
	}

	PostAISyncResponse(client, c, channel, parentTS, req)
}

// splitMessage splits a long message into parts at safe boundaries.
func splitMessage(msg string, maxLen int) []string {
	if len(msg) <= maxLen {
		return []string{msg}
	}

	var parts []string
	remaining := msg

	for len(remaining) > maxLen {
		// Try to split at paragraph break
		cutPoint := maxLen
		if idx := strings.LastIndex(remaining[:maxLen], "\n\n"); idx > maxLen*7/10 {
			cutPoint = idx + 2
		} else if idx := strings.LastIndex(remaining[:maxLen], "\n"); idx > maxLen*7/10 {
			cutPoint = idx + 1
		} else if idx := strings.LastIndex(remaining[:maxLen], " "); idx > maxLen*8/10 {
			cutPoint = idx + 1
		}

		parts = append(parts, remaining[:cutPoint])
		remaining = remaining[cutPoint:]
	}

	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}

	return parts
}
