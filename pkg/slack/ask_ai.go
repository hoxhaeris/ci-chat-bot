package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openshift/ci-chat-bot/pkg/manager"
	"github.com/openshift/ci-chat-bot/pkg/slack/parser"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/klog"
)

const (
	// aiFollowupActionPrefix identifies follow-up quick-action buttons. Each
	// button gets a unique action_id ("<prefix>_<n>"); the interaction router
	// matches by prefix and re-asks with the button's value.
	aiFollowupActionPrefix = "ai_followup_action"
	// aiRunCommandActionPrefix identifies "▶ Run" command buttons. Each button's
	// value is the exact command; the interaction router matches by prefix and
	// executes it as if the clicking user had typed it.
	aiRunCommandActionPrefix = "ai_run_command"
	// slackSectionMax leaves headroom under Slack's 3000-char section limit.
	slackSectionMax = 2900
	// slackMaxMsgBlocks leaves headroom under Slack's 50-block-per-message limit.
	slackMaxMsgBlocks = 45
	// aiBadge is the footer appended to every AI answer.
	aiBadge = "✨ _AI-generated — validate commands before running them._"
)

// postFinalAnswer updates the thinking message with the final answer rendered
// as Slack Block Kit (sections + an AI-generated badge + follow-up buttons),
// posting any overflow as additional thread replies.
func postFinalAnswer(client parser.SlackClient, channel, threadTS, messageTS string, resp *AskResponse) {
	messages := renderAIMessages(resp.Answer, resp.Commands, resp.Buttons)
	if len(messages) == 0 {
		return
	}
	_, _, _, updateErr := client.UpdateMessage(channel, messageTS,
		slack.MsgOptionBlocks(messages[0]...),
		slack.MsgOptionText(fallbackText(resp.Answer), false),
	)
	if updateErr != nil {
		klog.Errorf("Failed to update final AI message: %v", updateErr)
	}
	for _, blocks := range messages[1:] {
		_, _, postErr := client.PostMessage(channel,
			slack.MsgOptionBlocks(blocks...),
			slack.MsgOptionText(fallbackText(resp.Answer), false),
			slack.MsgOptionTS(threadTS),
		)
		if postErr != nil {
			klog.Errorf("Failed to post AI response part: %v", postErr)
		}
	}
}

// renderAIMessages splits the answer into one or more Slack messages of Block
// Kit blocks. Run-command buttons, follow-up buttons, and the AI badge (in that
// order) go on the last message.
func renderAIMessages(answer string, commands []AICommand, buttons []AIButton) [][]slack.Block {
	chunks := splitForBlocks(answer, slackSectionMax)
	if len(chunks) == 0 {
		chunks = []string{"_(no response)_"}
	}
	var messages [][]slack.Block
	var current []slack.Block
	for _, chunk := range chunks {
		if len(current) >= slackMaxMsgBlocks {
			messages = append(messages, current)
			current = nil
		}
		current = append(current, slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, chunk, false, false), nil, nil))
	}
	// Primary action row: one-click "▶ Run" buttons for validated commands.
	if els := runCommandElements(commands); len(els) > 0 {
		current = append(current, slack.NewActionBlock("ai_run_commands", els...))
	}
	// Secondary action row: follow-up questions that re-ask the AI.
	if els := followupButtonElements(buttons); len(els) > 0 {
		current = append(current, slack.NewActionBlock("ai_followups", els...))
	}
	current = append(current, slack.NewContextBlock("ai_badge",
		slack.NewTextBlockObject(slack.MarkdownType, aiBadge, false, false)))
	messages = append(messages, current)
	return messages
}

// followupButtonElements builds up to 5 Slack button elements from the
// AI-proposed follow-ups, each with a unique prefixed action_id.
func followupButtonElements(buttons []AIButton) []slack.BlockElement {
	var els []slack.BlockElement
	for _, b := range buttons {
		if len(els) >= 5 { // Slack allows at most 5 buttons; cap on valid output
			break
		}
		label := strings.TrimSpace(b.Label)
		msg := strings.TrimSpace(b.Message)
		if label == "" || msg == "" {
			continue
		}
		if len(label) > 75 {
			label = label[:75]
		}
		if len(msg) > 2000 {
			msg = msg[:2000]
		}
		btn := slack.NewButtonBlockElement(
			fmt.Sprintf("%s_%d", aiFollowupActionPrefix, len(els)), msg,
			slack.NewTextBlockObject(slack.PlainTextType, label, false, false))
		if b.Style == "primary" || b.Style == "danger" {
			btn.Style = slack.Style(b.Style)
		}
		els = append(els, btn)
	}
	return els
}

