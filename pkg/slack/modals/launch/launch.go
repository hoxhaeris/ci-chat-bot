package launch

import (
	"encoding/json"
	"github.com/openshift/ci-chat-bot/pkg/manager"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/openshift/ci-chat-bot/pkg/slack/interactions"
	"github.com/openshift/ci-chat-bot/pkg/slack/modals"
)

// Identifier is the view identifier for this modal
const Identifier modals.Identifier = "launch"
const Identifier2ndStep modals.Identifier = "launch2ndStep"
const Identifier3rdStep modals.Identifier = "launch3ddStep"

func launchNext(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return interactions.HandlerFunc("launch2", func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           SecondStepView(callback, nil, jobmanager),
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal FirstStepView update submission response.")
			return nil, err
		}
		return response, nil
	})
}

func launchNext2(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return interactions.HandlerFunc("launch2", func(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
		response, err := json.Marshal(&slack.ViewSubmissionResponse{
			ResponseAction: slack.RAUpdate,
			View:           ThirdStepView(callback, nil, jobmanager),
		})
		if err != nil {
			logger.WithError(err).Error("Failed to marshal FirstStepView update submission response.")
			return nil, err
		}
		return response, nil
	})
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return launchNext(updater, jobmanager)
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler2ndStep(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return launchNext2(updater, jobmanager)
}

// processSubmissionHandler files a Jira issue for this form
func processSubmissionHandler3rdStep(updater modals.ViewUpdater, jobmanager manager.JobManager) interactions.Handler {
	return launchNext(updater, jobmanager)
}

// RegisterFirstStep creates a registration entry for the enhancement request form
func RegisterFirstStep(client *slack.Client, jobmanager manager.JobManager) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier, FirstStepView(client, jobmanager)).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: processSubmissionHandler(client, jobmanager),
	})
}

// RegisterSecondStep creates a registration entry for the enhancement request form
func RegisterSecondStep(client *slack.Client, jobmanager manager.JobManager) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier2ndStep, FirstStepView(nil, nil)).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: processSubmissionHandler2ndStep(client, jobmanager),
	})
}

// RegisterThirdStep creates a registration entry for the enhancement request form
func RegisterThirdStep(client *slack.Client, jobmanager manager.JobManager) *modals.FlowWithViewAndFollowUps {
	return modals.ForView(Identifier3rdStep, FirstStepView(nil, nil)).WithFollowUps(map[slack.InteractionType]interactions.Handler{
		slack.InteractionTypeViewSubmission: processSubmissionHandler3rdStep(client, jobmanager),
	})
}