// runCommandElements builds up to 3 primary "▶ Run" button elements from the
// AI-proposed commands. Each button carries the exact command as its value and
// a unique prefixed action_id; the interaction router executes it on click.
func runCommandElements(commands []AICommand) []slack.BlockElement {
	var els []slack.BlockElement
	for _, c := range commands {
		if len(els) >= 3 { // keep the run row short and unambiguous
			break
		}
		cmd := strings.TrimSpace(c.Command)
		if cmd == "" {
			continue
		}
		label := strings.TrimSpace(c.Label)
		if label == "" {
			label = cmd
		}
		label = "▶ " + label
		if len(label) > 75 {
			label = label[:75]
		}
		if len(cmd) > 2000 {
			cmd = cmd[:2000]
		}
		btn := slack.NewButtonBlockElement(
			fmt.Sprintf("%s_%d", aiRunCommandActionPrefix, len(els)), cmd,
			slack.NewTextBlockObject(slack.PlainTextType, label, false, false))
		btn.Style = slack.StylePrimary // Run is the primary action
		els = append(els, btn)
	}
	return els
}

// fallbackText is a short plain-text fallback for notifications/accessibility
// when a message is delivered as Block Kit.
func fallbackText(answer string) string {
	t := strings.TrimSpace(answer)
	if t == "" {
		return "AI response"
	}
	if len(t) > 300 {
		t = t[:300] + "…"
	}
	return t
}

// splitForBlocks splits mrkdwn text into chunks no longer than maxLen,
// preferring paragraph/line/space boundaries and never cutting inside a
// Slack link (<...>).
func splitForBlocks(msg string, maxLen int) []string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}
	if len(msg) <= maxLen {
		return []string{msg}
	}
	var parts []string
	remaining := msg
	for len(remaining) > maxLen {
		cut := boundaryCut(remaining, maxLen)
		parts = append(parts, strings.TrimRight(remaining[:cut], " \n"))
		remaining = strings.TrimLeft(remaining[cut:], " \n")
	}
	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}
	return parts
}

// boundaryCut returns a cut index <= maxLen at the best paragraph/line/space
// boundary that does not fall inside a Slack link.
func boundaryCut(s string, maxLen int) int {
	window := s[:maxLen]
	candidates := make([]int, 0, 4)
	if i := strings.LastIndex(window, "\n\n"); i > maxLen*6/10 {
		candidates = append(candidates, i+2)
	}
	if i := strings.LastIndex(window, "\n"); i > maxLen*6/10 {
		candidates = append(candidates, i+1)
	}
	if i := strings.LastIndex(window, " "); i > maxLen*7/10 {
		candidates = append(candidates, i+1)
	}
	candidates = append(candidates, maxLen)
	for _, c := range candidates {
		if !insideLink(s, c) {
			return c
		}
	}
	return maxLen
}

// insideLink reports whether index i falls inside an unclosed Slack <...> link.
func insideLink(s string, i int) bool {
	if i > len(s) {
		i = len(s)
	}
	lastOpen := strings.LastIndex(s[:i], "<")
	if lastOpen == -1 {
		return false
	}
	return lastOpen > strings.LastIndex(s[:i], ">")
}

// PostAISyncResponse posts a placeholder message, sends the question to the AI
// service, animates a "still working" ticker on the placeholder while the call
// runs, then replaces it with the final answer. Returns the final answer text.
func PostAISyncResponse(client parser.SlackClient, aiClient *AIClient, channel, threadTS string, req AskRequest) string {
	// Post a placeholder we animate while working, then replace with the answer.
	_, thinkingTS, err := client.PostMessage(channel,
		slack.MsgOptionText(thinkingFrame(0, 0), false),
		slack.MsgOptionTS(threadTS),
	)
	if err != nil {
		klog.Errorf("Failed to post thinking message: %v", err)
		// Can't update in-place, just post the answer directly below
		thinkingTS = ""
	}

	// Reassure the user we're still working while the (blocking) call runs.
	var stopTicker func()
	if thinkingTS != "" {
		stopTicker = startThinkingTicker(client, channel, thinkingTS)
	}

	resp, syncErr := aiClient.Ask(req)
	if stopTicker != nil {
		stopTicker()
	}
	if syncErr != nil {
		klog.Errorf("AI service error: %v", syncErr)
		errMsg := aiErrorMessage(syncErr)
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

	if thinkingTS != "" {
		postFinalAnswer(client, channel, threadTS, thinkingTS, resp)
	} else {
		// Fallback: post as new message(s) if we couldn't create the thinking message
		for _, blocks := range renderAIMessages(resp.Answer, resp.Commands, resp.Buttons) {
			_, _, _ = client.PostMessage(channel,
				slack.MsgOptionBlocks(blocks...),
				slack.MsgOptionText(fallbackText(resp.Answer), false),
				slack.MsgOptionTS(threadTS),
			)
		}
	}
	return resp.Answer
}

// thinkingSpinner animates a small glyph so the placeholder visibly "moves"
// while we wait, mirroring ship-help-bot's progress feel.
var thinkingSpinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// thinkingFrame renders one animation frame: a spinner glyph plus an escalating
// reassurance line chosen by how long we've been waiting.
func thinkingFrame(frame int, elapsed time.Duration) string {
	spin := thinkingSpinner[frame%len(thinkingSpinner)]
	return fmt.Sprintf("%s  _%s_", spin, thinkingMessage(elapsed))
}

// thinkingMessage returns an escalating reassurance line, so a long research
// call reads as "still working" rather than "stuck".
func thinkingMessage(elapsed time.Duration) string {
	switch s := elapsed.Seconds(); {
	case s < 15:
		return "Thinking…"
	case s < 35:
		return "Researching your question…"
	case s < 70:
		return "Still working — digging through the CI knowledge base…"
	case s < 120:
		return "Still here — cross-checking Jira, Slack history, and docs…"
	case s < 200:
		return "Still on it — the research analysis is taking a while, hang tight…"
	default:
		return "Almost there — finalizing and fact-checking the response…"
	}
}

// aiErrorMessage maps a failed AI call to a clear, user-facing explanation.
// A timeout is called out specifically: the deep-research step can outlast the
// request budget, and "try again / narrow it" is the useful next step.
func aiErrorMessage(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "⏳ This one's taking longer than I can wait on here — the deep-research step didn't finish in time. Please try again (a more specific question often helps). If it keeps timing out, let us know in `#forum-ocp-crt`."
	case strings.Contains(err.Error(), "rate limited"):
		return "🚦 The AI assistant is busy right now (rate limited). Please try again in a moment."
	default:
		return "⚠️ I couldn't reach the AI assistant to finish your question. Please try again shortly — if it keeps failing, ask in `#forum-ocp-crt`."
	}
}

// startThinkingTicker animates the placeholder message with escalating
// reassurance while a blocking AI call runs. It returns a stop function that
// halts the animation and waits for the ticker goroutine to finish, so the
// final answer render can't race a late "thinking" update.
func startThinkingTicker(client parser.SlackClient, channel, messageTS string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		const interval = 5 * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		start := time.Now()
		frame := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				frame++
				if _, _, _, err := client.UpdateMessage(channel, messageTS,
					slack.MsgOptionText(thinkingFrame(frame, time.Since(start)), false),
				); err != nil {
					klog.V(2).Infof("Failed to animate AI thinking message: %v", err)
				}
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
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

	// Reply in a thread hung off the user's own "ask ..." message — no need to
	// echo the question back. The message timestamp anchors the AI conversation
	// thread so follow-ups continue the same session.
	threadTS := event.TimeStamp
	aiClient.TrackAIThread(threadTS)

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
			ThreadID:  threadTS,
			ChannelID: channel,
		}
		PostAISyncResponse(client, aiClient, channel, threadTS, req)
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
	var threadContext string

	switch {
	case threadTS == "":
		// Top-level message (not a thread reply) — reply in a thread hung off the
		// user's own message instead of echoing "*Question:* ..." back at them. The
		// user's message timestamp anchors the AI conversation thread, so follow-ups
		// in that thread continue the same session.
		threadTS = event.TimeStamp
		aiClient.TrackAIThread(threadTS)
	case !aiClient.IsAIThread(threadTS):
		// First AI engagement in a thread it didn't start — e.g. the user replies
		// to a cluster-status/failure message the bot posted and asks the AI to
		// analyze it. Those messages aren't in the AI session, so pull the preceding
		// thread messages (error text, log links, etc.) and pass them as context.
		threadContext = fetchThreadContext(client, event.Channel, threadTS, event.TimeStamp)
		aiClient.TrackAIThread(threadTS)
	}

	req := AskRequest{
		Question:  question,
		UserID:    event.User,
		ThreadID:  threadTS,
		Context:   threadContext,
		ChannelID: event.Channel,
	}

	PostAISyncResponse(client, aiClient, event.Channel, threadTS, req)
}

// fetchThreadContext returns the text of the messages in the thread rooted at
// threadTS, excluding the user's current follow-up (excludeTS), so the AI can
// act on their content (error text, log links) when the user follows up on a
// message the bot posted. Returns "" if the client can't read conversation
// history (missing scope) or nothing relevant is found.
func fetchThreadContext(client parser.SlackClient, channel, threadTS, excludeTS string) string {
	fetcher, ok := client.(interface {
		GetConversationReplies(*slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
	})
	if !ok {
		return ""
	}
	msgs, _, _, err := fetcher.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: channel,
		Timestamp: threadTS,
		Limit:     20,
	})
	if err != nil {
		klog.Errorf("Failed to fetch thread context for %s/%s: %v", channel, threadTS, err)
		return ""
	}
	var b strings.Builder
	for i := range msgs {
		m := &msgs[i]
		if m.Timestamp == excludeTS { // skip the user's current follow-up
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		who := "user"
		if m.BotID != "" || m.SubType == "bot_message" {
			who = "cluster-bot"
		}
		fmt.Fprintf(&b, "%s: %s\n", who, text)
	}
	ctx := strings.TrimSpace(b.String())
	const maxContext = 4000
	if len(ctx) > maxContext {
		ctx = ctx[:maxContext] + "\n…(truncated)"
	}
	return ctx
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

// HandleAIFollowUp handles a follow-up quick-action button click: it re-asks
// the AI with the button's message and posts the answer in the same thread.
func HandleAIFollowUp(client parser.SlackClient, aiClient *AIClient, userID, channel, threadTS, question string) {
	if aiClient == nil || !aiClient.IsConfigured() || strings.TrimSpace(question) == "" {
		return
	}
	aiClient.TrackAIThread(threadTS)
	// Echo the chosen follow-up so the thread reads naturally.
	if _, _, err := client.PostMessage(channel,
		slack.MsgOptionText(fmt.Sprintf("*Follow-up:* %s", question), false),
		slack.MsgOptionTS(threadTS),
	); err != nil {
		klog.Errorf("Failed to post AI follow-up prompt: %v", err)
	}
	if !aiClient.acquireSem() {
		return
	}
	defer aiClient.releaseSem()
	req := AskRequest{Question: question, UserID: userID, ThreadID: threadTS, ChannelID: channel}
	PostAISyncResponse(client, aiClient, channel, threadTS, req)
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

// RunCommandFromButton executes a cluster-bot command triggered by an AI "▶ Run"
// button click, as if the clicking user had typed it in a DM, and posts the
// result in the same thread. Access-controlled (private) commands are never run
// this way.
func RunCommandFromButton(client parser.SlackClient, jobManager manager.JobManager, commands []parser.BotCommand, userID, channel, threadTS, commandText string) {
	commandText = strings.TrimSpace(commandText)
	if commandText == "" {
		return
	}
	event := &slackevents.MessageEvent{
		User:            userID,
		Channel:         channel,
		Text:            commandText,
		ThreadTimeStamp: threadTS,
	}
	// Echo what's running so the thread reads naturally and the action is auditable.
	if _, _, err := client.PostMessage(channel,
		slack.MsgOptionText(fmt.Sprintf("▶ Running `%s` (requested by <@%s>)", commandText, userID), false),
		slack.MsgOptionTS(threadTS),
	); err != nil {
		klog.Errorf("Failed to post run-command echo: %v", err)
	}
	for _, command := range commands {
		if command.IsPrivate() {
			continue // never run access-controlled commands (e.g. mce) from a button
		}
		properties, match := command.Match(commandText)
		if !match {
			continue
		}
		if response := command.Execute(client, jobManager, event, properties); response != "" {
			if _, _, err := client.PostMessage(channel,
				slack.MsgOptionText(response, false),
				slack.MsgOptionTS(threadTS),
			); err != nil {
				klog.Errorf("Failed to post run-command result: %v", err)
			}
		}
		return
	}
	if _, _, err := client.PostMessage(channel,
		slack.MsgOptionText(fmt.Sprintf("Couldn't run `%s` — it isn't a recognized command.", commandText), false),
		slack.MsgOptionTS(threadTS),
	); err != nil {
		klog.Errorf("Failed to post run-command fallback: %v", err)
	}
}
